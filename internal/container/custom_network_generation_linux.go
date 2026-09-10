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

// setupCustomBridgeGeneration composes the live custom-network profile, a
// generation-scoped IP/DNS lease, host veth attachment, and DNS publication.
// The returned cleanup unwinds host networking before releasing the lease.
func setupCustomBridgeGeneration(cfg Config, pid int, pidStartTime uint64, networkOwner, networkName string, debug bool) (string, runtimeBridgeConfig, func() error, error) {
	return setupCustomBridgeGenerationWith(
		cfg,
		pid,
		pidStartTime,
		networkOwner,
		networkName,
		debug,
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
		if cleanupErr := releaseLease(); cleanupErr != nil {
			err = errors.Join(err, cleanupErr)
		}
		return "", runtimeBridgeConfig{}, nil, fmt.Errorf("configure custom bridge network %s: %w", networkName, err)
	}

	cleanup := func() error {
		var cleanupErr error
		if cleanupHost != nil {
			cleanupErr = cleanupHost()
		}
		if err := releaseLease(); err != nil {
			cleanupErr = errors.Join(cleanupErr, err)
		}
		return cleanupErr
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
