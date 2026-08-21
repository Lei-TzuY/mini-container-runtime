//go:build !linux

package cgroups

// MemoryReclaimOptions holds advanced arguments for Cgroup v2 memory.reclaim.
type MemoryReclaimOptions struct {
	BytesToReclaim int64
	Swappiness     int
	NumaNode       int
}

// ReclaimMemoryWithOptions is a non-Linux stub.
func ReclaimMemoryWithOptions(cgroupPath string, opts MemoryReclaimOptions) error {
	return nil
}
