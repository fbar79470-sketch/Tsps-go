// ClueBottles is the Go port of plugins/skills/fishing/
// ClueBottles.Fishing.js (commit bfd81e3): ordinary
// fishing clue bottles — every landed fish rolls each clue
// tier at 1 in floor(adjusted·tenths / ((100 + level)·10)),
// where the combat-achievements threshold met for the tier
// reduces the base by 5%, and an imbued ring of wealth
// doubles non-beginner chances while fishing in the
// Wilderness (not beginner).
//
// Line numbers in comments cite ClueBottles.Fishing.js
// unless another file is named.
//
// The can-receive / box-true handling follows
// internal/content/woodcutting/cluenests.go EXACTLY
// (rollClueNests, cluenests.go:270-390): the
// "treasure-trails:can-receive" query answers first
// (Allowed false means the player already owns the tier),
// and unanswered it falls back to an inventory + all 11
// bank tabs scan by display name ("Clue scroll (tier)",
// "Clue bottle (tier)", "Clue nest (tier)", "Clue geode
// (tier)", "Scroll box (tier)"); an owned tier emits
// "treasure-trails:missed-clue" and is skipped.
//
// WHAT LANDS AND WHAT NEVER LANDS: TS forceAdds the bottle
// into the inventory
// (player.getInventory().forceAdd(...)), even when the
// inventory is full. Go has NO forceAdd equivalent, and no
// answerer can set Box on the "treasure-trails:clue-item"
// request today — so the box-true path is NOT built, and
// the ground-drop path runs unconditionally, exactly as
// cluenests.go does for nests: the bottle falls to the
// ground at the player's feet (game.Hooks.DropGroundItem)
// and must be picked up; it never goes directly into the
// inventory. With no ground-item hook installed the drop
// is a no-op and the bottle is lost. TS sends no chat line
// for a bottle roll, so none is sent here either.
//
// Every custom event here is a query with no Go answerer
// yet ("treasure-trails:vessel-chance",
// "combat-achievements:tier", "treasure-trails:can-receive",
// "treasure-trails:missed-clue", "treasure-trails:clue-item"),
// so each degrades to its documented default: multiplier 1,
// CA unmet, inventory + bank name scan, ground drop. The
// request shapes are the TS shapes, so a future answerer
// (Treasure Trails, Combat Achievements, leagues) can
// answer without a port change.
package fishing

import (
	"math"
	"math/rand/v2"

	"tsps-go/internal/game"
)

// ---- custom-event names (ClueBottles.Fishing.js) ----

const (
	// treasureTrailsVesselChanceEventName is the
	// "treasure-trails:vessel-chance" query: content may
	// make clue bottles commoner by raising Multiplier
	// above 1, or block a tier by setting Allowed false.
	treasureTrailsVesselChanceEventName = "treasure-trails:vessel-chance"
	// combatAchievementsTierEventName is the
	// "combat-achievements:tier" query: content reports
	// whether the player met the tier's threshold
	// (non-beginner tiers only).
	combatAchievementsTierEventName = "combat-achievements:tier"
	// treasureTrailsCanReceiveEventName is the
	// "treasure-trails:can-receive" query: content reports
	// whether the player may receive a clue of the tier.
	treasureTrailsCanReceiveEventName = "treasure-trails:can-receive"
	// treasureTrailsMissedClueEventName is the
	// "treasure-trails:missed-clue" emit: the player owned
	// the clue already, so the roll was lost.
	treasureTrailsMissedClueEventName = "treasure-trails:missed-clue"
	// treasureTrailsClueItemEventName is the
	// "treasure-trails:clue-item" request: content may take
	// the bottle before it falls by setting Box true — see
	// the file header, the box-true path has no Go
	// equivalent.
	treasureTrailsClueItemEventName = "treasure-trails:clue-item"
)

// ---- custom-event payloads ----
// Each mirrors the TS request object of the same name; a
// handler answers by mutating the payload in place, exactly
// as TS does. (The same shapes as woodcutting's
// cluenests.go; fishing-local because those are
// unexported.)

// VesselChanceRequest is the "treasure-trails:vessel-chance"
// query: content may make clue bottles commoner (a league
// relic) by raising Multiplier above 1, or block a tier by
// setting Allowed to false. Allowed is nil when unanswered
// (the TS `allowed` starts absent); only an explicit false
// blocks the tier.
type VesselChanceRequest struct {
	Player     *game.Player
	Vessel     string
	Tier       string
	Multiplier float64
	Allowed    *bool
}

