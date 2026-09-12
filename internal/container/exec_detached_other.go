//go:build !linux

package container

import "os/exec"

func configureDetachedExecCommand(cmd *exec.Cmd) {
	cmd.Stdin = nil
	cmd.Stdout = nil
	cmd.Stderr = nil
}
