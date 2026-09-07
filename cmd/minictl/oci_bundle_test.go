package main

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func writeOCIBundle(t *testing.T, body string) string {
	t.Helper()
	d := t.TempDir()
	if err := os.Mkdir(filepath.Join(d, "rootfs"), 0755); err != nil { t.Fatal(err) }
	if err := os.WriteFile(filepath.Join(d, "config.json"), []byte(body), 0644); err != nil { t.Fatal(err) }
	return d
}

func TestLoadOCIBundleTranslatesExecutionConfig(t *testing.T) {
	d := writeOCIBundle(t, `{"ociVersion":"1.1.0","root":{"path":"rootfs","readonly":true},"process":{"args":["/bin/app","--serve"],"env":["A=1","B=two"],"cwd":"/work"},"hostname":"demo","linux":{"namespaces":[{"type":"pid"},{"type":"mount"},{"type":"user"}]}}`)
	cfg, err := loadOCIBundle(d)
	if err != nil { t.Fatal(err) }
	if cfg.RootFS != filepath.Join(d, "rootfs") || !cfg.ReadOnly || cfg.WorkDir != "/work" || cfg.Hostname != "demo" || !cfg.UserNS { t.Fatalf("unexpected config: %+v", cfg) }
	if !reflect.DeepEqual(cfg.Command, []string{"/bin/app", "--serve"}) || !reflect.DeepEqual(cfg.Env, []string{"A=1", "B=two"}) { t.Fatalf("unexpected process config: %+v", cfg) }
}

func TestLoadOCIBundleRejectsUnsupportedOrUnsafeSemantics(t *testing.T) {
	cases := []struct{name, body, want string}{
		{"escaping root", `{"ociVersion":"1.1.0","root":{"path":"../rootfs"},"process":{"args":["/bin/true"],"cwd":"/"}}`, "escapes bundle"},
		{"terminal", `{"ociVersion":"1.1.0","root":{"path":"rootfs"},"process":{"terminal":true,"args":["/bin/true"],"cwd":"/"}}`, "terminal is not supported"},
		{"namespace path", `{"ociVersion":"1.1.0","root":{"path":"rootfs"},"process":{"args":["/bin/true"],"cwd":"/"},"linux":{"namespaces":[{"type":"pid","path":"/proc/1/ns/pid"}]}}`, "joining existing pid namespace"},
		{"unknown field", `{"ociVersion":"1.1.0","root":{"path":"rootfs"},"process":{"args":["/bin/true"],"cwd":"/"},"mystery":true}`, "unknown field"},
	}
	for _, tc := range cases { t.Run(tc.name, func(t *testing.T) { _, err := loadOCIBundle(writeOCIBundle(t, tc.body)); if err == nil || !strings.Contains(err.Error(), tc.want) { t.Fatalf("error=%v want substring %q", err, tc.want) } }) }
}
