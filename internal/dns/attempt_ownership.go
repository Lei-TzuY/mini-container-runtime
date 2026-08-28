package dns

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync"
)

type attemptOwnershipKey struct {
	networkName string
	containerID string
}

var (
	attemptOwnershipMu sync.Mutex
	attemptOwners      = make(map[attemptOwnershipKey]string)
)

func newAttemptToken() (string, error) {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", fmt.Errorf("generate DNS attempt ownership token: %w", err)
	}
	return hex.EncodeToString(raw[:]), nil
}

// BeginHostRegistrationAttempt registers service discovery for one runtime
// attempt and returns an exact-attempt rollback. The persistent DNS entry keeps
// registrar process identity for crash recovery, while this process-local opaque
// token distinguishes sequential restart attempts launched by that same
// registrar. A stale rollback therefore cannot consume a newer attempt's
// otherwise-identical registration.
func BeginHostRegistrationAttempt(networkName, containerID, hostname, ipAddr string) (func() error, error) {
	token, err := newAttemptToken()
	if err != nil {
		return nil, err
	}
	key := attemptOwnershipKey{networkName: networkName, containerID: containerID}
	return beginHostRegistrationAttemptWith(
		key,
		token,
		func() error { return RegisterHost(networkName, containerID, hostname, ipAddr) },
		func() error { return UnregisterHostOwned(networkName, containerID) },
	)
}

func beginHostRegistrationAttemptWith(key attemptOwnershipKey, token string, register, unregister func() error) (func() error, error) {
	if token == "" {
		return nil, fmt.Errorf("DNS attempt ownership token cannot be empty")
	}
	if register == nil || unregister == nil {
		return nil, fmt.Errorf("DNS attempt ownership callbacks are incomplete")
	}

	// Serialize registration and token publication with rollback. In particular,
	// a newer same-registrar attempt cannot publish its token between an older
	// rollback's ownership check and persistent unregister.
	attemptOwnershipMu.Lock()
	defer attemptOwnershipMu.Unlock()
	if err := register(); err != nil {
		return nil, err
	}
	attemptOwners[key] = token

	rollback := func() error {
		attemptOwnershipMu.Lock()
		defer attemptOwnershipMu.Unlock()
		if attemptOwners[key] != token {
			return nil
		}
		if err := unregister(); err != nil {
			// Preserve ownership on failure so the exact attempt may retry cleanup.
			return err
		}
		delete(attemptOwners, key)
		return nil
	}
	return rollback, nil
}
