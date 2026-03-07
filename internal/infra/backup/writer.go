package backup

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"time"

	"github.com/prakersh/codexmultiauth/internal/domain"
	cmacrypto "github.com/prakersh/codexmultiauth/internal/infra/crypto"
	cmafs "github.com/prakersh/codexmultiauth/internal/infra/fs"
)

func Write(path string, accounts []Account, passphrase []byte) error {
	accountIDs := make([]string, 0, len(accounts))
	for _, account := range accounts {
		accountIDs = append(accountIDs, account.Account.ID)
	}
	plaintext := Plaintext{
		Manifest: domain.BackupManifest{
			Version:    BackupVersionV1,
			CreatedAt:  time.Now().UTC(),
			AccountIDs: accountIDs,
		},
		Accounts: accounts,
	}
	raw, err := json.Marshal(plaintext)
	if err != nil {
		return fmt.Errorf("marshal backup plaintext: %w", err)
	}
	envelope, err := cmacrypto.EncryptWithPassphrase(raw, passphrase, cmacrypto.DefaultArgon2idParams(), map[string]string{
		"kind": "backup",
	})
	if err != nil {
		return err
	}
	file := File{
		Version:   BackupVersionV1,
		CreatedAt: time.Now().UTC(),
		Envelope:  envelope,
	}
	data, err := json.MarshalIndent(file, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal backup file: %w", err)
	}
	if err := cmafs.EnsureDir(filepath.Dir(path)); err != nil {
		return err
	}
	return cmafs.WriteFileAtomic(path, data, cmafs.AtomicWriteOptions{Mode: cmafs.FileMode})
}
