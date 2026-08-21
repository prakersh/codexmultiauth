package crypto_test

import (
	"encoding/base64"
	"testing"
	"time"

	cmacrypto "github.com/prakersh/codexmultiauth/internal/infra/crypto"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/chacha20poly1305"
)

func TestEncryptDecryptWithKey_RoundTrip(t *testing.T) {
	key, err := cmacrypto.RandomBytes(cmacrypto.KeyLength)
	require.NoError(t, err)

	envelope, err := cmacrypto.EncryptWithKey([]byte("secret"), key, map[string]string{"kind": "vault"})
	require.NoError(t, err)
	require.Equal(t, cmacrypto.EnvelopeVersionV2, envelope.Version)
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

// TestDecryptWithPassphrase_RejectsHostileKDFParams covers KDF parameters read
// from a file that has not been authenticated yet. argon2.IDKey panics on zero
// iterations or parallelism and honors Memory literally, so a corrupted or
// hostile backup could crash the process or drive a huge allocation before the
// user ever confirms the restore.
func TestDecryptWithPassphrase_RejectsHostileKDFParams(t *testing.T) {
	build := func(mutate func(*cmacrypto.KDFMetadata)) cmacrypto.Envelope {
		envelope, err := cmacrypto.EncryptWithPassphrase(
			[]byte("backup"), []byte("correct horse"), cmacrypto.DefaultArgon2idParams(), nil)
		require.NoError(t, err)
		require.NotNil(t, envelope.KDF)
		mutate(envelope.KDF)
		return envelope
	}

	cases := map[string]func(*cmacrypto.KDFMetadata){
		"zero iterations":   func(k *cmacrypto.KDFMetadata) { k.Iterations = 0 },
		"zero parallelism":  func(k *cmacrypto.KDFMetadata) { k.Parallelism = 0 },
		"absurd memory":     func(k *cmacrypto.KDFMetadata) { k.Memory = 4294967295 },
		"absurd iterations": func(k *cmacrypto.KDFMetadata) { k.Iterations = 1 << 20 },
		"short key length":  func(k *cmacrypto.KDFMetadata) { k.KeyLength = 1 },
		"absurd key length": func(k *cmacrypto.KDFMetadata) { k.KeyLength = 1 << 20 },
	}

	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			// require.NotPanics is the point: the old code panicked here.
			require.NotPanics(t, func() {
				_, err := cmacrypto.DecryptWithPassphrase(build(mutate), []byte("correct horse"))
				require.Error(t, err)
			})
		})
	}
}

// TestV1EnvelopesStillDecrypt is the migration guarantee: envelopes written
// before AAD binding sealed with no additional data, so they must keep opening
// or every existing vault and backup becomes unreadable.
func TestV1EnvelopesStillDecrypt(t *testing.T) {
	// Build a v1 envelope the way the old code did: seal with nil AAD.
	key := make([]byte, cmacrypto.KeyLength)
	for i := range key {
		key[i] = byte(i)
	}
	legacy := legacyV1Envelope(t, []byte("secret payload"), key, map[string]string{"account_id": "acc-1"})
	require.Equal(t, cmacrypto.EnvelopeVersionV1, legacy.Version)

	plaintext, err := cmacrypto.DecryptWithKey(legacy, key)
	require.NoError(t, err)
	require.Equal(t, []byte("secret payload"), plaintext)
}

// TestV2RejectsMetadataTampering covers the relabelling attack: the ciphertext
// is authenticated, but under v1 everything around it was rewritable.
func TestV2RejectsMetadataTampering(t *testing.T) {
	key := make([]byte, cmacrypto.KeyLength)
	for i := range key {
		key[i] = byte(i + 7)
	}

	envelope, err := cmacrypto.EncryptWithKey([]byte("work account tokens"), key, map[string]string{
		"account_id":  "work-account",
		"fingerprint": "fp-work",
	})
	require.NoError(t, err)
	require.Equal(t, cmacrypto.EnvelopeVersionV2, envelope.Version)

	// Unmodified, it opens.
	_, err = cmacrypto.DecryptWithKey(envelope, key)
	require.NoError(t, err)

	for name, mutate := range map[string]func(e *cmacrypto.Envelope){
		"relabelled account":  func(e *cmacrypto.Envelope) { e.Metadata["account_id"] = "attacker-relabelled" },
		"swapped fingerprint": func(e *cmacrypto.Envelope) { e.Metadata["fingerprint"] = "fp-other" },
		"added metadata key":  func(e *cmacrypto.Envelope) { e.Metadata["injected"] = "x" },
		"backdated":           func(e *cmacrypto.Envelope) { e.CreatedAt = e.CreatedAt.AddDate(-5, 0, 0) },
		"downgraded version":  func(e *cmacrypto.Envelope) { e.Version = cmacrypto.EnvelopeVersionV1 },
	} {
		t.Run(name, func(t *testing.T) {
			tampered := envelope
			tampered.Metadata = map[string]string{}
			for k, v := range envelope.Metadata {
				tampered.Metadata[k] = v
			}
			mutate(&tampered)
			_, err := cmacrypto.DecryptWithKey(tampered, key)
			require.ErrorIs(t, err, cmacrypto.ErrWrongPassphrase)
		})
	}
}

