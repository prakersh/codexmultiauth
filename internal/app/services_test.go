package app

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/prakersh/codexmultiauth/internal/domain"
	cmafs "github.com/prakersh/codexmultiauth/internal/infra/fs"
	"github.com/prakersh/codexmultiauth/internal/infra/paths"
	"github.com/prakersh/codexmultiauth/internal/infra/store"
	"github.com/prakersh/codexmultiauth/test/testenv"
	"github.com/stretchr/testify/require"
)

type memoryAuthStore struct {
	record    store.AuthRecord
	exists    bool
	saveErr   error
	deleteErr error
}

func (m *memoryAuthStore) setRaw(t *testing.T, raw []byte, kind domain.AuthStoreKind) {
	t.Helper()
	parsed, canonical, err := store.NormalizeAndValidateAuth(raw)
	require.NoError(t, err)
	m.record = store.AuthRecord{
		Raw:         raw,
		Canonical:   canonical,
		Fingerprint: store.FingerprintAuth(canonical),
		StoreKind:   kind,
		Parsed:      parsed,
	}
	m.exists = true
}

func (m *memoryAuthStore) Load(ctx context.Context) (store.AuthRecord, error) {
	if !m.exists {
		return store.AuthRecord{}, os.ErrNotExist
	}
	return m.record, nil
}

func (m *memoryAuthStore) Save(ctx context.Context, raw []byte) error {
	if m.saveErr != nil {
		return m.saveErr
	}
	parsed, canonical, err := store.NormalizeAndValidateAuth(raw)
	if err != nil {
		return err
	}
	m.record = store.AuthRecord{
		Raw:         raw,
		Canonical:   canonical,
		Fingerprint: store.FingerprintAuth(canonical),
		StoreKind:   domain.AuthStoreFile,
		Parsed:      parsed,
	}
	m.exists = true
	return nil
}

func (m *memoryAuthStore) Delete(ctx context.Context) error {
	if m.deleteErr != nil {
		return m.deleteErr
	}
	m.exists = false
	return nil
}

type fakeCLI struct {
	login func(ctx context.Context, deviceAuth bool) error
}

func (f fakeCLI) Login(ctx context.Context, deviceAuth bool) error {
	return f.login(ctx, deviceAuth)
}

func (f fakeCLI) Status(ctx context.Context) (string, error) { return "", nil }

type fakeUsageFetcher struct {
	result domain.UsageSummary
	err    error
}

type brokenStateRepo struct{ StateRepository }

func (b brokenStateRepo) Save(state domain.State) error { return errors.New("save failed") }

func (f fakeUsageFetcher) Fetch(ctx context.Context, auth store.CodexAuth) (domain.UsageSummary, error) {
	return f.result, f.err
}

func newTestManager(t *testing.T) (*Manager, *memoryAuthStore, paths.Paths) {
	t.Helper()
	p := testenv.New(t).Paths
	configRepo := store.NewConfigRepo(p)
	authStore := &memoryAuthStore{}
	manager := NewManager(
		p,
		authStore,
		store.NewStateRepo(p),
		store.NewVaultRepo(p),
		store.NewVaultKeyManager(p, configRepo, nil),
		cmafs.NewFileLockManager(),
		fakeCLI{login: func(ctx context.Context, deviceAuth bool) error { return nil }},
	)
	return manager, authStore, p
}

