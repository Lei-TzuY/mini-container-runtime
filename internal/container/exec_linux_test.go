//go:build linux

package container

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"strings"
	"testing"

	"golang.org/x/sys/unix"
)

func TestPayloadEnvironmentStripsInternalSentinels(t *testing.T) {
	got := payloadEnvironment([]string{"PATH=/bin:/usr/bin", "MINICONTAINER_EXEC=1", "KEEP=value", "MINICONTAINER_INIT=1"})
	joined := strings.Join(got, "\n")
	if strings.Contains(joined, "MINICONTAINER_EXEC=") || strings.Contains(joined, "MINICONTAINER_INIT=") {
		t.Fatalf("internal sentinel leaked into payload environment: %q", joined)
	}
	if !strings.Contains(joined, "KEEP=value") {
		t.Fatalf("ordinary environment entry lost: %q", joined)
	}
}

func TestRunExecPayloadPreservesEnvironmentAndExitCode(t *testing.T) {
	var stdout bytes.Buffer
	if err := runExecPayload([]string{"sh", "-c", `printf '%s' "$VISIBLE"`}, []string{"PATH=/bin:/usr/bin", "VISIBLE=inside"}, nil, &stdout, &bytes.Buffer{}); err != nil {
		t.Fatalf("runExecPayload: %v", err)
	}
	if stdout.String() != "inside" {
		t.Fatalf("stdout=%q", stdout.String())
	}

	err := runExecPayload([]string{"sh", "-c", "exit 23"}, []string{"PATH=/bin:/usr/bin"}, nil, &bytes.Buffer{}, &bytes.Buffer{})
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) || exitErr.ExitCode() != 23 {
		t.Fatalf("nonzero exit not preserved: %v", err)
	}
}

func TestOpenExecTargetsCurrentProcess(t *testing.T) {
	targets, err := openExecTargets(os.Getpid())
	if err != nil {
		t.Fatalf("open current process exec targets: %v", err)
	}
	defer targets.close()
	if targets.rootFD < 0 || targets.startTime == 0 {
		t.Fatalf("invalid target capture: rootFD=%d start=%d", targets.rootFD, targets.startTime)
	}
	want := []string{"net", "ipc", "uts", "pid", "mnt"}
	if len(targets.ns) != len(want) {
		t.Fatalf("namespace count=%d", len(targets.ns))
	}
	for i, ns := range targets.ns {
		if ns.name != want[i] || ns.fd < 0 {
			t.Fatalf("namespace[%d]=%+v, want %q with open fd", i, ns, want[i])
		}
	}
}

func TestPrepareExecThreadUnsharesCloneFS(t *testing.T) {
	calls := 0
	err := prepareExecThreadWith(func(flags int) error {
		calls++
		if flags != unix.CLONE_FS {
			t.Fatalf("unshare flags=%#x, want %#x", flags, unix.CLONE_FS)
		}
		return nil
	})
	if err != nil || calls != 1 {
		t.Fatalf("prepare exec thread: calls=%d err=%v", calls, err)
	}

	cause := errors.New("unshare rejected")
	err = prepareExecThreadWith(func(flags int) error { return cause })
	if !errors.Is(err, cause) || !strings.Contains(err.Error(), "CLONE_FS") {
		t.Fatalf("unshare failure not preserved: %v", err)
	}
	if err := prepareExecThreadWith(nil); err == nil {
		t.Fatal("nil unshare implementation accepted")
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
