// MagicOnItemPacketListener: spell on an inventory item
// (castOnItem / castAlchemy, MagicOnItemPacketListener.ts:116-164,
// 264-464) and spell on a ground item (castGroundItem /
// landGroundItemGrab, lines 166-263), plus the widget_target
// spell-on-item route (NetworkBuilder.ts:1173-1216).
//
// Implemented (phase 27): Low/High Alchemy and Telekinetic Grab
// on the NORMAL spellbook — levels, rune costs with the bypass
// gate and infinite staves, the spell-disabled gate, alchemy
// pacing + queueing, coin payment, Magic XP, and the telegrab's
// delayed landing through the task seam. Other EffectSpells
// (Superheat Item, Bones to Bananas, Bones to Peaches, Bake Pie,
// Vengeance, Vengeance Other) and the Arceuus reanimations route
// to documented NOT STARTED handlers: castOnItem returns false
// for them and the click falls through to the interface-action
// handle, exactly as TS falls through when MAGIC_ITEMS returns
// false.
//
// Gaps, documented at their sites:
//   - Cast animations (712/713), graphics (112/113, 142/144),
//     the telegrab projectile (143), and the alchemy/telegrab
//     sounds: no animation/graphic/projectile/sound producers
//     exist in Go (Phase 29). The casts pay and grant XP
//     without them.
//   - The 500ms click delay on telegrab: no click-delay system
//     exists in Go.
//   - Spellbook Swap end (endSpellbookSwap): no spellbook-swap
//     state exists in Go (Worker C owns the spellbook).
//   - The alchemy confirm dialogue for untradeable items: the
//     warning message is sent, but the ItemStatement + Option
//     dialogue that asks "Really cast ... on it?" has no
//     listener-side seam, so the cast does not proceed —
//     fail-closed rather than alching past the warning.
//   - Alchemy's sendTab(6) after a cast: no tab-send seam in
//     the listeners.
//   - The queued-alchemy fire-time input-prompt term: no
//     input-prompt seam in the listeners (the dialogue and
//     interruptible-interface terms are ported).
//   - The rune pouch (checkSpellRuneSource /
//     consumeSpellRuneSource): no source exists until Worker C
//     wires it; the inventory must cover the whole cost.
//   - Cross-floor telegrab landing: the landing transfer runs
//     through the pickup-shaped transferToInventory, which
//     requires the same plane; a cross-floor grab fizzles at
//     the floor check first (TS's own floor rule), so the
//     transfer gap is unreachable in practice.
//   - Spell-on-object (opcode 2,
//     UseItemPacketListener.spellOnObject): NOT STARTED — it
//     needs the onSpellOnObject emit family, which no worker
//     owns this phase; the walk-to-object half alone would be
//     a walk to nowhere. Documented as a gap, not ported.

package listeners

import (
	"strconv"

	"tsps-go/internal/game"
	"tsps-go/internal/protocol"
)

// Alchemy constants (MagicOnItemPacketListener.ts:33-54):
// LOW_ALCH_SPELL_ID 1162, HIGH_ALCH_SPELL_ID 1178, levels 21/55,
// XP 31/65, fire runes 3/5, cast every 3/5 ticks, animations
// 712/713, graphics 112/113. The EffectSpells table's own XP
// figures (LOW_ALCHEMY 4000, HIGH_ALCHEMY 20000,
// EffectSpells.ts:151-182) are NOT used here: the listener's
// constants are the ones that pay XP, and the table figures
// disagree with them — Worker A owns that discrepancy.
const (
	alchLowLevel      = 21
	alchHighLevel     = 55
	alchLowXP         = 31
	alchHighXP        = 65
	alchLowFire       = 3
	alchHighFire      = 5
	alchLowCastTicks  = 3
	alchHighCastTicks = 5
)

