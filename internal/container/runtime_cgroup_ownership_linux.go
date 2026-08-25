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

// persistAppliedCgroupOwnership records the cleanup token for a managed cgroup
// before the blocked child is released. Apply has already returned success, so
// this parent has authoritative proof that it created and owns cgroupName.
// Failure to persist that proof is a runtime-control failure: the child is
// reaped and the known-owned cgroup is removed instead of running without a
// durable recovery key.
func persistAppliedCgroupOwnership(
	cmd *exec.Cmd,
	writePipe *os.File,
	st *state.Store,
	containerID string,
	pid int,
	pidStartTime uint64,
	cgroupName string,
	debug bool,
) error {
	if st == nil {
		return nil
	}
	if err := st.MarkCgroupOwnedIfIdentity(containerID, pid, pidStartTime, cgroupName); err == nil {
		return nil
	} else {
		persistErr := err
		abortBlockedChild(cmd, writePipe)

		resultErr := error(&runtimeStateError{err: fmt.Errorf("persist cgroup ownership for container %s: %w", containerID, persistErr)})
		if _, stateErr := st.MarkStoppedIfIdentity(containerID, pid, pidStartTime, -1, time.Now()); stateErr != nil {
			resultErr = errors.Join(resultErr, &runtimeStateError{err: fmt.Errorf("persist stopped state after cgroup ownership failure for container %s: %w", containerID, stateErr)})
		}

		if cleanupErr := cgroups.RemoveChecked(cgroupName, debug); cleanupErr != nil {
			return errors.Join(resultErr, &runtimeSetupError{err: fmt.Errorf("cleanup owned cgroup %s after ownership persistence failure: %w", cgroupName, cleanupErr)})
		}

		_, clearErr := st.ClearCgroupOwnershipIfMatch(containerID, state.CgroupOwnership{
			Name:         cgroupName,
			PID:          pid,
			PIDStartTime: pidStartTime,
		})
		if clearErr != nil {
			resultErr = errors.Join(resultErr, &runtimeStateError{err: fmt.Errorf("clear cgroup ownership after cleanup for container %s: %w", containerID, clearErr)})
		}
		return resultErr
	}
}
