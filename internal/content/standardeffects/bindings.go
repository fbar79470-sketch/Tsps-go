package standardeffects

import (
	"tsps-go/internal/game"
	"tsps-go/internal/plugins"
)

// Bind, Snare and Entangle spell ids (the standardSpellDefs
// table, internal/game/combat_spells_data.go:131, 187, 306).
const (
	spellBindID     = 1572
	spellSnareID    = 1582
	spellEntangleID = 1592
)

// bindHoldTicks is the base hold per bind spell
// (StandardBindings.plugin.js:11: BIND 8, SNARE 16,
// ENTANGLE 24).
var bindHoldTicks = map[int]int{
	spellBindID:     8,
	spellSnareID:    16,
	spellEntangleID: 24,
}

// holdTicksKey is the per-cast context slot carrying the
// Swampbark-adjusted hold (StandardBindings.plugin.js:4,
// HOLD_TICKS_KEY, "standard-bindings:hold-ticks").
const holdTicksKey = "standard-bindings:hold-ticks"

// Swampbark armour ids (ItemIdentifiers.ts:23413, 23422,
// 23425): the helm, body and legs in the equipment effects
// panel (enum 3047).
const (
	swampbarkHelmID = 25398
	swampbarkBodyID = 25389
	swampbarkLegsID = 25401
)

// swampbarkSetText is the set-text enum id for a full
// Swampbark set (StandardBindings.plugin.js:6,
// SWAMPBARK_SET_TEXT 25, enum 3047).
const swampbarkSetText = 25

// swampbarkPieces lists the (slot, id) pairs of the Swampbark
// set (StandardBindings.plugin.js:13-14, armour).
var swampbarkPieces = [][2]int{
	{game.HeadSlot, swampbarkHelmID},
	{game.BodySlot, swampbarkBodyID},
	{game.LegSlot, swampbarkLegsID},
}

// swampbarkWorn counts the Swampbark pieces the player wears,
// counting a piece only when its amount is positive
// (StandardBindings.plugin.js:44-45, committed: id match and
// amount > 0). A nil player or equipment wears nothing.
func swampbarkWorn(player *game.Player) int {
	if player == nil || player.Equipment() == nil {
		return 0
	}
	pieces := 0
	for _, piece := range swampbarkPieces {
		item := player.Equipment().ItemAt(piece[0])
		if item.ID() == piece[1] && item.Amount() > 0 {
			pieces++
		}
	}
	return pieces
}

// swampbarkSetWorn reports whether the player wears the full
// Swampbark set (StandardBindings.plugin.js:18-20,
// addSwampbarkSetText: id match only, no amount check).
func swampbarkSetWorn(player *game.Player) bool {
	if player == nil || player.Equipment() == nil {
		return false
	}
	for _, piece := range swampbarkPieces {
		if player.Equipment().ItemAt(piece[0]).ID() != piece[1] {
			return false
		}
	}
	return true
}

// bindDisabled is the onSpellDisabled gate
// (StandardBindings.plugin.js:23-38, disabled): a bind cast
// on a target already held is refused with a line, as is one
// on a target in its five-tick freeze grace period. The
// already-disabled answer is respected, and a spell that is
// not a bind passes through untouched.
func (s *standardEffects) bindDisabled(event *plugins.SpellDisabledEvent) {
	if event == nil || event.Player == nil {
		return
	}
	if event.Disabled != nil && *event.Disabled {
		return
	}
	if _, ok := bindHoldTicks[event.SpellID]; !ok {
		return
	}
	target := event.Player.GetCombat().Target()
	if target == nil {
		return
	}
	timers := target.GetTimers()
	if timers.Has(game.TimerKeyFreeze) {
		event.Player.SendMessage("Your target is already held by a magical force.")
		event.Disabled = plugins.BoolPtr(true)
	} else if timers.Has(game.TimerKeyFreezeImmunity) {
		// Wiki (Snare, Trivia): "The target is currently
		// immune to that spell." in the grace period. The
		// recorded "Your target is ..." line is Enfeeble's
		// on a curse-immune NPC (227/00095.txt:81131).
		event.Player.SendMessage("The target is currently immune to that spell.")
		event.Disabled = plugins.BoolPtr(true)
	}
}

