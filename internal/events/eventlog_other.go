//go:build !linux

package events

import (
	"fmt"
	"os"
	"path/filepath"
)

func openEventLogForAppend(path string) (*os.File, error) {
	return openEventLogPortable(path, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o600)
}

func openEventLogForRead(path string) (*os.File, error) {
	return openEventLogPortable(path, os.O_RDONLY, 0)
}

func openEventLogPortable(path string, flags int, mode os.FileMode) (*os.File, error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("create event log directory: %w", err)
	}
	if info, err := os.Lstat(path); err == nil {
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return nil, fmt.Errorf("event log is not a regular file")
		}
	} else if !os.IsNotExist(err) {
		return nil, fmt.Errorf("stat event log: %w", err)
	}
	f, err := os.OpenFile(path, flags, mode)
	if err != nil {
		return nil, err
	}
	if flags&os.O_WRONLY != 0 {
		if err := f.Chmod(0o600); err != nil {
			f.Close()
			return nil, err
		}
	}
	return f, nil
}
