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
	if err := os.Mkdir(filepath.Join(d, "rootfs"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(d, "config.json"), []byte(body), 0644); err != nil {
		t.Fatal(err)
	}
	return d
}

func TestLoadOCIBundleTranslatesExecutionConfig(t *testing.T) {
	d := writeOCIBundle(t, `{"ociVersion":"1.1.0","root":{"path":"rootfs","readonly":true},"process":{"args":["/bin/app","--serve"],"env":["A=1","B=two"],"cwd":"/work"},"hostname":"demo","mounts":[{"destination":"/data","type":"bind","source":"/srv/data","options":["rbind","ro"]}],"linux":{"namespaces":[{"type":"pid"},{"type":"mount"},{"type":"user"}]}}`)
	cfg, err := loadOCIBundle(d)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.RootFS != filepath.Join(d, "rootfs") || !cfg.ReadOnly || cfg.WorkDir != "/work" || cfg.Hostname != "demo" || !cfg.UserNS {
		t.Fatalf("unexpected config: %+v", cfg)
	}
	if !reflect.DeepEqual(cfg.Command, []string{"/bin/app", "--serve"}) || !reflect.DeepEqual(cfg.Env, []string{"A=1", "B=two"}) {
		t.Fatalf("unexpected process config: %+v", cfg)
	}
	if !reflect.DeepEqual(cfg.Volumes, []struct{ HostPath, ContainerPath string; ReadOnly bool }{}) {
		// Keep the assertion below typed against the runtime model; this branch only prevents accidental omission.
	}
	if len(cfg.Volumes) != 1 || cfg.Volumes[0].HostPath != "/srv/data" || cfg.Volumes[0].ContainerPath != "/data" || !cfg.Volumes[0].ReadOnly {
		t.Fatalf("unexpected OCI bind mounts: %+v", cfg.Volumes)
	}
}

func TestLoadOCIBundleTranslatesLinuxResources(t *testing.T) {
	d := writeOCIBundle(t, `{"ociVersion":"1.1.0","root":{"path":"rootfs"},"process":{"args":["/bin/true"],"cwd":"/"},"linux":{"resources":{"memory":{"limit":67108864},"cpu":{"shares":1024,"quota":50000,"period":100000},"pids":{"limit":32}}}}`)
	cfg, err := loadOCIBundle(d)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Memory != 67108864 || cfg.PidsLimit != 32 || cfg.CPUs != 0.5 {
		t.Fatalf("unexpected resource config: %+v", cfg)
	}
	wantWeight := int64(1 + (uint64(1024)-2)*9999/262142)
	if cfg.CPUWeight != wantWeight {
		t.Fatalf("CPUWeight=%d want %d", cfg.CPUWeight, wantWeight)
	}
}

func TestLoadOCIBundleRejectsUnsupportedOrUnsafeSemantics(t *testing.T) {
	cases := []struct {
		name string
		body string
		want string
	}{
		{"escaping root", `{"ociVersion":"1.1.0","root":{"path":"../rootfs"},"process":{"args":["/bin/true"],"cwd":"/"}}`, "escapes bundle"},
		{"terminal", `{"ociVersion":"1.1.0","root":{"path":"rootfs"},"process":{"terminal":true,"args":["/bin/true"],"cwd":"/"}}`, "terminal is not supported"},
		{"namespace path", `{"ociVersion":"1.1.0","root":{"path":"rootfs"},"process":{"args":["/bin/true"],"cwd":"/"},"linux":{"namespaces":[{"type":"pid","path":"/proc/1/ns/pid"}]}}`, "joining existing pid namespace"},
		{"unknown field", `{"ociVersion":"1.1.0","root":{"path":"rootfs"},"process":{"args":["/bin/true"],"cwd":"/"},"mystery":true}`, "unknown field"},
		{"unsupported mount type", `{"ociVersion":"1.1.0","root":{"path":"rootfs"},"process":{"args":["/bin/true"],"cwd":"/"},"mounts":[{"destination":"/sys","type":"sysfs","source":"sysfs"}]}`, "unsupported OCI mount type"},
		{"relative bind source", `{"ociVersion":"1.1.0","root":{"path":"rootfs"},"process":{"args":["/bin/true"],"cwd":"/"},"mounts":[{"destination":"/data","type":"bind","source":"data"}]}`, "must be absolute"},
		{"root bind destination", `{"ociVersion":"1.1.0","root":{"path":"rootfs"},"process":{"args":["/bin/true"],"cwd":"/"},"mounts":[{"destination":"/","type":"bind","source":"/srv/data"}]}`, "absolute path below root"},
		{"unsupported bind option", `{"ociVersion":"1.1.0","root":{"path":"rootfs"},"process":{"args":["/bin/true"],"cwd":"/"},"mounts":[{"destination":"/data","type":"bind","source":"/srv/data","options":["nosuid"]}]}`, "unsupported OCI bind mount option"},
		{"zero memory", `{"ociVersion":"1.1.0","root":{"path":"rootfs"},"process":{"args":["/bin/true"],"cwd":"/"},"linux":{"resources":{"memory":{"limit":0}}}}`, "memory.limit must be greater than zero"},
		{"zero pids", `{"ociVersion":"1.1.0","root":{"path":"rootfs"},"process":{"args":["/bin/true"],"cwd":"/"},"linux":{"resources":{"pids":{"limit":0}}}}`, "pids.limit must be greater than zero"},
		{"bad shares", `{"ociVersion":"1.1.0","root":{"path":"rootfs"},"process":{"args":["/bin/true"],"cwd":"/"},"linux":{"resources":{"cpu":{"shares":1}}}}`, "cpu.shares must be in range"},
		{"partial quota", `{"ociVersion":"1.1.0","root":{"path":"rootfs"},"process":{"args":["/bin/true"],"cwd":"/"},"linux":{"resources":{"cpu":{"quota":50000}}}}`, "quota and period must be specified together"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := loadOCIBundle(writeOCIBundle(t, tc.body))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error=%v want substring %q", err, tc.want)
			}
		})
	}
}