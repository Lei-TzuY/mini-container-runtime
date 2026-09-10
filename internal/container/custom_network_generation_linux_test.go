//go:build linux

package container

import (
	"fmt"
	"net"
	"os/exec"
	"testing"

	"minicontainer/internal/dns"
	"minicontainer/internal/network"
)

func TestSetupCustomBridgeGenerationComposesLiveProcessProfileLeaseAndAttachment(t *testing.T) {
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
		ContainerID: "custom-generation",
		StateDir:    t.TempDir(),
		Hostname:    "api",
		PortMappings: []PortMapping{{
			HostPort: 18080, ContainerPort: 8080, Protocol: "tcp",
		}},
	}
	const networkName = "app"
	const owner = "minicontainer:0123456789abcdef0123456789abcdef"

	rollbackAdmission, err := dns.BeginHostRegistrationAttempt(networkName, cfg.ContainerID, cfg.Hostname, "172.28.41.2")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rollbackAdmission() }()

	inspectCalls := 0
	inspect := func(name string) (network.BridgeIPv4Profile, error) {
		inspectCalls++
		if name != networkName {
			return network.BridgeIPv4Profile{}, fmt.Errorf("network=%q, want %q", name, networkName)
		}
		return network.BridgeIPv4Profile{
			BridgeName: "br-app",
			HostCIDR:   "172.28.41.1/24",
			SubnetCIDR: "172.28.41.0/24",
			Gateway:    "172.28.41.1",
		}, nil
	}

	setupCalls := 0
	setup := func(pid int, hostCIDR, containerIP string, mappings []PortMapping, gotOwner, gotNetwork string, debug bool) (func() error, error) {
		setupCalls++
		if pid != cmd.Process.Pid {
			t.Fatalf("setup pid=%d, want live child pid=%d", pid, cmd.Process.Pid)
		}
		if hostCIDR != "172.28.41.1/24" || gotOwner != owner || gotNetwork != networkName {
			t.Fatalf("setup profile hostCIDR=%q owner=%q network=%q", hostCIDR, gotOwner, gotNetwork)
		}
		if len(mappings) != 1 || mappings[0].HostPort != 18080 || mappings[0].ContainerPort != 8080 {
			t.Fatalf("setup mappings=%#v", mappings)
		}
		ip := net.ParseIP(containerIP)
		_, subnet, parseErr := net.ParseCIDR("172.28.41.0/24")
		if parseErr != nil || ip == nil || !subnet.Contains(ip) || containerIP == "172.28.41.1" {
			t.Fatalf("container IP=%q is not a non-gateway address in custom subnet", containerIP)
		}
		return func() error { return nil }, nil
	}

	containerIP, bridgeCfg, cleanup, err := setupCustomBridgeGenerationWith(
		cfg,
		cmd.Process.Pid,
		start,
		owner,
		networkName,
		false,
		inspect,
		setup,
	)
	if err != nil {
		t.Fatal(err)
	}
	if inspectCalls != 1 || setupCalls != 1 {
		t.Fatalf("composition calls inspect=%d setup=%d", inspectCalls, setupCalls)
	}
	if bridgeCfg.Gateway != "172.28.41.1" {
		t.Fatalf("bridge gateway=%q", bridgeCfg.Gateway)
	}
	ip, _, err := net.ParseCIDR(bridgeCfg.ContainerCIDR)
	if err != nil {
		t.Fatalf("container CIDR=%q: %v", bridgeCfg.ContainerCIDR, err)
	}
	if got := ip.String(); got != containerIP {
		t.Fatalf("bridge config IP=%q, lease IP=%q", got, containerIP)
	}
	if cleanup == nil {
		t.Fatal("custom network cleanup is nil")
	}
	if err := cleanup(); err != nil {
		t.Fatal(err)
	}
}
