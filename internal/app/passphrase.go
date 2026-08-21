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
		// hash: is not a hash. It hex-decodes to the passphrase itself, so the
		// material sits in argv exactly like pass: does and is visible to any
		// local user through ps. It belongs behind the same gate.
		if !allowPlain {
			return nil, errPlainPassphraseArg
		}
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
			return nil, errPlainPassphraseArg
		}
		return []byte(strings.TrimPrefix(source, "pass:")), nil
	case isBarePassphraseLiteral(source):
		if !allowPlain {
			return nil, errPlainPassphraseArg
		}
		return []byte(source), nil
	default:
		// Never echo the source. An unrecognized value is usually a passphrase
		// that happens to contain a colon, so quoting it here would print the
		// secret to stderr, into scrollback, and into any CI log.
		return nil, errors.New("unsupported passphrase source; a passphrase containing ':' must be passed as pass:<literal> or through env:VAR")
	}
}

// errPlainPassphraseArg is shared so no branch can accidentally phrase this
// with the passphrase interpolated into it.
var errPlainPassphraseArg = errors.New("plain passphrase arguments require --allow-plain-pass-arg; use prompt or env:VAR to keep the passphrase out of argv")

func isBarePassphraseLiteral(source string) bool {
	source = strings.TrimSpace(source)
	return source != "" && !strings.Contains(source, ":")
}
