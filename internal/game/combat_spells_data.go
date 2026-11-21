package game

// Standard-spellbook and utility-spell tables, script-extracted
// (brace-counted, verbatim) from upstream CombatSpells.ts (2,116
// lines) and EffectSpells.ts (268 lines) at commit bfd81e3. The
// extraction was verified by count + spot-checks (see
// combat_spells_test.go): upstream holds 62 public spell statics
// (36 standard + 10 Arceuus + 16 Ancients); this file holds the 36
// standard ones plus Charge's data in combat_spells.go. Ancients
// and Arceuus are separate books and are NOT here. Animation,
// start-graphic, projectile and end-graphic ids are read by the
// Phase 29 producers (combat_spells.go startSpellCast,
// combat_magic.go magicImpactPresentation); spell code reads only
// the id, level, max hit, XP, runes, equipment and flags.
//
// XP is the TS float, verbatim (5.5 etc.). The CombatSpell
// interface takes ints, so BaseExperience/SplashExperience round
// to nearest (see spellXP, combat_spells.go): Go stores integer
// experience and cannot keep tenths (skills.go:517-523).
//
// A -1 animation/graphic/projectile id means TS returns null or
// undefined there (god spells have no start graphic or
// projectile; Magic Dart has no start graphic; Tele Block has
// neither and sends its projectile from onHitCalc).

// spellRune is one rune cost: a TS `new Item(id, amount)` in a
// spell's itemsRequired (amount 1 when TS omits it).
type spellRune struct {
	id     int
	amount int
}

// spellEquipmentKind names how a spell's equipmentRequired is
// decided (CombatSpells.ts). none: TS returns null/undefined.
// godSaradomin/godGuthix/godZamorak: godStaffRequired(player,
// the matching set below). iban: the Iban's-staff ternary
// (IBAN_BLAST_STAVES).
type spellEquipmentKind int

const (
	spellEquipmentNone spellEquipmentKind = iota
	spellEquipmentGodSaradomin
	spellEquipmentGodGuthix
	spellEquipmentGodZamorak
	spellEquipmentIban
)

// standardSpellDef is one CombatSpells.ts standard static: the
// CombatNormalSpell / CombatEffectSpell options object, flattened.
// kind is "normal" or "effect" (the TS class). godDrain names
// the inline finishCast drain ("", "prayer", "defence",
// "magic"); crumbleUndead marks the canCastOnTarget override;
// teleblock marks the TeleBlockSpell inline spellEffect.
type standardSpellDef struct {
	name                     string
	kind                     string
	id, level, maxHit        int
	baseXP, splashXP         float64
	runes                    []spellRune
	castAnim                 int
	startGraphic, projectile int
	endGraphic               int
	members                  bool
	missingEquipmentMessage  string
	equipment                spellEquipmentKind
	crumbleUndead            bool
	godDrain                 string
	teleblock                bool
}

