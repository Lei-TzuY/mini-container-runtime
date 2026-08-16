//go:build linux

package cgroups

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type PSIValues struct {
	Avg10  float64 `json:"avg10"`
	Avg60  float64 `json:"avg60"`
	Avg300 float64 `json:"avg300"`
	Total  uint64  `json:"total"`
}

// ReadPSI reads Cgroup v2 pressure files (e.g. memory.pressure, cpu.pressure).
func ReadPSI(cgroupPath string, resource string) (*PSIValues, error) {
	psiFile := filepath.Join(cgroupPath, resource+".pressure")
	content, err := os.ReadFile(psiFile)
	if err != nil {
		return nil, fmt.Errorf("read %s.pressure: %w", resource, err)
	}

	res := &PSIValues{}
	lines := strings.Split(string(content), "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "some ") || strings.HasPrefix(line, "full ") {
			fields := strings.Fields(line)
			for _, f := range fields[1:] {
				kv := strings.Split(f, "=")
				if len(kv) == 2 {
					val, _ := strconv.ParseFloat(kv[1], 64)
					switch kv[0] {
					case "avg10":
						res.Avg10 = val
					case "avg60":
						res.Avg60 = val
					case "avg300":
						res.Avg300 = val
					case "total":
						tot, _ := strconv.ParseUint(kv[1], 10, 64)
						res.Total = tot
					}
				}
			}
			break
		}
	}

	return res, nil
}