// Telegrab constants (MagicOnItemPacketListener.ts:55-64):
// TELEKINETIC_GRAB_SPELL_ID 1168, level 33, XP 43, range 10,
// animation 723, graphics 142/144, projectile 143, 80 client
// cycles (20ms) to land — ceil(80/30) = 3 ticks on the task
// manager. Attribute names are the TS ones
// (ALCH_NEXT_CAST_ATTRIBUTE / ALCH_QUEUED_ATTRIBUTE).
const (
	telegrabLevel        = 33
	telegrabXP           = 43
	telegrabRange        = 10
	telegrabLandTicks    = 3
	telegrabVisibleRange = 64
	alchemyNextCastAttr  = "magic:alchemy-next-cast"
	alchemyQueuedAttr    = "magic:alchemy-queued"
)

// AlchemyWarningEvent is the "magic:alchemy-warning" custom-event
// payload (MagicOnItemPacketListener.ts:305-308): the item, the
// spell, and the warning text — "" when the item is tradeable
// (no warning). Content may change Text; a non-empty Text asks
// first.
type AlchemyWarningEvent struct {
	Player  *game.Player
	Item    game.Item
	ItemID  int
	SpellID int
	High    bool
	Text    string
}

// AlchemyPolicyEvent is the "magic:alchemy-policy" custom-event
// payload (MagicOnItemPacketListener.ts:316-324): content may
// lower the level, waive the runes, keep the item, cast again,
// or change the coins, pace, and confirmation.
type AlchemyPolicyEvent struct {
	Player        *game.Player
	ItemID        int
	SpellID       int
	High          bool
	Allowed       bool
	Coins         int
	CastTicks     int
	Confirm       bool
	RequiredLevel int
	FreeRunes     bool
	KeepItem      bool
	Recast        bool
}

// AlchemyPaceEvent is the "skills:action-interval" custom-event
// payload for alchemy (MagicOnItemPacketListener.ts:326-329):
// content may change the cast pace (Ticks).
type AlchemyPaceEvent struct {
	Player *game.Player
	Skill  int
	Kind   string
	Ticks  int
}

// AlchemyPaymentEvent is the "magic:alchemy-payment"
// custom-event payload (MagicOnItemPacketListener.ts:334-343):
// content may fund the cast another way (Free), change the
// payment source, or veto it (Allowed). Validate runs before
// payment commits; Commit runs after the coins land.
type AlchemyPaymentEvent struct {
	Player            *game.Player
	Item              game.Item
	ItemID            int
	Slot              int
	SpellID           int
	High              bool
	Coins             int
	HasRunes          bool
	Allowed           bool
	Free              bool
	Experience        int
	AllowAnySpellbook bool
	KeepTab           bool
	Validate          func() bool
	Commit            func()
}

// ItemSpellCastEvent is the "magic:item-spell-cast" custom-event
// payload (MagicOnItemPacketListener.ts:381-383): an alchemy
// cast on an item. The spell is carried as its id: no
// EffectSpells object exists in Go yet (Worker A ports it).
type ItemSpellCastEvent struct {
	Player  *game.Player
	SpellID int
}

// AlchemyCompletedEvent is the "magic:alchemy-completed"
// custom-event payload (MagicOnItemPacketListener.ts:393): what
// the cast paid.
type AlchemyCompletedEvent struct {
	Player  *game.Player
	ItemID  int
	SpellID int
	Coins   int
}

// TelekineticGrabCheckEvent is the "magic:can-telekinetic-grab"
// custom-event payload (MagicOnItemPacketListener.ts:225-231):
// content may refuse the grab by setting Allow false.
type TelekineticGrabCheckEvent struct {
	Player     *game.Player
	GroundItem *game.ItemOnGround
	Allow      bool
}

// TelekineticGrabCastEvent is the "magic:telekinetic-grab-cast"
// custom-event payload (MagicOnItemPacketListener.ts:258): the
// cast itself, before the item comes away.
type TelekineticGrabCastEvent struct {
	Player     *game.Player
	GroundItem *game.ItemOnGround
	ItemID     int
	Position   game.Location
}

