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
	// EnvelopeVersionV2 binds the envelope's own metadata into the AEAD as
	// additional authenticated data. v1 sealed with no AAD, so everything
	// outside the ciphertext (version, KDF parameters, created_at, and the
	// metadata map that names the account) could be rewritten and the
	// ciphertext would still open cleanly. v1 envelopes are still readable;
	// anything written now is v2.
	EnvelopeVersionV2 = "cma-envelope-v2"
	KDFNameArgon2id   = "argon2id"
	AEADXChaCha20     = "xchacha20poly1305"
)

var (
	ErrUnsupportedEnvelopeVersion = errors.New("unsupported envelope version; this file was written by a newer CMA, upgrade to read it")
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
	return seal(plaintext, key, metadata, nil)
}

// seal builds the envelope's public fields first, derives the AAD from them,
// and only then encrypts, so the ciphertext is cryptographically tied to the
// metadata that travels alongside it.
func seal(plaintext, key []byte, metadata map[string]string, kdf *KDFMetadata) (Envelope, error) {
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

	envelope := Envelope{
		Version:   EnvelopeVersionV2,
		CreatedAt: time.Now().UTC().Truncate(time.Second),
		KDF:       kdf,
		AEAD: AEADMetadata{
			Name:  AEADXChaCha20,
			Nonce: base64.StdEncoding.EncodeToString(nonce),
		},
		Metadata: metadata,
	}

	aad, err := envelopeAAD(envelope)
	if err != nil {
		return Envelope{}, err
	}
	envelope.Ciphertext = base64.StdEncoding.EncodeToString(aead.Seal(nil, nonce, plaintext, aad))
	return envelope, nil
}

// envelopeAAD produces the authenticated-but-unencrypted bytes for an
// envelope. Every field it covers is a string or an integer, and the metadata
// map is marshalled by encoding/json which sorts keys, so the encoding is
// stable across a write/read round trip. created_at is truncated to the second
// at seal time for the same reason.
func envelopeAAD(envelope Envelope) ([]byte, error) {
	if envelope.Version != EnvelopeVersionV2 {
		// v1 sealed without AAD; keep it that way so old files still open.
		return nil, nil
	}
	payload := struct {
		Version   string            `json:"version"`
		CreatedAt string            `json:"created_at"`
		AEAD      string            `json:"aead"`
		Nonce     string            `json:"nonce"`
		KDF       *KDFMetadata      `json:"kdf,omitempty"`
		Metadata  map[string]string `json:"metadata,omitempty"`
	}{
		Version:   envelope.Version,
		CreatedAt: envelope.CreatedAt.UTC().Format(time.RFC3339),
		AEAD:      envelope.AEAD.Name,
		Nonce:     envelope.AEAD.Nonce,
		KDF:       envelope.KDF,
		Metadata:  envelope.Metadata,
	}
	aad, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("build envelope aad: %w", err)
	}
	return aad, nil
}

func DecryptWithKey(envelope Envelope, key []byte) ([]byte, error) {
	if envelope.Version != EnvelopeVersionV1 && envelope.Version != EnvelopeVersionV2 {
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
	aad, err := envelopeAAD(envelope)
	if err != nil {
		return nil, err
	}
	// Any edit to the bound metadata changes the AAD, so Open fails here the
	// same way a wrong key does.
	plaintext, err := aead.Open(nil, nonce, ciphertext, aad)
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
	defer Zero(key)
	// The KDF block is built before sealing so it can be bound as AAD; a
	// tampered iteration count or salt then fails authentication instead of
	// silently steering key derivation.
	kdf := &KDFMetadata{
		Name:        KDFNameArgon2id,
		Salt:        base64.StdEncoding.EncodeToString(salt),
		Memory:      params.Memory,
		Iterations:  params.Iterations,
		Parallelism: params.Parallelism,
		KeyLength:   params.KeyLength,
	}
	envelope, err := seal(plaintext, key, metadata, kdf)
	if err != nil {
		return Envelope{}, err
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
	defer Zero(key)
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
