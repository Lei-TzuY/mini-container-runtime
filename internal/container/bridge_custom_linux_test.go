//go:build linux

package container

import (
	"errors"
	"os/exec"
	"reflect"
	"strings"
	"testing"

	"minicontainer/internal/network"
)

func TestSetupBridgeHostOwnedOnNetworkUsesLiveProcessGeneration(t *testing.T) {
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

	owner := "minicontainer:0123456789abcdef0123456789abcdef"
	hostVeth := network.VethHostIfaceOwned(owner)
	var events []string
	ops := bridgeHostOps{
		setupVeth: func(pid int, cidr string, debug bool) error {
			if pid != cmd.Process.Pid {
				t.Fatalf("setup pid=%d, want live child pid=%d", pid, cmd.Process.Pid)
			}
			if cidr != "172.28.0.1/24" {
				t.Fatalf("host cidr=%q", cidr)
			}
			events = append(events, "veth")
			return nil
		},
		removeVeth: func(pid int, debug bool) error {
			if pid != cmd.Process.Pid {
				t.Fatalf("cleanup pid=%d, want live child pid=%d", pid, cmd.Process.Pid)
			}
			events = append(events, "remove-veth")
			return nil
		},
		setupPort: func(hostPort, containerPort int, containerIP, protocol string, debug bool) error {
			events = append(events, "port")
			return nil
		},
		removePort: func(hostPort, containerPort int, containerIP, protocol string, debug bool) error {
			events = append(events, "remove-port")
			return nil
		},
	}
	attach := func(hostName, networkName string, debug bool) error {
		if hostName != hostVeth {
			t.Fatalf("host veth=%q, want %q", hostName, hostVeth)
		}
		if networkName != "app" {
			t.Fatalf("network=%q", networkName)
		}
		events = append(events, "attach")
		return nil
	}

	cleanup, err := setupBridgeHostOwnedOnNetworkWith(
		cmd.Process.Pid,
		"172.28.0.1/24",
		"172.28.0.2",
		[]PortMapping{{HostPort: 8080, ContainerPort: 80, Protocol: "tcp"}},
		owner,
		"app",
		false,
		ops,
		attach,
	)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := events, []string{"veth", "attach", "port"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("setup events=%v, want %v", got, want)
	}
	if err := cleanup(); err != nil {
		t.Fatal(err)
	}
	if got, want := events, []string{"veth", "attach", "port", "remove-port", "remove-veth"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("lifecycle events=%v, want %v", got, want)
	}
}

func TestSetupBridgeHostOwnedOnNetworkStopsBeforePortForwardingWhenAttachFails(t *testing.T) {
	owner := "minicontainer:fedcba9876543210fedcba9876543210"
	cause := errors.New("bridge rejected")
	portCalls := 0
	ops := bridgeHostOps{
		setupVeth:  func(int, string, bool) error { return nil },
		removeVeth: func(int, bool) error { return nil },
		setupPort: func(int, int, string, string, bool) error {
			portCalls++
			return nil
		},
		removePort: func(int, int, string, string, bool) error { return nil },
	}

	_, err := setupBridgeHostOwnedOnNetworkWith(
		42,
		"172.28.0.1/24",
		"172.28.0.2",
		[]PortMapping{{HostPort: 8080, ContainerPort: 80}},
		owner,
		"app",
		false,
		ops,
		func(string, string, bool) error { return cause },
	)
	if !errors.Is(err, cause) || !strings.Contains(err.Error(), "attach host veth") {
		t.Fatalf("attach failure=%v", err)
	}
	if portCalls != 0 {
		t.Fatalf("port forwarding ran %d times after attach failure", portCalls)
	}
}
