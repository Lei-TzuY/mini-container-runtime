//go:build linux

package dns

import (
	"os/exec"
	"testing"
)

func TestHostEntryOwnerActiveBoundGenerationIgnoresLiveRegistrarAfterChildDeath(t *testing.T) {
	owner, err := currentRegistrarIdentity()
	if err != nil {
		t.Fatalf("current registrar identity: %v", err)
	}

	child := exec.Command("sleep", "30")
	if err := child.Start(); err != nil {
		t.Fatalf("start child fixture: %v", err)
	}
	childStart, err := readProcessStartTime(child.Process.Pid)
	if err != nil {
		_ = child.Process.Kill()
		_ = child.Wait()
		t.Fatalf("child start time: %v", err)
	}
	if err := child.Process.Kill(); err != nil {
		_ = child.Wait()
		t.Fatalf("kill child fixture: %v", err)
	}
	if err := child.Wait(); err == nil {
		t.Fatal("killed child fixture unexpectedly exited successfully")
	}

	active, err := hostEntryOwnerActive(HostEntry{
		ContainerID:         "dead-child",
		Hostname:            "dead-child",
		IP:                  "172.20.0.2",
		OwnerPID:            owner.PID,
		OwnerStartTime:      owner.StartTime,
		GenerationAware:     true,
		GenerationPID:       child.Process.Pid,
		GenerationStartTime: childStart,
	})
	if err != nil {
		t.Fatalf("probe bound dead child: %v", err)
	}
	if active {
		t.Fatal("bound DNS entry remained active solely because registrar was alive")
	}
}

func TestHostEntryOwnerActiveBoundGenerationSurvivesWithoutRegistrarOrState(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	child := exec.Command("sleep", "30")
	if err := child.Start(); err != nil {
		t.Fatalf("start child fixture: %v", err)
	}
	defer func() {
		_ = child.Process.Kill()
		_ = child.Wait()
	}()
	childStart, err := readProcessStartTime(child.Process.Pid)
	if err != nil {
		t.Fatalf("child start time: %v", err)
	}

	active, err := hostEntryOwnerActive(HostEntry{
		ContainerID:         "live-child",
		Hostname:            "live-child",
		IP:                  "172.20.0.2",
		OwnerPID:            2147483647,
		OwnerStartTime:      1,
		GenerationAware:     true,
		GenerationPID:       child.Process.Pid,
		GenerationStartTime: childStart,
	})
	if err != nil {
		t.Fatalf("probe bound live child: %v", err)
	}
	if !active {
		t.Fatal("bound live child was not authoritative")
	}
}

func TestHostEntryOwnerActiveRejectsIncompleteBoundGeneration(t *testing.T) {
	_, err := hostEntryOwnerActive(HostEntry{
		ContainerID:     "partial-generation",
		GenerationAware: true,
		GenerationPID:   1234,
	})
	if err == nil {
		t.Fatal("incomplete child generation identity was accepted")
	}
}
