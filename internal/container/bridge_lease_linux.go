//go:build linux

package container

import (
	"errors"
	"fmt"
	"net"
	"path/filepath"

	"minicontainer/internal/dns"
	"minicontainer/internal/network"
	"minicontainer/internal/state"
)

const (
	defaultBridgeSubnetCIDR = "172.20.0.0/24"
	defaultBridgeHostCIDR   = "172.20.0.1/24"
	defaultBridgeGateway    = "172.20.0.1"
)

func allocateRuntimeBridgeLease(cfg Config, pid int, pidStartTime uint64) (string, runtimeBridgeConfig, func() error, error) {
	return allocateRuntimeBridgeLeaseWithProbe(cfg, pid, pidStartTime, probeProcessGeneration)
}

func allocateRuntimeBridgeLeaseWithProbe(cfg Config, pid int, pidStartTime uint64, probe processGenerationProbe) (string, runtimeBridgeConfig, func() error, error) {
	return allocateRuntimeBridgeLeaseOnNetworkWithProbe(
		cfg,
		pid,
		pidStartTime,
		defaultBridgeDNSNetwork,
		defaultBridgeSubnetCIDR,
		defaultBridgeGateway,
		probe,
	)
}

// allocateRuntimeBridgeLeaseOnNetwork scopes IPAM and DNS staging to one custom
// network. This lets independent custom bridges reuse address space without
// sharing leases or DNS ownership, while preserving generation-aware cleanup.
func allocateRuntimeBridgeLeaseOnNetwork(cfg Config, pid int, pidStartTime uint64, networkName, subnetCIDR, gateway string) (string, runtimeBridgeConfig, func() error, error) {
	return allocateRuntimeBridgeLeaseOnNetworkWithProbe(cfg, pid, pidStartTime, networkName, subnetCIDR, gateway, probeProcessGeneration)
}

func allocateRuntimeBridgeLeaseOnNetworkWithProbe(cfg Config, pid int, pidStartTime uint64, networkName, subnetCIDR, gateway string, probe processGenerationProbe) (string, runtimeBridgeConfig, func() error, error) {
	if cfg.ContainerID == "" {
		return "", runtimeBridgeConfig{}, nil, fmt.Errorf("bridge IP lease requires a container ID")
	}
	if networkName == "" {
		return "", runtimeBridgeConfig{}, nil, fmt.Errorf("bridge IP lease requires a network name")
	}
	if probe == nil {
		return "", runtimeBridgeConfig{}, nil, fmt.Errorf("bridge IP lease process probe is nil")
	}

	gatewayIP := net.ParseIP(gateway)
	_, subnet, err := net.ParseCIDR(subnetCIDR)
	if err != nil || subnet == nil || subnet.IP.To4() == nil {
		return "", runtimeBridgeConfig{}, nil, fmt.Errorf("invalid bridge subnet %q", subnetCIDR)
	}
	if gatewayIP == nil || gatewayIP.To4() == nil || !subnet.Contains(gatewayIP) {
		return "", runtimeBridgeConfig{}, nil, fmt.Errorf("bridge gateway %q is outside subnet %s", gateway, subnet.String())
	}
	prefixLen, bits := subnet.Mask.Size()
	if bits != 32 || prefixLen < 0 {
		return "", runtimeBridgeConfig{}, nil, fmt.Errorf("invalid bridge subnet mask %q", subnetCIDR)
	}

	stateDir := cfg.StateDir
	if stateDir == "" {
		stateDir = state.DefaultDir()
	}
	ipam, err := network.OpenIPAM(filepath.Join(stateDir, "ipam"))
	if err != nil {
		return "", runtimeBridgeConfig{}, nil, fmt.Errorf("open bridge IPAM: %w", err)
	}
	if err := reconcileRuntimeBridgeIPAMNetwork(ipam, networkName, probe); err != nil {
		return "", runtimeBridgeConfig{}, nil, err
	}

	owner := network.BridgeLeaseOwner{
		ContainerID:  cfg.ContainerID,
		PID:          pid,
		PIDStartTime: pidStartTime,
	}
	ip, err := ipam.AllocateGenerationIP(networkName, subnet.String(), owner)
	if err != nil {
		return "", runtimeBridgeConfig{}, nil, fmt.Errorf("allocate bridge IP for container generation: %w", err)
	}

	cancelDNSAddress, err := dns.StageHostRegistrationGenerationAddress(networkName, cfg.ContainerID, ip)
	if err != nil {
		releaseErr := ipam.ReleaseGenerationIP(networkName, owner)
		setupErr := error(fmt.Errorf("stage bridge DNS address for container generation: %w", err))
		if releaseErr != nil {
			setupErr = errors.Join(setupErr, fmt.Errorf("release bridge IP after DNS address staging failure: %w", releaseErr))
		}
		return "", runtimeBridgeConfig{}, nil, setupErr
	}

	config := runtimeBridgeConfig{
		ContainerCIDR: fmt.Sprintf("%s/%d", ip, prefixLen),
		Gateway:       gatewayIP.String(),
	}
	release := func() error {
		cancelDNSAddress()
		if err := ipam.ReleaseGenerationIP(networkName, owner); err != nil {
			return fmt.Errorf("release bridge IP for container generation: %w", err)
		}
		return nil
	}
	return ip, config, release, nil
}

func reconcileRuntimeBridgeIPAM(ipam *network.IPAM, probe processGenerationProbe) error {
	return reconcileRuntimeBridgeIPAMNetwork(ipam, defaultBridgeDNSNetwork, probe)
}

func reconcileRuntimeBridgeIPAMNetwork(ipam *network.IPAM, networkName string, probe processGenerationProbe) error {
	if ipam == nil {
		return fmt.Errorf("bridge IPAM is nil")
	}
	if networkName == "" {
		return fmt.Errorf("bridge IPAM network name is empty")
	}
	if probe == nil {
		return fmt.Errorf("bridge IP lease process probe is nil")
	}

	var probeErr error
	_, err := ipam.ReconcileGenerationIPs(networkName, func(owner network.BridgeLeaseOwner) bool {
		alive, err := probe(owner.PID, owner.PIDStartTime)
		if err != nil {
			// Fail closed: an indeterminate process identity must retain its lease.
			probeErr = errors.Join(probeErr, fmt.Errorf("probe bridge lease owner %s process %d/%d: %w", owner.ContainerID, owner.PID, owner.PIDStartTime, err))
			return true
		}
		return alive
	})
	if err != nil {
		return fmt.Errorf("reconcile bridge IP leases for network %s: %w", networkName, err)
	}
	if probeErr != nil {
		return fmt.Errorf("reconcile bridge IP lease liveness for network %s: %w", networkName, probeErr)
	}
	return nil
}
