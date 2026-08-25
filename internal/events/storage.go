package events

import (
	"path/filepath"

	"minicontainer/internal/state"
)

func eventStateDir() string {
	return filepath.Clean(state.DefaultDir())
}

func isManagedEventLogPath(path string) bool {
	return filepath.Clean(path) == filepath.Join(eventStateDir(), "events.log")
}
