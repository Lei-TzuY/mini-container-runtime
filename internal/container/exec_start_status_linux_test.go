//go:build linux

package container

import (
	"bufio"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"testing"

	"minicontainer/internal/events"
)

func readAllExecEvents(t *testing.T) []events.Event {
	t.Helper()
	f, err := os.Open(events.LogPath())
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	dec := json.NewDecoder(f)
	var out []events.Event
	for {
		var evt events.Event
		if err := dec.Decode(&evt); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			t.Fatal(err)
		}
		out = append(out, evt)
	}
	return out
}

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

func TestExecPayloadSignalForwardingHelper(t *testing.T) {
	if os.Getenv("MINICONTAINER_TEST_EXEC_SIGNAL_FORWARD") != "1" {
		return
	}
	err := runExecPayloadWithStartSignal(
		[]string{"sh", "-c", "parent=$PPID; trap 'exit 0' USR1; echo ready; while kill -0 \"$parent\" 2>/dev/null; do :; done; exit 78"},
		[]string{"PATH=/bin:/usr/bin"},
		nil,
		os.Stdout,
		os.Stderr,
		nil,
	)
	if err != nil {
		os.Exit(77)
	}
	os.Exit(0)
}

func TestRunExecPayloadForwardsSignalFromExecInit(t *testing.T) {
	cmd := exec.Command(os.Args[0], "-test.run=^TestExecPayloadSignalForwardingHelper$")
	cmd.Env = append(os.Environ(), "MINICONTAINER_TEST_EXEC_SIGNAL_FORWARD=1")
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}

	line, err := bufio.NewReader(stdout).ReadString('\n')
	if err != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		t.Fatalf("await payload readiness: %v", err)
	}
	if strings.TrimSpace(line) != "ready" {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		t.Fatalf("unexpected payload readiness %q", line)
	}
	if err := cmd.Process.Signal(syscall.SIGUSR1); err != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		t.Fatalf("signal exec-init helper: %v", err)
	}
	if err := cmd.Wait(); err != nil {
		t.Fatalf("exec-init helper did not forward SIGUSR1 to payload: %v", err)
	}
}

func TestRunExecInitCommandRecordsStartAndNonzeroCompletion(t *testing.T) {
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

	got := readAllExecEvents(t)
	if len(got) != 2 {
		t.Fatalf("events=%+v, want start and terminal outcome", got)
	}
	if got[0].Type != events.EventExec || got[0].ContainerID != containerID {
		t.Fatalf("start event=%+v", got[0])
	}
	if got[1].Type != events.EventExecExit || got[1].ContainerID != containerID || !strings.Contains(got[1].Message, "exit_code=29") {
		t.Fatalf("terminal event=%+v", got[1])
	}
}

func TestRunExecInitCommandRecordsFailureWhenProofMissing(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	const containerID = "exec-proof-missing"
	if err := events.Publish(events.EventExec, containerID, "rootfs", "exec [failed]"); err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command("sh", "-c", "exit 31")
	err := runExecInitCommand(cmd)
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) || exitErr.ExitCode() != 31 {
		t.Fatalf("exec-init exit=%v, want code 31", err)
	}
	got := readAllExecEvents(t)
	if len(got) != 1 || got[0].Type != events.EventExecFailed || got[0].ContainerID != containerID {
		t.Fatalf("events=%+v, want one exec_failed and no started event", got)
	}
	if !strings.Contains(got[0].Message, "payload start") {
		t.Fatalf("failure cause missing: %+v", got[0])
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
