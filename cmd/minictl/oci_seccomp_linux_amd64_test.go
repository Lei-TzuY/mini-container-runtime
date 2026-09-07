//go:build linux && amd64

package main

import (
	"encoding/json"
	"os/exec"
	"testing"
	"time"

	"minicontainer/internal/container"
	"minicontainer/internal/state"
)

func TestOCIBundleRunPreservesSeccompExecutionPolicy(t *testing.T) {
	stateDir := t.TempDir()
	body, err := json.Marshal(map[string]any{
		"ociVersion": "1.1.0",
		"root": map[string]any{"path": "rootfs"},
		"process": map[string]any{
			"args": []string{"/bin/sh", "-c", "printf seccomp-policy"},
			"cwd":  "/",
		},
		"linux": map[string]any{
			"namespaces": []any{},
			"seccomp": map[string]any{
				"defaultAction": "SCMP_ACT_ALLOW",
				"architectures": []string{"SCMP_ARCH_X86_64"},
				"syscalls": []any{map[string]any{
					"names":  builtinSeccompAMD64Syscalls,
					"action": "SCMP_ACT_KILL_PROCESS",
				}},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	bundle := writeOCIBundle(t, string(body))

	id, err := runOCIBundleWith(bundle, ociBundleRunDeps{
		load: loadOCIBundle,
		prepare: func(cfg *container.Config) (*state.Store, *state.Container, error) {
			return prepareManagedRunStateWith(cfg, runAdmissionDeps{
				openStore: func() (*state.Store, error) { return state.Open(stateDir) },
				newID:     func() (string, error) { return "oci-seccomp", nil },
				now:       func() time.Time { return time.Unix(10, 0) },
			})
		},
		run: func(cfg container.Config) error {
			if !cfg.Seccomp {
				t.Fatal("runner lost OCI seccomp policy")
			}
			out, err := exec.Command("/bin/sh", "-c", "printf seccomp-policy").Output()
			if err != nil {
				return err
			}
			if string(out) != "seccomp-policy" {
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

	st, err := state.Open(stateDir)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	spec, err := st.RestartSpec(id)
	if err != nil {
		t.Fatal(err)
	}
	if !spec.Seccomp {
		t.Fatalf("restart spec lost OCI seccomp policy: %+v", spec)
	}
}
