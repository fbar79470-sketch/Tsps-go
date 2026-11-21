package protocol

import (
	"encoding/binary"
	"fmt"
)

// Camera encoders: all 11 encodeCamera* functions
// (ClientProtocol.ts:1602-1816), opcode CAMERA_CONTROL (137,
// ServerPackets.ts:117), length-table -1 (1-byte length
// prefix), each payload led by a discriminator byte.
//
// Liveness: every encoder is reached via sendClientPacket
// from a PacketSender sender (PacketSender.ts:1111-1207) —
// the LIVE queued path. See internal/session/camera_sender.go.
// The cameraState tracking in PacketSender (follow/target/
// controlled, PacketSender.ts:185-199) has no Go equivalent:
// the Session struct cannot gain the field in this phase (see
// the session sender file); the frames below are complete
// without it.
//
// Validation mirrors TS exactly: the functions that throw
// RangeError in TS return an error here (the EncodeLocMerge
// precedent, loc.go); the functions that only mask values
// return bare bytes. Error messages keep the TS shape
// ("Invalid camera <name>: <value>").

// CameraEasing is one CAMERA_EASINGS entry
// (ClientProtocol.ts:1613-1614): the client decodes the same
// order, so the wire id is the index in cameraEasings.
type CameraEasing string

const (
	CameraEasingLinear        CameraEasing = "linear"
	CameraEasingEaseInOutSine CameraEasing = "ease_in_out_sine"
	CameraEasingEaseOutCubic  CameraEasing = "ease_out_cubic"
	CameraEasingEaseOutSine   CameraEasing = "ease_out_sine"
	CameraEasingEaseInSine    CameraEasing = "ease_in_sine"
)

// cameraEasings is CAMERA_EASINGS in wire order.
var cameraEasings = []CameraEasing{
	CameraEasingLinear,
	CameraEasingEaseInOutSine,
	CameraEasingEaseOutCubic,
	CameraEasingEaseOutSine,
	CameraEasingEaseInSine,
}

func cameraEasingIndex(easing CameraEasing) (int, bool) {
	for i, e := range cameraEasings {
		if e == easing {
			return i, true
		}
	}
	return 0, false
}

// EncodeCameraShake encodes a camera shake
// (encodeCameraShake, ClientProtocol.ts:1602-1610):
// discriminator 3, slot byte, random amplitude u16, sine
// amplitude u16, sine frequency u16. TS only masks; no
// validation.
func EncodeCameraShake(slot, randomAmplitude, sineAmplitude, sineFrequency int) []byte {
	payload := make([]byte, 8)
	payload[0] = 3
	payload[1] = byte(slot & 0xff)
	binary.BigEndian.PutUint16(payload[2:4], uint16(randomAmplitude&0xffff))
	binary.BigEndian.PutUint16(payload[4:6], uint16(sineAmplitude&0xffff))
	binary.BigEndian.PutUint16(payload[6:8], uint16(sineFrequency&0xffff))
	return mustEncode(ServerCameraControl, payload)
}

// CameraMotionKind selects the CameraMovementOptions form
// (ClientProtocol.ts:1616-1624): the boolean form (a bare
// true/false), the rate form ({rate, rate2}), or the timed
// form ({cycles, easing}).
type CameraMotionKind int

const (
	// CameraMotionInstant is the boolean form: an 8-byte
	// payload whose last byte is 1 (moving) or 0.
	CameraMotionInstant CameraMotionKind = iota
	// CameraMotionRate is the {rate, rate2} form: a 10-byte
	// payload, discriminator mode+3.
	CameraMotionRate
	// CameraMotionTimed is the {cycles, easing} form: an
	// 11-byte payload, discriminator mode+7 (or the
	// override for rotate-to-coordinate).
	CameraMotionTimed
)

// CameraMotion is one motion argument to EncodeCameraMoveTo /
// EncodeCameraLookAt: the boolean | CameraMovementOptions
// union of encodeCameraPosition (ClientProtocol.ts:1626).
// Only the fields for Kind are read; the constructors below
// build each form. TS validates the captured (rate/timed)
// forms and throws on out-of-range values; the instant form
// is unvalidated.
type CameraMotion struct {
	Kind           CameraMotionKind
	Instant        bool // Kind == CameraMotionInstant: the boolean value.
	Rate, Rate2    int  // Kind == CameraMotionRate: 0..255 each.
	HeightRelative bool // Rate and timed forms: flag bit 0.
	Cycles         int  // Kind == CameraMotionTimed: 1..65535.
	Easing         CameraEasing
	IgnoreTerrain  bool // Timed form: flag bit 1.
	TrackTarget    bool // Timed form: flag bit 2.
}

