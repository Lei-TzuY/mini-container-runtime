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

const processRlimitMemoryProbeEnv = "MINICONTAINER_TEST_PROCESS_RLIMIT_MEMORY_PROBE"

func TestProcessRlimitMemoryRuntimeMarkersApplied(t *testing.T) {
	if os.Getenv(processRlimitMemoryProbeEnv) == "1" {
		var asLimit unix.Rlimit
		if err := unix.Getrlimit(unix.RLIMIT_AS, &asLimit); err != nil {
			fmt.Fprintf(os.Stderr, "getrlimit AS: %v", err)
			os.Exit(2)
		}
		var dataLimit unix.Rlimit
		if err := unix.Getrlimit(unix.RLIMIT_DATA, &dataLimit); err != nil {
			fmt.Fprintf(os.Stderr, "getrlimit DATA: %v", err)
			os.Exit(2)
		}
		fmt.Printf("as=%d:%d|data=%d:%d|as-marker=%q|data-marker=%q",
			asLimit.Cur, asLimit.Max, dataLimit.Cur, dataLimit.Max,
			os.Getenv(processRlimitASEnv), os.Getenv(processRlimitDATAEnv))
		os.Exit(0)
	}

	exe, err := os.Executable()
	if err != nil {
		t.Fatalf("resolve test executable: %v", err)
	}
	cmd := exec.Command(exe, "-test.run=^TestProcessRlimitMemoryRuntimeMarkersApplied$")
	env := make([]string, 0, len(os.Environ())+4)
	for _, entry := range os.Environ() {
		if strings.HasPrefix(entry, sentinelEnvKey+"=") ||
			strings.HasPrefix(entry, processRlimitASEnv+"=") ||
			strings.HasPrefix(entry, processRlimitDATAEnv+"=") ||
			strings.HasPrefix(entry, processRlimitMemoryProbeEnv+"=") {
			continue
		}
		env = append(env, entry)
	}
	const asLimit = uint64(8 * 1024 * 1024 * 1024)
	const dataLimit = uint64(1024 * 1024 * 1024)
	env = append(env,
		sentinelEnvKey+"=1",
		fmt.Sprintf("%s=%d:%d", processRlimitASEnv, asLimit, asLimit),
		fmt.Sprintf("%s=%d:%d", processRlimitDATAEnv, dataLimit, dataLimit),
		processRlimitMemoryProbeEnv+"=1")
	cmd.Env = env
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("run memory rlimit probe: %v: %s", err, out)
	}
	want := fmt.Sprintf("as=%d:%d|data=%d:%d|as-marker=%q|data-marker=%q", asLimit, asLimit, dataLimit, dataLimit, "", "")
	if got := strings.TrimSpace(string(out)); got != want {
		t.Fatalf("payload memory rlimits/markers = %q, want %q", got, want)
	}
}