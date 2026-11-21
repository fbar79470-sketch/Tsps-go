package protocol

import "fmt"

// BitWriter packs bits MSB-first, ported from BitWriter.ts. It is the
// bit-level writer for player/NPC sync and rebuild payloads.
type BitWriter struct {
	buffer      []byte
	currentByte byte
	bitOffset   int // bits already written into currentByte, 0..7
}

// NewBitWriter returns an empty BitWriter.
func NewBitWriter() *BitWriter {
	return &BitWriter{}
}

// WriteBits writes the low n bits of value, most-significant bit first.
// It panics if n is not in 1..32, mirroring the TS RangeError
// (BitWriter.ts writeBits).
func (w *BitWriter) WriteBits(value uint32, n int) {
	if n <= 0 || n > 32 {
		panic(fmt.Sprintf("protocol: invalid bit count: %d", n))
	}
	remaining := n
	for remaining > 0 {
		free := 8 - w.bitOffset
		bits := free
		if remaining < bits {
			bits = remaining
		}
		shift := remaining - bits
		mask := byte(((value >> uint(shift)) & ((1 << uint(bits)) - 1)) << uint(free-bits))
		w.currentByte |= mask
		w.bitOffset += bits
		remaining -= bits
		if w.bitOffset == 8 {
			w.flush()
		}
	}
}

// AlignToByte pads the partial current byte with zero bits and flushes it.
// It is a no-op when already byte-aligned.
func (w *BitWriter) AlignToByte() {
	if w.bitOffset > 0 {
		w.flush()
	}
}

// Bytes aligns to a byte boundary (as the TS toBuffer does) and returns a
// copy of the packed bytes.
func (w *BitWriter) Bytes() []byte {
	w.AlignToByte()
	out := make([]byte, len(w.buffer))
	copy(out, w.buffer)
	return out
}

func (w *BitWriter) flush() {
	w.buffer = append(w.buffer, w.currentByte&0xff)
	w.currentByte = 0
	w.bitOffset = 0
}
