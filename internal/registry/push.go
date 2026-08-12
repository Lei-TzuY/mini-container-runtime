package registry

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"minicontainer/internal/state"
)

type OCIManifest struct {
	SchemaVersion int         `json:"schemaVersion"`
	MediaType     string      `json:"mediaType"`
	Config        OCIDescriptor `json:"config"`
	Layers        []OCIDescriptor `json:"layers"`
}

type OCIDescriptor struct {
	MediaType string `json:"mediaType"`
	Digest    string `json:"digest"`
	Size      int64  `json:"size"`
}

// BuildOCILayer packages a rootfs directory into a tar.gz blob and returns SHA256 digest and byte size.
func BuildOCILayer(rootfsDir string, destArchive string) (string, int64, error) {
	out, err := os.Create(destArchive)
	if err != nil {
		return "", 0, err
	}
	defer out.Close()

	hasher := sha256.New()
	mw := io.MultiWriter(out, hasher)

	gw := gzip.NewWriter(mw)
	tw := tar.NewWriter(gw)

	err = filepath.Walk(rootfsDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(rootfsDir, path)
		if err != nil || rel == "." {
			return nil
		}

		header, err := tar.FileInfoHeader(info, info.Name())
		if err != nil {
			return err
		}
		header.Name = filepath.ToSlash(rel)

		if err := tw.WriteHeader(header); err != nil {
			return err
		}

		if info.Mode().IsRegular() {
			file, err := os.Open(path)
			if err != nil {
				return err
			}
			defer file.Close()
			_, err = io.Copy(tw, file)
			return err
		}
		return nil
	})

	_ = tw.Close()
	_ = gw.Close()

	if err != nil {
		return "", 0, fmt.Errorf("package layer tar.gz: %w", err)
	}

	fi, err := os.Stat(destArchive)
	if err != nil {
		return "", 0, err
	}

	digest := "sha256:" + hex.EncodeToString(hasher.Sum(nil))
	return digest, fi.Size(), nil
}

// BuildOCIManifest constructs OCI manifest JSON.
func BuildOCIManifest(layerDigest string, layerSize int64) (*OCIManifest, []byte, error) {
	configData := []byte(fmt.Sprintf(`{"created":%q,"architecture":"amd64","os":"linux"}`, time.Now().Format(time.RFC3339)))
	configHash := sha256.Sum256(configData)
	configDigest := "sha256:" + hex.EncodeToString(configHash[:])

	manifest := &OCIManifest{
		SchemaVersion: 2,
		MediaType:     "application/vnd.oci.image.manifest.v1+json",
		Config: OCIDescriptor{
			MediaType: "application/vnd.oci.image.config.v1+json",
			Digest:    configDigest,
			Size:      int64(len(configData)),
		},
		Layers: []OCIDescriptor{
			{
				MediaType: "application/vnd.oci.image.layer.v1.tar+gzip",
				Digest:    layerDigest,
				Size:      layerSize,
			},
		},
	}

	manifestBytes, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return nil, nil, err
	}

	return manifest, manifestBytes, nil
}

// PushImage packages image into OCI format ready for registry dispatch.
func PushImage(st *state.Store, imageTag string, outputArchive string) error {
	img, err := st.GetImage(imageTag)
	if err != nil {
		return fmt.Errorf("get image %q: %w", imageTag, err)
	}

	if img.RootFS == "" {
		return fmt.Errorf("image %q has empty rootfs", imageTag)
	}

	digest, sz, err := BuildOCILayer(img.RootFS, outputArchive)
	if err != nil {
		return fmt.Errorf("build layer: %w", err)
	}

	_, manifestBytes, err := BuildOCIManifest(digest, sz)
	if err != nil {
		return fmt.Errorf("build manifest: %w", err)
	}

	manifestPath := outputArchive + ".manifest.json"
	return os.WriteFile(manifestPath, manifestBytes, 0644)
}
