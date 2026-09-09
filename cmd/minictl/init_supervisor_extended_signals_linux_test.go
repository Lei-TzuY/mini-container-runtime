//go:build linux

package main

import (
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"
	"testing"
)

const extendedSignalRole = "MINICONTAINER_TEST_EXTENDED_SIGNAL_ROLE"

func TestContainerInitSupervisorForwardsAlarmToProcessGroup(t *testing.T) {
	role := os.Getenv(extendedSignalRole)
	if role != "" {
		runExtendedSignalHelper(role)
		return
	}

	dir := t.TempDir()
	cmd := exec.Command(os.Args[0], "-test.run=^TestContainerInitSupervisorForwardsAlarmToProcessGroup$")
	cmd.Env = helperEnv(extendedSignalRole, "supervisor", initSupervisorTestDir, dir)
	if err := cmd.Start(); err != nil {
		t.Fatalf("start supervisor helper: %v", err)
	}
	finished := false
	defer func() {
		if !finished && cmd.Process != nil {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
		}
	}()

	waitForFile(t, filepath.Join(dir, "payload-ready"))
	waitForFile(t, filepath.Join(dir, "descendant-ready"))
	if err := cmd.Process.Signal(syscall.SIGALRM); err != nil {
		t.Fatalf("send SIGALRM to supervisor: %v", err)
	}
	waitForFile(t, filepath.Join(dir, "payload-signaled"))
	waitForFile(t, filepath.Join(dir, "descendant-signaled"))
	if err := cmd.Wait(); err != nil {
		t.Fatalf("supervisor helper failed: %v", err)
	}
	finished = true
}

func runExtendedSignalHelper(role string) {
	dir := os.Getenv(initSupervisorTestDir)
	switch role {
	case "supervisor":
		_ = os.Setenv(extendedSignalRole, "payload")
		code, err := runContainerInitSupervisor([]string{os.Args[0], "-test.run=^TestContainerInitSupervisorForwardsAlarmToProcessGroup$"})
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(150)
		}
		os.Exit(code)
	case "payload":
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGALRM)
		defer signal.Stop(sigCh)
		cmd := exec.Command(os.Args[0], "-test.run=^TestContainerInitSupervisorForwardsAlarmToProcessGroup$")
		cmd.Env = helperEnv(extendedSignalRole, "descendant", initSupervisorTestDir, dir)
		if err := cmd.Start(); err != nil {
			os.Exit(151)
		}
		waitForPathInHelper(filepath.Join(dir, "descendant-ready"))
		if err := os.WriteFile(filepath.Join(dir, "payload-ready"), []byte("1"), 0o600); err != nil {
			os.Exit(152)
		}
		<-sigCh
		if err := os.WriteFile(filepath.Join(dir, "payload-signaled"), []byte("1"), 0o600); err != nil {
			os.Exit(153)
		}
		if err := cmd.Wait(); err != nil {
			os.Exit(154)
		}
		return
	case "descendant":
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGALRM)
		defer signal.Stop(sigCh)
		if err := os.WriteFile(filepath.Join(dir, "descendant-ready"), []byte("1"), 0o600); err != nil {
			os.Exit(155)
		}
		<-sigCh
		if err := os.WriteFile(filepath.Join(dir, "descendant-signaled"), []byte("1"), 0o600); err != nil {
			os.Exit(156)
		}
		return
	default:
		os.Exit(157)
	}
}
