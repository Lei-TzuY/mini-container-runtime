//go:build linux

package main

import (
	"strings"
	"testing"
)

func TestOCISchedulerBecomesDurableRuntimeMarker(t *testing.T) {
	bundle := writeOCIRlimitBundle(t, `{"args":["/bin/true"],"cwd":"/","env":["A=B"],"scheduler":{"policy":"SCHED_BATCH","nice":5}}`)
	cfg, err := loadOCIBundle(bundle)
	if err != nil {
		t.Fatalf("load OCI bundle: %v", err)
	}
	want := processSchedulerEnv + "=SCHED_BATCH:5"
	found := false
	for _, entry := range cfg.Env {
		if entry == want {
			found = true
		}
	}
	if !found {
		t.Fatalf("runtime env = %#v, want %q", cfg.Env, want)
	}
}

func TestOCISchedulerDefaultsNiceToZero(t *testing.T) {
	bundle := writeOCIRlimitBundle(t, `{"args":["/bin/true"],"cwd":"/","scheduler":{"policy":"SCHED_OTHER"}}`)
	cfg, err := loadOCIBundle(bundle)
	if err != nil {
		t.Fatalf("load OCI bundle: %v", err)
	}
	want := processSchedulerEnv + "=SCHED_OTHER:0"
	for _, entry := range cfg.Env {
		if entry == want {
			return
		}
	}
	t.Fatalf("runtime env = %#v, want %q", cfg.Env, want)
}

func TestOCISchedulerValidationFailsClosed(t *testing.T) {
	tests := []struct {
		name      string
		scheduler string
		want      string
	}{
		{name: "null", scheduler: `null`, want: "must be an object"},
		{name: "missing policy", scheduler: `{}`, want: "policy \"\" is not supported"},
		{name: "realtime unsupported", scheduler: `{"policy":"SCHED_FIFO","priority":1}`, want: "policy \"SCHED_FIFO\" is not supported"},
		{name: "nice low", scheduler: `{"policy":"SCHED_BATCH","nice":-21}`, want: "outside [-20,19]"},
		{name: "nice high", scheduler: `{"policy":"SCHED_BATCH","nice":20}`, want: "outside [-20,19]"},
		{name: "priority", scheduler: `{"policy":"SCHED_BATCH","priority":1}`, want: "priority must be 0"},
		{name: "flags", scheduler: `{"policy":"SCHED_BATCH","flags":["SCHED_FLAG_RESET_ON_FORK"]}`, want: "flags are not supported"},
		{name: "runtime", scheduler: `{"policy":"SCHED_BATCH","runtime":1}`, want: "runtime must be 0"},
		{name: "deadline", scheduler: `{"policy":"SCHED_BATCH","deadline":1}`, want: "deadline must be 0"},
		{name: "period", scheduler: `{"policy":"SCHED_BATCH","period":1}`, want: "period must be 0"},
		{name: "unknown field", scheduler: `{"policy":"SCHED_BATCH","extra":true}`, want: "unknown field"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bundle := writeOCIRlimitBundle(t, `{"args":["/bin/true"],"cwd":"/","scheduler":`+tt.scheduler+`}`)
			_, err := loadOCIBundle(bundle)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("load error = %v, want substring %q", err, tt.want)
			}
		})
	}
}

func TestOCISchedulerRejectsReservedEnvironmentMarker(t *testing.T) {
	bundle := writeOCIRlimitBundle(t, `{"args":["/bin/true"],"cwd":"/","env":["`+processSchedulerEnv+`=SCHED_OTHER:0"],"scheduler":{"policy":"SCHED_BATCH","nice":5}}`)
	_, err := loadOCIBundle(bundle)
	if err == nil || !strings.Contains(err.Error(), "conflicts with internal scheduler policy") {
		t.Fatalf("load error = %v, want reserved-marker conflict", err)
	}
}
