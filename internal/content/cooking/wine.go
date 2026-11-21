// Wine.Cooking.js (234 lines, commit bfd81e3): grapes (or Zamorak's
// grapes) on a jug of water make unfermented wine, which ferments 20
// ticks after the last jug was made, in the inventory and the bank
// alike, into wine or a jug of bad wine. Levels, XP and success
// chances are the Wiki's (plugins/skills/data/cooking.json); the menu,
// animation, graphic, sound, line and timing are from rsprox captures
// (recordings 91, 1105).
//
// Worker C file (PHASE36_PLAN.md §3). It also holds the one shared
// seam the Worker C units need that Workers A/B did not define: the
// cache gameval resolver (gamevalResolver / newGamevalResolver) for
// the ids cooking.json never names — the wine animation/graphic, the
// sacred-eel ids, the leaping-fish ids, and every Gnome name and
// interface id. Tables.ItemID only answers names the JSON file uses
// (tables.go ItemID), so those units cannot resolve through it.
package cooking

import (
	"math/rand/v2"

	"tsps-go/internal/cache"
	"tsps-go/internal/game"
	"tsps-go/internal/plugins"
)

// ---- cache gameval resolver (Worker C seam) ----

// gamevalResolver resolves cache gameval names the Cooking units
// need outside cooking.json (Common.Cooking.js gameval, which
// throws on unknown — Go call sites check ok instead and fail
// closed). It extends the loader's IDResolver (tables.go) with
// the spotanim, interface and component kinds the Wine and Gnome
// units need. Production passes newGamevalResolver(defs) from
// the cmd/server wiring, over the same *cache.CacheDefinitions
// LoadFromCache used; tests pass a map-backed fake.
type gamevalResolver interface {
	IDResolver
	SpotanimID(name string) (int, bool, error)
	InterfaceID(name string) (int, bool, error)
	ComponentID(name string) (int, bool, error)
}

// cacheGamevalResolver adapts *cache.CacheDefinitions to
// gamevalResolver.
type cacheGamevalResolver struct{ defs *cache.CacheDefinitions }

func (r cacheGamevalResolver) ObjID(name string) (int, bool, error) {
	if r.defs == nil {
		return 0, false, nil
	}
	return r.defs.GetObjId(name)
}

func (r cacheGamevalResolver) SeqID(name string) (int, bool, error) {
	if r.defs == nil {
		return 0, false, nil
	}
	return r.defs.GetSeqId(name)
}

func (r cacheGamevalResolver) VarbitID(name string) (int, bool, error) {
	if r.defs == nil {
		return 0, false, nil
	}
	return r.defs.GetVarbitId(name)
}

func (r cacheGamevalResolver) IsMembersItem(id int) bool {
	if r.defs == nil {
		return false
	}
	if item := r.defs.GetItem(id); item != nil {
		return item.IsMembers
	}
	return false
}

func (r cacheGamevalResolver) SpotanimID(name string) (int, bool, error) {
	if r.defs == nil {
		return 0, false, nil
	}
	return r.defs.GetSpotanimId(name)
}

func (r cacheGamevalResolver) InterfaceID(name string) (int, bool, error) {
	if r.defs == nil {
		return 0, false, nil
	}
	return r.defs.GetInterfaceId(name)
}

func (r cacheGamevalResolver) ComponentID(name string) (int, bool, error) {
	if r.defs == nil {
		return 0, false, nil
	}
	return r.defs.GetComponentId(name)
}

// newGamevalResolver builds the production gamevalResolver over
// defs (nil defs resolve nothing — every unit fails closed).
func newGamevalResolver(defs *cache.CacheDefinitions) gamevalResolver {
	return cacheGamevalResolver{defs: defs}
}

// ---- Wine constants (Wine.Cooking.js:1-24) ----

