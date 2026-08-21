package fs

import (
	"fmt"
	"os"
	"path/filepath"
)

const (
	DirMode  os.FileMode = 0o700
	FileMode os.FileMode = 0o600
)

var chmodPath = os.Chmod

func EnsureDir(path string) error {
	if err := os.MkdirAll(path, DirMode); err != nil {
		return fmt.Errorf("ensure dir %s: %w", path, err)
	}
	if err := chmodPath(path, DirMode); err != nil {
		return fmt.Errorf("chmod dir %s: %w", path, err)
	}
	return nil
}

var statPath = os.Stat

// EnsureDirCreateOnly creates path with DirMode when it is missing but leaves
// an existing directory's permissions alone. Use it for destinations the user
// named: the backup path is arbitrary, and tightening a directory CMA did not
// create would silently re-permission something like a shared folder or a web
// root that the user happens to own.
func EnsureDirCreateOnly(path string) error {
	if _, err := statPath(path); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("ensure dir %s: %w", path, err)
	}
	return EnsureDir(path)
}

func EnsureParentDir(path string) error {
	return EnsureDir(filepath.Dir(path))
}

func EnsureParentDirCreateOnly(path string) error {
	return EnsureDirCreateOnly(filepath.Dir(path))
}

func EnsureFileMode(path string) error {
	if err := chmodPath(path, FileMode); err != nil {
		return fmt.Errorf("chmod file %s: %w", path, err)
	}
	return nil
}
