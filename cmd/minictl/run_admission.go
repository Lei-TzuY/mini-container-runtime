package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"minicontainer/internal/container"
	"minicontainer/internal/state"
)

type runAdmissionDeps struct {
	openStore func() (*state.Store, error)
	newID     func() (string, error)
	now       func() time.Time
}

func prepareManagedRunState(cfg *container.Config) (*state.Store, *state.Container, error) {
	return prepareManagedRunStateWith(cfg, runAdmissionDeps{
		openStore: openStore,
		newID:     state.NewID,
		now:       time.Now,
	})
}

func prepareManagedRunStateWith(cfg *container.Config, deps runAdmissionDeps) (*state.Store, *state.Container, error) {
	if cfg == nil {
		return nil, nil, fmt.Errorf("run config is nil")
	}
	if cfg.ContainerID != "" {
		return nil, nil, fmt.Errorf("run config already has container ID %q", cfg.ContainerID)
	}
	rootfs, err := normalizeRunAdmissionRootFS(cfg.RootFS)
	if err != nil {
		return nil, nil, err
	}
	if deps.openStore == nil || deps.newID == nil || deps.now == nil {
		return nil, nil, fmt.Errorf("run admission dependencies are incomplete")
	}

	st, err := deps.openStore()
	if err != nil {
		return nil, nil, fmt.Errorf("open state store: %w", err)
	}
	if st == nil {
		return nil, nil, fmt.Errorf("open state store returned nil store")
	}
	fail := func(cause error) (*state.Store, *state.Container, error) {
		if closeErr := st.Close(); closeErr != nil {
			cause = errors.Join(cause, fmt.Errorf("close state store after run admission failure: %w", closeErr))
		}
		return nil, nil, cause
	}

	id, err := deps.newID()
	if err != nil {
		return fail(fmt.Errorf("generate container ID: %w", err))
	}

	rec := &state.Container{
		ID:        id,
		Status:    state.StatusCreated,
		RootFS:    rootfs,
		Command:   cfg.Command,
		Hostname:  cfg.Hostname,
		CreatedAt: deps.now(),
	}
	if err := st.Save(rec); err != nil {
		return fail(fmt.Errorf("persist created state for container %s: %w", id, err))
	}

	// Publishing the normalized rootfs and ID is the admission commit point.
	// In particular, an uncertain state write that returned an error must never
	// mutate the runtime config even if a filesystem entry happened to become
	// visible before that error.
	cfg.RootFS = rootfs
	cfg.ContainerID = id
	return st, rec, nil
}

func normalizeRunAdmissionRootFS(rootfs string) (string, error) {
	if rootfs == "" {
		return "", fmt.Errorf("run config rootfs is empty")
	}
	abs, err := filepath.Abs(rootfs)
	if err != nil {
		return "", fmt.Errorf("resolve run rootfs %q: %w", rootfs, err)
	}
	abs = filepath.Clean(abs)
	info, err := os.Stat(abs)
	if err != nil {
		return "", fmt.Errorf("stat run rootfs %q: %w", abs, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("run rootfs %q is not a directory", abs)
	}

	// Persist and execute the resolved target rather than a symlink-bearing
	// pathname. Otherwise a symlink retarget after durable admission could make
	// the runtime execute a different filesystem tree than the one recorded in
	// lifecycle state.
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", fmt.Errorf("resolve run rootfs symlinks %q: %w", abs, err)
	}
	resolved = filepath.Clean(resolved)
	info, err = os.Stat(resolved)
	if err != nil {
		return "", fmt.Errorf("stat resolved run rootfs %q: %w", resolved, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("resolved run rootfs %q is not a directory", resolved)
	}
	return resolved, nil
}
