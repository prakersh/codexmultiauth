package app

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/prakersh/codexmultiauth/internal/domain"
	"github.com/prakersh/codexmultiauth/internal/infra/store"
	"github.com/stretchr/testify/require"
)

func identityAuth(t *testing.T, accountID, userID, email, accessToken string) []byte {
	t.Helper()
	claims := map[string]any{"email": email}
	if userID != "" {
		claims["sub"] = userID
		claims["https://api.openai.com/auth"] = map[string]any{"chatgpt_user_id": userID}
	}
	raw, err := json.Marshal(store.CodexAuth{
		AuthMode: "chatgpt",
		Tokens: &store.CodexTokens{
			AccountID: accountID, IDToken: buildJWT(t, claims),
			AccessToken: accessToken, RefreshToken: "refresh-" + accessToken,
		},
	})
	require.NoError(t, err)
	return raw
}

func TestSaveDoesNotMergeOpaqueUsersInSharedWorkspace(t *testing.T) {
	m, authStore, _ := newTestManager(t)
	ctx := context.Background()
	authStore.setRaw(t, []byte(`{"auth_mode":"chatgpt","tokens":{"access_token":"opaque-a","refresh_token":"refresh-a","account_id":"shared-workspace"}}`), domain.AuthStoreFile)
	first, err := m.Save(ctx, SaveInput{DisplayName: "first"})
	require.NoError(t, err)
	authStore.setRaw(t, []byte(`{"auth_mode":"chatgpt","tokens":{"access_token":"opaque-b","refresh_token":"refresh-b","account_id":"shared-workspace"}}`), domain.AuthStoreFile)
	second, err := m.Save(ctx, SaveInput{DisplayName: "second"})
	require.NoError(t, err)
	require.NotEqual(t, first.Account.ID, second.Account.ID)
	state, _, _, err := m.loadStateAndVault(ctx)
	require.NoError(t, err)
	require.Len(t, state.Accounts, 2)
	firstAuth, _, err := store.NormalizeAndValidateAuth(vaultPayloadFor(t, m, ctx, first.Account.ID))
	require.NoError(t, err)
	require.Equal(t, "opaque-a", firstAuth.Tokens.AccessToken)
}

func TestLoadAccountAuthUsesMatchingLiveLoginDespiteStaleState(t *testing.T) {
	m, authStore, _ := newTestManager(t)
	ctx := context.Background()
	authStore.setRaw(t, identityAuth(t, "account-a", "user-a", "a@example.com", "old-a"), domain.AuthStoreFile)
	a, err := m.Save(ctx, SaveInput{DisplayName: "a"})
	require.NoError(t, err)
	_, err = m.Activate(ctx, a.Account.ID)
	require.NoError(t, err)
	authStore.setRaw(t, identityAuth(t, "account-b", "user-b", "b@example.com", "old-b"), domain.AuthStoreFile)
	b, err := m.Save(ctx, SaveInput{DisplayName: "b"})
	require.NoError(t, err)
	authStore.setRaw(t, identityAuth(t, "account-b", "user-b", "b@example.com", "live-b"), domain.AuthStoreFile)

	auth, err := m.loadAccountAuth(ctx, b.Account.ID)
	require.NoError(t, err)
	require.Equal(t, "live-b", auth.Tokens.AccessToken)
	state, _, _, err := m.loadStateAndVault(ctx)
	require.NoError(t, err)
	require.Equal(t, a.Account.ID, state.ActiveAccountID, "reading auth should not mutate state")
}

func TestActivateCapturesMatchingManualLoginDespiteStaleState(t *testing.T) {
	m, authStore, _ := newTestManager(t)
	ctx := context.Background()
	authStore.setRaw(t, identityAuth(t, "account-a", "user-a", "a@example.com", "old-a"), domain.AuthStoreFile)
	a, err := m.Save(ctx, SaveInput{DisplayName: "a"})
	require.NoError(t, err)
	_, err = m.Activate(ctx, a.Account.ID)
	require.NoError(t, err)
	authStore.setRaw(t, identityAuth(t, "account-b", "user-b", "b@example.com", "old-b"), domain.AuthStoreFile)
	b, err := m.Save(ctx, SaveInput{DisplayName: "b"})
	require.NoError(t, err)
	authStore.setRaw(t, identityAuth(t, "account-b", "user-b", "b@example.com", "live-b"), domain.AuthStoreFile)
	_, err = m.Activate(ctx, a.Account.ID)
	require.NoError(t, err)
	stored, _, err := store.NormalizeAndValidateAuth(vaultPayloadFor(t, m, ctx, b.Account.ID))
	require.NoError(t, err)
	require.Equal(t, "live-b", stored.Tokens.AccessToken)
	_, err = m.Activate(ctx, b.Account.ID)
	require.NoError(t, err)
	live, err := authStore.Load(ctx)
	require.NoError(t, err)
	require.Equal(t, "live-b", live.Parsed.Tokens.AccessToken)
}

