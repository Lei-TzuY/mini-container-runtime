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

func TestRestartStoppedContainerRelaunchesPersistedSpecWithRealProcess(t *testing.T) {
	stateDir := t.TempDir()
	rootfs := t.TempDir()
	marker := filepath.Join(t.TempDir(), "restarted")
	id := "restart-real-process"
	command := []string{"/bin/sh", "-c", "printf restarted > \"$1\"", "sh", marker}

	st, err := state.Open(stateDir)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Save(&state.Container{
		ID:        id,
		Status:    state.StatusStopped,
		RootFS:    rootfs,
		Command:   append([]string(nil), command...),
		Hostname:  "restart-host",
		CreatedAt: time.Now(),
	}); err != nil {
		st.Close()
		t.Fatal(err)
	}
	wantSpec := state.RestartSpec{
		RootFS:          rootfs,
		Command:         append([]string(nil), command...),
		Env:             []string{"RESTART_TEST=1"},
		WorkDir:         "/work",
		Hostname:        "restart-host",
		Overlay:         true,
		ReadOnly:        true,
		Restart:         "on-failure:2",
		CapDrop:         []string{"CAP_SYS_ADMIN"},
		NoNewPrivileges: true,
		Memory:          32 << 20,
		CPUWeight:       400,
		CPUs:            0.5,
		PidsLimit:       16,
		Seccomp:         true,
		BridgeNetwork:   true,
		PortMappings: []state.RestartPortMapping{{
			HostPort: 18080, ContainerPort: 8080, Protocol: "tcp",
		}},
		Volumes: []state.RestartVolume{{
			HostPath: "/host/data", ContainerPath: "/data", ReadOnly: true,
		}},
		UserNS: false,
		Debug:  true,
	}
	if err := st.SaveRestartSpec(id, wantSpec); err != nil {
		st.Close()
		t.Fatal(err)
	}
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}

	var gotCfg container.Config
	rec, err := restartStoppedContainer(id, restartCommandDeps{
		openStore: func() (*state.Store, error) { return state.Open(stateDir) },
		stat:      os.Stat,
		run: func(cfg container.Config) error {
			gotCfg = cfg
			cmd := exec.Command(cfg.Command[0], cfg.Command[1:]...)
			cmd.Env = append(os.Environ(), cfg.Env...)
			return cmd.Run()
		},
	})
	if err != nil {
		t.Fatalf("restartStoppedContainer: %v", err)
	}
	if rec.ID != id || rec.Status != state.StatusStopped {
		t.Fatalf("reloaded state = %#v", rec)
	}
	if gotCfg.ContainerID != id || gotCfg.StateDir != stateDir || gotCfg.RootFS != rootfs {
		t.Fatalf("restart config identity = %#v", gotCfg)
	}
	if !reflect.DeepEqual(gotCfg.Command, wantSpec.Command) || !reflect.DeepEqual(gotCfg.Env, wantSpec.Env) || !reflect.DeepEqual(gotCfg.CapDrop, wantSpec.CapDrop) {
		t.Fatalf("restart config command/env/capabilities = %#v", gotCfg)
	}
	if gotCfg.WorkDir != wantSpec.WorkDir || gotCfg.Hostname != wantSpec.Hostname || gotCfg.UserNS != wantSpec.UserNS {
		t.Fatalf("restart config execution metadata = %#v", gotCfg)
	}
	if gotCfg.Overlay != wantSpec.Overlay || gotCfg.ReadOnly != wantSpec.ReadOnly || gotCfg.Restart != wantSpec.Restart || gotCfg.Seccomp != wantSpec.Seccomp || gotCfg.NoNewPrivileges != wantSpec.NoNewPrivileges || gotCfg.BridgeNetwork != wantSpec.BridgeNetwork || gotCfg.Debug != wantSpec.Debug {
		t.Fatalf("restart config isolation policy = %#v", gotCfg)
	}
	if gotCfg.Memory != wantSpec.Memory || gotCfg.CPUWeight != wantSpec.CPUWeight || gotCfg.CPUs != wantSpec.CPUs || gotCfg.PidsLimit != wantSpec.PidsLimit {
		t.Fatalf("restart config resource policy = %#v", gotCfg)
	}
	wantPorts := []container.PortMapping{{HostPort: 18080, ContainerPort: 8080, Protocol: "tcp"}}
	wantVolumes := []container.Volume{{HostPath: "/host/data", ContainerPath: "/data", ReadOnly: true}}
	if !reflect.DeepEqual(gotCfg.PortMappings, wantPorts) || !reflect.DeepEqual(gotCfg.Volumes, wantVolumes) {
		t.Fatalf("restart config network/mount policy = %#v", gotCfg)
	}
	if gotCfg.RootFSIdentity == nil || !gotCfg.RootFSIdentity.IsDir() {
		t.Fatalf("restart rootfs identity = %#v", gotCfg.RootFSIdentity)
	}
	data, err := os.ReadFile(marker)
	if err != nil {
		t.Fatalf("read real-process marker: %v", err)
	}
	if string(data) != "restarted" {
		t.Fatalf("marker = %q, want restarted", data)
	}
}

func TestRestartStoppedContainerRejectsNonStoppedContainer(t *testing.T) {
	stateDir := t.TempDir()
	st, err := state.Open(stateDir)
	if err != nil {
		t.Fatal(err)
	}
	id := "restart-created"
	if err := st.Save(&state.Container{
		ID:        id,
		Status:    state.StatusCreated,
		RootFS:    t.TempDir(),
		Command:   []string{"/bin/true"},
		CreatedAt: time.Now(),
	}); err != nil {
		st.Close()
		t.Fatal(err)
	}
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}

	called := false
	_, err = restartStoppedContainer(id, restartCommandDeps{
		openStore: func() (*state.Store, error) { return state.Open(stateDir) },
		stat:      os.Stat,
		run: func(container.Config) error {
			called = true
			return nil
		},
	})
	if err == nil {
		t.Fatal("expected non-stopped-container rejection")
	}
	if called {
		t.Fatal("runtime invoked for non-stopped container")
	}
}
