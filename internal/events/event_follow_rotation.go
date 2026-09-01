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
				// EOF on an inode that no longer has the logical pathname is a
				// completed generation. Reopen through the pathname so the outer
				// loop waits for the replacement instead of accepting later writes
				// through an orphaned descriptor.
				return true, nil
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

		// A copytruncate-style rotation can truncate and regrow the same inode
		// beyond our old offset between polling intervals. Size-only detection
		// misses that race. The first complete record is immutable for a true
		// append-only generation, so a changed/disappeared anchor proves that the
		// inode contents were reset even when its size has already recovered.
		currentAnchor, anchorErr := readEventGenerationAnchor(f)
		if anchorErr != nil {
			return false, anchorErr
		}
		if len(generationAnchor) > 0 && !bytes.Equal(generationAnchor, currentAnchor) {
			return true, nil
		}
		if len(generationAnchor) == 0 && len(currentAnchor) > 0 {
			generationAnchor = currentAnchor
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

// readEventGenerationAnchor returns the first complete record, bounded to a
// small prefix so ordinary event following never allocates based on potentially
// corrupt log contents. No complete record within the prefix means there is no
// anchor yet; size/identity checks continue to provide the fallback semantics.
// ReadAt deliberately leaves the follower's sequential offset untouched.
func readEventGenerationAnchor(f *os.File) ([]byte, error) {
	buf := make([]byte, eventGenerationAnchorLimit)
	n, err := f.ReadAt(buf, 0)
	if err != nil && err != io.EOF {
		return nil, fmt.Errorf("read event log generation anchor: %w", err)
	}
	if i := bytes.IndexByte(buf[:n], '\n'); i >= 0 {
		return bytes.Clone(buf[:i+1]), nil
	}
	return nil, nil
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
