package protocol

import "fmt"

// Listener payload decoders: the decodeClientPacket branches
// (ClientProtocol.ts lines 508-1032) whose messages reach the packet
// listeners in net/packet/impl/*. Each decoder mirrors one TS branch's
// Reader calls in order, including the OSRS-style transforms. The
// listeners package dispatches on opcode and drops what these decoders
// produce; no decoder performs a game action.
//
// One decoder is the exception to the "reaches a listener file" rule:
// DecodeVarpTransmit. The varp_transmit message has no listener file —
// dispatch() handles it inline (NetworkBuilder.ts lines 1147-1168) —
// but the run-toggle branch (varpId 173) is implemented in the
// listeners package, so its decoder lives here with the rest.
//
// Length discipline follows the TS split between the two table kinds:
//   - Fixed-length opcodes: the payload must be consumed exactly, as in
//     DecodeWalkFull/DecodeMoveGameclick. DecodeFrame already guarantees
//     the table length, so a trailing-bytes error here is unreachable
//     for framed packets and only reachable for hand-built TurnPackets.
//   - Variable-length opcodes (-1 in the tables): the fields are read
//     in TS order and trailing bytes are ignored, because no TS branch
//     re-checks remaining after its reads (decodeClientPacket asserts
//     the framed length before the switch, ClientProtocol.ts lines
//     514-516, and the branches never look back). The one exception is
//     IF_TRIGGEROPLOCAL, whose branch validates its own block length
//     and is enforced here.
// A short payload errors with ErrUnexpectedEnd (or
// ErrUnterminatedString for a string field), mirroring the TS Reader
// throws. No decoder validates semantic ranges (slot bounds, index
// ranges, option names): TS validates those in the listeners and the
// NetworkBuilder helpers, not in decode.
//
// Two opcode traps are preserved exactly, not "fixed":
//   - Opcode 57 is OPNPC1 in NativeClientPackets.ts but decodes as NPC
//     option 5, and opcode 76 (OPNPC1_ALT) decodes as NPC option 1
//     (the ClientPacket enum in ClientProtocol.ts lines 9-34 is what
//     the switch uses). DecodeNpcOption maps by the switch, not by the
//     native enum names.
//   - Opcode 50 (OPNPC5) has a length-table entry but NO decode branch,
//     so TS falls through to the default "raw" message
//     (ClientProtocol.ts line 1030). It is deliberately absent from
//     every decoder here.
//
// Opcodes with no listener behind them have no decoder here even when
// a branch exists: EXAMINE_NPC (9) and EXAMINE_LOC (85) are answered
// directly in the NetworkBuilder dispatch (NetworkBuilder.ts
// lines 1284-1307), and the world-entity opcodes (220-225) reach the
// PluginManager directly, never a file in net/packet/impl/. Former
// members of that group DO have decoders now: TRADE_ACTION (180)
// decodes in trade.go, the clan/friends/private/chat-filter/report
// opcodes (181, 196-199) decode below (DecodeClanChannelAction and
// friends), because the listeners package routes them to the
// social-packet emit (phase 19) instead of leaving them
// unrecognized, and the bank opcodes (243-245) decode below
// (DecodeBankDepositInventory, DecodeBankDepositEquipment,
// DecodeBankMove), because the listeners package routes them to the
// bank seam (phase 46) instead of leaving them unrecognized.

// unsignedItemID mirrors unsignedItemId (ClientProtocol.ts lines 377-379):
// an item id written as two unsigned bytes, with 65535 (the client's -1)
// for "no item". Ids 32768-65534 are real items, so the field must not
// be read as signed.
func unsignedItemID(raw uint16) int {
	if raw == 0xffff {
		return -1
	}
	return int(raw)
}

func trailingErr(name string, r *Reader) error {
	return fmt.Errorf("protocol: %s payload has %d trailing bytes", name, r.Remaining())
}

// NpcOption is one decoded "npc_option" message (ClientProtocol.ts
// lines 304-313): the NPC's world index, the clicked option (1-5, from
// the opcode), and the click's Ctrl byte. ModifierFlags is the raw
// decoded byte; TS omits the field when it is 0 (withCtrl,
// ClientProtocol.ts lines 369-371), so 0 means "no Ctrl byte set".
type NpcOption struct {
	Index         int
	ClickType     int
	ModifierFlags int
}

// DecodeNpcOption decodes the five NPC-option opcodes (fixed length 3).
// The read order and transform differ per opcode, exactly as the
// ClientPacket.NPC_OPTION_* branches do (ClientProtocol.ts lines
// 760-779). Opcode 50 is rejected: it has no branch (see the file
// header).
func DecodeNpcOption(opcode byte, payload []byte) (NpcOption, error) {
	r := NewReader(payload)
	var out NpcOption
	var err error
	switch opcode {
	case NativeOpNpc1Alt: // 76: NPC_OPTION_1, lines 760-763.
		var mods byte
		if mods, err = r.Byte(); err == nil {
			var idx uint16
			if idx, err = r.ShortAddLE(); err == nil {
				out = NpcOption{Index: int(idx), ClickType: 1, ModifierFlags: int(mods)}
			}
		}
	case NativeOpNpc2: // 12: NPC_OPTION_2, lines 764-767.
		var idx uint16
		if idx, err = r.ShortAddLE(); err == nil {
			var mods byte
			if mods, err = r.Byte(); err == nil {
				out = NpcOption{Index: int(idx), ClickType: 2, ModifierFlags: int(mods)}
			}
		}
	case NativeOpNpc3: // 34: NPC_OPTION_3, lines 768-771.
		var idx uint16
		if idx, err = r.ShortAdd(); err == nil {
			var mods byte
			if mods, err = r.ByteNeg(); err == nil {
				out = NpcOption{Index: int(idx), ClickType: 3, ModifierFlags: int(mods)}
			}
		}
	case NativeOpNpc4: // 70: NPC_OPTION_4, lines 772-775.
		var mods byte
		if mods, err = r.ByteNeg(); err == nil {
			var idx uint16
			if idx, err = r.ShortLE(); err == nil {
				out = NpcOption{Index: int(idx), ClickType: 4, ModifierFlags: int(mods)}
			}
		}
	case NativeOpNpc1: // 57: NPC_OPTION_5, lines 776-779 (not option 1).
		var mods byte
		if mods, err = r.ByteAdd(); err == nil {
			var idx uint16
			if idx, err = r.ShortLE(); err == nil {
				out = NpcOption{Index: int(idx), ClickType: 5, ModifierFlags: int(mods)}
			}
		}
	default:
		return NpcOption{}, fmt.Errorf("protocol: unsupported npc_option opcode %d", opcode)
	}
	if err != nil {
		return NpcOption{}, err
	}
	if r.Remaining() != 0 {
		return NpcOption{}, trailingErr("NPC_OPTION", r)
	}
	return out, nil
}

// ObjectOption is one decoded "object_option" message: the object id,
// its tile, and either the clicked option number (native opcodes,
// HasClickType) or the high-level action text (LOC_INTERACT, HasAction;
// both may be absent, in which case ObjectActionPacketListener derives
// the option from the definition, which this package cannot do).
// ModifierFlags is the raw Ctrl byte for native opcodes and 0 for
// LOC_INTERACT, whose branch carries none.
type ObjectOption struct {
	ID            int
	X, Y          int
	ClickType     int
	HasClickType  bool
	Action        string
	HasAction     bool
	ModifierFlags int
}

// Examine is one decoded examine_npc / examine_object message: the
// examined definition id (ClientProtocol.ts lines 556-559). The two
// opcodes differ only in the short transform.
type Examine struct {
	ID int
}

// DecodeExamineNpc decodes EXAMINE_NPC (opcode 9, fixed length 2):
// one shortAdd id (ClientProtocol.ts line 557).
func DecodeExamineNpc(payload []byte) (Examine, error) {
	r := NewReader(payload)
	id, err := r.ShortAdd()
	if err != nil {
		return Examine{}, err
	}
	if r.Remaining() != 0 {
		return Examine{}, fmt.Errorf("protocol: EXAMINE_NPC payload has %d trailing bytes", r.Remaining())
	}
	return Examine{ID: int(id)}, nil
}

// DecodeExamineLoc decodes EXAMINE_LOC (opcode 85, fixed length 2):
// one shortAddLE id (ClientProtocol.ts line 559).
func DecodeExamineLoc(payload []byte) (Examine, error) {
	r := NewReader(payload)
	id, err := r.ShortAddLE()
	if err != nil {
		return Examine{}, err
	}
	if r.Remaining() != 0 {
		return Examine{}, fmt.Errorf("protocol: EXAMINE_LOC payload has %d trailing bytes", r.Remaining())
	}
	return Examine{ID: int(id)}, nil
}

