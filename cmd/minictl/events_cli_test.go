package main

import (
	"bytes"
	"reflect"
	"strings"
	"testing"

	"minicontainer/internal/events"
)

func TestParseEventsCLIOptionsFollowAliases(t *testing.T) {
	for _, flagName := range []string{"-f", "--follow"} {
		t.Run(flagName, func(t *testing.T) {
			opts, err := parseEventsCLIOptions([]string{flagName}, &bytes.Buffer{})
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			if !opts.Follow {
				t.Fatalf("Follow = false for %s", flagName)
			}
		})
	}
}

func TestParseEventsCLIOptionsQuery(t *testing.T) {
	opts, err := parseEventsCLIOptions([]string{
		"--json",
		"--container", "deadbeef",
		"--type", "start",
		"--type=die",
	}, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !opts.JSON {
		t.Fatal("JSON = false")
	}
	if opts.ContainerPrefix != "deadbeef" {
		t.Fatalf("ContainerPrefix = %q", opts.ContainerPrefix)
	}
	wantTypes := []events.EventType{events.EventStart, events.EventDie}
	if !reflect.DeepEqual(opts.Types, wantTypes) {
		t.Fatalf("Types = %#v, want %#v", opts.Types, wantTypes)
	}
}

func TestParseEventsCLIOptionsRejectsEmptyType(t *testing.T) {
	var stderr bytes.Buffer
	_, err := parseEventsCLIOptions([]string{"--type", "   "}, &stderr)
	if err == nil || !strings.Contains(err.Error(), "must not be empty") {
		t.Fatalf("err = %v, stderr = %q", err, stderr.String())
	}
}

func TestParseEventsCLIOptionsRejectsTrailingArguments(t *testing.T) {
	_, err := parseEventsCLIOptions([]string{"--json", "unexpected"}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "unexpected positional argument") {
		t.Fatalf("err = %v", err)
	}
}

func TestParseEventsCLIOptionsRejectsUnknownFlag(t *testing.T) {
	_, err := parseEventsCLIOptions([]string{"--jsno"}, &bytes.Buffer{})
	if err == nil {
		t.Fatal("expected unknown flag error")
	}
}
