// Ground-item pickup: PickupItemPacketListener.ts:5-25 and
// ItemOnGroundManager.ts (commit bfd81e3) — getOperableGroundItem
// and takeOperation (lines 104-201), canOperate (lines 88-102),
// pickup (lines 225+), transferToInventory / transferPrepared
// (lines 276-400), canInteract (lines 55-86), and
// ItemOnGround.getPickupItem (ItemOnGround.ts:170-173).
//
// The GroundItems seam below is the minimal surface the listeners
// need from the world ground-item store. *game.GroundItemManager
// implements it once the coordinator adds the Items() snapshot
// accessor (see the interface comment); listener tests use a
// faithful fake, and manager semantics are tested in the game
// package.
//
// GAPS, documented at their sites:
//   - busy(): busy() degenerates to the hitpoints term (see
//     drop.go); every other term reads state that does not exist
//     in Go. The 300ms getLastItemPickup throttle IS ported, as
//     a cycle stamp on the player (see ItemPickupThrottled /
//     RecordItemPickup): ticks are 600ms and transfers happen
//     only on ticks, so "under 300ms" is exactly "the same
//     cycle".
//   - Private-area checks: no area system exists; the checks
//     degenerate to equal areas (pass).
//   - Plugin events emitGroundItemPickup, ground-items:reward,
//     ground-items:take, ground-items:taken: no plugin system.
//     With no hooks each leaves its default payload unchanged
//     (allow=true, amount=pickedUp, discard=0, noted=false).
//   - Sounds.sendSound and playReachAnimation (facing +
//     animation): no sound, facing, or animation systems exist.
//   - The "I can't reach that!" message is sent by the walk-to
//     machinery (MovementQueue.tickWalkTo), not by pickup():
//     TS pickup() itself sends no message
//     (ItemOnGroundManager.ts:230-234 resets the queue and
//     returns). The queue reset and task stop it accompanies
//     ARE ported in the walk-to machinery.
//   - Transfer reentrancy guards (WeakSets): no plugin callbacks
//     can run during a transfer, so no reentrancy is possible
//     and the guards degenerate to nothing.
//   - The unknown-item-id inventory-capacity guard: TS credits
//     zero for an id whose definition is the DEFAULT. No
//     HasDefinition accessor exists in Go, so capacity is read
//     through GetAddableAmount for every id; unknown ids read
//     as ordinary non-stackable items.

package listeners

import (
	"tsps-go/internal/game"
)

// GroundItems is the ground-item store surface the packet
// listeners need: the register/deregister/perform/visibility API
// of *game.GroundItemManager, plus an enumeration of the live
// stacks in registration order (World.getItems() order in TS).
//
// COORDINATOR WIRING REQUIRED: *game.GroundItemManager does not
// yet export Items(). Add a snapshot accessor returning the live
// stacks in list order (a copy of the slice, so callers cannot
// mutate the manager's list), and pass the manager as
// Deps.GroundItems in handleTurnPacket. Until then, pickup and
// drop degrade to ReasonGroundItemsUnwired and the item stays
// put. See the package doc.
type GroundItems interface {
	// Register adds a stack to the world (GroundItemManager
	// .Register): merges it when a mergeable stack exists.
	Register(item *game.ItemOnGround)
	// Deregister marks a stack pending removal
	// (GroundItemManager.Deregister).
	Deregister(item *game.ItemOnGround)
	// Perform re-sends a stack with new state, e.g. after a
	// partial pickup (GroundItemManager.Perform).
	Perform(item *game.ItemOnGround, op game.GroundItemOp)
	// CanSee reports whether player can see item, excluding
	// pending-removal stacks when includePendingRemoval is
	// false (GroundItemManager.CanSee).
	CanSee(player *game.Player, item *game.ItemOnGround, includePendingRemoval bool) bool
	// Items returns the live stacks in registration order.
	// The slice is a snapshot: mutating it must not affect the
	// store. Callers must not mutate the stacks except through
	// the transfer path below (OldAmount/Amount on a partial
	// pickup, exactly as TS mutates the stack item).
	Items() []*game.ItemOnGround
}

