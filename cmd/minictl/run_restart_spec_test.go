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
		RootFS:          rootfs,
		Command:         []string{"/bin/app", "serve"},
		Env:             []string{"A=B"},
		WorkDir:         "/work",
		Hostname:        "restart-host",
		Overlay:         true,
		ReadOnly:        true,
		Restart:         "on-failure:3",
		CapDrop:         []string{"CAP_SYS_ADMIN"},
		NoNewPrivileges: true,
		Memory:          64 << 20,
		CPUWeight:       250,
		CPUs:            1.5,
		PidsLimit:       32,
		Seccomp:         true,
		BridgeNetwork:   true,
		PortMappings: []container.PortMapping{{
			HostPort: 8080, ContainerPort: 80, Protocol: "tcp",
		}},
		Volumes: []container.Volume{{
			HostPath: "/host/data", ContainerPath: "/data", ReadOnly: true,
		}},
		UserNS: false,
		Debug:  true,
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
		RootFS:          cfg.RootFS,
		Command:         append([]string(nil), cfg.Command...),
		Env:             append([]string(nil), cfg.Env...),
		WorkDir:         cfg.WorkDir,
		Hostname:        cfg.Hostname,
		Overlay:         cfg.Overlay,
		ReadOnly:        cfg.ReadOnly,
		Restart:         cfg.Restart,
		CapDrop:         append([]string(nil), cfg.CapDrop...),
		NoNewPrivileges: cfg.NoNewPrivileges,
		Memory:          cfg.Memory,
		CPUWeight:       cfg.CPUWeight,
		CPUs:            cfg.CPUs,
		PidsLimit:       cfg.PidsLimit,
		Seccomp:         cfg.Seccomp,
		BridgeNetwork:   cfg.BridgeNetwork,
		PortMappings: []state.RestartPortMapping{{
			HostPort: 8080, ContainerPort: 80, Protocol: "tcp",
		}},
		Volumes: []state.RestartVolume{{
			HostPath: "/host/data", ContainerPath: "/data", ReadOnly: true,
		}},
		UserNS: cfg.UserNS,
		Debug:  cfg.Debug,
	}
	if !reflect.DeepEqual(spec, want) {
		t.Fatalf("RestartSpec=%#v, want %#v", spec, want)
	}
}
