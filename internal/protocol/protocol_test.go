package protocol

import (
	"bytes"
	"errors"
	"testing"
)

func TestReaderByte(t *testing.T) {
	r := NewReader([]byte{0xAB, 0x01})
	b, err := r.Byte()
	if err != nil {
		t.Fatalf("Byte() error = %v", err)
	}
	if b != 0xAB {
		t.Fatalf("Byte() = %#x, want 0xAB", b)
	}
	if got := r.Remaining(); got != 1 {
		t.Fatalf("Remaining() = %d, want 1", got)
	}
	if _, err := r.Byte(); err != nil {
		t.Fatalf("Byte() error = %v", err)
	}
	if _, err := r.Byte(); !errors.Is(err, ErrUnexpectedEnd) {
		t.Fatalf("Byte() past end error = %v, want ErrUnexpectedEnd", err)
	}
	if got := r.Remaining(); got != 0 {
		t.Fatalf("Remaining() = %d, want 0", got)
	}
}

func TestReaderShortAndSignedShort(t *testing.T) {
	r := NewReader([]byte{0x12, 0x34})
	if got, err := r.Short(); err != nil || got != 0x1234 {
		t.Fatalf("Short() = %#x, %v, want 0x1234", got, err)
	}
	r = NewReader([]byte{0xFF})
	if _, err := r.Short(); !errors.Is(err, ErrUnexpectedEnd) {
		t.Fatalf("Short() overrun error = %v, want ErrUnexpectedEnd", err)
	}
	for _, tc := range []struct {
		data []byte
		want int16
	}{
		{[]byte{0xFF, 0xFF}, -1},
		{[]byte{0x80, 0x00}, -32768},
		{[]byte{0x7F, 0xFF}, 32767},
		{[]byte{0x00, 0x01}, 1},
	} {
		r := NewReader(tc.data)
		got, err := r.SignedShort()
		if err != nil || got != tc.want {
			t.Fatalf("SignedShort(%x) = %d, %v, want %d", tc.data, got, err, tc.want)
		}
	}
}

func TestReaderIntAndIntLE(t *testing.T) {
	r := NewReader([]byte{0x12, 0x34, 0x56, 0x78})
	if got, err := r.Int(); err != nil || got != 0x12345678 {
		t.Fatalf("Int() = %#x, %v, want 0x12345678", got, err)
	}
	r = NewReader([]byte{0xFF, 0xFF, 0xFF, 0xFF})
	if got, err := r.Int(); err != nil || got != -1 {
		t.Fatalf("Int() = %d, %v, want -1", got, err)
	}
	r = NewReader([]byte{0x78, 0x56, 0x34, 0x12})
	if got, err := r.IntLE(); err != nil || got != 0x12345678 {
		t.Fatalf("IntLE() = %#x, %v, want 0x12345678", got, err)
	}
	r = NewReader([]byte{0x01, 0x02, 0x03})
	if _, err := r.Int(); !errors.Is(err, ErrUnexpectedEnd) {
		t.Fatalf("Int() overrun error = %v, want ErrUnexpectedEnd", err)
	}
	if _, err := r.IntLE(); !errors.Is(err, ErrUnexpectedEnd) {
		t.Fatalf("IntLE() overrun error = %v, want ErrUnexpectedEnd", err)
	}
}

func TestReaderAdjustedShorts(t *testing.T) {
	// ShortAdd: low byte first with -128, then high byte.
	for _, tc := range []struct {
		data []byte
		want uint16
	}{
		{[]byte{0x80, 0x01}, 0x0100}, // low (0x80-128)=0, high 1
		{[]byte{0x00, 0x02}, 0x0280}, // low (0-128)&0xff=0x80, high 2
		{[]byte{0xFF, 0x00}, 0x007F}, // low 0x7F, high 0
	} {
		r := NewReader(tc.data)
		got, err := r.ShortAdd()
		if err != nil || got != tc.want {
			t.Fatalf("ShortAdd(%x) = %#x, %v, want %#x", tc.data, got, err, tc.want)
		}
	}
	// ShortAddLE: high byte first, low byte with -128.
	for _, tc := range []struct {
		data []byte
		want uint16
	}{
		{[]byte{0x01, 0x80}, 0x0100},
		{[]byte{0x02, 0x00}, 0x0280},
		{[]byte{0x00, 0xFF}, 0x007F},
	} {
		r := NewReader(tc.data)
		got, err := r.ShortAddLE()
		if err != nil || got != tc.want {
			t.Fatalf("ShortAddLE(%x) = %#x, %v, want %#x", tc.data, got, err, tc.want)
		}
	}
	// ShortLE: low byte first, plain.
	r := NewReader([]byte{0x34, 0x12})
	if got, err := r.ShortLE(); err != nil || got != 0x1234 {
		t.Fatalf("ShortLE() = %#x, %v, want 0x1234", got, err)
	}
	// Overrun on the second byte of an adjusted short.
	r = NewReader([]byte{0x01})
	if _, err := r.ShortAdd(); !errors.Is(err, ErrUnexpectedEnd) {
		t.Fatalf("ShortAdd() overrun error = %v, want ErrUnexpectedEnd", err)
	}
}

