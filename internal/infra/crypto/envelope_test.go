package crypto_test

import (
	"encoding/base64"
	"testing"

	cmacrypto "github.com/prakersh/codexmultiauth/internal/infra/crypto"
	"github.com/stretchr/testify/require"
)

func TestEncryptDecryptWithKey_RoundTrip(t *testing.T) {
	key, err := cmacrypto.RandomBytes(cmacrypto.KeyLength)
	require.NoError(t, err)

	envelope, err := cmacrypto.EncryptWithKey([]byte("secret"), key, map[string]string{"kind": "vault"})
	require.NoError(t, err)
	require.Equal(t, cmacrypto.EnvelopeVersionV1, envelope.Version)
	require.Equal(t, "vault", envelope.Metadata["kind"])

	plaintext, err := cmacrypto.DecryptWithKey(envelope, key)
	require.NoError(t, err)
	require.Equal(t, []byte("secret"), plaintext)
}

func TestEncryptDecryptWithPassphrase_RoundTrip(t *testing.T) {
	params := cmacrypto.DefaultArgon2idParams()

	envelope, err := cmacrypto.EncryptWithPassphrase([]byte("backup"), []byte("correct horse"), params, nil)
	require.NoError(t, err)
	require.NotNil(t, envelope.KDF)

	plaintext, err := cmacrypto.DecryptWithPassphrase(envelope, []byte("correct horse"))
	require.NoError(t, err)
	require.Equal(t, []byte("backup"), plaintext)
}

func TestDecryptWithPassphrase_WrongPassphrase(t *testing.T) {
	params := cmacrypto.DefaultArgon2idParams()

	envelope, err := cmacrypto.EncryptWithPassphrase([]byte("backup"), []byte("correct horse"), params, nil)
	require.NoError(t, err)

	_, err = cmacrypto.DecryptWithPassphrase(envelope, []byte("wrong battery"))
	require.ErrorIs(t, err, cmacrypto.ErrWrongPassphrase)
}

func TestDecryptWithKey_TamperedCiphertext(t *testing.T) {
	key, err := cmacrypto.RandomBytes(cmacrypto.KeyLength)
	require.NoError(t, err)

	envelope, err := cmacrypto.EncryptWithKey([]byte("secret"), key, nil)
	require.NoError(t, err)

	rawCiphertext, err := base64.StdEncoding.DecodeString(envelope.Ciphertext)
	require.NoError(t, err)
	rawCiphertext[0] ^= 0xff
	envelope.Ciphertext = base64.StdEncoding.EncodeToString(rawCiphertext)

	_, err = cmacrypto.DecryptWithKey(envelope, key)
	require.ErrorIs(t, err, cmacrypto.ErrWrongPassphrase)
}

func TestEnvelope_ErrorPaths(t *testing.T) {
	key, err := cmacrypto.RandomBytes(cmacrypto.KeyLength)
	require.NoError(t, err)

	_, err = cmacrypto.EncryptWithKey([]byte("secret"), []byte("short"), nil)
	require.Error(t, err)

	envelope, err := cmacrypto.EncryptWithKey([]byte("secret"), key, nil)
	require.NoError(t, err)

	envelope.Version = "old"
	_, err = cmacrypto.DecryptWithKey(envelope, key)
	require.ErrorIs(t, err, cmacrypto.ErrUnsupportedEnvelopeVersion)

	envelope, err = cmacrypto.EncryptWithKey([]byte("secret"), key, nil)
	require.NoError(t, err)
	envelope.AEAD.Name = "aes-gcm"
	_, err = cmacrypto.DecryptWithKey(envelope, key)
	require.Error(t, err)

	envelope.AEAD.Name = cmacrypto.AEADXChaCha20
	envelope.AEAD.Nonce = "!!!"
	_, err = cmacrypto.DecryptWithKey(envelope, key)
	require.Error(t, err)

	_, err = cmacrypto.EncryptWithPassphrase([]byte("secret"), nil, cmacrypto.DefaultArgon2idParams(), nil)
	require.Error(t, err)

	_, err = cmacrypto.DecryptWithPassphrase(cmacrypto.Envelope{}, []byte("secret"))
	require.Error(t, err)

	envelope, err = cmacrypto.EncryptWithPassphrase([]byte("secret"), []byte("secret"), cmacrypto.DefaultArgon2idParams(), nil)
	require.NoError(t, err)
	envelope.KDF.Name = "pbkdf2"
	_, err = cmacrypto.DecryptWithPassphrase(envelope, []byte("secret"))
	require.Error(t, err)

	envelope.KDF.Name = cmacrypto.KDFNameArgon2id
	envelope.KDF.Salt = "!!!"
	_, err = cmacrypto.DecryptWithPassphrase(envelope, []byte("secret"))
	require.Error(t, err)
}

func TestMarshalUnmarshalEnvelope(t *testing.T) {
	key, err := cmacrypto.RandomBytes(cmacrypto.KeyLength)
	require.NoError(t, err)
	envelope, err := cmacrypto.EncryptWithKey([]byte("secret"), key, map[string]string{"kind": "vault"})
	require.NoError(t, err)

	data, err := cmacrypto.MarshalEnvelope(envelope)
	require.NoError(t, err)

	decoded, err := cmacrypto.UnmarshalEnvelope(data)
	require.NoError(t, err)
	require.Equal(t, envelope.Version, decoded.Version)
	require.Equal(t, "vault", decoded.Metadata["kind"])
}
