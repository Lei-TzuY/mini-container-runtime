//go:build linux

package dns

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"minicontainer/internal/state"
)

func TestDNSDeadRegistrarEntryIsAdoptedByLiveContainerGeneration(t *testing.T) {
	useTempDNSHome(t)
	identity, err := currentRegistrarIdentity()
	if err != nil {
		t.Fatal(err)
	}

	const containerID = "orphan-live-container"
	st, err := state.Open(state.DefaultDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Save(&state.Container{
		ID:        containerID,
		Status:    state.StatusCreated,
		RootFS:    "/tmp/rootfs",
		Command:   []string{"true"},
		Hostname:  "orphan-host",
		CreatedAt: time.Now(),
	}); err != nil {
		_ = st.Close()
		t.Fatal(err)
	}
	if err := st.MarkRunning(containerID, os.Getpid(), identity.StartTime, time.Now()); err != nil {
		_ = st.Close()
		t.Fatal(err)
	}
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}

	dir, err := ensureDNSDir()
	if err != nil {
		t.Fatal(err)
	}
	entry := HostEntry{
		ContainerID:    containerID,
		Hostname:       "orphan-host",
		IP:             "10.0.0.7",
		OwnerPID:       99999999,
		OwnerStartTime: 1,
	}
	if err := saveEntriesAtomic(dir, filepath.Join(dir, "default.json"), "default", []HostEntry{entry}); err != nil {
		t.Fatal(err)
	}

	content, err := GenerateHostsContentChecked("default")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(content, "10.0.0.7\torphan-host") {
		t.Fatalf("live orphan container registration was pruned:\n%s", content)
	}

	st, err = state.Open(state.DefaultDir())
	if err != nil {
		t.Fatal(err)
	}
	changed, err := st.MarkStoppedIfIdentity(containerID, os.Getpid(), identity.StartTime, 0, time.Now())
	if err != nil {
		_ = st.Close()
		t.Fatal(err)
	}
	if !changed {
		_ = st.Close()
		t.Fatal("live orphan generation did not transition to stopped")
	}
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}

	content, err = GenerateHostsContentChecked("default")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(content, "orphan-host") {
		t.Fatalf("stopped orphan registration remained:\n%s", content)
	}
}

func TestDNSDeadRegistrarCreatedStateDoesNotKeepEntryAlive(t *testing.T) {
	useTempDNSHome(t)
	const containerID = "abandoned-created-container"
	st, err := state.Open(state.DefaultDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Save(&state.Container{
		ID:        containerID,
		Status:    state.StatusCreated,
		RootFS:    "/tmp/rootfs",
		Command:   []string{"true"},
		Hostname:  "abandoned-host",
		CreatedAt: time.Now(),
	}); err != nil {
		_ = st.Close()
		t.Fatal(err)
	}
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}

	dir, err := ensureDNSDir()
	if err != nil {
		t.Fatal(err)
	}
	entry := HostEntry{
		ContainerID:    containerID,
		Hostname:       "abandoned-host",
		IP:             "10.0.0.8",
		OwnerPID:       99999999,
		OwnerStartTime: 1,
	}
	if err := saveEntriesAtomic(dir, filepath.Join(dir, "default.json"), "default", []HostEntry{entry}); err != nil {
		t.Fatal(err)
	}
	content, err := GenerateHostsContentChecked("default")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(content, "abandoned-host") {
		t.Fatalf("abandoned created-state registration survived:\n%s", content)
	}
}
