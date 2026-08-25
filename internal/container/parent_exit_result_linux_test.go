//go:build linux

package container

import (
	"errors"
	"os/exec"
	"testing"
)

func payloadExitError(t *testing.T, code string) *exec.ExitError {
	t.Helper()
	err := exec.Command("sh", "-c", "exit "+code).Run()
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("expected *exec.ExitError, got %T: %v", err, err)
	}
	return exitErr
}

func TestParentExitResultPreservesPayloadExitAndMarksCleanupFailureRuntimeControl(t *testing.T) {
	payloadErr := payloadExitError(t, "17")
	cleanupErr := errors.New("cgroup still populated")

	err := parentExitResult(payloadErr, cleanupErr, nil)
	if err == nil {
		t.Fatal("expected combined error")
	}
	if !errors.Is(err, cleanupErr) {
		t.Fatalf("cleanup error lost: %v", err)
	}
	var gotExit *exec.ExitError
	if !errors.As(err, &gotExit) || gotExit.ExitCode() != 17 {
		t.Fatalf("payload exit status lost: exit=%v err=%v", gotExit, err)
	}
	if !isRuntimeControlError(err) {
		t.Fatalf("cleanup failure must block restart: %v", err)
	}
}

func TestParentExitResultBridgeCleanupFailureBlocksRestart(t *testing.T) {
	bridgeErr := &runtimeSetupError{err: errors.New("remove veth")}
	err := parentExitResult(nil, nil, bridgeErr)
	if !errors.Is(err, bridgeErr) {
		t.Fatalf("bridge cleanup error lost: %v", err)
	}
	if !isRuntimeControlError(err) {
		t.Fatalf("bridge cleanup failure must block restart: %v", err)
	}
}

func TestParentExitResultCleanExitIsNil(t *testing.T) {
	if err := parentExitResult(nil, nil, nil); err != nil {
		t.Fatalf("clean exit returned error: %v", err)
	}
}
