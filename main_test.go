package main

import (
	"bytes"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMainSuccessAndFailure(t *testing.T) {
	originalExecute := execute
	originalExit := exit
	originalStderr := stderr
	defer func() {
		execute = originalExecute
		exit = originalExit
		stderr = originalStderr
	}()

	called := false
	var errOut bytes.Buffer
	stderr = &errOut
	execute = func() error { return nil }
	exit = func(code int) { called = true }
	main()
	require.False(t, called)
	require.Empty(t, errOut.String())

	errOut.Reset()
	execute = func() error { return errors.New("plain passphrase arguments require --allow-plain-pass-arg: pass:supersecret") }
	exit = func(code int) {
		called = true
		require.Equal(t, 1, code)
	}
	main()
	require.True(t, called)
	require.Contains(t, errOut.String(), "Error:")
	require.Contains(t, errOut.String(), "plain passphrase arguments require --allow-plain-pass-arg")
	require.NotContains(t, errOut.String(), "supersecret")
}
