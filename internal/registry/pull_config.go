package registry

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"minicontainer/internal/container"
	"minicontainer/internal/state"
)

type imageConfigDocument struct {
	Config struct {
		StopSignal string `json:"StopSignal"`
	} `json:"config"`
}

func pullImageStopSignal(client *http.Client, imageName, token, tmpDir string, desc Descriptor) (string, error) {
	if client == nil {
		return "", fmt.Errorf("image config HTTP client is nil")
	}
	if err := validateLayerDescriptor(desc); err != nil {
		return "", fmt.Errorf("invalid config descriptor: %w", err)
	}

	configPath := filepath.Join(tmpDir, "config.json")
	if err := downloadBlob(client, imageName, desc.Digest, token, configPath, desc.Size); err != nil {
		return "", fmt.Errorf("download config blob: %w", err)
	}
	data, err := os.ReadFile(configPath)
	if err != nil {
		return "", fmt.Errorf("read verified config blob: %w", err)
	}
	return parseImageConfigStopSignal(data)
}

func parseImageConfigStopSignal(data []byte) (string, error) {
	var cfg imageConfigDocument
	if err := json.Unmarshal(data, &cfg); err != nil {
		return "", fmt.Errorf("decode image config: %w", err)
	}
	signal := strings.TrimSpace(cfg.Config.StopSignal)
	if signal == "" {
		signal = "SIGTERM"
	}
	if _, err := container.ParseSignal(signal); err != nil {
		return "", fmt.Errorf("invalid image StopSignal %q: %w", signal, err)
	}
	return signal, nil
}

func persistPulledImageMetadata(imageRef, destDir, stopSignal string) error {
	st, err := state.Open(state.DefaultDir())
	if err != nil {
		return fmt.Errorf("open image state store: %w", err)
	}
	defer st.Close()

	if err := st.SaveImage(&state.Image{
		Name:     imageRef,
		RootFS:   destDir,
		LoadedAt: time.Now(),
	}); err != nil {
		return fmt.Errorf("save pulled image: %w", err)
	}
	if err := st.SaveImageStopSignal(imageRef, stopSignal); err != nil {
		return fmt.Errorf("save pulled image StopSignal: %w", err)
	}
	return nil
}
