//go:build linux

package dns

import (
	"errors"
	"fmt"
	"path/filepath"

	"golang.org/x/sys/unix"
)

func verifyDNSLockPath(fd int, lockPath, networkName string) error {
	var held unix.Stat_t
	if err := unix.Fstat(fd, &held); err != nil {
		return fmt.Errorf("inspect DNS lock %q: %w", networkName, err)
	}
	if held.Mode&unix.S_IFMT != unix.S_IFREG {
		return fmt.Errorf("DNS lock %q must be a regular file", networkName)
	}
	if held.Nlink != 1 {
		return fmt.Errorf("DNS lock %q must have exactly one link, got %d", networkName, held.Nlink)
	}

	var current unix.Stat_t
	if err := unix.Lstat(lockPath, &current); err != nil {
		return fmt.Errorf("verify DNS lock %q path identity: %w", networkName, err)
	}
	if current.Mode&unix.S_IFMT != unix.S_IFREG || current.Dev != held.Dev || current.Ino != held.Ino {
		return fmt.Errorf("DNS lock %q path changed while locked", networkName)
	}
	return nil
}

// withDNSNetworkLock serializes one DNS registry across independent minictl
// processes. The lock file is private, non-symlinked, single-linked, and lives
// beside the registry so every process using the same state root contends on
// one flock. Path identity is checked after acquisition and before release so
// unlink/replacement cannot silently split lock authority across inodes.
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
	if err := verifyDNSLockPath(fd, lockPath, networkName); err != nil {
		_ = unix.Flock(fd, unix.LOCK_UN)
		return err
	}

	callbackErr := fn()
	integrityErr := verifyDNSLockPath(fd, lockPath, networkName)
	unlockErr := unix.Flock(fd, unix.LOCK_UN)
	if unlockErr != nil {
		unlockErr = fmt.Errorf("unlock DNS network %q: %w", networkName, unlockErr)
	}
	return errors.Join(callbackErr, integrityErr, unlockErr)
}
