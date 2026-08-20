// Package imagestore provides OCI image configuration inspection utilities.
// This file implements a security auditor analyzing user execution privilege,
// low-numbered exposed ports (<1024), and environment variables.

package imagestore

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// SecurityAuditReport contains evaluated risk indicators from image config.
type SecurityAuditReport struct {
	User               string
	RunsAsRoot         bool
	HasPrivilegedPorts bool
	PrivilegedPorts    []int
	HardcodedSecrets   []string
	RiskScore          int // 0 (low risk) to 100 (high risk)
}

// AuditSecurityCapabilities parses image config JSON and evaluates container execution privilege risks.
func AuditSecurityCapabilities(configJSON []byte) (SecurityAuditReport, error) {
	var cfg struct {
		Config struct {
			User         string                 `json:"User,omitempty"`
			ExposedPorts map[string]interface{} `json:"ExposedPorts,omitempty"`
			Env          []string               `json:"Env,omitempty"`
		} `json:"config"`
	}
	if err := json.Unmarshal(configJSON, &cfg); err != nil {
		return SecurityAuditReport{}, fmt.Errorf("parse config for security audit: %w", err)
	}

	report := SecurityAuditReport{
		User: cfg.Config.User,
	}

	// 1. Check User (empty or "root" or "0" runs as root)
	u := strings.TrimSpace(cfg.Config.User)
	if u == "" || u == "root" || u == "0" || strings.HasPrefix(u, "0:") {
		report.RunsAsRoot = true
		report.RiskScore += 40
	}

	// 2. Check Privileged Ports (<1024)
	for portProto := range cfg.Config.ExposedPorts {
		parts := strings.Split(portProto, "/")
		if portNum, err := strconv.Atoi(parts[0]); err == nil {
			if portNum > 0 && portNum < 1024 {
				report.HasPrivilegedPorts = true
				report.PrivilegedPorts = append(report.PrivilegedPorts, portNum)
			}
		}
	}
	if report.HasPrivilegedPorts {
		report.RiskScore += 30
	}

	// 3. Check for hardcoded credentials in environment variables
	suspiciousKeys := []string{"PASSWORD", "PASSWD", "SECRET_KEY", "API_KEY", "PRIVATE_KEY"}
	for _, env := range cfg.Config.Env {
		upper := strings.ToUpper(env)
		for _, key := range suspiciousKeys {
			if strings.Contains(upper, key+"=") {
				report.HardcodedSecrets = append(report.HardcodedSecrets, strings.SplitN(env, "=", 2)[0])
				break
			}
		}
	}
	if len(report.HardcodedSecrets) > 0 {
		report.RiskScore += 30
	}

	if report.RiskScore > 100 {
		report.RiskScore = 100
	}

	return report, nil
}

// FormatSecurityAuditReport returns a human-readable security risk summary.
func FormatSecurityAuditReport(configJSON []byte) string {
	report, err := AuditSecurityCapabilities(configJSON)
	if err != nil {
		return fmt.Sprintf("error: %v", err)
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Security Risk Score: %d/100\n", report.RiskScore))
	sb.WriteString(fmt.Sprintf("  Runs as Root: %t (User: %q)\n", report.RunsAsRoot, report.User))
	sb.WriteString(fmt.Sprintf("  Privileged Ports: %t (%v)\n", report.HasPrivilegedPorts, report.PrivilegedPorts))
	sb.WriteString(fmt.Sprintf("  Suspicious Env Secrets: %d", len(report.HardcodedSecrets)))
	return sb.String()
}
