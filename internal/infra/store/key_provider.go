package store

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/99designs/keyring"

	cmacrypto "github.com/prakersh/codexmultiauth/internal/infra/crypto"
	cmafs "github.com/prakersh/codexmultiauth/internal/infra/fs"
	"github.com/prakersh/codexmultiauth/internal/infra/paths"
)

const (
	CMAVaultKeyringService = "CMA Vault"
	CMAVaultKeyringAccount = "vault-key"
	VaultKeyFileVersionV1  = "cma-vault-key-v1"
)

type VaultKeyProviderKind string

const (
	VaultKeyProviderKeyring VaultKeyProviderKind = "keyring"
	VaultKeyProviderFile    VaultKeyProviderKind = "file"
)

type KeyringClient interface {
	Get(service, account string) ([]byte, error)
	Set(service, account string, value []byte) error
	Delete(service, account string) error
}

type OSKeyringClient struct{}

var openKeyring = keyring.Open

func openConfiguredKeyring(service string) (keyring.Keyring, error) {
	return openKeyring(keyring.Config{
		ServiceName: service,
		AllowedBackends: []keyring.BackendType{
			keyring.KeychainBackend,
			keyring.SecretServiceBackend,
			keyring.KWalletBackend,
		},
	})
}

func (OSKeyringClient) Get(service, account string) ([]byte, error) {
	ring, err := openConfiguredKeyring(service)
	if err != nil {
		return nil, err
	}
	item, err := ring.Get(account)
	if err != nil {
		return nil, err
	}
	return item.Data, nil
}

func (OSKeyringClient) Set(service, account string, value []byte) error {
	ring, err := openConfiguredKeyring(service)
	if err != nil {
		return err
	}
	return ring.Set(keyring.Item{
		Key:  account,
		Data: value,
	})
}

func (OSKeyringClient) Delete(service, account string) error {
	ring, err := openConfiguredKeyring(service)
	if err != nil {
		return err
	}
	return ring.Remove(account)
}

type VaultKeyManager struct {
	paths      paths.Paths
	configRepo *ConfigRepo
	keyring    KeyringClient
}

type vaultKeyFile struct {
	Version   string    `json:"version"`
	CreatedAt time.Time `json:"created_at"`
	Key       string    `json:"key"`
}

func NewVaultKeyManager(p paths.Paths, configRepo *ConfigRepo, keyringClient KeyringClient) *VaultKeyManager {
	return &VaultKeyManager{
		paths:      p,
		configRepo: configRepo,
		keyring:    keyringClient,
	}
}

func (m *VaultKeyManager) LoadOrCreate(ctx context.Context) ([]byte, VaultKeyProviderKind, error) {
	_ = ctx

	cfg, err := m.configRepo.Load()
	if err != nil {
		return nil, "", err
	}
	if !cfg.DisableKeyring && m.keyring != nil {
		key, err := m.loadOrCreateKeyringKey(m.fileKeyExists())
		if err == nil {
			return key, VaultKeyProviderKeyring, nil
		}
	}
	key, err := m.loadOrCreateFileKey()
	if err != nil {
		return nil, "", err
	}
	return key, VaultKeyProviderFile, nil
}

// fileKeyExists reports whether a file-backed vault key is already on disk.
// Its presence is the only record that an earlier run encrypted the vault
// under the file provider, because the vault itself does not name its provider.
func (m *VaultKeyManager) fileKeyExists() bool {
	_, err := os.Stat(m.paths.VaultKeyFile)
	return err == nil
}

// loadOrCreateKeyringKey returns the keyring-held vault key, minting one only
// when it is safe to do so.
//
// Minting unconditionally used to orphan vaults: if the first run could not
// reach the keyring (headless box, denied keychain prompt) the vault was
// encrypted under a file key, and a later run with a working keyring would
// find no stored key, mint a fresh one, and fail every decrypt from then on
// with "wrong passphrase or corrupted ciphertext" while the key that actually
// works sat unread on disk. When a file key exists, defer to it instead.
func (m *VaultKeyManager) loadOrCreateKeyringKey(fileKeyExists bool) ([]byte, error) {
	key, err := m.keyring.Get(CMAVaultKeyringService, CMAVaultKeyringAccount)
	if err == nil {
		if len(key) != cmacrypto.KeyLength {
			return nil, fmt.Errorf("invalid keyring vault key length %d", len(key))
		}
		return key, nil
	}
	if !errors.Is(err, keyring.ErrKeyNotFound) {
		return nil, err
	}
	if fileKeyExists {
		return nil, errVaultKeyFilePreferred
	}
	key, err = cmacrypto.RandomBytes(cmacrypto.KeyLength)
	if err != nil {
		return nil, err
	}
	if err := m.keyring.Set(CMAVaultKeyringService, CMAVaultKeyringAccount, key); err != nil {
		return nil, err
	}
	return key, nil
}

// errVaultKeyFilePreferred signals that the keyring holds no key but a file
// key already exists, so the caller must fall back rather than mint.
var errVaultKeyFilePreferred = errors.New("vault key file already exists; preferring file provider")

func (m *VaultKeyManager) loadOrCreateFileKey() ([]byte, error) {
	data, err := os.ReadFile(m.paths.VaultKeyFile)
	if err == nil {
		var record vaultKeyFile
		if err := json.Unmarshal(data, &record); err != nil {
			return nil, fmt.Errorf("parse vault key file: %w", err)
		}
		key, err := base64.StdEncoding.DecodeString(record.Key)
		if err != nil {
			return nil, fmt.Errorf("decode vault key file: %w", err)
		}
		if len(key) != cmacrypto.KeyLength {
			return nil, fmt.Errorf("invalid file vault key length %d", len(key))
		}
		return key, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("read vault key file: %w", err)
	}

	key, err := cmacrypto.RandomBytes(cmacrypto.KeyLength)
	if err != nil {
		return nil, err
	}
	record := vaultKeyFile{
		Version:   VaultKeyFileVersionV1,
		CreatedAt: time.Now().UTC(),
		Key:       base64.StdEncoding.EncodeToString(key),
	}
	payload, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal vault key file: %w", err)
	}
	if err := cmafs.WriteFileAtomic(m.paths.VaultKeyFile, payload, cmafs.AtomicWriteOptions{Mode: cmafs.FileMode}); err != nil {
		return nil, err
	}
	return key, nil
}
