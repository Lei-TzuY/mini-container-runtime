package network

import (
	"testing"
)

func TestSetInterfaceMTU(t *testing.T) {
	if err := SetInterfaceMTU("lo", 1500); err != nil {
		t.Fatalf("SetInterfaceMTU error: %v", err)
	}
}
