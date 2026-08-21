package domain_test

import (
	"testing"
	"time"

	"github.com/prakersh/codexmultiauth/internal/domain"
	"github.com/stretchr/testify/require"
)

func testAccounts() []domain.Account {
	now := time.Now().UTC()
	return []domain.Account{
		{ID: "acc-123", DisplayName: "work", Aliases: []string{"w"}, Fingerprint: "fp-1", CreatedAt: now},
		{ID: "acc-999", DisplayName: "personal", Aliases: []string{"p"}, Fingerprint: "fp-2", CreatedAt: now},
	}
}

func TestResolveAccount_ByIndex(t *testing.T) {
	account, err := domain.ResolveAccount(testAccounts(), "2")
	require.NoError(t, err)
	require.Equal(t, "personal", account.DisplayName)
}

func TestResolveAccount_ByAlias(t *testing.T) {
	account, err := domain.ResolveAccount(testAccounts(), "w")
	require.NoError(t, err)
	require.Equal(t, "acc-123", account.ID)
}

func TestResolveAccount_ByUniquePrefix(t *testing.T) {
	account, err := domain.ResolveAccount(testAccounts(), "pers")
	require.NoError(t, err)
	require.Equal(t, "acc-999", account.ID)
}

func TestResolveAccount_AmbiguousPrefix(t *testing.T) {
	accounts := append(testAccounts(), domain.Account{ID: "acc-abc", DisplayName: "workbench", Fingerprint: "fp-3", CreatedAt: time.Now().UTC()})
	_, err := domain.ResolveAccount(accounts, "wo")
	require.ErrorIs(t, err, domain.ErrSelectorAmbiguous)
}

// TestResolveAccountRejectsAllSelector covers the destructive case: with a
// single saved account, "all" used to resolve, so `cma delete all` deleted the
// user's only account instead of being refused.
func TestResolveAccountRejectsAllSelector(t *testing.T) {
	one := []domain.Account{{ID: "a1", DisplayName: "work"}}
	_, err := domain.ResolveAccount(one, "all")
	require.ErrorIs(t, err, domain.ErrSelectorAmbiguous)
	require.Contains(t, err.Error(), "every account")

	_, err = domain.ResolveAccount(one, "ALL")
	require.ErrorIs(t, err, domain.ErrSelectorAmbiguous)

	// Bulk resolution still accepts it.
	many, err := domain.ResolveAccounts(one, "all")
	require.NoError(t, err)
	require.Len(t, many, 1)

	// An account genuinely named "all" is still reachable by index or ID.
	resolved, err := domain.ResolveAccount(one, "1")
	require.NoError(t, err)
	require.Equal(t, "a1", resolved.ID)
}

// TestResolveAccountAmbiguousErrorNamesSelector covers the bare sentinel that
// gave no clue which selector failed.
func TestResolveAccountAmbiguousErrorNamesSelector(t *testing.T) {
	accounts := []domain.Account{
		{ID: "a1", DisplayName: "work"},
		{ID: "a2", DisplayName: "worker"},
	}
	_, err := domain.ResolveAccount(accounts, "wor")
	require.Error(t, err)
	require.Contains(t, err.Error(), "wor")
}
