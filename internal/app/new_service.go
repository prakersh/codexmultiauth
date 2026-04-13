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

	result, err := m.Save(ctx, SaveInput{DisplayName: input.DisplayName, Aliases: input.Aliases})
	if err != nil {
		if rollbackErr := rollbackAuth(ctx, m.authStore, originalExists, original); rollbackErr != nil {
			return SaveResult{}, errors.Join(err, rollbackErr)
		}
		return SaveResult{}, err
	}
	return result, nil
}
