//go:build linux

package network

import (
	"encoding/json"
	"fmt"
	"net"
)

// BridgeIPv4Profile is the runtime-facing IPv4 configuration of an owned
// custom bridge. HostCIDR is the address configured on the bridge itself,
// SubnetCIDR is the canonical address pool, and Gateway is the bridge address
// used by attached containers as their default route.
type BridgeIPv4Profile struct {
	NetworkName string
	BridgeName  string
	HostCIDR    string
	SubnetCIDR  string
	Gateway     string
}

type bridgeAddrJSON struct {
	IfName   string `json:"ifname"`
	AddrInfo []struct {
		Family    string `json:"family"`
		Local     string `json:"local"`
		PrefixLen int    `json:"prefixlen"`
		Scope     string `json:"scope"`
	} `json:"addr_info"`
}

// InspectBridgeIPv4Owned resolves the live IPv4 profile for a custom bridge
// after verifying the kernel-held ownership alias. Runtime admission can use
// this instead of guessing a subnet or gateway from the network name.
func InspectBridgeIPv4Owned(networkName string) (BridgeIPv4Profile, error) {
	return inspectBridgeIPv4OwnedWith(networkName, runBridgeIPCommand)
}

func inspectBridgeIPv4OwnedWith(networkName string, run bridgeCommandRunner) (BridgeIPv4Profile, error) {
	if run == nil {
		return BridgeIPv4Profile{}, fmt.Errorf("bridge command runner is nil")
	}
	bridgeName, err := bridgeNameForNetwork(networkName)
	if err != nil {
		return BridgeIPv4Profile{}, err
	}

	out, err := run("-j", "link", "show", "dev", bridgeName)
	if err != nil {
		return BridgeIPv4Profile{}, fmt.Errorf("inspect bridge %s ownership: %w\n%s", bridgeName, err, out)
	}
	links, err := decodeBridgeLinks(out)
	if err != nil {
		return BridgeIPv4Profile{}, fmt.Errorf("inspect bridge %s ownership: %w", bridgeName, err)
	}
	if len(links) != 1 || links[0].IfName != bridgeName {
		return BridgeIPv4Profile{}, fmt.Errorf("inspect bridge %s ownership: expected exactly one matching interface, got %d", bridgeName, len(links))
	}
	expectedAlias := bridgeOwnershipAlias(networkName)
	if links[0].IfAlias != expectedAlias {
		return BridgeIPv4Profile{}, fmt.Errorf("refusing to use bridge %s without ownership tag %q (found %q)", bridgeName, expectedAlias, links[0].IfAlias)
	}

	out, err = run("-j", "addr", "show", "dev", bridgeName)
	if err != nil {
		return BridgeIPv4Profile{}, fmt.Errorf("inspect bridge %s IPv4 address: %w\n%s", bridgeName, err, out)
	}
	var addrs []bridgeAddrJSON
	if err := json.Unmarshal(out, &addrs); err != nil {
		return BridgeIPv4Profile{}, fmt.Errorf("decode bridge %s address JSON: %w", bridgeName, err)
	}
	if len(addrs) != 1 || addrs[0].IfName != bridgeName {
		return BridgeIPv4Profile{}, fmt.Errorf("inspect bridge %s IPv4 address: expected exactly one matching interface, got %d", bridgeName, len(addrs))
	}

	var local string
	var prefixLen int
	for _, addr := range addrs[0].AddrInfo {
		if addr.Family != "inet" || addr.Scope != "global" {
			continue
		}
		if local != "" {
			return BridgeIPv4Profile{}, fmt.Errorf("bridge %s has multiple global IPv4 addresses", bridgeName)
		}
		local = addr.Local
		prefixLen = addr.PrefixLen
	}
	if local == "" {
		return BridgeIPv4Profile{}, fmt.Errorf("bridge %s has no global IPv4 gateway address", bridgeName)
	}
	ip := net.ParseIP(local)
	if ip == nil || ip.To4() == nil || prefixLen < 0 || prefixLen > 32 {
		return BridgeIPv4Profile{}, fmt.Errorf("bridge %s has invalid IPv4 address %q/%d", bridgeName, local, prefixLen)
	}
	hostCIDR := fmt.Sprintf("%s/%d", local, prefixLen)
	_, subnet, err := net.ParseCIDR(hostCIDR)
	if err != nil {
		return BridgeIPv4Profile{}, fmt.Errorf("derive bridge %s subnet from %s: %w", bridgeName, hostCIDR, err)
	}

	return BridgeIPv4Profile{
		NetworkName: networkName,
		BridgeName:  bridgeName,
		HostCIDR:    hostCIDR,
		SubnetCIDR:  subnet.String(),
		Gateway:     local,
	}, nil
}
