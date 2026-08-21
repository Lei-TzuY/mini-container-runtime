package imagestore

import (
	"strings"
	"testing"
)

func TestValidateUserNamespaceMapping(t *testing.T) {
	subRange := SubIDRange{StartID: 100000, Length: 65536}

	tests := []struct {
		name         string
		json         string
		wantUID      int
		wantGID      int
		wantRoot     bool
		wantRootless bool
	}{
		{
			name:         "numeric non-root user 1000:1000",
			json:         `{"config":{"User":"1000:1000"}}`,
			wantUID:      1000,
			wantGID:      1000,
			wantRoot:     false,
			wantRootless: true,
		},
		{
			name:         "root user default",
			json:         `{"config":{"User":"root"}}`,
			wantUID:      0,
			wantGID:      0,
			wantRoot:     true,
			wantRootless: true,
		},
		{
			name:         "symbolic user nobody",
			json:         `{"config":{"User":"nobody"}}`,
			wantUID:      65534,
			wantGID:      65534,
			wantRoot:     false,
			wantRootless: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			res, err := ValidateUserNamespaceMapping([]byte(tc.json), subRange)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if res.UID != tc.wantUID {
				t.Errorf("UID = %d, want %d", res.UID, tc.wantUID)
			}
			if res.GID != tc.wantGID {
				t.Errorf("GID = %d, want %d", res.GID, tc.wantGID)
			}
			if res.IsRoot != tc.wantRoot {
				t.Errorf("IsRoot = %t, want %t", res.IsRoot, tc.wantRoot)
			}
			if res.IsRootlessAllowed != tc.wantRootless {
				t.Errorf("IsRootlessAllowed = %t, want %t", res.IsRootlessAllowed, tc.wantRootless)
			}
		})
	}
}

func TestFormatUserNamespaceMapping(t *testing.T) {
	subRange := SubIDRange{StartID: 100000, Length: 65536}
	got := FormatUserNamespaceMapping([]byte(`{"config":{"User":"1000:1000"}}`), subRange)
	if !strings.Contains(got, "User Namespace Mapping Evaluation:") {
		t.Errorf("expected evaluation header in %q", got)
	}
	if !strings.Contains(got, "Parsed UID:GID: 1000:1000") {
		t.Errorf("expected parsed UID:GID in %q", got)
	}
}
