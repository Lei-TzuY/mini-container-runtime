//go:build linux

package container

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"time"

	"minicontainer/internal/cgroups"
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

func (*runtimeStateError) runtimeControlFailure() {}

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

func runtimeCgroupName(containerID string, childPID int, childStartTime uint64, managed bool) (string, error) {
	if !managed {
		if childPID <= 0 {
			return "", fmt.Errorf("invalid cgroup target PID %d", childPID)
		}
		return fmt.Sprintf("minicontainer-%d", childPID), nil
	}
	return cgroups.NameForContainerProcess(containerID, childPID, childStartTime)
}

// abortRuntimeSetupFailure terminates and reaps a child that is still blocked
// on the parent/child sync pipe, cleans only cgroup paths that the child was
// observed to own, and reconciles the persisted running identity. Capturing
// cgroup membership before termination avoids deleting a same-named cgroup when
// Apply failed because the runtime never acquired that cgroup. If a prior Apply
// succeeded and durable ownership exists, cleanup is retried from that token
// after the stopped state is persisted so the token is cleared only on success.
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
	resultErr := error(setupErr)

	var ownedCgroups *cgroups.ProcessCleanup
	cgroupName, nameErr := runtimeCgroupName(containerID, childPID, childStartTime, lifecycleStore != nil)
	if nameErr != nil {
		resultErr = errors.Join(resultErr, fmt.Errorf("derive aborted cgroup identity: %w", nameErr))
	} else {
		captured, captureErr := cgroups.CaptureProcessCleanup(cgroupName, childPID)
		if captureErr != nil {
			resultErr = errors.Join(resultErr, fmt.Errorf("capture aborted cgroup ownership: %w", captureErr))
		} else {
			ownedCgroups = captured
		}
	}

	abortBlockedChild(cmd, writePipe)

	if ownedCgroups != nil && !ownedCgroups.Empty() {
		if err := ownedCgroups.Remove(false); err != nil {
			resultErr = errors.Join(resultErr, fmt.Errorf("cleanup aborted cgroup: %w", err))
		}
	}

	if lifecycleStore == nil {
		return resultErr
	}

	_, stateErr := lifecycleStore.MarkStoppedIfIdentity(
		containerID,
		childPID,
		childStartTime,
		-1,
		time.Now(),
	)
	if stateErr != nil {
		resultErr = errors.Join(
			resultErr,
			&runtimeStateError{err: fmt.Errorf("persist stopped state after runtime setup failure for container %s: %w", containerID, stateErr)},
		)
	}

	current, readErr := lifecycleStore.Get(containerID)
	if readErr != nil {
		resultErr = errors.Join(resultErr, &runtimeStateError{err: fmt.Errorf("reload container after runtime setup failure for container %s: %w", containerID, readErr)})
		return resultErr
	}
	if current.Status == state.StatusStopped {
		if cleanupErr := CleanupStoppedCgroup(lifecycleStore, current); cleanupErr != nil {
			resultErr = errors.Join(resultErr, &runtimeSetupError{err: fmt.Errorf("cleanup persisted cgroup after runtime setup failure for container %s: %w", containerID, cleanupErr)})
		}
	}
	return resultErr
}