const (
	// makeWineAnimationName is MAKE_WINE_ANIMATION
	// ("human_make_wine").
	makeWineAnimationName = "human_make_wine"
	// makeWineSpotanimName is MAKE_WINE_SPOTANIM
	// ("cooking_make_wine_spotanim").
	makeWineSpotanimName = "cooking_make_wine_spotanim"
	// makeWineSound is MAKE_WINE_SOUND (2489), with every jug,
	// 2 ticks late (rsprox captures).
	makeWineSound      = 2489
	makeWineSoundDelay = 2
	// fermentMessage is FERMENT_MESSAGE: once, as the last jug
	// of a batch is made (rsprox captures).
	fermentMessage = "You squeeze the grapes into the jug. The wine begins to ferment."
	// fermentTicks is FERMENT_TICKS (20): Wiki — one timer for
	// all of a player's wine, back to 20 ticks with every jug
	// made.
	fermentTicks = 20
	// fermentBatches is FERMENT_BATCHES (15): Wiki (Jug of
	// wine, Mod Ash) — a stack is rolled as this many batches,
	// plus a roll for each jug left over.
	fermentBatches = 15
	// wineWaterName is WATER ("jug_water").
	wineWaterName = "jug_water"
)

// wineTiming is WINE_TIMING ([2, 2]): a jug every 2 ticks from
// the choice (rsprox captures).
var wineTiming = []int{2, 2}

// wineUnit is the Wine unit's per-registration state: the
// fermenting map (player → ticks left) and its one global
// 1-tick task, the resolved animation/graphic ids, and the
// deterministic random source tests override. One instance
// serves the registered Plugin value (the Phase 21 race fix:
// no package-level mutable maps).
type wineUnit struct {
	c *cooking
	// resolve answers the animation/graphic gamevals; nil
	// resolves nothing (playMakeWine then plays no anim or
	// graphic — fail-closed).
	resolve gamevalResolver
	// fermenting maps a player to the ticks left on their
	// ferment timer (Wine.Cooking.js fermenting).
	fermenting map[*game.Player]int
	// taskScheduled reports whether the global 1-tick ferment
	// task is already scheduled (the woodcutting ensureTask
	// pattern: schedule once, lazily, on the lazily-resolved
	// TaskManager).
	taskScheduled bool
	// animID / spotanimID are the resolved
	// human_make_wine / cooking_make_wine_spotanim ids (0 when
	// unresolvable).
	animID     int
	spotanimID int
	// randFloat rolls [0, 1) for the ferment rolls; tests
	// override it for determinism (the fletching randInt
	// precedent).
	randFloat func() float64
}

// newWineUnit builds a Wine unit over c, resolving the
// animation and graphic ids eagerly (TS resolves them on every
// jug via Common.gameval, which throws on unknown — Go
// resolves once and plays nothing when unresolvable).
func newWineUnit(c *cooking, resolve gamevalResolver) *wineUnit {
	w := &wineUnit{
		c:          c,
		resolve:    resolve,
		fermenting: make(map[*game.Player]int),
		randFloat:  rand.Float64,
	}
	if resolve != nil {
		if id, ok, _ := resolve.SeqID(makeWineAnimationName); ok {
			w.animID = id
		}
		if id, ok, _ := resolve.SpotanimID(makeWineSpotanimName); ok {
			w.spotanimID = id
		}
	}
	return w
}

// wines returns the loaded wine rows, or nil without tables.
func (w *wineUnit) wines() []*Wine {
	if w == nil || w.c == nil || w.c.tables == nil {
		return nil
	}
	return w.c.tables.Wines
}

// wineFor is wineFor (Wine.Cooking.js:27-33): one side must be
// jug water, the other a grapes id from wineData().
func (w *wineUnit) wineFor(firstID, secondID int) *Wine {
	if w == nil || w.c == nil {
		return nil
	}
	waterID, ok := w.c.itemID(wineWaterName)
	if !ok {
		return nil
	}
	var otherID int
	switch {
	case firstID == waterID:
		otherID = secondID
	case secondID == waterID:
		otherID = firstID
	default:
		return nil
	}
	for _, wine := range w.wines() {
		if wine != nil && wine.GrapesID == otherID {
			return wine
		}
	}
	return nil
}

