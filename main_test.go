package main

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMainSuccessAndFailure(t *testing.T) {
	originalExecute := execute
	originalExit := exit
	defer func() {
		execute = originalExecute
		exit = originalExit
	}()

	called := false
	execute = func() error { return nil }
	exit = func(code int) { called = true }
	main()
	require.False(t, called)

	execute = func() error { return errors.New("boom") }
	exit = func(code int) {
		called = true
		require.Equal(t, 1, code)
	}
	main()
	require.True(t, called)
}
