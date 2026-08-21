package store_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/99designs/keyring"

	cmacrypto "github.com/prakersh/codexmultiauth/internal/infra/crypto"
	"github.com/prakersh/codexmultiauth/internal/infra/store"
	"github.com/prakersh/codexmultiauth/test/testenv"
	"github.com/stretchr/testify/require"
)

type fakeKeyring struct {
	values map[string][]byte
	getErr error
	setErr error
	delErr error
}

func (f *fakeKeyring) key(service, account string) string {
	return service + "|" + account
}

func (f *fakeKeyring) Get(service, account string) ([]byte, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}
	value, ok := f.values[f.key(service, account)]
	if !ok {
		return nil, keyring.ErrKeyNotFound
	}
	return value, nil
}

func (f *fakeKeyring) Set(service, account string, value []byte) error {
	if f.setErr != nil {
		return f.setErr
	}
	if f.values == nil {
		f.values = map[string][]byte{}
	}
	f.values[f.key(service, account)] = append([]byte(nil), value...)
	return nil
}

func (f *fakeKeyring) Delete(service, account string) error {
	if f.delErr != nil {
		return f.delErr
	}
	delete(f.values, f.key(service, account))
	return nil
}

func TestVaultKeyManager_UsesKeyringWhenAvailable(t *testing.T) {
	p := testenv.NewWithDisableKeyring(t, "").Paths

	ring := &fakeKeyring{}
	manager := store.NewVaultKeyManager(p, store.NewConfigRepo(p), ring)

	key, kind, err := manager.LoadOrCreate(context.Background())
	require.NoError(t, err)
	require.Equal(t, store.VaultKeyProviderKeyring, kind)
	require.Len(t, key, cmacrypto.KeyLength)

	second, secondKind, err := manager.LoadOrCreate(context.Background())
	require.NoError(t, err)
	require.Equal(t, store.VaultKeyProviderKeyring, secondKind)
	require.Equal(t, key, second)
}

func TestVaultKeyManager_UsesFileWhenKeyringDisabledByEnv(t *testing.T) {
	p := testenv.NewWithDisableKeyring(t, "1").Paths

	manager := store.NewVaultKeyManager(p, store.NewConfigRepo(p), &fakeKeyring{})

	key, kind, err := manager.LoadOrCreate(context.Background())
	require.NoError(t, err)
	require.Equal(t, store.VaultKeyProviderFile, kind)
	require.Len(t, key, cmacrypto.KeyLength)
}

func TestVaultKeyManager_FallsBackToFileOnKeyringFailure(t *testing.T) {
	p := testenv.NewWithDisableKeyring(t, "").Paths

	manager := store.NewVaultKeyManager(p, store.NewConfigRepo(p), &fakeKeyring{getErr: errors.New("keyring down")})

	key, kind, err := manager.LoadOrCreate(context.Background())
	require.NoError(t, err)
	require.Equal(t, store.VaultKeyProviderFile, kind)
	require.Len(t, key, cmacrypto.KeyLength)
}

func TestVaultKeyManager_InvalidStoredKeyFallsBackToFile(t *testing.T) {
	p := testenv.NewWithDisableKeyring(t, "").Paths

	ring := &fakeKeyring{values: map[string][]byte{
		store.CMAVaultKeyringService + "|" + store.CMAVaultKeyringAccount: []byte("short"),
	}}
	manager := store.NewVaultKeyManager(p, store.NewConfigRepo(p), ring)

	key, kind, err := manager.LoadOrCreate(context.Background())
	require.NoError(t, err)
	require.Equal(t, store.VaultKeyProviderFile, kind)
	require.Len(t, key, cmacrypto.KeyLength)
}

