//go:build linux

package container

import (
	"os"
	"testing"
)

func TestBridgeConfigHandoffDrivesContainerSetup(t *testing.T) {
	t.Setenv(sentinelEnvKey, "1")
	readPipe, writePipe, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}

	want := runtimeBridgeConfig{ContainerCIDR: "172.20.0.17/24", Gateway: "172.20.0.1"}
	if err := releaseBlockedChildWithBridge(writePipe, want); err != nil {
		t.Fatalf("release bridge config: %v", err)
	}
	if err := awaitParentReady(readPipe); err != nil {
		t.Fatalf("await bridge config: %v", err)
	}

	var gotCIDR, gotGateway string
	if err := setupBridgeContainerWith(true, "172.20.0.2/24", "172.20.0.1", false, func(cidr, gateway string, _ bool) error {
		gotCIDR, gotGateway = cidr, gateway
		return nil
	}); err != nil {
		t.Fatalf("setup bridge container: %v", err)
	}
	if gotCIDR != want.ContainerCIDR || gotGateway != want.Gateway {
		t.Fatalf("container bridge config=(%q,%q), want (%q,%q)", gotCIDR, gotGateway, want.ContainerCIDR, want.Gateway)
	}

	// The handoff is generation-local and single-use; a later setup must not
	// accidentally inherit the previous container's addressing.
	gotCIDR, gotGateway = "", ""
	if err := setupBridgeContainerWith(true, "172.20.0.2/24", "172.20.0.1", false, func(cidr, gateway string, _ bool) error {
		gotCIDR, gotGateway = cidr, gateway
		return nil
	}); err != nil {
		t.Fatalf("setup bridge container fallback: %v", err)
	}
	if gotCIDR != "172.20.0.2/24" || gotGateway != "172.20.0.1" {
		t.Fatalf("stale bridge handoff leaked into next setup: (%q,%q)", gotCIDR, gotGateway)
	}
}
