//go:build linux

package container

import (
	"errors"
	"os/exec"
	"testing"
)

func TestRunRestartLoopOnFailureRetriesRealProcessToBudget(t *testing.T) {
	policy, err := ParseRestartPolicy("on-failure:2")
	if err != nil {
		t.Fatalf("ParseRestartPolicy: %v", err)
	}

	attempts := 0
	err = runRestartLoop(policy, func() (int, error) {
		attempts++
		cmd := exec.Command("/bin/sh", "-c", "exit 17")
		runErr := cmd.Run()
		var exitErr *exec.ExitError
		if !errors.As(runErr, &exitErr) {
			t.Fatalf("expected ExitError, got %v", runErr)
		}
		return exitErr.ExitCode(), runErr
	}, nil)
	if err == nil {
		t.Fatal("expected final process failure")
	}
	if attempts != 3 {
		t.Fatalf("attempts=%d, want 3 (initial + 2 retries)", attempts)
	}
}

func TestRunRestartLoopOnFailureStopsAfterRealProcessSuccess(t *testing.T) {
	policy, err := ParseRestartPolicy("on-failure:5")
	if err != nil {
		t.Fatalf("ParseRestartPolicy: %v", err)
	}

	attempts := 0
	err = runRestartLoop(policy, func() (int, error) {
		attempts++
		if attempts == 1 {
			cmd := exec.Command("/bin/sh", "-c", "exit 9")
			runErr := cmd.Run()
			var exitErr *exec.ExitError
			if !errors.As(runErr, &exitErr) {
				t.Fatalf("expected ExitError, got %v", runErr)
			}
			return exitErr.ExitCode(), runErr
		}
		if err := exec.Command("/bin/sh", "-c", "exit 0").Run(); err != nil {
			t.Fatalf("successful child: %v", err)
		}
		return 0, nil
	}, nil)
	if err != nil {
		t.Fatalf("runRestartLoop: %v", err)
	}
	if attempts != 2 {
		t.Fatalf("attempts=%d, want 2", attempts)
	}
}

func TestRunRestartLoopDoesNotRetryTerminalRuntimeError(t *testing.T) {
	policy, err := ParseRestartPolicy("always")
	if err != nil {
		t.Fatalf("ParseRestartPolicy: %v", err)
	}

	terminalErr := errors.New("terminal runtime control error")
	attempts := 0
	err = runRestartLoop(policy, func() (int, error) {
		attempts++
		return 125, terminalErr
	}, func(err error) bool {
		return errors.Is(err, terminalErr)
	})
	if !errors.Is(err, terminalErr) {
		t.Fatalf("error=%v, want terminal error", err)
	}
	if attempts != 1 {
		t.Fatalf("attempts=%d, want 1", attempts)
	}
}
