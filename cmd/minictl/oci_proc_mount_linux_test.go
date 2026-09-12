//go:build linux

package main

import (
	"os/exec"
	"testing"
	"time"

	"minicontainer/internal/container"
	"minicontainer/internal/state"
)

func TestOCIBundleProcMountReachesManagedRun(t *testing.T) {
	bundle := writeOCIBundle(t, `{"ociVersion":"1.1.0","root":{"path":"rootfs"},"process":{"args":["/bin/sh","-c","test -r /proc/self/stat"],"cwd":"/"},"mounts":[{"destination":"/proc","type":"proc","source":"proc"}],"linux":{"namespaces":[{"type":"pid"},{"type":"mount"}]}}`)
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
}

func TestOCIProcMountRejectsUnenforcedOptions(t *testing.T) {
	bundle := writeOCIBundle(t, `{"ociVersion":"1.1.0","root":{"path":"rootfs"},"process":{"args":["/bin/true"],"cwd":"/"},"mounts":[{"destination":"/proc","type":"proc","source":"proc","options":["nosuid"]}]}`)
	if _, err := loadOCIBundle(bundle); err == nil {
		t.Fatal("expected proc mount with unenforced option to fail closed")
	}
}
