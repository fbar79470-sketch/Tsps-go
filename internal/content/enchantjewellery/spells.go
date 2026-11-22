// Enchant Jewellery: the Lvl-1 to Lvl-7 Enchant spells
// (Spells.EnchantJewellery.js, 285 lines, commit bfd81e3).
// One cast enchants the first piece of that jewellery in the
// inventory, in its own slot, then repeats every 7 ticks while
// more of it is carried; a cast clicked during the 3-tick delay
// waits for it. A level-up pop-up pauses the repeat in TS —
// Go has no SkillLevelUp emitter, so the repeat continues
// through a level-up here (residual, see enchantjewellery.go).
//
// This file is the ENCHANT_SPELLS table
// (Spells.EnchantJewellery.js:162-259), transcribed row by row.
// There is NO data file for it. Item ids are the
// ItemIdentifiers.ts values the TS table names (verified
// against that file, 2026-11-21); the spell ids are the cache
// spell widget ids (TS JewellerySpell.id = widget.widgetId),
// probe-verified against the real rev-242 cache with
// GetSpellByName (since deleted probe test): Lvl-1 14286864,
// Lvl-2 14286875, Lvl-3 14286888, Lvl-4 14286897, Lvl-5
// 14286912, Lvl-6 14286925, Lvl-7 14286928. The cache's
// GetSpellRequirements agrees with every level/rune row below.
//
// Looks (Spells.EnchantJewellery.js:162-167, 262-276): gold
// amulets play the spell's own animation/graphic/sound; every
// other piece plays the ring cast (animation 931, graphic 238,
// jewellerySound where the spell has one, else the spell
// sound); silver amulets play the spell's animation with the
// ring graphic and the spell sound; silver jewellery (the jade
// bracelet) plays the ring look with the spell's animation.
package enchantjewellery

import (
	"strings"

	"tsps-go/internal/game"
)

// ---- constants (Spells.EnchantJewellery.js:14-30) ----

// repeatTicks is REPEAT_TICKS: "if uninterrupted it will
// automatically cast it again every 7 ticks".
const repeatTicks = 7

// castTicks is CAST_TICKS: a cast's own cooldown, 3 ticks.
const castTicks = 3

// sharedCastDelayTicks is Spell.CAST_DELAY_TICKS (Spell.ts:51):
// the shared cast delay a consuming cast stamps on
// "magic:next-cast-at" (TS JewellerySpell.
// usesSharedCastDelay() is true).
const sharedCastDelayTicks = 1

// nextCastAttribute is COOLDOWN_ATTRIBUTE:
// "enchant-jewellery:next-cast-at".
const nextCastAttribute = "enchant-jewellery:next-cast-at"

// sharedCastAttribute is SHARED_CAST_ATTRIBUTE:
// "magic:next-cast-at".
const sharedCastAttribute = "magic:next-cast-at"

// jewelleryAnimation is JEWELLERY_ANIMATION (931):
// human_cast_enchantring, enchant_ring (rsprox).
const jewelleryAnimation = 931

// jewelleryGraphic is JEWELLERY_GRAPHIC (238).
const jewelleryGraphic = 238

// graphicHeight is GRAPHIC_HEIGHT (92): every enchant graphic
// sits at height 92.
const graphicHeight = 92

// tabSwitchScriptID is SIDE_BUTTON_SWITCH_SCRIPT (915):
// toplevel_sidebutton_switch; magicTab is MAGIC_TAB (6): the
// cast brings the spellbook back.
const tabSwitchScriptID = 915
const magicTab = 6

// rune ids (plan §1.1): fire 554, water 555, air 556, earth
// 557, cosmic 564, blood 565, soul 566.
const (
	runeFire   = 554
	runeWater  = 555
	runeAir    = 556
	runeEarth  = 557
	runeCosmic = 564
	runeBlood  = 565
	runeSoul   = 566
)

// look is one cast's presentation: the animation, the graphic
// and the sound (Spells.EnchantJewellery.js looks map).
type look struct {
	animation int
	graphic   int
	sound     int
}

// enchantSpell is one ENCHANT_SPELLS row: the spell's name,
// cache widget id, level, XP, rune cost, members flag (TS:
// members = level >= 68), and the jewellery it enchants, in
// TS's four groups — Amulets (gold, spell's own look),
// SilverAmulets (spell animation, ring graphic, spell sound),
// SilverJewellery (ring look, spell animation) and Jewellery
// (ring look). Each pair is [input, product].
type enchantSpell struct {
	name            string
	spellID         int
	level           int
	xp              float64
	runes           [][2]int
	members         bool
	animation       int
	graphic         int
	sound           int
	jewellerySound  int // 0 when the spell has none (Lvl-3, Lvl-4)
	amulets         [][2]int
	silverAmulets   [][2]int
	silverJewellery [][2]int
	jewellery       [][2]int
}

