//go:build linux

package container

import "os/exec"

// startContainerProcess is the last parent-side admission gate before the
// kernel creates a child process. Managed runs revalidate the filesystem object
// pinned at CLI admission here so setup performed earlier in the attempt cannot
// widen the RootFS pathname TOCTOU window all the way to exec.Cmd.Start.
func startContainerProcess(cfg Config, cmd *exec.Cmd) error {
	if err := validateAdmittedRootFSIdentity(cfg); err != nil {
		return err
	}
	return cmd.Start()
}
