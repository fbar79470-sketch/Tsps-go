// ClueNests is the Go port of the ClueNests sub-unit
// (server/plugins/skills/woodcutting/ClueNests.Woodcutting.js,
// 113 lines, commit bfd81e3): clue nests roll on every successful
// log. Every log rolls each clue tier the player does not already
// own, at 1 in floor(floor(base x CA) / (100 + Woodcutting) x tier
// modifier), where base is the tree's beaver rate. The matching
// combat-achievements reward reduces base by 5%, and an imbued
// ring of wealth doubles non-beginner clue chances while chopping
// in the Wilderness. Opening a nest gives the clue scroll and an
// empty bird nest.
//
// Line numbers in comments cite ClueNests.Woodcutting.js unless
// another file is named.
//
// Ids: the cache knows NO gameval names for clue nests or clue
// scrolls (probe, 2026-11-20: "clue_nest_beginner" etc. do not
// resolve), so the nest/scroll ids are numeric constants from
// ItemIdentifiers.ts, each verified by display name against the
// real rev-242 cache in the probe — the same grounding the
// woodcutting.go id block uses for ids outside the tables. The
// ring ids are the same: six have gamevals, but the three charged
// (__2) variants do not, so all eight are constants here rather
// than splitting the resolution. The empty nest reuses the
// resolved Tables.BirdNests.EmptyNest (5075).
//
// Deviations forced by missing Go seams, named inline:
//
//   - Every custom event here is a query with no Go answerer yet
//     ("treasure-trails:vessel-chance", "combat-achievements:
//     tier", "treasure-trails:can-receive", "treasure-trails:
//     missed-clue", "treasure-trails:clue-item", "treasure-trails:
//     open-vessel"), so each degrades to its documented default:
//     multiplier 1, CA unmet, inventory + all 11 bank tabs name
//     scan, ground drop. The request shapes are the TS shapes, so
//     a future answerer (Treasure Trails, Combat Achievements,
//     leagues) can answer without a port change.
//   - The "treasure-trails:clue-item" box-true path has NO Go
//     equivalent and is NOT built: it needs forceAdd (an
//     inventory add that ignores free slots), which Go has no
//     form of, and no answerer can set box true today. The roll
//     always ground-drops the nest (the box-false path).
//   - The Wilderness ring check reads the *game.Wilderness
//     through resolveWilderness: lazily, from the Api's AreaManager
//     (whose dynamic type is *game.AreaManager in production —
//     installed via Hub.SetAreaManager). A nil Wilderness reads as
//     not in the Wilderness, so the ring never doubles without one.
//   - XP is int throughout (SkillManager.AddExperiences takes an
//     int); the clue roll itself grants no XP, so no truncation
//     applies here.
package woodcutting

import (
	"math"

	"tsps-go/internal/game"
	"tsps-go/internal/plugins"
)

// ---- custom-event names (ClueNests.Woodcutting.js) ----

const (
	// treasureTrailsVesselChanceEventName is the
	// "treasure-trails:vessel-chance" query: content may make
	// clue nests commoner by raising Multiplier above 1, or block
	// a tier by setting Allowed false.
	treasureTrailsVesselChanceEventName = "treasure-trails:vessel-chance"
	// combatAchievementsTierEventName is the
	// "combat-achievements:tier" query: content reports whether
	// the player met the tier's threshold (non-beginner tiers
	// only).
	combatAchievementsTierEventName = "combat-achievements:tier"
	// treasureTrailsCanReceiveEventName is the
	// "treasure-trails:can-receive" query: content reports whether
	// the player may receive a clue of the tier.
	treasureTrailsCanReceiveEventName = "treasure-trails:can-receive"
	// treasureTrailsMissedClueEventName is the
	// "treasure-trails:missed-clue" emit: the player owned the
	// clue already, so the roll was lost.
	treasureTrailsMissedClueEventName = "treasure-trails:missed-clue"
	// treasureTrailsClueItemEventName is the
	// "treasure-trails:clue-item" request: content may take the
	// nest before it falls by setting Box true — see the file
	// header, the box-true path has no Go equivalent.
	treasureTrailsClueItemEventName = "treasure-trails:clue-item"
	// treasureTrailsOpenVesselEventName is the
	// "treasure-trails:open-vessel" request: content may take over
	// opening the nest by setting Handled true.
	treasureTrailsOpenVesselEventName = "treasure-trails:open-vessel"
)

// ---- ids (ItemIdentifiers.ts, probe-verified 2026-11-20) ----

