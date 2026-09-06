package main

import (
	"fmt"
	"os"

	"minicontainer/internal/container"
	"minicontainer/internal/state"
)

type restartCommandDeps struct {
	openStore func() (*state.Store, error)
	stat      func(string) (os.FileInfo, error)
	run       func(container.Config) error
}

func defaultRestartCommandDeps() restartCommandDeps {
	return restartCommandDeps{
		openStore: openStore,
		stat:      os.Stat,
		run:       container.Run,
	}
}

func parseRestartCommandArgs(args []string) (string, error) {
	if len(args) != 1 {
		return "", fmt.Errorf("expected exactly one container id")
	}
	if args[0] == "" {
		return "", fmt.Errorf("container id is empty")
	}
	return args[0], nil
}

func restartStoppedContainer(idOrPrefix string, deps restartCommandDeps) (*state.Container, error) {
	if deps.openStore == nil || deps.stat == nil || deps.run == nil {
		return nil, fmt.Errorf("restart command dependencies are incomplete")
	}

	st, err := deps.openStore()
	if err != nil {
		return nil, fmt.Errorf("open state store: %w", err)
	}
	if st == nil {
		return nil, fmt.Errorf("open state store returned nil store")
	}
	defer st.Close()

	rec, err := st.Resolve(idOrPrefix)
	if err != nil {
		return nil, err
	}
	rec, err = container.ReconcileContainerState(st, rec)
	if err != nil {
		return nil, fmt.Errorf("reconcile container %s before restart: %w", rec.ID, err)
	}
	if rec.Status != state.StatusStopped {
		return nil, fmt.Errorf("container %s is %s (must be stopped)", rec.ID, rec.Status)
	}

	spec, err := st.RestartSpec(rec.ID)
	if err != nil {
		return nil, fmt.Errorf("load restart spec for container %s: %w", rec.ID, err)
	}
	rootfsIdentity, err := deps.stat(spec.RootFS)
	if err != nil {
		return nil, fmt.Errorf("capture restart rootfs identity %q: %w", spec.RootFS, err)
	}
	if !rootfsIdentity.IsDir() {
		return nil, fmt.Errorf("restart rootfs %q is not a directory", spec.RootFS)
	}

	cfg := container.Config{
		ContainerID:    rec.ID,
		StateDir:       st.Dir(),
		RootFS:         spec.RootFS,
		RootFSIdentity: rootfsIdentity,
		Command:        append([]string(nil), spec.Command...),
		Env:            append([]string(nil), spec.Env...),
		WorkDir:        spec.WorkDir,
		Hostname:       spec.Hostname,
		UserNS:         true,
	}
	if err := deps.run(cfg); err != nil {
		return nil, fmt.Errorf("restart container %s: %w", rec.ID, err)
	}

	updated, err := st.Get(rec.ID)
	if err != nil {
		return nil, fmt.Errorf("reload restarted container %s: %w", rec.ID, err)
	}
	return updated, nil
}

func cmdRestartSafe(args []string) {
	id, err := parseRestartCommandArgs(args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "restart: %v\n", err)
		fmt.Fprintln(os.Stderr, "Usage: minictl restart <id>")
		return
	}

	rec, err := restartStoppedContainer(id, defaultRestartCommandDeps())
	if err != nil {
		fmt.Fprintf(os.Stderr, "restart error: %v\n", err)
		return
	}
	fmt.Printf("%s\n", shortContainerID(rec.ID))
}
