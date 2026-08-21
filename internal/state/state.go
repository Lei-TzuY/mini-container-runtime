// internal/state/state.go
//
// Container State Store
// ─────────────────────
// A real container runtime (Docker, containerd) stores state about running
// and stopped containers in a database (BoltDB in Docker, leveldb in
// containerd).  We use a simpler approach: one JSON file per container
// under a state directory, analogous to runc's state.json.
//
// State directory layout:
//
//   ~/.minicontainer/
//   ├── containers/
//   │   ├── <id>.json      ← Container record (see Container struct)
//   │   └── <id>.json
//   └── images/
//       └── <name>.json    ← Image metadata (see Image struct)

package state

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// DefaultDir returns ~/.minicontainer.
func DefaultDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		home = "/tmp"
	}
	return filepath.Join(home, ".minicontainer")
}

// Status represents the lifecycle state of a container.
type Status string

const (
	StatusCreated Status = "created"
	StatusRunning Status = "running"
	StatusStopped Status = "stopped"
)

// Container holds persisted metadata for one container.
type Container struct {
	// ID is a short random hex string used as a human-readable handle.
	ID string `json:"id"`

	// PID is the host PID of the container's init process.
	PID int `json:"pid"`

	// Status is the current lifecycle state.
	Status Status `json:"status"`

	// Health holds container health status ("healthy", "unhealthy", "starting", or "").
	Health string `json:"health,omitempty"`

	// RootFS is the rootfs directory path given at container creation.
	RootFS string `json:"rootfs"`

	// Command is the argv of the process running inside the container.
	Command []string `json:"command"`

	// Hostname is the UTS hostname set inside the container.
	Hostname string `json:"hostname"`

	// CreatedAt is the wall-clock time the container was created.
	CreatedAt time.Time `json:"created_at"`

	// StartedAt is when the init process was started.
	StartedAt *time.Time `json:"started_at,omitempty"`

	// FinishedAt is when the init process exited.
	FinishedAt *time.Time `json:"finished_at,omitempty"`

	// ExitCode is the exit status code of the container process (-1 if killed).
	ExitCode int `json:"exit_code"`

	// Env is environment variables set inside container.
	Env []string `json:"env,omitempty"`
}

// Image holds metadata for a registered rootfs image.
type Image struct {
	ID           string    `json:"id,omitempty"`
	Repository   string    `json:"repository,omitempty"`
	Tag          string    `json:"tag,omitempty"`
	Name         string    `json:"name"`
	RootFS       string    `json:"rootfs"`
	Size         int64     `json:"size,omitempty"`
	LoadedAt     time.Time `json:"loaded_at"`
	WorkDir      string    `json:"work_dir,omitempty"`
	Env          []string  `json:"env,omitempty"`
	Cmd          []string  `json:"cmd,omitempty"`
	ExposedPorts []string  `json:"exposed_ports,omitempty"`
}

// Store handles thread-safe persistence to disk under dir.
type Store struct {
	mu     sync.Mutex
	dir    string
	ctrDir string
	imgDir string
}

func Open(dir string) (*Store, error) {
	ctrDir := filepath.Join(dir, "containers")
	imgDir := filepath.Join(dir, "images")

	if err := os.MkdirAll(ctrDir, 0755); err != nil {
		return nil, fmt.Errorf("create container state dir: %w", err)
	}
	if err := os.MkdirAll(imgDir, 0755); err != nil {
		return nil, fmt.Errorf("create image state dir: %w", err)
	}

	return &Store{
		dir:    dir,
		ctrDir: ctrDir,
		imgDir: imgDir,
	}, nil
}

func (s *Store) Dir() string {
	return s.dir
}

func validateID(id string) error {
	if id == "" {
		return fmt.Errorf("id cannot be empty")
	}
	if id == "." || id == ".." || strings.ContainsAny(id, "/\\:\x00") {
		return fmt.Errorf("invalid id %q: path separators and relative components not allowed", id)
	}
	return nil
}

func atomicWriteFile(dir, target string, data []byte) error {
	tmpFile, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return fmt.Errorf("create state tmp file: %w", err)
	}
	tmpName := tmpFile.Name()
	closed := false
	defer func() {
		if !closed {
			_ = tmpFile.Close()
		}
		_ = os.Remove(tmpName)
	}()

	if _, err := tmpFile.Write(data); err != nil {
		return fmt.Errorf("write state tmp file: %w", err)
	}

	if err := tmpFile.Sync(); err != nil {
		return fmt.Errorf("sync state tmp file: %w", err)
	}

	closed = true
	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("close state tmp file: %w", err)
	}

	var renameErr error
	for attempts := 0; attempts < 10; attempts++ {
		renameErr = os.Rename(tmpName, target)
		if renameErr == nil {
			return nil
		}
		time.Sleep(time.Duration(attempts+1) * 2 * time.Millisecond)
	}

	return fmt.Errorf("atomic rename state file: %w", renameErr)
}

