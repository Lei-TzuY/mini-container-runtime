//go:build !linux

package logs

import (
	"fmt"
	"os"
	"path/filepath"
)

func openContainerLogForAppend(path string) (*os.File, error) {
	return openContainerLogPortable(path, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o600)
}

func openContainerLogForRead(path string) (*os.File, error) {
	return openContainerLogPortable(path, os.O_RDONLY, 0)
}

func openContainerLogPortable(path string, flags int, mode os.FileMode) (*os.File, error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("create container log directory: %w", err)
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return nil, fmt.Errorf("secure container log directory: %w", err)
	}

	if info, err := os.Lstat(path); err == nil {
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return nil, fmt.Errorf("container log is not a regular file")
		}
	} else if !os.IsNotExist(err) {
		return nil, fmt.Errorf("inspect container log: %w", err)
	}

	f, err := os.OpenFile(path, flags, mode)
	if err != nil {
		return nil, fmt.Errorf("open container log: %w", err)
	}
	info, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, fmt.Errorf("stat container log: %w", err)
	}
	if !info.Mode().IsRegular() {
		f.Close()
		return nil, fmt.Errorf("container log is not a regular file")
	}
	if flags&os.O_WRONLY != 0 {
		if err := f.Chmod(0o600); err != nil {
			f.Close()
			return nil, fmt.Errorf("secure container log permissions: %w", err)
		}
	}
	return f, nil
}
