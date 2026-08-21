package imagestore

import (
	"strings"
	"testing"
)

func TestDiffImageManifestLayers(t *testing.T) {
	baseJSON := `{
		"layers": [
			{"digest": "sha256:layer1", "size": 1000000},
			{"digest": "sha256:layer2", "size": 2000000},
			{"digest": "sha256:layer3", "size": 500000}
		]
	}`

	// Target shares layer1 and layer2, deletes layer3, adds layer4
	targetJSON := `{
		"layers": [
			{"digest": "sha256:layer1", "size": 1000000},
			{"digest": "sha256:layer2", "size": 2000000},
			{"digest": "sha256:layer4", "size": 1500000}
		]
	}`

	diff, err := DiffImageManifestLayers([]byte(baseJSON), []byte(targetJSON))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if diff.SharedLayersCount != 2 {
		t.Errorf("SharedLayersCount = %d, want 2", diff.SharedLayersCount)
	}
	if diff.AddedLayersCount != 1 {
		t.Errorf("AddedLayersCount = %d, want 1", diff.AddedLayersCount)
	}
	if diff.DeletedLayersCount != 1 {
		t.Errorf("DeletedLayersCount = %d, want 1", diff.DeletedLayersCount)
	}
	if diff.NetDeltaBytes != 1000000 { // 4.5MB - 3.5MB = 1MB
		t.Errorf("NetDeltaBytes = %d, want 1000000", diff.NetDeltaBytes)
	}
	if diff.SharedBytes != 3000000 {
		t.Errorf("SharedBytes = %d, want 3000000", diff.SharedBytes)
	}
}

func TestFormatManifestLayerDiff(t *testing.T) {
	base := `{"layers":[{"digest":"sha256:a","size":100}]}`
	target := `{"layers":[{"digest":"sha256:a","size":100},{"digest":"sha256:b","size":50}]}`

	got := FormatManifestLayerDiff([]byte(base), []byte(target))
	if !strings.Contains(got, "Manifest Layer Diff Summary:") {
		t.Errorf("expected summary header in %q", got)
	}
	if !strings.Contains(got, "Shared/Reused: 1 layers") {
		t.Errorf("expected shared layer info in %q", got)
	}
}
