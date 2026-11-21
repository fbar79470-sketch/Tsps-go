package cache

import (
	"bytes"
	"testing"
)

func mustByteBuffer(t *testing.T, data []byte) *ByteBuffer {
	t.Helper()
	return NewByteBuffer(data)
}

func TestByteBufferReads(t *testing.T) {
	buf := mustByteBuffer(t, []byte{0xff, 0x7f, 0x80, 0x00, 0x01, 0x02, 0x03, 0x04, 0x12, 0x34, 0x56, 0x78})
	v, err := buf.ReadSignedByte()
	if err != nil || v != -1 {
		t.Errorf("ReadSignedByte = %d, %v", v, err)
	}
	v, err = buf.ReadUnsignedByte()
	if err != nil || v != 0x7f {
		t.Errorf("ReadUnsignedByte = %d, %v", v, err)
	}
	// Short from 0x80 0x00 = -32768 signed.
	v, err = buf.ReadShort()
	if err != nil || v != -32768 {
		t.Errorf("ReadShort = %d, %v", v, err)
	}
	// Unsigned short from 0x01 0x02 = 258.
	v, err = buf.ReadUnsignedShort()
	if err != nil || v != 258 {
		t.Errorf("ReadUnsignedShort = %d, %v", v, err)
	}
	// Medium from 0x03 0x04 0x12 = 0x030412.
	v, err = buf.ReadMedium()
	if err != nil || v != 0x030412 {
		t.Errorf("ReadMedium = %x, %v", v, err)
	}
	// Int from 0x34 0x56 0x78 + 1 missing byte -> overflow.
	if _, err := buf.ReadInt(); err == nil {
		t.Error("ReadInt past end succeeded, want error")
	}
	if buf.Remaining() != 0 {
		t.Errorf("Remaining = %d", buf.Remaining())
	}
}

func TestByteBufferIntAndMedium(t *testing.T) {
	buf := mustByteBuffer(t, []byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff})
	v, err := buf.ReadInt()
	if err != nil || v != -1 {
		t.Errorf("ReadInt = %d, %v", v, err)
	}
	v2, err := buf.ReadSignedMedium()
	if err != nil || v2 != -1 {
		t.Errorf("ReadSignedMedium(0xffffff) = %d, %v", v2, err)
	}
	buf = mustByteBuffer(t, []byte{0x80, 0x00, 0x00})
	v2, err = buf.ReadSignedMedium()
	if err != nil || v2 != -8388608 {
		t.Errorf("ReadSignedMedium(0x800000) = %d, %v", v2, err)
	}
	buf = mustByteBuffer(t, []byte{0x3f, 0x80, 0x00, 0x00})
	f, err := buf.ReadFloat32()
	if err != nil || f != 1.0 {
		t.Errorf("ReadFloat32 = %v, %v", f, err)
	}
}

func TestByteBufferSmarts(t *testing.T) {
	// Big smart, short form: 0x1234.
	buf := mustByteBuffer(t, []byte{0x12, 0x34})
	v, err := buf.ReadBigSmart()
	if err != nil || v != 0x1234 {
		t.Errorf("ReadBigSmart short = %d, %v", v, err)
	}
	// Big smart, int form: first byte negative -> int & 0x7fffffff.
	buf = mustByteBuffer(t, []byte{0xff, 0xff, 0xff, 0xff})
	v, err = buf.ReadBigSmart()
	if err != nil || v != 0x7fffffff {
		t.Errorf("ReadBigSmart int = %x, %v", v, err)
	}
	// Big smart sentinel 32767 -> -1.
	buf = mustByteBuffer(t, []byte{0x7f, 0xff})
	v, err = buf.ReadBigSmart()
	if err != nil || v != -1 {
		t.Errorf("ReadBigSmart sentinel = %d, %v", v, err)
	}
	// Unsigned smart, one byte.
	buf = mustByteBuffer(t, []byte{0x40})
	v, err = buf.ReadUnsignedSmart()
	if err != nil || v != 64 {
		t.Errorf("ReadUnsignedSmart = %d, %v", v, err)
	}
	// Unsigned smart, two bytes: 0x8000 + 100.
	buf = mustByteBuffer(t, []byte{0x80, 0x64})
	v, err = buf.ReadUnsignedSmart()
	if err != nil || v != 100 {
		t.Errorf("ReadUnsignedSmart two-byte = %d, %v", v, err)
	}
	// Unsigned smart min 1.
	buf = mustByteBuffer(t, []byte{0x00})
	v, err = buf.ReadUnsignedSmartMin1()
	if err != nil || v != -1 {
		t.Errorf("ReadUnsignedSmartMin1 = %d, %v", v, err)
	}
	// Smart2: 0x40 -> 0; 0xC000 -> 0xC000-49152 = 0.
	buf = mustByteBuffer(t, []byte{0x40})
	v, err = buf.ReadSmart2()
	if err != nil || v != 0 {
		t.Errorf("ReadSmart2 = %d, %v", v, err)
	}
	buf = mustByteBuffer(t, []byte{0xc0, 0x00})
	v, err = buf.ReadSmart2()
	if err != nil || v != 0 {
		t.Errorf("ReadSmart2 two-byte = %d, %v", v, err)
	}
	// Smart3: 32767 sentinel chain: 0xFF 0xFF (smart 32767) then 0x05.
	buf = mustByteBuffer(t, []byte{0xff, 0xff, 0x05})
	v, err = buf.ReadSmart3()
	if err != nil || v != 32767+5 {
		t.Errorf("ReadSmart3 = %d, %v", v, err)
	}
}

