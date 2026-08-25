//go:build !linux

package container

import "testing"

func TestSecureVolumeTargetResolverIsLinuxOnly(t *testing.T) {
	// Container run/mount isolation is Linux-only. This file intentionally keeps
	// non-Linux test discovery explicit so platform builds do not accidentally
	// grow a pathname-based volume mount implementation without equivalent
	// kernel containment guarantees.
}