// DecodeObjectOption decodes the five native object-option opcodes
// (fixed length 7), following the ClientPacket.OBJECT_OPTION_* branches
// (ClientProtocol.ts lines 780-810). Each branch reads x, y, id, and
// the Ctrl byte in its own order with its own transforms.
func DecodeObjectOption(opcode byte, payload []byte) (ObjectOption, error) {
	r := NewReader(payload)
	var out ObjectOption
	var err error
	var x, y, id uint16
	var mods byte
	// step runs the next read only while no earlier read failed.
	step := func(rd func() error) {
		if err == nil {
			err = rd()
		}
	}
	switch opcode {
	case NativeOpLoc1: // 96: OBJECT_OPTION_1, lines 780-786.
		step(func() error { x, err = r.ShortAdd(); return err })
		step(func() error { y, err = r.ShortLE(); return err })
		step(func() error { mods, err = r.ByteNeg(); return err })
		step(func() error { id, err = r.ShortAddLE(); return err })
		out.ClickType = 1
	case NativeOpLoc2: // 28: OBJECT_OPTION_2, lines 787-792.
		step(func() error { x, err = r.ShortAddLE(); return err })
		step(func() error { y, err = r.ShortAddLE(); return err })
		step(func() error { mods, err = r.ByteSub(); return err })
		step(func() error { id, err = r.Short(); return err })
		out.ClickType = 2
	case NativeOpLoc3: // 42: OBJECT_OPTION_3, lines 793-798.
		step(func() error { y, err = r.ShortLE(); return err })
		step(func() error { id, err = r.ShortLE(); return err })
		step(func() error { x, err = r.ShortAddLE(); return err })
		step(func() error { mods, err = r.ByteSub(); return err })
		out.ClickType = 3
	case NativeOpLoc4: // 38: OBJECT_OPTION_4, lines 799-804.
		step(func() error { x, err = r.ShortAdd(); return err })
		step(func() error { id, err = r.ShortLE(); return err })
		step(func() error { y, err = r.ShortAdd(); return err })
		step(func() error { mods, err = r.ByteNeg(); return err })
		out.ClickType = 4
	case NativeOpLoc5: // 51: OBJECT_OPTION_5, lines 805-810.
		step(func() error { x, err = r.Short(); return err })
		step(func() error { mods, err = r.ByteAdd(); return err })
		step(func() error { y, err = r.ShortAdd(); return err })
		step(func() error { id, err = r.ShortAdd(); return err })
		out.ClickType = 5
	default:
		return ObjectOption{}, fmt.Errorf("protocol: unsupported object_option opcode %d", opcode)
	}
	if err != nil {
		return ObjectOption{}, err
	}
	if r.Remaining() != 0 {
		return ObjectOption{}, trailingErr("OBJECT_OPTION", r)
	}
	out.ID, out.X, out.Y = int(id), int(x), int(y)
	out.HasClickType = true
	out.ModifierFlags = int(mods)
	return out, nil
}

// DecodeLocInteract decodes a LOC_INTERACT packet (high-level opcode
// 231, variable length): id, x, y shorts, one skipped byte, an action
// string, and a click-type byte (ClientProtocol.ts lines 840-848). An
// empty action string and a zero click-type byte both decode to
// "absent" (the TS branch's `|| undefined`). Trailing bytes are
// ignored, as in TS.
func DecodeLocInteract(payload []byte) (ObjectOption, error) {
	r := NewReader(payload)
	id, err := r.Short()
	if err != nil {
		return ObjectOption{}, err
	}
	x, err := r.Short()
	if err != nil {
		return ObjectOption{}, err
	}
	y, err := r.Short()
	if err != nil {
		return ObjectOption{}, err
	}
	if _, err := r.Byte(); err != nil { // Skipped byte; TS reads and discards it.
		return ObjectOption{}, err
	}
	action, err := r.String()
	if err != nil {
		return ObjectOption{}, err
	}
	clickType, err := r.Byte()
	if err != nil {
		return ObjectOption{}, err
	}
	out := ObjectOption{ID: int(id), X: int(x), Y: int(y)}
	if action != "" {
		out.Action, out.HasAction = action, true
	}
	if clickType != 0 {
		out.ClickType, out.HasClickType = int(clickType), true
	}
	return out, nil
}

// PlayerOption is one decoded "player_option" message: the target
// player's world index, the clicked option (1-8), and the Ctrl byte
// (0 when the branch carries none, as with INTERACT).
type PlayerOption struct {
	Index         int
	Option        int
	ModifierFlags int
}

// DecodePlayerOption decodes the eight native player-option opcodes
// (fixed length 3), following the OPPLAYER* branches
// (ClientProtocol.ts lines 570-601). Each branch reads the index and
// the Ctrl byte in its own order with its own transform.
func DecodePlayerOption(opcode byte, payload []byte) (PlayerOption, error) {
	r := NewReader(payload)
	var out PlayerOption
	var err error
	var idx uint16
	var mods byte
	switch opcode {
	case NativeOpPlayer1: // 44: OPPLAYER1, lines 570-573.
		mods, err = r.ByteSub()
		if err == nil {
			idx, err = r.Short()
		}
		out.Option = 1
	case NativeOpPlayer2: // 45: OPPLAYER2, lines 574-577.
		mods, err = r.Byte()
		if err == nil {
			idx, err = r.Short()
		}
		out.Option = 2
	case NativeOpPlayer3: // 46: OPPLAYER3, lines 578-581.
		mods, err = r.ByteSub()
		if err == nil {
			idx, err = r.Short()
		}
		out.Option = 3
	case NativeOpPlayer4: // 73: OPPLAYER4, lines 582-585.
		idx, err = r.Short()
		if err == nil {
			mods, err = r.ByteNeg()
		}
		out.Option = 4
	case NativeOpPlayer5: // 6: OPPLAYER5, lines 586-589.
		idx, err = r.ShortAddLE()
		if err == nil {
			mods, err = r.ByteSub()
		}
		out.Option = 5
	case NativeOpPlayer6: // 48: OPPLAYER6, lines 590-593.
		idx, err = r.Short()
		if err == nil {
			mods, err = r.ByteNeg()
		}
		out.Option = 6
	case NativeOpPlayer7: // 10: OPPLAYER7, lines 594-597.
		idx, err = r.ShortAdd()
		if err == nil {
			mods, err = r.ByteSub()
		}
		out.Option = 7
	case NativeOpPlayer8: // 21: OPPLAYER8, lines 598-601.
		mods, err = r.Byte()
		if err == nil {
			idx, err = r.ShortAdd()
		}
		out.Option = 8
	default:
		return PlayerOption{}, fmt.Errorf("protocol: unsupported player_option opcode %d", opcode)
	}
	if err != nil {
		return PlayerOption{}, err
	}
	if r.Remaining() != 0 {
		return PlayerOption{}, trailingErr("PLAYER_OPTION", r)
	}
	out.Index, out.ModifierFlags = int(idx), int(mods)
	return out, nil
}

// DecodeInteract decodes an INTERACT packet (high-level opcode 233,
// variable length): one byte selecting option 3 (byte 0) or option 2
// (any other byte), then the target index short
// (ClientProtocol.ts lines 906-907). Trailing bytes are ignored, as in
// TS. Option 2 is the trade request and option 3 is follow; see
// PlayerOptionPacketListener.
func DecodeInteract(payload []byte) (PlayerOption, error) {
	r := NewReader(payload)
	optionByte, err := r.Byte()
	if err != nil {
		return PlayerOption{}, err
	}
	idx, err := r.Short()
	if err != nil {
		return PlayerOption{}, err
	}
	option := 2
	if optionByte == 0 {
		option = 3
	}
	return PlayerOption{Index: int(idx), Option: option}, nil
}

// GroundItemAction is one decoded "ground_item_action" message: the
// ground item id, its tile, and how the click named the option — by
// number (native opcodes, HasOptionIndex), by name (high-level opcode
// 232 and EXAMINE_OBJ, HasOption), or both. StackID is present only
// for opcode 232, which always carries the stack's int id. For
// EXAMINE_OBJ (104) the option is always "Examine" and there is no
// option index or stack id.
type GroundItemAction struct {
	ItemID         int
	X, Y           int
	OptionIndex    int
	HasOptionIndex bool
	Option         string
	HasOption      bool
	StackID        int32
	HasStackID     bool
	ModifierFlags  int
}

// DecodeGroundItemAction decodes the native ground-item opcodes: the
// five OPOBJ options (fixed length 7, ClientProtocol.ts lines 656-676)
// and EXAMINE_OBJ (fixed length 6, lines 677-678). Each OPOBJ branch
// reads item id, x, y, and the Ctrl byte in its own order.
func DecodeGroundItemAction(opcode byte, payload []byte) (GroundItemAction, error) {
	r := NewReader(payload)
	var out GroundItemAction
	var err error
	var itemID, x, y uint16
	var mods byte
	switch opcode {
	case NativeOpObj1: // 102: OPOBJ1, lines 656-659.
		mods, err = r.ByteSub()
		if err == nil {
			y, err = r.ShortLE()
		}
		if err == nil {
			itemID, err = r.ShortAdd()
		}
		if err == nil {
			x, err = r.ShortAdd()
		}
		out.OptionIndex = 1
	case NativeOpObj2: // 43: OPOBJ2, lines 660-663.
		x, err = r.ShortAdd()
		if err == nil {
			y, err = r.ShortLE()
		}
		if err == nil {
			itemID, err = r.Short()
		}
		if err == nil {
			mods, err = r.Byte()
		}
		out.OptionIndex = 2
	case NativeOpObj3: // 103: OPOBJ3, lines 664-667.
		itemID, err = r.ShortLE()
		if err == nil {
			x, err = r.ShortAdd()
		}
		if err == nil {
			y, err = r.ShortAddLE()
		}
		if err == nil {
			mods, err = r.ByteSub()
		}
		out.OptionIndex = 3
	case NativeOpObj4: // 56: OPOBJ4, lines 668-672.
		itemID, err = r.ShortAddLE()
		if err == nil {
			y, err = r.ShortAddLE()
		}
		if err == nil {
			mods, err = r.ByteNeg()
		}
		if err == nil {
			x, err = r.ShortAdd()
		}
		out.OptionIndex = 4
	case NativeOpObj5: // 82: OPOBJ5, lines 673-676.
		itemID, err = r.ShortAdd()
		if err == nil {
			x, err = r.ShortLE()
		}
		if err == nil {
			y, err = r.ShortLE()
		}
		if err == nil {
			mods, err = r.ByteSub()
		}
		out.OptionIndex = 5
	case NativeExamineObj: // 104: EXAMINE_OBJ, lines 677-678.
		itemID, err = r.Short()
		if err == nil {
			y, err = r.ShortLE()
		}
		if err == nil {
			x, err = r.ShortLE()
		}
		out.Option, out.HasOption = "Examine", true
	default:
		return GroundItemAction{}, fmt.Errorf("protocol: unsupported ground_item_action opcode %d", opcode)
	}
	if err != nil {
		return GroundItemAction{}, err
	}
	if r.Remaining() != 0 {
		return GroundItemAction{}, trailingErr("GROUND_ITEM_ACTION", r)
	}
	out.ItemID, out.X, out.Y = int(itemID), int(x), int(y)
	out.ModifierFlags = int(mods)
	if opcode != NativeExamineObj {
		out.HasOptionIndex = true
	}
	return out, nil
}

