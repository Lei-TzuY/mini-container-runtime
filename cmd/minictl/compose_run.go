package main

import (
	"errors"
	"fmt"
	"time"

	"minicontainer/internal/container"
	"minicontainer/internal/state"
)

type composeRunDeps struct {
	admit  func(*container.Config) (*state.Store, *state.Container, error)
	run    func(container.Config) error
	settle func(*state.Store, string, error, time.Time) (*state.Container, error)
	now    func() time.Time
}

func runManagedComposeService(cfg container.Config) (*state.Container, error) {
	return runManagedComposeServiceWith(cfg, composeRunDeps{
		admit:  prepareManagedRunState,
		run:    container.Run,
		settle: settleRunCommandState,
		now:    time.Now,
	})
}

func runManagedComposeServiceWith(cfg container.Config, deps composeRunDeps) (*state.Container, error) {
	if deps.admit == nil || deps.run == nil || deps.settle == nil || deps.now == nil {
		return nil, fmt.Errorf("compose run dependencies are incomplete")
	}

	st, admitted, err := deps.admit(&cfg)
	if err != nil {
		return nil, fmt.Errorf("admit compose service: %w", err)
	}
	if st == nil || admitted == nil || admitted.ID == "" || cfg.ContainerID != admitted.ID {
		if st != nil {
			_ = st.Close()
		}
		return admitted, fmt.Errorf("compose service admission returned inconsistent lifecycle state")
	}

	runErr := deps.run(cfg)
	settled, settleErr := deps.settle(st, admitted.ID, runErr, deps.now())
	closeErr := st.Close()
	if closeErr != nil {
		closeErr = fmt.Errorf("close compose service state store: %w", closeErr)
	}

	return settled, errors.Join(runErr, settleErr, closeErr)
}
