package main

import (
	"reflect"
	"testing"
)

func TestParseDetachedExecArgs(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		detached bool
		id       string
		command  []string
		wantErr  bool
	}{
		{name: "foreground untouched", args: []string{"abc", "echo", "ok"}},
		{name: "short detach", args: []string{"-d", "abc", "echo", "ok"}, detached: true, id: "abc", command: []string{"echo", "ok"}},
		{name: "long detach", args: []string{"--detach", "abc", "sleep", "10"}, detached: true, id: "abc", command: []string{"sleep", "10"}},
		{name: "missing payload", args: []string{"-d", "abc"}, detached: true, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			detached, id, command, err := parseDetachedExecArgs(tt.args)
			if (err != nil) != tt.wantErr {
				t.Fatalf("error = %v, wantErr %v", err, tt.wantErr)
			}
			if detached != tt.detached || id != tt.id || !reflect.DeepEqual(command, tt.command) {
				t.Fatalf("got detached=%v id=%q command=%v", detached, id, command)
			}
		})
	}
}
