// Gnome.Cooking.js (267 lines, commit bfd81e3): gnome cuisine —
// a synchronous custom interface (aluft_cooking), NOT a
// CreationMenu. Cuisine preparation is available independently
// of tutorial progress. Only the recipes listed below are
// supported by this unit.
//
// Worker C file (PHASE36_PLAN.md §3).
//
// INTERFACE SURFACE (verified against internal/session +
// internal/game before writing — plan §2 "check first"):
//   - Player interface-id get/set: game.Player.GetInterfaceId /
//     SetInterfaceId (internal/game/interface_state.go).
//   - PacketSender sub-interface mount/get/close: the session
//     implements MountSubInterface / GetSubInterfaceAt /
//     CloseSubInterfaceMount (internal/session/interface_sender.go)
//     and SendVarbit (runstatus.go), SendInterfaceFlagsRange
//     (dialogue_sender.go), SendSound (sound_sender.go).
//     Content reaches them through small-interface assertions
//     on player.Session (the fletching varbitSender /
//     CreationMenuSender precedent) — the gnomePacketSender
//     interface below. No new seam was needed.
//   - One adaptation: TS getSubInterfaceAt returns the mount or
//     null; Go GetSubInterfaceAt returns (*SubInterfaceMount,
//     bool). Comparisons below are pointer comparisons against
//     the mount (nil when absent), which is the same check.
//   - Player.IsPlayerBot exists (game.Player.IsPlayerBot).
//   - The event carries no Item reference (Go
//     PluginItemActionEvent has ItemID + Slot only), so openMenu
//     / pour / garnishDragon / emptyBurnt read the item from
//     the inventory slot at handler time — the same contents TS
//     captured, for a slot that has not changed.
//   - Interface ids/varbits resolve by cache gameval through
//     the gamevalResolver (ui()), because Tables only answers
//     names cooking.json uses. A unit whose ui() cannot resolve
//     fails closed (openMenu/interfaceAction no-op); the
//     load-prove verifies every name against the real cache.
package cooking

import (
	"tsps-go/internal/game"
	"tsps-go/internal/plugins"
	"tsps-go/internal/session"
)

// ---- Gnome constants (Gnome.Cooking.js:8-13) ----

const (
	// gnomeMenuName is MENU ("aluft_cooking").
	gnomeMenuName = "aluft_cooking"
	// gnomeMenuTypeVarbit is MENU_TYPE ("aluft_cooking_type").
	gnomeMenuTypeVarbit = "aluft_cooking_type"
	// gnomeChosenVarbit is CHOSEN ("gnome_chosen_cuisine").
	gnomeChosenVarbit = "gnome_chosen_cuisine"
	// gnomeOp1 is OP1 (1 << 1): the clickzone's access mask.
	gnomeOp1 = 1 << 1
	// mixedDragonShaker / mixedDragonPoured /
	// mixedDragonGarnished are MIXED_DRAGON_SHAKER (9574),
	// POURED (9575), GARNISHED (9576).
	mixedDragonShaker    = 9574
	mixedDragonPoured    = 9575
	mixedDragonGarnished = 9576
	// gnomeSpiceName is the gnome spice gameval name.
	gnomeSpiceName = "gnome_spice"
	// gnomeFruitPrompt is the fruit-cutting prompt title.
	gnomeFruitPrompt = "How would you like to cut the fruit?"
)

// gnomeNamePair is one recipe ingredient: a gameval obj name
// and a count (Gnome.Cooking.js mix/finish/ingredients lists).
type gnomeNamePair struct {
	Name  string
	Count int
}

// gnomeMenuEntry is one supported cuisine row, in the unified
// form createCuisine needs: the selector the interface sends,
// the level, whether gnome spice is required, the created item
// (cocktails: the mixed id; batta/crunchy: the result id), the
// cocktail product's gameval name (cocktails only), the
// ingredients, and the cocktail finish (pour) data.
type gnomeMenuEntry struct {
	Selector    int
	Level       int
	Spice       bool
	ProductID   int
	Product     string
	Ingredients []gnomeNamePair
	Finish      []gnomeNamePair
	FinishXP    float64
	HasFinishXP bool
	Poured      int
}

