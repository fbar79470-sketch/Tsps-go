// LeapingFish.Cooking.js (102 lines, commit bfd81e3): cutting
// leaping fish with a knife, the only source of roe and caviar
// (Wiki, quoting Mod Ash: Leaping trout, Leaping salmon,
// Leaping sturgeon, Roe, Caviar and Fish offcuts):
//   - every cut uses up one fish. A successful cut gives roe
//     (caviar for sturgeon), static Cooking XP and, on a
//     further roll, fish offcuts; a failed cut gives nothing;
//   - trout: roe 1/150 at Cooking 1, rising linearly to 99/150
//     at 99, offcuts 1/2, 10 XP; salmon: roe 1/80 at Cooking 1,
//     rising linearly to certain at 80, offcuts 3/4, 10 XP;
//     sturgeon: caviar at the salmon's rates, offcuts 5/6, 15 XP;
//   - the first cut happens on the knife use and the rest follow
//     automatically every 3 ticks (changelog 16 November 2022).
//     Using the knife again cuts at once, so re-using it is
//     faster, but only one fish is cut per tick.
//
// The rsprox recordings have leaping fish caught and dropped,
// never cut, and the Wiki gives no message or animation, so a
// cut plays and says nothing. Offcuts with no room fall to the
// floor (unconfirmed).
//
// Worker C file (PHASE36_PLAN.md §3).
package cooking

import (
	"math"
	"math/rand/v2"

	"tsps-go/internal/game"
	"tsps-go/internal/plugins"
)

// ---- LeapingFish constants (LeapingFish.Cooking.js:11-25) ----

const (
	// lastCutTickAttribute is LAST_CUT_TICK_ATTRIBUTE: the
	// game tick of the player's last cut, so repeated knife
	// uses cut one fish per tick.
	lastCutTickAttribute = "cooking:leaping-fish-cut-tick"
	// autoCutTicks is AUTO_CUT_TICKS (3): ticks between
	// automatic cuts (Wiki footnote on Roe and Fish offcuts).
	autoCutTicks = 3
	// fishOffcutsName is the offcuts' gameval name
	// ("brut_fish_cuts").
	fishOffcutsName = "brut_fish_cuts"
)

// leapingFishRow is one FISH row (LeapingFish.Cooking.js:16-20),
// by leaping fish gameval name: what a successful cut gives,
// the roe chance's denominator and cap, offcut odds and XP.
// FishID / ProductID are resolved through the gameval resolver
// (cooking.json never names them).
type leapingFishRow struct {
	FishName  string
	Product   string
	Per       int
	Offcuts   [2]int
	XP        float64
	FishID    int
	ProductID int
}

// leapingFishTable is FISH, in TS data order.
var leapingFishTable = []leapingFishRow{
	{FishName: "brut_spawning_trout", Product: "brut_roe", Per: 150, Offcuts: [2]int{1, 2}, XP: 10},
	{FishName: "brut_spawning_salmon", Product: "brut_roe", Per: 80, Offcuts: [2]int{3, 4}, XP: 10},
	{FishName: "brut_sturgeon", Product: "brut_caviar", Per: 80, Offcuts: [2]int{5, 6}, XP: 15},
}

// leapingFishUnit is the LeapingFish unit's per-registration
// state: the resolved fish table and offcuts id, and the
// deterministic random source / cycle source tests override.
type leapingFishUnit struct {
	c *cooking
	// resolve answers the fish/product/offcuts gamevals; nil
	// resolves nothing (cut then never matches — fail-closed).
	resolve gamevalResolver
	// fishByID is the resolved fish table (fishTable); built
	// lazily on first use. A row whose names do not resolve
	// is skipped (fail-closed per row).
	fishByID map[int]*leapingFishRow
	// offcutsID is the resolved brut_fish_cuts id (0 when
	// unresolvable — the offcut roll then drops nothing).
	offcutsID int
	// randFloat rolls [0, 1) for the roe/offcut rolls; tests
	// override it for determinism (the fletching randInt
	// precedent).
	randFloat func() float64
	// knifeIDs is the test override for the knives (nil means
	// c.knives(), the loader-resolved KNIFE_NAMES ids).
	knifeIDs []int
	// cycle is the test override for the current process cycle
	// (nil means Api.Core().Cycle() — see currentCycle).
	cycle func() int64
}