func TestSaveListActivateDeleteWorkflow(t *testing.T) {
	manager, authStore, _ := newTestManager(t)
	ctx := context.Background()

	authStore.setRaw(t, []byte(`{"auth_mode":"chatgpt","tokens":{"access_token":"token-1","refresh_token":"refresh-1","account_id":"acc-1"}}`), domain.AuthStoreFile)
	saved, err := manager.Save(ctx, SaveInput{DisplayName: "work", Aliases: []string{"main", "main"}})
	require.NoError(t, err)
	require.False(t, saved.Deduplicated)

	authStore.setRaw(t, []byte(`{"auth_mode":"chatgpt","tokens":{"access_token":"token-2","refresh_token":"refresh-2","account_id":"acc-2"}}`), domain.AuthStoreFile)
	saved2, err := manager.Save(ctx, SaveInput{DisplayName: "personal"})
	require.NoError(t, err)

	listed, err := manager.List(ctx)
	require.NoError(t, err)
	require.Len(t, listed, 2)
	require.Equal(t, []string{"main"}, listed[0].Account.Aliases)

	activated, err := manager.Activate(ctx, saved.Account.ID)
	require.NoError(t, err)
	require.Equal(t, "work", activated.DisplayName)

	listed, err = manager.List(ctx)
	require.NoError(t, err)
	require.True(t, listed[0].IsActive)

	require.NoError(t, manager.Delete(ctx, DeleteInput{Selector: saved2.Account.ID}))
	listed, err = manager.List(ctx)
	require.NoError(t, err)
	require.Len(t, listed, 1)
}

func TestSaveDeduplicatesAndDefaultDisplayName(t *testing.T) {
	manager, authStore, _ := newTestManager(t)
	ctx := context.Background()

	authStore.setRaw(t, []byte(`{"auth_mode":"chatgpt","tokens":{"access_token":"token-1","refresh_token":"refresh-1","account_id":"account-abcdef123456"}}`), domain.AuthStoreFile)
	saved, err := manager.Save(ctx, SaveInput{})
	require.NoError(t, err)
	require.Contains(t, saved.Account.DisplayName, "account-")

	again, err := manager.Save(ctx, SaveInput{})
	require.NoError(t, err)
	require.True(t, again.Deduplicated)
}

func TestList_UsesSingularDeterministicActiveMarker(t *testing.T) {
	t.Run("prefers fingerprint match over divergent state active id", func(t *testing.T) {
		manager, authStore, _ := newTestManager(t)
		ctx := context.Background()

		authStore.setRaw(t, []byte(`{"auth_mode":"chatgpt","tokens":{"access_token":"token-1","refresh_token":"refresh-1","account_id":"acc-1"}}`), domain.AuthStoreFile)
		savedA, err := manager.Save(ctx, SaveInput{DisplayName: "work"})
		require.NoError(t, err)

		authStore.setRaw(t, []byte(`{"auth_mode":"chatgpt","tokens":{"access_token":"token-2","refresh_token":"refresh-2","account_id":"acc-2"}}`), domain.AuthStoreFile)
		savedB, err := manager.Save(ctx, SaveInput{DisplayName: "personal"})
		require.NoError(t, err)

		state, err := manager.stateRepo.Load()
		require.NoError(t, err)
		state.ActiveAccountID = savedA.Account.ID
		require.NoError(t, manager.stateRepo.Save(state))

		authStore.setRaw(t, []byte(`{"auth_mode":"chatgpt","tokens":{"access_token":"token-2","refresh_token":"refresh-2","account_id":"acc-2"}}`), domain.AuthStoreFile)

		listed, err := manager.List(ctx)
		require.NoError(t, err)
		require.Len(t, listed, 2)
		require.False(t, listed[0].IsActive)
		require.True(t, listed[1].IsActive)
		require.Equal(t, savedB.Account.ID, listed[1].Account.ID)
	})

	t.Run("falls back to state active id when auth fingerprint is unavailable", func(t *testing.T) {
		manager, authStore, _ := newTestManager(t)
		ctx := context.Background()

		authStore.setRaw(t, []byte(`{"auth_mode":"chatgpt","tokens":{"access_token":"token-1","refresh_token":"refresh-1","account_id":"acc-1"}}`), domain.AuthStoreFile)
		savedA, err := manager.Save(ctx, SaveInput{DisplayName: "work"})
		require.NoError(t, err)

		authStore.setRaw(t, []byte(`{"auth_mode":"chatgpt","tokens":{"access_token":"token-2","refresh_token":"refresh-2","account_id":"acc-2"}}`), domain.AuthStoreFile)
		_, err = manager.Save(ctx, SaveInput{DisplayName: "personal"})
		require.NoError(t, err)

		state, err := manager.stateRepo.Load()
		require.NoError(t, err)
		state.ActiveAccountID = savedA.Account.ID
		require.NoError(t, manager.stateRepo.Save(state))

		authStore.exists = false

		listed, err := manager.List(ctx)
		require.NoError(t, err)
		require.Len(t, listed, 2)
		require.True(t, listed[0].IsActive)
		require.False(t, listed[1].IsActive)
	})
}

