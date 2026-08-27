//go:build linux

package dns

import (
	"os/exec"
	"testing"
	"time"

	"minicontainer/internal/state"
)

func TestCleanupStoppedHostRegistrationReclaimsDeadRegistrarAfterStoppedState(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	registrar := exec.Command("sleep", "30")
	if err := registrar.Start(); err != nil {
		t.Fatalf("start registrar fixture: %v", err)
	}
	registrarStart, err := readProcessStartTime(registrar.Process.Pid)
	if err != nil {
		_ = registrar.Process.Kill()
		_ = registrar.Wait()
		t.Fatalf("registrar start time: %v", err)
	}
	if err := registrar.Process.Kill(); err != nil {
		_ = registrar.Wait()
		t.Fatalf("kill registrar fixture: %v", err)
	}
	if err := registrar.Wait(); err == nil {
		t.Fatal("killed registrar fixture unexpectedly exited successfully")
	}

	const containerID = "fedcba9876543210"
	st, err := state.Open(state.DefaultDir())
	if err != nil {
		t.Fatalf("open state: %v", err)
	}
	defer st.Close()
	if err := st.Save(&state.Container{
		ID:        containerID,
		Status:    state.StatusStopped,
		CreatedAt: time.Now(),
	}); err != nil {
		t.Fatalf("save stopped state: %v", err)
	}

	writeOwnedDNSRegistry(t, "default", []HostEntry{{
		ContainerID:    containerID,
		Hostname:       "stale",
		IP:             "172.20.0.2",
		OwnerPID:       registrar.Process.Pid,
		OwnerStartTime: registrarStart,
	}})

	if err := CleanupStoppedHostRegistration("default", containerID); err != nil {
		t.Fatalf("CleanupStoppedHostRegistration: %v", err)
	}
	if entries := readOwnedDNSRegistry(t, "default"); len(entries) != 0 {
		t.Fatalf("dead registrar registration remains after stopped cleanup: %+v", entries)
	}
}
