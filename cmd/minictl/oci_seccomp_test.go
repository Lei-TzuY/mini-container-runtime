package main

import (
	"encoding/json"
	"runtime"
	"strings"
	"testing"
)

func exactBuiltinOCISeccompProfile() *ociSeccompConfig {
	profile := &ociSeccompConfig{
		DefaultAction: "SCMP_ACT_ALLOW",
		Architectures: []string{"SCMP_ARCH_X86_64"},
	}
	profile.Syscalls = append(profile.Syscalls, struct {
		Names    []string          `json:"names"`
		Action   string            `json:"action"`
		ErrnoRet *uint             `json:"errnoRet,omitempty"`
		Args     []json.RawMessage `json:"args,omitempty"`
	}{
		Names:  append([]string(nil), builtinSeccompAMD64Syscalls...),
		Action: "SCMP_ACT_KILL_PROCESS",
	})
	return profile
}

func TestTranslateOCISeccompExactBuiltInProfile(t *testing.T) {
	enabled, err := translateOCISeccomp(exactBuiltinOCISeccompProfile())
	if runtime.GOOS != "linux" || runtime.GOARCH != "amd64" {
		if err == nil || enabled {
			t.Fatalf("translateOCISeccomp() = %v, %v; want platform rejection", enabled, err)
		}
		return
	}
	if err != nil {
		t.Fatalf("translateOCISeccomp() error = %v", err)
	}
	if !enabled {
		t.Fatal("translateOCISeccomp() = false, want true")
	}
}

func TestTranslateOCISeccompRejectsSemanticDrift(t *testing.T) {
	if runtime.GOOS != "linux" || runtime.GOARCH != "amd64" {
		t.Skip("exact built-in OCI seccomp mapping is linux/amd64 only")
	}

	profile := exactBuiltinOCISeccompProfile()
	profile.Syscalls[0].Names = profile.Syscalls[0].Names[:len(profile.Syscalls[0].Names)-1]
	if _, err := translateOCISeccomp(profile); err == nil || !strings.Contains(err.Error(), "does not exactly match") {
		t.Fatalf("incomplete profile error = %v", err)
	}

	profile = exactBuiltinOCISeccompProfile()
	profile.Syscalls[0].Action = "SCMP_ACT_ERRNO"
	if _, err := translateOCISeccomp(profile); err == nil || !strings.Contains(err.Error(), "cannot be represented") {
		t.Fatalf("action mismatch error = %v", err)
	}
}
