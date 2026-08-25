//go:build linux

package container

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestPayloadEnvironmentStripsInternalSentinels(t *testing.T) {
	got := payloadEnvironment([]string{
		"PATH=/bin:/usr/bin",
		"MINICONTAINER_EXEC=1",
		"KEEP=value",
		"MINICONTAINER_INIT=1",
	})
	joined := strings.Join(got, "\n")
	if strings.Contains(joined, "MINICONTAINER_EXEC=") || strings.Contains(joined, "MINICONTAINER_INIT=") {
		t.Fatalf("internal sentinel leaked into payload environment: %q", joined)
	}
	if !strings.Contains(joined, "PATH=/bin:/usr/bin") || !strings.Contains(joined, "KEEP=value") {
		t.Fatalf("ordinary environment entries were lost: %q", joined)
	}
}

func TestRunExecPayloadSpawnsChildAndPreservesEnvironment(t *testing.T) {
	var stdout bytes.Buffer
	err := runExecPayload(
		[]string{"sh", "-c", `printf '%s' "$VISIBLE"`},
		[]string{"PATH=/bin:/usr/bin", "VISIBLE=inside"},
		nil,
		&stdout,
		&bytes.Buffer{},
	)
	if err != nil {
		t.Fatalf("runExecPayload: %v", err)
	}
	if got := stdout.String(); got != "inside" {
		t.Fatalf("stdout=%q, want %q", got, "inside")
	}
}

func TestRunExecPayloadPreservesNonzeroExitCode(t *testing.T) {
	err := runExecPayload(
		[]string{"sh", "-c", "exit 23"},
		[]string{"PATH=/bin:/usr/bin"},
		nil,
		&bytes.Buffer{},
		&bytes.Buffer{},
	)
	if err == nil {
		t.Fatal("expected non-zero payload exit")
	}
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("error type=%T, want *exec.ExitError: %v", err, err)
	}
	if got := exitErr.ExitCode(); got != 23 {
		t.Fatalf("exit code=%d, want 23", got)
	}
}

func TestOpenExecTargetsCurrentProcess(t *testing.T) {
	targets, err := openExecTargets(os.Getpid())
	if err != nil {
		t.Fatalf("open current process exec targets: %v", err)
	}
	defer targets.close()

	if targets.rootFD < 0 {
		t.Fatal("root fd not opened")
	}
	if targets.startTime == 0 {
		t.Fatal("process start time not captured")
	}
	if len(targets.ns) != 5 {
		t.Fatalf("namespace count=%d, want 5", len(targets.ns))
	}
	want := []string{"net", "ipc", "uts", "pid", "mnt"}
	for i, ns := range targets.ns {
		if ns.name != want[i] {
			t.Fatalf("namespace[%d]=%q, want %q", i, ns.name, want[i])
		}
		if ns.fd < 0 {
			t.Fatalf("namespace %s fd not opened", ns.name)
		}
	}
}

func TestExecRejectsInvalidConfigBeforeSpawn(t *testing.T) {
	if err := Exec(ExecConfig{ContainerPID: 0, Command: []string{"true"}}); err == nil {
		t.Fatal("invalid PID accepted")
	}
	if err := Exec(ExecConfig{ContainerPID: os.Getpid()}); err == nil {
		t.Fatal("empty command accepted")
	}
}