// TelekineticGrabEvent is the "magic:telekinetic-grab"
// custom-event payload (MagicOnItemPacketListener.ts:259-262):
// the projectile landed and the item came away.
type TelekineticGrabEvent struct {
	Player   *game.Player
	ItemID   int
	Position game.Location
}

// queuedAlchemyCast is one queued alchemy cast, stored under the
// "magic:alchemy-queued" attribute until its task fires
// (MagicOnItemPacketListener.ts:397-423).
type queuedAlchemyCast struct {
	SpellID   int
	Slot      int
	ItemID    int
	Item      game.Item
	Automatic bool
}

// isAlchSpell reports whether spellID is an alchemy spell
// (MagicOnItemPacketListener.ts:264-269).
func isAlchSpell(spellID int) bool {
	return spellID == spellLowAlchemyID || spellID == spellHighAlchemyID
}

// isReanimationSpell reports whether spellID is an Arceuus
// reanimation (ArceuusItemSpells.ts:33-36): its handler is
// Arceuus content, NOT STARTED.
func isReanimationSpell(spellID int) bool {
	switch spellID {
	case spellBasicReanimationID, spellAdeptReanimationID,
		spellExpertReanimationID, spellMasterReanimationID:
		return true
	default:
		return false
	}
}

// ItemAlchValueSource is the alchemy-value surface the listeners
// read: the high/low alchemy values of an item
// (ItemDefinition.ts:208-214, getHighAlchValue /
// getLowAlchValue). It is separate from ItemDefinitionSource
// because only alchemy reads it, and the production adapter
// implements it through game.ShopItemDefinitionSource. A source
// that does not implement it reads 0 for every id, and the cast
// refuses — the fail-closed answer for an item with no
// definition.
type ItemAlchValueSource interface {
	// ItemHighAlchValue returns the high alchemy value.
	ItemHighAlchValue(id int) int
	// ItemLowAlchValue returns the low alchemy value.
	ItemLowAlchValue(id int) int
}

// alchValues reads the high/low alchemy values for id through
// the definition source's value surface (see
// ItemAlchValueSource). ok is false when no value source is
// installed: the values read 0 and the cast refuses, the
// fail-closed answer for an item with no definition.
func alchValues(p *game.Player, id int) (high, low int, ok bool) {
	src, isSrc := defs(p).(ItemAlchValueSource)
	if !isSrc || src == nil {
		return 0, 0, false
	}
	return src.ItemHighAlchValue(id), src.ItemLowAlchValue(id), true
}

// routeSpellOnGround routes one decoded spell_on_ground message
// (opcode 94). A nil player or an unwired ground-item store
// drops with ReasonMagicOnItem; a refused cast returns
// noAction; a started cast or walk counts as acted.
func routeSpellOnGround(p *game.Player, msg protocol.SpellOnGround, cycle int64, deps Deps, drop func(DropReason) Outcome, acted, noAction func() Outcome) Outcome {
	if p == nil {
		return drop(ReasonMagicOnItem)
	}
	if !playerReady(p) {
		return noAction()
	}
	if deps.GroundItems == nil {
		return drop(ReasonMagicOnItem)
	}
	if castGroundItem(p, msg.GroundItemID, msg.X, msg.Y, resolveSpellID(deps, int(msg.SpellWidget), msg.SpellChild, msg.SpellItemID), cycle, deps) {
		return acted()
	}
	return noAction()
}

