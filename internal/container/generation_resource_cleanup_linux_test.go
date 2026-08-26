//go:build linux

package container

import (
	"testing"
	"time"

	"minicontainer/internal/state"
)

func persistRuntimeGenerationOwnership(t *testing.T, st *state.Store, id string, pid int, start uint64) {
	t.Helper()
	snapshot := &state.Container{
		ID:           id,
		Status:       state.StatusRunning,
		PID:          pid,
		PIDStartTime: start,
		RootFS:       "/tmp/rootfs",
		Command:      []string{"true"},
		CreatedAt:    time.Now(),
	}
	persistOwnedGeneration(t, st, snapshot)
	ownership := networkOwnershipForGeneration(
		"minicontainer:generation-cleanup-test",
		pid,
		start,
		"172.20.0.2",
		[]PortMapping{{HostPort: 18080, ContainerPort: 80}},
	)
	if err := st.MarkNetworkOwnedIfIdentity(id, ownership); err != nil {
		t.Fatal(err)
	}
	if _, err := st.MarkStoppedIfIdentity(id, pid, start, -1, time.Now()); err != nil {
		t.Fatal(err)
	}
}

func TestCleanupRuntimeGenerationResourcesSkipsNewerOwnership(t *testing.T) {
	st, err := state.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	const id = "ctr-stale-generation-cleanup"
	const newPID = 2222
	const newStart = 20
	persistRuntimeGenerationOwnership(t, st, id, newPID, newStart)

	cgroupCalls := 0
	portCalls := 0
	vethCalls := 0
	err = cleanupRuntimeGenerationResourcesWith(
		st,
		id,
		1111,
		10,
		func(string, int, uint64) error { cgroupCalls++; return nil },
		func(string, int, int, string, string, bool) error { portCalls++; return nil },
		func(string, string, bool) error { vethCalls++; return nil },
	)
	if err != nil {
		t.Fatalf("stale generation cleanup: %v", err)
	}
	if cgroupCalls != 0 || portCalls != 0 || vethCalls != 0 {
		t.Fatalf("stale actor touched newer resources: cgroup=%d port=%d veth=%d", cgroupCalls, portCalls, vethCalls)
	}
	if ownership, ok, err := st.GetCgroupOwnership(id); err != nil || !ok || ownership.PID != newPID || ownership.PIDStartTime != newStart {
		t.Fatalf("newer cgroup ownership changed: ownership=%+v ok=%v err=%v", ownership, ok, err)
	}
	if ownership, ok, err := st.GetNetworkOwnership(id); err != nil || !ok || ownership.PID != newPID || ownership.PIDStartTime != newStart {
		t.Fatalf("newer network ownership changed: ownership=%+v ok=%v err=%v", ownership, ok, err)
	}
}

func TestCleanupRuntimeGenerationResourcesConsumesMatchingOwnership(t *testing.T) {
	st, err := state.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	const id = "ctr-matching-generation-cleanup"
	const pid = 3333
	const start = 30
	persistRuntimeGenerationOwnership(t, st, id, pid, start)

	cgroupCalls := 0
	portCalls := 0
	vethCalls := 0
	err = cleanupRuntimeGenerationResourcesWith(
		st,
		id,
		pid,
		start,
		func(gotID string, gotPID int, gotStart uint64) error {
			cgroupCalls++
			if gotID != id || gotPID != pid || gotStart != start {
				t.Fatalf("wrong cgroup generation: %s %d/%d", gotID, gotPID, gotStart)
			}
			return nil
		},
		func(string, int, int, string, string, bool) error { portCalls++; return nil },
		func(string, string, bool) error { vethCalls++; return nil },
	)
	if err != nil {
		t.Fatalf("matching generation cleanup: %v", err)
	}
	if cgroupCalls != 1 || portCalls != 1 || vethCalls != 1 {
		t.Fatalf("matching cleanup calls: cgroup=%d port=%d veth=%d", cgroupCalls, portCalls, vethCalls)
	}
	if _, ok, err := st.GetCgroupOwnership(id); err != nil || ok {
		t.Fatalf("matching cgroup ownership remains: ok=%v err=%v", ok, err)
	}
	if _, ok, err := st.GetNetworkOwnership(id); err != nil || ok {
		t.Fatalf("matching network ownership remains: ok=%v err=%v", ok, err)
	}
}
