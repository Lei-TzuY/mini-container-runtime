package state

import (
	"errors"
	"testing"
	"time"
)

func TestUpdateRunningRestartSpecPersistsResourcePolicy(t *testing.T) {
	st, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer st.Close()

	rec := &Container{
		ID:           "live-update",
		Status:       StatusRunning,
		PID:          1234,
		PIDStartTime: 5678,
		RootFS:       "/rootfs",
		Command:      []string{"/bin/app"},
		CreatedAt:    time.Now(),
	}
	if err := st.Save(rec); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if err := st.SaveRestartSpec(rec.ID, RestartSpec{
		RootFS:    rec.RootFS,
		Command:   rec.Command,
		Memory:    64 << 20,
		CPUs:      1,
		CPUWeight: 100,
		PidsLimit: 64,
	}); err != nil {
		t.Fatalf("SaveRestartSpec: %v", err)
	}

	if err := st.UpdateRunningRestartSpec(rec.ID, rec.PID, rec.PIDStartTime, func(spec *RestartSpec) error {
		spec.Memory = 128 << 20
		spec.CPUs = 2.5
		spec.CPUWeight = 250
		spec.PidsLimit = 128
		return nil
	}); err != nil {
		t.Fatalf("UpdateRunningRestartSpec: %v", err)
	}

	got, err := st.RestartSpec(rec.ID)
	if err != nil {
		t.Fatalf("RestartSpec: %v", err)
	}
	if got.Memory != 128<<20 || got.CPUs != 2.5 || got.CPUWeight != 250 || got.PidsLimit != 128 {
		t.Fatalf("updated resources not durable: %#v", got)
	}
}

func TestUpdateRunningRestartSpecRejectsChangedGenerationBeforeCallback(t *testing.T) {
	st, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer st.Close()

	rec := &Container{ID: "generation-guard", Status: StatusRunning, PID: 1234, PIDStartTime: 5678, RootFS: "/rootfs", Command: []string{"/bin/app"}, CreatedAt: time.Now()}
	if err := st.Save(rec); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if err := st.SaveRestartSpec(rec.ID, RestartSpec{RootFS: rec.RootFS, Command: rec.Command, Memory: 64 << 20}); err != nil {
		t.Fatalf("SaveRestartSpec: %v", err)
	}

	called := false
	err = st.UpdateRunningRestartSpec(rec.ID, rec.PID, rec.PIDStartTime+1, func(spec *RestartSpec) error {
		called = true
		return errors.New("must not run")
	})
	if err == nil {
		t.Fatal("expected generation mismatch")
	}
	if called {
		t.Fatal("callback ran for stale generation")
	}
	got, err := st.RestartSpec(rec.ID)
	if err != nil {
		t.Fatalf("RestartSpec: %v", err)
	}
	if got.Memory != 64<<20 {
		t.Fatalf("stale generation mutated restart policy: %d", got.Memory)
	}
}
