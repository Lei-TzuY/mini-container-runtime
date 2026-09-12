//go:build linux

package container

import (
	"reflect"
	"syscall"
	"testing"
)

func TestExtractProcMountPolicyAppliesFlagsAndStripsInternalEnv(t *testing.T) {
	env := []string{"A=1", RuntimeProcMountOptionsEnvKey + "=rw,nosuid,noexec,nodev", "B=2"}
	clean, flags, err := extractProcMountPolicy(env)
	if err != nil {
		t.Fatal(err)
	}
	if want := uintptr(syscall.MS_NOSUID | syscall.MS_NOEXEC | syscall.MS_NODEV); flags != want {
		t.Fatalf("flags=%#x want %#x", flags, want)
	}
	if want := []string{"A=1", "B=2"}; !reflect.DeepEqual(clean, want) {
		t.Fatalf("clean env=%v want %v", clean, want)
	}
}

func TestProcMountFlagsHonorInverseOptions(t *testing.T) {
	flags, err := procMountFlags([]string{"ro", "nosuid", "noexec", "nodev", "rw", "suid", "exec", "dev"})
	if err != nil {
		t.Fatal(err)
	}
	if flags != 0 {
		t.Fatalf("flags=%#x want 0 after inverse options", flags)
	}
}

func TestExtractProcMountPolicyRejectsMalformedPolicy(t *testing.T) {
	for _, env := range [][]string{
		{RuntimeProcMountOptionsEnvKey + "="},
		{RuntimeProcMountOptionsEnvKey + "=nosuid,nosuid"},
		{RuntimeProcMountOptionsEnvKey + "=nosuid", RuntimeProcMountOptionsEnvKey + "=nodev"},
		{RuntimeProcMountOptionsEnvKey + "=unknown"},
	} {
		if _, _, err := extractProcMountPolicy(env); err == nil {
			t.Fatalf("extractProcMountPolicy(%v) succeeded, want error", env)
		}
	}
}
