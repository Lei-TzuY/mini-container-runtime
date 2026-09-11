package main

import (
	"fmt"
	"os"

	"minicontainer/internal/container"
	"minicontainer/internal/state"
)

func init() {
	if os.Getenv("MINICONTAINER_INIT") == "1" || os.Getenv("MINICONTAINER_EXEC") == "1" {
		return
	}
	if len(os.Args) >= 2 && os.Args[1] == "restart" {
		cmdRestartSafe(os.Args[2:])
		os.Exit(0)
	}
}

type restartCommandDeps struct {
	openStore func() (*state.Store, error)
	stat      func(string) (os.FileInfo, error)
	run       func(container.Config) error
}

func defaultRestartCommandDeps() restartCommandDeps {
	return restartCommandDeps{
		openStore: openStore,
		stat:      os.Stat,
		run:       container.RunWithSecurityPolicy,
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

	portMappings := make([]container.PortMapping, 0, len(spec.PortMappings))
	for _, p := range spec.PortMappings {
		portMappings = append(portMappings, container.PortMapping{
			HostPort:      p.HostPort,
			ContainerPort: p.ContainerPort,
			Protocol:      p.Protocol,
		})
	}
	volumes := make([]container.Volume, 0, len(spec.Volumes))
	for _, v := range spec.Volumes {
		volumes = append(volumes, container.Volume{
			HostPath:      v.HostPath,
			ContainerPath: v.ContainerPath,
			ReadOnly:      v.ReadOnly,
		})
	}
	tmpfsMounts := make([]container.TmpfsMount, 0, len(spec.TmpfsMounts))
	for _, mount := range spec.TmpfsMounts {
		tmpfsMounts = append(tmpfsMounts, container.TmpfsMount{
			ContainerPath: mount.ContainerPath,
			Options:       append([]string(nil), mount.Options...),
		})
	}

	cfg := container.Config{
		ContainerID:       rec.ID,
		StateDir:          st.Dir(),
		RootFS:            spec.RootFS,
		RootFSIdentity:    rootfsIdentity,
		Overlay:           spec.Overlay,
		ReadOnly:          spec.ReadOnly,
		Restart:           spec.Restart,
		CapDrop:           append([]string(nil), spec.CapDrop...),
		NoNewPrivileges:   spec.NoNewPrivileges,
		Command:           append([]string(nil), spec.Command...),
		Hostname:          spec.Hostname,
		WorkDir:           spec.WorkDir,
		Env:               append([]string(nil), spec.Env...),
		Memory:            spec.Memory,
		CPUWeight:         spec.CPUWeight,
		CPUs:              spec.CPUs,
		PidsLimit:         spec.PidsLimit,
		Seccomp:           spec.Seccomp,
		BridgeNetwork:     spec.BridgeNetwork,
		NetworkName:       spec.NetworkName,
		PortMappings:      portMappings,
		Volumes:           volumes,
		TmpfsMounts:       tmpfsMounts,
		MaskedDirectories: append([]string(nil), spec.MaskedDirectories...),
		UserNS:            spec.UserNS,
		CgroupNS:          spec.CgroupNS,
		Debug:             spec.Debug,
	}
	if spec.ProcessUserSet {
		cfg.ProcessUser = &container.ProcessUser{UID: spec.ProcessUID, GID: spec.ProcessGID, Groups: append([]uint32(nil), spec.ProcessGroups...)}
		if spec.ProcessUmaskSet {
			umask := spec.ProcessUmask
			cfg.ProcessUser.Umask = &umask
		}
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
		os.Exit(1)
	}

	rec, err := restartStoppedContainer(id, defaultRestartCommandDeps())
	if err != nil {
		fmt.Fprintf(os.Stderr, "restart error: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("%s\n", shortContainerID(rec.ID))
}
