package container

import "fmt"

// runRestartLoop executes attempts according to a parsed restart policy.
// The attempt callback returns the payload exit code plus the runtime error for
// that generation. terminal identifies control/admission errors that must never
// be retried. There is deliberately no retry sleep here: lifecycle policy is
// independent from backoff and remains deterministic under test.
func runRestartLoop(policy RestartPolicy, attempt func() (int, error), terminal func(error) bool) error {
	if attempt == nil {
		return fmt.Errorf("restart attempt callback is nil")
	}

	retries := 0
	for {
		exitCode, err := attempt()
		if err != nil && terminal != nil && terminal(err) {
			return err
		}
		if !ShouldRestart(policy, exitCode, retries) {
			return err
		}
		retries++
	}
}
