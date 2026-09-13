//go:build linux

package main

import (
	"strings"
	"testing"
)

func TestOCIIOPriorityBecomesDurableRuntimeMarker(t *testing.T) {
	bundle := writeOCIRlimitBundle(t, `{"args":["/bin/true"],"cwd":"/","env":["A=B"],"ioPriority":{"class":"IOPRIO_CLASS_IDLE","priority":4}}`)
	cfg, err := loadOCIBundle(bundle)
	if err != nil {
		t.Fatalf("load OCI bundle: %v", err)
	}
	want := processIOPriorityEnv + "=IOPRIO_CLASS_IDLE:4"
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

func TestOCIIOPriorityValidationFailsClosed(t *testing.T) {
	tests := []struct {
		name       string
		ioPriority string
		want       string
	}{
		{name: "null", ioPriority: `null`, want: "must be an object"},
		{name: "missing priority", ioPriority: `{"class":"IOPRIO_CLASS_BE"}`, want: "priority is required"},
		{name: "priority low", ioPriority: `{"class":"IOPRIO_CLASS_BE","priority":-1}`, want: "outside [0,7]"},
		{name: "priority high", ioPriority: `{"class":"IOPRIO_CLASS_BE","priority":8}`, want: "outside [0,7]"},
		{name: "unknown class", ioPriority: `{"class":"IOPRIO_CLASS_NONE","priority":4}`, want: "is not supported"},
		{name: "unknown field", ioPriority: `{"class":"IOPRIO_CLASS_BE","priority":4,"extra":true}`, want: "unknown field"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bundle := writeOCIRlimitBundle(t, `{"args":["/bin/true"],"cwd":"/","ioPriority":`+tt.ioPriority+`}`)
			_, err := loadOCIBundle(bundle)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("load error = %v, want substring %q", err, tt.want)
			}
		})
	}
}

func TestOCIIOPriorityRejectsReservedEnvironmentMarker(t *testing.T) {
	bundle := writeOCIRlimitBundle(t, `{"args":["/bin/true"],"cwd":"/","env":["`+processIOPriorityEnv+`=IOPRIO_CLASS_BE:7"],"ioPriority":{"class":"IOPRIO_CLASS_IDLE","priority":4}}`)
	_, err := loadOCIBundle(bundle)
	if err == nil || !strings.Contains(err.Error(), "conflicts with internal io priority policy") {
		t.Fatalf("load error = %v, want reserved-marker conflict", err)
	}
}
