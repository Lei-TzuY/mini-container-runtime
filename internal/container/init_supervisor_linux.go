//go:build linux

package container

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"strconv"
	"strings"
	"syscall"

	"golang.org/x/sys/unix"
)

var initSupervisorForwardSignals = []os.Signal{
	syscall.SIGHUP,
	syscall.SIGINT,
	syscall.SIGQUIT,
	syscall.SIGUSR1,
	syscall.SIGUSR2,
	syscall.SIGPIPE,
	syscall.SIGALRM,
	syscall.SIGTERM,
	syscall.SIGTSTP,
	syscall.SIGTTIN,
	syscall.SIGTTOU,
	syscall.SIGURG,
	syscall.SIGXCPU,
	syscall.SIGXFSZ,
	syscall.SIGVTALRM,
	syscall.SIGPROF,
	syscall.SIGCONT,
	syscall.SIGWINCH,
}

type preparedInitSupervisor struct {
	command    []string
	binary     string
	credential *syscall.Credential
	umask      *uint32
}

// PayloadCredentialFromRuntimeEnv consumes the parent-issued process identity
// policy before the reserved runtime environment is cleared.
func PayloadCredentialFromRuntimeEnv() (*syscall.Credential, error) {
	uidRaw, hasUID := os.LookupEnv(processUIDEnv)
	gidRaw, hasGID := os.LookupEnv(processGIDEnv)
	groupsRaw, hasGroups := os.LookupEnv(processGroupsEnv)
	if !hasUID && !hasGID && !hasGroups {
		return nil, nil
	}
	if !hasUID || !hasGID || !hasGroups {
		return nil, fmt.Errorf("incomplete process user runtime markers")
	}
	uid, err := strconv.ParseUint(uidRaw, 10, 32)
	if err != nil {
		return nil, fmt.Errorf("invalid process uid runtime marker %q: %w", uidRaw, err)
	}
	gid, err := strconv.ParseUint(gidRaw, 10, 32)
	if err != nil {
		return nil, fmt.Errorf("invalid process gid runtime marker %q: %w", gidRaw, err)
	}
	groups := []uint32(nil)
	if groupsRaw != "" {
		for _, raw := range strings.Split(groupsRaw, ",") {
			value, err := strconv.ParseUint(raw, 10, 32)
			if err != nil {
				return nil, fmt.Errorf("invalid process groups runtime marker %q: %w", groupsRaw, err)
			}
			groups = append(groups, uint32(value))
		}
	}
	for _, key := range []string{processUIDEnv, processGIDEnv, processGroupsEnv} {
		if err := os.Unsetenv(key); err != nil {
			return nil, fmt.Errorf("clear process user runtime marker %s: %w", key, err)
		}
	}
	return &syscall.Credential{Uid: uint32(uid), Gid: uint32(gid), Groups: groups}, nil
}

// PayloadUmaskFromRuntimeEnv consumes and validates the parent-issued umask.
func PayloadUmaskFromRuntimeEnv() (*uint32, error) {
	raw, ok := os.LookupEnv(processUmaskEnv)
	if !ok {
		return nil, nil
	}
	if err := os.Unsetenv(processUmaskEnv); err != nil {
		return nil, fmt.Errorf("clear process umask runtime marker: %w", err)
	}
	value, err := strconv.ParseUint(raw, 10, 32)
	if err != nil {
		return nil, fmt.Errorf("invalid process umask runtime marker %q: %w", raw, err)
	}
	if value > 0o777 {
		return nil, fmt.Errorf("invalid process umask runtime marker %q: exceeds 0777", raw)
	}
	umask := uint32(value)
	return &umask, nil
}

func prepareInitSupervisor(command []string) (*preparedInitSupervisor, error) {
	if len(command) == 0 || command[0] == "" {
		return nil, fmt.Errorf("payload command is empty")
	}
	binary, err := exec.LookPath(command[0])
	if err != nil {
		return nil, fmt.Errorf("resolve payload executable %q: %w", command[0], err)
	}
	credential, err := PayloadCredentialFromRuntimeEnv()
	if err != nil {
		return nil, err
	}
	payloadUmask, err := PayloadUmaskFromRuntimeEnv()
	if err != nil {
		return nil, err
	}
	return &preparedInitSupervisor{
		command:    append([]string(nil), command...),
		binary:     binary,
		credential: credential,
		umask:      payloadUmask,
	}, nil
}

