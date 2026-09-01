//go:build linux

package events

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"
)

const maxEventLogBytes int64 = 16 << 20

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

func verifyHeldEventPath(file *os.File, path, kind string) error {
	held, err := file.Stat()
	if err != nil {
		return fmt.Errorf("inspect held %s: %w", kind, err)
	}
	current, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("verify %s path identity: %w", kind, err)
	}
	if current.Mode()&os.ModeSymlink != 0 || !current.Mode().IsRegular() || !os.SameFile(held, current) {
		return fmt.Errorf("%s path changed while held", kind)
	}
	return nil
}

func verifyLockedEventPath(file *os.File, path string) error {
	return verifyHeldEventPath(file, path, "event lock")
}

func (f *lockedEventLogFile) verifyDataPath() error {
	if f == nil || f.File == nil {
		return fmt.Errorf("event log writer is closed")
	}
	return verifyHeldEventPath(f.File, f.File.Name(), "event log")
}

// Write refuses to append through an fd whose pathname was replaced after the
// writer entered its critical section. Without both checks, a same-user process
// could rename events.log and install a replacement while minictl still held an
// fd to the old inode, causing an audit record to disappear into an orphaned
// generation while the append itself appeared to succeed.
func (f *lockedEventLogFile) Write(p []byte) (int, error) {
	if err := f.verifyDataPath(); err != nil {
		return 0, fmt.Errorf("verify event log before write: %w", err)
	}
	n, err := f.File.Write(p)
	if err != nil {
		return n, err
	}
	if err := f.verifyDataPath(); err != nil {
		return n, fmt.Errorf("verify event log after write: %w", err)
	}
	return n, nil
}

// Sync revalidates pathname identity after durability is requested so callers
// cannot report a successful durable audit append after the logical events.log
// path was swapped away from the inode that was actually synced.
func (f *lockedEventLogFile) Sync() error {
	if f == nil || f.File == nil {
		return fmt.Errorf("event log writer is closed")
	}
	if err := f.File.Sync(); err != nil {
		return err
	}
	if err := f.verifyDataPath(); err != nil {
		return fmt.Errorf("verify event log after sync: %w", err)
	}
	return nil
}

// rotateEventLogIfNeeded bounds persistent lifecycle-audit growth to the active
// generation plus one retained generation. It runs only while the stable writer
// sidecar is exclusively locked, so independent minictl processes cannot race a
// rename against another append. The current generation is security-validated
// before rename; unsafe symlink/hard-link/ownership/permission states fail closed.
func rotateEventLogIfNeeded(path string) error {
	current, err := openEventLogForRead(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("inspect event log for rotation: %w", err)
	}
	info, statErr := current.Stat()
	if statErr != nil {
		_ = current.Close()
		return fmt.Errorf("stat event log for rotation: %w", statErr)
	}
	if info.Size() < maxEventLogBytes {
		return current.Close()
	}
	if err := verifyHeldEventPath(current, path, "event log rotation source"); err != nil {
		_ = current.Close()
		return err
	}
	if err := current.Close(); err != nil {
		return fmt.Errorf("close event log before rotation: %w", err)
	}

	rotated := path + ".1"
	if err := os.Rename(path, rotated); err != nil {
		return fmt.Errorf("rotate event log: %w", err)
	}

	// Persist the directory-entry replacement before returning to the append path.
	// If a crash happens before the new active file is created, the next append
	// safely recreates events.log while the retained generation remains intact.
	dir, err := os.Open(filepath.Dir(path))
	if err != nil {
		return fmt.Errorf("open event log directory after rotation: %w", err)
	}
	syncErr := dir.Sync()
	closeErr := dir.Close()
	if syncErr != nil {
		return fmt.Errorf("sync event log directory after rotation: %w", syncErr)
	}
	if closeErr != nil {
		return fmt.Errorf("close event log directory after rotation: %w", closeErr)
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
	if err := rotateEventLogIfNeeded(path); err != nil {
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
	if err := verifyHeldEventPath(file, path, "event log"); err != nil {
		_ = file.Close()
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
