//go:build linux

package container

import (
	"errors"
	"os"
	"os/exec"
	"syscall"
	"testing"
)

func TestApplyExecCapabilityDropsKernelPolicy(t *testing.T) {
	const capability = "CAP_NET_RAW"
	capValue := capMap[capability]

	if os.Getenv("MINICONTAINER_TEST_EXEC_CAP_DROP") == "1" {
		before, err := capabilityInBoundingSet(capValue)
		if err != nil {
			t.Fatalf("read %s from capability bounding set before exec policy: %v", capability, err)
		}
		if !before {
			t.Skipf("%s is already absent from the subprocess bounding set", capability)
		}
		if err := applyExecCapabilityDrops([]string{capability}); err != nil {
			// PR_CAPBSET_DROP requires CAP_SETPCAP. Hosted CI runners commonly run
			// tests without that capability; keep the kernel regression active on
			// capable Linux environments without turning a missing host privilege
			// into a product failure.
			if errors.Is(err, syscall.EPERM) {
				t.Skipf("kernel denied PR_CAPBSET_DROP without CAP_SETPCAP: %v", err)
			}
			t.Fatal(err)
		}
		after, err := capabilityInBoundingSet(capValue)
		if err != nil {
			t.Fatalf("read %s from capability bounding set after exec policy: %v", capability, err)
		}
		if after {
			t.Fatalf("%s remains in capability bounding set after exec policy application", capability)
		}
		return
	}

	cmd := exec.Command(os.Args[0], "-test.run=^TestApplyExecCapabilityDropsKernelPolicy$")
	cmd.Env = append(os.Environ(), "MINICONTAINER_TEST_EXEC_CAP_DROP=1")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("capability-drop subprocess failed: %v\n%s", err, output)
	}
}

func TestApplyExecCapabilityDropsRejectsUnknownCapability(t *testing.T) {
	if err := applyExecCapabilityDrops([]string{"CAP_MINICONTAINER_UNKNOWN"}); err == nil {
		t.Fatal("unknown capability unexpectedly accepted")
	}
}
