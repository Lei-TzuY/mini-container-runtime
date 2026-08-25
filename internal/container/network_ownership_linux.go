//go:build linux

package container

import (
	"errors"
	"fmt"

	"minicontainer/internal/network"
	"minicontainer/internal/state"
)

type ownedPortCleanupFunc func(owner string, hostPort, containerPort int, containerIP, protocol string, debug bool) error

func networkOwnershipForGeneration(owner string, pid int, pidStartTime uint64, containerIP string, mappings []PortMapping) state.NetworkOwnership {
	owned := state.NetworkOwnership{
		Owner:        owner,
		PID:          pid,
		PIDStartTime: pidStartTime,
		Mappings:     make([]state.PortForwardingOwnership, 0, len(mappings)),
	}
	for _, mapping := range mappings {
		protocol := normalizedProtocol(mapping.Protocol)
		owned.Mappings = append(owned.Mappings, state.PortForwardingOwnership{
			HostPort:      mapping.HostPort,
			ContainerPort: mapping.ContainerPort,
			ContainerIP:   containerIP,
			Protocol:      protocol,
		})
	}
	return owned
}

func cleanupNetworkOwnershipWith(
	st *state.Store,
	containerID string,
	ownership state.NetworkOwnership,
	debug bool,
	remove ownedPortCleanupFunc,
) error {
	if st == nil {
		return fmt.Errorf("state store is nil")
	}
	if remove == nil {
		return fmt.Errorf("owned port cleanup function is nil")
	}

	var cleanupErrs []error
	for i := len(ownership.Mappings) - 1; i >= 0; i-- {
		mapping := ownership.Mappings[i]
		if err := remove(
			ownership.Owner,
			mapping.HostPort,
			mapping.ContainerPort,
			mapping.ContainerIP,
			mapping.Protocol,
			debug,
		); err != nil {
			cleanupErrs = append(cleanupErrs, fmt.Errorf(
				"remove persisted port mapping %d:%d/%s: %w",
				mapping.HostPort,
				mapping.ContainerPort,
				mapping.Protocol,
				err,
			))
		}
	}
	if err := errors.Join(cleanupErrs...); err != nil {
		return err
	}

	cleared, err := st.ClearNetworkOwnershipIfMatch(containerID, ownership)
	if err != nil {
		return fmt.Errorf("clear network ownership after cleanup: %w", err)
	}
	if cleared {
		return nil
	}
	if _, ok, err := st.GetNetworkOwnership(containerID); err != nil {
		return fmt.Errorf("re-read network ownership after cleanup: %w", err)
	} else if !ok {
		// Another lifecycle actor completed the same idempotent cleanup first.
		return nil
	}
	return fmt.Errorf("network ownership changed or remained after successful cleanup")
}

func cleanupNetworkOwnership(st *state.Store, containerID string, ownership state.NetworkOwnership, debug bool) error {
	return cleanupNetworkOwnershipWith(st, containerID, ownership, debug, network.RemovePortForwardingOwned)
}

// CleanupStoppedNetwork retries generation-owned iptables cleanup after a
// parent crash or an earlier teardown failure. Legacy containers and bridge
// containers without published ports have no network sidecar and are no-ops.
func CleanupStoppedNetwork(st *state.Store, c *state.Container) error {
	if st == nil {
		return fmt.Errorf("state store is nil")
	}
	if c == nil {
		return fmt.Errorf("container snapshot is nil")
	}
	if c.ID == "" {
		return fmt.Errorf("container ID is empty")
	}
	if c.Status != state.StatusStopped {
		return fmt.Errorf("container %s is %s; network cleanup retry requires stopped state", c.ID, c.Status)
	}

	ownership, ok, err := st.GetNetworkOwnership(c.ID)
	if err != nil {
		return fmt.Errorf("read network ownership for stopped container %s: %w", c.ID, err)
	}
	if !ok {
		return nil
	}
	if err := cleanupNetworkOwnership(st, c.ID, ownership, false); err != nil {
		return fmt.Errorf("cleanup persisted network rules for stopped container %s: %w", c.ID, err)
	}
	return nil
}

// CleanupStoppedRuntimeResources retries every durable host-side cleanup token
// currently known for a stopped generation. Independent failures are joined so
// one resource class cannot prevent another from making progress.
func CleanupStoppedRuntimeResources(st *state.Store, c *state.Container) error {
	return errors.Join(CleanupStoppedCgroup(st, c), CleanupStoppedNetwork(st, c))
}