// gnomeMenus is MENUS (Gnome.Cooking.js:31-49): [COCKTAILS +
// [null, null], BATTA, CRUNCHY], in slot order (nil = an
// unsupported row: selecting it clears server state).
var gnomeMenus = [3][]*gnomeMenuEntry{
	{
		{Selector: 0, Level: 6, ProductID: 9568, Product: "fruit_blast", Ingredients: []gnomeNamePair{{"pineapple", 1}, {"lemon", 1}, {"orange", 1}}, Finish: []gnomeNamePair{{"lemon_slices", 1}}, FinishXP: 20, HasFinishXP: true},
		{Selector: 1, Level: 8, ProductID: 9569, Product: "pineapple_punch", Ingredients: []gnomeNamePair{{"pineapple", 2}, {"lemon", 1}, {"orange", 1}}, Finish: []gnomeNamePair{{"lime_chunks", 1}, {"pineapple_chunks", 1}, {"orange_slices", 1}}, FinishXP: 40, HasFinishXP: true},
		{Selector: 2, Level: 18, ProductID: 9566, Product: "wizard_blizzard", Ingredients: []gnomeNamePair{{"vodka", 2}, {"gin", 1}, {"lime", 1}, {"lemon", 1}, {"orange", 1}}, Finish: []gnomeNamePair{{"pineapple_chunks", 1}, {"lime_slices", 1}}, FinishXP: 80, HasFinishXP: true},
		{Selector: 3, Level: 20, ProductID: 9567, Product: "sgg", Ingredients: []gnomeNamePair{{"vodka", 1}, {"lime", 3}}, Finish: []gnomeNamePair{{"lime_slices", 1}, {"equa_leaves", 1}}, FinishXP: 90, HasFinishXP: true},
		{Selector: 4, Level: 32, ProductID: mixedDragonShaker, Poured: mixedDragonPoured, Product: "drunk_dragon", Ingredients: []gnomeNamePair{{"vodka", 1}, {"gin", 1}, {"dwellberries", 1}}},
		nil, nil,
	},
	{
		nil,
		{Selector: 8, Level: 26, ProductID: 9482, Spice: true, Ingredients: []gnomeNamePair{{"equa_leaves", 1}, {"toads_legs", 1}, {"cheese", 1}}},
		nil, nil,
		{Selector: 11, Level: 29, ProductID: 9483, Ingredients: []gnomeNamePair{{"tomato", 1}, {"cheese", 1}}},
	},
	{
		{Selector: 16, Level: 10, ProductID: 9581, Spice: true, Ingredients: []gnomeNamePair{{"toads_legs", 2}}},
		nil, nil,
		{Selector: 19, Level: 16, ProductID: 9577, Spice: true, Ingredients: []gnomeNamePair{{"chocolate_bar", 2}}},
	},
}

// gnomeFinishRow is one FINISH row (Gnome.Cooking.js:51-55):
// the source item (numeric id), the ingredient gameval name,
// the result gameval name, and the XP. The per-product
// Creation tables list Cooking 1 for these final garnish
// operations; the higher recipe levels apply to the earlier
// preparation.
type gnomeFinishRow struct {
	Source     int
	Ingredient string
	Result     string
	XP         float64
}

var gnomeFinish = []gnomeFinishRow{
	{Source: 9582, Ingredient: "equa_leaves", Result: "toad_crunchies", XP: 40},
	{Source: 9578, Ingredient: "chocolate_dust", Result: "chocchip_crunchies", XP: 46},
	{Source: 9484, Ingredient: "equa_leaves", Result: "cheese+tom_batta", XP: 88},
}

// gnomeCut is one FRUIT cut option (Gnome.Cooking.js:197-202):
// the prompt label, the output gameval name, and the count.
type gnomeCut struct {
	Label  string
	Output string
	Count  int
}

// gnomeFruit is FRUIT: pineapple → Dice pineapple_chunks ×1 /
// Slice pineapple_ring ×4; lemon → lemon_chunks ×1 /
// lemon_slices ×1; orange → orange_chunks ×1 / orange_slices
// ×1; lime → lime_chunks ×1 / lime_slices ×1.
var gnomeFruit = map[string][2]gnomeCut{
	"pineapple": {{"Dice", "pineapple_chunks", 1}, {"Slice", "pineapple_ring", 4}},
	"lemon":     {{"Dice", "lemon_chunks", 1}, {"Slice", "lemon_slices", 1}},
	"orange":    {{"Dice", "orange_chunks", 1}, {"Slice", "orange_slices", 1}},
	"lime":      {{"Dice", "lime_chunks", 1}, {"Slice", "lime_slices", 1}},
}

// gnomeUIIDs is the resolved ui() (Gnome.Cooking.js:104-112):
// interface id, clickzone + confirmation components, the type
// + chosen varbits, and the main-modal component — ALL resolved
// by cache gameval; ui() throws when any is missing (Go:
// resolveGnomeUI reports the missing name and the unit fails
// closed).
type gnomeUIIDs struct {
	Group        int
	Click        int
	Create       int
	TypeVarbit   int
	ChosenVarbit int
	Main         int
}

// gnomeSession is one player's open gnome menu (Gnome.Cooking.js
// sessions): the menu type, the source item (value copy) and its
// slot + id, the selected recipe (nil until a real click), and
// the sub-interface mount.
type gnomeSession struct {
	Type     int
	Item     game.Item
	Slot     int
	Source   int
	Selected *gnomeMenuEntry
	Mount    *session.SubInterfaceMount
}

// gnomeItemRef is one item-on-item side (Gnome.Cooking.js
// useItems pair entries): the id, the item value, and the slot.
type gnomeItemRef struct {
	ID   int
	Item game.Item
	Slot int
}

// gnomeCutToken is one pending fruit cut (Gnome.Cooking.js
// cuts): the fruit and knife refs; the cuts map holds one
// token per player.
type gnomeCutToken struct {
	Fruit gnomeItemRef
	Knife gnomeItemRef
}

// gnomePacketSender is the session-side sender surface Gnome
// needs (Gnome.Cooking.js player.getPacketSender() calls),
// reached through a type assertion on player.Session (the
// fletching varbitSender / CreationMenuSender precedent).
// *session.Session implements all of it.
type gnomePacketSender interface {
	SendVarbit(id, value int)
	SendInterfaceFlagsRange(uid, fromSlot, toSlot, flags int)
	MountSubInterface(targetUID, groupID, typ int) *session.SubInterfaceMount
	GetSubInterfaceAt(targetUID int) (*session.SubInterfaceMount, bool)
	CloseSubInterfaceMount(mount *session.SubInterfaceMount) bool
}

// gnomeSender returns the player's gnomePacketSender, or nil
// when the player has no session (or a session without the
// surface).
func gnomeSender(player *game.Player) gnomePacketSender {
	if player == nil || player.Session == nil {
		return nil
	}
	sender, ok := player.Session.(gnomePacketSender)
	if !ok {
		return nil
	}
	return sender
}