func TestReaderAdjustedBytes(t *testing.T) {
	for _, tc := range []struct {
		b                         byte
		wantAdd, wantSub, wantNeg byte
	}{
		{0x80, 0x00, 0x00, 0x80},
		{0x00, 0x80, 0x80, 0x00},
		{0xFF, 0x7F, 0x81, 0x01}, // Sub: (128-255)&0xff = 129
		{0x01, 0x81, 0x7F, 0xFF},
	} {
		if got, err := NewReader([]byte{tc.b}).ByteAdd(); err != nil || got != tc.wantAdd {
			t.Fatalf("ByteAdd(%#x) = %#x, %v, want %#x", tc.b, got, err, tc.wantAdd)
		}
		if got, err := NewReader([]byte{tc.b}).ByteSub(); err != nil || got != tc.wantSub {
			t.Fatalf("ByteSub(%#x) = %#x, %v, want %#x", tc.b, got, err, tc.wantSub)
		}
		if got, err := NewReader([]byte{tc.b}).ByteNeg(); err != nil || got != tc.wantNeg {
			t.Fatalf("ByteNeg(%#x) = %#x, %v, want %#x", tc.b, got, err, tc.wantNeg)
		}
	}
	if _, err := NewReader(nil).ByteAdd(); !errors.Is(err, ErrUnexpectedEnd) {
		t.Fatalf("ByteAdd() overrun error = %v, want ErrUnexpectedEnd", err)
	}
}

func TestReaderMiddleEndian(t *testing.T) {
	// IntME: (b2<<24)|(b3<<16)|(b0<<8)|b1
	r := NewReader([]byte{0x01, 0x02, 0x03, 0x04})
	if got, err := r.IntME(); err != nil || got != 0x03040102 {
		t.Fatalf("IntME() = %#x, %v, want 0x03040102", got, err)
	}
	// Sign: top byte 0xFF yields a negative int32.
	r = NewReader([]byte{0x00, 0x00, 0xFF, 0x00})
	if got, err := r.IntME(); err != nil || got != -16777216 {
		t.Fatalf("IntME() = %d, %v, want -16777216", got, err)
	}
	// IntIME: (b1<<24)|(b0<<16)|(b3<<8)|b2
	r = NewReader([]byte{0x01, 0x02, 0x03, 0x04})
	if got, err := r.IntIME(); err != nil || got != 0x02010403 {
		t.Fatalf("IntIME() = %#x, %v, want 0x02010403", got, err)
	}
	r = NewReader([]byte{0x01, 0x02, 0x03})
	if _, err := r.IntME(); !errors.Is(err, ErrUnexpectedEnd) {
		t.Fatalf("IntME() overrun error = %v, want ErrUnexpectedEnd", err)
	}
	if _, err := NewReader([]byte{0x01}).IntIME(); !errors.Is(err, ErrUnexpectedEnd) {
		t.Fatalf("IntIME() overrun error = %v, want ErrUnexpectedEnd", err)
	}
}

func TestReaderString(t *testing.T) {
	r := NewReader([]byte{'h', 'i', 0, 'x'})
	got, err := r.String()
	if err != nil || got != "hi" {
		t.Fatalf("String() = %q, %v, want %q", got, err, "hi")
	}
	if r.Remaining() != 1 {
		t.Fatalf("Remaining() = %d, want 1", r.Remaining())
	}
	// Empty string is just the terminator.
	got, err = NewReader([]byte{0}).String()
	if err != nil || got != "" {
		t.Fatalf("String() = %q, %v, want empty", got, err)
	}
	// Latin1: byte 0xE9 becomes U+00E9.
	got, err = NewReader([]byte{0xE9, 0}).String()
	if err != nil || got != "é" {
		t.Fatalf("String() = %q (%x), %v, want é", got, []byte(got), err)
	}
	// Unterminated.
	if _, err := NewReader([]byte{'a', 'b'}).String(); !errors.Is(err, ErrUnterminatedString) {
		t.Fatalf("String() unterminated error = %v, want ErrUnterminatedString", err)
	}
	if _, err := NewReader(nil).String(); !errors.Is(err, ErrUnterminatedString) {
		t.Fatalf("String() empty error = %v, want ErrUnterminatedString", err)
	}
}

