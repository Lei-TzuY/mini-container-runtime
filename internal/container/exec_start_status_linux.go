//go:build linux

package container

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"syscall"

	"minicontainer/internal/events"
	"golang.org/x/sys/unix"
)

const (
	execStartedFDKey       = "MINICONTAINER_EXEC_STARTED_FD"
	execPayloadStartedByte = byte(0xe7)
)

var execPayloadForwardSignals = []os.Signal{
	syscall.SIGHUP,
	syscall.SIGINT,
	syscall.SIGQUIT,
	syscall.SIGUSR1,
	syscall.SIGUSR2,
	syscall.SIGTERM,
	syscall.SIGTSTP,
	syscall.SIGTTIN,
	syscall.SIGTTOU,
	syscall.SIGCONT,
	syscall.SIGWINCH,
}

func discardPendingExecIntent() { _ = events.DiscardPendingExec() }

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
	if startedErr == nil { _ = events.CommitPendingExec() }
	waitErr := cmd.Wait()
	if startedErr != nil {
		failPendingExecIntent(startedErr)
		if waitErr != nil { return waitErr }
		return fmt.Errorf("exec payload start was not proven: %w", startedErr)
	}
	completePendingExecOutcome(waitErr)
	return waitErr
}

func awaitExecPayloadStarted(readPipe *os.File) error {
	if readPipe == nil { return fmt.Errorf("exec payload-start reader is nil") }
	defer readPipe.Close()
	var proof [1]byte
	if _, err := io.ReadFull(readPipe, proof[:]); err != nil { return fmt.Errorf("await exec payload start: %w", err) }
	if proof[0] != execPayloadStartedByte { return fmt.Errorf("invalid exec payload-start byte 0x%02x", proof[0]) }
	return nil
}

func execPayloadStartWriterFromEnv() (*os.File, error) {
	raw := os.Getenv(execStartedFDKey)
	fd, err := strconv.Atoi(raw)
	if err != nil || fd < 3 { return nil, fmt.Errorf("invalid internal exec payload-start fd %q", raw) }
	file := os.NewFile(uintptr(fd), "exec-payload-start")
	if file == nil { return nil, fmt.Errorf("open internal exec payload-start fd %d", fd) }
	unix.CloseOnExec(fd)
	return file, nil
}

func notifyExecPayloadStarted(writePipe *os.File) {
	if writePipe == nil { return }
	_, _ = writePipe.Write([]byte{execPayloadStartedByte})
	_ = writePipe.Close()
}

func runExecPayloadWithStartSignal(command, env []string, stdin io.Reader, stdout, stderr io.Writer, startWriter *os.File) error {
	if len(command) == 0 || command[0] == "" {
		if startWriter != nil { _ = startWriter.Close() }
		return fmt.Errorf("exec command is empty")
	}
	forwardedSignals := make(chan os.Signal, 16)
	signal.Notify(forwardedSignals, execPayloadForwardSignals...)
	defer signal.Stop(forwardedSignals)

	cmd := exec.Command(command[0], command[1:]...)
	cmd.Env = env
	cmd.Stdin = stdin
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	// Keep the exec workload in a dedicated process group. Signals delivered to
	// exec-init are lifecycle signals for the workload, not merely its leader;
	// group delivery prevents children from surviving when the leader handles or
	// exits on a forwarded signal.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		if startWriter != nil { _ = startWriter.Close() }
		return err
	}
	notifyExecPayloadStarted(startWriter)

	waitResult := make(chan error, 1)
	go func() { waitResult <- cmd.Wait() }()
	var forwardingErr error
	for {
		select {
		case sig := <-forwardedSignals:
			if sig == nil { continue }
			sysSig, ok := sig.(syscall.Signal)
			if !ok { forwardingErr = errors.Join(forwardingErr, fmt.Errorf("forward exec payload signal %v: unsupported signal type", sig)); continue }
			if err := syscall.Kill(-cmd.Process.Pid, sysSig); err != nil && !errors.Is(err, syscall.ESRCH) {
				forwardingErr = errors.Join(forwardingErr, fmt.Errorf("forward exec payload process-group signal %v: %w", sig, err))
			}
		case waitErr := <-waitResult:
			return errors.Join(waitErr, forwardingErr)
		}
	}
}