// gnomeUnit is the Gnome unit's per-registration state: the
// open-menu sessions, the pending fruit-cut tokens, the
// resolved ui ids, and the resolved obj-name cache. One
// instance serves the registered Plugin value (the Phase 21
// race fix: no package-level mutable maps).
type gnomeUnit struct {
	c *cooking
	// resolve answers the obj/interface/component/varbit
	// gamevals; nil resolves nothing (the unit fails closed).
	resolve gamevalResolver
	// sessions maps each player to their open menu (TS
	// sessions WeakMap).
	sessions map[*game.Player]*gnomeSession
	// cuts maps each player to their pending fruit cut (TS
	// cuts WeakMap).
	cuts map[*game.Player]*gnomeCutToken
	// ui is the resolved interface ids; uiOK reports whether
	// every id resolved.
	ui   *gnomeUIIDs
	uiOK bool
	// objIDs caches resolved obj gameval names; objMissing
	// caches the names that did not resolve.
	objIDs     map[string]int
	objMissing map[string]bool
	// prompt is the test override for the fruit-cutting
	// prompt (nil means Api.SendMultiChatboxPrompt).
	prompt func(player *game.Player, title string, pairs ...any) bool
}

// newGnomeUnit builds a Gnome unit over c, resolving the ui
// ids eagerly (TS ui() throws when any is missing — Go records
// uiOK=false and fails closed; the load-prove verifies every
// name against the real cache).
func newGnomeUnit(c *cooking, resolve gamevalResolver) *gnomeUnit {
	u := &gnomeUnit{
		c:          c,
		resolve:    resolve,
		sessions:   make(map[*game.Player]*gnomeSession),
		cuts:       make(map[*game.Player]*gnomeCutToken),
		objIDs:     make(map[string]int),
		objMissing: make(map[string]bool),
	}
	if resolve != nil {
		ui := &gnomeUIIDs{}
		ok := true
		if id, good, _ := resolve.InterfaceID(gnomeMenuName); good {
			ui.Group = id
		} else {
			ok = false
		}
		if id, good, _ := resolve.ComponentID(gnomeMenuName + ":cuisine_clickzone"); good {
			ui.Click = id
		} else {
			ok = false
		}
		if id, good, _ := resolve.ComponentID(gnomeMenuName + ":confirmation"); good {
			ui.Create = id
		} else {
			ok = false
		}
		if id, good, _ := resolve.VarbitID(gnomeMenuTypeVarbit); good {
			ui.TypeVarbit = id
		} else {
			ok = false
		}
		if id, good, _ := resolve.VarbitID(gnomeChosenVarbit); good {
			ui.ChosenVarbit = id
		} else {
			ok = false
		}
		if id, good, _ := resolve.ComponentID("toplevel_osrs_stretch:mainmodal"); good {
			ui.Main = id
		} else {
			ok = false
		}
		u.ui, u.uiOK = ui, ok
	}
	return u
}

// objID resolves a gameval obj name through the resolver,
// cached (TS Common.gameval("obj", name), which throws on
// unknown — Go fails closed instead).
func (u *gnomeUnit) objID(name string) (int, bool) {
	if id, ok := u.objIDs[name]; ok {
		return id, true
	}
	if u.objMissing[name] {
		return 0, false
	}
	if u.resolve == nil {
		u.objMissing[name] = true
		return 0, false
	}
	id, ok, _ := u.resolve.ObjID(name)
	if !ok {
		u.objMissing[name] = true
		return 0, false
	}
	u.objIDs[name] = id
	return id, true
}

// gnomePlainItem is plain (Gnome.Cooking.js plain): not
// preset / untradeable / unbankable.
func gnomePlainItem(item game.Item) bool {
	return !item.IsPresetItem() && !item.IsUntradeable() && !item.IsUnbankable()
}

// alive is alive (Gnome.Cooking.js alive): members world,
// registered, HP > 0.
func (u *gnomeUnit) alive(player *game.Player) bool {
	return u.c.membersWorld && player != nil && player.IsRegistered() && hitpoints(player) > 0
}

// ---- plan / apply (Gnome.Cooking.js:66-102) ----

// gnomePair is one planned input/output: a resolved item id
// and a count.
type gnomePair struct {
	ID    int
	Count int
}

// gnomePreferred is one preferred slot (Gnome.Cooking.js plan
// preferred entries): the item id, the slot, and the item
// value that must still be there.
type gnomePreferred struct {
	ID   int
	Slot int
	Item game.Item
}

// gnomeTransaction is a planned transaction: the inventory,
// the input slots to delete, and the outputs to add.
type gnomeTransaction struct {
	Inventory *game.Inventory
	Slots     []int
	Outputs   []gnomePair
}

// resolvePairs resolves name pairs to id pairs (a name that
// does not resolve fails the plan).
func (u *gnomeUnit) resolvePairs(pairs []gnomeNamePair) ([]gnomePair, bool) {
	out := make([]gnomePair, 0, len(pairs))
	for _, p := range pairs {
		id, ok := u.objID(p.Name)
		if !ok {
			return nil, false
		}
		out = append(out, gnomePair{ID: id, Count: p.Count})
	}
	return out, true
}

