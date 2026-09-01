// internal/events/events.go
//
// Container Real-time Lifecycle Event Audit Stream (`minictl events`)
// ───────────────────────────────────────────────────────────────────
// Emits and logs container lifecycle events (create, start, exec, pause, unpause, stop, signal, die, rm).

package events

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
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
	EventSignal  EventType = "signal"
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
	// ContainerPID and ContainerPIDStartTime identify the exact init-process
	// generation to which an exec lifecycle event was admitted. PID alone is not
	// sufficient because Linux can reuse numeric process IDs after restart.
	ContainerPID          int      `json:"container_pid,omitempty"`
	ContainerPIDStartTime uint64   `json:"container_pid_start_time,omitempty"`
	Command               []string `json:"command,omitempty"`
	// ExitCode is structured terminal status for events that represent a process
	// outcome. A pointer distinguishes an actual exit code of zero from events
	// that have no exit-code semantics.
	ExitCode *int `json:"exit_code,omitempty"`
	// Error carries a machine-readable failure detail alongside the legacy human
	// Message. It is populated for failed exec admission/setup and abnormal waits.
	Error string `json:"error,omitempty"`
}

// StreamOptions controls event query and rendering without changing the durable
// append-only log schema. ContainerPrefix is intentionally a prefix selector so
// callers can use the same short IDs exposed by other minictl commands. Types is
// an OR filter; an empty slice selects every event type.
type StreamOptions struct {
	Follow          bool
	JSON            bool
	ContainerPrefix string
	Types           []EventType
}

var mu sync.Mutex

// LogPath returns the path to the events append-only log file.
func LogPath() string {
	return filepath.Join(state.DefaultDir(), "events.log")
}

// Publish records a new event to the global log. Runtime-admission events are
// staged when the CLI announces intent before the operation can prove success:
// start is committed only after payload exec and exec is committed only after
// the exec payload process itself starts. Create is persisted only after the
// exact durable container record exists. A die event is persisted only when
// this process has a committed start proof for the same container.
func Publish(evtType EventType, containerID, image, message string) error {
	mu.Lock()
	defer mu.Unlock()

	if evtType == EventCreate {
		if err := validatePersistedCreate(containerID); err != nil {
			return err
		}
	}
	if evtType == EventStart || evtType == EventExec {
		if err := validateEventStagingStorage(); err != nil {
			return err
		}
		if evtType == EventStart {
			return stageStartEvent(containerID, image, message)
		}
		return stageExecEvent(containerID, image, message)
	}
	if evtType == EventDie {
		if !consumeDieProof(containerID) {
			return nil
		}
		// Event persistence is deliberately best-effort for an already-finished
		// generation. Never wedge the in-process proof if appending the audit log
		// fails; otherwise a restart attempt could not establish its own Start.
		defer finishDieProof(containerID)
	}

	evt := Event{
		Timestamp:   time.Now(),
		Type:        evtType,
		ContainerID: containerID,
		Image:       image,
		Message:     message,
	}
	if err := appendEventUnlocked(evt); err != nil {
		return err
	}
	return nil
}