// DecodeGroundItemActionHigh decodes a GROUND_ITEM_ACTION packet
// (high-level opcode 232, variable length): stack id int, x/y shorts,
// one skipped byte, item id short, one skipped int, an option string,
// and an option-index byte (ClientProtocol.ts lines 938-946, in the
// GROUND_ITEM_ACTION branch). An empty option string and a zero
// option-index byte both decode to "absent" (the TS branch's
// `|| undefined`). Trailing bytes are ignored, as in TS.
func DecodeGroundItemActionHigh(payload []byte) (GroundItemAction, error) {
	r := NewReader(payload)
	stackID, err := r.Int()
	if err != nil {
		return GroundItemAction{}, err
	}
	x, err := r.Short()
	if err != nil {
		return GroundItemAction{}, err
	}
	y, err := r.Short()
	if err != nil {
		return GroundItemAction{}, err
	}
	if _, err := r.Byte(); err != nil { // Skipped byte; TS reads and discards it.
		return GroundItemAction{}, err
	}
	itemID, err := r.Short()
	if err != nil {
		return GroundItemAction{}, err
	}
	if _, err := r.Int(); err != nil { // Skipped int; TS reads and discards it.
		return GroundItemAction{}, err
	}
	option, err := r.String()
	if err != nil {
		return GroundItemAction{}, err
	}
	optionIndex, err := r.Byte()
	if err != nil {
		return GroundItemAction{}, err
	}
	out := GroundItemAction{
		ItemID: int(itemID), X: int(x), Y: int(y),
		StackID: stackID, HasStackID: true,
	}
	if option != "" {
		out.Option, out.HasOption = option, true
	}
	if optionIndex != 0 {
		out.OptionIndex, out.HasOptionIndex = int(optionIndex), true
	}
	return out, nil
}

// ItemOnPlayer is one decoded "item_on_player" message
// (OPPLAYER_U, opcode 65, fixed length 11): the target player index,
// the used item id and inventory slot, the source widget id, and the
// Ctrl byte (ClientProtocol.ts lines 602-609).
type ItemOnPlayer struct {
	TargetIndex   int
	ItemID        int
	Slot          int
	WidgetID      int32
	ModifierFlags int
}

// DecodeItemOnPlayer decodes OPPLAYER_U: targetIndex shortAddLE, itemId
// shortAddLE, slot shortAdd, widgetId int, modifierFlags byteAdd.
func DecodeItemOnPlayer(payload []byte) (ItemOnPlayer, error) {
	r := NewReader(payload)
	target, err := r.ShortAddLE()
	if err != nil {
		return ItemOnPlayer{}, err
	}
	itemID, err := r.ShortAddLE()
	if err != nil {
		return ItemOnPlayer{}, err
	}
	slot, err := r.ShortAdd()
	if err != nil {
		return ItemOnPlayer{}, err
	}
	widgetID, err := r.Int()
	if err != nil {
		return ItemOnPlayer{}, err
	}
	mods, err := r.ByteAdd()
	if err != nil {
		return ItemOnPlayer{}, err
	}
	if r.Remaining() != 0 {
		return ItemOnPlayer{}, trailingErr("ITEM_ON_PLAYER", r)
	}
	return ItemOnPlayer{TargetIndex: int(target), ItemID: int(itemID), Slot: int(slot), WidgetID: widgetID, ModifierFlags: int(mods)}, nil
}

// ItemOnNpc is one decoded "item_on_npc" message (OPNPC_U, opcode 36,
// fixed length 11): the target NPC index, the used item id and
// inventory slot, the source widget id, and the Ctrl byte
// (ClientProtocol.ts lines 628-635).
type ItemOnNpc struct {
	TargetIndex   int
	ItemID        int
	Slot          int
	WidgetID      int32
	ModifierFlags int
}

// DecodeItemOnNpc decodes OPNPC_U: slot shortAddLE, modifierFlags byte,
// widgetId intME, itemId shortLE, targetIndex shortAddLE.
func DecodeItemOnNpc(payload []byte) (ItemOnNpc, error) {
	r := NewReader(payload)
	slot, err := r.ShortAddLE()
	if err != nil {
		return ItemOnNpc{}, err
	}
	mods, err := r.Byte()
	if err != nil {
		return ItemOnNpc{}, err
	}
	widgetID, err := r.IntME()
	if err != nil {
		return ItemOnNpc{}, err
	}
	itemID, err := r.ShortLE()
	if err != nil {
		return ItemOnNpc{}, err
	}
	target, err := r.ShortAddLE()
	if err != nil {
		return ItemOnNpc{}, err
	}
	if r.Remaining() != 0 {
		return ItemOnNpc{}, trailingErr("ITEM_ON_NPC", r)
	}
	return ItemOnNpc{TargetIndex: int(target), ItemID: int(itemID), Slot: int(slot), WidgetID: widgetID, ModifierFlags: int(mods)}, nil
}

// ItemOnObject is one decoded "item_on_object" message (OPLOCU, opcode
// 86, fixed length 15): the object id and tile, the used item id and
// inventory slot, the source widget id, and the Ctrl byte
// (ClientProtocol.ts lines 636-645).
type ItemOnObject struct {
	ObjectID      int
	X, Y          int
	ItemID        int
	Slot          int
	WidgetID      int32
	ModifierFlags int
}

// DecodeItemOnObject decodes OPLOCU: slot shortAddLE, objectId
// shortAdd, widgetId intLE, modifierFlags byteSub, x shortLE, y short,
// itemId shortAddLE.
func DecodeItemOnObject(payload []byte) (ItemOnObject, error) {
	r := NewReader(payload)
	slot, err := r.ShortAddLE()
	if err != nil {
		return ItemOnObject{}, err
	}
	objectID, err := r.ShortAdd()
	if err != nil {
		return ItemOnObject{}, err
	}
	widgetID, err := r.IntLE()
	if err != nil {
		return ItemOnObject{}, err
	}
	mods, err := r.ByteSub()
	if err != nil {
		return ItemOnObject{}, err
	}
	x, err := r.ShortLE()
	if err != nil {
		return ItemOnObject{}, err
	}
	y, err := r.Short()
	if err != nil {
		return ItemOnObject{}, err
	}
	itemID, err := r.ShortAddLE()
	if err != nil {
		return ItemOnObject{}, err
	}
	if r.Remaining() != 0 {
		return ItemOnObject{}, trailingErr("ITEM_ON_OBJECT", r)
	}
	return ItemOnObject{ObjectID: int(objectID), X: int(x), Y: int(y), ItemID: int(itemID), Slot: int(slot), WidgetID: widgetID, ModifierFlags: int(mods)}, nil
}

// ItemOnGround is one decoded "item_on_ground" message (OPOBJ_U,
// opcode 79, fixed length 15): the ground item id and tile, the used
// item id and inventory slot, the source widget id, and the Ctrl byte
// (ClientProtocol.ts lines 679-687).
type ItemOnGround struct {
	GroundItemID  int
	X, Y          int
	ItemID        int
	Slot          int
	WidgetID      int32
	ModifierFlags int
}

// DecodeItemOnGround decodes OPOBJ_U: groundItemId shortAddLE, y
// shortAdd, widgetId intME, x shortAdd, slot shortAddLE, modifierFlags
// byteSub, itemId shortLE (the item id is read last).
func DecodeItemOnGround(payload []byte) (ItemOnGround, error) {
	r := NewReader(payload)
	groundID, err := r.ShortAddLE()
	if err != nil {
		return ItemOnGround{}, err
	}
	y, err := r.ShortAdd()
	if err != nil {
		return ItemOnGround{}, err
	}
	widgetID, err := r.IntME()
	if err != nil {
		return ItemOnGround{}, err
	}
	x, err := r.ShortAdd()
	if err != nil {
		return ItemOnGround{}, err
	}
	slot, err := r.ShortAddLE()
	if err != nil {
		return ItemOnGround{}, err
	}
	mods, err := r.ByteSub()
	if err != nil {
		return ItemOnGround{}, err
	}
	itemID, err := r.ShortLE()
	if err != nil {
		return ItemOnGround{}, err
	}
	if r.Remaining() != 0 {
		return ItemOnGround{}, trailingErr("ITEM_ON_GROUND", r)
	}
	return ItemOnGround{GroundItemID: int(groundID), X: int(x), Y: int(y), ItemID: int(itemID), Slot: int(slot), WidgetID: widgetID, ModifierFlags: int(mods)}, nil
}

// SpellOnPlayer is one decoded "spell_on_player" message (OPPLAYER_T,
// opcode 32, fixed length 11): the target player index and the spell
// selection (widget, child, item id), plus the Ctrl byte
// (ClientProtocol.ts lines 610-619). The spell id itself is resolved
// later from the selection by NetworkBuilder.resolveClientCombatSpellId
// (NetworkBuilder.ts lines 1791-1805), which needs the combat spell
// tables and the cache; no spell id is decoded here.
type SpellOnPlayer struct {
	TargetIndex   int
	SpellWidget   int32
	SpellChild    int
	SpellItemID   int
	ModifierFlags int
}

