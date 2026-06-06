//go:build windows

package fs

// syncDir is a no-op on Windows: fsync on a directory handle is unsupported and
// fails with ERROR_ACCESS_DENIED. The atomic rename is already durable on NTFS.
func syncDir(path string) error {
	return nil
}
