package cgroups

import "errors"

// ErrPIDSPeakUnavailable reports that the kernel/platform does not expose
// the cgroup v2 pids.peak read-only telemetry file.
var ErrPIDSPeakUnavailable = errors.New("cgroup pids.peak is unavailable")