// DecodeSpellOnPlayer decodes OPPLAYER_T: modifierFlags byteNeg,
// spellItemId shortLE, spellChild shortLE, spellWidget intIME,
// targetIndex shortLE.
func DecodeSpellOnPlayer(payload []byte) (SpellOnPlayer, error) {
	r := NewReader(payload)
	mods, err := r.ByteNeg()
	if err != nil {
		return SpellOnPlayer{}, err
	}
	spellItemID, err := r.ShortLE()
	if err != nil {
		return SpellOnPlayer{}, err
	}
	spellChild, err := r.ShortLE()
	if err != nil {
		return SpellOnPlayer{}, err
	}
	spellWidget, err := r.IntIME()
	if err != nil {
		return SpellOnPlayer{}, err
	}
	target, err := r.ShortLE()
	if err != nil {
		return SpellOnPlayer{}, err
	}
	if r.Remaining() != 0 {
		return SpellOnPlayer{}, trailingErr("SPELL_ON_PLAYER", r)
	}
	return SpellOnPlayer{TargetIndex: int(target), SpellWidget: spellWidget, SpellChild: int(spellChild), SpellItemID: int(spellItemID), ModifierFlags: int(mods)}, nil
}

// SpellOnNpc is one decoded "spell_on_npc" message (OPNPC_T, opcode
// 75, fixed length 11): the target NPC index and the spell selection,
// plus the Ctrl byte (ClientProtocol.ts lines 620-627). The spell id
// is resolved later, as with SpellOnPlayer.
type SpellOnNpc struct {
	TargetIndex   int
	SpellWidget   int32
	SpellChild    int
	SpellItemID   int
	ModifierFlags int
}

// DecodeSpellOnNpc decodes OPNPC_T: targetIndex short, spellWidget
// intLE, spellChild short, spellItemId shortAdd, modifierFlags byteAdd.
func DecodeSpellOnNpc(payload []byte) (SpellOnNpc, error) {
	r := NewReader(payload)
	target, err := r.Short()
	if err != nil {
		return SpellOnNpc{}, err
	}
	spellWidget, err := r.IntLE()
	if err != nil {
		return SpellOnNpc{}, err
	}
	spellChild, err := r.Short()
	if err != nil {
		return SpellOnNpc{}, err
	}
	spellItemID, err := r.ShortAdd()
	if err != nil {
		return SpellOnNpc{}, err
	}
	mods, err := r.ByteAdd()
	if err != nil {
		return SpellOnNpc{}, err
	}
	if r.Remaining() != 0 {
		return SpellOnNpc{}, trailingErr("SPELL_ON_NPC", r)
	}
	return SpellOnNpc{TargetIndex: int(target), SpellWidget: spellWidget, SpellChild: int(spellChild), SpellItemID: int(spellItemID), ModifierFlags: int(mods)}, nil
}

// SpellOnObject is one decoded "spell_on_object" message (OPLOC_T,
// opcode 2, fixed length 15): the object id and tile and the spell
// selection, plus the Ctrl byte (ClientProtocol.ts lines 646-655).
// The spell id is resolved later, as with SpellOnPlayer.
type SpellOnObject struct {
	ObjectID      int
	X, Y          int
	SpellWidget   int32
	SpellChild    int
	SpellItemID   int
	ModifierFlags int
}

// DecodeSpellOnObject decodes OPLOC_T: y shortAddLE, objectId shortAdd,
// spellChild shortAdd, spellWidget intLE, x short, spellItemId
// shortLE, modifierFlags byteNeg.
func DecodeSpellOnObject(payload []byte) (SpellOnObject, error) {
	r := NewReader(payload)
	y, err := r.ShortAddLE()
	if err != nil {
		return SpellOnObject{}, err
	}
	objectID, err := r.ShortAdd()
	if err != nil {
		return SpellOnObject{}, err
	}
	spellChild, err := r.ShortAdd()
	if err != nil {
		return SpellOnObject{}, err
	}
	spellWidget, err := r.IntLE()
	if err != nil {
		return SpellOnObject{}, err
	}
	x, err := r.Short()
	if err != nil {
		return SpellOnObject{}, err
	}
	spellItemID, err := r.ShortLE()
	if err != nil {
		return SpellOnObject{}, err
	}
	mods, err := r.ByteNeg()
	if err != nil {
		return SpellOnObject{}, err
	}
	if r.Remaining() != 0 {
		return SpellOnObject{}, trailingErr("SPELL_ON_OBJECT", r)
	}
	return SpellOnObject{ObjectID: int(objectID), X: int(x), Y: int(y), SpellWidget: spellWidget, SpellChild: int(spellChild), SpellItemID: int(spellItemID), ModifierFlags: int(mods)}, nil
}

// SpellOnGround is one decoded "spell_on_ground" message (OPLOC_T_ALT,
// opcode 94, fixed length 15): the ground item id and tile and the
// spell selection, plus the Ctrl byte (ClientProtocol.ts lines
// 688-693). Despite the OPLOC_T_ALT name, the branch decodes a ground
// item target (the Telekinetic Grab path); it is not a loc target.
type SpellOnGround struct {
	GroundItemID  int
	X, Y          int
	SpellWidget   int32
	SpellChild    int
	SpellItemID   int
	ModifierFlags int
}

// DecodeSpellOnGround decodes OPLOC_T_ALT: spellWidget intLE,
// spellChild shortAdd, groundItemId shortAdd, x shortAddLE, y short,
// modifierFlags byte, spellItemId shortAddLE (the spell item id is
// read last).
func DecodeSpellOnGround(payload []byte) (SpellOnGround, error) {
	r := NewReader(payload)
	spellWidget, err := r.IntLE()
	if err != nil {
		return SpellOnGround{}, err
	}
	spellChild, err := r.ShortAdd()
	if err != nil {
		return SpellOnGround{}, err
	}
	groundID, err := r.ShortAdd()
	if err != nil {
		return SpellOnGround{}, err
	}
	x, err := r.ShortAddLE()
	if err != nil {
		return SpellOnGround{}, err
	}
	y, err := r.Short()
	if err != nil {
		return SpellOnGround{}, err
	}
	mods, err := r.Byte()
	if err != nil {
		return SpellOnGround{}, err
	}
	spellItemID, err := r.ShortAddLE()
	if err != nil {
		return SpellOnGround{}, err
	}
	if r.Remaining() != 0 {
		return SpellOnGround{}, trailingErr("SPELL_ON_GROUND", r)
	}
	return SpellOnGround{GroundItemID: int(groundID), X: int(x), Y: int(y), SpellWidget: spellWidget, SpellChild: int(spellChild), SpellItemID: int(spellItemID), ModifierFlags: int(mods)}, nil
}

// Appearance is one decoded "appearance" message (APPEARANCE_SET,
// opcode 37, fixed length 20): the gender byte, seven signed-short
// identity kits (negative means "no kit" for that slot), and five
// colour bytes (ClientProtocol.ts lines 560-565). The table length is
// 20, not the 13 the enum comment claims; see the opcodes.go note.
type Appearance struct {
	Gender int
	Kits   [7]int
	Colors [5]int
}

// DecodeAppearance decodes APPEARANCE_SET: gender byte, 7 signed
// shorts, 5 bytes.
func DecodeAppearance(payload []byte) (Appearance, error) {
	r := NewReader(payload)
	gender, err := r.Byte()
	if err != nil {
		return Appearance{}, err
	}
	var out Appearance
	out.Gender = int(gender)
	for i := range out.Kits {
		kit, err := r.SignedShort()
		if err != nil {
			return Appearance{}, err
		}
		out.Kits[i] = int(kit)
	}
	for i := range out.Colors {
		color, err := r.Byte()
		if err != nil {
			return Appearance{}, err
		}
		out.Colors[i] = int(color)
	}
	if r.Remaining() != 0 {
		return Appearance{}, trailingErr("APPEARANCE_SET", r)
	}
	return out, nil
}

// ChatStyle is public chat's colour and effect, as typed with prefixes
// like "red:wave:" (ClientProtocol.ts lines 1153-1156). Colours 13-20
// are patterns: colour-12 colour indices follow in Pattern. A nil
// *ChatStyle means the packet carried no style (or an invalid one;
// see DecodeChat).
type ChatStyle struct {
	Colour  int
	Effect  int
	Pattern []byte
}

// ChatMessageType values for ChatMessage.MessageType, from the CHAT
// branch's kind table (ClientProtocol.ts line 856).
const (
	ChatMessagePublic      = "public"
	ChatMessageGame        = "game"
	ChatMessageFriendsChat = "friends_chat"
)

// ChatMessage is one decoded CHAT packet (high-level opcode 190,
// variable length). Either a clan message (Clan, with ClanType; -1 is
// the listened guest clan) or a chat message (MessageType, Text, and
// optionally AutoChat and Style). Kind is the raw kind byte: 0 public,
// 1 game, 2 friends chat, 3 public autochat, 4 clan; any other value
// decodes as public without autochat, exactly as the TS branch's
// fallback does (ClientProtocol.ts lines 849-862).
type ChatMessage struct {
	Kind        int
	Clan        bool
	ClanType    int
	MessageType string
	Text        string
	AutoChat    bool
	Style       *ChatStyle
}

// DecodeChat decodes a CHAT packet. The kind byte selects the clan
// variant (kind 4: a signed clan-type byte, then the text) or the chat
// variant (the text, then an optional style read by readChatStyle).
// Trailing bytes after the style are ignored, as in TS: readChatStyle
// consumes at most the style, and the branch never re-checks
// remaining.
func DecodeChat(payload []byte) (ChatMessage, error) {
	r := NewReader(payload)
	kind, err := r.Byte()
	if err != nil {
		return ChatMessage{}, err
	}
	if kind == 4 { // CHAT_KIND_CLAN (ClientProtocol.ts line 484).
		clanTypeByte, err := r.Byte()
		if err != nil {
			return ChatMessage{}, err
		}
		text, err := r.String()
		if err != nil {
			return ChatMessage{}, err
		}
		return ChatMessage{Kind: int(kind), Clan: true, ClanType: int(int8(clanTypeByte)), Text: text}, nil
	}
	messageType := ChatMessagePublic
	switch kind {
	case 1:
		messageType = ChatMessageGame
	case 2:
		messageType = ChatMessageFriendsChat
	}
	text, err := r.String()
	if err != nil {
		return ChatMessage{}, err
	}
	style, err := readChatStyle(r)
	if err != nil {
		return ChatMessage{}, err
	}
	out := ChatMessage{Kind: int(kind), MessageType: messageType, Text: text, Style: style}
	if kind == 3 { // CHAT_KIND_AUTOCHAT (ClientProtocol.ts line 482).
		out.MessageType = ChatMessagePublic
		out.AutoChat = true
	}
	return out, nil
}

