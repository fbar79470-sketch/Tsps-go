package protocol

import "encoding/binary"

// State encoders: varps/varbits, inventory/bank slots, skills, and run
// energy, ported from ClientProtocol.ts (TSPS server, commit bfd81e3).
// There is no encodeVarpBatch in the TS source: the VARP_BATCH opcode (43)
// exists in the ServerPackets table, but no encoder writes it, so none is
// ported here rather than inventing a layout.

// EncodeVarp encodes a varp (encodeVarp, ClientProtocol.ts lines
// 1397-1404): VARP_SMALL (opcode 40, 3 bytes) when the value fits in a
// byte, otherwise VARP_LARGE (opcode 41, 6 bytes with an int32 value).
func EncodeVarp(id, value int) []byte {
	if value >= 0 && value <= 255 {
		payload := make([]byte, 3)
		binary.BigEndian.PutUint16(payload[0:2], uint16(id))
		payload[2] = byte(value)
		return mustEncode(ServerVarpSmall, payload)
	}
	payload := make([]byte, 6)
	binary.BigEndian.PutUint16(payload[0:2], uint16(id))
	binary.BigEndian.PutUint32(payload[2:6], uint32(value))
	return mustEncode(ServerVarpLarge, payload)
}

// EncodeVarpLong encodes a 64-bit varp (encodeVarpLong, ClientProtocol.ts
// lines 1406-1411): VARP_LONG (opcode 44, 10 bytes), id plus a big-endian
// int64.
func EncodeVarpLong(id int, value int64) []byte {
	payload := make([]byte, 10)
	binary.BigEndian.PutUint16(payload[0:2], uint16(id))
	binary.BigEndian.PutUint64(payload[2:10], uint64(value))
	return mustEncode(ServerVarpLong, payload)
}

// EncodeVarbit encodes a varbit (encodeVarbit, ClientProtocol.ts lines
// 1413-1418): VARBIT (opcode 42, 6 bytes), id plus an int32 value.
func EncodeVarbit(id, value int) []byte {
	payload := make([]byte, 6)
	binary.BigEndian.PutUint16(payload[0:2], uint16(id))
	binary.BigEndian.PutUint32(payload[2:6], uint32(value))
	return mustEncode(ServerVarbit, payload)
}

// ItemSlot is one inventory/bank/script-inventory slot: its index, the
// item id (-1 empty), and the quantity.
type ItemSlot struct {
	Slot     int
	ItemID   int
	Quantity int
}

// encodeItemSlot renders one item slot (encodeItemSlot, ClientProtocol.ts
// lines 1420-1428): slot u16, item id + 1 as u16 (so -1 encodes as 0),
// then the quantity OSRS-style — one byte, or 255 followed by an int32
// when the quantity is 255 or more. Shared by the inventory, bank, and
// script-inventory encoders, as in TS.
func encodeItemSlot(slot, itemID, quantity int) []byte {
	large := quantity >= 255
	payload := make([]byte, 5)
	if large {
		payload = make([]byte, 9)
	}
	binary.BigEndian.PutUint16(payload[0:2], uint16(slot))
	binary.BigEndian.PutUint16(payload[2:4], uint16(itemID+1))
	if large {
		payload[4] = 255
		binary.BigEndian.PutUint32(payload[5:9], uint32(quantity))
	} else if quantity > 0 {
		payload[4] = byte(quantity)
	}
	// A non-positive small quantity writes 0 (TS: Math.max(0,
	// quantity)); the zero-filled byte already holds it.
	return payload
}

// EncodeInventorySnapshot encodes a whole inventory (encodeInventorySnapshot,
// ClientProtocol.ts lines 1430-1438): INVENTORY_SNAPSHOT (opcode 50,
// 2-byte length prefix), a u16 slot count followed by one item slot per
// entry.
func EncodeInventorySnapshot(slots []ItemSlot) []byte {
	payload := make([]byte, 2)
	binary.BigEndian.PutUint16(payload[0:2], uint16(len(slots)))
	for _, slot := range slots {
		payload = append(payload, encodeItemSlot(slot.Slot, slot.ItemID, slot.Quantity)...)
	}
	return mustEncode(ServerInventorySnapshot, payload)
}

// EncodeInventoryStopTransmit encodes UPDATE_INV_STOPTRANSMIT
// (encodeInventoryStopTransmit, ClientProtocol.ts lines 1440-1447): it
// clears client data only, never authoritative item storage. It panics
// when the inventory id is not an unsigned short, mirroring the TS
// RangeError.
func EncodeInventoryStopTransmit(inventoryID int) []byte {
	if inventoryID < 0 || inventoryID > 0xffff {
		panic("protocol: inventoryId must be an unsigned short")
	}
	payload := make([]byte, 2)
	binary.BigEndian.PutUint16(payload[0:2], uint16(inventoryID))
	return mustEncode(ServerInventoryStopTransmit, payload)
}

// EncodeInventorySlot encodes one changed inventory slot
// (encodeInventorySlot, ClientProtocol.ts lines 1449-1451): INVENTORY_SLOT
// (opcode 51, 1-byte length prefix).
func EncodeInventorySlot(slot, itemID, quantity int) []byte {
	return mustEncode(ServerInventorySlot, encodeItemSlot(slot, itemID, quantity))
}

// BankSlot is one bank slot (BankSlotView, ClientProtocol.ts line 1470).
// Tab defaults to 0, matching the TS `tab ?? 0`.
type BankSlot struct {
	Slot        int
	ItemID      int
	Quantity    int
	Placeholder bool
	Tab         int
}

