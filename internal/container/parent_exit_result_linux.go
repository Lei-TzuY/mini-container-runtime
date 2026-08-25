//go:build linux

package container

import (
	"errors"
	"fmt"
	"os/exec"
)

// parentExitResult combines the payload result with authoritative parent-side
// teardown failures. Teardown failures are runtime-control failures: restart
// policies must not launch another generation while isolation cleanup or
// lifecycle finalization is incomplete. The original *exec.ExitError remains
// discoverable through errors.As when the payload itself exited non-zero.
func parentExitResult(waitErr, finalizationErr, bridgeCleanupErr error) error {
	var resultErr error
	if waitErr != nil {
		if exitErr, ok := waitErr.(*exec.ExitError); ok {
			resultErr = exitErr
		} else {
			resultErr = fmt.Errorf("container exited with error: %w", waitErr)
		}
	}
	if finalizationErr != nil {
		resultErr = errors.Join(resultErr, &runtimeSetupError{err: fmt.Errorf("finalize stopped process generation: %w", finalizationErr)})
	}
	if bridgeCleanupErr != nil {
		resultErr = errors.Join(resultErr, bridgeCleanupErr)
	}
	return resultErr
}
