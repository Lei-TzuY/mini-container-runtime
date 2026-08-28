//go:build linux

package dns

import (
	"errors"
	"os"
	"os/exec"
	"testing"
	"time"

	"minicontainer/internal/state"
)

func TestCleanupStoppedHostRegistrationConsumesCurrentOwner(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	owner := registrarIdentity{PID: 101, StartTime: 1001}
	writeOwnedDNSRegistry(t, "default", []HostEntry{
		{ContainerID: "target", Hostname: "target", IP: "172.20.0.2", OwnerPID: owner.PID, OwnerStartTime: owner.StartTime},
		{ContainerID: "other", Hostname: "other", IP: "172.20.0.3", OwnerPID: 202, OwnerStartTime: 2002},
	})

	if err := cleanupStoppedHostRegistrationWith("default", "target", owner, func(HostEntry) (bool, error) {
		t.Fatal("ownership probe called for current owner")
		return false, nil
	}); err != nil {
		t.Fatalf("cleanup current owner: %v", err)
	}
	entries := readOwnedDNSRegistry(t, "default")
	if len(entries) != 1 || entries[0].ContainerID != "other" {
		t.Fatalf("registry after current-owner cleanup: %+v", entries)
	}
}

func TestCleanupStoppedHostRegistrationReclaimsProvablyStaleForeignOwner(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	foreign := HostEntry{ContainerID: "target", Hostname: "target", IP: "172.20.0.2", OwnerPID: 303, OwnerStartTime: 3003}
	writeOwnedDNSRegistry(t, "default", []HostEntry{foreign})

	probes := 0
	err := cleanupStoppedHostRegistrationWith("default", "target", registrarIdentity{PID: 404, StartTime: 4004}, func(entry HostEntry) (bool, error) {
		probes++
		if entry != foreign {
			t.Fatalf("probed entry=%+v, want %+v", entry, foreign)
		}
		return false, nil
	})
	if err != nil {
		t.Fatalf("cleanup stale foreign owner: %v", err)
	}
	if probes != 1 {
		t.Fatalf("ownership probes=%d, want 1", probes)
	}
	if entries := readOwnedDNSRegistry(t, "default"); len(entries) != 0 {
		t.Fatalf("stale registration remains: %+v", entries)
	}
}

func TestCleanupStoppedHostRegistrationRetainsActiveForeignOwner(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	foreign := HostEntry{ContainerID: "target", Hostname: "replacement", IP: "172.20.0.2", OwnerPID: 505, OwnerStartTime: 5005}
	writeOwnedDNSRegistry(t, "default", []HostEntry{foreign})

	err := cleanupStoppedHostRegistrationWith("default", "target", registrarIdentity{PID: 606, StartTime: 6006}, func(HostEntry) (bool, error) {
		return true, nil
	})
	if err != nil {
		t.Fatalf("cleanup active foreign owner: %v", err)
	}
	entries := readOwnedDNSRegistry(t, "default")
	if len(entries) != 1 || entries[0] != foreign {
		t.Fatalf("active replacement registration changed: %+v", entries)
	}
}

