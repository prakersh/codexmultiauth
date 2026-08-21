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
	pass, err := app.ResolvePassphrase("hash:616263", true, nil)
	require.NoError(t, err)
	require.Equal(t, []byte("abc"), pass)
}

// hash: hex-decodes straight to the passphrase, so it puts the secret in argv
// exactly like pass: does and must sit behind the same flag.
func TestResolvePassphrase_HashRequiresPlainFlag(t *testing.T) {
	_, err := app.ResolvePassphrase("hash:616263", false, nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "--allow-plain-pass-arg")
}

// An unrecognized source is usually a passphrase containing a colon; the error
// must never echo it back.
func TestResolvePassphrase_UnsupportedSourceDoesNotEchoSecret(t *testing.T) {
	_, err := app.ResolvePassphrase("S3cret:horse!", true, nil)
	require.Error(t, err)
	require.NotContains(t, err.Error(), "S3cret")
	require.NotContains(t, err.Error(), "horse")
}

func TestResolvePassphrase_PlainRejectedWithoutFlag(t *testing.T) {
	_, err := app.ResolvePassphrase("pass:secret", false, nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "--allow-plain-pass-arg")

	_, err = app.ResolvePassphrase("secret", false, nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "--allow-plain-pass-arg")
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

	_, err = app.ResolvePassphrase("hash:", true, nil)
	require.Error(t, err)

	_, err = app.ResolvePassphrase("hash:not-hex", true, nil)
	require.Error(t, err)

	pass, err := app.ResolvePassphrase("pass:secret", true, nil)
	require.NoError(t, err)
	require.Equal(t, []byte("secret"), pass)

	pass, err = app.ResolvePassphrase("secret", true, nil)
	require.NoError(t, err)
	require.Equal(t, []byte("secret"), pass)
}
