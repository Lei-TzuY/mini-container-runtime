package container

import (
	"errors"
	"fmt"
	"time"

	"minicontainer/internal/state"
)

const forcedStopWait = 5 * time.Second

// openRunningProcess resolves a running container and opens a stable process
// handle for the exact PID/start-time identity persisted in state. Callers must
// keep the returned handle open for the whole control operation.
func openRunningProcess(st *state.Store, containerID string) (*state.Container, *ProcessHandle, error) {
	if st == nil {
		return nil, nil, fmt.Errorf("state store is nil")
	}

	c, err := st.Resolve(containerID)
	if err != nil {
		return nil, nil, fmt.Errorf("resolve container: %w", err)
	}
	shortID := c.ID
	if len(shortID) > 8 {
		shortID = shortID[:8]
	}
	if c.Status != state.StatusRunning {
		return nil, nil, fmt.Errorf("container %s is not running", shortID)
	}
	if c.PID <= 0 || c.PIDStartTime == 0 {
		return nil, nil, fmt.Errorf("container %s: %w", shortID, ErrProcessIdentityUnavailable)
	}

	handle, err := OpenProcessHandle(c.PID, c.PIDStartTime)
	if err != nil {
		if errors.Is(err, ErrProcessNotFound) {
			return nil, nil, fmt.Errorf("container %s process exited: %w", shortID, err)
		}
		return nil, nil, fmt.Errorf("container %s process verification: %w", shortID, err)
	}
	return c, handle, nil
}

// StopContainer gracefully terminates the exact process identity stored for a
// running container. It sends SIGTERM through pidfd, waits for that pidfd to
// become readable, then escalates to SIGKILL on timeout. State is reconciled
// only after the referenced process is confirmed exited, and only if the state
// record still points at the same PID/start-time pair. The exact generation's
// cgroup is then removed even if a concurrent lifecycle actor already updated
// the state record.
func StopContainer(st *state.Store, containerID string, timeout time.Duration) (*state.Container, error) {
	if timeout < 0 {
		return nil, fmt.Errorf("stop timeout must not be negative")
	}

	c, handle, err := openRunningProcess(st, containerID)
	if err != nil {
		return nil, err
	}
	defer handle.Close()

	term, err := ParseSignal("SIGTERM")
	if err != nil {
		return nil, fmt.Errorf("resolve SIGTERM: %w", err)
	}
	if err := handle.Signal(term); err != nil && !errors.Is(err, ErrProcessNotFound) {
		return nil, fmt.Errorf("gracefully stop container: %w", err)
	}

	exited, err := handle.WaitExit(timeout)
	if err != nil {
		return nil, fmt.Errorf("wait for graceful stop: %w", err)
	}
	if !exited {
		kill, err := ParseSignal("SIGKILL")
		if err != nil {
			return nil, fmt.Errorf("resolve SIGKILL: %w", err)
		}
		if err := handle.Signal(kill); err != nil && !errors.Is(err, ErrProcessNotFound) {
			return nil, fmt.Errorf("force stop container: %w", err)
		}
		exited, err = handle.WaitExit(forcedStopWait)
		if err != nil {
			return nil, fmt.Errorf("wait for forced stop: %w", err)
		}
		if !exited {
			return nil, fmt.Errorf("container process %d did not exit after SIGKILL", c.PID)
		}
	}

	if _, err := FinalizeStoppedGeneration(st, c, -1, time.Now()); err != nil {
		return c, fmt.Errorf("finalize stopped container: %w", err)
	}
	return c, nil
}
