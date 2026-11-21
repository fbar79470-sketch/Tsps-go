package persist

import (
	"bytes"
	"encoding/json"
	"math"
	"sort"
	"strings"
	"time"
)

// Save-shape operations: the merge form of PlayerSave.applyToPlayer /
// fromPlayer (game/entity/impl/player/persistence/PlayerSave.ts:633-862).
//
// TS moves state between a live Player and a PlayerSave in both
// directions. The Go Player owns position, running, run-energy
// units, and — since phase 9 — the live containers, skills, and
// appearance (see the owned-vs-preserved split below), so the
// round-trip is expressed as operations on the save itself: the
// carried base save is preserved verbatim and only the owned fields
// are overwritten from live state (WithOwned), while entry-time
// mutations (appearance sanitize, attribute-key migration,
// legacy-field fold) are applied in place (MigrateForEntry). New
// characters are built from TS defaults (NewCharacterSave), and
// Unmake ports unmake().
//
// GAPS (documented, not invented): the "persistence:snapshot" plugin
// event (PlayerSave.ts:851-856) does not exist in Go — no plugin system
// — so no handler can rewrite a snapshot. The trade-escrow fold in
// fromPlayer (PlayerSave.ts:782-791) IS ported as FoldTradeEscrow
// (escrow.go); the snapshot builder calls it, since this package
// cannot see the live player.
//
// This package imports no other tsps-go package; that layering is
// deliberate (see doc.go) and is preserved here.

// OwnedState is the live-player-owned subset of a save: the fields the
// Go Player demonstrably owns and snapshots overwrite. Everything
// else rides in the carried base save, preserved verbatim.
//
// Owned: position, running, run-energy units, and — since phase 16 —
// the special-attack energy (specPercentage), and — since phase 9
// — the inventory, equipment, appearance, skills, and banks, all
// written from live state exactly as fromPlayer writes them
// (PlayerSave.ts:783-851): inventory/equipment as copied item
// arrays (the inventory copy folds offered items back in through
// FoldTradeEscrow, escrow.go), appearance sanitized
// (sanitizeAppearance), skills as cloned arrays, banks as per-tab
// getValidItems arrays for tabs 0-9 (the search tab is never
// written). Since phase 11 the attributes are
// owned too: fromPlayer rebuilds the attributes object from scratch
// from the live persistent keys (PlayerSave.ts:811-816), so a
// snapshot built from live state carries exactly the live persistent
// attributes, minus default-equal values — and DROPS every other key
// the carried save held, including keys no longer in the registry.
// See WithOwned for the consequence, stated plainly. Since phase 19
// the relations fields are owned too: friends/ignores as normalized
// stringified name hashes, friendRanks as the live rank map
// filtered to current friends, and the friends-chat channel
// name/last owner/entry-talk-kick ranks, all written from live
// relations state exactly as fromPlayer writes them
// (PlayerSave.ts:819-830). RunEnergyUnits is
// the live energy in OSRS units (0..10,000); WithOwned writes it as
// units/100, the percent getRunEnergy returns
// (PlayerSave.ts:762, fromPlayer). Entry application sets live units
// from the save percent through OwnedState (applyToPlayer calls
// player.setRunEnergy(this.runEnergy), PlayerSave.ts:654), and
// loads the containers, skills, and appearance through the same
// state (applyToPlayer calls setItems/setSkills/setLookArray,
// PlayerSave.ts:672-680, and loads banks through add,
// PlayerSave.ts:726-735). Entry application loads live attributes
// from the save's Attributes directly (not through OwnedState);
// OwnedState carries them only for the snapshot direction.
//
// A nil Inventory, Equipment, Appearance, Attributes, or Banks in an
// OwnedState leaves the carried value untouched, as does a Skills
// value whose three arrays are all nil: the coordinator always
// supplies complete live state, and the nil guard keeps partial
// OwnedState constructions (older callers, tests) preserving instead
// of wiping. Callers that own a field must supply it non-nil. A
// NON-nil Attributes map — even an empty one — overwrites the
// carried attributes outright (see WithOwned): nil means preserve,
// empty means "the player holds no persistent attributes". The
// relations fields follow the same rule: a nil Friends, Ignores,
// or FriendRanks preserves the carried list or map, while a
// non-nil value — even an empty one — overwrites it outright;
// the channel name/owner and entry-talk-kick ranks are pointers
// for the same reason (a plain string or int cannot express
// "unsupplied"), so a nil pointer preserves and a non-nil
// pointer overwrites, even when it points at "" or 0.
//
// Preserved verbatim: displayName, rights, spellbook,
// timers, quick prayers, flags,
// and Extra — including unknown keys. (Attributes LEFT
// this list in phase 11: they are owned state now, rebuilt from
// live state on every snapshot. Friends/ignores, friendRanks, and
// the friends-chat channel fields LEFT this list in phase 19:
// they are owned state now, written from live relations on every
// snapshot whose OwnedState supplies them. Sailing LEFT this
// list in phase 21: it is owned state now, written from live
// sailing state — normalized, as fromPlayer writes
// normalizeSailingState(player.getSailing()),
// PlayerSave.ts:810 — on every snapshot whose OwnedState
// supplies it. AutocastSpellId LEFT this list in phase 27: it is
// owned state now, written from the live combat autocast spell
// on every snapshot (PlayerSave.ts:756). FightType LEFT this
// list in phase 30: it is owned state now, written from the
// live combat fight type on every snapshot (PlayerSave.ts:754).
// TeleblockTimer is always
// written 0 by WithOwned, as TS fromPlayer does; it is not live
// state.)
type OwnedState struct {
	Position       Location
	Running        bool
	RunEnergyUnits int
	// SpecPercentage is the live special-attack energy, 0..100
	// (fromPlayer writes player.getSpecialPercentage(),
	// PlayerSave.ts:763; applyToPlayer restores it,
	// PlayerSave.ts:655). It is always written by WithOwned,
	// like Running and RunEnergyUnits: callers that build an
	// OwnedState must supply the live value.
	SpecPercentage int
	// AutocastSpellID is the live autocast spell id, or -1 when
	// no autocast spell is set (fromPlayer writes
	// player.getCombat().getAutocastSpell()?.spellId?.() ?? -1,
	// PlayerSave.ts:756). It is always written by WithOwned,
	// like SpecPercentage: the snapshot builder always supplies
	// it from live combat state. It is owned since phase 27:
	// before that, nothing in Go set an autocast spell, so the
	// field stayed preserved and a live pick would have been
	// lost on the next snapshot.
	AutocastSpellID int
	Inventory       []Item
	Equipment       []Item
	Appearance      []int
	Skills          Skills
	Banks           Banks
	// Attributes is the rebuilt save attribute map: the live
	// persistent attributes as canonical JSON, default-equal values
	// already stripped by the snapshot builder. Nil means preserve
	// the carried attributes (with default-stripping, the pre-phase-11
	// behavior); non-nil overwrites them outright, dropping every
	// key the map does not hold.
	Attributes map[string]json.RawMessage
	// Friends and Ignores are the live relations lists as
	// normalized stringified name hashes (fromPlayer,
	// PlayerSave.ts:819-826). Nil means preserve the carried
	// list; non-nil overwrites it outright, even when empty.
	// The snapshot builder supplies the live lists already
	// normalized (the relations layer caps and dedupes them);
	// WithOwned clones but does not re-normalize, matching
	// the container treatment above.
	Friends []string
	Ignores []string
	// FriendRanks is the live friend-rank map, stringified
	// name hash to rank, filtered to current friends
	// (fromPlayer, PlayerSave.ts:828, via getFriendRanks).
	// Nil means preserve the carried map; non-nil overwrites
	// it outright, even when empty.
	FriendRanks map[string]int
	// FriendsChatChannelName, FriendsChatLastOwner, and the
	// entry-talk-kick ranks are the live friends-chat channel
	// fields (fromPlayer, PlayerSave.ts:829-830). Each is a
	// pointer so an unsupplied field preserves the carried
	// value: nil means preserve, non-nil overwrites, even
	// when pointing at "" or 0. The snapshot builder always
	// supplies all five from live relations state.
	FriendsChatChannelName *string
	FriendsChatLastOwner   *string
	FriendsChatEntryRank   *int
	FriendsChatTalkRank    *int
	FriendsChatKickRank    *int
	// Sailing is the live sailing state as normalized save
	// JSON (fromPlayer, PlayerSave.ts:810, via
	// normalizeSailingState). Nil means preserve the carried
	// sailing value; non-nil overwrites it outright. The
	// snapshot builder supplies the normalized JSON (the
	// game layer normalizes it); WithOwned clones but does
	// not re-normalize, matching the relations treatment
	// above. This package cannot import the game package
	// (see doc.go layering), so the state travels as raw
	// JSON, as FightType does.
	Sailing json.RawMessage
	// FightType is the live fight type as save JSON
	// (fromPlayer, PlayerSave.ts:754: playerSave.fightType =
	// player.getFightType()). Nil means preserve the carried
	// fightType value; non-nil overwrites it outright. The
	// snapshot builder supplies the game-encoded JSON (see
	// FightTypeSaveJSON); WithOwned clones but does not
	// re-encode, matching the sailing treatment above. This
	// package cannot import the game package (see doc.go
	// layering), so the state travels as raw JSON, as
	// Sailing does.
	FightType json.RawMessage
}