func TestNewRollsBackOnLoginFailure(t *testing.T) {
	manager, authStore, _ := newTestManager(t)
	ctx := context.Background()
	authStore.setRaw(t, []byte(`{"auth_mode":"chatgpt","tokens":{"access_token":"old","refresh_token":"refresh-old","account_id":"acc-old"}}`), domain.AuthStoreFile)
	manager.codexCLI = fakeCLI{login: func(ctx context.Context, deviceAuth bool) error {
		return errors.New("login failed")
	}}

	_, err := manager.New(ctx, NewInput{DisplayName: "fresh", DeviceAuth: true})
	require.Error(t, err)

	record, loadErr := authStore.Load(ctx)
	require.NoError(t, loadErr)
	require.Contains(t, string(record.Canonical), `"access_token":"old"`)
}

func TestBackupInspectRestoreAndConflictDecisions(t *testing.T) {
	manager, authStore, _ := newTestManager(t)
	ctx := context.Background()
	authStore.setRaw(t, []byte(`{"auth_mode":"chatgpt","tokens":{"access_token":"token-1","refresh_token":"refresh-1","account_id":"acc-1"}}`), domain.AuthStoreFile)
	saved, err := manager.Save(ctx, SaveInput{DisplayName: "work"})
	require.NoError(t, err)

	backupPath, err := manager.Backup(ctx, BackupInput{Passphrase: []byte("secret"), Target: "unit-backup"})
	require.NoError(t, err)
	require.FileExists(t, backupPath)

	artifact, candidates, err := manager.InspectBackup(RestoreInput{Passphrase: []byte("secret"), Source: backupPath})
	require.NoError(t, err)
	require.Len(t, artifact.Accounts, 1)
	require.Len(t, candidates, 1)

	manager2, authStore2, _ := newTestManager(t)
	authStore2.setRaw(t, []byte(`{"auth_mode":"chatgpt","tokens":{"access_token":"seed","refresh_token":"seed","account_id":"seed"}}`), domain.AuthStoreFile)
	_, err = manager2.Save(ctx, SaveInput{DisplayName: "work"})
	require.NoError(t, err)

	summary, err := manager2.Restore(ctx, RestoreInput{
		Passphrase: []byte("secret"),
		Source:     backupPath,
		All:        true,
		Conflict:   domain.ConflictAsk,
		Decisions:  map[string]domain.ConflictPolicy{saved.Account.ID: domain.ConflictRename},
	})
	require.NoError(t, err)
	require.Equal(t, 1, summary.Imported)
}

func TestUsageConfirmedAndBestEffort(t *testing.T) {
	manager, authStore, _ := newTestManager(t)
	ctx := context.Background()
	authStore.setRaw(t, []byte(`{"auth_mode":"chatgpt","tokens":{"access_token":"token-1","refresh_token":"refresh-1","account_id":"acc-1"}}`), domain.AuthStoreFile)
	saved, err := manager.Save(ctx, SaveInput{DisplayName: "work"})
	require.NoError(t, err)

	manager.SetUsageFetcher(fakeUsageFetcher{result: domain.UsageSummary{Confidence: domain.UsageConfidenceConfirmed, PlanType: "team"}})
	results, err := manager.Usage(ctx, saved.Account.ID)
	require.NoError(t, err)
	require.Equal(t, domain.UsageConfidenceConfirmed, results[0].Usage.Confidence)

	bestEffortToken := buildJWT(t, map[string]any{
		"https://api.openai.com/auth": map[string]any{"chatgpt_plan_type": "plus"},
	})
	authStore.setRaw(t, []byte(`{"auth_mode":"chatgpt","tokens":{"access_token":"`+bestEffortToken+`","refresh_token":"refresh-2","account_id":"acc-2"}}`), domain.AuthStoreFile)
	_, err = manager.Save(ctx, SaveInput{DisplayName: "fallback"})
	require.NoError(t, err)
	manager.SetUsageFetcher(fakeUsageFetcher{err: errors.New("offline")})
	results, err = manager.Usage(ctx, "fallback")
	require.NoError(t, err)
	require.Equal(t, domain.UsageConfidenceBestEffort, results[0].Usage.Confidence)
}

