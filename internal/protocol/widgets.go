package protocol

import (
	"encoding/binary"
	"sort"
)

// Widget encoders, ported from ClientProtocol.ts (TSPS server, commit
// bfd81e3). These are the encoders the login bootstrap and PacketSender's
// tab/interface-script paths use: sendTabInterface mounts tabs with
// encodeWidgetOpenSub, sendInterfaceScript runs scripts with
// encodeWidgetRunScript, and the gameframe bootstrap mounts its panes
// with the open-sub layout plus widget flags.

// ScriptArg is one client-script argument: a number, a bigint, or a
// string (the `(number | string | bigint)[]` args of encodeWidgetRunScript
// and encodeRunClientScript, ClientProtocol.ts lines 2471-2520). Build
// values with ScriptArgInt, ScriptArgLong, or ScriptArgString.
type ScriptArg struct {
	kind scriptArgKind
	num  int32
	long int64
	str  string
}

type scriptArgKind int

const (
	scriptArgString scriptArgKind = iota
	scriptArgInt
	scriptArgLong
)

// ScriptArgInt builds a number script argument, written as a tagged
// int32 (TS `arg | 0`).
func ScriptArgInt(value int) ScriptArg { return ScriptArg{kind: scriptArgInt, num: int32(value)} }

// ScriptArgLong builds a bigint script argument, written as a tagged
// int64.
func ScriptArgLong(value int64) ScriptArg { return ScriptArg{kind: scriptArgLong, long: value} }

// ScriptArgString builds a string script argument, written as a tagged
// latin1 NUL-terminated string.
func ScriptArgString(value string) ScriptArg { return ScriptArg{kind: scriptArgString, str: value} }

// scriptArgs renders script arguments (scriptArgs, ClientProtocol.ts
// lines 2506-2520): a 1-byte count, then per argument a tag byte (0
// string, 1 number, 2 bigint) plus the value.
func scriptArgs(args []ScriptArg) []byte {
	payload := []byte{byte(len(args))}
	for _, arg := range args {
		switch arg.kind {
		case scriptArgLong:
			value := make([]byte, 9)
			value[0] = 2
			binary.BigEndian.PutUint64(value[1:9], uint64(arg.long))
			payload = append(payload, value...)
		case scriptArgInt:
			value := make([]byte, 5)
			value[0] = 1
			binary.BigEndian.PutUint32(value[1:5], uint32(arg.num))
			payload = append(payload, value...)
		default:
			payload = append(payload, 0)
			payload = append(payload, latin1Bytes(arg.str)...)
		}
	}
	return payload
}

// sortedIntKeys returns a map's keys in ascending order. TS iterates
// Record<number, number> with Object.entries, which orders integer-like
// keys numerically ascending; Go map iteration is random, so the encoder
// must sort to produce the same bytes.
func sortedIntKeys[V any](values map[int]V) []int {
	keys := make([]int, 0, len(values))
	for k := range values {
		keys = append(keys, k)
	}
	sort.Ints(keys)
	return keys
}

// intPairs renders id->value pairs (intPairs, ClientProtocol.ts lines
// 2255-2271): a 1-byte count, then per entry the key (u16 when shortKeys,
// int32 otherwise) plus an int32 value.
func intPairs(values map[int]int, shortKeys bool) []byte {
	keys := sortedIntKeys(values)
	size := 1 + len(keys)*8
	if shortKeys {
		size = 1 + len(keys)*6
	}
	payload := make([]byte, size)
	payload[0] = byte(len(keys))
	offset := 1
	for _, key := range keys {
		if shortKeys {
			binary.BigEndian.PutUint16(payload[offset:offset+2], uint16(key))
			offset += 2
		} else {
			binary.BigEndian.PutUint32(payload[offset:offset+4], uint32(key))
			offset += 4
		}
		binary.BigEndian.PutUint32(payload[offset:offset+4], uint32(values[key]))
		offset += 4
	}
	return payload
}

// WidgetScript is one pre/post script attached to an open-sub mount
// (the `{ scriptId, args? }` entries of encodeWidgetOpenSub,
// ClientProtocol.ts lines 2286-2300).
type WidgetScript struct {
	ScriptID int
	Args     []ScriptArg
}

