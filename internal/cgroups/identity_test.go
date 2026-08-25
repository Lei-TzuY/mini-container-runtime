package cgroups

import (
	"strings"
	"testing"
)

func TestNameForContainerProcessBindsProcessGeneration(t *testing.T) {
	first, err := NameForContainerProcess("0123456789abcdef", 12345)
	if err != nil {
		t.Fatalf("NameForContainerProcess: %v", err)
	}
	second, err := NameForContainerProcess("0123456789abcdef", 12346)
	if err != nil {
		t.Fatalf("NameForContainerProcess second generation: %v", err)
	}
	if first == second {
		t.Fatalf("different process generations produced same cgroup name %q", first)
	}
	if first != "minicontainer-0123456789abcdef-12345" {
		t.Fatalf("name = %q", first)
	}
}

func TestNameForContainerProcessRejectsMissingIdentity(t *testing.T) {
	for _, tc := range []struct {
		id        string
		startTime uint64
	}{
		{id: "", startTime: 1},
		{id: "ctr", startTime: 0},
	} {
		if _, err := NameForContainerProcess(tc.id, tc.startTime); err == nil {
			t.Fatalf("NameForContainerProcess(%q, %d) succeeded, want error", tc.id, tc.startTime)
		}
	}
}

func TestNameForContainerProcessRejectsUnsafeOrOversizedID(t *testing.T) {
	for _, id := range []string{"../escape", "bad id", strings.Repeat("a", maxCgroupNameLen)} {
		if _, err := NameForContainerProcess(id, 1); err == nil {
			t.Fatalf("NameForContainerProcess(%q, 1) succeeded, want error", id)
		}
	}
}
