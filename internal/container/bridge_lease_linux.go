//go:build linux

package container

import (
	"fmt"
	"path/filepath"

	"minicontainer/internal/network"
	"minicontainer/internal/state"
)

const (
	defaultBridgeSubnetCIDR = "172.20.0.0/24"
	defaultBridgeHostCIDR   = "172.20.0.1/24"
	defaultBridgeGateway    = "172.20.0.1"
)

func allocateRuntimeBridgeLease(cfg Config, pid int, pidStartTime uint64) (string, runtimeBridgeConfig, func() error, error) {
	if cfg.ContainerID == "" {
		return "", runtimeBridgeConfig{}, nil, fmt.Errorf("bridge IP lease requires a container ID")
	}

	stateDir := cfg.StateDir
	if stateDir == "" {
		stateDir = state.DefaultDir()
	}
	ipam, err := network.OpenIPAM(filepath.Join(stateDir, "ipam"))
	if err != nil {
		return "", runtimeBridgeConfig{}, nil, fmt.Errorf("open bridge IPAM: %w", err)
	}

	owner := network.BridgeLeaseOwner{
		ContainerID:  cfg.ContainerID,
		PID:          pid,
		PIDStartTime: pidStartTime,
	}
	ip, err := ipam.AllocateGenerationIP(defaultBridgeDNSNetwork, defaultBridgeSubnetCIDR, owner)
	if err != nil {
		return "", runtimeBridgeConfig{}, nil, fmt.Errorf("allocate bridge IP for container generation: %w", err)
	}

	config := runtimeBridgeConfig{
		ContainerCIDR: ip + "/24",
		Gateway:       defaultBridgeGateway,
	}
	release := func() error {
		if err := ipam.ReleaseGenerationIP(defaultBridgeDNSNetwork, owner); err != nil {
			return fmt.Errorf("release bridge IP for container generation: %w", err)
		}
		return nil
	}
	return ip, config, release, nil
}
