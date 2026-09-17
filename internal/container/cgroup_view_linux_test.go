//go:build linux

package container

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

func TestMountCgroupV2ViewBindsAndRemountsReadOnly(t *testing.T) {
	rootfs := "/staged/rootfs"
	var mkdirPath string
	type mountCall struct {
		source string
		target string
		flags  uintptr
	}
	var calls []mountCall
	err := mountCgroupV2ViewWithOps(rootfs, true, false, cgroupViewMountOps{
		mkdirAll: func(path string, mode os.FileMode) error {
			mkdirPath = path
			if mode != 0o755 {
				t.Fatalf("mkdir mode = %04o, want 0755", mode)
			}
			return nil
		},
		mount: func(source, target, fsType string, flags uintptr, data string) error {
			if fsType != "" || data != "" {
				t.Fatalf("unexpected mount fsType=%q data=%q", fsType, data)
			}
			calls = append(calls, mountCall{source: source, target: target, flags: flags})
			return nil
		},
	})
	if err != nil {
		t.Fatalf("mountCgroupV2ViewWithOps: %v", err)
	}
	wantTarget := filepath.Join(rootfs, "sys", "fs", "cgroup")
	if mkdirPath != wantTarget {
		t.Fatalf("mkdir path = %q, want %q", mkdirPath, wantTarget)
	}
	if len(calls) != 2 {
		t.Fatalf("mount calls = %d, want 2", len(calls))
	}
	if calls[0] != (mountCall{source: hostCgroupV2Mount, target: wantTarget, flags: syscall.MS_BIND}) {
		t.Fatalf("bind call = %#v", calls[0])
	}
	wantRemount := uintptr(syscall.MS_BIND | syscall.MS_REMOUNT | syscall.MS_RDONLY | syscall.MS_NOSUID | syscall.MS_NODEV | syscall.MS_NOEXEC)
	if calls[1] != (mountCall{target: wantTarget, flags: wantRemount}) {
		t.Fatalf("read-only remount call = %#v", calls[1])
	}
}

func TestMountCgroupV2ViewDisabledDoesNothing(t *testing.T) {
	called := false
	err := mountCgroupV2ViewWithOps("", false, false, cgroupViewMountOps{
		mkdirAll: func(string, os.FileMode) error { called = true; return nil },
		mount:    func(string, string, string, uintptr, string) error { called = true; return nil },
	})
	if err != nil || called {
		t.Fatalf("disabled mount err=%v called=%v", err, called)
	}
}

func TestMountCgroupV2ViewFailsClosedOnReadOnlyRemountError(t *testing.T) {
	cause := errors.New("remount denied")
	calls := 0
	err := mountCgroupV2ViewWithOps("/rootfs", true, false, cgroupViewMountOps{
		mkdirAll: func(string, os.FileMode) error { return nil },
		mount: func(string, string, string, uintptr, string) error {
			calls++
			if calls == 2 {
				return cause
			}
			return nil
		},
	})
	if !errors.Is(err, cause) {
		t.Fatalf("error = %v, want remount cause", err)
	}
}
