package network

import (
	"fmt"
)

// GenerateDNSEDNS0AttemptsTimeoutNdotsConfig formats combined options
// edns0 attempts:N timeout:M ndots:K flags for /etc/resolv.conf.
// edns0 enables Extension Mechanisms for DNS; attempts sets query retries;
// timeout sets query timeout ceiling in seconds; ndots sets threshold for absolute domain lookups.
func GenerateDNSEDNS0AttemptsTimeoutNdotsConfig(attempts, timeoutSeconds, ndots int) string {
	if attempts <= 0 {
		attempts = 2
	}
	if timeoutSeconds <= 0 {
		timeoutSeconds = 5
	}
	if ndots <= 0 {
		ndots = 1
	}
	return fmt.Sprintf("options edns0 attempts:%d timeout:%d ndots:%d\n", attempts, timeoutSeconds, ndots)
}