func (s *Store) Save(c *Container) error {
	if err := validateID(c.ID); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal container: %w", err)
	}

	target := filepath.Join(s.ctrDir, c.ID+".json")
	return atomicWriteFile(s.ctrDir, target, data)
}

func (s *Store) Get(id string) (*Container, error) {
	if err := validateID(id); err != nil {
		return nil, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	return s.getUnlocked(id)
}

func (s *Store) Load(id string) (*Container, error) {
	return s.Get(id)
}

func (s *Store) getUnlocked(id string) (*Container, error) {
	file := filepath.Join(s.ctrDir, id+".json")
	data, err := os.ReadFile(file)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("container %q not found", id)
		}
		return nil, fmt.Errorf("read container state: %w", err)
	}

	var c Container
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("unmarshal container state: %w", err)
	}
	return &c, nil
}

func (s *Store) Resolve(idOrPrefix string) (*Container, error) {
	if err := validateID(idOrPrefix); err != nil {
		return nil, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if c, err := s.getUnlocked(idOrPrefix); err == nil {
		return c, nil
	}

	entries, err := os.ReadDir(s.ctrDir)
	if err != nil {
		return nil, fmt.Errorf("read container state dir: %w", err)
	}

	var matches []string
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		name := entry.Name()
		id := name[:len(name)-len(".json")]
		if len(id) >= len(idOrPrefix) && id[:len(idOrPrefix)] == idOrPrefix {
			matches = append(matches, id)
		}
	}

	if len(matches) == 0 {
		return nil, fmt.Errorf("no container matched prefix %q", idOrPrefix)
	}
	if len(matches) > 1 {
		return nil, fmt.Errorf("ambiguous container prefix %q matched multiple IDs (%v)", idOrPrefix, matches)
	}

	return s.getUnlocked(matches[0])
}

func (s *Store) List() ([]*Container, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	entries, err := os.ReadDir(s.ctrDir)
	if err != nil {
		return nil, fmt.Errorf("read container state dir: %w", err)
	}

	var out []*Container
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		id := entry.Name()[:len(entry.Name())-len(".json")]
		if c, err := s.getUnlocked(id); err == nil {
			out = append(out, c)
		}
	}

	return out, nil
}

func (s *Store) Delete(id string) error {
	if err := validateID(id); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	file := filepath.Join(s.ctrDir, id+".json")
	if err := os.Remove(file); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove container state: %w", err)
	}
	return nil
}

func (s *Store) SaveImage(img *Image) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := json.MarshalIndent(img, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal image: %w", err)
	}

	key := img.Name
	if key == "" {
		key = img.ID
	}
	filename := sanitizeImageFilename(key)
	target := filepath.Join(s.imgDir, filename+".json")
	return atomicWriteFile(s.imgDir, target, data)
}

func (s *Store) GetImage(nameOrID string) (*Image, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	images, err := s.listImagesUnlocked()
	if err != nil {
		return nil, err
	}

	for _, img := range images {
		if img.Name == nameOrID || img.ID == nameOrID || (img.Repository+":"+img.Tag) == nameOrID {
			return img, nil
		}
		if len(img.ID) >= len(nameOrID) && img.ID[:len(nameOrID)] == nameOrID {
			return img, nil
		}
	}
	return nil, fmt.Errorf("image %q not found", nameOrID)
}

func (s *Store) DeleteImage(nameOrID string) (*Image, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	img, err := s.GetImageUnlocked(nameOrID)
	if err != nil {
		return nil, err
	}

	key := img.Name
	if key == "" {
		key = img.ID
	}
	file := filepath.Join(s.imgDir, sanitizeImageFilename(key)+".json")
	if err := os.Remove(file); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("remove image metadata: %w", err)
	}
	return img, nil
}

func (s *Store) GetImageUnlocked(nameOrID string) (*Image, error) {
	images, err := s.listImagesUnlocked()
	if err != nil {
		return nil, err
	}

	for _, img := range images {
		if img.Name == nameOrID || img.ID == nameOrID || (img.Repository+":"+img.Tag) == nameOrID {
			return img, nil
		}
		if len(img.ID) >= len(nameOrID) && img.ID[:len(nameOrID)] == nameOrID {
			return img, nil
		}
	}
	return nil, fmt.Errorf("image %q not found", nameOrID)
}

func (s *Store) ListImages() ([]*Image, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.listImagesUnlocked()
}

func (s *Store) listImagesUnlocked() ([]*Image, error) {
	entries, err := os.ReadDir(s.imgDir)
	if err != nil {
		return nil, fmt.Errorf("read image state dir: %w", err)
	}

	var out []*Image
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(s.imgDir, entry.Name()))
		if err == nil {
			var img Image
			if err := json.Unmarshal(data, &img); err == nil {
				out = append(out, &img)
			}
		}
	}
	return out, nil
}

func sanitizeImageFilename(name string) string {
	r := strings.NewReplacer("/", "_", ":", "_", "\\", "_", "..", "_")
	cleaned := strings.Trim(r.Replace(name), " ._")
	if cleaned == "" {
		return "default"
	}
	return cleaned
}
