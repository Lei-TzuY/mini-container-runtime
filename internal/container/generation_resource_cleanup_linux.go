//go:build linux

package container

import (
	"errors"
	"fmt"

	"minicontainer/internal/cgroups"
	"minicontainer/internal/network"
	"minicontainer/internal/state"
)

func cleanupNetworkGenerationIfOwnedWith(
	st *state.Store,
	containerID string,
	pid int,
	pidStartTime uint64,
	debug bool,
	removePort ownedPortCleanupFunc,
	removeVeth ownedVethCleanupFunc,
) error {
	ownership, ok, err := st.GetNetworkOwnership(containerID)
	if err != nil {
		return fmt.Errorf("read network ownership for container %s: %w", containerID, err)
	}
	if !ok {
		return nil
	}
	if ownership.PID != pid || ownership.PIDStartTime != pidStartTime {
		// A stale lifecycle actor must never consume durable cleanup evidence
		// belonging to a newer process generation. Legacy unbound ownership is
		// intentionally left for generic stopped-state recovery.
		return nil
	}
	if err := cleanupNetworkOwnershipWith(st, containerID, ownership, debug, removePort, removeVeth); err != nil {
		return fmt.Errorf("cleanup network ownership for generation %d/%d: %w", pid, pidStartTime, err)
	}
	return nil
}

func cleanupNetworkGenerationIfOwned(st *state.Store, containerID string, pid int, pidStartTime uint64, debug bool) error {
	return cleanupNetworkGenerationIfOwnedWith(
		st,
		containerID,
		pid,
		pidStartTime,
		debug,
		network.RemovePortForwardingOwned,
		network.RemoveVethHostOwned,
	)
}

func cleanupCgroupGenerationIfOwnedWith(
	st *state.Store,
	containerID string,
	pid int,
	pidStartTime uint64,
	cleanup generationCleanupFunc,
) error {
	ownership, ok, err := st.GetCgroupOwnership(containerID)
	if err != nil {
		return fmt.Errorf("read cgroup ownership for container %s: %w", containerID, err)
	}
	if !ok {
		return nil
	}
	if ownership.PID != pid || ownership.PIDStartTime != pidStartTime {
		return nil
	}
	if err := cleanupOwnedGenerationWith(st, containerID, ownership, cleanup); err != nil {
		return fmt.Errorf("cleanup cgroup ownership for generation %d/%d: %w", pid, pidStartTime, err)
	}
	return nil
}

func cleanupRuntimeGenerationResourcesWith(
	st *state.Store,
	containerID string,
	pid int,
	pidStartTime uint64,
	cgroupCleanup generationCleanupFunc,
	removePort ownedPortCleanupFunc,
	removeVeth ownedVethCleanupFunc,
) error {
	return errors.Join(
		cleanupCgroupGenerationIfOwnedWith(st, containerID, pid, pidStartTime, cgroupCleanup),
		cleanupNetworkGenerationIfOwnedWith(st, containerID, pid, pidStartTime, false, removePort, removeVeth),
	)
}

func cleanupRuntimeGenerationResources(st *state.Store, containerID string, pid int, pidStartTime uint64) error {
	return cleanupRuntimeGenerationResourcesWith(
		st,
		containerID,
		pid,
		pidStartTime,
		cleanupContainerProcessGeneration,
		network.RemovePortForwardingOwned,
		network.RemoveVethHostOwned,
	)
}

var _ = cgroups.NameForContainerProcess