// standardSpellDefs is every standard-book static in upstream
// declaration order (CombatSpells.ts:407-1719).
var standardSpellDefs = []standardSpellDef{
	{
		name: "WIND_STRIKE", kind: "normal", id: 1152, level: 1, maxHit: 2,
		baseXP: 5.5, splashXP: 5.5, runes: []spellRune{{556, 1}, {558, 1}},
		castAnim: 711, startGraphic: 90, projectile: 91, endGraphic: 92,
		members: false, missingEquipmentMessage: "", equipment: spellEquipmentNone,
		crumbleUndead: false, godDrain: "", teleblock: false,
	},
	{
		name: "CONFUSE", kind: "effect", id: 1153, level: 3, maxHit: 0,
		baseXP: 13.0, splashXP: 13.0, runes: []spellRune{{555, 3}, {557, 2}, {559, 1}},
		castAnim: 716, startGraphic: 102, projectile: 103, endGraphic: 104,
		members: false, missingEquipmentMessage: "", equipment: spellEquipmentNone,
		crumbleUndead: false, godDrain: "", teleblock: false,
	},
	{
		name: "WATER_STRIKE", kind: "normal", id: 1154, level: 5, maxHit: 4,
		baseXP: 7.5, splashXP: 7.5, runes: []spellRune{{555, 1}, {556, 1}, {558, 1}},
		castAnim: 711, startGraphic: 93, projectile: 94, endGraphic: 95,
		members: false, missingEquipmentMessage: "", equipment: spellEquipmentNone,
		crumbleUndead: false, godDrain: "", teleblock: false,
	},
	{
		name: "EARTH_STRIKE", kind: "normal", id: 1156, level: 9, maxHit: 6,
		baseXP: 9.5, splashXP: 9.5, runes: []spellRune{{556, 1}, {558, 1}, {557, 2}},
		castAnim: 711, startGraphic: 96, projectile: 97, endGraphic: 98,
		members: false, missingEquipmentMessage: "", equipment: spellEquipmentNone,
		crumbleUndead: false, godDrain: "", teleblock: false,
	},
	{
		name: "WEAKEN", kind: "effect", id: 1157, level: 11, maxHit: 0,
		baseXP: 21.0, splashXP: 21.0, runes: []spellRune{{555, 3}, {557, 2}, {559, 1}},
		castAnim: 717, startGraphic: 105, projectile: 106, endGraphic: 107,
		members: false, missingEquipmentMessage: "", equipment: spellEquipmentNone,
		crumbleUndead: false, godDrain: "", teleblock: false,
	},
	{
		name: "FIRE_STRIKE", kind: "normal", id: 1158, level: 13, maxHit: 8,
		baseXP: 11.5, splashXP: 11.5, runes: []spellRune{{556, 2}, {558, 1}, {554, 3}},
		castAnim: 711, startGraphic: 99, projectile: 100, endGraphic: 101,
		members: false, missingEquipmentMessage: "", equipment: spellEquipmentNone,
		crumbleUndead: false, godDrain: "", teleblock: false,
	},
	{
		name: "WIND_BOLT", kind: "normal", id: 1160, level: 17, maxHit: 9,
		baseXP: 13.5, splashXP: 13.5, runes: []spellRune{{556, 2}, {562, 1}},
		castAnim: 711, startGraphic: 117, projectile: 118, endGraphic: 119,
		members: false, missingEquipmentMessage: "", equipment: spellEquipmentNone,
		crumbleUndead: false, godDrain: "", teleblock: false,
	},
	{
		name: "CURSE", kind: "effect", id: 1161, level: 19, maxHit: 0,
		baseXP: 29.0, splashXP: 29.0, runes: []spellRune{{555, 2}, {557, 3}, {559, 1}},
		castAnim: 718, startGraphic: 108, projectile: 109, endGraphic: 110,
		members: false, missingEquipmentMessage: "", equipment: spellEquipmentNone,
		crumbleUndead: false, godDrain: "", teleblock: false,
	},
	{
		name: "BIND", kind: "effect", id: 1572, level: 20, maxHit: 0,
		baseXP: 31.0, splashXP: 30.0, runes: []spellRune{{555, 3}, {557, 3}, {561, 2}},
		castAnim: 710, startGraphic: 177, projectile: 178, endGraphic: 181,
		members: false, missingEquipmentMessage: "", equipment: spellEquipmentNone,
		crumbleUndead: false, godDrain: "", teleblock: false,
	},
	{
		name: "WATER_BOLT", kind: "normal", id: 1163, level: 23, maxHit: 10,
		baseXP: 16.5, splashXP: 16.5, runes: []spellRune{{556, 2}, {562, 1}, {555, 2}},
		castAnim: 711, startGraphic: 120, projectile: 121, endGraphic: 122,
		members: false, missingEquipmentMessage: "", equipment: spellEquipmentNone,
		crumbleUndead: false, godDrain: "", teleblock: false,
	},
	{
		name: "EARTH_BOLT", kind: "normal", id: 1166, level: 29, maxHit: 11,
		baseXP: 19.5, splashXP: 19.5, runes: []spellRune{{556, 2}, {562, 1}, {557, 3}},
		castAnim: 711, startGraphic: 123, projectile: 124, endGraphic: 125,
		members: false, missingEquipmentMessage: "", equipment: spellEquipmentNone,
		crumbleUndead: false, godDrain: "", teleblock: false,
	},
	{
		name: "FIRE_BOLT", kind: "normal", id: 1169, level: 35, maxHit: 12,
		baseXP: 22.5, splashXP: 22.5, runes: []spellRune{{556, 3}, {562, 1}, {554, 4}},
		castAnim: 711, startGraphic: 126, projectile: 127, endGraphic: 128,
		members: false, missingEquipmentMessage: "", equipment: spellEquipmentNone,
		crumbleUndead: false, godDrain: "", teleblock: false,
	},
	{
		name: "CRUMBLE_UNDEAD", kind: "normal", id: 1171, level: 39, maxHit: 15,
		baseXP: 24.5, splashXP: 24.5, runes: []spellRune{{556, 2}, {562, 1}, {557, 2}},
		castAnim: 724, startGraphic: 145, projectile: 146, endGraphic: 147,
		members: false, missingEquipmentMessage: "", equipment: spellEquipmentNone,
		crumbleUndead: true, godDrain: "", teleblock: false,
	},
	{
		name: "WIND_BLAST", kind: "normal", id: 1172, level: 41, maxHit: 13,
		baseXP: 25.5, splashXP: 25.5, runes: []spellRune{{556, 3}, {560, 1}},
		castAnim: 711, startGraphic: 132, projectile: 133, endGraphic: 134,
		members: false, missingEquipmentMessage: "", equipment: spellEquipmentNone,
		crumbleUndead: false, godDrain: "", teleblock: false,
	},
	{
		name: "WATER_BLAST", kind: "normal", id: 1175, level: 47, maxHit: 14,
		baseXP: 28.5, splashXP: 28.5, runes: []spellRune{{555, 3}, {556, 3}, {560, 1}},
		castAnim: 711, startGraphic: 135, projectile: 136, endGraphic: 137,
		members: false, missingEquipmentMessage: "", equipment: spellEquipmentNone,
		crumbleUndead: false, godDrain: "", teleblock: false,
	},
	{
		name: "IBAN_BLAST", kind: "normal", id: 1539, level: 50, maxHit: 25,
		baseXP: 30.0, splashXP: 30.0, runes: []spellRune{{560, 1}, {554, 5}},
		castAnim: 708, startGraphic: 87, projectile: 88, endGraphic: 89,
		members: true, missingEquipmentMessage: "", equipment: spellEquipmentIban,
		crumbleUndead: false, godDrain: "", teleblock: false,
	},
	{
		name: "SNARE", kind: "effect", id: 1582, level: 50, maxHit: 2,
		baseXP: 61.0, splashXP: 60.0, runes: []spellRune{{555, 4}, {557, 4}, {561, 3}},
		castAnim: 710, startGraphic: 177, projectile: 178, endGraphic: 180,
		members: false, missingEquipmentMessage: "", equipment: spellEquipmentNone,
		crumbleUndead: false, godDrain: "", teleblock: false,
	},
	{
		name: "MAGIC_DART", kind: "normal", id: 12037, level: 50, maxHit: 10,
		baseXP: 30.0, splashXP: 30.0, runes: []spellRune{{558, 4}, {560, 1}},
		castAnim: 1576, startGraphic: -1, projectile: 328, endGraphic: 329,
		members: true, missingEquipmentMessage: "", equipment: spellEquipmentNone,
		crumbleUndead: false, godDrain: "", teleblock: false,
	},
	{
		name: "EARTH_BLAST", kind: "normal", id: 1177, level: 53, maxHit: 15,
		baseXP: 31.5, splashXP: 31.5, runes: []spellRune{{556, 3}, {560, 1}, {557, 4}},
		castAnim: 711, startGraphic: 138, projectile: 139, endGraphic: 140,
		members: false, missingEquipmentMessage: "", equipment: spellEquipmentNone,
		crumbleUndead: false, godDrain: "", teleblock: false,
	},
	{
		name: "FIRE_BLAST", kind: "normal", id: 1181, level: 59, maxHit: 16,
		baseXP: 34.5, splashXP: 34.5, runes: []spellRune{{556, 4}, {560, 1}, {554, 5}},
		castAnim: 711, startGraphic: 129, projectile: 130, endGraphic: 131,
		members: false, missingEquipmentMessage: "", equipment: spellEquipmentNone,
		crumbleUndead: false, godDrain: "", teleblock: false,
	},
	{
		name: "SARADOMIN_STRIKE", kind: "normal", id: 1190, level: 60, maxHit: 20,
		baseXP: 35.0, splashXP: 35.0, runes: []spellRune{{556, 4}, {565, 2}, {554, 2}},
		castAnim: 811, startGraphic: -1, projectile: -1, endGraphic: 76,
		members: true, missingEquipmentMessage: "You must be wielding the Staff of Saradomin or the Staff of Light to cast this spell.", equipment: spellEquipmentGodSaradomin,
		crumbleUndead: false, godDrain: "prayer", teleblock: false,
	},
	{
		name: "CLAWS_OF_GUTHIX", kind: "normal", id: 1191, level: 60, maxHit: 20,
		baseXP: 35.0, splashXP: 35.0, runes: []spellRune{{556, 4}, {565, 2}, {554, 1}},
		castAnim: 811, startGraphic: -1, projectile: -1, endGraphic: 77,
		members: true, missingEquipmentMessage: "", equipment: spellEquipmentGodGuthix,
		crumbleUndead: false, godDrain: "defence", teleblock: false,
	},
	{
		name: "FLAMES_OF_ZAMORAK", kind: "normal", id: 1192, level: 60, maxHit: 20,
		baseXP: 35.0, splashXP: 35.0, runes: []spellRune{{556, 1}, {565, 2}, {554, 4}},
		castAnim: 811, startGraphic: -1, projectile: -1, endGraphic: 78,
		members: true, missingEquipmentMessage: "You must be wielding the Staff of Zamorak, Staff of the Dead, Thammaron's Sceptre, or the Accursed Sceptre to cast this spell.", equipment: spellEquipmentGodZamorak,
		crumbleUndead: false, godDrain: "magic", teleblock: false,
	},
	{
		name: "WIND_WAVE", kind: "normal", id: 1183, level: 62, maxHit: 17,
		baseXP: 36.0, splashXP: 36.0, runes: []spellRune{{556, 5}, {565, 1}},
		castAnim: 727, startGraphic: 158, projectile: 159, endGraphic: 160,
		members: true, missingEquipmentMessage: "", equipment: spellEquipmentNone,
		crumbleUndead: false, godDrain: "", teleblock: false,
	},
	{
		name: "WATER_WAVE", kind: "normal", id: 1185, level: 65, maxHit: 18,
		baseXP: 37.5, splashXP: 37.5, runes: []spellRune{{556, 5}, {565, 1}, {555, 7}},
		castAnim: 727, startGraphic: 161, projectile: 162, endGraphic: 163,
		members: true, missingEquipmentMessage: "", equipment: spellEquipmentNone,
		crumbleUndead: false, godDrain: "", teleblock: false,
	},
	{
		name: "VULNERABILITY", kind: "effect", id: 1542, level: 66, maxHit: 0,
		baseXP: 76.0, splashXP: 76.0, runes: []spellRune{{557, 5}, {555, 5}, {566, 1}},
		castAnim: 718, startGraphic: 167, projectile: 168, endGraphic: 169,
		members: true, missingEquipmentMessage: "", equipment: spellEquipmentNone,
		crumbleUndead: false, godDrain: "", teleblock: false,
	},
	{
		name: "EARTH_WAVE", kind: "normal", id: 1188, level: 70, maxHit: 19,
		baseXP: 40.0, splashXP: 40.0, runes: []spellRune{{556, 5}, {565, 1}, {557, 7}},
		castAnim: 727, startGraphic: 164, projectile: 165, endGraphic: 166,
		members: true, missingEquipmentMessage: "", equipment: spellEquipmentNone,
		crumbleUndead: false, godDrain: "", teleblock: false,
	},
	{
		name: "ENFEEBLE", kind: "effect", id: 1543, level: 73, maxHit: 0,
		baseXP: 83.0, splashXP: 83.0, runes: []spellRune{{557, 8}, {555, 8}, {566, 1}},
		castAnim: 728, startGraphic: 170, projectile: 171, endGraphic: 172,
		members: true, missingEquipmentMessage: "", equipment: spellEquipmentNone,
		crumbleUndead: false, godDrain: "", teleblock: false,
	},
	{
		name: "FIRE_WAVE", kind: "normal", id: 1189, level: 75, maxHit: 20,
		baseXP: 42.5, splashXP: 42.5, runes: []spellRune{{556, 5}, {565, 1}, {554, 7}},
		castAnim: 727, startGraphic: 155, projectile: 156, endGraphic: 157,
		members: true, missingEquipmentMessage: "", equipment: spellEquipmentNone,
		crumbleUndead: false, godDrain: "", teleblock: false,
	},
	{
		name: "WIND_SURGE", kind: "normal", id: 21876, level: 81, maxHit: 21,
		baseXP: 44.5, splashXP: 44.5, runes: []spellRune{{556, 7}, {21880, 1}},
		castAnim: 7855, startGraphic: 1455, projectile: 1456, endGraphic: 1457,
		members: true, missingEquipmentMessage: "", equipment: spellEquipmentNone,
		crumbleUndead: false, godDrain: "", teleblock: false,
	},
	{
		name: "WATER_SURGE", kind: "normal", id: 21877, level: 85, maxHit: 22,
		baseXP: 46.5, splashXP: 46.5, runes: []spellRune{{556, 7}, {555, 10}, {21880, 1}},
		castAnim: 7855, startGraphic: 1458, projectile: 1459, endGraphic: 1460,
		members: true, missingEquipmentMessage: "", equipment: spellEquipmentNone,
		crumbleUndead: false, godDrain: "", teleblock: false,
	},
	{
		name: "EARTH_SURGE", kind: "normal", id: 21878, level: 90, maxHit: 23,
		baseXP: 48.2, splashXP: 48.2, runes: []spellRune{{556, 7}, {557, 10}, {21880, 1}},
		castAnim: 7855, startGraphic: 1461, projectile: 1462, endGraphic: 1463,
		members: true, missingEquipmentMessage: "", equipment: spellEquipmentNone,
		crumbleUndead: false, godDrain: "", teleblock: false,
	},
	{
		name: "FIRE_SURGE", kind: "normal", id: 21879, level: 95, maxHit: 24,
		baseXP: 50.5, splashXP: 50.5, runes: []spellRune{{556, 7}, {554, 10}, {21880, 1}},
		castAnim: 7855, startGraphic: 1464, projectile: 1465, endGraphic: 1466,
		members: true, missingEquipmentMessage: "", equipment: spellEquipmentNone,
		crumbleUndead: false, godDrain: "", teleblock: false,
	},
	{
		name: "ENTANGLE", kind: "effect", id: 1592, level: 79, maxHit: 5,
		baseXP: 89.0, splashXP: 70.0, runes: []spellRune{{555, 5}, {557, 5}, {561, 4}},
		castAnim: 710, startGraphic: 177, projectile: 178, endGraphic: 179,
		members: true, missingEquipmentMessage: "", equipment: spellEquipmentNone,
		crumbleUndead: false, godDrain: "", teleblock: false,
	},
	{
		name: "STUN", kind: "effect", id: 1562, level: 80, maxHit: 0,
		baseXP: 90.0, splashXP: 90.0, runes: []spellRune{{557, 12}, {555, 12}, {566, 1}},
		castAnim: 729, startGraphic: 173, projectile: 174, endGraphic: 80,
		members: true, missingEquipmentMessage: "", equipment: spellEquipmentNone,
		crumbleUndead: false, godDrain: "", teleblock: false,
	},
	{
		name: "TELEBLOCK", kind: "effect", id: 12445, level: 85, maxHit: 0,
		baseXP: 95.0, splashXP: 84.0, runes: []spellRune{{563, 1}, {562, 1}, {560, 1}},
		castAnim: 1819, startGraphic: -1, projectile: -1, endGraphic: 345,
		members: false, missingEquipmentMessage: "", equipment: spellEquipmentNone,
		crumbleUndead: false, godDrain: "", teleblock: true,
	},
}