// readChatStyle mirrors readChatStyle (ClientProtocol.ts lines
// 1164-1172): an optional style after the chat text. Fewer than 2
// remaining bytes means no style. An out-of-range colour (>20) or
// effect (>5), or too few bytes for the pattern, also means no style —
// the bytes already read are consumed but the style is dropped, never
// an error. Colour 0 with effect 0 means no style. It never returns a
// non-nil error; the signature keeps the Reader-error shape for
// symmetry with the other decoders.
func readChatStyle(r *Reader) (*ChatStyle, error) {
	if r.Remaining() < 2 {
		return nil, nil
	}
	colour, err := r.Byte()
	if err != nil {
		return nil, err
	}
	effect, err := r.Byte()
	if err != nil {
		return nil, err
	}
	patternLength := 0
	if colour >= 13 { // CHAT_STYLE_FIRST_PATTERN.
		patternLength = int(colour) - 13 + 1
	}
	if colour > 20 || effect > 5 || r.Remaining() < patternLength {
		return nil, nil
	}
	var pattern []byte
	if patternLength > 0 {
		pattern, err = r.Bytes(patternLength)
		if err != nil {
			return nil, err
		}
	}
	if colour == 0 && effect == 0 {
		return nil, nil
	}
	return &ChatStyle{Colour: int(colour), Effect: int(effect), Pattern: pattern}, nil
}

// Clan channel action values: the CLAN_CHANNEL_ACTION branch's
// action table (ClientProtocol.ts line 864).
const (
	ClanChannelActionKick = "kick"
	ClanChannelActionBan  = "ban"
	ClanChannelActionMute = "mute"
)

// ClanChannelAction is one decoded "clan_channel_action" message
// (high-level opcode 181, variable length): the action, the value
// byte, and the target name (ClientProtocol.ts lines 863-867).
type ClanChannelAction struct {
	Action string
	Value  int
	Name   string
}

// DecodeClanChannelAction decodes a CLAN_CHANNEL_ACTION packet: the
// action byte, the value byte, then the name string, in TS read
// order. An action byte outside 0..2 is an error, following the
// DecodeTradeAction precedent for the unknown-action "raw" return:
// TS returns {type: "raw"} for it (the branch yields no
// clan_channel_action), and no message exists to dispatch, so the Go
// form is a decode failure, which the listener layer drops as
// malformed rather than routing. The reads run before the action
// check, exactly as in TS: a truncated payload errors as truncated
// even when the action byte is also unknown. Trailing bytes are
// ignored, as in TS.
func DecodeClanChannelAction(payload []byte) (ClanChannelAction, error) {
	r := NewReader(payload)
	actionByte, err := r.Byte()
	if err != nil {
		return ClanChannelAction{}, err
	}
	value, err := r.Byte()
	if err != nil {
		return ClanChannelAction{}, err
	}
	name, err := r.String()
	if err != nil {
		return ClanChannelAction{}, err
	}
	var action string
	switch actionByte {
	case 0:
		action = ClanChannelActionKick
	case 1:
		action = ClanChannelActionBan
	case 2:
		action = ClanChannelActionMute
	default:
		return ClanChannelAction{}, fmt.Errorf("protocol: unknown clan channel action %d", actionByte)
	}
	return ClanChannelAction{Action: action, Value: int(value), Name: name}, nil
}

// Friends-chat action values: the FRIENDS_CHAT_ACTION branch's
// named-actions table (ClientProtocol.ts lines 872-881), in wire
// order.
const (
	FriendsChatActionJoin          = "join"
	FriendsChatActionLeave         = "leave"
	FriendsChatActionKick          = "kick"
	FriendsChatActionAddFriend     = "add_friend"
	FriendsChatActionRemoveFriend  = "remove_friend"
	FriendsChatActionSetFriendRank = "set_friend_rank"
	FriendsChatActionAddIgnore     = "add_ignore"
	FriendsChatActionRemoveIgnore  = "remove_ignore"
)

// FriendsChatAction is one decoded "friends_chat_action" message
// (high-level opcode 196, variable length): the FriendsChatAction
// union (ClientProtocol.ts lines 276-284). Name is empty for the
// leave action (TS drops the decoded name for it), and Rank carries
// the signed rank byte only for set_friend_rank (TS drops the
// decoded rank for every other action); both fields are still read
// from the payload for every action, as the TS branch reads them
// before selecting the variant.
type FriendsChatAction struct {
	Action string
	Name   string
	Rank   int
}

// DecodeFriendsChatAction decodes a FRIENDS_CHAT_ACTION packet: the
// action-code byte, the name string, then the rank byte read as
// signed (rankByte > 0x7f subtracts 0x100,
// ClientProtocol.ts lines 869-871). An action code outside 0..7 is
// an error, following the DecodeTradeAction precedent for the
// unknown-action "raw" return (see DecodeClanChannelAction). The
// reads run before the action check, exactly as in TS. Trailing
// bytes are ignored, as in TS.
func DecodeFriendsChatAction(payload []byte) (FriendsChatAction, error) {
	r := NewReader(payload)
	actionCode, err := r.Byte()
	if err != nil {
		return FriendsChatAction{}, err
	}
	name, err := r.String()
	if err != nil {
		return FriendsChatAction{}, err
	}
	rankByte, err := r.Byte()
	if err != nil {
		return FriendsChatAction{}, err
	}
	rank := int(rankByte)
	if rankByte > 0x7f {
		rank = int(rankByte) - 0x100
	}
	switch actionCode {
	case 0:
		return FriendsChatAction{Action: FriendsChatActionJoin, Name: name}, nil
	case 1:
		return FriendsChatAction{Action: FriendsChatActionLeave}, nil
	case 2:
		return FriendsChatAction{Action: FriendsChatActionKick, Name: name}, nil
	case 3:
		return FriendsChatAction{Action: FriendsChatActionAddFriend, Name: name}, nil
	case 4:
		return FriendsChatAction{Action: FriendsChatActionRemoveFriend, Name: name}, nil
	case 5:
		return FriendsChatAction{Action: FriendsChatActionSetFriendRank, Name: name, Rank: rank}, nil
	case 6:
		return FriendsChatAction{Action: FriendsChatActionAddIgnore, Name: name}, nil
	case 7:
		return FriendsChatAction{Action: FriendsChatActionRemoveIgnore, Name: name}, nil
	default:
		return FriendsChatAction{}, fmt.Errorf("protocol: unknown friends chat action %d", actionCode)
	}
}

// PrivateMessage is one decoded "private_message" message
// (high-level opcode 197, variable length): the recipient string
// and the text string (ClientProtocol.ts lines 888-889).
type PrivateMessage struct {
	Recipient string
	Text      string
}

// DecodePrivateMessage decodes a PRIVATE_MESSAGE packet: the
// recipient string, then the text string. Trailing bytes are
// ignored, as in TS.
func DecodePrivateMessage(payload []byte) (PrivateMessage, error) {
	r := NewReader(payload)
	recipient, err := r.String()
	if err != nil {
		return PrivateMessage{}, err
	}
	text, err := r.String()
	if err != nil {
		return PrivateMessage{}, err
	}
	return PrivateMessage{Recipient: recipient, Text: text}, nil
}

// ChatFilter is one decoded "chat_filter" message (high-level
// opcode 198, fixed length 3): the public, private, and trade mode
// bytes (ClientProtocol.ts lines 890-895).
type ChatFilter struct {
	PublicMode  int
	PrivateMode int
	TradeMode   int
}

// DecodeChatFilter decodes a CHAT_FILTER packet: the public, private,
// and trade mode bytes. The payload must be consumed exactly: the
// opcode is fixed-length 3 in the table, following the fixed-length
// discipline in the file header.
func DecodeChatFilter(payload []byte) (ChatFilter, error) {
	r := NewReader(payload)
	publicMode, err := r.Byte()
	if err != nil {
		return ChatFilter{}, err
	}
	privateMode, err := r.Byte()
	if err != nil {
		return ChatFilter{}, err
	}
	tradeMode, err := r.Byte()
	if err != nil {
		return ChatFilter{}, err
	}
	if r.Remaining() != 0 {
		return ChatFilter{}, trailingErr("CHAT_FILTER", r)
	}
	return ChatFilter{PublicMode: int(publicMode), PrivateMode: int(privateMode), TradeMode: int(tradeMode)}, nil
}

// ReportAbuse is one decoded "report_abuse" message (high-level
// opcode 199, variable length): the reported name, the internal
// rule id, and the mute request (ClientProtocol.ts lines 896-899).
type ReportAbuse struct {
	Name   string
	RuleID int
	Mute   bool
}

// DecodeReportAbuse decodes a REPORT_ABUSE packet: the name string,
// the rule-id byte, then the mute byte, which is true only when
// exactly 1. Trailing bytes are ignored, as in TS.
func DecodeReportAbuse(payload []byte) (ReportAbuse, error) {
	r := NewReader(payload)
	name, err := r.String()
	if err != nil {
		return ReportAbuse{}, err
	}
	ruleID, err := r.Byte()
	if err != nil {
		return ReportAbuse{}, err
	}
	mute, err := r.Byte()
	if err != nil {
		return ReportAbuse{}, err
	}
	return ReportAbuse{Name: name, RuleID: int(ruleID), Mute: mute == 1}, nil
}