func TestRefreshPreservesDifferentLiveLogin(t *testing.T) {
	for _, sharedAccountID := range []bool{false, true} {
		name := "different account"
		if sharedAccountID {
			name = "different user in shared workspace"
		}
		t.Run(name, func(t *testing.T) {
			m, authStore, _ := newTestManager(t)
			ctx := context.Background()
			authStore.setRaw(t, identityAuth(t, "workspace-a", "user-a", "a@example.com", "old-a"), domain.AuthStoreFile)
			saved, err := m.Save(ctx, SaveInput{DisplayName: "a"})
			require.NoError(t, err)
			_, err = m.Activate(ctx, saved.Account.ID)
			require.NoError(t, err)
			liveAccountID := "workspace-b"
			if sharedAccountID {
				liveAccountID = "workspace-a"
			}
			authStore.setRaw(t, identityAuth(t, liveAccountID, "user-b", "b@example.com", "live-b"), domain.AuthStoreFile)
			before, err := authStore.Load(ctx)
			require.NoError(t, err)
			m.SetTokenRefresher(&countingRefresher{})

			auth, changed, err := m.ensureFreshAuth(ctx, saved.Account.ID, freshAuthForce)
			require.NoError(t, err)
			require.True(t, changed)
			require.Equal(t, "refreshed-1", auth.Tokens.AccessToken)
			after, err := authStore.Load(ctx)
			require.NoError(t, err)
			require.Equal(t, before.Fingerprint, after.Fingerprint)
			stored, _, err := store.NormalizeAndValidateAuth(vaultPayloadFor(t, m, ctx, saved.Account.ID))
			require.NoError(t, err)
			require.Equal(t, "refreshed-1", stored.Tokens.AccessToken)
		})
	}
}

func TestRefreshPreservesCodexRotationDuringTokenRequest(t *testing.T) {
	m, authStore, _ := newTestManager(t)
	ctx := context.Background()
	authStore.setRaw(t, identityAuth(t, "workspace", "user", "a@example.com", "old"), domain.AuthStoreFile)
	saved, err := m.Save(ctx, SaveInput{DisplayName: "work"})
	require.NoError(t, err)
	_, err = m.Activate(ctx, saved.Account.ID)
	require.NoError(t, err)
	m.SetTokenRefresher(fakeTokenRefresher{refresh: func(ctx context.Context, auth store.CodexAuth) (store.CodexAuth, bool, error) {
		// Codex writes a newer token while CMA's refresh request is in flight.
		authStore.setRaw(t, identityAuth(t, "workspace", "user", "a@example.com", "codex-new"), domain.AuthStoreFile)
		updated := auth
		tokens := *auth.Tokens
		tokens.AccessToken, tokens.RefreshToken = "cma-response", "refresh-cma-response"
		updated.Tokens = &tokens
		return updated, true, nil
	}})

	auth, changed, err := m.ensureFreshAuth(ctx, saved.Account.ID, freshAuthForce)
	require.NoError(t, err)
	require.False(t, changed, "a superseded server result should not count as a persisted refresh")
	require.Equal(t, "codex-new", auth.Tokens.AccessToken)
	current, err := authStore.Load(ctx)
	require.NoError(t, err)
	require.Equal(t, "codex-new", current.Parsed.Tokens.AccessToken)
	require.NotContains(t, string(vaultPayloadFor(t, m, ctx, saved.Account.ID)), "cma-response")
}

