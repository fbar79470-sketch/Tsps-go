package game

// Weapon profiles: the attack-animation layer of
// game/content/combat/WeaponProfile.ts (450 lines).
//
// TS WeaponProfiles keeps two registries — byItem (register) and
// byWeapon (add) — and resolves attackAnimation in this order
// (WeaponProfile.ts:105-119, the attackAnimation doc comment):
//   1. the profile registered for the wielded item id
//      (ItemAliases.lookup on byItem),
//   2. the item's own data (ItemDefinition.getAttackAnim,
//      ItemDefinition.ts:308-311),
//   3. the profile for the wielded weapon's type (byWeapon),
//   4. the fallback (the fight style's animation against the
//      target kind, FightType.getAnimationAgainst).
// Against an NPC, a profile's npcAttackAnimation wins over its
// attackAnimation at the same layer
// (WeaponProfile.ts:115-116); against a player only
// attackAnimation applies. The callers are the combat methods
// (MeleeCombatMethod.ts:10, RangedCombatMethod.ts:133, and
// MagicCombatMethod.ts:268 via character.getAttackAnim) through
// Player.getAttackAnim (Player.ts:333-338), which passes the
// resolved fight type's animation as the fallback.
//
// Only the animation fields are ported here. The rest of the
// profile (attackSpeed, attackDistance, hitDelays, projectiles,
// sounds, ammo) is already mirrored where the engine needs it
// — the ranged data tables (ranged_data.go,
// ranged_data_tables.go) and the combat-style registry — and
// porting those fields here would fork the tables; they stay
// where they are. This file is the registry + the two profile
// layers (1 and 3); layer 2 is the existing
// WeaponInfoSource.WeaponAttackAnim read, and layer 4 is
// FightType.AnimationAgainst. playerAttackAnim
// (mobile_animation.go) applies the four layers in TS order.
//
// An unset animation is -1, never 0: TS profiles carry
// undefined for a missing field and fall through with ??, so a
// profile that sets only attackSpeed (KARILS_CROSSBOW,
// THROWNAXE, HALBERD, SALAMANDER, BALLISTA, OBBY_RINGS) or only
// a long-range style (VENATOR_BOW) contributes no animation
// here and the later layers decide.
//
// DEVIATION — ItemAliases: TS layer 1 looks the item up through
// ItemAliases.lookup, so an alias copy (an LMS br_ item) reads
// its base item's profile. Go has no ItemAliases port (see
// autocasting.go / combat_spells.go), so layer 1 here is an
// exact-id lookup; an alias copy falls through to layers 2-4
// instead of its base's profile. The ids below are the base ids
// from WeaponProfile.ts, resolved through ItemIdentifiers.ts
// and CrystalBow.ts (CRYSTAL_BOW_ALL_WEAPON_IDS,
// BOW_OF_FAERDHINEN_IDS).
//
// DEVIATION — layer 3 reach: the weapon-type layer resolves
// through CurrentWeaponInterface, which reads the
// weaponInterface name from item-gameplay.json. Thrown-ammo
// items (javelins: equipmentType ARROWS, e.g. 825/830) carry no
// weaponInterface in that export, so their JAVELIN profile
// entry is unreachable in Go until the interface seam covers
// them — the same pre-existing seam gap that makes
// AssignWeaponInterface read UNARMED for a javelin wielder.
// The entry is still registered, so it fires as soon as the
// seam does.

// WeaponProfile is one WeaponCombatProfile's attack-animation
// layer (WeaponProfile.ts:19-23): attackAnimation and the
// cache's separate animation against NPCs
// (npcAttackAnimation). Either field is -1 when the profile
// does not set it.
type WeaponProfile struct {
	// AttackAnimation is the profile's attackAnimation, or -1.
	AttackAnimation int
	// NPCAttackAnimation is the profile's npcAttackAnimation,
	// or -1.
	NPCAttackAnimation int
}

// animationAgainst returns the profile's animation for an
// attack on the given target kind (WeaponProfile.ts:115-116):
// the NPC animation against an NPC when the profile sets one,
// else the attack animation. -1 when the profile sets neither
// (the caller falls through to the next layer).
func (p WeaponProfile) animationAgainst(againstNPC bool) int {
	if againstNPC && p.NPCAttackAnimation > 0 {
		return p.NPCAttackAnimation
	}
	if p.AttackAnimation > 0 {
		return p.AttackAnimation
	}
	return -1
}

// weaponProfile is the shared constructor: -1 for an unset
// field, as TS leaves it undefined.
func weaponProfile(attackAnim, npcAttackAnim int) WeaponProfile {
	return WeaponProfile{AttackAnimation: attackAnim, NPCAttackAnimation: npcAttackAnim}
}

