package container

import (
	"fmt"

	"minicontainer/internal/cgroups"
	"minicontainer/internal/state"
)

func openRunningCgroup(st *state.Store, containerID string) (*state.Container, *ProcessHandle, string, error) {
	c, handle, err := openRunningProcess(st, containerID)
	if err != nil {
		return nil, nil, "", err
	}

	name, err := cgroups.NameForContainerProcess(c.ID, c.PID, c.PIDStartTime)
	if err != nil {
		handle.Close()
		return nil, nil, "", fmt.Errorf("derive cgroup identity: %w", err)
	}
	return c, handle, name, nil
}

// FreezeContainer pauses the exact running process generation stored for a
// container. The generation-derived cgroup name prevents a concurrent restart
// or PID reuse from redirecting the freeze operation to another process.
func FreezeContainer(st *state.Store, containerID string) error {
	_, handle, cgroupName, err := openRunningCgroup(st, containerID)
	if err != nil {
		return err
	}
	defer handle.Close()

	if err := cgroups.Freeze(cgroupName); err != nil {
		return fmt.Errorf("freeze container cgroup: %w", err)
	}
	return nil
}

// ThawContainer resumes the exact running process generation stored for a
// container.
func ThawContainer(st *state.Store, containerID string) error {
	_, handle, cgroupName, err := openRunningCgroup(st, containerID)
	if err != nil {
		return err
	}
	defer handle.Close()

	if err := cgroups.Unfreeze(cgroupName); err != nil {
		return fmt.Errorf("unfreeze container cgroup: %w", err)
	}
	return nil
}

// UpdateContainerResources applies resource changes only to the cgroup for the
// currently persisted process generation.
func UpdateContainerResources(st *state.Store, containerID string, cfg cgroups.UpdateConfig, debug bool) error {
	_, handle, cgroupName, err := openRunningCgroup(st, containerID)
	if err != nil {
		return err
	}
	defer handle.Close()

	if err := cgroups.UpdateLimits(cgroupName, cfg, debug); err != nil {
		return fmt.Errorf("update container cgroup: %w", err)
	}
	return nil
}
