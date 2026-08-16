package logs

import (
	"fmt"
	"os"
)

// RotateLogFile truncates logFile to maxBytes if it exceeds the limit.
func RotateLogFile(logPath string, maxBytes int64) error {
	if maxBytes <= 0 {
		return nil
	}

	fi, err := os.Stat(logPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("stat log file: %w", err)
	}

	if fi.Size() > maxBytes {
		data, err := os.ReadFile(logPath)
		if err != nil {
			return err
		}
		truncated := data[int64(len(data))-maxBytes:]
		return os.WriteFile(logPath, truncated, 0644)
	}

	return nil
}