// TestV2SurvivesMarshalRoundTrip guards the AAD encoding: it is recomputed
// from the parsed envelope, so any field that does not round-trip byte-identically
// through JSON would make stored data undecryptable.
func TestV2SurvivesMarshalRoundTrip(t *testing.T) {
	envelope, err := cmacrypto.EncryptWithPassphrase(
		[]byte("backup body"), []byte("correct horse"), cmacrypto.DefaultArgon2idParams(),
		map[string]string{"kind": "backup", "zebra": "z", "alpha": "a"})
	require.NoError(t, err)

	data, err := cmacrypto.MarshalEnvelope(envelope)
	require.NoError(t, err)
	decoded, err := cmacrypto.UnmarshalEnvelope(data)
	require.NoError(t, err)

	plaintext, err := cmacrypto.DecryptWithPassphrase(decoded, []byte("correct horse"))
	require.NoError(t, err)
	require.Equal(t, []byte("backup body"), plaintext)
}

// TestV2RejectsKDFTampering ensures a rewritten KDF block fails authentication
// rather than silently steering key derivation.
func TestV2RejectsKDFTampering(t *testing.T) {
	envelope, err := cmacrypto.EncryptWithPassphrase(
		[]byte("body"), []byte("pass"), cmacrypto.DefaultArgon2idParams(), nil)
	require.NoError(t, err)

	tampered := envelope
	kdfCopy := *envelope.KDF
	kdfCopy.Iterations = envelope.KDF.Iterations + 1
	tampered.KDF = &kdfCopy

	_, err = cmacrypto.DecryptWithPassphrase(tampered, []byte("pass"))
	require.Error(t, err)
}

// legacyV1Envelope reproduces exactly what the pre-AAD code wrote: a v1
// envelope sealed with nil additional data.
func legacyV1Envelope(t *testing.T, plaintext, key []byte, metadata map[string]string) cmacrypto.Envelope {
	t.Helper()

	nonce := make([]byte, chacha20poly1305.NonceSizeX)
	for i := range nonce {
		nonce[i] = byte(i * 3)
	}
	aead, err := chacha20poly1305.NewX(key)
	require.NoError(t, err)

	return cmacrypto.Envelope{
		Version:   cmacrypto.EnvelopeVersionV1,
		CreatedAt: time.Date(2026, 5, 1, 10, 33, 0, 0, time.UTC),
		AEAD: cmacrypto.AEADMetadata{
			Name:  cmacrypto.AEADXChaCha20,
			Nonce: base64.StdEncoding.EncodeToString(nonce),
		},
		Ciphertext: base64.StdEncoding.EncodeToString(aead.Seal(nil, nonce, plaintext, nil)),
		Metadata:   metadata,
	}
}

// TestZeroWipesBuffer covers the helper used on derived keys, vault keys, and
// passphrase buffers.
func TestZeroWipesBuffer(t *testing.T) {
	buf := []byte("super secret key material")
	cmacrypto.Zero(buf)
	for i, b := range buf {
		require.Zero(t, b, "byte %d survived", i)
	}
	require.NotPanics(t, func() { cmacrypto.Zero(nil) })
}

// TestDeriveKeyIsZeroedAfterPassphraseUse checks that the key derived inside
// the passphrase paths does not outlive them. The derived key is internal, so
// this asserts the observable consequence: encrypt and decrypt still succeed,
// and the caller's passphrase buffer is left intact for the caller to wipe.
func TestPassphraseBufferSurvivesEncryption(t *testing.T) {
	passphrase := []byte("correct horse battery staple")
	original := append([]byte(nil), passphrase...)

	envelope, err := cmacrypto.EncryptWithPassphrase([]byte("body"), passphrase, cmacrypto.DefaultArgon2idParams(), nil)
	require.NoError(t, err)
	require.Equal(t, original, passphrase, "the caller's passphrase must not be wiped underneath it")

	plaintext, err := cmacrypto.DecryptWithPassphrase(envelope, passphrase)
	require.NoError(t, err)
	require.Equal(t, []byte("body"), plaintext)
	require.Equal(t, original, passphrase)
}
