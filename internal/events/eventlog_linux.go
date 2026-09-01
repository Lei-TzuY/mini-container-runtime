//go:build linux

package events

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"
)

type lockedEventLogFile struct {
	*os.File
	lockFile *os.File
}

func (f *lockedEventLogFile) Close() error {
	if f == nil {
		return nil
	}
	var fileErr error
	if f.File != nil {
		fileErr = f.File.Close()
		f.File = nil
	}
	if f.lockFile == nil {
		return fileErr
	}
	unlockErr := unix.Flock(int(f.lockFile.Fd()), unix.LOCK_UN)
	if unlockErr != nil {
		unlockErr = fmt.Errorf("unlock event log writer: %w", unlockErr)
	}
	lockCloseErr := f.lockFile.Close()
	f.lockFile = nil
	return errors.Join(fileErr, unlockErr, lockCloseErr)
}

func verifyLockedEventPath(file *os.File, path string) error {
	held, err := file.Stat()
	if err != nil {
		return fmt.Errorf("inspect held event lock: %w", err)
	}
	current, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("verify event lock path identity: %w", err)
	}
	if current.Mode()&os.ModeSymlink != 0 || !current.Mode().IsRegular() || !os.SameFile(held, current) {
		return fmt.Errorf("event lock path changed while locked")
	}
	return nil
}

func openEventLogForAppend(path string) (*lockedEventLogFile, error) {
	// Lock a stable sidecar rather than events.log itself. The audit log pathname
	// may later be replaced during retention/rotation; an inode lock on the old
	// generation would then allow a second writer to lock the new generation and
	// enter the append critical section concurrently.
	lockPath := path + ".lock"
	lockFile, err := openEventLog(lockPath, unix.O_RDWR|unix.O_CREAT, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open event log writer lock: %w", err)
	}
	if err := unix.Flock(int(lockFile.Fd()), unix.LOCK_EX); err != nil {
		_ = lockFile.Close()
		return nil, fmt.Errorf("lock event log writer: %w", err)
	}
	if err := verifyLockedEventPath(lockFile, lockPath); err != nil {
		_ = unix.Flock(int(lockFile.Fd()), unix.LOCK_UN)
		_ = lockFile.Close()
		return nil, err
	}

	file, err := openEventLog(path, unix.O_WRONLY|unix.O_CREAT|unix.O_APPEND, 0o600)
	if err != nil {
		_ = unix.Flock(int(lockFile.Fd()), unix.LOCK_UN)
		_ = lockFile.Close()
		return nil, err
	}
	return &lockedEventLogFile{File: file, lockFile: lockFile}, nil
}

func openEventLogForRead(path string) (*os.File, error) {
	return openEventLog(path, unix.O_RDONLY, 0)
}

func openEventLog(path string, flags int, mode uint32) (*os.File, error) {
	if isManagedEventLogPath(path) {
		return openManagedEventLog(path, flags, mode)
	}

	dir := filepath.Dir(path)
	if flags&unix.O_CREAT != 0 {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return nil, fmt.Errorf("create event log directory: %w", err)
		}
	}
	info, err := os.Lstat(dir)
	if err != nil {
		return nil, fmt.Errorf("inspect event log directory: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return nil, fmt.Errorf("event log directory is not a real directory")
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return nil, fmt.Errorf("secure event log directory: %w", err)
	}

	dfd, err := unix.Open(dir, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return nil, fmt.Errorf("open event log directory: %w", err)
	}
	defer unix.Close(dfd)
	return openEventAt(dfd, filepath.Base(path), path, flags, mode)
}

func openManagedEventLog(path string, flags int, mode uint32) (*os.File, error) {
	base := eventStateDir()
	if flags&unix.O_CREAT != 0 {
		if err := unix.Mkdir(base, 0o700); err != nil && err != unix.EEXIST {
			return nil, fmt.Errorf("create event state directory: %w", err)
		}
	}

	dfd, err := unix.Open(base, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return nil, fmt.Errorf("open event state directory: %w", err)
	}
	defer unix.Close(dfd)
	if err := unix.Fchmod(dfd, 0o700); err != nil {
		return nil, fmt.Errorf("secure event state directory: %w", err)
	}
	return openEventAt(dfd, filepath.Base(path), path, flags, mode)
}

func openEventAt(dirFD int, name, displayPath string, flags int, mode uint32) (*os.File, error) {
	fd, err := unix.Openat(dirFD, name, flags|unix.O_CLOEXEC|unix.O_NOFOLLOW|unix.O_NONBLOCK, mode)
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
	if st.Uid != uint32(unix.Geteuid()) {
		unix.Close(fd)
		return nil, fmt.Errorf("event log owner does not match runtime user")
	}
	if st.Nlink != 1 {
		unix.Close(fd)
		return nil, fmt.Errorf("event log has unexpected hard links")
	}

	writable := flags&(unix.O_WRONLY|unix.O_RDWR) != 0
	if writable {
		if err := unix.Fchmod(fd, 0o600); err != nil {
			unix.Close(fd)
			return nil, fmt.Errorf("secure event log permissions: %w", err)
		}
	} else if st.Mode&0o077 != 0 {
		unix.Close(fd)
		return nil, fmt.Errorf("event log permissions are not private")
	}

	file := os.NewFile(uintptr(fd), displayPath)
	if file == nil {
		unix.Close(fd)
		return nil, fmt.Errorf("wrap event log fd")
	}
	return file, nil
}
