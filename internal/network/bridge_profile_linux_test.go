//go:build linux

package network

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"testing"
)

func TestInspectBridgeIPv4OwnedRejectsForeignBridgeBeforeAddressRead(t *testing.T) {
	var calls [][]string
	run := func(args ...string) ([]byte, error) {
		calls = append(calls, append([]string(nil), args...))
		return []byte(`[{"ifname":"br-app","ifalias":"foreign"}]`), nil
	}

	_, err := inspectBridgeIPv4OwnedWith("app", run)
	if err == nil || !strings.Contains(err.Error(), "without ownership tag") {
		t.Fatalf("expected ownership rejection, got %v", err)
	}
	if len(calls) != 1 {
		t.Fatalf("foreign bridge address must not be read; calls=%v", calls)
	}
}

func TestInspectBridgeIPv4OwnedResolvesCanonicalProfile(t *testing.T) {
	var calls [][]string
	run := func(args ...string) ([]byte, error) {
		calls = append(calls, append([]string(nil), args...))
		switch len(calls) {
		case 1:
			return []byte(`[{"ifname":"br-app","ifalias":"minicontainer-network:app"}]`), nil
		case 2:
			return []byte(`[{"ifname":"br-app","addr_info":[{"family":"inet6","local":"fe80::1","prefixlen":64,"scope":"link"},{"family":"inet","local":"172.29.7.1","prefixlen":24,"scope":"global"}]}]`), nil
		default:
			t.Fatalf("unexpected call %v", args)
			return nil, nil
		}
	}

	profile, err := inspectBridgeIPv4OwnedWith("app", run)
	if err != nil {
		t.Fatal(err)
	}
	if profile.NetworkName != "app" || profile.BridgeName != "br-app" || profile.HostCIDR != "172.29.7.1/24" || profile.SubnetCIDR != "172.29.7.0/24" || profile.Gateway != "172.29.7.1" {
		t.Fatalf("profile=%+v", profile)
	}
	if got := strings.Join(calls[1], " "); got != "-j addr show dev br-app" {
		t.Fatalf("address command=%q", got)
	}
}

func TestInspectBridgeIPv4OwnedRejectsAmbiguousGateway(t *testing.T) {
	run := func(args ...string) ([]byte, error) {
		if len(args) > 1 && args[1] == "link" {
			return []byte(`[{"ifname":"br-app","ifalias":"minicontainer-network:app"}]`), nil
		}
		return []byte(`[{"ifname":"br-app","addr_info":[{"family":"inet","local":"172.29.0.1","prefixlen":24,"scope":"global"},{"family":"inet","local":"172.30.0.1","prefixlen":24,"scope":"global"}]}]`), nil
	}

	_, err := inspectBridgeIPv4OwnedWith("app", run)
	if err == nil || !strings.Contains(err.Error(), "multiple global IPv4") {
		t.Fatalf("expected ambiguous gateway rejection, got %v", err)
	}
}

func TestInspectBridgeIPv4OwnedKernel(t *testing.T) {
	if os.Getenv("MINICTL_BRIDGE_PROFILE_KERNEL_CHILD") == "1" {
		if err := CreateBridge("itest", "172.29.17.1/24", false); err != nil {
			fmt.Fprintln(os.Stderr, "create bridge:", err)
			os.Exit(2)
		}
		profile, err := InspectBridgeIPv4Owned("itest")
		if err != nil {
			fmt.Fprintln(os.Stderr, "inspect bridge:", err)
			os.Exit(3)
		}
		if profile.BridgeName != "br-itest" || profile.HostCIDR != "172.29.17.1/24" || profile.SubnetCIDR != "172.29.17.0/24" || profile.Gateway != "172.29.17.1" {
			fmt.Fprintf(os.Stderr, "unexpected profile: %+v\n", profile)
			os.Exit(4)
		}
		return
	}

	cmd := exec.Command(os.Args[0], "-test.run=^TestInspectBridgeIPv4OwnedKernel$")
	cmd.Env = append(os.Environ(), "MINICTL_BRIDGE_PROFILE_KERNEL_CHILD=1")
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Cloneflags: syscall.CLONE_NEWUSER | syscall.CLONE_NEWNET,
		UidMappings: []syscall.SysProcIDMap{{ContainerID: 0, HostID: os.Getuid(), Size: 1}},
		GidMappings: []syscall.SysProcIDMap{{ContainerID: 0, HostID: os.Getgid(), Size: 1}},
		GidMappingsEnableSetgroups: false,
	}
	out, err := cmd.CombinedOutput()
	if err == nil {
		return
	}
	lowerOut := strings.ToLower(string(out))
	lowerErr := strings.ToLower(err.Error())
	if exitErr, ok := err.(*exec.ExitError); ok {
		if status, ok := exitErr.Sys().(syscall.WaitStatus); ok && status.ExitStatus() == 2 && strings.Contains(lowerOut, "operation not permitted") {
			t.Skipf("kernel/user namespace does not permit bridge profile integration test: %s", strings.TrimSpace(string(out)))
		}
	}
	if strings.Contains(lowerErr, "operation not permitted") {
		t.Skipf("kernel/user namespace creation unavailable: %v", err)
	}
	t.Fatalf("kernel bridge profile regression failed: %v\n%s", err, out)
}
