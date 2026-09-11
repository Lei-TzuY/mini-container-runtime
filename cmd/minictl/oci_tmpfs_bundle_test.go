package main

import (
	"reflect"
	"runtime"
	"strings"
	"testing"
)

func TestLoadOCIBundleTranslatesTmpfsMount(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("OCI tmpfs mounts require linux")
	}
	d := writeOCIBundle(t, `{"ociVersion":"1.1.0","root":{"path":"rootfs"},"process":{"args":["/bin/true"],"cwd":"/"},"mounts":[{"destination":"/run/cache","type":"tmpfs","source":"tmpfs","options":["nosuid","nodev","mode=0755","size=64k"]}]}`)
	cfg, err := loadOCIBundle(d)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.TmpfsMounts) != 1 {
		t.Fatalf("tmpfs mounts = %#v, want one mount", cfg.TmpfsMounts)
	}
	got := cfg.TmpfsMounts[0]
	if got.ContainerPath != "/run/cache" {
		t.Fatalf("tmpfs destination = %q, want /run/cache", got.ContainerPath)
	}
	wantOptions := []string{"nosuid", "nodev", "mode=0755", "size=64k"}
	if !reflect.DeepEqual(got.Options, wantOptions) {
		t.Fatalf("tmpfs options = %#v, want %#v", got.Options, wantOptions)
	}
	if len(cfg.Volumes) != 0 {
		t.Fatalf("tmpfs mount was incorrectly translated as bind volume: %#v", cfg.Volumes)
	}
}

func TestLoadOCIBundleRejectsUnsafeTmpfsMounts(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("OCI tmpfs mounts require linux")
	}
	cases := []struct {
		name string
		body string
		want string
	}{
		{
			name: "unexpected source",
			body: `{"ociVersion":"1.1.0","root":{"path":"rootfs"},"process":{"args":["/bin/true"],"cwd":"/"},"mounts":[{"destination":"/run/cache","type":"tmpfs","source":"host-data"}]}`,
			want: "must be empty or tmpfs",
		},
		{
			name: "root destination",
			body: `{"ociVersion":"1.1.0","root":{"path":"rootfs"},"process":{"args":["/bin/true"],"cwd":"/"},"mounts":[{"destination":"/","type":"tmpfs","source":"tmpfs"}]}`,
			want: "absolute path below root",
		},
		{
			name: "non canonical destination",
			body: `{"ociVersion":"1.1.0","root":{"path":"rootfs"},"process":{"args":["/bin/true"],"cwd":"/"},"mounts":[{"destination":"/run/../cache","type":"tmpfs","source":"tmpfs"}]}`,
			want: "must be canonical",
		},
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
