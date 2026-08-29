//go:build linux

package dns

import "testing"

func TestCleanupStoppedOwnerlessLegacyHostRegistrationRemovesOnlyTarget(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	ownerlessTarget := HostEntry{
		ContainerID: "target",
		Hostname:    "old-target",
		IP:          "172.20.0.2",
	}
	otherOwnerless := HostEntry{
		ContainerID: "other",
		Hostname:    "other",
		IP:          "172.20.0.5",
	}
	writeOwnedDNSRegistry(t, "default", []HostEntry{ownerlessTarget, otherOwnerless})

	if err := CleanupStoppedOwnerlessLegacyHostRegistration("default", "target"); err != nil {
		t.Fatalf("cleanup ownerless legacy registration: %v", err)
	}

	entries := readOwnedDNSRegistry(t, "default")
	if len(entries) != 1 || entries[0] != otherOwnerless {
		t.Fatalf("cleanup changed non-target ownerless entry: %+v", entries)
	}
}

func TestCleanupStoppedOwnerlessLegacyHostRegistrationPreservesRegistrarOwnedTarget(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	owned := HostEntry{
		ContainerID:    "target",
		Hostname:       "target",
		IP:             "172.20.0.3",
		OwnerPID:       101,
		OwnerStartTime: 1001,
	}
	writeOwnedDNSRegistry(t, "default", []HostEntry{owned})

	if err := CleanupStoppedOwnerlessLegacyHostRegistration("default", "target"); err != nil {
		t.Fatalf("cleanup registrar-owned target: %v", err)
	}
	entries := readOwnedDNSRegistry(t, "default")
	if len(entries) != 1 || entries[0] != owned {
		t.Fatalf("registrar-owned target changed: %+v", entries)
	}
}

func TestCleanupStoppedOwnerlessLegacyHostRegistrationPreservesModernTarget(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	modern := HostEntry{
		ContainerID:         "target",
		Hostname:            "target",
		IP:                  "172.20.0.4",
		OwnerPID:            202,
		OwnerStartTime:      2002,
		GenerationAware:     true,
		GenerationPID:       303,
		GenerationStartTime: 3003,
	}
	writeOwnedDNSRegistry(t, "default", []HostEntry{modern})

	if err := CleanupStoppedOwnerlessLegacyHostRegistration("default", "target"); err != nil {
		t.Fatalf("cleanup modern target: %v", err)
	}
	entries := readOwnedDNSRegistry(t, "default")
	if len(entries) != 1 || entries[0] != modern {
		t.Fatalf("modern target changed: %+v", entries)
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
