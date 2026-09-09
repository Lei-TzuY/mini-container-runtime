//go:build linux

package container

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"minicontainer/internal/network"
)

func TestAllocateRuntimeBridgeLeaseIsGenerationScoped(t *testing.T) {
	cfg := Config{ContainerID: "bridge-generation-test", StateDir: t.TempDir()}
	alive := func(int, uint64) (bool, error) { return true, nil }

	ip1, config1, release1, err := allocateRuntimeBridgeLeaseWithProbe(cfg, 101, 1001, alive)
	if err != nil {
		t.Fatal(err)
	}
	if ip1 != "172.20.0.2" || config1.ContainerCIDR != "172.20.0.2/24" || config1.Gateway != "172.20.0.1" {
		t.Fatalf("first lease = ip %q config %+v", ip1, config1)
	}

	ip2, config2, release2, err := allocateRuntimeBridgeLeaseWithProbe(cfg, 202, 2002, alive)
	if err != nil {
		t.Fatal(err)
	}
	if ip2 != "172.20.0.3" || config2.ContainerCIDR != "172.20.0.3/24" || config2.Gateway != "172.20.0.1" {
		t.Fatalf("second lease = ip %q config %+v", ip2, config2)
	}

	if err := release1(); err != nil {
		t.Fatal(err)
	}
	ip3, _, release3, err := allocateRuntimeBridgeLeaseWithProbe(cfg, 303, 3003, alive)
	if err != nil {
		t.Fatal(err)
	}
	if ip3 != "172.20.0.2" {
		t.Fatalf("third generation IP = %q, want recycled 172.20.0.2", ip3)
	}

	if err := release2(); err != nil {
		t.Fatal(err)
	}
	if err := release3(); err != nil {
		t.Fatal(err)
	}
}

func TestAllocateRuntimeBridgeLeaseReconcilesExactLinuxProcessGenerations(t *testing.T) {
	stateDir := t.TempDir()
	cfg := Config{ContainerID: "new-generation", StateDir: stateDir}
	ipam, err := network.OpenIPAM(filepath.Join(stateDir, "ipam"))
	if err != nil {
		t.Fatal(err)
	}

	pid := os.Getpid()
	start, err := ProcessStartTime(pid)
	if err != nil {
		t.Fatalf("current process start time: %v", err)
	}
	live := network.BridgeLeaseOwner{ContainerID: "live-generation", PID: pid, PIDStartTime: start}
	stale := network.BridgeLeaseOwner{ContainerID: "stale-generation", PID: pid, PIDStartTime: start + 1}
	liveIP, err := ipam.AllocateGenerationIP(defaultBridgeDNSNetwork, defaultBridgeSubnetCIDR, live)
	if err != nil {
		t.Fatal(err)
	}
	staleIP, err := ipam.AllocateGenerationIP(defaultBridgeDNSNetwork, defaultBridgeSubnetCIDR, stale)
	if err != nil {
		t.Fatal(err)
	}
	if liveIP != "172.20.0.2" || staleIP != "172.20.0.3" {
		t.Fatalf("seed leases = live %q stale %q", liveIP, staleIP)
	}

	ip, _, release, err := allocateRuntimeBridgeLease(cfg, pid, start)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = release() }()
	if ip != "172.20.0.3" {
		t.Fatalf("new generation IP = %q, want reclaimed stale address 172.20.0.3", ip)
	}

	gotLive, err := ipam.AllocateGenerationIP(defaultBridgeDNSNetwork, defaultBridgeSubnetCIDR, live)
	if err != nil {
		t.Fatal(err)
	}
	if gotLive != liveIP {
		t.Fatalf("live generation moved from %q to %q", liveIP, gotLive)
	}
}

func TestReconcileRuntimeBridgeIPAMFailsClosedOnProbeError(t *testing.T) {
	ipam, err := network.OpenIPAM(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	owner := network.BridgeLeaseOwner{ContainerID: "indeterminate", PID: 77, PIDStartTime: 88}
	ip, err := ipam.AllocateGenerationIP(defaultBridgeDNSNetwork, defaultBridgeSubnetCIDR, owner)
	if err != nil {
		t.Fatal(err)
	}
	probeErr := errors.New("probe unavailable")
	if err := reconcileRuntimeBridgeIPAM(ipam, func(int, uint64) (bool, error) {
		return false, probeErr
	}); err == nil || !strings.Contains(err.Error(), probeErr.Error()) {
		t.Fatalf("reconcile error = %v, want probe error", err)
	}

	got, err := ipam.AllocateGenerationIP(defaultBridgeDNSNetwork, defaultBridgeSubnetCIDR, owner)
	if err != nil {
		t.Fatal(err)
	}
	if got != ip {
		t.Fatalf("indeterminate lease moved from %q to %q", ip, got)
	}
}
