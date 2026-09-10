//go:build linux

package container

import (
	"fmt"

	"minicontainer/internal/network"
)

type bridgeNetworkAttacher func(hostName, networkName string, debug bool) error

// setupBridgeHostOwnedOnNetwork composes the generation-owned veth setup used
// by the runtime with attachment to a verified minictl-owned custom bridge.
// Port forwarding is installed only after the host veth has joined that bridge.
func setupBridgeHostOwnedOnNetwork(containerPID int, hostCIDR, containerIP string, mappings []PortMapping, owner, networkName string, debug bool) (func() error, error) {
	if owner == "" {
		return nil, fmt.Errorf("bridge ownership marker is required")
	}
	if networkName == "" {
		return nil, fmt.Errorf("custom bridge network name is required")
	}
	return setupBridgeHostOwnedOnNetworkWith(
		containerPID,
		hostCIDR,
		containerIP,
		mappings,
		owner,
		networkName,
		debug,
		defaultBridgeHostOps(owner),
		network.AttachVethHostToBridgeOwned,
	)
}

func setupBridgeHostOwnedOnNetworkWith(containerPID int, hostCIDR, containerIP string, mappings []PortMapping, owner, networkName string, debug bool, ops bridgeHostOps, attach bridgeNetworkAttacher) (func() error, error) {
	if owner == "" {
		return nil, fmt.Errorf("bridge ownership marker is required")
	}
	if networkName == "" {
		return nil, fmt.Errorf("custom bridge network name is required")
	}
	if attach == nil {
		return nil, fmt.Errorf("custom bridge attachment operation is nil")
	}
	if ops.setupVeth == nil || ops.removeVeth == nil || ops.setupPort == nil || ops.removePort == nil {
		return nil, fmt.Errorf("bridge host network operation is nil")
	}

	setupVeth := ops.setupVeth
	hostVeth := network.VethHostIfaceOwned(owner)
	ops.setupVeth = func(pid int, cidr string, debug bool) error {
		if err := setupVeth(pid, cidr, debug); err != nil {
			return err
		}
		if err := attach(hostVeth, networkName, debug); err != nil {
			return fmt.Errorf("attach host veth %s to custom network %s: %w", hostVeth, networkName, err)
		}
		return nil
	}

	// The managed runtime persists network ownership before host mutation and
	// owns reconciliation after failures, matching setupBridgeHostOwned.
	return setupBridgeHostWithOpsPolicy(containerPID, hostCIDR, containerIP, mappings, debug, ops, false)
}
