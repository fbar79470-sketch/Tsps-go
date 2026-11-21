package cache

import (
	"errors"
	"math"
)

// ErrBufferOverflow is returned when a read or write would go past
// the end of the buffer. The TypeScript ByteBuffer throws
// Error("Buffer overflow") from readByte in the same situation
// (io/ByteBuffer.ts:28-33); reads that TypeScript performs through
// unchecked peeks (getByte) return an error here instead of
// operating on undefined values.
var ErrBufferOverflow = errors.New("buffer overflow")

// ByteBuffer is the cache codec's byte buffer, a port of ByteBuffer
// (io/ByteBuffer.ts). All multi-byte values are big-endian. Unlike the
// TypeScript version, which throws on overrun, every read and write
// returns an error; callers must check it. A failed read leaves
// Offset advanced past whatever bytes were consumed before the
// failure, mirroring the TypeScript behavior of throwing mid-read.
//
// Data is the underlying byte slice. ReadBytes returns views sharing
// it, as the TypeScript subarray does; callers that need to keep the
// bytes past a mutation of Data must copy them.
type ByteBuffer struct {
	Data   []byte
	Offset int
}

// NewByteBuffer wraps data in a ByteBuffer at offset 0.
func NewByteBuffer(data []byte) *ByteBuffer {
	return &ByteBuffer{Data: data}
}

// NewByteBufferSize returns a zero-filled ByteBuffer of the given size.
func NewByteBufferSize(size int) *ByteBuffer {
	return &ByteBuffer{Data: make([]byte, size)}
}

// Len returns the total length of the buffer.
func (b *ByteBuffer) Len() int { return len(b.Data) }

// Remaining returns the number of unread bytes.
func (b *ByteBuffer) Remaining() int { return len(b.Data) - b.Offset }

// ReadSignedByte reads one signed byte (-128..127). It is readByte
// in ByteBuffer.ts, renamed because Go's vet requires a ReadByte
// method to have the io.ByteReader signature (byte, error).
func (b *ByteBuffer) ReadSignedByte() (int, error) {
	if b.Offset > len(b.Data)-1 {
		return 0, ErrBufferOverflow
	}
	v := int(int8(b.Data[b.Offset]))
	b.Offset++
	return v, nil
}

// ReadUnsignedByte reads one unsigned byte (0..255).
func (b *ByteBuffer) ReadUnsignedByte() (int, error) {
	v, err := b.ReadSignedByte()
	if err != nil {
		return 0, err
	}
	return v & 0xff, nil
}

// ReadShort reads a signed 16-bit big-endian value.
func (b *ByteBuffer) ReadShort() (int, error) {
	hi, err := b.ReadUnsignedByte()
	if err != nil {
		return 0, err
	}
	lo, err := b.ReadUnsignedByte()
	if err != nil {
		return 0, err
	}
	return int(int16(uint16(hi<<8 | lo))), nil
}

// ReadUnsignedShort reads an unsigned 16-bit big-endian value.
func (b *ByteBuffer) ReadUnsignedShort() (int, error) {
	v, err := b.ReadShort()
	if err != nil {
		return 0, err
	}
	return v & 0xffff, nil
}

// ReadSignedShort reads a signed 16-bit big-endian value
// (readSignedShort in ByteBuffer.ts; identical to ReadShort).
func (b *ByteBuffer) ReadSignedShort() (int, error) {
	return b.ReadShort()
}

// ReadMedium reads an unsigned 24-bit big-endian value.
func (b *ByteBuffer) ReadMedium() (int, error) {
	b0, err := b.ReadUnsignedByte()
	if err != nil {
		return 0, err
	}
	b1, err := b.ReadUnsignedByte()
	if err != nil {
		return 0, err
	}
	b2, err := b.ReadUnsignedByte()
	if err != nil {
		return 0, err
	}
	return b0<<16 | b1<<8 | b2, nil
}

