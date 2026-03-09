package testenv

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/prakersh/codexmultiauth/internal/infra/paths"
)

type Sandbox struct {
	Root          string
	Home          string
	XDGConfigHome string
	CodexHome     string
	Paths         paths.Paths
}

func New(t testing.TB) Sandbox {
	t.Helper()
	return NewWithDisableKeyring(t, "1")
}

func NewWithDisableKeyring(t testing.TB, value string) Sandbox {
	t.Helper()

	root := t.TempDir()
	home := filepath.Join(root, "home")
	xdgConfigHome := filepath.Join(home, ".config")
	codexHome := filepath.Join(home, ".codex")

	for _, dir := range []string{home, xdgConfigHome, codexHome} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatalf("create sandbox dir %s: %v", dir, err)
		}
	}

	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", xdgConfigHome)
	t.Setenv("CODEX_HOME", codexHome)
	t.Setenv("CMA_DISABLE_KEYRING", value)

	resolved, err := paths.Resolve()
	if err != nil {
		t.Fatalf("resolve sandbox paths: %v", err)
	}

	return Sandbox{
		Root:          root,
		Home:          home,
		XDGConfigHome: xdgConfigHome,
		CodexHome:     codexHome,
		Paths:         resolved,
	}
}
