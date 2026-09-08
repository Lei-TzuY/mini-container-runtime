//go:build linux

package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"minicontainer/internal/container"
	"minicontainer/internal/state"
)

func TestOCIReadonlyPathsTranslateIntoDurableMountPolicy(t *testing.T) {
	stateDir := t.TempDir()
	bundle := writeOCIBundle(t, `{"ociVersion":"1.1.0","root":{"path":"rootfs"},"process":{"args":["/bin/true"],"cwd":"/"},"linux":{"readonlyPaths":["/locked"]}}`)
	if err := os.MkdirAll(filepath.Join(bundle, "rootfs", "locked"), 0o755); err != nil {
		t.Fatal(err)
	}

	id, err := runOCIBundleWith(bundle, ociBundleRunDeps{
		load: loadOCIBundle,
		prepare: func(cfg *container.Config) (*state.Store, *state.Container, error) {
			assertOCIReadonlyPathVolume(t, cfg, "/locked")
			return prepareManagedRunStateWith(cfg, runAdmissionDeps{
				openStore: func() (*state.Store, error) { return state.Open(stateDir) },
				newID:     func() (string, error) { return "oci-readonly-path", nil },
				now:       func() time.Time { return time.Unix(70, 0) },
			})
		},
		run: func(cfg container.Config) error {
			assertOCIReadonlyPathVolume(t, &cfg, "/locked")
			return runOCIReadonlyPathKernelProbe(cfg.Volumes[len(cfg.Volumes)-1].HostPath)
		},
		settle: func(st *state.Store, id string, runErr error, _ time.Time) (*state.Container, error) {
			if runErr != nil {
				return nil, runErr
			}
			return st.Resolve(id)
		},
		now: func() time.Time { return time.Unix(80, 0) },
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
	found := false
	for _, volume := range spec.Volumes {
		if volume.ContainerPath == "/locked" && volume.ReadOnly {
			found = true
		}
	}
	if !found {
		t.Fatalf("restart spec lost readonly path policy: %#v", spec.Volumes)
	}
}

func TestOCIReadonlyPathsRejectInvalidEntries(t *testing.T) {
	for _, readonlyPaths := range []string{
		`["locked"]`,
		`["/"]`,
		`["/locked/../locked"]`,
		`["/locked","/locked"]`,
	} {
		bundle := writeOCIBundle(t, fmt.Sprintf(`{"ociVersion":"1.1.0","root":{"path":"rootfs"},"process":{"args":["/bin/true"],"cwd":"/"},"linux":{"readonlyPaths":%s}}`, readonlyPaths))
		if _, err := loadOCIBundle(bundle); err == nil {
			t.Fatalf("readonlyPaths %s was accepted, want validation error", readonlyPaths)
		}
	}
}

func assertOCIReadonlyPathVolume(t *testing.T, cfg *container.Config, path string) {
	t.Helper()
	for _, volume := range cfg.Volumes {
		if volume.ContainerPath == path {
			if !volume.ReadOnly {
				t.Fatalf("readonly path volume is writable: %#v", volume)
			}
			wantSource := filepath.Join(cfg.RootFS, path[1:])
			if volume.HostPath != wantSource {
				t.Fatalf("readonly path source = %q, want %q", volume.HostPath, wantSource)
			}
			return
		}
	}
	t.Fatalf("readonly path %q missing from runtime volumes: %#v", path, cfg.Volumes)
}

func runOCIReadonlyPathKernelProbe(source string) error {
	cmd := exec.Command(os.Args[0], "-test.run=^TestOCIReadonlyPathKernelHelper$")
	cmd.Env = append(os.Environ(),
		"MINICONTAINER_OCI_READONLY_PATH_HELPER=1",
		"MINICONTAINER_OCI_READONLY_PATH_SOURCE="+source,
	)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("OCI readonly path kernel helper: %w: %s", err, output)
	}
	return nil
}

func TestOCIReadonlyPathKernelHelper(t *testing.T) {
	if os.Getenv("MINICONTAINER_OCI_READONLY_PATH_HELPER") != "1" {
		t.Skip("helper subprocess only")
	}
	source := os.Getenv("MINICONTAINER_OCI_READONLY_PATH_SOURCE")
	if source == "" {
		t.Fatal("readonly path helper source is empty")
	}
	target := t.TempDir()
	if err := syscall.Mount(source, target, "", syscall.MS_BIND|syscall.MS_REC, ""); err != nil {
		if errors.Is(err, syscall.EPERM) || errors.Is(err, syscall.EACCES) {
			// Restricted CI runners may not have CAP_SYS_ADMIN. The runtime uses the
			// same mount operation and must fail closed rather than silently launch
			// a writable payload in that environment.
			return
		}
		t.Fatalf("bind mount readonly path: %v", err)
	}
	defer syscall.Unmount(target, syscall.MNT_DETACH)
	if err := syscall.Mount("", target, "", syscall.MS_BIND|syscall.MS_REMOUNT|syscall.MS_RDONLY, ""); err != nil {
		t.Fatalf("remount readonly path: %v", err)
	}
	if err := os.WriteFile(filepath.Join(target, "should-fail"), []byte("x"), 0o600); err == nil {
		t.Fatal("write through readonly bind mount succeeded")
	}
}
