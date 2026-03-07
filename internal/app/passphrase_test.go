package app_test

import (
	"testing"

	"github.com/prakersh/codexmultiauth/internal/app"
	"github.com/stretchr/testify/require"
)

func TestResolvePassphrase_Env(t *testing.T) {
	t.Setenv("CMA_PASS", "secret")
	pass, err := app.ResolvePassphrase("env:CMA_PASS", false, nil)
	require.NoError(t, err)
	require.Equal(t, []byte("secret"), pass)
}

func TestResolvePassphrase_Hash(t *testing.T) {
	pass, err := app.ResolvePassphrase("hash:616263", false, nil)
	require.NoError(t, err)
	require.Equal(t, []byte("abc"), pass)
}

func TestResolvePassphrase_PlainRejectedWithoutFlag(t *testing.T) {
	_, err := app.ResolvePassphrase("pass:secret", false, nil)
	require.Error(t, err)
}

func TestResolvePassphrase_PromptAndUnsupported(t *testing.T) {
	pass, err := app.ResolvePassphrase("prompt", false, func(prompt string) ([]byte, error) {
		return []byte("typed"), nil
	})
	require.NoError(t, err)
	require.Equal(t, []byte("typed"), pass)

	_, err = app.ResolvePassphrase("wat", false, nil)
	require.Error(t, err)
}

func TestResolvePassphrase_ErrorBranches(t *testing.T) {
	_, err := app.ResolvePassphrase("prompt", false, nil)
	require.Error(t, err)

	_, err = app.ResolvePassphrase("env:", false, nil)
	require.Error(t, err)

	t.Setenv("EMPTY_PASS", "")
	_, err = app.ResolvePassphrase("env:EMPTY_PASS", false, nil)
	require.Error(t, err)

	_, err = app.ResolvePassphrase("hash:", false, nil)
	require.Error(t, err)

	_, err = app.ResolvePassphrase("hash:not-hex", false, nil)
	require.Error(t, err)

	pass, err := app.ResolvePassphrase("pass:secret", true, nil)
	require.NoError(t, err)
	require.Equal(t, []byte("secret"), pass)
}
