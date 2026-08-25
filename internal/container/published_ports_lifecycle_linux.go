//go:build linux

package container

import (
	"fmt"

	"minicontainer/internal/state"
)

// requireDurableNetworkOwnership fails closed before process creation whenever
// host networking can create resources that outlive the runtime parent. Bridge
// veth ownership and published-port DNAT ownership both require a managed state
// store so a later lifecycle actor can safely recover after a parent crash.
func requireDurableNetworkOwnership(cfg Config, lifecycleStore *state.Store) error {
	if !cfg.BridgeNetwork && len(cfg.PortMappings) == 0 {
		return nil
	}
	if lifecycleStore != nil {
		return nil
	}
	if cfg.BridgeNetwork {
		return &runtimeStateError{err: fmt.Errorf("bridge networking requires managed lifecycle state for durable network cleanup")}
	}
	return &runtimeStateError{err: fmt.Errorf("published ports require managed lifecycle state for durable network cleanup")}
}
