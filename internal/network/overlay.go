package network

import (
	"fmt"
	"os/exec"
	"runtime"
)

type OverlayConfig struct {
	VNI           int    `json:"vni"`
	RemoteIP      string `json:"remote_ip"`
	InterfaceName string `json:"interface_name"`
	LocalIP       string `json:"local_ip"`
}

// CreateOverlayInterface creates a Linux VXLAN overlay network device for multi-host communication.
func CreateOverlayInterface(cfg OverlayConfig) error {
	if cfg.InterfaceName == "" {
		cfg.InterfaceName = fmt.Sprintf("vxlan%d", cfg.VNI)
	}

	if runtime.GOOS != "linux" {
		return nil
	}

	cmd := exec.Command("ip", "link", "add", cfg.InterfaceName, "type", "vxlan",
		"id", fmt.Sprintf("%d", cfg.VNI),
		"remote", cfg.RemoteIP,
		"dstport", "4789",
	)

	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("ip link add vxlan failed: %s (%w)", string(output), err)
	}

	setUpCmd := exec.Command("ip", "link", "set", cfg.InterfaceName, "up")
	if output, err := setUpCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("ip link set up vxlan failed: %s (%w)", string(output), err)
	}

	return nil
}
