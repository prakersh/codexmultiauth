package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/prakersh/codexmultiauth/internal/infra/crypto"
	cmafs "github.com/prakersh/codexmultiauth/internal/infra/fs"
	"github.com/prakersh/codexmultiauth/internal/infra/paths"
)

const VaultVersionV1 = "cma-vault-v1"

type VaultEntry struct {
	AccountID   string
	Fingerprint string
	Payload     []byte
	Source      string
	SavedAt     time.Time
}

type Vault struct {
	Version string
	Entries []VaultEntry
}

type vaultFile struct {
	Version string           `json:"version"`
	Entries []vaultFileEntry `json:"entries"`
}

type vaultFileEntry struct {
	AccountID   string          `json:"account_id"`
	Fingerprint string          `json:"fingerprint"`
	Source      string          `json:"source"`
	SavedAt     time.Time       `json:"saved_at"`
	Envelope    crypto.Envelope `json:"envelope"`
}

type VaultRepo struct {
	paths paths.Paths
}

func NewVaultRepo(p paths.Paths) *VaultRepo {
	return &VaultRepo{paths: p}
}

func (r *VaultRepo) Load(key []byte) (Vault, error) {
	data, err := os.ReadFile(r.paths.VaultFile)
	if errors.Is(err, os.ErrNotExist) {
		return Vault{Version: VaultVersionV1}, nil
	}
	if err != nil {
		return Vault{}, fmt.Errorf("load vault: %w", err)
	}
	var file vaultFile
	if err := json.Unmarshal(data, &file); err != nil {
		return Vault{}, fmt.Errorf("parse vault: %w", err)
	}
	if file.Version == "" {
		file.Version = VaultVersionV1
	}

	vault := Vault{Version: file.Version}
	for _, entry := range file.Entries {
		payload, err := crypto.DecryptWithKey(entry.Envelope, key)
		if err != nil {
			return Vault{}, fmt.Errorf("decrypt vault entry %s: %w", entry.AccountID, err)
		}
		vault.Entries = append(vault.Entries, VaultEntry{
			AccountID:   entry.AccountID,
			Fingerprint: entry.Fingerprint,
			Payload:     payload,
			Source:      entry.Source,
			SavedAt:     entry.SavedAt,
		})
	}
	return vault, nil
}

func (r *VaultRepo) Save(vault Vault, key []byte) error {
	if vault.Version == "" {
		vault.Version = VaultVersionV1
	}
	file := vaultFile{Version: vault.Version}
	for _, entry := range vault.Entries {
		envelope, err := crypto.EncryptWithKey(entry.Payload, key, map[string]string{
			"account_id": entry.AccountID,
			"source":     entry.Source,
		})
		if err != nil {
			return fmt.Errorf("encrypt vault entry %s: %w", entry.AccountID, err)
		}
		file.Entries = append(file.Entries, vaultFileEntry{
			AccountID:   entry.AccountID,
			Fingerprint: entry.Fingerprint,
			Source:      entry.Source,
			SavedAt:     entry.SavedAt,
			Envelope:    envelope,
		})
	}
	data, err := json.MarshalIndent(file, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal vault: %w", err)
	}
	return cmafs.WriteFileAtomic(r.paths.VaultFile, data, cmafs.AtomicWriteOptions{Mode: cmafs.FileMode})
}
