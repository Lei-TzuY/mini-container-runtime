package main

import (
	"errors"
	"strings"
	"testing"
	"time"

	"minicontainer/internal/container"
	"minicontainer/internal/state"
)

func TestRunManagedComposeServiceDoesNotRunAfterAdmissionFailure(t *testing.T) {
	cause := errors.New("state unavailable")
	runCalled := false

	settled, err := runManagedComposeServiceWith(container.Config{RootFS: "/rootfs"}, composeRunDeps{
		admit: func(*container.Config) (*state.Store, *state.Container, error) {
			return nil, nil, cause
		},
		run: func(container.Config) error {
			runCalled = true
			return nil
		},
		settle: settleRunCommandState,
		now:    time.Now,
	})
	if !errors.Is(err, cause) {
		t.Fatalf("error=%v, want admission cause", err)
	}
	if runCalled || settled != nil {
		t.Fatalf("runCalled=%v settled=%v after failed admission", runCalled, settled)
	}
}

func TestRunManagedComposeServicePublishesAdmittedIDToRuntimeAndSettlesPreStartFailure(t *testing.T) {
	dir := t.TempDir()
	id := "1111111111111111"
	finishedAt := time.Unix(1_700_000_100, 0)
	var runtimeID string

	settled, err := runManagedComposeServiceWith(container.Config{
		RootFS:   "/rootfs-compose",
		Command:  []string{"/bin/false"},
		Hostname: "web",
	}, composeRunDeps{
		admit: composeTestAdmission(t, dir, id),
		run: func(cfg container.Config) error {
			runtimeID = cfg.ContainerID
			return errors.New("spawn failed before running")
		},
		settle: settleRunCommandState,
		now:    func() time.Time { return finishedAt },
	})
	if err == nil || !strings.Contains(err.Error(), "spawn failed") {
		t.Fatalf("error=%v, want runtime failure", err)
	}
	if runtimeID != id {
		t.Fatalf("runtime ContainerID=%q, want admitted %q", runtimeID, id)
	}
	if settled == nil || settled.ID != id || settled.Status != state.StatusStopped || settled.ExitCode != 1 {
		t.Fatalf("settled=%+v, want stopped startup-failure state", settled)
	}

	reopened, err := state.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	persisted, err := reopened.Get(id)
	if err != nil {
		t.Fatal(err)
	}
	if persisted.Status != state.StatusStopped || persisted.ExitCode != 1 || persisted.FinishedAt == nil || !persisted.FinishedAt.Equal(finishedAt) {
		t.Fatalf("persisted=%+v, want settled startup failure", persisted)
	}
}

func TestRunManagedComposeServiceUsesDurableStoppedStateAsAuthority(t *testing.T) {
	dir := t.TempDir()
	id := "2222222222222222"
	finishedAt := time.Unix(1_700_000_200, 0)
	var admittedStore *state.Store

	admit := composeTestAdmission(t, dir, id)
	settled, err := runManagedComposeServiceWith(container.Config{
		RootFS:  "/rootfs-compose",
		Command: []string{"/bin/sh"},
	}, composeRunDeps{
		admit: func(cfg *container.Config) (*state.Store, *state.Container, error) {
			st, rec, err := admit(cfg)
			admittedStore = st
			return st, rec, err
		},
		run: func(container.Config) error {
			changed, err := admittedStore.MarkStoppedIfCreated(id, 42, finishedAt)
			if err != nil {
				t.Fatalf("MarkStoppedIfCreated: %v", err)
			}
			if !changed {
				t.Fatal("expected created-to-stopped transition")
			}
			return errors.New("payload exited 42")
		},
		settle: settleRunCommandState,
		now:    time.Now,
	})
	if err == nil || !strings.Contains(err.Error(), "payload exited 42") {
		t.Fatalf("error=%v, want payload error preserved", err)
	}
	if settled == nil || settled.ExitCode != 42 || settled.Status != state.StatusStopped {
		t.Fatalf("settled=%+v, want durable exit 42", settled)
	}
}

func TestRunManagedComposeServiceRejectsSuccessfulRuntimeThatNeverLeftCreated(t *testing.T) {
	dir := t.TempDir()
	id := "3333333333333333"

	settled, err := runManagedComposeServiceWith(container.Config{
		RootFS:  "/rootfs-compose",
		Command: []string{"/bin/true"},
	}, composeRunDeps{
		admit:  composeTestAdmission(t, dir, id),
		run:    func(container.Config) error { return nil },
		settle: settleRunCommandState,
		now:    time.Now,
	})
	if err == nil || !strings.Contains(err.Error(), "never left created state") {
		t.Fatalf("error=%v, want lifecycle invariant failure", err)
	}
	if settled == nil || settled.Status != state.StatusCreated {
		t.Fatalf("settled=%+v, want created snapshot on invariant failure", settled)
	}
}

func composeTestAdmission(t *testing.T, dir, id string) func(*container.Config) (*state.Store, *state.Container, error) {
	t.Helper()
	return func(cfg *container.Config) (*state.Store, *state.Container, error) {
		return prepareManagedRunStateWith(cfg, runAdmissionDeps{
			openStore: func() (*state.Store, error) { return state.Open(dir) },
			newID:     func() (string, error) { return id, nil },
			now:       time.Now,
		})
	}
}