// InventoryAction is one decoded "inventory_action" message from
// INVENTORY_USE (high-level opcode 240, variable length): the
// inventory slot and item id, the fixed widget id 3214, and the option
// text (ClientProtocol.ts lines 910-911). The branch also reads and
// discards an int before the option string; it is read here and
// dropped, as in TS. Option is kept verbatim, including the empty
// string (the branch applies no `|| undefined`).
type InventoryAction struct {
	Slot     int
	ItemID   int
	WidgetID int32
	Option   string
}

// DecodeInventoryUse decodes INVENTORY_USE: slot short, itemId short,
// one discarded int, option string. Trailing bytes are ignored, as in
// TS.
func DecodeInventoryUse(payload []byte) (InventoryAction, error) {
	r := NewReader(payload)
	slot, err := r.Short()
	if err != nil {
		return InventoryAction{}, err
	}
	itemID, err := r.Short()
	if err != nil {
		return InventoryAction{}, err
	}
	if _, err := r.Int(); err != nil { // Discarded int; TS reads it in a comma expression.
		return InventoryAction{}, err
	}
	option, err := r.String()
	if err != nil {
		return InventoryAction{}, err
	}
	return InventoryAction{Slot: int(slot), ItemID: int(itemID), WidgetID: 3214, Option: option}, nil
}

// Inventory use-on target kinds, from the INVENTORY_USE_ON branch's
// kind table (ClientProtocol.ts lines 912-927). Kind 4 is the
// inventory target; kinds 0-3 are npc/loc/ground/player; any other
// kind byte decodes as "npc" (the TS branch's `?? "npc"` fallback).
const (
	UseOnTargetNpc       = "npc"
	UseOnTargetLoc       = "loc"
	UseOnTargetGround    = "ground"
	UseOnTargetPlayer    = "player"
	UseOnTargetInventory = "inventory"
)

// UseOnTarget is the target of an "inventory_use_on" message. For
// npc/loc/ground/player targets, ID is the entity id and X/Y are
// present only when HasTile. Level is the decoded level byte; TS
// carries it on entity targets and reads-but-discards it for
// inventory targets, so it is returned for every kind. For inventory
// targets, Slot and ItemID name the other inventory slot.
type UseOnTarget struct {
	Kind    string
	ID      int
	HasTile bool
	X, Y    int
	Level   int
	Slot    int
	ItemID  int
}

// InventoryUseOn is one decoded "inventory_use_on" message
// (INVENTORY_USE_ON, high-level opcode 241, variable length): the used
// item's slot and id, and its target.
type InventoryUseOn struct {
	Slot   int
	ItemID int
	Target UseOnTarget
}

// DecodeInventoryUseOn decodes INVENTORY_USE_ON: slot short, itemId
// short, kind byte, id short, hasTile byte, x/y shorts when hasTile is
// set, level byte, then — only for kind 4 — the target slot and item
// id shorts. Trailing bytes are ignored, as in TS.
func DecodeInventoryUseOn(payload []byte) (InventoryUseOn, error) {
	r := NewReader(payload)
	slot, err := r.Short()
	if err != nil {
		return InventoryUseOn{}, err
	}
	itemID, err := r.Short()
	if err != nil {
		return InventoryUseOn{}, err
	}
	kind, err := r.Byte()
	if err != nil {
		return InventoryUseOn{}, err
	}
	id, err := r.Short()
	if err != nil {
		return InventoryUseOn{}, err
	}
	hasTile, err := r.Byte()
	if err != nil {
		return InventoryUseOn{}, err
	}
	var x, y uint16
	if hasTile != 0 {
		x, err = r.Short()
		if err != nil {
			return InventoryUseOn{}, err
		}
		y, err = r.Short()
		if err != nil {
			return InventoryUseOn{}, err
		}
	}
	level, err := r.Byte()
	if err != nil {
		return InventoryUseOn{}, err
	}
	out := InventoryUseOn{Slot: int(slot), ItemID: int(itemID)}
	if kind == 4 {
		targetSlot, err := r.Short()
		if err != nil {
			return InventoryUseOn{}, err
		}
		targetItemID, err := r.Short()
		if err != nil {
			return InventoryUseOn{}, err
		}
		out.Target = UseOnTarget{Kind: UseOnTargetInventory, ID: int(id), Level: int(level), Slot: int(targetSlot), ItemID: int(targetItemID)}
		return out, nil
	}
	kindName := UseOnTargetNpc
	switch kind {
	case 1:
		kindName = UseOnTargetLoc
	case 2:
		kindName = UseOnTargetGround
	case 3:
		kindName = UseOnTargetPlayer
	}
	out.Target = UseOnTarget{Kind: kindName, ID: int(id), HasTile: hasTile != 0, X: int(x), Y: int(y), Level: int(level)}
	return out, nil
}

// InventoryMove is one decoded "inventory_move" message
// (INVENTORY_MOVE, high-level opcode 242, fixed length 4): the from/to
// slots and the fixed widget id 3214 (ClientProtocol.ts lines 928-929).
type InventoryMove struct {
	From     int
	To       int
	WidgetID int32
}

// DecodeInventoryMove decodes INVENTORY_MOVE: from short, to short.
func DecodeInventoryMove(payload []byte) (InventoryMove, error) {
	r := NewReader(payload)
	from, err := r.Short()
	if err != nil {
		return InventoryMove{}, err
	}
	to, err := r.Short()
	if err != nil {
		return InventoryMove{}, err
	}
	if r.Remaining() != 0 {
		return InventoryMove{}, trailingErr("INVENTORY_MOVE", r)
	}
	return InventoryMove{From: int(from), To: int(to), WidgetID: 3214}, nil
}

// DecodeBankDepositInventory decodes the bank deposit-inventory
// opcode (CLIENT_BANK_DEPOSIT_INVENTORY, 243, fixed length 0,
// ClientProtocol.ts lines 934-936): the packet carries no
// fields — the deposit-all itself is the message. A non-empty
// payload is malformed, as for the other fixed-length-0
// decodes (see DecodeInterfaceClose).
func DecodeBankDepositInventory(payload []byte) error {
	if len(payload) != 0 {
		return trailingErr("BANK_DEPOSIT_INVENTORY", NewReader(payload))
	}
	return nil
}

// DecodeBankDepositEquipment decodes the bank deposit-equipment
// opcode (CLIENT_BANK_DEPOSIT_EQUIPMENT, 244, fixed length 0,
// ClientProtocol.ts lines 934-936): the packet carries no
// fields — the deposit-all itself is the message. A non-empty
// payload is malformed, as for the other fixed-length-0
// decodes (see DecodeInterfaceClose).
func DecodeBankDepositEquipment(payload []byte) error {
	if len(payload) != 0 {
		return trailingErr("BANK_DEPOSIT_EQUIPMENT", NewReader(payload))
	}
	return nil
}

// BankMove is one decoded "bank_move" message (CLIENT_BANK_MOVE,
// 245, variable length, ClientProtocol.ts lines 934-936):
// the source and target display slots, the mode byte (1 =
// insert, anything else = swap), and the destination tab when
// the packet carries one. Tab is absent (HasTab false) when
// the packet ends after the mode byte: TS leaves the field
// undefined and the move stays in the current tab.
type BankMove struct {
	From   int
	To     int
	Insert bool
	Tab    int
	HasTab bool
}

// DecodeBankMove decodes BANK_MOVE: from short, to short, mode
// byte, and an optional tab byte. The mode byte maps 1 to
// insert and anything else to swap (NetworkBuilder.ts:860-878).
// The packet is variable-length, so trailing bytes after the
// tab are ignored, as for the other variable-length decodes
// (see the file header). A payload shorter than the from/to/
// mode fields errors with ErrUnexpectedEnd.
func DecodeBankMove(payload []byte) (BankMove, error) {
	r := NewReader(payload)
	from, err := r.Short()
	if err != nil {
		return BankMove{}, err
	}
	to, err := r.Short()
	if err != nil {
		return BankMove{}, err
	}
	mode, err := r.Byte()
	if err != nil {
		return BankMove{}, err
	}
	out := BankMove{From: int(from), To: int(to), Insert: mode == 1}
	if r.Remaining() > 0 {
		tab, err := r.Byte()
		if err != nil {
			return BankMove{}, err
		}
		out.Tab, out.HasTab = int(tab), true
	}
	return out, nil
}

// WidgetAction is one decoded "widget_action" message, from either the
// native IF_BUTTON family or the high-level WIDGET_ACTION opcode.
// WidgetID packs group and child (group = id >>> 16, child = id &
// 0xffff) for native opcodes; the high-level branch carries group and
// child as separate shorts, which may disagree with the packed id, and
// both are kept. Slot and ItemID are -1 when absent (a 0xffff slot or
// item id in the native branches, a negative slot or 0xffff item id
// in the high-level branch) — except IF_BUTTON_SUB, whose branch
// passes the slot through raw (see DecodeWidgetAction). ButtonNum and
// OpID are absent (Has* false) only in the high-level branch, where a
// zero byte means "absent". SubOpID is 0 when absent (IF_BUTTON_SUB
// sends byte+1, so a present value is always >= 1). Simple marks the
// plain IF_BUTTON click, which the dispatch additionally forwards as
// a button click when no interface-action handler takes it.
type WidgetAction struct {
	WidgetID     int32
	GroupID      int
	ChildID      int
	ButtonNum    int
	HasButtonNum bool
	OpID         int
	HasOpID      bool
	Option       string
	HasOption    bool
	Target       string
	HasTarget    bool
	Slot         int
	ItemID       int
	SubOpID      int
	Simple       bool
}

