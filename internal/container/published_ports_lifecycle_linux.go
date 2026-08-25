//go:build linux

package container

import (
	"fmt"

	"minicontainer/internal/state"
)

// requireDurablePublishedPortOwnership fails closed before process creation when
// host-persistent port-forwarding rules would have no durable cleanup owner.
// Pure bridge networking does not need this gate because it installs no tagged
// DNAT rules through the publish path.
func requireDurablePublishedPortOwnership(cfg Config, lifecycleStore *state.Store) error {
	if len(cfg.PortMappings) == 0 {
		return nil
	}
	if lifecycleStore != nil {
		return nil
	}
	return &runtimeStateError{err: fmt.Errorf("published ports require managed lifecycle state for durable network cleanup")}
}
