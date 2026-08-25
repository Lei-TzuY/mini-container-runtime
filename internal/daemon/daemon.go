package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"minicontainer/internal/imagestore"
	"minicontainer/internal/metrics"
	"minicontainer/internal/state"
	runtimestats "minicontainer/internal/stats"
)

// Server represents the minictl REST API Daemon.
type Server struct {
	addr       string
	network    string
	listener   net.Listener
	httpServer *http.Server
	store      *state.Store
	mu         sync.Mutex
}

// Config options for starting daemon server.
type Config struct {
	ListenAddr string // e.g. "unix:///tmp/minictl.sock" or "tcp://127.0.0.1:2375"
	StoreDir   string
}

// NewServer initializes daemon server.
func NewServer(cfg Config) (*Server, error) {
	stDir := cfg.StoreDir
	if stDir == "" {
		stDir = state.DefaultDir()
	}

	st, err := state.Open(stDir)
	if err != nil {
		return nil, fmt.Errorf("open state store: %w", err)
	}

	addr := cfg.ListenAddr
	if addr == "" {
		addr = "unix:///tmp/minictl.sock"
	}

	network := "tcp"
	listenPath := addr
	if strings.HasPrefix(addr, "unix://") {
		network = "unix"
		listenPath = strings.TrimPrefix(addr, "unix://")
		_ = os.Remove(listenPath)
	} else if strings.HasPrefix(addr, "tcp://") {
		network = "tcp"
		listenPath = strings.TrimPrefix(addr, "tcp://")
	}

	l, err := net.Listen(network, listenPath)
	if err != nil {
		return nil, fmt.Errorf("listen %s %s: %w", network, listenPath, err)
	}

	srv := &Server{
		addr:     listenPath,
		network:  network,
		listener: l,
		store:    st,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/v1/system/info", srv.handleSystemInfo)
	mux.HandleFunc("/v1/containers/json", srv.handleListContainers)
	mux.HandleFunc("/v1/containers/", srv.handleContainerRoute)
	mux.HandleFunc("/v1/images/json", srv.handleListImages)
	mux.HandleFunc("/v1/stats", srv.handleStats)
	mux.HandleFunc("/v1/metrics", srv.handleMetrics)

	srv.httpServer = &http.Server{
		Handler: mux,
	}

	return srv, nil
}

// Start runs the HTTP server loop.
func (s *Server) Start() error {
	return s.httpServer.Serve(s.listener)
}

// Stop gracefully shuts down daemon server.
func (s *Server) Stop(ctx context.Context) error {
	err := s.httpServer.Shutdown(ctx)
	if s.network == "unix" {
		_ = os.Remove(s.addr)
	}
	return err
}

func (s *Server) handleSystemInfo(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"version":    "minictl/1.2.0",
		"go_version": "go1.21+",
		"os":         "linux",
		"time":       time.Now().Format(time.RFC3339),
	})
}

func (s *Server) handleListContainers(w http.ResponseWriter, r *http.Request) {
	ctrs, err := s.store.List()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, ctrs)
}

func (s *Server) handleContainerRoute(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/v1/containers/")
	parts := strings.Split(path, "/")
	if len(parts) == 0 || parts[0] == "" {
		http.Error(w, "missing container id", http.StatusBadRequest)
		return
	}

	id := parts[0]

	if len(parts) == 1 && r.Method == http.MethodGet {
		// Inspect container
		c, err := s.store.Resolve(id)
		if err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, c)
		return
	}

	if len(parts) == 1 && r.Method == http.MethodDelete {
		if err := s.store.Delete(id); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "deleted", "id": id})
		return
	}

	if len(parts) == 2 && parts[1] == "stop" && r.Method == http.MethodPost {
		c, err := s.store.Resolve(id)
		if err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
			return
		}
		c.Status = state.StatusStopped
		now := time.Now()
		c.FinishedAt = &now
		_ = s.store.Save(c)
		writeJSON(w, http.StatusOK, map[string]string{"status": "stopped", "id": c.ID})
		return
	}

	http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
}

func (s *Server) handleListImages(w http.ResponseWriter, r *http.Request) {
	imgs, err := s.store.ListImages()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	// Calculate size if zero
	for _, img := range imgs {
		if img.Size == 0 && img.RootFS != "" {
			sz, _ := imagestore.CalculateDirSize(img.RootFS)
			img.Size = sz
		}
	}
	writeJSON(w, http.StatusOK, imgs)
}

func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	interval := 200 * time.Millisecond
	if raw := r.URL.Query().Get("interval"); raw != "" {
		parsed, err := time.ParseDuration(raw)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid interval: " + err.Error()})
			return
		}
		interval = parsed
	}
	if interval < 10*time.Millisecond || interval > 5*time.Second {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "interval must be between 10ms and 5s"})
		return
	}

	values, err := runtimestats.CollectStatsSampled(s.store, interval)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if values == nil {
		values = []runtimestats.ContainerStat{}
	}
	writeJSON(w, http.StatusOK, values)
}

func (s *Server) handleMetrics(w http.ResponseWriter, r *http.Request) {
	content, err := metrics.GeneratePrometheusMetrics(s.store)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/plain; version=0.0.4")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(content))
}

func writeJSON(w http.ResponseWriter, code int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}
