package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"github.com/prakersh/codexmultiauth/internal/domain"
	cmafs "github.com/prakersh/codexmultiauth/internal/infra/fs"
	"github.com/prakersh/codexmultiauth/internal/infra/paths"
)

type StateRepo struct {
	paths paths.Paths
}

func NewStateRepo(p paths.Paths) *StateRepo {
	return &StateRepo{paths: p}
}

func (r *StateRepo) Load() (domain.State, error) {
	data, err := os.ReadFile(r.paths.StateFile)
	if errors.Is(err, os.ErrNotExist) {
		return domain.State{Version: domain.StateVersionV1}, nil
	}
	if err != nil {
		return domain.State{}, fmt.Errorf("load state: %w", err)
	}
	var state domain.State
	if err := json.Unmarshal(data, &state); err != nil {
		return domain.State{}, fmt.Errorf("parse state: %w", err)
	}
	if state.Version == "" {
		state.Version = domain.StateVersionV1
	}
	return state, nil
}

func (r *StateRepo) Save(state domain.State) error {
	if state.Version == "" {
		state.Version = domain.StateVersionV1
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal state: %w", err)
	}
	return cmafs.WriteFileAtomic(r.paths.StateFile, data, cmafs.AtomicWriteOptions{Mode: cmafs.FileMode})
}
