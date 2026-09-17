//go:build linux

package main

import (
	"syscall"

	"minicontainer/internal/container"
)

const (
	processUIDRuntimeEnv    = "MINICONTAINER_PROCESS_UID"
	processGIDRuntimeEnv    = "MINICONTAINER_PROCESS_GID"
	processGroupsRuntimeEnv = "MINICONTAINER_PROCESS_GROUPS"
	processUmaskRuntimeEnv  = "MINICONTAINER_PROCESS_UMASK"
)

func runContainerInitSupervisor(command []string) (int, error) {
	return container.RunInitSupervisor(command)
}

func payloadCredentialFromRuntimeEnv() (*syscall.Credential, error) {
	return container.PayloadCredentialFromRuntimeEnv()
}

func payloadUmaskFromRuntimeEnv() (*uint32, error) {
	return container.PayloadUmaskFromRuntimeEnv()
}
