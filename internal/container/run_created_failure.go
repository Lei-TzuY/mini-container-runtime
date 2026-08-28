package container

import (
	"errors"
	"fmt"
	"time"

	"minicontainer/internal/state"
)

// finalizeCreatedRunFailure records a managed Run failure that happened before
// any process generation was durably admitted. Once the record has left
// StatusCreated, generation-specific finalization is authoritative and this is
// deliberately a no-op.
func finalizeCreatedRunFailure(st *state.Store, id string, runErr error, finishedAt time.Time) error {
	if st == nil || id == "" || runErr == nil {
		return runErr
	}

	changed, err := st.MarkStoppedIfCreated(id, 1, finishedAt)
	if err != nil {
		return errors.Join(
			runErr,
			&runtimeStateError{err: fmt.Errorf("persist pre-generation stopped state for container %s: %w", id, err)},
		)
	}
	if !changed {
		return runErr
	}
	return runErr
}