func TestRefreshPreservesSavedLoginDuringTokenRequest(t *testing.T) {
	m, authStore, _ := newTestManager(t)
	ctx := context.Background()
	authStore.setRaw(t, identityAuth(t, "workspace-a", "user-a", "a@example.com", "old-a"), domain.AuthStoreFile)
	saved, err := m.Save(ctx, SaveInput{DisplayName: "a"})
	require.NoError(t, err)
	otherLogin := identityAuth(t, "workspace-b", "user-b", "b@example.com", "live-b")
	authStore.setRaw(t, otherLogin, domain.AuthStoreFile)
	m.SetTokenRefresher(fakeTokenRefresher{refresh: func(ctx context.Context, auth store.CodexAuth) (store.CodexAuth, bool, error) {
		// A new sign-in is saved before the old token request finishes.
		authStore.setRaw(t, identityAuth(t, "workspace-a", "user-a", "a@example.com", "saved-new"), domain.AuthStoreFile)
		updated, err := m.Save(ctx, SaveInput{})
		require.NoError(t, err)
		require.Equal(t, saved.Account.ID, updated.Account.ID)
		authStore.setRaw(t, otherLogin, domain.AuthStoreFile)
		tokens := *auth.Tokens
		tokens.AccessToken = "cma-response"
		auth.Tokens = &tokens
		return auth, true, nil
	}})

	auth, changed, err := m.ensureFreshAuth(ctx, saved.Account.ID, freshAuthForce)
	require.NoError(t, err)
	require.False(t, changed)
	require.Equal(t, "saved-new", auth.Tokens.AccessToken)
	stored, _, err := store.NormalizeAndValidateAuth(vaultPayloadFor(t, m, ctx, saved.Account.ID))
	require.NoError(t, err)
	require.Equal(t, "saved-new", stored.Tokens.AccessToken)
	current, err := authStore.Load(ctx)
	require.NoError(t, err)
	require.Equal(t, "live-b", current.Parsed.Tokens.AccessToken)
}

func TestRefreshRollsBackLiveLoginWhenCommitFails(t *testing.T) {
	m, authStore, _ := newTestManager(t)
	ctx := context.Background()
	authStore.setRaw(t, identityAuth(t, "workspace", "user", "a@example.com", "old"), domain.AuthStoreFile)
	saved, err := m.Save(ctx, SaveInput{DisplayName: "work"})
	require.NoError(t, err)
	before, err := authStore.Load(ctx)
	require.NoError(t, err)
	m.stateRepo = brokenStateRepo{StateRepository: m.stateRepo}
	m.SetTokenRefresher(&countingRefresher{})

	_, changed, err := m.ensureFreshAuth(ctx, saved.Account.ID, freshAuthForce)
	require.ErrorContains(t, err, "save failed")
	require.False(t, changed)
	current, err := authStore.Load(ctx)
	require.NoError(t, err)
	require.Equal(t, before.Fingerprint, current.Fingerprint)
	stored, _, err := store.NormalizeAndValidateAuth(vaultPayloadFor(t, m, ctx, saved.Account.ID))
	require.NoError(t, err)
	require.Equal(t, "old", stored.Tokens.AccessToken)
}

func TestSaveRotatedLoginUpdatesExistingIdentity(t *testing.T) {
	m, authStore, _ := newTestManager(t)
	ctx := context.Background()
	authStore.setRaw(t, identityAuth(t, "workspace", "user", "a@example.com", "old"), domain.AuthStoreFile)
	saved, err := m.Save(ctx, SaveInput{DisplayName: "work", Aliases: []string{"main"}})
	require.NoError(t, err)
	activated, err := m.Activate(ctx, saved.Account.ID)
	require.NoError(t, err)
	authStore.setRaw(t, identityAuth(t, "workspace", "user", "a@example.com", "new"), domain.AuthStoreFile)

	updated, err := m.Save(ctx, SaveInput{})
	require.NoError(t, err)
	require.True(t, updated.Updated)
	require.Equal(t, activated.ID, updated.Account.ID)
	require.Equal(t, activated.DisplayName, updated.Account.DisplayName)
	require.Equal(t, activated.CreatedAt, updated.Account.CreatedAt)
	require.Equal(t, activated.LastUsedAt, updated.Account.LastUsedAt)
	require.Equal(t, activated.Aliases, updated.Account.Aliases)
	state, vault, _, err := m.loadStateAndVault(ctx)
	require.NoError(t, err)
	require.Len(t, state.Accounts, 1)
	require.Len(t, vault.Entries, 1)
	parsed, _, err := store.NormalizeAndValidateAuth(vault.Entries[0].Payload)
	require.NoError(t, err)
	require.Equal(t, "new", parsed.Tokens.AccessToken)
	require.Equal(t, updated.Account.Fingerprint, vault.Entries[0].Fingerprint)
}

