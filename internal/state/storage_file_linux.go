//go:build linux

package state

import (
	"fmt"
	"io"
	"os"

	"golang.org/x/sys/unix"
)

// readRegularStateFile opens the state file without following a final symlink,
// then validates and tightens permissions on the already-open descriptor. This
// avoids the Lstat/open TOCTOU window where a pathname could be swapped to a
// symlink between validation and reading.
func readRegularStateFile(path, label string) ([]byte, error) {
	fd, err := unix.Open(path, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0)
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(fd), path)
	if file == nil {
		_ = unix.Close(fd)
		return nil, fmt.Errorf("wrap %s fd", label)
	}
	defer file.Close()

	var st unix.Stat_t
	if err := unix.Fstat(fd, &st); err != nil {
		return nil, fmt.Errorf("inspect %s %q: %w", label, path, err)
	}
	if st.Mode&unix.S_IFMT != unix.S_IFREG {
		return nil, fmt.Errorf("%s %q must be a regular file", label, path)
	}
	if err := unix.Fchmod(fd, 0o600); err != nil {
		return nil, fmt.Errorf("secure %s permissions: %w", label, err)
	}
	data, err := io.ReadAll(file)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", label, err)
	}
	return data, nil
}
