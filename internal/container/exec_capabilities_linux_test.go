//go:build linux

package container

import (
	"os"
	"os/exec"
	"syscall"
	"testing"
)

const prCapBsetRead = 23 // PR_CAPBSET_READ

func TestApplyExecCapabilityDropsKernelPolicy(t *testing.T) {
	const capability = "CAP_NET_RAW"
	capValue := capMap[capability]

	if os.Getenv("MINICONTAINER_TEST_EXEC_CAP_DROP") == "1" {
		before, _, errno := syscall.RawSyscall(syscall.SYS_PRCTL, prCapBsetRead, capValue, 0)
		if errno != 0 {
			t.Fatalf("prctl(PR_CAPBSET_READ, %s): %v", capability, errno)
		}
		if before != 1 {
			t.Skipf("%s is already absent from the subprocess bounding set", capability)
		}
		if err := applyExecCapabilityDrops([]string{capability}); err != nil {
			t.Fatal(err)
		}
		after, _, errno := syscall.RawSyscall(syscall.SYS_PRCTL, prCapBsetRead, capValue, 0)
		if errno != 0 {
			t.Fatalf("prctl(PR_CAPBSET_READ, %s): %v", capability, errno)
		}
		if after != 0 {
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
