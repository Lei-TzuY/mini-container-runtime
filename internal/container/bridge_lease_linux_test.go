//go:build linux

package container

import "testing"

func TestAllocateRuntimeBridgeLeaseIsGenerationScoped(t *testing.T) {
	cfg := Config{ContainerID: "bridge-generation-test", StateDir: t.TempDir()}

	ip1, config1, release1, err := allocateRuntimeBridgeLease(cfg, 101, 1001)
	if err != nil {
		t.Fatal(err)
	}
	if ip1 != "172.20.0.2" || config1.ContainerCIDR != "172.20.0.2/24" || config1.Gateway != "172.20.0.1" {
		t.Fatalf("first lease = ip %q config %+v", ip1, config1)
	}

	ip2, config2, release2, err := allocateRuntimeBridgeLease(cfg, 202, 2002)
	if err != nil {
		t.Fatal(err)
	}
	if ip2 != "172.20.0.3" || config2.ContainerCIDR != "172.20.0.3/24" || config2.Gateway != "172.20.0.1" {
		t.Fatalf("second lease = ip %q config %+v", ip2, config2)
	}

	if err := release1(); err != nil {
		t.Fatal(err)
	}
	ip3, _, release3, err := allocateRuntimeBridgeLease(cfg, 303, 3003)
	if err != nil {
		t.Fatal(err)
	}
	if ip3 != "172.20.0.2" {
		t.Fatalf("third generation IP = %q, want recycled 172.20.0.2", ip3)
	}

	if err := release2(); err != nil {
		t.Fatal(err)
	}
	if err := release3(); err != nil {
		t.Fatal(err)
	}
}