func TestReaderBytes(t *testing.T) {
	r := NewReader([]byte{1, 2, 3, 4, 5})
	got, err := r.Bytes(3)
	if err != nil || !bytes.Equal(got, []byte{1, 2, 3}) {
		t.Fatalf("Bytes(3) = %v, %v, want [1 2 3]", got, err)
	}
	if r.Remaining() != 2 {
		t.Fatalf("Remaining() = %d, want 2", r.Remaining())
	}
	if _, err := r.Bytes(3); !errors.Is(err, ErrUnexpectedEnd) {
		t.Fatalf("Bytes(3) overrun error = %v, want ErrUnexpectedEnd", err)
	}
	if _, err := r.Bytes(-1); err == nil {
		t.Fatalf("Bytes(-1) error = nil, want error")
	}
}

func TestOpcodeConstants(t *testing.T) {
	if ClientLogin != 204 || NativeOpLoc1 != 96 || ServerWelcome != 0 {
		t.Fatalf("spot constants: ClientLogin=%d NativeOpLoc1=%d ServerWelcome=%d", ClientLogin, NativeOpLoc1, ServerWelcome)
	}
	if ClientHandshake != 202 || NativeAppearanceSet != 37 || ServerLoginResponse != 3 {
		t.Fatalf("spot constants: ClientHandshake=%d NativeAppearanceSet=%d ServerLoginResponse=%d", ClientHandshake, NativeAppearanceSet, ServerLoginResponse)
	}
}

func TestLengthTables(t *testing.T) {
	// Table sizes catch transcription slips (44 high-level, 63 native, 106 server).
	if got := len(clientPacketLengths); got != 44 {
		t.Fatalf("clientPacketLengths has %d entries, want 44", got)
	}
	if got := len(nativeClientPacketLengths); got != 63 {
		t.Fatalf("nativeClientPacketLengths has %d entries, want 63", got)
	}
	if got := len(serverPacketLengths); got != 106 {
		t.Fatalf("serverPacketLengths has %d entries, want 106", got)
	}
	for _, tc := range []struct {
		opcode byte
		want   int
	}{
		{ClientPing, 4},
		{ClientLogin, -2},
		{ClientWalk, 5},
		{ClientLogout, 0},
		{ClientMovementInput, 12},
		{ClientResumePausebutton, 6},
		{ClientIfButtond, 16},
		{ClientDebug, -2},
		{NativeOpLoc1, 7},
		{NativeAppearanceSet, 20}, // table value, not the stale "len=13" enum comment
		{NativeIfTriggeroplocal, -2},
		{NativeOpWorldEntity6, 2},
		{NativeOpWorldEntity1, 3},
		{NativeIfClose, 0},
		{NativeSetHeading, 1},
		{62, 6}, // RESUME_PAUSEBUTTON, present in both client tables
	} {
		got, ok := ClientPacketLength(tc.opcode)
		if !ok || got != tc.want {
			t.Fatalf("ClientPacketLength(%d) = %d, %v, want %d, true", tc.opcode, got, ok, tc.want)
		}
	}
	for _, tc := range []struct {
		opcode byte
		want   int
	}{
		{ServerWelcome, 8},
		{ServerTick, 8},
		{ServerLoginResponse, -1},
		{ServerPlayerSync, -2},
		{ServerNpcInfo, -2},
		{ServerVarpLong, 10},
		{ServerVarpSmall, 3},
		{ServerLocMerge, 18},
		{ServerPlayJingle, 5},
		{ServerPlaySong, 10},
		{ServerRunEnergy, 10},
		{ServerRebuildNormal, -2},
		{ServerTriggerOndialogabort, 0},
		{ServerHideNpcOps, 1},
		{ServerSystemUpdate, 4},
	} {
		got, ok := ServerPacketLength(tc.opcode)
		if !ok || got != tc.want {
			t.Fatalf("ServerPacketLength(%d) = %d, %v, want %d, true", tc.opcode, got, ok, tc.want)
		}
	}
	// Unknown opcodes in each direction.
	if _, ok := ClientPacketLength(150); ok {
		t.Fatalf("ClientPacketLength(150) ok, want false (150 is a server opcode)")
	}
	if _, ok := ClientPacketLength(0); ok {
		t.Fatalf("ClientPacketLength(0) ok, want false")
	}
	if _, ok := ServerPacketLength(9); ok {
		t.Fatalf("ServerPacketLength(9) ok, want false")
	}
	if _, ok := ServerPacketLength(201); ok {
		t.Fatalf("ServerPacketLength(201) ok, want false (201 is a client opcode)")
	}
}

