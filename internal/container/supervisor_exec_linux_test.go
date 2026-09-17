//go:build linux

package container

import (
	"fmt"
	"os"
	"testing"

	"golang.org/x/sys/unix"
)

func TestInitSupervisorExecutablePathPreservesDescriptorAcrossExec(t *testing.T) {
	file, err := os.Open("/proc/self/exe")
	if err != nil {
		t.Fatalf("open current executable: %v", err)
	}
	defer file.Close()

	path, err := initSupervisorExecutablePath(file)
	if err != nil {
		t.Fatalf("initSupervisorExecutablePath: %v", err)
	}
	wantPath := fmt.Sprintf("/proc/self/fd/%d", file.Fd())
	if path != wantPath {
		t.Fatalf("path = %q, want %q", path, wantPath)
	}

	flags, err := unix.FcntlInt(file.Fd(), unix.F_GETFD, 0)
	if err != nil {
		t.Fatalf("inspect descriptor flags: %v", err)
	}
	if flags&unix.FD_CLOEXEC != 0 {
		t.Fatal("pinned supervisor executable descriptor still has FD_CLOEXEC")
	}
}

func TestInitSupervisorExecutablePathRejectsNilFile(t *testing.T) {
	if _, err := initSupervisorExecutablePath(nil); err == nil {
		t.Fatal("expected nil supervisor executable to fail")
	}
}
