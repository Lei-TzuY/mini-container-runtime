package network

import (
	"fmt"
	"os/exec"
	"runtime"
)

// SetInterfaceMTU configures the MTU byte size for a network interface.
func SetInterfaceMTU(ifName string, mtu int) error {
	if ifName == "" || mtu <= 0 {
		return nil
	}

	if runtime.GOOS != "linux" {
		return nil
	}

	cmd := exec.Command("ip", "link", "set", "dev", ifName, "mtu", fmt.Sprintf("%d", mtu))
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("set mtu failed: %s (%w)", string(output), err)
	}

	return nil
}