// plan is plan (Gnome.Cooking.js:66-92): selected ingredients
// and results are normal, nonstackable cache items. Plan actual
// plain slots and the entire output capacity before changing
// inventory. Tools that are not transformed are checked
// separately and never consumed. Returns nil when unplannable.
func (u *gnomeUnit) plan(player *game.Player, inputs, outputs []gnomePair, preferred []gnomePreferred) *gnomeTransaction {
	if player == nil {
		return nil
	}
	inventory := player.Inventory()
	if inventory == nil {
		return nil
	}
	items := inventory.Items()
	stackable := func(id int) bool {
		if src := player.ItemDefinitions(); src != nil {
			return src.IsStackable(id)
		}
		return false
	}
	used := make(map[int]bool)
	var slots []int
	for _, in := range inputs {
		if stackable(in.ID) {
			return nil
		}
		var required []gnomePreferred
		for _, entry := range preferred {
			if entry.ID == in.ID {
				required = append(required, entry)
			}
		}
		if len(required) > in.Count {
			return nil
		}
		for _, entry := range required {
			if entry.Slot < 0 || entry.Slot >= len(items) ||
				!items[entry.Slot].Equals(entry.Item) ||
				entry.Item.ID() != in.ID || entry.Item.Amount() != 1 ||
				!gnomePlainItem(entry.Item) {
				return nil
			}
		}
		order := make([]int, 0, len(required)+len(items))
		for _, entry := range required {
			order = append(order, entry.Slot)
		}
		for slot := range items {
			order = append(order, slot)
		}
		remaining := in.Count
		for _, slot := range order {
			if remaining <= 0 {
				break
			}
			item := items[slot]
			if used[slot] || item.ID() != in.ID || item.Amount() != 1 || !gnomePlainItem(item) {
				continue
			}
			used[slot] = true
			slots = append(slots, slot)
			remaining--
		}
		if remaining > 0 {
			return nil
		}
	}
	needed := 0
	for _, out := range outputs {
		if stackable(out.ID) {
			return nil
		}
		needed += out.Count
	}
	if inventory.GetFreeSlots()+len(slots) < needed {
		return nil
	}
	return &gnomeTransaction{Inventory: inventory, Slots: slots, Outputs: outputs}
}

// apply is apply (Gnome.Cooking.js:94-102): deleteAtSlot per
// input slot, add outputs, refresh, addXp, skills:produced
// (kind "make") — kind "" emits nothing (the emptyBurnt null).
func (u *gnomeUnit) apply(player *game.Player, txn *gnomeTransaction, productID int, xp float64, kind string) {
	if player == nil || txn == nil {
		return
	}
	for _, slot := range txn.Slots {
		txn.Inventory.DeleteAtSlot(slot, 1, false)
	}
	for _, out := range txn.Outputs {
		txn.Inventory.Add(game.NewItem(out.ID, out.Count), false)
	}
	txn.Inventory.RefreshItems()
	u.c.addXP(player, xp)
	if kind == "" {
		return
	}
	amount := 1
	for _, out := range txn.Outputs {
		if out.ID == productID {
			amount = out.Count
			break
		}
	}
	u.c.emitEvent(SkillsProducedEventName, &ProducedEvent{
		Player: player,
		Skill:  game.SkillCooking,
		ItemID: productID,
		Amount: amount,
		XP:     xp,
		Kind:   kind,
	})
}

// valid is valid (Gnome.Cooking.js:113-118): members world,
// registered, HP > 0, current interface id == group,
// sub-interface at main == session.mount, the slot still holds
// the same item, item id == source, item plain. The slot check
// compares item values (Go Items() returns copies — Equals on
// id/amount/meta — where TS compares references).
func (u *gnomeUnit) valid(player *game.Player, session *gnomeSession) bool {
	if session == nil || !u.alive(player) || !u.uiOK {
		return false
	}
	if player.GetInterfaceId() != u.ui.Group {
		return false
	}
	sender := gnomeSender(player)
	if sender == nil {
		return false
	}
	mount, ok := sender.GetSubInterfaceAt(u.ui.Main)
	if !ok || mount != session.Mount {
		return false
	}
	inventory := player.Inventory()
	if inventory == nil {
		return false
	}
	items := inventory.Items()
	if session.Slot < 0 || session.Slot >= len(items) {
		return false
	}
	return items[session.Slot].Equals(session.Item) &&
		session.Item.ID() == session.Source && gnomePlainItem(session.Item)
}

// ---- open / select / create (Gnome.Cooking.js:120-174) ----

// openMenu is openMenu (Gnome.Cooking.js:120-141): members,
// not a bot, plain item, slot holds the item; send varbit type
// + chosen default [0, 7, 16][type]; setInterfaceId (group);
// mountSubInterface(main, group, 0); send interface flags
// range on the clickzone (0..len-1, OP1). Two
// synchronous-replacement guards (an interface-opened hook may
// have replaced the mount — return without storing a session).
// Returns false (falls through) when the guards fail or the ui
// is unresolvable, true when the menu opened.
func (u *gnomeUnit) openMenu(event *plugins.PluginItemActionEvent, menuType int) bool {
	if event == nil || event.Player == nil || !u.uiOK {
		return false
	}
	player := event.Player
	inventory := player.Inventory()
	if inventory == nil {
		return false
	}
	item := inventory.ItemAt(event.Slot)
	if !u.alive(player) || player.IsPlayerBot() || !gnomePlainItem(item) ||
		item.ID() != event.ItemID {
		return false
	}
	session := &gnomeSession{Type: menuType, Item: item, Slot: event.Slot, Source: event.ItemID}
	delete(u.sessions, player)
	sender := gnomeSender(player)
	if sender == nil {
		return false
	}
	sender.SendVarbit(u.ui.TypeVarbit, menuType)
	// The interface script needs a valid default selector even
	// before the actor selects a supported recipe; server state
	// remains null until a real click.
	sender.SendVarbit(u.ui.ChosenVarbit, [3]int{0, 7, 16}[menuType])
	previousMount, _ := sender.GetSubInterfaceAt(u.ui.Main)
	player.SetInterfaceId(u.ui.Group)
	// A synchronous interface:changed listener may have opened
	// a different modal.
	currentMount, _ := sender.GetSubInterfaceAt(u.ui.Main)
	if player.GetInterfaceId() != u.ui.Group || currentMount != previousMount {
		return false
	}
	session.Mount = sender.MountSubInterface(u.ui.Main, u.ui.Group, 0)
	// An interface-opened hook may have replaced our mount
	// synchronously.
	if mounted, _ := sender.GetSubInterfaceAt(u.ui.Main); mounted != session.Mount {
		return false
	}
	u.sessions[player] = session
	sender.SendInterfaceFlagsRange(u.ui.Click, 0, len(gnomeMenus[menuType])-1, gnomeOp1)
	return true
}

