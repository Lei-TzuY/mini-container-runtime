//go:build linux

package container

import (
	"fmt"

	"golang.org/x/sys/unix"
)

type unshareCall func(flags int) error

func prepareExecThread() error {
	return prepareExecThreadWith(unix.Unshare)
}

func prepareExecThreadWith(unshare unshareCall) error {
	if unshare == nil {
		return fmt.Errorf("exec thread unshare function is nil")
	}
	if err := unshare(unix.CLONE_FS); err != nil {
		return fmt.Errorf("unshare CLONE_FS before namespace entry: %w", err)
	}
	return nil
}
