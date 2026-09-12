//go:build linux

package main

import (
	"os/exec"
	"testing"
	"time"

	"minicontainer/internal/container"
	"minicontainer/internal/state"
)

func TestOCIBundleSysfsMountReachesManagedRun(t *testing.T) {
	bundle := writeOCIBundle(t, `{"ociVersion":"1.1.0","root":{"path":"rootfs"},"process":{"args":["/bin/sh","-c","test -r /sys/kernel"],"cwd":"/"},"mounts":[{"destination":"/sys","type":"sysfs","source":"sysfs","options":["nosuid","noexec","nodev","ro"]}],"linux":{"namespaces":[{"type":"pid"},{"type":"mount"}]}}`)
	stateDir := t.TempDir()
	var ran bool
	_, err := runOCIBundleWith(bundle, ociBundleRunDeps{
		load: loadOCIBundle,
		prepare: func(cfg *container.Config) (*state.Store, *state.Container, error) {
			return prepareManagedRunStateWith(cfg, runAdmissionDeps{
				openStore: func() (*state.Store, error) { return state.Open(stateDir) },
				newID:     func() (string, error) { return "oci-sysfs-process", nil },
				now:       func() time.Time { return time.Unix(10, 0) },
			})
		},
		run: func(cfg container.Config) error {
			ran = true
			if cfg.ContainerID != "oci-sysfs-process" {
				t.Fatalf("runner received container id %q", cfg.ContainerID)
			}
			return exec.Command("/bin/sh", "-c", "test -r /sys/kernel").Run()
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
		t.Fatalf("run OCI bundle with sysfs mount: %v", err)
	}
	if !ran {
		t.Fatal("managed runner was not reached")
	}
}

func TestOCISysfsMountRejectsNonRepresentableSemantics(t *testing.T) {
	cases := []string{
		`{"ociVersion":"1.1.0","root":{"path":"rootfs"},"process":{"args":["/bin/true"],"cwd":"/"},"mounts":[{"destination":"/sys","type":"sysfs","source":"sysfs","options":["rw","nosuid","noexec","nodev"]}]}`,
		`{"ociVersion":"1.1.0","root":{"path":"rootfs"},"process":{"args":["/bin/true"],"cwd":"/"},"mounts":[{"destination":"/sys","type":"sysfs","source":"none","options":["ro","nosuid","noexec","nodev"]}]}`,
		`{"ociVersion":"1.1.0","root":{"path":"rootfs"},"process":{"args":["/bin/true"],"cwd":"/"},"mounts":[{"destination":"/system","type":"sysfs","source":"sysfs","options":["ro","nosuid","noexec","nodev"]}]}`,
		`{"ociVersion":"1.1.0","root":{"path":"rootfs"},"process":{"args":["/bin/true"],"cwd":"/"},"mounts":[{"destination":"/sys","type":"sysfs","source":"sysfs","options":["ro","nosuid","noexec","nodev"]},{"destination":"/sys","type":"sysfs","source":"sysfs","options":["ro","nosuid","noexec","nodev"]}]}`,
	}
	for _, body := range cases {
		if _, err := loadOCIBundle(writeOCIBundle(t, body)); err == nil {
			t.Fatalf("expected non-representable sysfs declaration to fail closed: %s", body)
		}
	}
}
