package cgroups

import (
	"fmt"
	"strconv"
)

const managedCgroupPrefix = "minicontainer-"

// NameForContainerProcess returns the cgroup name for one exact container
// process generation. PID start time, unlike a numeric PID, does not change
// meaning when Linux later reuses the PID for another process. Including the
// container ID also keeps restarts of the same container in distinct cgroups.
func NameForContainerProcess(containerID string, pidStartTime uint64) (string, error) {
	if containerID == "" {
		return "", fmt.Errorf("container ID must not be empty")
	}
	if pidStartTime == 0 {
		return "", fmt.Errorf("PID start time must not be zero")
	}

	name := managedCgroupPrefix + containerID + "-" + strconv.FormatUint(pidStartTime, 10)
	if err := validateCgroupName(name); err != nil {
		return "", fmt.Errorf("derive cgroup name: %w", err)
	}
	return name, nil
}
