package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sync"
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
	AcquireShared(ctx context.Context, path string) (cmafs.Unlocker, error)
}

type CodexCLI interface {
	Login(ctx context.Context, deviceAuth bool, withAPIKey bool) error
	Status(ctx context.Context) (string, error)
}

type UsageFetcher interface {
	Fetch(ctx context.Context, auth store.CodexAuth) (domain.UsageSummary, error)
}

type TokenRefresher interface {
	MaybeRefresh(ctx context.Context, auth store.CodexAuth) (store.CodexAuth, bool, error)
	Refresh(ctx context.Context, auth store.CodexAuth) (store.CodexAuth, bool, error)
}

type Manager struct {
	paths          paths.Paths
	authStore      AuthStore
	stateRepo      StateRepository
	vaultRepo      VaultRepository
	keyManager     KeyManager
	lockManager    LockManager
	codexCLI       CodexCLI
	usage          UsageFetcher
	tokenRefresher TokenRefresher
	now            func() time.Time
	newID          func() string

	// Per-account refresh serialization. See ensureFreshAuth.
	refreshMuGuard sync.Mutex
	refreshMuMap   map[string]*sync.Mutex
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
		paths:          p,
		authStore:      authStore,
		stateRepo:      stateRepo,
		vaultRepo:      vaultRepo,
		keyManager:     keyManager,
		lockManager:    lockManager,
		codexCLI:       codexCLI,
		usage:          nil,
		tokenRefresher: nil,
		now:            func() time.Time { return time.Now().UTC() },
		newID:          uuid.NewString,
	}
}

func (m *Manager) SetUsageFetcher(fetcher UsageFetcher) {
	m.usage = fetcher
}

func (m *Manager) SetTokenRefresher(refresher TokenRefresher) {
	m.tokenRefresher = refresher
}

func (m *Manager) lockPath() string {
	return m.paths.LockDir + "/cma.lock"
}

func (m *Manager) withMutationLock(ctx context.Context, fn func() error) error {
	lockPath := m.lockPath()
	lock, err := m.lockManager.Acquire(ctx, lockPath)
	if err != nil {
		return err
	}
	defer func() { _ = lock.Unlock() }()
	// Checked under the lock, not before it. A mutation that passed the check
	// and then waited on the lock would otherwise proceed even though the
	// holder marked the state torn while it waited.
	if err := m.checkTornState(); err != nil {
		return err
	}
	return fn()
}

// ErrTornState is returned when a prior mutation left state and vault in an
// inconsistent state that automatic rollback could not repair. The user must
// run `cma doctor` to re-verify and clear the flag before any further
// mutations are accepted.
var ErrTornState = errors.New("state and vault are in an inconsistent (torn) state; run `cma doctor` to verify and recover")

func (m *Manager) checkTornState() error {
	if m.paths.TornFile == "" {
		return nil
	}
	if _, err := os.Stat(m.paths.TornFile); err == nil {
		return ErrTornState
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("check torn-state marker %s: %w", m.paths.TornFile, err)
	}
	return nil
}

func (m *Manager) markTornState(cause error) {
	if m.paths.TornFile == "" {
		return
	}
	payload := fmt.Sprintf("torn at %s\n%v\n", m.now().Format(time.RFC3339Nano), cause)
	_ = cmafs.WriteFileAtomic(m.paths.TornFile, []byte(payload), cmafs.AtomicWriteOptions{Mode: cmafs.FileMode})
}

func (m *Manager) clearTornState() error {
	if m.paths.TornFile == "" {
		return nil
	}
	if err := os.Remove(m.paths.TornFile); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("clear torn-state marker %s: %w", m.paths.TornFile, err)
	}
	return nil
}

