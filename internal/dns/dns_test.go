package dns

import (
	"fmt"
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

func TestDNSValidationAndInjectionDefense(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	t.Setenv("USERPROFILE", tmpHome)

	invalidNetworks := []string{"", ".", "..", "../escape", "../../etc", "foo/bar", "colon:net"}
	for _, net := range invalidNetworks {
		if err := RegisterHost(net, "ctr1", "host1", "172.20.0.2"); err == nil {
			t.Errorf("RegisterHost(%q) expected error, got nil", net)
		}
		if err := UnregisterHost(net, "ctr1"); err == nil {
			t.Errorf("UnregisterHost(%q) expected error, got nil", net)
		}
		if content := GenerateHostsContent(net); content != "" {
			t.Errorf("GenerateHostsContent(%q) expected empty string, got %q", net, content)
		}
	}

	invalidHosts := []string{
		"host\ninjection",
		"host\r\ninjection",
		"host\tinjection",
		"host injection",
		"-bad-leading",
		".bad.dot",
		"",
	}
	for _, h := range invalidHosts {
		if err := RegisterHost("valid-net", "ctr1", h, "172.20.0.2"); err == nil {
			t.Errorf("RegisterHost with invalid hostname %q expected error, got nil", h)
		}
	}

	invalidIPs := []string{
		"not-an-ip",
		"172.20.0.2\n1.2.3.4 evil.com",
		"999.999.999.999",
		"",
	}
	for _, ip := range invalidIPs {
		if err := RegisterHost("valid-net", "ctr1", "valid-host", ip); err == nil {
			t.Errorf("RegisterHost with invalid IP %q expected error, got nil", ip)
		}
	}
}

func TestDNSConcurrency(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	t.Setenv("USERPROFILE", tmpHome)

	netName := "concurrent-net"
	done := make(chan bool)

	for i := 0; i < 10; i++ {
		go func(idx int) {
			ctrID := fmt.Sprintf("ctr-%d", idx)
			hostname := fmt.Sprintf("host-%d", idx)
			ip := fmt.Sprintf("10.0.0.%d", idx+2)
			_ = RegisterHost(netName, ctrID, hostname, ip)
			_ = GenerateHostsContent(netName)
			_ = UnregisterHost(netName, ctrID)
			done <- true
		}(i)
	}

	for i := 0; i < 10; i++ {
		<-done
	}
}