// scriptList renders a list of scripts (scriptList, ClientProtocol.ts
// lines 2273-2282): a 1-byte count, then per script its int32 id plus its
// arguments.
func scriptList(scripts []WidgetScript) []byte {
	payload := []byte{byte(len(scripts))}
	for _, script := range scripts {
		id := make([]byte, 4)
		binary.BigEndian.PutUint32(id, uint32(script.ScriptID))
		payload = append(payload, id...)
		payload = append(payload, scriptArgs(script.Args)...)
	}
	return payload
}

// EncodeWidgetOpen opens a widget group (encodeWidgetOpen,
// ClientProtocol.ts lines 2238-2243): WIDGET_OPEN (opcode 100, fixed
// length 3), group id plus a modal byte. TS defaults modal to true; the
// caller passes it explicitly here.
func EncodeWidgetOpen(groupID int, modal bool) []byte {
	payload := make([]byte, 3)
	binary.BigEndian.PutUint16(payload[0:2], uint16(groupID))
	if modal {
		payload[2] = 1
	}
	return mustEncode(ServerWidgetOpen, payload)
}

// EncodeWidgetClose closes a widget group (encodeWidgetClose,
// ClientProtocol.ts lines 2245-2249): WIDGET_CLOSE (opcode 101, fixed
// length 2).
func EncodeWidgetClose(groupID int) []byte {
	payload := make([]byte, 2)
	binary.BigEndian.PutUint16(payload[0:2], uint16(groupID))
	return mustEncode(ServerWidgetClose, payload)
}

// EncodeWidgetSetRoot sets the root widget group (encodeWidgetSetRoot,
// ClientProtocol.ts lines 2251-2255): WIDGET_SET_ROOT (opcode 102, fixed
// length 2).
func EncodeWidgetSetRoot(groupID int) []byte {
	payload := make([]byte, 2)
	binary.BigEndian.PutUint16(payload[0:2], uint16(groupID))
	return mustEncode(ServerWidgetSetRoot, payload)
}

// WidgetOpenSubOptions carries the initial state for an open-sub mount
// (the options of encodeWidgetOpenSub, ClientProtocol.ts lines
// 2286-2300): varps/varbits to set, component uids to hide, and scripts
// to run before/after the mount. A nil map writes a zero count, matching
// an absent TS record.
type WidgetOpenSubOptions struct {
	Varps       map[int]int
	Varbits     map[int]int
	HiddenUIDs  []int
	PreScripts  []WidgetScript
	PostScripts []WidgetScript
}

// EncodeWidgetOpenSub mounts a widget group into a parent component
// (encodeWidgetOpenSub, ClientProtocol.ts lines 2286-2311): WIDGET_OPEN_SUB
// (opcode 103, 2-byte length prefix). targetUID is the (group << 16) |
// child uid; typ defaults to 1 in TS and is passed explicitly here.
// This is the encoder sendTabInterface uses for every tab mount.
func EncodeWidgetOpenSub(targetUID, groupID, typ int, options WidgetOpenSubOptions) []byte {
	header := make([]byte, 7)
	binary.BigEndian.PutUint32(header[0:4], uint32(targetUID))
	binary.BigEndian.PutUint16(header[4:6], uint16(groupID))
	header[6] = byte(typ)
	hidden := make([]byte, 1+len(options.HiddenUIDs)*4)
	hidden[0] = byte(len(options.HiddenUIDs))
	for i, uid := range options.HiddenUIDs {
		binary.BigEndian.PutUint32(hidden[1+i*4:], uint32(uid))
	}
	payload := header
	payload = append(payload, intPairs(options.Varps, true)...)
	payload = append(payload, intPairs(options.Varbits, true)...)
	payload = append(payload, hidden...)
	payload = append(payload, scriptList(options.PreScripts)...)
	payload = append(payload, scriptList(options.PostScripts)...)
	return mustEncode(ServerWidgetOpenSub, payload)
}

// widgetUIDPacket renders a uid-only widget packet (widgetIntPacket
// without a value, ClientProtocol.ts lines 2313-2318).
func widgetUIDPacket(opcode byte, uid int) []byte {
	payload := make([]byte, 4)
	binary.BigEndian.PutUint32(payload[0:4], uint32(uid))
	return mustEncode(opcode, payload)
}

