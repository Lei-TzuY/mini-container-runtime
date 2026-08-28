//go:build linux

package dns

import (
	"os"
	"strings"
	"testing"
)

func TestGenerateHostsHidesUnboundGenerationAwareReservation(t *testing.T) {
	useTempDNSHome(t)
	if err := RegisterHost("default", "ctr-pending", "pending-host", "10.0.0.20"); err != nil {
		t.Fatal(err)
	}

	content, err := GenerateHostsContentChecked("default")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(content, "pending-host") {
		t.Fatalf("unbound reservation leaked into service discovery:\n%s", content)
	}

	entry := readSingleHostEntry(t, "default")
	if !entry.GenerationAware || entry.GenerationPID != 0 || entry.GenerationStartTime != 0 {
		t.Fatalf("hidden reservation was unexpectedly changed: %+v", entry)
	}
}

func TestUnboundReservationStillRejectsCompetingHostname(t *testing.T) {
	useTempDNSHome(t)
	if err := RegisterHost("default", "ctr-owner", "reserved-host", "10.0.0.21"); err != nil {
		t.Fatal(err)
	}
	if err := RegisterHost("default", "ctr-racer", "reserved-host", "10.0.0.22"); err == nil {
		t.Fatal("competing registration stole an unbound hostname reservation")
	}

	content, err := GenerateHostsContentChecked("default")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(content, "reserved-host") {
		t.Fatalf("reserved but unbound hostname became discoverable:\n%s", content)
	}
}

func TestGenerateHostsPublishesReservationAfterExactGenerationBind(t *testing.T) {
	useTempDNSHome(t)
	identity, err := currentRegistrarIdentity()
	if err != nil {
		t.Fatal(err)
	}
	if err := RegisterHost("default", "ctr-ready", "ready-host", "10.0.0.23"); err != nil {
		t.Fatal(err)
	}
	if err := BindHostRegistrationGeneration("default", "ctr-ready", os.Getpid(), identity.StartTime); err != nil {
		t.Fatal(err)
	}

	content, err := GenerateHostsContentChecked("default")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(content, "10.0.0.23\tready-host") {
		t.Fatalf("bound generation was not published to service discovery:\n%s", content)
	}
}
