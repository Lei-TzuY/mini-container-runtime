//go:build linux

package container

import (
	"errors"
	"fmt"
	"os/exec"
	"time"

	"minicontainer/internal/state"
)

type managedGenerationFinalizer func(*state.Store, *state.Container, int, time.Time) (bool, error)

// finalizeManagedParentExit reconciles the authoritative parent-side lifecycle
// state after the child has exited. A successful cgroup Apply is the ownership
// proof required before deleting a generation cgroup: deriving the same name is
// not sufficient because Apply may have failed on a pre-existing cgroup.
func finalizeManagedParentExit(
	st *state.Store,
	snapshot *state.Container,
	exitCode int,
	finishedAt time.Time,
	cgroupApplied bool,
	finalizeGeneration managedGenerationFinalizer,
) error {
	if st == nil {
		return fmt.Errorf("state store is nil")
	}
	if snapshot == nil {
		return fmt.Errorf("container snapshot is nil")
	}

	if cgroupApplied {
		if finalizeGeneration == nil {
			return fmt.Errorf("generation finalizer is nil")
		}
		_, err := finalizeGeneration(st, snapshot, exitCode, finishedAt)
		return err
	}

	_, err := st.MarkStoppedIfIdentity(
		snapshot.ID,
		snapshot.PID,
		snapshot.PIDStartTime,
		exitCode,
		finishedAt,
	)
	if err != nil {
		return fmt.Errorf("persist stopped state for container %s: %w", snapshot.ID, err)
	}
	return nil
}

// parentExitResult combines the payload result with authoritative parent-side
// teardown failures. Teardown failures are runtime-control failures: restart
// policies must not launch another generation while isolation cleanup or
// lifecycle finalization is incomplete. The original *exec.ExitError remains
// discoverable through errors.As when the payload itself exited non-zero.
func parentExitResult(waitErr, finalizationErr, bridgeCleanupErr error) error {
	var resultErr error
	if waitErr != nil {
		if exitErr, ok := waitErr.(*exec.ExitError); ok {
			resultErr = exitErr
		} else {
			resultErr = fmt.Errorf("container exited with error: %w", waitErr)
		}
	}
	if finalizationErr != nil {
		resultErr = errors.Join(resultErr, &runtimeSetupError{err: fmt.Errorf("finalize stopped process generation: %w", finalizationErr)})
	}
	if bridgeCleanupErr != nil {
		resultErr = errors.Join(resultErr, bridgeCleanupErr)
	}
	return resultErr
}