func TestVaultKeyManager_FileCorruptionAndSetFailure(t *testing.T) {
	p := testenv.NewWithDisableKeyring(t, "").Paths

	manager := store.NewVaultKeyManager(p, store.NewConfigRepo(p), nil)
	require.NoError(t, os.MkdirAll(filepath.Dir(p.VaultKeyFile), 0o700))
	require.NoError(t, os.WriteFile(p.VaultKeyFile, []byte("{bad"), 0o600))
	_, _, err := manager.LoadOrCreate(context.Background())
	require.Error(t, err)

	require.NoError(t, os.Remove(p.VaultKeyFile))
	manager = store.NewVaultKeyManager(p, store.NewConfigRepo(p), &fakeKeyring{setErr: errors.New("no keyring write")})
	_, kind, err := manager.LoadOrCreate(context.Background())
	require.NoError(t, err)
	require.Equal(t, store.VaultKeyProviderFile, kind)
}

func TestVaultKeyManager_IsDeterministicWhenExternalDisableKeyringIsPreset(t *testing.T) {
	t.Setenv("CMA_DISABLE_KEYRING", "1")
	p := testenv.NewWithDisableKeyring(t, "").Paths

	manager := store.NewVaultKeyManager(p, store.NewConfigRepo(p), &fakeKeyring{})
	_, kind, err := manager.LoadOrCreate(context.Background())
	require.NoError(t, err)
	require.Equal(t, store.VaultKeyProviderKeyring, kind)
}

// TestVaultKeyManager_DoesNotOrphanVaultAfterKeyringOutage reproduces the
// sequence that used to make a vault permanently undecryptable: a first run
// with no reachable keyring falls back to a file key and encrypts the vault
// under it, then a later run with a working but empty keyring minted a fresh
// key and returned that instead. The file key must win once it exists.
func TestVaultKeyManager_DoesNotOrphanVaultAfterKeyringOutage(t *testing.T) {
	p := testenv.NewWithDisableKeyring(t, "").Paths

	// Run 1: keyring unreachable (headless box, or a denied keychain prompt).
	brokenRing := &fakeKeyring{getErr: errors.New("Specified keyring backend not available")}
	firstKey, firstKind, err := store.NewVaultKeyManager(p, store.NewConfigRepo(p), brokenRing).
		LoadOrCreate(context.Background())
	require.NoError(t, err)
	require.Equal(t, store.VaultKeyProviderFile, firstKind)

	// Run 2: keyring now works but holds no key for CMA.
	workingRing := &fakeKeyring{}
	secondKey, secondKind, err := store.NewVaultKeyManager(p, store.NewConfigRepo(p), workingRing).
		LoadOrCreate(context.Background())
	require.NoError(t, err)
	require.Equal(t, store.VaultKeyProviderFile, secondKind)
	require.Equal(t, firstKey, secondKey, "vault would be undecryptable under a newly minted key")

	// Nothing was written to the keyring, so run 3 stays on the file key too.
	require.Empty(t, workingRing.values)
}

// A keyring key that already exists still takes precedence, so the normal
// keyring-backed setup is unaffected by the fallback guard.
func TestVaultKeyManager_PrefersExistingKeyringKeyOverFileKey(t *testing.T) {
	p := testenv.NewWithDisableKeyring(t, "").Paths

	broken := &fakeKeyring{getErr: errors.New("backend unavailable")}
	fileKey, kind, err := store.NewVaultKeyManager(p, store.NewConfigRepo(p), broken).
		LoadOrCreate(context.Background())
	require.NoError(t, err)
	require.Equal(t, store.VaultKeyProviderFile, kind)

	stored := make([]byte, cmacrypto.KeyLength)
	for i := range stored {
		stored[i] = byte(i + 1)
	}
	ring := &fakeKeyring{}
	require.NoError(t, ring.Set(store.CMAVaultKeyringService, store.CMAVaultKeyringAccount, stored))

	key, kind, err := store.NewVaultKeyManager(p, store.NewConfigRepo(p), ring).
		LoadOrCreate(context.Background())
	require.NoError(t, err)
	require.Equal(t, store.VaultKeyProviderKeyring, kind)
	require.Equal(t, stored, key)
	require.NotEqual(t, fileKey, key)
}
