//go:build linux

package container

import "testing"

func TestBridgeLeaseCrashRecoveryPreservesOtherLiveContainer(t *testing.T) {
	stateDir := t.TempDir()
	alive := map[int]bool{101: true, 202: true}
	probe := func(pid int, _ uint64) (bool, error) { return alive[pid], nil }

	cfgA := Config{ContainerID: "bridge-a", StateDir: stateDir}
	cfgB := Config{ContainerID: "bridge-b", StateDir: stateDir}

	ipA, _, _, err := allocateRuntimeBridgeLeaseWithProbe(cfgA, 101, 1001, probe)
	if err != nil {
		t.Fatal(err)
	}
	ipB, _, releaseB, err := allocateRuntimeBridgeLeaseWithProbe(cfgB, 202, 2002, probe)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = releaseB() }()

	if ipA != "172.20.0.2" || ipB != "172.20.0.3" {
		t.Fatalf("initial bridge addresses = %q/%q, want .2/.3", ipA, ipB)
	}

	// Model an abnormal runtime death: A never executes its release callback,
	// while B remains a live generation. The next admission must reclaim only
	// A's stale lease and must not disturb B.
	alive[101] = false
	cfgRestart := Config{ContainerID: "bridge-a-restart", StateDir: stateDir}
	ipRestart, _, releaseRestart, err := allocateRuntimeBridgeLeaseWithProbe(cfgRestart, 303, 3003, probe)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = releaseRestart() }()

	if ipRestart != ipA {
		t.Fatalf("restart address = %q, want reclaimed crashed address %q", ipRestart, ipA)
	}

	// Re-admitting B's exact live generation must return its original lease,
	// proving reconciliation did not steal or move the unaffected container.
	ipBAgain, _, releaseBAgain, err := allocateRuntimeBridgeLeaseWithProbe(cfgB, 202, 2002, probe)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = releaseBAgain() }()
	if ipBAgain != ipB {
		t.Fatalf("live container B moved from %q to %q during A crash recovery", ipB, ipBAgain)
	}
}
