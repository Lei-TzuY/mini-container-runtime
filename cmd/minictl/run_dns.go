package main

import (
	"errors"
	"fmt"

	"minicontainer/internal/container"
	"minicontainer/internal/dns"
)

const runDNSNetworkName = "default"
const runDNSBridgeIP = "172.20.0.2"

type runDNSRegisterFunc func(networkName, containerID, hostname, ipAddr string) error
type runDNSInjectFunc func(rootfsPath, networkName string) error
type runDNSUnregisterFunc func(networkName, containerID string) error

type runDNSRegistration struct {
	containerID string
	unregister  runDNSUnregisterFunc
	active      bool
}

func admitRunDNS(cfg container.Config, containerID string) (*runDNSRegistration, error) {
	return admitRunDNSWith(cfg, containerID, dns.RegisterHost, dns.InjectHostsIntoRootFS, dns.UnregisterHost)
}

func admitRunDNSWith(
	cfg container.Config,
	containerID string,
	register runDNSRegisterFunc,
	inject runDNSInjectFunc,
	unregister runDNSUnregisterFunc,
) (*runDNSRegistration, error) {
	if !cfg.BridgeNetwork {
		return &runDNSRegistration{}, nil
	}
	if containerID == "" {
		return nil, fmt.Errorf("bridge DNS admission: container ID is empty")
	}
	if register == nil || inject == nil || unregister == nil {
		return nil, fmt.Errorf("bridge DNS admission: operation is nil")
	}
	if err := register(runDNSNetworkName, containerID, cfg.Hostname, runDNSBridgeIP); err != nil {
		return nil, fmt.Errorf("register bridge DNS host: %w", err)
	}

	registration := &runDNSRegistration{
		containerID: containerID,
		unregister:  unregister,
		active:      true,
	}
	if err := inject(cfg.RootFS, runDNSNetworkName); err != nil {
		rollbackErr := registration.Close()
		return nil, errors.Join(
			fmt.Errorf("validate bridge hosts injection: %w", err),
			rollbackErr,
		)
	}
	return registration, nil
}

func (r *runDNSRegistration) Close() error {
	if r == nil || !r.active {
		return nil
	}
	if r.unregister == nil {
		return fmt.Errorf("unregister bridge DNS host: operation is nil")
	}
	if err := r.unregister(runDNSNetworkName, r.containerID); err != nil {
		return fmt.Errorf("unregister bridge DNS host: %w", err)
	}
	r.active = false
	return nil
}
