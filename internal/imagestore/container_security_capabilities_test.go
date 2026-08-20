package imagestore

import (
	"strings"
	"testing"
)

func TestAuditSecurityCapabilities(t *testing.T) {
	tests := []struct {
		name       string
		json       string
		wantRoot   bool
		wantPrivP  bool
		wantScore  int
		wantErr    bool
	}{
		{
			name: "high risk container: root, port 80, password env",
			json: `{
				"config": {
					"User": "0:0",
					"ExposedPorts": {"80/tcp": {}, "443/tcp": {}},
					"Env": ["API_KEY=12345", "PATH=/bin"]
				}
			}`,
			wantRoot:  true,
			wantPrivP: true,
			wantScore: 100, // 40 + 30 + 30
			wantErr:   false,
		},
		{
			name: "low risk container: non-root, unprivileged port",
			json: `{
				"config": {
					"User": "1001",
					"ExposedPorts": {"8080/tcp": {}},
					"Env": ["PORT=8080"]
				}
			}`,
			wantRoot:  false,
			wantPrivP: false,
			wantScore: 0,
			wantErr:   false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			report, err := AuditSecurityCapabilities([]byte(tc.json))
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if report.RunsAsRoot != tc.wantRoot {
				t.Errorf("RunsAsRoot = %t, want %t", report.RunsAsRoot, tc.wantRoot)
			}
			if report.HasPrivilegedPorts != tc.wantPrivP {
				t.Errorf("HasPrivilegedPorts = %t, want %t", report.HasPrivilegedPorts, tc.wantPrivP)
			}
			if report.RiskScore != tc.wantScore {
				t.Errorf("RiskScore = %d, want %d", report.RiskScore, tc.wantScore)
			}
		})
	}
}

func TestFormatSecurityAuditReport(t *testing.T) {
	jsonBlob := `{"config":{"User":"1000","ExposedPorts":{"8080/tcp":{}}}}`
	got := FormatSecurityAuditReport([]byte(jsonBlob))
	if !strings.Contains(got, "Security Risk Score: 0/100") {
		t.Errorf("expected 0/100 risk score, got %q", got)
	}
}
