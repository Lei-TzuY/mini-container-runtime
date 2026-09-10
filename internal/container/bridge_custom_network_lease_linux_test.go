//go:build linux

package container

import (
	"os/exec"
	"testing"
)

func TestCustomNetworkBridgeLeasesAreScopedAcrossLiveGenerations(t *testing.T) {
	startBlocked := func(t *testing.T) (*exec.Cmd, func()) {
		t.Helper()
		cmd := exec.Command("sh", "-c", "read line")
		stdin, err := cmd.StdinPipe()
		if err != nil {
			t.Fatal(err)
		}
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		cleanup := func() {
			_ = stdin.Close()
			_ = cmd.Wait()
		}
		return cmd, cleanup
	}

	cmdA, cleanupA := startBlocked(t)
	defer cleanupA()
	cmdB, cleanupB := startBlocked(t)
	defer cleanupB()

	startA, err := ProcessStartTime(cmdA.Process.Pid)
	if err != nil {
		t.Fatal(err)
	}
	startB, err := ProcessStartTime(cmdB.Process.Pid)
	if err != nil {
		t.Fatal(err)
	}

	stateDir := t.TempDir()
	cfgA := Config{ContainerID: "custom-network-a", StateDir: stateDir, BridgeNetwork: true}
	cfgB := Config{ContainerID: "custom-network-b", StateDir: stateDir, BridgeNetwork: true}

	ipA, bridgeA, releaseA, err := allocateRuntimeBridgeLeaseOnNetwork(
		cfgA,
		cmdA.Process.Pid,
		startA,
		"frontend",
		"10.77.0.0/24",
		"10.77.0.1",
	)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = releaseA() }()

	ipB, bridgeB, releaseB, err := allocateRuntimeBridgeLeaseOnNetwork(
		cfgB,
		cmdB.Process.Pid,
		startB,
		"backend",
		"10.77.0.0/24",
		"10.77.0.1",
	)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = releaseB() }()

	// Address pools are intentionally identical. Network-scoped ownership means
	// each bridge can independently assign the first usable container address.
	if ipA != ipB {
		t.Fatalf("independent custom networks allocated different first addresses: frontend=%s backend=%s", ipA, ipB)
	}
	if bridgeA.ContainerCIDR != ipA+"/24" || bridgeB.ContainerCIDR != ipB+"/24" {
		t.Fatalf("container CIDRs not derived from custom subnet: frontend=%q backend=%q", bridgeA.ContainerCIDR, bridgeB.ContainerCIDR)
	}
	if bridgeA.Gateway != "10.77.0.1" || bridgeB.Gateway != "10.77.0.1" {
		t.Fatalf("custom gateways not propagated: frontend=%q backend=%q", bridgeA.Gateway, bridgeB.Gateway)
	}
}

func TestCustomNetworkBridgeLeaseRejectsGatewayOutsideSubnet(t *testing.T) {
	cfg := Config{ContainerID: "custom-network-invalid", StateDir: t.TempDir(), BridgeNetwork: true}
	_, _, _, err := allocateRuntimeBridgeLeaseOnNetworkWithProbe(
		cfg,
		101,
		1001,
		"frontend",
		"10.77.0.0/24",
		"10.78.0.1",
		func(int, uint64) (bool, error) { return true, nil },
	)
	if err == nil {
		t.Fatal("gateway outside custom network subnet was accepted")
	}
}
