//go:build linux

package container

import (
	"os"
	"os/exec"
	"syscall"
	"testing"
)

func TestApplyExecNoNewPrivilegesKernelPolicy(t *testing.T) {
	if os.Getenv("MINICONTAINER_TEST_EXEC_NNP") == "1" {
		if err := applyExecNoNewPrivileges(true); err != nil {
			t.Fatal(err)
		}
		value, _, errno := syscall.RawSyscall(syscall.SYS_PRCTL, prGetNoNewPrivs, 0, 0)
		if errno != 0 {
			t.Fatalf("prctl(PR_GET_NO_NEW_PRIVS): %v", errno)
		}
		if value != 1 {
			t.Fatalf("PR_GET_NO_NEW_PRIVS = %d, want 1", value)
		}
		return
	}

	cmd := exec.Command(os.Args[0], "-test.run=^TestApplyExecNoNewPrivilegesKernelPolicy$")
	cmd.Env = append(os.Environ(), "MINICONTAINER_TEST_EXEC_NNP=1")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("no-new-privileges subprocess failed: %v\n%s", err, output)
	}
}

func TestApplyExecNoNewPrivilegesDisabledIsNoop(t *testing.T) {
	valueBefore, _, errno := syscall.RawSyscall(syscall.SYS_PRCTL, prGetNoNewPrivs, 0, 0)
	if errno != 0 {
		t.Fatalf("prctl(PR_GET_NO_NEW_PRIVS): %v", errno)
	}
	if err := applyExecNoNewPrivileges(false); err != nil {
		t.Fatal(err)
	}
	valueAfter, _, errno := syscall.RawSyscall(syscall.SYS_PRCTL, prGetNoNewPrivs, 0, 0)
	if errno != 0 {
		t.Fatalf("prctl(PR_GET_NO_NEW_PRIVS): %v", errno)
	}
	if valueAfter != valueBefore {
		t.Fatalf("disabled policy changed PR_GET_NO_NEW_PRIVS from %d to %d", valueBefore, valueAfter)
	}
}