// God-staff sets (CombatSpells.ts:331-340, ids resolved through
// ItemIdentifiers.ts). godStaffRequired returns the wielded
// staff when it is in the set, else the set's first item, which
// the cast then reports missing.
var (
	godSaradominStaves = []int{2415, 22296}                             // Staff of Saradomin, Staff of Light.
	godGuthixStaves    = []int{2416, 8841, 24181, 24144}                // Guthix staff, void knight mace, void knight mace (l), Staff of Balance.
	godZamorakStaves   = []int{2417, 11791, 12902, 12904, 27788, 27679} // Zamorak staff, Staff of the Dead, toxic staff (uncharged) 12902, Toxic Staff of the Dead, Thammaron's Sceptre (a), Accursed Sceptre (a).
	ibanBlastStaves    = []int{1409, 12658, 33330, 33332}               // Iban's staff and its (u)/(o)/(u)(o) forms.
)

// godSpellCapes maps a god spell's id to the capes that raise its
// max hit to 30 while Charged (CombatSpells.ts:389-394,
// GOD_SPELL_CAPES; applyChargeMaxHit, CombatSpells.ts:399-407).
var godSpellCapes = map[int][]int{
	1190: []int{2412, 15615, 21791, 21792, 23607, 23608},
	1191: []int{2413, 15616, 21793, 21794, 23603, 23604},
	1192: []int{2414, 15617, 21795, 21796, 23605, 23606},
}

