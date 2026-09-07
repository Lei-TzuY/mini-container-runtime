//go:build linux

package ns

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
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

func TestBuildCloneFlagsCreatesDistinctCgroupNamespace(t *testing.T) {
	parentNS, err := os.Readlink("/proc/self/ns/cgroup")
	if err != nil {
		t.Skipf("cgroup namespace identity unavailable: %v", err)
	}
	t.Setenv(CgroupNamespaceEnv, "1")
	cmd := exec.Command(os.Args[0], "-test.run=^TestCgroupNamespaceChildProbe$")
	cmd.Env = append(os.Environ(), "MINICONTAINER_CGROUP_NS_CHILD=1")
	cmd.SysProcAttr = BuildCloneFlags(Options{
		UserNS:  true,
		HostUID: os.Getuid(),
		HostGID: os.Getgid(),
	})
	out, err := cmd.CombinedOutput()
	if err != nil {
		if errors.Is(err, syscall.EPERM) || errors.Is(err, syscall.EINVAL) {
			t.Skipf("kernel does not permit requested namespace clone: %v", err)
		}
		t.Fatalf("clone child: %v: %s", err, out)
	}
	childNS := strings.TrimSpace(string(out))
	if childNS == "" {
		t.Fatal("child did not report cgroup namespace identity")
	}
	if childNS == parentNS {
		t.Fatalf("child cgroup namespace %q matches parent %q", childNS, parentNS)
	}
}

func TestCgroupNamespaceChildProbe(t *testing.T) {
	if os.Getenv("MINICONTAINER_CGROUP_NS_CHILD") != "1" {
		return
	}
	identity, err := os.Readlink("/proc/self/ns/cgroup")
	if err != nil {
		t.Fatal(err)
	}
	fmt.Print(identity)
}