// widgetUIDValuePacket renders a uid-plus-value widget packet
// (widgetIntPacket with a value, ClientProtocol.ts lines 2313-2318).
func widgetUIDValuePacket(opcode byte, uid, value int) []byte {
	payload := make([]byte, 8)
	binary.BigEndian.PutUint32(payload[0:4], uint32(uid))
	binary.BigEndian.PutUint32(payload[4:8], uint32(value))
	return mustEncode(opcode, payload)
}

// EncodeWidgetCloseSub closes a mounted sub-widget (encodeWidgetCloseSub,
// ClientProtocol.ts lines 2317-2319): WIDGET_CLOSE_SUB (opcode 104, fixed
// length 4).
func EncodeWidgetCloseSub(uid int) []byte {
	return widgetUIDPacket(ServerWidgetCloseSub, uid)
}

// EncodeWidgetSetText sets a component's text (encodeWidgetSetText,
// ClientProtocol.ts lines 2325-2331): WIDGET_SET_TEXT (opcode 105, 2-byte
// length prefix). A nil serverOwned omits the trailing ownership byte,
// matching an undefined TS argument; a non-nil value writes 1 or 0.
func EncodeWidgetSetText(uid int, text string, serverOwned *bool) []byte {
	payload := make([]byte, 4)
	binary.BigEndian.PutUint32(payload[0:4], uint32(uid))
	payload = append(payload, latin1Bytes(text)...)
	if serverOwned != nil {
		if *serverOwned {
			payload = append(payload, 1)
		} else {
			payload = append(payload, 0)
		}
	}
	return mustEncode(ServerWidgetSetText, payload)
}

// EncodeWidgetSetHidden shows or hides a component (encodeWidgetSetHidden,
// ClientProtocol.ts lines 2333-2337): WIDGET_SET_HIDDEN (opcode 106,
// fixed length 5).
func EncodeWidgetSetHidden(uid int, hidden bool) []byte {
	payload := make([]byte, 5)
	binary.BigEndian.PutUint32(payload[0:4], uint32(uid))
	if hidden {
		payload[4] = 1
	}
	return mustEncode(ServerWidgetSetHidden, payload)
}

// EncodeWidgetSetModel sets a raw cache model on a component, preserving
// its camera and zoom (encodeWidgetSetModel, ClientProtocol.ts lines
// 2341-2346): WIDGET_SET_MODEL (opcode 117, fixed length 8).
func EncodeWidgetSetModel(uid, modelID int) []byte {
	return widgetUIDValuePacket(ServerWidgetSetModel, uid, modelID)
}

// EncodeWidgetSetColour sets a text or rectangle component's colour as
// 15-bit RGB (encodeWidgetSetColour, ClientProtocol.ts lines 2352-2357):
// WIDGET_SET_COLOUR (opcode 119, fixed length 6). The spelling follows
// the TS name.
func EncodeWidgetSetColour(uid, colour int) []byte {
	payload := make([]byte, 6)
	binary.BigEndian.PutUint32(payload[0:4], uint32(uid))
	binary.BigEndian.PutUint16(payload[4:6], uint16(colour&0x7fff))
	return mustEncode(ServerWidgetSetColour, payload)
}

// EncodeWidgetSetPosition moves a component within its parent
// (encodeWidgetSetPosition, ClientProtocol.ts lines 2365-2371):
// WIDGET_SET_POSITION (opcode 118, fixed length 8).
func EncodeWidgetSetPosition(uid, x, y int) []byte {
	payload := make([]byte, 8)
	binary.BigEndian.PutUint32(payload[0:4], uint32(uid))
	binary.BigEndian.PutUint16(payload[4:6], uint16(int16(x)))
	binary.BigEndian.PutUint16(payload[6:8], uint16(int16(y)))
	return mustEncode(ServerWidgetSetPosition, payload)
}

// EncodeWidgetSetAngle sets a model component's camera angles and zoom
// (encodeWidgetSetAngle, ClientProtocol.ts lines 2374-2381):
// WIDGET_SET_ANGLE (opcode 113, fixed length 10).
func EncodeWidgetSetAngle(uid, angleX, angleY, zoom int) []byte {
	payload := make([]byte, 10)
	binary.BigEndian.PutUint32(payload[0:4], uint32(uid))
	binary.BigEndian.PutUint16(payload[4:6], uint16(angleX))
	binary.BigEndian.PutUint16(payload[6:8], uint16(angleY))
	binary.BigEndian.PutUint16(payload[8:10], uint16(zoom))
	return mustEncode(ServerWidgetSetAngle, payload)
}

