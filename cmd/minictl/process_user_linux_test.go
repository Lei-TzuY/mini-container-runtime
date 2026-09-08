//go:build linux

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

const processUserProbeFileEnv = "MINICONTAINER_TEST_PROCESS_USER_PROBE_FILE"

func TestContainerInitSupervisorAppliesProcessUser(t *testing.T) {
	if probe := os.Getenv(processUserProbeFileEnv); probe != "" {
		groups, err := os.Getgroups()
		if err != nil {
			os.Exit(92)
		}
		parts := make([]string, 0, len(groups))
		for _, gid := range groups {
			parts = append(parts, strconv.Itoa(gid))
		}
		data := []byte(fmt.Sprintf("%d:%d:%s", os.Geteuid(), os.Getegid(), strings.Join(parts, ",")))
		if err := os.WriteFile(probe, data, 0o666); err != nil {
			os.Exit(91)
		}
		return
	}
	if os.Geteuid() != 0 {
		t.Skip("credential transition regression requires root; OCI userns 0:0 remains covered by admission tests")
	}

	dir := t.TempDir()
	if err := os.Chmod(dir, 0o777); err != nil {
		t.Fatal(err)
	}
	probe := filepath.Join(dir, "identity")
	if err := os.WriteFile(probe, nil, 0o666); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(probe, 0o666); err != nil {
		t.Fatal(err)
	}
	t.Setenv(processUserProbeFileEnv, probe)
	t.Setenv(processUIDRuntimeEnv, "65534")
	t.Setenv(processGIDRuntimeEnv, "65534")
	t.Setenv(processGroupsRuntimeEnv, "65533,65532")

	code, err := runContainerInitSupervisor([]string{os.Args[0], "-test.run=^TestContainerInitSupervisorAppliesProcessUser$"})
	if err != nil {
		t.Fatalf("supervisor: %v", err)
	}
	if code != 0 {
		t.Fatalf("payload exit code = %d, want 0", code)
	}
	data, err := os.ReadFile(probe)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(string(data)); got != "65534:65534:65533,65532" && got != "65534:65534:65532,65533" {
		t.Fatalf("payload identity/groups = %q, want uid/gid 65534 with supplementary groups 65533,65532", got)
	}
}

func TestPayloadCredentialFromRuntimeEnvConsumesMarkers(t *testing.T) {
	t.Setenv(processUIDRuntimeEnv, strconv.FormatUint(123, 10))
	t.Setenv(processGIDRuntimeEnv, strconv.FormatUint(456, 10))
	t.Setenv(processGroupsRuntimeEnv, "789,790")
	cred, err := payloadCredentialFromRuntimeEnv()
	if err != nil {
		t.Fatal(err)
	}
	if cred == nil || cred.Uid != 123 || cred.Gid != 456 || len(cred.Groups) != 2 || cred.Groups[0] != 789 || cred.Groups[1] != 790 {
		t.Fatalf("credential = %#v, want 123:456 groups [789 790]", cred)
	}
	for _, key := range []string{processUIDRuntimeEnv, processGIDRuntimeEnv, processGroupsRuntimeEnv} {
		if _, ok := os.LookupEnv(key); ok {
			t.Fatalf("%s still present after consumption", key)
		}
	}
}

func TestPayloadCredentialFromRuntimeEnvRejectsPartialMarker(t *testing.T) {
	t.Setenv(processUIDRuntimeEnv, "123")
	_ = os.Unsetenv(processGIDRuntimeEnv)
	_ = os.Unsetenv(processGroupsRuntimeEnv)
	if _, err := payloadCredentialFromRuntimeEnv(); err == nil || !strings.Contains(err.Error(), "incomplete") {
		t.Fatalf("error = %v, want incomplete marker rejection", err)
	}
}
