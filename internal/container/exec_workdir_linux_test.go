//go:build linux

package container

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

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
	original, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := os.Chdir(original); err != nil {
			t.Errorf("restore cwd: %v", err)
		}
	}()

	workDir := filepath.Join(t.TempDir(), "app", "current")
	if err := enterWorkDir(workDir); err != nil {
		t.Fatalf("enter configured workdir: %v", err)
	}

	var stdout bytes.Buffer
	if err := runExecPayload([]string{"/bin/pwd"}, os.Environ(), nil, &stdout, &bytes.Buffer{}); err != nil {
		t.Fatalf("run real exec payload: %v", err)
	}
	got := strings.TrimSpace(stdout.String())
	if got != workDir {
		t.Fatalf("exec payload cwd = %q, want %q", got, workDir)
	}
}
