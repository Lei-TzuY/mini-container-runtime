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
//   3. Download and verify every layer blob.
//   4. Only after all blobs pass size/digest validation, extract layers sequentially
//      to destDir applying OCI whiteout rules.

package registry

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math"
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

// Descriptor is the subset of an OCI descriptor used by the pull path.
type Descriptor struct {
	MediaType string `json:"mediaType"`
	Size      int64  `json:"size"`
	Digest    string `json:"digest"`
}

// ManifestV2 represents a Docker Schema 2 / OCI manifest.
type ManifestV2 struct {
	SchemaVersion int          `json:"schemaVersion"`
	Config        Descriptor   `json:"config"`
	Layers        []Descriptor `json:"layers"`
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

	// Step 2: Fetch Image Manifest and preflight every descriptor before any
	// download or extraction can mutate local image state.
	manifest, err := getManifest(imageName, tag, token)
	if err != nil {
		return fmt.Errorf("fetch manifest: %w", err)
	}
	if err := validateManifestLayers(manifest); err != nil {
		return fmt.Errorf("validate manifest: %w", err)
	}

	fmt.Printf("Image manifest loaded: %d layer(s)\n", len(manifest.Layers))

	// Step 3: Download and verify every layer into an isolated temp directory.
	// Do not begin extraction until the entire layer set has passed integrity
	// validation; a later corrupt/missing blob must not leave a partially applied
	// rootfs behind.
	tmpDir, err := os.MkdirTemp("", "minicontainer-pull-*")
	if err != nil {
		return fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	client := &http.Client{Timeout: 60 * time.Second}
	layerFiles := make([]string, len(manifest.Layers))
	for i, layer := range manifest.Layers {
		short, err := shortDigest(layer.Digest)
		if err != nil {
			// validateManifestLayers already checked every descriptor. Keep this
			// defensive guard so future callers cannot reintroduce unsafe slicing.
			return fmt.Errorf("layer %d digest: %w", i+1, err)
		}
		fmt.Printf("  [%d/%d] downloading layer %s (%.2f MB) …\n",
			i+1, len(manifest.Layers), short, float64(layer.Size)/(1024*1024))

		layerFile := filepath.Join(tmpDir, fmt.Sprintf("layer-%d.tar.gz", i))
		if err := downloadBlob(client, imageName, layer.Digest, token, layerFile, layer.Size); err != nil {
			return fmt.Errorf("layer %d download failed: %w", i+1, err)
		}
		layerFiles[i] = layerFile
	}

	// Step 4: All blobs are locally verified; only now mutate destDir.
	for i, layerFile := range layerFiles {
		fmt.Printf("  [%d/%d] applying verified layer to %s …\n", i+1, len(layerFiles), destDir)
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

func parseSHA256Digest(value string) ([]byte, error) {
	algorithm, encoded, ok := strings.Cut(value, ":")
	if !ok || algorithm != "sha256" {
		return nil, fmt.Errorf("unsupported or malformed digest %q", value)
	}
	if len(encoded) != sha256.Size*2 {
		return nil, fmt.Errorf("sha256 digest %q has invalid length", value)
	}
	digest, err := hex.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("sha256 digest %q is not hexadecimal: %w", value, err)
	}
	if len(digest) != sha256.Size {
		return nil, fmt.Errorf("sha256 digest %q has invalid decoded length", value)
	}
	return digest, nil
}

func validateLayerDescriptor(layer Descriptor) error {
	if layer.Size < 0 || layer.Size == math.MaxInt64 {
		return fmt.Errorf("invalid layer size %d", layer.Size)
	}
	if _, err := parseSHA256Digest(layer.Digest); err != nil {
		return err
	}
	return nil
}

func validateManifestLayers(manifest *ManifestV2) error {
	if manifest == nil {
		return fmt.Errorf("manifest is nil")
	}
	if manifest.SchemaVersion != 2 {
		return fmt.Errorf("unsupported schema version %d", manifest.SchemaVersion)
	}
	for i, layer := range manifest.Layers {
		if err := validateLayerDescriptor(layer); err != nil {
			return fmt.Errorf("layer %d: %w", i+1, err)
		}
	}
	return nil
}

func shortDigest(value string) (string, error) {
	if _, err := parseSHA256Digest(value); err != nil {
		return "", err
	}
	_, encoded, _ := strings.Cut(value, ":")
	return encoded[:12], nil
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

func writeVerifiedBlob(src io.Reader, destPath, digest string, expectedSize int64) error {
	expectedDigest, err := parseSHA256Digest(digest)
	if err != nil {
		return err
	}
	if expectedSize < 0 || expectedSize == math.MaxInt64 {
		return fmt.Errorf("invalid expected blob size %d", expectedSize)
	}

	out, err := os.OpenFile(destPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	keep := false
	defer func() {
		_ = out.Close()
		if !keep {
			_ = os.Remove(destPath)
		}
	}()

	hasher := sha256.New()
	limited := io.LimitReader(src, expectedSize+1)
	written, err := io.Copy(io.MultiWriter(out, hasher), limited)
	if err != nil {
		return fmt.Errorf("write blob: %w", err)
	}
	if written != expectedSize {
		return fmt.Errorf("blob size mismatch: got %d bytes, want %d", written, expectedSize)
	}
	actualDigest := hasher.Sum(nil)
	if subtle.ConstantTimeCompare(actualDigest, expectedDigest) != 1 {
		return fmt.Errorf("blob digest mismatch: got sha256:%s, want %s", hex.EncodeToString(actualDigest), digest)
	}
	if err := out.Sync(); err != nil {
		return fmt.Errorf("sync verified blob: %w", err)
	}
	if err := out.Close(); err != nil {
		return fmt.Errorf("close verified blob: %w", err)
	}
	keep = true
	return nil
}

func downloadBlob(client *http.Client, imageName, digest, token, destPath string, expectedSize int64) error {
	if err := validateLayerDescriptor(Descriptor{Digest: digest, Size: expectedSize}); err != nil {
		return err
	}
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
	if resp.ContentLength >= 0 && resp.ContentLength != expectedSize {
		return fmt.Errorf("blob content length mismatch: got %d bytes, want %d", resp.ContentLength, expectedSize)
	}
	return writeVerifiedBlob(resp.Body, destPath, digest, expectedSize)
}
