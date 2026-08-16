package container

import (
	"fmt"
	"path/filepath"

	"minicontainer/internal/cgroups"
	"minicontainer/internal/state"
)

// FreezeContainer pauses a running container via Cgroup v2 freeze.
func FreezeContainer(st *state.Store, containerID string) error {
	c, err := st.Resolve(containerID)
	if err != nil {
		return fmt.Errorf("resolve container: %w", err)
	}

	if c.Status != state.StatusRunning {
		return fmt.Errorf("container %s is not running", c.ID[:8])
	}

	cgroupPath := filepath.Join("/sys/fs/cgroup", c.ID)
	return cgroups.Freeze(cgroupPath)
}

// ThawContainer unpauses a frozen container via Cgroup v2 unfreeze.
func ThawContainer(st *state.Store, containerID string) error {
	c, err := st.Resolve(containerID)
	if err != nil {
		return fmt.Errorf("resolve container: %w", err)
	}

	cgroupPath := filepath.Join("/sys/fs/cgroup", c.ID)
	return cgroups.Unfreeze(cgroupPath)
}