// wineRecipe is wineRecipe (Wine.Cooking.js:35-43): product =
// unfermented, xp 0, ingredients grapes 1 + jug water 1, no
// returns, timing [2, 2]. Members follows the cache
// isMembers flag of the unfermented jug (Recipes.Cooking.js
// isMembersRecipe, via the resolver — the loader's
// IsMembersItem, which A resolves for JSON products at load;
// a wine recipe is built at use time, so it asks here).
func (w *wineUnit) wineRecipe(wine *Wine) *Recipe {
	waterID, _ := w.c.itemID(wineWaterName)
	recipe := &Recipe{
		ProductID: wine.UnfermentedID,
		Level:     wine.Level,
		XP:        0,
		Ingredients: []Ingredient{
			{ID: wine.GrapesID, Count: 1},
			{ID: waterID, Count: 1},
		},
		Timing: append([]int(nil), wineTiming...),
	}
	if w.resolve != nil {
		recipe.Members = w.resolve.IsMembersItem(wine.UnfermentedID)
	}
	return recipe
}

// playMakeWine is playMakeWine (Wine.Cooking.js:45-50): anim +
// graphic + sound (delay 2). The graphic is
// Graphic(spotanim) with the TS default height (LOW, 0) and no
// delay. An unresolvable anim/graphic id plays nothing (the Go
// fail-closed form of TS's throw — the ids are load-proven
// against the real cache instead).
func (w *wineUnit) playMakeWine(player *game.Player) {
	if player == nil {
		return
	}
	if w.animID > 0 {
		player.PerformAnimation(w.animID, 0)
	}
	if w.spotanimID > 0 {
		player.PerformGraphic(w.spotanimID, 0, 0)
	}
	game.SendSoundEffect(player, makeWineSound, 1, makeWineSoundDelay, 1)
}

// makeWine is makeWine (Wine.Cooking.js:52-64): Recipes.offer
// with the cycle/canMake/made/done hooks — cycle plays the
// animation, canMake enforces the pending limit, made restarts
// the ferment timer, done spams FERMENT_MESSAGE once.
func (w *wineUnit) makeWine(event *plugins.PluginItemOnItemEvent) {
	if event == nil || event.Player == nil {
		return
	}
	wine := w.wineFor(event.UsedItemID, event.UsedWithItemID)
	if wine == nil {
		return
	}
	player := event.Player
	event.Handled = true
	w.c.offer(player, w.wineRecipe(wine), RecipeHooks{
		Cycle: func(remaining int) { w.playMakeWine(player) },
		CanMake: func() bool {
			return !wine.HasPendingLimit || w.pendingWine(player, wine) < wine.PendingLimit
		},
		Made: func() { w.queueFerment(player) },
		Done: func() { w.c.spam(player, fermentMessage) },
	})
}

// wineSuccessChance is successChance (Wine.Cooking.js:67-72):
// the level capped at 99, the Common interp formula on
// wine.chance.
func (w *wineUnit) wineSuccessChance(player *game.Player, wine *Wine) float64 {
	level := w.c.cookingLevel(player)
	if level > 99 {
		level = 99
	}
	return successChance(wine.Chance[0], wine.Chance[1], level)
}

// pendingWine is pendingWine (Wine.Cooking.js:75-78): inventory
// amount + all bank tabs (skip BankSearchTabIndex,
// game.BankSearchTabIndex = 10; tabs via Player.Bank(tab)).
func (w *wineUnit) pendingWine(player *game.Player, wine *Wine) int {
	if player == nil || wine == nil {
		return 0
	}
	total := 0
	if inv := player.Inventory(); inv != nil {
		total += inv.GetAmount(wine.UnfermentedID)
	}
	for _, tab := range w.bankTabs(player) {
		total += tab.GetAmount(wine.UnfermentedID)
	}
	return total
}