// CameraMotionBool builds the boolean (instant) motion form.
func CameraMotionBool(moving bool) CameraMotion {
	return CameraMotion{Kind: CameraMotionInstant, Instant: moving}
}

// CameraMotionRates builds the {rate, rate2} motion form.
func CameraMotionRates(rate, rate2 int, heightRelative bool) CameraMotion {
	return CameraMotion{Kind: CameraMotionRate, Rate: rate, Rate2: rate2, HeightRelative: heightRelative}
}

// CameraMotionCycles builds the {cycles, easing} motion form.
func CameraMotionCycles(cycles int, easing CameraEasing, heightRelative, ignoreTerrain, trackTarget bool) CameraMotion {
	return CameraMotion{Kind: CameraMotionTimed, Cycles: cycles, Easing: easing, HeightRelative: heightRelative, IgnoreTerrain: ignoreTerrain, TrackTarget: trackTarget}
}

// encodeCameraPosition is the shared moveTo/lookAt/
// rotate-to-coordinate encoder (encodeCameraPosition,
// ClientProtocol.ts:1626-1660). mode is the instant
// discriminator (1 moveTo, 2 lookAt); timedMode overrides
// the timed discriminator (mode+7 by default; 13 for
// rotate-to-coordinate). It validates exactly as TS does
// for captured motions and returns the TS-shaped error.
func encodeCameraPosition(mode, x, y, height int, motion CameraMotion, timedMode int) ([]byte, error) {
	captured := motion.Kind != CameraMotionInstant
	timed := motion.Kind == CameraMotionTimed
	if captured {
		checks := []struct {
			name       string
			value, min int
			max        int
		}{
			{"x", x, 0, 65535},
			{"y", y, 0, 65535},
			{"height", height, -32768, 32767},
		}
		if timed {
			checks = append(checks, struct {
				name       string
				value, min int
				max        int
			}{"cycles", motion.Cycles, 1, 65535})
		} else {
			checks = append(checks,
				struct {
					name       string
					value, min int
					max        int
				}{"rate", motion.Rate, 0, 255},
				struct {
					name       string
					value, min int
					max        int
				}{"rate2", motion.Rate2, 0, 255},
			)
		}
		for _, check := range checks {
			if check.value < check.min || check.value > check.max {
				return nil, fmt.Errorf("Invalid camera %s: %d", check.name, check.value)
			}
		}
		if timed {
			if _, ok := cameraEasingIndex(motion.Easing); !ok {
				return nil, fmt.Errorf("Invalid camera easing")
			}
		}
	}
	var payload []byte
	switch {
	case timed:
		payload = make([]byte, 11)
		payload[0] = byte(timedMode)
	case captured:
		payload = make([]byte, 10)
		payload[0] = byte(mode + 3)
	default:
		payload = make([]byte, 8)
		payload[0] = byte(mode)
	}
	binary.BigEndian.PutUint16(payload[1:3], uint16(x&0xffff))
	binary.BigEndian.PutUint16(payload[3:5], uint16(y&0xffff))
	binary.BigEndian.PutUint16(payload[5:7], uint16(height&0xffff))
	switch {
	case timed:
		binary.BigEndian.PutUint16(payload[7:9], uint16(motion.Cycles))
		easingIndex, _ := cameraEasingIndex(motion.Easing)
		payload[9] = byte(easingIndex)
		flags := 0
		if motion.HeightRelative {
			flags |= 1
		}
		if motion.IgnoreTerrain {
			flags |= 2
		}
		if motion.TrackTarget {
			flags |= 4
		}
		payload[10] = byte(flags)
	case captured:
		payload[7] = byte(motion.Rate)
		payload[8] = byte(motion.Rate2)
		if motion.HeightRelative {
			payload[9] = 1
		}
	default:
		if motion.Instant {
			payload[7] = 1
		}
	}
	return EncodePacket(ServerCameraControl, payload)
}

// EncodeCameraMoveTo encodes a camera move-to
// (encodeCameraMoveTo, ClientProtocol.ts:1662-1664): the
// shared position encoder with mode 1 (timed discriminator
// 8). Coordinates are absolute world tiles; height may be
// negative.
func EncodeCameraMoveTo(x, y, height int, motion CameraMotion) ([]byte, error) {
	return encodeCameraPosition(1, x, y, height, motion, 1+7)
}

// EncodeCameraLookAt encodes a camera look-at
// (encodeCameraLookAt, ClientProtocol.ts:1666-1668): the
// shared position encoder with mode 2 (timed discriminator
// 9).
func EncodeCameraLookAt(x, y, height int, motion CameraMotion) ([]byte, error) {
	return encodeCameraPosition(2, x, y, height, motion, 2+7)
}

