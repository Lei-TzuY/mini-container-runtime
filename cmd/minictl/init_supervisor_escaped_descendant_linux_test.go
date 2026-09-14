//go:build linux

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"
	"testing"
)

const initSupervisorEscapedRole = "MINICONTAINER_TEST_INIT_SUPERVISOR_ESCAPED_ROLE"

func TestContainerInitSupervisorDrainsEscapedSessionDescendant(t *testing.T) {
	role := os.Getenv(initSupervisorEscapedRole)
	if role != "" {
		runInitSupervisorEscapedHelper(t, role)
		return
	}

	dir := t.TempDir()
	cmd := exec.Command(os.Args[0], "-test.run=^TestContainerInitSupervisorDrainsEscapedSessionDescendant$")
	cmd.Env = helperEnv(initSupervisorEscapedRole, "supervisor", initSupervisorTestDir, dir)
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

	pidData := waitForFile(t, filepath.Join(dir, "escaped.pid"))
	pid, err := strconv.Atoi(string(pidData))
	if err != nil || pid <= 0 {
		t.Fatalf("invalid escaped descendant pid %q: %v", pidData, err)
	}
	childPID = pid
	waitForFile(t, filepath.Join(dir, "escaped-ready"))
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

func runInitSupervisorEscapedHelper(t *testing.T, role string) {
	t.Helper()
	dir := os.Getenv(initSupervisorTestDir)
	switch role {
	case "supervisor":
		if err := os.Setenv(initSupervisorEscapedRole, "payload"); err != nil {
			os.Exit(160)
		}
		code, err := runContainerInitSupervisor([]string{os.Args[0], "-test.run=^TestContainerInitSupervisorDrainsEscapedSessionDescendant$"})
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(161)
		}
		os.Exit(code)
	case "payload":
		cmd := exec.Command(os.Args[0], "-test.run=^TestContainerInitSupervisorDrainsEscapedSessionDescendant$")
		cmd.Env = helperEnv(initSupervisorEscapedRole, "descendant", initSupervisorTestDir, dir)
		cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
		if err := cmd.Start(); err != nil {
			os.Exit(162)
		}
		waitForPathInHelper(filepath.Join(dir, "escaped-ready"))
		waitForPathInHelper(filepath.Join(dir, "release-payload"))
		return
	case "descendant":
		if err := os.WriteFile(filepath.Join(dir, "escaped.pid"), []byte(strconv.Itoa(os.Getpid())), 0o600); err != nil {
			os.Exit(163)
		}
		if err := os.WriteFile(filepath.Join(dir, "escaped-ready"), []byte("1"), 0o600); err != nil {
			os.Exit(164)
		}
		select {}
	default:
		os.Exit(165)
	}
}