// bankTabs returns the player's real bank tabs (Wine.Cooking.js
// bankTabs: player.getBanks() filtered to non-nil tabs other
// than Bank.BANK_SEARCH_TAB_INDEX).
func (w *wineUnit) bankTabs(player *game.Player) []*game.Bank {
	if player == nil {
		return nil
	}
	var tabs []*game.Bank
	for tab := 0; tab < player.BankCount(); tab++ {
		if tab == game.BankSearchTabIndex {
			continue
		}
		if bank := player.Bank(tab); bank != nil {
			tabs = append(tabs, bank)
		}
	}
	return tabs
}

// successes is successes (Wine.Cooking.js:81-87): how many of
// count jugs ferment into wine — FERMENT_BATCHES rolls for
// equal batches, and one for each jug left over.
func (w *wineUnit) successes(count int, chance float64) int {
	if count <= 0 {
		return 0
	}
	roll := w.randFloat
	if roll == nil {
		roll = rand.Float64
	}
	batch := count / fermentBatches
	good := 0
	if batch > 0 {
		for i := 0; i < fermentBatches; i++ {
			if roll() < chance {
				good += batch
			}
		}
	}
	for i := 0; i < count-batch*fermentBatches; i++ {
		if roll() < chance {
			good++
		}
	}
	return good
}

// bankedStack is one banked stack of an item: its tab and slot
// (Wine.Cooking.js bankedStacks).
type bankedStack struct {
	tab  *game.Bank
	slot int
}

// bankedStacks returns every banked stack of the item, as
// { tab, slot } (Wine.Cooking.js bankedStacks).
func bankedStacks(tabs []*game.Bank, itemID int) []bankedStack {
	var stacks []bankedStack
	for _, tab := range tabs {
		if tab == nil {
			continue
		}
		for slot, item := range tab.Items() {
			if item.ID() == itemID && item.Amount() > 0 {
				stacks = append(stacks, bankedStack{tab: tab, slot: slot})
			}
		}
	}
	return stacks
}

// bankFreeSlots is Bank.freeSlots(player): the free slots
// across the real bank tabs. Go has no Bank.freeSlots static,
// so it sums GetFreeSlots over the same tabs bankTabs returns.
func bankFreeSlots(tabs []*game.Bank) int {
	total := 0
	for _, tab := range tabs {
		if tab != nil {
			total += tab.GetFreeSlots()
		}
	}
	return total
}

// bankFits is bankFits (Wine.Cooking.js:101-112): whether the
// banked jugs' wine and bad wine fit — each output joins a
// stack the bank holds, or takes the unfermented stack's slot
// or a free one. An existing output stack still needs room for
// the entire banked result.
func (w *wineUnit) bankFits(tabs []*game.Bank, wine *Wine, good, bad int) bool {
	holds := func(id int) bool { return len(bankedStacks(tabs, id)) > 0 }
	newStacks := 0
	if good > 0 && !holds(wine.WineID) {
		newStacks++
	}
	if bad > 0 && !holds(wine.BadID) {
		newStacks++
	}
	for _, out := range []struct {
		id, amount int
	}{{wine.WineID, good}, {wine.BadID, bad}} {
		if out.amount <= 0 {
			continue
		}
		if held := bankedStacks(tabs, out.id); len(held) > 0 {
			if held[0].tab.GetAddableAmount(game.NewItem(out.id, out.amount)) < out.amount {
				return false
			}
		}
	}
	return newStacks <= len(bankedStacks(tabs, wine.UnfermentedID))+bankFreeSlots(tabs)
}