// RunInitSupervisor runs the same PID-1 supervision path used by ContainerInit.
// It is exported so process-level tests can exercise signal forwarding and
// descendant cleanup without constructing a privileged container.
func RunInitSupervisor(command []string) (int, error) {
	supervisor, err := prepareInitSupervisor(command)
	if err != nil {
		return 0, err
	}
	return supervisor.run(os.Environ())
}

func terminateInitSupervisorChildren() error {
	path := fmt.Sprintf("/proc/self/task/%d/children", os.Getpid())
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("enumerate adopted payload descendants: %w", err)
	}
	for _, raw := range strings.Fields(string(data)) {
		pid, err := strconv.Atoi(raw)
		if err != nil || pid <= 0 {
			return fmt.Errorf("invalid adopted payload descendant pid %q", raw)
		}
		if err := syscall.Kill(pid, syscall.SIGKILL); err != nil && !errors.Is(err, syscall.ESRCH) {
			return fmt.Errorf("terminate adopted payload descendant %d: %w", pid, err)
		}
	}
	return nil
}

func drainInitSupervisorProcessGroup(pgid int) error {
	if err := syscall.Kill(-pgid, syscall.SIGKILL); err != nil && !errors.Is(err, syscall.ESRCH) {
		return fmt.Errorf("terminate payload process group %d: %w", pgid, err)
	}
	// Descendants may call setsid/setpgid and escape the payload process group.
	// Once the primary payload has been reaped, subreaper/PID-1 adoption makes
	// those surviving descendants direct children; terminate them explicitly.
	if err := terminateInitSupervisorChildren(); err != nil {
		return err
	}
	for {
		var status syscall.WaitStatus
		_, err := syscall.Wait4(-1, &status, 0, nil)
		if errors.Is(err, syscall.EINTR) {
			runtime.Gosched()
			continue
		}
		if errors.Is(err, syscall.ECHILD) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("reap payload descendant: %w", err)
		}
	}
}

func (s *preparedInitSupervisor) run(env []string) (int, error) {
	if err := unix.Prctl(unix.PR_SET_CHILD_SUBREAPER, 1, 0, 0, 0); err != nil {
		return 0, fmt.Errorf("enable child subreaper: %w", err)
	}

	forwardedSignals := make(chan os.Signal, 16)
	signal.Notify(forwardedSignals, initSupervisorForwardSignals...)
	defer signal.Stop(forwardedSignals)

	oldUmask := -1
	if s.umask != nil {
		oldUmask = syscall.Umask(int(*s.umask))
	}
	pid, err := syscall.ForkExec(s.binary, s.command, &syscall.ProcAttr{
		Env:   env,
		Files: []uintptr{os.Stdin.Fd(), os.Stdout.Fd(), os.Stderr.Fd()},
		Sys: &syscall.SysProcAttr{
			Setpgid:    true,
			Credential: s.credential,
		},
	})
	if oldUmask >= 0 {
		syscall.Umask(oldUmask)
	}
	if err != nil {
		return 0, fmt.Errorf("start payload: %w", err)
	}

	doneForwarding := make(chan struct{})
	defer close(doneForwarding)
	forwardingErr := make(chan error, 1)
	go func() {
		for {
			select {
			case <-doneForwarding:
				return
			case sig := <-forwardedSignals:
				if sig == nil {
					continue
				}
				native, ok := sig.(syscall.Signal)
				if !ok {
					continue
				}
				if err := syscall.Kill(-pid, native); err != nil && !errors.Is(err, syscall.ESRCH) {
					select {
					case forwardingErr <- fmt.Errorf("forward signal %v to payload process group %d: %w", sig, pid, err):
					default:
					}
				}
			}
		}
	}()

	for {
		var status syscall.WaitStatus
		reapedPID, err := syscall.Wait4(-1, &status, 0, nil)
		if err != nil {
			if errors.Is(err, syscall.EINTR) {
				runtime.Gosched()
				continue
			}
			return 0, fmt.Errorf("reap child process: %w", err)
		}
		if reapedPID != pid {
			continue
		}

		select {
		case err := <-forwardingErr:
			return 0, err
		default:
		}

		exitCode := 0
		if status.Exited() {
			exitCode = status.ExitStatus()
		} else if status.Signaled() {
			exitCode = 128 + int(status.Signal())
		} else {
			return 1, fmt.Errorf("payload %d exited with unsupported wait status %#x", pid, uint32(status))
		}
		if err := drainInitSupervisorProcessGroup(pid); err != nil {
			return 0, err
		}
		return exitCode, nil
	}
}