// DecodeWidgetAction decodes the native widget-action opcodes: IF_BUTTON
// (13, fixed length 4, ClientProtocol.ts lines 521-524), IF_BUTTON_SUB
// (89, fixed length 10, lines 525-528), IF_BUTTONX (91, fixed length
// 9, lines 694-702), and IF_BUTTON1..IF_BUTTON10 (fixed length 8,
// lines 703-725), whose button number is the opcode's position in the
// TS list [1,2,3,4,5,6,7,8,9,10] over opcodes
// [23,25,31,63,69,11,14,19,20,84].
func DecodeWidgetAction(opcode byte, payload []byte) (WidgetAction, error) {
	r := NewReader(payload)
	widgetID, err := r.Int()
	if err != nil {
		return WidgetAction{}, err
	}
	out := WidgetAction{
		WidgetID: widgetID,
		GroupID:  int(uint32(widgetID) >> 16),
		ChildID:  int(uint32(widgetID) & 0xffff),
		Slot:     -1,
		ItemID:   -1,
	}
	switch opcode {
	case NativeIfButton: // 13: widgetId only; buttonNum 1, simple.
		out.ButtonNum, out.HasButtonNum = 1, true
		out.Simple = true
	case NativeIfButtonSub: // 89: slot, itemId, buttonNum, subOpId bytes.
		slot, err := r.Short()
		if err != nil {
			return WidgetAction{}, err
		}
		itemID, err := r.Short()
		if err != nil {
			return WidgetAction{}, err
		}
		buttonNum, err := r.Byte()
		if err != nil {
			return WidgetAction{}, err
		}
		subOp, err := r.Byte()
		if err != nil {
			return WidgetAction{}, err
		}
		// The branch passes slot raw (no 0xffff check) but maps a
		// 0xffff item id to -1.
		out.Slot = int(slot)
		out.ItemID = unsignedItemID(itemID)
		out.ButtonNum, out.HasButtonNum = int(buttonNum), true
		out.SubOpID = int(subOp) + 1
	case NativeIfButtonX: // 91: slot, itemId, buttonNum byte.
		slot, err := r.Short()
		if err != nil {
			return WidgetAction{}, err
		}
		itemID, err := r.Short()
		if err != nil {
			return WidgetAction{}, err
		}
		buttonNum, err := r.Byte()
		if err != nil {
			return WidgetAction{}, err
		}
		if slot != 0xffff {
			out.Slot = int(slot)
		}
		if itemID != 0xffff {
			out.ItemID = int(itemID)
		}
		out.ButtonNum, out.HasButtonNum = int(buttonNum), true
	default:
		buttonNum, ok := ifButtonNumbers[opcode]
		if !ok {
			return WidgetAction{}, fmt.Errorf("protocol: unsupported widget_action opcode %d", opcode)
		}
		slot, err := r.Short()
		if err != nil {
			return WidgetAction{}, err
		}
		itemID, err := r.Short()
		if err != nil {
			return WidgetAction{}, err
		}
		if slot != 0xffff {
			out.Slot = int(slot)
		}
		if itemID != 0xffff {
			out.ItemID = int(itemID)
		}
		out.ButtonNum, out.HasButtonNum = buttonNum, true
	}
	if r.Remaining() != 0 {
		return WidgetAction{}, trailingErr("WIDGET_ACTION", r)
	}
	return out, nil
}

// ifButtonNumbers is the IF_BUTTON1..IF_BUTTON10 opcode order from the
// shared decode branch (ClientProtocol.ts lines 706-711): the button
// number is the opcode's index in the list plus one.
var ifButtonNumbers = map[byte]int{
	NativeIfButton1:  1,
	NativeIfButton2:  2,
	NativeIfButton3:  3,
	NativeIfButton4:  4,
	NativeIfButton5:  5,
	NativeIfButton6:  6,
	NativeIfButton7:  7,
	NativeIfButton8:  8,
	NativeIfButton9:  9,
	NativeIfButton10: 10,
}

// DecodeWidgetActionHigh decodes a WIDGET_ACTION packet (high-level
// opcode 251, variable length): widget id int, group/child shorts,
// option and target strings, op-id and button-number bytes, two
// skipped shorts and one skipped byte, a signed slot short, and an
// item id short (ClientProtocol.ts lines 951-964). Empty strings and
// zero op/button bytes decode to "absent"; a negative slot and a
// 0xffff item id decode to -1. Trailing bytes are ignored, as in TS.
func DecodeWidgetActionHigh(payload []byte) (WidgetAction, error) {
	r := NewReader(payload)
	widgetID, err := r.Int()
	if err != nil {
		return WidgetAction{}, err
	}
	groupID, err := r.Short()
	if err != nil {
		return WidgetAction{}, err
	}
	childID, err := r.Short()
	if err != nil {
		return WidgetAction{}, err
	}
	option, err := r.String()
	if err != nil {
		return WidgetAction{}, err
	}
	target, err := r.String()
	if err != nil {
		return WidgetAction{}, err
	}
	opID, err := r.Byte()
	if err != nil {
		return WidgetAction{}, err
	}
	buttonNum, err := r.Byte()
	if err != nil {
		return WidgetAction{}, err
	}
	if _, err := r.Short(); err != nil { // Skipped shorts/byte; TS reads and discards them.
		return WidgetAction{}, err
	}
	if _, err := r.Short(); err != nil {
		return WidgetAction{}, err
	}
	if _, err := r.Byte(); err != nil {
		return WidgetAction{}, err
	}
	slot, err := r.SignedShort()
	if err != nil {
		return WidgetAction{}, err
	}
	itemID, err := r.Short()
	if err != nil {
		return WidgetAction{}, err
	}
	out := WidgetAction{WidgetID: widgetID, GroupID: int(groupID), ChildID: int(childID), Slot: -1, ItemID: -1}
	if option != "" {
		out.Option, out.HasOption = option, true
	}
	if target != "" {
		out.Target, out.HasTarget = target, true
	}
	if opID != 0 {
		out.OpID, out.HasOpID = int(opID), true
	}
	if buttonNum != 0 {
		out.ButtonNum, out.HasButtonNum = int(buttonNum), true
	}
	if slot >= 0 {
		out.Slot = int(slot)
	}
	if id := unsignedItemID(itemID); id >= 0 {
		out.ItemID = id
	}
	return out, nil
}

// WidgetTarget is one decoded "widget_target" message (IF_BUTTONT,
// opcode 90, fixed length 16): the target and source widgets, slots,
// and item ids (ClientProtocol.ts lines 529-535). The source item id
// uses unsignedItemId (0xffff becomes -1); the target item id is the
// raw shortAddLE value.
type WidgetTarget struct {
	TargetWidgetID int32
	TargetSlot     int
	TargetItemID   int
	SourceWidgetID int32
	SourceSlot     int
	SourceItemID   int
}

// DecodeWidgetTarget decodes IF_BUTTONT: targetWidgetId intIME,
// targetSlot shortAddLE, sourceWidgetId intLE, sourceSlot shortLE,
// sourceItemId short (unsignedItemId), targetItemId shortAddLE.
func DecodeWidgetTarget(payload []byte) (WidgetTarget, error) {
	r := NewReader(payload)
	targetWidgetID, err := r.IntIME()
	if err != nil {
		return WidgetTarget{}, err
	}
	targetSlot, err := r.ShortAddLE()
	if err != nil {
		return WidgetTarget{}, err
	}
	sourceWidgetID, err := r.IntLE()
	if err != nil {
		return WidgetTarget{}, err
	}
	sourceSlot, err := r.ShortLE()
	if err != nil {
		return WidgetTarget{}, err
	}
	sourceItemRaw, err := r.Short()
	if err != nil {
		return WidgetTarget{}, err
	}
	targetItemID, err := r.ShortAddLE()
	if err != nil {
		return WidgetTarget{}, err
	}
	if r.Remaining() != 0 {
		return WidgetTarget{}, trailingErr("WIDGET_TARGET", r)
	}
	return WidgetTarget{
		TargetWidgetID: targetWidgetID, TargetSlot: int(targetSlot), TargetItemID: int(targetItemID),
		SourceWidgetID: sourceWidgetID, SourceSlot: int(sourceSlot), SourceItemID: unsignedItemID(sourceItemRaw),
	}, nil
}

// WidgetDrag is one decoded "widget_drag" message, from either
// IF_BUTTOND (native opcode 1) or IF_BUTTOND (high-level opcode 253);
// both branches read the same 16-byte layout
// (ClientProtocol.ts lines 753-759 and 965-970).
type WidgetDrag struct {
	TargetItemID   int
	TargetWidgetID int32
	SourceItemID   int
	SourceSlot     int
	SourceWidgetID int32
	TargetSlot     int
}

// DecodeWidgetDrag decodes IF_BUTTOND (either opcode): targetItemId
// shortLE, targetWidgetId intLE, sourceItemId short, sourceSlot
// shortAddLE, sourceWidgetId intME, targetSlot shortLE.
func DecodeWidgetDrag(payload []byte) (WidgetDrag, error) {
	r := NewReader(payload)
	targetItemID, err := r.ShortLE()
	if err != nil {
		return WidgetDrag{}, err
	}
	targetWidgetID, err := r.IntLE()
	if err != nil {
		return WidgetDrag{}, err
	}
	sourceItemID, err := r.Short()
	if err != nil {
		return WidgetDrag{}, err
	}
	sourceSlot, err := r.ShortAddLE()
	if err != nil {
		return WidgetDrag{}, err
	}
	sourceWidgetID, err := r.IntME()
	if err != nil {
		return WidgetDrag{}, err
	}
	targetSlot, err := r.ShortLE()
	if err != nil {
		return WidgetDrag{}, err
	}
	if r.Remaining() != 0 {
		return WidgetDrag{}, trailingErr("WIDGET_DRAG", r)
	}
	return WidgetDrag{
		TargetItemID: int(targetItemID), TargetWidgetID: targetWidgetID,
		SourceItemID: int(sourceItemID), SourceSlot: int(sourceSlot),
		SourceWidgetID: sourceWidgetID, TargetSlot: int(targetSlot),
	}, nil
}