// EncodeCameraRotateToCoordinate encodes a rotate-to-
// coordinate (encodeCameraRotateToCoordinate,
// ClientProtocol.ts:1671-1676): the shared position encoder
// with mode 2 and the timed discriminator overridden to 13
// (TS passes timedMode 13 explicitly, keeping a distinct
// wire contract from the look-at timed form). The motion is
// always the timed {cycles, easing} form with no flags.
func EncodeCameraRotateToCoordinate(x, y, height, cycles int, easing CameraEasing) ([]byte, error) {
	return encodeCameraPosition(2, x, y, height, CameraMotionCycles(cycles, easing, false, false, false), 13)
}

// EncodeCameraRotateBy encodes a rotate-by
// (encodeCameraRotateBy, ClientProtocol.ts:1679-1691):
// discriminator 10, pitch i16, yaw i16, cycles u16, easing
// byte. Pitch/yaw are 2048-units-a-turn deltas; cycles must
// be 1..65535.
func EncodeCameraRotateBy(pitch, yaw, cycles int, easing CameraEasing) ([]byte, error) {
	for _, check := range []struct {
		name       string
		value, min int
		max        int
	}{
		{"pitch", pitch, -32768, 32767},
		{"yaw", yaw, -32768, 32767},
		{"cycles", cycles, 1, 65535},
	} {
		if check.value < check.min || check.value > check.max {
			return nil, fmt.Errorf("Invalid camera %s: %d", check.name, check.value)
		}
	}
	easingIndex, ok := cameraEasingIndex(easing)
	if !ok {
		return nil, fmt.Errorf("Invalid camera easing")
	}
	payload := make([]byte, 8)
	payload[0] = 10
	binary.BigEndian.PutUint16(payload[1:3], uint16(int16(pitch)))
	binary.BigEndian.PutUint16(payload[3:5], uint16(int16(yaw)))
	binary.BigEndian.PutUint16(payload[5:7], uint16(cycles))
	payload[7] = byte(easingIndex)
	return EncodePacket(ServerCameraControl, payload)
}

// EncodeCameraMoveArc encodes a move-arc
// (encodeCameraMoveArc, ClientProtocol.ts:1697-1717):
// discriminator 11, centre x/y u16, target x/y u16, height
// i16, cycles u16, easing byte, flags byte (2 when
// ignoreTerrain, else 0 — note the flag value 2, not 1, as
// TS writes `ignoreTerrain ? 2 : 0`).
func EncodeCameraMoveArc(centreX, centreY, x, y, height, cycles int, easing CameraEasing, ignoreTerrain bool) ([]byte, error) {
	for _, check := range []struct {
		name       string
		value, min int
		max        int
	}{
		{"centre x", centreX, 0, 65535},
		{"centre y", centreY, 0, 65535},
		{"x", x, 0, 65535},
		{"y", y, 0, 65535},
		{"height", height, -32768, 32767},
		{"cycles", cycles, 1, 65535},
	} {
		if check.value < check.min || check.value > check.max {
			return nil, fmt.Errorf("Invalid camera %s: %d", check.name, check.value)
		}
	}
	easingIndex, ok := cameraEasingIndex(easing)
	if !ok {
		return nil, fmt.Errorf("Invalid camera easing")
	}
	payload := make([]byte, 15)
	payload[0] = 11
	binary.BigEndian.PutUint16(payload[1:3], uint16(centreX))
	binary.BigEndian.PutUint16(payload[3:5], uint16(centreY))
	binary.BigEndian.PutUint16(payload[5:7], uint16(x))
	binary.BigEndian.PutUint16(payload[7:9], uint16(y))
	binary.BigEndian.PutUint16(payload[9:11], uint16(int16(height)))
	binary.BigEndian.PutUint16(payload[11:13], uint16(cycles))
	payload[13] = byte(easingIndex)
	if ignoreTerrain {
		payload[14] = 2
	}
	return EncodePacket(ServerCameraControl, payload)
}

// EncodeCameraUnlock encodes a camera unlock
// (encodeCameraUnlock, ClientProtocol.ts:1719-1721):
// discriminator 6, unlock byte.
func EncodeCameraUnlock(unlock bool) []byte {
	payload := []byte{6, 0}
	if unlock {
		payload[1] = 1
	}
	return mustEncode(ServerCameraControl, payload)
}

// EncodeCameraSkybox encodes a skybox model
// (encodeCameraSkybox, ClientProtocol.ts:1727-1733):
// discriminator 14, model i32. -1 clears the skybox. The
// model must be -1..0x7fffffff (TS: safe integer in that
// range); cam_reset does not clear it.
func EncodeCameraSkybox(model int) ([]byte, error) {
	if model < -1 || model > 0x7fffffff {
		return nil, fmt.Errorf("Invalid skybox model")
	}
	payload := make([]byte, 5)
	payload[0] = 14
	binary.BigEndian.PutUint32(payload[1:5], uint32(int32(model)))
	return EncodePacket(ServerCameraControl, payload)
}

