package network

import (
	"testing"
)

func TestCreateOverlayInterface(t *testing.T) {
	cfg := OverlayConfig{
		VNI:           100,
		RemoteIP:      "192.168.1.50",
		InterfaceName: "vxlan100",
	}

	err := CreateOverlayInterface(cfg)
	if err != nil {
		t.Fatalf("CreateOverlayInterface error: %v", err)
	}
}
