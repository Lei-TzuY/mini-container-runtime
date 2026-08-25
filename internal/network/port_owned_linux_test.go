//go:build linux

package network

import (
	"errors"
	"strings"
	"testing"
)

func TestOwnedPortSetupTagsRulesAndRollbackWithSameOwner(t *testing.T) {
	owner := "minicontainer:test-generation"
	outputCause := errors.New("output rejected")
	var calls []string
	err := setupPortForwardingOwnedWith(owner, 8080, 80, "172.20.0.2", "", false, func(args ...string) ([]byte, error) {
		calls = append(calls, strings.Join(args, " "))
		if len(calls) == 2 {
			return []byte("output"), outputCause
		}
		return nil, nil
	})
	if !errors.Is(err, outputCause) {
		t.Fatalf("setup cause not preserved: %v", err)
	}
	if len(calls) != 3 {
		t.Fatalf("calls=%v, want add/add/rollback", calls)
	}
	for i, call := range calls {
		if !strings.Contains(call, "-m comment --comment "+owner) {
			t.Fatalf("call %d missing owner tag: %s", i, call)
		}
	}
	if !strings.Contains(calls[0], "-A PREROUTING") || !strings.Contains(calls[1], "-A OUTPUT") || !strings.Contains(calls[2], "-D PREROUTING") {
		t.Fatalf("unexpected setup/rollback sequence: %v", calls)
	}
}

func TestOwnedPortCleanupAttemptsBothTaggedRulesAndJoinsFailures(t *testing.T) {
	owner := "minicontainer:test-generation"
	preCause := errors.New("prerouting delete failed")
	outCause := errors.New("output delete failed")
	var calls []string
	err := removePortForwardingOwnedWith(owner, 8080, 80, "172.20.0.2", "tcp", false, func(args ...string) ([]byte, error) {
		calls = append(calls, strings.Join(args, " "))
		if len(calls) == 1 {
			return []byte("pre"), preCause
		}
		return []byte("out"), outCause
	})
	if len(calls) != 2 {
		t.Fatalf("calls=%v, want both tagged deletes attempted", calls)
	}
	for i, call := range calls {
		if !strings.Contains(call, "-m comment --comment "+owner) {
			t.Fatalf("cleanup call %d missing owner tag: %s", i, call)
		}
	}
	if !errors.Is(err, preCause) || !errors.Is(err, outCause) {
		t.Fatalf("cleanup errors not joined: %v", err)
	}
}

func TestOwnedPortRulesRejectMissingOwnerBeforeIPTables(t *testing.T) {
	calls := 0
	run := func(args ...string) ([]byte, error) {
		calls++
		return nil, nil
	}
	if err := setupPortForwardingOwnedWith("", 8080, 80, "172.20.0.2", "tcp", false, run); err == nil {
		t.Fatal("empty setup owner unexpectedly accepted")
	}
	if err := removePortForwardingOwnedWith("", 8080, 80, "172.20.0.2", "tcp", false, run); err == nil {
		t.Fatal("empty cleanup owner unexpectedly accepted")
	}
	if calls != 0 {
		t.Fatalf("iptables called %d times for invalid owner", calls)
	}
}

func TestNewPortForwardingOwnerIsGenerationScoped(t *testing.T) {
	first, err := NewPortForwardingOwner()
	if err != nil {
		t.Fatalf("first owner: %v", err)
	}
	second, err := NewPortForwardingOwner()
	if err != nil {
		t.Fatalf("second owner: %v", err)
	}
	if !strings.HasPrefix(first, portForwardingOwnerPrefix) || !strings.HasPrefix(second, portForwardingOwnerPrefix) {
		t.Fatalf("owner prefix missing: %q %q", first, second)
	}
	if first == second {
		t.Fatalf("generation owners unexpectedly identical: %q", first)
	}
}