// encodeBankSlotPayload renders one bank slot (encodeBankSlotPayload,
// ClientProtocol.ts lines 1472-1477): an item slot plus one flag byte
// (placeholder bit 0, tab in bits 1+).
func encodeBankSlotPayload(slot BankSlot) []byte {
	payload := encodeItemSlot(slot.Slot, slot.ItemID, slot.Quantity)
	flags := slot.Tab << 1
	if slot.Placeholder {
		flags |= 1
	}
	return append(payload, byte(flags))
}

// EncodeBankSnapshot encodes a whole bank (encodeBankSnapshot,
// ClientProtocol.ts lines 1479-1484): BANK_SNAPSHOT (opcode 52, 2-byte
// length prefix), capacity u16, slot count u16, then one bank slot per
// entry.
func EncodeBankSnapshot(capacity int, slots []BankSlot) []byte {
	payload := make([]byte, 4)
	binary.BigEndian.PutUint16(payload[0:2], uint16(capacity))
	binary.BigEndian.PutUint16(payload[2:4], uint16(len(slots)))
	for _, slot := range slots {
		payload = append(payload, encodeBankSlotPayload(slot)...)
	}
	return mustEncode(ServerBankSnapshot, payload)
}

// EncodeBankSlot encodes one changed bank slot (encodeBankSlot,
// ClientProtocol.ts lines 1486-1488): BANK_SLOT (opcode 53, 1-byte length
// prefix).
func EncodeBankSlot(slot BankSlot) []byte {
	return mustEncode(ServerBankSlot, encodeBankSlotPayload(slot))
}

// encodeSkills renders a skills packet body (encodeSkills,
// ClientProtocol.ts lines 2155-2171): a 1-byte skill count, 9 bytes per
// skill (id, xp int32, base level, virtual level, boost + 128, current
// level), then total level u16 and combat level byte. TS allocates 10
// bytes per skill but writes 9, so the payload ends with one zero slack
// byte per skill; this mirrors that allocation exactly, slack included.
// Shared by the snapshot and delta encoders, as in TS.
func encodeSkills(opcode byte, skills []SkillView, totalLevel, combatLevel int) []byte {
	payload := make([]byte, 1+len(skills)*10+3)
	payload[0] = byte(len(skills))
	offset := 1
	for _, skill := range skills {
		payload[offset] = byte(skill.ID)
		offset++
		binary.BigEndian.PutUint32(payload[offset:offset+4], uint32(skill.XP))
		offset += 4
		payload[offset] = byte(skill.BaseLevel)
		offset++
		payload[offset] = byte(skill.VirtualLevel)
		offset++
		payload[offset] = byte(skill.Boost + 128)
		offset++
		payload[offset] = byte(skill.CurrentLevel)
		offset++
	}
	binary.BigEndian.PutUint16(payload[offset:offset+2], uint16(totalLevel))
	payload[offset+2] = byte(combatLevel)
	return mustEncode(opcode, payload)
}

// EncodeSkillsSnapshot encodes all skills (encodeSkillsSnapshot,
// ClientProtocol.ts lines 2173-2175): SKILLS_SNAPSHOT (opcode 70, 1-byte
// length prefix).
func EncodeSkillsSnapshot(skills []SkillView, totalLevel, combatLevel int) []byte {
	return encodeSkills(ServerSkillsSnapshot, skills, totalLevel, combatLevel)
}

// EncodeSkillsDelta encodes changed skills (encodeSkillsDelta,
// ClientProtocol.ts lines 2177-2179): SKILLS_DELTA (opcode 71, 1-byte
// length prefix).
func EncodeSkillsDelta(skills []SkillView, totalLevel, combatLevel int) []byte {
	return encodeSkills(ServerSkillsDelta, skills, totalLevel, combatLevel)
}

// runEnergyMaxUnits is RUN_ENERGY_MAX_UNITS (ClientProtocol.ts line 2188):
// OSRS run energy is 0-10,000 units; 100 units is 1%.
const runEnergyMaxUnits = 10000

// EncodeRunEnergy encodes the run energy orb state (encodeRunEnergy,
// ClientProtocol.ts lines 2193-2206): RUN_ENERGY (opcode 81, fixed length
// 10): percent byte, running byte, units u16, weight kg signed, stamina
// ticks u16, stamina active u16. Units are clamped to 0-10000, weight to
// int16 range, stamina to u16 range. TS takes weightKg as a float and
// rounds it; this signature takes an int, so no rounding applies.
func EncodeRunEnergy(units int, running bool, weightKg int, staminaTicks int) []byte {
	clampedUnits := units
	if clampedUnits < 0 {
		clampedUnits = 0
	}
	if clampedUnits > runEnergyMaxUnits {
		clampedUnits = runEnergyMaxUnits
	}
	clampedWeight := weightKg
	if clampedWeight < -32768 {
		clampedWeight = -32768
	}
	if clampedWeight > 32767 {
		clampedWeight = 32767
	}
	clampedStamina := staminaTicks
	if clampedStamina < 0 {
		clampedStamina = 0
	}
	if clampedStamina > 65535 {
		clampedStamina = 65535
	}
	payload := make([]byte, 10)
	payload[0] = byte(clampedUnits / 100)
	if running {
		payload[1] = 1
	}
	binary.BigEndian.PutUint16(payload[2:4], uint16(clampedUnits))
	binary.BigEndian.PutUint16(payload[4:6], uint16(int16(clampedWeight)))
	binary.BigEndian.PutUint16(payload[6:8], uint16(clampedStamina))
	if clampedStamina > 0 {
		binary.BigEndian.PutUint16(payload[8:10], 1)
	}
	return mustEncode(ServerRunEnergy, payload)
}
