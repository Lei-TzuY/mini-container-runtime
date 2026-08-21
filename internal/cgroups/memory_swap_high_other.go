//go:build !linux

package cgroups

// ReadMemorySwapCurrent is a non-Linux stub.
func ReadMemorySwapCurrent(cgroupPath string) (uint64, error) {
	return 0, nil
}

// ReadMemorySwapHigh is a non-Linux stub. Returns isMax=true.
func ReadMemorySwapHigh(cgroupPath string) (uint64, bool, error) {
	return 0, true, nil
}

// WriteMemorySwapHigh is a non-Linux stub.
func WriteMemorySwapHigh(cgroupPath string, limitBytes int64) error {
	return nil
}