// takeOperation returns the 1-based ground operation number for
// taking id: the index+1 of the "take" ground action when the
// action list names one, else 3 when the third action is unset,
// else 0 (no take operation). ItemOnGroundManager.ts:196-201
// (takeOperation). With the ObjType defaults the third action is
// "Take", so the operation is 3. Operation 0 means the item
// cannot be taken: IsOperationEnabled always reads false for it,
// so canOperate refuses the stack.
func takeOperation(p *game.Player, id int) int {
	actions := groundActions(actionSource(p), id)
	if index := findActionIndex(actions, "take"); index >= 0 {
		return index + 1
	}
	if actions[2] == "" {
		return 3
	}
	return 0
}

// containsStack reports whether stack is in the world's live list.
// TS checks World.getItems().includes(groundItem) (identity, not
// id equality). The lookup scans the snapshot; the manager list
// is the authority, and a stack deregistered since the snapshot
// was taken reads PendingRemoval, which callers check separately.
func containsStack(store GroundItems, stack *game.ItemOnGround) bool {
	for _, item := range store.Items() {
		if item == stack {
			return true
		}
	}
	return false
}

// canOperate reports whether player may operate stack with
// operation: the stack is in the world list, on the player's
// plane, visible to the player, and the operation is enabled for
// the player. ItemOnGroundManager.ts:88-102 (canOperate). The
// private-area term degenerates: no area system exists, so the
// stack's area always equals the player's.
func canOperate(store GroundItems, player *game.Player, stack *game.ItemOnGround, operation int) bool {
	return containsStack(store, stack) &&
		stack.Position.Z == player.Location().Z &&
		store.CanSee(player, stack, false) &&
		stack.IsOperationEnabled(player, operation)
}

// canInteract reports whether player may interact with stack via
// pickup: same plane, take operation enabled, within CrossFloor
// range 64, and visible. ItemOnGroundManager.ts:55-86
// (canInteract). CanSee already includes the CrossFloor range
// check, the area check, and the visibility-state terms, so
// given the plane and operation checks, canInteract is exactly
// CanSee — the same collapse TS performs when it ANDs
// CrossFloor.visible with canSee. The telegrab-denial branch is
// absent: pickup never passes the telegrab operation.
func canInteract(store GroundItems, player *game.Player, stack *game.ItemOnGround) bool {
	if stack == nil {
		return false
	}
	if stack.Position.Z != player.Location().Z ||
		!stack.IsOperationEnabled(player, takeOperation(player, stack.ItemID)) {
		return false
	}
	return store.CanSee(player, stack, false)
}

// getGroundItemByID returns the live stack with the world id, or
// nil. ItemOnGroundManager.ts:150-166 (getGroundItemById):
// pending-removal stacks are skipped, the private-area check
// degenerates (no areas), and a SEEN_BY_PLAYER stack owned by
// someone else is invisible to the requester.
func getGroundItemByID(store GroundItems, stackID int, player *game.Player) *game.ItemOnGround {
	for _, stack := range store.Items() {
		if stack.ID() != stackID || stack.PendingRemoval {
			continue
		}
		if stack.State == game.GroundItemSeenByPlayer && !isOwnerName(stack.Owner, player.Username) {
			return nil
		}
		return stack
	}
	return nil
}

// getOperableGroundItem returns the stack the player may operate
// for itemID at pos with operation, or nil.
// ItemOnGroundManager.ts:168-194 (getOperableGroundItem). With a
// stack id, only that stack is considered. Otherwise the
// preference order is: the player's own stack, then an unowned
// stack, then any stack — all matching id and tile and passing
// canOperate. Iteration is in registration order, as in TS
// (World.getItems().find).
func getOperableGroundItem(store GroundItems, player *game.Player, itemID int, pos game.Location, operation int, stackID int, hasStackID bool) *game.ItemOnGround {
	matches := func(stack *game.ItemOnGround) bool {
		return stack.ItemID == itemID &&
			stack.Position.Equals(pos) &&
			canOperate(store, player, stack, operation)
	}
	if hasStackID {
		stack := getGroundItemByID(store, stackID, player)
		if stack != nil && matches(stack) {
			return stack
		}
		return nil
	}
	var unowned, any *game.ItemOnGround
	for _, stack := range store.Items() {
		if !matches(stack) {
			continue
		}
		if isOwnerName(stack.Owner, player.Username) {
			return stack
		}
		if stack.Owner == "" && unowned == nil {
			unowned = stack
		}
		if any == nil {
			any = stack
		}
	}
	if unowned != nil {
		return unowned
	}
	return any
}

