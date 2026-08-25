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
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

const maxLinuxInterfaceNameLen = 15

type bridgeCommandRunner func(args ...string) ([]byte, error)

// NetworkInfo describes a custom container network.
type NetworkInfo struct {
	Name   string
	Bridge string
	Subnet string
	Status string
}

func bridgeNameForNetwork(netName string) (string, error) {
	bridgeName := "br-" + netName
	if len(bridgeName) > maxLinuxInterfaceNameLen {
		return "", fmt.Errorf(
			"network name %q is too long: bridge interface %q exceeds Linux %d-byte limit",
			netName,
			bridgeName,
			maxLinuxInterfaceNameLen,
		)
	}
	return bridgeName, nil
}

// CreateBridge creates a custom Linux bridge interface with the given name and CIDR gateway.
func CreateBridge(netName, cidr string, debug bool) error {
	return createBridgeWith(netName, cidr, debug, runBridgeIPCommand)
}

func runBridgeIPCommand(args ...string) ([]byte, error) {
	return exec.Command("ip", args...).CombinedOutput()
}

func createBridgeWith(netName, cidr string, debug bool, run bridgeCommandRunner) error {
	if run == nil {
		return fmt.Errorf("bridge command runner is nil")
	}

	bridgeName, err := bridgeNameForNetwork(netName)
	if err != nil {
		return err
	}

	if cidr == "" {
		cidr = "172.28.0.1/24"
	}

	// 1. Create bridge interface
	if out, err := run("link", "add", bridgeName, "type", "bridge"); err != nil {
		return fmt.Errorf("create bridge %s: %w\n%s", bridgeName, err, out)
	}

	// 2. Assign IP address
	if out, err := run("addr", "add", cidr, "dev", bridgeName); err != nil {
		setupErr := fmt.Errorf("assign IP %s to %s: %w\n%s", cidr, bridgeName, err, out)
		return rollbackCreatedBridge(run, bridgeName, setupErr)
	}

	// 3. Bring bridge UP
	if out, err := run("link", "set", bridgeName, "up"); err != nil {
		setupErr := fmt.Errorf("set %s up: %w\n%s", bridgeName, err, out)
		return rollbackCreatedBridge(run, bridgeName, setupErr)
	}

	if debug {
		fmt.Printf("[net] created custom bridge network %q (%s, %s)\n", netName, bridgeName, cidr)
	}
	return nil
}

func rollbackCreatedBridge(run bridgeCommandRunner, bridgeName string, setupErr error) error {
	out, err := run("link", "delete", bridgeName)
	if err == nil {
		return setupErr
	}
	return errors.Join(
		setupErr,
		fmt.Errorf("rollback bridge %s after setup failure: %w\n%s", bridgeName, err, out),
	)
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
	return deleteBridgeWith(netName, debug, runBridgeIPCommand)
}

func deleteBridgeWith(netName string, debug bool, run bridgeCommandRunner) error {
	if run == nil {
		return fmt.Errorf("bridge command runner is nil")
	}
	bridgeName, err := bridgeNameForNetwork(netName)
	if err != nil {
		return err
	}

	if out, err := run("link", "delete", bridgeName); err != nil {
		return fmt.Errorf("delete bridge %s: %w\n%s", bridgeName, err, out)
	}

	if debug {
		fmt.Printf("[net] deleted custom bridge network %q (%s)\n", netName, bridgeName)
	}
	return nil
}
