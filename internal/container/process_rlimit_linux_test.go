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
		var nofile, core, fsize unix.Rlimit
		if err := unix.Getrlimit(unix.RLIMIT_NOFILE, &nofile); err != nil {
			fmt.Fprintf(os.Stderr, "getrlimit NOFILE: %v", err)
			os.Exit(2)
		}
		if err := unix.Getrlimit(unix.RLIMIT_CORE, &core); err != nil {
			fmt.Fprintf(os.Stderr, "getrlimit CORE: %v", err)
			os.Exit(2)
		}
		if err := unix.Getrlimit(unix.RLIMIT_FSIZE, &fsize); err != nil {
			fmt.Fprintf(os.Stderr, "getrlimit FSIZE: %v", err)
			os.Exit(2)
		}
		fmt.Printf("%d:%d|%d:%d|%d:%d|%q|%q|%q",
			nofile.Cur, nofile.Max,
			core.Cur, core.Max,
			fsize.Cur, fsize.Max,
			os.Getenv(processRlimitNOFILEEnv),
			os.Getenv(processRlimitCOREEnv),
			os.Getenv(processRlimitFSIZEEnv),
		)
		os.Exit(0)
	}

	var current unix.Rlimit
	if err := unix.Getrlimit(unix.RLIMIT_NOFILE, &current); err != nil {
		t.Fatalf("get current RLIMIT_NOFILE: %v", err)
	}
	if current.Max < 16 {
		t.Skipf("RLIMIT_NOFILE hard limit %d is too small for deterministic probe", current.Max)
	}
	hard := current.Max
	if hard > 512 {
		hard = 512
	}
	soft := hard
	if soft > 256 {
		soft = 256
	}

	exe, err := os.Executable()
	if err != nil {
		t.Fatalf("resolve test executable: %v", err)
	}
	cmd := exec.Command(exe, "-test.run=^TestProcessRlimitRuntimeMarkersApplied$")
	env := make([]string, 0, len(os.Environ())+5)
	for _, entry := range os.Environ() {
		if strings.HasPrefix(entry, sentinelEnvKey+"=") ||
			strings.HasPrefix(entry, processRlimitNOFILEEnv+"=") ||
			strings.HasPrefix(entry, processRlimitCOREEnv+"=") ||
			strings.HasPrefix(entry, processRlimitFSIZEEnv+"=") ||
			strings.HasPrefix(entry, processRlimitProbeEnv+"=") {
			continue
		}
		env = append(env, entry)
	}
	env = append(env,
		sentinelEnvKey+"=1",
		processRlimitNOFILEEnv+"="+strconv.FormatUint(soft, 10)+":"+strconv.FormatUint(hard, 10),
		processRlimitCOREEnv+"=0:0",
		processRlimitFSIZEEnv+"=4096:8192",
		processRlimitProbeEnv+"=1",
	)
	cmd.Env = env
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("run rlimit probe: %v: %s", err, out)
	}
	want := fmt.Sprintf("%d:%d|0:0|4096:8192|%q|%q|%q", soft, hard, "", "", "")
	if got := strings.TrimSpace(string(out)); got != want {
		t.Fatalf("payload rlimits/markers = %q, want %q", got, want)
	}
}
