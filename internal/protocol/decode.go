package protocol

import "fmt"

// HandshakeCapabilityNativeClient is the handshake capabilities bit sent
// only by the official-client gateway (HANDSHAKE_CAPABILITY_NATIVE_CLIENT,
// ClientProtocol.ts line 2662).
const HandshakeCapabilityNativeClient = 0x01

// DecodeLogin decodes a LOGIN packet payload (opcode 204, var-short):
// username string, password string, revision int32, and an optional
// trailing flags byte absent on old clients (ClientProtocol.ts lines
// 997-1005; flag meanings live in net/login/LoginTokens.ts). hasFlags
// reports whether the trailing byte was present. Trailing bytes after the
// flags byte are ignored, as in the TS branch, which never re-checks
// remaining.
func DecodeLogin(payload []byte) (username, password string, revision int32, flags byte, hasFlags bool, err error) {
	r := NewReader(payload)
	username, err = r.String()
	if err != nil {
		return "", "", 0, 0, false, err
	}
	password, err = r.String()
	if err != nil {
		return "", "", 0, 0, false, err
	}
	revision, err = r.Int()
	if err != nil {
		return "", "", 0, 0, false, err
	}
	if r.Remaining() > 0 {
		flags, err = r.Byte()
		if err != nil {
			return "", "", 0, 0, false, err
		}
		hasFlags = true
	}
	return username, password, revision, flags, hasFlags, nil
}

// DecodeHandshake decodes a HANDSHAKE packet payload (opcode 202,
// var-byte): a name string, an appearance block, and optional trailing
// clientType (1 = mobile) and capabilities bytes, defaulting to 0 when
// absent (ClientProtocol.ts lines 1014-1032).
//
// The appearance block skip is grounded line-by-line in that branch: one
// flag byte (0 = no block); when set, one byte, then a counted run of
// bytes, then two counted runs of shorts, each run prefixed by its count
// byte. No part of the skip length is guessed.
func DecodeHandshake(payload []byte) (name string, clientType byte, capabilities byte, err error) {
	r := NewReader(payload)
	name, err = r.String()
	if err != nil {
		return "", 0, 0, err
	}
	hasAppearance, err := r.Byte()
	if err != nil {
		return "", 0, 0, err
	}
	if hasAppearance != 0 {
		if _, err := r.Byte(); err != nil {
			return "", 0, 0, err
		}
		count, err := r.Byte()
		if err != nil {
			return "", 0, 0, err
		}
		for i := count; i > 0; i-- {
			if _, err := r.Byte(); err != nil {
				return "", 0, 0, err
			}
		}
		for run := 0; run < 2; run++ {
			count, err := r.Byte()
			if err != nil {
				return "", 0, 0, err
			}
			for i := count; i > 0; i-- {
				if _, err := r.Short(); err != nil {
					return "", 0, 0, err
				}
			}
		}
	}
	if r.Remaining() > 0 {
		clientType, err = r.Byte()
		if err != nil {
			return "", 0, 0, err
		}
	}
	if r.Remaining() > 0 {
		capabilities, err = r.Byte()
		if err != nil {
			return "", 0, 0, err
		}
	}
	return name, clientType, capabilities, nil
}

// DecodePing decodes a PING packet payload (opcode 201, fixed length 4): a
// single int32, which the TS branch reads and discards (ClientProtocol.ts
// lines 992-994, returning a bare {type:"ping"}). The raw value is returned
// here for the session layer (keepalive/RTT bookkeeping); its meaning is
// not defined by the TS decode branch. The payload must be exactly 4 bytes.
func DecodePing(payload []byte) (value int32, err error) {
	r := NewReader(payload)
	value, err = r.Int()
	if err != nil {
		return 0, err
	}
	if r.Remaining() != 0 {
		return 0, fmt.Errorf("protocol: PING payload has %d trailing bytes", r.Remaining())
	}
	return value, nil
}

// DecodeWalk decodes a WALK packet payload (opcode 210, fixed length 5):
// worldX short, worldY short, flags byte (ClientProtocol.ts lines 818-821).
// The flags byte splits in TS into run = (flags & 1) != 0 and modifierFlags
// = flags >> 1; this signature returns only the coordinates, so callers
// that need the run/modifier bits must decode the flags byte themselves
// from the payload. The payload must be exactly 5 bytes.
func DecodeWalk(payload []byte) (x, y int, err error) {
	r := NewReader(payload)
	wx, err := r.Short()
	if err != nil {
		return 0, 0, err
	}
	wy, err := r.Short()
	if err != nil {
		return 0, 0, err
	}
	if _, err := r.Byte(); err != nil {
		return 0, 0, err
	}
	if r.Remaining() != 0 {
		return 0, 0, fmt.Errorf("protocol: WALK payload has %d trailing bytes", r.Remaining())
	}
	return int(wx), int(wy), nil
}
