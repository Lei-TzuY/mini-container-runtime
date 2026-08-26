//go:build linux

package container

import (
	"strings"
	"testing"

	"minicontainer/internal/state"
)

func TestBridgeNetworkingRequiresDurableLifecycleStore(t *testing.T) {
	err := requireDurableNetworkOwnership(Config{BridgeNetwork: true}, nil)
	if err == nil || !strings.Contains(err.Error(), "bridge networking requires managed lifecycle state") {
		t.Fatalf("unmanaged bridge error=%v", err)
	}
	if !isRuntimeControlError(err) {
		t.Fatalf("unmanaged bridge was not classified as runtime-control: %v", err)
	}
}

func TestPublishedPortsRequireBridgeNetworking(t *testing.T) {
	st, err := state.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	cfg := Config{PortMappings: []PortMapping{{HostPort: 8080, ContainerPort: 80}}}
	for name, store := range map[string]*state.Store{
		"unmanaged": nil,
		"managed":   st,
	} {
		t.Run(name, func(t *testing.T) {
			err := requireDurableNetworkOwnership(cfg, store)
			if err == nil || !strings.Contains(err.Error(), "published ports require bridge networking") {
				t.Fatalf("publish-without-bridge error=%v", err)
			}
			if !isRuntimeControlError(err) {
				t.Fatalf("publish-without-bridge was not classified as runtime-control: %v", err)
			}
		})
	}
}

func TestNetworkPreflightAllowsManagedBridgeAndNonNetworkingRuns(t *testing.T) {
	st, err := state.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	for _, cfg := range []Config{
		{BridgeNetwork: true},
		{BridgeNetwork: true, PortMappings: []PortMapping{{HostPort: 8080, ContainerPort: 80}}},
	} {
		if err := requireDurableNetworkOwnership(cfg, st); err != nil {
			t.Fatalf("managed network config %+v rejected: %v", cfg, err)
		}
	}
	if err := requireDurableNetworkOwnership(Config{}, nil); err != nil {
		t.Fatalf("ordinary unmanaged run rejected: %v", err)
	}
}
