package network

import "testing"

func TestReconcileGenerationIPsReclaimsOnlyDeadExactGenerations(t *testing.T) {
	ipam, err := OpenIPAM(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	live := BridgeLeaseOwner{ContainerID: "live", PID: 101, PIDStartTime: 1001}
	dead := BridgeLeaseOwner{ContainerID: "dead", PID: 202, PIDStartTime: 2002}
	liveIP, err := ipam.AllocateGenerationIP("bridge0", "172.20.0.0/24", live)
	if err != nil {
		t.Fatal(err)
	}
	deadIP, err := ipam.AllocateGenerationIP("bridge0", "172.20.0.0/24", dead)
	if err != nil {
		t.Fatal(err)
	}

	reclaimed, err := ipam.ReconcileGenerationIPs("bridge0", func(owner BridgeLeaseOwner) bool {
		return owner == live
	})
	if err != nil {
		t.Fatal(err)
	}
	if reclaimed != 1 {
		t.Fatalf("reclaimed = %d, want 1", reclaimed)
	}

	reopened, err := OpenIPAM(ipam.dir)
	if err != nil {
		t.Fatal(err)
	}
	gotLive, err := reopened.AllocateGenerationIP("bridge0", "172.20.0.0/24", live)
	if err != nil {
		t.Fatal(err)
	}
	if gotLive != liveIP {
		t.Fatalf("live generation moved from %s to %s", liveIP, gotLive)
	}
	newOwner := BridgeLeaseOwner{ContainerID: "new", PID: 303, PIDStartTime: 3003}
	gotNew, err := reopened.AllocateGenerationIP("bridge0", "172.20.0.0/24", newOwner)
	if err != nil {
		t.Fatal(err)
	}
	if gotNew != deadIP {
		t.Fatalf("reclaimed address = %s, want %s", gotNew, deadIP)
	}
}

func TestParseBridgeLeaseOwnerRejectsLegacyOwner(t *testing.T) {
	if _, err := ParseBridgeLeaseOwner("legacy-container-id"); err == nil {
		t.Fatal("expected legacy owner to be rejected")
	}
}
