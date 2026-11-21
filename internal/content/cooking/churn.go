// Churn.Cooking.js (181 lines, commit bfd81e3): dairy churns —
// Churn (or milk, cream or butter used on a churn) opens "What
// would you like to make?" with Cream, Butter and Cheese, and
// churns one item per cycle. Levels, XP, inputs and cycle
// lengths are the Wiki's (Dairy churn, Pot of cream, Pat of
// butter, Cheese) in plugins/skills/data/cooking.json; the menu,
// animation, butter line, slots and timing are from rsprox
// captures (recordings 90 and 1105).
//
// Worker C file (PHASE36_PLAN.md §3).
package cooking

import (
	"fmt"
	"strings"

	"tsps-go/internal/game"
	"tsps-go/internal/plugins"
)

// ---- Churn constants (Churn.Cooking.js:14-18) ----

const (
	// churnMenuTitle is MENU_TITLE ("What would you like to
	// make?"): the menu title and clientscript
	// skillmulti_setup's first argument (rsprox captures 90,
	// 1105).
	churnMenuTitle = "What would you like to make?"
	// churnMenuMode is MENU_MODE (0).
	churnMenuMode = 0
	// nothingToChurnMessage is NOTHING_TO_CHURN_MESSAGE: ours
	// — no capture shows Churn with nothing to churn.
	nothingToChurnMessage = "You have nothing to churn."
)

// churnProductOrderNames is the churn products' order
// (Churn.Cooking.js churnData: productOrder from the labels
// keys): butter before cream before milk — i.e. the
// cooking.json labels key order (pot_of_cream, pot_of_butter,
// cheese), resolved to product ids. Go maps lose JSON key
// order, so the order is spelled out here in the file's key
// order (verified by parse of data/definitions/cooking.json).
var churnProductOrderNames = []string{"pot_of_cream", "pot_of_butter", "cheese"}

// churnUnit is the Churn unit's per-registration state. It
// holds only the shared *cooking: every table lives in
// Tables.Churn (resolved by the loader), and the 1-tick churn
// loop runs on the shared Common.run keyed task — so the unit
// carries no per-player state of its own.
type churnUnit struct {
	c *cooking
}

// newChurnUnit builds a Churn unit over c.
func newChurnUnit(c *cooking) *churnUnit {
	return &churnUnit{c: c}
}

// churnData returns the resolved churn tables, or nil without
// them (Churn.Cooking.js churnData).
func (u *churnUnit) churnData() *ChurnData {
	if u == nil || u.c == nil || u.c.tables == nil {
		return nil
	}
	return u.c.tables.Churn
}

// productOrder returns the churn products' ids in menu order
// (Churn.Cooking.js productOrder): the labels keys' order,
// resolved through Tables.ItemID, skipping any name the file
// never names.
func (u *churnUnit) productOrder() []int {
	if u.c == nil {
		return nil
	}
	var order []int
	for _, name := range churnProductOrderNames {
		if id, ok := u.c.itemID(name); ok {
			order = append(order, id)
		}
	}
	return order
}

// inputFor returns the churn input row for an input id, or nil.
func (u *churnUnit) inputFor(inputID int) *ChurnInput {
	data := u.churnData()
	if data == nil {
		return nil
	}
	for i := range data.Inputs {
		if data.Inputs[i].InputID == inputID {
			return &data.Inputs[i]
		}
	}
	return nil
}

// choices is choices (Churn.Cooking.js:33-45): each product is
// churned from the inputs carried, the most processed first
// (butter before cream before milk), moving to the next when
// one runs out; onlyInput (0 = any) limits it to the item used
// on the churn. Product id → its rows, in that order.
func (u *churnUnit) choices(player *game.Player, onlyInput int) map[int][]ChurnProduct {
	data := u.churnData()
	if data == nil || player == nil {
		return nil
	}
	inventory := player.Inventory()
	if inventory == nil {
		return nil
	}
	var carried []ChurnInput
	for _, input := range data.Inputs {
		if onlyInput != 0 && input.InputID != onlyInput {
			continue
		}
		if inventory.GetAmount(input.InputID) > 0 {
			carried = append(carried, input)
		}
	}
	chosen := make(map[int][]ChurnProduct)
	for _, productID := range u.productOrder() {
		var rows []ChurnProduct
		for _, input := range carried {
			for _, product := range data.Products {
				if product.ProductID == productID && product.InputID == input.InputID {
					rows = append(rows, product)
				}
			}
		}
		if len(rows) > 0 {
			chosen[productID] = rows
		}
	}
	return chosen
}