// fermentBanked is fermentBanked (Wine.Cooking.js:119-144): the
// banked jugs become good wine and bad bad wine. A new output
// stack takes the unfermented stack's own slot first, so the
// conversion never needs more room than bankFits allowed.
// DEVIATION (documented): TS sends a leftover fresh output to
// player.getBank(Bank.getTabForItem(player, itemId)); Go has
// no GetTabForItem, so a leftover goes to the first tab (in
// tab order) with a free slot — bankFits guarantees one
// exists.
func (w *wineUnit) fermentBanked(tabs []*game.Bank, wine *Wine, good, bad int) {
	type pending struct {
		held *bankedStack
		item game.Item
	}
	var joining, fresh []pending
	for _, out := range []struct {
		id, amount int
	}{{wine.WineID, good}, {wine.BadID, bad}} {
		if out.amount <= 0 {
			continue
		}
		if held := bankedStacks(tabs, out.id); len(held) > 0 {
			h := held[0]
			joining = append(joining, pending{held: &h, item: game.NewItem(out.id, out.amount)})
		} else {
			fresh = append(fresh, pending{item: game.NewItem(out.id, out.amount)})
		}
	}
	touched := make(map[*game.Bank]bool)
	var touchedOrder []*game.Bank
	touch := func(tab *game.Bank) {
		if tab != nil && !touched[tab] {
			touched[tab] = true
			touchedOrder = append(touchedOrder, tab)
		}
	}
	for _, stack := range bankedStacks(tabs, wine.UnfermentedID) {
		var item game.Item
		if len(fresh) > 0 {
			item = fresh[0].item
			fresh = fresh[1:]
		} else {
			item = game.NewItem(-1, 0)
		}
		stack.tab.SetItem(stack.slot, item)
		touch(stack.tab)
	}
	for _, p := range joining {
		p.held.tab.Add(p.item, false)
		touch(p.held.tab)
	}
	for _, p := range fresh {
		// No GetTabForItem in Go: first tab with a free slot.
		for _, tab := range tabs {
			if tab != nil && tab.GetFreeSlots() > 0 {
				tab.Add(p.item, false)
				touch(tab)
				break
			}
		}
	}
	for _, tab := range touchedOrder {
		tab.SortItems()
	}
	// One refresh sends the whole bank.
	if len(touchedOrder) > 0 {
		touchedOrder[0].RefreshItems()
	}
}

// fermentWine is fermentWine (Wine.Cooking.js:151-179): every
// unfermented jug of this wine, carried and banked, turns into
// wine or bad wine at once, with all its XP. A bank without
// room for the outcome keeps its jugs unfermented, for the next
// timer, rather than lose any. XP is good * wine.xp (only when
// good > 0); skills:produced kind "ferment".
//
// NOTE on the carried loop: TS replaces each unfermented STACK
// with a single wine/bad-wine item (setItem(slot, new Item(id,
// 1))) — faithful here, and correct because unfermented wine
// is unstackable (each jug its own slot, amount 1). If the
// cache ever marked it stackable, a multi-jug carried stack
// would collapse to one jug; the bank path already converts
// per jug.
func (w *wineUnit) fermentWine(player *game.Player, wine *Wine) {
	if player == nil || wine == nil {
		return
	}
	inventory := player.Inventory()
	if inventory == nil {
		return
	}
	tabs := w.bankTabs(player)
	carried := inventory.GetAmount(wine.UnfermentedID)
	banked := 0
	for _, tab := range tabs {
		banked += tab.GetAmount(wine.UnfermentedID)
	}
	if carried+banked <= 0 {
		return
	}
	chance := w.wineSuccessChance(player, wine)
	good := w.successes(carried+banked, chance)
	bankedGood := func() int { return good - min(good, carried) }
	if banked > 0 && !w.bankFits(tabs, wine, bankedGood(), banked-bankedGood()) {
		banked = 0
		good = w.successes(carried, chance)
	}

	carriedGood := min(good, carried)
	for slot, item := range inventory.Items() {
		if item.ID() != wine.UnfermentedID {
			continue
		}
		id := wine.BadID
		if carriedGood > 0 {
			id = wine.WineID
		}
		inventory.SetItem(slot, game.NewItem(id, 1))
		carriedGood--
	}
	if carried > 0 {
		inventory.RefreshItems()
	}

	if banked > 0 {
		w.fermentBanked(tabs, wine, bankedGood(), banked-bankedGood())
	}

	if good <= 0 {
		return
	}
	xp := float64(good) * wine.XP
	w.c.addXP(player, xp)
	w.c.emitEvent(SkillsProducedEventName, &ProducedEvent{
		Player: player,
		Skill:  game.SkillCooking,
		ItemID: wine.WineID,
		Amount: good,
		XP:     xp,
		Kind:   "ferment",
	})
}

