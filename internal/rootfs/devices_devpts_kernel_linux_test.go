//go:build linux

package rootfs

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

const privateDevptsKernelChildEnv = "MINICONTAINER_DEVPTS_KERNEL_CHILD"

func TestPrivateDevptsMountKernel(t *testing.T) {
	if os.Getenv(privateDevptsKernelChildEnv) == "1" {
		root := os.Getenv("MINICONTAINER_DEVPTS_KERNEL_ROOT")
		if root == "" {
			t.Fatal("missing devpts kernel root")
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

		if err := mountPrivateDevpts(dev, defaultDeviceMountOps()); err != nil {
			if isRootfsNamespacePermissionError(err) {
				fmt.Fprintf(os.Stderr, "namespace permission denied: devpts mount: %v\n", err)
				os.Exit(2)
			}
			t.Fatal(err)
		}
		pts := filepath.Join(dev, "pts")
		defer syscall.Unmount(pts, syscall.MNT_DETACH)

		target, err := os.Readlink(filepath.Join(dev, "ptmx"))
		if err != nil {
			t.Fatal(err)
		}
		if target != "pts/ptmx" {
			t.Fatalf("/dev/ptmx -> %q, want pts/ptmx", target)
		}

		data, err := os.ReadFile("/proc/self/mountinfo")
		if err != nil {
			t.Fatal(err)
		}
		for _, line := range strings.Split(string(data), "\n") {
			fields := strings.Fields(line)
			if len(fields) < 10 || fields[4] != pts {
				continue
			}
			separator := -1
			for i, field := range fields {
				if field == "-" {
					separator = i
					break
				}
			}
			if separator < 0 || separator+1 >= len(fields) || fields[separator+1] != "devpts" {
				t.Fatalf("mountinfo entry for %s is not devpts: %q", pts, line)
			}
			options := "," + fields[5] + ","
			for _, want := range []string{"nosuid", "noexec"} {
				if !strings.Contains(options, ","+want+",") {
					t.Fatalf("devpts mount options %q missing %s", fields[5], want)
				}
			}
			return
		}
		t.Fatalf("devpts mount %q not found in mountinfo", pts)
	}

	root := t.TempDir()
	cmd := exec.Command(os.Args[0], "-test.run=^TestPrivateDevptsMountKernel$")
	cmd.Env = append(os.Environ(), privateDevptsKernelChildEnv+"=1", "MINICONTAINER_DEVPTS_KERNEL_ROOT="+root)
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
		t.Skipf("kernel/user namespace does not permit devpts integration test: %s", strings.TrimSpace(string(out)))
	}
	t.Fatalf("devpts kernel integration failed: %v: %s", err, strings.TrimSpace(string(out)))
}