// castSpellOnItem routes one decoded widget_target message's
// spell-on-item half (NetworkBuilder.ts:1182-1204): Lunar
// handling, the emitSpellOnItem hook family, then
// MAGIC_ITEMS.castOnItem. It reports whether the spell route
// claimed the click. Only inventory targets reach here — the
// caller gates on the target widget group. LunarSpells and
// emitSpellOnItem have no Go form yet, so both read false and
// the cast is the only live route.
func castSpellOnItem(p *game.Player, msg protocol.WidgetTarget, cycle int64, deps Deps) bool {
	// LunarSpells.handleItemTarget: GAP, reads false (Lunar
	// is out of scope).
	// PluginManager.emitSpellOnItem: GAP, reads false (the
	// onSpellOnItem hook family is NOT STARTED).
	spellID := game.ResolveClientCombatSpellID(
		int(msg.SourceWidgetID), int(msg.SourceWidgetID)&0xffff, msg.SourceItemID,
		deps.SpellHooks.SpellName,
	)
	return castOnItem(p, spellID, msg.TargetItemID, msg.TargetSlot, cycle, deps)
}

// castOnItem ports MagicOnItemPacketListener.castOnItem
// (MagicOnItemPacketListener.ts:116-164): the emitCanUseItem
// gate, the Arceuus reanimations, the slot/id gates, then
// castAlchemy. It reports whether the cast was taken: true
// means the click is consumed (even when the cast itself then
// refuses with a message), false means the click falls through
// to the interface-action handle.
func castOnItem(p *game.Player, spellID, itemID, slot int, cycle int64, deps Deps) bool {
	// PluginManager.emitCanUseItem(player, itemId, "magic"):
	// GAP, reads as allowed — no can-use seam exists in the
	// listeners (see the file header).
	if isReanimationSpell(spellID) {
		// ArceuusItemSpells.castOnItem: NOT STARTED.
		return false
	}
	if !playerReady(p) || slot < 0 || slot >= p.Inventory().Capacity() || !isAlchSpell(spellID) {
		return false
	}
	item := p.Inventory().ItemAt(slot)
	if !item.IsValid() || item.ID() != itemID {
		return false
	}
	castAlchemy(p, spellID, slot, itemID, false, cycle, deps)
	return true
}

