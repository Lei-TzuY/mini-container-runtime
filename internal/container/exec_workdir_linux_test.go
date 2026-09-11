//go:build linux

package container

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const execWorkDirHelperKey = "MINICONTAINER_TEST_EXEC_WORKDIR_HELPER"

func TestExecWorkDirFromEnvDefaultsValidatesAndCleans(t *testing.T) {
	t.Setenv(execWorkDirKey, "")
	got, err := execWorkDirFromEnv()
	if err != nil || got != "/" {
		t.Fatalf("default exec workdir = %q, %v", got, err)
	}

	t.Setenv(execWorkDirKey, "/work/../srv/app")
	got, err = execWorkDirFromEnv()
	if err != nil || got != "/srv/app" {
		t.Fatalf("cleaned exec workdir = %q, %v", got, err)
	}

	t.Setenv(execWorkDirKey, "relative")
	if _, err := execWorkDirFromEnv(); err == nil {
		t.Fatal("relative exec workdir accepted")
	}
}

func TestPayloadEnvironmentStripsExecWorkDirSentinel(t *testing.T) {
	got := payloadEnvironment([]string{
		"PATH=/bin:/usr/bin",
		execWorkDirKey + "=/srv/app",
		"KEEP=value",
	})
	joined := strings.Join(got, "\n")
	if strings.Contains(joined, execWorkDirKey+"=") {
		t.Fatalf("exec workdir sentinel leaked into payload environment: %q", joined)
	}
	if !strings.Contains(joined, "KEEP=value") {
		t.Fatalf("payload environment lost user entry: %q", joined)
	}
}

func TestExecPayloadRunsFromConfiguredWorkDir(t *testing.T) {
	workDir := filepath.Join(t.TempDir(), "app", "current")
	cmd := exec.Command(os.Args[0], "-test.run=^TestExecPayloadConfiguredWorkDirHelper$")
	cmd.Env = append(os.Environ(), execWorkDirHelperKey+"=1", execWorkDirKey+"="+workDir)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("configured workdir helper: %v\n%s", err, out)
	}
	got := strings.TrimSpace(string(out))
	if got != workDir {
		t.Fatalf("exec payload cwd = %q, want %q", got, workDir)
	}
}

func TestExecPayloadConfiguredWorkDirHelper(t *testing.T) {
	if os.Getenv(execWorkDirHelperKey) != "1" {
		return
	}
	workDir, err := execWorkDirFromEnv()
	if err != nil {
		t.Fatal(err)
	}
	if err := enterWorkDir(workDir); err != nil {
		t.Fatalf("enter configured workdir: %v", err)
	}
	if err := runExecPayload([]string{"/bin/pwd"}, os.Environ(), nil, os.Stdout, &bytes.Buffer{}); err != nil {
		t.Fatalf("run real exec payload: %v", err)
	}
}