// ferment ferments every wine for the player
// (Wine.Cooking.js:181-183).
func (w *wineUnit) ferment(player *game.Player) {
	for _, wine := range w.wines() {
		w.fermentWine(player, wine)
	}
}

// queueFerment is queueFerment (Wine.Cooking.js:186-195):
// starts the timer again from FERMENT_TICKS (Wiki: every jug
// made resets it), with one global 1-tick task.
func (w *wineUnit) queueFerment(player *game.Player) {
	if player == nil {
		return
	}
	w.fermenting[player] = fermentTicks
	if w.taskScheduled {
		return
	}
	tm := w.c.taskManager()
	if tm == nil {
		return
	}
	task := game.NewTask(1, func(cycle int64) bool {
		w.fermentTick()
		return false
	})
	task.Name = "WineFermentTask"
	tm.Schedule(task)
	w.taskScheduled = true
}

// fermentTick is fermentTick (Wine.Cooking.js:197-207): each
// tick decrements every timer; unregistered players are
// dropped; a timer at 0 ferments.
func (w *wineUnit) fermentTick() {
	for player, ticks := range w.fermenting {
		if !player.IsRegistered() {
			delete(w.fermenting, player)
		} else if ticks <= 1 {
			delete(w.fermenting, player)
			w.ferment(player)
		} else {
			w.fermenting[player] = ticks - 1
		}
	}
}

// resumeFermenting is resumeFermenting
// (Wine.Cooking.js:210-214): wine still unfermented from an
// earlier session ferments the usual 20 ticks after login.
func (w *wineUnit) resumeFermenting(event *plugins.PlayerLoginEvent) {
	if event == nil || event.Player == nil {
		return
	}
	player := event.Player
	holds := func(id int) bool {
		if inv := player.Inventory(); inv != nil && inv.Contains(id) {
			return true
		}
		for _, tab := range w.bankTabs(player) {
			if tab.GetAmount(id) > 0 {
				return true
			}
		}
		return false
	}
	for _, wine := range w.wines() {
		if holds(wine.UnfermentedID) {
			w.queueFerment(player)
			return
		}
	}
}

// forget drops a disconnecting player's ferment timer
// (Wine.Cooking.js:216-218).
func (w *wineUnit) forget(event *plugins.PlayerDisconnectEvent) {
	if event == nil || event.Player == nil {
		return
	}
	delete(w.fermenting, event.Player)
}

// registerWine wires the Wine unit (Wine.Cooking.js
// attachWine): onItemOnItem (noted:false), onPlayerLogin,
// onPlayerDisconnect. resolve answers the animation/graphic
// gamevals; nil resolves nothing (playMakeWine then plays no
// anim or graphic — the load-prove verifies the names
// resolve against the real cache instead).
func (c *cooking) registerWine(api *plugins.Api, resolve gamevalResolver) {
	if api == nil {
		return
	}
	w := newWineUnit(c, resolve)
	api.OnItemOnItem(w.makeWine, &plugins.ItemUseFilter{Noted: plugins.BoolPtr(false)})
	api.OnPlayerLogin(w.resumeFermenting)
	api.OnPlayerDisconnect(w.forget)
}
