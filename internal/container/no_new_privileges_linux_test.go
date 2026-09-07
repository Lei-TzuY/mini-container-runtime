//go:build linux

package container

import (
	"os"
	"os/exec"
	"strings"
	"syscall"
	"testing"
)

const prGetNoNewPrivs = 39

func TestNoNewPrivilegesInitMarkerEnforcesKernelPolicy(t *testing.T) {
	cmd := exec.Command(os.Args[0], "-test.run=^TestNoNewPrivilegesHelper$")
	cmd.Env = append(os.Environ(), sentinelEnv, noNewPrivilegesEnv+"=1", "MINICONTAINER_NNP_HELPER=1")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("helper failed: %v\n%s", err, out)
	}
	if got := strings.TrimSpace(string(out)); got != "1" {
		t.Fatalf("PR_GET_NO_NEW_PRIVS = %q, want 1", got)
	}
}

func TestNoNewPrivilegesHelper(t *testing.T) {
	if os.Getenv("MINICONTAINER_NNP_HELPER") != "1" {
		return
	}
	r1, _, errno := syscall.RawSyscall(syscall.SYS_PRCTL, prGetNoNewPrivs, 0, 0)
	if errno != 0 {
		t.Fatalf("prctl(PR_GET_NO_NEW_PRIVS): %v", errno)
	}
	if os.Getenv(noNewPrivilegesEnv) != "" {
		t.Fatalf("runtime marker leaked into helper environment")
	}
	_, _ = os.Stdout.WriteString(string(rune('0' + r1)))
}
