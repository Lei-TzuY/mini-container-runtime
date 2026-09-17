//go:build linux

package container

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	runtimeWorkDirEnv       = "MINICONTAINER_RUNTIME_WORKDIR"
	runtimeWorkDirPrefix    = "minicontainer-runtime-"
	runtimeControlEnvPrefix = "MINICONTAINER_"
)

type runtimeMkdirTemp func(dir, pattern string) (string, error)
type runtimeRemoveAll func(path string) error

// createParentRuntimeWorkDir allocates child mount-staging storage before the
// child starts, so the parent owns the exact path and can remove it after the
// child exits. Allocation failure is a runtime-control failure: no payload
// restart can repair missing isolation storage.
func createParentRuntimeWorkDir(mkdirTemp runtimeMkdirTemp) (string, error) {
	if mkdirTemp == nil {
		return "", &runtimeSetupError{err: fmt.Errorf("create runtime workdir: mkdir operation is nil")}
	}
	dir, err := mkdirTemp("", runtimeWorkDirPrefix+"*")
	if err != nil {
		return "", &runtimeSetupError{err: fmt.Errorf("create runtime workdir: %w", err)}
	}
	if dir == "" {
		return "", &runtimeSetupError{err: fmt.Errorf("create runtime workdir: empty path returned")}
	}
	return dir, nil
}

func appendRuntimeWorkDirEnv(env []string, dir string) []string {
	if dir == "" {
		return env
	}
	return append(env, runtimeWorkDirEnv+"="+dir)
}

// clearRuntimeControlEnvironment removes ambient bootstrap/control variables
// after their dedicated consumers have run. Explicit payload environment
// entries are carried in Config.Env and appended afterwards.
func clearRuntimeControlEnvironment() error {
	for _, entry := range os.Environ() {
		key, _, ok := strings.Cut(entry, "=")
		if !ok || !strings.HasPrefix(key, runtimeControlEnvPrefix) {
			continue
		}
		if err := os.Unsetenv(key); err != nil {
			return fmt.Errorf("clear runtime control environment %q: %w", key, err)
		}
	}
	return nil
}

// consumeRuntimeWorkDir consumes only the parent-issued staging directory.
// Other bootstrap markers remain available to their dedicated consumers.
func consumeRuntimeWorkDir() (string, error) {
	dir, present := os.LookupEnv(runtimeWorkDirEnv)
	if present {
		if err := os.Unsetenv(runtimeWorkDirEnv); err != nil {
			return "", fmt.Errorf("clear runtime workdir environment: %w", err)
		}
	}
	if dir == "" {
		return "", fmt.Errorf("runtime parent did not provide a workdir")
	}
	if !filepath.IsAbs(dir) {
		return "", fmt.Errorf("runtime workdir %q is not absolute", dir)
	}
	if !strings.HasPrefix(filepath.Base(dir), runtimeWorkDirPrefix) {
		return "", fmt.Errorf("runtime workdir %q has an unexpected name", dir)
	}

	info, err := os.Lstat(dir)
	if err != nil {
		return "", fmt.Errorf("inspect runtime workdir %q: %w", dir, err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return "", fmt.Errorf("runtime workdir %q is not a real directory", dir)
	}
	if info.Mode().Perm()&0o077 != 0 {
		return "", fmt.Errorf("runtime workdir %q is not private: mode %04o", dir, info.Mode().Perm())
	}
	return dir, nil
}

// finishRuntimeWorkDir removes the exact parent-owned path on every exit path.
// Cleanup errors remain runtime-control failures and are joined with any
// existing payload/setup error rather than replacing it.
func finishRuntimeWorkDir(resultErr error, dir string, removeAll runtimeRemoveAll) error {
	if dir == "" {
		return resultErr
	}
	if removeAll == nil {
		cleanupErr := &runtimeSetupError{err: fmt.Errorf("cleanup runtime workdir %q: remove operation is nil", dir)}
		return errors.Join(resultErr, cleanupErr)
	}
	if err := removeAll(dir); err != nil {
		cleanupErr := &runtimeSetupError{err: fmt.Errorf("cleanup runtime workdir %q: %w", dir, err)}
		return errors.Join(resultErr, cleanupErr)
	}
	return resultErr
}
