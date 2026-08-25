//go:build linux

// internal/container/exec_linux.go
//
// `minictl exec` — Enter an Existing Container
// ─────────────────────────────────────────────
// setns(CLONE_NEWPID) changes only the PID namespace used for subsequently
// created children; it does not move the calling process itself. Therefore the
// exec-init helper must join namespaces first and then spawn the payload as a
// child. Directly calling execve after PID setns would leave the payload in the
// caller's original PID namespace.

package container

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"golang.org/x/sys/unix"
)

// ExecConfig describes a command to be run inside an existing container.
type ExecConfig struct {
	// ContainerPID is the host PID of the container's init process.
	ContainerPID int

	// RootFS is retained for the re-exec command-line contract. ExecInit uses a
	// pre-opened /proc/<pid>/root descriptor instead of trusting this path after
	// namespace transitions.
	RootFS string

	// Command is the argv to exec inside the container.
	Command []string

	// Debug enables verbose logging.
	Debug bool
}

const (
	execSentinelKey = "MINICONTAINER_EXEC"
	execSentinelEnv = execSentinelKey + "=1"
)

type execNamespaceTarget struct {
	name string
	flag int
	fd   int
}

type execTargets struct {
	rootFD    int
	startTime uint64
	ns        []execNamespaceTarget
}

func (t *execTargets) close() {
	if t == nil {
		return
	}
	if t.rootFD >= 0 {
		_ = unix.Close(t.rootFD)
		t.rootFD = -1
	}
	for i := range t.ns {
		if t.ns[i].fd >= 0 {
			_ = unix.Close(t.ns[i].fd)
			t.ns[i].fd = -1
		}
	}
}

// Exec re-execs the current binary as a child, enters the running container's
// namespaces via setns(2), and then starts the requested payload there.
func Exec(cfg ExecConfig) error {
	if cfg.ContainerPID <= 0 {
		return fmt.Errorf("invalid container PID %d", cfg.ContainerPID)
	}
	if len(cfg.Command) == 0 || cfg.Command[0] == "" {
		return fmt.Errorf("exec command is empty")
	}

	if cfg.Debug {
		fmt.Printf("[exec] entering namespaces of PID %d\n", cfg.ContainerPID)
	}

	self, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve executable: %w", err)
	}

	childArgs := append(
		[]string{"exec", strconv.Itoa(cfg.ContainerPID), cfg.RootFS},
		cfg.Command...,
	)
	cmd := exec.Command(self, childArgs...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = append(os.Environ(), execSentinelEnv)

	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			os.Exit(exitErr.ExitCode())
		}
		return err
	}
	return nil
}

