package game

// Shop stock and restocking: the port of the stock half of
// ShopManager.ts (restockAll, restockShop,
// stockChangeIntervalTicks, addStock, takeStock, removeStock,
// ensureRestockTask) plus the ShopRestockTask cadence.
//
// SCHEDULING. TS runs restocking as a TaskManager task:
// ShopRestockTask (delay 1, i.e. every tick) executes
// restockAll, and stops itself when nothing is pending;
// ensureRestockTask submits it whenever a trade or a stock add
// leaves a shop out of balance. Go has no TaskManager (see
// doc.go: timed systems advance from the tick through
// coordinator-installed hooks, as the ground-item manager does
// behind WorldHooks.GroundItems). The manager therefore exposes
// Tick, which the coordinator calls once per game tick: it runs
// a restock pass only while the restock task is "running", and
// the pass itself clears the flag when nothing is pending —
// the same start/stop semantics, driven from the tick instead
// of a task queue. RestockAll is also exported for direct use
// (it is public in TS); calling it runs a pass unconditionally.

// Tick runs one restock pass when the restock task is running,
// and no-ops otherwise (ShopRestockTask.execute: restockAll,
// stopping when it reports nothing pending). The coordinator
// calls it once per game tick.
func (m *ShopManager) Tick() {
	if !m.restockTaskRunning {
		return
	}
	m.RestockAll()
}

// RestockTaskRunning reports whether the restock task is
// running — whether a future Tick would do work. The
// coordinator may skip calling Tick when it is false, though
// calling Tick is equally cheap.
func (m *ShopManager) RestockTaskRunning() bool {
	return m.restockTaskRunning
}

// RestockAll ports ShopManager.restockAll (ShopManager.ts): one
// restock pass over every shop, restocking a shared stock once
// with its owner. It reports whether any shop is still out of
// balance (pending); when nothing is pending the restock task
// stops. Viewers of a changed shop are refreshed by the pass.
func (m *ShopManager) RestockAll() bool {
	pending := false
	for _, id := range m.shopOrder {
		shop := m.shops[id]
		// A shared stock restocks once, with its owner.
		if !shop.sharesStock && m.restockShop(shop) {
			pending = true
		}
	}
	if !pending {
		m.restockTaskRunning = false
	}
	return pending
}

// restockShop ports ShopManager.restockShop (ShopManager.ts):
// step one shop's stock toward its original amounts, one unit
// per item per interval. It reports whether the shop is still
// out of balance (pending), and refreshes viewers when anything
// changed. Each out-of-balance item carries a countdown timer
// in changeTimers, seeded with its interval; the timer ticks
// down once per pass and fires at zero, then reseeds — so an
// item 1 below stock with interval 4 returns after 4 passes.
//
// Iteration order is the display order, then any original-stock
// id missing from it (unreachable in practice: original ids
// are never removed from the order — see removeStock — so the
// union TS builds from order + originalAmounts keys is exactly
// the order; the second loop only guards the invariant).
func (m *ShopManager) restockShop(shop *runtimeShop) bool {
	state := shop.state
	seen := make(map[int]bool, len(state.order))
	var itemIDs []int
	for _, itemID := range state.order {
		seen[itemID] = true
		itemIDs = append(itemIDs, itemID)
	}
	for itemID := range state.originalAmounts {
		if !seen[itemID] {
			itemIDs = append(itemIDs, itemID)
		}
	}
	changed := false
	pending := false
	for _, itemID := range itemIDs {
		original := state.originalAmounts[itemID]
		current := state.stock[itemID]
		decrease := current > original
		increase := current < original
		if !decrease && !increase {
			delete(state.changeTimers, itemID)
			continue
		}

		pending = true
		interval := m.stockChangeIntervalTicks(shop, itemID, decrease)
		timer, ok := state.changeTimers[itemID]
		if !ok || timer <= 0 {
			timer = interval
		}
		timer--
		if timer <= 0 {
			if decrease {
				if m.removeStock(shop, itemID, 1) > 0 {
					changed = true
				}
			} else if m.addStock(shop, itemID, 1) {
				changed = true
			}
			timer = interval
		}

		next := state.stock[itemID]
		if (decrease && next > original) || (increase && next < original) {
			state.changeTimers[itemID] = timer
		} else {
			delete(state.changeTimers, itemID)
		}
	}
	if changed {
		m.refresh(shop.definition.ID)
	}
	return pending
}

