//go:build linux

package container

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestStartContainerProcessRejectsRootFSReplacementBeforeSpawn(t *testing.T) {
	parent := t.TempDir()
	rootfs := filepath.Join(parent, "rootfs")
	if err := os.Mkdir(rootfs, 0o755); err != nil {
		t.Fatal(err)
	}
	admitted, err := os.Stat(rootfs)
	if err != nil {
		t.Fatal(err)
	}

	original := filepath.Join(parent, "original")
	if err := os.Rename(rootfs, original); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(rootfs, 0o755); err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command("/bin/sh", "-c", "exit 0")
	err = startContainerProcess(Config{RootFS: rootfs, RootFSIdentity: admitted}, cmd)
	if err == nil {
		t.Fatal("expected spawn-boundary rootfs identity rejection")
	}
	if !strings.Contains(err.Error(), "filesystem identity changed before runtime attempt") {
		t.Fatalf("unexpected error: %v", err)
	}
	if cmd.Process != nil {
		t.Fatalf("process was created despite rootfs identity drift: pid=%d", cmd.Process.Pid)
	}
}

func TestStartContainerProcessAllowsStableAdmittedRootFS(t *testing.T) {
	rootfs := t.TempDir()
	admitted, err := os.Stat(rootfs)
	if err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command("/bin/sh", "-c", "exit 0")
	if err := startContainerProcess(Config{RootFS: rootfs, RootFSIdentity: admitted}, cmd); err != nil {
		t.Fatalf("start stable rootfs process: %v", err)
	}
	if err := cmd.Wait(); err != nil {
		t.Fatalf("wait stable rootfs process: %v", err)
	}
}

func TestStartContainerProcessPreservesUnmanagedRunCompatibility(t *testing.T) {
	cmd := exec.Command("/bin/sh", "-c", "exit 0")
	if err := startContainerProcess(Config{}, cmd); err != nil {
		t.Fatalf("start unmanaged process: %v", err)
	}
	if err := cmd.Wait(); err != nil {
		t.Fatalf("wait unmanaged process: %v", err)
	}
}