// staffCastAnimations is CombatSpell.ts:21-25
// (STAFF_CAST_ANIMATIONS): OSRS casts with a staff variant while
// a staff is wielded. Recorded as data only: no performAnimation
// exists yet (Phase 29), so StartCast does not play them.
var staffCastAnimations = map[int]int{
	710:  1161,
	711:  1162,
	716:  1163,
	717:  1164,
	718:  1165,
	724:  1166,
	727:  1167,
	728:  1168,
	729:  1169,
	1819: 1820,
}

// effectSpellEntry is one EffectSpells.ts static: id, level, base
// experience, runes, spellbook and members flag. baseXP is the TS
// third constructor argument, VERBATIM — including values TS never
// pays as XP: Low Alchemy 4000, Superheat Item 6544, High Alchemy
// 20000, Bake Pie 5121 and Vengeance Other 10000. The alchemy and
// Telekinetic Grab handlers pay their own constants instead
// (MagicOnItemPacketListener.ts:35-36, 53: LOW_ALCH_XP 31,
// HIGH_ALCH_XP 65, TELEKINETIC_GRAB_XP 43); only Bones to
// Bananas/Peaches (25 / 35.5) and Vengeance (112) pay
// baseExperience() (EffectSpells.handleSpell). Do not read these
// five numbers as XP. aliases are the extra ids the map static
// block registers for the spell (3280, 6884).
type effectSpellEntry struct {
	name      string
	id        int
	level     int
	baseXP    float64
	runes     []spellRune
	spellbook Spellbook
	members   bool
	aliases   []int
}

