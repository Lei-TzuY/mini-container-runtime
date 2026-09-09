//go:build linux

package container

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/unix"
)

const maskedDirectoriesEnvKey = "MINICONTAINER_MASKED_DIRECTORIES"

func appendMaskedDirectoriesEnv(env []string, paths []string) ([]string, error) {
	prefix := maskedDirectoriesEnvKey + "="
	clean := make([]string, 0, len(env)+1)
	for _, entry := range env {
		if !strings.HasPrefix(entry, prefix) {
			clean = append(clean, entry)
		}
	}
	if len(paths) == 0 {
		return clean, nil
	}
	if err := validateMaskedDirectories(paths); err != nil {
		return nil, err
	}
	data, err := json.Marshal(paths)
	if err != nil {
		return nil, fmt.Errorf("encode masked directory policy: %w", err)
	}
	return append(clean, prefix+string(data)), nil
}

func consumeMaskedDirectoriesEnv() ([]string, error) {
	raw, ok := os.LookupEnv(maskedDirectoriesEnvKey)
	if !ok {
		return nil, nil
	}
	if err := os.Unsetenv(maskedDirectoriesEnvKey); err != nil {
		return nil, fmt.Errorf("clear masked directory policy environment: %w", err)
	}
	var paths []string
	if err := json.Unmarshal([]byte(raw), &paths); err != nil {
		return nil, fmt.Errorf("decode masked directory policy: %w", err)
	}
	if err := validateMaskedDirectories(paths); err != nil {
		return nil, err
	}
	return paths, nil
}

func validateMaskedDirectories(paths []string) error {
	seen := make(map[string]struct{}, len(paths))
	for _, target := range paths {
		if target == "" || !filepath.IsAbs(target) || filepath.Clean(target) != target || target == "/" {
			return fmt.Errorf("masked directory target %q must be a canonical absolute path below root", target)
		}
		if _, duplicate := seen[target]; duplicate {
			return fmt.Errorf("duplicate masked directory target %q", target)
		}
		seen[target] = struct{}{}
	}
	return nil
}

func mountMaskedDirectoryInRoot(rootfs, containerPath string) error {
	rel, err := normalizeVolumeContainerPath(containerPath)
	if err != nil {
		return err
	}
	if rel == "." {
		return fmt.Errorf("masked directory target must be below root")
	}

	rootFD, err := unix.Open(rootfs, unix.O_PATH|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	if err != nil {
		return fmt.Errorf("open rootfs %q for masked directory: %w", rootfs, err)
	}
	defer unix.Close(rootFD)

	targetFD, err := openVolumeDirInRoot(rootFD, rel)
	if errors.Is(err, unix.ENOSYS) {
		targetFD, err = openExistingMaskedDirectoryNoSymlink(rootFD, rel)
	}
	if err != nil {
		return fmt.Errorf("resolve masked directory target %q: %w", containerPath, err)
	}
	defer unix.Close(targetFD)

	if err := mountMaskedDirectory(volumeTargetFDPath(targetFD)); err != nil {
		return fmt.Errorf("mask directory %q: %w", containerPath, err)
	}
	return nil
}

func openExistingMaskedDirectoryNoSymlink(rootFD int, rel string) (int, error) {
	currentFD, err := unix.Openat(rootFD, ".", unix.O_PATH|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return -1, err
	}
	for _, component := range strings.Split(rel, "/") {
		nextFD, openErr := unix.Openat(currentFD, component, unix.O_PATH|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
		if openErr != nil {
			_ = unix.Close(currentFD)
			return -1, openErr
		}
		_ = unix.Close(currentFD)
		currentFD = nextFD
	}
	return currentFD, nil
}
