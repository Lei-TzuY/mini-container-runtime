// internal/logs/logs.go
//
// Container Log Storage & Retrieval
// ─────────────────────────────────
// When a container is launched in background or attached mode, its stdout and
// stderr streams can be tee'd or written to a log file under:
//
//   ~/.minicontainer/containers/<id>/console.log
//
// This file implements log writing, tailing, and log following (`minictl logs -f`).

package logs

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"minicontainer/internal/state"
)

// LogFilePath returns the canonical path to a container's log file.
func LogFilePath(containerID string) string {
	return filepath.Join(state.DefaultDir(), "containers", containerID+".log")
}

// CreateLogFile creates or opens the container's log file for writing.
func CreateLogFile(containerID string) (*os.File, error) {
	path := LogFilePath(containerID)
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, err
	}
	return os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
}

// PrintLogs prints the contents of the container's log file.
// If tail > 0, only the last `tail` lines are printed.
// If follow is true, it continuously streams new lines until interrupted.
func PrintLogs(containerID string, tail int, follow bool, out io.Writer) error {
	path := LogFilePath(containerID)
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("no logs found for container %s", containerID[:8])
		}
		return fmt.Errorf("open log file: %w", err)
	}
	defer f.Close()

	if tail > 0 {
		lines, err := readLastNLines(f, tail)
		if err != nil {
			return fmt.Errorf("read last lines: %w", err)
		}
		for _, l := range lines {
			fmt.Fprintln(out, l)
		}
	} else {
		if _, err := io.Copy(out, f); err != nil {
			return err
		}
	}

	if !follow {
		return nil
	}

	// Follow mode: poll file for new content
	reader := bufio.NewReader(f)
	for {
		line, err := reader.ReadString('\n')
		if err == nil {
			fmt.Fprint(out, line)
			continue
		}
		if err == io.EOF {
			time.Sleep(200 * time.Millisecond)
			continue
		}
		return err
	}
}

func readLastNLines(r io.ReadSeeker, n int) ([]string, error) {
	var lines []string
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return lines, nil
}
