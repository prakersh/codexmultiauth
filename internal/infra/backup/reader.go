package backup

import (
	"encoding/json"
	"fmt"
	"os"

	cmacrypto "github.com/prakersh/codexmultiauth/internal/infra/crypto"
)

func Read(path string, passphrase []byte) (Plaintext, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Plaintext{}, fmt.Errorf("read backup file: %w", err)
	}
	var file File
	if err := json.Unmarshal(data, &file); err != nil {
		return Plaintext{}, fmt.Errorf("parse backup file: %w", err)
	}
	if file.Version != BackupVersionV1 {
		return Plaintext{}, fmt.Errorf("unsupported backup version %q", file.Version)
	}
	raw, err := cmacrypto.DecryptWithPassphrase(file.Envelope, passphrase)
	if err != nil {
		return Plaintext{}, err
	}
	var plaintext Plaintext
	if err := json.Unmarshal(raw, &plaintext); err != nil {
		return Plaintext{}, fmt.Errorf("parse backup plaintext: %w", err)
	}
	return plaintext, nil
}
