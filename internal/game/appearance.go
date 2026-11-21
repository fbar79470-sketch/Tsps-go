package game

import (
	"encoding/json"
	"errors"
	"math"
)

// Appearance ports game/model/Appearance.ts (146 lines) at commit
// bfd81e3: the 13-entry look array (gender, 7 body kits, 5 colours),
// the head hint, the change-appearance flag, pronouns, and the
// Halloween skin-colour unlocks. The live appearance block itself is
// built by the session package from this state (see
// internal/session/liveappearance.go); the sync machinery marks
// snapshots dirty from Flag.APPEARANCE, which every mutator here
// sets, exactly as TS does.
//
// ATTRIBUTE-BACKED STATE: TS reads pronouns and skin unlocks
// from player attributes (getAttribute/setAttribute,
// Appearance.ts:103-119), and so does Go: Pronouns and
// IsSkinColourUnlocked read the owning player's attribute store
// (see attributes.go), SetPronouns and SetSkinUnlock write it.
// The coordinator initializes them at entry from the save's
// attributes via SetPronouns and SetSkinUnlock (see the method
// docs for the exact entry shapes); live changes land in the
// same store the save snapshot rebuilds from, so they round-trip
// like any other persistent attribute.
//
// OTHER GAPS (state that does not exist in Go):
//   - Displayed equipment (Player.setDisplayedEquipment /
//     getAppearanceEquipmentIds, Player.ts:1242-1255): the
//     appearance always shows what is worn. The builder reads the
//     live equipment container directly.
//   - Appearance hidden (Player.isAppearanceHidden, Player.ts:1062):
//     always false; the builder takes Hidden=false.
//   - NPC transformation (Player.getNpcTransformationId): no
//     transformation state; the builder sends no transformation id.
//   - Skull icon overrides (Player.getHeadSkullIcon,
//     Player.ts:1571-1576): the live skull state lives on
//     Player (skull.go); the coordinator's appearance
//     rebuild hook reads HeadSkullIcon() on every rebuild,
//     so a skull gained or lost mid-session reaches the
//     sync block at the next rebuild (a skull change
//     triggers one, see skull.go).
//
// TICK-GOROUTINE-ONLY: like the rest of Player. Entry loading calls
// SetLookArray/SetPronouns/SetSkinUnlock before the player is
// queued, so no tick races them.

// Look indices (Appearance.ts:4-16). The save's appearance array is
// 13 entries: gender, 7 kits, 5 colours. The session package's
// bootstrap.go duplicates these as unexported constants for the
// save-time sanitize; the values are identical by construction and
// pinned by the builder byte-equality test.
const (
	LookGender     = 0
	LookHead       = 1
	LookChest      = 2
	LookArms       = 3
	LookHands      = 4
	LookLegs       = 5
	LookFeet       = 6
	LookBeard      = 7
	LookHairColour = 8
	LookTorsoCol   = 9
	LookLegColour  = 10
	LookFeetColour = 11
	LookSkinColour = 12
)

// LookLength is the look array length: 13 entries (Appearance.ts:35,
// `new Array(13)`). SetLookArray accepts 12 or more (TS throws below
// 12) and normalizes to 13; see SetLookArray.
const LookLength = 13

// PronounsAttribute is Appearance.PRONOUNS_ATTRIBUTE
// (Appearance.ts:17): the player attribute key for pronouns.
// Pronouns reads it and SetPronouns writes it; the coordinator
// also calls SetPronouns at entry when the save holds the key.
const PronounsAttribute = "appearance:pronouns"

// SkinColourCount is Appearance.SKIN_COLOUR_COUNT
// (Appearance.ts:19): the skin palette's length (the cache's red,
// 13, is not in it).
const SkinColourCount = 13

