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

const processRlimitMemlockProbeEnv = "MINICONTAINER_TEST_PROCESS_RLIMIT_MEMLOCK_PROBE"

func TestProcessRlimitMemlockRuntimeMarkerApplied(t *testing.T) {
	if os.Getenv(processRlimitMemlockProbeEnv) == "1" {
		var limit unix.Rlimit
		if err := unix.Getrlimit(unix.RLIMIT_MEMLOCK, &limit); err != nil {
			fmt.Fprintf(os.Stderr, "getrlimit MEMLOCK: %v", err)
			os.Exit(2)
		}
		fmt.Printf("%d:%d|%q", limit.Cur, limit.Max, os.Getenv(processRlimitMEMLOCKEnv))
		os.Exit(0)
	}

	exe, err := os.Executable()
	if err != nil {
		t.Fatalf("resolve test executable: %v", err)
	}
	cmd := exec.Command(exe, "-test.run=^TestProcessRlimitMemlockRuntimeMarkerApplied$")
	env := make([]string, 0, len(os.Environ())+3)
	for _, entry := range os.Environ() {
		if strings.HasPrefix(entry, sentinelEnvKey+"=") || strings.HasPrefix(entry, processRlimitMEMLOCKEnv+"=") || strings.HasPrefix(entry, processRlimitMemlockProbeEnv+"=") {
			continue
		}
		env = append(env, entry)
	}
	env = append(env,
		sentinelEnvKey+"=1",
		processRlimitMEMLOCKEnv+"=0:0",
		processRlimitMemlockProbeEnv+"=1")
	cmd.Env = env
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("run MEMLOCK rlimit probe: %v: %s", err, out)
	}
	if got, want := strings.TrimSpace(string(out)), `0:0|""`; got != want {
		t.Fatalf("payload RLIMIT_MEMLOCK/marker = %q, want %q", got, want)
	}
}
