//go:build !linux

package state

import (
	"fmt"
	"os"
)

func openStateLock(path string) (*os.File, error) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open state lock: %w", err)
	}
	return file, nil
}

// Non-Linux builds retain the existing process-local mutex semantics. The
// container runtime itself is Linux-only; these stubs keep utility packages and
// tests portable without pretending to provide a cross-process guarantee.
func lockStateFile(file *os.File) error   { return nil }
func unlockStateFile(file *os.File) error { return nil }