// runEnergyMaxUnits is RUN_ENERGY_MAX_UNITS (GameConstants.ts:30):
// OSRS run energy is 0-10,000 units, 100 units = 1%. This package
// cannot import the game package (see doc.go layering), so the
// constant is restated here with its citation, as the session
// package restates it in runEnergyState.
const runEnergyMaxUnits = 10000

// runEnergyUnitsFromPercent converts a save percent to live units,
// following setRunEnergy (Player.ts:1261-1274): floor(percent*100 +
// 1e-6), clamped to 0..10,000. The +1e-6 hair keeps floor semantics
// for real fractions: 0.29 * 100 is 28.999... in floating point,
// which flooring would lose a unit of on every relog. The clamp
// runs in the float domain before the int conversion so an extreme
// percent cannot overflow the conversion.
func runEnergyUnitsFromPercent(percent float64) int {
	units := math.Floor(percent*100 + 1e-6)
	if math.IsNaN(units) || units < 0 {
		return 0
	}
	if units > runEnergyMaxUnits {
		return runEnergyMaxUnits
	}
	return int(units)
}

// OwnedState extracts the owned fields from the save.
//
// Precondition: the receiver must be non-nil. A nil Position (only
// possible on a save that skipped load-time normalization) yields the
// zero Location; callers guarantee normalized saves (normalizeLoadedSave
// defaults a missing position to the Lumbridge spawn), so a zero
// Location here signals a caller bug, not a spawn. The behavior is
// pinned by TestOwnedStateNilPosition. RunEnergyUnits is derived
// from the save percent through runEnergyUnitsFromPercent (the
// setRunEnergy rule); a missing percent (0) yields 0 units, which
// entry application then sets live — callers guarantee saves that
// went through load-time normalization, where runEnergy defaults to
// 100 for a new character. The container, appearance, skills, bank,
// attribute, relations-list, and sailing values are defensive copies:
// mutating live state loaded from them must never alias the carried
// save. The channel name/owner/rank pointers always point at fresh
// copies of the save values, so extraction always supplies them.
func (s *PlayerSave) OwnedState() OwnedState {
	channelName := s.FriendsChatChannelName
	lastOwner := s.FriendsChatLastOwner
	entryRank := s.FriendsChatEntryRank
	talkRank := s.FriendsChatTalkRank
	kickRank := s.FriendsChatKickRank
	state := OwnedState{
		Running:         s.Running,
		RunEnergyUnits:  runEnergyUnitsFromPercent(s.RunEnergy),
		SpecPercentage:  s.SpecPercentage,
		AutocastSpellID: s.AutocastSpellId,
		Inventory:       cloneItems(s.Inventory),
		Equipment:       cloneItems(s.Equipment),
		Appearance:      cloneInts(s.Appearance),
		Skills: Skills{
			Level:      cloneInts(s.Skills.Level),
			MaxLevel:   cloneInts(s.Skills.MaxLevel),
			Experience: cloneInts(s.Skills.Experience),
		},
		Banks:                  cloneBanks(s.Banks),
		Attributes:             cloneRawMessageMap(s.Attributes),
		Friends:                cloneStrings(s.Friends),
		Ignores:                cloneStrings(s.Ignores),
		FriendRanks:            cloneStringIntMap(s.FriendRanks),
		FriendsChatChannelName: &channelName,
		FriendsChatLastOwner:   &lastOwner,
		FriendsChatEntryRank:   &entryRank,
		FriendsChatTalkRank:    &talkRank,
		FriendsChatKickRank:    &kickRank,
		Sailing:                cloneRawMessage(s.Sailing),
	}
	if s.Position != nil {
		state.Position = *s.Position
	}
	return state
}

