//go:build linux

package container

import (
	"errors"
	"fmt"

	"minicontainer/internal/network"
)

type bridgeHostOps struct {
	setupVeth  func(containerPID int, hostCIDR string, debug bool) error
	removeVeth func(containerPID int, debug bool) error
	setupPort  func(hostPort, containerPort int, containerIP, protocol string, debug bool) error
	removePort func(hostPort, containerPort int, containerIP, protocol string, debug bool) error
}

func defaultBridgeHostOps() bridgeHostOps {
	return bridgeHostOps{
		setupVeth:  network.SetupVethHost,
		removeVeth: network.RemoveVethHost,
		setupPort:  network.SetupPortForwarding,
		removePort: network.RemovePortForwarding,
	}
}

// setupBridgeHost establishes all requested host-side bridge networking before
// the container child is released from its sync pipe. On failure it rolls back
// every side effect it knows was installed. The returned cleanup function owns
// the same resources after successful setup and should be called when the
// container exits.
func setupBridgeHost(containerPID int, hostCIDR, containerIP string, mappings []PortMapping, debug bool) (func() error, error) {
	return setupBridgeHostWithOps(containerPID, hostCIDR, containerIP, mappings, debug, defaultBridgeHostOps())
}

func setupBridgeHostWithOps(containerPID int, hostCIDR, containerIP string, mappings []PortMapping, debug bool, ops bridgeHostOps) (func() error, error) {
	if ops.setupVeth == nil || ops.removeVeth == nil || ops.setupPort == nil || ops.removePort == nil {
		return nil, fmt.Errorf("bridge host network operation is nil")
	}

	rollbackPorts := func(installed []PortMapping) error {
		var rollbackErrs []error
		for i := len(installed) - 1; i >= 0; i-- {
			p := installed[i]
			if err := ops.removePort(p.HostPort, p.ContainerPort, containerIP, p.Protocol, debug); err != nil {
				rollbackErrs = append(rollbackErrs,
					fmt.Errorf("remove port mapping %d:%d/%s: %w", p.HostPort, p.ContainerPort, normalizedProtocol(p.Protocol), err))
			}
		}
		return errors.Join(rollbackErrs...)
	}

	cleanup := func(installed []PortMapping) error {
		portErr := rollbackPorts(installed)
		var vethErr error
		if err := ops.removeVeth(containerPID, debug); err != nil {
			vethErr = fmt.Errorf("remove host veth during bridge cleanup: %w", err)
		}
		return errors.Join(portErr, vethErr)
	}

	if err := ops.setupVeth(containerPID, hostCIDR, debug); err != nil {
		setupErr := fmt.Errorf("setup host veth: %w", err)
		if cleanupErr := ops.removeVeth(containerPID, debug); cleanupErr != nil {
			return nil, errors.Join(setupErr, fmt.Errorf("rollback partial host veth: %w", cleanupErr))
		}
		return nil, setupErr
	}

	installed := make([]PortMapping, 0, len(mappings))
	for _, p := range mappings {
		if err := ops.setupPort(p.HostPort, p.ContainerPort, containerIP, p.Protocol, debug); err != nil {
			setupErr := fmt.Errorf("setup port mapping %d:%d/%s: %w", p.HostPort, p.ContainerPort, normalizedProtocol(p.Protocol), err)
			if cleanupErr := cleanup(installed); cleanupErr != nil {
				return nil, errors.Join(setupErr, cleanupErr)
			}
			return nil, setupErr
		}
		installed = append(installed, p)
	}

	return func() error { return cleanup(installed) }, nil
}

func normalizedProtocol(protocol string) string {
	if protocol == "" {
		return "tcp"
	}
	return protocol
}

type bridgeContainerSetup func(containerCIDR, gateway string, debug bool) error

func setupBridgeContainer(enabled bool, containerCIDR, gateway string, debug bool) error {
	return setupBridgeContainerWith(enabled, containerCIDR, gateway, debug, network.SetupVethContainer)
}

func setupBridgeContainerWith(enabled bool, containerCIDR, gateway string, debug bool, setup bridgeContainerSetup) error {
	if !enabled {
		return nil
	}
	if setup == nil {
		return fmt.Errorf("bridge container network operation is nil")
	}
	if err := setup(containerCIDR, gateway, debug); err != nil {
		return fmt.Errorf("configure container bridge network: %w", err)
	}
	return nil
}
