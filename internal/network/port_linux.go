//go:build linux

// internal/network/port_linux.go
//
// Port Forwarding / Port Mapping (`-p hostPort:containerPort`)
// ─────────────────────────────────────────────────────────────
// Port mapping forwards traffic arriving at a port on the host to a target
// port inside the container's network namespace (at container IP 172.20.0.2).
//
// Mechanism: iptables DNAT (Destination NAT)
// ───────────────────────────────────────────
//   1. Append a DNAT rule to the PREROUTING chain in the nat table:
//      iptables -t nat -A PREROUTING -p tcp --dport <hostPort> \
//               -j DNAT --to-destination 172.20.0.2:<containerPort>
//
//   2. Append an OUTPUT rule for host-local traffic (so `curl localhost:hostPort`
//      on the host reaches the container as well):
//      iptables -t nat -A OUTPUT -p tcp --dport <hostPort> \
//               -j DNAT --to-destination 172.20.0.2:<containerPort>
//
// When the container stops, we clean up the rules using `-D` (Delete).

package network

import (
	"fmt"
	"os/exec"
	"strconv"
)

// SetupPortForwarding configures iptables DNAT rules for port mapping.
func SetupPortForwarding(hostPort, containerPort int, containerIP, protocol string, debug bool) error {
	if protocol == "" {
		protocol = "tcp"
	}
	target := fmt.Sprintf("%s:%d", containerIP, containerPort)
	portStr := strconv.Itoa(hostPort)

	// PREROUTING chain (external packets)
	rule1 := []string{"-t", "nat", "-A", "PREROUTING", "-p", protocol,
		"--dport", portStr, "-j", "DNAT", "--to-destination", target}

	// OUTPUT chain (locally generated host packets)
	rule2 := []string{"-t", "nat", "-A", "OUTPUT", "-p", protocol,
		"-m", "addrtype", "--dst-type", "LOCAL",
		"--dport", portStr, "-j", "DNAT", "--to-destination", target}

	if out, err := exec.Command("iptables", rule1...).CombinedOutput(); err != nil {
		return fmt.Errorf("iptables PREROUTING DNAT: %w\n%s", err, out)
	}
	if out, err := exec.Command("iptables", rule2...).CombinedOutput(); err != nil {
		// Non-fatal if LOCAL addrtype match fails on older iptables
		if debug {
			fmt.Printf("[parent] iptables OUTPUT DNAT warning: %v\n%s\n", err, out)
		}
	}

	if debug {
		fmt.Printf("[parent] port mapping: host %s/%d → container %s\n", protocol, hostPort, target)
	}
	return nil
}

// RemovePortForwarding deletes the iptables DNAT rules when the container stops.
func RemovePortForwarding(hostPort, containerPort int, containerIP, protocol string, debug bool) {
	if protocol == "" {
		protocol = "tcp"
	}
	target := fmt.Sprintf("%s:%d", containerIP, containerPort)
	portStr := strconv.Itoa(hostPort)

	rule1 := []string{"-t", "nat", "-D", "PREROUTING", "-p", protocol,
		"--dport", portStr, "-j", "DNAT", "--to-destination", target}

	rule2 := []string{"-t", "nat", "-D", "OUTPUT", "-p", protocol,
		"-m", "addrtype", "--dst-type", "LOCAL",
		"--dport", portStr, "-j", "DNAT", "--to-destination", target}

	_ = exec.Command("iptables", rule1...).Run()
	_ = exec.Command("iptables", rule2...).Run()

	if debug {
		fmt.Printf("[parent] cleaned up port mapping: host %s/%d\n", protocol, hostPort)
	}
}
