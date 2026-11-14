package bzz

import "errors"

// ErrNotImplemented is returned until the decoder lands (Todo phase 3).
var ErrNotImplemented = errors.New("bzz: not implemented")

// Decode decompresses a BZZ stream.
func Decode(b []byte) ([]byte, error) {
	return nil, ErrNotImplemented
}
