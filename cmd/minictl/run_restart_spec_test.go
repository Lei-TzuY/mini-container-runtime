package main

import (
	"reflect"
	"testing"
	"time"

	"minicontainer/internal/container"
	"minicontainer/internal/state"
)

func TestPrepareManagedRunStatePersistsResolvedRestartSpec(t *testing.T) {
	rootfs := t.TempDir()
	stateDir := t.TempDir()
	cfg := container.Config{
		RootFS:   rootfs,
		Command:  []string{"/bin/app", "serve"},
		Env:      []string{"A=B"},
		WorkDir:  "/work",
		Hostname: "restart-host",
	}

	st, rec, err := prepareManagedRunStateWith(&cfg, runAdmissionDeps{
		openStore: func() (*state.Store, error) { return state.Open(stateDir) },
		newID:     func() (string, error) { return "restart-admission", nil },
		now:       func() time.Time { return time.Unix(1_700_000_000, 0) },
	})
	if err != nil {
		t.Fatalf("prepareManagedRunStateWith: %v", err)
	}
	defer st.Close()

	spec, err := st.RestartSpec(rec.ID)
	if err != nil {
		t.Fatalf("RestartSpec: %v", err)
	}
	want := state.RestartSpec{
		RootFS:   cfg.RootFS,
		Command:  append([]string(nil), cfg.Command...),
		Env:      append([]string(nil), cfg.Env...),
		WorkDir:  cfg.WorkDir,
		Hostname: cfg.Hostname,
	}
	if !reflect.DeepEqual(spec, want) {
		t.Fatalf("RestartSpec=%#v, want %#v", spec, want)
	}
}
