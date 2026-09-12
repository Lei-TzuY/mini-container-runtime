package container

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"minicontainer/internal/state"
)

func TestExecDetached(t *testing.T) {
	tmpDir := t.TempDir()
	st, err := state.Open(tmpDir)
	if err != nil {
		t.Fatalf("Open state store error: %v", err)
	}
	defer st.Close()

	c := &state.Container{
		ID:        "ctr-det-1",
		Status:    state.StatusRunning,
		PID:       os.Getpid(),
		CreatedAt: time.Now(),
	}
	if err := st.Save(c); err != nil {
		t.Fatalf("save container: %v", err)
	}

	pid, err := ExecDetached(st, c.ID, []string{"echo", "hello"})
	if err != nil {
		t.Fatalf("ExecDetached error: %v", err)
	}
	if pid <= 0 {
		t.Fatalf("returned pid = %d, want > 0", pid)
	}
}

func TestManagedDetachedExecCommandUsesManagedExecEntryPoint(t *testing.T) {
	executable := "/opt/minicontainer/minictl"
	cmd := newManagedDetachedExecCommand(executable, "ctr-123", []string{"sh", "-c", "echo ok"})

	want := []string{executable, "exec", "ctr-123", "sh", "-c", "echo ok"}
	if len(cmd.Args) != len(want) {
		t.Fatalf("args = %q, want %q", cmd.Args, want)
	}
	for i := range want {
		if cmd.Args[i] != want[i] {
			t.Fatalf("args[%d] = %q, want %q", i, cmd.Args[i], want[i])
		}
	}
	if cmd.Stdin != nil || cmd.Stdout != nil || cmd.Stderr != nil {
		t.Fatalf("detached command retained terminal streams")
	}
	if runtime.GOOS == "linux" && (cmd.SysProcAttr == nil || !cmd.SysProcAttr.Setsid) {
		t.Fatalf("Linux detached command does not create a new session")
	}
}

func TestManagedDetachedExecCommandRunsRealProcess(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell helper requires a POSIX host")
	}

	dir := t.TempDir()
	out := filepath.Join(dir, "args.txt")
	helper := filepath.Join(dir, "minictl-helper")
	script := "#!/bin/sh\nprintf '%s\\n' \"$@\" > \"$MINICONTAINER_DETACHED_TEST_OUT\"\n"
	if err := os.WriteFile(helper, []byte(script), 0o755); err != nil {
		t.Fatalf("write helper: %v", err)
	}
	t.Setenv("MINICONTAINER_DETACHED_TEST_OUT", out)

	cmd := newManagedDetachedExecCommand(helper, "ctr-real", []string{"echo", "hello"})
	if err := cmd.Run(); err != nil {
		t.Fatalf("run detached launcher helper: %v", err)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read helper argv: %v", err)
	}
	got := strings.Fields(string(data))
	want := []string{"exec", "ctr-real", "echo", "hello"}
	if len(got) != len(want) {
		t.Fatalf("helper argv = %q, want %q", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("helper argv[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}
