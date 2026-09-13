//go:build linux

package main

import (
	"testing"
	"time"

	"minicontainer/internal/container"
	"minicontainer/internal/state"
)

func TestOCIBundleDevptsMountReachesManagedRun(t *testing.T) {
	bundle := writeOCIBundle(t, `{"ociVersion":"1.1.0","root":{"path":"rootfs"},"process":{"args":["/bin/true"],"cwd":"/"},"mounts":[{"destination":"/dev/pts","type":"devpts","source":"devpts","options":["nosuid","noexec","newinstance","ptmxmode=0666","mode=0666"]}],"linux":{"namespaces":[{"type":"pid"},{"type":"mount"}]}}`)
	stateDir := t.TempDir()
	var ran bool
	_, err := runOCIBundleWith(bundle, ociBundleRunDeps{
		load: loadOCIBundle,
		prepare: func(cfg *container.Config) (*state.Store, *state.Container, error) {
			return prepareManagedRunStateWith(cfg, runAdmissionDeps{
				openStore: func() (*state.Store, error) { return state.Open(stateDir) },
				newID:     func() (string, error) { return "oci-devpts-process", nil },
				now:       func() time.Time { return time.Unix(10, 0) },
			})
		},
		run: func(cfg container.Config) error {
			ran = true
			if cfg.ContainerID != "oci-devpts-process" {
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
		t.Fatalf("run OCI bundle with devpts mount: %v", err)
	}
	if !ran {
		t.Fatal("managed runner was not reached")
	}
}

func TestOCIDevptsMountRejectsNonRepresentableSemantics(t *testing.T) {
	cases := []string{
		`{"ociVersion":"1.1.0","root":{"path":"rootfs"},"process":{"args":["/bin/true"],"cwd":"/"},"mounts":[{"destination":"/dev/pts","type":"devpts","source":"devpts","options":["nosuid","noexec","newinstance","ptmxmode=0666"]}]}`,
		`{"ociVersion":"1.1.0","root":{"path":"rootfs"},"process":{"args":["/bin/true"],"cwd":"/"},"mounts":[{"destination":"/dev/pts","type":"devpts","source":"none","options":["nosuid","noexec","newinstance","ptmxmode=0666","mode=0666"]}]}`,
		`{"ociVersion":"1.1.0","root":{"path":"rootfs"},"process":{"args":["/bin/true"],"cwd":"/"},"mounts":[{"destination":"/pts","type":"devpts","source":"devpts","options":["nosuid","noexec","newinstance","ptmxmode=0666","mode=0666"]}]}`,
		`{"ociVersion":"1.1.0","root":{"path":"rootfs"},"process":{"args":["/bin/true"],"cwd":"/"},"mounts":[{"destination":"/dev/pts","type":"devpts","source":"devpts","options":["nosuid","noexec","newinstance","ptmxmode=0666","mode=0666","gid=5"]}]}`,
		`{"ociVersion":"1.1.0","root":{"path":"rootfs"},"process":{"args":["/bin/true"],"cwd":"/"},"mounts":[{"destination":"/dev/pts","type":"devpts","source":"devpts","options":["nosuid","noexec","newinstance","ptmxmode=0666","mode=0666"]},{"destination":"/dev/pts","type":"devpts","source":"devpts","options":["nosuid","noexec","newinstance","ptmxmode=0666","mode=0666"]}]}`,
	}
	for _, body := range cases {
		if _, err := loadOCIBundle(writeOCIBundle(t, body)); err == nil {
			t.Fatalf("expected non-representable devpts declaration to fail closed: %s", body)
		}
	}
}
