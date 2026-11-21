package game

// BonusManager: the port of game/model/equipment/BonusManager.ts
// (100 lines) — attack/defence/other bonus computation from
// equipment.
//
// TS caches the three arrays on the player and recomputes them
// in BonusManager.update, called on equip/unequip, login,
// level changes, prayer changes and autocast changes
// (BonusManager.ts callers: Player.ts:281, SkillManager.ts:278,
// :392, EquipPacketListener.ts:235, :274, Bank.ts:1228,
// CombatFactory.ts:1853, PrayerHandler.ts:419,
// Autocasting.ts:278, NetworkBuilder.ts:1053, :1154). Go does
// NOT cache: the combat formulas read bonuses through the
// Combatant interface (AttackBonus/DefenceBonus/OtherBonus),
// and playerCombatant.equipmentBonusSum computes them live
// from ComputeBonuses, so every formula call site reads the
// same values TS's cached arrays hold, with no refresh call
// to miss. The equip/unequip call sites therefore carry a
// note instead of an update call (see listeners/equip.go).
// BonusManager itself is the canonical computation, kept as a
// value so callers that want a snapshot (tests, future
// content) can hold one.
//
// The 14-entry layout (BonusManager.ts BONUS_COUNT = 14):
// 0..4 attack (stab/slash/crush/magic/range), 5..9 defence
// (same order), 10 melee strength, 11 ranged strength, 12
// magic damage percent, 13 prayer.
//
// Deltas from a plain equipment sum, all ported or named:
//   - emitCanUseItem veto ("bonus"): plugins can switch an
//     item's stats off (members items on a free-to-play
//     world). There is no Go hub surface for it (the
//     onCanUseItem family exists in the hub; no game-side
//     seam reads it), so the veto is a BonusOptions hook:
//     nil allows every item, which is the no-plugins TS
//     behavior (the emit returns null → counts).
//   - Ammo-slot ranged strength: a thrown or self-ammo weapon
//     does not fire the ammo slot, so the ammo's ranged
//     strength does not count (BonusManager.ts comment). The
//     predicate is firesFromAmmoSlot (RangedData.ts:317-322);
//     see bonusFiresFromAmmoSlot for the ported branches.
//   - Crystal bow attack/strength override (BonusManager.ts):
//     NOT PORTED — crystal-bow special cases are item-specific
//     content, out of scope for Phase 26 (PHASE26_PLAN.md).
//     A crystal bow reads its plain definition bonuses here.
//   - applyBonusProviders (PluginManager): NOT PORTED —
//     registerBonusProvider is table-only in the Go hub
//     (plugins/hooks.go), with no game-side seam; no provider
//     can register, so the no-op is exact.

// BonusManager holds the computed attack, defence and other
// bonus arrays (BonusManager.ts attackBonus/defenceBonus/
// otherBonus). The zero value reads as naked (all zeros).
type BonusManager struct {
	attack  [5]int
	defence [5]int
	other   [4]int
}

// AttackBonus returns the attack bonus at index
// (BonusAttackStab..BonusAttackRange); out of range reads 0.
func (b *BonusManager) AttackBonus(index int) int {
	if b == nil || index < 0 || index >= len(b.attack) {
		return 0
	}
	return b.attack[index]
}

// DefenceBonus returns the defence bonus at index; out of
// range reads 0.
func (b *BonusManager) DefenceBonus(index int) int {
	if b == nil || index < 0 || index >= len(b.defence) {
		return 0
	}
	return b.defence[index]
}

// OtherBonus returns the other bonus at index
// (OtherBonusStrength..OtherBonusPrayer); out of range reads 0.
func (b *BonusManager) OtherBonus(index int) int {
	if b == nil || index < 0 || index >= len(b.other) {
		return 0
	}
	return b.other[index]
}

// AttackBonuses returns a copy of the attack bonus array.
func (b *BonusManager) AttackBonuses() [5]int {
	if b == nil {
		return [5]int{}
	}
	return b.attack
}

// DefenceBonuses returns a copy of the defence bonus array.
func (b *BonusManager) DefenceBonuses() [5]int {
	if b == nil {
		return [5]int{}
	}
	return b.defence
}

// OtherBonuses returns a copy of the other bonus array.
func (b *BonusManager) OtherBonuses() [4]int {
	if b == nil {
		return [4]int{}
	}
	return b.other
}

