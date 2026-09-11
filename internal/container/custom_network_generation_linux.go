//go:build linux

package container

import (
	"errors"
	"fmt"

	"minicontainer/internal/dns"
	"minicontainer/internal/network"
)

type customBridgeProfileInspector func(string) (network.BridgeIPv4Profile, error)
type customBridgeGenerationSetup func(int, string, string, []PortMapping, string, string, bool) (func() error, error)
type customBridgeLeaseOwnership func(string) (func() error, error)

// setupCustomBridgeGeneration composes the live custom-network profile, a
// generation-scoped IP/DNS lease, host veth attachment, and DNS publication.
// The returned cleanup unwinds host networking before releasing the lease.
func setupCustomBridgeGeneration(cfg Config, pid int, pidStartTime uint64, networkOwner, networkName string, debug bool) (string, runtimeBridgeConfig, func() error, error) {
	return setupCustomBridgeGenerationWithLeaseOwnership(
		cfg,
		pid,
		pidStartTime,
		networkOwner,
		networkName,
		debug,
		nil,
		network.InspectBridgeIPv4Owned,
		setupBridgeHostOwnedOnNetwork,
	)
}

// setupCustomBridgeGenerationDurable adds a durability hook after a generation
// lease is allocated but before host networking is materialized. This lets the
// runtime persist enough ownership to recover veth/DNAT resources if it dies
// during setup without widening the crash window used by the default bridge.
func setupCustomBridgeGenerationDurable(
	cfg Config,
	pid int,
	pidStartTime uint64,
	networkOwner string,
	networkName string,
	debug bool,
	ownLease customBridgeLeaseOwnership,
) (string, runtimeBridgeConfig, func() error, error) {
	return setupCustomBridgeGenerationWithLeaseOwnership(
		cfg,
		pid,
		pidStartTime,
		networkOwner,
		networkName,
		debug,
		ownLease,
		network.InspectBridgeIPv4Owned,
		setupBridgeHostOwnedOnNetwork,
	)
}

func setupCustomBridgeGenerationWith(
	cfg Config,
	pid int,
	pidStartTime uint64,
	networkOwner string,
	networkName string,
	debug bool,
	inspect customBridgeProfileInspector,
	setup customBridgeGenerationSetup,
) (string, runtimeBridgeConfig, func() error, error) {
	return setupCustomBridgeGenerationWithLeaseOwnership(
		cfg,
		pid,
		pidStartTime,
		networkOwner,
		networkName,
		debug,
		nil,
		inspect,
		setup,
	)
}

func setupCustomBridgeGenerationWithLeaseOwnership(
	cfg Config,
	pid int,
	pidStartTime uint64,
	networkOwner string,
	networkName string,
	debug bool,
	ownLease customBridgeLeaseOwnership,
	inspect customBridgeProfileInspector,
	setup customBridgeGenerationSetup,
) (string, runtimeBridgeConfig, func() error, error) {
	if inspect == nil {
		return "", runtimeBridgeConfig{}, nil, fmt.Errorf("custom bridge profile inspector is nil")
	}
	if setup == nil {
		return "", runtimeBridgeConfig{}, nil, fmt.Errorf("custom bridge generation setup is nil")
	}
	if networkOwner == "" {
		return "", runtimeBridgeConfig{}, nil, fmt.Errorf("bridge ownership marker is required")
	}
	if networkName == "" {
		return "", runtimeBridgeConfig{}, nil, fmt.Errorf("custom bridge network name is required")
	}

	profile, err := inspect(networkName)
	if err != nil {
		return "", runtimeBridgeConfig{}, nil, fmt.Errorf("inspect custom bridge network %s: %w", networkName, err)
	}

	containerIP, bridgeConfig, releaseLease, err := allocateRuntimeBridgeLeaseOnNetwork(
		cfg,
		pid,
		pidStartTime,
		networkName,
		profile.SubnetCIDR,
		profile.Gateway,
	)
	if err != nil {
		return "", runtimeBridgeConfig{}, nil, err
	}

	var cleanupOwnership func() error
	if ownLease != nil {
		cleanupOwnership, err = ownLease(containerIP)
		if err != nil {
			if cleanupErr := releaseLease(); cleanupErr != nil {
				err = errors.Join(err, cleanupErr)
			}
			return "", runtimeBridgeConfig{}, nil, err
		}
	}

	cleanupDurableOwnership := func() error {
		if cleanupOwnership == nil {
			return nil
		}
		return cleanupOwnership()
	}

	cleanupAllocatedGeneration := func(cleanupHost func() error) error {
		var cleanupErr error
		if cleanupHost != nil {
			cleanupErr = cleanupHost()
		}
		if err := cleanupDurableOwnership(); err != nil {
			cleanupErr = errors.Join(cleanupErr, err)
		}
		if err := releaseLease(); err != nil {
			cleanupErr = errors.Join(cleanupErr, err)
		}
		return cleanupErr
	}

	cleanupHost, err := setup(
		pid,
		profile.HostCIDR,
		containerIP,
		cfg.PortMappings,
		networkOwner,
		networkName,
		debug,
	)
	if err != nil {
		if cleanupErr := cleanupAllocatedGeneration(nil); cleanupErr != nil {
			err = errors.Join(err, cleanupErr)
		}
		return "", runtimeBridgeConfig{}, nil, fmt.Errorf("configure custom bridge network %s: %w", networkName, err)
	}

	cleanup := func() error {
		return cleanupAllocatedGeneration(cleanupHost)
	}

	if err := dns.BindHostRegistrationGeneration(networkName, cfg.ContainerID, pid, pidStartTime); err != nil {
		setupErr := error(fmt.Errorf("bind custom bridge DNS registration to child generation: %w", err))
		if cleanupErr := cleanup(); cleanupErr != nil {
			setupErr = errors.Join(setupErr, fmt.Errorf("cleanup custom bridge network after DNS generation bind failure: %w", cleanupErr))
		}
		return "", runtimeBridgeConfig{}, nil, setupErr
	}

	return containerIP, bridgeConfig, cleanup, nil
}
