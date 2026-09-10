//go:build linux

package container

import "testing"

func TestMultiContainerBridgeLeaseFeedsDistinctDNATTargets(t *testing.T) {
	stateDir := t.TempDir()
	cfgA := Config{ContainerID: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", StateDir: stateDir, BridgeNetwork: true}
	cfgB := Config{ContainerID: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", StateDir: stateDir, BridgeNetwork: true}
	probeAlive := func(pid int, start uint64) (bool, error) { return true, nil }

	ipA, _, releaseA, err := allocateRuntimeBridgeLeaseWithProbe(cfgA, 41001, 1, probeAlive)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := releaseA(); err != nil {
			t.Errorf("release A lease: %v", err)
		}
	}()
	ipB, _, releaseB, err := allocateRuntimeBridgeLeaseWithProbe(cfgB, 41002, 2, probeAlive)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := releaseB(); err != nil {
			t.Errorf("release B lease: %v", err)
		}
	}()

	if ipA == ipB {
		t.Fatalf("distinct live generations received same bridge IP %q", ipA)
	}

	type target struct {
		owner string
		ip    string
	}
	var targets []target
	setup := func(owner string, pid int, ip string, hostPort int) func() error {
		t.Helper()
		cleanup, err := setupBridgeHostWithOps(pid, defaultBridgeHostCIDR, ip, []PortMapping{{HostPort: hostPort, ContainerPort: 80, Protocol: "tcp"}}, false, bridgeHostOps{
			setupVeth:  func(int, string, bool) error { return nil },
			removeVeth: func(int, bool) error { return nil },
			setupPort: func(_, _ int, containerIP, _ string, _ bool) error {
				targets = append(targets, target{owner: owner, ip: containerIP})
				return nil
			},
			removePort: func(_, _ int, containerIP, _ string, _ bool) error {
				if containerIP != ip {
					t.Fatalf("cleanup target for %s = %q, want %q", owner, containerIP, ip)
				}
				return nil
			},
		})
		if err != nil {
			t.Fatalf("setup %s bridge: %v", owner, err)
		}
		return cleanup
	}

	cleanupA := setup(cfgA.ContainerID, 41001, ipA, 18081)
	cleanupB := setup(cfgB.ContainerID, 41002, ipB, 18082)

	if len(targets) != 2 {
		t.Fatalf("DNAT setup calls = %d, want 2", len(targets))
	}
	if targets[0].ip != ipA || targets[1].ip != ipB {
		t.Fatalf("DNAT targets = %#v, want %s then %s", targets, ipA, ipB)
	}

	if err := cleanupA(); err != nil {
		t.Fatalf("cleanup A bridge: %v", err)
	}
	if len(targets) != 2 || targets[1].ip != ipB {
		t.Fatalf("cleaning A disturbed B DNAT target: %#v", targets)
	}
	if err := cleanupB(); err != nil {
		t.Fatalf("cleanup B bridge: %v", err)
	}
}
