//go:build linux

package container

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

const execProcessUserHelperEnv = "MINICONTAINER_TEST_EXEC_PROCESS_USER_HELPER"

func TestExecProcessUserPayloadHelper(t *testing.T) {
	if os.Getenv(execProcessUserHelperEnv) != "1" {
		return
	}
	path := os.Getenv("MINICONTAINER_TEST_EXEC_PROCESS_USER_FILE")
	if path == "" {
		t.Fatal("missing helper file path")
	}
	if err := os.WriteFile(path, []byte("ok"), 0o666); err != nil {
		t.Fatalf("write helper file: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat helper file: %v", err)
	}
	fmt.Printf("%d:%d:%#o:%s\n", os.Getuid(), os.Getgid(), info.Mode().Perm(), os.Getenv(processUIDEnv))
}

func TestRunExecPayloadAppliesProcessUserAndUmaskWithoutLeakingMarkers(t *testing.T) {
	target := filepath.Join(t.TempDir(), "created")
	env := append(os.Environ(),
		execProcessUserHelperEnv+"=1",
		"MINICONTAINER_TEST_EXEC_PROCESS_USER_FILE="+target,
		processUIDEnv+"="+strconv.Itoa(os.Getuid()),
		processGIDEnv+"="+strconv.Itoa(os.Getgid()),
		processGroupsEnv+"=",
		processUmaskEnv+"="+strconv.FormatUint(uint64(0o027), 10),
	)
	var stdout, stderr bytes.Buffer
	if err := runExecPayload([]string{os.Args[0], "-test.run=^TestExecProcessUserPayloadHelper$"}, env, nil, &stdout, &stderr); err != nil {
		t.Fatalf("run exec payload: %v stderr=%q", err, stderr.String())
	}
	lines := strings.Split(strings.TrimSpace(stdout.String()), "\n")
	got := lines[0]
	want := fmt.Sprintf("%d:%d:0640:", os.Getuid(), os.Getgid())
	if got != want {
		t.Fatalf("payload identity/mode/marker = %q, want %q", got, want)
	}
}

func TestExecPayloadSecurityPolicyRejectsIncompleteIdentity(t *testing.T) {
	_, _, _, err := execPayloadSecurityPolicy([]string{processUIDEnv + "=1"})
	if err == nil || !strings.Contains(err.Error(), "incomplete") {
		t.Fatalf("error = %v, want incomplete process user marker rejection", err)
	}
}

func TestExecPayloadSecurityPolicyRejectsInvalidUmask(t *testing.T) {
	_, _, _, err := execPayloadSecurityPolicy([]string{processUmaskEnv + "=512"})
	if err == nil || !strings.Contains(err.Error(), "0777") {
		t.Fatalf("error = %v, want invalid umask rejection", err)
	}
}
