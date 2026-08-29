//go:build linux

package dns

import (
	"errors"
	"testing"
)

func TestCleanupStoppedLegacyRegistrarHostRegistrationScopesMigrationAuthority(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	stale := HostEntry{ContainerID: "target", Hostname: "stale", IP: "172.20.0.2", OwnerPID: 101, OwnerStartTime: 1001}
	live := HostEntry{ContainerID: "target", Hostname: "live", IP: "172.20.0.3", OwnerPID: 202, OwnerStartTime: 2002}
	modern := HostEntry{ContainerID: "target", Hostname: "modern", IP: "172.20.0.4", OwnerPID: 303, OwnerStartTime: 3003, GenerationAware: true, GenerationPID: 404, GenerationStartTime: 4004}
	ownerless := HostEntry{ContainerID: "target", Hostname: "ownerless", IP: "172.20.0.5"}
	foreign := HostEntry{ContainerID: "other", Hostname: "foreign", IP: "172.20.0.6", OwnerPID: 505, OwnerStartTime: 5005}
	writeOwnedDNSRegistry(t, "default", []HostEntry{stale, live, modern, ownerless, foreign})

	probes := 0
	if err := cleanupStoppedLegacyRegistrarHostRegistrationWith("default", "target", func(entry HostEntry) (bool, error) {
		probes++
		switch entry.Hostname {
		case stale.Hostname:
			return false, nil
		case live.Hostname:
			return true, nil
		default:
			t.Fatalf("unexpected liveness probe for %+v", entry)
			return false, nil
		}
	}); err != nil {
		t.Fatalf("cleanup legacy registrar DNS: %v", err)
	}
	if probes != 2 {
		t.Fatalf("liveness probes=%d, want 2", probes)
	}

	got := readOwnedDNSRegistry(t, "default")
	if len(got) != 4 {
		t.Fatalf("entries after cleanup=%+v, want four preserved entries", got)
	}
	for _, entry := range got {
		if entry == stale {
			t.Fatalf("stale legacy registrar entry survived: %+v", got)
		}
	}
}

func TestCleanupStoppedLegacyRegistrarHostRegistrationProbeErrorFailsClosed(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	entry := HostEntry{ContainerID: "target", Hostname: "legacy", IP: "172.20.0.2", OwnerPID: 101, OwnerStartTime: 1001}
	writeOwnedDNSRegistry(t, "default", []HostEntry{entry})
	probeErr := errors.New("probe failed")

	err := cleanupStoppedLegacyRegistrarHostRegistrationWith("default", "target", func(HostEntry) (bool, error) {
		return false, probeErr
	})
	if !errors.Is(err, probeErr) {
		t.Fatalf("cleanup error=%v, want probe error", err)
	}
	got := readOwnedDNSRegistry(t, "default")
	if len(got) != 1 || got[0] != entry {
		t.Fatalf("probe failure mutated registry: %+v", got)
	}
}
