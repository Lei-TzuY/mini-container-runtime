//go:build linux

package main

import (
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"minicontainer/internal/container"
	"minicontainer/internal/state"
)

func TestOCIBundleRunConnectsAdmissionResourcesAndRealProcess(t *testing.T) {
	stateDir := t.TempDir()
	bundle := writeOCIBundle(t, `{"ociVersion":"1.1.0","root":{"path":"rootfs"},"process":{"args":["/bin/sh","-c","printf oci-process"],"env":["A=1"],"cwd":"/"},"hostname":"oci-test","mounts":[{"destination":"/data","type":"bind","source":"/srv/oci-data","options":["rbind","ro"]}],"linux":{"namespaces":[{"type":"pid"},{"type":"mount"},{"type":"user"}],"resources":{"memory":{"limit":67108864},"cpu":{"shares":1024,"quota":50000,"period":100000},"pids":{"limit":32}}}}`)

	var ran bool
	id, err := runOCIBundleWith(bundle, ociBundleRunDeps{
		load: loadOCIBundle,
		prepare: func(cfg *container.Config) (*state.Store, *state.Container, error) {
			return prepareManagedRunStateWith(cfg, runAdmissionDeps{
				openStore: func() (*state.Store, error) { return state.Open(stateDir) },
				newID:     func() (string, error) { return "oci-managed-process", nil },
				now:       func() time.Time { return time.Unix(10, 0) },
			})
		},
		run: func(cfg container.Config) error {
			ran = true
			if cfg.RootFS != filepath.Join(bundle, "rootfs") || cfg.ContainerID != "oci-managed-process" {
				t.Fatalf("runner received unadmitted config: %+v", cfg)
			}
			wantWeight := int64(1 + (uint64(1024)-2)*9999/262142)
			if cfg.Memory != 67108864 || cfg.PidsLimit != 32 || cfg.CPUs != 0.5 || cfg.CPUWeight != wantWeight {
				t.Fatalf("runner lost OCI resource policy: %+v", cfg)
			}
			if len(cfg.Volumes) != 1 || cfg.Volumes[0].HostPath != "/srv/oci-data" || cfg.Volumes[0].ContainerPath != "/data" || !cfg.Volumes[0].ReadOnly {
				t.Fatalf("runner lost OCI bind mount policy: %+v", cfg.Volumes)
			}
			out, err := exec.Command("/bin/sh", "-c", "printf oci-process").Output()
			if err != nil {
				return err
			}
			if string(out) != "oci-process" {
				t.Fatalf("process output = %q", out)
			}
			return nil
		},
		settle: func(st *state.Store, id string, runErr error, _ time.Time) (*state.Container, error) {
			if runErr != nil {
				return nil, runErr
			}
			return st.Resolve(id)
		},
		now: func() time.Time { return time.Unix(20, 0) },
	})
	if err != nil {
		t.Fatalf("runOCIBundleWith: %v", err)
	}
	if id != "oci-managed-process" || !ran {
		t.Fatalf("id=%q ran=%v", id, ran)
	}

	st, err := state.Open(stateDir)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	rec, err := st.Resolve(id)
	if err != nil {
		t.Fatal(err)
	}
	if rec.RootFS != filepath.Join(bundle, "rootfs") || rec.Hostname != "oci-test" {
		t.Fatalf("unexpected persisted OCI state: %+v", rec)
	}
	spec, err := st.RestartSpec(id)
	if err != nil {
		t.Fatal(err)
	}
	if spec.Memory != 67108864 || spec.PidsLimit != 32 || spec.CPUs != 0.5 {
		t.Fatalf("persisted restart spec lost OCI resources: %+v", spec)
	}
	if len(spec.Volumes) != 1 || spec.Volumes[0].HostPath != "/srv/oci-data" || spec.Volumes[0].ContainerPath != "/data" || !spec.Volumes[0].ReadOnly {
		t.Fatalf("persisted restart spec lost OCI bind mount: %+v", spec.Volumes)
	}
}
