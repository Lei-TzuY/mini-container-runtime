//go:build linux

package dns

import "testing"

func TestLegacyStoppedCleanupRetainsGenerationAwareCurrentOwner(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	owner := registrarIdentity{PID: 101, StartTime: 1001}
	modern := HostEntry{
		ContainerID:        "target",
		Hostname:           "target",
		IP:                 "172.20.0.2",
		OwnerPID:           owner.PID,
		OwnerStartTime:     owner.StartTime,
		GenerationAware:    true,
		GenerationPID:      202,
		GenerationStartTime: 2002,
	}
	writeOwnedDNSRegistry(t, "default", []HostEntry{modern})

	if err := cleanupStoppedHostRegistrationWithPolicy("default", "target", owner, func(HostEntry) (bool, error) {
		t.Fatal("ownership probe called for generation-aware entry")
		return false, nil
	}, true); err != nil {
		t.Fatalf("legacy cleanup modern current-owner entry: %v", err)
	}

	entries := readOwnedDNSRegistry(t, "default")
	if len(entries) != 1 || entries[0] != modern {
		t.Fatalf("generation-aware entry changed without exact generation proof: %+v", entries)
	}
}

func TestLegacyStoppedCleanupRetainsGenerationAwareStaleForeignOwner(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	modern := HostEntry{
		ContainerID:        "target",
		Hostname:           "replacement",
		IP:                 "172.20.0.3",
		OwnerPID:           303,
		OwnerStartTime:     3003,
		GenerationAware:    true,
		GenerationPID:      404,
		GenerationStartTime: 4004,
	}
	writeOwnedDNSRegistry(t, "default", []HostEntry{modern})

	if err := cleanupStoppedHostRegistrationWithPolicy("default", "target", registrarIdentity{PID: 505, StartTime: 5005}, func(HostEntry) (bool, error) {
		t.Fatal("ownership probe called for generation-aware replacement")
		return false, nil
	}, false); err != nil {
		t.Fatalf("legacy retry cleanup modern foreign entry: %v", err)
	}

	entries := readOwnedDNSRegistry(t, "default")
	if len(entries) != 1 || entries[0] != modern {
		t.Fatalf("generation-aware replacement changed by registrar fallback: %+v", entries)
	}
}
