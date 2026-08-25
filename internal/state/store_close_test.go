package state

import (
	"os"
	"strconv"
	"testing"
	"time"
)

func TestStoreCloseIsIdempotentAndDisablesFurtherMutation(t *testing.T) {
	st, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if st.lockFile == nil {
		t.Fatal("Open did not retain state lock descriptor")
	}

	if err := st.Close(); err != nil {
		t.Fatalf("first Close: %v", err)
	}
	if st.lockFile != nil {
		t.Fatal("Close retained state lock descriptor")
	}
	if len(st.storagePins) != 0 {
		t.Fatalf("Close retained %d storage pin(s)", len(st.storagePins))
	}
	if err := st.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}

	err = st.Save(&Container{ID: "after-close", Status: StatusStopped, CreatedAt: time.Now()})
	if err == nil {
		t.Fatal("Save succeeded after Store.Close")
	}
}

func TestNilStoreCloseIsNoop(t *testing.T) {
	var st *Store
	if err := st.Close(); err != nil {
		t.Fatalf("nil Store.Close: %v", err)
	}
}

func TestStoreCloseReleasesPinnedDirectoryDescriptors(t *testing.T) {
	if _, err := os.Stat("/proc/self/fd"); err != nil {
		t.Skip("procfs fd view unavailable")
	}
	st, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if len(st.storagePins) == 0 {
		_ = st.Close()
		t.Skip("state storage pinning unavailable")
	}
	pinnedRoot := "/proc/self/fd/" + strconv.FormatUint(uint64(st.storagePins[0].Fd()), 10)
	if _, err := os.Stat(pinnedRoot); err != nil {
		t.Fatalf("pinned root descriptor unavailable before Close: %v", err)
	}
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(pinnedRoot); err == nil {
		t.Fatalf("pinned root descriptor %q remained resolvable after Close", pinnedRoot)
	}
}
