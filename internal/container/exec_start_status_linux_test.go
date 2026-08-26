//go:build linux

package container

import (
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"strings"
	"testing"

	"minicontainer/internal/events"
)

func TestRunExecPayloadSignalsAfterSuccessfulStartEvenOnNonzeroExit(t *testing.T) {
	readPipe, writePipe, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}

	err = runExecPayloadWithStartSignal(
		[]string{"sh", "-c", "exit 23"},
		[]string{"PATH=/bin:/usr/bin"},
		nil,
		os.Stdout,
		os.Stderr,
		writePipe,
	)
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) || exitErr.ExitCode() != 23 {
		t.Fatalf("payload exit=%v, want code 23", err)
	}
	if err := awaitExecPayloadStarted(readPipe); err != nil {
		t.Fatalf("nonzero payload did not prove successful Start: %v", err)
	}
}

func TestRunExecPayloadDoesNotSignalWhenStartFails(t *testing.T) {
	readPipe, writePipe, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}

	err = runExecPayloadWithStartSignal(
		[]string{"/definitely/not/a/minicontainer-command"},
		nil,
		nil,
		os.Stdout,
		os.Stderr,
		writePipe,
	)
	if err == nil {
		t.Fatal("missing payload unexpectedly started")
	}
	if err := awaitExecPayloadStarted(readPipe); err == nil {
		t.Fatal("failed payload Start produced success proof")
	}
}

func TestRunExecInitCommandCommitsExecBeforeNonzeroCompletion(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	const containerID = "exec-proof-nonzero"
	if err := events.Publish(events.EventExec, containerID, "rootfs", "exec [sh]"); err != nil {
		t.Fatalf("stage exec: %v", err)
	}

	// runExecInitCommand has no pre-existing ExtraFiles here, so its proof pipe
	// is fd 3. The helper writes the exact proof byte, then exits nonzero.
	cmd := exec.Command("sh", "-c", "printf '\\347' >&3; exit 29")
	err := runExecInitCommand(cmd)
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) || exitErr.ExitCode() != 29 {
		t.Fatalf("exec-init exit=%v, want code 29", err)
	}

	f, err := os.Open(events.LogPath())
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var evt events.Event
	if err := json.NewDecoder(f).Decode(&evt); err != nil {
		t.Fatal(err)
	}
	if evt.Type != events.EventExec || evt.ContainerID != containerID {
		t.Fatalf("event=%+v, want committed exec", evt)
	}
}

func TestRunExecInitCommandDiscardsExecWhenProofMissing(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	if err := events.Publish(events.EventExec, "exec-proof-missing", "rootfs", "exec [failed]"); err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command("sh", "-c", "exit 31")
	err := runExecInitCommand(cmd)
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) || exitErr.ExitCode() != 31 {
		t.Fatalf("exec-init exit=%v, want code 31", err)
	}
	if _, err := os.Stat(events.LogPath()); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing payload-start proof wrote exec event: err=%v", err)
	}
}

func TestPayloadEnvironmentStripsExecStartedFD(t *testing.T) {
	got := strings.Join(payloadEnvironment([]string{
		"PATH=/bin:/usr/bin",
		execStartedFDKey + "=9",
		"VISIBLE=yes",
	}), "\n")
	if strings.Contains(got, execStartedFDKey+"=") {
		t.Fatalf("exec payload-start descriptor leaked into payload env: %q", got)
	}
	if !strings.Contains(got, "VISIBLE=yes") {
		t.Fatalf("ordinary environment entry lost: %q", got)
	}
}
