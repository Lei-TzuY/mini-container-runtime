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
	fmt.Printf("%#o:%s\n", info.Mode().Perm(), os.Getenv(processUmaskEnv))
}

func TestRunExecPayloadAppliesUmaskWithoutLeakingMarker(t *testing.T) {
	target := filepath.Join(t.TempDir(), "created")
	env := append(os.Environ(),
		execProcessUserHelperEnv+"=1",
		"MINICONTAINER_TEST_EXEC_PROCESS_USER_FILE="+target,
		processUmaskEnv+"="+strconv.FormatUint(uint64(0o027), 10),
	)
	var stdout, stderr bytes.Buffer
	if err := runExecPayload([]string{os.Args[0], "-test.run=^TestExecProcessUserPayloadHelper$"}, env, nil, &stdout, &stderr); err != nil {
		t.Fatalf("run exec payload: %v stderr=%q", err, stderr.String())
	}
	lines := strings.Split(strings.TrimSpace(stdout.String()), "\n")
	got := lines[0]
	if got != "0640:" {
		t.Fatalf("payload mode/marker = %q, want %q", got, "0640:")
	}
}

func TestExecPayloadSecurityPolicyBuildsCredentialAndStripsMarkers(t *testing.T) {
	env, credential, umask, err := execPayloadSecurityPolicy([]string{
		"VISIBLE=inside",
		processUIDEnv + "=123",
		processGIDEnv + "=456",
		processGroupsEnv + "=7,8",
		processUmaskEnv + "=23",
	})
	if err != nil {
		t.Fatal(err)
	}
	if credential == nil || credential.Uid != 123 || credential.Gid != 456 || len(credential.Groups) != 2 || credential.Groups[0] != 7 || credential.Groups[1] != 8 {
		t.Fatalf("credential = %#v", credential)
	}
	if umask == nil || *umask != 0o027 {
		t.Fatalf("umask = %#v, want 0027", umask)
	}
	if len(env) != 1 || env[0] != "VISIBLE=inside" {
		t.Fatalf("payload environment = %#v, runtime markers leaked", env)
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
