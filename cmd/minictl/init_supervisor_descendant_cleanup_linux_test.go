//go:build linux

package main

import (
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"
	"testing"
)

const initSupervisorDrainRole = "MINICONTAINER_TEST_INIT_SUPERVISOR_DRAIN_ROLE"

func TestContainerInitSupervisorDrainsPayloadGroupAfterLeaderExit(t *testing.T) {
	role := os.Getenv(initSupervisorDrainRole)
	if role != "" {
		runInitSupervisorDrainHelper(t, role)
		return
	}

	dir := t.TempDir()
	cmd := exec.Command(os.Args[0], "-test.run=^TestContainerInitSupervisorDrainsPayloadGroupAfterLeaderExit$")
	cmd.Env = helperEnv(initSupervisorDrainRole, "supervisor", initSupervisorTestDir, dir)
	if err := cmd.Start(); err != nil {
		t.Fatalf("start supervisor helper: %v", err)
	}
	finished := false
	childPID := 0
	defer func() {
		if !finished && cmd.Process != nil {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
		}
		if childPID > 0 {
			_ = syscall.Kill(childPID, syscall.SIGKILL)
		}
	}()

	pidData := waitForFile(t, filepath.Join(dir, "descendant.pid"))
	pid, err := strconv.Atoi(string(pidData))
	if err != nil || pid <= 0 {
		t.Fatalf("invalid descendant pid %q: %v", pidData, err)
	}
	childPID = pid
	waitForFile(t, filepath.Join(dir, "descendant-ready"))

	if err := os.WriteFile(filepath.Join(dir, "release-payload"), []byte("1"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Wait(); err != nil {
		t.Fatalf("supervisor helper failed: %v", err)
	}
	finished = true
	waitForProcessGone(t, childPID)
	childPID = 0
}

func runInitSupervisorDrainHelper(t *testing.T, role string) {
	t.Helper()
	dir := os.Getenv(initSupervisorTestDir)
	switch role {
	case "supervisor":
		if err := os.Setenv(initSupervisorDrainRole, "payload"); err != nil {
			os.Exit(150)
		}
		code, err := runContainerInitSupervisor([]string{os.Args[0], "-test.run=^TestContainerInitSupervisorDrainsPayloadGroupAfterLeaderExit$"})
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(151)
		}
		os.Exit(code)
	case "payload":
		cmd := exec.Command(os.Args[0], "-test.run=^TestContainerInitSupervisorDrainsPayloadGroupAfterLeaderExit$")
		cmd.Env = helperEnv(initSupervisorDrainRole, "descendant", initSupervisorTestDir, dir)
		if err := cmd.Start(); err != nil {
			os.Exit(152)
		}
		waitForPathInHelper(filepath.Join(dir, "descendant-ready"))
		waitForPathInHelper(filepath.Join(dir, "release-payload"))
		return
	case "descendant":
		if err := os.WriteFile(filepath.Join(dir, "descendant.pid"), []byte(strconv.Itoa(os.Getpid())), 0o600); err != nil {
			os.Exit(153)
		}
		if err := os.WriteFile(filepath.Join(dir, "descendant-ready"), []byte("1"), 0o600); err != nil {
			os.Exit(154)
		}
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGUSR1)
		defer signal.Stop(sigCh)
		<-sigCh
		os.Exit(155)
	default:
		os.Exit(156)
	}
}
