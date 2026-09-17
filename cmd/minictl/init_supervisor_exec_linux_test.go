//go:build linux

package main

import (
	"errors"
	"fmt"
	"testing"

	"golang.org/x/sys/unix"
)

func TestCloseInitSupervisorExecutablePathClosesPinnedDescriptor(t *testing.T) {
	fd, err := unix.Dup(0)
	if err != nil {
		t.Fatalf("duplicate stdin: %v", err)
	}
	if fd <= 2 {
		_ = unix.Close(fd)
		t.Fatalf("duplicate fd = %d, want descriptor above stdio", fd)
	}

	path := fmt.Sprintf("/proc/self/fd/%d", fd)
	if err := closeInitSupervisorExecutablePath(path); err != nil {
		_ = unix.Close(fd)
		t.Fatalf("closeInitSupervisorExecutablePath: %v", err)
	}
	if _, err := unix.FcntlInt(uintptr(fd), unix.F_GETFD, 0); !errors.Is(err, unix.EBADF) {
		if err == nil {
			_ = unix.Close(fd)
		}
		t.Fatalf("descriptor probe error = %v, want EBADF", err)
	}
}

func TestCloseInitSupervisorExecutablePathIgnoresNormalArgvZero(t *testing.T) {
	if err := closeInitSupervisorExecutablePath("/usr/bin/minictl"); err != nil {
		t.Fatalf("normal argv[0] should be ignored: %v", err)
	}
}

func TestCloseInitSupervisorExecutablePathRejectsInvalidDescriptor(t *testing.T) {
	for _, path := range []string{"/proc/self/fd/not-a-number", "/proc/self/fd/2"} {
		if err := closeInitSupervisorExecutablePath(path); err == nil {
			t.Fatalf("path %q unexpectedly accepted", path)
		}
	}
}
