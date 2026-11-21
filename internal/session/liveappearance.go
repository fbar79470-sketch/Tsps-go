package session

import (
	"tsps-go/internal/persist"
	"tsps-go/internal/protocol"
)

// This file ports the live appearance-block construction in
// PlayerSession.createPlayerView (PlayerSession.ts:662) as a
// plain-data builder: the cmd/server adapter rebuilds a player's
// stored sync appearance block from LIVE state (look + equipment +
// skills) whenever equipment changes, without the session package
// importing game. The bootstrap-time construction
// (bootstrapAppearance, bootstrap.go) stays as-is; this builder
// only refreshes the block AFTER entry.
//
// The builder reuses the bootstrap's derivation rules exactly:
// sanitizeLook for the look array, the same Colors/Kits/Equip/
// EquipQty layout, and protocol.EncodePlayerAppearance. A
// regression test (liveappearance_test.go) proves that building
// from save-derived inputs produces a byte-identical block to the
// bootstrap path for the same save.

// EquippedItem is one worn equipment slot as the appearance block
// needs it: the item id (-1 when the slot is empty) and the amount.
// It mirrors persist.Item's ID/Amount fields without importing a
// container type; the adapter fills it from the live equipment
// container (game.Item ID()/Amount()).
type EquippedItem struct {
	ID     int
	Amount int
}

// AppearanceInput is everything BuildAppearanceBlock needs, as
// plain data. All values are already resolved by the caller:
//   - Look: the 13-entry look array (sanitized by the builder with
//     sanitizeLook, as the bootstrap sanitizes the save's array).
//     Short arrays read as the gender-appropriate defaults.
//   - Equipment: worn items in equipment-slot order (slot 0 = head
//     .. slot 13 = ammunition, 14 slots). Slots past
//     persist.EquipmentSize are ignored; missing slots read as
//     empty (-1, amount 0).
//   - DisplayName: the name other players see (getDisplayName:
//     display name falling back to username).
//   - CombatLevel/TotalLevel: the live skill totals, as
//     createPlayerView passes getCombatLevel()/getTotalLevel().
//   - Animations: the 7 stand/turn/walk/run sequence ids
//     (Player.SyncAnimations); short arrays read as 0xffff per
//     EncodePlayerAppearance.
//   - SkullIcon: the head skull icon id, or -1 for none. The
//     rebuild reads it live from the player (HeadSkullIcon),
//     so a skull gained or lost mid-session shows on the next
//     appearance rebuild (phase 28); the bootstrap seeds the
//     entry-time value from the save.
//   - Pronouns: the resolved pronouns value (0..2), already
//     through the gender fallback (Appearance.Pronouns). The
//     builder does not re-derive it.
//   - HeadHint: the prayer head hint (Appearance.HeadHint), -1
//     when none.
//   - Hidden: whether the appearance is hidden. Always false in
//     Go (no appearance-hidden state); carried so the builder
//     matches createPlayerView's shape if that state ever lands.
//
// NOT carried: the NPC transformation id (no transformation state;
// always absent, as at bootstrap) and displayed-equipment
// overrides (no displayed-equipment state; Equipment is what is
// worn).
type AppearanceInput struct {
	Look        []int
	Equipment   []EquippedItem
	DisplayName string
	CombatLevel int
	TotalLevel  int
	Animations  []int
	SkullIcon   int
	Pronouns    int
	HeadHint    int
	Hidden      bool
}

// BuildAppearanceBlock encodes one player's sync appearance block
// from live plain-data inputs, porting the encodePlayerAppearance
// call in createPlayerView (PlayerSession.ts:700-733) over the
// bootstrap's derivation rules (getPlayerAppearance,
// NetworkBuilder.ts:1885-1912, as bootstrapAppearance ports it).
// It returns the raw block, NOT a framed packet, exactly as
// EncodePlayerAppearance does; the caller stores it with
// SetSyncAppearance.
func BuildAppearanceBlock(in AppearanceInput) []byte {
	look := sanitizeLook(in.Look)
	gender := look[lookGender]
	pronouns := in.Pronouns
	equipIDs := make([]int, persist.EquipmentSize)
	equipQty := make([]int, persist.EquipmentSize)
	for i := range equipIDs {
		equipIDs[i] = -1
		if i < len(in.Equipment) {
			equipIDs[i] = in.Equipment[i].ID
			equipQty[i] = in.Equipment[i].Amount
		}
	}
	appearance := protocol.PlayerAppearance{
		Gender:   gender,
		Hidden:   in.Hidden,
		Pronouns: &pronouns,
		Colors:   []int{look[lookHairColour], look[lookTorsoCol], look[lookLegColour], look[lookFeetColour], look[lookSkinColour]},
		Kits:     []int{look[lookHead], look[lookBeard], look[lookChest], look[lookArms], look[lookHands], look[lookLegs], look[lookFeet]},
		Equip:    equipIDs,
		EquipQty: equipQty,
		HeadIcons: &protocol.HeadIcons{
			Skull:  in.SkullIcon,
			Prayer: in.HeadHint,
		},
	}
	return protocol.EncodePlayerAppearance(appearance, in.DisplayName, in.CombatLevel, in.TotalLevel, in.Animations)
}
