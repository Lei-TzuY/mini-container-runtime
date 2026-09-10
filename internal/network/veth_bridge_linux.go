//go:build linux

package network

import "fmt"

// AttachVethHostToBridgeOwned attaches a host-side veth endpoint to an existing
// minictl-owned Linux bridge. The bridge ownership alias is verified before the
// link is mutated so a same-named host bridge created by another tool is never
// used implicitly.
func AttachVethHostToBridgeOwned(hostName, networkName string, debug bool) error {
	return attachVethHostToBridgeOwnedWith(hostName, networkName, debug, runBridgeIPCommand)
}

func attachVethHostToBridgeOwnedWith(hostName, networkName string, debug bool, run bridgeCommandRunner) error {
	if run == nil {
		return fmt.Errorf("bridge command runner is nil")
	}
	if hostName == "" {
		return fmt.Errorf("host veth name is required")
	}
	if len(hostName) > maxLinuxInterfaceNameLen {
		return fmt.Errorf("host veth interface %q exceeds Linux %d-byte limit", hostName, maxLinuxInterfaceNameLen)
	}

	bridgeName, err := bridgeNameForNetwork(networkName)
	if err != nil {
		return err
	}

	out, err := run("-j", "link", "show", "dev", bridgeName)
	if err != nil {
		return fmt.Errorf("inspect bridge %s ownership: %w\n%s", bridgeName, err, out)
	}
	links, err := decodeBridgeLinks(out)
	if err != nil {
		return fmt.Errorf("inspect bridge %s ownership: %w", bridgeName, err)
	}
	if len(links) != 1 || links[0].IfName != bridgeName {
		return fmt.Errorf("inspect bridge %s ownership: expected exactly one matching interface, got %d", bridgeName, len(links))
	}
	expectedAlias := bridgeOwnershipAlias(networkName)
	if links[0].IfAlias != expectedAlias {
		return fmt.Errorf("refusing to attach %s to bridge %s without ownership tag %q (found %q)", hostName, bridgeName, expectedAlias, links[0].IfAlias)
	}

	if out, err := run("link", "set", "dev", hostName, "master", bridgeName); err != nil {
		return fmt.Errorf("attach veth %s to bridge %s: %w\n%s", hostName, bridgeName, err, out)
	}
	if debug {
		fmt.Printf("[net] attached host veth %s to bridge %s\n", hostName, bridgeName)
	}
	return nil
}
