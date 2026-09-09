package network

import (
	"strings"
	"testing"
)

func TestAllocateBridgeIPv4SkipsGatewayAndOwnedAddresses(t *testing.T) {
	used := map[string]struct{}{
		"172.20.0.2": {},
		"172.20.0.3": {},
	}
	got, err := AllocateBridgeIPv4("172.20.0.0/24", "172.20.0.1", used)
	if err != nil {
		t.Fatalf("AllocateBridgeIPv4: %v", err)
	}
	if got != "172.20.0.4" {
		t.Fatalf("allocated %q, want 172.20.0.4", got)
	}
}

func TestAllocateBridgeIPv4PreservesDefaultFirstAddress(t *testing.T) {
	got, err := AllocateBridgeIPv4("172.20.0.0/24", "172.20.0.1", nil)
	if err != nil {
		t.Fatalf("AllocateBridgeIPv4: %v", err)
	}
	if got != "172.20.0.2" {
		t.Fatalf("allocated %q, want compatibility address 172.20.0.2", got)
	}
}

func TestAllocateBridgeIPv4ReportsExhaustion(t *testing.T) {
	used := map[string]struct{}{"10.0.0.2": {}}
	_, err := AllocateBridgeIPv4("10.0.0.0/30", "10.0.0.1", used)
	if err == nil || !strings.Contains(err.Error(), "no free") {
		t.Fatalf("error=%v, want subnet exhaustion", err)
	}
}

func TestAllocateBridgeIPv4RejectsInvalidTopology(t *testing.T) {
	for _, tc := range []struct {
		name    string
		cidr    string
		gateway string
	}{
		{name: "ipv6", cidr: "fd00::/64", gateway: "fd00::1"},
		{name: "gateway outside", cidr: "172.20.0.0/24", gateway: "172.21.0.1"},
		{name: "no hosts", cidr: "172.20.0.0/31", gateway: "172.20.0.0"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := AllocateBridgeIPv4(tc.cidr, tc.gateway, nil); err == nil {
				t.Fatal("expected topology validation error")
			}
		})
	}
}
