//go:build linux

package ns

import (
	"syscall"
	"testing"
)

func TestBuildCloneFlagsTracksCgroupNamespaceMarker(t *testing.T) {
	t.Setenv(CgroupNamespaceEnv, "1")
	attr := BuildCloneFlags(Options{})
	if attr.Cloneflags&syscall.CLONE_NEWCGROUP == 0 {
		t.Fatalf("clone flags %#x do not contain CLONE_NEWCGROUP", attr.Cloneflags)
	}
}

func TestBuildCloneFlagsOmitsCgroupNamespaceWithoutMarker(t *testing.T) {
	t.Setenv(CgroupNamespaceEnv, "")
	attr := BuildCloneFlags(Options{})
	if attr.Cloneflags&syscall.CLONE_NEWCGROUP != 0 {
		t.Fatalf("clone flags %#x unexpectedly contain CLONE_NEWCGROUP", attr.Cloneflags)
	}
}