// openCocktails / openBatta / openCrunchy open the three
// menus (Gnome.Cooking.js:142-144).
func (u *gnomeUnit) openCocktails(event *plugins.PluginItemActionEvent) bool {
	return u.openMenu(event, 0)
}

func (u *gnomeUnit) openBatta(event *plugins.PluginItemActionEvent) bool {
	return u.openMenu(event, 1)
}

func (u *gnomeUnit) openCrunchy(event *plugins.PluginItemActionEvent) bool {
	return u.openMenu(event, 2)
}

// selectCuisine is selectCuisine (Gnome.Cooking.js:146-155):
// interface click on the clickzone component, action 1 —
// session.selected = MENUS[type][slot] ?? null; a null clears
// server state; a selection sends varbit chosen = selector.
func (u *gnomeUnit) selectCuisine(event *plugins.InterfaceActionClickEvent) {
	if event == nil || event.Player == nil {
		return
	}
	player := event.Player
	session := u.sessions[player]
	if !u.valid(player, session) || event.Action != 1 {
		return
	}
	event.Handled = true
	var selected *gnomeMenuEntry
	if event.HasSlot && event.Slot >= 0 && event.Slot < len(gnomeMenus[session.Type]) {
		selected = gnomeMenus[session.Type][event.Slot]
	}
	session.Selected = selected
	if selected != nil {
		if sender := gnomeSender(player); sender != nil {
			sender.SendVarbit(u.ui.ChosenVarbit, selected.Selector)
		}
	}
}

// createCuisine is createCuisine (Gnome.Cooking.js:157-174):
// interface click on the confirmation component, action 1 —
// level gate (silent return); spice gate (gnome_spice ≥ 1 when
// recipe.spice); plan with [[source, 1], ...ingredients] →
// [[product, 1]]; apply with XP 30 (cocktails) / 40 (batta) /
// 30 (crunchy), kind "make"; close the mount (closeSubInterface
// Mount, clear interface id when nothing is mounted).
func (u *gnomeUnit) createCuisine(event *plugins.InterfaceActionClickEvent) {
	if event == nil || event.Player == nil {
		return
	}
	player := event.Player
	session := u.sessions[player]
	if !u.valid(player, session) || event.Action != 1 {
		return
	}
	event.Handled = true
	recipe := session.Selected
	if recipe == nil || u.c.cookingLevel(player) < recipe.Level {
		return
	}
	if recipe.Spice {
		spiceID, ok := u.objID(gnomeSpiceName)
		if !ok {
			return
		}
		if inv := player.Inventory(); inv == nil || inv.GetAmount(spiceID) < 1 {
			return
		}
	}
	ingredients, ok := u.resolvePairs(recipe.Ingredients)
	if !ok {
		return
	}
	inputs := append([]gnomePair{{ID: session.Source, Count: 1}}, ingredients...)
	txn := u.plan(player, inputs, []gnomePair{{ID: recipe.ProductID, Count: 1}},
		[]gnomePreferred{{ID: session.Source, Slot: session.Slot, Item: session.Item}})
	if txn == nil {
		return
	}
	delete(u.sessions, player)
	xp := 30.0
	if session.Type == 1 {
		xp = 40
	}
	u.apply(player, txn, recipe.ProductID, xp, "make")
	sender := gnomeSender(player)
	if sender == nil {
		return
	}
	if mount, ok := sender.GetSubInterfaceAt(u.ui.Main); ok && mount == session.Mount {
		closed := sender.CloseSubInterfaceMount(session.Mount)
		if closed {
			if _, ok := sender.GetSubInterfaceAt(u.ui.Main); !ok && player.GetInterfaceId() == u.ui.Group {
				player.SetInterfaceId(-1)
			}
		}
	}
}

// ---- pour / garnish / empty (Gnome.Cooking.js:176-197) ----

// cocktailByMixed finds a cocktail by its mixed id, or nil.
func cocktailByMixed(mixedID int) *gnomeMenuEntry {
	for _, entry := range gnomeMenus[0] {
		if entry != nil && entry.ProductID == mixedID {
			return entry
		}
	}
	return nil
}

// itemAtSlot reads the inventory item at a slot, or an empty
// item when the slot is out of range.
func itemAtSlot(player *game.Player, slot int) game.Item {
	if player == nil {
		return game.NewItem(-1, 0)
	}
	inventory := player.Inventory()
	if inventory == nil {
		return game.NewItem(-1, 0)
	}
	return inventory.ItemAt(slot)
}

