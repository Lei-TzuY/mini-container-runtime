//go:build linux

package container

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
)

const (
	processOOMScoreProbeEnv     = "MINICONTAINER_TEST_PROCESS_OOM_SCORE_PROBE"
	execProcessOOMScoreProbeEnv = "MINICONTAINER_TEST_EXEC_PROCESS_OOM_SCORE_PROBE"
)

func TestProcessOOMScoreRuntimeMarkerApplied(t *testing.T) {
	if os.Getenv(processOOMScoreProbeEnv) == "1" {
		value, err := os.ReadFile("/proc/self/oom_score_adj")
		if err != nil {
			fmt.Fprintf(os.Stderr, "read oom_score_adj: %v", err)
			os.Exit(2)
		}
		fmt.Printf("%s|%q", strings.TrimSpace(string(value)), os.Getenv(processOOMScoreAdjEnv))
		os.Exit(0)
	}

	currentRaw, err := os.ReadFile("/proc/self/oom_score_adj")
	if err != nil {
		t.Fatalf("read current oom_score_adj: %v", err)
	}
	current, err := strconv.Atoi(strings.TrimSpace(string(currentRaw)))
	if err != nil {
		t.Fatalf("parse current oom_score_adj: %v", err)
	}
	target := current
	if target < 500 {
		target = 500
	}

	exe, err := os.Executable()
	if err != nil {
		t.Fatalf("resolve test executable: %v", err)
	}
	cmd := exec.Command(exe, "-test.run=^TestProcessOOMScoreRuntimeMarkerApplied$")
	env := make([]string, 0, len(os.Environ())+3)
	for _, entry := range os.Environ() {
		if strings.HasPrefix(entry, sentinelEnvKey+"=") || strings.HasPrefix(entry, processOOMScoreAdjEnv+"=") || strings.HasPrefix(entry, processOOMScoreProbeEnv+"=") {
			continue
		}
		env = append(env, entry)
	}
	env = append(env,
		sentinelEnvKey+"=1",
		processOOMScoreAdjEnv+"="+strconv.Itoa(target),
		processOOMScoreProbeEnv+"=1")
	cmd.Env = env
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("run oom score probe: %v: %s", err, out)
	}
	want := fmt.Sprintf("%d|%q", target, "")
	if got := strings.TrimSpace(string(out)); got != want {
		t.Fatalf("payload oom_score_adj/marker = %q, want %q", got, want)
	}
}

func TestExecProcessOOMScoreRuntimeMarkerApplied(t *testing.T) {
	if os.Getenv(execProcessOOMScoreProbeEnv) == "1" {
		value, err := os.ReadFile("/proc/self/oom_score_adj")
		if err != nil {
			fmt.Fprintf(os.Stderr, "read exec oom_score_adj: %v", err)
			os.Exit(2)
		}
		fmt.Printf("%s|%q", strings.TrimSpace(string(value)), os.Getenv(processOOMScoreAdjEnv))
		os.Exit(0)
	}

	currentRaw, err := os.ReadFile("/proc/self/oom_score_adj")
	if err != nil {
		t.Fatalf("read current exec oom_score_adj: %v", err)
	}
	current, err := strconv.Atoi(strings.TrimSpace(string(currentRaw)))
	if err != nil {
		t.Fatalf("parse current exec oom_score_adj: %v", err)
	}
	if current >= 1000 {
		t.Skip("current oom_score_adj is already 1000; cannot make an unprivileged deterministic increase")
	}
	target := current + 100
	if target > 1000 {
		target = 1000
	}

	exe, err := os.Executable()
	if err != nil {
		t.Fatalf("resolve test executable: %v", err)
	}
	cmd := exec.Command(exe, "-test.run=^TestExecProcessOOMScoreRuntimeMarkerApplied$")
	env := make([]string, 0, len(os.Environ())+3)
	for _, entry := range os.Environ() {
		if strings.HasPrefix(entry, execSentinelKey+"=") || strings.HasPrefix(entry, processOOMScoreAdjEnv+"=") || strings.HasPrefix(entry, execProcessOOMScoreProbeEnv+"=") {
			continue
		}
		env = append(env, entry)
	}
	env = append(env,
		execSentinelKey+"=1",
		processOOMScoreAdjEnv+"="+strconv.Itoa(target),
		execProcessOOMScoreProbeEnv+"=1")
	cmd.Env = env
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("run exec oom score probe: %v: %s", err, out)
	}
	want := fmt.Sprintf("%d|%q", target, "")
	if got := strings.TrimSpace(string(out)); got != want {
		t.Fatalf("exec payload oom_score_adj/marker = %q, want %q", got, want)
	}
}
