package protocol

// Player appearance, ported from ClientProtocol.ts (TSPS server, commit
// bfd81e3).

// HeadIcons is the pair of overhead icons in a PlayerAppearance
// (ClientProtocol.ts line 64): the skull and prayer icons, each -1 when
// absent.
type HeadIcons struct {
	Skull  int
	Prayer int
}

// PlayerAppearance is a player's look (ClientProtocol.ts lines 56-66).
// Pronouns and NpcTransformationID are pointers because TS distinguishes
// an absent value (pronouns fall back to gender; no transformation) from
// any number, including 0. Colors, Kits, and Equip are indexed positionally
// by the encoder; missing entries read as -1 (equip/kits) or 0 (colors),
// matching the TS `??` defaults. HeadIcons is nil when the player shows
// neither icon.
type PlayerAppearance struct {
	Gender              int
	Hidden              bool
	Pronouns            *int
	Colors              []int
	Kits                []int
	Equip               []int
	NpcTransformationID *int
	EquipQty            []int
	HeadIcons           *HeadIcons
}

// EncodePlayerAppearance renders the raw appearance block for one player
// (encodePlayerAppearance, ClientProtocol.ts lines 2987-3062).
//
// It returns the raw block, NOT a framed packet: TS returns the Buffer
// directly, and the block is embedded in player sync update blocks (and
// sent in the handshake in a different, shorter form by
// EncodeHandshake). Callers must not frame it with EncodePacket.
//
// animations holds the 7 stand/walk/turn sequence ids; missing entries
// read as 0xffff, matching the TS `?? 0xffff`. skillLevel is the total
// level written after the combat level.
func EncodePlayerAppearance(appearance PlayerAppearance, name string, combatLevel, skillLevel int, animations []int) []byte {
	var bytes []byte
	push := func(value int) { bytes = append(bytes, byte(value&0xff)) }
	short := func(value int) {
		bytes = append(bytes, byte((value>>8)&0xff), byte(value&0xff))
	}
	int32be := func(value int) {
		bytes = append(bytes, byte((value>>24)&0xff), byte((value>>16)&0xff), byte((value>>8)&0xff), byte(value&0xff))
	}
	text := func(value string) { bytes = append(bytes, latin1Bytes(value)...) }
	// at reads list[i], or -1 past the end, matching `list[i] ?? -1`.
	at := func(list []int, i int) int {
		if i < 0 || i >= len(list) {
			return -1
		}
		return list[i]
	}
	// atZero reads list[i], or 0 past the end, matching `list[i] ?? 0`.
	atZero := func(list []int, i int) int {
		if i < 0 || i >= len(list) {
			return 0
		}
		return list[i]
	}
	// itemSlots maps a wire slot to the equip index shown there, and
	// kitSlots maps a wire slot to the body-kit index used when no item
	// is equipped there (ClientProtocol.ts lines 3011-3017).
	itemSlots := map[int]int{0: 0, 1: 1, 2: 2, 3: 3, 4: 4, 5: 5, 7: 7, 9: 9, 10: 10}
	kitSlots := map[int]int{4: 2, 6: 3, 7: 5, 8: 0, 9: 4, 10: 6, 11: 1}
	equipmentSlot := func(wireSlot int) int {
		itemID := -1
		if itemSlot, ok := itemSlots[wireSlot]; ok {
			itemID = at(appearance.Equip, itemSlot)
		}
		if itemID >= 0 {
			return itemID + 2048
		}
		kitID := -1
		if kitSlot, ok := kitSlots[wireSlot]; ok {
			kitID = at(appearance.Kits, kitSlot)
		}
		if kitID >= 0 {
			return kitID + 256
		}
		return 0
	}

	push(appearance.Gender)
	skull, prayer := -1, -1
	if appearance.HeadIcons != nil {
		skull, prayer = appearance.HeadIcons.Skull, appearance.HeadIcons.Prayer
	}
	push(skull)
	push(prayer)
	npcID := -1
	if appearance.NpcTransformationID != nil {
		npcID = *appearance.NpcTransformationID
	}
	if npcID >= 0 {
		short(0xffff)
		short(npcID)
	} else {
		for copy := 0; copy < 2; copy++ {
			for slot := 0; slot < 12; slot++ {
				value := equipmentSlot(slot)
				if value == 0 {
					push(0)
				} else {
					short(value)
				}
			}
		}
	}
	for index := 0; index < 5; index++ {
		push(atZero(appearance.Colors, index))
	}
	for index := 0; index < 7; index++ {
		animation := 0xffff
		if index < len(animations) {
			animation = animations[index]
		}
		short(animation)
	}
	text(name)
	push(combatLevel)
	short(skillLevel)
	if appearance.Hidden {
		push(1)
	} else {
		push(0)
	}
	short(0) // no colour/texture overrides
	text("")
	text("")
	text("")
	pronouns := appearance.Gender
	if appearance.Pronouns != nil {
		pronouns = *appearance.Pronouns
	}
	push(pronouns)
	int32be(atZero(appearance.EquipQty, 13))
	int32be(at(appearance.Equip, 13))
	int32be(at(appearance.Equip, 12))
	int32be(atZero(appearance.EquipQty, 3))
	return bytes
}
