package events

import (
	"fmt"
	"time"
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

func stageExecEvent(containerID, image, message string) error {
	if containerID == "" {
		return fmt.Errorf("exec event container ID is empty")
	}
	if _, ok := stagedExecs[containerID]; ok {
		return fmt.Errorf("exec event for container %s is already staged", containerID)
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
// Zero pending events is a no-op; multiple pending events fail closed.
func CommitPendingExec() error {
	mu.Lock()
	defer mu.Unlock()

	if len(stagedExecs) == 0 {
		return nil
	}
	if len(stagedExecs) != 1 {
		return fmt.Errorf("cannot commit exec event: %d staged exec events", len(stagedExecs))
	}

	var staged stagedExecEvent
	for _, candidate := range stagedExecs {
		staged = candidate
	}
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
	return nil
}

// DiscardPendingExec removes the only staged exec after a pre-payload failure.
// Ambiguous pending state is retained and reported rather than deleting an
// unrelated container's audit intent.
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
