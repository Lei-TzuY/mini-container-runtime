package image

import (
	"archive/tar"
	"bytes"
	"fmt"
	"time"
)

// tarDeclaredMetadata captures the inode metadata that tar can declare for an
// entry independently of its pathname. Hardlink entries share an inode with
// their source, so two headers for the same inode cannot truthfully declare
// different values for these fields.
type tarDeclaredMetadata struct {
	mode    int64
	uid     int
	gid     int
	modTime time.Time
	xattrs  map[string][]byte
}

type tarMetadataTracker map[string]tarDeclaredMetadata

func metadataDeclaredBy(hdr *tar.Header) tarDeclaredMetadata {
	return tarDeclaredMetadata{
		mode:    hdr.Mode & 0o7777,
		uid:     hdr.Uid,
		gid:     hdr.Gid,
		modTime: hdr.ModTime,
		xattrs:  tarXattrsPortable(hdr),
	}
}

func (t tarMetadataTracker) remember(target string, hdr *tar.Header) {
	if t == nil || hdr == nil {
		return
	}
	t[target] = metadataDeclaredBy(hdr)
}

func (t tarMetadataTracker) verifyHardlink(linkTarget string, hdr *tar.Header) error {
	if t == nil || hdr == nil {
		return nil
	}
	source, ok := t[linkTarget]
	if !ok {
		// The source can legitimately predate this archive/layer. In that case
		// the filesystem is still the authority and createHardlinkSecure pins
		// the exact source inode before publishing the destination.
		return nil
	}
	link := metadataDeclaredBy(hdr)
	if source.mode != link.mode {
		return fmt.Errorf("hardlink metadata conflicts with source %q: mode %#o != %#o", linkTarget, link.mode, source.mode)
	}
	if source.uid != link.uid || source.gid != link.gid {
		return fmt.Errorf("hardlink metadata conflicts with source %q: ownership %d:%d != %d:%d", linkTarget, link.uid, link.gid, source.uid, source.gid)
	}
	if !source.modTime.Equal(link.modTime) {
		return fmt.Errorf("hardlink metadata conflicts with source %q: mtime %s != %s", linkTarget, link.modTime.Format(time.RFC3339Nano), source.modTime.Format(time.RFC3339Nano))
	}
	if !equalTarXattrs(source.xattrs, link.xattrs) {
		return fmt.Errorf("hardlink metadata conflicts with source %q: xattrs differ", linkTarget)
	}
	return nil
}

func equalTarXattrs(a, b map[string][]byte) bool {
	if len(a) != len(b) {
		return false
	}
	for name, value := range a {
		other, ok := b[name]
		if !ok || !bytes.Equal(value, other) {
			return false
		}
	}
	return true
}