// nextRow is nextRow (Churn.Cooking.js:48-50): the first of
// these rows whose input the player still carries, or none.
func (u *churnUnit) nextRow(player *game.Player, rows []ChurnProduct) (ChurnProduct, bool) {
	if player == nil {
		return ChurnProduct{}, false
	}
	inventory := player.Inventory()
	if inventory == nil {
		return ChurnProduct{}, false
	}
	for _, row := range rows {
		if inventory.GetAmount(row.InputID) > 0 {
			return row, true
		}
	}
	return ChurnProduct{}, false
}

// carriedFor is carriedFor (Churn.Cooking.js:52-54): how many
// of these rows' inputs the player carries.
func (u *churnUnit) carriedFor(player *game.Player, rows []ChurnProduct) int {
	if player == nil {
		return 0
	}
	inventory := player.Inventory()
	if inventory == nil {
		return 0
	}
	total := 0
	for _, row := range rows {
		total += inventory.GetAmount(row.InputID)
	}
	return total
}

// churnMessage is churnMessage (Churn.Cooking.js:57-59): "You
// churn your milk to make butter." (rsprox captures 90, 1105);
// the other pairs follow it (Ours).
func (u *churnUnit) churnMessage(row ChurnProduct) string {
	name := row.Name
	if input := u.inputFor(row.InputID); input != nil {
		name = input.Name
	}
	return fmt.Sprintf("You churn your %s to make %s.", name, row.Name)
}

// dropGroundItem drops an item at the player's tile through
// the engine ground-item seam (game.Hooks.DropGroundItem, the
// Woodcutting precedent): a nil hook drops the item silently
// (the seam's documented gap).
func (u *churnUnit) dropGroundItem(player *game.Player, item game.Item) {
	if player == nil || game.Hooks.DropGroundItem == nil {
		return
	}
	game.Hooks.DropGroundItem(item, player.Location(), player)
}

// churnOne is churnOne (Churn.Cooking.js:67-86): one item —
// the product takes the input's slot and the milk's bucket a
// free one (rsprox capture 90). Wiki: with no room, the product
// drops to the ground instead, and the bucket stays. Go has no
// forceAdd (Phase 35 gap): the product goes to the ground via
// the DropGroundItem seam (documented deviation — the ground
// item is the Wiki outcome either way).
func (u *churnUnit) churnOne(player *game.Player, row ChurnProduct) bool {
	if player == nil {
		return false
	}
	inventory := player.Inventory()
	if inventory == nil {
		return false
	}
	slot := inventory.GetSlotForItemID(row.InputID)
	if slot < 0 {
		return false
	}
	input := u.inputFor(row.InputID)
	product := game.NewItem(row.ProductID, 1)
	if input == nil || !input.HasReturn {
		inventory.SetItem(slot, product)
	} else if inventory.GetFreeSlots() > 0 {
		inventory.SetItem(slot, product)
		inventory.AddItem(game.NewItem(input.ReturnID, 1))
	} else {
		inventory.SetItem(slot, game.NewItem(input.ReturnID, 1))
		u.dropGroundItem(player, product)
	}
	inventory.RefreshItems()
	u.c.spam(player, u.churnMessage(row))
	u.c.addXP(player, row.XP)
	u.c.emitEvent(SkillsProducedEventName, &ProducedEvent{
		Player: player,
		Skill:  game.SkillCooking,
		ItemID: row.ProductID,
		Amount: 1,
		XP:     row.XP,
		Kind:   "churn",
	})
	return true
}

// playChurn plays a row's churn animation
// (Churn.Cooking.js:88-90).
func (u *churnUnit) playChurn(player *game.Player, row ChurnProduct) {
	if player == nil {
		return
	}
	player.PerformAnimation(row.SeqID, 0)
}

// start is start (Churn.Cooking.js:99-122): churns amount —
// the animation plays on the choice, the first item comes a
// cycle less a tick later and each next one a full cycle after
// that, the animation starting again with each item that has
// another after it (rsprox capture 90: butter 18 ticks after
// the choice, then 19). TaskManager.cancelTasks(player index)
// first, as TS does.
func (u *churnUnit) start(player *game.Player, rows []ChurnProduct, amount int) {
	if player == nil || amount <= 0 {
		return
	}
	if tm := u.c.taskManager(); tm != nil {
		tm.CancelTasks(player.Index)
	}
	row, ok := u.nextRow(player, rows)
	if !ok {
		return
	}
	remaining := amount
	wait := row.Ticks - 1
	u.playChurn(player, row)
	u.c.run(player, func(tick int64) bool {
		wait--
		if wait > 0 {
			return true
		}
		actions := u.c.actionsPerCycle(player, "make")
		for done := 0; done < actions && remaining > 0; done++ {
			var ok bool
			row, ok = u.nextRow(player, rows)
			if !ok || !u.churnOne(player, row) {
				return false
			}
			remaining--
		}
		var ok bool
		row, ok = u.nextRow(player, rows)
		if remaining <= 0 || !ok {
			return false
		}
		u.playChurn(player, row)
		wait = row.Ticks
		return true
	})
}

