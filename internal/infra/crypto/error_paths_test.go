package crypto

import (
	"encoding/json"
	"errors"
	"io"
	"testing"

	"github.com/stretchr/testify/require"
)

type errReader struct{}

func (errReader) Read(p []byte) (int, error) {
	return 0, errors.New("entropy unavailable")
}

func TestRandomAndEnvelopeErrorPaths(t *testing.T) {
	originalRand := randReader
	originalMarshal := marshalEnvelopeJSON
	originalUnmarshal := unmarshalEnvelopeJSON
	defer func() {
		randReader = originalRand
		marshalEnvelopeJSON = originalMarshal
		unmarshalEnvelopeJSON = originalUnmarshal
	}()

	randReader = errReader{}
	_, err := RandomBytes(8)
	require.Error(t, err)

	_, err = EncryptWithKey([]byte("secret"), make([]byte, KeyLength), nil)
	require.Error(t, err)

	_, err = EncryptWithPassphrase([]byte("secret"), []byte("pass"), DefaultArgon2idParams(), nil)
	require.Error(t, err)

	marshalEnvelopeJSON = func(v any, prefix, indent string) ([]byte, error) {
		return nil, errors.New("marshal failed")
	}
	_, err = MarshalEnvelope(Envelope{})
	require.Error(t, err)

	unmarshalEnvelopeJSON = func(data []byte, v any) error {
		return errors.New("unmarshal failed")
	}
	_, err = UnmarshalEnvelope([]byte(`{}`))
	require.Error(t, err)
}

func TestDeriveKeyAndDecryptErrorPaths(t *testing.T) {
	key := DeriveKey([]byte("secret"), []byte("salt"), Argon2idParams{Iterations: 1, Memory: 8, Parallelism: 1})
	require.Len(t, key, KeyLength)

	envelope, err := EncryptWithPassphrase([]byte("secret"), []byte("pass"), Argon2idParams{
		Iterations:  1,
		Memory:      8,
		Parallelism: 1,
		KeyLength:   KeyLength,
		SaltLength:  16,
	}, nil)
	require.NoError(t, err)
	require.Equal(t, KeyLength, envelope.KDF.KeyLength)

	_, err = DecryptWithPassphrase(envelope, nil)
	require.Error(t, err)

	key, err = RandomBytes(KeyLength)
	require.NoError(t, err)
	keyEnvelope, err := EncryptWithKey([]byte("secret"), key, nil)
	require.NoError(t, err)

	keyEnvelope.Ciphertext = "!!!"
	_, err = DecryptWithKey(keyEnvelope, key)
	require.Error(t, err)

	keyEnvelope, err = EncryptWithKey([]byte("secret"), key, nil)
	require.NoError(t, err)
	_, err = DecryptWithKey(keyEnvelope, []byte("short"))
	require.Error(t, err)

	_, err = UnmarshalEnvelope([]byte("{"))
	require.Error(t, err)

	data, err := json.Marshal(Envelope{Version: EnvelopeVersionV1})
	require.NoError(t, err)
	decoded, err := UnmarshalEnvelope(data)
	require.NoError(t, err)
	require.Equal(t, EnvelopeVersionV1, decoded.Version)
}

var _ io.Reader = errReader{}
