//go:build linux

package container

import (
	"fmt"

	"minicontainer/internal/dns"
	"minicontainer/internal/state"
)

const defaultBridgeDNSNetwork = "default"
const defaultBridgeContainerIP = "172.20.0.2"

type networkAdmissionDeps struct {
	validateDNSRootFS func(rootfsPath, networkName string) error
	registerDNSHost   func(networkName, containerID, hostname, ipAddr string) error
}

func defaultNetworkAdmissionDeps() networkAdmissionDeps {
	return networkAdmissionDeps{
		validateDNSRootFS: dns.InjectHostsIntoRootFS,
		registerDNSHost:   dns.RegisterHost,
	}
}

// requireDurableNetworkOwnership fails closed before process creation whenever
// host networking can create resources that outlive the runtime parent. Bridge
// veth ownership and published-port DNAT ownership both require a managed state
// store so a later lifecycle actor can safely recover after a parent crash.
//
// Bridge service discovery is part of the same admission contract. The CLI has
// historically treated DNS registration as best effort; the runtime must not
// therefore trust that caller-side setup succeeded. Re-validating and
// registering here makes DNS failure authoritative before runOnce can spawn a
// process. Registrar generation ownership keeps a committed entry recoverable
// if a later runtime setup failure exits through os.Exit and skips CLI defers.
func requireDurableNetworkOwnership(cfg Config, lifecycleStore *state.Store) error {
	return requireDurableNetworkOwnershipWith(cfg, lifecycleStore, defaultNetworkAdmissionDeps())
}

func requireDurableNetworkOwnershipWith(cfg Config, lifecycleStore *state.Store, deps networkAdmissionDeps) error {
	if len(cfg.PortMappings) > 0 && !cfg.BridgeNetwork {
		return &runtimeSetupError{err: fmt.Errorf("published ports require bridge networking")}
	}
	if !cfg.BridgeNetwork && len(cfg.PortMappings) == 0 {
		return nil
	}
	if lifecycleStore == nil {
		if cfg.BridgeNetwork {
			return &runtimeStateError{err: fmt.Errorf("bridge networking requires managed lifecycle state for durable network cleanup")}
		}
		return &runtimeStateError{err: fmt.Errorf("published ports require managed lifecycle state for durable network cleanup")}
	}
	if !cfg.BridgeNetwork {
		return nil
	}
	if cfg.ContainerID == "" {
		return &runtimeStateError{err: fmt.Errorf("bridge networking requires a managed container ID")}
	}
	if deps.validateDNSRootFS == nil || deps.registerDNSHost == nil {
		return &runtimeSetupError{err: fmt.Errorf("bridge DNS admission dependencies are incomplete")}
	}
	if err := deps.validateDNSRootFS(cfg.RootFS, defaultBridgeDNSNetwork); err != nil {
		return &runtimeSetupError{err: fmt.Errorf("validate bridge DNS rootfs: %w", err)}
	}
	if err := deps.registerDNSHost(defaultBridgeDNSNetwork, cfg.ContainerID, cfg.Hostname, defaultBridgeContainerIP); err != nil {
		return &runtimeSetupError{err: fmt.Errorf("register bridge DNS host: %w", err)}
	}
	return nil
}
