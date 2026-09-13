//go:build linux

package main

import (
	"testing"
	"time"

	"minicontainer/internal/container"
	"minicontainer/internal/state"
)

func TestOCIBundleMqueueMountReachesManagedRun(t *testing.T) {
	bundle := writeOCIBundle(t, `{"ociVersion":"1.1.0","root":{"path":"rootfs"},"process":{"args":["/bin/true"],"cwd":"/"},"mounts":[{"destination":"/dev/mqueue","type":"mqueue","source":"mqueue","options":["nosuid","noexec","nodev"]}],"linux":{"namespaces":[{"type":"pid"},{"type":"mount"},{"type":"ipc"}]}}`)
	stateDir := t.TempDir()
	var ran bool
	_, err := runOCIBundleWith(bundle, ociBundleRunDeps{
		load: loadOCIBundle,
		prepare: func(cfg *container.Config) (*state.Store, *state.Container, error) {
			return prepareManagedRunStateWith(cfg, runAdmissionDeps{
				openStore: func() (*state.Store, error) { return state.Open(stateDir) },
				newID:     func() (string, error) { return "oci-mqueue-process", nil },
				now:       func() time.Time { return time.Unix(10, 0) },
			})
		},
		run: func(cfg container.Config) error {
			ran = true
			if cfg.ContainerID != "oci-mqueue-process" {
				t.Fatalf("runner received container id %q", cfg.ContainerID)
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
		t.Fatalf("run OCI bundle with mqueue mount: %v", err)
	}
	if !ran {
		t.Fatal("managed runner was not reached")
	}
}

func TestOCIMqueueMountRejectsNonRepresentableSemantics(t *testing.T) {
	cases := []string{
		`{"ociVersion":"1.1.0","root":{"path":"rootfs"},"process":{"args":["/bin/true"],"cwd":"/"},"mounts":[{"destination":"/dev/mqueue","type":"mqueue","source":"mqueue","options":["nosuid","noexec"]}]}`,
		`{"ociVersion":"1.1.0","root":{"path":"rootfs"},"process":{"args":["/bin/true"],"cwd":"/"},"mounts":[{"destination":"/dev/mqueue","type":"mqueue","source":"none","options":["nosuid","noexec","nodev"]}]}`,
		`{"ociVersion":"1.1.0","root":{"path":"rootfs"},"process":{"args":["/bin/true"],"cwd":"/"},"mounts":[{"destination":"/mqueue","type":"mqueue","source":"mqueue","options":["nosuid","noexec","nodev"]}]}`,
		`{"ociVersion":"1.1.0","root":{"path":"rootfs"},"process":{"args":["/bin/true"],"cwd":"/"},"mounts":[{"destination":"/dev/mqueue","type":"mqueue","source":"mqueue","options":["nosuid","noexec","nodev","rw"]}]}`,
		`{"ociVersion":"1.1.0","root":{"path":"rootfs"},"process":{"args":["/bin/true"],"cwd":"/"},"mounts":[{"destination":"/dev/mqueue","type":"mqueue","source":"mqueue","options":["nosuid","noexec","nodev"]},{"destination":"/dev/mqueue","type":"mqueue","source":"mqueue","options":["nosuid","noexec","nodev"]}]}`,
	}
	for _, body := range cases {
		if _, err := loadOCIBundle(writeOCIBundle(t, body)); err == nil {
			t.Fatalf("expected non-representable mqueue declaration to fail closed: %s", body)
		}
	}
}
