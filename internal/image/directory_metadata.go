package image

import (
	"os"
	"time"
)

type directoryMetadata struct {
	target  string
	mode    os.FileMode
	modTime time.Time
}