// stockChangeIntervalTicks ports ShopManager.
// stockChangeIntervalTicks (ShopManager.ts): the destock
// interval for a sold-in item in a shop that deletes items
// (a general store), else the item's configured restock ticks,
// else the shop-wide destock/restock default for the direction.
// The interval is at least 1 tick. Note the per-item ticks
// apply to BOTH directions in TS, despite the field name.
func (m *ShopManager) stockChangeIntervalTicks(shop *runtimeShop, itemID int, decrease bool) int {
	if decrease && m.deletesItems(shop) {
		if _, original := shop.state.originalAmounts[itemID]; !original {
			if ticks := shop.definition.SoldItemDestockTicks; ticks > 1 {
				return ticks
			}
			return 1
		}
	}
	if ticks, ok := shop.state.itemRestockTicks[itemID]; ok {
		if ticks > 1 {
			return ticks
		}
		return 1
	}
	configured := shop.definition.DefaultRestockTicks
	if decrease {
		configured = shop.definition.DefaultDestockTicks
	}
	if configured > 1 {
		return configured
	}
	return 1
}

// addStock ports ShopManager.addStock (ShopManager.ts): put
// amount of an item into a shop's stock. A non-positive amount
// adds nothing; a new item is refused when the shop already
// holds MAX_SHOP_ITEMS distinct items. New items append to the
// display order; the pile caps at Number.MAX_SAFE_INTEGER.
func (m *ShopManager) addStock(shop *runtimeShop, itemID, amount int) bool {
	quantity := normalizeAmount(amount)
	if quantity <= 0 {
		return false
	}
	state := shop.state
	current := state.stock[itemID]
	if current <= 0 && len(state.order) >= MaxShopItems {
		return false
	}
	found := false
	for _, id := range state.order {
		if id == itemID {
			found = true
			break
		}
	}
	if !found {
		state.order = append(state.order, itemID)
	}
	total := current + quantity
	if total > maxSafeInteger {
		total = maxSafeInteger
	}
	state.stock[itemID] = total
	return true
}

// takeStock ports ShopManager.takeStock (ShopManager.ts): take
// a purchase out of the stock. Content may keep the shop's own
// stock (shop:keep-stock): then only stock above the shop's own
// amount (what players sold it) goes, but the returned taken
// count still covers the whole purchase — the player paid for
// all of it. Otherwise the full amount is removed. It returns
// how many were taken (the purchase quantity, capped at the
// stock on hand).
func (m *ShopManager) takeStock(player *Player, shop *runtimeShop, itemID, amount int) int {
	keep := false
	if m.Hooks.OnKeepStock != nil {
		request := &ShopKeepStockEvent{
			Player: player, ShopID: shop.definition.ID,
			ItemID: itemID, Amount: amount, Keep: false,
		}
		m.Hooks.OnKeepStock(request)
		keep = request.Keep
	}
	if !keep {
		return m.removeStock(shop, itemID, amount)
	}
	current := shop.state.stock[itemID]
	taken := current
	if quantity := normalizeAmount(amount); quantity < taken {
		taken = quantity
	}
	surplus := current - shop.state.originalAmounts[itemID]
	if surplus < 0 {
		surplus = 0
	}
	if removable := minInt(taken, surplus); removable > 0 {
		m.removeStock(shop, itemID, removable)
	}
	return taken
}

// removeStock ports ShopManager.removeStock (ShopManager.ts):
// remove up to amount of an item from a shop's stock, returning
// how many were removed. An emptied pile leaves the stock map;
// a sold-in item (not in the original stock) also leaves the
// display order, spliced in place so versions sharing the stock
// see the same order.
func (m *ShopManager) removeStock(shop *runtimeShop, itemID, amount int) int {
	quantity := normalizeAmount(amount)
	state := shop.state
	current := state.stock[itemID]
	removed := current
	if quantity < removed {
		removed = quantity
	}
	if removed <= 0 {
		return 0
	}
	remaining := current - removed
	if remaining <= 0 {
		delete(state.stock, itemID)
		if _, original := state.originalAmounts[itemID]; !original {
			for slot, id := range state.order {
				if id == itemID {
					state.order = append(state.order[:slot], state.order[slot+1:]...)
					break
				}
			}
		}
	} else {
		state.stock[itemID] = remaining
	}
	return removed
}

// ensureRestockTask ports ShopManager.ensureRestockTask
// (ShopManager.ts): start the restock task if it is not already
// running. In Go "running" is the flag Tick consults (see the
// file header); no task is submitted anywhere.
func (m *ShopManager) ensureRestockTask() {
	m.restockTaskRunning = true
}

// minInt is the two-argument integer minimum.
func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
