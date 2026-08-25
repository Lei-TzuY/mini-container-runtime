//go:build linux

package container

import (
	"errors"
	"strings"
	"testing"
	"time"

	"minicontainer/internal/state"
)

func saveNetworkOwnershipContainer(t *testing.T, st *state.Store, id string, pid int, start uint64) state.NetworkOwnership {
	t.Helper()
	if err := st.Save(&state.Container{
		ID:        id,
		Status:    state.StatusCreated,
		RootFS:    "/tmp/rootfs",
		Command:   []string{"true"},
		CreatedAt: time.Now(),
	}); err != nil {
		t.Fatal(err)
	}
	if err := st.MarkRunning(id, pid, start, time.Now()); err != nil {
		t.Fatal(err)
	}
	ownership := networkOwnershipForGeneration(
		"minicontainer:test-owner",
		pid,
		start,
		"172.20.0.2",
		[]PortMapping{
			{HostPort: 8080, ContainerPort: 80},
			{HostPort: 5353, ContainerPort: 53, Protocol: "udp"},
		},
	)
	if err := st.MarkNetworkOwnedIfIdentity(id, ownership); err != nil {
		t.Fatal(err)
	}
	if _, err := st.MarkStoppedIfIdentity(id, pid, start, -1, time.Now()); err != nil {
		t.Fatal(err)
	}
	return ownership
}

func TestNetworkOwnershipForGenerationNormalizesProtocol(t *testing.T) {
	ownership := networkOwnershipForGeneration(
		"minicontainer:test-owner",
		1,
		2,
		"172.20.0.2",
		[]PortMapping{{HostPort: 8080, ContainerPort: 80}, {HostPort: 5353, ContainerPort: 53, Protocol: "udp"}},
	)
	if len(ownership.Mappings) != 2 || ownership.Mappings[0].Protocol != "tcp" || ownership.Mappings[1].Protocol != "udp" {
		t.Fatalf("unexpected normalized ownership: %+v", ownership)
	}
}

func TestCleanupNetworkOwnershipAttemptsAllMappingsAndClearsSidecar(t *testing.T) {
	st, err := state.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	const id = "ctr-network-cleanup"
	ownership := saveNetworkOwnershipContainer(t, st, id, 101, 202)

	var calls []string
	remove := func(owner string, hostPort, containerPort int, containerIP, protocol string, debug bool) error {
		calls = append(calls, owner+" "+protocol+" "+containerIP)
		return nil
	}
	if err := cleanupNetworkOwnershipWith(st, id, ownership, false, remove); err != nil {
		t.Fatalf("cleanup network ownership: %v", err)
	}
	if len(calls) != 2 || !strings.Contains(calls[0], "udp") || !strings.Contains(calls[1], "tcp") {
		t.Fatalf("cleanup order/calls=%v", calls)
	}
	if _, ok, err := st.GetNetworkOwnership(id); err != nil || ok {
		t.Fatalf("ownership remains after cleanup: ok=%v err=%v", ok, err)
	}
}

func TestCleanupNetworkOwnershipFailurePreservesSidecarAndAttemptsAll(t *testing.T) {
	st, err := state.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	const id = "ctr-network-cleanup-fail"
	ownership := saveNetworkOwnershipContainer(t, st, id, 301, 302)
	cause := errors.New("iptables unavailable")
	calls := 0
	remove := func(owner string, hostPort, containerPort int, containerIP, protocol string, debug bool) error {
		calls++
		if protocol == "udp" {
			return cause
		}
		return nil
	}
	err = cleanupNetworkOwnershipWith(st, id, ownership, false, remove)
	if !errors.Is(err, cause) {
		t.Fatalf("cleanup cause not preserved: %v", err)
	}
	if calls != 2 {
		t.Fatalf("cleanup calls=%d, want all mappings attempted", calls)
	}
	got, ok, readErr := st.GetNetworkOwnership(id)
	if readErr != nil || !ok || got.Owner != ownership.Owner {
		t.Fatalf("cleanup failure lost recovery sidecar: got=%+v ok=%v err=%v", got, ok, readErr)
	}
}

func TestCleanupStoppedNetworkIsNoopWithoutOwnership(t *testing.T) {
	st, err := state.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	c := &state.Container{ID: "ctr-network-none", Status: state.StatusStopped, RootFS: "/tmp/rootfs", Command: []string{"true"}, CreatedAt: time.Now()}
	if err := st.Save(c); err != nil {
		t.Fatal(err)
	}
	if err := CleanupStoppedNetwork(st, c); err != nil {
		t.Fatalf("legacy stopped container cleanup: %v", err)
	}
}
