package daemon

import (
	"errors"
	"fmt"
	"os"

	"minicontainer/internal/state"
)

var errContainerRestartedDuringStop = errors.New("container restarted during stop")

// verifyStopSettlement checks the durable state after the exact process
// generation targeted by a stop request has been confirmed exited and
// finalization has completed. A concurrent deletion is already a non-running
// outcome, but a newer running generation must never be reported as stopped.
func verifyStopSettlement(st *state.Store, stoppedGeneration *state.Container) error {
	if st == nil {
		return fmt.Errorf("state store is nil")
	}
	if stoppedGeneration == nil || stoppedGeneration.ID == "" {
		return fmt.Errorf("stopped generation snapshot is incomplete")
	}

	current, err := st.Get(stoppedGeneration.ID)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("reload container after stop finalization: %w", err)
	}

	switch current.Status {
	case state.StatusStopped:
		return nil
	case state.StatusRunning:
		if current.PID != stoppedGeneration.PID || current.PIDStartTime != stoppedGeneration.PIDStartTime {
			return fmt.Errorf(
				"%w: old generation %d/%d, current generation %d/%d",
				errContainerRestartedDuringStop,
				stoppedGeneration.PID,
				stoppedGeneration.PIDStartTime,
				current.PID,
				current.PIDStartTime,
			)
		}
		return fmt.Errorf(
			"container %s generation %d/%d remained running after confirmed exit finalization",
			current.ID,
			current.PID,
			current.PIDStartTime,
		)
	default:
		return fmt.Errorf("container %s has unexpected post-stop status %q", current.ID, current.Status)
	}
}