// SkinUnlock is one entry of Appearance.SKIN_UNLOCKS
// (Appearance.ts:26-30): the skin colours behind a Halloween unlock,
// the saved attribute that records it, and the varbit shown to the
// client. The varbit is carried for grounding only: no varbit store
// exists in Go, and nothing sends it.
type SkinUnlock struct {
	Attribute string
	Varbit    string
	Colours   []int
}

// SkinUnlocks is Appearance.SKIN_UNLOCKS (Appearance.ts:26-30).
// Colours 0-7 and 12 have no entry and are open to everyone; each
// unlock is a player attribute, read with strict `=== true`
// semantics by IsSkinColourUnlocked. The coordinator initializes
// the unlock state at entry by calling SetSkinUnlock for each
// entry with the save attribute's strict boolean value.
var SkinUnlocks = []SkinUnlock{
	{Attribute: "appearance:hw07-skin-colours", Varbit: "hw07_skincols", Colours: []int{8}},     // green
	{Attribute: "appearance:hw15-skin-colours", Varbit: "hw15_skincols", Colours: []int{9, 10}}, // black, white
	{Attribute: "appearance:hw17-skin-colours", Varbit: "hw17_skincols", Colours: []int{11}},    // blue
}

// Appearance is one player's live appearance state. The look slice
// is always length LookLength. Pronouns and skin unlocks are NOT
// stored here: they live in the owning player's attribute store
// (see the file header) and are read through it on every access,
// as TS reads them through getAttribute.
type Appearance struct {
	player              *Player
	look                []int
	headHint            int
	canChangeAppearance bool
}

// NewAppearance returns an Appearance for p with the TS constructor
// behavior (Appearance.ts:38-41): a zeroed 13-entry look, headHint
// -1, canChangeAppearance false, then set() — which, with gender 0
// (male), installs the male defaults and flags Flag.APPEARANCE.
// Entry loading replaces the look via SetLookArray afterwards.
func NewAppearance(p *Player) *Appearance {
	a := &Appearance{
		player:   p,
		look:     make([]int, LookLength),
		headHint: -1,
	}
	a.Set()
	return a
}

// flagAppearance sets Flag.APPEARANCE on the owning player, as
// every TS Appearance mutator does through getUpdateFlag().flag()
// (Appearance.ts:48, 68, 74, 100, 111). A nil player (a bare
// Appearance in a test) flags nothing.
func (a *Appearance) flagAppearance() {
	if a.player != nil {
		a.player.Flags = a.player.Flags.Set(FlagAppearance)
	}
}

// HeadHint returns the prayer head hint (Appearance.ts:42-44,
// getHeadHint). It defaults to -1: no save field sets it, and the
// bootstrap sends -1 as the prayer head icon.
func (a *Appearance) HeadHint() int { return a.headHint }

// SetHeadHint ports Appearance.setHeadHint (Appearance.ts:46-50):
// store the hint and flag Flag.APPEARANCE. TS returns `this`; the
// Go form returns nothing.
func (a *Appearance) SetHeadHint(headHint int) {
	a.headHint = headHint
	a.flagAppearance()
}

// CanChangeAppearance reports whether the player may change
// appearance (Appearance.ts:32, 52-54, getCanChangeAppearance).
func (a *Appearance) CanChangeAppearance() bool { return a.canChangeAppearance }

// SetCanChangeAppearance ports Appearance.setCanChangeAppearance
// (Appearance.ts:56-58). Unlike the other mutators it does NOT flag
// Flag.APPEARANCE in TS, and does not here either.
func (a *Appearance) SetCanChangeAppearance(can bool) { a.canChangeAppearance = can }

// Look returns a defensive copy of the look array. TS getLook
// (Appearance.ts:60-62) returns the live array; the Go copy keeps
// callers (the appearance-block builder, the save snapshot) from
// aliasing live state. The save snapshot sanitizes the copy on
// write (fromPlayer, PlayerSave.ts:801); Look itself does not
// sanitize.
func (a *Appearance) Look() []int {
	return append([]int(nil), a.look...)
}

