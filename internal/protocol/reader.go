package protocol

import (
	"errors"
	"fmt"
)

// ErrUnexpectedEnd is returned when a Reader read runs past the end of its
// data. It mirrors the TS Reader's "Unexpected end of packet" throw
// (ClientProtocol.ts, Reader methods, lines 381-474).
var ErrUnexpectedEnd = errors.New("protocol: unexpected end of packet")

// ErrUnterminatedString is returned by Reader.String when no NUL terminator
// is found. It mirrors the TS Reader's "Unterminated packet string" throw
// (ClientProtocol.ts line 454).
var ErrUnterminatedString = errors.New("protocol: unterminated packet string")

// Reader is a bounds-checked cursor over one packet's bytes, ported from the
// TS Reader class (ClientProtocol.ts lines 381-474). Every read returns an
// error on overrun instead of throwing. Multi-byte integers are big-endian
// unless the method name says otherwise (LE/ME/IME); the Add/Sub/Neg methods
// apply the OSRS-style +/-128 adjustments the native opcode set uses.
type Reader struct {
	data   []byte
	offset int
}

// NewReader returns a Reader over data. The data is not copied; callers must
// not mutate it while the Reader is in use.
func NewReader(data []byte) *Reader {
	return &Reader{data: data}
}

// Remaining returns the number of unread bytes.
func (r *Reader) Remaining() int {
	return len(r.data) - r.offset
}

// Byte reads one unsigned byte (ClientProtocol.ts line 386).
func (r *Reader) Byte() (byte, error) {
	if r.offset >= len(r.data) {
		return 0, ErrUnexpectedEnd
	}
	b := r.data[r.offset]
	r.offset++
	return b, nil
}

// Short reads a u16, big-endian (ClientProtocol.ts line 391).
func (r *Reader) Short() (uint16, error) {
	if r.offset+2 > len(r.data) {
		return 0, ErrUnexpectedEnd
	}
	v := uint16(r.data[r.offset])<<8 | uint16(r.data[r.offset+1])
	r.offset += 2
	return v, nil
}

// SignedShort reads a signed 16-bit integer, big-endian
// (ClientProtocol.ts line 398).
func (r *Reader) SignedShort() (int16, error) {
	v, err := r.Short()
	if err != nil {
		return 0, err
	}
	return int16(v), nil
}

// Int reads a signed 32-bit integer, big-endian (ClientProtocol.ts line 430).
func (r *Reader) Int() (int32, error) {
	if r.offset+4 > len(r.data) {
		return 0, ErrUnexpectedEnd
	}
	v := uint32(r.data[r.offset])<<24 | uint32(r.data[r.offset+1])<<16 |
		uint32(r.data[r.offset+2])<<8 | uint32(r.data[r.offset+3])
	r.offset += 4
	return int32(v), nil
}

// IntLE reads a signed 32-bit integer, little-endian
// (ClientProtocol.ts line 437).
func (r *Reader) IntLE() (int32, error) {
	if r.offset+4 > len(r.data) {
		return 0, ErrUnexpectedEnd
	}
	v := uint32(r.data[r.offset]) | uint32(r.data[r.offset+1])<<8 |
		uint32(r.data[r.offset+2])<<16 | uint32(r.data[r.offset+3])<<24
	r.offset += 4
	return int32(v), nil
}

// ShortAdd reads a u16 whose low byte comes first with a -128 adjustment:
// low = (first byte - 128), high = second byte (ClientProtocol.ts line 403).
func (r *Reader) ShortAdd() (uint16, error) {
	low, err := r.Byte()
	if err != nil {
		return 0, err
	}
	high, err := r.Byte()
	if err != nil {
		return 0, err
	}
	return uint16(high)<<8 | uint16(low-128), nil
}

// ShortAddLE reads a u16 whose high byte comes first and whose low byte
// carries a -128 adjustment (ClientProtocol.ts line 408).
func (r *Reader) ShortAddLE() (uint16, error) {
	high, err := r.Byte()
	if err != nil {
		return 0, err
	}
	low, err := r.Byte()
	if err != nil {
		return 0, err
	}
	return uint16(high)<<8 | uint16(low-128), nil
}

// ShortLE reads a u16, little-endian (ClientProtocol.ts line 413).
func (r *Reader) ShortLE() (uint16, error) {
	low, err := r.Byte()
	if err != nil {
		return 0, err
	}
	high, err := r.Byte()
	if err != nil {
		return 0, err
	}
	return uint16(low) | uint16(high)<<8, nil
}