// CombatAchievementsTierRequest is the
// "combat-achievements:tier" query: CurrentThresholdMet
// answers whether the player met the tier's threshold; it
// defaults false.
type CombatAchievementsTierRequest struct {
	Player              *game.Player
	Tier                string
	CurrentThresholdMet bool
}

// CanReceiveClueRequest is the "treasure-trails:can-receive"
// query: Allowed answers whether the player may receive a
// clue of the tier; nil when unanswered, in which case
// ownsClueBottle falls back to the inventory + bank name
// scan.
type CanReceiveClueRequest struct {
	Player  *game.Player
	Tier    string
	Allowed *bool
}

// MissedClueEvent is the "treasure-trails:missed-clue"
// emit: the player owned the clue already, so the roll was
// lost.
type MissedClueEvent struct {
	Player *game.Player
	Tier   string
}

// ClueItemRequest is the "treasure-trails:clue-item"
// request: Box answers whether content takes the bottle
// into the inventory (forceAdd); it defaults false, and
// the box-true path has no Go equivalent (see the file
// header), so the roll always takes the ground-drop path.
type ClueItemRequest struct {
	Player *game.Player
	ItemID int
	Box    bool
}

// ---- tiers ----

// clueBottleTier is one row of the tiers table
// (ClueBottles.Fishing.js attach): the tier name, its
// tenths, and its bottle item id.
type clueBottleTier struct {
	name     string
	tenths   int
	bottleID int
}

// clueBottleTiers builds the five tiers (attach): tenths
// 2/17/20/33/100, bottle ids from the resolved FishingIDs.
func (f *fishing) clueBottleTiers() []clueBottleTier {
	if f == nil || f.ids == nil {
		return nil
	}
	return []clueBottleTier{
		{name: "beginner", tenths: 2, bottleID: f.ids.ClueBottleBeginner},
		{name: "easy", tenths: 17, bottleID: f.ids.ClueBottleEasy},
		{name: "medium", tenths: 20, bottleID: f.ids.ClueBottleMedium},
		{name: "hard", tenths: 33, bottleID: f.ids.ClueBottleHard},
		{name: "elite", tenths: 100, bottleID: f.ids.ClueBottleElite},
	}
}

// clueBottleFormNames are the five clue forms for a tier
// (ownsClue, ClueNests.Woodcutting.js:29-31): "Clue scroll
// (tier)", "Clue bottle (tier)", "Clue nest (tier)", "Clue
// geode (tier)", "Scroll box (tier)".
func clueBottleFormNames(tier string) map[string]bool {
	names := make(map[string]bool, 5)
	for _, form := range []string{"Clue scroll", "Clue bottle", "Clue nest", "Clue geode", "Scroll box"} {
		names[form+" ("+tier+")"] = true
	}
	return names
}