// clueNestIDs / clueScrollIDs are the tier nest/scroll ids in
// buildTiers order (beginner, easy, medium, hard, elite):
// CLUE_NEST_BEGINNER_ 23127 / CLUE_SCROLL_BEGINNER_ 23182,
// CLUE_NEST_EASY_ 19712 / CLUE_SCROLL_EASY_ 2677,
// CLUE_NEST_MEDIUM_ 19714 / CLUE_SCROLL_MEDIUM_ 2801,
// CLUE_NEST_HARD_ 19716 / CLUE_SCROLL_HARD_ 2722,
// CLUE_NEST_ELITE_ 19718 / CLUE_SCROLL_ELITE_ 12073.
// Each display name was verified against the real rev-242 cache
// ("Clue nest (beginner)" etc.).
var clueNestIDs = []int{23127, 19712, 19714, 19716, 19718}
var clueScrollIDs = []int{23182, 2677, 2801, 2722, 12073}

// clueTierNames are the tier names in buildTiers order.
var clueTierNames = []string{"beginner", "easy", "medium", "hard", "elite"}

// clueTierModifiers are the tier chance modifiers in buildTiers
// order (buildTiers).
var clueTierModifiers = []float64{0.2, 1.7, 2, 3.3, 10}

// clueRingOfWealthIDs are the imbued ring of wealth ids the
// Wilderness check tests (rollClueNests: rings): RING_OF_WEALTH_I_
// 12785, I1_ 20790, I2_ 20789, I3_ 20788, I4_ 20787, I5_ 20786,
// I1__2 21459, I5__2 21458. Each display name was verified against
// the real rev-242 cache ("Ring of wealth (i)", "(i1)" … "(i5)").
// RING_OF_WEALTH_I__2 (15089) is NOT in the TS list and is not
// included here.
var clueRingOfWealthIDs = []int{12785, 20790, 20789, 20788, 20787, 20786, 21459, 21458}

// ---- custom-event payloads ----
// Each mirrors the TS request object of the same name; a handler
// answers by mutating the payload in place, exactly as TS does.

// VesselChanceRequest is the "treasure-trails:vessel-chance" query:
// content may make clue nests commoner (a league relic) by raising
// Multiplier above 1, or block a tier by setting Allowed to false.
// Allowed is nil when unanswered (the TS `allowed` starts absent);
// only an explicit false blocks the tier.
type VesselChanceRequest struct {
	Player     *game.Player
	Vessel     string
	Tier       string
	Multiplier float64
	Allowed    *bool
}

// CombatAchievementsTierRequest is the "combat-achievements:tier"
// query: CurrentThresholdMet answers whether the player met the
// tier's threshold; it defaults false.
type CombatAchievementsTierRequest struct {
	Player              *game.Player
	Tier                string
	CurrentThresholdMet bool
}

// CanReceiveClueRequest is the "treasure-trails:can-receive" query:
// Allowed answers whether the player may receive a clue of the
// tier; nil when unanswered, in which case ownsClue falls back to
// the inventory + bank name scan.
type CanReceiveClueRequest struct {
	Player  *game.Player
	Tier    string
	Allowed *bool
}

// MissedClueEvent is the "treasure-trails:missed-clue" emit: the
// player owned the clue already, so the roll was lost.
type MissedClueEvent struct {
	Player *game.Player
	Tier   string
}

// ClueItemRequest is the "treasure-trails:clue-item" request: Box
// answers whether content takes the nest into the inventory
// (forceAdd); it defaults false, and the box-true path has no Go
// equivalent (see the file header), so the roll always takes the
// ground-drop path.
type ClueItemRequest struct {
	Player *game.Player
	ItemID int
	Box    bool
}

// OpenVesselRequest is the "treasure-trails:open-vessel" request:
// Handled answers whether content took over opening the nest; it
// defaults false.
type OpenVesselRequest struct {
	Player  *game.Player
	ItemID  int
	Slot    int
	Handled bool
}

// ---- tiers ----

// clueTier is one row of the buildTiers table.
type clueTier struct {
	name     string
	modifier float64
	nestID   int
	scrollID int
}

// buildClueTiers builds the five tiers (buildTiers): modifiers
// 0.2/1.7/2/3.3/10, nest + scroll ids from the constants above.
func buildClueTiers() []clueTier {
	tiers := make([]clueTier, 0, len(clueTierNames))
	for i, name := range clueTierNames {
		tiers = append(tiers, clueTier{
			name:     name,
			modifier: clueTierModifiers[i],
			nestID:   clueNestIDs[i],
			scrollID: clueScrollIDs[i],
		})
	}
	return tiers
}

// clueTiers is the built tier table (attach: tiers =
// buildTiers(core.ItemIdentifiers)). Immutable after build.
var clueTiers = buildClueTiers()