func TestDecodeFrame(t *testing.T) {
	// Single fixed-length packet (LOGOUT, length 0).
	packets, err := DecodeFrame([]byte{ClientLogout})
	if err != nil || len(packets) != 1 || packets[0].Opcode != ClientLogout || len(packets[0].Payload) != 0 {
		t.Fatalf("DecodeFrame(logout) = %+v, %v", packets, err)
	}
	// Single -1 packet (HELLO): opcode, length byte, payload.
	packets, err = DecodeFrame([]byte{ClientHello, 2, 'A', 0})
	if err != nil || len(packets) != 1 || packets[0].Opcode != ClientHello || !bytes.Equal(packets[0].Payload, []byte{'A', 0}) {
		t.Fatalf("DecodeFrame(hello) = %+v, %v", packets, err)
	}
	// Single -2 packet (LOGIN): opcode, 2-byte BE length, payload.
	packets, err = DecodeFrame([]byte{ClientLogin, 0, 5, 'a', 0, 'b', 0, 1})
	if err != nil || len(packets) != 1 || packets[0].Opcode != ClientLogin || !bytes.Equal(packets[0].Payload, []byte{'a', 0, 'b', 0, 1}) {
		t.Fatalf("DecodeFrame(login) = %+v, %v", packets, err)
	}
	// Concatenated: LOGOUT + PING + native OPLOC1.
	frame := []byte{ClientLogout, ClientPing, 0, 0, 0, 7, NativeOpLoc1, 1, 2, 3, 4, 5, 6, 7}
	packets, err = DecodeFrame(frame)
	if err != nil || len(packets) != 3 {
		t.Fatalf("DecodeFrame(concat) = %+v, %v, want 3 packets", packets, err)
	}
	if packets[1].Opcode != ClientPing || !bytes.Equal(packets[1].Payload, []byte{0, 0, 0, 7}) {
		t.Fatalf("packet[1] = %+v, want PING [0 0 0 7]", packets[1])
	}
	if packets[2].Opcode != NativeOpLoc1 || len(packets[2].Payload) != 7 {
		t.Fatalf("packet[2] = %+v, want OPLOC1 len 7", packets[2])
	}
	// Empty frame.
	packets, err = DecodeFrame(nil)
	if err != nil || len(packets) != 0 {
		t.Fatalf("DecodeFrame(nil) = %+v, %v, want no packets", packets, err)
	}
	// Unknown opcode.
	if _, err := DecodeFrame([]byte{150}); err == nil {
		t.Fatalf("DecodeFrame(unknown opcode) error = nil, want error")
	}
	// Truncated fixed packet: PING needs 4 payload bytes, has 2.
	if _, err := DecodeFrame([]byte{ClientPing, 0, 0}); err == nil {
		t.Fatalf("DecodeFrame(truncated PING) error = nil, want error")
	}
	// Truncated -2 header: only 2 of 3 header bytes.
	if _, err := DecodeFrame([]byte{ClientLogin, 0}); err == nil {
		t.Fatalf("DecodeFrame(truncated -2 header) error = nil, want error")
	}
	// Truncated -1 header: opcode only.
	if _, err := DecodeFrame([]byte{ClientHello}); err == nil {
		t.Fatalf("DecodeFrame(truncated -1 header) error = nil, want error")
	}
	// -1 length overruns the frame.
	if _, err := DecodeFrame([]byte{ClientHello, 5, 1, 2}); err == nil {
		t.Fatalf("DecodeFrame(-1 overrun) error = nil, want error")
	}
	// Second packet in a frame truncated: exact-length assertion per packet.
	if _, err := DecodeFrame([]byte{ClientLogout, ClientPing, 0}); err == nil {
		t.Fatalf("DecodeFrame(trailing partial) error = nil, want error")
	}
	// Payload is a copy: mutating the frame must not mutate the packet.
	frame = []byte{ClientPing, 0, 0, 0, 7}
	packets, err = DecodeFrame(frame)
	if err != nil {
		t.Fatalf("DecodeFrame error = %v", err)
	}
	frame[4] = 9
	if packets[0].Payload[3] != 7 {
		t.Fatalf("packet payload aliased the frame: %v", packets[0].Payload)
	}
}

