//go:build linux

package daemon

import (
	"errors"
	"fmt"
	"net/http"
	"syscall"
	"time"

	"minicontainer/internal/container"
	"minicontainer/internal/state"
)

const (
	defaultContainerStopTimeout = 5 * time.Second
	maxContainerStopTimeout     = 7 * time.Second
	parentStateSettleTimeout    = 500 * time.Millisecond
	postKillWaitTimeout         = 2 * time.Second
)

func (s *Server) handleDeleteContainer(w http.ResponseWriter, id string) {
	c, err := s.store.Resolve(id)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		return
	}

	if c.Status == state.StatusRunning {
		handle, err := container.OpenProcessHandle(c.PID, c.PIDStartTime)
		if err == nil {
			defer handle.Close()
			writeJSON(w, http.StatusConflict, map[string]string{"error": "container is still running; stop it before deletion"})
			return
		}
		switch {
		case errors.Is(err, container.ErrProcessNotFound):
			// The persisted generation is gone. Reconcile it before deleting the
			// state record. Finalization consults durable cgroup ownership and never
			// deletes a merely derived legacy/unowned name.
			if _, finalizeErr := container.FinalizeStoppedGeneration(s.store, c, -1, time.Now()); finalizeErr != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": finalizeErr.Error()})
				return
			}
		case errors.Is(err, container.ErrProcessIdentityMismatch):
			writeJSON(w, http.StatusConflict, map[string]string{"error": "stored PID now belongs to a different process; refusing deletion until state is reconciled"})
			return
		case errors.Is(err, container.ErrProcessIdentityUnavailable):
			writeJSON(w, http.StatusConflict, map[string]string{"error": "running container lacks a verified process identity; refusing deletion"})
			return
		default:
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
	}

	// Reload after reconciliation so a concurrent restart cannot be deleted by a
	// stale pre-stop snapshot. A stopped record may retain a durable cgroup token
	// after a previous cleanup failure; retry it before discarding state.
	current, err := s.store.Get(c.ID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if current.Status == state.StatusRunning {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "container became running while deletion was being reconciled"})
		return
	}
	if current.Status == state.StatusStopped {
		if err := container.CleanupStoppedCgroup(s.store, current); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
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
		if c.Status == state.StatusStopped {
			if err := container.CleanupStoppedCgroup(s.store, c); err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
				return
			}
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"status": "stopped", "id": c.ID, "already_stopped": true})
		return
	}

	timeout, err := parseContainerStopTimeout(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	handle, err := container.OpenProcessHandle(c.PID, c.PIDStartTime)
	if err != nil {
		switch {
		case errors.Is(err, container.ErrProcessIdentityUnavailable):
			writeJSON(w, http.StatusConflict, map[string]string{"error": "running container lacks PID starttime identity; refusing to signal by PID alone"})
			return
		case errors.Is(err, container.ErrProcessIdentityMismatch):
			writeJSON(w, http.StatusConflict, map[string]string{"error": "stored PID has been reused by another process; refusing to signal it"})
			return
		case errors.Is(err, container.ErrProcessNotFound):
			if _, finalizeErr := container.FinalizeStoppedGeneration(s.store, c, -1, time.Now()); finalizeErr != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": finalizeErr.Error()})
				return
			}
			writeJSON(w, http.StatusOK, map[string]interface{}{"status": "stopped", "id": c.ID, "already_exited": true})
			return
		default:
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
	}
	defer handle.Close()

	if err := handle.Signal(syscall.SIGTERM); err != nil && !errors.Is(err, container.ErrProcessNotFound) {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("send SIGTERM: %v", err)})
		return
	}

	exited, err := handle.WaitExit(timeout)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	escalated := false
	if !exited {
		escalated = true
		if err := handle.Signal(syscall.SIGKILL); err != nil && !errors.Is(err, container.ErrProcessNotFound) {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("send SIGKILL: %v", err)})
			return
		}
		exited, err = handle.WaitExit(postKillWaitTimeout)
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
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	exitCode := -1
	if current.Status == state.StatusStopped {
		exitCode = current.ExitCode
	}
	// Finalize the exact generation captured before signaling. If the parent
	// already stopped it or a restart has installed a new PID/start-time pair,
	// the CAS is a no-op. Cgroup cleanup additionally requires the durable
	// ownership token for that exact generation.
	if _, err := container.FinalizeStoppedGeneration(s.store, c, exitCode, time.Now()); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
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

func waitForParentStoppedState(st *state.Store, id string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		c, err := st.Get(id)
		if err != nil {
			return fmt.Errorf("read container state while settling stop: %w", err)
		}
		if c.Status != state.StatusRunning {
			return nil
		}
		time.Sleep(25 * time.Millisecond)
	}
	return nil
}
