//go:build linux

package container

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
)

// abortPreRunningChildFailure is the fail-closed boundary for failures after
// cmd.Start succeeds but before the process generation is durably admitted as
// running. The runtime must prove the blocked child was reaped before returning;
// otherwise it preserves the original lifecycle evidence and reports a runtime
// control failure rather than pretending startup cleanly aborted.
func abortPreRunningChildFailure(cmd *exec.Cmd, writePipe *os.File, cause error) error {
	resultErr := cause
	if resultErr == nil {
		resultErr = &runtimeStateError{err: fmt.Errorf("pre-running child startup failed")}
	}

	reaped, abortErr := abortBlockedChildChecked(cmd, writePipe)
	if abortErr != nil {
		resultErr = errors.Join(resultErr, &runtimeSetupError{err: fmt.Errorf("abort pre-running child: %w", abortErr)})
	}
	if !reaped {
		resultErr = errors.Join(resultErr, &runtimeSetupError{err: fmt.Errorf("pre-running child was not confirmed reaped; preserving created lifecycle state for recovery")})
	}
	return resultErr
}
