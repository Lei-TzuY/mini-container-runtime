//go:build linux

package dns

import "testing"

func TestCleanupStoppedHostRegistrationRetryPreservesCurrentOwner(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	owner := registrarIdentity{PID: 101, StartTime: 1001}
	entry := HostEntry{
		ContainerID:    "target",
		Hostname:       "attempt-two",
		IP:             "172.20.0.2",
		OwnerPID:       owner.PID,
		OwnerStartTime: owner.StartTime,
	}
	writeOwnedDNSRegistry(t, "default", []HostEntry{entry})

	if err := cleanupStoppedHostRegistrationWithPolicy("default", "target", owner, func(HostEntry) (bool, error) {
		t.Fatal("ownership probe called for current owner")
		return false, nil
	}, false); err != nil {
		t.Fatalf("retry cleanup: %v", err)
	}

	entries := readOwnedDNSRegistry(t, "default")
	if len(entries) != 1 || entries[0] != entry {
		t.Fatalf("live next-attempt registration changed by stale finalizer retry: %+v", entries)
	}
}

func TestCleanupStoppedHostRegistrationRetryStillReclaimsStaleForeignOwner(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	current := registrarIdentity{PID: 101, StartTime: 1001}
	foreign := HostEntry{
		ContainerID:    "target",
		Hostname:       "stale",
		IP:             "172.20.0.2",
		OwnerPID:       202,
		OwnerStartTime: 2002,
	}
	writeOwnedDNSRegistry(t, "default", []HostEntry{foreign})

	probes := 0
	if err := cleanupStoppedHostRegistrationWithPolicy("default", "target", current, func(entry HostEntry) (bool, error) {
		probes++
		if entry != foreign {
			t.Fatalf("probed entry=%+v, want %+v", entry, foreign)
		}
		return false, nil
	}, false); err != nil {
		t.Fatalf("retry stale-owner cleanup: %v", err)
	}
	if probes != 1 {
		t.Fatalf("ownership probes=%d, want 1", probes)
	}
	if entries := readOwnedDNSRegistry(t, "default"); len(entries) != 0 {
		t.Fatalf("stale foreign registration remains: %+v", entries)
	}
}
