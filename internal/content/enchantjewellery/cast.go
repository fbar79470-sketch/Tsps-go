// Enchant Jewellery: the onSpellOnItem handler and the cast
// (Spells.EnchantJewellery.js handleSpellOnItem / enchant /
// resolveRecipe / ticksUntilCast / returnToSpellbook).
package enchantjewellery

import (
	"tsps-go/internal/game"
	"tsps-go/internal/plugins"
)

// ---- custom-event payloads (Spells.EnchantJewellery.js) ----

// EnchantRecipeRequest is the "magic:enchant-item-recipe" /
// "magic:enchant-item-policy" payload (resolveRecipe): the
// player, the spell name, the item, and the product the spell
// makes of it. ResultID 0 is TS undefined (no recipe). Quest
// hooks answer the recipe event; world-restriction hooks
// (FreeToPlay's blockMembersEnchantment) answer the policy
// event by setting Declined, with Message sent to the player.
// Unanswered both events leave the recipe allowed.
type EnchantRecipeRequest struct {
	Player    *game.Player
	SpellName string
	ItemID    int
	ResultID  int
	Declined  bool
	Message   string
}

// EnchantPiecesRequest is the "magic:enchant-pieces" payload
// (enchant): content may enchant more than one piece a cast
// (a league fragment), each with its XP. Pieces defaults 1.
type EnchantPiecesRequest struct {
	Player    *game.Player
	SpellName string
	ItemID    int
	Pieces    int
}

// EnchantedEvent is the "magic:enchanted" payload (enchant):
// the player, the spell, the item, and the product made.
type EnchantedEvent struct {
	Player    *game.Player
	SpellName string
	ItemID    int
	ProductID int
}

// EnchantItemRequest is the "magic:enchant-item" payload
// (handleSpellOnItem): something else this spell works on
// (the Mage Training Arena's shapes), cast with this spell's
// own runes, level and XP. A handler sets Handled to claim
// it; unhandled, the player is told the spell cannot be cast
// on that item.
type EnchantItemRequest struct {
	Player    *game.Player
	SpellName string
	ItemID    int
	Slot      int
	Handled   bool
}

// ---- the handler ----

// castEnchantOnItem is the onSpellOnItem handler
// (Spells.EnchantJewellery.js handleSpellOnItem): only the
// seven Enchant spells claim the cast. The claim, the recipe
// events, the slot/id gate and the stop of any running loop
// run in TS order; a cast clicked during the 3-tick delay
// waits for it (a queued loop, no immediate cast); otherwise
// the loop registers before the first cast, so its first XP
// drop can pause it too.
func (e *enchanter) castEnchantOnItem(event *plugins.SpellOnItemEvent) {
	if event == nil || event.Player == nil {
		return
	}
	spell := spellFor(event.SpellName)
	if spell == nil {
		return
	}
	event.Handled = true
	player := event.Player
	recipe := e.resolveRecipe(player, spell, event.ItemID)
	if recipe.Declined {
		if recipe.Message != "" {
			player.SendMessage(recipe.Message)
		}
		return
	}
	if recipe.ResultID == 0 {
		other := &EnchantItemRequest{Player: player, SpellName: spell.name, ItemID: event.ItemID, Slot: event.Slot}
		e.emitEvent("magic:enchant-item", other)
		if !other.Handled {
			player.SendMessage("This spell cannot be cast on that item.")
		}
		return
	}
	inv := player.Inventory()
	if inv == nil || event.Slot < 0 || event.Slot >= inv.Capacity() {
		return
	}
	if held := inv.ItemAt(event.Slot); !held.IsValid() || held.ID() != event.ItemID {
		return
	}
	e.stopEnchanting(player)
	caster := spell.spellCaster()
	if caster == nil {
		return
	}
	if !e.membersAllowed(player, spell) {
		return
	}
	// Wiki: enchant spells "queue up one spell if you cast
	// them too quickly".
	wait := e.ticksUntilCast(player, event.Cycle)
	if wait > 0 {
		if !caster.CanCast(player, false) {
			return
		}
		e.startEnchanting(player, spell, event.ItemID, event.Cycle, wait)
		e.tabSwitch(player)
		return
	}
	// Register the loop before the cast so its first XP drop
	// can pause it too.
	e.startEnchanting(player, spell, event.ItemID, event.Cycle, e.repeatTicksFor(player))
	if !e.enchant(player, spell, event.ItemID, event.Cycle) {
		e.stopEnchanting(player)
		return
	}
	e.tabSwitch(player)
}