// SetLookArray ports Appearance.setLookArray (Appearance.ts:64-70):
// replace the look and flag Flag.APPEARANCE. TS throws
// Error("Array length must be 12.") for length < 12; Go returns an
// error and changes nothing. DELIBERATE DEVIATIONS from TS
// reference assignment: the input is copied (TS aliases the
// caller's array), over-long inputs are truncated to LookLength,
// and a 12-entry input gains a 0 skin colour (TS would leave
// look[12] undefined, which the encoder reads as 0 anyway). The
// result is always a 13-entry look, matching the save shape.
func (a *Appearance) SetLookArray(look []int) error {
	if len(look) < 12 {
		return errors.New("appearance look array length must be at least 12")
	}
	next := make([]int, LookLength)
	copy(next, look)
	a.look = next
	a.flagAppearance()
	return nil
}

// SetLook ports Appearance.setLook (Appearance.ts:72-75): set one
// look entry and flag Flag.APPEARANCE. An out-of-range index is
// ignored: TS would extend the JS array, which no encoder reads
// past index 12 and no Go caller does.
func (a *Appearance) SetLook(index, value int) {
	if index < 0 || index >= len(a.look) {
		return
	}
	a.look[index] = value
	a.flagAppearance()
}

// Set ports Appearance.set() (Appearance.ts:77-101): install the
// gender-appropriate default kits and the fixed colours for the
// CURRENT gender slot (it does not touch the gender slot itself),
// and flag Flag.APPEARANCE. Male (gender 0): head 3, chest 18,
// arms 26, hands 34, legs 38, feet 42, beard 14. Female: head 48,
// chest 57, arms 65, hands 68, legs 77, feet 80, beard -1. Colours
// in both cases: hair 2, torso 14, legs 5, feet 4, skin 0. These
// are the same arrays persist.SanitizeAppearance and the bootstrap
// use as defaults.
func (a *Appearance) Set() {
	if a.IsMale() {
		a.look[LookHead] = 3
		a.look[LookChest] = 18
		a.look[LookArms] = 26
		a.look[LookHands] = 34
		a.look[LookLegs] = 38
		a.look[LookFeet] = 42
		a.look[LookBeard] = 14
	} else {
		a.look[LookHead] = 48
		a.look[LookChest] = 57
		a.look[LookArms] = 65
		a.look[LookHands] = 68
		a.look[LookLegs] = 77
		a.look[LookFeet] = 80
		a.look[LookBeard] = -1
	}
	a.look[LookHairColour] = 2
	a.look[LookTorsoCol] = 14
	a.look[LookLegColour] = 5
	a.look[LookFeetColour] = 4
	a.look[LookSkinColour] = 0
	a.flagAppearance()
}

// Pronouns ports Appearance.getPronouns (Appearance.ts:103-106):
// the appearance:pronouns attribute when it is an integer 0..2,
// else 0 for male / 1 for female. The attribute is re-read and
// re-validated on every call, as TS re-validates the attribute
// value on every read. A nil player reads as a missing attribute
// and falls back to gender.
func (a *Appearance) Pronouns() int {
	if a.player != nil {
		if value, ok := pronounsAttributeValue(a.player.GetAttribute(PronounsAttribute)); ok {
			return value
		}
	}
	if a.IsMale() {
		return 0
	}
	return 1
}

