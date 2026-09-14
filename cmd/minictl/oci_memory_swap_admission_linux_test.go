//go:build linux

package main

import (
	"strings"
	"testing"
	"time"

	"minicontainer/internal/state"
)

func TestOCIMemorySwapAdmissionPersistsDurablePolicy(t *testing.T) {
	bundle := writeOCIBundle(t, `{"ociVersion":"1.1.0","root":{"path":"rootfs"},"process":{"args":["/bin/true"],"cwd":"/"},"linux":{"resources":{"memory":{"limit":67108864,"swap":100663296}}}}`)
	cfg, err := loadOCIBundle(bundle)
	if err != nil {
		t.Fatalf("loadOCIBundle: %v", err)
	}
	if cfg.Memory != 67108864 {
		t.Fatalf("Memory=%d want 67108864", cfg.Memory)
	}

	stateDir := t.TempDir()
	st, rec, err := prepareManagedRunStateWith(&cfg, runAdmissionDeps{
		openStore: func() (*state.Store, error) { return state.Open(stateDir) },
		newID:     func() (string, error) { return "oci-memory-swap", nil },
		now:       func() time.Time { return time.Unix(1_700_000_000, 0) },
	})
	if err != nil {
		t.Fatalf("prepareManagedRunStateWith: %v", err)
	}
	defer st.Close()

	if cfg.MemorySwap != 100663296 {
		t.Fatalf("MemorySwap=%d want 100663296", cfg.MemorySwap)
	}
	for _, entry := range cfg.Env {
		if strings.HasPrefix(entry, processMemorySwapEnv+"=") {
			t.Fatalf("internal memory swap marker leaked into payload env: %q", entry)
		}
	}
	spec, err := st.RestartSpec(rec.ID)
	if err != nil {
		t.Fatalf("RestartSpec: %v", err)
	}
	if spec.MemorySwap != 100663296 {
		t.Fatalf("RestartSpec.MemorySwap=%d want 100663296", spec.MemorySwap)
	}
}

func TestOCIMemorySwapAdmissionSupportsUnlimited(t *testing.T) {
	bundle := writeOCIBundle(t, `{"ociVersion":"1.1.0","root":{"path":"rootfs"},"process":{"args":["/bin/true"],"cwd":"/"},"linux":{"resources":{"memory":{"limit":67108864,"swap":-1}}}}`)
	cfg, err := loadOCIBundle(bundle)
	if err != nil {
		t.Fatalf("loadOCIBundle: %v", err)
	}
	_, _, swap, err := extractOCIMemoryPolicy(cfg.Env)
	if err != nil {
		t.Fatalf("extractOCIMemoryPolicy: %v", err)
	}
	if swap != -1 {
		t.Fatalf("swap=%d want -1", swap)
	}
}

func TestOCIMemorySwapAdmissionRejectsUnsafeTotals(t *testing.T) {
	cases := []struct {
		name string
		body string
		want string
	}{
		{"missing memory limit", `{"ociVersion":"1.1.0","root":{"path":"rootfs"},"process":{"args":["/bin/true"],"cwd":"/"},"linux":{"resources":{"memory":{"swap":100663296}}}}`, "requires linux.resources.memory.limit"},
		{"swap below memory", `{"ociVersion":"1.1.0","root":{"path":"rootfs"},"process":{"args":["/bin/true"],"cwd":"/"},"linux":{"resources":{"memory":{"limit":67108864,"swap":33554432}}}}`, "must be at least linux.resources.memory.limit"},
		{"zero swap", `{"ociVersion":"1.1.0","root":{"path":"rootfs"},"process":{"args":["/bin/true"],"cwd":"/"},"linux":{"resources":{"memory":{"limit":67108864,"swap":0}}}}`, "must be -1 or greater than zero"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := loadOCIBundle(writeOCIBundle(t, tc.body))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error=%v want substring %q", err, tc.want)
			}
		})
	}
}
