package store_test

import (
	"context"
	"os"
	"errors"
	"path/filepath"
	"testing"

	"github.com/99designs/keyring"

	cmacrypto "github.com/prakersh/codexmultiauth/internal/infra/crypto"
	"github.com/prakersh/codexmultiauth/internal/infra/paths"
	"github.com/prakersh/codexmultiauth/internal/infra/store"
	"github.com/stretchr/testify/require"
)

type fakeKeyring struct {
	values  map[string][]byte
	getErr  error
	setErr  error
	delErr  error
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
	t.Setenv("HOME", t.TempDir())
	p, err := paths.Resolve()
	require.NoError(t, err)

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
	t.Setenv("HOME", t.TempDir())
	t.Setenv("CMA_DISABLE_KEYRING", "1")
	p, err := paths.Resolve()
	require.NoError(t, err)

	manager := store.NewVaultKeyManager(p, store.NewConfigRepo(p), &fakeKeyring{})

	key, kind, err := manager.LoadOrCreate(context.Background())
	require.NoError(t, err)
	require.Equal(t, store.VaultKeyProviderFile, kind)
	require.Len(t, key, cmacrypto.KeyLength)
}

func TestVaultKeyManager_FallsBackToFileOnKeyringFailure(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	p, err := paths.Resolve()
	require.NoError(t, err)

	manager := store.NewVaultKeyManager(p, store.NewConfigRepo(p), &fakeKeyring{getErr: errors.New("keyring down")})

	key, kind, err := manager.LoadOrCreate(context.Background())
	require.NoError(t, err)
	require.Equal(t, store.VaultKeyProviderFile, kind)
	require.Len(t, key, cmacrypto.KeyLength)
}

func TestVaultKeyManager_InvalidStoredKeyFallsBackToFile(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	p, err := paths.Resolve()
	require.NoError(t, err)

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
	t.Setenv("HOME", t.TempDir())
	p, err := paths.Resolve()
	require.NoError(t, err)

	manager := store.NewVaultKeyManager(p, store.NewConfigRepo(p), nil)
	require.NoError(t, os.MkdirAll(filepath.Dir(p.VaultKeyFile), 0o700))
	require.NoError(t, os.WriteFile(p.VaultKeyFile, []byte("{bad"), 0o600))
	_, _, err = manager.LoadOrCreate(context.Background())
	require.Error(t, err)

	require.NoError(t, os.Remove(p.VaultKeyFile))
	manager = store.NewVaultKeyManager(p, store.NewConfigRepo(p), &fakeKeyring{setErr: errors.New("no keyring write")})
	_, kind, err := manager.LoadOrCreate(context.Background())
	require.NoError(t, err)
	require.Equal(t, store.VaultKeyProviderFile, kind)
}
