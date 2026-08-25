package container

import (
	"errors"
	"fmt"
	"time"

	"minicontainer/internal/state"
)

type generationCleanupFunc func(containerID string, pid int, pidStartTime uint64) error

// FinalizeStoppedGeneration reconciles lifecycle state and cleans the cgroup
// belonging to one exact container process generation. Callers must invoke it
// only after they have established that the referenced PID/start-time process
// has exited (or that the PID now belongs to another generation).
//
// State reconciliation and cgroup cleanup are deliberately independent: a
// cleanup failure must not leave a dead process recorded as running, while a
// concurrent restart must not prevent cleanup of the old generation.
func FinalizeStoppedGeneration(st *state.Store, c *state.Container, exitCode int, finishedAt time.Time) (bool, error) {
	return finalizeStoppedGenerationWithCleanup(st, c, exitCode, finishedAt, cleanupContainerProcessGeneration)
}

func finalizeStoppedGenerationWithCleanup(
	st *state.Store,
	c *state.Container,
	exitCode int,
	finishedAt time.Time,
	cleanup generationCleanupFunc,
) (bool, error) {
	if st == nil {
		return false, fmt.Errorf("state store is nil")
	}
	if c == nil {
		return false, fmt.Errorf("container snapshot is nil")
	}
	if c.ID == "" || c.PID <= 0 || c.PIDStartTime == 0 {
		return false, fmt.Errorf("container process generation is incomplete")
	}
	if cleanup == nil {
		return false, fmt.Errorf("generation cleanup function is nil")
	}

	changed, stateErr := st.MarkStoppedIfIdentity(c.ID, c.PID, c.PIDStartTime, exitCode, finishedAt)
	if stateErr != nil {
		stateErr = fmt.Errorf("persist stopped state for container %s: %w", c.ID, stateErr)
	}

	cleanupErr := cleanup(c.ID, c.PID, c.PIDStartTime)
	if cleanupErr != nil {
		cleanupErr = fmt.Errorf("cleanup stopped process generation for container %s: %w", c.ID, cleanupErr)
	}

	return changed, errors.Join(stateErr, cleanupErr)
}
