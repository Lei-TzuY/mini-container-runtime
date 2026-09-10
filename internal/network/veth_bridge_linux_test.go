//go:build linux

package network

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

func TestAttachVethHostToBridgeOwnedRejectsForeignBridge(t *testing.T) {
	var calls [][]string
	run := func(args ...string) ([]byte, error) {
		calls = append(calls, append([]string(nil), args...))
		return []byte(`[{"ifname":"br-app","ifalias":"foreign"}]`), nil
	}

	err := attachVethHostToBridgeOwnedWith("vh-test", "app", false, run)
	if err == nil || !strings.Contains(err.Error(), "without ownership tag") {
		t.Fatalf("expected ownership rejection, got %v", err)
	}
	if len(calls) != 1 {
		t.Fatalf("foreign bridge must not be mutated; calls=%v", calls)
	}
}

func TestAttachVethHostToBridgeOwnedCommand(t *testing.T) {
	var calls [][]string
	run := func(args ...string) ([]byte, error) {
		calls = append(calls, append([]string(nil), args...))
		if len(calls) == 1 {
			return []byte(`[{"ifname":"br-app","ifalias":"minicontainer-network:app"}]`), nil
		}
		return nil, nil
	}

	if err := attachVethHostToBridgeOwnedWith("vh-test", "app", false, run); err != nil {
		t.Fatal(err)
	}
	if len(calls) != 2 {
		t.Fatalf("calls=%v", calls)
	}
	got := strings.Join(calls[1], " ")
	if got != "link set dev vh-test master br-app" {
		t.Fatalf("attach command=%q", got)
	}
}

func TestAttachVethHostToOwnedBridgeKernel(t *testing.T) {
	if os.Getenv("MINICTL_VETH_BRIDGE_KERNEL_CHILD") == "1" {
		if err := CreateBridge("itest", "172.29.0.1/24", false); err != nil {
			fmt.Fprintln(os.Stderr, "create bridge:", err)
			os.Exit(2)
		}
		if err := createVethPair("vh-itest", "vp-itest"); err != nil {
			fmt.Fprintln(os.Stderr, "create veth:", err)
			os.Exit(3)
		}
		if err := AttachVethHostToBridgeOwned("vh-itest", "itest", false); err != nil {
			fmt.Fprintln(os.Stderr, "attach veth:", err)
			os.Exit(4)
		}
		master, err := os.Readlink("/sys/class/net/vh-itest/master")
		if err != nil {
			fmt.Fprintln(os.Stderr, "read veth master:", err)
			os.Exit(5)
		}
		if filepath.Base(master) != "br-itest" {
			fmt.Fprintf(os.Stderr, "veth master=%q\n", master)
			os.Exit(6)
		}
		return
	}

	cmd := exec.Command(os.Args[0], "-test.run=^TestAttachVethHostToOwnedBridgeKernel$")
	cmd.Env = append(os.Environ(), "MINICTL_VETH_BRIDGE_KERNEL_CHILD=1")
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
	if exitErr, ok := err.(*exec.ExitError); ok {
		if status, ok := exitErr.Sys().(syscall.WaitStatus); ok && status.ExitStatus() == 2 && strings.Contains(string(out), "operation not permitted") {
			t.Skipf("kernel/user namespace does not permit bridge integration test: %s", strings.TrimSpace(string(out)))
		}
	}
	if strings.Contains(err.Error(), "operation not permitted") {
		t.Skipf("kernel/user namespace creation unavailable: %v", err)
	}
	t.Fatalf("kernel bridge attachment regression failed: %v\n%s", err, out)
}
