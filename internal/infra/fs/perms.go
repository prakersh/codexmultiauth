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

func EnsureParentDir(path string) error {
	return EnsureDir(filepath.Dir(path))
}

func EnsureFileMode(path string) error {
	if err := chmodPath(path, FileMode); err != nil {
		return fmt.Errorf("chmod file %s: %w", path, err)
	}
	return nil
}