func TestSaveRotatedLoginAppliesExplicitMetadata(t *testing.T) {
	m, authStore, _ := newTestManager(t)
	ctx := context.Background()
	authStore.setRaw(t, identityAuth(t, "workspace", "user", "a@example.com", "old"), domain.AuthStoreFile)
	saved, err := m.Save(ctx, SaveInput{DisplayName: "work", Aliases: []string{"main"}})
	require.NoError(t, err)
	authStore.setRaw(t, identityAuth(t, "workspace", "user", "a@example.com", "new"), domain.AuthStoreFile)

	updated, err := m.Save(ctx, SaveInput{DisplayName: "renamed", Aliases: []string{"new-alias"}})
	require.NoError(t, err)
	require.Equal(t, saved.Account.ID, updated.Account.ID)
	require.Equal(t, "renamed", updated.Account.DisplayName)
	require.Equal(t, []string{"new-alias"}, updated.Account.Aliases)
}

func TestSaveKeepsDistinctUsersInSharedWorkspace(t *testing.T) {
	for _, onlyEmail := range []bool{false, true} {
		name := "user id and email"
		if onlyEmail {
			name = "email only"
		}
		t.Run(name, func(t *testing.T) {
			m, authStore, _ := newTestManager(t)
			ctx := context.Background()
			firstID, secondID := "user-a", "user-b"
			if onlyEmail {
				firstID, secondID = "", ""
			}
			authStore.setRaw(t, identityAuth(t, "workspace", firstID, "a@example.com", "token-a"), domain.AuthStoreFile)
			first, err := m.Save(ctx, SaveInput{DisplayName: "a"})
			require.NoError(t, err)
			authStore.setRaw(t, identityAuth(t, "workspace", secondID, "b@example.com", "token-b"), domain.AuthStoreFile)
			second, err := m.Save(ctx, SaveInput{DisplayName: "b"})
			require.NoError(t, err)
			require.NotEqual(t, first.Account.ID, second.Account.ID)
			state, _, _, err := m.loadStateAndVault(ctx)
			require.NoError(t, err)
			require.Len(t, state.Accounts, 2)
		})
	}
}

func TestSameCodexIdentityRejectsDifferentSubjectWithSameEmail(t *testing.T) {
	first := identityAuth(t, "workspace", "user-a", "same@example.com", "token-a")
	second := identityAuth(t, "workspace", "user-b", "same@example.com", "token-b")
	require.False(t, sameCodexIdentity(first, second))
}

func TestSaveKeepsDifferentAPIKeysSeparate(t *testing.T) {
	m, authStore, _ := newTestManager(t)
	ctx := context.Background()
	authStore.setRaw(t, []byte(`{"OPENAI_API_KEY":"synthetic-key-a"}`), domain.AuthStoreFile)
	first, err := m.Save(ctx, SaveInput{DisplayName: "a"})
	require.NoError(t, err)
	authStore.setRaw(t, []byte(`{"OPENAI_API_KEY":"synthetic-key-b"}`), domain.AuthStoreFile)
	second, err := m.Save(ctx, SaveInput{DisplayName: "b"})
	require.NoError(t, err)
	require.NotEqual(t, first.Account.ID, second.Account.ID)
}

func TestSameCodexIdentityRejectsAPIKeysWithLeftoverTokens(t *testing.T) {
	first := []byte(`{"auth_mode":"api_key","OPENAI_API_KEY":"synthetic-a","tokens":{"account_id":"workspace","access_token":"leftover"}}`)
	second := []byte(`{"auth_mode":"api_key","OPENAI_API_KEY":"synthetic-b","tokens":{"account_id":"workspace","access_token":"leftover"}}`)
	require.False(t, sameCodexIdentity(first, second))
}
