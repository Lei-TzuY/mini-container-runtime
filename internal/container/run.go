//go:build linux

// internal/container/run.go
//
// Container Runtime — Process Creation and Namespace Isolation

package container

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"minicontainer/internal/cgroups"
	"minicontainer/internal/network"
	"minicontainer/internal/ns"
	"minicontainer/internal/rootfs"
)

const sentinelEnv = "MINICONTAINER_INIT=1"

// Run launches a new container, optionally handles restart policies.
func Run(cfg Config) error {
	maxAttempts := 1
	if cfg.Restart == "always" || cfg.Restart == "on-failure" {
		maxAttempts = 5
	}

	attempt := 0
	for {
		attempt++
		err := runOnce(cfg)
		if err == nil {
			if cfg.Restart == "always" && attempt < maxAttempts {
				if cfg.Debug {
					fmt.Printf("[parent] restart policy %q: restarting container (attempt %d)\n", cfg.Restart, attempt+1)
				}
				time.Sleep(1 * time.Second)
				continue
			}
			return nil
		}

		if (cfg.Restart == "always" || cfg.Restart == "on-failure") && attempt < maxAttempts {
			if cfg.Debug {
				fmt.Printf("[parent] container failed (%v); restart policy %q: retrying (attempt %d)\n", err, cfg.Restart, attempt+1)
			}
			time.Sleep(1 * time.Second)
			continue
		}
		return err
	}
}

func runOnce(cfg Config) error {
	if cfg.Debug {
		fmt.Println("[parent] spawning child with new namespaces")
	}

	self, err := os.Executable()
	if err != nil {
		return fmt.Errorf("could not resolve executable path: %w", err)
	}

	childArgs := []string{"run"}
	if !cfg.UserNS {
		childArgs = append(childArgs, "--no-user-ns")
	}
	if cfg.Hostname != "" {
		childArgs = append(childArgs, "--hostname", cfg.Hostname)
	}
	if cfg.WorkDir != "" {
		childArgs = append(childArgs, "--workdir", cfg.WorkDir)
	}
	if cfg.Overlay {
		childArgs = append(childArgs, "--overlay")
	}
	if cfg.ReadOnly {
		childArgs = append(childArgs, "--read-only")
	}
	for _, c := range cfg.CapDrop {
		childArgs = append(childArgs, "--cap-drop", c)
	}
	for _, env := range cfg.Env {
		childArgs = append(childArgs, "--env", env)
	}
	if cfg.Memory > 0 {
		childArgs = append(childArgs, "--memory", strconv.FormatInt(cfg.Memory, 10))
	}
	if cfg.CPUWeight > 0 {
		childArgs = append(childArgs, "--cpu-weight", strconv.FormatInt(cfg.CPUWeight, 10))
	}
	if cfg.CPUs > 0 {
		childArgs = append(childArgs, "--cpus", strconv.FormatFloat(cfg.CPUs, 'f', -1, 64))
	}
	if cfg.PidsLimit > 0 {
		childArgs = append(childArgs, "--pids-limit", strconv.FormatInt(cfg.PidsLimit, 10))
	}
	if cfg.BridgeNetwork {
		childArgs = append(childArgs, "--bridge")
	}
	if cfg.Seccomp {
		childArgs = append(childArgs, "--seccomp")
	}
	for _, p := range cfg.PortMappings {
		spec := fmt.Sprintf("%d:%d", p.HostPort, p.ContainerPort)
		if p.Protocol != "" && p.Protocol != "tcp" {
			spec += "/" + p.Protocol
		}
		childArgs = append(childArgs, "--publish", spec)
	}
	for _, v := range cfg.Volumes {
		spec := v.HostPath + ":" + v.ContainerPath
		if v.ReadOnly {
			spec += ":ro"
		}
		childArgs = append(childArgs, "--volume", spec)
	}
	childArgs = append(childArgs, cfg.RootFS)
	childArgs = append(childArgs, cfg.Command...)
	cmd := exec.Command(self, childArgs...)

	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = append(os.Environ(), sentinelEnv)

	cmd.SysProcAttr = ns.BuildCloneFlags(ns.Options{
		UserNS:  cfg.UserNS,
		HostUID: os.Getuid(),
		HostGID: os.Getgid(),
	})

	readPipe, writePipe, err := os.Pipe()
	if err != nil {
		return fmt.Errorf("creating sync pipe: %w", err)
	}
	cmd.ExtraFiles = []*os.File{readPipe}

	if err := cmd.Start(); err != nil {
		_ = readPipe.Close()
		_ = writePipe.Close()
		return fmt.Errorf("starting container process: %w", err)
	}
	_ = readPipe.Close()

	childPID := cmd.Process.Pid
	if cfg.Debug {
		fmt.Printf("[parent] child started, PID=%d\n", childPID)
	}

	cgCfg := cgroups.Config{
		Name:      fmt.Sprintf("minicontainer-%d", childPID),
		MemoryMax: cfg.Memory,
		CPUWeight: cfg.CPUWeight,
		CPUs:      cfg.CPUs,
		PidsMax:   cfg.PidsLimit,
	}
	cgroupApplied := false
	if err := cgroups.Apply(childPID, cgCfg, cfg.Debug); err != nil {
		fmt.Fprintf(os.Stderr, "[parent] warning: cgroup setup failed: %v\n", err)
	} else {
		cgroupApplied = true
	}

	const (
		hostCIDR    = "172.20.0.1/24"
		containerIP = "172.20.0.2"
	)

	if cfg.BridgeNetwork {
		if err := network.SetupVethHost(childPID, hostCIDR, cfg.Debug); err != nil {
			fmt.Fprintf(os.Stderr, "[parent] warning: veth setup failed: %v\n", err)
		}
		for _, p := range cfg.PortMappings {
			if err := network.SetupPortForwarding(p.HostPort, p.ContainerPort, containerIP, p.Protocol, cfg.Debug); err != nil {
				fmt.Fprintf(os.Stderr, "[parent] warning: port mapping %d:%d failed: %v\n",
					p.HostPort, p.ContainerPort, err)
			}
		}
	}

	_ = writePipe.Close()

	waitErr := cmd.Wait()

	if cfg.BridgeNetwork {
		for _, p := range cfg.PortMappings {
			network.RemovePortForwarding(p.HostPort, p.ContainerPort, containerIP, p.Protocol, cfg.Debug)
		}
	}
	if cgroupApplied {
		cgroups.Remove(cgCfg.Name, cfg.Debug)
	}

	if waitErr != nil {
		if exitErr, ok := waitErr.(*exec.ExitError); ok {
			return exitErr
		}
		return fmt.Errorf("container exited with error: %w", waitErr)
	}

	if cfg.Debug {
		fmt.Println("[parent] container exited cleanly")
	}
	return nil
}

