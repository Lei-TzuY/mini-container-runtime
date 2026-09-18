//go:build linux

package container

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"minicontainer/internal/cgroups"
	"minicontainer/internal/state"
)

func TestCgroupControlsRejectProcessIdentityMismatch(t *testing.T) {
	st, err := state.Open(t.TempDir())
	if err != nil {
		t.Fatalf("Open state store: %v", err)
	}
	startTime, err := ProcessStartTime(os.Getpid())
	if err != nil {
		t.Fatalf("ProcessStartTime: %v", err)
	}
	wrongStartTime := startTime + 1
	if wrongStartTime == 0 {
		wrongStartTime = startTime - 1
	}
	c := &state.Container{
		ID:           "ctr-cgroup-reuse",
		Status:       state.StatusRunning,
		PID:          os.Getpid(),
		PIDStartTime: wrongStartTime,
		CreatedAt:    time.Now(),
	}
	if err := st.Save(c); err != nil {
		t.Fatalf("Save: %v", err)
	}

	for name, fn := range map[string]func() error{
		"freeze": func() error { return FreezeContainer(st, c.ID) },
		"thaw":   func() error { return ThawContainer(st, c.ID) },
		"update": func() error { return UpdateContainerResources(st, c.ID, cgroups.UpdateConfig{}, false) },
	} {
		t.Run(name, func(t *testing.T) {
			if err := fn(); !errors.Is(err, ErrProcessIdentityMismatch) {
				t.Fatalf("error = %v, want ErrProcessIdentityMismatch", err)
			}
		})
	}
}

// TestUpdateContainerResourcesPersistsLiveKernelLimits is privileged acceptance
// evidence for the dynamic-update durability contract. It uses a real child
// process and cgroup v2 hierarchy: the same update must become visible in the
// kernel and in the RestartSpec consumed by the restart path.
func TestUpdateContainerResourcesPersistsLiveKernelLimits(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("requires root for live cgroup mutation")
	}
	if _, err := os.Stat("/sys/fs/cgroup/cgroup.controllers"); err != nil {
		t.Skip("requires cgroup v2")
	}

	cmd := exec.Command("sleep", "30")
	if err := cmd.Start(); err != nil {
		t.Fatalf("start child: %v", err)
	}
	defer func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	}()

	startTime, err := ProcessStartTime(cmd.Process.Pid)
	if err != nil {
		t.Fatalf("child start time: %v", err)
	}
	const id = "live-update-kernel"
	cgName, err := cgroups.NameForContainerProcess(id, cmd.Process.Pid, startTime)
	if err != nil {
		t.Fatalf("derive cgroup name: %v", err)
	}
	if err := cgroups.Apply(cmd.Process.Pid, cgroups.Config{
		Name:      cgName,
		MemoryMax: 256 << 20,
		PidsMax:   64,
	}, false); err != nil {
		t.Fatalf("create live cgroup: %v", err)
	}
	defer cgroups.Remove(cgName, false)

	st, err := state.Open(t.TempDir())
	if err != nil {
		t.Fatalf("open state store: %v", err)
	}
	defer st.Close()
	rootfs := t.TempDir()
	rec := &state.Container{
		ID:           id,
		Status:       state.StatusRunning,
		PID:          cmd.Process.Pid,
		PIDStartTime: startTime,
		RootFS:       rootfs,
		Command:      []string{"/bin/true"},
		CreatedAt:    time.Now(),
	}
	if err := st.Save(rec); err != nil {
		t.Fatalf("save running state: %v", err)
	}
	if err := st.SaveRestartSpec(id, state.RestartSpec{
		RootFS:    rootfs,
		Command:   rec.Command,
		Memory:    256 << 20,
		PidsLimit: 64,
	}); err != nil {
		t.Fatalf("save restart spec: %v", err)
	}

	const wantMemory = int64(192 << 20)
	const wantPids = int64(48)
	if _, err := UpdateContainerResourcesResolved(st, id, cgroups.UpdateConfig{
		MemoryMax: wantMemory,
		PidsMax:   wantPids,
	}, false); err != nil {
		t.Fatalf("live resource update: %v", err)
	}

	readKnob := func(name string) string {
		t.Helper()
		data, err := os.ReadFile(filepath.Join("/sys/fs/cgroup", cgName, name))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		return strings.TrimSpace(string(data))
	}
	if got := readKnob("memory.max"); got != "201326592" {
		t.Fatalf("live memory.max = %q, want %d", got, wantMemory)
	}
	if got := readKnob("pids.max"); got != "48" {
		t.Fatalf("live pids.max = %q, want %d", got, wantPids)
	}

	spec, err := st.RestartSpec(id)
	if err != nil {
		t.Fatalf("reload restart spec: %v", err)
	}
	if spec.Memory != wantMemory || spec.PidsLimit != wantPids {
		t.Fatalf("durable restart limits = memory:%d pids:%d, want memory:%d pids:%d", spec.Memory, spec.PidsLimit, wantMemory, wantPids)
	}
}