// pour is pour (Gnome.Cooking.js:176-184): product = poured ??
// product; plan [[mixed, 1], [cocktail_glass_empty, 1],
// ...finish] → [[cocktail_shaker, 1], [product, 1]]; apply
// with finishXp ?? 0.
func (u *gnomeUnit) pour(event *plugins.PluginItemActionEvent) bool {
	if event == nil || event.Player == nil {
		return false
	}
	player := event.Player
	recipe := cocktailByMixed(event.ItemID)
	if recipe == nil {
		return false
	}
	item := itemAtSlot(player, event.Slot)
	if !u.alive(player) || !gnomePlainItem(item) || item.ID() != event.ItemID ||
		u.c.cookingLevel(player) < recipe.Level {
		return false
	}
	productID := recipe.Poured
	if productID == 0 {
		id, ok := u.objID(recipe.Product)
		if !ok {
			return false
		}
		productID = id
	}
	glassID, ok := u.objID("cocktail_glass_empty")
	if !ok {
		return false
	}
	shakerID, ok := u.objID("cocktail_shaker")
	if !ok {
		return false
	}
	finish, ok := u.resolvePairs(recipe.Finish)
	if !ok {
		return false
	}
	inputs := append([]gnomePair{{ID: event.ItemID, Count: 1}, {ID: glassID, Count: 1}}, finish...)
	txn := u.plan(player, inputs,
		[]gnomePair{{ID: shakerID, Count: 1}, {ID: productID, Count: 1}},
		[]gnomePreferred{{ID: event.ItemID, Slot: event.Slot, Item: item}})
	if txn == nil {
		return false
	}
	xp := 0.0
	if recipe.HasFinishXP {
		xp = recipe.FinishXP
	}
	u.apply(player, txn, productID, xp, "make")
	return true
}

// garnishDragon is garnishDragon (Gnome.Cooking.js:186-192):
// 9575 + pineapple_chunks + pot_of_cream → 9576, xp 0, kind
// "make".
func (u *gnomeUnit) garnishDragon(event *plugins.PluginItemActionEvent) bool {
	if event == nil || event.Player == nil {
		return false
	}
	player := event.Player
	if event.ItemID != mixedDragonPoured {
		return false
	}
	item := itemAtSlot(player, event.Slot)
	if !u.alive(player) || !gnomePlainItem(item) || item.ID() != event.ItemID ||
		u.c.cookingLevel(player) < 32 {
		return false
	}
	chunksID, ok := u.objID("pineapple_chunks")
	if !ok {
		return false
	}
	creamID, ok := u.objID("pot_of_cream")
	if !ok {
		return false
	}
	txn := u.plan(player,
		[]gnomePair{{ID: event.ItemID, Count: 1}, {ID: chunksID, Count: 1}, {ID: creamID, Count: 1}},
		[]gnomePair{{ID: mixedDragonGarnished, Count: 1}},
		[]gnomePreferred{{ID: event.ItemID, Slot: event.Slot, Item: item}})
	if txn == nil {
		return false
	}
	u.apply(player, txn, mixedDragonGarnished, 0, "make")
	return true
}

// emptyBurnt is emptyBurnt (Gnome.Cooking.js:194-202): burnt
// crunchies → crunchy_tray / burnt batta → batta_tin, xp 0,
// kind null (no skills:produced emit).
func (u *gnomeUnit) emptyBurnt(event *plugins.PluginItemActionEvent) bool {
	if event == nil || event.Player == nil {
		return false
	}
	player := event.Player
	item := itemAtSlot(player, event.Slot)
	if !u.alive(player) || !gnomePlainItem(item) || item.ID() != event.ItemID {
		return false
	}
	var resultName string
	burntCrunchies, ok1 := u.objID("burnt_crunchies")
	burntBatta, ok2 := u.objID("burnt_batta")
	switch {
	case ok1 && event.ItemID == burntCrunchies:
		resultName = "crunchy_tray"
	case ok2 && event.ItemID == burntBatta:
		resultName = "batta_tin"
	default:
		return false
	}
	resultID, ok := u.objID(resultName)
	if !ok {
		return false
	}
	txn := u.plan(player,
		[]gnomePair{{ID: event.ItemID, Count: 1}},
		[]gnomePair{{ID: resultID, Count: 1}},
		[]gnomePreferred{{ID: event.ItemID, Slot: event.Slot, Item: item}})
	if txn == nil {
		return false
	}
	u.apply(player, txn, resultID, 0, "")
	return true
}

// interfaceClosed is interfaceClosed (Gnome.Cooking.js:
// 204-209): only when interfaceId == group; drop the session
// when the mount no longer matches.
func (u *gnomeUnit) interfaceClosed(payload any) {
	if !u.uiOK {
		return
	}
	var player *game.Player
	var interfaceID int
	switch ev := payload.(type) {
	case game.InterfaceClosedEvent:
		player, interfaceID = ev.Player, ev.InterfaceID
	case *game.InterfaceClosedEvent:
		if ev == nil {
			return
		}
		player, interfaceID = ev.Player, ev.InterfaceID
	default:
		return
	}
	if interfaceID != u.ui.Group || player == nil {
		return
	}
	session := u.sessions[player]
	if session == nil {
		return
	}
	// A previous close listener may have synchronously opened
	// a new group 49.
	if sender := gnomeSender(player); sender != nil {
		if mount, ok := sender.GetSubInterfaceAt(u.ui.Main); ok && mount == session.Mount {
			return
		}
	}
	delete(u.sessions, player)
}

