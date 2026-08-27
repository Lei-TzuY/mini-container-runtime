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
	unregisterDNSHost func(networkName, containerID string) error
}

func defaultNetworkAdmissionDeps() networkAdmissionDeps {
	return networkAdmissionDeps{
		validateDNSRootFS: dns.InjectHostsIntoRootFS,
		registerDNSHost:   dns.RegisterHost,
		unregisterDNSHost: dns.UnregisterHostOwned,
	}
}

// requireDurableNetworkOwnership preserves the narrow validation API used by
// focused tests. Production Run uses beginNetworkAttemptAdmission so each
// restart attempt receives its own DNS registration and owned rollback token.
func requireDurableNetworkOwnership(cfg Config, lifecycleStore *state.Store) error {
	_, err := beginNetworkAttemptAdmissionWith(cfg, lifecycleStore, defaultNetworkAdmissionDeps())
	return err
}

func requireDurableNetworkOwnershipWith(cfg Config, lifecycleStore *state.Store, deps networkAdmissionDeps) error {
	_, err := beginNetworkAttemptAdmissionWith(cfg, lifecycleStore, deps)
	return err
}

// beginNetworkAttemptAdmission fails closed before process creation whenever
// host networking can create resources that outlive the runtime parent. Bridge
// veth ownership and published-port DNAT ownership both require a managed state
// store so a later lifecycle actor can safely recover after a parent crash.
//
// Bridge service discovery is attempt-scoped: every restart attempt validates
// and registers independently, and receives an owned rollback that is safe to
// invoke even after authoritative generation finalization already removed the
// same entry. This closes both pre-spawn registration leaks and the historical
// gap where attempt 1 finalization removed DNS state before attempt 2 started.
func beginNetworkAttemptAdmission(cfg Config, lifecycleStore *state.Store) (func() error, error) {
	return beginNetworkAttemptAdmissionWith(cfg, lifecycleStore, defaultNetworkAdmissionDeps())
}

func beginNetworkAttemptAdmissionWith(cfg Config, lifecycleStore *state.Store, deps networkAdmissionDeps) (func() error, error) {
	if len(cfg.PortMappings) > 0 && !cfg.BridgeNetwork {
		return nil, &runtimeSetupError{err: fmt.Errorf("published ports require bridge networking")}
	}
	if !cfg.BridgeNetwork && len(cfg.PortMappings) == 0 {
		return nil, nil
	}
	if lifecycleStore == nil {
		if cfg.BridgeNetwork {
			return nil, &runtimeStateError{err: fmt.Errorf("bridge networking requires managed lifecycle state for durable network cleanup")}
		}
		return nil, &runtimeStateError{err: fmt.Errorf("published ports require managed lifecycle state for durable network cleanup")}
	}
	if !cfg.BridgeNetwork {
		return nil, nil
	}
	if cfg.ContainerID == "" {
		return nil, &runtimeStateError{err: fmt.Errorf("bridge networking requires a managed container ID")}
	}
	if deps.validateDNSRootFS == nil || deps.registerDNSHost == nil || deps.unregisterDNSHost == nil {
		return nil, &runtimeSetupError{err: fmt.Errorf("bridge DNS admission dependencies are incomplete")}
	}
	if err := deps.validateDNSRootFS(cfg.RootFS, defaultBridgeDNSNetwork); err != nil {
		return nil, &runtimeSetupError{err: fmt.Errorf("validate bridge DNS rootfs: %w", err)}
	}
	if err := deps.registerDNSHost(defaultBridgeDNSNetwork, cfg.ContainerID, cfg.Hostname, defaultBridgeContainerIP); err != nil {
		return nil, &runtimeSetupError{err: fmt.Errorf("register bridge DNS host: %w", err)}
	}

	rollback := func() error {
		if err := deps.unregisterDNSHost(defaultBridgeDNSNetwork, cfg.ContainerID); err != nil {
			return fmt.Errorf("unregister bridge DNS host: %w", err)
		}
		return nil
	}
	return rollback, nil
}
