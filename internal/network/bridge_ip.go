package network

import (
	"encoding/binary"
	"fmt"
	"net"
)

// AllocateBridgeIPv4 selects the lowest free usable IPv4 address in bridgeCIDR.
// The network address, broadcast address, and gateway are never returned.
// used contains canonical textual IP addresses already owned by live or
// recoverable container generations. Deterministic lowest-address selection
// makes allocation reproducible while durable ownership remains the authority
// that prevents reuse across concurrent runtime attempts.
func AllocateBridgeIPv4(bridgeCIDR, gateway string, used map[string]struct{}) (string, error) {
	gatewayIP := net.ParseIP(gateway).To4()
	if gatewayIP == nil {
		return "", fmt.Errorf("bridge gateway %q is not IPv4", gateway)
	}
	_, subnet, err := net.ParseCIDR(bridgeCIDR)
	if err != nil {
		return "", fmt.Errorf("parse bridge CIDR %q: %w", bridgeCIDR, err)
	}
	networkIP := subnet.IP.To4()
	if networkIP == nil {
		return "", fmt.Errorf("bridge CIDR %q is not IPv4", bridgeCIDR)
	}
	ones, bits := subnet.Mask.Size()
	if bits != 32 || ones > 30 {
		return "", fmt.Errorf("bridge CIDR %q has no allocatable container IPv4 addresses", bridgeCIDR)
	}
	if !subnet.Contains(gatewayIP) {
		return "", fmt.Errorf("bridge gateway %q is outside %q", gateway, bridgeCIDR)
	}

	first := binary.BigEndian.Uint32(networkIP)
	last := first | ^binary.BigEndian.Uint32(net.IP(subnet.Mask).To4())
	gatewayValue := binary.BigEndian.Uint32(gatewayIP)
	for value := first + 1; value < last; value++ {
		if value == gatewayValue {
			continue
		}
		var raw [4]byte
		binary.BigEndian.PutUint32(raw[:], value)
		candidate := net.IP(raw[:]).String()
		if _, occupied := used[candidate]; occupied {
			continue
		}
		return candidate, nil
	}
	return "", fmt.Errorf("bridge CIDR %q has no free container IPv4 addresses", bridgeCIDR)
}