// membersAllowed is the members-spell gate (TS: the spell's
// members flag, enforced on free worlds by FreeToPlay's
// disableMembersSpell through the onSpellDisabled hook inside
// canCast — no such hook exists in Go yet, so the content
// package gates it directly, the crafting members-recipe
// precedent). A members spell on a free world refuses with
// FreeToPlay's line. Non-members spells always pass.
func (e *enchanter) membersAllowed(player *game.Player, spell *enchantSpell) bool {
	if spell == nil || !spell.members || e.isMembersWorld() {
		return true
	}
	player.SendMessage("You need to be on a members' world to cast this spell.")
	return false
}

// isMembersWorld reports whether this world is a members world.
func (e *enchanter) isMembersWorld() bool {
	if e == nil {
		return true
	}
	return e.membersWorld
}

// resolveRecipe resolves the product of itemID under this
// spell, with the quest hooks' say
// ("magic:enchant-item-recipe") and the world-restriction
// hooks' say after them ("magic:enchant-item-policy").
func (e *enchanter) resolveRecipe(player *game.Player, spell *enchantSpell, itemID int) *EnchantRecipeRequest {
	recipe := &EnchantRecipeRequest{Player: player, SpellName: spell.name, ItemID: itemID, ResultID: spell.resultFor(itemID)}
	e.emitEvent("magic:enchant-item-recipe", recipe)
	e.emitEvent("magic:enchant-item-policy", recipe)
	return recipe
}

// ticksUntilCast is the ticks until the spell's cast delay
// lets the next enchant go off (Spells.EnchantJewellery.js
// ticksUntilCast): the max of the spell's own cooldown
// attribute and the shared cast attribute, against the world
// cycle. A 0 cycle (no clock) never waits (the Superheat
// precedent).
func (e *enchanter) ticksUntilCast(player *game.Player, cycle int64) int {
	if player == nil || cycle <= 0 {
		return 0
	}
	next := int64(0)
	if v, ok := player.GetAttribute(nextCastAttribute).(int64); ok && v > next {
		next = v
	}
	if v, ok := player.GetAttribute(sharedCastAttribute).(int64); ok && v > next {
		next = v
	}
	if next <= cycle {
		return 0
	}
	return int(next - cycle)
}

// ready reports whether the cast delay allows a cast on this
// cycle (the consuming canCast's delay gate, which the Go
// utility spell cannot enforce itself — its base carries no
// shared delay or cooldown attribute — so the handler stamps
// and checks the attributes, the Superheat precedent).
func (e *enchanter) ready(player *game.Player, cycle int64) bool {
	return e.ticksUntilCast(player, cycle) == 0
}

// stamp records the next castable cycle after a consumed cast
// (TS Spell.canCast's cooldown + shared-delay stamps, which
// the Go utility spell does not make): the spell's own
// 3-tick cooldown and the 1-tick shared cast delay. A 0 cycle
// stamps nothing.
func (e *enchanter) stamp(player *game.Player, cycle int64) {
	if player == nil || cycle <= 0 {
		return
	}
	player.SetAttribute(nextCastAttribute, cycle+int64(e.castTicksFor(player)))
	player.SetAttribute(sharedCastAttribute, cycle+sharedCastDelayTicks)
}

// ---- one cast ----