// pronounsAttributeValue applies the TS getPronouns validation
// (Appearance.ts:104: `typeof value === "number" &&
// Number.isInteger(value) && value >= 0 && value <= 2`) to a Go
// attribute value. Attribute values arrive as ints from Go
// writers (SetPronouns) and as float64s from JSON-decoded saves,
// so both integral forms count, as do the other integer types
// and json.Number; strings, booleans, and non-integral floats
// are not numbers in the TS sense and report invalid.
func pronounsAttributeValue(value any) (int, bool) {
	switch v := value.(type) {
	case int:
		if v >= 0 && v <= 2 {
			return v, true
		}
	case int8:
		if v >= 0 && v <= 2 {
			return int(v), true
		}
	case int16:
		if v >= 0 && v <= 2 {
			return int(v), true
		}
	case int32:
		if v >= 0 && v <= 2 {
			return int(v), true
		}
	case int64:
		if v >= 0 && v <= 2 {
			return int(v), true
		}
	case uint:
		if v <= 2 {
			return int(v), true
		}
	case uint8:
		if v <= 2 {
			return int(v), true
		}
	case uint16:
		if v <= 2 {
			return int(v), true
		}
	case uint32:
		if v <= 2 {
			return int(v), true
		}
	case uint64:
		if v <= 2 {
			return int(v), true
		}
	case float32:
		f := float64(v)
		if f == math.Trunc(f) && f >= 0 && f <= 2 {
			return int(f), true
		}
	case float64:
		if v == math.Trunc(v) && v >= 0 && v <= 2 {
			return int(v), true
		}
	case json.Number:
		if i, err := v.Int64(); err == nil && i >= 0 && i <= 2 {
			return int(i), true
		}
		if f, err := v.Float64(); err == nil && f == math.Trunc(f) && f >= 0 && f <= 2 {
			return int(f), true
		}
	}
	return 0, false
}

// SetPronouns ports Appearance.setPronouns (Appearance.ts:108-112):
// an out-of-range value (not an integer 0..2) returns silently with
// no flag and no change; a valid value is written to the
// appearance:pronouns attribute and flags Flag.APPEARANCE. A nil
// player stores nothing (there is no attribute store to write)
// but the call is still safe.
// ENTRY USE: the coordinator calls this at entry when the save's
// appearance:pronouns attribute is a JSON integer 0..2; when the
// attribute is absent or invalid, it calls nothing and Pronouns
// falls back to gender, exactly as TS does for a missing
// attribute.
func (a *Appearance) SetPronouns(value int) {
	if value < 0 || value > 2 {
		return
	}
	if a.player != nil {
		a.player.SetAttribute(PronounsAttribute, value)
	}
	a.flagAppearance()
}

// IsSkinColourUnlocked ports Appearance.isSkinColourUnlocked
// (Appearance.ts:115-119): whether the player may choose a skin
// colour. Out-of-palette values are never unlocked. A colour with
// no unlock entry is open to everyone; a gated colour needs its
// unlock attribute strictly true (`=== true`: only the boolean
// true counts, never 1 or "true"). A nil player reads every gated
// colour as locked, as a missing attribute does in TS.
func (a *Appearance) IsSkinColourUnlocked(colour int) bool {
	if colour < 0 || colour >= SkinColourCount {
		return false
	}
	for _, entry := range SkinUnlocks {
		for _, gated := range entry.Colours {
			if gated == colour {
				return a.player != nil && a.player.GetAttribute(entry.Attribute) == true
			}
		}
	}
	return true
}

// UnlockedSkinColours ports Appearance.unlockedSkinColours
// (Appearance.ts:122-125): the skin colours this player may
// choose, in palette order. Default (no unlocks loaded): 0-7 and
// 12.
func (a *Appearance) UnlockedSkinColours() []int {
	var unlocked []int
	for colour := 0; colour < SkinColourCount; colour++ {
		if a.IsSkinColourUnlocked(colour) {
			unlocked = append(unlocked, colour)
		}
	}
	return unlocked
}

