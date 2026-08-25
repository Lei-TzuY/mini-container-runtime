//go:build !linux

package state

import (
	"fmt"
	"io"
	"os"
)

func readRegularStateFile(path, label string) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return nil, fmt.Errorf("%s %q must be a regular file", label, path)
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	openedInfo, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("inspect %s %q: %w", label, path, err)
	}
	if !openedInfo.Mode().IsRegular() {
		return nil, fmt.Errorf("%s %q must be a regular file", label, path)
	}
	if openedInfo.Mode().Perm() != 0o600 {
		if err := file.Chmod(0o600); err != nil {
			return nil, fmt.Errorf("secure %s permissions: %w", label, err)
		}
	}
	data, err := io.ReadAll(file)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", label, err)
	}
	return data, nil
}