// results maps every enchantable input id to its product id
// (TS JewellerySpell.results: amulets + silverAmulets +
// silverJewellery + jewellery).
func (s *enchantSpell) results() map[int]int {
	out := make(map[int]int)
	for _, group := range [][][2]int{s.amulets, s.silverAmulets, s.silverJewellery, s.jewellery} {
		for _, pair := range group {
			out[pair[0]] = pair[1]
		}
	}
	return out
}

// resultFor is the product of itemID under this spell, or 0
// when the spell does not enchant it.
func (s *enchantSpell) resultFor(itemID int) int {
	return s.results()[itemID]
}

// lookFor is the cast presentation for itemID (TS
// JewellerySpell.looks, falling back to the spell's own data
// for gold amulets): jewellery and silver jewellery play the
// ring look (jewellerySound where the spell has one, else the
// spell sound), silver amulets the spell animation with the
// ring graphic and the spell sound.
func (s *enchantSpell) lookFor(itemID int) look {
	own := look{animation: s.animation, graphic: s.graphic, sound: s.sound}
	ringSound := s.sound
	if s.jewellerySound != 0 {
		ringSound = s.jewellerySound
	}
	ring := look{animation: jewelleryAnimation, graphic: jewelleryGraphic, sound: ringSound}
	for _, pair := range s.jewellery {
		if pair[0] == itemID {
			return ring
		}
	}
	for _, pair := range s.silverAmulets {
		if pair[0] == itemID {
			return look{animation: s.animation, graphic: jewelleryGraphic, sound: s.sound}
		}
	}
	for _, pair := range s.silverJewellery {
		if pair[0] == itemID {
			return look{animation: s.animation, graphic: jewelleryGraphic, sound: ringSound}
		}
	}
	return own
}

// spellCaster resolves the spell's game utility spell (the
// Superheat precedent: game.GetUtilitySpell). A nil spell
// (unregistered) fails the cast closed.
func (s *enchantSpell) spellCaster() game.SpellCaster {
	if s == nil {
		return nil
	}
	return game.GetUtilitySpell(s.spellID)
}

