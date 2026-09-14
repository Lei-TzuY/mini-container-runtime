//go:build linux

package main

import (
	"os"
	"os/exec"
	"testing"
	"time"

	"minicontainer/internal/container"
	"minicontainer/internal/state"
)

func TestRestartRunningContainerReloadsDurableStoppedStateAfterStop(t *testing.T) {
	stateDir := t.TempDir()
	rootfs := t.TempDir()
	id := "restart-running-stale-stop-snapshot"

	old := exec.Command("/bin/sleep", "30")
	if err := old.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if old.ProcessState == nil {
			_ = old.Process.Kill()
			_, _ = old.Process.Wait()
		}
	}()
	startTime, err := container.ProcessStartTime(old.Process.Pid)
	if err != nil {
		t.Fatal(err)
	}

	st, err := state.Open(stateDir)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Save(&state.Container{
		ID:           id,
		PID:          old.Process.Pid,
		PIDStartTime: startTime,
		Status:       state.StatusRunning,
		RootFS:       rootfs,
		Command:      []string{"/bin/sleep", "30"},
		CreatedAt:    time.Now(),
	}); err != nil {
		st.Close()
		t.Fatal(err)
	}
	if err := st.SaveRestartSpec(id, state.RestartSpec{
		RootFS:  rootfs,
		Command: []string{"/bin/true"},
	}); err != nil {
		st.Close()
		t.Fatal(err)
	}
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}

	runCalled := false
	_, err = restartStoppedContainer(id, restartCommandDeps{
		openStore: func() (*state.Store, error) { return state.Open(stateDir) },
		stat:      os.Stat,
		stop: func(st *state.Store, gotID string, timeout time.Duration) (*state.Container, error) {
			if gotID != id {
				t.Fatalf("stop id = %q, want %q", gotID, id)
			}
			if timeout != restartStopTimeout {
				t.Fatalf("stop timeout = %v, want %v", timeout, restartStopTimeout)
			}
			running, err := st.Get(id)
			if err != nil {
				return nil, err
			}
			stale := *running
			if err := old.Process.Kill(); err != nil {
				return nil, err
			}
			if err := old.Wait(); err == nil {
				t.Fatal("killed process unexpectedly exited successfully")
			}
			running.Status = state.StatusStopped
			running.PID = 0
			running.PIDStartTime = 0
			if err := st.Save(running); err != nil {
				return nil, err
			}
			return &stale, nil
		},
		run: func(cfg container.Config) error {
			runCalled = true
			if cfg.ContainerID != id {
				t.Fatalf("run container id = %q, want %q", cfg.ContainerID, id)
			}
			return nil
		},
	})
	if err != nil {
		t.Fatalf("restart running container with stale stop snapshot: %v", err)
	}
	if !runCalled {
		t.Fatal("runtime was not invoked after durable stopped state was persisted")
	}
}
