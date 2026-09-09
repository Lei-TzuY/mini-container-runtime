//go:build linux

package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"minicontainer/internal/container"
	"minicontainer/internal/state"
)

func TestOCIMaskedDirectoryBecomesDurableRuntimePolicy(t *testing.T) {
	stateDir := t.TempDir()
	bundle := writeOCIBundle(t, `{"ociVersion":"1.1.0","root":{"path":"rootfs"},"process":{"args":["/bin/true"],"cwd":"/"},"linux":{"maskedPaths":["/secrets"]}}`)
	if err := os.Mkdir(filepath.Join(bundle, "rootfs", "secrets"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bundle, "rootfs", "secrets", "token"), []byte("sensitive"), 0o600); err != nil {
		t.Fatal(err)
	}

	id, err := runOCIBundleWith(bundle, ociBundleRunDeps{
		load: loadOCIBundle,
		prepare: func(cfg *container.Config) (*state.Store, *state.Container, error) {
			assertMaskedDirectoryPolicy(t, cfg, "/secrets")
			return prepareManagedRunStateWith(cfg, runAdmissionDeps{
				openStore: func() (*state.Store, error) { return state.Open(stateDir) },
				newID:     func() (string, error) { return "oci-masked-directory", nil },
				now:       func() time.Time { return time.Unix(110, 0) },
			})
		},
		run: func(cfg container.Config) error {
			assertMaskedDirectoryPolicy(t, &cfg, "/secrets")
			return nil
		},
		settle: func(st *state.Store, id string, runErr error, _ time.Time) (*state.Container, error) {
			return st.Resolve(id)
		},
		now: func() time.Time { return time.Unix(120, 0) },
	})
	if err != nil {
		t.Fatalf("runOCIBundleWith: %v", err)
	}

	st, err := state.Open(stateDir)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	spec, err := st.RestartSpec(id)
	if err != nil {
		t.Fatal(err)
	}
	if len(spec.MaskedDirectories) != 1 || spec.MaskedDirectories[0] != "/secrets" {
		t.Fatalf("restart masked directories = %#v, want [/secrets]", spec.MaskedDirectories)
	}
	for _, volume := range spec.Volumes {
		if volume.ContainerPath == "/secrets" && volume.HostPath == "/dev/null" {
			t.Fatalf("directory mask leaked into bind volume policy: %#v", volume)
		}
	}
}

func TestOCIMaskedDirectoryLeavesFileMaskAsDevNullVolume(t *testing.T) {
	bundle := writeOCIBundle(t, `{"ociVersion":"1.1.0","root":{"path":"rootfs"},"process":{"args":["/bin/true"],"cwd":"/"},"linux":{"maskedPaths":["/secret"]}}`)
	if err := os.WriteFile(filepath.Join(bundle, "rootfs", "secret"), []byte("sensitive"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := loadOCIBundle(bundle)
	if err != nil {
		t.Fatal(err)
	}
	if err := promoteOCIMaskedDirectories(bundle, &cfg); err != nil {
		t.Fatal(err)
	}
	if len(cfg.MaskedDirectories) != 0 {
		t.Fatalf("file mask was promoted as directory: %#v", cfg.MaskedDirectories)
	}
	assertOCIMaskedPathVolume(t, &cfg, "/secret")
}

func assertMaskedDirectoryPolicy(t *testing.T, cfg *container.Config, want string) {
	t.Helper()
	if len(cfg.MaskedDirectories) != 1 || cfg.MaskedDirectories[0] != want {
		t.Fatalf("masked directories = %#v, want [%s]", cfg.MaskedDirectories, want)
	}
	for _, volume := range cfg.Volumes {
		if volume.ContainerPath == want && volume.HostPath == "/dev/null" {
			t.Fatalf("directory mask still represented as /dev/null volume: %#v", volume)
		}
	}
}