func TestEncodePacket(t *testing.T) {
	// Fixed length: opcode + payload, no prefix.
	got, err := EncodePacket(ServerWelcome, []byte{0, 0, 2, 0x58, 0, 0, 0, 0})
	if err != nil || !bytes.Equal(got, []byte{0, 0, 0, 2, 0x58, 0, 0, 0, 0}) {
		t.Fatalf("EncodePacket(WELCOME) = %v, %v", got, err)
	}
	// Fixed length mismatch.
	if _, err := EncodePacket(ServerWelcome, make([]byte, 7)); err == nil {
		t.Fatalf("EncodePacket(WELCOME, 7 bytes) error = nil, want error")
	}
	if _, err := EncodePacket(ServerWelcome, make([]byte, 9)); err == nil {
		t.Fatalf("EncodePacket(WELCOME, 9 bytes) error = nil, want error")
	}
	// -1: 1-byte length prefix.
	got, err = EncodePacket(ServerLoginResponse, []byte{'A', 'B'})
	if err != nil || !bytes.Equal(got, []byte{3, 2, 'A', 'B'}) {
		t.Fatalf("EncodePacket(LOGIN_RESPONSE) = %v, %v", got, err)
	}
	if _, err := EncodePacket(ServerLoginResponse, make([]byte, 255)); err != nil {
		t.Fatalf("EncodePacket(-1, 255 bytes) error = %v, want nil", err)
	}
	if _, err := EncodePacket(ServerLoginResponse, make([]byte, 256)); err == nil {
		t.Fatalf("EncodePacket(-1, 256 bytes) error = nil, want error")
	}
	// -2: 2-byte BE length prefix.
	got, err = EncodePacket(ServerPlayerSync, []byte{1, 2, 3})
	if err != nil || !bytes.Equal(got, []byte{20, 0, 3, 1, 2, 3}) {
		t.Fatalf("EncodePacket(PLAYER_SYNC) = %v, %v", got, err)
	}
	got, err = EncodePacket(ServerPlayerSync, make([]byte, 65535))
	if err != nil || len(got) != 65538 || got[1] != 0xFF || got[2] != 0xFF {
		t.Fatalf("EncodePacket(-2, 65535 bytes) len=%d header=%v err=%v", len(got), got[:3], err)
	}
	if _, err := EncodePacket(ServerPlayerSync, make([]byte, 65536)); err == nil {
		t.Fatalf("EncodePacket(-2, 65536 bytes) error = nil, want error")
	}
	// Unknown opcode.
	if _, err := EncodePacket(9, nil); err == nil {
		t.Fatalf("EncodePacket(9) error = nil, want error")
	}
	// Zero-length fixed packet.
	got, err = EncodePacket(ServerTriggerOndialogabort, nil)
	if err != nil || !bytes.Equal(got, []byte{123}) {
		t.Fatalf("EncodePacket(TRIGGER_ONDIALOGABORT) = %v, %v", got, err)
	}
}

func TestEncodeWelcomeAndTick(t *testing.T) {
	// tickMs=600 (0x258), serverTime=0x12345678, hand-built expected bytes.
	want := []byte{0, 0, 0, 2, 0x58, 0x12, 0x34, 0x56, 0x78}
	if got := EncodeWelcome(600, 0x12345678); !bytes.Equal(got, want) {
		t.Fatalf("EncodeWelcome = %v, want %v", got, want)
	}
	want = []byte{1, 0, 0, 0, 1, 0, 0, 0, 2}
	if got := EncodeTick(1, 2); !bytes.Equal(got, want) {
		t.Fatalf("EncodeTick = %v, want %v", got, want)
	}
	// Negative values are written as two's complement int32.
	want = []byte{0, 0xFF, 0xFF, 0xFF, 0xFF, 0x80, 0, 0, 0}
	if got := EncodeWelcome(-1, -2147483648); !bytes.Equal(got, want) {
		t.Fatalf("EncodeWelcome(negatives) = %v, want %v", got, want)
	}
}

func TestEncodeLoginResponse(t *testing.T) {
	// Success: [opcode, len, 1, -1 as int32, "" NUL, "Tyler" NUL, "abc" NUL].
	want := []byte{3, 16, 1, 0xFF, 0xFF, 0xFF, 0xFF, 0, 'T', 'y', 'l', 'e', 'r', 0, 'a', 'b', 'c', 0}
	if got := EncodeLoginResponse(true, -1, "", "Tyler", "abc"); !bytes.Equal(got, want) {
		t.Fatalf("EncodeLoginResponse(success) = %v, want %v", got, want)
	}
	// Failure: all three strings still present; message + two empty NULs.
	msg := "Invalid username or password."
	want = append([]byte{3, byte(5 + len(msg) + 1 + 1 + 1), 0, 0, 0, 0, 3}, []byte(msg)...)
	want = append(want, 0, 0, 0)
	if got := EncodeLoginResponse(false, 3, msg, "", ""); !bytes.Equal(got, want) {
		t.Fatalf("EncodeLoginResponse(failure) = %v, want %v", got, want)
	}
	// Oversize payload panics, mirroring the TS packet() throw.
	defer func() {
		if recover() == nil {
			t.Fatalf("EncodeLoginResponse(oversize) did not panic")
		}
	}()
	EncodeLoginResponse(false, 3, string(make([]byte, 300)), "", "")
}