func TestCleanupStoppedHostRegistrationRetainsAdoptedLiveGeneration(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	deadRegistrar := exec.Command("sleep", "30")
	if err := deadRegistrar.Start(); err != nil {
		t.Fatalf("start registrar fixture: %v", err)
	}
	deadStart, err := readProcessStartTime(deadRegistrar.Process.Pid)
	if err != nil {
		_ = deadRegistrar.Process.Kill()
		_ = deadRegistrar.Wait()
		t.Fatalf("registrar start time: %v", err)
	}
	if err := deadRegistrar.Process.Kill(); err != nil {
		_ = deadRegistrar.Wait()
		t.Fatalf("kill registrar fixture: %v", err)
	}
	if err := deadRegistrar.Wait(); err == nil {
		t.Fatal("killed registrar fixture unexpectedly exited successfully")
	}

	child := exec.Command("sleep", "30")
	if err := child.Start(); err != nil {
		t.Fatalf("start adopted child fixture: %v", err)
	}
	defer func() {
		_ = child.Process.Kill()
		_ = child.Wait()
	}()
	childStart, err := readProcessStartTime(child.Process.Pid)
	if err != nil {
		t.Fatalf("child start time: %v", err)
	}

	const containerID = "0123456789abcdef"
	st, err := state.Open(state.DefaultDir())
	if err != nil {
		t.Fatalf("open state: %v", err)
	}
	defer st.Close()
	if err := st.Save(&state.Container{
		ID:           containerID,
		Status:       state.StatusRunning,
		PID:          child.Process.Pid,
		PIDStartTime: childStart,
		Hostname:     "adopted",
		CreatedAt:    time.Now(),
	}); err != nil {
		t.Fatalf("save adopted running state: %v", err)
	}
	markDNSBridgeOwnership(t, st, containerID, child.Process.Pid, childStart)

	foreign := HostEntry{
		ContainerID:    containerID,
		Hostname:       "adopted",
		IP:             "172.20.0.2",
		OwnerPID:       deadRegistrar.Process.Pid,
		OwnerStartTime: deadStart,
	}
	writeOwnedDNSRegistry(t, "default", []HostEntry{foreign})
	current, err := currentRegistrarIdentity()
	if err != nil {
		t.Fatalf("current registrar identity: %v", err)
	}

	if err := cleanupStoppedHostRegistrationWith("default", containerID, current, hostEntryOwnerActive); err != nil {
		t.Fatalf("cleanup adopted live generation: %v", err)
	}
	entries := readOwnedDNSRegistry(t, "default")
	if len(entries) != 1 || entries[0] != foreign {
		t.Fatalf("adopted live registration was removed: %+v", entries)
	}
}

func TestCleanupStoppedHostRegistrationRetainsLegacyEntryWithoutGuessing(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	legacy := HostEntry{ContainerID: "target", Hostname: "legacy", IP: "172.20.0.2"}
	writeOwnedDNSRegistry(t, "default", []HostEntry{legacy})

	err := cleanupStoppedHostRegistrationWith("default", "target", registrarIdentity{PID: 707, StartTime: 7007}, func(HostEntry) (bool, error) {
		t.Fatal("ownership probe called for legacy entry")
		return false, nil
	})
	if err != nil {
		t.Fatalf("cleanup legacy entry: %v", err)
	}
	entries := readOwnedDNSRegistry(t, "default")
	if len(entries) != 1 || entries[0] != legacy {
		t.Fatalf("legacy registration changed without ownership proof: %+v", entries)
	}
}

func TestCleanupStoppedHostRegistrationFailsClosedOnOwnershipUncertainty(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	foreign := HostEntry{ContainerID: "target", Hostname: "target", IP: "172.20.0.2", OwnerPID: 808, OwnerStartTime: 8008}
	writeOwnedDNSRegistry(t, "default", []HostEntry{foreign})
	cause := errors.New("process identity unavailable")

	err := cleanupStoppedHostRegistrationWith("default", "target", registrarIdentity{PID: 909, StartTime: 9009}, func(HostEntry) (bool, error) {
		return false, cause
	})
	if !errors.Is(err, cause) {
		t.Fatalf("cleanup error=%v, want ownership cause", err)
	}
	entries := readOwnedDNSRegistry(t, "default")
	if len(entries) != 1 || entries[0] != foreign {
		t.Fatalf("registry changed after uncertain ownership probe: %+v", entries)
	}
}

func TestCleanupStoppedHostRegistrationMissingDirIsSideEffectFree(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	err := cleanupStoppedHostRegistrationWith("default", "target", registrarIdentity{PID: 1001, StartTime: 10001}, func(HostEntry) (bool, error) {
		t.Fatal("ownership probe called without DNS registry")
		return false, nil
	})
	if err != nil {
		t.Fatalf("missing registry cleanup: %v", err)
	}
	if _, err := os.Lstat(DefaultDNSDir()); !os.IsNotExist(err) {
		t.Fatalf("DNS directory created by no-op stopped cleanup: %v", err)
	}
}
