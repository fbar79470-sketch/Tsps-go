// SacredEel.Cooking.js (54 lines, commit bfd81e3): dissect a
// sacred eel with a knife (Wiki: Sacred eel). Cooking level
// controls the 3–9 scale yield in eight-level bands; each eel
// gives 100 base Cooking XP plus 3 XP per scale. There is no
// RSProx action capture in the available recordings, so this
// implements the documented item, level, yield, and XP behavior
// without guessing an action message or animation — dissect
// plays and says NOTHING, and the comment says so.
//
// Worker C file (PHASE36_PLAN.md §3).
package cooking

import (
	"math/rand/v2"

	"tsps-go/internal/game"
	"tsps-go/internal/plugins"
)

// ---- SacredEel constants (SacredEel.Cooking.js:7-15) ----

const (
	// sacredEelMinLevel is MIN_LEVEL (72).
	sacredEelMinLevel = 72
	// sacredEelName is the eel's ItemIds name (ItemIds.
	// SACRED_EEL = 13339): TS reads it through core.ItemIds
	// (SacredEel.Cooking.js:15), NOT a cache gameval — the
	// rev-242 cache has no "sacred_eel" gameval (probe,
	// 2026-11-21). resolveIDs tries the gameval first (so
	// tests can inject fixture ids through the resolver) and
	// falls back to itemIDConstants, which production always
	// takes.
	sacredEelName = "sacred_eel"
	// zulrahsScalesName is the scales' ItemIds name (ItemIds.
	// ZULRAHS_SCALES = 12934), resolved the same way.
	zulrahsScalesName = "zulrahs_scales"
)

// sacredEelUnit is the SacredEel unit's per-registration
// state: the resolved eel/scales ids and the deterministic
// random source tests override.
type sacredEelUnit struct {
	c *cooking
	// resolve answers the eel/scales gamevals; nil resolves
	// nothing, and resolveIDs then takes the ItemIds
	// constants (the production path).
	resolve gamevalResolver
	// eelID / scalesID are the resolved ids (0 when
	// unresolvable); resolved reports whether the lookup ran.
	eelID    int
	scalesID int
	resolved bool
	// randFloat rolls [0, 1) for the scale roll; tests
	// override it for determinism (the fletching randInt
	// precedent).
	randFloat func() float64
	// knifeIDs is the test override for the knives (nil means
	// c.knives(), the loader-resolved KNIFE_NAMES ids).
	knifeIDs []int
}

// newSacredEelUnit builds a SacredEel unit over c, resolving
// the eel/scales ids eagerly (TS resolves them at load via
// ItemIds, which throws on unknown — Go resolves once and
// fails closed when unresolvable).
func newSacredEelUnit(c *cooking, resolve gamevalResolver) *sacredEelUnit {
	u := &sacredEelUnit{c: c, resolve: resolve, randFloat: rand.Float64}
	u.resolveIDs()
	return u
}

// resolveIDs resolves the eel/scales ids once: the gameval
// attempt first (tests inject fixture ids through the
// resolver), then the ItemIds constants — production always
// takes the constants, because the rev-242 cache has no such
// gamevals (probe, 2026-11-21).
func (u *sacredEelUnit) resolveIDs() {
	if u.resolved {
		return
	}
	u.resolved = true
	if u.resolve != nil {
		if id, ok, _ := u.resolve.ObjID(sacredEelName); ok {
			u.eelID = id
		}
		if id, ok, _ := u.resolve.ObjID(zulrahsScalesName); ok {
			u.scalesID = id
		}
	}
	if u.eelID == 0 {
		u.eelID = itemIDConstants["SACRED_EEL"]
	}
	if u.scalesID == 0 {
		u.scalesID = itemIDConstants["ZULRAHS_SCALES"]
	}
}

// knives returns the knife ids this cut accepts: the test
// override, else c.knives().
func (u *sacredEelUnit) knives() []int {
	if u.knifeIDs != nil {
		return u.knifeIDs
	}
	return u.c.knives()
}

// pair is pair (SacredEel.Cooking.js:17-21): knife+eel in
// either order.
func (u *sacredEelUnit) pair(event *plugins.PluginItemOnItemEvent) bool {
	if event == nil || u.eelID == 0 {
		return false
	}
	knives := make(map[int]bool, len(u.knives()))
	for _, id := range u.knives() {
		knives[id] = true
	}
	if event.UsedItemID == u.eelID && knives[event.UsedWithItemID] {
		return true
	}
	return event.UsedWithItemID == u.eelID && knives[event.UsedItemID]
}

// dissect is dissect (SacredEel.Cooking.js:23-46): knife+eel
// in either order; members world only (silent return
// otherwise); handled; level < 72 → "You need a Cooking level
// of 72 to dissect sacred eels."; band = min(4,
// floor((level - 72) / 8)); scales = 3 + band +
// floor(random * 3) (3–9); xp = 100 + 3 * scales; delete 1
// eel, add scales, addXp; emit cooking:sacred-eel-dissected
// and skills:produced (kind "sacred-eel"). NO message or
// animation — none is captured; the unit plays and says
// nothing.
func (u *sacredEelUnit) dissect(event *plugins.PluginItemOnItemEvent) {
	if event == nil || event.Player == nil {
		return
	}
	if !u.pair(event) || !u.c.membersWorld {
		return
	}
	player := event.Player
	event.Handled = true
	level := u.c.cookingLevel(player)
	if level < sacredEelMinLevel {
		player.SendMessage("You need a Cooking level of 72 to dissect sacred eels.")
		return
	}
	roll := u.randFloat
	if roll == nil {
		roll = rand.Float64
	}
	band := (level - sacredEelMinLevel) / 8
	if band > 4 {
		band = 4
	}
	scales := 3 + band + int(roll()*3)
	xp := 100 + 3*scales
	inventory := player.Inventory()
	if inventory == nil || !inventory.Contains(u.eelID) {
		return
	}
	inventory.DeleteNumber(u.eelID, 1)
	inventory.AddItem(game.NewItem(u.scalesID, scales))
	u.c.addXP(player, float64(xp))
	u.c.emitEvent(SacredEelEventName, &SacredEelEvent{
		Player: player,
		ItemID: u.eelID,
		Scales: scales,
	})
	u.c.emitEvent(SkillsProducedEventName, &ProducedEvent{
		Player: player,
		Skill:  game.SkillCooking,
		ItemID: u.scalesID,
		Amount: scales,
		XP:     float64(xp),
		Kind:   "sacred-eel",
	})
}

// registerSacredEel wires the SacredEel unit
// (SacredEel.Cooking.js attach): onItemOnItem (noted:false).
// resolve answers the eel/scales gamevals for tests; nil
// resolves nothing and the unit takes the ItemIds constants
// (the production path).
func (c *cooking) registerSacredEel(api *plugins.Api, resolve gamevalResolver) {
	if api == nil {
		return
	}
	u := newSacredEelUnit(c, resolve)
	api.OnItemOnItem(u.dissect, &plugins.ItemUseFilter{Noted: plugins.BoolPtr(false)})
}
