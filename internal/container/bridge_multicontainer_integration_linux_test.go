//go:build linux

package container

import (
	"os/exec"
	"testing"
)

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

func TestBridgeLeaseCrashRecoveryUsesRealProcessGenerations(t *testing.T) {
	stateDir := t.TempDir()
	startBlockedProcess := func() (*exec.Cmd, func()) {
		t.Helper()
		cmd := exec.Command("sh", "-c", "read _")
		stdin, err := cmd.StdinPipe()
		if err != nil {
			t.Fatal(err)
		}
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		return cmd, func() {
			_ = stdin.Close()
			_ = cmd.Wait()
		}
	}

	cmdA, stopA := startBlockedProcess()
	cmdB, stopB := startBlockedProcess()
	defer stopB()

	startA, err := ProcessStartTime(cmdA.Process.Pid)
	if err != nil {
		t.Fatal(err)
	}
	startB, err := ProcessStartTime(cmdB.Process.Pid)
	if err != nil {
		t.Fatal(err)
	}

	cfgA := Config{ContainerID: "real-a", StateDir: stateDir}
	cfgB := Config{ContainerID: "real-b", StateDir: stateDir}
	ipA, _, _, err := allocateRuntimeBridgeLease(cfgA, cmdA.Process.Pid, startA)
	if err != nil {
		t.Fatal(err)
	}
	ipB, _, releaseB, err := allocateRuntimeBridgeLease(cfgB, cmdB.Process.Pid, startB)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = releaseB() }()
	if ipA == ipB {
		t.Fatalf("live process generations received duplicate bridge IP %q", ipA)
	}

	if err := cmdA.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	stopA()

	cmdC, stopC := startBlockedProcess()
	defer stopC()
	startC, err := ProcessStartTime(cmdC.Process.Pid)
	if err != nil {
		t.Fatal(err)
	}
	cfgC := Config{ContainerID: "real-a-restart", StateDir: stateDir}
	ipC, _, releaseC, err := allocateRuntimeBridgeLease(cfgC, cmdC.Process.Pid, startC)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = releaseC() }()
	if ipC != ipA {
		t.Fatalf("replacement process got %q, want crashed generation address %q", ipC, ipA)
	}

	ipBAgain, _, releaseBAgain, err := allocateRuntimeBridgeLease(cfgB, cmdB.Process.Pid, startB)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = releaseBAgain() }()
	if ipBAgain != ipB {
		t.Fatalf("surviving real process moved from %q to %q during peer crash recovery", ipB, ipBAgain)
	}
}
