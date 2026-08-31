package events

import (
	"fmt"
	"time"
)

const (
	EventExecExit   EventType = "exec_exit"
	EventExecFailed EventType = "exec_failed"
)

type stagedExecEvent struct {
	containerID string
	image       string
	message     string
}

var stagedExecs = make(map[string]stagedExecEvent)
var activeExecs = make(map[string]stagedExecEvent)

func stageExecEvent(containerID, image, message string) error {
	if containerID == "" {
		return fmt.Errorf("exec event container ID is empty")
	}
	if _, ok := stagedExecs[containerID]; ok {
		return fmt.Errorf("exec event for container %s is already staged", containerID)
	}
	if _, ok := activeExecs[containerID]; ok {
		return fmt.Errorf("exec event for container %s is already active", containerID)
	}
	stagedExecs[containerID] = stagedExecEvent{containerID: containerID, image: image, message: message}
	return nil
}

func CommitPendingExec() error {
	mu.Lock()
	defer mu.Unlock()
	if len(stagedExecs) == 0 {
		return nil
	}
	if len(stagedExecs) != 1 {
		return fmt.Errorf("cannot commit exec event: %d staged exec events", len(stagedExecs))
	}
	if len(activeExecs) != 0 {
		return fmt.Errorf("cannot commit exec event: %d active exec events", len(activeExecs))
	}

	var staged stagedExecEvent
	for _, candidate := range stagedExecs {
		staged = candidate
	}

	// The durable start event is the commit point. Do not publish in-memory
	// active attribution before append+fsync succeeds; otherwise a transient log
	// failure can manufacture an active exec whose start was never recorded.
	if err := appendEventUnlocked(Event{
		Timestamp:   time.Now(),
		Type:        EventExec,
		ContainerID: staged.containerID,
		Image:       staged.image,
		Message:     staged.message,
	}); err != nil {
		return err
	}
	delete(stagedExecs, staged.containerID)
	activeExecs[staged.containerID] = staged
	return nil
}

func CompletePendingExec(exitCode int, detail string) error {
	mu.Lock()
	defer mu.Unlock()
	if len(activeExecs) == 0 {
		return nil
	}
	if len(activeExecs) != 1 {
		return fmt.Errorf("cannot complete exec event: %d active exec events", len(activeExecs))
	}
	var active stagedExecEvent
	for _, candidate := range activeExecs {
		active = candidate
	}
	delete(activeExecs, active.containerID)
	message := fmt.Sprintf("%s; exit_code=%d", active.message, exitCode)
	if detail != "" {
		message += "; " + detail
	}
	code := exitCode
	return appendEventUnlocked(Event{Timestamp: time.Now(), Type: EventExecExit, ContainerID: active.containerID, Image: active.image, Message: message, ExitCode: &code, Error: detail})
}

func FailPendingExec(detail string) error {
	mu.Lock()
	defer mu.Unlock()
	if len(stagedExecs) == 0 {
		return nil
	}
	if len(stagedExecs) != 1 {
		return fmt.Errorf("cannot fail exec event: %d staged exec events", len(stagedExecs))
	}
	var staged stagedExecEvent
	for _, candidate := range stagedExecs {
		staged = candidate
	}
	delete(stagedExecs, staged.containerID)
	message := staged.message
	if detail != "" {
		message += "; " + detail
	}
	return appendEventUnlocked(Event{Timestamp: time.Now(), Type: EventExecFailed, ContainerID: staged.containerID, Image: staged.image, Message: message, Error: detail})
}

func DiscardPendingExec() error {
	mu.Lock()
	defer mu.Unlock()
	if len(stagedExecs) == 0 {
		return nil
	}
	if len(stagedExecs) != 1 {
		return fmt.Errorf("cannot discard exec event: %d staged exec events", len(stagedExecs))
	}
	for containerID := range stagedExecs {
		delete(stagedExecs, containerID)
	}
	return nil
}
