//go:build linux

// internal/container/exec_linux.go
//
// `minictl exec` — Enter an Existing Container
// ─────────────────────────────────────────────
// Once a container is running, you can execute additional processes inside
// its namespaces without re-doing all the setup.  This is what `docker exec`
// does under the hood.
//
// The mechanism: setns(2)
// ────────────────────────
// setns(2) attaches the calling thread/process to an already-existing
// namespace identified by a file descriptor opened from the namespace's
// /proc/<pid>/ns/<type> entry.
//
//   For a running container with host PID = 4321, its namespaces live at:
//     /proc/4321/ns/pid    ← PID namespace
//     /proc/4321/ns/uts    ← UTS namespace
//     /proc/4321/ns/mnt    ← mount namespace
//     /proc/4321/ns/net    ← network namespace
//     /proc/4321/ns/ipc    ← IPC namespace
//
//   We open each one, call setns() with the appropriate CLONE_NEW* flag,
//   then chroot into the container's rootfs (which is already set up).
//
// Why must exec re-enter the user namespace last?
// ─────────────────────────────────────────────────
// The user namespace controls the capability set.  When running as root we
// skip re-entering it.  When running unprivileged we must enter it first
// (because other namespaces require capabilities inside the user namespace)
// — but the user namespace file is typically unreadable without privilege.
// For simplicity this implementation re-enters all namespaces with the
// effective UID of the caller.
//
// Why is exec harder than run?
// ─────────────────────────────
// The Go runtime uses multiple OS threads. setns() affects only one thread.
// If the Go scheduler moves the goroutine to a different thread, the setns()
// effect is lost.  The standard solution is to use a C constructor (init_)
// that runs setns() before the Go runtime starts — exactly what runc does.
// We sidestep this by re-exec'ing ourselves with MINICONTAINER_EXEC=1 and
// using runtime.LockOSThread() to pin the goroutine to its OS thread.

package container

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"

	"golang.org/x/sys/unix"
)

// ExecConfig describes a command to be run inside an existing container.
type ExecConfig struct {
	// ContainerPID is the host PID of the container's init process.
	ContainerPID int

	// RootFS is the container's root directory (for chroot after setns).
	RootFS string

	// Command is the argv to exec inside the container.
	Command []string

	// Debug enables verbose logging.
	Debug bool
}

// execSentinelEnv is set on the re-exec child to trigger Exec init.
const execSentinelEnv = "MINICONTAINER_EXEC=1"

// Exec re-execs the current binary as a child, enters the running container's
// namespaces via setns(2), and then execve(2)s the requested command.
//
// This is safe because we re-exec ourselves: the child binary will detect
// execSentinelEnv and call ExecInit() before Go's scheduler can interfere.
func Exec(cfg ExecConfig) error {
	if cfg.Debug {
		fmt.Printf("[exec] entering namespaces of PID %d\n", cfg.ContainerPID)
	}

	self, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve executable: %w", err)
	}

	// Pass the config to the child via environment variables + args.
	// Arg0 = "exec" (subcommand), Arg1 = pid, Arg2... = command.
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

// ExecInit is the child side of Exec.  It is called when the binary detects
// execSentinelEnv.  It runs with runtime.LockOSThread() to ensure setns()
// affects the correct OS thread throughout.
func ExecInit(containerPID int, rootfs string, command []string, debug bool) error {
	// Pin this goroutine permanently to its OS thread.
	// setns(2) operates on the calling thread; Go's M:N scheduler would
	// otherwise move us to a different thread that hasn't called setns().
	runtime.LockOSThread()

	if debug {
		fmt.Printf("[exec-init] attaching to namespaces of host PID %d\n", containerPID)
	}

	// The namespaces we want to enter, in order.
	// Order matters: mnt must come after pid so /proc can be resolved.
	// net and ipc are independent and can be done in any order.
	nsTypes := []struct {
		name string
		flag uintptr
	}{
		{"net", syscall.CLONE_NEWNET},
		{"ipc", syscall.CLONE_NEWIPC},
		{"uts", syscall.CLONE_NEWUTS},
		{"pid", syscall.CLONE_NEWPID},
		{"mnt", syscall.CLONE_NEWNS},
	}

	for _, ns := range nsTypes {
		nsPath := fmt.Sprintf("/proc/%d/ns/%s", containerPID, ns.name)
		fd, err := os.Open(nsPath)
		if err != nil {
			if os.IsPermission(err) {
				return fmt.Errorf("cannot open %s: permission denied\n"+
					"  Hint: run 'minictl exec' as root (sudo) when not using user namespaces", nsPath)
			}
			// Namespace file not found — process may have exited.
			return fmt.Errorf("open %s: %w (is the container still running?)", nsPath, err)
		}

		// setns(fd, nstype) — attaches the current thread to the namespace.
		// nstype is a CLONE_NEW* flag that validates the fd's type.
		// Using 0 as nstype would work too but the flag makes the code explicit.
		if err := unix.Setns(int(fd.Fd()), int(ns.flag)); err != nil {
			fd.Close()
			return fmt.Errorf("setns(%s): %w", ns.name, err)
		}
		fd.Close()

		if debug {
			fmt.Printf("[exec-init] joined %s namespace\n", ns.name)
		}
	}

	// After joining the mount namespace we are inside the container's view
	// of the filesystem, but the CWD and root are still the host's.
	// We need to chroot into the container's rootfs to resolve binaries.
	if err := syscall.Chroot(rootfs); err != nil {
		return fmt.Errorf("chroot %s: %w", rootfs, err)
	}
	if err := syscall.Chdir("/"); err != nil {
		return fmt.Errorf("chdir /: %w", err)
	}

	if debug {
		fmt.Printf("[exec-init] chroot'd into %s, exec'ing %v\n", rootfs, command)
	}

	// Resolve the binary in the new root.
	binary, err := exec.LookPath(command[0])
	if err != nil {
		binary = command[0]
	}

	// Replace the current process image.  The exec'd process lives inside
	// the container's namespaces and sees the container's filesystem.
	if err := syscall.Exec(binary, command, os.Environ()); err != nil {
		return fmt.Errorf("exec %s: %w", binary, err)
	}
	return nil // unreachable
}

// IsRunning checks whether the process with the given host PID is still alive.
// It reads /proc/<pid>/status rather than sending a signal, so it doesn't
// inadvertently affect the process.
func IsRunning(pid int) bool {
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/status", pid))
	if err != nil {
		return false
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "State:") {
			// State values: R (running), S (sleeping), D (disk-wait),
			//               T (stopped), Z (zombie), X (dead)
			return !strings.Contains(line, "Z") && !strings.Contains(line, "X")
		}
	}
	return true
}

// ContainerCwd returns the working directory of the container init process.
// This is read from /proc/<pid>/cwd, which is a symlink to the CWD.
func ContainerCwd(pid int) string {
	cwd, err := filepath.EvalSymlinks(fmt.Sprintf("/proc/%d/cwd", pid))
	if err != nil {
		return "/"
	}
	return cwd
}
