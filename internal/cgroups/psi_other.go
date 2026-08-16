//go:build !linux

package cgroups

type PSIValues struct {
	Avg10  float64 `json:"avg10"`
	Avg60  float64 `json:"avg60"`
	Avg300 float64 `json:"avg300"`
	Total  uint64  `json:"total"`
}

func ReadPSI(cgroupPath string, resource string) (*PSIValues, error) {
	return &PSIValues{}, nil
}
