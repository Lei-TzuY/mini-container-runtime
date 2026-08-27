//go:build linux

package container

import (
	"errors"
	"strings"
	"testing"

	"minicontainer/internal/state"
)

func noopNetworkAdmissionDeps() networkAdmissionDeps {
	return networkAdmissionDeps{
		validateDNSRootFS: func(string, string) error { return nil },
		registerDNSHost:   func(string, string, string, string) error { return nil },
	}
}

func TestBridgeNetworkingRequiresDurableLifecycleStore(t *testing.T) {
	err := requireDurableNetworkOwnershipWith(Config{BridgeNetwork: true}, nil, noopNetworkAdmissionDeps())
	if err == nil || !strings.Contains(err.Error(), "bridge networking requires managed lifecycle state") {
		t.Fatalf("unmanaged bridge error=%v", err)
	}
	if !isRuntimeControlError(err) {
		t.Fatalf("unmanaged bridge was not classified as runtime-control: %v", err)
	}
}

func TestPublishedPortsRequireBridgeNetworking(t *testing.T) {
	st, err := state.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	cfg := Config{PortMappings: []PortMapping{{HostPort: 8080, ContainerPort: 80}}}
	for name, store := range map[string]*state.Store{
		"unmanaged": nil,
		"managed":   st,
	} {
		t.Run(name, func(t *testing.T) {
			err := requireDurableNetworkOwnershipWith(cfg, store, noopNetworkAdmissionDeps())
			if err == nil || !strings.Contains(err.Error(), "published ports require bridge networking") {
				t.Fatalf("publish-without-bridge error=%v", err)
			}
			if !isRuntimeControlError(err) {
				t.Fatalf("publish-without-bridge was not classified as runtime-control: %v", err)
			}
		})
	}
}

func TestNetworkPreflightAllowsManagedBridgeAndNonNetworkingRuns(t *testing.T) {
	st, err := state.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	for _, cfg := range []Config{
		{ContainerID: "ctr-bridge", Hostname: "demo", RootFS: "/rootfs", BridgeNetwork: true},
		{ContainerID: "ctr-publish", Hostname: "demo", RootFS: "/rootfs", BridgeNetwork: true, PortMappings: []PortMapping{{HostPort: 8080, ContainerPort: 80}}},
	} {
		if err := requireDurableNetworkOwnershipWith(cfg, st, noopNetworkAdmissionDeps()); err != nil {
			t.Fatalf("managed network config %+v rejected: %v", cfg, err)
		}
	}
	if err := requireDurableNetworkOwnershipWith(Config{}, nil, noopNetworkAdmissionDeps()); err != nil {
		t.Fatalf("ordinary unmanaged run rejected: %v", err)
	}
}

func TestBridgeDNSValidationFailsClosedBeforeRegistration(t *testing.T) {
	st, err := state.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	cause := errors.New("rootfs disappeared")
	registerCalls := 0
	cfg := Config{ContainerID: "ctr-dns-validation", Hostname: "demo", RootFS: "/missing", BridgeNetwork: true}
	err = requireDurableNetworkOwnershipWith(cfg, st, networkAdmissionDeps{
		validateDNSRootFS: func(rootfsPath, networkName string) error {
			if rootfsPath != cfg.RootFS || networkName != defaultBridgeDNSNetwork {
				t.Fatalf("validation args=%q/%q", rootfsPath, networkName)
			}
			return cause
		},
		registerDNSHost: func(string, string, string, string) error {
			registerCalls++
			return nil
		},
	})
	if !errors.Is(err, cause) {
		t.Fatalf("DNS validation error=%v, want cause", err)
	}
	if registerCalls != 0 {
		t.Fatalf("DNS registration called %d times after validation failure", registerCalls)
	}
	if !isRuntimeControlError(err) {
		t.Fatalf("DNS validation failure was retryable payload error: %v", err)
	}
}

func TestBridgeDNSRegistrationFailureIsAuthoritativeAdmissionError(t *testing.T) {
	st, err := state.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	cause := errors.New("registry fsync failed")
	validateCalls := 0
	registerCalls := 0
	cfg := Config{ContainerID: "ctr-dns-register", Hostname: "demo", RootFS: "/rootfs", BridgeNetwork: true}
	err = requireDurableNetworkOwnershipWith(cfg, st, networkAdmissionDeps{
		validateDNSRootFS: func(rootfsPath, networkName string) error {
			validateCalls++
			return nil
		},
		registerDNSHost: func(networkName, containerID, hostname, ipAddr string) error {
			registerCalls++
			if networkName != defaultBridgeDNSNetwork || containerID != cfg.ContainerID || hostname != cfg.Hostname || ipAddr != defaultBridgeContainerIP {
				t.Fatalf("registration args=%q/%q/%q/%q", networkName, containerID, hostname, ipAddr)
			}
			return cause
		},
	})
	if !errors.Is(err, cause) {
		t.Fatalf("DNS registration error=%v, want cause", err)
	}
	if validateCalls != 1 || registerCalls != 1 {
		t.Fatalf("DNS admission calls validate=%d register=%d", validateCalls, registerCalls)
	}
	if !isRuntimeControlError(err) {
		t.Fatalf("DNS registration failure was retryable payload error: %v", err)
	}
}

func TestBridgeDNSAdmissionRejectsMissingManagedContainerID(t *testing.T) {
	st, err := state.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	calls := 0
	err = requireDurableNetworkOwnershipWith(Config{BridgeNetwork: true}, st, networkAdmissionDeps{
		validateDNSRootFS: func(string, string) error { calls++; return nil },
		registerDNSHost:   func(string, string, string, string) error { calls++; return nil },
	})
	if err == nil || !strings.Contains(err.Error(), "managed container ID") {
		t.Fatalf("missing ID error=%v", err)
	}
	if calls != 0 {
		t.Fatalf("DNS side effects called %d times before managed ID proof", calls)
	}
}
