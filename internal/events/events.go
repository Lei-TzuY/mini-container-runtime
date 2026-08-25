// internal/events/events.go
//
// Container Real-time Lifecycle Event Audit Stream (`minictl events`)
// ───────────────────────────────────────────────────────────────────
// Emits and logs container lifecycle events (create, start, exec, pause, unpause, stop, die, rm).

package events

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"minicontainer/internal/state"
)

// EventType represents the category of container action.
type EventType string

const (
	EventCreate  EventType = "create"
	EventStart   EventType = "start"
	EventExec    EventType = "exec"
	EventPause   EventType = "pause"
	EventUnpause EventType = "unpause"
	EventStop    EventType = "stop"
	EventDie     EventType = "die"
	EventRemove  EventType = "destroy"
)

// Event describes a single container lifecycle event.
type Event struct {
	Timestamp   time.Time `json:"timestamp"`
	Type        EventType `json:"type"`
	ContainerID string    `json:"container_id"`
	Image       string    `json:"image,omitempty"`
	Message     string    `json:"message,omitempty"`
}

var mu sync.Mutex

// LogPath returns the path to the events append-only log file.
func LogPath() string {
	return filepath.Join(state.DefaultDir(), "events.log")
}

// Publish records a new event to the global log. The file helper opens the
// audit log without following symlinks and with private permissions so a
// compromised state path cannot redirect privileged append operations.
func Publish(evtType EventType, containerID, image, message string) error {
	mu.Lock()
	defer mu.Unlock()

	evt := Event{
		Timestamp:   time.Now(),
		Type:        evtType,
		ContainerID: containerID,
		Image:       image,
		Message:     message,
	}

	data, err := json.Marshal(evt)
	if err != nil {
		return err
	}

	f, err := openEventLogForAppend(LogPath())
	if err != nil {
		return err
	}
	defer f.Close()

	if _, err := fmt.Fprintln(f, string(data)); err != nil {
		return err
	}
	if err := f.Sync(); err != nil {
		return fmt.Errorf("sync event log: %w", err)
	}
	return nil
}

// StreamEvents reads and outputs historical and real-time events to w.
func StreamEvents(follow bool, w io.Writer) error {
	logFile := LogPath()
	f, err := openEventLogForRead(logFile)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	defer f.Close()

	dec := json.NewDecoder(f)
	for {
		var evt Event
		if err := dec.Decode(&evt); err != nil {
			if err == io.EOF {
				if !follow {
					break
				}
				time.Sleep(200 * time.Millisecond)
				continue
			}
			return fmt.Errorf("decode event log: %w", err)
		}
		shortID := evt.ContainerID
		if len(shortID) > 12 {
			shortID = shortID[:12]
		}
		fmt.Fprintf(w, "%s container %s %s (%s)\n",
			evt.Timestamp.Format(time.RFC3339), evt.Type, shortID, evt.Message)
	}
	return nil
}
