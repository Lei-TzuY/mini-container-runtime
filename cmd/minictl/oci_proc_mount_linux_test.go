//go:build linux

package main

import (
	"os/exec"
	"strings"
	"testing"
	"time"

	"minicontainer/internal/container"
	"minicontainer/internal/state"
)

func TestOCIBundleProcMountReachesManagedRun(t *testing.T) {
	bundle := writeOCIBundle(t, `{"ociVersion":"1.1.0","root":{"path":"rootfs"},"process":{"args":["/bin/sh","-c","test -r /proc/self/stat"],"cwd":"/"},"mounts":[{"destination":"/proc","type":"proc","source":"proc","options":["rw","nosuid","noexec","nodev"]}],"linux":{"namespaces":[{"type":"pid"},{"type":"mount"}]}}`)
	stateDir := t.TempDir()
	var ran bool
	_, err := runOCIBundleWith(bundle, ociBundleRunDeps{
		load: loadOCIBundle,
		prepare: func(cfg *container.Config) (*state.Store, *state.Container, error) {
			return prepareManagedRunStateWith(cfg, runAdmissionDeps{
				openStore: func() (*state.Store, error) { return state.Open(stateDir) },
				newID: func() (string, error) { return "oci-proc-process", nil },
				now: func() time.Time { return time.Unix(10, 0) },
			})
		},
		run: func(cfg container.Config) error {
			ran = true
			var policy string
			for _, entry := range cfg.Env {
				if strings.HasPrefix(entry, container.RuntimeProcMountOptionsEnvKey+"=") {
					policy = entry
					break
				}
			}
			if policy != container.RuntimeProcMountOptionsEnvKey+"=rw,nosuid,noexec,nodev" {
				t.Fatalf("proc mount policy env=%q", policy)
			}
			return exec.Command("/bin/sh", "-c", "test -r /proc/self/stat").Run()
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
		t.Fatalf("run OCI bundle with proc mount: %v", err)
	}
	if !ran {
		t.Fatal("managed runner was not reached")
	}

	st, err := state.Open(stateDir)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	spec, err := st.RestartSpec("oci-proc-process")
	if err != nil {
		t.Fatal(err)
	}
	var persisted bool
	for _, entry := range spec.Env {
		if entry == container.RuntimeProcMountOptionsEnvKey+"=rw,nosuid,noexec,nodev" {
			persisted = true
			break
		}
	}
	if !persisted {
		t.Fatalf("restart spec did not preserve proc mount policy: %v", spec.Env)
	}
}

func TestOCIProcMountRejectsUnenforcedOptions(t *testing.T) {
	bundle := writeOCIBundle(t, `{"ociVersion":"1.1.0","root":{"path":"rootfs"},"process":{"args":["/bin/true"],"cwd":"/"},"mounts":[{"destination":"/proc","type":"proc","source":"proc","options":["relatime"]}]}`)
	if _, err := loadOCIBundle(bundle); err == nil {
		t.Fatal("expected proc mount with unsupported option to fail closed")
	}
}

func TestOCIProcMountRejectsReservedPolicyEnvironment(t *testing.T) {
	bundle := writeOCIBundle(t, `{"ociVersion":"1.1.0","root":{"path":"rootfs"},"process":{"args":["/bin/true"],"cwd":"/","env":["MINICONTAINER_INTERNAL_PROC_MOUNT_OPTIONS=nodev"]}}`)
	if _, err := loadOCIBundle(bundle); err == nil {
		t.Fatal("expected reserved proc policy environment to fail closed")
	}
}