func TestBitWriter(t *testing.T) {
	w := NewBitWriter()
	w.WriteBits(0b101, 3)
	if got := w.Bytes(); !bytes.Equal(got, []byte{0xA0}) {
		t.Fatalf("WriteBits(101,3) = %x, want a0", got)
	}
	w = NewBitWriter()
	w.WriteBits(0xFF, 8)
	if got := w.Bytes(); !bytes.Equal(got, []byte{0xFF}) {
		t.Fatalf("WriteBits(0xFF,8) = %x, want ff", got)
	}
	w = NewBitWriter()
	w.WriteBits(0xA, 4)
	w.WriteBits(0xB, 4)
	if got := w.Bytes(); !bytes.Equal(got, []byte{0xAB}) {
		t.Fatalf("WriteBits nibbles = %x, want ab", got)
	}
	// 12 bits crossing a byte boundary: 101010111100 -> AB C0.
	w = NewBitWriter()
	w.WriteBits(0xABC, 12)
	if got := w.Bytes(); !bytes.Equal(got, []byte{0xAB, 0xC0}) {
		t.Fatalf("WriteBits(0xABC,12) = %x, want abc0", got)
	}
	// AlignToByte pads with zeros and starts a fresh byte.
	w = NewBitWriter()
	w.WriteBits(1, 1)
	w.AlignToByte()
	w.WriteBits(1, 1)
	if got := w.Bytes(); !bytes.Equal(got, []byte{0x80, 0x80}) {
		t.Fatalf("align test = %x, want 8080", got)
	}
	// Full 32-bit write.
	w = NewBitWriter()
	w.WriteBits(0xDEADBEEF, 32)
	if got := w.Bytes(); !bytes.Equal(got, []byte{0xDE, 0xAD, 0xBE, 0xEF}) {
		t.Fatalf("WriteBits(32) = %x, want deadbeef", got)
	}
	// Only the low n bits are consumed.
	w = NewBitWriter()
	w.WriteBits(0x1FF, 8)
	if got := w.Bytes(); !bytes.Equal(got, []byte{0xFF}) {
		t.Fatalf("WriteBits(0x1FF,8) = %x, want ff", got)
	}
	// Empty writer yields no bytes.
	if got := NewBitWriter().Bytes(); len(got) != 0 {
		t.Fatalf("empty Bytes() = %x, want empty", got)
	}
	// Invalid counts panic, mirroring the TS RangeError.
	for _, n := range []int{0, -1, 33} {
		func() {
			defer func() {
				if recover() == nil {
					t.Fatalf("WriteBits(n=%d) did not panic", n)
				}
			}()
			NewBitWriter().WriteBits(0, n)
		}()
	}
}

func TestDecodeLogin(t *testing.T) {
	payload := []byte{'t', 'y', 'l', 'e', 'r', 0, 'p', 'w', 0, 0, 0, 0, 0xF2, 0x03}
	username, password, revision, flags, hasFlags, err := DecodeLogin(payload)
	if err != nil {
		t.Fatalf("DecodeLogin error = %v", err)
	}
	if username != "tyler" || password != "pw" || revision != 242 || flags != 3 || !hasFlags {
		t.Fatalf("DecodeLogin = %q %q %d %d %v", username, password, revision, flags, hasFlags)
	}
	// No trailing flags byte (old clients).
	payload = []byte{'a', 0, 'b', 0, 0, 0, 0, 1}
	_, _, revision, flags, hasFlags, err = DecodeLogin(payload)
	if err != nil || revision != 1 || flags != 0 || hasFlags {
		t.Fatalf("DecodeLogin(no flags) = rev %d flags %d hasFlags %v err %v", revision, flags, hasFlags, err)
	}
	// Unterminated username.
	if _, _, _, _, _, err := DecodeLogin([]byte{'a', 'b'}); err == nil {
		t.Fatalf("DecodeLogin(unterminated) error = nil, want error")
	}
	// Truncated revision.
	if _, _, _, _, _, err := DecodeLogin([]byte{'a', 0, 'b', 0, 0, 0}); err == nil {
		t.Fatalf("DecodeLogin(truncated revision) error = nil, want error")
	}
}

