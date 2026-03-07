package fs

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

var (
	createTempFile = os.CreateTemp
	renameFile     = os.Rename
	openDirFile    = os.Open
)

type AtomicWriteHooks struct {
	AfterFileSync func() error
	AfterRename   func() error
	AfterDirSync  func() error
}

type AtomicWriteOptions struct {
	Mode   os.FileMode
	Verify func(path string) error
	Hooks  AtomicWriteHooks
}

func WriteFileAtomic(path string, data []byte, opts AtomicWriteOptions) error {
	mode := opts.Mode
	if mode == 0 {
		mode = FileMode
	}

	var originalData []byte
	var originalMode os.FileMode
	existed := false
	if current, err := os.ReadFile(path); err == nil {
		originalData = current
		existed = true
		if info, statErr := os.Stat(path); statErr == nil {
			originalMode = info.Mode().Perm()
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("read existing file %s: %w", path, err)
	}
	if originalMode == 0 {
		originalMode = mode
	}

	committed, err := writeFileAtomicNoRollback(path, data, mode, opts.Hooks)
	if err != nil {
		if committed {
			rollbackErr := rollbackAtomicWrite(path, existed, originalData, originalMode)
			if rollbackErr != nil {
				return errors.Join(err, rollbackErr)
			}
		}
		return err
	}

	if opts.Verify != nil {
		if err := opts.Verify(path); err != nil {
			rollbackErr := rollbackAtomicWrite(path, existed, originalData, originalMode)
			if rollbackErr != nil {
				return errors.Join(fmt.Errorf("verify %s: %w", path, err), rollbackErr)
			}
			return fmt.Errorf("verify %s: %w", path, err)
		}
	}

	return nil
}

func writeFileAtomicNoRollback(path string, data []byte, mode os.FileMode, hooks AtomicWriteHooks) (bool, error) {
	if err := EnsureParentDir(path); err != nil {
		return false, err
	}

	dir := filepath.Dir(path)
	tmp, err := createTempFile(dir, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return false, fmt.Errorf("create temp file for %s: %w", path, err)
	}
	tmpPath := tmp.Name()
	defer func() {
		_ = os.Remove(tmpPath)
	}()

	if err := tmp.Chmod(mode); err != nil {
		_ = tmp.Close()
		return false, fmt.Errorf("chmod temp file %s: %w", tmpPath, err)
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return false, fmt.Errorf("write temp file %s: %w", tmpPath, err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return false, fmt.Errorf("sync temp file %s: %w", tmpPath, err)
	}
	if hooks.AfterFileSync != nil {
		if err := hooks.AfterFileSync(); err != nil {
			_ = tmp.Close()
			return false, fmt.Errorf("post file sync hook %s: %w", path, err)
		}
	}
	if err := tmp.Close(); err != nil {
		return false, fmt.Errorf("close temp file %s: %w", tmpPath, err)
	}

	if err := renameFile(tmpPath, path); err != nil {
		return false, fmt.Errorf("rename temp file %s to %s: %w", tmpPath, path, err)
	}
	if hooks.AfterRename != nil {
		if err := hooks.AfterRename(); err != nil {
			return true, fmt.Errorf("post rename hook %s: %w", path, err)
		}
	}

	if err := syncDir(dir); err != nil {
		return true, fmt.Errorf("sync parent dir %s: %w", dir, err)
	}
	if hooks.AfterDirSync != nil {
		if err := hooks.AfterDirSync(); err != nil {
			return true, fmt.Errorf("post dir sync hook %s: %w", path, err)
		}
	}
	if err := EnsureFileMode(path); err != nil {
		return true, err
	}

	return true, nil
}

func rollbackAtomicWrite(path string, existed bool, originalData []byte, mode os.FileMode) error {
	if !existed {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("remove new file %s during rollback: %w", path, err)
		}
		return syncDir(filepath.Dir(path))
	}
	if _, err := writeFileAtomicNoRollback(path, originalData, mode, AtomicWriteHooks{}); err != nil {
		return fmt.Errorf("restore original file %s: %w", path, err)
	}
	return nil
}

func syncDir(path string) error {
	dir, err := openDirFile(path)
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}
