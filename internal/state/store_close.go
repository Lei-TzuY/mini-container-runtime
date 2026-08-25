package state

import (
	"errors"
	"fmt"
)

// Close releases the cross-process lock descriptor and any filesystem pins held
// by the Store. Close is idempotent. Callers must not use the Store after Close.
func (s *Store) Close() error {
	if s == nil {
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	var closeErrs []error
	if s.lockFile != nil {
		if err := s.lockFile.Close(); err != nil {
			closeErrs = append(closeErrs, fmt.Errorf("close state lock: %w", err))
		}
		s.lockFile = nil
	}
	for i := len(s.storagePins) - 1; i >= 0; i-- {
		if s.storagePins[i] == nil {
			continue
		}
		if err := s.storagePins[i].Close(); err != nil {
			closeErrs = append(closeErrs, fmt.Errorf("close state storage pin: %w", err))
		}
	}
	s.storagePins = nil

	return errors.Join(closeErrs...)
}