// ByteAdd reads a byte with a -128 adjustment: (b - 128) mod 256
// (ClientProtocol.ts line 418).
func (r *Reader) ByteAdd() (byte, error) {
	b, err := r.Byte()
	if err != nil {
		return 0, err
	}
	return b - 128, nil
}

// ByteSub reads a byte as (128 - b) mod 256 (ClientProtocol.ts line 422).
func (r *Reader) ByteSub() (byte, error) {
	b, err := r.Byte()
	if err != nil {
		return 0, err
	}
	return 128 - b, nil
}

// ByteNeg reads a byte as (-b) mod 256 (ClientProtocol.ts line 426).
func (r *Reader) ByteNeg() (byte, error) {
	b, err := r.Byte()
	if err != nil {
		return 0, err
	}
	return -b, nil
}

// IntME reads a middle-endian int32: bytes b0 b1 b2 b3 map to
// (b2<<24)|(b3<<16)|(b0<<8)|b1 (ClientProtocol.ts line 444).
func (r *Reader) IntME() (int32, error) {
	b0, err := r.Byte()
	if err != nil {
		return 0, err
	}
	b1, err := r.Byte()
	if err != nil {
		return 0, err
	}
	b2, err := r.Byte()
	if err != nil {
		return 0, err
	}
	b3, err := r.Byte()
	if err != nil {
		return 0, err
	}
	return int32(uint32(b2)<<24 | uint32(b3)<<16 | uint32(b0)<<8 | uint32(b1)), nil
}

// IntIME reads an inverse-middle-endian int32: bytes b0 b1 b2 b3 map to
// (b1<<24)|(b0<<16)|(b3<<8)|b2 (ClientProtocol.ts line 449).
func (r *Reader) IntIME() (int32, error) {
	b0, err := r.Byte()
	if err != nil {
		return 0, err
	}
	b1, err := r.Byte()
	if err != nil {
		return 0, err
	}
	b2, err := r.Byte()
	if err != nil {
		return 0, err
	}
	b3, err := r.Byte()
	if err != nil {
		return 0, err
	}
	return int32(uint32(b1)<<24 | uint32(b0)<<16 | uint32(b3)<<8 | uint32(b2)), nil
}

// String reads a latin1 string terminated by a NUL byte, returning it as a
// Go string (each latin1 byte becomes the rune of the same value, matching
// the TS latin1 decode). It errors if no terminator is found
// (ClientProtocol.ts line 454).
func (r *Reader) String() (string, error) {
	end := -1
	for i := r.offset; i < len(r.data); i++ {
		if r.data[i] == 0 {
			end = i
			break
		}
	}
	if end == -1 {
		return "", ErrUnterminatedString
	}
	s := latin1ToString(r.data[r.offset:end])
	r.offset = end + 1
	return s, nil
}

// Bytes returns a copy of the next n bytes (ClientProtocol.ts line 462; the
// TS version returns a view, this copy is semantically identical at read
// time and safe to retain). It errors if n is negative or overruns the data.
func (r *Reader) Bytes(n int) ([]byte, error) {
	if n < 0 || r.offset+n > len(r.data) {
		return nil, fmt.Errorf("%w: cannot read %d bytes with %d remaining", ErrUnexpectedEnd, n, r.Remaining())
	}
	out := make([]byte, n)
	copy(out, r.data[r.offset:r.offset+n])
	r.offset += n
	return out, nil
}

// latin1ToString converts latin1 bytes to a Go string, mapping each byte to
// the Unicode code point of the same value (as Buffer.toString("latin1")
// does in TS).
func latin1ToString(b []byte) string {
	runes := make([]rune, len(b))
	for i, c := range b {
		runes[i] = rune(c)
	}
	return string(runes)
}

// latin1Bytes encodes s as latin1 bytes followed by a NUL terminator,
// mirroring the TS string() helper (ClientProtocol.ts lines 1096-1098,
// Buffer.from(value, "latin1") plus a zero byte). Runes above U+00FF are
// truncated to their low 8 bits, as Node's latin1 encoding does.
func latin1Bytes(s string) []byte {
	out := make([]byte, 0, len(s)+1)
	for _, r := range s {
		out = append(out, byte(r))
	}
	return append(out, 0)
}
