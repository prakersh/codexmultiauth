package codexcli_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/prakersh/codexmultiauth/internal/infra/codexcli"
	"github.com/stretchr/testify/require"
)

func TestClientLoginAndStatus(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "calls.log")
	script := filepath.Join(dir, "codex")
	require.NoError(t, os.WriteFile(script, []byte("#!/bin/sh\nif [ \"$1\" = \"login\" ] && [ \"$2\" = \"status\" ]; then\n  echo \"logged in\"\n  exit 0\nfi\necho \"$@\" >> \""+logPath+"\"\n"), 0o700))

	client := codexcli.NewClient(script)
	require.NoError(t, client.Login(context.Background(), true, false))
	require.NoError(t, client.Login(context.Background(), false, true))

	out, err := client.Status(context.Background())
	require.NoError(t, err)
	require.Equal(t, "logged in", out)

	calls, err := os.ReadFile(logPath)
	require.NoError(t, err)
	require.Contains(t, string(calls), "login --device-auth")
	require.Contains(t, string(calls), "login --with-api-key")
}