// newLeapingFishUnit builds a LeapingFish unit over c.
func newLeapingFishUnit(c *cooking, resolve gamevalResolver) *leapingFishUnit {
	return &leapingFishUnit{c: c, resolve: resolve, randFloat: rand.Float64}
}

// resolveFish builds the fish table once (LeapingFish.Cooking.js
// fishTable).
func (u *leapingFishUnit) resolveFish() {
	if u.fishByID != nil {
		return
	}
	u.fishByID = make(map[int]*leapingFishRow)
	if u.resolve == nil {
		return
	}
	for i := range leapingFishTable {
		row := leapingFishTable[i]
		fishID, ok1, _ := u.resolve.ObjID(row.FishName)
		productID, ok2, _ := u.resolve.ObjID(row.Product)
		if !ok1 || !ok2 {
			continue
		}
		row.FishID = fishID
		row.ProductID = productID
		r := row
		u.fishByID[fishID] = &r
	}
	if id, ok, _ := u.resolve.ObjID(fishOffcutsName); ok {
		u.offcutsID = id
	}
}

// knives returns the knife ids this cut accepts: the test
// override, else c.knives().
func (u *leapingFishUnit) knives() []int {
	if u.knifeIDs != nil {
		return u.knifeIDs
	}
	return u.c.knives()
}

// currentCycle is the current process cycle (TS
// World.getProcessCycle()): Api.Core().Cycle(), installed by
// the cmd/server adapter from world.ProcessCycle
// (pluginwiring.go: hub.SetCycleSource(world.ProcessCycle)) —
// nil-safe, 0 with no source. Woodcutting keeps its own tick
// counter in its 1-tick task; there is no content-reachable
// Engine.TickCount, so the hub cycle source is the grounded
// Go form of the TS call. Tests override cycle directly.
func (u *leapingFishUnit) currentCycle() int64 {
	if u.cycle != nil {
		return u.cycle()
	}
	if u.c == nil || u.c.api == nil {
		return 0
	}
	return u.c.api.Core().Cycle()
}

// fishFor is fishFor (LeapingFish.Cooking.js:36-44): the
// leaping fish this knife use is on, or nil.
func (u *leapingFishUnit) fishFor(event *plugins.PluginItemOnItemEvent) *leapingFishRow {
	if event == nil {
		return nil
	}
	u.resolveFish()
	knives := make(map[int]bool, len(u.knives()))
	for _, id := range u.knives() {
		knives[id] = true
	}
	if knives[event.UsedItemID] {
		if row, ok := u.fishByID[event.UsedWithItemID]; ok {
			return row
		}
	}
	if knives[event.UsedWithItemID] {
		if row, ok := u.fishByID[event.UsedItemID]; ok {
			return row
		}
	}
	return nil
}

// roeChance is roeChance (LeapingFish.Cooking.js:47-49): the
// chance a cut gives roe or caviar at this Cooking level —
// level/150 for trout, level/80 (capped) otherwise.
func roeChance(fish *leapingFishRow, level int) float64 {
	return math.Min(1, math.Max(1, float64(level))/float64(fish.Per))
}

// hasKnife is hasKnife (LeapingFish.Cooking.js:52-54):
// whether a knife is still carried — dropping it does NOT stop
// automatic cutting; the check is at cut time.
func (u *leapingFishUnit) hasKnife(inventory *game.Inventory) bool {
	if inventory == nil {
		return false
	}
	for _, id := range u.knives() {
		if inventory.Contains(id) {
			return true
		}
	}
	return false
}

// lastCutTick reads the player's last-cut-tick attribute (0
// when absent or of another type).
func lastCutTick(player *game.Player) int64 {
	if player == nil {
		return 0
	}
	switch v := player.GetAttribute(lastCutTickAttribute).(type) {
	case int64:
		return v
	case int:
		return int64(v)
	case float64:
		return int64(v)
	default:
		return 0
	}
}

// dropGroundItem drops an item at the player's tile through
// the engine ground-item seam (game.Hooks.DropGroundItem, the
// Woodcutting precedent): a nil hook drops the item silently
// (the seam's documented gap).
func (u *leapingFishUnit) dropGroundItem(player *game.Player, item game.Item) {
	if player == nil || game.Hooks.DropGroundItem == nil {
		return
	}
	game.Hooks.DropGroundItem(item, player.Location(), player)
}