// clearPlayer drops a player's session and cut token on
// logout / disconnect / death (Gnome.Cooking.js:211).
func (u *gnomeUnit) clearPlayer(player *game.Player) {
	if player == nil {
		return
	}
	delete(u.sessions, player)
	delete(u.cuts, player)
}

// ---- fruit (Gnome.Cooking.js:213-259) ----

// chooseCut is chooseCut (Gnome.Cooking.js:224-234): token
// must still match (the cuts map holds one token per player);
// plan fruit → output; apply xp 0, kind "make".
func (u *gnomeUnit) chooseCut(player *game.Player, token *gnomeCutToken, output string, count int) {
	if player == nil || token == nil || u.cuts[player] != token || !u.alive(player) {
		return
	}
	delete(u.cuts, player)
	inventory := player.Inventory()
	if inventory == nil {
		return
	}
	items := inventory.Items()
	if token.Fruit.Slot < 0 || token.Fruit.Slot >= len(items) ||
		token.Knife.Slot < 0 || token.Knife.Slot >= len(items) {
		return
	}
	if !items[token.Fruit.Slot].Equals(token.Fruit.Item) ||
		!items[token.Knife.Slot].Equals(token.Knife.Item) ||
		token.Knife.Item.ID() != token.Knife.ID {
		return
	}
	outputID, ok := u.objID(output)
	if !ok {
		return
	}
	txn := u.plan(player,
		[]gnomePair{{ID: token.Fruit.ID, Count: 1}},
		[]gnomePair{{ID: outputID, Count: count}},
		[]gnomePreferred{{ID: token.Fruit.ID, Slot: token.Fruit.Slot, Item: token.Fruit.Item}})
	if txn == nil {
		return
	}
	u.apply(player, txn, outputID, 0, "make")
}

// sendPrompt shows the fruit-cutting prompt (Gnome.Cooking.js
// useItems: Api.SendMultiChatboxPrompt — pairs are
// text/callback alternates; false → drop the token). The
// prompt field is the test override; nil means the Api call
// (nil-safe: a nil Api reports false).
func (u *gnomeUnit) sendPrompt(player *game.Player, title string, pairs ...any) bool {
	if u.prompt != nil {
		return u.prompt(player, title, pairs...)
	}
	if u.c == nil || u.c.api == nil {
		return false
	}
	return u.c.api.SendMultiChatboxPrompt(player, title, pairs...)
}

// useItems is useItems (Gnome.Cooking.js:236-259):
// gianne_dough + crunchy_tray → raw_crunchies xp 0;
// gianne_dough + batta_tin → raw_batta xp 0; FINISH pairs →
// product + xp; knife + fruit → cuts.set(player, token) +
// sendMultiChatboxPrompt(player, "How would you like to cut
// the fruit?", "Dice", fn, "Slice", fn).
func (u *gnomeUnit) useItems(event *plugins.PluginItemOnItemEvent) {
	if event == nil || event.Player == nil {
		return
	}
	player := event.Player
	if event.UsedItem.ID() < 0 || event.UsedWithItem.ID() < 0 {
		return
	}
	if !u.alive(player) {
		return
	}
	pair := []gnomeItemRef{
		{ID: event.UsedItemID, Item: event.UsedItem, Slot: event.UsedItemSlot},
		{ID: event.UsedWithItemID, Item: event.UsedWithItem, Slot: event.UsedWithItemSlot},
	}
	hasPair := func(a, b string) bool {
		aID, okA := u.objID(a)
		bID, okB := u.objID(b)
		if !okA || !okB {
			return false
		}
		return (pair[0].ID == aID && pair[1].ID == bID) ||
			(pair[0].ID == bID && pair[1].ID == aID)
	}
	type simple struct {
		a, b, product string
		xp            float64
	}
	var simpleMade *simple
	if hasPair("gianne_dough", "crunchy_tray") {
		simpleMade = &simple{"gianne_dough", "crunchy_tray", "raw_crunchies", 0}
	} else if hasPair("gianne_dough", "batta_tin") {
		simpleMade = &simple{"gianne_dough", "batta_tin", "raw_batta", 0}
	}
	var finish *gnomeFinishRow
	for i := range gnomeFinish {
		row := &gnomeFinish[i]
		ingredientID, ok := u.objID(row.Ingredient)
		if !ok {
			continue
		}
		if (pair[0].ID == row.Source && pair[1].ID == ingredientID) ||
			(pair[1].ID == row.Source && pair[0].ID == ingredientID) {
			finish = row
			break
		}
	}
	if simpleMade != nil || finish != nil {
		event.Handled = true
		var inIDs []gnomePair
		var product string
		var xp float64
		if simpleMade != nil {
			inputs := []gnomeNamePair{{simpleMade.a, 1}, {simpleMade.b, 1}}
			resolved, ok := u.resolvePairs(inputs)
			if !ok {
				return
			}
			inIDs = resolved
			product, xp = simpleMade.product, simpleMade.xp
		} else {
			// The finish source is a numeric id, not a name.
			ingredientID, ok := u.objID(finish.Ingredient)
			if !ok {
				return
			}
			inIDs = []gnomePair{{ID: finish.Source, Count: 1}, {ID: ingredientID, Count: 1}}
			product, xp = finish.Result, finish.XP
		}
		productID, ok := u.objID(product)
		if !ok {
			return
		}
		preferred := []gnomePreferred{
			{ID: pair[0].ID, Slot: pair[0].Slot, Item: pair[0].Item},
			{ID: pair[1].ID, Slot: pair[1].Slot, Item: pair[1].Item},
		}
		if txn := u.plan(player, inIDs, []gnomePair{{ID: productID, Count: 1}}, preferred); txn != nil {
			u.apply(player, txn, productID, xp, "make")
		}
		return
	}
	knives := make(map[int]bool, len(u.c.knives()))
	for _, id := range u.c.knives() {
		knives[id] = true
	}
	var knife, fruit *gnomeItemRef
	for i := range pair {
		if knives[pair[i].ID] && knife == nil {
			knife = &pair[i]
		}
	}
	if knife == nil {
		return
	}
	var fruitName string
	for i := range pair {
		if &pair[i] == knife {
			continue
		}
		for name := range gnomeFruit {
			if id, ok := u.objID(name); ok && pair[i].ID == id {
				fruit = &pair[i]
				fruitName = name
				break
			}
		}
		if fruit != nil {
			break
		}
	}
	if fruit == nil || !gnomePlainItem(fruit.Item) {
		return
	}
	token := &gnomeCutToken{Fruit: *fruit, Knife: *knife}
	event.Handled = true
	u.cuts[player] = token
	var pairs []any
	for _, cut := range gnomeFruit[fruitName] {
		cut := cut
		pairs = append(pairs, cut.Label, plugins.PromptCallback(func(p *game.Player, _ int, _ string) {
			u.chooseCut(p, token, cut.Output, cut.Count)
		}))
	}
	if !u.sendPrompt(player, gnomeFruitPrompt, pairs...) {
		delete(u.cuts, player)
	}
}