// effectSpellEntries is every EffectSpells static in upstream
// declaration order (EffectSpells.ts:138-211).
var effectSpellEntries = []effectSpellEntry{
	{name: "BONES_TO_BANANAS", id: 1159, level: 15, baseXP: 25.0, runes: []spellRune{{561, 1}, {555, 2}, {557, 2}}, spellbook: SpellbookNormal, members: false, aliases: []int{3280}},
	{name: "LOW_ALCHEMY", id: 1162, level: 21, baseXP: 4000.0, runes: []spellRune{{554, 3}, {561, 1}}, spellbook: SpellbookNormal, members: false, aliases: nil},
	{name: "TELEKINETIC_GRAB", id: 1168, level: 33, baseXP: 43.0, runes: []spellRune{{563, 1}, {556, 1}}, spellbook: SpellbookNormal, members: false, aliases: nil},
	{name: "SUPERHEAT_ITEM", id: 1173, level: 43, baseXP: 6544.0, runes: []spellRune{{554, 4}, {561, 1}}, spellbook: SpellbookNormal, members: false, aliases: nil},
	{name: "HIGH_ALCHEMY", id: 1178, level: 55, baseXP: 20000.0, runes: []spellRune{{554, 5}, {561, 1}}, spellbook: SpellbookNormal, members: false, aliases: nil},
	{name: "BONES_TO_PEACHES", id: 15877, level: 60, baseXP: 35.5, runes: []spellRune{{561, 2}, {555, 4}, {557, 4}}, spellbook: SpellbookNormal, members: true, aliases: []int{6884}},
	{name: "BAKE_PIE", id: 9107, level: 65, baseXP: 5121.0, runes: []spellRune{{9075, 1}, {554, 5}, {555, 4}}, spellbook: SpellbookLunar, members: false, aliases: nil},
	{name: "VENGEANCE_OTHER", id: 9135, level: 93, baseXP: 10000.0, runes: []spellRune{{9075, 3}, {557, 10}, {560, 2}}, spellbook: SpellbookLunar, members: false, aliases: nil},
	{name: "VENGEANCE", id: 9136, level: 94, baseXP: 112.0, runes: []spellRune{{9075, 4}, {557, 10}, {560, 2}}, spellbook: SpellbookLunar, members: false, aliases: nil},
}

// effectSpellFruitBones is EffectSpells.FRUIT_BONES
// (EffectSpells.ts:19-21): the bones Bones to Bananas and Bones to
// Peaches turn into fruit (unnoted only).
var effectSpellFruitBones = []int{526, 528, 530, 532, 2859, 3125, 3183, 25422, 30973}