// clueTierByNestID finds the tier for a nest id, or false.
func clueTierByNestID(nestID int) (clueTier, bool) {
	for _, tier := range clueTiers {
		if tier.nestID == nestID {
			return tier, true
		}
	}
	return clueTier{}, false
}

// clueFormNames are the five clue forms for a tier (ownsClue):
// "Clue scroll (tier)", "Clue bottle (tier)", "Clue nest (tier)",
// "Clue geode (tier)", "Scroll box (tier)".
func clueFormNames(tier string) map[string]bool {
	names := make(map[string]bool, 5)
	for _, form := range []string{"Clue scroll", "Clue bottle", "Clue nest", "Clue geode", "Scroll box"} {
		names[form+" ("+tier+")"] = true
	}
	return names
}

// resolveWilderness returns the world's Wilderness for the ring
// check, or nil (the woodcutting taskManager/objectManager
// pattern: the live Api value wins when present; the directly-set
// wilderness field is the unit-test override). The Api's
// AreaManager is *game.AreaManager in production (installed via
// Hub.SetAreaManager); a differently-typed manager reads as no
// Wilderness, so the ring never doubles without one.
func (w *woodcutting) resolveWilderness() *game.Wilderness {
	if w.wilderness != nil {
		return w.wilderness
	}
	if w.api != nil {
		if am, ok := w.api.GetAreaManager().(*game.AreaManager); ok && am != nil {
			return am.Wilderness()
		}
	}
	return nil
}

// hasWildernessRing reports whether the player wears an imbued
// ring of wealth while chopping in the Wilderness (rollClueNests:
// location in the Wilderness, wilderness level > 0, and any ring
// id equipped). Equipment only — an inventory ring does not count.
func (w *woodcutting) hasWildernessRing(player *game.Player) bool {
	if player == nil {
		return false
	}
	loc := player.Location()
	wild := w.resolveWilderness()
	if wild == nil || !wild.IsInLocation(loc) || wild.LevelAt(loc.X, loc.Y) <= 0 {
		return false
	}
	equipment := player.Equipment()
	if equipment == nil {
		return false
	}
	for _, id := range clueRingOfWealthIDs {
		if equipment.Contains(id) {
			return true
		}
	}
	return false
}

// ownsClue reports whether the player already owns a clue of the
// tier, in any of its forms, carried or banked (ownsClue). The
// "treasure-trails:can-receive" query answers first (Allowed false
// means owned); unanswered, the inventory and all 11 bank tabs are
// scanned by display name.
func (w *woodcutting) ownsClue(player *game.Player, tier clueTier) bool {
	if player == nil {
		return false
	}
	request := &CanReceiveClueRequest{Player: player, Tier: tier.name}
	w.emitEvent(treasureTrailsCanReceiveEventName, request)
	if request.Allowed != nil {
		return !*request.Allowed
	}
	names := clueFormNames(tier.name)
	contains := func(items []game.Item) bool {
		for _, item := range items {
			if item.Amount() <= 0 {
				continue
			}
			if names[player.ItemName(item.ID())] {
				return true
			}
		}
		return false
	}
	if inventory := player.Inventory(); inventory != nil && contains(inventory.Items()) {
		return true
	}
	for tab := 0; tab < game.TotalBankTabs; tab++ {
		if bank := player.Bank(tab); bank != nil && contains(bank.Items()) {
			return true
		}
	}
	return false
}

// clueChance computes one tier's roll chance (rollClueNests): base
// = floor(floor(petBase x 0.95 if the CA threshold is met else
// petBase) / (100 + level) x modifier); chance = base when base <
// 1, else max(1, floor(base / multiplier)) with a multiplier <= 1
// reading as 1. A chance < 1 means the tier never rolls.
func clueChance(petBase, level int, modifier float64, thresholdMet bool, multiplier float64) (base, chance int) {
	adjusted := petBase
	if thresholdMet {
		adjusted = petBase * 95 / 100
	}
	base = int(math.Floor(float64(adjusted) / float64(100+level) * modifier))
	chance = base
	if base >= 1 {
		if multiplier <= 1 {
			multiplier = 1
		}
		chance = max(1, int(math.Floor(float64(base)/multiplier)))
	}
	return base, chance
}

