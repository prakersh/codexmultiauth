package crypto

import (
	"crypto/rand"
	"fmt"
	"io"
	"runtime"

	"golang.org/x/crypto/argon2"
)

const KeyLength = 32

var randReader io.Reader = rand.Reader

type Argon2idParams struct {
	Memory      uint32 `json:"memory"`
	Iterations  uint32 `json:"iterations"`
	Parallelism uint8  `json:"parallelism"`
	SaltLength  int    `json:"salt_length"`
	KeyLength   int    `json:"key_length"`
}

func DefaultArgon2idParams() Argon2idParams {
	parallelism := runtime.NumCPU()
	if parallelism > 4 {
		parallelism = 4
	}
	if parallelism < 1 {
		parallelism = 1
	}
	return Argon2idParams{
		Memory:      64 * 1024,
		Iterations:  3,
		Parallelism: uint8(parallelism),
		SaltLength:  16,
		KeyLength:   KeyLength,
	}
}

func RandomBytes(length int) ([]byte, error) {
	buf := make([]byte, length)
	if _, err := io.ReadFull(randReader, buf); err != nil {
		return nil, fmt.Errorf("random bytes: %w", err)
	}
	return buf, nil
}

func RandomSalt(params Argon2idParams) ([]byte, error) {
	return RandomBytes(params.SaltLength)
}

func DeriveKey(passphrase, salt []byte, params Argon2idParams) []byte {
	keyLength := params.KeyLength
	if keyLength == 0 {
		keyLength = KeyLength
	}
	return argon2.IDKey(passphrase, salt, params.Iterations, params.Memory, params.Parallelism, uint32(keyLength))
}
