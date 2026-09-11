//go:build linux

package container

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

const tmpfsMountsEnvKey = "MINICONTAINER_TMPFS_MOUNTS"

func appendTmpfsMountsEnv(env []string, mounts []TmpfsMount) ([]string, error) {
	if len(mounts) == 0 {
		return env, nil
	}
	for _, mount := range mounts {
		if _, _, err := tmpfsMountFlagsAndData(mount); err != nil {
			return nil, err
		}
	}
	data, err := json.Marshal(mounts)
	if err != nil {
		return nil, fmt.Errorf("marshal tmpfs mounts: %w", err)
	}
	return append(env, tmpfsMountsEnvKey+"="+string(data)), nil
}

func consumeTmpfsMountsEnv() ([]TmpfsMount, error) {
	raw := os.Getenv(tmpfsMountsEnvKey)
	if err := os.Unsetenv(tmpfsMountsEnvKey); err != nil {
		return nil, fmt.Errorf("clear tmpfs mount environment: %w", err)
	}
	if raw == "" {
		return nil, nil
	}
	var mounts []TmpfsMount
	if err := json.Unmarshal([]byte(raw), &mounts); err != nil {
		return nil, fmt.Errorf("decode tmpfs mounts: %w", err)
	}
	for _, mount := range mounts {
		if _, _, err := tmpfsMountFlagsAndData(mount); err != nil {
			return nil, err
		}
	}
	return mounts, nil
}

func mountTmpfsMount(mount TmpfsMount, rootfs string, debug bool) error {
	flags, data, err := tmpfsMountFlagsAndData(mount)
	if err != nil {
		return err
	}
	targetFD, err := openVolumeTarget(rootfs, mount.ContainerPath)
	if err != nil {
		return fmt.Errorf("secure tmpfs target: %w", err)
	}
	defer syscall.Close(targetFD)
	target := volumeTargetFDPath(targetFD)
	if err := syscall.Mount("tmpfs", target, "tmpfs", flags, data); err != nil {
		return fmt.Errorf("mount tmpfs: %w", err)
	}
	if debug {
		fmt.Printf("[init] tmpfs: %s (%s)\n", mount.ContainerPath, strings.Join(mount.Options, ","))
	}
	return nil
}

func tmpfsMountFlagsAndData(mount TmpfsMount) (uintptr, string, error) {
	if mount.ContainerPath == "" || !filepath.IsAbs(mount.ContainerPath) {
		return 0, "", fmt.Errorf("tmpfs container path %q must be absolute", mount.ContainerPath)
	}
	flags := uintptr(0)
	data := make([]string, 0, len(mount.Options))
	for _, raw := range mount.Options {
		option := strings.TrimSpace(raw)
		if option == "" {
			return 0, "", fmt.Errorf("tmpfs mount %s has empty option", mount.ContainerPath)
		}
		switch option {
		case "ro":
			flags |= syscall.MS_RDONLY
		case "rw":
			flags &^= syscall.MS_RDONLY
		case "nosuid":
			flags |= syscall.MS_NOSUID
		case "nodev":
			flags |= syscall.MS_NODEV
		case "noexec":
			flags |= syscall.MS_NOEXEC
		case "strictatime":
			flags |= syscall.MS_STRICTATIME
		default:
			if strings.HasPrefix(option, "size=") || strings.HasPrefix(option, "mode=") || strings.HasPrefix(option, "uid=") || strings.HasPrefix(option, "gid=") || strings.HasPrefix(option, "nr_inodes=") {
				if strings.HasSuffix(option, "=") {
					return 0, "", fmt.Errorf("tmpfs mount %s has invalid option %q", mount.ContainerPath, option)
				}
				data = append(data, option)
				continue
			}
			return 0, "", fmt.Errorf("unsupported tmpfs mount option %q", option)
		}
	}
	return flags, strings.Join(data, ","), nil
}
