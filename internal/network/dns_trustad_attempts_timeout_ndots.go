package network

import (
	"fmt"
)

// GenerateDNSTrustADAttemptsTimeoutNdotsConfig formats combined options
// trust-ad attempts:N timeout:M ndots:K flags for /etc/resolv.conf.
// trust-ad sets the AD (Authenticated Data) bit in DNSSEC queries; attempts sets retries;
// timeout sets timeout in seconds; ndots sets threshold for absolute domain lookups.
func GenerateDNSTrustADAttemptsTimeoutNdotsConfig(attempts, timeoutSeconds, ndots int) string {
	if attempts <= 0 {
		attempts = 2
	}
	if timeoutSeconds <= 0 {
		timeoutSeconds = 5
	}
	if ndots <= 0 {
		ndots = 1
	}
	return fmt.Sprintf("options trust-ad attempts:%d timeout:%d ndots:%d\n", attempts, timeoutSeconds, ndots)
}