// cutOne is cutOne (LeapingFish.Cooking.js:57-73): cuts one
// fish — the fish goes, and on a success the roe or caviar,
// any offcuts and the XP come. It needs the fish + a knife
// still carried; sets the last-cut-tick attribute to the
// current process cycle; delete 1 fish; roe roll misses →
// return true (fish consumed, nothing given); roe/caviar
// added; offcuts roll → forceAdd brut_fish_cuts.
// DEVIATION (documented): Go has no forceAdd (Phase 35 gap).
// The offcuts try inventory Add (GetAddableAmount first) and,
// with no room, drop to the ground via the DropGroundItem seam
// — the Wiki outcome for offcuts with no room (unconfirmed in
// TS, which force-adds). addXp; skills:produced (kind
// "leaping-fish").
func (u *leapingFishUnit) cutOne(player *game.Player, fish *leapingFishRow) bool {
	if player == nil || fish == nil {
		return false
	}
	inventory := player.Inventory()
	if inventory == nil {
		return false
	}
	if !inventory.Contains(fish.FishID) || !u.hasKnife(inventory) {
		return false
	}
	player.SetAttribute(lastCutTickAttribute, u.currentCycle())
	inventory.DeleteNumber(fish.FishID, 1)
	roll := u.randFloat
	if roll == nil {
		roll = rand.Float64
	}
	if roll() >= roeChance(fish, u.c.cookingLevel(player)) {
		return true
	}
	inventory.AddItem(game.NewItem(fish.ProductID, 1))
	if roll() < float64(fish.Offcuts[0])/float64(fish.Offcuts[1]) && u.offcutsID > 0 {
		offcuts := game.NewItem(u.offcutsID, 1)
		if inventory.GetAddableAmount(offcuts) >= 1 {
			inventory.AddItem(offcuts)
		} else {
			u.dropGroundItem(player, offcuts)
		}
	}
	u.c.addXP(player, fish.XP)
	u.c.emitEvent(SkillsProducedEventName, &ProducedEvent{
		Player: player,
		Skill:  game.SkillCooking,
		ItemID: fish.ProductID,
		Amount: 1,
		XP:     fish.XP,
		Kind:   "leaping-fish",
	})
	return true
}

// autoCut is autoCut (LeapingFish.Cooking.js:76-82): cuts the
// same kind of fish every 3 ticks until none are left; a walk
// or a click stops it (via Common.run's keyed task).
func (u *leapingFishUnit) autoCut(player *game.Player, fish *leapingFishRow) {
	wait := autoCutTicks
	u.c.run(player, func(tick int64) bool {
		wait--
		if wait > 0 {
			return true
		}
		wait = autoCutTicks
		inventory := player.Inventory()
		return u.cutOne(player, fish) && inventory != nil && inventory.Contains(fish.FishID)
	})
}

// cut is cut (LeapingFish.Cooking.js:84-91): knife+fish either
// order; members world only (silent return); handled;
// same-tick guard (attribute == current cycle → return);
// TaskManager.cancelTasks(player index); cutOne at once, then
// autoCut while fish remain. Only one fish is cut per tick
// even when the knife is re-used.
func (u *leapingFishUnit) cut(event *plugins.PluginItemOnItemEvent) {
	if event == nil || event.Player == nil {
		return
	}
	fish := u.fishFor(event)
	if fish == nil || !u.c.membersWorld {
		return
	}
	player := event.Player
	event.Handled = true
	if lastCutTick(player) == u.currentCycle() {
		return
	}
	if tm := u.c.taskManager(); tm != nil {
		tm.CancelTasks(player.Index)
	}
	inventory := player.Inventory()
	if u.cutOne(player, fish) && inventory != nil && inventory.Contains(fish.FishID) {
		u.autoCut(player, fish)
	}
}

// registerLeapingFish wires the LeapingFish unit
// (LeapingFish.Cooking.js attachLeapingFish): onItemOnItem
// (noted:false). resolve answers the fish/product/offcuts
// gamevals; nil resolves nothing (cut then never matches —
// the load-prove verifies the names resolve against the real
// cache instead).
func (c *cooking) registerLeapingFish(api *plugins.Api, resolve gamevalResolver) {
	if api == nil {
		return
	}
	u := newLeapingFishUnit(c, resolve)
	api.OnItemOnItem(u.cut, &plugins.ItemUseFilter{Noted: plugins.BoolPtr(false)})
}
