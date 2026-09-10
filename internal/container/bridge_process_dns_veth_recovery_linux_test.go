//go:build linux

package container

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"minicontainer/internal/dns"
)

func TestRealProcessBridgeCrashRecoveryKeepsDNSVethAndDNATOwnershipAligned(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	stateDir := filepath.Join(home, ".minicontainer")

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

	rollbackA, err := dns.BeginHostRegistrationAttempt(defaultBridgeDNSNetwork, "process-stack-a", "alpha", defaultBridgeContainerIP)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rollbackA() }()
	rollbackB, err := dns.BeginHostRegistrationAttempt(defaultBridgeDNSNetwork, "process-stack-b", "beta", defaultBridgeContainerIP)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rollbackB() }()

	cfgA := Config{ContainerID: "process-stack-a", StateDir: stateDir, BridgeNetwork: true}
	cfgB := Config{ContainerID: "process-stack-b", StateDir: stateDir, BridgeNetwork: true}
	ipA, _, _, err := allocateRuntimeBridgeLease(cfgA, cmdA.Process.Pid, startA)
	if err != nil {
		t.Fatal(err)
	}
	ipB, _, releaseB, err := allocateRuntimeBridgeLease(cfgB, cmdB.Process.Pid, startB)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = releaseB() }()
	if err := dns.BindHostRegistrationGeneration(defaultBridgeDNSNetwork, cfgA.ContainerID, cmdA.Process.Pid, startA); err != nil {
		t.Fatal(err)
	}
	if err := dns.BindHostRegistrationGeneration(defaultBridgeDNSNetwork, cfgB.ContainerID, cmdB.Process.Pid, startB); err != nil {
		t.Fatal(err)
	}

	type resourceEvent struct {
		owner string
		kind  string
		ip    string
		add   bool
	}
	var events []resourceEvent
	setup := func(owner string, pid int, ip string, hostPort int) func() error {
		t.Helper()
		cleanup, err := setupBridgeHostWithOps(pid, defaultBridgeHostCIDR, ip, []PortMapping{{HostPort: hostPort, ContainerPort: 80, Protocol: "tcp"}}, false, bridgeHostOps{
			setupVeth: func(_ int, _ string, _ bool) error {
				events = append(events, resourceEvent{owner: owner, kind: "veth", add: true})
				return nil
			},
			removeVeth: func(_ int, _ bool) error {
				events = append(events, resourceEvent{owner: owner, kind: "veth", add: false})
				return nil
			},
			setupPort: func(_, _ int, containerIP, _ string, _ bool) error {
				events = append(events, resourceEvent{owner: owner, kind: "dnat", ip: containerIP, add: true})
				return nil
			},
			removePort: func(_, _ int, containerIP, _ string, _ bool) error {
				events = append(events, resourceEvent{owner: owner, kind: "dnat", ip: containerIP, add: false})
				return nil
			},
		})
		if err != nil {
			t.Fatalf("setup %s bridge: %v", owner, err)
		}
		return cleanup
	}

	cleanupA := setup("a", cmdA.Process.Pid, ipA, 19181)
	cleanupB := setup("b", cmdB.Process.Pid, ipB, 19182)
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
	rollbackC, err := dns.BeginHostRegistrationAttempt(defaultBridgeDNSNetwork, cfgA.ContainerID, "alpha", defaultBridgeContainerIP)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rollbackC() }()
	ipC, _, releaseC, err := allocateRuntimeBridgeLease(cfgA, cmdC.Process.Pid, startC)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = releaseC() }()
	if ipC != ipA {
		t.Fatalf("replacement process got %q, want crashed address %q", ipC, ipA)
	}
	if err := dns.BindHostRegistrationGeneration(defaultBridgeDNSNetwork, cfgA.ContainerID, cmdC.Process.Pid, startC); err != nil {
		t.Fatal(err)
	}
	cleanupC := setup("c", cmdC.Process.Pid, ipC, 19181)
	defer func() { _ = cleanupC() }()

	hosts, err := dns.GenerateHostsContentChecked(defaultBridgeDNSNetwork)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(hosts, ipC+"\talpha") || !strings.Contains(hosts, ipB+"\tbeta") {
		t.Fatalf("service discovery lost surviving/replacement ownership:\n%s", hosts)
	}
	if strings.Count(hosts, "\talpha") != 1 || strings.Count(hosts, "\tbeta") != 1 {
		t.Fatalf("service discovery contains duplicate owners:\n%s", hosts)
	}

	want := []resourceEvent{
		{owner: "a", kind: "veth", add: true},
		{owner: "a", kind: "dnat", ip: ipA, add: true},
		{owner: "b", kind: "veth", add: true},
		{owner: "b", kind: "dnat", ip: ipB, add: true},
		{owner: "a", kind: "dnat", ip: ipA, add: false},
		{owner: "a", kind: "veth", add: false},
		{owner: "c", kind: "veth", add: true},
		{owner: "c", kind: "dnat", ip: ipC, add: true},
	}
	if len(events) != len(want) {
		t.Fatalf("resource events = %#v, want %#v", events, want)
	}
	for i := range want {
		if events[i] != want[i] {
			t.Fatalf("resource event %d = %#v, want %#v", i, events[i], want[i])
		}
	}
}