func TestDecodeHandshake(t *testing.T) {
	// No appearance block, both trailing bytes present.
	name, clientType, caps, err := DecodeHandshake([]byte{'T', 'y', 'l', 'e', 'r', 0, 0, 1, 1})
	if err != nil || name != "Tyler" || clientType != 1 || caps != 1 {
		t.Fatalf("DecodeHandshake = %q %d %d %v", name, clientType, caps, err)
	}
	// No trailing bytes: defaults 0.
	name, clientType, caps, err = DecodeHandshake([]byte{'A', 0, 0})
	if err != nil || name != "A" || clientType != 0 || caps != 0 {
		t.Fatalf("DecodeHandshake(minimal) = %q %d %d %v", name, clientType, caps, err)
	}
	// Only clientType present.
	_, clientType, caps, err = DecodeHandshake([]byte{'A', 0, 0, 1})
	if err != nil || clientType != 1 || caps != 0 {
		t.Fatalf("DecodeHandshake(clientType only) = %d %d %v", clientType, caps, err)
	}
	// Appearance block: flag 1, one byte, 2 bytes, 1 short, 2 shorts, then trailers.
	payload := []byte{'B', 'o', 'b', 0,
		0x01,             // hasAppearance
		0x00,             // one byte
		0x02, 0xAA, 0xBB, // 2 bytes
		0x01, 0x00, 0x05, // 1 short
		0x02, 0x00, 0x06, 0x00, 0x07, // 2 shorts
		0x01, 0x01, // clientType, capabilities
	}
	name, clientType, caps, err = DecodeHandshake(payload)
	if err != nil || name != "Bob" || clientType != 1 || caps != 1 {
		t.Fatalf("DecodeHandshake(appearance) = %q %d %d %v", name, clientType, caps, err)
	}
	// Appearance block truncated mid-run.
	payload = []byte{'B', 0, 0x01, 0x00, 0x02, 0xAA}
	if _, _, _, err := DecodeHandshake(payload); err == nil {
		t.Fatalf("DecodeHandshake(truncated appearance) error = nil, want error")
	}
	// Missing appearance flag byte.
	if _, _, _, err := DecodeHandshake([]byte{'B', 0}); err == nil {
		t.Fatalf("DecodeHandshake(missing flag) error = nil, want error")
	}
	// Capability constant from ClientProtocol.ts line 2662.
	if HandshakeCapabilityNativeClient != 0x01 {
		t.Fatalf("HandshakeCapabilityNativeClient = %#x, want 0x01", HandshakeCapabilityNativeClient)
	}
}

func TestDecodePing(t *testing.T) {
	value, err := DecodePing([]byte{0, 0, 0, 42})
	if err != nil || value != 42 {
		t.Fatalf("DecodePing = %d, %v, want 42", value, err)
	}
	value, err = DecodePing([]byte{0xFF, 0xFF, 0xFF, 0xFF})
	if err != nil || value != -1 {
		t.Fatalf("DecodePing = %d, %v, want -1", value, err)
	}
	if _, err := DecodePing([]byte{0, 0, 0}); err == nil {
		t.Fatalf("DecodePing(short) error = nil, want error")
	}
	if _, err := DecodePing([]byte{0, 0, 0, 1, 0}); err == nil {
		t.Fatalf("DecodePing(trailing) error = nil, want error")
	}
}

func TestDecodeWalk(t *testing.T) {
	// Spawn coords (3089, 3524) + flags byte 0x03 (run + modifier 1).
	x, y, err := DecodeWalk([]byte{0x0C, 0x11, 0x0D, 0xC4, 0x03})
	if err != nil || x != 3089 || y != 3524 {
		t.Fatalf("DecodeWalk = %d, %d, %v, want 3089, 3524", x, y, err)
	}
	if _, _, err := DecodeWalk([]byte{0x0C, 0x11, 0x0D, 0xC4}); err == nil {
		t.Fatalf("DecodeWalk(short) error = nil, want error")
	}
	if _, _, err := DecodeWalk([]byte{0x0C, 0x11, 0x0D, 0xC4, 0x03, 0x00}); err == nil {
		t.Fatalf("DecodeWalk(trailing) error = nil, want error")
	}
}