// ReadUnsignedMedium reads an unsigned 24-bit big-endian value
// (readUnsignedMedium in ByteBuffer.ts; identical to ReadMedium).
func (b *ByteBuffer) ReadUnsignedMedium() (int, error) {
	return b.ReadMedium()
}

// ReadSignedMedium reads a signed 24-bit big-endian value (g3s):
// 0xFFFFFF reads as -1.
func (b *ByteBuffer) ReadSignedMedium() (int, error) {
	v, err := b.ReadMedium()
	if err != nil {
		return 0, err
	}
	return int(int32(v<<8) >> 8), nil
}

// ReadInt reads a signed 32-bit big-endian value.
func (b *ByteBuffer) ReadInt() (int32, error) {
	b0, err := b.ReadUnsignedByte()
	if err != nil {
		return 0, err
	}
	b1, err := b.ReadUnsignedByte()
	if err != nil {
		return 0, err
	}
	b2, err := b.ReadUnsignedByte()
	if err != nil {
		return 0, err
	}
	b3, err := b.ReadUnsignedByte()
	if err != nil {
		return 0, err
	}
	return int32(uint32(b0)<<24 | uint32(b1)<<16 | uint32(b2)<<8 | uint32(b3)), nil
}

// ReadFloat32 reads a 32-bit IEEE float, big-endian
// (readFloat via FloatUtil.intBitsToFloat in ByteBuffer.ts).
func (b *ByteBuffer) ReadFloat32() (float32, error) {
	v, err := b.ReadInt()
	if err != nil {
		return 0, err
	}
	return math.Float32frombits(uint32(v)), nil
}

// ReadBigSmart reads a "big smart": if the next byte is negative
// (as a signed byte) it reads a 32-bit int masked to 31 bits,
// otherwise an unsigned short, with 32767 meaning -1.
func (b *ByteBuffer) ReadBigSmart() (int, error) {
	peek, err := b.GetByte(b.Offset)
	if err != nil {
		return 0, err
	}
	if peek < 0 {
		v, err := b.ReadInt()
		if err != nil {
			return 0, err
		}
		return int(v & 0x7fffffff), nil
	}
	v, err := b.ReadUnsignedShort()
	if err != nil {
		return 0, err
	}
	if v == 32767 {
		return -1, nil
	}
	return v, nil
}

// ReadUnsignedSmart reads an unsigned smart: one byte when it is
// below 128, otherwise an unsigned short minus 0x8000.
func (b *ByteBuffer) ReadUnsignedSmart() (int, error) {
	peek, err := b.GetUnsignedByte(b.Offset)
	if err != nil {
		return 0, err
	}
	if peek < 128 {
		return b.ReadUnsignedByte()
	}
	v, err := b.ReadUnsignedShort()
	if err != nil {
		return 0, err
	}
	return v - 0x8000, nil
}

// ReadUnsignedSmartMin1 is ReadUnsignedSmart minus one: values are
// stored biased by 1 so that 0 can mean "none".
func (b *ByteBuffer) ReadUnsignedSmartMin1() (int, error) {
	peek, err := b.GetUnsignedByte(b.Offset)
	if err != nil {
		return 0, err
	}
	if peek < 128 {
		v, err := b.ReadUnsignedByte()
		if err != nil {
			return 0, err
		}
		return v - 1, nil
	}
	v, err := b.ReadUnsignedShort()
	if err != nil {
		return 0, err
	}
	return v - 0x8001, nil
}

// ReadSmart2 reads a signed smart with a 64 bias in the one-byte form.
func (b *ByteBuffer) ReadSmart2() (int, error) {
	peek, err := b.GetByte(b.Offset)
	if err != nil {
		return 0, err
	}
	if peek >= 0 {
		v, err := b.ReadUnsignedByte()
		if err != nil {
			return 0, err
		}
		return v - 64, nil
	}
	v, err := b.ReadUnsignedShort()
	if err != nil {
		return 0, err
	}
	return v - 49152, nil
}

