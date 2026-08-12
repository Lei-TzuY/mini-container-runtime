package dns

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestContainerDNS(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	t.Setenv("USERPROFILE", tmpHome)

	netName := "demo-net"
	ctrID1 := "ctr111"
	ctrID2 := "ctr222"

	if err := RegisterHost(netName, ctrID1, "web", "172.20.0.2"); err != nil {
		t.Fatalf("RegisterHost ctr1 error: %v", err)
	}

	if err := RegisterHost(netName, ctrID2, "db", "172.20.0.3"); err != nil {
		t.Fatalf("RegisterHost ctr2 error: %v", err)
	}

	hostsContent := GenerateHostsContent(netName)
	if !strings.Contains(hostsContent, "172.20.0.2\tweb") || !strings.Contains(hostsContent, "172.20.0.3\tdb") {
		t.Fatalf("Hosts content missing expected entries:\n%s", hostsContent)
	}

	rootfs := filepath.Join(tmpHome, "rootfs")
	if err := InjectHostsIntoRootFS(rootfs, netName); err != nil {
		t.Fatalf("InjectHostsIntoRootFS error: %v", err)
	}

	etcHosts := filepath.Join(rootfs, "etc", "hosts")
	data, err := os.ReadFile(etcHosts)
	if err != nil || !strings.Contains(string(data), "172.20.0.3\tdb") {
		t.Fatalf("Injected /etc/hosts invalid: %v", err)
	}

	if err := UnregisterHost(netName, ctrID1); err != nil {
		t.Fatalf("UnregisterHost error: %v", err)
	}

	updatedContent := GenerateHostsContent(netName)
	if strings.Contains(updatedContent, "172.20.0.2\tweb") {
		t.Fatalf("Unregistered web host should not be in content:\n%s", updatedContent)
	}
}
