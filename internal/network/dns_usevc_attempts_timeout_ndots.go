package network

import (
	"fmt"
)

// GenerateDNSUseVCAttemptsTimeoutNdotsConfig formats combined options
// use-vc attempts:N timeout:M ndots:K flags for /etc/resolv.conf.
// use-vc forces the use of TCP (virtual circuits) for DNS queries;
// attempts sets query retries; timeout sets query timeout in seconds; ndots sets search domain threshold.
func GenerateDNSUseVCAttemptsTimeoutNdotsConfig(attempts, timeoutSeconds, ndots int) string {
	if attempts <= 0 {
		attempts = 2
	}
	if timeoutSeconds <= 0 {
		timeoutSeconds = 5
	}
	if ndots <= 0 {
		ndots = 1
	}
	return fmt.Sprintf("options use-vc attempts:%d timeout:%d ndots:%d\n", attempts, timeoutSeconds, ndots)
}
