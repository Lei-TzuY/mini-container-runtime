//go:build linux

package rootfs

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

const privateMqueueKernelChildEnv = "MINICONTAINER_MQUEUE_KERNEL_CHILD"

func TestPrivateMqueueMountKernel(t *testing.T) {
	if os.Getenv(privateMqueueKernelChildEnv) == "1" {
		root := os.Getenv("MINICONTAINER_MQUEUE_KERNEL_ROOT")
		if root == "" {
			t.Fatal("missing mqueue kernel root")
		}
		if err := syscall.Mount("", "/", "", syscall.MS_REC|syscall.MS_PRIVATE, ""); err != nil {
			if isRootfsNamespacePermissionError(err) {
				fmt.Fprintf(os.Stderr, "namespace permission denied: private mount namespace: %v\n", err)
				os.Exit(2)
			}
			t.Fatal(err)
		}

		dev := filepath.Join(root, "dev")
		if err := os.MkdirAll(dev, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := syscall.Mount("tmpfs", dev, "tmpfs", syscall.MS_NOSUID|syscall.MS_NOEXEC, "mode=0755,size=4m"); err != nil {
			if isRootfsNamespacePermissionError(err) {
				fmt.Fprintf(os.Stderr, "namespace permission denied: tmpfs mount: %v\n", err)
				os.Exit(2)
			}
			t.Fatal(err)
		}
		defer syscall.Unmount(dev, syscall.MNT_DETACH)

		mqueue := filepath.Join(dev, "mqueue")
		if err := os.MkdirAll(mqueue, 0o755); err != nil {
			t.Fatal(err)
		}
		flags := uintptr(syscall.MS_NOSUID | syscall.MS_NODEV | syscall.MS_NOEXEC)
		if err := syscall.Mount("mqueue", mqueue, "mqueue", flags, ""); err != nil {
			if isRootfsNamespacePermissionError(err) {
				fmt.Fprintf(os.Stderr, "namespace permission denied: mqueue mount: %v\n", err)
				os.Exit(2)
			}
			t.Fatal(err)
		}
		defer syscall.Unmount(mqueue, syscall.MNT_DETACH)

		data, err := os.ReadFile("/proc/self/mountinfo")
		if err != nil {
			t.Fatal(err)
		}
		for _, line := range strings.Split(string(data), "\n") {
			fields := strings.Fields(line)
			if len(fields) < 10 || fields[4] != mqueue {
				continue
			}
			separator := -1
			for i, field := range fields {
				if field == "-" {
					separator = i
					break
				}
			}
			if separator < 0 || separator+1 >= len(fields) || fields[separator+1] != "mqueue" {
				t.Fatalf("mountinfo entry for %s is not mqueue: %q", mqueue, line)
			}
			options := "," + fields[5] + ","
			for _, want := range []string{"nosuid", "noexec", "nodev"} {
				if !strings.Contains(options, ","+want+",") {
					t.Fatalf("mqueue mount options %q missing %s", fields[5], want)
				}
			}
			return
		}
		t.Fatalf("mqueue mount %q not found in mountinfo", mqueue)
	}

	root := t.TempDir()
	cmd := exec.Command(os.Args[0], "-test.run=^TestPrivateMqueueMountKernel$")
	cmd.Env = append(os.Environ(), privateMqueueKernelChildEnv+"=1", "MINICONTAINER_MQUEUE_KERNEL_ROOT="+root)
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
	if isRootfsNamespacePermissionError(err) {
		t.Skipf("kernel/user namespace startup is unavailable: %v", err)
	}
	if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 2 && strings.Contains(strings.ToLower(string(out)), "namespace permission denied") {
		t.Skipf("kernel/user namespace does not permit mqueue integration test: %s", strings.TrimSpace(string(out)))
	}
	t.Fatalf("mqueue kernel integration failed: %v: %s", err, strings.TrimSpace(string(out)))
}

func isRootfsNamespacePermissionError(err error) bool {
	return errors.Is(err, syscall.EPERM) || errors.Is(err, syscall.EACCES)
}
