package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/prakersh/codexmultiauth/internal/domain"
	cmafs "github.com/prakersh/codexmultiauth/internal/infra/fs"
	"github.com/prakersh/codexmultiauth/internal/infra/paths"
	"github.com/prakersh/codexmultiauth/internal/infra/store"
	"github.com/stretchr/testify/require"
)

type funcAuthStore struct {
	load   func(context.Context) (store.AuthRecord, error)
	save   func(context.Context, []byte) error
	delete func(context.Context) error
}

func (f funcAuthStore) Load(ctx context.Context) (store.AuthRecord, error) {
	return f.load(ctx)
}

func (f funcAuthStore) Save(ctx context.Context, raw []byte) error {
	return f.save(ctx, raw)
}

func (f funcAuthStore) Delete(ctx context.Context) error {
	return f.delete(ctx)
}

type stubStateRepo struct {
	state   domain.State
	loadErr error
	saveErr error
}

func (s *stubStateRepo) Load() (domain.State, error) {
	if s.loadErr != nil {
		return domain.State{}, s.loadErr
	}
	return s.state, nil
}

func (s *stubStateRepo) Save(state domain.State) error {
	if s.saveErr != nil {
		return s.saveErr
	}
	s.state = state
	return nil
}

type stubVaultRepo struct {
	vault   store.Vault
	loadErr error
	saveErr error
}

func (s *stubVaultRepo) Load(key []byte) (store.Vault, error) {
	if s.loadErr != nil {
		return store.Vault{}, s.loadErr
	}
	return s.vault, nil
}

func (s *stubVaultRepo) Save(vault store.Vault, key []byte) error {
	if s.saveErr != nil {
		return s.saveErr
	}
	s.vault = vault
	return nil
}

type stubKeyManager struct {
	key []byte
	err error
}

func (s stubKeyManager) LoadOrCreate(ctx context.Context) ([]byte, store.VaultKeyProviderKind, error) {
	if s.err != nil {
		return nil, "", s.err
	}
	return s.key, store.VaultKeyProviderFile, nil
}

type noopUnlocker struct{}

func (noopUnlocker) Unlock() error { return nil }

type noopLockManager struct {
	err error
}

func (n noopLockManager) Acquire(ctx context.Context, path string) (cmafs.Unlocker, error) {
	if n.err != nil {
		return nil, n.err
	}
	return noopUnlocker{}, nil
}

func TestActivateRollbackOnFingerprintMismatch(t *testing.T) {
	manager, authStore, _ := newTestManager(t)
	ctx := context.Background()

	authStore.setRaw(t, []byte(`{"auth_mode":"chatgpt","tokens":{"access_token":"target","refresh_token":"refresh-target","account_id":"acc-target"}}`), domain.AuthStoreFile)
	saved, err := manager.Save(ctx, SaveInput{DisplayName: "target"})
	require.NoError(t, err)

	authStore.setRaw(t, []byte(`{"auth_mode":"chatgpt","tokens":{"access_token":"original","refresh_token":"refresh-original","account_id":"acc-original"}}`), domain.AuthStoreFile)

	key, _, err := manager.keyManager.LoadOrCreate(ctx)
	require.NoError(t, err)
	vault, err := manager.vaultRepo.Load(key)
	require.NoError(t, err)
	vault.Entries[0].Payload = []byte(`{"auth_mode":"chatgpt","tokens":{"access_token":"wrong","refresh_token":"wrong","account_id":"acc-wrong"}}`)
	require.NoError(t, manager.vaultRepo.Save(vault, key))

	_, err = manager.Activate(ctx, saved.Account.ID)
	require.Error(t, err)

	record, loadErr := authStore.Load(ctx)
	require.NoError(t, loadErr)
	require.Contains(t, string(record.Canonical), `"access_token":"original"`)
}

func TestActivateFailsForMissingVaultEntry(t *testing.T) {
	manager, authStore, _ := newTestManager(t)
	ctx := context.Background()

	authStore.setRaw(t, []byte(`{"auth_mode":"chatgpt","tokens":{"access_token":"token","refresh_token":"refresh","account_id":"acc-1"}}`), domain.AuthStoreFile)
	saved, err := manager.Save(ctx, SaveInput{DisplayName: "work"})
	require.NoError(t, err)

	key, _, err := manager.keyManager.LoadOrCreate(ctx)
	require.NoError(t, err)
	require.NoError(t, manager.vaultRepo.Save(store.Vault{Version: store.VaultVersionV1}, key))

	_, err = manager.Activate(ctx, saved.Account.ID)
	require.Error(t, err)
}

