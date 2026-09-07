//go:build linux

package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"testing"
	"time"

	"minicontainer/internal/container"
	"minicontainer/internal/state"
)

const ociCapBsetRead = 23

func TestOCIBoundingCapabilitiesTranslateIntoDurableRuntimePolicy(t *testing.T) {
	stateDir := t.TempDir()
	bundle := writeOCIBundle(t, `{"ociVersion":"1.1.0","root":{"path":"rootfs"},"process":{"args":["/bin/true"],"cwd":"/","capabilities":{"bounding":["CAP_CHOWN","CAP_NET_RAW"]}}}`)

	id, err := runOCIBundleWith(bundle, ociBundleRunDeps{
		load: loadOCIBundle,
		prepare: func(cfg *container.Config) (*state.Store, *state.Container, error) {
			if containsCapability(cfg.CapDrop, "CAP_CHOWN") || containsCapability(cfg.CapDrop, "CAP_NET_RAW") {
				t.Fatalf("requested bounding capabilities were translated as drops: %#v", cfg.CapDrop)
			}
			if !containsCapability(cfg.CapDrop, "CAP_SYS_ADMIN") || !containsCapability(cfg.CapDrop, "CAP_BPF") {
				t.Fatalf("bounding complement is incomplete: %#v", cfg.CapDrop)
			}
			return prepareManagedRunStateWith(cfg, runAdmissionDeps{
				openStore: func() (*state.Store, error) { return state.Open(stateDir) },
				newID:     func() (string, error) { return "oci-cap-bounding", nil },
				now:       func() time.Time { return time.Unix(50, 0) },
			})
		},
		run: func(cfg container.Config) error {
			return runOCICapabilityKernelProbe(t, cfg.CapDrop)
		},
		settle: func(st *state.Store, id string, runErr error, _ time.Time) (*state.Container, error) {
			if runErr != nil {
				return nil, runErr
			}
			return st.Resolve(id)
		},
		now: func() time.Time { return time.Unix(60, 0) },
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
	if containsCapability(spec.CapDrop, "CAP_CHOWN") || !containsCapability(spec.CapDrop, "CAP_SYS_ADMIN") {
		t.Fatalf("restart spec lost OCI bounding-set policy: %#v", spec.CapDrop)
	}
}

func TestOCICapabilitiesRejectUnrepresentableSets(t *testing.T) {
	for _, capabilities := range []string{
		`{"effective":[]}`,
		`{"permitted":["CAP_CHOWN"]}`,
		`{"inheritable":["CAP_CHOWN"]}`,
		`{"ambient":["CAP_CHOWN"]}`,
	} {
		bundle := writeOCIBundle(t, fmt.Sprintf(`{"ociVersion":"1.1.0","root":{"path":"rootfs"},"process":{"args":["/bin/true"],"cwd":"/","capabilities":%s}}`, capabilities))
		if _, err := loadOCIBundle(bundle); err == nil || !strings.Contains(err.Error(), "not yet representable") {
			t.Fatalf("capabilities %s error = %v, want fail-closed unsupported-set error", capabilities, err)
		}
	}
}

func TestOCIBoundingCapabilitiesRejectInvalidNames(t *testing.T) {
	for _, bounding := range []string{
		`["CHOWN"]`,
		`["CAP_NOT_REAL"]`,
		`["CAP_CHOWN","CAP_CHOWN"]`,
	} {
		bundle := writeOCIBundle(t, fmt.Sprintf(`{"ociVersion":"1.1.0","root":{"path":"rootfs"},"process":{"args":["/bin/true"],"cwd":"/","capabilities":{"bounding":%s}}}`, bounding))
		if _, err := loadOCIBundle(bundle); err == nil {
			t.Fatalf("bounding %s was accepted, want validation error", bounding)
		}
	}
}

func runOCICapabilityKernelProbe(t *testing.T, drops []string) error {
	t.Helper()
	if os.Getenv("MINICONTAINER_OCI_CAP_HELPER") == "1" {
		return nil
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestOCICapabilityKernelHelper$")
	cmd.Env = append(os.Environ(),
		"MINICONTAINER_OCI_CAP_HELPER=1",
		"MINICONTAINER_OCI_CAP_DROPS="+strings.Join(drops, ","),
	)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("OCI capability kernel helper: %w: %s", err, output)
	}
	return nil
}

func TestOCICapabilityKernelHelper(t *testing.T) {
	if os.Getenv("MINICONTAINER_OCI_CAP_HELPER") != "1" {
		t.Skip("helper subprocess only")
	}
	drops := strings.Split(os.Getenv("MINICONTAINER_OCI_CAP_DROPS"), ",")
	const capDACOverride = uintptr(1)
	before, _, errno := syscall.RawSyscall(syscall.SYS_PRCTL, ociCapBsetRead, capDACOverride, 0)
	if errno != 0 {
		t.Fatalf("PR_CAPBSET_READ before drop: %v", errno)
	}

	err := container.DropCapabilities(drops, false)
	after, _, readErrno := syscall.RawSyscall(syscall.SYS_PRCTL, ociCapBsetRead, capDACOverride, 0)
	if readErrno != 0 {
		t.Fatalf("PR_CAPBSET_READ after drop: %v", readErrno)
	}
	requestedDrop := containsCapability(drops, "CAP_DAC_OVERRIDE")
	if !requestedDrop {
		t.Fatal("kernel helper did not receive CAP_DAC_OVERRIDE in translated drop policy")
	}
	if err == nil && before == 1 && after != 0 {
		t.Fatalf("capability policy reported success but CAP_DAC_OVERRIDE remained in bounding set")
	}
	if err != nil && before == 1 && after == 0 {
		t.Fatalf("capability policy changed kernel state but returned error: %v", err)
	}
	// A restricted runner may lack CAP_SETPCAP. In that case the runtime must
	// fail closed; successful enforcement is required never to silently retain
	// the dropped capability.
}

func containsCapability(caps []string, want string) bool {
	for _, capability := range caps {
		if capability == want {
			return true
		}
	}
	return false
}