// EncodeCameraTranslate encodes a camera translation
// (encodeCameraTranslate, ClientProtocol.ts:1736-1742):
// discriminator 7, dx i32, dy i32. Both deltas must fit in
// an int32 (TS: |value| <= 0x7fffffff).
func EncodeCameraTranslate(dx, dy int) ([]byte, error) {
	if dx > 0x7fffffff || dx < -0x7fffffff || dy > 0x7fffffff || dy < -0x7fffffff {
		return nil, fmt.Errorf("Invalid camera translation")
	}
	payload := make([]byte, 9)
	payload[0] = 7
	binary.BigEndian.PutUint32(payload[1:5], uint32(int32(dx)))
	binary.BigEndian.PutUint32(payload[5:9], uint32(int32(dy)))
	return EncodePacket(ServerCameraControl, payload)
}

// CameraTargetKind selects the encodeCameraTarget union arm
// (ClientProtocol.ts:1783-1786): a tile, a player, an NPC,
// or a world entity (a boat), each by index except the tile.
type CameraTargetKind int

const (
	// CameraTargetTile follows a tile; Glide selects the
	// gliding form (sub 4) over the instant form (sub 0).
	CameraTargetTile CameraTargetKind = iota
	// CameraTargetPlayer follows a player by index (sub 1).
	CameraTargetPlayer
	// CameraTargetNPC follows an NPC by index (sub 2).
	CameraTargetNPC
	// CameraTargetWorldEntity follows a world entity by
	// index (sub 3).
	CameraTargetWorldEntity
)

// CameraTarget is one encodeCameraTarget argument
// (ClientProtocol.ts:1783-1786). Tile targets use X/Y/Level
// (Level masked to 2 bits, as TS masks with & 0x3) and the
// Glide/Cycles pair (Glide = TS `cycles !== undefined`);
// actor targets use Index. Only the fields for Kind are
// read.
type CameraTarget struct {
	Kind  CameraTargetKind
	X, Y  int
	Level int
	Glide bool
	// Cycles is the glide duration in client cycles; only
	// read when Glide is set. Negative values clamp to 0,
	// as TS writes Math.max(0, cycles).
	Cycles int
	Index  int
}

// EncodeCameraTarget encodes a camera target
// (encodeCameraTarget, ClientProtocol.ts:1783-1805):
// discriminator 12, then a sub-discriminator: 1/2/3 + index
// u16 for player/NPC/world-entity targets, or 0 (instant)
// / 4 (glide) + x u16 + y u16 + level byte for tile
// targets, plus cycles u16 when gliding. TS only masks
// values here (and throws only for a target of neither
// shape, which this typed argument cannot express), so no
// error is returned.
func EncodeCameraTarget(target CameraTarget) []byte {
	switch target.Kind {
	case CameraTargetPlayer, CameraTargetNPC, CameraTargetWorldEntity:
		payload := make([]byte, 4)
		payload[0] = 12
		switch target.Kind {
		case CameraTargetPlayer:
			payload[1] = 1
		case CameraTargetNPC:
			payload[1] = 2
		default:
			payload[1] = 3
		}
		binary.BigEndian.PutUint16(payload[2:4], uint16(target.Index&0xffff))
		return mustEncode(ServerCameraControl, payload)
	default:
		if target.Glide {
			payload := make([]byte, 9)
			payload[0] = 12
			payload[1] = 4
			binary.BigEndian.PutUint16(payload[2:4], uint16(target.X&0xffff))
			binary.BigEndian.PutUint16(payload[4:6], uint16(target.Y&0xffff))
			payload[6] = byte(target.Level & 0x3)
			cycles := target.Cycles
			if cycles < 0 {
				cycles = 0
			}
			binary.BigEndian.PutUint16(payload[7:9], uint16(cycles&0xffff))
			return mustEncode(ServerCameraControl, payload)
		}
		payload := make([]byte, 7)
		payload[0] = 12
		payload[1] = 0
		binary.BigEndian.PutUint16(payload[2:4], uint16(target.X&0xffff))
		binary.BigEndian.PutUint16(payload[4:6], uint16(target.Y&0xffff))
		payload[6] = byte(target.Level & 0x3)
		return mustEncode(ServerCameraControl, payload)
	}
}

// EncodeCameraReset encodes a camera reset
// (encodeCameraReset, ClientProtocol.ts:1807-1809):
// discriminator 0 alone. It puts the camera back on the
// player and stops any shake; it does not clear a skybox.
func EncodeCameraReset() []byte {
	return mustEncode(ServerCameraControl, []byte{0})
}
