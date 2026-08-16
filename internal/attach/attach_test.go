package attach

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"minicontainer/internal/logs"
	"minicontainer/internal/state"
)

func TestAttachContainer(t *testing.T) {
	tmpDir := t.TempDir()
	st, err := state.Open(tmpDir)
	if err != nil {
		t.Fatalf("Open state store error: %v", err)
	}

	c := &state.Container{
		ID:        "ctr-attach-1",
		Status:    state.StatusRunning,
		RootFS:    tmpDir,
		CreatedAt: time.Now(),
	}
	if err := st.Save(c); err != nil {
		t.Fatalf("Save container error: %v", err)
	}

	logPath := logs.LogFilePath(c.ID)
	_ = os.MkdirAll(filepath.Dir(logPath), 0755)
	_ = os.WriteFile(logPath, []byte("container log output line 1\n"), 0644)

	var inBuf bytes.Buffer
	var outBuf bytes.Buffer

	if err := AttachContainer(st, c.ID, &inBuf, &outBuf); err != nil {
		t.Fatalf("AttachContainer error: %v", err)
	}

	if !strings.Contains(outBuf.String(), "container log output line 1") {
		t.Fatalf("Attached output missing expected log contents:\n%s", outBuf.String())
	}
}
