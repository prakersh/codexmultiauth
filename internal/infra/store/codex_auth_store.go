package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/BurntSushi/toml"

	"github.com/99designs/keyring"

	"github.com/prakersh/codexmultiauth/internal/domain"
	cmafs "github.com/prakersh/codexmultiauth/internal/infra/fs"
	"github.com/prakersh/codexmultiauth/internal/infra/paths"
)

const (
	CodexAuthKeyringService = "Codex Auth"
	CodexAuthKeyringAccount = "default"
)

type CodexAuth struct {
	AuthMode     string       `json:"auth_mode,omitempty"`
	OpenAIAPIKey string       `json:"OPENAI_API_KEY,omitempty"`
	Tokens       *CodexTokens `json:"tokens,omitempty"`
	LastRefresh  *time.Time   `json:"last_refresh,omitempty"`
}

type CodexTokens struct {
	IDToken      string `json:"id_token,omitempty"`
	AccessToken  string `json:"access_token,omitempty"`
	RefreshToken string `json:"refresh_token,omitempty"`
	AccountID    string `json:"account_id,omitempty"`
}

type AuthRecord struct {
	Raw         []byte
	Canonical   []byte
	Fingerprint string
	StoreKind   domain.AuthStoreKind
	Parsed      CodexAuth
}

type codexConfig struct {
	CredentialsStore string `toml:"cli_auth_credentials_store"`
}

type CodexAuthStore struct {
	paths      paths.Paths
	keyring    KeyringClient
	configRepo *ConfigRepo
}

func NewCodexAuthStore(p paths.Paths, keyringClient KeyringClient, configRepo *ConfigRepo) *CodexAuthStore {
	return &CodexAuthStore{
		paths:      p,
		keyring:    keyringClient,
		configRepo: configRepo,
	}
}

func NormalizeAndValidateAuth(raw []byte) (CodexAuth, []byte, error) {
	var auth CodexAuth
	if err := json.Unmarshal(raw, &auth); err != nil {
		return CodexAuth{}, nil, fmt.Errorf("parse auth payload: %w", err)
	}
	auth.AuthMode = strings.TrimSpace(auth.AuthMode)
	auth.OpenAIAPIKey = strings.TrimSpace(auth.OpenAIAPIKey)
	if auth.Tokens != nil {
		auth.Tokens.IDToken = strings.TrimSpace(auth.Tokens.IDToken)
		auth.Tokens.AccessToken = strings.TrimSpace(auth.Tokens.AccessToken)
		auth.Tokens.RefreshToken = strings.TrimSpace(auth.Tokens.RefreshToken)
		auth.Tokens.AccountID = strings.TrimSpace(auth.Tokens.AccountID)
	}

	if auth.OpenAIAPIKey == "" && (auth.Tokens == nil || auth.Tokens.AccessToken == "") {
		return CodexAuth{}, nil, errors.New("auth payload missing usable credentials")
	}
	canonical, err := json.Marshal(auth)
	if err != nil {
		return CodexAuth{}, nil, fmt.Errorf("canonicalize auth payload: %w", err)
	}
	return auth, canonical, nil
}

func FingerprintAuth(canonical []byte) string {
	sum := sha256.Sum256(canonical)
	return hex.EncodeToString(sum[:])
}

func (s *CodexAuthStore) Load(ctx context.Context) (AuthRecord, error) {
	_ = ctx
	if record, err := s.loadFileRecord(); err == nil {
		return record, nil
	}
	if s.shouldAttemptKeyring() {
		if record, err := s.loadKeyringRecord(); err == nil {
			return record, nil
		}
	}
	return AuthRecord{}, os.ErrNotExist
}

func (s *CodexAuthStore) Save(ctx context.Context, raw []byte) error {
	_ = ctx
	_, canonical, err := NormalizeAndValidateAuth(raw)
	if err != nil {
		return err
	}

	if err := cmafs.WriteFileAtomic(s.paths.CodexAuth, canonical, cmafs.AtomicWriteOptions{
		Mode: cmafs.FileMode,
		Verify: func(path string) error {
			written, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			_, normalized, err := NormalizeAndValidateAuth(written)
			if err != nil {
				return err
			}
			if FingerprintAuth(normalized) != FingerprintAuth(canonical) {
				return errors.New("fingerprint mismatch after write")
			}
			return nil
		},
	}); err != nil {
		return err
	}

	if s.shouldAttemptKeyring() {
		if err := s.keyring.Set(CodexAuthKeyringService, CodexAuthKeyringAccount, canonical); err != nil {
			return fmt.Errorf("save auth to keyring: %w", err)
		}
	}
	return nil
}

func (s *CodexAuthStore) Delete(ctx context.Context) error {
	_ = ctx
	if err := os.Remove(s.paths.CodexAuth); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("delete auth file: %w", err)
	}
	if s.shouldAttemptKeyring() {
		if err := s.keyring.Delete(CodexAuthKeyringService, CodexAuthKeyringAccount); err != nil && !errors.Is(err, keyring.ErrKeyNotFound) {
			return fmt.Errorf("delete auth keyring entry: %w", err)
		}
	}
	return nil
}

func (s *CodexAuthStore) loadFileRecord() (AuthRecord, error) {
	raw, err := os.ReadFile(s.paths.CodexAuth)
	if err != nil {
		return AuthRecord{}, err
	}
	auth, canonical, err := NormalizeAndValidateAuth(raw)
	if err != nil {
		return AuthRecord{}, err
	}
	return AuthRecord{
		Raw:         raw,
		Canonical:   canonical,
		Fingerprint: FingerprintAuth(canonical),
		StoreKind:   domain.AuthStoreFile,
		Parsed:      auth,
	}, nil
}

func (s *CodexAuthStore) loadKeyringRecord() (AuthRecord, error) {
	if s.keyring == nil {
		return AuthRecord{}, os.ErrNotExist
	}
	raw, err := s.keyring.Get(CodexAuthKeyringService, CodexAuthKeyringAccount)
	if err != nil {
		return AuthRecord{}, err
	}
	auth, canonical, err := NormalizeAndValidateAuth(raw)
	if err != nil {
		return AuthRecord{}, err
	}
	return AuthRecord{
		Raw:         raw,
		Canonical:   canonical,
		Fingerprint: FingerprintAuth(canonical),
		StoreKind:   domain.AuthStoreKeyring,
		Parsed:      auth,
	}, nil
}

func (s *CodexAuthStore) shouldAttemptKeyring() bool {
	if s.keyring == nil {
		return false
	}
	if s.configRepo != nil {
		cfg, err := s.configRepo.Load()
		if err == nil && cfg.DisableKeyring {
			return false
		}
	}
	codexCfg, err := s.loadCodexConfig()
	if err != nil {
		return true
	}
	switch strings.ToLower(strings.TrimSpace(codexCfg.CredentialsStore)) {
	case "", "auto":
		return true
	case "keyring":
		return true
	case "file":
		return false
	default:
		return true
	}
}

func (s *CodexAuthStore) loadCodexConfig() (codexConfig, error) {
	data, err := os.ReadFile(filepathJoin(s.paths.CodexHome, "config.toml"))
	if err != nil {
		return codexConfig{}, err
	}
	var cfg codexConfig
	if err := toml.Unmarshal(data, &cfg); err != nil {
		return codexConfig{}, err
	}
	return cfg, nil
}

func filepathJoin(elem ...string) string {
	if len(elem) == 0 {
		return ""
	}
	path := elem[0]
	for _, part := range elem[1:] {
		path = strings.TrimRight(path, "/") + "/" + strings.TrimLeft(part, "/")
	}
	return path
}
