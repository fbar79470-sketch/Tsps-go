package protocol

import "fmt"

// DecodeMoveGameclick decodes a MOVE_GAMECLICK packet payload (native
// opcode 16, fixed length 7): worldY shortAddLE, modifierFlags byteNeg,
// worldX shortAddLE, then a shortAdd target location id (ClientProtocol.ts
// lines 811-816). The target id is zero for a walk-here click; TS reads
// it and discards it (the decoded "move" message carries no target), but
// this decoder returns it so the movement layer can keep it. The payload
// must be exactly 7 bytes, as the native length table requires.
func DecodeMoveGameclick(payload []byte) (x, y, modifierFlags, targetLocID int, err error) {
	r := NewReader(payload)
	wy, err := r.ShortAddLE()
	if err != nil {
		return 0, 0, 0, 0, err
	}
	flags, err := r.ByteNeg()
	if err != nil {
		return 0, 0, 0, 0, err
	}
	wx, err := r.ShortAddLE()
	if err != nil {
		return 0, 0, 0, 0, err
	}
	target, err := r.ShortAdd()
	if err != nil {
		return 0, 0, 0, 0, err
	}
	if r.Remaining() != 0 {
		return 0, 0, 0, 0, fmt.Errorf("protocol: MOVE_GAMECLICK payload has %d trailing bytes", r.Remaining())
	}
	return int(wx), int(wy), int(flags), int(target), nil
}

// DecodeWalkFull decodes a WALK packet payload (opcode 210, fixed length
// 5): worldX short, worldY short, flags byte (ClientProtocol.ts lines
// 818-821). The flags byte splits into run = (flags & 1) != 0 and
// modifierFlags = flags >> 1, exactly as the TS branch builds the "move"
// message. DecodeWalk (decode.go) drops the flags byte; callers that
// need the run/modifier bits use this decoder. The payload must be
// exactly 5 bytes.
func DecodeWalkFull(payload []byte) (x, y int, run bool, modifierFlags int, err error) {
	r := NewReader(payload)
	wx, err := r.Short()
	if err != nil {
		return 0, 0, false, 0, err
	}
	wy, err := r.Short()
	if err != nil {
		return 0, 0, false, 0, err
	}
	flags, err := r.Byte()
	if err != nil {
		return 0, 0, false, 0, err
	}
	if r.Remaining() != 0 {
		return 0, 0, false, 0, fmt.Errorf("protocol: WALK payload has %d trailing bytes", r.Remaining())
	}
	return int(wx), int(wy), flags&1 != 0, int(flags >> 1), nil
}
