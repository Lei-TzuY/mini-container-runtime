package state

import "errors"

// ErrStoreClosed is returned when an operation requiring Store-owned resources
// is attempted after Close has released them.
var ErrStoreClosed = errors.New("state store is closed")
