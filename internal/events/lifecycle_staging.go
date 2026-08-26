package events

import (
	"fmt"
	"time"
)

type stagedStartEvent struct {
	containerID string
	image       string
	message     string
}

// stagedStarts and committedStarts are process-local lifecycle proofs guarded by
// mu in events.go. minictl currently runs one container lifecycle per process;
// committing is deliberately rejected when more than one pending start exists
// so one runtime readiness signal can never be attributed to another container.
var (
	stagedStarts    = make(map[string]stagedStartEvent)
	committedStarts = make(map[string]struct{})
)

func stageStartEvent(containerID, image, message string) error {
	if containerID == "" {
		return fmt.Errorf("start event container ID is empty")
	}
	if _, ok := committedStarts[containerID]; ok {
		return fmt.Errorf("start event for container %s is already committed", containerID)
	}
	if _, ok := stagedStarts[containerID]; ok {
		return fmt.Errorf("start event for container %s is already staged", containerID)
	}
	stagedStarts[containerID] = stagedStartEvent{
		containerID: containerID,
		image:       image,
		message:     message,
	}
	return nil
}

// CommitPendingStart publishes the single staged start event after the runtime
// parent has successfully delivered its explicit readiness byte. No pending
// event is a no-op. Multiple pending events are ambiguous and fail closed.
func CommitPendingStart() error {
	mu.Lock()
	defer mu.Unlock()

	if len(stagedStarts) == 0 {
		return nil
	}
	if len(stagedStarts) != 1 {
		return fmt.Errorf("cannot commit runtime start: %d staged start events", len(stagedStarts))
	}

	var staged stagedStartEvent
	for _, candidate := range stagedStarts {
		staged = candidate
	}
	if err := appendEventUnlocked(Event{
		Timestamp:   time.Now(),
		Type:        EventStart,
		ContainerID: staged.containerID,
		Image:       staged.image,
		Message:     staged.message,
	}); err != nil {
		return err
	}
	delete(stagedStarts, staged.containerID)
	committedStarts[staged.containerID] = struct{}{}
	return nil
}

func consumeDieProof(containerID string) bool {
	if _, ok := committedStarts[containerID]; !ok {
		delete(stagedStarts, containerID)
		return false
	}
	return true
}

func finishDieProof(containerID string) {
	delete(committedStarts, containerID)
}
