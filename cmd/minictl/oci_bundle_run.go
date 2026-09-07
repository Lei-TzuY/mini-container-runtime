package main

import (
	"errors"
	"fmt"
	"time"

	"minicontainer/internal/container"
	"minicontainer/internal/state"
)

type ociBundleRunDeps struct {
	load    func(string) (container.Config, error)
	prepare func(*container.Config) (*state.Store, *state.Container, error)
	run     func(container.Config) error
	settle  func(*state.Store, string, error, time.Time) (*state.Container, error)
	now     func() time.Time
}

func runOCIBundle(bundle string) (string, error) {
	return runOCIBundleWith(bundle, ociBundleRunDeps{
		load:    loadOCIBundle,
		prepare: prepareManagedRunState,
		run:     container.RunWithSecurityPolicy,
		settle:  settleRunCommandState,
		now:     time.Now,
	})
}

func runOCIBundleWith(bundle string, deps ociBundleRunDeps) (string, error) {
	if deps.load == nil || deps.prepare == nil || deps.run == nil || deps.settle == nil || deps.now == nil {
		return "", fmt.Errorf("OCI bundle run dependencies are incomplete")
	}
	cfg, err := deps.load(bundle)
	if err != nil {
		return "", err
	}
	store, rec, err := deps.prepare(&cfg)
	if err != nil {
		return "", fmt.Errorf("prepare OCI bundle state: %w", err)
	}
	if store == nil || rec == nil || rec.ID == "" {
		if store != nil {
			_ = store.Close()
		}
		return "", fmt.Errorf("prepare OCI bundle state returned incomplete managed state")
	}
	defer store.Close()

	runErr := deps.run(cfg)
	_, settleErr := deps.settle(store, rec.ID, runErr, deps.now())
	if runErr != nil || settleErr != nil {
		return rec.ID, errors.Join(runErr, settleErr)
	}
	return rec.ID, nil
}
