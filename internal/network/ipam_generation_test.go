package network

import "testing"

func TestGenerationScopedBridgeLeasesPersistAndReleaseExactly(t *testing.T) {
	dir := t.TempDir()
	first, err := OpenIPAM(dir)
	if err != nil {
		t.Fatal(err)
	}

	gen1 := BridgeLeaseOwner{ContainerID: "demo", PID: 101, PIDStartTime: 1001}
	gen2 := BridgeLeaseOwner{ContainerID: "demo", PID: 202, PIDStartTime: 2002}

	ip1, err := first.AllocateGenerationIP("bridge0", "172.20.0.0/24", gen1)
	if err != nil {
		t.Fatal(err)
	}
	if ip1 != "172.20.0.2" {
		t.Fatalf("first generation IP = %q, want 172.20.0.2", ip1)
	}
	ip2, err := first.AllocateGenerationIP("bridge0", "172.20.0.0/24", gen2)
	if err != nil {
		t.Fatal(err)
	}
	if ip2 != "172.20.0.3" {
		t.Fatalf("second generation IP = %q, want 172.20.0.3", ip2)
	}

	// A second manager instance observes the durable pool, matching independent
	// runtime processes sharing the same state directory.
	second, err := OpenIPAM(dir)
	if err != nil {
		t.Fatal(err)
	}
	repeated, err := second.AllocateGenerationIP("bridge0", "172.20.0.0/24", gen2)
	if err != nil {
		t.Fatal(err)
	}
	if repeated != ip2 {
		t.Fatalf("reopened manager returned %q, want persisted %q", repeated, ip2)
	}

	// Releasing the stale generation must not disturb the newer generation.
	if err := second.ReleaseGenerationIP("bridge0", gen1); err != nil {
		t.Fatal(err)
	}
	repeated, err = first.AllocateGenerationIP("bridge0", "172.20.0.0/24", gen2)
	if err != nil {
		t.Fatal(err)
	}
	if repeated != ip2 {
		t.Fatalf("newer generation moved to %q after stale release; want %q", repeated, ip2)
	}

	gen3 := BridgeLeaseOwner{ContainerID: "other", PID: 303, PIDStartTime: 3003}
	ip3, err := first.AllocateGenerationIP("bridge0", "172.20.0.0/24", gen3)
	if err != nil {
		t.Fatal(err)
	}
	if ip3 != "172.20.0.2" {
		t.Fatalf("released address was not reusable: got %q, want 172.20.0.2", ip3)
	}
}

func TestGenerationScopedBridgeLeaseRejectsMissingIdentity(t *testing.T) {
	ipam, err := OpenIPAM(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, owner := range []BridgeLeaseOwner{
		{},
		{ContainerID: "demo", PID: 1},
		{ContainerID: "demo", PIDStartTime: 1},
	} {
		if _, err := ipam.AllocateGenerationIP("bridge0", "172.20.0.0/24", owner); err == nil {
			t.Fatalf("AllocateGenerationIP(%+v) succeeded, want identity validation error", owner)
		}
	}
}
