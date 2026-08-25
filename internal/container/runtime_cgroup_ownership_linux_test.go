//go:build linux

package container

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"minicontainer/internal/cgroups"
	"minicontainer/internal/state"
)

func saveOwnershipTestRunning(t *testing.T, st *state.Store, id string, pid int, start uint64) string {
	t.Helper()
	if err := st.Save(&state.Container{
		ID:        id,
		Status:    state.StatusCreated,
		RootFS:    "/tmp/rootfs",
		Command:   []string{"true"},
		CreatedAt: time.Now(),
	}); err != nil {
		t.Fatal(err)
	}
	if err := st.MarkRunning(id, pid, start, time.Now()); err != nil {
		t.Fatal(err)
	}
	name, err := cgroups.NameForContainerProcess(id, pid, start)
	if err != nil {
		t.Fatal(err)
	}
	return name
}

func TestPersistAppliedCgroupOwnershipDurableBeforeRelease(t *testing.T) {
	st, err := state.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	const (
		id    = "ctr-applied-ownership"
		pid   = 4242
		start = uint64(99)
	)
	name := saveOwnershipTestRunning(t, st, id, pid, start)

	// A nil cmd/pipe is intentional: the success path must only persist the
	// ownership token and must not touch the blocked child or cleanup path.
	if err := persistAppliedCgroupOwnership(nil, nil, st, id, pid, start, name, false); err != nil {
		t.Fatalf("persist applied ownership: %v", err)
	}
	ownership, ok, err := st.GetCgroupOwnership(id)
	if err != nil {
		t.Fatal(err)
	}
	if !ok || ownership.Name != name || ownership.PID != pid || ownership.PIDStartTime != start {
		t.Fatalf("ownership=%+v ok=%v", ownership, ok)
	}
}

func TestPersistAppliedCgroupOwnershipFailureStopsGeneration(t *testing.T) {
	dir := t.TempDir()
	st, err := state.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	const (
		id    = "ctr-applied-persist-fail"
		pid   = 5252
		start = uint64(77)
	)
	name := saveOwnershipTestRunning(t, st, id, pid, start)

	// Block the sidecar target with a directory. atomic rename cannot replace a
	// directory, forcing ownership persistence to fail after Apply would have
	// succeeded. The fail-closed path must still reconcile lifecycle state and
	// report a runtime-control failure rather than releasing the payload.
	sidecar := filepath.Join(dir, "containers", id+".cgroup")
	if err := os.Mkdir(sidecar, 0o700); err != nil {
		t.Fatal(err)
	}

	err = persistAppliedCgroupOwnership(nil, nil, st, id, pid, start, name, false)
	if err == nil {
		t.Fatal("ownership persistence failure unexpectedly succeeded")
	}
	if !isRuntimeControlError(err) {
		t.Fatalf("ownership persistence failure not classified runtime-control: %v", err)
	}
	if !strings.Contains(err.Error(), "persist cgroup ownership") {
		t.Fatalf("missing ownership persistence context: %v", err)
	}

	current, getErr := st.Get(id)
	if getErr != nil {
		t.Fatal(getErr)
	}
	if current.Status != state.StatusStopped || current.PID != 0 || current.PIDStartTime != 0 {
		t.Fatalf("generation not stopped after ownership persistence failure: %+v", current)
	}
}
