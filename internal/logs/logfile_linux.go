//go:build linux

package logs

import (
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"
)

func openContainerLogForAppend(path string) (*os.File, error) {
	return openContainerLog(path, unix.O_WRONLY|unix.O_CREAT|unix.O_APPEND, 0o600, true)
}

func openContainerLogForRead(path string) (*os.File, error) {
	return openContainerLog(path, unix.O_RDONLY, 0, false)
}

func openContainerLogForRotate(path string) (*os.File, error) {
	return openContainerLog(path, unix.O_RDWR, 0, false)
}

func openContainerLog(path string, flags int, mode uint32, createDir bool) (*os.File, error) {
	dir := filepath.Dir(path)
	if createDir {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return nil, fmt.Errorf("create container log directory: %w", err)
		}
	}

	dfd, err := unix.Open(dir, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return nil, fmt.Errorf("open container log directory: %w", err)
	}
	defer unix.Close(dfd)
	if err := unix.Fchmod(dfd, 0o700); err != nil {
		return nil, fmt.Errorf("secure container log directory: %w", err)
	}

	fd, err := unix.Openat(dfd, filepath.Base(path), flags|unix.O_CLOEXEC|unix.O_NOFOLLOW|unix.O_NONBLOCK, mode)
	if err != nil {
		return nil, fmt.Errorf("open container log: %w", err)
	}

	var st unix.Stat_t
	if err := unix.Fstat(fd, &st); err != nil {
		unix.Close(fd)
		return nil, fmt.Errorf("stat container log: %w", err)
	}
	if st.Mode&unix.S_IFMT != unix.S_IFREG {
		unix.Close(fd)
		return nil, fmt.Errorf("container log is not a regular file")
	}
	if err := unix.Fchmod(fd, 0o600); err != nil {
		unix.Close(fd)
		return nil, fmt.Errorf("secure container log permissions: %w", err)
	}

	file := os.NewFile(uintptr(fd), path)
	if file == nil {
		unix.Close(fd)
		return nil, fmt.Errorf("wrap container log fd")
	}
	return file, nil
}