// weaponProfilesByWeapon is WeaponProfiles.byWeapon
// (WeaponProfile.ts:62, 176-294): the profile per weapon
// interface, keyed by the item-combat-styles.json key
// (WeaponInterfaces.ts). Only interfaces with an
// attackAnimation entry appear: SHORTBOW and LONGBOW share the
// `bow` object (attackAnimation 426, WeaponProfile.ts:182-191),
// CROSSBOW 4230 / npc 7552 (:200-201), BLOWPIPE 5061 (:222),
// KNIFE 806 (:233), DART 806 (:243), JAVELIN 929 (:253),
// WHIP 1658 (:277), DARK_BOW 426 (:291).
var weaponProfilesByWeapon = map[string]WeaponProfile{
	"SHORTBOW": weaponProfile(426, -1),
	"LONGBOW":  weaponProfile(426, -1),
	"CROSSBOW": weaponProfile(4230, 7552),
	"BLOWPIPE": weaponProfile(5061, -1),
	"KNIFE":    weaponProfile(806, -1),
	"DART":     weaponProfile(806, -1),
	"JAVELIN":  weaponProfile(929, -1),
	"WHIP":     weaponProfile(1658, -1),
	"DARK_BOW": weaponProfile(426, -1),
}

// crystalBowProfileIDs are CRYSTAL_BOW_ALL_WEAPON_IDS
// (CrystalBow.ts:35-44): the empty bows 4212, 11748, 23985,
// the ten charged stages with their (i) variants
// 4214-4223 / 11749-11758, and the static bows 23983, 24123.
var crystalBowProfileIDs = []int{
	4212, 11748, 23985,
	4214, 11749, 4215, 11750, 4216, 11751, 4217, 11752,
	4218, 11753, 4219, 11754, 4220, 11755, 4221, 11756,
	4222, 11757, 4223, 11758,
	23983, 24123,
}

// faerdhinenProfileIDs are BOW_OF_FAERDHINEN_IDS
// (CrystalBow.ts:47-60): 25865, 27187, 25867, 25884, 25886,
// 25888, 25890, 25892, 25894, 25896, 33021.
var faerdhinenProfileIDs = []int{
	25865, 27187, 25867, 25884, 25886,
	25888, 25890, 25892, 25894, 25896, 33021,
}

// weaponProfilesByItem is WeaponProfiles.byItem
// (WeaponProfile.ts:61, 300-431): the profile per item id.
// Every register call with an attackAnimation entry, in file
// order:
//   - crystal bows (426, :303-311), bows of Faerdhinen (426,
//     :312-319), twisted bow 20997 (426, :320-327), scorching
//     bow 29591 (426, :342-349),
//   - 21902 (4230 / npc 7552, :351-360), ascension crossbows
//     34681 (15113 / npc 15114, :363-377),
//   - comp ogre + composite bows 4827, 10280, 10282, 10284
//     (426, :391-399), 3rd age bow 12424 (426, :402-410),
//   - black chinchompa 11959 (7618, :424), blisterwood stake
//     33716 (7617 / npc 7617, :428-437).
//
// Registers without an attackAnimation — venator bow, the
// regular/red chinchompas, wands (attackSpeed only), blackjacks
// (attackSpeed only) — set no animation and are not listed.
var weaponProfilesByItem = func() map[int]WeaponProfile {
	profiles := make(map[int]WeaponProfile)
	bow := weaponProfile(426, -1)
	for _, id := range crystalBowProfileIDs {
		profiles[id] = bow
	}
	for _, id := range faerdhinenProfileIDs {
		profiles[id] = bow
	}
	profiles[20997] = bow // Twisted bow.
	profiles[29591] = bow // Scorching bow.
	profiles[21902] = weaponProfile(4230, 7552)
	profiles[34681] = weaponProfile(15113, 15114) // Ascension crossbows.
	for _, id := range []int{4827, 10280, 10282, 10284} {
		profiles[id] = bow // Comp ogre + composite bows.
	}
	profiles[12424] = bow                       // 3rd Age bow.
	profiles[11959] = weaponProfile(7618, -1)   // Black chinchompa.
	profiles[33716] = weaponProfile(7617, 7617) // Blisterwood stake.
	return profiles
}()

// WeaponProfileForItem returns the profile registered for an
// item id (layer 1, WeaponProfile.ts:111-119 via
// ItemAliases.lookup on byItem), or nil when none is
// registered. Exact-id only: no ItemAliases port exists in Go
// (see the file comment).
func WeaponProfileForItem(id int) *WeaponProfile {
	if p, ok := weaponProfilesByItem[id]; ok {
		profile := p
		return &profile
	}
	return nil
}

// WeaponProfileForWeapon returns the profile for a weapon
// interface name (layer 3, WeaponProfile.ts:111-119 via
// byWeapon), or nil when the interface has no profile entry
// or no attackAnimation. The name is the
// item-combat-styles.json key ("SHORTBOW", "WHIP", ...).
func WeaponProfileForWeapon(name string) *WeaponProfile {
	if p, ok := weaponProfilesByWeapon[name]; ok {
		profile := p
		return &profile
	}
	return nil
}

// weaponProfileAttackAnim applies one profile layer for an
// attack on the given target kind: the profile's animation, or
// -1 when the profile is nil or sets no animation for that
// kind (the caller falls through to the next layer).
func weaponProfileAttackAnim(profile *WeaponProfile, againstNPC bool) int {
	if profile == nil {
		return -1
	}
	return profile.animationAgainst(againstNPC)
}