// pickupItem starts a pickup: it resolves the operable stack for
// the click and walks the player to it, running the pickup on
// arrival. PickupItemPacketListener.ts:5-25 (pickup). It returns
// true when a walk-to interaction started (or the arrival action
// ran immediately, when the player already stands on the tile);
// false when any gate refused. cycle is the current world process
// cycle, for the walk-to re-route throttle.
func pickupItem(p *game.Player, itemID, x, y int, stackID int, hasStackID bool, cycle int64, store GroundItems) bool {
	if !playerReady(p) || store == nil {
		return false
	}
	// requestedPosition: the clicked tile on the player's plane
	// (line 9). The packet carries no plane; TS reads the
	// player's z.
	requested := game.Location{X: x, Y: y, Z: p.Location().Z}
	operation := takeOperation(p, itemID)
	groundItem := getOperableGroundItem(store, p, itemID, requested, operation, stackID, hasStackID)
	// Id + same-tile re-checks (lines 11-14): the operable
	// lookup already enforces both, but TS re-checks after the
	// lookup and the checks are ported for shape.
	if groundItem == nil || groundItem.ItemID != itemID || !groundItem.Position.Equals(requested) {
		return false
	}
	// canOperate re-check (line 15).
	if !canOperate(store, p, groundItem, operation) {
		return false
	}
	// busy() + 300ms throttle gates (lines 16-18): busy()
	// degenerates to the hitpoints term (see drop.go), and the
	// throttle is the cycle stamp on the player (see
	// ItemPickupThrottled): a pickup already stamped this cycle
	// blocks a second one.
	if hitpoints(p) <= 0 || p.ItemPickupThrottled(cycle) {
		return false
	}

	// walkToGroundItem (line 19): the arrival callback is the
	// pickup. The position is a value copy, as TS clones the
	// location. The callback stamps the click cycle on a
	// successful transfer (see RecordItemPickup): on the
	// immediate path the click cycle IS the transfer cycle, and
	// the throttle gates the listener entry, not the arrival.
	position := groundItem.Position
	p.MovementQueue().WalkToGroundItem(position, func() { pickup(p, groundItem, store, cycle) }, cycle)
	return true
}

// pickup takes groundItem into the player's inventory. It is the
// walk-to arrival callback.
// ItemOnGroundManager.ts:225-274 (pickup). cycle is the cycle the
// pickup was requested on, stamped on a successful transfer (see
// RecordItemPickup).
func pickup(p *game.Player, groundItem *game.ItemOnGround, store GroundItems, cycle int64) {
	// Membership / pending-removal / private-area checks (lines
	// 227-229): the area check degenerates to equal areas (no
	// area system exists).
	if !containsStack(store, groundItem) || groundItem.PendingRemoval {
		return
	}
	position := groundItem.Position
	// Same-plane + reachedObj check (lines 230-234): when the
	// player is not in reach, the queue resets and the pickup
	// does not happen. ReachedObj is the 1x1 ground-item reach
	// test (on the tile or orthogonally adjacent, no wall).
	if p.Location().Z != position.Z || !p.MovementQueue().ReachedObj(position) {
		p.MovementQueue().Reset(true)
		return
	}
	// canOperate re-check (line 235).
	if !canOperate(store, p, groundItem, takeOperation(p, groundItem.ItemID)) {
		return
	}
	// emitGroundItemPickup (lines 236-238): GAP, degenerates to
	// proceed. With no hooks the emit returns handled=false
	// (PluginManager.ts:1908-1913).
	if transferToInventory(p, groundItem, store) <= 0 {
		return
	}
	// Sounds.sendSound + playReachAnimation (lines 240-244):
	// GAPs. No sound, facing, or animation systems exist.
	// getLastItemPickup reset (line 245): stamp the request
	// cycle (see RecordItemPickup).
	p.RecordItemPickup(cycle)
}

