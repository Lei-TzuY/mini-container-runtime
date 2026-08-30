package dns

import "fmt"

const (
	dnsFilenameMaxBytes         = 255
	dnsRegistryTempNonceHexBytes = 16
	dnsRegistryTempFixedBytes    = len(".") + len(".json.tmp-") + dnsRegistryTempNonceHexBytes
	maxDNSNetworkNameBytes       = dnsFilenameMaxBytes - dnsRegistryTempFixedBytes
)

func validateDNSNetworkFilenameLength(networkName string) error {
	if len(networkName) > maxDNSNetworkNameBytes {
		return fmt.Errorf("invalid network name %q: exceeds %d-byte filesystem-safe limit", networkName, maxDNSNetworkNameBytes)
	}
	return nil
}
