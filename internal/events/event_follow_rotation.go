package events

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"time"
)

const eventFollowPollInterval = 200 * time.Millisecond
const eventGenerationAnchorLimit = 4096

type eventGenerationCheckpoint struct {
	offset int64
	data   []byte
}

func followDeadlineReached(until, now time.Time) bool {
	return !until.IsZero() && !now.Before(until)
}

func followPollDelay(until, now time.Time) time.Duration {
	if until.IsZero() {
		return eventFollowPollInterval
	}
	remaining := until.Sub(now)
	if remaining <= 0 {
		return 0
	}
	if remaining < eventFollowPollInterval {
		return remaining
	}
	return eventFollowPollInterval
}

func openEventLogForFollowWith(logFile string, until time.Time, open eventLogOpenFunc, now func() time.Time, wait func(time.Duration)) (*os.File, bool, error) {
	for {
		f, err := open(logFile)
		if err == nil {
			return f, false, nil
		}
		if !os.IsNotExist(err) {
			return nil, false, err
		}
		delay := followPollDelay(until, now())
		if delay == 0 {
			return nil, true, nil
		}
		wait(delay)
	}
}

// followEventLogFile follows the logical events.log pathname, not merely the
// inode that happened to exist when the command started. This matters when an
// administrator rotates/replaces the audit log or truncates it in place: a
// long-running observer must not remain pinned forever to an orphaned inode or
// an offset beyond the new end of file.
func followEventLogFile(logFile string, opts StreamOptions, w io.Writer) error {
	for {
		f, expired, err := openEventLogForFollowWith(logFile, opts.Until, openEventLogForRead, time.Now, time.Sleep)
		if err != nil {
			return err
		}
		if expired {
			return nil
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
// different file, disappears, or the current file was truncated behind our
// read offset. Once EOF proves we have consumed the current generation, a
// missing pathname is also a generation boundary: waiting on the orphaned open
// inode would otherwise allow post-unlink appends to leak into the logical
// events.log stream.
func followOpenEventLog(f *os.File, logFile string, opts StreamOptions, w io.Writer) (bool, error) {
	reader := bufio.NewReader(f)
	var pending []byte
	generationAnchor, err := readEventGenerationAnchor(f)
	if err != nil {
		return false, err
	}
	var checkpoint eventGenerationCheckpoint

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

		reopen, updatedAnchor, inspectErr := inspectEventLogGenerationWithCheckpoint(f, logFile, reader.Buffered(), generationAnchor, checkpoint)
		if inspectErr != nil {
			return false, inspectErr
		}
		generationAnchor = updatedAnchor
		if reopen {
			// Never emit a pending unterminated record from the old generation. A
			// complete event is durable only after its terminating newline.
			return true, nil
		}
		checkpoint, inspectErr = readEventGenerationCheckpoint(f, reader.Buffered())
		if inspectErr != nil {
			return false, inspectErr
		}
		if followDeadlineReached(opts.Until, time.Now()) {
			// Reaching --until only terminates after EOF and generation validation,
			// so durable records already present (including a just-rotated file) are
			// drained while a torn tail remains intentionally uncommitted.
			return false, nil
		}

		if len(pending) > 0 {
			// Rebuild the reader so a record that was torn at EOF can be completed
			// when later bytes arrive, without emitting or losing its prefix.
			reader = bufio.NewReader(io.MultiReader(bytes.NewReader(pending), reader))
			pending = pending[:0]
		}
		delay := followPollDelay(opts.Until, time.Now())
		if delay == 0 {
			return false, nil
		}
		time.Sleep(delay)

		// Rotation can happen after the EOF inspection but before the next read.
		// Revalidate here so we never consume bytes from a reset generation at an
		// offset inherited from the previous one.
		reopen, updatedAnchor, inspectErr = inspectEventLogGenerationWithCheckpoint(f, logFile, reader.Buffered(), generationAnchor, checkpoint)
		if inspectErr != nil {
			return false, inspectErr
		}
		generationAnchor = updatedAnchor
		if reopen {
			return true, nil
		}
	}
}

// inspectEventLogGeneration verifies that the open descriptor still represents
// the same append-only generation exposed by logFile. The compatibility wrapper
// retains the focused prefix-anchor API used by earlier tests.
func inspectEventLogGeneration(f *os.File, logFile string, buffered int, generationAnchor []byte) (bool, []byte, error) {
	return inspectEventLogGenerationWithCheckpoint(f, logFile, buffered, generationAnchor, eventGenerationCheckpoint{})
}

// inspectEventLogGenerationWithCheckpoint combines the bounded prefix anchor
// with a bounded checkpoint immediately behind the follower's last consumed
// offset. The second anchor closes the common copytruncate race where a tool
// preserves the first prefix while replacing later bytes and regrowing past the
// old offset before the next poll.
func inspectEventLogGenerationWithCheckpoint(f *os.File, logFile string, buffered int, generationAnchor []byte, checkpoint eventGenerationCheckpoint) (bool, []byte, error) {
	currentInfo, err := f.Stat()
	if err != nil {
		return false, generationAnchor, fmt.Errorf("stat open event log: %w", err)
	}
	pathInfo, err := os.Stat(logFile)
	if err != nil {
		if os.IsNotExist(err) {
			return true, generationAnchor, nil
		}
		return false, generationAnchor, fmt.Errorf("stat event log path: %w", err)
	}

	offset, err := f.Seek(0, io.SeekCurrent)
	if err != nil {
		return false, generationAnchor, fmt.Errorf("inspect event log offset: %w", err)
	}
	logicalOffset := offset - int64(buffered)
	if !os.SameFile(currentInfo, pathInfo) || pathInfo.Size() < logicalOffset {
		return true, generationAnchor, nil
	}

	currentAnchor, err := readEventGenerationAnchor(f)
	if err != nil {
		return false, generationAnchor, err
	}
	if len(generationAnchor) > 0 {
		if len(currentAnchor) < len(generationAnchor) || !bytes.Equal(generationAnchor, currentAnchor[:len(generationAnchor)]) {
			return true, generationAnchor, nil
		}
	} else if len(currentAnchor) > 0 {
		generationAnchor = currentAnchor
	}

	if len(checkpoint.data) > 0 {
		currentCheckpoint := make([]byte, len(checkpoint.data))
		n, readErr := f.ReadAt(currentCheckpoint, checkpoint.offset)
		if readErr != nil && readErr != io.EOF {
			return false, generationAnchor, fmt.Errorf("read event log generation checkpoint: %w", readErr)
		}
		if n != len(checkpoint.data) || !bytes.Equal(checkpoint.data, currentCheckpoint) {
			return true, generationAnchor, nil
		}
	}
	return false, generationAnchor, nil
}

// readEventGenerationAnchor returns a bounded prefix of the current log
// generation. The prefix does not require a terminating newline: anchoring the
// bytes of a large or temporarily torn first record still detects an in-place
// generation reset, while later append-only growth leaves the captured prefix
// unchanged. ReadAt deliberately leaves the follower's sequential offset
// untouched.
func readEventGenerationAnchor(f *os.File) ([]byte, error) {
	buf := make([]byte, eventGenerationAnchorLimit)
	n, err := f.ReadAt(buf, 0)
	if err != nil && err != io.EOF {
		return nil, fmt.Errorf("read event log generation anchor: %w", err)
	}
	return bytes.Clone(buf[:n]), nil
}

// readEventGenerationCheckpoint captures a bounded immutable window ending at
// the follower's logical consumed offset. Later append-only writes cannot alter
// it, so a mismatch proves that the same inode was rewritten in place.
func readEventGenerationCheckpoint(f *os.File, buffered int) (eventGenerationCheckpoint, error) {
	offset, err := f.Seek(0, io.SeekCurrent)
	if err != nil {
		return eventGenerationCheckpoint{}, fmt.Errorf("inspect event log checkpoint offset: %w", err)
	}
	logicalOffset := offset - int64(buffered)
	if logicalOffset <= 0 {
		return eventGenerationCheckpoint{}, nil
	}
	length := int64(eventGenerationAnchorLimit)
	if logicalOffset < length {
		length = logicalOffset
	}
	start := logicalOffset - length
	data := make([]byte, int(length))
	n, err := f.ReadAt(data, start)
	if err != nil && err != io.EOF {
		return eventGenerationCheckpoint{}, fmt.Errorf("read event log generation checkpoint: %w", err)
	}
	if n != len(data) {
		return eventGenerationCheckpoint{}, fmt.Errorf("read event log generation checkpoint: short read %d/%d", n, len(data))
	}
	return eventGenerationCheckpoint{offset: start, data: data}, nil
}

func writeCompleteEventRecord(line []byte, opts StreamOptions, w io.Writer) error {
	evt, err := decodeEventRecord(line)
	if err != nil {
		return err
	}
	if !eventMatchesQuery(evt, opts) {
		return nil
	}
	return writeQueriedEvent(w, evt, opts.JSON)
}
