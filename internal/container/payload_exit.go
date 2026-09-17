package container

import (
	"errors"
	"fmt"
)

// PayloadExitError carries a supervised payload's exit status through the
// ContainerInit error-only API without misclassifying it as runtime setup
// failure. The CLI converts it back to the exact process exit status.
type PayloadExitError struct {
	Code int
}

func (e *PayloadExitError) Error() string {
	return fmt.Sprintf("container payload exited with status %d", e.Code)
}

func PayloadExitCode(err error) (int, bool) {
	var exitErr *PayloadExitError
	if !errors.As(err, &exitErr) {
		return 0, false
	}
	return exitErr.Code, true
}
