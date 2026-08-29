//go:build linux

package dns

import "testing"

func TestCleanupStoppedOwnerlessLegacyHostRegistrationIsMigrationScoped(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	ownerlessTarget := HostEntry{
		ContainerID: "target",
		Hostname:    "old-target",
		IP:          "172.20.0.2",
	}
	registrarOwnedTarget := HostEntry{
		ContainerID:    "target",
		Hostname:       "owned-target",
		IP:             "172.20.0.3",
		OwnerPID:       101,
		OwnerStartTime: 1001,
	}
	modernTarget := HostEntry{
		ContainerID:         "target",
		Hostname:            "modern-target",
		IP:                  "172.20.0.4",
		OwnerPID:            202,
		OwnerStartTime:      2002,
		GenerationAware:     true,
		GenerationPID:       303,
		GenerationStartTime: 3003,
	}
	otherOwnerless := HostEntry{
		ContainerID: "other",
		Hostname:    "other",
		IP:          "172.20.0.5",
	}
	writeOwnedDNSRegistry(t, "default", []HostEntry{
		ownerlessTarget,
		registrarOwnedTarget,
		modernTarget,
		otherOwnerless,
	})

	if err := CleanupStoppedOwnerlessLegacyHostRegistration("default", "target"); err != nil {
		t.Fatalf("cleanup ownerless legacy registration: %v", err)
	}

	entries := readOwnedDNSRegistry(t, "default")
	if len(entries) != 3 {
		t.Fatalf("expected only one ownerless target entry removed, got %+v", entries)
	}
	want := []HostEntry{registrarOwnedTarget, modernTarget, otherOwnerless}
	for i := range want {
		if entries[i] != want[i] {
			t.Fatalf("entry %d changed: got %+v want %+v", i, entries[i], want[i])
		}
	}
}

func TestCleanupStoppedOwnerlessLegacyHostRegistrationIsIdempotent(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	writeOwnedDNSRegistry(t, "default", []HostEntry{{
		ContainerID: "target",
		Hostname:    "target",
		IP:          "172.20.0.2",
	}})

	for i := 0; i < 2; i++ {
		if err := CleanupStoppedOwnerlessLegacyHostRegistration("default", "target"); err != nil {
			t.Fatalf("cleanup pass %d: %v", i+1, err)
		}
	}
	if entries := readOwnedDNSRegistry(t, "default"); len(entries) != 0 {
		t.Fatalf("ownerless legacy entry remains after idempotent cleanup: %+v", entries)
	}
}
