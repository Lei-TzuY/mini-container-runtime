//go:build linux

package dns

import (
	"path/filepath"
	"testing"
)

func TestAllocatedBridgeAddressesPublishPerGeneration(t *testing.T) {
	useTempDNSHome(t)

	rollbackA, err := BeginHostRegistrationAttempt("default", "ctr-a", "alpha", "172.20.0.2")
	if err != nil {
		t.Fatal(err)
	}
	rollbackB, err := BeginHostRegistrationAttempt("default", "ctr-b", "beta", "172.20.0.2")
	if err != nil {
		_ = rollbackA()
		t.Fatal(err)
	}
	defer func() { _ = rollbackB() }()

	cancelA, err := StageHostRegistrationGenerationAddress("default", "ctr-a", "172.20.0.2")
	if err != nil {
		t.Fatal(err)
	}
	defer cancelA()
	cancelB, err := StageHostRegistrationGenerationAddress("default", "ctr-b", "172.20.0.3")
	if err != nil {
		t.Fatal(err)
	}
	defer cancelB()

	if err := BindHostRegistrationGeneration("default", "ctr-a", 4101, 101); err != nil {
		t.Fatal(err)
	}
	if err := BindHostRegistrationGeneration("default", "ctr-b", 4102, 102); err != nil {
		t.Fatal(err)
	}

	entries, exists, err := loadEntriesChecked(filepath.Join(DefaultDNSDir(), "default.json"), "default")
	if err != nil {
		t.Fatal(err)
	}
	if !exists || len(entries) != 2 {
		t.Fatalf("registry exists=%v entries=%d, want true/2", exists, len(entries))
	}
	byID := make(map[string]HostEntry, len(entries))
	for _, entry := range entries {
		byID[entry.ContainerID] = entry
	}
	if got := byID["ctr-a"]; got.IP != "172.20.0.2" || got.GenerationPID != 4101 || got.GenerationStartTime != 101 || got.AdmissionPending {
		t.Fatalf("ctr-a registration = %+v", got)
	}
	if got := byID["ctr-b"]; got.IP != "172.20.0.3" || got.GenerationPID != 4102 || got.GenerationStartTime != 102 || got.AdmissionPending {
		t.Fatalf("ctr-b registration = %+v", got)
	}

	// Rolling back one exact admission attempt must not erase the other visible
	// generation, which models independent concurrent bridge runtimes.
	if err := rollbackA(); err != nil {
		t.Fatal(err)
	}
	entries, exists, err = loadEntriesChecked(filepath.Join(DefaultDNSDir(), "default.json"), "default")
	if err != nil {
		t.Fatal(err)
	}
	if !exists || len(entries) != 1 || entries[0].ContainerID != "ctr-b" || entries[0].IP != "172.20.0.3" {
		t.Fatalf("registry after ctr-a rollback = %+v", entries)
	}
}

func TestGenerationAddressRollbackIsAttemptScoped(t *testing.T) {
	useTempDNSHome(t)

	cancelOld, err := StageHostRegistrationGenerationAddress("default", "ctr-retry", "172.20.0.2")
	if err != nil {
		t.Fatal(err)
	}
	cancelNew, err := StageHostRegistrationGenerationAddress("default", "ctr-retry", "172.20.0.9")
	if err != nil {
		t.Fatal(err)
	}
	defer cancelNew()
	cancelOld()

	rollback, err := BeginHostRegistrationAttempt("default", "ctr-retry", "retry", "172.20.0.2")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rollback() }()
	if err := BindHostRegistrationGeneration("default", "ctr-retry", 4201, 201); err != nil {
		t.Fatal(err)
	}
	entry := readSingleHostEntry(t, "default")
	if entry.IP != "172.20.0.9" {
		t.Fatalf("published IP = %q, want newer staged address", entry.IP)
	}
}
