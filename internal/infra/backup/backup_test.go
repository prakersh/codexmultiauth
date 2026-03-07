package backup_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/prakersh/codexmultiauth/internal/domain"
	"github.com/prakersh/codexmultiauth/internal/infra/backup"
	"github.com/stretchr/testify/require"
)

func TestWriteReadRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "backup.cma.bak")
	accounts := []backup.Account{
		{
			Account: domain.Account{ID: "acc-1", DisplayName: "work", Fingerprint: "fp-1"},
			Payload: []byte(`{"auth_mode":"chatgpt"}`),
		},
	}

	require.NoError(t, backup.Write(path, accounts, []byte("secret")))

	artifact, err := backup.Read(path, []byte("secret"))
	require.NoError(t, err)
	require.Len(t, artifact.Accounts, 1)
	require.Equal(t, "work", artifact.Accounts[0].Account.DisplayName)
}

func TestReadWrongPassphrase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "backup.cma.bak")
	require.NoError(t, backup.Write(path, nil, []byte("secret")))

	_, err := backup.Read(path, []byte("wrong"))
	require.Error(t, err)
}

func TestReadUnsupportedVersion(t *testing.T) {
	path := filepath.Join(t.TempDir(), "backup.cma.bak")
	require.NoError(t, os.WriteFile(path, []byte(`{"version":"old","envelope":{}}`), 0o600))

	_, err := backup.Read(path, []byte("secret"))
	require.Error(t, err)
}
