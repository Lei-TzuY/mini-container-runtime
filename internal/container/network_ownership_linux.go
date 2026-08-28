//go:build linux

package container

import (
	"errors"
	"fmt"

	"minicontainer/internal/dns"
	"minicontainer/internal/network"
	"minicontainer/internal/state"
)

type ownedPortCleanupFunc func(owner string, hostPort, containerPort int, containerIP, protocol string, debug bool) error
type ownedVethCleanupFunc func(name, owner string, debug bool) error

func networkOwnershipForGeneration(owner string, pid int, pidStartTime uint64, containerIP string, mappings []PortMapping) state.NetworkOwnership {
	owned := state.NetworkOwnership{
		Owner:        owner,
		PID:          pid,
		PIDStartTime: pidStartTime,
		VethHost:     network.VethHostIfaceOwned(owner),
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
	removePort ownedPortCleanupFunc,
	removeVeth ownedVethCleanupFunc,
) error {
	if st == nil {
		return fmt.Errorf("state store is nil")
	}
	if removePort == nil || removeVeth == nil {
		return fmt.Errorf("owned network cleanup function is nil")
	}

	var cleanupErrs []error
	for i := len(ownership.Mappings) - 1; i >= 0; i-- {
		mapping := ownership.Mappings[i]
		if err := removePort(
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
	if ownership.VethHost != "" {
		if err := removeVeth(ownership.VethHost, ownership.Owner, debug); err != nil {
			cleanupErrs = append(cleanupErrs, fmt.Errorf("remove persisted host veth %s: %w", ownership.VethHost, err))
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

// cleanupNetworkOwnershipAfterDurableStopWith is the destructive-cleanup gate
// for generic callers that may run before authoritative lifecycle finalization.
// A running/created record is a durable claim that host networking may still be
// in use, so cleanup must be a no-op until stopped has committed. State read
// failures fail closed and preserve both host resources and the ownership token.
func cleanupNetworkOwnershipAfterDurableStopWith(
	st *state.Store,
	containerID string,
	ownership state.NetworkOwnership,
	debug bool,
	removePort ownedPortCleanupFunc,
	removeVeth ownedVethCleanupFunc,
) error {
	if st == nil {
		return fmt.Errorf("state store is nil")
	}
	current, err := st.Get(containerID)
	if err != nil {
		return fmt.Errorf("read lifecycle state before network cleanup for container %s: %w", containerID, err)
	}
	if current.Status != state.StatusStopped {
		return nil
	}
	return cleanupNetworkOwnershipWith(st, containerID, ownership, debug, removePort, removeVeth)
}

func cleanupNetworkOwnership(st *state.Store, containerID string, ownership state.NetworkOwnership, debug bool) error {
	return cleanupNetworkOwnershipAfterDurableStopWith(
		st,
		containerID,
		ownership,
		debug,
		network.RemovePortForwardingOwned,
		network.RemoveVethHostOwned,
	)
}

// CleanupStoppedNetwork retries generation-owned host-network cleanup after a
// parent crash or an earlier teardown failure. Legacy containers without a
// network sidecar are no-ops; rules-only sidecars from older runtimes remain
// supported.
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
		return fmt.Errorf("cleanup persisted network resources for stopped container %s: %w", c.ID, err)
	}
	return nil
}

func cleanupStoppedDNSRegistration(c *state.Container) error {
	if c.PID > 0 && c.PIDStartTime != 0 {
		return dns.CleanupStoppedHostRegistrationGeneration(defaultBridgeDNSNetwork, c.ID, c.PID, c.PIDStartTime)
	}
	// Historical stopped records may predate durable child-generation identity.
	// They cannot participate in exact CAS teardown, so retain the prior retry
	// semantics: reclaim provably stale foreign entries, but never consume a
	// registration owned by this current registrar process.
	return dns.CleanupStoppedHostRegistrationForFinalization(defaultBridgeDNSNetwork, c.ID, false)
}

// CleanupStoppedRuntimeResources retries every durable host-side cleanup token
// currently known for a stopped generation. Independent failures are joined so
// one resource class cannot prevent another from making progress. Modern DNS
// teardown is generation-scoped; legacy stopped records without child identity
// fall back to conservative registrar-scoped recovery.
func CleanupStoppedRuntimeResources(st *state.Store, c *state.Container) error {
	if c == nil {
		return errors.Join(CleanupStoppedCgroup(st, c), CleanupStoppedNetwork(st, c))
	}
	return errors.Join(
		CleanupStoppedCgroup(st, c),
		CleanupStoppedNetwork(st, c),
		cleanupStoppedDNSRegistration(c),
	)
}
