//go:build linux

package container

import (
	"testing"
	"time"

	"minicontainer/internal/cgroups"
	"minicontainer/internal/state"
)

func TestApplyCgroupUsesDurableMemorySwapPolicy(t *testing.T) {
	st, err := state.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	const (
		id         = "ctr-cgroup-memory-swap"
		pid        = 4545
		start      = uint64(103)
		memoryMax  = int64(128 * 1024 * 1024)
		memorySwap = int64(256 * 1024 * 1024)
	)
	if err := st.Save(&state.Container{
		ID: id, Status: state.StatusCreated, RootFS: "/tmp/rootfs",
		Command: []string{"true"}, CreatedAt: time.Now(),
	}); err != nil {
		t.Fatal(err)
	}
	if err := st.SaveRestartSpec(id, state.RestartSpec{
		RootFS: "/tmp/rootfs", Command: []string{"true"}, Memory: memoryMax, MemorySwap: memorySwap,
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

	applied, err := applyCgroupWithDurableOwnership(st, id, pid, start, cgroups.Config{Name: name, MemoryMax: memoryMax}, false, func(gotPID int, gotCfg cgroups.Config, _ bool) error {
		if gotPID != pid {
			t.Fatalf("apply pid=%d want %d", gotPID, pid)
		}
		if gotCfg.MemoryMax != memoryMax {
			t.Fatalf("memory.max changed before cgroup apply: got %d want %d", gotCfg.MemoryMax, memoryMax)
		}
		if gotCfg.MemorySwap != memorySwap {
			t.Fatalf("durable memory swap policy lost before cgroup apply: got %d want %d", gotCfg.MemorySwap, memorySwap)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if !applied {
		t.Fatal("durable memory swap cgroup policy was not applied")
	}
}