// WithOwned returns a deep-cloned save with Position/Running/RunEnergy/
// SpecPercentage/AutocastSpellId overwritten from state (RunEnergy as
// RunEnergyUnits/100, the getRunEnergy percent), TeleblockTimer set
// to 0, and — when the state supplies them — the inventory, equipment,
// appearance (sanitized, as fromPlayer sanitizes the live look),
// skills, banks, attributes, relations fields, sailing, and fightType
// overwritten from live state. A nil container/appearance/attributes/banks/
// relations/sailing/fightType field, or a Skills value with all three arrays
// nil, preserves the carried value (see OwnedState). The receiver is
// NOT mutated.
//
// UNKNOWN-KEY DROP (deliberate, TS-faithful behavior change): when
// state.Attributes is non-nil, the clone's attributes are REPLACED by
// the supplied map — every key the carried save held that the map does
// not hold is DROPPED, including keys no longer in the persistent
// registry and keys that were never persistent at all. That is what TS
// fromPlayer does: it rebuilds the attributes object from scratch from
// the live persistent keys (PlayerSave.ts:811-816), so a save written
// by TS never carries an unknown or unregistered key forward. Before
// phase 11 the Go merge preserved carried attributes verbatim, so an
// unregistered key (e.g. a plugin key whose plugin no longer registers
// it, or an appearance key read only at entry) survived snapshots
// indefinitely; from phase 11 on, the first snapshot built from live
// state drops it, exactly as a TS save would. When state.Attributes is
// nil, the old behavior stands: the carried attributes are preserved
// with default-equal values stripped.
//
// This is the observable save-side effect of fromPlayer
// (PlayerSave.ts:737-862) restricted to the owned subset: fromPlayer
// always writes teleblockTimer 0 (PlayerSave.ts:775 — logging out ends
// Tele Block) and strips attributes equal to their registered default
// (PlayerSave.ts:811-816, via StripDefaultAttributes). The supplied
// attribute map is default-stripped again here, so a caller that hands
// WithOwned an unstripped map still gets the fromPlayer omission (the
// snapshot builder strips first; stripping twice is a no-op). Every
// other field — including Extra, Profile, and all other json.RawMessage
// fields (spellBook.teleportType) — is preserved
// verbatim in the clone. The clone shares no mutable state with the
// receiver.
//
// Precondition: the receiver must be non-nil.
func (s *PlayerSave) WithOwned(state OwnedState) *PlayerSave {
	clone := clonePlayerSave(s)
	position := state.Position
	clone.Position = &position
	clone.Running = state.Running
	clone.RunEnergy = float64(state.RunEnergyUnits) / 100
	clone.SpecPercentage = state.SpecPercentage
	clone.AutocastSpellId = state.AutocastSpellID
	clone.TeleblockTimer = 0
	if state.Attributes != nil {
		// Owned overwrite: the supplied map replaces the carried
		// attributes outright (see the unknown-key-drop note above),
		// default-stripped as fromPlayer strips the rebuild.
		clone.Attributes = StripDefaultAttributes(cloneRawMessageMap(state.Attributes))
	} else {
		clone.Attributes = StripDefaultAttributes(clone.Attributes)
	}
	if state.Inventory != nil {
		clone.Inventory = cloneItems(state.Inventory)
	}
	if state.Equipment != nil {
		clone.Equipment = cloneItems(state.Equipment)
	}
	if state.Appearance != nil {
		clone.Appearance = SanitizeAppearance(state.Appearance)
	}
	if state.Skills.Level != nil || state.Skills.MaxLevel != nil || state.Skills.Experience != nil {
		clone.Skills = Skills{
			Level:      cloneInts(state.Skills.Level),
			MaxLevel:   cloneInts(state.Skills.MaxLevel),
			Experience: cloneInts(state.Skills.Experience),
		}
	}
	if state.Banks != nil {
		clone.Banks = cloneBanks(state.Banks)
	}
	if state.Friends != nil {
		clone.Friends = cloneStrings(state.Friends)
	}
	if state.Ignores != nil {
		clone.Ignores = cloneStrings(state.Ignores)
	}
	if state.FriendRanks != nil {
		clone.FriendRanks = cloneStringIntMap(state.FriendRanks)
	}
	if state.FriendsChatChannelName != nil {
		clone.FriendsChatChannelName = *state.FriendsChatChannelName
	}
	if state.FriendsChatLastOwner != nil {
		clone.FriendsChatLastOwner = *state.FriendsChatLastOwner
	}
	if state.FriendsChatEntryRank != nil {
		clone.FriendsChatEntryRank = *state.FriendsChatEntryRank
	}
	if state.FriendsChatTalkRank != nil {
		clone.FriendsChatTalkRank = *state.FriendsChatTalkRank
	}
	if state.FriendsChatKickRank != nil {
		clone.FriendsChatKickRank = *state.FriendsChatKickRank
	}
	if state.Sailing != nil {
		clone.Sailing = cloneRawMessage(state.Sailing)
	}
	if state.FightType != nil {
		clone.FightType = cloneRawMessage(state.FightType)
	}
	return clone
}

