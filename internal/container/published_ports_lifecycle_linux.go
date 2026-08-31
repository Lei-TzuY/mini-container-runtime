//go:build linux

package container

import (
	"fmt"
	"os"

	"minicontainer/internal/dns"
	"minicontainer/internal/events"
	"minicontainer/internal/state"
)

const defaultBridgeDNSNetwork = "default"
const defaultBridgeContainerIP = "172.20.0.2"

type networkAdmissionDeps struct {
	validateDNSRootFS func(rootfsPath, networkName string) error
	beginDNSAttempt   func(networkName, containerID, hostname, ipAddr string) (func() error, error)
	registerDNSHost   func(networkName, containerID, hostname, ipAddr string) error
	unregisterDNSHost func(networkName, containerID string) error
}

func defaultNetworkAdmissionDeps() networkAdmissionDeps {
	return networkAdmissionDeps{
		validateDNSRootFS: dns.InjectHostsIntoRootFS,
		beginDNSAttempt:   dns.BeginHostRegistrationAttempt,
		registerDNSHost:   dns.RegisterHost,
		unregisterDNSHost: dns.UnregisterHostOwned,
	}
}

func validateAdmittedRootFSIdentity(cfg Config) error {
	if cfg.RootFSIdentity == nil {
		return nil
	}
	current, err := os.Stat(cfg.RootFS)
	if err != nil {
		return &runtimeSetupError{err: fmt.Errorf("revalidate admitted rootfs %q: %w", cfg.RootFS, err)}
	}
	if !current.IsDir() {
		return &runtimeSetupError{err: fmt.Errorf("revalidate admitted rootfs %q: no longer a directory", cfg.RootFS)}
	}
	if !os.SameFile(cfg.RootFSIdentity, current) {
		return &runtimeSetupError{err: fmt.Errorf("revalidate admitted rootfs %q: filesystem identity changed before runtime attempt", cfg.RootFS)}
	}
	return nil
}

func requireDurableNetworkOwnership(cfg Config, lifecycleStore *state.Store) error {
	return requireDurableNetworkOwnershipWith(cfg, lifecycleStore, defaultNetworkAdmissionDeps())
}

func requireDurableNetworkOwnershipWith(cfg Config, lifecycleStore *state.Store, deps networkAdmissionDeps) error {
	rollback, err := beginNetworkAttemptAdmissionWith(cfg, lifecycleStore, deps)
	if err != nil {
		return err
	}
	if rollback == nil {
		return nil
	}
	if err := rollback(); err != nil {
		return &runtimeSetupError{err: fmt.Errorf("rollback network validation admission: %w", err)}
	}
	return nil
}

func rollbackNetworkAdmissionAfterRun(lifecycleStore *state.Store, containerID string, rollback func() error) error {
	if rollback == nil {
		return nil
	}
	if lifecycleStore == nil {
		return fmt.Errorf("lifecycle store is nil while rolling back bridge DNS admission")
	}
	current, err := lifecycleStore.Get(containerID)
	if err != nil {
		return fmt.Errorf("read lifecycle state before bridge DNS rollback: %w", err)
	}
	switch current.Status {
	case state.StatusCreated:
		return rollback()
	case state.StatusRunning, state.StatusStopped:
		return nil
	default:
		return fmt.Errorf("refuse bridge DNS rollback for container %s with unknown lifecycle state %q", containerID, current.Status)
	}
}

func beginNetworkAttemptAdmission(cfg Config, lifecycleStore *state.Store) (func() error, error) {
	if err := validateAdmittedRootFSIdentity(cfg); err != nil {
		return nil, err
	}
	if cfg.ContainerID == "" {
		return beginNetworkAttemptAdmissionWith(cfg, lifecycleStore, defaultNetworkAdmissionDeps())
	}
	if err := events.StageRuntimeStart(cfg.ContainerID, cfg.RootFS, "started container"); err != nil {
		return nil, &runtimeSetupError{err: fmt.Errorf("stage runtime start event: %w", err)}
	}

	networkRollback, err := beginNetworkAttemptAdmissionWith(cfg, lifecycleStore, defaultNetworkAdmissionDeps())
	if err != nil {
		events.CancelPendingStart(cfg.ContainerID)
		return nil, err
	}
	rollback := func() error {
		events.CancelPendingStart(cfg.ContainerID)
		if networkRollback == nil {
			return nil
		}
		return rollbackNetworkAdmissionAfterRun(lifecycleStore, cfg.ContainerID, networkRollback)
	}
	return rollback, nil
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
	if deps.validateDNSRootFS == nil || (deps.beginDNSAttempt == nil && (deps.registerDNSHost == nil || deps.unregisterDNSHost == nil)) {
		return nil, &runtimeSetupError{err: fmt.Errorf("bridge DNS admission dependencies are incomplete")}
	}
	if err := deps.validateDNSRootFS(cfg.RootFS, defaultBridgeDNSNetwork); err != nil {
		return nil, &runtimeSetupError{err: fmt.Errorf("validate bridge DNS rootfs: %w", err)}
	}

	if deps.beginDNSAttempt != nil {
		dnsRollback, err := deps.beginDNSAttempt(defaultBridgeDNSNetwork, cfg.ContainerID, cfg.Hostname, defaultBridgeContainerIP)
		if err != nil {
			return nil, &runtimeSetupError{err: fmt.Errorf("register bridge DNS host: %w", err)}
		}
		if dnsRollback == nil {
			return nil, &runtimeSetupError{err: fmt.Errorf("register bridge DNS host returned nil attempt rollback")}
		}
		return func() error {
			if err := dnsRollback(); err != nil {
				return fmt.Errorf("unregister bridge DNS host: %w", err)
			}
			return nil
		}, nil
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