// transferToInventory moves as much of groundItem as fits into
// the player's inventory and updates or removes the stack. It
// returns the amount credited to the inventory (0 when nothing
// moved). ItemOnGroundManager.ts:276-400
// (transferToInventory / transferPrepared).
func transferToInventory(p *game.Player, groundItem *game.ItemOnGround, store GroundItems) int {
	// Reentrancy guards (lines 278-280): GAP, degenerates to
	// nothing. The guards exist because plugin callbacks run
	// during the transfer; with no plugin system no callback
	// can re-enter, so no guard is needed.
	if !containsStack(store, groundItem) || !canInteract(store, p, groundItem) {
		return 0
	}
	groundAmount := groundItem.Amount
	if groundAmount <= 0 {
		return 0
	}

	// The reward item (lines 288-296): the pickup replacement
	// when one is set, else the stack item itself, in the stack
	// amount and with the stack meta. ItemOnGround.getPickupItem
	// (ItemOnGround.ts:170-173) returns the replacement id with
	// the stack amount and meta, else the stack item. The
	// ground-items:reward plugin event (noted flag) is a GAP:
	// with no hooks noted stays false and the item is returned
	// as-is.
	reward := rewardItem(groundItem, groundAmount)

	// Inventory capacity (lines 297-303): GetAddableAmount for
	// the reward. The unknown-id DEFAULT guard degenerates (see
	// the file comment): no HasDefinition accessor exists.
	capacity := p.Inventory().GetAddableAmount(reward)
	pickedUp := min(groundAmount, capacity)
	if pickedUp <= 0 {
		p.Inventory().Full()
		return 0
	}

	// ground-items:take plugin event (lines 305-330): GAP. The
	// default payload is amount=pickedUp, discard=0, allow=true,
	// and with no hooks it is unchanged. The post-event
	// revalidation (membership, capacity, amount clamps)
	// degenerates: no callback ran, so nothing changed since
	// the checks above, and the clamped amount equals pickedUp.
	amount := pickedUp
	discard := 0

	// Credit the inventory and measure what actually landed
	// (lines 331-343): before/after matching-amount credit, so
	// a partially-absorbed add credits only what arrived.
	// matchingAmount (lines 386-390) sums inventory stacks with
	// the reward id and equal meta.
	before := matchingAmount(p, reward)
	pickedUpItem := reward.Clone()
	pickedUpItem.SetAmount(amount)
	p.Inventory().AddItem(pickedUpItem)
	after := matchingAmount(p, reward)
	credited := min(amount, max(0, after-before))
	if credited <= 0 {
		return 0
	}

	// Update the stack (lines 344-368): a full take deregisters
	// it (respawn timers run from Deregister); a partial take
	// records the old amount, decrements, and re-sends with
	// ALTER. The discarded amount is 0 (no plugin event), so
	// removed equals credited.
	removed := credited + discard
	if removed == groundAmount {
		store.Deregister(groundItem)
	} else {
		groundItem.OldAmount = groundAmount
		groundItem.Amount = groundAmount - removed
		store.Perform(groundItem, game.GroundItemAlter)
	}

	// ground-items:taken event (lines 369-374): GAP. No plugin
	// system.
	return credited
}

// rewardItem builds the item a take credits: the pickup
// replacement id when the stack carries one, else the stack id,
// in amount with the stack meta converted to item meta. See
// transferToInventory.
func rewardItem(stack *game.ItemOnGround, amount int) game.Item {
	id := stack.ItemID
	if stack.PickupReplacement != nil {
		id = stack.PickupReplacement.ID
	}
	item := game.NewItem(id, amount)
	if stack.Meta != nil {
		item.SetMeta(groundMetaToItem(stack.Meta))
	}
	return item
}

// matchingAmount sums the amounts of inventory stacks matching
// reward by id and meta. ItemOnGroundManager.ts:386-390
// (matchingAmount).
func matchingAmount(p *game.Player, reward game.Item) int {
	total := 0
	for _, held := range p.Inventory().Items() {
		if held.ID() == reward.ID() && metaFingerprint(held.Meta()) == metaFingerprint(reward.Meta()) {
			total += held.Amount()
		}
	}
	return total
}