// openExecTargets opens every namespace plus the target process root before
// performing any setns call. Opening first avoids resolving /proc paths after
// mount/PID namespace transitions and lets us detect PID reuse during setup.
func openExecTargets(containerPID int) (*execTargets, error) {
	if containerPID <= 0 {
		return nil, fmt.Errorf("invalid container PID %d", containerPID)
	}

	startTime, err := ProcessStartTime(containerPID)
	if err != nil {
		return nil, fmt.Errorf("capture exec target identity for PID %d: %w", containerPID, err)
	}

	targets := &execTargets{rootFD: -1, startTime: startTime}
	fail := func(err error) (*execTargets, error) {
		targets.close()
		return nil, err
	}

	rootPath := fmt.Sprintf("/proc/%d/root", containerPID)
	rootFD, err := unix.Open(rootPath, unix.O_PATH|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	if err != nil {
		return fail(fmt.Errorf("open %s: %w", rootPath, err))
	}
	targets.rootFD = rootFD

	nsSpecs := []struct {
		name string
		flag int
	}{
		{"net", unix.CLONE_NEWNET},
		{"ipc", unix.CLONE_NEWIPC},
		{"uts", unix.CLONE_NEWUTS},
		{"pid", unix.CLONE_NEWPID},
		{"mnt", unix.CLONE_NEWNS},
	}
	for _, spec := range nsSpecs {
		nsPath := fmt.Sprintf("/proc/%d/ns/%s", containerPID, spec.name)
		fd, err := unix.Open(nsPath, unix.O_RDONLY|unix.O_CLOEXEC, 0)
		if err != nil {
			if err == unix.EACCES || err == unix.EPERM {
				return fail(fmt.Errorf("cannot open %s: permission denied; run exec with sufficient namespace privileges", nsPath))
			}
			return fail(fmt.Errorf("open %s: %w (is the container still running?)", nsPath, err))
		}
		targets.ns = append(targets.ns, execNamespaceTarget{name: spec.name, flag: spec.flag, fd: fd})
	}

	endStartTime, err := ProcessStartTime(containerPID)
	if err != nil {
		return fail(fmt.Errorf("recheck exec target identity for PID %d: %w", containerPID, err))
	}
	if endStartTime != startTime {
		return fail(fmt.Errorf("exec target PID %d changed identity during namespace capture", containerPID))
	}
	return targets, nil
}

// ExecInit is the child side of Exec. The goroutine is pinned because setns is
// thread-scoped. After CLONE_NEWPID, spawning (rather than execve-ing in place)
// is mandatory: only a subsequently created child enters the target PID ns.
func ExecInit(containerPID int, _ string, command []string, debug bool) error {
	if len(command) == 0 || command[0] == "" {
		return fmt.Errorf("exec command is empty")
	}

	runtime.LockOSThread()

	if debug {
		fmt.Printf("[exec-init] attaching to namespaces of host PID %d\n", containerPID)
	}

	targets, err := openExecTargets(containerPID)
	if err != nil {
		return err
	}
	defer targets.close()

	for _, ns := range targets.ns {
		if err := unix.Setns(ns.fd, ns.flag); err != nil {
			return fmt.Errorf("setns(%s): %w", ns.name, err)
		}
		if debug {
			fmt.Printf("[exec-init] joined %s namespace\n", ns.name)
		}
	}

	// The root descriptor was opened before namespace transitions, so it still
	// refers to the exact target process root even after /proc now has the
	// container's view. This avoids a path-based chroot race.
	if err := unix.Fchdir(targets.rootFD); err != nil {
		return fmt.Errorf("fchdir container root: %w", err)
	}
	if err := unix.Chroot("."); err != nil {
		return fmt.Errorf("chroot container root: %w", err)
	}
	if err := unix.Chdir("/"); err != nil {
		return fmt.Errorf("chdir /: %w", err)
	}

	if debug {
		fmt.Printf("[exec-init] spawning payload in target PID namespace: %v\n", command)
	}

	err = runExecPayload(command, payloadEnvironment(os.Environ()), os.Stdin, os.Stdout, os.Stderr)
	if err == nil {
		return nil
	}
	if exitErr, ok := err.(*exec.ExitError); ok {
		// This process exists only as the exec shim. Preserve the payload's exit
		// code so the outer Exec call observes the same status.
		os.Exit(exitErr.ExitCode())
	}
	return fmt.Errorf("start exec payload: %w", err)
}

func runExecPayload(command, env []string, stdin io.Reader, stdout, stderr io.Writer) error {
	if len(command) == 0 || command[0] == "" {
		return fmt.Errorf("exec command is empty")
	}
	cmd := exec.Command(command[0], command[1:]...)
	cmd.Env = env
	cmd.Stdin = stdin
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	return cmd.Run()
}

func payloadEnvironment(env []string) []string {
	out := make([]string, 0, len(env))
	for _, entry := range env {
		if strings.HasPrefix(entry, execSentinelKey+"=") || strings.HasPrefix(entry, "MINICONTAINER_INIT=") {
			continue
		}
		out = append(out, entry)
	}
	return out
}

// IsRunning checks whether the process with the given host PID is still alive.
func IsRunning(pid int) bool {
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/status", pid))
	if err != nil {
		return false
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "State:") {
			return !strings.Contains(line, "Z") && !strings.Contains(line, "X")
		}
	}
	return true
}

// ContainerCwd returns the working directory of the container init process.
func ContainerCwd(pid int) string {
	cwd, err := filepath.EvalSymlinks(fmt.Sprintf("/proc/%d/cwd", pid))
	if err != nil {
		return "/"
	}
	return cwd
}
