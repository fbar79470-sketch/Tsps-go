package protocol

import (
	"encoding/binary"
	"fmt"
)

// Loc encoders: encodeLocAddChange / encodeLocDel / encodeLocMerge /
// encodeLocAnim (ClientProtocol.ts:1533-1580), behind the
// PacketSender methods sendObject / sendObjectRemoval /
// sendObjectMerge / sendObjectAnimation (PacketSender.ts:1313-1360).
// Coordinates are ABSOLUTE world tiles, masked to 16 bits — the
// payloads carry no scene-relative offset. Scene gating happens
// server-side, in ObjectManager.onRegionChange and the
// isTileInScene check behind perform (see internal/session
// objects.go), never in these encoders.
//
// Shape/rotation packing is identical in all four encoders: one
// byte, (shape << 2) | (rotation & 3) (TS masks only the rotation;
// the Buffer byte assignment truncates the shape shift to 8 bits,
// as the Go byte conversion does).
//
// LOC_CHANGE (opcode 130) has no encoder: no encodeLocChange
// exists in ClientProtocol.ts, and nothing in TS sends it. It
// stays unported, not stubbed.
//
// Producers: sendObject/sendObjectRemoval are produced by the
// per-session object sync (internal/session objects.go). The
// merge and animation encoders below are self-contained and are
// ported with NO producer in Go: sendObjectMerge is reached only
// through Player.mergeObject (Player.ts:1112-1127), whose sole
// production caller is agility content
// (ObstacleRunner.Agility.js:313), and sendObjectAnimation only
// through GameObject.performAnimation (GameObject.ts:101-110),
// which fans out to players whose location can view the tile.
// Neither Player.mergeObject, GameObject.performAnimation, nor an
// animation system exists in Go; when one lands it sends through
// these encoders plus Session.SendPacket, and must reproduce the
// TS fan-out gating (merge: the player plus viewers who have the
// player local; animation: same private area + isViewableFrom),
// not invent its own.

// EncodeLocAddChange encodes a loc added or changed
// (encodeLocAddChange, ClientProtocol.ts:1533-1542): LOC_ADD_CHANGE
// (opcode 134, length-table -1, 1-byte length prefix): id u16,
// x u16, y u16, level byte, shape/rotation byte. This is the
// no-opFlags form: without opFlags the client shows every op
// (the opFlags form is EncodeLocAddChangeWithOpFlags).
// PacketSender.sendObject (PacketSender.ts:1313-1328) calls this
// with object.getOpFlags?.(), which no GameObject implements, so
// the sendObject path always takes this 8-byte form; only the
// sailing world-entity sync passes opFlags
// (WorldEntitySync.ts:183).
func EncodeLocAddChange(id, x, y, level, shape, rotation int) []byte {
	payload := make([]byte, 8)
	binary.BigEndian.PutUint16(payload[0:2], uint16(id&0xffff))
	binary.BigEndian.PutUint16(payload[2:4], uint16(x&0xffff))
	binary.BigEndian.PutUint16(payload[4:6], uint16(y&0xffff))
	payload[6] = byte(level)
	payload[7] = byte((shape << 2) | (rotation & 3))
	return mustEncode(ServerLocAddChange, payload)
}

// EncodeLocAddChangeWithOpFlags encodes a loc added or changed
// with the op-flags byte (encodeLocAddChange with opFlags,
// ClientProtocol.ts:1533-1542; "live's loc_add_change_v2"):
// the 8-byte layout of EncodeLocAddChange plus a ninth byte,
// opFlags masked to 8 bits, showing only the ops whose bit is
// set (bit 0 is op1). In TS only the sailing world-entity sync
// passes opFlags (WorldEntitySync.ts:183); sailing is NOT
// STARTED in Go, so this form has no production caller yet.
func EncodeLocAddChangeWithOpFlags(id, x, y, level, shape, rotation, opFlags int) []byte {
	payload := make([]byte, 9)
	binary.BigEndian.PutUint16(payload[0:2], uint16(id&0xffff))
	binary.BigEndian.PutUint16(payload[2:4], uint16(x&0xffff))
	binary.BigEndian.PutUint16(payload[4:6], uint16(y&0xffff))
	payload[6] = byte(level)
	payload[7] = byte((shape << 2) | (rotation & 3))
	payload[8] = byte(opFlags & 0xff)
	return mustEncode(ServerLocAddChange, payload)
}

// EncodeLocDel encodes a loc removal (encodeLocDel,
// ClientProtocol.ts:1544-1551): LOC_DEL (opcode 135,
// length-table -1, 1-byte length prefix): x u16, y u16, level
// byte, shape/rotation byte. No id: the client removes whatever
// stands in that tile/shape slot. PacketSender.sendObjectRemoval
// (PacketSender.ts:1330-1343) calls this; the nil/object-shape
// guards there belong to PacketSender's untyped argument, and
// have no Go equivalent — callers always pass full values.
func EncodeLocDel(x, y, level, shape, rotation int) []byte {
	payload := make([]byte, 6)
	binary.BigEndian.PutUint16(payload[0:2], uint16(x&0xffff))
	binary.BigEndian.PutUint16(payload[2:4], uint16(y&0xffff))
	payload[4] = byte(level)
	payload[5] = byte((shape << 2) | (rotation & 3))
	return mustEncode(ServerLocDel, payload)
}

