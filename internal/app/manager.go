package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/google/uuid"

	"github.com/prakersh/codexmultiauth/internal/domain"
	cmafs "github.com/prakersh/codexmultiauth/internal/infra/fs"
	"github.com/prakersh/codexmultiauth/internal/infra/paths"
	"github.com/prakersh/codexmultiauth/internal/infra/store"
)

type AuthStore interface {
	Load(ctx context.Context) (store.AuthRecord, error)
	Save(ctx context.Context, raw []byte) error
	Delete(ctx context.Context) error
}

type StateRepository interface {
	Load() (domain.State, error)
	Save(state domain.State) error
}

type VaultRepository interface {
	Load(key []byte) (store.Vault, error)
	Save(vault store.Vault, key []byte) error
}

type KeyManager interface {
	LoadOrCreate(ctx context.Context) ([]byte, store.VaultKeyProviderKind, error)
}

type LockManager interface {
	Acquire(ctx context.Context, path string) (cmafs.Unlocker, error)
}

type CodexCLI interface {
	Login(ctx context.Context, deviceAuth bool) error
	Status(ctx context.Context) (string, error)
}

type UsageFetcher interface {
	Fetch(ctx context.Context, auth store.CodexAuth) (domain.UsageSummary, error)
}

type Manager struct {
	paths       paths.Paths
	authStore   AuthStore
	stateRepo   StateRepository
	vaultRepo   VaultRepository
	keyManager  KeyManager
	lockManager LockManager
	codexCLI    CodexCLI
	usage       UsageFetcher
	now         func() time.Time
	newID       func() string
}

func NewManager(
	p paths.Paths,
	authStore AuthStore,
	stateRepo StateRepository,
	vaultRepo VaultRepository,
	keyManager KeyManager,
	lockManager LockManager,
	codexCLI CodexCLI,
) *Manager {
	return &Manager{
		paths:       p,
		authStore:   authStore,
		stateRepo:   stateRepo,
		vaultRepo:   vaultRepo,
		keyManager:  keyManager,
		lockManager: lockManager,
		codexCLI:    codexCLI,
		usage:       nil,
		now:         func() time.Time { return time.Now().UTC() },
		newID:       uuid.NewString,
	}
}

func (m *Manager) SetUsageFetcher(fetcher UsageFetcher) {
	m.usage = fetcher
}

func (m *Manager) withMutationLock(ctx context.Context, fn func() error) error {
	lockPath := m.paths.LockDir + "/cma.lock"
	lock, err := m.lockManager.Acquire(ctx, lockPath)
	if err != nil {
		return err
	}
	defer func() { _ = lock.Unlock() }()
	return fn()
}

func (m *Manager) loadStateAndVault(ctx context.Context) (domain.State, store.Vault, []byte, error) {
	key, _, err := m.keyManager.LoadOrCreate(ctx)
	if err != nil {
		return domain.State{}, store.Vault{}, nil, err
	}
	state, err := m.stateRepo.Load()
	if err != nil {
		return domain.State{}, store.Vault{}, nil, err
	}
	vault, err := m.vaultRepo.Load(key)
	if err != nil {
		return domain.State{}, store.Vault{}, nil, err
	}
	return state, vault, key, nil
}

func (m *Manager) commitStateAndVault(state domain.State, vault store.Vault, key []byte) error {
	originalState, stateExists, err := readOptionalFile(m.paths.StateFile)
	if err != nil {
		return err
	}
	originalVault, vaultExists, err := readOptionalFile(m.paths.VaultFile)
	if err != nil {
		return err
	}

	if err := m.vaultRepo.Save(vault, key); err != nil {
		return err
	}
	if err := m.stateRepo.Save(state); err != nil {
		rollbackErr := restoreOptionalFile(m.paths.VaultFile, originalVault, vaultExists)
		if rollbackErr != nil {
			return errors.Join(err, rollbackErr)
		}
		return err
	}

	if err := verifyStateAndVault(m.stateRepo, m.vaultRepo, key, state, vault); err != nil {
		restoreStateErr := restoreOptionalFile(m.paths.StateFile, originalState, stateExists)
		restoreVaultErr := restoreOptionalFile(m.paths.VaultFile, originalVault, vaultExists)
		return errors.Join(err, restoreStateErr, restoreVaultErr)
	}
	return nil
}

func verifyStateAndVault(stateRepo StateRepository, vaultRepo VaultRepository, key []byte, wantState domain.State, wantVault store.Vault) error {
	gotState, err := stateRepo.Load()
	if err != nil {
		return fmt.Errorf("verify state load: %w", err)
	}
	gotVault, err := vaultRepo.Load(key)
	if err != nil {
		return fmt.Errorf("verify vault load: %w", err)
	}
	if len(gotState.Accounts) != len(wantState.Accounts) || gotState.ActiveAccountID != wantState.ActiveAccountID {
		return errors.New("state verification mismatch")
	}
	if len(gotVault.Entries) != len(wantVault.Entries) {
		return errors.New("vault verification mismatch")
	}
	return nil
}

func readOptionalFile(path string) ([]byte, bool, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("read %s: %w", path, err)
	}
	return data, true, nil
}

func restoreOptionalFile(path string, data []byte, existed bool) error {
	if !existed {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("remove %s during rollback: %w", path, err)
		}
		return nil
	}
	return cmafs.WriteFileAtomic(path, data, cmafs.AtomicWriteOptions{Mode: cmafs.FileMode})
}

func findVaultEntry(vault store.Vault, accountID string) (store.VaultEntry, bool) {
	for _, entry := range vault.Entries {
		if entry.AccountID == accountID {
			return entry, true
		}
	}
	return store.VaultEntry{}, false
}

func removeVaultEntry(vault store.Vault, accountID string) store.Vault {
	filtered := store.Vault{Version: vault.Version}
	for _, entry := range vault.Entries {
		if entry.AccountID != accountID {
			filtered.Entries = append(filtered.Entries, entry)
		}
	}
	return filtered
}

func upsertAccount(state domain.State, account domain.Account) domain.State {
	for i, existing := range state.Accounts {
		if existing.ID == account.ID {
			state.Accounts[i] = account
			return state
		}
	}
	state.Accounts = append(state.Accounts, account)
	return state
}

func removeAccount(state domain.State, accountID string) domain.State {
	filtered := state
	filtered.Accounts = nil
	for _, account := range state.Accounts {
		if account.ID != accountID {
			filtered.Accounts = append(filtered.Accounts, account)
		}
	}
	if filtered.ActiveAccountID == accountID {
		filtered.ActiveAccountID = ""
	}
	return filtered
}
