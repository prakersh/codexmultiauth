package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	cmafs "github.com/prakersh/codexmultiauth/internal/infra/fs"
	"github.com/prakersh/codexmultiauth/internal/infra/paths"
)

const ConfigVersionV1 = "cma-config-v1"

type Config struct {
	Version        string `json:"version"`
	DisableKeyring bool   `json:"disable_keyring"`
}

type ConfigRepo struct {
	paths paths.Paths
}

func NewConfigRepo(p paths.Paths) *ConfigRepo {
	return &ConfigRepo{paths: p}
}

func (r *ConfigRepo) Load() (Config, error) {
	if disableKeyringFromEnv() {
		return Config{Version: ConfigVersionV1, DisableKeyring: true}, nil
	}

	data, err := os.ReadFile(r.paths.ConfigFile)
	if errors.Is(err, os.ErrNotExist) {
		return Config{Version: ConfigVersionV1}, nil
	}
	if err != nil {
		return Config{}, fmt.Errorf("load config: %w", err)
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("parse config: %w", err)
	}
	if cfg.Version == "" {
		cfg.Version = ConfigVersionV1
	}
	if disableKeyringFromEnv() {
		cfg.DisableKeyring = true
	}
	return cfg, nil
}

func (r *ConfigRepo) Save(cfg Config) error {
	if cfg.Version == "" {
		cfg.Version = ConfigVersionV1
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	return cmafs.WriteFileAtomic(r.paths.ConfigFile, data, cmafs.AtomicWriteOptions{Mode: cmafs.FileMode})
}

func disableKeyringFromEnv() bool {
	value := strings.TrimSpace(strings.ToLower(os.Getenv("CMA_DISABLE_KEYRING")))
	return value == "1" || value == "true" || value == "yes"
}