// ReadSmart3 reads a chain of unsigned smarts, accumulating 32767
// for each 32767 sentinel.
func (b *ByteBuffer) ReadSmart3() (int, error) {
	total := 0
	for {
		v, err := b.ReadUnsignedSmart()
		if err != nil {
			return 0, err
		}
		if v == 32767 {
			total += 32767
			continue
		}
		return total + v, nil
	}
}

// ReadUnsignedShortSmart is ReadUnsignedSmart under its other name in
// ByteBuffer.ts.
func (b *ByteBuffer) ReadUnsignedShortSmart() (int, error) {
	return b.ReadUnsignedSmart()
}

// ReadUnsignedShortSmartMinusOne is ReadUnsignedSmartMin1 under its
// other name in ByteBuffer.ts.
func (b *ByteBuffer) ReadUnsignedShortSmartMinusOne() (int, error) {
	return b.ReadUnsignedSmartMin1()
}

// ReadVarInt reads a big-endian base-128 varint: groups of 7 bits,
// most significant first, with the high bit set on all but the last
// group. Arithmetic is 32-bit, as in ByteBuffer.ts.
func (b *ByteBuffer) ReadVarInt() (int, error) {
	var value int32
	for {
		c, err := b.ReadUnsignedByte()
		if err != nil {
			return 0, err
		}
		if c&0x80 != 0 {
			value = (value | int32(c&0x7f)) << 7
		} else {
			return int(value | int32(c)), nil
		}
	}
}

// ReadVarInt2 reads a little-endian base-128 varint (LEB128-style):
// groups of 7 bits, least significant first. Arithmetic is 32-bit,
// as in ByteBuffer.ts.
func (b *ByteBuffer) ReadVarInt2() (int, error) {
	var value int32
	shift := uint(0)
	for {
		c, err := b.ReadUnsignedByte()
		if err != nil {
			return 0, err
		}
		value |= int32(c&0x7f) << shift
		if c <= 0x7f {
			break
		}
		shift += 7
	}
	return int(value), nil
}

// ReadString reads a latin1 NUL-terminated string. Bytes map to the
// Unicode code points of the same value, as String.fromCharCode does
// in ByteBuffer.ts. An unterminated string returns ErrBufferOverflow.
func (b *ByteBuffer) ReadString() (string, error) {
	return b.ReadStringTerminated(0)
}

// ReadStringTerminated reads a latin1 string terminated by the given
// byte value, consuming the terminator.
func (b *ByteBuffer) ReadStringTerminated(end byte) (string, error) {
	runes := make([]rune, 0, 16)
	for {
		if b.Offset >= len(b.Data) {
			return "", ErrBufferOverflow
		}
		if b.Data[b.Offset] == end {
			b.Offset++
			return string(runes), nil
		}
		c, err := b.ReadUnsignedByte()
		if err != nil {
			return "", err
		}
		runes = append(runes, rune(c))
	}
}

// Peek returns the unsigned byte at Offset+delta without consuming it.
func (b *ByteBuffer) Peek(delta int) (int, error) {
	return b.GetUnsignedByte(b.Offset + delta)
}

// ReadNullString reads a possibly-null string: a leading NUL byte
// means absent (present=false); otherwise a NUL-terminated string.
func (b *ByteBuffer) ReadNullString() (string, bool, error) {
	if b.Offset < len(b.Data) && b.Data[b.Offset] == 0 {
		b.Offset++
		return "", false, nil
	}
	s, err := b.ReadString()
	if err != nil {
		return "", false, err
	}
	return s, true, nil
}

// ReadVerString reads a versioned string: a leading non-zero byte
// means absent (present=false); a leading zero byte is followed by a
// NUL-terminated string.
func (b *ByteBuffer) ReadVerString() (string, bool, error) {
	v, err := b.ReadSignedByte()
	if err != nil {
		return "", false, err
	}
	if v != 0 {
		return "", false, nil
	}
	s, err := b.ReadString()
	if err != nil {
		return "", false, err
	}
	return s, true, nil
}