func TestByteBufferVarInts(t *testing.T) {
	// VarInt (big-endian groups): 300 = 0x82 0x2c.
	buf := mustByteBuffer(t, []byte{0x82, 0x2c})
	v, err := buf.ReadVarInt()
	if err != nil || v != 300 {
		t.Errorf("ReadVarInt = %d, %v", v, err)
	}
	// VarInt2 (LEB128): 300 = 0xAC 0x02.
	buf = mustByteBuffer(t, []byte{0xac, 0x02})
	v, err = buf.ReadVarInt2()
	if err != nil || v != 300 {
		t.Errorf("ReadVarInt2 = %d, %v", v, err)
	}
}

func TestByteBufferStrings(t *testing.T) {
	buf := mustByteBuffer(t, []byte{'h', 'i', 0, 'x'})
	s, err := buf.ReadString()
	if err != nil || s != "hi" {
		t.Errorf("ReadString = %q, %v", s, err)
	}
	// Latin1: byte 0xe9 becomes U+00E9.
	buf = mustByteBuffer(t, []byte{0xe9, 0x00})
	s, err = buf.ReadString()
	if err != nil || s != "é" {
		t.Errorf("ReadString latin1 = %q, %v", s, err)
	}
	// Unterminated.
	buf = mustByteBuffer(t, []byte{'a', 'b'})
	if _, err := buf.ReadString(); err == nil {
		t.Error("unterminated string read, want error")
	}
	// Null string: leading NUL means absent.
	buf = mustByteBuffer(t, []byte{0, 'a', 0})
	s, present, err := buf.ReadNullString()
	if err != nil || present || s != "" {
		t.Errorf("ReadNullString = %q, %v, %v", s, present, err)
	}
	s, present, err = buf.ReadNullString()
	if err != nil || !present || s != "a" {
		t.Errorf("ReadNullString = %q, %v, %v", s, present, err)
	}
	// Ver string: leading nonzero means absent; zero + string present.
	buf = mustByteBuffer(t, []byte{1, 0, 'b', 0})
	_, present, err = buf.ReadVerString()
	if err != nil || present {
		t.Errorf("ReadVerString nonzero = %v, %v", present, err)
	}
	s, present, err = buf.ReadVerString()
	if err != nil || !present || s != "b" {
		t.Errorf("ReadVerString = %q, %v, %v", s, present, err)
	}
}

func TestByteBufferRandomAccessAndWrites(t *testing.T) {
	buf := mustByteBuffer(t, []byte{0x01, 0x02, 0x03, 0x04, 0x05})
	v, err := buf.GetByte(0)
	if err != nil || v != 1 {
		t.Errorf("GetByte = %d, %v", v, err)
	}
	if _, err := buf.GetByte(5); err == nil {
		t.Error("GetByte out of range succeeded, want error")
	}
	v, err = buf.GetShort(1)
	if err != nil || v != 0x0203 {
		t.Errorf("GetShort = %x, %v", v, err)
	}
	iv, err := buf.GetInt(1)
	if err != nil || iv != 0x02030405 {
		t.Errorf("GetInt = %x, %v", iv, err)
	}
	if buf.Offset != 0 {
		t.Error("random access moved Offset")
	}
	peek, err := buf.Peek(2)
	if err != nil || peek != 3 {
		t.Errorf("Peek = %d, %v", peek, err)
	}
	// Writes.
	w := NewByteBufferSize(8)
	if err := w.WriteInt(-1); err != nil {
		t.Fatal(err)
	}
	if err := w.WriteBytes([]byte{1, 2}); err != nil {
		t.Fatal(err)
	}
	if err := w.SetInt(4, 0x01020304); err != nil {
		t.Fatal(err)
	}
	if want := []byte{0xff, 0xff, 0xff, 0xff, 0x01, 0x02, 0x03, 0x04}; !bytes.Equal(w.Data, want) {
		t.Errorf("buffer = %x, want %x", w.Data, want)
	}
}

func TestByteBufferWriteOverflow(t *testing.T) {
	w := NewByteBufferSize(2)
	if err := w.WriteInt(1); err == nil {
		t.Error("WriteInt overflow succeeded, want error")
	}
	if err := w.SetInt(0, 1); err == nil {
		t.Error("SetInt overflow succeeded, want error")
	}
	if err := w.WriteBytes([]byte{1, 2, 3}); err == nil {
		t.Error("WriteBytes overflow succeeded, want error")
	}
	if _, err := NewByteBuffer([]byte{1}).ReadBytes(2); err == nil {
		t.Error("ReadBytes overflow succeeded, want error")
	}
}