// rollClueNests rolls every clue tier the player does not already
// own on a successful log (rollClueNests), in TS order per tier:
// vessel-chance query (Allowed false skips the tier) →
// combat-achievements query (non-beginner only; met reduces the
// pet base by 5%) → base = floor(floor(petBase) / (100 + level) x
// modifier) → chance = max(1, floor(base / multiplier)) when
// base >= 1, else base → roll (random x chance >= ring multiplier
// skips; the ring doubles non-beginner chances in the Wilderness)
// → ownsClue (missed-clue emit + skip) → clue-item emit → ground
// drop + red nest message.
//
// The box-true path (forceAdd + a BIRD_NEST_6 ground drop on
// members worlds) is NOT built: Go has no forceAdd equivalent and
// no answerer can set Box — see the file header. Trees with no
// pet base never roll.
func (w *woodcutting) rollClueNests(player *game.Player, tree *Tree) {
	if player == nil || tree == nil || tree.PetBase == 0 {
		return
	}
	level := woodcuttingLevel(player)
	wildernessRing := w.hasWildernessRing(player)
	for _, tier := range clueTiers {
		boost := &VesselChanceRequest{Player: player, Vessel: "nest", Tier: tier.name, Multiplier: 1}
		w.emitEvent(treasureTrailsVesselChanceEventName, boost)
		if boost.Allowed != nil && !*boost.Allowed {
			continue
		}
		thresholdMet := false
		if tier.name != "beginner" {
			achievement := &CombatAchievementsTierRequest{Player: player, Tier: tier.name}
			w.emitEvent(combatAchievementsTierEventName, achievement)
			thresholdMet = achievement.CurrentThresholdMet
		}
		_, chance := clueChance(tree.PetBase, level, tier.modifier, thresholdMet, boost.Multiplier)
		ringMultiplier := 1
		if wildernessRing && tier.name != "beginner" {
			ringMultiplier = 2
		}
		if chance < 1 || w.randFloat()*float64(chance) >= float64(ringMultiplier) {
			continue
		}
		if w.ownsClue(player, tier) {
			w.emitEvent(treasureTrailsMissedClueEventName, &MissedClueEvent{Player: player, Tier: tier.name})
			continue
		}
		reward := &ClueItemRequest{Player: player, ItemID: tier.nestID}
		w.emitEvent(treasureTrailsClueItemEventName, reward)
		// Box true would forceAdd the nest and ground-drop an
		// empty nest on members worlds — unbuildable (no forceAdd
		// in Go) and unanswerable today, so the ground-drop path
		// runs unconditionally; see the file header.
		w.dropGroundItem(player, game.NewItem(tier.nestID, 1))
		player.SendMessage("<col=ff0000>A bird's nest falls out of the tree.")
	}
}

// openClueNest opens a clue nest from the inventory (openClueNest):
// the "treasure-trails:open-vessel" request first (Handled takes
// over); the default path checks the slot identity, the free-slot
// gate, then deletes the nest and adds an empty nest + the clue
// scroll. It always claims the event when the tier matches (true
// in every TS return path after the tier lookup); an unknown nest
// id falls through (false).
//
// The TS item-identity term (event.item !== the slot item) has no
// Go form — PluginItemActionEvent carries no Item — so the slot
// identity is the whole check. With no tables the empty-nest id is
// unknown and the nest falls through (false).
func (w *woodcutting) openClueNest(event *plugins.PluginItemActionEvent) bool {
	if event == nil {
		return false
	}
	request := &OpenVesselRequest{Player: event.Player, ItemID: event.ItemID, Slot: event.Slot}
	w.emitEvent(treasureTrailsOpenVesselEventName, request)
	if request.Handled {
		return true
	}
	player := event.Player
	if player == nil || w.tables == nil {
		return false
	}
	tier, ok := clueTierByNestID(event.ItemID)
	if !ok {
		return false
	}
	inventory := player.Inventory()
	if inventory == nil {
		return true
	}
	item := inventory.ItemAt(event.Slot)
	if item.ID() != tier.nestID || item.Amount() < 1 {
		return true
	}
	if inventory.GetFreeSlots() <= 0 {
		player.SendMessage("You need a free inventory space to open the bird's nest.")
		return true
	}
	inventory.DeleteAtSlot(event.Slot, 1, true)
	inventory.Adds(w.tables.BirdNests.EmptyNest, 1)
	inventory.Adds(tier.scrollID, 1)
	player.SendMessage("You take a clue scroll out of the bird's nest.")
	return true
}

// registerClueNests wires the ClueNests registrations (attach):
// onItemAction "Open" per tier name ("Clue nest (tier)").
func (w *woodcutting) registerClueNests(api *plugins.Api) {
	for _, tier := range clueTiers {
		api.OnItemActionNamed("Clue nest ("+tier.name+")", map[string]plugins.ItemActionHandler{"Open": w.openClueNest}, nil)
	}
}
