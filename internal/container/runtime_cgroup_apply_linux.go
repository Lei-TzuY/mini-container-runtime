//go:build linux

package container

import (
	"errors"
	"fmt"
	"os"

	"minicontainer/internal/cgroups"
	"minicontainer/internal/state"
)

type cgroupApplyFunc func(pid int, cfg cgroups.Config, debug bool) error

// applyCgroupWithDurableOwnership closes the crash window between host cgroup
// mutation and persistence of the generation-scoped cleanup token. Managed
// runtimes durably reserve the exact cgroup name before Apply is allowed to
// mutate /sys/fs/cgroup. A failed or partial Apply intentionally leaves that
// token in place so stopped-generation recovery can remove any debris.
func applyCgroupWithDurableOwnership(
	st *state.Store,
	containerID string,
	pid int,
	pidStartTime uint64,
	cfg cgroups.Config,
	debug bool,
	apply cgroupApplyFunc,
) (bool, error) {
	if apply == nil {
		return false, &runtimeSetupError{err: fmt.Errorf("cgroup apply operation is nil")}
	}
	if st != nil {
		spec, err := st.RestartSpec(containerID)
		switch {
		case err == nil:
			cfg.CPUSetCPUs = spec.CPUSetCPUs
			cfg.CPUSetMems = spec.CPUSetMems
		case errors.Is(err, os.ErrNotExist):
			// Older/direct managed callers may not have a durable restart spec.
			// Preserve their existing cgroup semantics while refusing to hide any
			// other state corruption or read failure.
		default:
			return false, &runtimeStateError{err: fmt.Errorf("load durable resource policy before cgroup apply for container %s: %w", containerID, err)}
		}
		if err := st.MarkCgroupOwnedIfIdentity(containerID, pid, pidStartTime, cfg.Name); err != nil {
			return false, &runtimeStateError{err: fmt.Errorf("persist cgroup ownership before apply for container %s: %w", containerID, err)}
		}
	}
	if err := apply(pid, cfg, debug); err != nil {
		return false, err
	}
	return true, nil
}
