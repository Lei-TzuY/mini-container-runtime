//go:build linux && (amd64 || arm64)

package container

import (
	"os"
	"os/exec"
	"syscall"
	"testing"
)

func TestApplyExecSeccompKernelPolicy(t *testing.T) {
	if os.Getenv("MINICONTAINER_TEST_EXEC_SECCOMP") == "1" {
		if err := applyExecSeccomp(true); err != nil {
			os.Exit(90)
		}
		// Prove the installed filter still permits ordinary native-ABI syscalls.
		if _, _, errno := syscall.RawSyscall(syscall.SYS_GETPID, 0, 0, 0); errno != 0 {
			os.Exit(91)
		}
		// unshare is part of the built-in blocked syscall table on all supported
		// architectures; the exec security policy must terminate the subprocess.
		syscall.RawSyscall(syscall.SYS_UNSHARE, uintptr(syscall.CLONE_NEWNS), 0, 0)
		os.Exit(92)
	}

	cmd := exec.Command(os.Args[0], "-test.run=^TestApplyExecSeccompKernelPolicy$")
	cmd.Env = append(os.Environ(), "MINICONTAINER_TEST_EXEC_SECCOMP=1")
	err := cmd.Run()
	exitErr, ok := err.(*exec.ExitError)
	if !ok {
		t.Fatalf("seccomp subprocess error = %v, want signal termination", err)
	}
	status, ok := exitErr.Sys().(syscall.WaitStatus)
	if !ok || !status.Signaled() || status.Signal() != syscall.SIGSYS {
		t.Fatalf("seccomp subprocess status = %v, want SIGSYS from blocked syscall", exitErr.Sys())
	}
}

func TestApplyExecSeccompDisabled(t *testing.T) {
	if err := applyExecSeccomp(false); err != nil {
		t.Fatalf("disabled exec seccomp policy returned error: %v", err)
	}
}
