package dns

import (
	"errors"
	"fmt"
	"os"

	"minicontainer/internal/state"
)

// hostEntryOwnerActive decides whether a process-owned DNS entry is still
// authoritative. The registrar process owns setup and the whole restart loop.
// If that process is gone, a committed live container generation may adopt the
// registration; otherwise the entry is stale. State/probe uncertainty fails
// closed rather than deleting a possibly-live container's discovery record.
func hostEntryOwnerActive(entry HostEntry) (bool, error) {
	alive, err := registrarGenerationAlive(entry.OwnerPID, entry.OwnerStartTime)
	if err != nil {
		return false, err
	}
	if alive {
		return true, nil
	}

	st, err := state.Open(state.DefaultDir())
	if err != nil {
		return false, fmt.Errorf("open state while recovering DNS owner for container %s: %w", entry.ContainerID, err)
	}
	defer st.Close()

	c, err := st.Get(entry.ContainerID)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, fmt.Errorf("read state while recovering DNS owner for container %s: %w", entry.ContainerID, err)
	}
	switch c.Status {
	case state.StatusRunning:
		if c.PID <= 0 || c.PIDStartTime == 0 {
			return false, fmt.Errorf("running container %s has invalid process identity %d/%d", c.ID, c.PID, c.PIDStartTime)
		}
		childAlive, err := registrarGenerationAlive(c.PID, c.PIDStartTime)
		if err != nil {
			return false, fmt.Errorf("probe adopted container generation %s %d/%d: %w", c.ID, c.PID, c.PIDStartTime, err)
		}
		return childAlive, nil
	case state.StatusCreated, state.StatusStopped:
		return false, nil
	default:
		return false, fmt.Errorf("container %s has unknown lifecycle status %q while recovering DNS ownership", c.ID, c.Status)
	}
}
