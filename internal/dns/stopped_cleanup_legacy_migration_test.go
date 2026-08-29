//go:build linux

package dns

import "testing"

func TestCleanupStoppedGenerationRetiresRegistrarOwnedLegacyWithoutLivenessProbe(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	legacy := HostEntry{
		ContainerID:    "target",
		Hostname:       "target",
		IP:             "172.20.0.2",
		OwnerPID:       606,
		OwnerStartTime: 6006,
	}
	writeOwnedDNSRegistry(t, "default", []HostEntry{legacy})

	err := cleanupStoppedHostRegistrationWithGenerationPolicy(
		"default",
		"target",
		registrarIdentity{PID: 808, StartTime: 8008},
		childGenerationIdentity{PID: 707, StartTime: 7007},
		func(HostEntry) (bool, error) {
			t.Fatal("legacy migration cleanup must not depend on registrar liveness")
			return true, nil
		},
	)
	if err != nil {
		t.Fatalf("cleanup registrar-owned legacy registration: %v", err)
	}
	if entries := readOwnedDNSRegistry(t, "default"); len(entries) != 0 {
		t.Fatalf("registrar-owned legacy registration remains: %+v", entries)
	}
}

func TestCleanupStoppedGenerationPreservesModernReplacementWhileRetiringLegacyPolicy(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	replacement := HostEntry{
		ContainerID:         "target",
		Hostname:            "target",
		IP:                  "172.20.0.3",
		OwnerPID:            909,
		OwnerStartTime:      9009,
		GenerationAware:     true,
		GenerationPID:       1001,
		GenerationStartTime: 10001,
	}
	writeOwnedDNSRegistry(t, "default", []HostEntry{replacement})

	err := cleanupStoppedHostRegistrationWithGenerationPolicy(
		"default",
		"target",
		registrarIdentity{PID: 808, StartTime: 8008},
		childGenerationIdentity{PID: 707, StartTime: 7007},
		func(HostEntry) (bool, error) {
			t.Fatal("modern replacement must never fall back to registrar liveness")
			return false, nil
		},
	)
	if err != nil {
		t.Fatalf("cleanup stale stopped generation: %v", err)
	}
	entries := readOwnedDNSRegistry(t, "default")
	if len(entries) != 1 || entries[0] != replacement {
		t.Fatalf("modern replacement changed: %+v", entries)
	}
}
