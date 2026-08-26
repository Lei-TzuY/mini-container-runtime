//go:build linux

package dns

import (
	"errors"
	"fmt"
	"path/filepath"

	"golang.org/x/sys/unix"
)

// withDNSNetworkLock serializes one DNS registry across independent minictl
// processes. The lock file is private, non-symlinked, and lives beside the
// registry so every process using the same state root contends on one flock.
func withDNSNetworkLock(dir, networkName string, fn func() error) error {
	if fn == nil {
		return fmt.Errorf("DNS lock callback is nil")
	}
	lockPath := filepath.Join(dir, networkName+".lock")
	fd, err := unix.Open(lockPath, unix.O_CREAT|unix.O_RDWR|unix.O_CLOEXEC|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0o600)
	if err != nil {
		return fmt.Errorf("open DNS lock %q: %w", networkName, err)
	}
	defer unix.Close(fd)

	var st unix.Stat_t
	if err := unix.Fstat(fd, &st); err != nil {
		return fmt.Errorf("inspect DNS lock %q: %w", networkName, err)
	}
	if st.Mode&unix.S_IFMT != unix.S_IFREG {
		return fmt.Errorf("DNS lock %q must be a regular file", networkName)
	}
	if err := unix.Fchmod(fd, 0o600); err != nil {
		return fmt.Errorf("chmod DNS lock %q: %w", networkName, err)
	}
	if err := unix.Flock(fd, unix.LOCK_EX); err != nil {
		return fmt.Errorf("lock DNS network %q: %w", networkName, err)
	}

	callbackErr := fn()
	unlockErr := unix.Flock(fd, unix.LOCK_UN)
	if unlockErr != nil {
		unlockErr = fmt.Errorf("unlock DNS network %q: %w", networkName, unlockErr)
	}
	return errors.Join(callbackErr, unlockErr)
}
