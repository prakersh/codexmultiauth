package paths

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

type Paths struct {
	HomeDir      string
	ConfigDir    string
	ConfigFile   string
	StateFile    string
	VaultFile    string
	VaultKeyFile string
	BackupDir    string
	LockDir      string
	CodexHome    string
	CodexAuth    string
}

func Resolve() (Paths, error) {
	home, err := os.UserHomeDir()
	if err != nil || strings.TrimSpace(home) == "" {
		return Paths{}, errors.New("resolve paths: home directory unavailable")
	}

	configRoot := strings.TrimSpace(os.Getenv("XDG_CONFIG_HOME"))
	if configRoot == "" {
		configRoot = filepath.Join(home, ".config")
	}

	codexHome := strings.TrimSpace(os.Getenv("CODEX_HOME"))
	if codexHome == "" {
		codexHome = filepath.Join(home, ".codex")
	}

	configDir := filepath.Join(configRoot, "cma")
	return Paths{
		HomeDir:      home,
		ConfigDir:    configDir,
		ConfigFile:   filepath.Join(configDir, "config.json"),
		StateFile:    filepath.Join(configDir, "state.json"),
		VaultFile:    filepath.Join(configDir, "vault.v1.json"),
		VaultKeyFile: filepath.Join(configDir, "vault.key.v1"),
		BackupDir:    filepath.Join(configDir, "backups"),
		LockDir:      filepath.Join(configDir, "locks"),
		CodexHome:    codexHome,
		CodexAuth:    filepath.Join(codexHome, "auth.json"),
	}, nil
}
