//go:build linux

package dns

import (
	"os"
	"strings"
	"testing"
	"time"

	"minicontainer/internal/state"
)

func TestDNSDeadRegistrarRulesOnlyOwnershipDoesNotProveBridgeAdmission(t *testing.T) {
	useTempDNSHome(t)
	identity, err := currentRegistrarIdentity()
	if err != nil {
		t.Fatal(err)
	}

	const containerID = "rules-only-dns-owner"
	st, err := state.Open(state.DefaultDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Save(&state.Container{
		ID:        containerID,
		Status:    state.StatusCreated,
		RootFS:    "/tmp/rootfs",
		Command:   []string{"true"},
		Hostname:  "rules-only-host",
		CreatedAt: time.Now(),
	}); err != nil {
		_ = st.Close()
		t.Fatal(err)
	}
	if err := st.MarkRunning(containerID, os.Getpid(), identity.StartTime, time.Now()); err != nil {
		_ = st.Close()
		t.Fatal(err)
	}
	if err := st.MarkNetworkOwnedIfIdentity(containerID, state.NetworkOwnership{
		Owner:        "minicontainer:dns-rules-only-test",
		PID:          os.Getpid(),
		PIDStartTime: identity.StartTime,
		Mappings: []state.PortForwardingOwnership{{
			HostPort:      18080,
			ContainerPort: 8080,
			ContainerIP:   "172.20.0.2",
			Protocol:      "tcp",
		}},
	}); err != nil {
		_ = st.Close()
		t.Fatalf("persist rules-only ownership: %v", err)
	}
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}

	saveDeadRegistrarEntry(t, HostEntry{
		ContainerID:    containerID,
		Hostname:       "rules-only-host",
		IP:             "172.20.0.2",
		OwnerPID:       99999999,
		OwnerStartTime: 1,
	})

	content, err := GenerateHostsContentChecked("default")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(content, "172.20.0.2\trules-only-host") {
		t.Fatalf("rules-only network ownership incorrectly adopted DNS entry:\n%s", content)
	}
}
