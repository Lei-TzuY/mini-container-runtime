//go:build linux

package container

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"time"

	"minicontainer/internal/state"
)

// runtimeControlError marks failures owned by the runtime rather than the
// container payload. Restart policies must never retry these errors because a
// retry cannot fix missing isolation/resource controls and may create repeated
// unmanaged or incorrectly constrained processes.
type runtimeControlError interface {
	error
	runtimeControlFailure()
}

type runtimeSetupError struct {
	err error
}

func (e *runtimeSetupError) Error() string { return e.err.Error() }
func (e *runtimeSetupError) Unwrap() error { return e.err }
func (e *runtimeSetupError) runtimeControlFailure() {}

func isRuntimeControlError(err error) bool {
	if err == nil {
		return false
	}
	var controlErr runtimeControlError
	return errors.As(err, &controlErr)
}

func resourceLimitsRequested(cfg Config) bool {
	return cfg.Memory != 0 || cfg.CPUWeight != 0 || cfg.CPUs != 0 || cfg.PidsLimit != 0
}

// abortRuntimeSetupFailure terminates a child that is still blocked on the
// parent/child sync pipe and reconciles the persisted running identity. The
// payload has not been released yet, so failing closed here prevents a
// container from running without controls that the caller explicitly required.
func abortRuntimeSetupFailure(
	cmd *exec.Cmd,
	writePipe *os.File,
	lifecycleStore *state.Store,
	containerID string,
	childPID int,
	childStartTime uint64,
	cause error,
) error {
	setupErr := &runtimeSetupError{err: cause}
	abortBlockedChild(cmd, writePipe)

	if lifecycleStore == nil {
		return setupErr
	}

	_, stateErr := lifecycleStore.MarkStoppedIfIdentity(
		containerID,
		childPID,
		childStartTime,
		-1,
		time.Now(),
	)
	if stateErr != nil {
		return errors.Join(
			setupErr,
			&runtimeStateError{err: fmt.Errorf("persist stopped state after runtime setup failure for container %s: %w", containerID, stateErr)},
		)
	}
	return setupErr
}
