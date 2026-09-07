package main

import (
	"errors"
	"strings"
	"testing"
)

func TestRunOCIBundleCommandRoutesValidatedBundle(t *testing.T) {
	var gotBundle string
	id, err := runOCIBundleCommand([]string{"/tmp/oci-bundle"}, ociRunCommandDeps{
		run: func(bundle string) (string, error) {
			gotBundle = bundle
			return "abcdef0123456789", nil
		},
	})
	if err != nil {
		t.Fatalf("runOCIBundleCommand: %v", err)
	}
	if gotBundle != "/tmp/oci-bundle" {
		t.Fatalf("bundle = %q", gotBundle)
	}
	if id != "abcdef0123456789" {
		t.Fatalf("id = %q", id)
	}
}

func TestRunOCIBundleCommandRejectsInvalidInvocation(t *testing.T) {
	for _, args := range [][]string{nil, {}, {"a", "b"}} {
		_, err := runOCIBundleCommand(args, ociRunCommandDeps{run: func(string) (string, error) {
			t.Fatal("runner must not be called for invalid CLI arguments")
			return "", nil
		}})
		if err == nil || !strings.Contains(err.Error(), "exactly one OCI bundle") {
			t.Fatalf("args=%v error=%v", args, err)
		}
	}
}

func TestRunOCIBundleCommandPropagatesRuntimeError(t *testing.T) {
	want := errors.New("runtime failed")
	_, err := runOCIBundleCommand([]string{"bundle"}, ociRunCommandDeps{
		run: func(string) (string, error) { return "container-id", want },
	})
	if !errors.Is(err, want) {
		t.Fatalf("error = %v, want %v", err, want)
	}
}
