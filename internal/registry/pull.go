// internal/registry/pull.go
//
// OCI Image Pulling (`minictl pull <image>`)
// ─────────────────────────────────────────
// Downloads container image manifests and layer blobs directly from Docker Hub
// or OCI-compliant registries via HTTP REST API v2 without relying on Docker daemon.
//
// Protocol sequence:
//   1. Authenticate with auth.docker.io to get Bearer Token.
//   2. Fetch OCI Manifest (GET /v2/library/<image>/manifests/<tag>).
//   3. Download layer blobs (GET /v2/library/<image>/blobs/<digest>).
//   4. Extract layers sequentially to destDir applying OCI whiteout rules.

package registry

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"minicontainer/internal/image"
)

const (
	defaultRegistryHost = "registry-1.docker.io"
	defaultAuthHost     = "auth.docker.io"
	manifestV2Header    = "application/vnd.docker.distribution.manifest.v2+json"
)

// ManifestV2 represents a Docker Schema 2 / OCI manifest.
type ManifestV2 struct {
	SchemaVersion int `json:"schemaVersion"`
	Config        struct {
		MediaType string `json:"mediaType"`
		Size      int64  `json:"size"`
		Digest    string `json:"digest"`
	} `json:"config"`
	Layers []struct {
		MediaType string `json:"mediaType"`
		Size      int64  `json:"size"`
		Digest    string `json:"digest"`
	} `json:"layers"`
}

type authResponse struct {
	Token       string `json:"token"`
	AccessToken string `json:"access_token"`
}

// PullImage fetches an image from Docker Hub (e.g. "alpine" or "alpine:3.19") and extracts it to destDir.
func PullImage(imageRef, destDir string) error {
	imageName, tag := parseImageRef(imageRef)
	fmt.Printf("Pulling image %s:%s from Docker Hub …\n", imageName, tag)

	// Step 1: Obtain Auth Token
	token, err := getAuthToken(imageName)
	if err != nil {
		return fmt.Errorf("authentication failed: %w", err)
	}

	// Step 2: Fetch Image Manifest
	manifest, err := getManifest(imageName, tag, token)
	if err != nil {
		return fmt.Errorf("fetch manifest: %w", err)
	}

	fmt.Printf("Image manifest loaded: %d layer(s)\n", len(manifest.Layers))

	// Step 3: Download layer blobs to temp directory
	tmpDir, err := os.MkdirTemp("", "minicontainer-pull-*")
	if err != nil {
		return fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	client := &http.Client{Timeout: 60 * time.Second}

	for i, layer := range manifest.Layers {
		fmt.Printf("  [%d/%d] downloading layer %s (%.2f MB) …\n",
			i+1, len(manifest.Layers), layer.Digest[:12], float64(layer.Size)/(1024*1024))

		layerFile := filepath.Join(tmpDir, fmt.Sprintf("layer-%d.tar.gz", i))
		if err := downloadBlob(client, imageName, layer.Digest, token, layerFile); err != nil {
			return fmt.Errorf("layer %d download failed: %w", i+1, err)
		}

		fmt.Printf("  [%d/%d] applying layer to %s …\n", i+1, len(manifest.Layers), destDir)
		if err := image.Unpack(layerFile, destDir); err != nil {
			return fmt.Errorf("apply layer %d: %w", i+1, err)
		}
	}

	fmt.Printf("Successfully pulled %s:%s -> %s\n", imageName, tag, destDir)
	return nil
}

func parseImageRef(ref string) (string, string) {
	tag := "latest"
	name := ref
	if idx := strings.LastIndex(ref, ":"); idx != -1 && !strings.Contains(ref[idx:], "/") {
		name = ref[:idx]
		tag = ref[idx+1:]
	}
	if !strings.Contains(name, "/") {
		name = "library/" + name
	}
	return name, tag
}

func getAuthToken(imageName string) (string, error) {
	url := fmt.Sprintf("https://%s/token?service=registry.docker.io&scope=repository:%s:pull",
		defaultAuthHost, imageName)

	resp, err := http.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("auth service returned status %d", resp.StatusCode)
	}

	var auth authResponse
	if err := json.NewDecoder(resp.Body).Decode(&auth); err != nil {
		return "", err
	}

	if auth.Token != "" {
		return auth.Token, nil
	}
	return auth.AccessToken, nil
}

func getManifest(imageName, tag, token string) (*ManifestV2, error) {
	url := fmt.Sprintf("https://%s/v2/%s/manifests/%s", defaultRegistryHost, imageName, tag)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", manifestV2Header)
	req.Header.Set("Authorization", "Bearer "+token)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("registry returned status %d", resp.StatusCode)
	}

	var manifest ManifestV2
	if err := json.NewDecoder(resp.Body).Decode(&manifest); err != nil {
		return nil, err
	}
	return &manifest, nil
}

func downloadBlob(client *http.Client, imageName, digest, token, destPath string) error {
	url := fmt.Sprintf("https://%s/v2/%s/blobs/%s", defaultRegistryHost, imageName, digest)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("blob download status %d", resp.StatusCode)
	}

	out, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	return err
}
