//go:build linux

package dns

import (
	"errors"
	"testing"
)

func TestCleanupStoppedLegacyHostRegistrationsScopesMigrationAuthority(t *testing.T) {
	t.Run("reclaims ownerless and stale registrar records atomically", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("HOME", home)
		t.Setenv("USERPROFILE", home)

		ownerless := HostEntry{ContainerID: "target", Hostname: "ownerless", IP: "172.20.0.2"}
		stale := HostEntry{ContainerID: "target", Hostname: "stale", IP: "172.20.0.3", OwnerPID: 101, OwnerStartTime: 1001}
		foreign := HostEntry{ContainerID: "other", Hostname: "foreign", IP: "172.20.0.6", OwnerPID: 505, OwnerStartTime: 5005}
		writeOwnedDNSRegistry(t, "default", []HostEntry{ownerless, stale, foreign})

		probes := 0
		if err := cleanupStoppedLegacyHostRegistrationsWith("default", "target", func(entry HostEntry) (bool, error) {
			probes++
			if entry != stale {
				t.Fatalf("probed entry=%+v, want stale target", entry)
			}
			return false, nil
		}); err != nil {
			t.Fatalf("cleanup legacy DNS migration records: %v", err)
		}
		if probes != 1 {
			t.Fatalf("liveness probes=%d, want 1", probes)
		}
		got := readOwnedDNSRegistry(t, "default")
		if len(got) != 1 || got[0] != foreign {
			t.Fatalf("cleanup result=%+v, want only foreign entry", got)
		}
	})

	t.Run("preserves live legacy owner while retiring ownerless debris", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("HOME", home)
		t.Setenv("USERPROFILE", home)

		ownerless := HostEntry{ContainerID: "target", Hostname: "ownerless", IP: "172.20.0.2"}
		live := HostEntry{ContainerID: "target", Hostname: "live", IP: "172.20.0.3", OwnerPID: 202, OwnerStartTime: 2002}
		writeOwnedDNSRegistry(t, "default", []HostEntry{ownerless, live})

		if err := cleanupStoppedLegacyHostRegistrationsWith("default", "target", func(entry HostEntry) (bool, error) {
			if entry != live {
				t.Fatalf("probed entry=%+v, want live target", entry)
			}
			return true, nil
		}); err != nil {
			t.Fatalf("cleanup live legacy registrar DNS: %v", err)
		}
		got := readOwnedDNSRegistry(t, "default")
		if len(got) != 1 || got[0] != live {
			t.Fatalf("live legacy entry changed: %+v", got)
		}
	})

	t.Run("preserves modern generation without probing", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("HOME", home)
		t.Setenv("USERPROFILE", home)

		modern := HostEntry{ContainerID: "target", Hostname: "modern", IP: "172.20.0.4", OwnerPID: 303, OwnerStartTime: 3003, GenerationAware: true, GenerationPID: 404, GenerationStartTime: 4004}
		writeOwnedDNSRegistry(t, "default", []HostEntry{modern})

		if err := cleanupStoppedLegacyHostRegistrationsWith("default", "target", func(HostEntry) (bool, error) {
			t.Fatal("liveness probe called for modern generation-aware entry")
			return false, nil
		}); err != nil {
			t.Fatalf("cleanup modern DNS: %v", err)
		}
		got := readOwnedDNSRegistry(t, "default")
		if len(got) != 1 || got[0] != modern {
			t.Fatalf("modern generation changed: %+v", got)
		}
	})
}

func TestCleanupStoppedLegacyHostRegistrationsProbeErrorFailsClosed(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	ownerless := HostEntry{ContainerID: "target", Hostname: "ownerless", IP: "172.20.0.2"}
	owned := HostEntry{ContainerID: "target", Hostname: "legacy", IP: "172.20.0.3", OwnerPID: 101, OwnerStartTime: 1001}
	writeOwnedDNSRegistry(t, "default", []HostEntry{ownerless, owned})
	probeErr := errors.New("probe failed")

	err := cleanupStoppedLegacyHostRegistrationsWith("default", "target", func(HostEntry) (bool, error) {
		return false, probeErr
	})
	if !errors.Is(err, probeErr) {
		t.Fatalf("cleanup error=%v, want probe error", err)
	}
	got := readOwnedDNSRegistry(t, "default")
	if len(got) != 2 || got[0] != ownerless || got[1] != owned {
		t.Fatalf("probe failure partially mutated registry: %+v", got)
	}
}
