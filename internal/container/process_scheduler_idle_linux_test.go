//go:build linux

package container

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"

	"golang.org/x/sys/unix"
)

const processSchedulerIdleProbeEnv = "MINICONTAINER_TEST_PROCESS_SCHEDULER_IDLE_PROBE"

func TestProcessSchedulerIdleRuntimeMarkerApplied(t *testing.T) {
	if os.Getenv(processSchedulerIdleProbeEnv) == "1" {
		if processSchedulerThreadID <= 0 {
			fmt.Fprint(os.Stderr, "scheduler thread was not pinned")
			os.Exit(2)
		}
		attr, err := unix.SchedGetAttr(processSchedulerThreadID, 0)
		if err != nil {
			fmt.Fprintf(os.Stderr, "sched_getattr pinned thread: %v", err)
			os.Exit(2)
		}
		fmt.Printf("policy=%d|marker=%q", attr.Policy, os.Getenv(processSchedulerEnv))
		os.Exit(0)
	}

	exe, err := os.Executable()
	if err != nil {
		t.Fatalf("resolve test executable: %v", err)
	}
	cmd := exec.Command(exe, "-test.run=^TestProcessSchedulerIdleRuntimeMarkerApplied$")
	env := make([]string, 0, len(os.Environ())+3)
	for _, entry := range os.Environ() {
		if strings.HasPrefix(entry, sentinelEnvKey+"=") ||
			strings.HasPrefix(entry, processSchedulerEnv+"=") ||
			strings.HasPrefix(entry, processSchedulerIdleProbeEnv+"=") {
			continue
		}
		env = append(env, entry)
	}
	env = append(env,
		sentinelEnvKey+"=1",
		processSchedulerEnv+"=SCHED_IDLE:0",
		processSchedulerIdleProbeEnv+"=1")
	cmd.Env = env
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("run SCHED_IDLE probe: %v: %s", err, out)
	}
	want := fmt.Sprintf("policy=%d|marker=%q", unix.SCHED_IDLE, "")
	if got := strings.TrimSpace(string(out)); got != want {
		t.Fatalf("pinned scheduler/marker = %q, want %q", got, want)
	}
}