// castAlchemy ports MagicOnItemPacketListener.castAlchemy
// (MagicOnItemPacketListener.ts:264-395): one alchemy cast on
// the item in slot. confirmed=true skips the warning/policy
// re-check (a queued or confirmed re-cast). cycle is the
// current world process cycle, for the cast pacing.
func castAlchemy(p *game.Player, spellID, slot, itemID int, confirmed bool, cycle int64, deps Deps) {
	inv := p.Inventory()
	clickedItem := inv.ItemAt(slot)
	if !clickedItem.IsValid() || clickedItem.ID() != itemID {
		return
	}
	// Alchemy takes the first equivalent copy in inventory
	// order (lines 275-278): the same id and meta as the
	// clicked item.
	alchemySlot := -1
	for i, held := range inv.Items() {
		if held.IsValid() && sameStack(held, clickedItem) {
			alchemySlot = i
			break
		}
	}
	if alchemySlot < 0 {
		return
	}
	item := inv.Items()[alchemySlot]
	isHigh := spellID == spellHighAlchemyID
	requiredLevel := alchLowLevel
	fireRunes := alchLowFire
	experience := alchLowXP
	castTicks := alchLowCastTicks
	if isHigh {
		requiredLevel = alchHighLevel
		fireRunes = alchHighFire
		experience = alchHighXP
		castTicks = alchHighCastTicks
	}
	highAlch, lowAlch, _ := alchValues(p, itemID)
	coins := lowAlch
	if isHigh {
		coins = highAlch
	}
	tradeable := false
	if src := defs(p); src != nil {
		tradeable = src.IsTradeable(itemID)
	}
	// Warning event (lines 305-308): an untradeable item
	// warns; a tradeable one carries no text.
	warning := &AlchemyWarningEvent{Player: p, Item: item, ItemID: itemID, SpellID: spellID, High: isHigh}
	if !tradeable {
		warning.Text = "That item cannot be traded."
	}
	if !confirmed {
		emitCustom(p, "magic:alchemy-warning", warning)
	}
	// Policy event (lines 316-324): content may change the
	// level, runes, coins, pace, and confirmation. The
	// allowed rule is verbatim: not untradeable (by the
	// item's own flag), and both alchemy values above 0.
	policy := &AlchemyPolicyEvent{
		Player: p, ItemID: itemID, SpellID: spellID, High: isHigh,
		Allowed:       !item.IsUntradeable() && highAlch > 0 && lowAlch > 0,
		Coins:         coins,
		CastTicks:     castTicks,
		Confirm:       warning.Text != "",
		RequiredLevel: requiredLevel,
	}
	emitCustom(p, "magic:alchemy-policy", policy)
	// Pace event (lines 326-329): content may change the
	// cast pace; the pace never drops below 1 tick.
	pace := &AlchemyPaceEvent{Player: p, Skill: game.SkillMagic, Kind: "alchemy", Ticks: policy.CastTicks}
	emitCustom(p, "skills:action-interval", pace)
	policy.CastTicks = pace.Ticks
	if policy.CastTicks < 1 {
		policy.CastTicks = 1
	}
	if p.Skills().CurrentLevel(game.SkillMagic) < policy.RequiredLevel {
		p.SendMessage("You need a Magic level of " + strconv.Itoa(policy.RequiredLevel) + " to cast this spell.")
		return
	}
	// Zone rules ride the spell-disabled gate (line 331).
	if deps.SpellHooks.SpellDisabled != nil && deps.SpellHooks.SpellDisabled(p, spellID, nil) {
		return
	}
	if itemID == itemCoinsID || !policy.Allowed || policy.Coins < 0 {
		p.SendMessage("This spell can not be cast on this item.")
		return
	}
	runes := []game.Item{}
	if !policy.FreeRunes {
		runes = runeCosts(deps, p, spellID, []game.Item{
			game.NewItem(runeNatureID, 1),
			game.NewItem(runeFireID, fireRunes),
		})
	}
	// Payment event (lines 334-343): content may fund the
	// cast another way, change the source, or veto it. No
	// payment commits until the item converts.
	payment := &AlchemyPaymentEvent{
		Player: p, Item: item, ItemID: itemID, Slot: alchemySlot, SpellID: spellID,
		High: isHigh, Coins: policy.Coins, HasRunes: hasRunes(deps, p, runes),
		Allowed: true, Experience: experience,
	}
	emitCustom(p, "magic:alchemy-payment", payment)
	if !payment.Allowed || (payment.Validate != nil && !payment.Validate()) {
		return
	}
	if !payment.Free && !payment.HasRunes {
		p.SendMessage("You do not have the required items to cast this spell.")
		return
	}
	if !payment.AllowAnySpellbook && !isNormalSpellbook(deps, p) {
		return
	}
	// Coin space (lines 348-362): an existing matching coin
	// stack cannot overflow even when other slots are free; a
	// new stack may use a slot released by the item or spent
	// runes.
	if policy.Coins > 0 {
		coinStack := game.NewItem(itemCoinsID, policy.Coins)
		matchingCoins := false
		for _, held := range inv.Items() {
			if held.IsValid() && sameStack(held, coinStack) {
				matchingCoins = true
				break
			}
		}
		fitsNow := inv.GetAddableAmount(coinStack) == policy.Coins
		freesItemSlot := !policy.KeepItem && item.Amount() == 1
		freesRuneSlot := false
		if !payment.Free {
			for _, rune := range runes {
				if held := inv.GetAmount(rune.ID()); held > 0 && held <= rune.Amount() {
					freesRuneSlot = true
					break
				}
			}
		}
		if !fitsNow && (matchingCoins || policy.Coins > game.MaxStackAmount || !(freesItemSlot || freesRuneSlot)) {
			p.SendMessage("You don't have enough inventory space.")
			return
		}
	}
	if !confirmed && policy.Confirm {
		// The confirm dialogue (lines 425-443,
		// ItemStatement + Option) has no listener-side seam:
		// the warning message is sent and the cast does not
		// proceed — fail-closed rather than alching past the
		// warning. See the file header.
		spellName := "Low Level Alchemy"
		if isHigh {
			spellName = "High Level Alchemy"
		}
		p.SendMessage(warning.Text + " Really cast " + spellName + " on it?")
		return
	}
	// Pacing (lines 364-368): a cast clicked before the last
	// one finishes waits for it.
	now := cycle
	nextCast := attrInt(p, alchemyNextCastAttr)
	if now < nextCast {
		queueAlchemy(p, queuedAlchemyCast{SpellID: spellID, Slot: slot, ItemID: itemID, Item: clickedItem}, int(nextCast-now), deps)
		return
	}
	// Slot re-validation (lines 370-372): the clicked item
	// must still be where the cast checked it.
	if !sameStack(inv.ItemAt(slot), clickedItem) || !sameStack(inv.ItemAt(alchemySlot), item) ||
		(payment.Validate != nil && !payment.Validate()) {
		return
	}
	if !policy.KeepItem {
		inv.DeleteAtSlot(alchemySlot, 1, true)
	}
	if !payment.Free {
		consumeRunes(deps, p, runes)
	}
	// Spell.endSpellbookSwap: GAP, no spellbook-swap state in
	// Go (Worker C owns the spellbook).
	p.SetAttribute(alchemyNextCastAttr, now+int64(policy.CastTicks))
	emitCustom(p, "magic:item-spell-cast", &ItemSpellCastEvent{Player: p, SpellID: spellID})
	// Animation (712/713), graphic (112/113), and sound: GAPs
	// (Phase 29). The cast pays and grants XP without them.
	if policy.Coins > 0 {
		inv.Adds(itemCoinsID, policy.Coins)
	}
	if payment.Commit != nil {
		payment.Commit()
	}
	emitCustom(p, "magic:alchemy-completed", &AlchemyCompletedEvent{Player: p, ItemID: itemID, SpellID: spellID, Coins: policy.Coins})
	if payment.Experience > 0 {
		p.Skills().AddExperiences(game.SkillMagic, payment.Experience)
	}
	// sendTab(6): GAP, no tab-send seam in the listeners.
	if policy.Recast {
		if held := inv.ItemAt(slot); held.IsValid() && held.ID() == itemID {
			queueAlchemy(p, queuedAlchemyCast{SpellID: spellID, Slot: slot, ItemID: itemID, Item: held, Automatic: true}, policy.CastTicks, deps)
		}
	}
}