// hasLevel is hasLevel (Churn.Cooking.js:125-131): ours — no
// capture refuses a churn for level; the line is the one the
// item-on-item recipes use.
func (u *churnUnit) hasLevel(player *game.Player, row ChurnProduct) bool {
	if u.c.cookingLevel(player) >= row.Level {
		return true
	}
	name := strings.ToLower(player.ItemName(row.ProductID))
	u.c.mesbox(player, fmt.Sprintf("You need a Cooking level of %d to make %s.", row.Level, withArticle(name, "")))
	return false
}

// offer is offer (Churn.Cooking.js:136-155): empty → "You have
// nothing to churn." (plain message); menu with product ids,
// labels as the menu labels, maxAmount/lastAmount = most
// carried; execute → hasLevel → start(min(amount, carried)).
// Captured with one and two buckets of milk: the menu opens
// either way, on the most it can make.
func (u *churnUnit) offer(player *game.Player, onlyInput int) {
	if player == nil {
		return
	}
	data := u.churnData()
	if data == nil {
		return
	}
	chosen := u.choices(player, onlyInput)
	if len(chosen) == 0 {
		player.SendMessage(nothingToChurnMessage)
		return
	}
	var items []int
	var labels []string
	most := 0
	for _, productID := range u.productOrder() {
		rows, ok := chosen[productID]
		if !ok {
			continue
		}
		items = append(items, productID)
		labels = append(labels, data.LabelsByProductID[productID])
		if n := u.carriedFor(player, rows); n > most {
			most = n
		}
	}
	menu := game.NewCreationMenu(
		churnMenuTitle,
		items,
		func(itemID, amount int) {
			rows := chosen[itemID]
			row, ok := u.nextRow(player, rows)
			if !ok || !u.hasLevel(player, row) {
				return
			}
			count := min(amount, u.carriedFor(player, rows))
			if count > 0 {
				u.start(player, rows, count)
			}
		},
		game.CreationMenuOptions{Mode: churnMenuMode, MaxAmount: &most, LastAmount: &most, Labels: labels},
	)
	u.c.sendCreationMenu(player, menu)
}

// isChurnObject is isChurn (Churn.Cooking.js:157-159): the
// object definition's name is "Dairy churn". The hub's
// EmitItemOnObject carries no Definition, so the name comes
// from the installed objectInfo lookup (the Cookers handleCook
// precedent, cookers.go); with none installed the use never
// matches.
func (u *churnUnit) isChurnObject(objectID int) bool {
	info, ok := u.c.objectInfoOf(objectID)
	return ok && info.Name == "Dairy churn"
}

// churnClick is churnClick (Churn.Cooking.js:166-172): Wiki
// (Dairy churn, Cheese) — churning is members-only. Ours: on
// a free world Churn gets the members' item line (no capture),
// as the members-only recipes do. It claims the click either
// way (a named object-interaction handler that ran).
func (u *churnUnit) churnClick(event *plugins.ObjectInteractionEvent) bool {
	if event == nil || event.Player == nil {
		return false
	}
	if !u.c.membersWorld {
		event.Player.SendMessage(FreeToPlayItemMessage)
		return true
	}
	u.offer(event.Player, 0)
	return true
}

// useOnChurn is useOnChurn (Churn.Cooking.js:174-179): only
// when the item is a churn input AND a members world; handled,
// offer(player, itemId). The members check is a silent return
// (no message), as TS does.
func (u *churnUnit) useOnChurn(event *plugins.ItemOnObjectEvent) {
	if event == nil || event.Player == nil {
		return
	}
	if !u.isChurnObject(event.ObjectID) || !u.c.membersWorld {
		return
	}
	data := u.churnData()
	if data == nil {
		return
	}
	for _, input := range data.Inputs {
		if input.InputID == event.ItemID {
			event.Handled = true
			u.offer(event.Player, event.ItemID)
			return
		}
	}
}

// registerChurn wires the Churn unit (Churn.Cooking.js
// attachChurn): onObjectInteraction("Dairy churn", {Churn}),
// onItemOnObject (noted:false).
func (c *cooking) registerChurn(api *plugins.Api) {
	if api == nil {
		return
	}
	u := newChurnUnit(c)
	api.OnObjectInteractionNamed("Dairy churn", map[string]plugins.ObjectInteractionHandler{"Churn": u.churnClick})
	api.OnItemOnObject(u.useOnChurn, &plugins.ItemUseFilter{Noted: plugins.BoolPtr(false)})
}