// SetSkinUnlock records one Halloween skin-colour unlock attribute
// (see SkinUnlocks) in the player's attribute store. ENTRY USE:
// the coordinator calls this at entry for each SkinUnlocks entry
// with the save attribute's strict boolean value (only the JSON
// literal true unlocks, matching the TS `=== true` check).
// Unknown attributes are ignored. A false value DELETES the key
// rather than storing false, restoring the TS absent state: TS
// entry loading only sets keys present in the save, so an absent
// key must stay absent rather than gain a stored false. A nil
// player stores nothing. It does not flag Flag.APPEARANCE: TS
// never writes these attributes through Appearance (content sets
// them), so no flag rule exists to port.
func (a *Appearance) SetSkinUnlock(attribute string, unlocked bool) {
	for _, entry := range SkinUnlocks {
		if entry.Attribute == attribute {
			if a.player == nil {
				return
			}
			if unlocked {
				a.player.SetAttribute(attribute, true)
			} else {
				a.player.SetAttribute(attribute, nil)
			}
			return
		}
	}
}

// StepSkinColour ports Appearance.stepSkinColour
// (Appearance.ts:132-141): the next skin colour from `current` in
// `direction` (1 or -1; any non-negative direction steps forward)
// through the palette, wrapping and skipping colours not in
// `unlocked`. A locked or out-of-palette `current` steps off it,
// so a kept locked colour moves to its neighbour. When no colour
// in the palette is unlocked, it returns `current` unchanged.
func StepSkinColour(current, direction int, unlocked []int) int {
	count := SkinColourCount
	dir := 1
	if direction < 0 {
		dir = -1
	}
	colour := current
	if colour < 0 || colour >= count {
		if dir == 1 {
			colour = -1
		} else {
			colour = count
		}
	}
	for i := 0; i < count; i++ {
		colour = (colour + dir + count) % count
		for _, allowed := range unlocked {
			if allowed == colour {
				return colour
			}
		}
	}
	return current
}

// IsMale ports Appearance.isMale (Appearance.ts:143-145): the
// gender slot is 0. Any other value (including invalid ones) reads
// as female, as the TS `== 0` comparison does.
func (a *Appearance) IsMale() bool {
	return a.look[LookGender] == 0
}

// SyncAnimations returns the 7 stand/turn/walk/run sequence ids for
// this player's appearance block, porting the animation computation
// in PlayerSession.createPlayerView (PlayerSession.ts:677-699).
// The TS priority is: a positive skill animation fills all 7 slots;
// else render animations (Player.getRenderAnimations,
// Player.ts:1225); else the transformed NPC's sequences; else the
// weapon fallback [stand ?? 808, 823, walk ?? 819, 820, 821, 822,
// run ?? 824], where stand/walk/run come from the equipped weapon's
// ItemDefinition (equipment slot 3, Equipment.WEAPON_SLOT,
// Equipment.ts:20; ItemDefinition.getStandAnim/getWalkAnim/
// getRunAnim, ItemDefinition.ts:257-267, backed by the standAnim/
// walkAnim/runAnim fields defaulting to 808/819/824,
// ItemDefinition.ts:95-97, overridable through custom-item JSON).
//
// Go returns the weapon fallback: the equipped weapon's
// stand/walk/run overrides when the installed definition source
// implements ItemAnimationSource, else the TS `??` defaults. The
// gaps, in priority order:
//   - Skill animation (Player.getSkillAnimation, Player.ts:1215):
//     no skill-animation state exists in Go (no skilling content).
//   - Render animations: no render-animation state exists.
//   - Transformation: no transformation state exists.
//
// An empty weapon slot (id < 0) yields the defaults without
// consulting the source, matching the TS `?.` chain over an
// absent weapon definition. A nil player yields the defaults.
//
// The returned slice is freshly allocated on every call.
func (p *Player) SyncAnimations() []int {
	defaults := []int{808, 823, 819, 820, 821, 822, 824}
	if p == nil || p.equipment == nil {
		return defaults
	}
	weapon := p.equipment.ItemAt(WeaponSlot)
	if weapon.ID() < 0 {
		return defaults
	}
	src, ok := p.itemDefs.(ItemAnimationSource)
	if !ok || src == nil {
		return defaults
	}
	return []int{src.StandAnim(weapon.ID()), 823, src.WalkAnim(weapon.ID()), 820, 821, 822, src.RunAnim(weapon.ID())}
}
