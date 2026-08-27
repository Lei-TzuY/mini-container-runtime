package main

import (
	"errors"
	"fmt"

	"minicontainer/internal/container"
	"minicontainer/internal/dns"
	"minicontainer/internal/state"
)

const defaultRunDNSNetwork = "default"
const defaultRunBridgeIP = "172.20.0.2"

type runDNSDeps struct {
	validateRootFS func(rootfsPath, networkName string) error
	registerHost   func(networkName, containerID, hostname, ipAddr string) error
	unregisterHost func(networkName, containerID string) error
}

func defaultRunDNSDeps() runDNSDeps {
	return runDNSDeps{
		validateRootFS: dns.InjectHostsIntoRootFS,
		registerHost:   dns.RegisterHost,
		unregisterHost: dns.UnregisterHost,
	}
}

// admitRunDNS establishes bridge service-discovery state before the runtime is
// allowed to spawn a child. Validation runs before the durable registration so
// deterministic rootfs failures do not need rollback. A registration failure is
// admission failure: silently running without the requested discovery contract
// is not acceptable.
func admitRunDNS(cfg *container.Config, containerID string, deps runDNSDeps) (bool, error) {
	if cfg == nil {
		return false, fmt.Errorf("run config is nil")
	}
	if !cfg.BridgeNetwork {
		return false, nil
	}
	if containerID == "" {
		return false, fmt.Errorf("container ID is empty")
	}
	if deps.validateRootFS == nil || deps.registerHost == nil || deps.unregisterHost == nil {
		return false, fmt.Errorf("run DNS dependencies are incomplete")
	}
	if err := deps.validateRootFS(cfg.RootFS, defaultRunDNSNetwork); err != nil {
		return false, fmt.Errorf("validate bridge DNS rootfs: %w", err)
	}
	if err := deps.registerHost(defaultRunDNSNetwork, containerID, cfg.Hostname, defaultRunBridgeIP); err != nil {
		return false, fmt.Errorf("register bridge DNS host: %w", err)
	}
	return true, nil
}

// completeRunDNS removes discovery state only after the lifecycle store proves
// that the synchronous run reached stopped state. If state is unresolved or the
// process still appears running, the process-generation ownership recorded by
// dns.RegisterHost remains the safer cleanup authority and stale-entry pruning
// can recover it after the registrar exits.
func completeRunDNS(registered bool, settled *state.Container, containerID string, deps runDNSDeps) error {
	if !registered {
		return nil
	}
	if settled == nil || settled.Status != state.StatusStopped {
		return nil
	}
	if containerID == "" {
		return fmt.Errorf("container ID is empty")
	}
	if deps.unregisterHost == nil {
		return fmt.Errorf("run DNS unregister dependency is nil")
	}
	if err := deps.unregisterHost(defaultRunDNSNetwork, containerID); err != nil {
		return fmt.Errorf("unregister stopped container from bridge DNS: %w", err)
	}
	return nil
}

func joinRunDNSCompletion(runErr, dnsErr error) error {
	return errors.Join(runErr, dnsErr)
}