// clonePlayerSave deep-copies a save: every slice, map, pointer, and
// json.RawMessage gets fresh storage, so mutating the clone can never
// affect the original. Nil vs empty is preserved for every reference
// field, so the clone marshals exactly like the original. No
// normalization is applied: unlike a JSON round-trip, cloning never
// renames item meta keys (Item.UnmarshalJSON) or otherwise rewrites a
// value.
func clonePlayerSave(s *PlayerSave) *PlayerSave {
	c := *s
	if s.Position != nil {
		position := *s.Position
		c.Position = &position
	}
	if s.Rights.YellTag != nil {
		yellTag := *s.Rights.YellTag
		c.Rights.YellTag = &yellTag
	}
	c.SpellBook.TeleportType = cloneRawMessage(s.SpellBook.TeleportType)
	c.FightType = cloneRawMessage(s.FightType)
	c.AudioSettings = cloneIntIntMap(s.AudioSettings)
	if s.LastVenomDamage != nil {
		lastVenomDamage := *s.LastVenomDamage
		c.LastVenomDamage = &lastVenomDamage
	}
	if s.PoisonClock != nil {
		poisonClock := *s.PoisonClock
		if s.PoisonClock.ProtectionTicksRemaining != nil {
			remaining := *s.PoisonClock.ProtectionTicksRemaining
			poisonClock.ProtectionTicksRemaining = &remaining
		}
		if s.PoisonClock.BriefPoisonSeconds != nil {
			seconds := *s.PoisonClock.BriefPoisonSeconds
			poisonClock.BriefPoisonSeconds = &seconds
		}
		if s.PoisonClock.BriefVenomSeconds != nil {
			seconds := *s.PoisonClock.BriefVenomSeconds
			poisonClock.BriefVenomSeconds = &seconds
		}
		c.PoisonClock = &poisonClock
	}
	c.Inventory = cloneItems(s.Inventory)
	c.Equipment = cloneItems(s.Equipment)
	c.Appearance = cloneInts(s.Appearance)
	c.Skills = Skills{
		Level:      cloneInts(s.Skills.Level),
		MaxLevel:   cloneInts(s.Skills.MaxLevel),
		Experience: cloneInts(s.Skills.Experience),
	}
	c.QuickPrayers = cloneQuickPrayers(s.QuickPrayers)
	c.Friends = cloneStrings(s.Friends)
	c.Ignores = cloneStrings(s.Ignores)
	c.FriendRanks = cloneStringIntMap(s.FriendRanks)
	c.Banks = cloneBanks(s.Banks)
	c.Flags = cloneStrings(s.Flags)
	c.Sailing = cloneRawMessage(s.Sailing)
	c.Attributes = cloneRawMessageMap(s.Attributes)
	c.Extra = cloneRawMessageMap(s.Extra)
	return &c
}

func cloneRawMessage(raw json.RawMessage) json.RawMessage {
	if raw == nil {
		return nil
	}
	return append(json.RawMessage(nil), raw...)
}

func cloneInts(values []int) []int {
	if values == nil {
		return nil
	}
	return append([]int(nil), values...)
}

func cloneStrings(values []string) []string {
	if values == nil {
		return nil
	}
	return append([]string(nil), values...)
}

