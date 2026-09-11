//go:build linux

package container

import (
	"fmt"
	"os/exec"
	"testing"

	"minicontainer/internal/dns"
	"minicontainer/internal/network"
	"minicontainer/internal/state"
)

func TestCustomNetworkAdmissionFlowsIntoLiveGeneration(t *testing.T) {
	lifecycleStore, err := state.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer lifecycleStore.Close()

	cfg := Config{
		ContainerID:   "custom-admission-generation",
		StateDir:      t.TempDir(),
		Hostname:      "api",
		RootFS:        "/rootfs",
		BridgeNetwork: true,
		NetworkName:   "admission-app",
		PortMappings: []PortMapping{{
			HostPort: 18081, ContainerPort: 8081, Protocol: "tcp",
		}},
	}
	profile := network.BridgeIPv4Profile{
		BridgeName: "br-admission-app",
		HostCIDR:   "172.28.42.1/24",
		SubnetCIDR: "172.28.42.0/24",
		Gateway:    "172.28.42.1",
	}

	inspectCalls := 0
	inspect := func(name string) (network.BridgeIPv4Profile, error) {
		inspectCalls++
		if name != cfg.NetworkName {
			return network.BridgeIPv4Profile{}, fmt.Errorf("network=%q, want %q", name, cfg.NetworkName)
		}
		return profile, nil
	}

	rollbackAdmission, err := beginNetworkAttemptAdmissionWith(cfg, lifecycleStore, networkAdmissionDeps{
		validateDNSRootFS: func(rootfsPath, networkName string) error {
			if rootfsPath != cfg.RootFS || networkName != cfg.NetworkName {
				return fmt.Errorf("DNS validation=%q/%q, want %q/%q", rootfsPath, networkName, cfg.RootFS, cfg.NetworkName)
			}
			return nil
		},
		beginDNSAttempt: func(networkName, containerID, hostname, ipAddr string) (func() error, error) {
			if networkName != cfg.NetworkName || containerID != cfg.ContainerID || hostname != cfg.Hostname || ipAddr != profile.Gateway {
				return nil, fmt.Errorf("DNS admission=%q/%q/%q/%q", networkName, containerID, hostname, ipAddr)
			}
			return dns.BeginHostRegistrationAttempt(networkName, containerID, hostname, ipAddr)
		},
		inspectBridgeProfile: inspect,
	})
	if err != nil {
		t.Fatalf("custom network admission: %v", err)
	}
	if rollbackAdmission == nil {
		t.Fatal("custom network admission returned nil rollback")
	}
	defer func() {
		if err := rollbackAdmission(); err != nil {
			t.Errorf("rollback custom network admission: %v", err)
		}
	}()

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

	const owner = "minicontainer:1123456789abcdef0123456789abcdef"
	setupCalls := 0
	containerIP, bridgeCfg, cleanup, err := setupCustomBridgeGenerationWith(
		cfg,
		cmd.Process.Pid,
		start,
		owner,
		cfg.NetworkName,
		false,
		inspect,
		func(pid int, hostCIDR, containerIP string, mappings []PortMapping, gotOwner, gotNetwork string, debug bool) (func() error, error) {
			setupCalls++
			if pid != cmd.Process.Pid || hostCIDR != profile.HostCIDR || gotOwner != owner || gotNetwork != cfg.NetworkName {
				return nil, fmt.Errorf("custom setup pid=%d hostCIDR=%q owner=%q network=%q", pid, hostCIDR, gotOwner, gotNetwork)
			}
			return func() error { return nil }, nil
		},
	)
	if err != nil {
		t.Fatalf("custom generation setup: %v", err)
	}
	if inspectCalls != 2 || setupCalls != 1 {
		t.Fatalf("custom launch calls inspect=%d setup=%d, want 2/1", inspectCalls, setupCalls)
	}
	if containerIP == "" || bridgeCfg.Gateway != profile.Gateway {
		t.Fatalf("custom generation address=%q bridge=%#v", containerIP, bridgeCfg)
	}
	if cleanup == nil {
		t.Fatal("custom generation cleanup is nil")
	}
	if err := cleanup(); err != nil {
		t.Fatal(err)
	}
}

func TestCustomNetworkAdmissionRejectsMissingBridgeMode(t *testing.T) {
	cfg := Config{ContainerID: "custom-no-bridge", NetworkName: "app"}
	_, err := beginNetworkAttemptAdmissionWith(cfg, nil, noopNetworkAdmissionDeps())
	if err == nil {
		t.Fatal("custom network without bridge mode unexpectedly admitted")
	}
}