// LocalTrigger is one decoded "local_trigger" message
// (IF_TRIGGEROPLOCAL, opcode 30, variable short length): the opcode
// parameter, the widget id, the child index (a signed short), the item
// id (-1 for 0xffff), and the script argument bytes
// (ClientProtocol.ts lines 536-551).
type LocalTrigger struct {
	OpcodeParam int32
	WidgetID    int32
	ChildIndex  int
	ItemID      int
	ArgsData    []byte
}

// DecodeLocalTrigger decodes IF_TRIGGEROPLOCAL: a block-length short
// that must be at least 12 and must equal the remaining payload, then
// opcodeParam intLE, child shortLE (signed), widgetId intLE, itemId
// shortLE (unsignedItemId), and the remaining blockLength-12 argument
// bytes. A bad block length is an error, mirroring the TS throw.
func DecodeLocalTrigger(payload []byte) (LocalTrigger, error) {
	r := NewReader(payload)
	blockLength, err := r.Short()
	if err != nil {
		return LocalTrigger{}, err
	}
	if blockLength < 12 || int(blockLength) != r.Remaining() {
		return LocalTrigger{}, fmt.Errorf("protocol: invalid IF_TRIGGEROPLOCAL payload length")
	}
	opcodeParam, err := r.IntLE()
	if err != nil {
		return LocalTrigger{}, err
	}
	childRaw, err := r.ShortLE()
	if err != nil {
		return LocalTrigger{}, err
	}
	widgetID, err := r.IntLE()
	if err != nil {
		return LocalTrigger{}, err
	}
	itemRaw, err := r.ShortLE()
	if err != nil {
		return LocalTrigger{}, err
	}
	argsData, err := r.Bytes(int(blockLength) - 12)
	if err != nil {
		return LocalTrigger{}, err
	}
	childIndex := int(childRaw)
	if childRaw > 0x7fff {
		childIndex = int(childRaw) - 0x10000
	}
	return LocalTrigger{
		OpcodeParam: opcodeParam, WidgetID: widgetID,
		ChildIndex: childIndex, ItemID: unsignedItemID(itemRaw),
		ArgsData: argsData,
	}, nil
}

// VarpTransmit is one decoded "varp_transmit" message (VARP_TRANSMIT,
// high-level opcode 191, fixed length 6): the varp id and the
// transmitted value (ClientProtocol.ts lines 904-905). VarpID is the
// unsigned big-endian short (Reader.short, ClientProtocol.ts lines
// 391-396); Value is the signed big-endian int (Reader.int, lines
// 430-435), so negative values survive — the dispatch case only tests
// value !== 0 (NetworkBuilder.ts line 1162). No listener file owns
// this message; dispatch() handles it inline (see the file header).
type VarpTransmit struct {
	VarpID int
	Value  int32
}

// DialogueContinue is one decoded "dialogue_continue" message: the
// widget id the continue click/space keypress names, and the child
// index — the one-based choice for a native option resume, or the
// literal chosen quantity for a skillmulti/creation-menu click
// (NetworkBuilder.ts lines 908-912). No listener file owns this
// message; dispatch() handles it inline (see the file header): the
// creation-menu branch, then the interface-action-click emit, then
// DialogueManager.handleResume (NetworkBuilder.ts lines 908-951).
// WidgetID is the signed big-endian int (group = id >>> 16, child
// = id & 0xffff); ChildIndex is the unsigned short, decoded
// without a sign adjustment in both branches.
type DialogueContinue struct {
	WidgetID   int32
	ChildIndex int
}

// DecodeDialogueContinue decodes the two RESUME_PAUSEBUTTON
// opcodes (fixed length 6). The native branch (opcode 62) reads
// the child index first, as a shortAddLE, then the widget id int
// (ClientProtocol.ts line 555); the high-level branch (opcode
// 252) reads the widget id int first, then the child index as a
// plain short (ClientProtocol.ts line 982). The read order and
// transforms differ, exactly as the branches do; any other opcode
// is rejected.
func DecodeDialogueContinue(opcode byte, payload []byte) (DialogueContinue, error) {
	r := NewReader(payload)
	var widgetID int32
	var childIndex uint16
	var err error
	switch opcode {
	case NativeResumePausebutton: // 62: childIndex shortAddLE, widgetId int.
		if childIndex, err = r.ShortAddLE(); err == nil {
			widgetID, err = r.Int()
		}
	case ClientResumePausebutton: // 252: widgetId int, childIndex short.
		if widgetID, err = r.Int(); err == nil {
			childIndex, err = r.Short()
		}
	default:
		return DialogueContinue{}, fmt.Errorf("protocol: unsupported dialogue_continue opcode %d", opcode)
	}
	if err != nil {
		return DialogueContinue{}, err
	}
	if r.Remaining() != 0 {
		return DialogueContinue{}, trailingErr("DIALOGUE_CONTINUE", r)
	}
	return DialogueContinue{WidgetID: widgetID, ChildIndex: int(childIndex)}, nil
}

// DecodeVarpTransmit decodes VARP_TRANSMIT: varpId short, value int,
// in that order, with no transforms. The payload must be consumed
// exactly: the opcode is fixed-length 6 in the length table
// (ClientPackets.ts line 137).
func DecodeVarpTransmit(payload []byte) (VarpTransmit, error) {
	r := NewReader(payload)
	varpID, err := r.Short()
	if err != nil {
		return VarpTransmit{}, err
	}
	value, err := r.Int()
	if err != nil {
		return VarpTransmit{}, err
	}
	if r.Remaining() != 0 {
		return VarpTransmit{}, trailingErr("VARP_TRANSMIT", r)
	}
	return VarpTransmit{VarpID: int(varpID), Value: value}, nil
}

// DialogueAmount is one decoded "dialogue_amount" message
// (RESUME_COUNTDIALOG, opcode 192, fixed length 4): the amount
// the player entered in an enter-amount prompt
// (ClientProtocol.ts line 984).
type DialogueAmount struct {
	Amount int32
}

// DecodeDialogueAmount decodes RESUME_COUNTDIALOG: a single int.
// The payload must be consumed exactly: the opcode is fixed-length
// 4 in the length table (ClientPackets.ts line 138).
func DecodeDialogueAmount(payload []byte) (DialogueAmount, error) {
	r := NewReader(payload)
	amount, err := r.Int()
	if err != nil {
		return DialogueAmount{}, err
	}
	if r.Remaining() != 0 {
		return DialogueAmount{}, trailingErr("DIALOGUE_AMOUNT", r)
	}
	return DialogueAmount{Amount: amount}, nil
}

// DialogueInput is one decoded "dialogue_input" message
// (RESUME_NAMEDIALOG, opcode 193, or RESUME_STRINGDIALOG, opcode
// 194, both variable length): the text the player entered in an
// enter-input prompt (ClientProtocol.ts lines 985-987). Both
// opcodes decode identically; the caller does not distinguish
// them, as TS does not.
type DialogueInput struct {
	Value string
}

// DecodeDialogueInput decodes RESUME_NAMEDIALOG /
// RESUME_STRINGDIALOG: a NUL-terminated string. Trailing bytes
// after the string are ignored, as in TS: the branch reads only
// the string and never re-checks remaining (the DecodeChat
// convention for variable-length string packets). An
// unterminated string errors.
func DecodeDialogueInput(payload []byte) (DialogueInput, error) {
	r := NewReader(payload)
	value, err := r.String()
	if err != nil {
		return DialogueInput{}, err
	}
	return DialogueInput{Value: value}, nil
}

// WidgetActionKind names the action of a decoded "widget"
// message: the client reporting a widget opened or closed.
type WidgetActionKind string

const (
	// WidgetOpened is a widget "open" report (action byte 0).
	WidgetOpened WidgetActionKind = "open"
	// WidgetClosed is a widget "close" report (any nonzero
	// action byte).
	WidgetClosed WidgetActionKind = "close"
)

// Widget is one decoded "widget" message (WIDGET, high-level
// opcode 250, variable length): the open/close action, the
// widget group id, and the modal flag
// (ClientProtocol.ts lines 947-949). Only the close action acts
// in dispatch (NetworkBuilder.ts line 1170); an open report is
// routed and ignored.
type Widget struct {
	Action  WidgetActionKind
	GroupID int
	Modal   bool
}

// DecodeWidget decodes WIDGET: an action byte (0 means "open",
// anything else "close"), a group-id short, and a modal byte.
// Trailing bytes are ignored, as in TS: the branch reads only
// these fields and never re-checks remaining (the
// DecodeWidgetActionHigh convention for variable-length packets).
func DecodeWidget(payload []byte) (Widget, error) {
	r := NewReader(payload)
	actionByte, err := r.Byte()
	if err != nil {
		return Widget{}, err
	}
	groupID, err := r.Short()
	if err != nil {
		return Widget{}, err
	}
	modalByte, err := r.Byte()
	if err != nil {
		return Widget{}, err
	}
	action := WidgetClosed
	if actionByte == 0 {
		action = WidgetOpened
	}
	return Widget{Action: action, GroupID: int(groupID), Modal: modalByte != 0}, nil
}

// DecodeInterfaceClose decodes IF_CLOSE (native opcode 55, fixed
// length 0): the client's explicit interface-close request
// (ClientProtocol.ts lines 552-553). It carries no fields; a
// non-empty payload errors, as TS throws when the remaining
// length differs from the table length 0
// (NativeClientPackets.ts line 144; ClientProtocol.ts lines
// 510-516).
func DecodeInterfaceClose(payload []byte) error {
	if len(payload) != 0 {
		return trailingErr("INTERFACE_CLOSE", NewReader(payload))
	}
	return nil
}
