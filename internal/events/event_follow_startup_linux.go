//go:build linux

package events

import (
	"errors"
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

type eventFollowStartupSnapshot struct {
	retained []eventLogSnapshotFile
	active   *os.File
}

func (snapshot *eventFollowStartupSnapshot) close() {
	for _, generation := range snapshot.retained {
		_ = generation.file.Close()
	}
	if snapshot.active != nil {
		_ = snapshot.active.Close()
	}
}

// openEventLogFollowStartupSnapshot captures the retained generation and the
// active descriptor under the same writer lock. Holding both descriptors across
// the handoff makes a concurrent rename safe: the follower can drain the exact
// generations it observed without reopening pathnames in between.
func openEventLogFollowStartupSnapshot(path string) (*eventFollowStartupSnapshot, error) {
	lockPath := path + ".lock"
	lockFile, err := openEventLog(lockPath, unix.O_RDWR|unix.O_CREAT, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open event log follow snapshot lock: %w", err)
	}
	defer lockFile.Close()

	if err := unix.Flock(int(lockFile.Fd()), unix.LOCK_SH); err != nil {
		return nil, fmt.Errorf("lock event log follow snapshot: %w", err)
	}
	defer unix.Flock(int(lockFile.Fd()), unix.LOCK_UN)
	if err := verifyLockedEventPath(lockFile, lockPath); err != nil {
		return nil, err
	}

	snapshot := &eventFollowStartupSnapshot{}
	retainedPath := path + ".1"
	retained, err := openEventLogForRead(retainedPath)
	if err == nil {
		if err := verifyHeldEventPath(retained, retainedPath, "event log follow retained snapshot"); err != nil {
			_ = retained.Close()
			return nil, err
		}
		info, err := retained.Stat()
		if err != nil {
			_ = retained.Close()
			return nil, fmt.Errorf("stat retained event log follow snapshot: %w", err)
		}
		snapshot.retained = append(snapshot.retained, eventLogSnapshotFile{file: retained, size: info.Size()})
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}

	active, err := openEventLogForRead(path)
	if err == nil {
		if err := verifyHeldEventPath(active, path, "event log follow active snapshot"); err != nil {
			_ = active.Close()
			snapshot.close()
			return nil, err
		}
		snapshot.active = active
	} else if !errors.Is(err, os.ErrNotExist) {
		snapshot.close()
		return nil, err
	}
	return snapshot, nil
}