// GetByte returns the signed byte at an absolute offset without
// moving Offset.
func (b *ByteBuffer) GetByte(offset int) (int, error) {
	if offset < 0 || offset >= len(b.Data) {
		return 0, ErrBufferOverflow
	}
	return int(int8(b.Data[offset])), nil
}

// GetUnsignedByte returns the unsigned byte at an absolute offset
// without moving Offset.
func (b *ByteBuffer) GetUnsignedByte(offset int) (int, error) {
	v, err := b.GetByte(offset)
	if err != nil {
		return 0, err
	}
	return v & 0xff, nil
}

// GetShort returns the unsigned 16-bit big-endian value at an
// absolute offset without moving Offset. (Despite the name,
// ByteBuffer.ts getShort returns an unsigned value.)
func (b *ByteBuffer) GetShort(offset int) (int, error) {
	hi, err := b.GetUnsignedByte(offset)
	if err != nil {
		return 0, err
	}
	lo, err := b.GetUnsignedByte(offset + 1)
	if err != nil {
		return 0, err
	}
	return hi<<8 | lo, nil
}

// GetUnsignedShort returns the unsigned 16-bit big-endian value at an
// absolute offset without moving Offset.
func (b *ByteBuffer) GetUnsignedShort(offset int) (int, error) {
	return b.GetShort(offset)
}

// GetInt returns the signed 32-bit big-endian value at an absolute
// offset without moving Offset.
func (b *ByteBuffer) GetInt(offset int) (int32, error) {
	if offset < 0 || offset+4 > len(b.Data) {
		return 0, ErrBufferOverflow
	}
	return int32(uint32(b.Data[offset])<<24 | uint32(b.Data[offset+1])<<16 | uint32(b.Data[offset+2])<<8 | uint32(b.Data[offset+3])), nil
}

// ReadBytes returns a view of the next n bytes and advances Offset.
// It returns an error when fewer than n bytes remain; the TypeScript
// subarray silently clamps in that case, so this is stricter.
func (b *ByteBuffer) ReadBytes(n int) ([]byte, error) {
	if n < 0 || n > b.Remaining() {
		return nil, ErrBufferOverflow
	}
	view := b.Data[b.Offset : b.Offset+n]
	b.Offset += n
	return view, nil
}

// ReadUnsignedBytes is ReadBytes under its other name in ByteBuffer.ts
// (Go bytes are unsigned, so there is no separate unsigned view).
func (b *ByteBuffer) ReadUnsignedBytes(n int) ([]byte, error) {
	return b.ReadBytes(n)
}

// WriteBytes copies bytes at Offset and advances past them.
func (b *ByteBuffer) WriteBytes(p []byte) error {
	if len(p) > b.Remaining() {
		return ErrBufferOverflow
	}
	copy(b.Data[b.Offset:], p)
	b.Offset += len(p)
	return nil
}

// WriteInt writes a signed 32-bit big-endian value at Offset and
// advances past it.
func (b *ByteBuffer) WriteInt(v int32) error {
	if b.Remaining() < 4 {
		return ErrBufferOverflow
	}
	b.Data[b.Offset] = byte(v >> 24)
	b.Data[b.Offset+1] = byte(v >> 16)
	b.Data[b.Offset+2] = byte(v >> 8)
	b.Data[b.Offset+3] = byte(v)
	b.Offset += 4
	return nil
}

// SetInt writes a signed 32-bit big-endian value at an absolute
// offset without moving Offset.
func (b *ByteBuffer) SetInt(offset int, v int32) error {
	if offset < 0 || offset+4 > len(b.Data) {
		return ErrBufferOverflow
	}
	b.Data[offset] = byte(v >> 24)
	b.Data[offset+1] = byte(v >> 16)
	b.Data[offset+2] = byte(v >> 8)
	b.Data[offset+3] = byte(v)
	return nil
}
