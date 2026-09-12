//go:build linux

package container

import (
	"os/exec"
	"syscall"
)

func configureDetachedExecCommand(cmd *exec.Cmd) {
	cmd.Stdin = nil
	cmd.Stdout = nil
	cmd.Stderr = nil
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
}
