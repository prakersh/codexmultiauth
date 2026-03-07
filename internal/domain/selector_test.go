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