// enchantSpells is ENCHANT_SPELLS
// (Spells.EnchantJewellery.js:162-259), in TS order.
var enchantSpells = []enchantSpell{
	{
		name: "Lvl-1 Enchant", spellID: 14286864, level: 7, xp: 17.5,
		animation: 719, graphic: 114, sound: 136, jewellerySound: 147,
		runes:         [][2]int{{runeCosmic, 1}, {runeWater, 1}},
		amulets:       [][2]int{{1694, 1727}},   // SAPPHIRE_AMULET -> AMULET_OF_MAGIC
		silverAmulets: [][2]int{{21108, 21160}}, // OPAL_AMULET -> AMULET_OF_BOUNTY
		jewellery: [][2]int{
			{1637, 2550},   // SAPPHIRE_RING -> RING_OF_RECOIL
			{1656, 3853},   // SAPPHIRE_NECKLACE -> GAMES_NECKLACE_8_
			{11072, 11074}, // SAPPHIRE_BRACELET_2 -> BRACELET_OF_CLAY
			{21081, 21126}, // OPAL_RING -> RING_OF_PURSUIT
			{21090, 21143}, // OPAL_NECKLACE -> DODGY_NECKLACE
			{21117, 21177}, // OPAL_BRACELET -> EXPEDITIOUS_BRACELET
		},
	},
	{
		name: "Lvl-2 Enchant", spellID: 14286875, level: 27, xp: 37,
		animation: 720, graphic: 115, sound: 141, jewellerySound: 142,
		runes:           [][2]int{{runeCosmic, 1}, {runeAir, 3}},
		amulets:         [][2]int{{1696, 1729}},   // EMERALD_AMULET -> AMULET_OF_DEFENCE
		silverAmulets:   [][2]int{{21111, 21163}}, // JADE_AMULET -> AMULET_OF_CHEMISTRY
		silverJewellery: [][2]int{{21120, 21180}}, // JADE_BRACELET -> FLAMTAER_BRACELET
		jewellery: [][2]int{
			{1639, 2552},   // EMERALD_RING -> RING_OF_DUELING_8_
			{1658, 5521},   // EMERALD_NECKLACE -> BINDING_NECKLACE
			{11076, 11079}, // EMERALD_BRACELET -> CASTLE_WARS_BRACELET_3_
			{21084, 21129}, // JADE_RING -> RING_OF_RETURNING_5_
			{21093, 21146}, // JADE_NECKLACE -> NECKLACE_OF_PASSAGE_5_
		},
	},
	{
		name: "Lvl-3 Enchant", spellID: 14286888, level: 49, xp: 59,
		animation: 721, graphic: 116, sound: 145,
		runes: [][2]int{{runeCosmic, 1}, {runeFire, 5}},
		// The ruby necklace plays the amulet cast (rsprox:
		// human_enchantamuletlvl3, enchant_amulet_lvl3).
		amulets:       [][2]int{{1698, 1725}, {1660, 11194}}, // RUBY_AMULET -> AMULET_OF_STRENGTH, RUBY_NECKLACE -> DIGSITE_PENDANT_5_
		silverAmulets: [][2]int{{21114, 21166}},              // TOPAZ_AMULET -> BURNING_AMULET_5_
		jewellery: [][2]int{
			{1641, 2568},   // RUBY_RING -> RING_OF_FORGING
			{11085, 11088}, // RUBY_BRACELET -> INOCULATION_BRACELET
			{21087, 21140}, // TOPAZ_RING -> EFARITAYS_AID
			{21096, 21157}, // TOPAZ_NECKLACE -> NECKLACE_OF_FAITH
			{21123, 21183}, // TOPAZ_BRACELET -> BRACELET_OF_SLAUGHTER
		},
	},
	{
		name: "Lvl-4 Enchant", spellID: 14286897, level: 57, xp: 67,
		animation: 719, graphic: 153, sound: 137,
		runes:   [][2]int{{runeCosmic, 1}, {runeEarth, 10}},
		amulets: [][2]int{{1700, 1731}}, // DIAMOND_AMULET -> AMULET_OF_POWER
		jewellery: [][2]int{
			{1643, 2570},   // DIAMOND_RING -> RING_OF_LIFE
			{1662, 11090},  // DIAMOND_NECKLACE -> PHOENIX_NECKLACE
			{11092, 11095}, // DIAMOND_BRACELET -> ABYSSAL_BRACELET_5_
		},
	},
	{
		name: "Lvl-5 Enchant", spellID: 14286912, level: 68, xp: 78, members: true,
		animation: 720, graphic: 154, sound: 139, jewellerySound: 140,
		runes:   [][2]int{{runeCosmic, 1}, {runeWater, 15}, {runeEarth, 15}},
		amulets: [][2]int{{1702, 1704}}, // DRAGONSTONE_AMULET -> AMULET_OF_GLORY
		jewellery: [][2]int{
			{1645, 2572},   // DRAGONSTONE_RING -> RING_OF_WEALTH
			{1664, 11113},  // DRAGON_NECKLACE -> SKILLS_NECKLACE
			{11115, 11126}, // DRAGONSTONE_BRACELET -> COMBAT_BRACELET
		},
	},
	{
		name: "Lvl-6 Enchant", spellID: 14286925, level: 87, xp: 97, members: true,
		animation: 721, graphic: 452, sound: 143, jewellerySound: 144,
		runes:   [][2]int{{runeCosmic, 1}, {runeFire, 20}, {runeEarth, 20}},
		amulets: [][2]int{{6581, 6585}}, // ONYX_AMULET -> AMULET_OF_FURY
		jewellery: [][2]int{
			{6575, 6583},   // ONYX_RING -> RING_OF_STONE
			{6577, 11128},  // ONYX_NECKLACE -> BERSERKER_NECKLACE
			{11130, 11133}, // ONYX_BRACELET -> REGEN_BRACELET
		},
	},
	{
		name: "Lvl-7 Enchant", spellID: 14286928, level: 93, xp: 110, members: true,
		animation: 721, graphic: 452, sound: 143, jewellerySound: 144,
		runes:   [][2]int{{runeCosmic, 1}, {runeBlood, 20}, {runeSoul, 20}},
		amulets: [][2]int{{19541, 19553}}, // ZENYTE_AMULET -> AMULET_OF_TORTURE
		jewellery: [][2]int{
			{19538, 19550}, // ZENYTE_RING -> RING_OF_SUFFERING
			{19535, 19547}, // ZENYTE_NECKLACE -> NECKLACE_OF_ANGUISH
			{19532, 19544}, // ZENYTE_BRACELET_2 -> TORMENTED_BRACELET
		},
	},
}

// spellByName is the spells map (Spells.EnchantJewellery.js:
// spells.set(data.name.toLowerCase(), ...)), keyed by the
// lower-cased spell name.
var spellByName = func() map[string]*enchantSpell {
	out := make(map[string]*enchantSpell, len(enchantSpells))
	for i := range enchantSpells {
		out[strings.ToLower(enchantSpells[i].name)] = &enchantSpells[i]
	}
	return out
}()

// spellFor resolves a spell by its event name (TS
// handleSpellOnItem: spells.get(event.spellName?.trim().
// toLowerCase())), or nil when no enchant spell has that name.
func spellFor(name string) *enchantSpell {
	return spellByName[strings.ToLower(strings.TrimSpace(name))]
}
