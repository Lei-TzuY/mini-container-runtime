package events

import (
	"fmt"
	"time"
)

const (
	// EventExecExit records completion of a payload that crossed the exec-start
	// admission boundary. Non-zero command exits are outcomes, not setup failures.
	EventExecExit EventType = "exec_exit"
	// EventExecFailed records an exec attempt that failed before payload start
	// could be proven.
	EventExecFailed EventType = "exec_failed"
)

type stagedExecEvent struct {
	containerID string
	image       string
	message     string
}

// stagedExecs is process-local and guarded by mu in events.go. The current CLI
// performs one exec lifecycle per process. Multiple pending exec events are
// therefore ambiguous and are never attributed to a single payload-start proof.
var stagedExecs = make(map[string]stagedExecEvent)

// activeExecs contains execs whose payload start was proven. Keeping the audit
// identity through wait lets the parent emit a terminal outcome for every
// admitted exec rather than leaving a start-only black box in the event stream.
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
	stagedExecs[containerID] = stagedExecEvent{
		containerID: containerID,
		image:       image,
		message:     message,
	}
	return nil
}

// CommitPendingExec publishes the single staged exec event only after the
// exec-init helper proves that the payload process itself started successfully.
// The identity remains active until CompletePendingExec records its terminal
// outcome. Zero pending events is a no-op; multiple pending events fail closed.
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
	delete(stagedExecs, staged.containerID)
	activeExecs[staged.containerID] = staged

	return appendEventUnlocked(Event{
		Timestamp:   time.Now(),
		Type:        EventExec,
		ContainerID: staged.containerID,
		Image:       staged.image,
		Message:     staged.message,
	})
}

// CompletePendingExec records the terminal status of the single admitted exec.
// The active identity is consumed even if event persistence fails so a later
// operation can never inherit stale attribution from an already-finished exec.
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
	return appendEventUnlocked(Event{
		Timestamp:   time.Now(),
		Type:        EventExecExit,
		ContainerID: active.containerID,
		Image:       active.image,
		Message:     message,
	})
}

// FailPendingExec records a setup/admission failure before payload start was
// proven. It intentionally does not fabricate a normal EventExec start event.
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
	return appendEventUnlocked(Event{
		Timestamp:   time.Now(),
		Type:        EventExecFailed,
		ContainerID: staged.containerID,
		Image:       staged.image,
		Message:     message,
	})
}

// DiscardPendingExec removes the only staged exec after a pre-payload failure
// when the caller explicitly does not want a failure audit record. Ambiguous
// pending state is retained and reported rather than deleting unrelated intent.
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