// EncodeWidgetSetRotateSpeed sets a model component's rotation speeds
// (encodeWidgetSetRotateSpeed, ClientProtocol.ts lines 2388-2393):
// WIDGET_SET_ROTATE_SPEED (opcode 116, fixed length 8). The speeds are
// one packed int: the y (yaw) speed in the high short, the x speed in
// the low short, both signed.
func EncodeWidgetSetRotateSpeed(uid, speedX, speedY int) []byte {
	payload := make([]byte, 8)
	binary.BigEndian.PutUint32(payload[0:4], uint32(uid))
	binary.BigEndian.PutUint32(payload[4:8], uint32(((speedY&0xffff)<<16)|((speedX)&0xffff)))
	return mustEncode(ServerWidgetSetRotateSpeed, payload)
}

// EncodeWidgetSetItem sets a component's item (encodeWidgetSetItem,
// ClientProtocol.ts lines 2395-2403): WIDGET_SET_ITEM (opcode 107, fixed
// length 10). TS defaults quantity to 1; the caller passes it explicitly
// here. Item ids wrap into the u16 field instead of throwing, as in TS.
func EncodeWidgetSetItem(uid, itemID, quantity int) []byte {
	payload := make([]byte, 10)
	binary.BigEndian.PutUint32(payload[0:4], uint32(uid))
	binary.BigEndian.PutUint16(payload[4:6], uint16(itemID))
	binary.BigEndian.PutUint32(payload[6:10], uint32(quantity))
	return mustEncode(ServerWidgetSetItem, payload)
}

// EncodeWidgetSetNpcHead sets a component's NPC head (encodeWidgetSetNpcHead,
// ClientProtocol.ts lines 2405-2410): WIDGET_SET_NPC_HEAD (opcode 108,
// fixed length 6).
func EncodeWidgetSetNpcHead(uid, npcID int) []byte {
	payload := make([]byte, 6)
	binary.BigEndian.PutUint32(payload[0:4], uint32(uid))
	binary.BigEndian.PutUint16(payload[4:6], uint16(npcID))
	return mustEncode(ServerWidgetSetNpcHead, payload)
}

// EncodeWidgetSetPlayerHead sets a component's player head
// (encodeWidgetSetPlayerHead, ClientProtocol.ts lines 2412-2414):
// WIDGET_SET_PLAYER_HEAD (opcode 115, fixed length 4).
func EncodeWidgetSetPlayerHead(uid int) []byte {
	return widgetUIDPacket(ServerWidgetSetPlayerHead, uid)
}

// EncodeWidgetSetFlags sets a component's option flags
// (encodeWidgetSetFlags, ClientProtocol.ts lines 2416-2418):
// WIDGET_SET_FLAGS (opcode 111, fixed length 8).
func EncodeWidgetSetFlags(uid, flags int) []byte {
	return widgetUIDValuePacket(ServerWidgetSetFlags, uid, flags)
}

// EncodeWidgetSetFlagsRange sets option flags over a slot range
// (encodeWidgetSetFlagsRange, ClientProtocol.ts lines 2420-2428):
// WIDGET_SET_FLAGS_RANGE (opcode 109, fixed length 12).
func EncodeWidgetSetFlagsRange(uid, fromSlot, toSlot, flags int) []byte {
	payload := make([]byte, 12)
	binary.BigEndian.PutUint32(payload[0:4], uint32(uid))
	binary.BigEndian.PutUint16(payload[4:6], uint16(fromSlot))
	binary.BigEndian.PutUint16(payload[6:8], uint16(toSlot))
	binary.BigEndian.PutUint32(payload[8:12], uint32(flags))
	return mustEncode(ServerWidgetSetFlagsRange, payload)
}

// EncodeWidgetSetFlagsRangeV2 sets option flags plus a transmit mask
// over a slot range (encodeWidgetSetFlagsRangeV2, ClientProtocol.ts lines
// 2430-2438): WIDGET_SET_FLAGS_RANGE_V2 (opcode 112, fixed length 16).
// ops is the transmit mask for ops 1-32 (bit N for op N+1).
func EncodeWidgetSetFlagsRangeV2(uid, fromSlot, toSlot, flags, ops int) []byte {
	payload := make([]byte, 16)
	binary.BigEndian.PutUint32(payload[0:4], uint32(uid))
	binary.BigEndian.PutUint16(payload[4:6], uint16(fromSlot))
	binary.BigEndian.PutUint16(payload[6:8], uint16(toSlot))
	binary.BigEndian.PutUint32(payload[8:12], uint32(flags))
	binary.BigEndian.PutUint32(payload[12:16], uint32(ops))
	return mustEncode(ServerWidgetSetFlagsRangeV2, payload)
}