// queueAlchemy queues one alchemy cast for when the current one
// ends (MagicOnItemPacketListener.ts:397-423): only one waits
// at a time, and a later click replaces the waiting one. The
// task fires the cast; a dialogue, menu, or prompt opened while
// it waited drops it (an automatic re-cast carries on).
func queueAlchemy(p *game.Player, cast queuedAlchemyCast, ticks int, deps Deps) {
	if deps.SpellHooks.ScheduleTask == nil {
		// No task seam: the queued cast can never fire, so
		// the click is dropped instead of queuing forever.
		return
	}
	waiting := p.GetAttribute(alchemyQueuedAttr)
	p.SetAttribute(alchemyQueuedAttr, cast)
	if waiting != nil {
		return
	}
	task := game.NewTask(ticks, func(cycle int64) bool {
		queued, ok := p.GetAttribute(alchemyQueuedAttr).(queuedAlchemyCast)
		p.SetAttribute(alchemyQueuedAttr, nil)
		if !ok || !p.IsRegistered() || hitpoints(p) <= 0 {
			return true
		}
		if !sameStack(p.Inventory().ItemAt(queued.Slot), queued.Item) {
			return true
		}
		if !queued.Automatic {
			dm := p.GetDialogueManager()
			if (dm != nil && dm.IsActive()) ||
				(deps.InterfaceSends.HasInterruptibleInterface != nil &&
					deps.InterfaceSends.HasInterruptibleInterface(p)) {
				return true
			}
			// The input-prompt term has no seam (see the
			// file header).
		}
		castAlchemy(p, queued.SpellID, queued.Slot, queued.ItemID, true, cycle, deps)
		return true
	})
	task.Name = "alchemy-queue"
	deps.SpellHooks.ScheduleTask(task)
}