// ownsClueBottle reports whether the player already owns a
// clue of the tier, in any of its forms, carried or banked
// (ownsClue, ClueNests.Woodcutting.js:24-42, the
// cluenests.go precedent). The
// "treasure-trails:can-receive" query answers first
// (Allowed false means owned); unanswered, the inventory
// and all 11 bank tabs are scanned by display name.
func (f *fishing) ownsClueBottle(player *game.Player, tier string) bool {
	if player == nil {
		return false
	}
	request := &CanReceiveClueRequest{Player: player, Tier: tier}
	f.emitCustomEvent(treasureTrailsCanReceiveEventName, request)
	if request.Allowed != nil {
		return !*request.Allowed
	}
	names := clueBottleFormNames(tier)
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

// hasWildernessRing reports whether the player wears an
// imbued ring of wealth while fishing in the Wilderness
// (rollClueBottles: location in the Wilderness, wilderness
// level > 0, and any ring id equipped). Equipment only —
// an inventory ring does not count. A nil Wilderness reads
// as not in the Wilderness, so the ring never doubles
// without one (world.go).
func (f *fishing) hasWildernessRing(player *game.Player) bool {
	if player == nil || f == nil || f.ids == nil {
		return false
	}
	wild := wilderness()
	if wild == nil {
		return false
	}
	loc := player.Location()
	if !wild.IsInLocation(loc) || wild.LevelAt(loc.X, loc.Y) <= 0 {
		return false
	}
	equipment := player.Equipment()
	if equipment == nil {
		return false
	}
	for _, id := range f.ids.ImbuedRings {
		if equipment.Contains(id) {
			return true
		}
	}
	return false
}

// bottleDenominator is one tier's roll denominator
// (bottleDenominator, ClueBottles.Fishing.js:7-10): the 95%
// combat-achievement adjustment first (the base reduced by
// 5% when the tier's threshold is met), then
// floor(adjusted·tenths / ((100+level)·10)). A denominator
// below 1 means the tier never rolls.
func bottleDenominator(base, level, tenths int, combatAchievement bool) int {
	adjusted := base
	if combatAchievement {
		adjusted = base * 95 / 100
	}
	return adjusted * tenths / ((100 + level) * 10)
}

// dropGroundItem drops an item at the player's feet
// (the woodcutting dropGroundItem shape). A nil hook is a
// no-op.
func dropGroundItem(player *game.Player, item game.Item) {
	if player == nil || game.Hooks.DropGroundItem == nil {
		return
	}
	game.Hooks.DropGroundItem(item, player.Location(), player)
}

// rollClueBottles rolls every clue tier the player does not
// already own on a landed fish (rollClueBottles,
// ClueBottles.Fishing.js:12-54), in TS order per tier:
// vessel-chance query (Allowed false skips the tier) →
// combat-achievements query (non-beginner only; met
// reduces the base by 5%) → denominator → roll (random ×
// denominator >= chance multiplier skips; the ring doubles
// non-beginner chances in the Wilderness) → ownsClue
// (missed-clue emit + skip) → clue-item emit → ground
// drop. Non-beginner tiers are skipped on free worlds. The
// box-true path (forceAdd into the inventory) is NOT built:
// Go has no forceAdd — see the file header. A base of 0 or
// less rolls nothing (the tutorial spot's tools carry no
// clue base; the loop skips the tutorial spot id 3317
// itself).
func (f *fishing) rollClueBottles(player *game.Player, base int) {
	if player == nil || base <= 0 || f == nil || f.ids == nil {
		return
	}
	skills := player.Skills()
	level := 0
	if skills != nil {
		level = skills.CurrentLevel(game.SkillFishing)
	}
	wildernessRing := f.hasWildernessRing(player)
	members := f.membersWorld
	for _, tier := range f.clueBottleTiers() {
		if !members && tier.name != "beginner" {
			continue
		}
		boost := &VesselChanceRequest{Player: player, Vessel: "bottle", Tier: tier.name, Multiplier: 1}
		f.emitCustomEvent(treasureTrailsVesselChanceEventName, boost)
		if boost.Allowed != nil && !*boost.Allowed {
			continue
		}
		thresholdMet := false
		if tier.name != "beginner" {
			achievement := &CombatAchievementsTierRequest{Player: player, Tier: tier.name}
			f.emitCustomEvent(combatAchievementsTierEventName, achievement)
			thresholdMet = achievement.CurrentThresholdMet
		}
		denominator := bottleDenominator(base, level, tier.tenths, thresholdMet)
		if denominator < 1 {
			continue
		}
		multiplier := 1.0
		if !math.IsNaN(boost.Multiplier) && !math.IsInf(boost.Multiplier, 0) && boost.Multiplier > 1 {
			multiplier = boost.Multiplier
		}
		chanceMultiplier := multiplier
		if wildernessRing && tier.name != "beginner" {
			chanceMultiplier *= 2
		}
		if rand.Float64()*float64(denominator) >= chanceMultiplier {
			continue
		}
		if f.ownsClueBottle(player, tier.name) {
			f.emitCustomEvent(treasureTrailsMissedClueEventName, &MissedClueEvent{Player: player, Tier: tier.name})
			continue
		}
		reward := &ClueItemRequest{Player: player, ItemID: tier.bottleID}
		f.emitCustomEvent(treasureTrailsClueItemEventName, reward)
		// Box true would forceAdd the bottle into the
		// inventory — unbuildable (no forceAdd in Go) and
		// unanswerable today, so the ground-drop path runs
		// unconditionally; see the file header.
		dropGroundItem(player, game.NewItem(reward.ItemID, 1))
	}
}
