//go:build linux

package container

import (
	"os/exec"
	"testing"
)

func TestRealProcessBridgeCrashRecoveryRetargetsDNATWithoutDisturbingSurvivor(t *testing.T) {
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

	cfgA := Config{ContainerID: "process-dnat-a", StateDir: stateDir, BridgeNetwork: true}
	cfgB := Config{ContainerID: "process-dnat-b", StateDir: stateDir, BridgeNetwork: true}
	ipA, _, _, err := allocateRuntimeBridgeLease(cfgA, cmdA.Process.Pid, startA)
	if err != nil {
		t.Fatal(err)
	}
	ipB, _, releaseB, err := allocateRuntimeBridgeLease(cfgB, cmdB.Process.Pid, startB)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = releaseB() }()

	type portEvent struct {
		owner string
		ip    string
		add   bool
	}
	var events []portEvent
	setup := func(owner string, pid int, ip string, hostPort int) func() error {
		t.Helper()
		cleanup, err := setupBridgeHostWithOps(pid, defaultBridgeHostCIDR, ip, []PortMapping{{HostPort: hostPort, ContainerPort: 80, Protocol: "tcp"}}, false, bridgeHostOps{
			setupVeth:  func(int, string, bool) error { return nil },
			removeVeth: func(int, bool) error { return nil },
			setupPort: func(_, _ int, containerIP, _ string, _ bool) error {
				events = append(events, portEvent{owner: owner, ip: containerIP, add: true})
				return nil
			},
			removePort: func(_, _ int, containerIP, _ string, _ bool) error {
				events = append(events, portEvent{owner: owner, ip: containerIP, add: false})
				return nil
			},
		})
		if err != nil {
			t.Fatalf("setup %s bridge: %v", owner, err)
		}
		return cleanup
	}

	cleanupA := setup("a", cmdA.Process.Pid, ipA, 19081)
	cleanupB := setup("b", cmdB.Process.Pid, ipB, 19082)
	defer func() { _ = cleanupB() }()

	if err := cmdA.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	stopA()
	if err := cleanupA(); err != nil {
		t.Fatalf("cleanup crashed A bridge: %v", err)
	}

	cmdC, stopC := startBlockedProcess()
	defer stopC()
	startC, err := ProcessStartTime(cmdC.Process.Pid)
	if err != nil {
		t.Fatal(err)
	}
	cfgC := Config{ContainerID: "process-dnat-a-restart", StateDir: stateDir, BridgeNetwork: true}
	ipC, _, releaseC, err := allocateRuntimeBridgeLease(cfgC, cmdC.Process.Pid, startC)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = releaseC() }()
	if ipC != ipA {
		t.Fatalf("replacement process got %q, want crashed address %q", ipC, ipA)
	}
	cleanupC := setup("c", cmdC.Process.Pid, ipC, 19081)
	defer func() { _ = cleanupC() }()

	ipBAgain, _, releaseBAgain, err := allocateRuntimeBridgeLease(cfgB, cmdB.Process.Pid, startB)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = releaseBAgain() }()
	if ipBAgain != ipB {
		t.Fatalf("surviving process moved from %q to %q", ipB, ipBAgain)
	}

	want := []portEvent{
		{owner: "a", ip: ipA, add: true},
		{owner: "b", ip: ipB, add: true},
		{owner: "a", ip: ipA, add: false},
		{owner: "c", ip: ipA, add: true},
	}
	if len(events) != len(want) {
		t.Fatalf("DNAT events = %#v, want %#v", events, want)
	}
	for i := range want {
		if events[i] != want[i] {
			t.Fatalf("DNAT event %d = %#v, want %#v", i, events[i], want[i])
		}
	}
}
