//go:build linux

package container

import (
	"errors"
	"os/exec"
	"testing"
)

func TestExitCodeFromWaitError(t *testing.T) {
	if got := exitCodeFromWaitError(nil); got != 0 {
		t.Fatalf("nil error exit code = %d, want 0", got)
	}
	if got := exitCodeFromWaitError(errors.New("runtime failure")); got != 1 {
		t.Fatalf("generic error exit code = %d, want 1", got)
	}
}

func TestRuntimeStateErrorIsDiscoverableThroughJoin(t *testing.T) {
	stateErr := &runtimeStateError{err: errors.New("state failed")}
	joined := errors.Join(errors.New("payload failed"), stateErr)
	var got *runtimeStateError
	if !errors.As(joined, &got) || got != stateErr {
		t.Fatalf("runtime state error not discoverable through errors.Join: %v", joined)
	}
}

func TestExitCodeFromRealProcess(t *testing.T) {
	cmd := exec.Command("sh", "-c", "exit 23")
	err := cmd.Run()
	if err == nil {
		t.Fatal("expected non-zero exit")
	}
	if got := exitCodeFromWaitError(err); got != 23 {
		t.Fatalf("exit code = %d, want 23", got)
	}
}
