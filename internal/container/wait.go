package container

import (
	"errors"
	"fmt"
	"time"

	"minicontainer/internal/state"
)

const waitStateRefresh = 250 * time.Millisecond

// WaitContainer waits for the lifecycle represented by containerID to reach a
// stopped state and returns its persisted exit code. For a running record it
// binds to the exact PID/start-time identity with a pidfd instead of polling a
// numeric PID. If the persisted identity is already gone or has been reused,
// the stale running record is reconciled to stopped with an unknown exit code.
func WaitContainer(st *state.Store, containerID string) (int, error) {
	if st == nil {
		return -1, fmt.Errorf("state store is nil")
	}

	for {
		c, err := st.Resolve(containerID)
		if err != nil {
			return -1, fmt.Errorf("resolve container: %w", err)
		}
		if c.Status == state.StatusStopped {
			return c.ExitCode, nil
		}
		if c.Status != state.StatusRunning {
			return -1, fmt.Errorf("container %s is %s; wait requires running or stopped state", c.ID, c.Status)
		}
		if c.PID <= 0 || c.PIDStartTime == 0 {
			return -1, fmt.Errorf("container %s: %w", c.ID, ErrProcessIdentityUnavailable)
		}

		handle, err := OpenProcessHandle(c.PID, c.PIDStartTime)
		if err != nil {
			if errors.Is(err, ErrProcessNotFound) || errors.Is(err, ErrProcessIdentityMismatch) {
				changed, stateErr := st.MarkStoppedIfIdentity(c.ID, c.PID, c.PIDStartTime, -1, time.Now())
				if stateErr != nil {
					return -1, fmt.Errorf("reconcile stale running state for container %s: %w", c.ID, stateErr)
				}
				if changed {
					return -1, nil
				}
				continue
			}
			return -1, fmt.Errorf("open process handle for container %s: %w", c.ID, err)
		}

		for {
			exited, waitErr := handle.WaitExit(waitStateRefresh)
			if waitErr != nil {
				_ = handle.Close()
				return -1, fmt.Errorf("wait for container %s process: %w", c.ID, waitErr)
			}

			latest, stateErr := st.Get(c.ID)
			if stateErr != nil {
				_ = handle.Close()
				return -1, fmt.Errorf("reload container %s state while waiting: %w", c.ID, stateErr)
			}
			if latest.Status == state.StatusStopped {
				_ = handle.Close()
				return latest.ExitCode, nil
			}
			if latest.Status != state.StatusRunning || latest.PID != c.PID || latest.PIDStartTime != c.PIDStartTime {
				_ = handle.Close()
				break
			}
			if !exited {
				continue
			}

			_ = handle.Close()
			changed, markErr := st.MarkStoppedIfIdentity(c.ID, c.PID, c.PIDStartTime, -1, time.Now())
			if markErr != nil {
				return -1, fmt.Errorf("reconcile exited container %s: %w", c.ID, markErr)
			}
			if changed {
				return -1, nil
			}
			break
		}
	}
}