// loadStateAndVault reads the pair under a shared lock. Reading them unlocked
// let a reader get past state.json, have a lock-holding mutation commit both
// files, and then read the new vault: the resulting snapshot references an
// account whose entry is already gone, which made cma usage hard-error and
// cma doctor report a torn state that was never on disk.
//
// The shared lock admits concurrent readers and only excludes the exclusive
// lock a mutation takes. Callers already holding that exclusive lock must use
// loadStateAndVaultLocked instead, or they will deadlock against themselves.
func (m *Manager) loadStateAndVault(ctx context.Context) (domain.State, store.Vault, []byte, error) {
	lock, err := m.lockManager.AcquireShared(ctx, m.lockPath())
	if err != nil {
		return domain.State{}, store.Vault{}, nil, err
	}
	defer func() { _ = lock.Unlock() }()
	return m.loadStateAndVaultLocked(ctx)
}

func (m *Manager) loadStateAndVaultLocked(ctx context.Context) (domain.State, store.Vault, []byte, error) {
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

	// The dangerous crash residue is a state pointer to a missing vault row:
	// it makes `cma usage` hard-error and `cma doctor` fail. An inert vault
	// entry with no pointer is harmless by comparison. Which write order
	// avoids it depends on the direction of the change, so pick per commit
	// rather than always writing vault first.
	//
	// Adding: vault first, so a published pointer is always already backed.
	// Removing: state first, so the pointer is gone before its row is.
	//
	// A commit that both adds and removes cannot be made safe by ordering
	// alone; those take the add-safe order, and the removal half is covered by
	// the rollback below and by `cma doctor`.
	stateFirst := vaultOnlyRemovesEntries(originalVault, vaultExists, vault)

	saveVault := func() error { return m.vaultRepo.Save(vault, key) }
	saveState := func() error { return m.stateRepo.Save(state) }

	first, second := saveVault, saveState
	secondRollbackPath, secondRollbackData, secondRollbackExists := m.paths.VaultFile, originalVault, vaultExists
	if stateFirst {
		first, second = saveState, saveVault
		secondRollbackPath, secondRollbackData, secondRollbackExists = m.paths.StateFile, originalState, stateExists
	}

	if err := first(); err != nil {
		return err
	}
	if err := second(); err != nil {
		rollbackErr := restoreOptionalFile(secondRollbackPath, secondRollbackData, secondRollbackExists)
		if rollbackErr != nil {
			joined := errors.Join(err, rollbackErr)
			m.markTornState(joined)
			return joined
		}
		return err
	}

	if err := verifyStateAndVault(m.stateRepo, m.vaultRepo, key, state, vault); err != nil {
		restoreStateErr := restoreOptionalFile(m.paths.StateFile, originalState, stateExists)
		restoreVaultErr := restoreOptionalFile(m.paths.VaultFile, originalVault, vaultExists)
		if restoreStateErr != nil || restoreVaultErr != nil {
			joined := errors.Join(err, restoreStateErr, restoreVaultErr)
			m.markTornState(joined)
			return joined
		}
		return err
	}
	return nil
}

// vaultOnlyRemovesEntries reports whether the pending vault drops entries that
// are on disk without introducing any new ones. Account IDs sit in plaintext
// beside each ciphertext, so this needs no key.
func vaultOnlyRemovesEntries(originalVault []byte, vaultExists bool, next store.Vault) bool {
	if !vaultExists {
		return false
	}
	var current struct {
		Entries []struct {
			AccountID string `json:"account_id"`
		} `json:"entries"`
	}
	if err := json.Unmarshal(originalVault, &current); err != nil {
		// Unreadable on-disk vault: keep the add-safe order.
		return false
	}

	nextIDs := make(map[string]struct{}, len(next.Entries))
	for _, entry := range next.Entries {
		nextIDs[entry.AccountID] = struct{}{}
	}
	currentIDs := make(map[string]struct{}, len(current.Entries))
	for _, entry := range current.Entries {
		currentIDs[entry.AccountID] = struct{}{}
	}

	for id := range nextIDs {
		if _, ok := currentIDs[id]; !ok {
			return false // adds something, so use the add-safe order
		}
	}
	return len(nextIDs) < len(currentIDs)
}

