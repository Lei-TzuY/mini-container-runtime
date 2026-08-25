//go:build linux

package daemon

import (
	"errors"
	"fmt"
	"net/http"
	"time"

	"golang.org/x/sys/unix"

	"minicontainer/internal/container"
	"minicontainer/internal/state"
)

const (
	defaultContainerStopTimeout = 5 * time.Second
	maxContainerStopTimeout     = 10 * time.Second
	parentStateSettleTimeout    = 500 * time.Millisecond
	postKillWaitTimeout         = 2 * time.Second
)

type managedProcessStatus int

const (
	managedProcessDead managedProcessStatus = iota
	managedProcessMatching
	managedProcessIdentityMismatch
	managedProcessMissingIdentity
)

func (s *Server) handleDeleteContainer(w http.ResponseWriter, id string) {
	c, err := s.store.Resolve(id)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		return
	}

	if c.Status == state.StatusRunning {
		fd, status, err := openVerifiedPidfd(c)
		if fd >= 0 {
			defer unix.Close(fd)
		}
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		switch status {
		case managedProcessMatching:
			writeJSON(w, http.StatusConflict, map[string]string{"error": "container is still running; stop it before deletion"})
			return
		case managedProcessIdentityMismatch:
			writeJSON(w, http.StatusConflict, map[string]string{"error": "stored PID now belongs to a different process; refusing deletion until state is reconciled"})
			return
		case managedProcessMissingIdentity:
			writeJSON(w, http.StatusConflict, map[string]string{"error": "running container lacks a verified process identity; refusing deletion"})
			return
		case managedProcessDead:
			// No process currently owns the stored PID. Deleting stale state cannot
			// orphan or signal a live process, so it is safe to proceed.
		}
	}

	if err := s.store.Delete(c.ID); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted", "id": c.ID})
}

func (s *Server) handleStopContainer(w http.ResponseWriter, r *http.Request, id string) {
	c, err := s.store.Resolve(id)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		return
	}
	if c.Status != state.StatusRunning {
		writeJSON(w, http.StatusOK, map[string]interface{}{"status": "stopped", "id": c.ID, "already_stopped": true})
		return
	}

	timeout, err := parseContainerStopTimeout(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	fd, status, err := openVerifiedPidfd(c)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if fd >= 0 {
		defer unix.Close(fd)
	}

	switch status {
	case managedProcessMissingIdentity:
		writeJSON(w, http.StatusConflict, map[string]string{"error": "running container lacks PID starttime identity; refusing to signal by PID alone"})
		return
	case managedProcessIdentityMismatch:
		writeJSON(w, http.StatusConflict, map[string]string{"error": "stored PID has been reused by another process; refusing to signal it"})
		return
	case managedProcessDead:
		if _, err := s.store.MarkStoppedIfIdentity(c.ID, c.PID, c.PIDStartTime, -1, time.Now()); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"status": "stopped", "id": c.ID, "already_exited": true})
		return
	case managedProcessMatching:
	}

	if err := unix.PidfdSendSignal(fd, unix.SIGTERM, nil, 0); err != nil && !errors.Is(err, unix.ESRCH) {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("send SIGTERM: %v", err)})
		return
	}

	exited, err := waitPidfdExit(fd, timeout)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	escalated := false
	if !exited {
		escalated = true
		if err := unix.PidfdSendSignal(fd, unix.SIGKILL, nil, 0); err != nil && !errors.Is(err, unix.ESRCH) {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("send SIGKILL: %v", err)})
			return
		}
		exited, err = waitPidfdExit(fd, postKillWaitTimeout)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		if !exited {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "container process did not exit after SIGKILL"})
			return
		}
	}

	// The CLI parent owns wait(2) and therefore knows the real exit status. Give
	// it a short window to publish the authoritative stopped state; only fall
	// back to an unknown exit code if that parent is gone or unresponsive.
	if err := waitForParentStoppedState(s.store, c.ID, parentStateSettleTimeout); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	current, err := s.store.Get(c.ID)
	if err == nil && current.Status == state.StatusRunning && current.PID == c.PID && current.PIDStartTime == c.PIDStartTime {
		if _, err := s.store.MarkStoppedIfIdentity(c.ID, c.PID, c.PIDStartTime, -1, time.Now()); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status":    "stopped",
		"id":        c.ID,
		"escalated": escalated,
	})
}

func parseContainerStopTimeout(r *http.Request) (time.Duration, error) {
	raw := r.URL.Query().Get("timeout")
	if raw == "" {
		return defaultContainerStopTimeout, nil
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		return 0, fmt.Errorf("invalid stop timeout: %w", err)
	}
	if d < 0 || d > maxContainerStopTimeout {
		return 0, fmt.Errorf("stop timeout must be between 0 and %s", maxContainerStopTimeout)
	}
	return d, nil
}

func openVerifiedPidfd(c *state.Container) (fd int, status managedProcessStatus, err error) {
	if c == nil || c.PID <= 0 || c.PIDStartTime == 0 {
		return -1, managedProcessMissingIdentity, nil
	}

	fd, err = unix.PidfdOpen(c.PID, 0)
	if err != nil {
		if errors.Is(err, unix.ESRCH) {
			return -1, managedProcessDead, nil
		}
		return -1, managedProcessDead, fmt.Errorf("open pidfd for PID %d: %w", c.PID, err)
	}

	start, err := container.ProcessStartTime(c.PID)
	if err != nil {
		unix.Close(fd)
		if errors.Is(err, unix.ENOENT) {
			return -1, managedProcessDead, nil
		}
		return -1, managedProcessDead, fmt.Errorf("verify process identity for PID %d: %w", c.PID, err)
	}
	if start != c.PIDStartTime {
		return fd, managedProcessIdentityMismatch, nil
	}
	return fd, managedProcessMatching, nil
}

func waitPidfdExit(fd int, timeout time.Duration) (bool, error) {
	if fd < 0 {
		return true, nil
	}
	deadline := time.Now().Add(timeout)
	for {
		remaining := time.Until(deadline)
		if timeout == 0 {
			remaining = 0
		}
		if remaining < 0 {
			return false, nil
		}
		ms := int((remaining + time.Millisecond - 1) / time.Millisecond)
		fds := []unix.PollFd{{Fd: int32(fd), Events: unix.POLLIN}}
		n, err := unix.Poll(fds, ms)
		if err != nil {
			if errors.Is(err, unix.EINTR) {
				continue
			}
			return false, fmt.Errorf("poll pidfd: %w", err)
		}
		if n == 0 {
			return false, nil
		}
		if fds[0].Revents&(unix.POLLIN|unix.POLLHUP|unix.POLLERR) != 0 {
			return true, nil
		}
	}
}

func waitForParentStoppedState(st *state.Store, id string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		c, err := st.Get(id)
		if err != nil {
			// A concurrent explicit deletion after process exit is a valid terminal
			// state; do not recreate it.
			return nil
		}
		if c.Status != state.StatusRunning {
			return nil
		}
		time.Sleep(25 * time.Millisecond)
	}
	return nil
}
