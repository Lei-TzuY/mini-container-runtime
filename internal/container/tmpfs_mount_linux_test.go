//go:build linux

package container

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

func TestTmpfsMountEnvRoundTripAndClear(t *testing.T) {
	mounts := []TmpfsMount{{ContainerPath: "/run/cache", Options: []string{"nosuid", "nodev", "mode=0755", "size=64k"}}}
	env, err := appendTmpfsMountsEnv([]string{"BASE=1"}, mounts)
	if err != nil {
		t.Fatal(err)
	}
	var raw string
	for _, item := range env {
		if len(item) > len(tmpfsMountsEnvKey) && item[:len(tmpfsMountsEnvKey)+1] == tmpfsMountsEnvKey+"=" {
			raw = item[len(tmpfsMountsEnvKey)+1:]
			break
		}
	}
	if raw == "" {
		t.Fatal("tmpfs mount environment was not appended")
	}
	t.Setenv(tmpfsMountsEnvKey, raw)
	got, err := consumeTmpfsMountsEnv()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ContainerPath != mounts[0].ContainerPath || len(got[0].Options) != 4 {
		t.Fatalf("unexpected tmpfs mounts: %#v", got)
	}
	if value := os.Getenv(tmpfsMountsEnvKey); value != "" {
		t.Fatalf("tmpfs runtime environment leaked after consume: %q", value)
	}
}

func TestTmpfsMountRejectsUnsupportedOption(t *testing.T) {
	_, _, err := tmpfsMountFlagsAndData(TmpfsMount{ContainerPath: "/cache", Options: []string{"bind"}})
	if err == nil {
		t.Fatal("expected unsupported tmpfs option to fail closed")
	}
}

func TestMountTmpfsMountKernel(t *testing.T) {
	root := t.TempDir()
	mount := TmpfsMount{ContainerPath: "/cache", Options: []string{"nosuid", "nodev", "mode=0755", "size=64k"}}
	if err := mountTmpfsMount(mount, root, false); err != nil {
		if errors.Is(err, syscall.EPERM) || errors.Is(err, syscall.EACCES) {
			t.Skipf("tmpfs mount requires CAP_SYS_ADMIN: %v", err)
		}
		t.Fatal(err)
	}
	target := filepath.Join(root, "cache")
	defer syscall.Unmount(target, 0)
	if err := os.WriteFile(filepath.Join(target, "probe"), []byte("ok"), 0o600); err != nil {
		t.Fatalf("write through tmpfs mount: %v", err)
	}
	var stat syscall.Statfs_t
	if err := syscall.Statfs(target, &stat); err != nil {
		t.Fatal(err)
	}
	const tmpfsMagic = 0x01021994
	if uint64(stat.Type) != uint64(tmpfsMagic) {
		t.Fatalf("mounted filesystem type = %#x, want tmpfs %#x", stat.Type, tmpfsMagic)
	}
}
