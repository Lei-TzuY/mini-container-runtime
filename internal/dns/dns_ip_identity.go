package dns

import (
	"fmt"
	"net"
)

func canonicalIPAddress(ipAddr string) (string, error) {
	if ipAddr == "" {
		return "", fmt.Errorf("IP address cannot be empty")
	}
	parsed := net.ParseIP(ipAddr)
	if parsed == nil {
		return "", fmt.Errorf("invalid IP address %q", ipAddr)
	}
	return parsed.String(), nil
}
