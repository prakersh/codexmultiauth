package backup

import (
	"time"

	"github.com/prakersh/codexmultiauth/internal/domain"
	"github.com/prakersh/codexmultiauth/internal/infra/crypto"
)

const BackupVersionV1 = "cma-backup-v1"

type File struct {
	Version   string          `json:"version"`
	CreatedAt time.Time       `json:"created_at"`
	Envelope  crypto.Envelope `json:"envelope"`
}

type Plaintext struct {
	Manifest domain.BackupManifest `json:"manifest"`
	Accounts []Account             `json:"accounts"`
}

type Account struct {
	Account domain.Account `json:"account"`
	Payload []byte         `json:"payload"`
}
