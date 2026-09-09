//go:build linux

package rootfs

import (
	"errors"
	"os"
	"syscall"
	"testing"
)

func TestPivotRootMakesMountTreePrivateBeforeRootfsBind(t *testing.T) {
	ops := successfulPivotRootOps()
	type mountCall struct {
		source string
		target string
		flags  uintptr
	}
	var calls []mountCall
	ops.mount = func(source, target, fstype string, flags uintptr, data string) error {
		calls = append(calls, mountCall{source: source, target: target, flags: flags})
		return nil
	}

	if err := pivotRootWithOps("/fake/root", false, ops); err != nil {
		t.Fatalf("pivotRootWithOps: %v", err)
	}
	if len(calls) != 2 {
		t.Fatalf("mount calls=%v, want propagation gate then rootfs bind", calls)
	}
	if got := calls[0]; got.source != "" || got.target != "/" || got.flags != syscall.MS_REC|syscall.MS_PRIVATE {
		t.Fatalf("first mount=%+v, want recursive private /", got)
	}
	if got := calls[1]; got.source != "/fake/root" || got.target != "/fake/root" || got.flags != syscall.MS_BIND|syscall.MS_REC {
		t.Fatalf("second mount=%+v, want recursive rootfs self-bind", got)
	}
}

func TestPivotRootFailsClosedWhenPrivatePropagationFails(t *testing.T) {
	cause := errors.New("private propagation denied")
	ops := successfulPivotRootOps()
	mountCalls := 0
	mkdirCalls := 0
	pivotCalls := 0
	ops.mount = func(source, target, fstype string, flags uintptr, data string) error {
		mountCalls++
		return cause
	}
	ops.mkdir = func(path string, mode os.FileMode) error {
		mkdirCalls++
		return nil
	}
	ops.pivot = func(newRoot, putOld string) error {
		pivotCalls++
		return nil
	}

	err := pivotRootWithOps("/fake/root", false, ops)
	if !errors.Is(err, cause) {
		t.Fatalf("error=%v, want propagation cause", err)
	}
	if mountCalls != 1 || mkdirCalls != 0 || pivotCalls != 0 {
		t.Fatalf("calls mount=%d mkdir=%d pivot=%d; rootfs changed after propagation failure", mountCalls, mkdirCalls, pivotCalls)
	}
}
