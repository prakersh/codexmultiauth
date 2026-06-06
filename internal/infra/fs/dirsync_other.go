//go:build !windows

package fs

func syncDir(path string) error {
	dir, err := openDirFile(path)
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}
