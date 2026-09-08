//go:build linux

package container

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"

	"golang.org/x/sys/unix"
)

const processRlimitProbeEnv = "MINICONTAINER_TEST_PROCESS_RLIMIT_PROBE"

func TestProcessRlimitRuntimeMarkersApplied(t *testing.T) {
	if os.Getenv(processRlimitProbeEnv) == "1" {
		var nofile, core, fsize, stack, nproc unix.Rlimit
		if err := unix.Getrlimit(unix.RLIMIT_NOFILE, &nofile); err != nil { fmt.Fprintf(os.Stderr, "getrlimit NOFILE: %v", err); os.Exit(2) }
		if err := unix.Getrlimit(unix.RLIMIT_CORE, &core); err != nil { fmt.Fprintf(os.Stderr, "getrlimit CORE: %v", err); os.Exit(2) }
		if err := unix.Getrlimit(unix.RLIMIT_FSIZE, &fsize); err != nil { fmt.Fprintf(os.Stderr, "getrlimit FSIZE: %v", err); os.Exit(2) }
		if err := unix.Getrlimit(unix.RLIMIT_STACK, &stack); err != nil { fmt.Fprintf(os.Stderr, "getrlimit STACK: %v", err); os.Exit(2) }
		if err := unix.Getrlimit(unix.RLIMIT_NPROC, &nproc); err != nil { fmt.Fprintf(os.Stderr, "getrlimit NPROC: %v", err); os.Exit(2) }
		fmt.Printf("%d:%d|%d:%d|%d:%d|%d:%d|%d:%d|%q|%q|%q|%q|%q",
			nofile.Cur, nofile.Max, core.Cur, core.Max, fsize.Cur, fsize.Max, stack.Cur, stack.Max, nproc.Cur, nproc.Max,
			os.Getenv(processRlimitNOFILEEnv), os.Getenv(processRlimitCOREEnv), os.Getenv(processRlimitFSIZEEnv), os.Getenv(processRlimitSTACKEnv), os.Getenv(processRlimitNPROCEnv))
		os.Exit(0)
	}

	var current unix.Rlimit
	if err := unix.Getrlimit(unix.RLIMIT_NOFILE, &current); err != nil { t.Fatalf("get current RLIMIT_NOFILE: %v", err) }
	if current.Max < 16 { t.Skipf("RLIMIT_NOFILE hard limit %d is too small for deterministic probe", current.Max) }
	hard := current.Max
	if hard > 512 { hard = 512 }
	soft := hard
	if soft > 256 { soft = 256 }

	var currentStack unix.Rlimit
	if err := unix.Getrlimit(unix.RLIMIT_STACK, &currentStack); err != nil { t.Fatalf("get current RLIMIT_STACK: %v", err) }
	const minimumStack = 1 << 20
	const preferredStack = 8 << 20
	if currentStack.Max < minimumStack { t.Skipf("RLIMIT_STACK hard limit %d is too small for deterministic probe", currentStack.Max) }
	stackLimit := currentStack.Max
	if stackLimit > preferredStack { stackLimit = preferredStack }

	var currentNproc unix.Rlimit
	if err := unix.Getrlimit(unix.RLIMIT_NPROC, &currentNproc); err != nil { t.Fatalf("get current RLIMIT_NPROC: %v", err) }

	exe, err := os.Executable()
	if err != nil { t.Fatalf("resolve test executable: %v", err) }
	cmd := exec.Command(exe, "-test.run=^TestProcessRlimitRuntimeMarkersApplied$")
	env := make([]string, 0, len(os.Environ())+7)
	for _, entry := range os.Environ() {
		if strings.HasPrefix(entry, sentinelEnvKey+"=") || strings.HasPrefix(entry, processRlimitNOFILEEnv+"=") || strings.HasPrefix(entry, processRlimitCOREEnv+"=") || strings.HasPrefix(entry, processRlimitFSIZEEnv+"=") || strings.HasPrefix(entry, processRlimitSTACKEnv+"=") || strings.HasPrefix(entry, processRlimitNPROCEnv+"=") || strings.HasPrefix(entry, processRlimitProbeEnv+"=") { continue }
		env = append(env, entry)
	}
	env = append(env,
		sentinelEnvKey+"=1",
		processRlimitNOFILEEnv+"="+strconv.FormatUint(soft, 10)+":"+strconv.FormatUint(hard, 10),
		processRlimitCOREEnv+"=0:0",
		processRlimitFSIZEEnv+"=4096:8192",
		processRlimitSTACKEnv+"="+strconv.FormatUint(stackLimit, 10)+":"+strconv.FormatUint(stackLimit, 10),
		processRlimitNPROCEnv+"="+strconv.FormatUint(currentNproc.Cur, 10)+":"+strconv.FormatUint(currentNproc.Max, 10),
		processRlimitProbeEnv+"=1")
	cmd.Env = env
	out, err := cmd.CombinedOutput()
	if err != nil { t.Fatalf("run rlimit probe: %v: %s", err, out) }
	want := fmt.Sprintf("%d:%d|0:0|4096:8192|%d:%d|%d:%d|%q|%q|%q|%q|%q", soft, hard, stackLimit, stackLimit, currentNproc.Cur, currentNproc.Max, "", "", "", "", "")
	if got := strings.TrimSpace(string(out)); got != want { t.Fatalf("payload rlimits/markers = %q, want %q", got, want) }
}