// interfaceAction is interfaceAction (Gnome.Cooking.js:
// 261-266): ignore when cachedUi is nil or (buttonId >>> 16)
// !== group; click component → selectCuisine; create
// component → createCuisine.
func (u *gnomeUnit) interfaceAction(event *plugins.InterfaceActionClickEvent) {
	if event == nil || event.Player == nil || !u.uiOK {
		return
	}
	if event.ButtonID>>16 != u.ui.Group {
		return
	}
	if event.ButtonID == u.ui.Click {
		u.selectCuisine(event)
	} else if event.ButtonID == u.ui.Create {
		u.createCuisine(event)
	}
}

// registerGnome wires the Gnome unit (Gnome.Cooking.js attach):
// 10× onItemAction (Cocktail shaker Mix-cocktail; Half baked
// batta Prepare; Half baked crunchy Prepare; Mixed blast/punch/
// blizzard/sgg/dragon Pour; Mixed dragon Add-ingreds; Burnt
// crunchies Empty Tray; Burnt batta Empty Tin);
// onItemOnItem (noted:false); onInterfaceActionClick;
// onCustomEvent("interface:closed"); onPlayerLogout,
// onPlayerDisconnect, onPlayerDeath (all → clearPlayer).
// resolve answers every obj/interface/component/varbit gameval
// the unit names; nil resolves nothing (the unit fails closed
// — the load-prove verifies the names resolve against the real
// cache instead).
func (c *cooking) registerGnome(api *plugins.Api, resolve gamevalResolver) {
	if api == nil {
		return
	}
	u := newGnomeUnit(c, resolve)
	api.OnItemActionNamed("Cocktail shaker", map[string]plugins.ItemActionHandler{"Mix-cocktail": u.openCocktails}, nil)
	api.OnItemActionNamed("Half baked batta", map[string]plugins.ItemActionHandler{"Prepare": u.openBatta}, nil)
	api.OnItemActionNamed("Half baked crunchy", map[string]plugins.ItemActionHandler{"Prepare": u.openCrunchy}, nil)
	api.OnItemActionNamed("Mixed blast", map[string]plugins.ItemActionHandler{"Pour": u.pour}, nil)
	api.OnItemActionNamed("Mixed punch", map[string]plugins.ItemActionHandler{"Pour": u.pour}, nil)
	api.OnItemActionNamed("Mixed blizzard", map[string]plugins.ItemActionHandler{"Pour": u.pour}, nil)
	api.OnItemActionNamed("Mixed sgg", map[string]plugins.ItemActionHandler{"Pour": u.pour}, nil)
	api.OnItemActionNamed("Mixed dragon", map[string]plugins.ItemActionHandler{"Pour": u.pour, "Add-ingreds": u.garnishDragon}, nil)
	api.OnItemActionNamed("Burnt crunchies", map[string]plugins.ItemActionHandler{"Empty Tray": u.emptyBurnt}, nil)
	api.OnItemActionNamed("Burnt batta", map[string]plugins.ItemActionHandler{"Empty Tin": u.emptyBurnt}, nil)
	api.OnItemOnItem(u.useItems, &plugins.ItemUseFilter{Noted: plugins.BoolPtr(false)})
	api.OnInterfaceActionClick(u.interfaceAction)
	api.OnCustomEvent(game.InterfaceClosedEventName, u.interfaceClosed)
	clear := func(player *game.Player) { u.clearPlayer(player) }
	api.OnPlayerLogout(func(event *plugins.PlayerLogoutEvent) {
		if event != nil {
			clear(event.Player)
		}
	})
	api.OnPlayerDisconnect(func(event *plugins.PlayerDisconnectEvent) {
		if event != nil {
			clear(event.Player)
		}
	})
	api.OnPlayerDeath(func(event *plugins.PlayerDeathEvent) {
		if event != nil {
			clear(event.Player)
		}
	})
}
