//go:build linux

package rootfs

import (
	"errors"
	"strings"
	"testing"
)

func TestIsolateWithPivotFailsClosed(t *testing.T) {
	cause := errors.New("pivot unavailable")
	calls := 0

	err := isolateWithPivot("/fake/root", false, func(newRoot string, debug bool) error {
		calls++
		if newRoot != "/fake/root" {
			t.Fatalf("newRoot=%q", newRoot)
		}
		if debug {
			t.Fatal("unexpected debug=true")
		}
		return cause
	})

	if calls != 1 {
		t.Fatalf("pivot calls=%d, want exactly 1", calls)
	}
	if !errors.Is(err, cause) {
		t.Fatalf("pivot failure not preserved: %v", err)
	}
	if !strings.Contains(err.Error(), "pivot_root isolation required") {
		t.Fatalf("failure does not explain fail-closed requirement: %v", err)
	}
}

func TestIsolateWithPivotSucceedsOnlyAfterPivotSuccess(t *testing.T) {
	calls := 0
	err := isolateWithPivot("/fake/root", true, func(newRoot string, debug bool) error {
		calls++
		if newRoot != "/fake/root" || !debug {
			t.Fatalf("unexpected arguments: root=%q debug=%v", newRoot, debug)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("isolateWithPivot: %v", err)
	}
	if calls != 1 {
		t.Fatalf("pivot calls=%d, want 1", calls)
	}
}

func TestIsolateWithPivotRejectsNilImplementation(t *testing.T) {
	err := isolateWithPivot("/fake/root", false, nil)
	if err == nil || !strings.Contains(err.Error(), "pivot_root isolation function is nil") {
		t.Fatalf("nil pivot error=%v", err)
	}
}
