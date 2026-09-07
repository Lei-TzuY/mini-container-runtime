//go:build linux

package main

import (
	"os/exec"
	"testing"
	"time"

	"minicontainer/internal/container"
	"minicontainer/internal/state"
)

func TestOCIBundleRunPreservesUserNamespaceSelection(t *testing.T) {
	stateDir := t.TempDir()
	bundle := writeOCIBundle(t, `{"ociVersion":"1.1.0","root":{"path":"rootfs"},"process":{"args":["/bin/sh","-c","printf namespace-policy"],"cwd":"/"},"linux":{"namespaces":[{"type":"pid"},{"type":"mount"},{"type":"uts"},{"type":"ipc"},{"type":"network"}]}}`)

	id, err := runOCIBundleWith(bundle, ociBundleRunDeps{
		load: loadOCIBundle,
		prepare: func(cfg *container.Config) (*state.Store, *state.Container, error) {
			return prepareManagedRunStateWith(cfg, runAdmissionDeps{
				openStore: func() (*state.Store, error) { return state.Open(stateDir) },
				newID:     func() (string, error) { return "oci-no-userns", nil },
				now:       func() time.Time { return time.Unix(10, 0) },
			})
		},
		run: func(cfg container.Config) error {
			if cfg.UserNS {
				t.Fatalf("runner enabled user namespace omitted by OCI config")
			}
			out, err := exec.Command("/bin/sh", "-c", "printf namespace-policy").Output()
			if err != nil {
				return err
			}
			if string(out) != "namespace-policy" {
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
	if spec.UserNS {
		t.Fatalf("restart spec enabled user namespace omitted by OCI config: %+v", spec)
	}
}
