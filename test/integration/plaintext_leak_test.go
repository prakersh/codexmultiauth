package integration_test

import (
	"context"
	"os"
	"testing"

	"github.com/prakersh/codexmultiauth/internal/app"
	"github.com/stretchr/testify/require"
)

func TestEncryptedArtifactsDoNotContainPlaintextTokens(t *testing.T) {
	manager, p, authStore := newManager(t)
	ctx := context.Background()

	token := "very-secret-access-token"
	require.NoError(t, authStore.Save(ctx, []byte(`{"auth_mode":"chatgpt","tokens":{"access_token":"`+token+`","refresh_token":"refresh-1","account_id":"acc-1"}}`)))
	_, err := manager.Save(ctx, app.SaveInput{DisplayName: "work"})
	require.NoError(t, err)

	vaultData, err := os.ReadFile(p.VaultFile)
	require.NoError(t, err)
	require.NotContains(t, string(vaultData), token)

	backupPath, err := manager.Backup(ctx, app.BackupInput{Passphrase: []byte("secret"), Target: "guard"})
	require.NoError(t, err)

	backupData, err := os.ReadFile(backupPath)
	require.NoError(t, err)
	require.NotContains(t, string(backupData), token)
}