func cloneIntIntMap(m map[int]int) map[int]int {
	if m == nil {
		return nil
	}
	out := make(map[int]int, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

func cloneStringIntMap(m map[string]int) map[string]int {
	if m == nil {
		return nil
	}
	out := make(map[string]int, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

func cloneRawMessageMap(m map[string]json.RawMessage) map[string]json.RawMessage {
	if m == nil {
		return nil
	}
	out := make(map[string]json.RawMessage, len(m))
	for k, v := range m {
		out[k] = cloneRawMessage(v)
	}
	return out
}

func cloneItems(items []Item) []Item {
	if items == nil {
		return nil
	}
	out := make([]Item, len(items))
	for i, item := range items {
		out[i] = item
		out[i].Meta = cloneRawMessageMap(item.Meta)
	}
	return out
}

func cloneQuickPrayers(prayers []*QuickPrayer) []*QuickPrayer {
	if prayers == nil {
		return nil
	}
	out := make([]*QuickPrayer, len(prayers))
	for i, prayer := range prayers {
		if prayer != nil {
			copied := *prayer
			out[i] = &copied
		}
	}
	return out
}

func cloneBanks(banks Banks) Banks {
	if banks == nil {
		return nil
	}
	out := make(Banks, len(banks))
	for tab, items := range banks {
		out[tab] = cloneItems(items)
	}
	return out
}

// Appearance indices (model/Appearance.ts:3-11).
const (
	appearanceGender     = 0
	appearanceHead       = 1
	appearanceChest      = 2
	appearanceArms       = 3
	appearanceHands      = 4
	appearanceLegs       = 5
	appearanceFeet       = 6
	appearanceBeard      = 7
	appearanceHairColour = 8
	appearanceTorsoCol   = 9
	appearanceLegColour  = 10
	appearanceFeetCol    = 11
	appearanceSkinColour = 12
)

// Default appearances and look/colour ranges: PlayerSave.ts:104-140
// (DEFAULT_MALE/FEMALE_APPEARANCE, MALE/FEMALE_LOOK_RANGES,
// COLOR_RANGES). The male default equals Appearance.set()'s male result
// (Appearance.ts:76-89); the female default equals its female result.
var (
	defaultMaleAppearance   = []int{0, 3, 18, 26, 34, 38, 42, 14, 2, 14, 5, 4, 0}
	defaultFemaleAppearance = []int{1, 48, 57, 65, 68, 77, 80, -1, 2, 14, 5, 4, 0}
	maleLookRanges          = [][2]int{{0, 8}, {10, 17}, {18, 25}, {26, 31}, {33, 34}, {36, 40}, {42, 43}}
	femaleLookRanges        = [][2]int{{45, 54}, {-1, -1}, {56, 60}, {61, 65}, {67, 68}, {70, 77}, {79, 80}}
	appearanceColorRanges   = [][2]int{{0, 29}, {0, 28}, {0, 28}, {0, 5}, {0, 12}}
)

// lookIndices is the look-slot order sanitizeAppearance walks, in body
// part order (PlayerSave.ts:600-608): each entry pairs with the same
// position in the look-range tables.
var appearanceLookIndices = []int{
	appearanceHead, appearanceBeard, appearanceChest, appearanceArms,
	appearanceHands, appearanceLegs, appearanceFeet,
}

var appearanceColorIndices = []int{
	appearanceHairColour, appearanceTorsoCol, appearanceLegColour,
	appearanceFeetCol, appearanceSkinColour,
}

// SanitizeAppearance ports PlayerSave.sanitizeAppearance
// (PlayerSave.ts:581-631): gender coercion (1 is female, anything else
// male), per-range clamping of the seven look slots, the -1 jaw rule
// (no kit: clean-shaven, and females have no beard kit), and the
// unknown-skin rule (an out-of-palette skin, e.g. a native client's red
// 13, resets to the default instead of clamping to a nearby colour).
// Missing entries (a short or nil input) fall back to the gender
// default; extra entries are ignored. The result is always a fresh
// 13-entry slice.
//
// GAP — isSelectableKit: TS also preserves a kit the cache offers
// players for the body part, beyond the legacy ranges
// (PlayerSave.ts:556-575, reading CacheDefinitions.getIdentKits()).
// This package imports no cache data (see the layering note above), so
// the clamp-only fallback applies — the same fallback TS itself uses
// when no cache is loaded (the catch in isSelectableKit returns false)
// and the same one the login bootstrap documents using. A cache-valid
// style outside the legacy ranges is therefore clamped, never
// preserved. A caller with ident-kit data must re-check before
// trusting the clamped result.
func SanitizeAppearance(appearance []int) []int {
	gender := 0
	if len(appearance) > appearanceGender && appearance[appearanceGender] == 1 {
		gender = 1
	}
	defaults := defaultMaleAppearance
	lookRanges := maleLookRanges
	if gender == 1 {
		defaults = defaultFemaleAppearance
		lookRanges = femaleLookRanges
	}
	sanitized := append([]int(nil), defaults...)
	for i, lookIndex := range appearanceLookIndices {
		min, max := lookRanges[i][0], lookRanges[i][1]
		candidate := defaults[lookIndex]
		if lookIndex < len(appearance) {
			candidate = appearance[lookIndex]
		}
		// -1 is "no kit" for the jaw (PlayerSave.ts:612-616). The
		// isSelectableKit check is the clamp-only fallback (see the GAP
		// note on SanitizeAppearance): always false here.
		noJaw := lookIndex == appearanceBeard && candidate == -1
		if noJaw {
			sanitized[lookIndex] = candidate
		} else {
			sanitized[lookIndex] = clampInt(candidate, min, max)
		}
	}
	for i, colorIndex := range appearanceColorIndices {
		min, max := appearanceColorRanges[i][0], appearanceColorRanges[i][1]
		candidate := defaults[colorIndex]
		if colorIndex < len(appearance) {
			candidate = appearance[colorIndex]
		}
		// An unknown skin is no nearby colour: back to the default
		// (PlayerSave.ts:628-630).
		unknownSkin := colorIndex == appearanceSkinColour && (candidate < min || candidate > max)
		if unknownSkin {
			sanitized[colorIndex] = defaults[colorIndex]
		} else {
			sanitized[colorIndex] = clampInt(candidate, min, max)
		}
	}
	sanitized[appearanceGender] = gender
	return sanitized
}

func clampInt(value, min, max int) int {
	if value < min {
		return min
	}
	if value > max {
		return max
	}
	return value
}

// legacyFieldAttributes is LEGACY_FIELD_ATTRIBUTES (PlayerSave.ts:27-50):
// save fields that are player attributes now; an older save's value is
// read once on load. All 22 entries, in TS order. The five bank values
// are the Player.BANK_*_ATTRIBUTE string constants
// (Player.ts:87-91), verified verbatim.
var legacyFieldAttributes = map[string]string{
	"title":                        "loyalty:title",
	"xpLocked":                     "skills:xp-locked",
	"targetTeleportUnlocked":       "bounty-hunter:target-teleport-unlocked",
	"preserveUnlocked":             "prayer:preserve-unlocked",
	"rigourUnlocked":               "prayer:rigour-unlocked",
	"auguryUnlocked":               "prayer:augury-unlocked",
	"recoilDamage":                 "ring-of-recoil:damage",
	"crystalBowShotsInStage":       "crystal-bow:shots-in-stage",
	"crystalBowTrackedStageItemId": "crystal-bow:tracked-item",
	"totalKills":                   "killstreaks:total-kills",
	"killstreak":                   "killstreaks:streak",
	"highestKillstreak":            "killstreaks:highest-streak",
	"deaths":                       "killstreaks:deaths",
	"recentKills":                  "killstreaks:recent-kills",
	"points":                       "shop:points",
	"pcPoints":                     "pest-control:points",
	"pouches":                      "runecrafting:pouches",
	"bankNoteWithdrawal":           "bank:note-withdrawal",
	"bankInsertMode":               "bank:insert-mode",
	"bankQuantityMode":             "bank:quantity-mode",
	"bankCustomQuantity":           "bank:custom-quantity",
	"bankPlaceholders":             "bank:placeholders",
}

// migrateAttributeKey ports PlayerSave.migrateAttributeKey
// (PlayerSave.ts:84-89): a key already in the persistent set is kept;
// otherwise its kebab-case form is used when that form is persistent,
// else the key is kept as-is. The set consulted is the LIVE registry
// (attributes.go), including plugin-registered keys: a camelCase key
// whose kebab form is only plugin-persistent migrates, as it does in
// TS once the plugin has registered it.
func migrateAttributeKey(key string) string {
	if isPersistentAttributeKey(key) {
		return key
	}
	if kebab := toKebabCase(key); isPersistentAttributeKey(kebab) {
		return kebab
	}
	return key
}

// toKebabCase replicates the camelCase-to-kebab conversion in
// migrateAttributeKey: key.replace(/([a-z0-9])([A-Z])/g, "$1-$2")
// .toLowerCase() (PlayerSave.ts:83-87). A dash is inserted before an
// uppercase letter whose preceding character is a lowercase letter or
// digit; that preceding character can never have been consumed by an
// earlier match (a match's second character is always uppercase), so a
// single left-to-right pass over the original key equals the regex.
// The classes are ASCII-only, as in the JS regex.
func toKebabCase(key string) string {
	var out strings.Builder
	out.Grow(len(key) + 4)
	var prev rune
	for i, r := range key {
		if i > 0 && r >= 'A' && r <= 'Z' &&
			((prev >= 'a' && prev <= 'z') || (prev >= '0' && prev <= '9')) {
			out.WriteByte('-')
		}
		out.WriteRune(r)
		prev = r
	}
	return strings.ToLower(out.String())
}

// MigrateForEntry applies the entry-time save mutations in place, in
// the order applyToPlayer applies them (PlayerSave.ts:674-692):
// sanitize the appearance and write it back to the save, migrate
// attribute keys, then fold LEGACY_FIELD_ATTRIBUTES values found in
// Extra into Attributes.
//
// The legacy fold follows the applyToPlayer rule: a legacy field folds
// only when its value is non-null and the attribute is not already
// present (the attribute wins when present); a null legacy value folds
// nothing. In TS the fold writes onto the live player and fromPlayer
// later writes the persistent attributes back into the next save; in
// this merge form the carried save is the attribute store, so the fold
// writes into Attributes directly. Every legacy field key present in
// Extra is dropped from Extra afterwards, whether it folded or not: TS
// reads an older save's legacy value once on load (PlayerSave.ts:26
// comment) and fromPlayer builds a fresh save that never re-emits
// legacy fields, so keeping a consumed key in Extra would resurrect a
// field TS drops. Non-legacy Extra keys are untouched.
//
// When both the camelCase and kebab forms of a key are present in
// Attributes, the kebab form wins and the camelCase value is dropped.
// TS builds the migrated map with Object.fromEntries over insertion
// order, so its winner is insertion-order-dependent; the kebab-wins
// rule is the deterministic choice pinned here. Among multiple distinct
// camelCase spellings mapping to the same kebab form, the
// lexicographically smallest key wins (sorted iteration), also
// deterministic.
//
// Precondition: the receiver must be non-nil.
func (s *PlayerSave) MigrateForEntry() {
	s.Appearance = SanitizeAppearance(s.Appearance)
	if len(s.Attributes) > 0 {
		s.Attributes = migrateAttributeKeys(s.Attributes)
	}
	if len(s.Extra) > 0 {
		for field, key := range legacyFieldAttributes {
			raw, ok := s.Extra[field]
			if !ok {
				continue
			}
			if !isNullJSON(raw) {
				if _, present := s.Attributes[key]; !present {
					if s.Attributes == nil {
						s.Attributes = map[string]json.RawMessage{}
					}
					s.Attributes[key] = cloneRawMessage(raw)
				}
			}
			delete(s.Extra, field)
		}
	}
}

// migrateAttributeKeys renames attribute keys per migrateAttributeKey,
// deterministically: keys already in their migrated form are copied
// first (so the kebab form wins a collision), then renamed keys fill
// only targets no stable key claimed, in sorted-key order.
func migrateAttributeKeys(attrs map[string]json.RawMessage) map[string]json.RawMessage {
	keys := make([]string, 0, len(attrs))
	for key := range attrs {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	migrated := make(map[string]json.RawMessage, len(attrs))
	for _, key := range keys {
		if migrateAttributeKey(key) == key {
			migrated[key] = attrs[key]
		}
	}
	for _, key := range keys {
		target := migrateAttributeKey(key)
		if target == key {
			continue
		}
		if _, claimed := migrated[target]; !claimed {
			migrated[target] = attrs[key]
		}
	}
	return migrated
}

// isNullJSON reports whether raw is JSON null (or empty, which folds
// nothing just as null does). TS's fold condition is `!= null`
// (PlayerSave.ts:688).
func isNullJSON(raw json.RawMessage) bool {
	trimmed := bytes.TrimSpace(raw)
	return len(trimmed) == 0 || string(trimmed) == "null"
}

// Unmake ports PlayerSave.unmake (PlayerSave.ts:204-213) literally:
// mark the save unmade, keep only the remembered-login tokens
// attribute, map inventory/equipment element-wise to empty items (a nil
// array becomes an empty array, per `?? []`), empty the banks, and
// reset skills to a fresh Skills with level = maxLevel.
//
// The skill reset does NOT equal DefaultSkills(): a fresh Skills
// (SkillManager.ts:687-698) holds hitpoints experience
// getExperienceForLevel(10) = 1154 (EXP_ARRAY[9]), while
// DefaultSkills() follows the SkillManager constructor /
// defaultSkills override of 1184 (SkillManager.ts:55-62,
// JsonPlayerPersistence.plugin.js defaultSkills). Unmake uses the
// fresh-Skills value, 1154, as TS does. Verified, not assumed: the
// brief's suggested equality does not hold.
//
// Precondition: the receiver must be non-nil.
func (s *PlayerSave) Unmake() {
	s.Unmade = true
	var tokens json.RawMessage
	hasTokens := false
	if s.Attributes != nil {
		tokens, hasTokens = s.Attributes[LoginTokensAttribute]
	}
	if hasTokens {
		s.Attributes = map[string]json.RawMessage{LoginTokensAttribute: tokens}
	} else {
		s.Attributes = map[string]json.RawMessage{}
	}
	s.Inventory = emptyItems(len(s.Inventory))
	s.Equipment = emptyItems(len(s.Equipment))
	s.Banks = Banks{}
	s.Skills = freshSkills()
}

func emptyItems(n int) []Item {
	items := make([]Item, n)
	for i := range items {
		items[i] = EmptyItem()
	}
	return items
}

// freshSkillsHitpointsXP is the hitpoints experience of a fresh Skills:
// SkillManager.getExperienceForLevel(10) (SkillManager.ts:65-67),
// EXP_ARRAY[9] = 1154. It deliberately differs from DefaultHitpointsXP
// (1184); see Unmake.
const freshSkillsHitpointsXP = 1154

// freshSkills is `new Skills()` with level = [...maxLevel], as unmake
// builds it (PlayerSave.ts:211-212; Skills constructor,
// SkillManager.ts:687-698).
func freshSkills() Skills {
	skills := Skills{
		Level:      make([]int, SkillCount),
		MaxLevel:   make([]int, SkillCount),
		Experience: make([]int, SkillCount),
	}
	for i := range skills.MaxLevel {
		skills.MaxLevel[i] = 1
	}
	skills.Level[HitpointsIndex] = 10
	skills.MaxLevel[HitpointsIndex] = 10
	skills.Experience[HitpointsIndex] = freshSkillsHitpointsXP
	copy(skills.Level, skills.MaxLevel)
	return skills
}

// NewCharacterParams carries the inputs NewCharacterSave needs. Every
// other field of the save comes from TS new-player defaults.
type NewCharacterParams struct {
	Username     string
	PasswordHash string
	Profile      Profile
	Spawn        Location
	Developer    bool
	Now          time.Time
}

// defaultFightTypeJSON is the JSON.stringify of FightType.UNARMED_PUNCH,
// the Player field default (Player.ts:230). FightTypeClass has no
// toJSON, so serialization emits its constructor fields in order
// (FightType.ts:49-59, 60-77): animation/childId/attackType/style/sound
// from the UNARMED_PUNCH definition in
// data/definitions/item-combat-styles.json (animation 422, childId 0,
// CRUSH, ACCURATE, WEAPON_UNARMED_PUNCH), parentId 43
// (COMBAT_STYLE_PARENT_ID, FightType.ts:21), bonusType 2 (ATTACK_CRUSH,
// FightType.ts:11), style serializing as {} (a FightStyle subclass
// instance with no own properties, FightStyle.ts), attackSound
// Sound(2566, 1, 0, 0) with fields id/volume/delay/loopType
// (Sound.ts:90, 248-259), rapid false, npcAnimation -1.
const defaultFightTypeJSON = `{"animation":422,"parentId":43,"childId":0,"bonusType":2,"style":{},"attackSound":{"id":2566,"volume":1,"delay":0,"loopType":0},"rapid":false,"npcAnimation":-1}`

// defaultTeleportTypeJSON is the JSON.stringify of TeleportType.NORMAL,
// the teleport type of MagicSpellbook.NORMAL (MagicSpellbook.ts:12).
// TeleportType has no toJSON; its constructor assigns fields in the
// order options, startTick, startAnim, middleAnim, endAnim,
// startGraphic, middleGraphic, endGraphic (TeleportType.ts:36-46, 102-113).
// NORMAL (TeleportType.ts:36-46): startTick 3, startAnim Animation(714,
// HIGH) = {id, delay 0, priority 2} (Animation.ts, Priority HIGH = 2),
// endAnim the default reset Animation(65535) = {id, delay 0, priority
// 0}, startGraphic Graphic(111, 0, 92) = {id, delay 0, height 92,
// priority 0} (Graphic.ts), options {busy, repeatEndAnimation,
// areaSoundRange 10}. Hydration keys off interfaceId only
// (JsonPlayerPersistence hydrateSpellbook), so this shape is preserved
// verbatim and never interpreted here.
const defaultTeleportTypeJSON = `{"options":{"busy":true,"repeatEndAnimation":true,"areaSoundRange":10},"startTick":3,"startAnim":{"id":714,"delay":0,"priority":2},"middleAnim":null,"endAnim":{"id":65535,"delay":0,"priority":0},"startGraphic":{"id":111,"delay":0,"height":92,"priority":0},"middleGraphic":null,"endGraphic":null}`

// emptySailingStateJSON is emptySailingState() (SailingState.ts):
// normalizeSailingState(undefined) returns it, and a fresh Player's
// sailing field holds it (Player.ts:141), so fromPlayer writes this
// object for a new character (PlayerSave.ts:810).
const emptySailingStateJSON = `{"boats":[],"activeBoatSlot":null,"returnPoint":null,"tools":[],"lastDock":null,"lastStandardDock":null}`

// defaultQuickPrayerCount is the length of a new character's
// quick-prayer array: QuickPrayers.prayers has one entry per
// PRAYER_VALUES entry, all null (QuickPrayers.ts:156-159), and
// PrayerData.values() yields 30 prayers including Zeal
// (PrayerHandler.ts:599-630 statics, values() at 688). Hydration pads /
// truncates to the same length (hydrateQuickPrayers).
const defaultQuickPrayerCount = 30

// NewCharacterSave builds a new-character save: every field at the TS
// new-player default, grounded field-by-field in the citations below.
// The result passes validateSaveForWrite. A zero Now falls back to the
// current time (TS constructs the creation date as `new Date()` at
// Player construction, Player.ts:211); callers normally pass the login
// time explicitly. Profile is stored as given ("" means main, per
// EffectiveProfile).
//
// Defaults, field by field:
//   - displayName / passwordHashWithSalt: the params (the name as first
//     typed; the hash as supplied by the login path).
//   - creationDate: Now, epoch milliseconds (PlayerSave.ts:744,
//     creationDate.getTime()).
//   - isDiscordLogin false, cachedDiscordAccessToken "" (Player field
//     defaults; hydratePlayerSave fallbacks).
//   - rights: DEVELOPER (id 2, sprite 619) when Developer, else NONE,
//     via rightsByID (PlayerRights.ts; the brief pins the dev grant).
//   - donatorRights: NONE via donatorRightsByID(0) (Player.ts:238,
//     DonatorRights.NONE field default).
//   - position: Spawn. spellBook: NORMAL, interface 1151
//     (Player.ts:201; MagicSpellbook.ts:12) with its teleportType JSON.
//   - fightType: UNARMED_PUNCH JSON (Player.ts:230).
//   - autocastSpellId -1 (fromPlayer's `?? -1`, PlayerSave.ts:756).
//   - autoRetaliate true (Player.ts:232).
//   - audioSettings: DEFAULT_AUDIO_SETTINGS (Player.ts:70-81): varp 19
//     is Music.SETTINGS_VARP with DEFAULT_SETTINGS = 1 << LOOP_BIT = 1
//     (Music.ts:88-104).
//   - hasVengeance false, lastVengeanceTimer 0 (fresh SecondsTimer;
//     hydrate fallbacks), specPercentage 100 (Mobile.ts:133),
//     poisonDamage 0, venomed false, no lastVenomDamage / poisonClock
//     (fromPlayer omits both unless venomed/poisoned,
//     PlayerSave.ts:765-770), poisonImmunityTimer 0,
//     fireImmunityTimer 0, teleblockTimer 0 (PlayerSave.ts:775),
//     skullTimer 0, skullType WHITE_SKULL icon 0 (Player.ts:278-279 in
//     resetAttributes; hydrateSkullType defaults icon 0).
//   - running true (Player.ts:160), runEnergy 100 (RUN_ENERGY_MAX_UNITS
//     10_000 / 100, Player.ts:185, GameConstants.ts:30, getRunEnergy
//     Player.ts:1258).
//   - inventory/equipment: 28/14 empty items (ItemContainer fills with
//     Item(-1, 0), ItemContainer.ts:55-56; Inventory/Equipment
//     capacities 28/14).
//   - appearance: the male default (Appearance.set(), Appearance.ts).
//   - skills: DefaultSkills() (the SkillManager constructor override,
//     SkillManager.ts:55-62: 1s, hitpoints 10/10/1184).
//   - quickPrayers: 30 nulls (see defaultQuickPrayerCount).
//   - friends/ignores: empty; friendRanks: empty; friends-chat name/
//     owner "": entry/talk rank -1, kick rank 2 (PlayerRelations.ts:
//     17-22).
//   - banks: empty; flags: empty; sailing: the empty state (above).
//   - attributes: {} — a fresh player holds only default-valued
//     attributes (applyAttributeDefaults, PlayerSave.ts:76-80), which
//     fromPlayer strips (PlayerSave.ts:811-816).
//   - unmade: false.
func NewCharacterSave(p NewCharacterParams) *PlayerSave {
	now := p.Now
	if now.IsZero() {
		now = time.Now()
	}
	rights := rightsByID(0)
	if p.Developer {
		rights = rightsByID(2)
	}
	spawn := p.Spawn
	return &PlayerSave{
		DisplayName:              p.Username,
		CreationDate:             now.UnixMilli(),
		PasswordHashWithSalt:     p.PasswordHash,
		IsDiscordLogin:           false,
		CachedDiscordAccessToken: "",
		Rights:                   rights,
		DonatorRights:            donatorRightsByID(0),
		Position:                 &Location{X: spawn.X, Y: spawn.Y, Z: spawn.Z},
		SpellBook: SpellBook{
			InterfaceID:  SpellbookNormalInterfaceID,
			TeleportType: json.RawMessage(defaultTeleportTypeJSON),
		},
		FightType:              json.RawMessage(defaultFightTypeJSON),
		AutocastSpellId:        -1,
		AutoRetaliate:          true,
		AudioSettings:          map[int]int{18: 0, 19: 1, 168: 100, 169: 100, 872: 100, 3796: 100, 3109: 0, 3797: 0},
		HasVengeance:           false,
		LastVengeanceTimer:     0,
		SpecPercentage:         100,
		PoisonDamage:           0,
		Venomed:                false,
		LastVenomDamage:        nil,
		PoisonImmunityTimer:    0,
		PoisonClock:            nil,
		FireImmunityTimer:      0,
		TeleblockTimer:         0,
		SkullTimer:             0,
		SkullType:              SkullType{IconID: 0},
		Running:                true,
		RunEnergy:              100,
		Inventory:              emptyItems(InventorySize),
		Equipment:              emptyItems(EquipmentSize),
		Appearance:             append([]int(nil), defaultMaleAppearance...),
		Skills:                 DefaultSkills(),
		QuickPrayers:           make([]*QuickPrayer, defaultQuickPrayerCount),
		Friends:                []string{},
		Ignores:                []string{},
		FriendRanks:            map[string]int{},
		FriendsChatChannelName: "",
		FriendsChatLastOwner:   "",
		FriendsChatEntryRank:   -1,
		FriendsChatTalkRank:    -1,
		FriendsChatKickRank:    2,
		Banks:                  Banks{},
		Flags:                  []string{},
		Sailing:                json.RawMessage(emptySailingStateJSON),
		Attributes:             map[string]json.RawMessage{},
		Unmade:                 false,
		Profile:                p.Profile,
		Extra:                  nil,
	}
}
