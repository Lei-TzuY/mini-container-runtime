package main

import (
	"errors"
	"testing"
	"time"

	"minicontainer/internal/container"
	"minicontainer/internal/state"
)

const runAdmissionTestID = "0123456789abcdef"

func TestPrepareManagedRunStateFailsClosedWhenStoreOpenFails(t *testing.T) {
	cause := errors.New("state unavailable")
	cfg := runAdmissionTestConfig()
	openCalls := 0

	st, rec, err := prepareManagedRunStateWith(&cfg, runAdmissionDeps{
		openStore: func() (*state.Store, error) {
			openCalls++
			return nil, cause
		},
		newID: func() (string, error) {
			t.Fatal("newID called after state open failure")
			return "", nil
		},
		now: time.Now,
	})
	if !errors.Is(err, cause) {
		t.Fatalf("error=%v, want open cause", err)
	}
	if openCalls != 1 || st != nil || rec != nil {
		t.Fatalf("openCalls=%d store=%v rec=%v, want 1/nil/nil", openCalls, st, rec)
	}
	if cfg.ContainerID != "" {
		t.Fatalf("ContainerID=%q after failed open, want empty", cfg.ContainerID)
	}
}

func TestPrepareManagedRunStateFailsClosedWhenIDGenerationFails(t *testing.T) {
	cause := errors.New("entropy unavailable")
	cfg := runAdmissionTestConfig()
	opened, err := state.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	st, rec, err := prepareManagedRunStateWith(&cfg, runAdmissionDeps{
		openStore: func() (*state.Store, error) { return opened, nil },
		newID:     func() (string, error) { return "", cause },
		now:       time.Now,
	})
	if !errors.Is(err, cause) {
		t.Fatalf("error=%v, want ID cause", err)
	}
	if st != nil || rec != nil || cfg.ContainerID != "" {
		t.Fatalf("store=%v rec=%v ContainerID=%q after failed ID generation", st, rec, cfg.ContainerID)
	}
	if _, err := opened.List(); !errors.Is(err, state.ErrStoreClosed) {
		t.Fatalf("failed admission did not close store: %v", err)
	}
}

func TestPrepareManagedRunStateFailsClosedWhenCreatedStateSaveFails(t *testing.T) {
	cfg := runAdmissionTestConfig()
	closed, err := state.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := closed.Close(); err != nil {
		t.Fatal(err)
	}

	st, rec, err := prepareManagedRunStateWith(&cfg, runAdmissionDeps{
		openStore: func() (*state.Store, error) { return closed, nil },
		newID:     func() (string, error) { return runAdmissionTestID, nil },
		now:       time.Now,
	})
	if !errors.Is(err, state.ErrStoreClosed) {
		t.Fatalf("error=%v, want closed-store save failure", err)
	}
	if st != nil || rec != nil || cfg.ContainerID != "" {
		t.Fatalf("store=%v rec=%v ContainerID=%q after failed Save", st, rec, cfg.ContainerID)
	}
}

func TestPrepareManagedRunStatePublishesIDOnlyAfterDurableSave(t *testing.T) {
	cfg := runAdmissionTestConfig()
	createdAt := time.Unix(1_700_000_000, 123)
	opened, err := state.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer opened.Close()

	st, rec, err := prepareManagedRunStateWith(&cfg, runAdmissionDeps{
		openStore: func() (*state.Store, error) { return opened, nil },
		newID:     func() (string, error) { return runAdmissionTestID, nil },
		now:       func() time.Time { return createdAt },
	})
	if err != nil {
		t.Fatalf("prepareManagedRunStateWith: %v", err)
	}
	if st != opened || rec == nil {
		t.Fatalf("store=%v rec=%v, want opened store and record", st, rec)
	}
	if cfg.ContainerID != runAdmissionTestID || rec.ID != runAdmissionTestID {
		t.Fatalf("cfg ID=%q rec ID=%q, want %q", cfg.ContainerID, rec.ID, runAdmissionTestID)
	}
	if rec.Revision != 1 || rec.Status != state.StatusCreated || !rec.CreatedAt.Equal(createdAt) {
		t.Fatalf("record=%+v, want durable revision-1 created state", rec)
	}

	persisted, err := opened.Get(runAdmissionTestID)
	if err != nil {
		t.Fatalf("Get persisted record: %v", err)
	}
	if persisted.Revision != 1 || persisted.Status != state.StatusCreated || persisted.RootFS != cfg.RootFS || persisted.Hostname != cfg.Hostname {
		t.Fatalf("persisted=%+v, want admitted config", persisted)
	}
	if len(persisted.Command) != len(cfg.Command) || persisted.Command[0] != cfg.Command[0] {
		t.Fatalf("persisted command=%v, want %v", persisted.Command, cfg.Command)
	}
}

func TestPrepareManagedRunStateRejectsPreassignedContainerID(t *testing.T) {
	cfg := runAdmissionTestConfig()
	cfg.ContainerID = "already-assigned"
	opened := false

	st, rec, err := prepareManagedRunStateWith(&cfg, runAdmissionDeps{
		openStore: func() (*state.Store, error) {
			opened = true
			return nil, nil
		},
		newID: func() (string, error) { return runAdmissionTestID, nil },
		now:   time.Now,
	})
	if err == nil {
		t.Fatal("preassigned ContainerID was accepted")
	}
	if opened || st != nil || rec != nil || cfg.ContainerID != "already-assigned" {
		t.Fatalf("opened=%v store=%v rec=%v ContainerID=%q", opened, st, rec, cfg.ContainerID)
	}
}

func runAdmissionTestConfig() container.Config {
	return container.Config{
		RootFS:   "/rootfs",
		Command:  []string{"/bin/true"},
		Hostname: "minicontainer",
	}
}
