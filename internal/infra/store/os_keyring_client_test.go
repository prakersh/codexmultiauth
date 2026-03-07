package store

import (
	"errors"
	"testing"

	"github.com/99designs/keyring"
	"github.com/stretchr/testify/require"
)

type fakeNativeKeyring struct {
	item      keyring.Item
	getErr    error
	setErr    error
	removeErr error
}

func (f *fakeNativeKeyring) Get(key string) (keyring.Item, error) {
	if f.getErr != nil {
		return keyring.Item{}, f.getErr
	}
	return f.item, nil
}

func (f *fakeNativeKeyring) GetMetadata(key string) (keyring.Metadata, error) {
	return keyring.Metadata{}, nil
}
func (f *fakeNativeKeyring) Set(item keyring.Item) error {
	if f.setErr != nil {
		return f.setErr
	}
	f.item = item
	return nil
}
func (f *fakeNativeKeyring) Remove(key string) error {
	return f.removeErr
}
func (f *fakeNativeKeyring) Keys() ([]string, error) { return nil, nil }

func TestOSKeyringClient_UsesOpenKeyring(t *testing.T) {
	original := openKeyring
	defer func() { openKeyring = original }()

	ring := &fakeNativeKeyring{item: keyring.Item{Data: []byte("value")}}
	openKeyring = func(cfg keyring.Config) (keyring.Keyring, error) {
		require.Equal(t, "service", cfg.ServiceName)
		return ring, nil
	}

	client := OSKeyringClient{}
	value, err := client.Get("service", "account")
	require.NoError(t, err)
	require.Equal(t, []byte("value"), value)

	require.NoError(t, client.Set("service", "account", []byte("new")))
	require.Equal(t, []byte("new"), ring.item.Data)

	require.NoError(t, client.Delete("service", "account"))
}

func TestOSKeyringClient_OpenAndOperationErrors(t *testing.T) {
	original := openKeyring
	defer func() { openKeyring = original }()

	openKeyring = func(cfg keyring.Config) (keyring.Keyring, error) {
		return nil, errors.New("open fail")
	}
	client := OSKeyringClient{}
	_, err := client.Get("service", "account")
	require.Error(t, err)

	ring := &fakeNativeKeyring{getErr: errors.New("get fail"), setErr: errors.New("set fail"), removeErr: errors.New("remove fail")}
	openKeyring = func(cfg keyring.Config) (keyring.Keyring, error) {
		return ring, nil
	}
	_, err = client.Get("service", "account")
	require.Error(t, err)
	err = client.Set("service", "account", []byte("x"))
	require.Error(t, err)
	err = client.Delete("service", "account")
	require.Error(t, err)
}
