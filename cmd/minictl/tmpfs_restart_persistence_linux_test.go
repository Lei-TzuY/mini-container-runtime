//go:build linux

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"minicontainer/internal/container"
	"minicontainer/internal/state"
)

func TestManagedRunPersistsTmpfsMountsForRestart(t *testing.T) {
	stateDir := t.TempDir()
	rootfs := t.TempDir()
	cfg := &container.Config{
		RootFS:  rootfs,
		Command: []string{"/bin/true"},
		TmpfsMounts: []container.TmpfsMount{{
			ContainerPath: "/run/cache",
			Options:       []string{"nosuid", "nodev", "mode=0755", "size=64k"},
		}},
	}

	st, rec, err := prepareManagedRunStateWith(cfg, runAdmissionDeps{
		openStore: func() (*state.Store, error) { return state.Open(stateDir) },
		newID:     func() (string, error) { return "tmpfs-persist", nil },
		now:       time.Now,
	})
	if err != nil {
		t.Fatalf("prepareManagedRunStateWith: %v", err)
	}
	defer st.Close()

	spec, err := st.RestartSpec(rec.ID)
	if err != nil {
		t.Fatalf("RestartSpec: %v", err)
	}
	want := []state.RestartTmpfsMount{{
		ContainerPath: "/run/cache",
		Options:       []string{"nosuid", "nodev", "mode=0755", "size=64k"},
	}}
	if !reflect.DeepEqual(spec.TmpfsMounts, want) {
		t.Fatalf("persisted tmpfs mounts = %#v, want %#v", spec.TmpfsMounts, want)
	}

	cfg.TmpfsMounts[0].Options[0] = "ro"
	if spec.TmpfsMounts[0].Options[0] != "nosuid" {
		t.Fatalf("persisted tmpfs options alias caller memory: %#v", spec.TmpfsMounts)
	}
}

func TestRestartStoppedContainerRestoresTmpfsMountsWithRealProcess(t *testing.T) {
	stateDir := t.TempDir()
	rootfs := t.TempDir()
	marker := filepath.Join(t.TempDir(), "restarted")
	id := "tmpfs-restart-real-process"
	command := []string{"/bin/sh", "-c", "printf restarted > \"$1\"", "sh", marker}
	wantTmpfs := []state.RestartTmpfsMount{{
		ContainerPath: "/run/cache",
		Options:       []string{"nosuid", "nodev", "mode=0755", "size=64k"},
	}}

	st, err := state.Open(stateDir)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Save(&state.Container{
		ID:        id,
		Status:    state.StatusStopped,
		RootFS:    rootfs,
		Command:   append([]string(nil), command...),
		CreatedAt: time.Now(),
	}); err != nil {
		st.Close()
		t.Fatal(err)
	}
	if err := st.SaveRestartSpec(id, state.RestartSpec{
		RootFS:      rootfs,
		Command:     append([]string(nil), command...),
		TmpfsMounts: wantTmpfs,
	}); err != nil {
		st.Close()
		t.Fatal(err)
	}
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}

	var gotTmpfs []container.TmpfsMount
	_, err = restartStoppedContainer(id, restartCommandDeps{
		openStore: func() (*state.Store, error) { return state.Open(stateDir) },
		stat:      os.Stat,
		run: func(cfg container.Config) error {
			gotTmpfs = append([]container.TmpfsMount(nil), cfg.TmpfsMounts...)
			cmd := exec.Command(cfg.Command[0], cfg.Command[1:]...)
			return cmd.Run()
		},
	})
	if err != nil {
		t.Fatalf("restartStoppedContainer: %v", err)
	}
	wantRuntimeTmpfs := []container.TmpfsMount{{
		ContainerPath: "/run/cache",
		Options:       []string{"nosuid", "nodev", "mode=0755", "size=64k"},
	}}
	if !reflect.DeepEqual(gotTmpfs, wantRuntimeTmpfs) {
		t.Fatalf("restart tmpfs mounts = %#v, want %#v", gotTmpfs, wantRuntimeTmpfs)
	}
	data, err := os.ReadFile(marker)
	if err != nil {
		t.Fatalf("read real-process marker: %v", err)
	}
	if string(data) != "restarted" {
		t.Fatalf("marker = %q, want restarted", data)
	}
}
