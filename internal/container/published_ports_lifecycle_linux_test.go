//go:build linux

package container

import (
	"strings"
	"testing"

	"minicontainer/internal/state"
)

func TestPublishedPortsRequireDurableLifecycleStore(t *testing.T) {
	cfg := Config{
		BridgeNetwork: true,
		PortMappings: []PortMapping{{HostPort: 8080, ContainerPort: 80}},
	}
	err := requireDurablePublishedPortOwnership(cfg, nil)
	if err == nil || !strings.Contains(err.Error(), "published ports require managed lifecycle state") {
		t.Fatalf("unmanaged published ports error=%v", err)
	}
	if !isRuntimeControlError(err) {
		t.Fatalf("unmanaged published ports were not classified as runtime-control: %v", err)
	}
}

func TestPublishedPortPreflightAllowsManagedAndNonPublishingRuns(t *testing.T) {
	st, err := state.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	published := Config{
		BridgeNetwork: true,
		PortMappings: []PortMapping{{HostPort: 8080, ContainerPort: 80}},
	}
	if err := requireDurablePublishedPortOwnership(published, st); err != nil {
		t.Fatalf("managed published ports rejected: %v", err)
	}
	if err := requireDurablePublishedPortOwnership(Config{BridgeNetwork: true}, nil); err != nil {
		t.Fatalf("pure unmanaged bridge rejected: %v", err)
	}
	if err := requireDurablePublishedPortOwnership(Config{}, nil); err != nil {
		t.Fatalf("ordinary unmanaged run rejected: %v", err)
	}
}
