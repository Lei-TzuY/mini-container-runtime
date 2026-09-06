#!/usr/bin/env python3
from pathlib import Path
import sys

root = Path(sys.argv[1]) if len(sys.argv) > 1 else Path(".")
main = root / "cmd/minictl/main.go"
text = main.read_text()

replacements = [
    (
        "  minictl run     [flags] <rootfs-dir> <command> [args...]   Launch a new container",
        "  minictl run     [flags] <rootfs-dir> [command [args...]]  Launch a new container",
    ),
    (
        'fmt.Fprintln(os.Stderr, "Usage: minictl run [flags] <rootfs-dir> <command> [args...]")',
        'fmt.Fprintln(os.Stderr, "Usage: minictl run [flags] <rootfs-dir> [command [args...]]")',
    ),
    (
        'if len(rest) < 2 {\n\t\treturn container.Config{}, fmt.Errorf("missing rootfs or command")\n\t}',
        'if len(rest) < 1 {\n\t\treturn container.Config{}, fmt.Errorf("missing rootfs")\n\t}',
    ),
]
for old, new in replacements:
    count = text.count(old)
    if count != 1:
        raise SystemExit(f"expected exactly one main.go anchor, got {count}: {old!r}")
    text = text.replace(old, new, 1)
main.write_text(text)

main_test = root / "cmd/minictl/main_test.go"
test_text = main_test.read_text()
marker = "func TestParseRunConfigAllowsRootFSOnlyForImageDefaults"
if marker in test_text:
    raise SystemExit("rootfs-only parser regression already exists")
test_text += r'''

func TestParseRunConfigAllowsRootFSOnlyForImageDefaults(t *testing.T) {
	cfg, err := parseRunConfig([]string{"./rootfs"})
	if err != nil {
		t.Fatalf("parseRunConfig rootfs-only returned error: %v", err)
	}
	if cfg.RootFS != "./rootfs" {
		t.Fatalf("RootFS = %q, want ./rootfs", cfg.RootFS)
	}
	if len(cfg.Command) != 0 {
		t.Fatalf("Command = %#v, want empty so image defaults can resolve it", cfg.Command)
	}
}

func TestParseRunConfigStillRequiresRootFS(t *testing.T) {
	_, err := parseRunConfig(nil)
	if err == nil {
		t.Fatal("parseRunConfig(nil) succeeded, want missing-rootfs error")
	}
	if !strings.Contains(err.Error(), "missing rootfs") {
		t.Fatalf("parseRunConfig(nil) error = %q, want missing rootfs", err)
	}
}
'''
main_test.write_text(test_text)

image_test = root / "cmd/minictl/run_image_command_test.go"
image_text = image_test.read_text()
marker = "func TestRootFSOnlyRunConfigResolvesImageDefaults"
if marker in image_text:
    raise SystemExit("parser-to-image-default regression already exists")
image_text += r'''

func TestRootFSOnlyRunConfigResolvesImageDefaults(t *testing.T) {
	st, err := state.Open(t.TempDir())
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer st.Close()

	rootfs := t.TempDir()
	if err := st.SaveImage(&state.Image{Name: "example:latest", RootFS: rootfs, LoadedAt: time.Now()}); err != nil {
		t.Fatalf("SaveImage() error = %v", err)
	}
	if err := st.SaveImageCommand("example:latest", state.ImageCommand{
		Entrypoint: []string{"/bin/app"},
		Cmd:        []string{"serve", "--foreground"},
	}); err != nil {
		t.Fatalf("SaveImageCommand() error = %v", err)
	}

	cfg, err := parseRunConfig([]string{rootfs})
	if err != nil {
		t.Fatalf("parseRunConfig rootfs-only error = %v", err)
	}
	got, err := imageCommandForRootFS(st, cfg.RootFS, cfg.Command)
	if err != nil {
		t.Fatalf("imageCommandForRootFS() error = %v", err)
	}
	if want := []string{"/bin/app", "serve", "--foreground"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("resolved command = %#v, want %#v", got, want)
	}
}
'''
image_test.write_text(image_text)