// castGroundItem ports MagicOnItemPacketListener.castGroundItem
// (MagicOnItemPacketListener.ts:166-215): Telekinetic Grab on a
// ground item. It reports whether the cast was taken: true
// means the click is consumed (a walk started, or the
// projectile is flying), false means nothing happened. The item
// comes away when the projectile lands, not on the cast.
func castGroundItem(p *game.Player, groundItemID, x, y, spellID int, cycle int64, deps Deps) bool {
	if spellID != spellTelegrabID {
		return false
	}
	if !playerReady(p) || hitpoints(p) <= 0 {
		return false
	}
	// A standard-book spell, as alchemy checks: the packet
	// names the spell id, whatever book the player is on.
	if !isNormalSpellbook(deps, p) {
		return false
	}
	// The 500ms click delay (line 179): GAP, no click-delay
	// system exists in Go.
	if p.Skills().CurrentLevel(game.SkillMagic) < telegrabLevel {
		p.SendMessage("You need a Magic level of " + strconv.Itoa(telegrabLevel) + " to cast this spell.")
		return false
	}
	runes := runeCosts(deps, p, spellID, []game.Item{
		game.NewItem(runeLawID, 1),
		game.NewItem(runeAirID, 1),
	})
	if !hasRunes(deps, p, runes) {
		p.SendMessage(missingItemsMessage(p, inventoryShortfall(p, runes)))
		return false
	}
	groundItem := findVisibleGroundItem(deps.GroundItems, p, groundItemID, x, y, -1)
	if groundItem == nil {
		p.SendMessage("Nothing interesting happens.")
		return false
	}
	position := groundItem.Position
	if !withinTelegrabRange(p, position) {
		// Out of approach distance: walk in and resolve the
		// cast on arrival, rather than rejecting the click
		// outright (lines 196-208).
		approachTile := game.Location{X: x, Y: y, Z: p.Location().Z}
		p.MovementQueue().WalkToTile(approachTile, func() {
			castGroundItem(p, groundItemID, x, y, spellID, cycle, deps)
		}, func(dest game.Location) bool {
			target := findVisibleGroundItem(deps.GroundItems, p, groundItemID, x, y, -1)
			return target != nil && withinTelegrabRange(p, target.Position)
		}, true, cycle)
		return true
	}
	if !canGrabGroundItem(p, groundItem) {
		return false
	}
	if p.Inventory().GetAddableAmount(rewardItem(groundItem, groundItem.Amount)) <= 0 {
		p.Inventory().Full()
		return false
	}
	if deps.SpellHooks.ScheduleTask == nil {
		// No task seam: the grab can never land, so the cast
		// is refused before any rune is spent.
		return false
	}
	consumeRunes(deps, p, runes)
	// Spell.endSpellbookSwap: GAP (see castAlchemy).
	p.Skills().StopSkillable()
	// Facing, animation (723), graphics (142/144), the
	// projectile (143), and the sound: GAPs (Phase 29).
	// The click-delay reset: GAP, no click-delay system.
	emitCustom(p, "magic:telekinetic-grab-cast", &TelekineticGrabCastEvent{
		Player: p, GroundItem: groundItem, ItemID: groundItemID, Position: position,
	})
	// The item comes away when the projectile lands, not on
	// the cast (lines 216-224): the task is not keyed to the
	// player, so walking off or clicking does not cancel a
	// grab already flying.
	casterFloor := p.Location().Z
	task := game.NewTask(telegrabLandTicks, func(cycle int64) bool {
		landGroundItemGrab(p, groundItemID, position, casterFloor, deps)
		return true
	})
	task.Name = "telegrab-land"
	deps.SpellHooks.ScheduleTask(task)
	return true
}

