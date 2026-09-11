//go:build linux

package container

import (
	"fmt"
	"os/exec"
	"testing"

	"minicontainer/internal/dns"
	"minicontainer/internal/network"
)

func TestSetupCustomBridgeGenerationDurableOwnsBeforeHostSetupAndCleansAfter(t *testing.T) {
	cmd := exec.Command("sh", "-c", "read line")
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = stdin.Close()
		_ = cmd.Wait()
	}()

	start, err := ProcessStartTime(cmd.Process.Pid)
	if err != nil {
		t.Fatal(err)
	}

	cfg := Config{
		ContainerID: "custom-generation-durable",
		StateDir:    t.TempDir(),
		Hostname:    "api",
	}
	const networkName = "app"
	const owner = "minicontainer:fedcba9876543210fedcba9876543210"

	rollbackAdmission, err := dns.BeginHostRegistrationAttempt(networkName, cfg.ContainerID, cfg.Hostname, "172.29.51.2")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rollbackAdmission() }()

	sequence := make([]string, 0, 4)
	inspect := func(name string) (network.BridgeIPv4Profile, error) {
		if name != networkName {
			return network.BridgeIPv4Profile{}, fmt.Errorf("network=%q, want %q", name, networkName)
		}
		return network.BridgeIPv4Profile{
			BridgeName: "br-app",
			HostCIDR:   "172.29.51.1/24",
			SubnetCIDR: "172.29.51.0/24",
			Gateway:    "172.29.51.1",
		}, nil
	}
	ownLease := func(containerIP string) (func() error, error) {
		if containerIP == "" || containerIP == "172.29.51.1" {
			return nil, fmt.Errorf("invalid leased IP %q", containerIP)
		}
		sequence = append(sequence, "persist")
		return func() error {
			sequence = append(sequence, "ownership-cleanup")
			return nil
		}, nil
	}
	setup := func(pid int, hostCIDR, containerIP string, mappings []PortMapping, gotOwner, gotNetwork string, debug bool) (func() error, error) {
		if pid != cmd.Process.Pid || hostCIDR != "172.29.51.1/24" || gotOwner != owner || gotNetwork != networkName {
			return nil, fmt.Errorf("unexpected setup pid=%d host=%q owner=%q network=%q", pid, hostCIDR, gotOwner, gotNetwork)
		}
		if len(sequence) != 1 || sequence[0] != "persist" {
			return nil, fmt.Errorf("host setup ran before durable ownership: %v", sequence)
		}
		sequence = append(sequence, "host-setup")
		return func() error {
			sequence = append(sequence, "host-cleanup")
			return nil
		}, nil
	}

	_, bridgeCfg, cleanup, err := setupCustomBridgeGenerationWithLeaseOwnership(
		cfg,
		cmd.Process.Pid,
		start,
		owner,
		networkName,
		false,
		ownLease,
		inspect,
		setup,
	)
	if err != nil {
		t.Fatal(err)
	}
	if bridgeCfg.Gateway != "172.29.51.1" {
		t.Fatalf("gateway=%q", bridgeCfg.Gateway)
	}
	if cleanup == nil {
		t.Fatal("cleanup is nil")
	}
	if err := cleanup(); err != nil {
		t.Fatal(err)
	}

	want := []string{"persist", "host-setup", "host-cleanup", "ownership-cleanup"}
	if fmt.Sprint(sequence) != fmt.Sprint(want) {
		t.Fatalf("sequence=%v, want %v", sequence, want)
	}
}
