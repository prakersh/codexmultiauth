package app

import (
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"strings"
)

type PromptFunc func(prompt string) ([]byte, error)

func ResolvePassphrase(source string, allowPlain bool, prompt PromptFunc) ([]byte, error) {
	switch {
	case source == "prompt":
		if prompt == nil {
			return nil, errors.New("prompt source requires prompt function")
		}
		return prompt("Passphrase")
	case strings.HasPrefix(source, "env:"):
		name := strings.TrimSpace(strings.TrimPrefix(source, "env:"))
		if name == "" {
			return nil, errors.New("env passphrase source requires variable name")
		}
		value := os.Getenv(name)
		if value == "" {
			return nil, fmt.Errorf("environment variable %s is empty", name)
		}
		return []byte(value), nil
	case strings.HasPrefix(source, "hash:"):
		raw := strings.TrimSpace(strings.TrimPrefix(source, "hash:"))
		if raw == "" {
			return nil, errors.New("hash passphrase source requires hex payload")
		}
		data, err := hex.DecodeString(raw)
		if err != nil {
			return nil, fmt.Errorf("decode passphrase hash: %w", err)
		}
		return data, nil
	case strings.HasPrefix(source, "pass:"):
		if !allowPlain {
			return nil, errors.New("plain passphrase arguments require --allow-plain-pass-arg")
		}
		return []byte(strings.TrimPrefix(source, "pass:")), nil
	default:
		return nil, fmt.Errorf("unsupported passphrase source %q", source)
	}
}