// LocMerge is the temporary player/loc model attachment payload:
// LocMergePayload (LocMerge.ts:9-18). Delays (Start/End) are
// 20 ms client cycles; the bounds are signed bytes relative to
// the loc's anchor tile, and reversed bounds are valid on the
// wire (TS comment, LocMerge.ts:8).
type LocMerge struct {
	LocID       int
	X, Y        int
	Level       int
	Shape       int
	Rotation    int
	PlayerIndex int
	Start       int
	End         int
	MinX        int
	MinZ        int
	MaxX        int
	MaxZ        int
}

// EncodeLocMerge encodes a loc merge (encodeLocMerge,
// ClientProtocol.ts:1553-1569): LOC_MERGE (opcode 138, fixed
// length 18, no length prefix): locId u16, tile x u16, tile y
// u16, level byte, shape/rotation byte, playerIndex u16, start
// u16, end u16, then the four bounds as signed bytes. It
// validates exactly as validateLocMerge does (LocMerge.ts:20-34)
// and returns an error, mirroring the TS RangeError throw, with
// the TS message shape; a valid merge never fails the length
// table (18 bytes fixed). No producer exists in Go (see the
// file header).
func EncodeLocMerge(merge LocMerge) ([]byte, error) {
	integer := func(name string, value, min, max int) error {
		if value < min || value > max {
			return fmt.Errorf("loc_merge %s must be an integer in %d..%d", name, min, max)
		}
		return nil
	}
	// Field order and bounds follow validateLocMerge exactly;
	// the "z" name for tile.y is TS's (LocMerge.ts:26).
	for _, check := range []struct {
		name       string
		value, min int
		max        int
	}{
		{"locId", merge.LocID, 0, 0xffff},
		{"x", merge.X, 0, 0xffff},
		{"z", merge.Y, 0, 0xffff},
		{"level", merge.Level, 0, 3},
		{"shape", merge.Shape, 0, 31},
		{"rotation", merge.Rotation, 0, 3},
		{"playerIndex", merge.PlayerIndex, 0, 2047},
		{"start", merge.Start, 0, 0xffff},
		{"end", merge.End, merge.Start + 1, 0xffff},
		{"minX", merge.MinX, -128, 127},
		{"minZ", merge.MinZ, -128, 127},
		{"maxX", merge.MaxX, -128, 127},
		{"maxZ", merge.MaxZ, -128, 127},
	} {
		if err := integer(check.name, check.value, check.min, check.max); err != nil {
			return nil, err
		}
	}
	payload := make([]byte, 18)
	binary.BigEndian.PutUint16(payload[0:2], uint16(merge.LocID))
	binary.BigEndian.PutUint16(payload[2:4], uint16(merge.X))
	binary.BigEndian.PutUint16(payload[4:6], uint16(merge.Y))
	payload[6] = byte(merge.Level)
	payload[7] = byte((merge.Shape << 2) | merge.Rotation)
	binary.BigEndian.PutUint16(payload[8:10], uint16(merge.PlayerIndex))
	binary.BigEndian.PutUint16(payload[10:12], uint16(merge.Start))
	binary.BigEndian.PutUint16(payload[12:14], uint16(merge.End))
	payload[14] = byte(int8(merge.MinX))
	payload[15] = byte(int8(merge.MinZ))
	payload[16] = byte(int8(merge.MaxX))
	payload[17] = byte(int8(merge.MaxZ))
	return EncodePacket(ServerLocMerge, payload)
}

// EncodeLocAnim encodes a loc animation (encodeLocAnim,
// ClientProtocol.ts:1571-1580): LOC_ANIM (opcode 136, fixed
// length 10, no length prefix): id u16, x u16, y u16, level
// byte, shape/rotation byte, animationId u16. An animation id
// of -1 (PacketSender.sendObjectAnimation's fallback when the
// animation carries no id, PacketSender.ts:1352-1359) encodes
// as 0xffff. No producer exists in Go (see the file header).
func EncodeLocAnim(id, x, y, level, shape, rotation, animationID int) []byte {
	payload := make([]byte, 10)
	binary.BigEndian.PutUint16(payload[0:2], uint16(id&0xffff))
	binary.BigEndian.PutUint16(payload[2:4], uint16(x&0xffff))
	binary.BigEndian.PutUint16(payload[4:6], uint16(y&0xffff))
	payload[6] = byte(level)
	payload[7] = byte((shape << 2) | (rotation & 3))
	binary.BigEndian.PutUint16(payload[8:10], uint16(animationID&0xffff))
	return mustEncode(ServerLocAnim, payload)
}
