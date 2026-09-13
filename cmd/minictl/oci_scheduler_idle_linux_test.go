//go:build linux

package main

import "testing"

func TestOCISchedulerIdleBecomesDurableRuntimeMarker(t *testing.T) {
	bundle := writeOCIRlimitBundle(t, `{"args":["/bin/true"],"cwd":"/","scheduler":{"policy":"SCHED_IDLE"}}`)
	cfg, err := loadOCIBundle(bundle)
	if err != nil {
		t.Fatalf("load OCI bundle: %v", err)
	}
	want := processSchedulerEnv + "=SCHED_IDLE:0"
	for _, entry := range cfg.Env {
		if entry == want {
			return
		}
	}
	t.Fatalf("runtime env = %#v, want %q", cfg.Env, want)
}
