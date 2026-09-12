//go:build linux

package container

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"testing"
)

const procMountKernelChildEnv = "MINICONTAINER_PROC_MOUNT_KERNEL_CHILD"

func TestProcMountFlagsKernel(t *testing.T) {
	if os.Getenv(procMountKernelChildEnv) == "1" {
		target := os.Getenv("MINICONTAINER_PROC_MOUNT_KERNEL_TARGET")
		if target == "" {
			t.Fatal("missing proc mount target")
		}
		flags, err := procMountFlags([]string{"rw", "nosuid", "noexec", "nodev"})
		if err != nil {
			t.Fatal(err)
		}
		if err := syscall.Mount("", "/", "", syscall.MS_REC|syscall.MS_PRIVATE, ""); err != nil {
			if err == syscall.EPERM {
				fmt.Fprintln(os.Stderr, "operation not permitted: private mount namespace")
				os.Exit(2)
			}
			t.Fatal(err)
		}
		if err := syscall.Mount("proc", target, "proc", flags, ""); err != nil {
			if err == syscall.EPERM {
				fmt.Fprintln(os.Stderr, "operation not permitted: proc mount")
				os.Exit(2)
			}
			t.Fatal(err)
		}
		defer syscall.Unmount(target, syscall.MNT_DETACH)

		data, err := os.ReadFile("/proc/self/mountinfo")
		if err != nil {
			t.Fatal(err)
		}
		for _, line := range strings.Split(string(data), "\n") {
			fields := strings.Fields(line)
			if len(fields) < 6 || fields[4] != target {
				continue
			}
			options := "," + fields[5] + ","
			for _, want := range []string{"nosuid", "noexec", "nodev"} {
				if !strings.Contains(options, ","+want+",") {
					t.Fatalf("mount options %q missing %s", fields[5], want)
				}
			}
			return
		}
		t.Fatalf("proc mount %q not found in mountinfo", target)
	}

	target := t.TempDir()
	cmd := exec.Command(os.Args[0], "-test.run=^TestProcMountFlagsKernel$")
	cmd.Env = append(os.Environ(), procMountKernelChildEnv+"=1", "MINICONTAINER_PROC_MOUNT_KERNEL_TARGET="+target)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Cloneflags: syscall.CLONE_NEWUSER | syscall.CLONE_NEWNS | syscall.CLONE_NEWPID,
		UidMappings: []syscall.SysProcIDMap{{ContainerID: 0, HostID: os.Getuid(), Size: 1}},
		GidMappings: []syscall.SysProcIDMap{{ContainerID: 0, HostID: os.Getgid(), Size: 1}},
		GidMappingsEnableSetgroups: false,
	}
	out, err := cmd.CombinedOutput()
	if err == nil {
		return
	}
	if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 2 && strings.Contains(strings.ToLower(string(out)), "operation not permitted") {
		t.Skipf("kernel/user namespace does not permit proc mount integration test: %s", strings.TrimSpace(string(out)))
	}
	t.Fatalf("proc mount kernel integration failed: %v: %s", err, strings.TrimSpace(string(out)))
}
