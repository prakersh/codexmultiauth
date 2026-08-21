package crypto

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"golang.org/x/crypto/chacha20poly1305"
)

const (
	EnvelopeVersionV1 = "cma-envelope-v1"
	KDFNameArgon2id   = "argon2id"
	AEADXChaCha20     = "xchacha20poly1305"
)

var (
	ErrUnsupportedEnvelopeVersion = errors.New("unsupported envelope version")
	ErrWrongPassphrase            = errors.New("wrong passphrase or corrupted ciphertext")
	marshalEnvelopeJSON           = json.MarshalIndent
	unmarshalEnvelopeJSON         = json.Unmarshal
)

type KDFMetadata struct {
	Name        string         `json:"name,omitempty"`
	Salt        string         `json:"salt,omitempty"`
	Memory      uint32         `json:"memory,omitempty"`
	Iterations  uint32         `json:"iterations,omitempty"`
	Parallelism uint8          `json:"parallelism,omitempty"`
	KeyLength   int            `json:"key_length,omitempty"`
	Params      Argon2idParams `json:"-"`
}

type AEADMetadata struct {
	Name  string `json:"name"`
	Nonce string `json:"nonce"`
}

type Envelope struct {
	Version    string            `json:"version"`
	CreatedAt  time.Time         `json:"created_at"`
	KDF        *KDFMetadata      `json:"kdf,omitempty"`
	AEAD       AEADMetadata      `json:"aead"`
	Ciphertext string            `json:"ciphertext"`
	Metadata   map[string]string `json:"metadata,omitempty"`
}

func EncryptWithKey(plaintext, key []byte, metadata map[string]string) (Envelope, error) {
	if len(key) != KeyLength {
		return Envelope{}, fmt.Errorf("encrypt with key: key length must be %d", KeyLength)
	}
	nonce, err := RandomBytes(chacha20poly1305.NonceSizeX)
	if err != nil {
		return Envelope{}, err
	}
	aead, err := chacha20poly1305.NewX(key)
	if err != nil {
		return Envelope{}, fmt.Errorf("create xchacha20poly1305: %w", err)
	}
	ciphertext := aead.Seal(nil, nonce, plaintext, nil)
	return Envelope{
		Version:   EnvelopeVersionV1,
		CreatedAt: time.Now().UTC(),
		AEAD: AEADMetadata{
			Name:  AEADXChaCha20,
			Nonce: base64.StdEncoding.EncodeToString(nonce),
		},
		Ciphertext: base64.StdEncoding.EncodeToString(ciphertext),
		Metadata:   metadata,
	}, nil
}

func DecryptWithKey(envelope Envelope, key []byte) ([]byte, error) {
	if envelope.Version != EnvelopeVersionV1 {
		return nil, ErrUnsupportedEnvelopeVersion
	}
	if len(key) != KeyLength {
		return nil, fmt.Errorf("decrypt with key: key length must be %d", KeyLength)
	}
	if envelope.AEAD.Name != AEADXChaCha20 {
		return nil, fmt.Errorf("unsupported aead %q", envelope.AEAD.Name)
	}

	nonce, err := base64.StdEncoding.DecodeString(envelope.AEAD.Nonce)
	if err != nil {
		return nil, fmt.Errorf("decode nonce: %w", err)
	}
	ciphertext, err := base64.StdEncoding.DecodeString(envelope.Ciphertext)
	if err != nil {
		return nil, fmt.Errorf("decode ciphertext: %w", err)
	}
	aead, err := chacha20poly1305.NewX(key)
	if err != nil {
		return nil, fmt.Errorf("create xchacha20poly1305: %w", err)
	}
	plaintext, err := aead.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, ErrWrongPassphrase
	}
	return plaintext, nil
}

func EncryptWithPassphrase(plaintext, passphrase []byte, params Argon2idParams, metadata map[string]string) (Envelope, error) {
	if len(passphrase) == 0 {
		return Envelope{}, errors.New("encrypt with passphrase: empty passphrase")
	}
	if params.KeyLength == 0 {
		params.KeyLength = KeyLength
	}
	salt, err := RandomSalt(params)
	if err != nil {
		return Envelope{}, err
	}
	key := DeriveKey(passphrase, salt, params)
	envelope, err := EncryptWithKey(plaintext, key, metadata)
	if err != nil {
		return Envelope{}, err
	}
	envelope.KDF = &KDFMetadata{
		Name:        KDFNameArgon2id,
		Salt:        base64.StdEncoding.EncodeToString(salt),
		Memory:      params.Memory,
		Iterations:  params.Iterations,
		Parallelism: params.Parallelism,
		KeyLength:   params.KeyLength,
	}
	return envelope, nil
}

func DecryptWithPassphrase(envelope Envelope, passphrase []byte) ([]byte, error) {
	if envelope.KDF == nil {
		return nil, errors.New("decrypt with passphrase: envelope missing kdf metadata")
	}
	if len(passphrase) == 0 {
		return nil, errors.New("decrypt with passphrase: empty passphrase")
	}
	if envelope.KDF.Name != KDFNameArgon2id {
		return nil, fmt.Errorf("unsupported kdf %q", envelope.KDF.Name)
	}
	salt, err := base64.StdEncoding.DecodeString(envelope.KDF.Salt)
	if err != nil {
		return nil, fmt.Errorf("decode salt: %w", err)
	}
	// KDF parameters come from the file, which is unauthenticated at this
	// point: DecryptWithPassphrase has to derive the key before it can verify
	// anything. argon2.IDKey panics outright on zero iterations or
	// parallelism, and honors Memory literally, so a corrupted or hostile
	// backup could crash the process or drive a multi-terabyte allocation.
	params := Argon2idParams{
		Memory:      envelope.KDF.Memory,
		Iterations:  envelope.KDF.Iterations,
		Parallelism: envelope.KDF.Parallelism,
		KeyLength:   envelope.KDF.KeyLength,
	}
	if err := validateKDFParams(params); err != nil {
		return nil, err
	}
	key := DeriveKey(passphrase, salt, params)
	return DecryptWithKey(envelope, key)
}

// Ceilings for KDF parameters read from a file. The upper bounds are far above
// anything Default() produces, so they reject absurd values without
// constraining a legitimately expensive envelope.
const (
	maxKDFMemory      = 4 << 20 // 4 GiB, expressed in KiB
	maxKDFIterations  = 64
	maxKDFParallelism = 64
	maxKDFKeyLength   = 1 << 10
)

func validateKDFParams(params Argon2idParams) error {
	switch {
	case params.Iterations < 1 || params.Iterations > maxKDFIterations:
		return fmt.Errorf("invalid kdf iterations %d", params.Iterations)
	case params.Parallelism < 1 || params.Parallelism > maxKDFParallelism:
		return fmt.Errorf("invalid kdf parallelism %d", params.Parallelism)
	case params.Memory < 8 || params.Memory > maxKDFMemory:
		return fmt.Errorf("invalid kdf memory %d", params.Memory)
	case params.KeyLength < 16 || params.KeyLength > maxKDFKeyLength:
		return fmt.Errorf("invalid kdf key length %d", params.KeyLength)
	}
	return nil
}

func MarshalEnvelope(envelope Envelope) ([]byte, error) {
	data, err := marshalEnvelopeJSON(envelope, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal envelope: %w", err)
	}
	return data, nil
}

func UnmarshalEnvelope(data []byte) (Envelope, error) {
	var envelope Envelope
	if err := unmarshalEnvelopeJSON(data, &envelope); err != nil {
		return Envelope{}, fmt.Errorf("unmarshal envelope: %w", err)
	}
	return envelope, nil
}