// Doctor inspects on-disk state and vault for consistency. If they verify
// cleanly, it clears any torn-state marker left by a prior failed rollback
// and returns a human-readable status string.
//
// Doctor takes the mutation lock for the whole check so it cannot read a
// half-committed state/vault pair, and so it cannot clear the marker while a
// mutation is still in flight. Callers must not already hold the lock.
func (m *Manager) Doctor(ctx context.Context) (string, error) {
	lock, lockErr := m.lockManager.Acquire(ctx, m.lockPath())
	if lockErr != nil {
		return "", fmt.Errorf("doctor: acquire lock: %w", lockErr)
	}
	defer func() { _ = lock.Unlock() }()

	key, _, err := m.keyManager.LoadOrCreate(ctx)
	if err != nil {
		return "", fmt.Errorf("doctor: load key: %w", err)
	}
	state, err := m.stateRepo.Load()
	if err != nil {
		return "", fmt.Errorf("doctor: load state: %w", err)
	}
	vault, err := m.vaultRepo.Load(key)
	if err != nil {
		return "", fmt.Errorf("doctor: load vault: %w", err)
	}
	if err := checkStateVaultInvariants(state, vault); err != nil {
		return "", fmt.Errorf("doctor: %w", err)
	}
	if err := m.clearTornState(); err != nil {
		return "", err
	}

	// Integrity is reported rather than enforced. The ciphertext is
	// authenticated by the AEAD, but the surrounding entry fields and
	// state.json are plaintext, so a mismatch means an entry was relabelled or
	// swapped outside CMA. Reporting keeps a legitimately drifted vault
	// usable while still surfacing tampering.
	summary := fmt.Sprintf("ok: %d account(s), %d vault entry(ies)", len(state.Accounts), len(vault.Entries))
	if mismatches := checkVaultFingerprints(state, vault); len(mismatches) > 0 {
		summary += fmt.Sprintf("\nwarning: %d entry(ies) failed fingerprint verification:", len(mismatches))
		for _, mismatch := range mismatches {
			summary += "\n  " + mismatch
		}
	}
	return summary, nil
}

// checkVaultFingerprints recomputes each entry's fingerprint from its
// decrypted payload and compares it against the fingerprint stored beside the
// ciphertext and the one recorded in state.json.
func checkVaultFingerprints(state domain.State, vault store.Vault) []string {
	accounts := make(map[string]domain.Account, len(state.Accounts))
	for _, account := range state.Accounts {
		accounts[account.ID] = account
	}

	var mismatches []string
	for _, entry := range vault.Entries {
		_, canonical, err := store.NormalizeAndValidateAuth(entry.Payload)
		if err != nil {
			mismatches = append(mismatches, fmt.Sprintf("%s: payload does not parse as Codex auth", entry.AccountID))
			continue
		}
		actual := store.FingerprintAuth(canonical)
		if entry.Fingerprint != "" && entry.Fingerprint != actual {
			mismatches = append(mismatches, fmt.Sprintf("%s: vault entry fingerprint does not match its payload", entry.AccountID))
			continue
		}
		if account, ok := accounts[entry.AccountID]; ok && account.Fingerprint != "" && account.Fingerprint != actual {
			mismatches = append(mismatches, fmt.Sprintf("%s: state fingerprint does not match the stored payload", entry.AccountID))
		}
	}
	return mismatches
}

func checkStateVaultInvariants(state domain.State, vault store.Vault) error {
	vaultIDs := map[string]struct{}{}
	for _, entry := range vault.Entries {
		vaultIDs[entry.AccountID] = struct{}{}
	}
	for _, account := range state.Accounts {
		if _, ok := vaultIDs[account.ID]; !ok {
			return fmt.Errorf("state references account %q with no vault entry", account.ID)
		}
	}
	stateIDs := map[string]struct{}{}
	for _, account := range state.Accounts {
		stateIDs[account.ID] = struct{}{}
	}
	for _, entry := range vault.Entries {
		if _, ok := stateIDs[entry.AccountID]; !ok {
			return fmt.Errorf("vault contains orphan entry for account %q", entry.AccountID)
		}
	}
	if state.ActiveAccountID != "" {
		if _, ok := stateIDs[state.ActiveAccountID]; !ok {
			return fmt.Errorf("active account %q not present in state", state.ActiveAccountID)
		}
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
