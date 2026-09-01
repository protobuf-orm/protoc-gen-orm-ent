// Package entuuid holds what a generated server needs of a UUID and the
// standard library does not give it.
//
// It is deliberately thin. Everything else about a UUID -- making one,
// parsing one, writing one out -- is the uuid package's, and code that needs
// those should reach for it directly.
package entuuid

import (
	"fmt"
	"uuid"
)

// Size is the number of bytes a UUID is.
const Size = 16

// FromBytes returns the UUID the given bytes are.
//
// A UUID crosses the wire as protobuf `bytes`, where nothing enforces its
// length, so the length is what this checks. The standard library parses text
// and converts a [Size]byte, and neither is what a message carries.
func FromBytes(b []byte) (uuid.UUID, error) {
	if len(b) != Size {
		return uuid.Nil(), fmt.Errorf("uuid is %d bytes, got %d", Size, len(b))
	}
	return uuid.UUID(b), nil
}

// Must returns v, and panics if err is not nil. It is for a UUID that cannot
// fail to be one -- a literal in a test, a value just read out of a UUID.
func Must(v uuid.UUID, err error) uuid.UUID {
	if err != nil {
		panic(err)
	}
	return v
}
