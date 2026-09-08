//go:build linux

package main

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

const (
	initSupervisorArg      = "__minicontainer-init-supervisor"
	processUIDRuntimeEnv   = "MINICONTAINER_PROCESS_UID"
	processGIDRuntimeEnv   = "MINICONTAINER_PROCESS_GID"
	processGroupsRuntimeEnv = "MINICONTAINER_PROCESS_GROUPS"
)

var initSupervisorForwardSignals = []os.Signal{
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

// init installs a tiny internal wrapper around the payload command only in the
// re-executed container-init process. ContainerInit still performs all existing
// namespace/rootfs/security setup, then execs /proc/self/exe. That process stays
// PID 1 and supervises the real payload as its child.
func init() {
	if os.Getenv("MINICONTAINER_INIT") == "1" {
		wrapContainerInitPayload()
		return
	}
	if len(os.Args) >= 2 && os.Args[1] == initSupervisorArg {
		code, err := runContainerInitSupervisor(os.Args[2:])
		if err != nil {
			fmt.Fprintf(os.Stderr, "container init supervisor: %v\n", err)
			os.Exit(125)
		}
		os.Exit(code)
	}
}

func wrapContainerInitPayload() {
	if len(os.Args) < 3 {
		return
	}
	cfg, err := parseRunConfig(os.Args[2:])
	if err != nil || len(cfg.Command) == 0 {
		return
	}
	commandStart := len(os.Args) - len(cfg.Command)
	if commandStart < 2 || commandStart > len(os.Args) {
		return
	}
	wrapped := make([]string, 0, len(os.Args)+2)
	wrapped = append(wrapped, os.Args[:commandStart]...)
	wrapped = append(wrapped, "/proc/self/exe", initSupervisorArg)
	wrapped = append(wrapped, cfg.Command...)
	os.Args = wrapped
}

func payloadCredentialFromRuntimeEnv() (*syscall.Credential, error) {
	uidRaw, hasUID := os.LookupEnv(processUIDRuntimeEnv)
	gidRaw, hasGID := os.LookupEnv(processGIDRuntimeEnv)
	groupsRaw, hasGroups := os.LookupEnv(processGroupsRuntimeEnv)
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
	for _, key := range []string{processUIDRuntimeEnv, processGIDRuntimeEnv, processGroupsRuntimeEnv} {
		if err := os.Unsetenv(key); err != nil {
			return nil, fmt.Errorf("clear process user runtime marker %s: %w", key, err)
		}
	}
	return &syscall.Credential{Uid: uint32(uid), Gid: uint32(gid), Groups: groups}, nil
}

func runContainerInitSupervisor(command []string) (int, error) {
	if len(command) == 0 || command[0] == "" {
		return 0, fmt.Errorf("payload command is empty")
	}
	if err := unix.Prctl(unix.PR_SET_CHILD_SUBREAPER, 1, 0, 0, 0); err != nil {
		return 0, fmt.Errorf("enable child subreaper: %w", err)
	}

	binary, err := exec.LookPath(command[0])
	if err != nil {
		return 0, fmt.Errorf("resolve payload executable %q: %w", command[0], err)
	}
	credential, err := payloadCredentialFromRuntimeEnv()
	if err != nil {
		return 0, err
	}

	forwardedSignals := make(chan os.Signal, 16)
	signal.Notify(forwardedSignals, initSupervisorForwardSignals...)
	defer signal.Stop(forwardedSignals)

	pid, err := syscall.ForkExec(binary, command, &syscall.ProcAttr{
		Env:   os.Environ(),
		Files: []uintptr{os.Stdin.Fd(), os.Stdout.Fd(), os.Stderr.Fd()},
		Sys: &syscall.SysProcAttr{
			Setpgid:    true,
			Credential: credential,
		},
	})
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
				s, ok := sig.(syscall.Signal)
				if !ok {
					continue
				}
				if err := syscall.Kill(-pid, s); err != nil && !errors.Is(err, syscall.ESRCH) {
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
		if status.Exited() {
			return status.ExitStatus(), nil
		}
		if status.Signaled() {
			return 128 + int(status.Signal()), nil
		}
		return 1, fmt.Errorf("payload %d exited with unsupported wait status %#x", pid, uint32(status))
	}
}
