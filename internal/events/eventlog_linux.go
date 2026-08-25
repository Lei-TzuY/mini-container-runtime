//go:build linux

package events

import (
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"
)

func openEventLogForAppend(path string) (*os.File, error) {
	return openEventLog(path, unix.O_WRONLY|unix.O_CREAT|unix.O_APPEND, 0o600)
}

func openEventLogForRead(path string) (*os.File, error) {
	return openEventLog(path, unix.O_RDONLY, 0)
}

func openEventLog(path string, flags int, mode uint32) (*os.File, error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("create event log directory: %w", err)
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return nil, fmt.Errorf("secure event log directory: %w", err)
	}

	dfd, err := unix.Open(dir, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return nil, fmt.Errorf("open event log directory: %w", err)
	}
	defer unix.Close(dfd)

	fd, err := unix.Openat(dfd, filepath.Base(path), flags|unix.O_CLOEXEC|unix.O_NOFOLLOW, mode)
	if err != nil {
		return nil, fmt.Errorf("open event log: %w", err)
	}

	var st unix.Stat_t
	if err := unix.Fstat(fd, &st); err != nil {
		unix.Close(fd)
		return nil, fmt.Errorf("stat event log: %w", err)
	}
	if st.Mode&unix.S_IFMT != unix.S_IFREG {
		unix.Close(fd)
		return nil, fmt.Errorf("event log is not a regular file")
	}
	if flags&unix.O_WRONLY != 0 {
		if err := unix.Fchmod(fd, 0o600); err != nil {
			unix.Close(fd)
			return nil, fmt.Errorf("secure event log permissions: %w", err)
		}
	}

	return os.NewFile(uintptr(fd), path), nil
}
