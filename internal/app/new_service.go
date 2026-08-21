package app

import (
	"context"
	"errors"
)

type NewInput struct {
	DisplayName string
	Aliases     []string
	DeviceAuth  bool
	WithAPIKey  bool
}

func (m *Manager) New(ctx context.Context, input NewInput) (SaveResult, error) {
	if m.codexCLI == nil {
		return SaveResult{}, errors.New("codex CLI runner is not configured")
	}

	var original []byte
	originalExists := false
	if current, err := m.authStore.Load(ctx); err == nil {
		original = append([]byte(nil), current.Canonical...)
		originalExists = true
	}

	if err := m.codexCLI.Login(ctx, input.DeviceAuth, input.WithAPIKey); err != nil {
		if rollbackErr := rollbackAuth(ctx, m.authStore, originalExists, original); rollbackErr != nil {
			return SaveResult{}, errors.Join(err, rollbackErr)
		}
		return SaveResult{}, err
	}

	// Login rewrote ~/.codex/auth.json without the mutation lock held, because
	// an interactive browser flow can take minutes and holding the lock would
	// block every other command. Pin what it produced instead: without this, a
	// concurrent `cma activate` landing in the gap would make Save store the
	// pre-existing account under the new name and silently drop the
	// credentials that were just logged in.
	expect := ""
	if record, loadErr := m.authStore.Load(ctx); loadErr == nil {
		expect = record.Fingerprint
	}

	result, err := m.Save(ctx, SaveInput{
		DisplayName:       input.DisplayName,
		Aliases:           input.Aliases,
		ExpectFingerprint: expect,
	})
	if err != nil {
		if rollbackErr := rollbackAuth(ctx, m.authStore, originalExists, original); rollbackErr != nil {
			return SaveResult{}, errors.Join(err, rollbackErr)
		}
		return SaveResult{}, err
	}
	return result, nil
}
