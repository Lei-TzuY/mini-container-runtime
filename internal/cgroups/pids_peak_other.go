//go:build !linux

package cgroups

// ReadPIDSPeak is a non-Linux stub.
func ReadPIDSPeak(cgroupPath string) (uint64, error) {
	return 0, nil
}

// ResetPIDSPeak is a non-Linux stub.
func ResetPIDSPeak(cgroupPath string) error {
	return nil
}
