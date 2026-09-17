//go:build linux

package container

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCreateParentRuntimeWorkDirUsesPrivatePrefix(t *testing.T) {
	dir, err := createParentRuntimeWorkDir(func(parent, pattern string) (string, error) {
		if parent != "" || pattern != runtimeWorkDirPrefix+"*" {
			t.Fatalf("mkdir args parent=%q pattern=%q", parent, pattern)
		}
		return "/tmp/" + runtimeWorkDirPrefix + "123", nil
	})
	if err != nil {
		t.Fatalf("createParentRuntimeWorkDir: %v", err)
	}
	if dir != "/tmp/"+runtimeWorkDirPrefix+"123" {
		t.Fatalf("workdir = %q", dir)
	}
}

func TestCreateParentRuntimeWorkDirFailuresAreRuntimeControl(t *testing.T) {
	cause := errors.New("tmp storage unavailable")
	for _, allocate := range []runtimeMkdirTemp{
		nil,
		func(string, string) (string, error) { return "", nil },
		func(string, string) (string, error) { return "", cause },
	} {
		_, err := createParentRuntimeWorkDir(allocate)
		if err == nil || !isRuntimeControlError(err) {
			t.Fatalf("allocation error = %v, want runtime-control failure", err)
		}
	}
}

func TestAppendRuntimeWorkDirEnv(t *testing.T) {
	base := []string{"PATH=/bin"}
	if got := appendRuntimeWorkDirEnv(base, ""); len(got) != 1 {
		t.Fatalf("empty workdir changed environment: %q", got)
	}
	got := appendRuntimeWorkDirEnv(base, "/tmp/"+runtimeWorkDirPrefix+"1")
	if got[len(got)-1] != runtimeWorkDirEnv+"=/tmp/"+runtimeWorkDirPrefix+"1" {
		t.Fatalf("runtime workdir marker = %q", got[len(got)-1])
	}
}

func TestClearRuntimeControlEnvironmentRemovesAmbientNamespaceOnly(t *testing.T) {
	for key, value := range map[string]string{
		sentinelEnvKey:                  "1",
		execSentinelKey:                 "1",
		execStartTimeKey:                "123",
		runtimeWorkDirEnv:               "/tmp/owned",
		"MINICONTAINER_DEBUG":          "1",
		"MINICONTAINER_FUTURE_CONTROL": "secret",
	} {
		t.Setenv(key, value)
	}
	t.Setenv("MINI_CONTAINER_USER_VALUE", "keep")
	t.Setenv("PATH", "/bin:/usr/bin")

	if err := clearRuntimeControlEnvironment(); err != nil {
		t.Fatalf("clearRuntimeControlEnvironment: %v", err)
	}
	for _, key := range []string{
		sentinelEnvKey,
		execSentinelKey,
		execStartTimeKey,
		runtimeWorkDirEnv,
		"MINICONTAINER_DEBUG",
		"MINICONTAINER_FUTURE_CONTROL",
	} {
		if _, ok := os.LookupEnv(key); ok {
			t.Fatalf("runtime control environment %q survived isolation", key)
		}
	}
	if got := os.Getenv("MINI_CONTAINER_USER_VALUE"); got != "keep" {
		t.Fatalf("non-runtime environment changed: %q", got)
	}
	if got := os.Getenv("PATH"); got != "/bin:/usr/bin" {
		t.Fatalf("ordinary PATH changed: %q", got)
	}
}

func TestConsumeRuntimeWorkDirAcceptsPrivateParentDirectory(t *testing.T) {
	base := t.TempDir()
	dir, err := os.MkdirTemp(base, runtimeWorkDirPrefix+"*")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	t.Setenv(runtimeWorkDirEnv, dir)
	t.Setenv(sentinelEnvKey, "1")

	got, err := consumeRuntimeWorkDir()
	if err != nil {
		t.Fatalf("consumeRuntimeWorkDir: %v", err)
	}
	if got != dir {
		t.Fatalf("workdir = %q, want %q", got, dir)
	}
	if _, ok := os.LookupEnv(runtimeWorkDirEnv); ok {
		t.Fatal("runtime workdir marker survived consumption")
	}
	if _, ok := os.LookupEnv(sentinelEnvKey); !ok {
		t.Fatal("unrelated runtime marker was consumed too early")
	}
}

func TestConsumeRuntimeWorkDirRejectsInvalidPaths(t *testing.T) {
	t.Setenv(runtimeWorkDirEnv, "")
	if _, err := consumeRuntimeWorkDir(); err == nil || !strings.Contains(err.Error(), "did not provide") {
		t.Fatalf("missing workdir error = %v", err)
	}

	t.Setenv(runtimeWorkDirEnv, "relative/path")
	if _, err := consumeRuntimeWorkDir(); err == nil || !strings.Contains(err.Error(), "not absolute") {
		t.Fatalf("relative workdir error = %v", err)
	}

	t.Setenv(runtimeWorkDirEnv, t.TempDir())
	if _, err := consumeRuntimeWorkDir(); err == nil || !strings.Contains(err.Error(), "unexpected name") {
		t.Fatalf("unexpected-name workdir error = %v", err)
	}
}

func TestConsumeRuntimeWorkDirRejectsSymlinkAndPublicDirectory(t *testing.T) {
	base := t.TempDir()
	realDir, err := os.MkdirTemp(base, runtimeWorkDirPrefix+"real-*")
	if err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(base, runtimeWorkDirPrefix+"link")
	if err := os.Symlink(realDir, link); err != nil {
		t.Fatal(err)
	}
	t.Setenv(runtimeWorkDirEnv, link)
	if _, err := consumeRuntimeWorkDir(); err == nil || !strings.Contains(err.Error(), "not a real directory") {
		t.Fatalf("symlink workdir error = %v", err)
	}

	publicDir, err := os.MkdirTemp(base, runtimeWorkDirPrefix+"public-*")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(publicDir, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv(runtimeWorkDirEnv, publicDir)
	if _, err := consumeRuntimeWorkDir(); err == nil || !strings.Contains(err.Error(), "not private") {
		t.Fatalf("public workdir error = %v", err)
	}
}

func TestFinishRuntimeWorkDirPreservesAndJoinsErrors(t *testing.T) {
	payloadErr := errors.New("payload exit")
	if got := finishRuntimeWorkDir(payloadErr, "/tmp/owned", func(path string) error {
		if path != "/tmp/owned" {
			t.Fatalf("remove path = %q", path)
		}
		return nil
	}); got != payloadErr {
		t.Fatalf("successful cleanup changed result: %v", got)
	}

	cleanupErr := errors.New("remove denied")
	got := finishRuntimeWorkDir(payloadErr, "/tmp/owned", func(string) error { return cleanupErr })
	if !errors.Is(got, payloadErr) || !errors.Is(got, cleanupErr) || !isRuntimeControlError(got) {
		t.Fatalf("joined cleanup result = %v", got)
	}
	if got := finishRuntimeWorkDir(nil, "/tmp/owned", nil); got == nil || !isRuntimeControlError(got) {
		t.Fatalf("nil remover result = %v", got)
	}
}