// EncodeWidgetSetAnimation sets a component's animation
// (encodeWidgetSetAnimation, ClientProtocol.ts lines 2440-2445):
// WIDGET_SET_ANIMATION (opcode 114, fixed length 6). Deviation: TS
// writeInt16BE throws outside int16 range; Go wraps the value into the
// field. Real animation ids fit, so the bytes agree for valid input.
func EncodeWidgetSetAnimation(uid, animationID int) []byte {
	payload := make([]byte, 6)
	binary.BigEndian.PutUint32(payload[0:4], uint32(uid))
	binary.BigEndian.PutUint16(payload[4:6], uint16(int16(animationID)))
	return mustEncode(ServerWidgetSetAnimation, payload)
}

// ScriptInventorySnapshot is one inventory carried by a run-script packet
// (ScriptInventorySnapshot, ClientProtocol.ts lines 2447-2450).
type ScriptInventorySnapshot struct {
	Capacity int
	Slots    []ItemSlot
}

// scriptInventories renders the inventories section of a run-script
// packet (scriptInventories, ClientProtocol.ts lines 2452-2470). A nil
// map writes nothing at all; a non-nil (even empty) map writes a count
// byte, matching the TS `if (!inventories)` check.
func scriptInventories(inventories map[int]ScriptInventorySnapshot) []byte {
	if inventories == nil {
		return []byte{}
	}
	ids := sortedIntKeys(inventories)
	payload := []byte{byte(len(ids))}
	for _, id := range ids {
		inventory := inventories[id]
		header := make([]byte, 6)
		binary.BigEndian.PutUint16(header[0:2], uint16(id))
		binary.BigEndian.PutUint16(header[2:4], uint16(inventory.Capacity))
		binary.BigEndian.PutUint16(header[4:6], uint16(len(inventory.Slots)))
		payload = append(payload, header...)
		for _, slot := range inventory.Slots {
			payload = append(payload, encodeItemSlot(slot.Slot, slot.ItemID, slot.Quantity)...)
		}
	}
	return payload
}

// EncodeWidgetRunScript runs a client script (encodeWidgetRunScript,
// ClientProtocol.ts lines 2471-2496): WIDGET_RUN_SCRIPT (opcode 110,
// 2-byte length prefix). The payload is the script id, the arguments, the
// varp/varbit pairs (each headed by a u16 count, unlike the u16-keyed
// pairs of EncodeWidgetOpenSub), and the script inventories. This is the
// encoder sendInterfaceScript uses; script id -1 with inventories and no
// arguments sends whole inventories by cache id.
func EncodeWidgetRunScript(scriptID int, args []ScriptArg, varps, varbits map[int]int, inventories map[int]ScriptInventorySnapshot) []byte {
	payload := make([]byte, 4)
	binary.BigEndian.PutUint32(payload[0:4], uint32(scriptID))
	payload = append(payload, scriptArgs(args)...)
	vars := func(values map[int]int) []byte {
		pairs := intPairs(values, false)
		count := make([]byte, 2)
		binary.BigEndian.PutUint16(count, uint16(pairs[0]))
		return append(count, pairs[1:]...)
	}
	payload = append(payload, vars(varps)...)
	payload = append(payload, vars(varbits)...)
	payload = append(payload, scriptInventories(inventories)...)
	return mustEncode(ServerWidgetRunScript, payload)
}

// EncodeRunClientScript runs a client script by its u16 id
// (encodeRunClientScript, ClientProtocol.ts lines 2498-2503):
// RUN_CLIENT_SCRIPT (opcode 170, 2-byte length prefix).
func EncodeRunClientScript(scriptID int, args []ScriptArg) []byte {
	payload := make([]byte, 2)
	binary.BigEndian.PutUint16(payload[0:2], uint16(scriptID))
	payload = append(payload, scriptArgs(args)...)
	return mustEncode(ServerRunClientScript, payload)
}
