package main

import (
	"errors"
	"fmt"
	"time"

	"minicontainer/internal/state"
)

// settleRunCommandState reconciles the synchronous `minictl run` return with
// the lifecycle state owned by container.Run. Once a process generation has
// existed, the runtime parent's durable stopped record is authoritative for the
// payload exit code. The CLI may synthesize exit code 1 only when Run failed
// before the record ever left StatusCreated.
func settleRunCommandState(st *state.Store, id string, runErr error, finishedAt time.Time) (*state.Container, error) {
	if st == nil {
		return nil, fmt.Errorf("state store is nil")
	}
	if id == "" {
		return nil, fmt.Errorf("container ID is empty")
	}

	current, err := st.Get(id)
	if err != nil {
		return nil, fmt.Errorf("reload container %s after run: %w", id, err)
	}

	switch current.Status {
	case state.StatusStopped:
		return current, nil

	case state.StatusCreated:
		if runErr == nil {
			return current, fmt.Errorf("runtime returned successfully but container %s never left created state", id)
		}
		changed, err := st.MarkStoppedIfCreated(id, 1, finishedAt)
		if err != nil {
			return current, fmt.Errorf("record startup failure for container %s: %w", id, err)
		}
		latest, err := st.Get(id)
		if err != nil {
			return current, fmt.Errorf("reload container %s after startup failure: %w", id, err)
		}
		if !changed {
			if latest.Status == state.StatusStopped {
				return latest, nil
			}
			return latest, fmt.Errorf(
				"container %s changed from created to %s while recording startup failure",
				id,
				latest.Status,
			)
		}
		if latest.Status != state.StatusStopped {
			return latest, fmt.Errorf(
				"container %s startup failure transition produced unexpected status %s",
				id,
				latest.Status,
			)
		}
		return latest, nil

	case state.StatusRunning:
		return current, fmt.Errorf(
			"runtime returned while container %s remains running as process %d/%d",
			id,
			current.PID,
			current.PIDStartTime,
		)

	default:
		return current, fmt.Errorf("container %s has unknown lifecycle status %q after run", id, current.Status)
	}
}

func joinRunCommandErrors(runErr, stateErr error) error {
	return errors.Join(runErr, stateErr)
}
