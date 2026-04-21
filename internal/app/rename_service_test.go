package app

import (
	"context"
	"testing"

	"github.com/prakersh/codexmultiauth/internal/domain"
	"github.com/stretchr/testify/require"
)

func TestRename_FreeNameSucceeds(t *testing.T) {
	manager, authStore, _ := newTestManager(t)
	ctx := context.Background()

	authStore.setRaw(t, []byte(`{"auth_mode":"chatgpt","tokens":{"access_token":"a","refresh_token":"r","account_id":"acc-1"}}`), domain.AuthStoreFile)
	saved, err := manager.Save(ctx, SaveInput{DisplayName: "work"})
	require.NoError(t, err)

	require.NoError(t, manager.Rename(ctx, RenameInput{Selector: saved.Account.ID, NewName: "work-renamed"}))

	list, err := manager.List(ctx)
	require.NoError(t, err)
	require.Len(t, list, 1)
	require.Equal(t, "work-renamed", list[0].Account.DisplayName)
}

func TestRename_ToOwnNameIsNoOp(t *testing.T) {
	manager, authStore, _ := newTestManager(t)
	ctx := context.Background()

	authStore.setRaw(t, []byte(`{"auth_mode":"chatgpt","tokens":{"access_token":"a","refresh_token":"r","account_id":"acc-1"}}`), domain.AuthStoreFile)
	saved, err := manager.Save(ctx, SaveInput{DisplayName: "work"})
	require.NoError(t, err)

	require.NoError(t, manager.Rename(ctx, RenameInput{Selector: saved.Account.ID, NewName: "work"}))

	list, err := manager.List(ctx)
	require.NoError(t, err)
	require.Equal(t, "work", list[0].Account.DisplayName)
}

func TestRename_CollisionWithOtherDisplayNameRejected(t *testing.T) {
	manager, authStore, _ := newTestManager(t)
	ctx := context.Background()

	authStore.setRaw(t, []byte(`{"auth_mode":"chatgpt","tokens":{"access_token":"a","refresh_token":"r","account_id":"acc-1"}}`), domain.AuthStoreFile)
	_, err := manager.Save(ctx, SaveInput{DisplayName: "work"})
	require.NoError(t, err)

	authStore.setRaw(t, []byte(`{"auth_mode":"chatgpt","tokens":{"access_token":"b","refresh_token":"r2","account_id":"acc-2"}}`), domain.AuthStoreFile)
	savedB, err := manager.Save(ctx, SaveInput{DisplayName: "personal"})
	require.NoError(t, err)

	err = manager.Rename(ctx, RenameInput{Selector: savedB.Account.ID, NewName: "work"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "already exists")

	list, err := manager.List(ctx)
	require.NoError(t, err)
	require.Len(t, list, 2)
	var names []string
	for _, row := range list {
		names = append(names, row.Account.DisplayName)
	}
	require.ElementsMatch(t, []string{"work", "personal"}, names)
}

func TestRename_CollisionWithAliasRejected(t *testing.T) {
	manager, authStore, _ := newTestManager(t)
	ctx := context.Background()

	authStore.setRaw(t, []byte(`{"auth_mode":"chatgpt","tokens":{"access_token":"a","refresh_token":"r","account_id":"acc-1"}}`), domain.AuthStoreFile)
	_, err := manager.Save(ctx, SaveInput{DisplayName: "work", Aliases: []string{"main"}})
	require.NoError(t, err)

	authStore.setRaw(t, []byte(`{"auth_mode":"chatgpt","tokens":{"access_token":"b","refresh_token":"r2","account_id":"acc-2"}}`), domain.AuthStoreFile)
	savedB, err := manager.Save(ctx, SaveInput{DisplayName: "personal"})
	require.NoError(t, err)

	err = manager.Rename(ctx, RenameInput{Selector: savedB.Account.ID, NewName: "main"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "alias")
}
