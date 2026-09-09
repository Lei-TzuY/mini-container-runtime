//go:build linux

package container

import (
	"errors"
	"fmt"
	"os"
	"path"
	"strconv"
	"strings"

	"golang.org/x/sys/unix"
)

const volumeTargetResolveFlags = unix.RESOLVE_IN_ROOT | unix.RESOLVE_NO_MAGICLINKS

var errVolumeTargetCrossMount = errors.New("volume target creation crosses rootfs mount boundary")

func normalizeVolumeContainerPath(containerPath string) (string, error) {
	if containerPath == "" {
		return "", fmt.Errorf("container mount path must not be empty")
	}
	if strings.IndexByte(containerPath, 0) >= 0 {
		return "", fmt.Errorf("container mount path contains NUL")
	}
	if !strings.HasPrefix(containerPath, "/") {
		return "", fmt.Errorf("container mount path %q must be absolute", containerPath)
	}
	for _, component := range strings.Split(containerPath, "/") {
		if component == ".." {
			return "", fmt.Errorf("container mount path %q contains parent traversal", containerPath)
		}
	}
	cleaned := path.Clean(containerPath)
	if cleaned == "/" {
		return ".", nil
	}
	return strings.TrimPrefix(cleaned, "/"), nil
}

// openVolumeTarget returns an O_PATH fd for an existing file or directory
// target. Missing targets retain the historical directory-creation behavior.
// This lets bind mounts such as OCI masked files target an existing regular
// file without weakening fd-rooted rootfs resolution.
func openVolumeTarget(rootfs, containerPath string) (int, error) {
	rel, err := normalizeVolumeContainerPath(containerPath)
	if err != nil {
		return -1, err
	}
	rootFD, err := unix.Open(rootfs, unix.O_PATH|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	if err != nil {
		return -1, fmt.Errorf("open rootfs %q: %w", rootfs, err)
	}
	defer unix.Close(rootFD)

	if fd, err := openExistingVolumeTargetOpenat2(rootFD, rel); err == nil {
		return fd, nil
	} else if errors.Is(err, unix.ENOSYS) {
		fd, fallbackErr := openExistingVolumeTargetNoSymlink(rootFD, rel)
		if fallbackErr == nil {
			return fd, nil
		}
		if !errors.Is(fallbackErr, unix.ENOENT) {
			return -1, fmt.Errorf("secure existing volume target fallback: %w", fallbackErr)
		}
	} else if !errors.Is(err, unix.ENOENT) {
		return -1, fmt.Errorf("resolve existing volume target %q: %w", rel, err)
	}

	fd, err := openOrCreateVolumeTargetOpenat2(rootFD, rel)
	if err == nil {
		return fd, nil
	}
	if !errors.Is(err, unix.ENOSYS) {
		return -1, err
	}
	fd, err = openOrCreateVolumeTargetNoSymlink(rootFD, rel)
	if err != nil {
		return -1, fmt.Errorf("secure volume target fallback: %w", err)
	}
	return fd, nil
}

func openExistingVolumeTargetOpenat2(rootFD int, rel string) (int, error) {
	how := &unix.OpenHow{Flags: uint64(unix.O_PATH | unix.O_CLOEXEC), Resolve: uint64(volumeTargetResolveFlags)}
	return unix.Openat2(rootFD, rel, how)
}

func openExistingVolumeTargetNoSymlink(rootFD int, rel string) (int, error) {
	currentFD, err := unix.Openat(rootFD, ".", unix.O_PATH|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return -1, err
	}
	if rel == "." {
		return currentFD, nil
	}
	components := strings.Split(rel, "/")
	for i, component := range components {
		flags := unix.O_PATH | unix.O_CLOEXEC | unix.O_NOFOLLOW
		if i < len(components)-1 {
			flags |= unix.O_DIRECTORY
		}
		nextFD, openErr := unix.Openat(currentFD, component, flags, 0)
		_ = unix.Close(currentFD)
		if openErr != nil {
			return -1, openErr
		}
		currentFD = nextFD
	}
	return currentFD, nil
}

func openOrCreateVolumeTargetOpenat2(rootFD int, rel string) (int, error) {
	if fd, err := openVolumeDirInRoot(rootFD, rel); err == nil {
		return fd, nil
	} else if errors.Is(err, unix.ENOSYS) {
		return -1, err
	} else if !errors.Is(err, unix.ENOENT) {
		return -1, fmt.Errorf("resolve volume target %q: %w", rel, err)
	}
	rootMountID, err := fdMountID(rootFD)
	if err != nil {
		return -1, fmt.Errorf("read rootfs mount identity: %w", err)
	}
	components := strings.Split(rel, "/")
	var parentFD = -1
	defer func() { if parentFD >= 0 { _ = unix.Close(parentFD) } }()
	for i, component := range components {
		prefix := strings.Join(components[:i+1], "/")
		fd, err := openVolumeDirInRoot(rootFD, prefix)
		if err == nil {
			if parentFD >= 0 { _ = unix.Close(parentFD) }
			parentFD = fd
			continue
		}
		if errors.Is(err, unix.ENOSYS) { return -1, err }
		if !errors.Is(err, unix.ENOENT) { return -1, fmt.Errorf("resolve volume target component %q: %w", prefix, err) }
		mkdirParent := rootFD
		if parentFD >= 0 { mkdirParent = parentFD }
		if err := requireMountID(mkdirParent, rootMountID, prefix); err != nil { return -1, err }
		if err := unix.Mkdirat(mkdirParent, component, 0o755); err != nil && !errors.Is(err, unix.EEXIST) { return -1, fmt.Errorf("create volume target component %q: %w", prefix, err) }
		fd, err = openVolumeDirInRoot(rootFD, prefix)
		if err != nil { return -1, fmt.Errorf("re-resolve volume target component %q: %w", prefix, err) }
		if parentFD >= 0 { _ = unix.Close(parentFD) }
		parentFD = fd
	}
	if parentFD < 0 { return -1, fmt.Errorf("volume target %q resolved to no directory", rel) }
	fd := parentFD
	parentFD = -1
	return fd, nil
}

func openVolumeDirInRoot(rootFD int, rel string) (int, error) {
	how := &unix.OpenHow{Flags: uint64(unix.O_PATH | unix.O_DIRECTORY | unix.O_CLOEXEC), Resolve: uint64(volumeTargetResolveFlags)}
	return unix.Openat2(rootFD, rel, how)
}

func openOrCreateVolumeTargetNoSymlink(rootFD int, rel string) (int, error) {
	rootMountID, err := fdMountID(rootFD)
	if err != nil { return -1, fmt.Errorf("read rootfs mount identity: %w", err) }
	currentFD, err := unix.Openat(rootFD, ".", unix.O_PATH|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil { return -1, fmt.Errorf("open rootfs fd: %w", err) }
	if rel == "." { return currentFD, nil }
	for _, component := range strings.Split(rel, "/") {
		nextFD, err := unix.Openat(currentFD, component, unix.O_PATH|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
		if errors.Is(err, unix.ENOENT) {
			if mountErr := requireMountID(currentFD, rootMountID, component); mountErr != nil { _ = unix.Close(currentFD); return -1, mountErr }
			if mkdirErr := unix.Mkdirat(currentFD, component, 0o755); mkdirErr != nil && !errors.Is(mkdirErr, unix.EEXIST) { _ = unix.Close(currentFD); return -1, fmt.Errorf("create component %q: %w", component, mkdirErr) }
			nextFD, err = unix.Openat(currentFD, component, unix.O_PATH|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
		}
		if err != nil { _ = unix.Close(currentFD); return -1, fmt.Errorf("open component %q without symlink traversal: %w", component, err) }
		_ = unix.Close(currentFD)
		currentFD = nextFD
	}
	return currentFD, nil
}

func requireMountID(fd int, expected uint64, component string) error {
	actual, err := fdMountID(fd)
	if err != nil { return fmt.Errorf("read mount identity for %q: %w", component, err) }
	if actual != expected { return fmt.Errorf("%w at %q (%d -> %d)", errVolumeTargetCrossMount, component, expected, actual) }
	return nil
}

func fdMountID(fd int) (uint64, error) {
	if fd < 0 { return 0, fmt.Errorf("invalid fd %d", fd) }
	data, err := os.ReadFile("/proc/self/fdinfo/" + strconv.Itoa(fd))
	if err != nil { return 0, fmt.Errorf("read fdinfo: %w", err) }
	for _, line := range strings.Split(string(data), "\n") {
		if !strings.HasPrefix(line, "mnt_id:") { continue }
		raw := strings.TrimSpace(strings.TrimPrefix(line, "mnt_id:"))
		id, err := strconv.ParseUint(raw, 10, 64)
		if err != nil { return 0, fmt.Errorf("parse mnt_id %q: %w", raw, err) }
		if id == 0 { return 0, fmt.Errorf("invalid zero mnt_id") }
		return id, nil
	}
	return 0, fmt.Errorf("mnt_id missing from fdinfo")
}

func volumeTargetFDPath(fd int) string { return "/proc/self/fd/" + strconv.Itoa(fd) }
