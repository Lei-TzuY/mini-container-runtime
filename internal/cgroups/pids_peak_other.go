//go:build !linux

package cgroups

// ReadPIDSPeak reports unsupported telemetry on non-Linux platforms.
func ReadPIDSPeak(cgroupPath string) (uint64, error) {
	return 0, ErrPIDSPeakUnavailable
}
