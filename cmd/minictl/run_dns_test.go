package main

import (
	"errors"
	"testing"

	"minicontainer/internal/container"
	"minicontainer/internal/state"
)

func TestAdmitRunDNSFailsClosedBeforeSpawn(t *testing.T) {
	cause := errors.New("registry unavailable")
	validateCalls := 0
	registerCalls := 0
	cfg := &container.Config{BridgeNetwork: true, RootFS: "/rootfs", Hostname: "demo"}

	registered, err := admitRunDNS(cfg, "ctr-1", runDNSDeps{
		validateRootFS: func(rootfsPath, networkName string) error {
			validateCalls++
			if rootfsPath != "/rootfs" || networkName != defaultRunDNSNetwork {
				t.Fatalf("validate args=%q/%q", rootfsPath, networkName)
			}
			return nil
		},
		registerHost: func(networkName, containerID, hostname, ipAddr string) error {
			registerCalls++
			if networkName != defaultRunDNSNetwork || containerID != "ctr-1" || hostname != "demo" || ipAddr != defaultRunBridgeIP {
				t.Fatalf("register args=%q/%q/%q/%q", networkName, containerID, hostname, ipAddr)
			}
			return cause
		},
		unregisterHost: func(string, string) error { return nil },
	})
	if registered {
		t.Fatal("failed registration reported admitted DNS")
	}
	if !errors.Is(err, cause) {
		t.Fatalf("admission error=%v, want cause", err)
	}
	if validateCalls != 1 || registerCalls != 1 {
		t.Fatalf("calls validate=%d register=%d", validateCalls, registerCalls)
	}
}

func TestAdmitRunDNSValidatesBeforeRegistration(t *testing.T) {
	cause := errors.New("bad rootfs")
	registerCalls := 0
	registered, err := admitRunDNS(&container.Config{BridgeNetwork: true, RootFS: "/bad", Hostname: "demo"}, "ctr-2", runDNSDeps{
		validateRootFS: func(string, string) error { return cause },
		registerHost: func(string, string, string, string) error {
			registerCalls++
			return nil
		},
		unregisterHost: func(string, string) error { return nil },
	})
	if registered {
		t.Fatal("validation failure reported registered DNS")
	}
	if !errors.Is(err, cause) {
		t.Fatalf("admission error=%v, want validation cause", err)
	}
	if registerCalls != 0 {
		t.Fatalf("registration called %d times after validation failure", registerCalls)
	}
}

func TestAdmitRunDNSSkipsNonBridgeRun(t *testing.T) {
	calls := 0
	registered, err := admitRunDNS(&container.Config{RootFS: "/rootfs"}, "ctr-3", runDNSDeps{
		validateRootFS: func(string, string) error { calls++; return nil },
		registerHost: func(string, string, string, string) error { calls++; return nil },
		unregisterHost: func(string, string) error { calls++; return nil },
	})
	if err != nil || registered || calls != 0 {
		t.Fatalf("non-bridge admission registered=%v calls=%d err=%v", registered, calls, err)
	}
}

func TestCompleteRunDNSOnlyUnregistersAuthoritativeStoppedState(t *testing.T) {
	calls := 0
	deps := runDNSDeps{unregisterHost: func(networkName, containerID string) error {
		calls++
		if networkName != defaultRunDNSNetwork || containerID != "ctr-4" {
			t.Fatalf("unregister args=%q/%q", networkName, containerID)
		}
		return nil
	}}

	if err := completeRunDNS(true, nil, "ctr-4", deps); err != nil {
		t.Fatalf("nil settled state: %v", err)
	}
	if err := completeRunDNS(true, &state.Container{Status: state.StatusRunning}, "ctr-4", deps); err != nil {
		t.Fatalf("running state: %v", err)
	}
	if calls != 0 {
		t.Fatalf("unregister called %d times before stopped proof", calls)
	}
	if err := completeRunDNS(true, &state.Container{Status: state.StatusStopped}, "ctr-4", deps); err != nil {
		t.Fatalf("stopped cleanup: %v", err)
	}
	if calls != 1 {
		t.Fatalf("unregister calls=%d, want 1", calls)
	}
}

func TestCompleteRunDNSPropagatesCleanupFailure(t *testing.T) {
	cause := errors.New("disk full")
	err := completeRunDNS(true, &state.Container{Status: state.StatusStopped}, "ctr-5", runDNSDeps{
		unregisterHost: func(string, string) error { return cause },
	})
	if !errors.Is(err, cause) {
		t.Fatalf("cleanup error=%v, want cause", err)
	}

	runCause := errors.New("payload failed")
	joined := joinRunDNSCompletion(runCause, err)
	if !errors.Is(joined, runCause) || !errors.Is(joined, cause) {
		t.Fatalf("joined error=%v does not preserve both causes", joined)
	}
}
