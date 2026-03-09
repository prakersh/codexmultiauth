package store_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/99designs/keyring"

	"github.com/prakersh/codexmultiauth/internal/domain"
	cmafs "github.com/prakersh/codexmultiauth/internal/infra/fs"
	"github.com/prakersh/codexmultiauth/internal/infra/store"
	"github.com/prakersh/codexmultiauth/test/testenv"
	"github.com/stretchr/testify/require"
)

func authFixture() []byte {
	return []byte(`{"auth_mode":"chatgpt","tokens":{"access_token":"token","refresh_token":"refresh","account_id":"acc-1"}}`)
}

func TestNormalizeAndValidateAuth(t *testing.T) {
	auth, canonical, err := store.NormalizeAndValidateAuth(authFixture())
	require.NoError(t, err)
	require.Equal(t, "chatgpt", auth.AuthMode)
	require.NotEmpty(t, canonical)
	require.NotEmpty(t, store.FingerprintAuth(canonical))
}

func TestNormalizeAndValidateAuth_Errors(t *testing.T) {
	_, _, err := store.NormalizeAndValidateAuth([]byte("{bad"))
	require.Error(t, err)

	_, _, err = store.NormalizeAndValidateAuth([]byte(`{"auth_mode":"chatgpt"}`))
	require.Error(t, err)
}

func TestCodexAuthStore_LoadPrefersFile(t *testing.T) {
	p := testenv.NewWithDisableKeyring(t, "").Paths

	require.NoError(t, os.MkdirAll(p.CodexHome, cmafs.DirMode))
	require.NoError(t, os.WriteFile(p.CodexAuth, authFixture(), cmafs.FileMode))

	ring := &fakeKeyring{values: map[string][]byte{
		store.CodexAuthKeyringService + "|" + store.CodexAuthKeyringAccount: []byte(`{"OPENAI_API_KEY":"sk-key"}`),
	}}

	record, err := store.NewCodexAuthStore(p, ring, store.NewConfigRepo(p)).Load(context.Background())
	require.NoError(t, err)
	require.Equal(t, domain.AuthStoreFile, record.StoreKind)
}

func TestCodexAuthStore_LoadFallsBackToKeyring(t *testing.T) {
	p := testenv.NewWithDisableKeyring(t, "").Paths

	ring := &fakeKeyring{values: map[string][]byte{
		store.CodexAuthKeyringService + "|" + store.CodexAuthKeyringAccount: authFixture(),
	}}

	record, err := store.NewCodexAuthStore(p, ring, store.NewConfigRepo(p)).Load(context.Background())
	require.NoError(t, err)
	require.Equal(t, domain.AuthStoreKeyring, record.StoreKind)
}

func TestCodexAuthStore_SaveWritesFileAndKeyringWhenEnabled(t *testing.T) {
	p := testenv.NewWithDisableKeyring(t, "").Paths

	ring := &fakeKeyring{}
	authStore := store.NewCodexAuthStore(p, ring, store.NewConfigRepo(p))
	require.NoError(t, authStore.Save(context.Background(), authFixture()))

	info, err := os.Stat(p.CodexAuth)
	require.NoError(t, err)
	require.Equal(t, cmafs.FileMode, info.Mode().Perm())
	require.NotEmpty(t, ring.values[store.CodexAuthKeyringService+"|"+store.CodexAuthKeyringAccount])
}

func TestCodexAuthStore_SaveSkipsKeyringWhenDisabled(t *testing.T) {
	p := testenv.NewWithDisableKeyring(t, "1").Paths

	ring := &fakeKeyring{}
	authStore := store.NewCodexAuthStore(p, ring, store.NewConfigRepo(p))
	require.NoError(t, authStore.Save(context.Background(), authFixture()))

	_, ok := ring.values[store.CodexAuthKeyringService+"|"+store.CodexAuthKeyringAccount]
	require.False(t, ok)
}

func TestCodexAuthStore_DeleteIgnoresMissingKeyring(t *testing.T) {
	p := testenv.NewWithDisableKeyring(t, "").Paths

	ring := &fakeKeyring{delErr: keyring.ErrKeyNotFound}
	authStore := store.NewCodexAuthStore(p, ring, store.NewConfigRepo(p))

	require.NoError(t, authStore.Delete(context.Background()))
}

func TestCodexAuthStore_ConfigAndErrorPaths(t *testing.T) {
	p := testenv.NewWithDisableKeyring(t, "").Paths

	require.NoError(t, os.MkdirAll(p.CodexHome, cmafs.DirMode))
	require.NoError(t, os.WriteFile(filepath.Join(p.CodexHome, "config.toml"), []byte("cli_auth_credentials_store = 'file'\n"), cmafs.FileMode))

	ring := &fakeKeyring{values: map[string][]byte{
		store.CodexAuthKeyringService + "|" + store.CodexAuthKeyringAccount: authFixture(),
	}}
	authStore := store.NewCodexAuthStore(p, ring, store.NewConfigRepo(p))
	_, err := authStore.Load(context.Background())
	require.ErrorIs(t, err, os.ErrNotExist)

	require.NoError(t, os.WriteFile(filepath.Join(p.CodexHome, "config.toml"), []byte("cli_auth_credentials_store = 'keyring'\n"), cmafs.FileMode))
	record, err := authStore.Load(context.Background())
	require.NoError(t, err)
	require.Equal(t, domain.AuthStoreKeyring, record.StoreKind)

	require.NoError(t, os.WriteFile(filepath.Join(p.CodexHome, "config.toml"), []byte("bad = ["), cmafs.FileMode))
	require.NoError(t, authStore.Save(context.Background(), authFixture()))
	require.NoError(t, os.Remove(p.CodexAuth))

	invalidRing := &fakeKeyring{values: map[string][]byte{
		store.CodexAuthKeyringService + "|" + store.CodexAuthKeyringAccount: []byte(`{"auth_mode":"chatgpt"}`),
	}}
	invalidStore := store.NewCodexAuthStore(p, invalidRing, store.NewConfigRepo(p))
	_, err = invalidStore.Load(context.Background())
	require.ErrorIs(t, err, os.ErrNotExist)
}

func TestCodexAuthStore_SaveDeleteErrors(t *testing.T) {
	p := testenv.NewWithDisableKeyring(t, "").Paths

	ring := &fakeKeyring{setErr: errors.New("boom")}
	authStore := store.NewCodexAuthStore(p, ring, store.NewConfigRepo(p))
	err := authStore.Save(context.Background(), authFixture())
	require.Error(t, err)

	require.NoError(t, os.RemoveAll(p.CodexHome))
	require.NoError(t, os.MkdirAll(p.CodexAuth, cmafs.DirMode))
	require.NoError(t, os.WriteFile(filepath.Join(p.CodexAuth, "child"), []byte("x"), cmafs.FileMode))
	err = authStore.Delete(context.Background())
	require.Error(t, err)
}