// enchant enchants one cast on the first itemID in the
// inventory (Spells.EnchantJewellery.js enchant); false when
// nothing was enchanted. The recipe resolves again, the
// consuming canCast pays the runes (its level/rune messages
// already sent on refusal), the look plays, and each piece is
// swapped in its own slot with the spell's XP.
func (e *enchanter) enchant(player *game.Player, spell *enchantSpell, itemID int, cycle int64) bool {
	if player == nil || spell == nil {
		return false
	}
	recipe := e.resolveRecipe(player, spell, itemID)
	if recipe.Declined {
		if recipe.Message != "" {
			player.SendMessage(recipe.Message)
		}
		return false
	}
	if recipe.ResultID == 0 {
		return false
	}
	inv := player.Inventory()
	if inv == nil {
		return false
	}
	slot := firstSlotOf(inv, itemID)
	if slot < 0 {
		return false
	}
	if !e.membersAllowed(player, spell) {
		return false
	}
	if !e.ready(player, cycle) {
		return false
	}
	caster := spell.spellCaster()
	if caster == nil || !caster.CanCast(player, true) {
		return false
	}
	look := spell.lookFor(itemID)
	player.PerformAnimation(look.animation, 0)
	player.PerformGraphic(look.graphic, graphicHeight, 0)
	game.SendSound(player, game.NewSound(look.sound, 1, 0, 0))
	// Content may enchant more than one piece a cast
	// ("magic:enchant-pieces", a league fragment), each with
	// its XP.
	pieces := &EnchantPiecesRequest{Player: player, SpellName: spell.name, ItemID: itemID, Pieces: 1}
	e.emitEvent("magic:enchant-pieces", pieces)
	count := pieces.Pieces
	if amount := inv.GetAmount(itemID); amount < count {
		count = amount
	}
	if count < 1 {
		count = 1
	}
	for piece := 0; piece < count; piece++ {
		at := slot
		if piece > 0 {
			at = firstSlotOf(inv, itemID)
		}
		if at < 0 {
			break
		}
		inv.SetItem(at, game.NewItem(recipe.ResultID, 1))
		e.addXP(player, spell.xp)
	}
	inv.RefreshItems()
	e.emitEvent("magic:enchanted", &EnchantedEvent{Player: player, SpellName: spell.name, ItemID: itemID, ProductID: recipe.ResultID})
	e.emitEvent("magic:enchant-item-cast", recipe)
	if recipe.Message != "" {
		player.SendMessage(recipe.Message)
	}
	e.stamp(player, cycle)
	return true
}

// firstSlotOf is the first inventory slot holding itemID, or
// -1 (TS inventory.getItems().findIndex(...)).
func firstSlotOf(inv *game.Inventory, itemID int) int {
	for i, item := range inv.Items() {
		if item.IsValid() && item.ID() == itemID {
			return i
		}
	}
	return -1
}

// addXP grants Magic XP, truncating floats at the call site
// (the mining/smithing precedent; residual (a): 17.5 -> 17).
func (e *enchanter) addXP(player *game.Player, xp float64) {
	if player == nil || xp <= 0 {
		return
	}
	if skills := player.Skills(); skills != nil {
		skills.AddExperiences(game.SkillMagic, int(xp))
	}
}

// clientScriptSender is the session-side client-script send,
// reached through a type assertion on the player's Session
// (the cooking varbitSender / superheat precedent).
type clientScriptSender interface {
	SendClientScript(scriptID int, args ...game.ClientScriptArg)
}

// tabSwitch switches back to the spellbook tab (script 915,
// 6). Once per accepted click; the automatic repeats don't
// send it (rsprox 4119). A session-less player switches
// nowhere.
func (e *enchanter) tabSwitch(player *game.Player) {
	if player == nil || player.Session == nil {
		return
	}
	if sender, ok := player.Session.(clientScriptSender); ok {
		sender.SendClientScript(tabSwitchScriptID, game.ClientScriptInt(magicTab))
	}
}