// canGrabGroundItem is the generic permission query shared by
// cast and arrival checks
// (MagicOnItemPacketListener.ts:226-233): a pickup-replacement
// denial message, then the magic:can-telekinetic-grab custom
// event.
func canGrabGroundItem(p *game.Player, groundItem *game.ItemOnGround) bool {
	if groundItem.PickupReplacement != nil && groundItem.PickupReplacement.TelegrabDenial != "" {
		p.SendMessage(groundItem.PickupReplacement.TelegrabDenial)
		return false
	}
	ev := &TelekineticGrabCheckEvent{Player: p, GroundItem: groundItem, Allow: true}
	emitCustom(p, "magic:can-telekinetic-grab", ev)
	return ev.Allow
}

// withinTelegrabRange ports
// MagicOnItemPacketListener.withinTelegrabRange (lines 235-242):
// CombatRange.withinApproachDistance on the same plane, else
// CrossFloor.visible. Go has no footprint-aware approach
// distance or CrossFloor policy, so both terms are the
// same-plane Chebyshev test (see visibility.go:
// crossFloorVisible).
func withinTelegrabRange(p *game.Player, position game.Location) bool {
	return p.Location().IsWithinDistance(position, telegrabRange)
}

// landGroundItemGrab ports
// MagicOnItemPacketListener.landGroundItemGrab (lines 244-262):
// the telegrab projectile has landed, so the item is taken if
// it is still there. XP is granted before the re-lookup, as in
// TS.
func landGroundItemGrab(p *game.Player, groundItemID int, position game.Location, casterFloor int, deps Deps) {
	if !p.IsRegistered() || hitpoints(p) <= 0 {
		return
	}
	if casterFloor == position.Z {
		if p.Location().Z != casterFloor {
			p.SendMessage("Your telegrab fizzles as you move too far away.")
			return
		}
	} else if !withinTelegrabRange(p, position) {
		p.SendMessage("Your telegrab fizzles as you move too far away.")
		return
	}
	p.Skills().AddExperiences(game.SkillMagic, telegrabXP)
	groundItem := findVisibleGroundItem(deps.GroundItems, p, groundItemID, position.X, position.Y, position.Z)
	if groundItem == nil {
		p.SendMessage("Too late - it's gone!")
		return
	}
	if !canGrabGroundItem(p, groundItem) {
		return
	}
	if transferToInventory(p, groundItem, deps.GroundItems) <= 0 {
		return
	}
	emitCustom(p, "magic:telekinetic-grab", &TelekineticGrabEvent{
		Player: p, ItemID: groundItemID, Position: position,
	})
}

// findVisibleGroundItem ports
// MagicOnItemPacketListener.findVisibleGroundItem (lines
// 465-480): the visible stack with the id on the tile, preferring
// the highest plane. z < 0 means any plane (the cast call); the
// landing call passes the plane. The private-area term
// degenerates (no area on the Go stack), and the CrossFloor 64
// term is the same-plane Chebyshev test CanSee already applies.
func findVisibleGroundItem(store GroundItems, p *game.Player, groundItemID, x, y, z int) *game.ItemOnGround {
	if store == nil {
		return nil
	}
	var best *game.ItemOnGround
	for _, stack := range store.Items() {
		if stack == nil || stack.PendingRemoval {
			continue
		}
		if stack.ItemID != groundItemID {
			continue
		}
		if stack.Position.X != x || stack.Position.Y != y {
			continue
		}
		if z >= 0 && stack.Position.Z != z {
			continue
		}
		if !p.Location().IsWithinDistance(stack.Position, telegrabVisibleRange) {
			continue
		}
		if stack.State == game.GroundItemSeenByPlayer && !isOwnerName(stack.Owner, p.Username) {
			continue
		}
		if !store.CanSee(p, stack, false) {
			continue
		}
		if best == nil || stack.Position.Z > best.Position.Z {
			best = stack
		}
	}
	return best
}