func TestNewErrorBranches(t *testing.T) {
	t.Run("requires codex cli", func(t *testing.T) {
		manager, _, _ := newTestManager(t)
		manager.codexCLI = nil
		_, err := manager.New(context.Background(), NewInput{})
		require.Error(t, err)
	})

	t.Run("restores original auth when saving new auth fails", func(t *testing.T) {
		t.Setenv("HOME", t.TempDir())
		t.Setenv("CMA_DISABLE_KEYRING", "1")
		p, err := paths.Resolve()
		require.NoError(t, err)
		configRepo := store.NewConfigRepo(p)
		currentRaw := []byte(`{"auth_mode":"chatgpt","tokens":{"access_token":"old","refresh_token":"refresh-old","account_id":"acc-old"}}`)
		parsed, canonical, err := store.NormalizeAndValidateAuth(currentRaw)
		require.NoError(t, err)
		current := store.AuthRecord{
			Canonical:   canonical,
			Fingerprint: store.FingerprintAuth(canonical),
			StoreKind:   domain.AuthStoreFile,
			Parsed:      parsed,
		}
		afterLogin := false
		authStore := funcAuthStore{
			load: func(ctx context.Context) (store.AuthRecord, error) {
				if afterLogin {
					return store.AuthRecord{}, os.ErrNotExist
				}
				return current, nil
			},
			save: func(ctx context.Context, raw []byte) error {
				parsed, canonical, err := store.NormalizeAndValidateAuth(raw)
				if err != nil {
					return err
				}
				current = store.AuthRecord{
					Canonical:   canonical,
					Fingerprint: store.FingerprintAuth(canonical),
					StoreKind:   domain.AuthStoreFile,
					Parsed:      parsed,
				}
				afterLogin = false
				return nil
			},
			delete: func(ctx context.Context) error {
				return nil
			},
		}
		manager := NewManager(
			p,
			authStore,
			store.NewStateRepo(p),
			store.NewVaultRepo(p),
			store.NewVaultKeyManager(p, configRepo, nil),
			cmafs.NewFileLockManager(),
			fakeCLI{login: func(ctx context.Context, deviceAuth bool) error {
				afterLogin = true
				return nil
			}},
		)

		_, err = manager.New(context.Background(), NewInput{DisplayName: "broken"})
		require.Error(t, err)

		record, loadErr := authStore.Load(context.Background())
		require.NoError(t, loadErr)
		require.Contains(t, string(record.Canonical), `"access_token":"old"`)
	})

	t.Run("rollback delete failure is surfaced when no original auth existed", func(t *testing.T) {
		manager, authStore, _ := newTestManager(t)
		ctx := context.Background()
		authStore.deleteErr = errors.New("delete failed")
		manager.codexCLI = fakeCLI{login: func(ctx context.Context, deviceAuth bool) error {
			return errors.New("login failed")
		}}

		_, err := manager.New(ctx, NewInput{})
		require.Error(t, err)
		require.Contains(t, err.Error(), "delete failed")
	})
}

func TestManagerHelperBranches(t *testing.T) {
	key := make([]byte, 32)
	p := paths.Paths{
		StateFile: filepath.Join(t.TempDir(), "state.json"),
		VaultFile: filepath.Join(t.TempDir(), "vault.json"),
		BackupDir: filepath.Join(t.TempDir(), "backups"),
	}

	stateRepo := &stubStateRepo{state: domain.State{Version: domain.StateVersionV1}}
	vaultRepo := &stubVaultRepo{vault: store.Vault{Version: store.VaultVersionV1}}

	manager := &Manager{
		paths:       p,
		stateRepo:   stateRepo,
		vaultRepo:   vaultRepo,
		keyManager:  stubKeyManager{key: key},
		lockManager: noopLockManager{},
	}

	_, _, _, err := manager.loadStateAndVault(context.Background())
	require.NoError(t, err)

	manager.keyManager = stubKeyManager{err: errors.New("key failed")}
	_, _, _, err = manager.loadStateAndVault(context.Background())
	require.Error(t, err)

	manager.keyManager = stubKeyManager{key: key}
	stateRepo.loadErr = errors.New("state failed")
	_, _, _, err = manager.loadStateAndVault(context.Background())
	require.Error(t, err)

	stateRepo.loadErr = nil
	vaultRepo.loadErr = errors.New("vault failed")
	_, _, _, err = manager.loadStateAndVault(context.Background())
	require.Error(t, err)

	stateRepo.loadErr = errors.New("load state")
	err = verifyStateAndVault(stateRepo, vaultRepo, key, domain.State{}, store.Vault{})
	require.Error(t, err)

	stateRepo.loadErr = nil
	vaultRepo.loadErr = errors.New("load vault")
	err = verifyStateAndVault(stateRepo, vaultRepo, key, domain.State{}, store.Vault{})
	require.Error(t, err)

	vaultRepo.loadErr = nil
	err = verifyStateAndVault(stateRepo, vaultRepo, key, domain.State{Accounts: []domain.Account{{ID: "a"}}}, store.Vault{})
	require.Error(t, err)

	stateRepo.state = domain.State{}
	vaultRepo.vault = store.Vault{Entries: []store.VaultEntry{{AccountID: "a"}}}
	err = verifyStateAndVault(stateRepo, vaultRepo, key, domain.State{}, store.Vault{})
	require.Error(t, err)
}

func TestReadRestoreAndResolvePathHelpers(t *testing.T) {
	manager, _, p := newTestManager(t)

	absolute := filepath.Join(t.TempDir(), "backup.cma.bak")
	require.Equal(t, absolute, manager.resolveRestorePath(absolute))
	require.Equal(t, filepath.Join(p.BackupDir, "named.cma.bak"), manager.resolveRestorePath("named"))

	_, _, err := readOptionalFile(t.TempDir())
	require.Error(t, err)

	blocked := filepath.Join(t.TempDir(), "non-empty")
	require.NoError(t, os.MkdirAll(blocked, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(blocked, "child"), []byte("x"), 0o600))
	err = restoreOptionalFile(blocked, nil, false)
	require.Error(t, err)
}

func TestInspectBackupStateLoadFailure(t *testing.T) {
	manager, authStore, _ := newTestManager(t)
	ctx := context.Background()
	authStore.setRaw(t, []byte(`{"auth_mode":"chatgpt","tokens":{"access_token":"token","refresh_token":"refresh","account_id":"acc-1"}}`), domain.AuthStoreFile)
	_, err := manager.Save(ctx, SaveInput{DisplayName: "work"})
	require.NoError(t, err)

	backupPath, err := manager.Backup(ctx, BackupInput{Passphrase: []byte("secret"), Target: "inspect-backup"})
	require.NoError(t, err)

	manager.stateRepo = &stubStateRepo{loadErr: errors.New("state failed")}
	_, _, err = manager.InspectBackup(RestoreInput{Passphrase: []byte("secret"), Source: backupPath})
	require.Error(t, err)
}