func TestHelperFunctions(t *testing.T) {
	manager, _, p := newTestManager(t)

	data, existed, err := readOptionalFile(filepath.Join(t.TempDir(), "missing.json"))
	require.NoError(t, err)
	require.False(t, existed)
	require.Nil(t, data)

	file := filepath.Join(t.TempDir(), "restored.json")
	require.NoError(t, restoreOptionalFile(file, []byte("hello"), true))
	loaded, err := os.ReadFile(file)
	require.NoError(t, err)
	require.Equal(t, "hello", string(loaded))
	require.NoError(t, restoreOptionalFile(filepath.Join(t.TempDir(), "absent.json"), nil, false))

	state, vault, key, err := manager.loadStateAndVault(context.Background())
	require.NoError(t, err)
	require.Equal(t, domain.StateVersionV1, state.Version)
	require.Equal(t, store.VaultVersionV1, vault.Version)
	require.NotEmpty(t, key)
	require.FileExists(t, p.VaultKeyFile)
}

func TestDeleteActiveWithoutAllowanceAndUsageMissingEntry(t *testing.T) {
	manager, authStore, _ := newTestManager(t)
	ctx := context.Background()

	authStore.setRaw(t, []byte(`{"auth_mode":"chatgpt","tokens":{"access_token":"token-1","refresh_token":"refresh-1","account_id":"acc-1"}}`), domain.AuthStoreFile)
	saved, err := manager.Save(ctx, SaveInput{DisplayName: "work"})
	require.NoError(t, err)
	_, err = manager.Activate(ctx, saved.Account.ID)
	require.NoError(t, err)

	err = manager.Delete(ctx, DeleteInput{Selector: saved.Account.ID})
	require.Error(t, err)

	vault, key, err := func() (store.Vault, []byte, error) {
		k, _, err := manager.keyManager.LoadOrCreate(ctx)
		if err != nil {
			return store.Vault{}, nil, err
		}
		v, err := manager.vaultRepo.Load(k)
		return v, k, err
	}()
	require.NoError(t, err)
	_ = key
	vault.Entries = nil
	require.NoError(t, manager.vaultRepo.Save(vault, key))
	_, err = manager.Usage(ctx, saved.Account.ID)
	require.Error(t, err)
}

func TestCommitStateAndVaultRollbackAndFilterCandidates(t *testing.T) {
	manager, _, p := newTestManager(t)

	manager.stateRepo = brokenStateRepo{StateRepository: manager.stateRepo}
	key, _, err := manager.keyManager.LoadOrCreate(context.Background())
	require.NoError(t, err)
	err = manager.commitStateAndVault(domain.State{Version: domain.StateVersionV1}, store.Vault{Version: store.VaultVersionV1}, key)
	require.Error(t, err)
	_, statErr := os.Stat(p.StateFile)
	require.ErrorIs(t, statErr, os.ErrNotExist)

	filtered := filterCandidates([]RestoreCandidate{{Account: domain.Account{ID: "a"}}, {Account: domain.Account{ID: "b"}}}, []string{"b"})
	require.Len(t, filtered, 1)
	require.Equal(t, "b", filtered[0].Account.ID)
}

func buildJWT(t *testing.T, payload map[string]any) string {
	t.Helper()
	header, err := json.Marshal(map[string]string{"alg": "none", "typ": "JWT"})
	require.NoError(t, err)
	body, err := json.Marshal(payload)
	require.NoError(t, err)
	return base64.RawURLEncoding.EncodeToString(header) + "." + base64.RawURLEncoding.EncodeToString(body) + ".sig"
}
