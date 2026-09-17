//go:build linux

package container

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"testing"

	"golang.org/x/sys/unix"
)

const processIOPriorityProbeEnv = "MINICONTAINER_TEST_PROCESS_IO_PRIORITY_PROBE"

func TestProcessIOPriorityRuntimeMarkerApplied(t *testing.T) {
	if os.Getenv(processIOPriorityProbeEnv) == "1" {
		runtime.LockOSThread()
		if err := applyProcessIOPriorityPolicy(); err != nil {
			fmt.Fprintf(os.Stderr, "apply ioprio: %v", err)
			os.Exit(2)
		}
		value, _, errno := unix.Syscall(unix.SYS_IOPRIO_GET, uintptr(ioPriorityWhoProcess), 0, 0)
		if errno != 0 {
			fmt.Fprintf(os.Stderr, "ioprio_get: %v", errno)
			os.Exit(2)
		}
		class := int(value) >> ioPriorityClassShift
		priority := int(value) & ((1 << ioPriorityClassShift) - 1)
		fmt.Printf("class=%d|priority=%d|marker=%q", class, priority, os.Getenv(processIOPriorityEnv))
		os.Exit(0)
	}

	exe, err := os.Executable()
	if err != nil {
		t.Fatalf("resolve test executable: %v", err)
	}
	cmd := exec.Command(exe, "-test.run=^TestProcessIOPriorityRuntimeMarkerApplied$")
	env := make([]string, 0, len(os.Environ())+3)
	for _, entry := range os.Environ() {
		if strings.HasPrefix(entry, sentinelEnvKey+"=") ||
			strings.HasPrefix(entry, processIOPriorityEnv+"=") ||
			strings.HasPrefix(entry, processIOPriorityProbeEnv+"=") {
			continue
		}
		env = append(env, entry)
	}
	env = append(env,
		sentinelEnvKey+"=1",
		processIOPriorityEnv+"=IOPRIO_CLASS_BE:4",
		processIOPriorityProbeEnv+"=1")
	cmd.Env = env
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("run io priority probe: %v: %s", err, out)
	}
	want := fmt.Sprintf("class=%d|priority=4|marker=%q", ioPriorityClassBE, "")
	if got := strings.TrimSpace(string(out)); got != want {
		t.Fatalf("payload io priority/marker = %q, want %q", got, want)
	}
}
