package events

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"
)

const eventFollowPollInterval = 200 * time.Millisecond

// followEventLogFile follows the logical events.log pathname, not merely the
// inode that happened to exist when the command started. This matters when an
// administrator rotates/replaces the audit log or truncates it in place: a
// long-running observer must not remain pinned forever to an orphaned inode or
// an offset beyond the new end of file.
func followEventLogFile(logFile string, opts StreamOptions, w io.Writer) error {
	for {
		f, err := openEventLogForStream(logFile, true)
		if err != nil {
			return err
		}
		reopen, err := followOpenEventLog(f, logFile, opts, w)
		closeErr := f.Close()
		if err != nil {
			return err
		}
		if closeErr != nil {
			return fmt.Errorf("close event log: %w", closeErr)
		}
		if !reopen {
			return nil
		}
	}
}

// followOpenEventLog returns reopen=true when the pathname now identifies a
// different file or the current file was truncated behind our read offset.
func followOpenEventLog(f *os.File, logFile string, opts StreamOptions, w io.Writer) (bool, error) {
	reader := bufio.NewReader(f)
	var pending []byte

	for {
		line, err := reader.ReadBytes('\n')
		if len(line) > 0 {
			pending = append(pending, line...)
			if err == nil {
				if decodeErr := writeCompleteEventRecord(pending, opts, w); decodeErr != nil {
					return false, decodeErr
				}
				pending = pending[:0]
			}
		}
		if err == nil {
			continue
		}
		if err != io.EOF {
			return false, fmt.Errorf("read event log: %w", err)
		}

		currentInfo, statErr := f.Stat()
		if statErr != nil {
			return false, fmt.Errorf("stat open event log: %w", statErr)
		}
		pathInfo, pathErr := os.Stat(logFile)
		if pathErr != nil {
			if os.IsNotExist(pathErr) {
				time.Sleep(eventFollowPollInterval)
				continue
			}
			return false, fmt.Errorf("stat event log path: %w", pathErr)
		}

		offset, seekErr := f.Seek(0, io.SeekCurrent)
		if seekErr != nil {
			return false, fmt.Errorf("inspect event log offset: %w", seekErr)
		}
		if !os.SameFile(currentInfo, pathInfo) || pathInfo.Size() < offset-int64(reader.Buffered()) {
			// Never emit a pending unterminated record from the old generation. A
			// complete event is durable only after its terminating newline.
			return true, nil
		}

		if len(pending) > 0 {
			// Rebuild the reader so a record that was torn at EOF can be completed
			// when later bytes arrive, without emitting or losing its prefix.
			reader = bufio.NewReader(io.MultiReader(bytes.NewReader(pending), reader))
			pending = pending[:0]
		}
		time.Sleep(eventFollowPollInterval)
	}
}

func writeCompleteEventRecord(line []byte, opts StreamOptions, w io.Writer) error {
	var evt Event
	if err := json.Unmarshal(line, &evt); err != nil {
		return fmt.Errorf("decode event log: %w", err)
	}
	if !eventMatchesQuery(evt, opts) {
		return nil
	}
	return writeQueriedEvent(w, evt, opts.JSON)
}
