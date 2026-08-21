//go:build !linux

package cgroups

// ReadCPUMaxBurst is a non-Linux stub.
func ReadCPUMaxBurst(cgroupPath string) (uint64, error) {
	return 0, nil
}

// WriteCPUMaxBurst is a non-Linux stub.
func WriteCPUMaxBurst(cgroupPath string, burstUsec uint64) error {
	return nil
}