// bindCommitted is the "magic:cast-committed" handler
// (StandardBindings.plugin.js:40-47, committed): a paid bind
// cast stores its hold in the cast context — the base hold
// plus two ticks per Swampbark piece worn.
func (s *standardEffects) bindCommitted(payload any) {
	committed, ok := payload.(*game.MagicCastCommittedPayload)
	if !ok || committed == nil || committed.Player == nil || committed.Context == nil {
		return
	}
	spell, ok := committed.Spell.(game.CombatSpell)
	if !ok || spell == nil {
		return
	}
	ticks, ok := bindHoldTicks[spell.SpellID()]
	if !ok {
		return
	}
	committed.Context.Data[holdTicksKey] = ticks + swampbarkWorn(committed.Player)*2
}

// bindImpact is the "magic:utility-effect" handler
// (StandardBindings.plugin.js:49-58, impact): a bind that
// lands holds the target. The hold comes from the committed
// context when it is in range (base .. base+6, even steps —
// the Swampbark bonus), and falls back to the base hold
// otherwise. The generic service owns NPC immunity, movement
// interruption and the five-tick grace period; binds say
// nothing when they land (Freezes sends the fade warning),
// so the factory message is suppressed with an empty string
// — TS passes null, which the factory treats the same way
// (CombatFactory.ts:1625, 1659-1661: message != null).
func (s *standardEffects) bindImpact(payload any) {
	event, ok := payload.(*game.MagicUtilityEffectPayload)
	if !ok || event == nil || event.Spell == nil || event.Target == nil {
		return
	}
	if !event.Allowed || event.Handled {
		return
	}
	ticks, ok := bindHoldTicks[event.Spell.SpellID()]
	if !ok {
		return
	}
	event.Handled = true
	if ctx := event.Context; ctx != nil && ctx.Spell == event.Spell {
		if saved, ok := ctx.Data[holdTicksKey].(int); ok &&
			saved >= ticks && saved <= ticks+6 && (saved-ticks)%2 == 0 {
			ticks = saved
		}
	}
	hadFreeze := event.Target.GetTimers().Has(game.TimerKeyFreeze)
	empty := ""
	game.FreezeTicks(event.Target, ticks, &empty, event.Attacker, "bind")
	if !hadFreeze && event.Target.GetTimers().Has(game.TimerKeyFreeze) {
		s.emitFrozen(event.Target, event.Attacker, ticks, "bind")
	}
}

// emitFrozen emits the "combat:frozen" event for a freeze this
// package applied. TS emits it from CombatFactory.freezeTicks
// itself (CombatFactory.ts:1665); Go's FreezeTicks does not,
// so the applying consumer emits it — here for binds, the
// Ancient consumer for ice freezes.
func (s *standardEffects) emitFrozen(target, source *game.Mobile, ticks int, kind string) {
	if s == nil || s.api == nil || target == nil {
		return
	}
	s.api.EmitCustomEvent(game.CombatFrozenEventName, &game.CombatFrozenPayload{
		Target: target,
		Source: source,
		Ticks:  ticks,
		Kind:   kind,
	})
}

// addSwampbarkSetText is the "equipment:effects" handler
// (StandardBindings.plugin.js:17-20, addSwampbarkSetText): a
// full Swampbark set adds its text to the equipment effects
// panel. There is NO producer in Go (game.EquipmentEffectsEventName:
// EquipmentStats is not ported, so the event never fires), so
// this handler never runs — a documented gap, not a silent one.
func (s *standardEffects) addSwampbarkSetText(payload any) {
	request, ok := payload.(*game.EquipmentEffectsRequest)
	if !ok || request == nil || request.Player == nil {
		return
	}
	if swampbarkSetWorn(request.Player) {
		request.SetTexts = append(request.SetTexts, swampbarkSetText)
	}
}
