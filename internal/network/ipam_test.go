package network

import (
	"testing"
)

func TestIPAMAllocation(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	t.Setenv("USERPROFILE", tmpHome)

	ipam := NewIPAM()

	ip1, err := ipam.AllocateIP("demo-net", "172.28.0.0/24", "ctr1")
	if err != nil || ip1 != "172.28.0.2" {
		t.Fatalf("AllocateIP ctr1 = %s, err: %v (want 172.28.0.2)", ip1, err)
	}

	ip2, err := ipam.AllocateIP("demo-net", "172.28.0.0/24", "ctr2")
	if err != nil || ip2 != "172.28.0.3" {
		t.Fatalf("AllocateIP ctr2 = %s, err: %v (want 172.28.0.3)", ip2, err)
	}

	// Idempotent allocation for same container
	ip1Again, err := ipam.AllocateIP("demo-net", "172.28.0.0/24", "ctr1")
	if err != nil || ip1Again != ip1 {
		t.Fatalf("Re-AllocateIP ctr1 = %s, want %s", ip1Again, ip1)
	}

	// Release ctr1 IP
	if err := ipam.ReleaseIP("demo-net", "ctr1"); err != nil {
		t.Fatalf("ReleaseIP ctr1 error: %v", err)
	}

	// Allocate ctr3 should reuse 172.28.0.2
	ip3, err := ipam.AllocateIP("demo-net", "172.28.0.0/24", "ctr3")
	if err != nil || ip3 != "172.28.0.2" {
		t.Fatalf("AllocateIP ctr3 after release = %s, want 172.28.0.2", ip3)
	}
}