func TestEncodeRebuildNormal(t *testing.T) {
	keys := [][4]int32{{1, 2, 3, 4}, {0, 0, 0, 0}}
	got, err := EncodeRebuildNormal(48, 55, true, keys)
	if err != nil {
		t.Fatalf("EncodeRebuildNormal error = %v", err)
	}
	// Header: opcode 141, length 39 (7 + 2*16); payload: regionX, regionY,
	// reload byte 0 (forceReload), count 2, keys.
	want := []byte{141, 0, 39, 0, 48, 0, 55, 0, 0, 2,
		0, 0, 0, 1, 0, 0, 0, 2, 0, 0, 0, 3, 0, 0, 0, 4}
	want = append(want, make([]byte, 16)...)
	if !bytes.Equal(got, want) {
		t.Fatalf("EncodeRebuildNormal = %v, want %v", got, want)
	}
	// forceReload=false inverts the reload byte to 1.
	got, err = EncodeRebuildNormal(48, 55, false, nil)
	if err != nil {
		t.Fatalf("EncodeRebuildNormal error = %v", err)
	}
	want = []byte{141, 0, 7, 0, 48, 0, 55, 1, 0, 0}
	if !bytes.Equal(got, want) {
		t.Fatalf("EncodeRebuildNormal(no reload) = %v, want %v", got, want)
	}
	// Negative key values are two's complement int32.
	got, err = EncodeRebuildNormal(0, 0, true, [][4]int32{{-1, 0, 0, 0}})
	if err != nil || !bytes.Equal(got[3:7], []byte{0, 0, 0, 0}) || !bytes.Equal(got[10:14], []byte{0xFF, 0xFF, 0xFF, 0xFF}) {
		t.Fatalf("EncodeRebuildNormal(negative key) = %v, %v", got, err)
	}
}

func TestEncodeRebuildRegion(t *testing.T) {
	var chunks [4][13][13]int32
	for p := range chunks {
		for x := range chunks[p] {
			for y := range chunks[p][x] {
				chunks[p][x][y] = -1
			}
		}
	}
	got, err := EncodeRebuildRegion(48, 55, false, chunks, nil)
	if err != nil {
		t.Fatalf("EncodeRebuildRegion error = %v", err)
	}
	// All absent: 676 presence bits -> ceil(676/8)=85 bit bytes; payload 92.
	// Header order is regionY, forceReload, regionX, key count.
	if len(got) != 3+92 {
		t.Fatalf("EncodeRebuildRegion len = %d, want %d", len(got), 3+92)
	}
	wantHeader := []byte{140, 0, 92, 0, 55, 0, 0, 48, 0, 0}
	if !bytes.Equal(got[:len(wantHeader)], wantHeader) {
		t.Fatalf("EncodeRebuildRegion header = %v, want %v", got[:len(wantHeader)], wantHeader)
	}
	for i, b := range got[len(wantHeader):] {
		if b != 0 {
			t.Fatalf("EncodeRebuildRegion bit byte %d = %#x, want 0", i, b)
		}
	}
	// One present chunk: chunk[0][0][0]=1. Bits: presence 1 at bit offset 56
	// (payload byte 7, top bit), then 26 value bits; value bit 0 lands at bit
	// offset 82 (payload byte 10, bit 5). Total bits 702 -> 88 bit bytes.
	chunks[0][0][0] = 1
	got, err = EncodeRebuildRegion(48, 55, true, chunks, nil)
	if err != nil {
		t.Fatalf("EncodeRebuildRegion error = %v", err)
	}
	if len(got) != 3+95 {
		t.Fatalf("EncodeRebuildRegion(one chunk) len = %d, want %d", len(got), 3+95)
	}
	payload := got[3:]
	if payload[2] != 1 {
		t.Fatalf("forceReload byte = %d, want 1", payload[2])
	}
	if payload[7] != 0x80 || payload[8] != 0 || payload[9] != 0 || payload[10] != 0x20 {
		t.Fatalf("bit bytes = %x %x %x %x, want 80 00 00 20", payload[7], payload[8], payload[9], payload[10])
	}
	for i := 11; i < len(payload); i++ {
		if payload[i] != 0 {
			t.Fatalf("bit byte %d = %#x, want 0", i, payload[i])
		}
	}
	// Keys follow the bit section.
	got, err = EncodeRebuildRegion(1, 2, false, chunks, [][4]int32{{7, 8, 9, 10}})
	if err != nil {
		t.Fatalf("EncodeRebuildRegion error = %v", err)
	}
	tail := got[len(got)-16:]
	wantTail := []byte{0, 0, 0, 7, 0, 0, 0, 8, 0, 0, 0, 9, 0, 0, 0, 10}
	if !bytes.Equal(tail, wantTail) {
		t.Fatalf("key tail = %v, want %v", tail, wantTail)
	}
}
