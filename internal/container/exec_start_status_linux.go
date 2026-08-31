//go:build linux

package container

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"

	"minicontainer/internal/events"
	"golang.org/x/sys/unix"
)

const (
	execStartedFDKey       = "MINICONTAINER_EXEC_STARTED_FD"
	execPayloadStartedByte = byte(0xe7)
)

func discardPendingExecIntent() {
	_ = events.DiscardPendingExec()
}

func failPendingExecIntent(err error) {
	if err == nil {
		discardPendingExecIntent()
		return
	}
	_ = events.FailPendingExec(err.Error())
}

func completePendingExecOutcome(waitErr error) {
	if waitErr == nil {
		_ = events.CompletePendingExec(0, "")
		return
	}
	if exitErr, ok := waitErr.(*exec.ExitError); ok {
		_ = events.CompletePendingExec(exitErr.ExitCode(), "")
		return
	}
	_ = events.CompletePendingExec(-1, waitErr.Error())
}

// runExecInitCommand starts the namespace-entering helper, waits for explicit
// proof that the payload process itself was successfully started, commits the
// staged exec event at that boundary, and records a terminal outcome after the
// helper exits. Setup failures publish exec_failed without fabricating a start.
func runExecInitCommand(cmd *exec.Cmd) error {
	if cmd == nil {
		err := fmt.Errorf("exec-init command is nil")
		failPendingExecIntent(err)
		return err
	}

	readPipe, writePipe, err := os.Pipe()
	if err != nil {
		err = fmt.Errorf("create exec payload-start pipe: %w", err)
		failPendingExecIntent(err)
		return err
	}
	fd := 3 + len(cmd.ExtraFiles)
	cmd.ExtraFiles = append(cmd.ExtraFiles, writePipe)
	cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%d", execStartedFDKey, fd))

	if err := cmd.Start(); err != nil {
		_ = readPipe.Close()
		_ = writePipe.Close()
		err = fmt.Errorf("start exec-init helper: %w", err)
		failPendingExecIntent(err)
		return err
	}
	_ = writePipe.Close()

	startedErr := awaitExecPayloadStarted(readPipe)
	if startedErr == nil {
		// Once the child proves Start succeeded, the payload may already be
		// running. Audit-log failure cannot revoke that process admission.
		_ = events.CommitPendingExec()
	}

	waitErr := cmd.Wait()
	if startedErr != nil {
		failPendingExecIntent(startedErr)
		if waitErr != nil {
			return waitErr
		}
		return fmt.Errorf("exec payload start was not proven: %w", startedErr)
	}

	// A proven start receives exactly one terminal outcome, including non-zero
	// payload exits. Persistence remains observability-only and never changes the
	// command's exit semantics.
	completePendingExecOutcome(waitErr)
	if waitErr != nil {
		return waitErr
	}
	return nil
}

func awaitExecPayloadStarted(readPipe *os.File) error {
	if readPipe == nil {
		return fmt.Errorf("exec payload-start reader is nil")
	}
	defer readPipe.Close()

	var proof [1]byte
	if _, err := io.ReadFull(readPipe, proof[:]); err != nil {
		return fmt.Errorf("await exec payload start: %w", err)
	}
	if proof[0] != execPayloadStartedByte {
		return fmt.Errorf("invalid exec payload-start byte 0x%02x", proof[0])
	}
	return nil
}

func execPayloadStartWriterFromEnv() (*os.File, error) {
	raw := os.Getenv(execStartedFDKey)
	fd, err := strconv.Atoi(raw)
	if err != nil || fd < 3 {
		return nil, fmt.Errorf("invalid internal exec payload-start fd %q", raw)
	}
	file := os.NewFile(uintptr(fd), "exec-payload-start")
	if file == nil {
		return nil, fmt.Errorf("open internal exec payload-start fd %d", fd)
	}
	// The exec-init process needs this descriptor until payload Start succeeds,
	// but the payload itself must never inherit the runtime-control channel.
	unix.CloseOnExec(fd)
	return file, nil
}

func notifyExecPayloadStarted(writePipe *os.File) {
	if writePipe == nil {
		return
	}
	_, _ = writePipe.Write([]byte{execPayloadStartedByte})
	_ = writePipe.Close()
}

func runExecPayloadWithStartSignal(command, env []string, stdin io.Reader, stdout, stderr io.Writer, startWriter *os.File) error {
	if len(command) == 0 || command[0] == "" {
		if startWriter != nil {
			_ = startWriter.Close()
		}
		return fmt.Errorf("exec command is empty")
	}
	cmd := exec.Command(command[0], command[1:]...)
	cmd.Env = env
	cmd.Stdin = stdin
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	if err := cmd.Start(); err != nil {
		if startWriter != nil {
			_ = startWriter.Close()
		}
		return err
	}
	// cmd.Start returning nil is the admission boundary: the payload process
	// exists. A failed observer write cannot safely revoke or kill it.
	notifyExecPayloadStarted(startWriter)
	return cmd.Wait()
}