func appendEventUnlocked(evt Event) error {
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

// FormatEvent renders one lifecycle event for the human-facing CLI. Structured
// fields are emitted as explicit key=value attributes so recently added exec
// generation/outcome metadata is observable without parsing the append-only JSON
// log directly. Command argv is JSON encoded to preserve argument boundaries.
func FormatEvent(evt Event) string {
	shortID := evt.ContainerID
	if len(shortID) > 12 {
		shortID = shortID[:12]
	}

	var b strings.Builder
	fmt.Fprintf(&b, "%s container %s %s", evt.Timestamp.Format(time.RFC3339), evt.Type, shortID)
	if evt.ContainerPID > 0 {
		fmt.Fprintf(&b, " pid=%d", evt.ContainerPID)
	}
	if evt.ContainerPIDStartTime != 0 {
		fmt.Fprintf(&b, " pid_start=%d", evt.ContainerPIDStartTime)
	}
	if len(evt.Command) > 0 {
		if command, err := json.Marshal(evt.Command); err == nil {
			fmt.Fprintf(&b, " command=%s", command)
		}
	}
	if evt.ExitCode != nil {
		fmt.Fprintf(&b, " exit_code=%d", *evt.ExitCode)
	}
	if evt.Error != "" {
		fmt.Fprintf(&b, " error=%s", strconv.Quote(evt.Error))
	}
	if evt.Message != "" {
		fmt.Fprintf(&b, " (%s)", evt.Message)
	}
	return b.String()
}

func eventMatchesQuery(evt Event, opts StreamOptions) bool {
	if opts.ContainerPrefix != "" && !strings.HasPrefix(evt.ContainerID, opts.ContainerPrefix) {
		return false
	}
	if len(opts.Types) == 0 {
		return true
	}
	for _, eventType := range opts.Types {
		if evt.Type == eventType {
			return true
		}
	}
	return false
}

func writeQueriedEvent(w io.Writer, evt Event, jsonOutput bool) error {
	if jsonOutput {
		data, err := json.Marshal(evt)
		if err != nil {
			return fmt.Errorf("encode event stream: %w", err)
		}
		if _, err := fmt.Fprintln(w, string(data)); err != nil {
			return fmt.Errorf("write event stream: %w", err)
		}
		return nil
	}
	if _, err := fmt.Fprintln(w, FormatEvent(evt)); err != nil {
		return fmt.Errorf("write event stream: %w", err)
	}
	return nil
}

// streamEventLogWithOptions consumes the append-only log one record boundary at
// a time and applies query filters only after a complete record has been decoded.
// This preserves corruption detection: an unmatched malformed complete record is
// still corruption and cannot be hidden by a filter.
func streamEventLogWithOptions(r io.Reader, opts StreamOptions, w io.Writer) error {
	reader := bufio.NewReader(r)
	for {
		line, err := reader.ReadBytes('\n')
		if len(line) > 0 {
			if opts.Follow && err == io.EOF {
				// A follower cannot treat an unterminated record as committed yet: the
				// writer may still append the newline (or the rest of a torn JSON value).
				// Re-prepend exactly what was consumed and retry after the file grows.
				reader = bufio.NewReader(io.MultiReader(bytes.NewReader(line), reader))
			} else {
				var evt Event
				if decodeErr := json.Unmarshal(line, &evt); decodeErr != nil {
					if err != io.EOF {
						return fmt.Errorf("decode event log: %w", decodeErr)
					}
					// A non-following reader ignores only an invalid unterminated tail.
				} else if eventMatchesQuery(evt, opts) {
					if writeErr := writeQueriedEvent(w, evt, opts.JSON); writeErr != nil {
						return writeErr
					}
				}
			}
		}

		if err == nil {
			continue
		}
		if err != io.EOF {
			return fmt.Errorf("read event log: %w", err)
		}
		if !opts.Follow {
			return nil
		}
		time.Sleep(200 * time.Millisecond)
	}
}

// streamEventLog preserves the historical internal API for focused recovery
// tests and callers that only need the human renderer.
func streamEventLog(r io.Reader, follow bool, w io.Writer) error {
	return streamEventLogWithOptions(r, StreamOptions{Follow: follow}, w)
}

// StreamEventsWithOptions reads and outputs historical and real-time events
// using structured query/render options.
func StreamEventsWithOptions(opts StreamOptions, w io.Writer) error {
	logFile := LogPath()
	f, err := openEventLogForRead(logFile)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	defer f.Close()

	return streamEventLogWithOptions(f, opts, w)
}

// StreamEvents reads and outputs historical and real-time events to w.
func StreamEvents(follow bool, w io.Writer) error {
	return StreamEventsWithOptions(StreamOptions{Follow: follow}, w)
}
