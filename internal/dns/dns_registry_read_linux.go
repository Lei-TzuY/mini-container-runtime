//go:build linux

package dns

import (
	"fmt"
	"io"
	"os"

	"golang.org/x/sys/unix"
)

// readDNSRegistryFile opens, validates, and reads a registry through one file
// descriptor. O_NOFOLLOW prevents a terminal symlink from being followed and
// O_NONBLOCK prevents a substituted FIFO/device from blocking before Fstat can
// reject it. Reading from the same descriptor closes the Lstat/open TOCTOU gap.
func readDNSRegistryFile(path, networkName string) ([]byte, bool, error) {
	fd, err := unix.Open(path, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0)
	if err != nil {
		if err == unix.ENOENT {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("open DNS registry %q: %w", networkName, err)
	}
	file := os.NewFile(uintptr(fd), path)
	if file == nil {
		_ = unix.Close(fd)
		return nil, false, fmt.Errorf("open DNS registry %q: invalid file descriptor", networkName)
	}
	defer file.Close()

	var st unix.Stat_t
	if err := unix.Fstat(fd, &st); err != nil {
		return nil, false, fmt.Errorf("inspect DNS registry %q: %w", networkName, err)
	}
	if st.Mode&unix.S_IFMT != unix.S_IFREG {
		return nil, false, fmt.Errorf("DNS registry %q must be a regular file", networkName)
	}

	data, err := io.ReadAll(file)
	if err != nil {
		return nil, false, fmt.Errorf("read DNS registry %q: %w", networkName, err)
	}
	return data, true, nil
}
