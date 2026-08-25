package state

import (
	"fmt"
	"os"
	"strings"
)

func ensurePrivateStateDir(path, label string) error {
	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("%s directory cannot be empty", label)
	}
	if err := os.MkdirAll(path, 0o700); err != nil {
		return fmt.Errorf("create %s directory: %w", label, err)
	}
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("inspect %s directory: %w", label, err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return fmt.Errorf("%s path %q must be a real directory", label, path)
	}
	if err := os.Chmod(path, 0o700); err != nil {
		return fmt.Errorf("secure %s directory permissions: %w", label, err)
	}
	return nil
}

func readRegularStateFile(path, label string) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return nil, fmt.Errorf("%s %q must be a regular file", label, path)
	}
	if info.Mode().Perm() != 0o600 {
		if err := os.Chmod(path, 0o600); err != nil {
			return nil, fmt.Errorf("secure %s permissions: %w", label, err)
		}
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return data, nil
}

func syncStateDirectory(dir, label string) error {
	f, err := os.Open(dir)
	if err != nil {
		return fmt.Errorf("open %s directory for fsync: %w", label, err)
	}
	defer f.Close()
	if err := f.Sync(); err != nil {
		return fmt.Errorf("fsync %s directory: %w", label, err)
	}
	return nil
}

func removeStateFileDurable(dir, path, label string) error {
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove %s: %w", label, err)
	}
	// Sync even when the file is already absent. This lets a retry repair the
	// durability of a previous unlink whose directory fsync reported an error.
	return syncStateDirectory(dir, label)
}
