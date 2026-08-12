//go:build linux

// internal/network/bridge_linux.go
//
// Custom Container Networks (`minictl network create/ls/rm`)
// ─────────────────────────────────────────────────────────
// Custom bridge networks allow multiple containers to communicate on an
// isolated virtual Layer 2 network switch (Linux Bridge interface).
//
// Commands:
//   ip link add <name> type bridge
//   ip addr add <subnet-gw>/24 dev <name>
//   ip link set <name> up

package network

import (
	"fmt"
	"os/exec"
	"strings"
)

// NetworkInfo describes a custom container network.
type NetworkInfo struct {
	Name   string
	Bridge string
	Subnet string
	Status string
}

// CreateBridge creates a custom Linux bridge interface with the given name and CIDR gateway.
func CreateBridge(netName, cidr string, debug bool) error {
	bridgeName := "br-" + netName
	if len(bridgeName) > 15 {
		bridgeName = bridgeName[:15] // Linux IFNAMSIZ = 16 (15 chars + NUL)
	}

	if cidr == "" {
		cidr = "172.28.0.1/24"
	}

	// 1. Create bridge interface
	if out, err := exec.Command("ip", "link", "add", bridgeName, "type", "bridge").CombinedOutput(); err != nil {
		return fmt.Errorf("create bridge %s: %w\n%s", bridgeName, err, out)
	}

	// 2. Assign IP address
	if out, err := exec.Command("ip", "addr", "add", cidr, "dev", bridgeName).CombinedOutput(); err != nil {
		_ = exec.Command("ip", "link", "delete", bridgeName).Run()
		return fmt.Errorf("assign IP %s to %s: %w\n%s", cidr, bridgeName, err, out)
	}

	// 3. Bring bridge UP
	if out, err := exec.Command("ip", "link", "set", bridgeName, "up").CombinedOutput(); err != nil {
		_ = exec.Command("ip", "link", "delete", bridgeName).Run()
		return fmt.Errorf("set %s up: %w\n%s", bridgeName, err, out)
	}

	if debug {
		fmt.Printf("[net] created custom bridge network %q (%s, %s)\n", netName, bridgeName, cidr)
	}
	return nil
}

// ListBridges lists all minictl custom bridge networks.
func ListBridges() ([]NetworkInfo, error) {
	out, err := exec.Command("ip", "link", "show", "type", "bridge").Output()
	if err != nil {
		return nil, nil // No bridges
	}

	var networks []NetworkInfo
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 {
			iface := strings.TrimSuffix(fields[1], ":")
			if strings.HasPrefix(iface, "br-") {
				netName := strings.TrimPrefix(iface, "br-")
				networks = append(networks, NetworkInfo{
					Name:   netName,
					Bridge: iface,
					Status: "UP",
				})
			}
		}
	}
	return networks, nil
}

// DeleteBridge deletes a custom bridge network.
func DeleteBridge(netName string, debug bool) error {
	bridgeName := "br-" + netName
	if len(bridgeName) > 15 {
		bridgeName = bridgeName[:15]
	}

	if out, err := exec.Command("ip", "link", "delete", bridgeName).CombinedOutput(); err != nil {
		return fmt.Errorf("delete bridge %s: %w\n%s", bridgeName, err, out)
	}

	if debug {
		fmt.Printf("[net] deleted custom bridge network %q (%s)\n", netName, bridgeName)
	}
	return nil
}
