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

func TestOCIMaskedPathsTranslateIntoDurableMountPolicy(t *testing.T) {
	stateDir := t.TempDir()
	bundle := writeOCIBundle(t, `{"ociVersion":"1.1.0","root":{"path":"rootfs"},"process":{"args":["/bin/true"],"cwd":"/"},"linux":{"maskedPaths":["/secret"]}}`)
	secret := filepath.Join(bundle, "rootfs", "secret")
	if err := os.WriteFile(secret, []byte("sensitive"), 0o600); err != nil {
		t.Fatal(err)
	}

	id, err := runOCIBundleWith(bundle, ociBundleRunDeps{
		load: loadOCIBundle,
		prepare: func(cfg *container.Config) (*state.Store, *state.Container, error) {
			assertOCIMaskedPathVolume(t, cfg, "/secret")
			return prepareManagedRunStateWith(cfg, runAdmissionDeps{
				openStore: func() (*state.Store, error) { return state.Open(stateDir) },
				newID:     func() (string, error) { return "oci-masked-path", nil },
				now:       func() time.Time { return time.Unix(90, 0) },
			})
		},
		run: func(cfg container.Config) error {
			assertOCIMaskedPathVolume(t, &cfg, "/secret")
			return runOCIMaskedPathKernelProbe(secret)
		},
		settle: func(st *state.Store, id string, runErr error, _ time.Time) (*state.Container, error) {
			if runErr != nil {
				return nil, runErr
			}
			return st.Resolve(id)
		},
		now: func() time.Time { return time.Unix(100, 0) },
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
		if volume.ContainerPath == "/secret" && volume.HostPath == "/dev/null" && volume.ReadOnly {
			found = true
		}
	}
	if !found {
		t.Fatalf("restart spec lost masked path policy: %#v", spec.Volumes)
	}
}

func TestOCIMaskedPathsRejectInvalidEntries(t *testing.T) {
	for _, maskedPaths := range []string{
		`["secret"]`,
		`["/"]`,
		`["/secret/../secret"]`,
		`["/secret","/secret"]`,
	} {
		bundle := writeOCIBundle(t, fmt.Sprintf(`{"ociVersion":"1.1.0","root":{"path":"rootfs"},"process":{"args":["/bin/true"],"cwd":"/"},"linux":{"maskedPaths":%s}}`, maskedPaths))
		if _, err := loadOCIBundle(bundle); err == nil {
			t.Fatalf("maskedPaths %s was accepted, want validation error", maskedPaths)
		}
	}
}

func assertOCIMaskedPathVolume(t *testing.T, cfg *container.Config, path string) {
	t.Helper()
	for _, volume := range cfg.Volumes {
		if volume.ContainerPath == path {
			if volume.HostPath != "/dev/null" || !volume.ReadOnly {
				t.Fatalf("masked path volume = %#v, want readonly /dev/null bind", volume)
			}
			return
		}
	}
	t.Fatalf("masked path %q missing from runtime volumes: %#v", path, cfg.Volumes)
}

func runOCIMaskedPathKernelProbe(targetSource string) error {
	cmd := exec.Command(os.Args[0], "-test.run=^TestOCIMaskedPathKernelHelper$")
	cmd.Env = append(os.Environ(),
		"MINICONTAINER_OCI_MASKED_PATH_HELPER=1",
		"MINICONTAINER_OCI_MASKED_PATH_TARGET="+targetSource,
	)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("OCI masked path kernel helper: %w: %s", err, output)
	}
	return nil
}

func TestOCIMaskedPathKernelHelper(t *testing.T) {
	if os.Getenv("MINICONTAINER_OCI_MASKED_PATH_HELPER") != "1" {
		t.Skip("helper subprocess only")
	}
	target := os.Getenv("MINICONTAINER_OCI_MASKED_PATH_TARGET")
	if target == "" {
		t.Fatal("masked path helper target is empty")
	}
	if err := syscall.Mount("/dev/null", target, "", syscall.MS_BIND, ""); err != nil {
		if errors.Is(err, syscall.EPERM) || errors.Is(err, syscall.EACCES) {
			// Restricted CI runners may not have CAP_SYS_ADMIN. The runtime uses the
			// same bind mount and must fail closed rather than expose the file.
			return
		}
		t.Fatalf("bind /dev/null over masked path: %v", err)
	}
	defer syscall.Unmount(target, syscall.MNT_DETACH)
	if err := syscall.Mount("", target, "", syscall.MS_BIND|syscall.MS_REMOUNT|syscall.MS_RDONLY, ""); err != nil {
		t.Fatalf("remount masked path readonly: %v", err)
	}
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read masked path: %v", err)
	}
	if len(data) != 0 {
		t.Fatalf("masked path leaked %q", data)
	}
}