// Bonus returns the flat 14-entry bonus array in TS order
// (0..4 attack, 5..9 defence, 10..13 other).
func (b *BonusManager) Bonus() [14]int {
	var out [14]int
	if b == nil {
		return out
	}
	copy(out[0:5], b.attack[:])
	copy(out[5:10], b.defence[:])
	copy(out[10:14], b.other[:])
	return out
}

// BonusOptions tunes ComputeBonuses. A nil options value, or
// a nil hook, takes the no-plugins default.
type BonusOptions struct {
	// CanUseItem vetoes one item's stats (PluginManager
	// .emitCanUseItem(player, itemId, "bonus") !== false).
	// Nil allows every item.
	CanUseItem func(itemID int) bool
	// FiresFromAmmoSlot overrides the ammo-slot predicate
	// (RangedData.Ammunition.firesFromAmmoSlot). Nil uses
	// bonusFiresFromAmmoSlot.
	FiresFromAmmoSlot func(p *Player) bool
}

// ComputeBonuses computes the equipment bonuses for p
// (BonusManager.update, BonusManager.ts:52-89): the 14-entry
// sum over the worn items, minus the ammo slot's ranged
// strength when the weapon does not fire from the ammo slot.
// A nil player, no equipment, or no bonus source reads as
// naked (all zeros), matching the playerCombatant behavior
// for a bare test player. Item bonus arrays shorter than 14
// contribute only their present entries (TS clamps to
// min(length, 14)); a nil definition bonus array contributes
// nothing.
func ComputeBonuses(p *Player, opts *BonusOptions) *BonusManager {
	out := &BonusManager{}
	if p == nil {
		return out
	}
	eq := p.Equipment()
	if eq == nil {
		return out
	}
	var src ItemBonusSource
	if defs := p.ItemDefinitions(); defs != nil {
		src, _ = defs.(ItemBonusSource)
	}
	if src == nil {
		return out
	}
	counts := func(itemID int) bool {
		return opts == nil || opts.CanUseItem == nil || opts.CanUseItem(itemID)
	}
	var bonuses [14]float64
	for _, item := range eq.Items() {
		if !item.IsValid() || !counts(item.ID()) {
			continue
		}
		for i, v := range src.ItemBonuses(item.ID()) {
			if i >= len(bonuses) {
				break
			}
			bonuses[i] += v
		}
	}
	// A thrown or self-ammo weapon doesn't fire the ammo slot,
	// so its ranged strength doesn't count (BonusManager.ts
	// comment: a dragon knife with dragon arrows equipped gets
	// the knife's +30 only).
	ammo := eq.ItemAt(AmmunitionSlot)
	if ammo.IsValid() && counts(ammo.ID()) && !bonusFiresFromAmmoSlot(p, opts) {
		if ammoBonuses := src.ItemBonuses(ammo.ID()); len(ammoBonuses) > 11 {
			bonuses[11] -= ammoBonuses[11]
		}
	}
	for i := 0; i < len(bonuses); i++ {
		v := int(bonuses[i])
		switch {
		case i <= 4:
			out.attack[i] = v
		case i <= 9:
			out.defence[i-5] = v
		default:
			out.other[i-10] = v
		}
	}
	return out
}

// bonusFiresFromAmmoSlot is Ammunition.firesFromAmmoSlot
// (RangedData.ts:317-322) as far as Go can evaluate it:
//   - a self-ammo weapon (RangedWeapon.UsesWeaponSlotAmmo —
//     thrown weapons and self-charging bows) does not fire
//     from the ammo slot;
//   - the crystal-bow branch (isCrystalBow) is NOT PORTED
//     (item-specific content, Phase 26 out of scope);
//   - the plugin-ammo branch (resolveRangedAmmunition) is NOT
//     PORTED (no plugin ammo system);
//   - the rangedAmmunition alias lookup is Worker A's ranged
//     data (NOT STARTED): without it a weapon not known to be
//     self-ammo reads as firing from the ammo slot, so the
//     ammo's ranged strength counts — the TS result for every
//     non-thrown weapon, and the documented gap for thrown
//     weapons until the alias table lands.
func bonusFiresFromAmmoSlot(p *Player, opts *BonusOptions) bool {
	if opts != nil && opts.FiresFromAmmoSlot != nil {
		return opts.FiresFromAmmoSlot(p)
	}
	if p == nil {
		return true
	}
	if weapon := p.GetCombat().RangedWeapon(); weapon != nil && weapon.UsesWeaponSlotAmmo {
		return false
	}
	return true
}
