package container

import (
	"errors"
	"fmt"
	"time"

	"minicontainer/internal/state"
)

type processGenerationProbe func(pid int, pidStartTime uint64) (alive bool, err error)
type stoppedGenerationCleanup func(st *state.Store, c *state.Container) error

// ReconcileContainerState refreshes one persisted container lifecycle using its
// exact process generation. Running state is never inferred from a numeric PID
// alone: a pidfd is opened and verified against the persisted /proc starttime.
//
// A missing process or a reused PID proves that the persisted generation is
// gone, so the transition to stopped is performed with MarkStoppedIfIdentity.
// This keeps a concurrent restart safe: a stale observation cannot stop a newer
// PID/start-time generation. Stopped records also retry every durable runtime
// cleanup token before callers such as rm/prune are allowed to discard state.
//
// Once a non-nil snapshot is supplied, errors preserve at least that snapshot
// (or a newer one already read from disk) so callers can report failures without
// dereferencing a record that disappeared during reconciliation.
func ReconcileContainerState(st *state.Store, c *state.Container) (*state.Container, error) {
	return reconcileContainerStateWith(st, c, probeProcessGeneration, CleanupStoppedRuntimeResources, time.Now)
}

func probeProcessGeneration(pid int, pidStartTime uint64) (bool, error) {
	handle, err := OpenProcessHandle(pid, pidStartTime)
	if err != nil {
		if errors.Is(err, ErrProcessNotFound) || errors.Is(err, ErrProcessIdentityMismatch) {
			return false, nil
		}
		return false, err
	}

	exited, waitErr := handle.WaitExit(0)
	closeErr := handle.Close()
	if waitErr != nil {
		return false, waitErr
	}
	if closeErr != nil {
		return false, closeErr
	}
	return !exited, nil
}

func reconcileContainerStateWith(
	st *state.Store,
	c *state.Container,
	probe processGenerationProbe,
	cleanup stoppedGenerationCleanup,
	now func() time.Time,
) (*state.Container, error) {
	if c == nil {
		return nil, fmt.Errorf("container snapshot is nil")
	}
	if st == nil {
		return c, fmt.Errorf("state store is nil")
	}
	if c.ID == "" {
		return c, fmt.Errorf("container ID is empty")
	}
	if probe == nil {
		return c, fmt.Errorf("process generation probe is nil")
	}
	if cleanup == nil {
		return c, fmt.Errorf("stopped generation cleanup is nil")
	}
	if now == nil {
		return c, fmt.Errorf("clock is nil")
	}

	current, err := st.Get(c.ID)
	if err != nil {
		return c, fmt.Errorf("reload container %s before reconciliation: %w", c.ID, err)
	}

	if current.Status == state.StatusStopped {
		if err := cleanup(st, current); err != nil {
			return current, fmt.Errorf("cleanup stopped container %s during reconciliation: %w", current.ID, err)
		}
		latest, err := st.Get(current.ID)
		if err != nil {
			return current, fmt.Errorf("reload stopped container %s after cleanup: %w", current.ID, err)
		}
		return latest, nil
	}
	if current.Status != state.StatusRunning {
		return current, nil
	}
	if current.PID <= 0 || current.PIDStartTime == 0 {
		return current, fmt.Errorf("%w: container %s has invalid process identity %d/%d", ErrProcessIdentityUnavailable, current.ID, current.PID, current.PIDStartTime)
	}

	pid := current.PID
	pidStartTime := current.PIDStartTime
	alive, err := probe(pid, pidStartTime)
	if err != nil {
		return current, fmt.Errorf("probe container %s process %d/%d: %w", current.ID, pid, pidStartTime, err)
	}
	if alive {
		latest, err := st.Get(current.ID)
		if err != nil {
			return current, fmt.Errorf("reload live container %s after process probe: %w", current.ID, err)
		}
		return latest, nil
	}

	if _, err := st.MarkStoppedIfIdentity(current.ID, pid, pidStartTime, -1, now()); err != nil {
		return current, fmt.Errorf("reconcile exited container %s generation %d/%d: %w", current.ID, pid, pidStartTime, err)
	}

	latest, err := st.Get(current.ID)
	if err != nil {
		return current, fmt.Errorf("reload container %s after stopped reconciliation: %w", current.ID, err)
	}
	if latest.Status != state.StatusStopped {
		// A concurrent lifecycle actor may have installed a newer generation.
		// The identity-CAS above deliberately loses that race.
		return latest, nil
	}
	if err := cleanup(st, latest); err != nil {
		return latest, fmt.Errorf("cleanup reconciled container %s: %w", latest.ID, err)
	}
	cleaned, err := st.Get(latest.ID)
	if err != nil {
		return latest, fmt.Errorf("reload reconciled container %s after cleanup: %w", latest.ID, err)
	}
	return cleaned, nil
}