// ContainerInit is called when the re-executed child detects sentinelEnv.
func ContainerInit(cfg Config) error {
	if cfg.Debug {
		fmt.Println("[init] running inside new namespaces")
	}

	syncPipe := os.NewFile(3, "sync-pipe")
	buf := make([]byte, 1)
	_, _ = syncPipe.Read(buf)
	_ = syncPipe.Close()

	if cfg.Debug {
		fmt.Println("[init] received sync signal from parent")
	}

	if err := syscall.Mount("", "/", "", syscall.MS_REC|syscall.MS_PRIVATE, ""); err != nil {
		return fmt.Errorf("make mount namespace private: %w", err)
	}
	if cfg.Debug {
		fmt.Println("[init] mount namespace propagation set to private")
	}

	targetRootFS := cfg.RootFS
	if cfg.Overlay {
		overlayTmp, err := os.MkdirTemp("", "minicontainer-overlay-*")
		if err != nil {
			return fmt.Errorf("create overlay temp dir: %w", err)
		}
		overlayDirs, err := rootfs.PrepareOverlay(cfg.RootFS, overlayTmp)
		if err != nil {
			return fmt.Errorf("prepare overlay: %w", err)
		}
		targetRootFS = overlayDirs.Merged
		if cfg.Debug {
			fmt.Printf("[init] overlayfs mounted (%s -> %s)\n", cfg.RootFS, targetRootFS)
		}
	}

	hostname := cfg.Hostname
	if hostname == "" {
		hostname = "minicontainer"
	}
	if err := syscall.Sethostname([]byte(hostname)); err != nil {
		return fmt.Errorf("sethostname: %w", err)
	}
	if cfg.Debug {
		fmt.Printf("[init] hostname set to %q\n", hostname)
	}

	procPath := filepath.Join(targetRootFS, "proc")
	if err := os.MkdirAll(procPath, 0755); err != nil {
		return fmt.Errorf("mkdir proc: %w", err)
	}
	if err := syscall.Mount("proc", procPath, "proc", 0, ""); err != nil {
		return fmt.Errorf("mount proc: %w", err)
	}
	if cfg.Debug {
		fmt.Println("[init] /proc mounted")
	}

	sysPath := filepath.Join(targetRootFS, "sys")
	if err := os.MkdirAll(sysPath, 0755); err != nil {
		return fmt.Errorf("mkdir sys: %w", err)
	}
	if err := syscall.Mount("sysfs", sysPath, "sysfs",
		syscall.MS_RDONLY|syscall.MS_NOSUID|syscall.MS_NOEXEC|syscall.MS_NODEV, ""); err != nil {
		if cfg.Debug {
			fmt.Printf("[init] mount sysfs: %v (ignored)\n", err)
		}
	}

	devPath := filepath.Join(targetRootFS, "dev")
	if err := os.MkdirAll(devPath, 0755); err != nil {
		return fmt.Errorf("mkdir dev: %w", err)
	}
	if err := syscall.Mount("/dev", devPath, "", syscall.MS_BIND|syscall.MS_REC, ""); err != nil {
		if cfg.Debug {
			fmt.Printf("[init] bind-mount /dev: %v (ignored)\n", err)
		}
	}

	if err := network.SetupLoopback(cfg.Debug); err != nil {
		if cfg.Debug {
			fmt.Printf("[init] loopback setup: %v (ignored)\n", err)
		}
	}

	if cfg.BridgeNetwork {
		const (
			containerCIDR = "172.20.0.2/24"
			gateway       = "172.20.0.1"
		)
		if err := network.SetupVethContainer(containerCIDR, gateway, cfg.Debug); err != nil {
			if cfg.Debug {
				fmt.Printf("[init] veth setup: %v (ignored)\n", err)
			}
		}
	}

	for _, v := range cfg.Volumes {
		if err := mountVolume(v, targetRootFS, cfg.Debug); err != nil {
			return fmt.Errorf("volume %s:%s: %w", v.HostPath, v.ContainerPath, err)
		}
	}

	if err := rootfs.Isolate(targetRootFS, cfg.Debug); err != nil {
		return fmt.Errorf("rootfs isolation: %w", err)
	}

	if cfg.ReadOnly {
		if err := syscall.Mount("", "/", "", syscall.MS_BIND|syscall.MS_REMOUNT|syscall.MS_RDONLY, ""); err != nil {
			if cfg.Debug {
				fmt.Printf("[init] remount root read-only: %v (ignored)\n", err)
			}
		} else if cfg.Debug {
			fmt.Println("[init] container rootfs remounted read-only")
		}
	}

	if cfg.WorkDir != "" && cfg.WorkDir != "/" {
		if err := os.MkdirAll(cfg.WorkDir, 0755); err == nil {
			_ = syscall.Chdir(cfg.WorkDir)
		}
	}

	// Drop Linux Capabilities if specified
	if len(cfg.CapDrop) > 0 {
		if err := DropCapabilities(cfg.CapDrop, cfg.Debug); err != nil {
			return fmt.Errorf("drop capabilities: %w", err)
		}
	}

	if cfg.Seccomp {
		if err := applySeccomp(cfg.Debug); err != nil {
			return fmt.Errorf("seccomp: %w", err)
		}
	}

	binary, err := exec.LookPath(cfg.Command[0])
	if err != nil {
		binary = cfg.Command[0]
	}

	if cfg.Debug {
		fmt.Printf("[init] exec: %s %v\n", binary, cfg.Command[1:])
	}

	env := os.Environ()
	if len(cfg.Env) > 0 {
		env = append(env, cfg.Env...)
	}

	if err := syscall.Exec(binary, cfg.Command, env); err != nil {
		return fmt.Errorf("exec %s: %w", binary, err)
	}

	return nil
}

func mountVolume(v Volume, rootfs string, debug bool) error {
	target := filepath.Join(rootfs, v.ContainerPath)

	if err := os.MkdirAll(target, 0755); err != nil {
		return fmt.Errorf("create mount point: %w", err)
	}

	if err := syscall.Mount(v.HostPath, target, "",
		syscall.MS_BIND|syscall.MS_REC, ""); err != nil {
		return fmt.Errorf("bind mount: %w", err)
	}

	if v.ReadOnly {
		if err := syscall.Mount("", target, "",
			syscall.MS_BIND|syscall.MS_REMOUNT|syscall.MS_RDONLY, ""); err != nil {
			return fmt.Errorf("remount read-only: %w", err)
		}
	}

	if debug {
		mode := "rw"
		if v.ReadOnly {
			mode = "ro"
		}
		fmt.Printf("[init] volume: %s → %s (%s)\n", v.HostPath, v.ContainerPath, mode)
	}
	return nil
}
