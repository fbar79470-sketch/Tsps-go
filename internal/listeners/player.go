// PlayerOptionPacketListener: player_option (opcodes 44, 45, 46, 73, 6,
// 48, 10, 21, decoding as options 1-8) + INTERACT (opcode 233, options
// 2-3), porting executeClientOption (PlayerOptionPacketListener.ts:17-40)
// for the attack and trade options.
//
// Opcode map (verified against the TS listener + the Go decode table):
// 44 decodes as option 1, 45 as option 2, 46 as option 3, 73 as option
// 4, 6 as option 5, 48 as option 6, 10 as option 7, and 21 as option 8
// (ClientProtocol.ts lines 570-601; DecodePlayerOption). INTERACT
// decodes byte 0 as option 3 and any other byte as option 2
// (DecodeInteract).
//
// Option 1 (attack), TS order. Gates (lines 18-21): the option in
// 1..8 (decoder-guaranteed; the range check guards hand-built
// messages), the player existing with hitpoints above 0 and not
// busy() — busy() degenerates to the hitpoints term, as for NPCs
// and objects — and the target existing, not being the player
// themself, and having hitpoints above 0. Then the route intercept
// (line 24): the "player:option-route" custom event lets a plugin
// that gave slot 1 its own label take the click; a handled route
// clears following (degenerate: no following system exists) and
// returns without attacking. Then the attack starts through
// the combat seam (line 26) — the seam performs the
// autocast-conditional clear first (when an autocast spell is
// set, the single-cast spell is cleared, line 25) — and the
// player-attack emit fires (lines 27-28).
//
// What the listener does NOT check, because TS does not check it
// here: distance (no range test in executeClientOption at all),
// the plane (the plane check at line 30 sits AFTER the option-1
// block and governs only options 2-8), Wilderness/PvP ground,
// wilderness levels, combat-level difference, skull state, or
// multi-combat. All of those live in CombatFactory — validTarget
// (CombatFactory.ts:554-593: the 40-tile cross-floor distance, the
// plane rule, the owner rule), canAttackPermission
// (CombatFactory.ts:705-837: multi-combat, area/plugin policy,
// stun, duel rules), canAttackByPolicy (CombatFactory.ts:838-871:
// safe buildings, area canAttack, the PvP default-deny), and
// handleSkull/wouldSkull (CombatFactory.ts:1390-1440) — and are
// Worker 2's combat-core work. Adding any of them to the listener
// would double-gate (or wrongly gate) what the combat process
// already decides every tick, including the walksInOnFoughtNpc
// approach case (Combat.ts:357-360), where a permission refusal
// still walks the attacker in.
//
// The WORLD_INTERACTIONS preamble (NetworkBuilder.ts:680-693)
// runs in Handle before routing (worldInputPreamble): the
// animation-stall hold is a gap (no stalled-input queue), the
// TRADING skip and the player:world-input emit run, and
// closeInterruptibleInterfaces runs unless the packet is
// skipped. A mid-trade option-2 click is therefore dropped by
// the preamble before this listener's busy gate ever sees it.
// The INTERACTION_CLICKS preamble (NetworkBuilder.ts:713-719) IS
// ported: player_option is in INTERACTION_CLICKS, so the click-run
// override is set before routing, even for options that then drop
// — including INTERACT, whose packet carries no modifier flags (0,
// the preamble's default for a packet without the field).
//
// Option 2 (trade) is implemented (phase 17, trade.go): the shared
// gates, the plane check, the distance/registration gates, the
// trade-request emit, and the walk-to-entity with the sendRequest
// arrival callback (TradeRequestPacketListener). An unwired player
// lookup drops with ReasonTradeRequest; a refused packet returns
// noAction; a started walk/request counts as acted.
//
// Options 3-8 stay decode-drop, each with its own reason:
//
//   - Option 3 (follow): ReasonFollowPlayer. TS checks the plane
//     and calls FollowPlayerPacketListener.request (following /
//     pursuit NOT STARTED). Same unobservable plane gate.
//   - Options 4-8 (plugin-owned): ReasonPlayerOption. TS checks
//     the plane, fires the route intercept, and walks to the
//     target with walkToEntity before emitting onPlayerOption
//     (lines 36-39). walkToEntity is ported (phase 17), but
//     onPlayerOption stays table-only, so the walk has no
//     arrival emit to run. The route intercept is NOT fired for
//     these options: firing it without the emit would be a
//     half-port, and a plugin that claims the slot has no
//     arrival to walk itself to.
//
// Seams: the coordinator wires PlayerLookup over the World's
// player list, PlayerHooks over the hub (the option-route custom
// event and the player-attack emit), and Deps.CombatAttack over
// the combat contract (see listeners.go). When the lookup or the
// combat seam is unwired, the attack option drops with
// ReasonPlayerOption; when the lookup is unwired, the trade
// option drops with ReasonTradeRequest; a refused packet (gates,
// lookup miss, self, dead target, handled route/emit) returns
// noAction, the silent return TS gives; a started attack or
// trade walk counts as acted.

package listeners

import (
	"tsps-go/internal/game"
	"tsps-go/internal/protocol"
)

// PlayerLookup resolves a player by world index: the Go form of
// World.getPlayers().get(index)
// (PlayerOptionPacketListener.ts:20). A miss (empty slot,
// out-of-range index) returns nil. The coordinator implements it
// over World.Players().
type PlayerLookup interface {
	Find(index int) *game.Player
}

// PlayerOptionRouteEvent is the listener-facing form of the
// "player:option-route" custom-event payload
// (PlayerOptionPacketListener.ts:10-15): the player, the target,
// the option number, and the handled flag. A hook (or the emit
// adapter) sets Handled to claim the slot; the listener treats
// either the seam's return value or the flag as a claim, matching
// the NPC interaction dual-claim pattern.
type PlayerOptionRouteEvent struct {
	Player  *game.Player
	Target  *game.Player
	Option  int
	Handled bool
}

// PlayerAttackEvent is the listener-facing form of the
// PluginPlayerAttackEvent payload (PluginTypes.ts:450-453): the
// attacking player and the target player. It fires after the
// attack starts and carries no handled flag: hooks observe.
type PlayerAttackEvent struct {
	Player *game.Player
	Target *game.Player
}

// PlayerHooks carries the player-option hub emits as func fields,
// following the NpcHooks precedent. Every field is optional: nil
// means no hooks, which degenerates to the no-hooks defaults (the
// route unclaimed, no attack delivery). The coordinator wires
// these in cmd/server.
type PlayerHooks struct {
	// EmitOptionRoute fires the "player:option-route" custom
	// event and reports whether a plugin claimed the slot.
	// The event's Handled flag carries the same claim; either
	// one claiming is enough.
	EmitOptionRoute func(*PlayerOptionRouteEvent) bool
	// EmitPlayerAttack delivers the onPlayerAttack event for a
	// started attack.
	EmitPlayerAttack func(*PlayerAttackEvent)
}

// playerRouteResult is playerAttack's outcome: the attack started,
// the packet was refused silently, or the combat seam is unwired.
type playerRouteResult int

const (
	playerAttacked playerRouteResult = iota
	playerRefused
	playerCombatUnwired
)

// routePlayerOption routes one decoded player_option message. The
// click-run override is set before routing, even when routing then
// refuses or drops the packet (the INTERACTION_CLICKS preamble).
// Options 3-8 drop with their listener reasons without touching
// game state (the option number comes from the opcode, so the
// split needs no lookup); option 1 runs the attack path and
// option 2 the trade-request path. A nil player on option 1 or 2
// drops with ReasonNoPlayer (the implemented routes' rule — TS
// guards the dispatch case on `if (this.player)`); a nil player
// on options 3-8 keeps the pure-drop reason, as those paths never
// dereference the player.
func routePlayerOption(p *game.Player, msg protocol.PlayerOption, deps Deps, drop func(DropReason) Outcome, acted, noAction func() Outcome) Outcome {
	if p != nil {
		if q := p.MovementQueue(); q != nil {
			q.SetClickRunOverride(game.ClickRunOverride(p.IsRunning(), msg.ModifierFlags))
		}
	}
	switch msg.Option {
	case 1:
		if p == nil {
			return drop(ReasonNoPlayer)
		}
		switch playerAttack(p, msg, deps) {
		case playerAttacked:
			return acted()
		case playerCombatUnwired:
			return drop(ReasonPlayerOption)
		default:
			return noAction()
		}
	case 2:
		if p == nil {
			return drop(ReasonNoPlayer)
		}
		switch requestTrade(p, msg, deps) {
		case tradeRequestStarted:
			return acted()
		case tradeRequestUnwired:
			return drop(ReasonTradeRequest)
		default:
			return noAction()
		}
	default:
		return drop(playerOptionReason(msg.Option))
	}
}

// playerAttack ports the option-1 branch of executeClientOption
// (PlayerOptionPacketListener.ts:17-29): gates, the route
// intercept, and attack initiation through the combat seam plus
// the player-attack emit. It reports whether the attack started,
// the packet was refused, or the combat seam is unwired.
func playerAttack(p *game.Player, msg protocol.PlayerOption, deps Deps) playerRouteResult {
	if !playerReady(p) {
		return playerRefused
	}
	// Gates (lines 18-19): the option range (decoder-guaranteed;
	// the check guards hand-built messages) and hitpoints above
	// 0 — busy() degenerates to the hitpoints term in Go (see
	// the file header).
	if msg.Option < 1 || msg.Option > 8 || hitpoints(p) <= 0 {
		return playerRefused
	}

	// Lookup (line 20): the index range and slot checks collapse
	// into the lookup miss — a nil lookup result is an empty or
	// out-of-range slot. An unwired lookup drops named: the
	// target cannot be resolved at all.
	if deps.Players == nil {
		return playerCombatUnwired
	}
	target := deps.Players.Find(msg.Index)

	// Target gates (line 21): the target must exist, must not be
	// the player themself, and must have hitpoints above 0. A
	// target without a SkillManager (a hand-built fixture, never
	// a world player) cannot be verified alive and refuses.
	if target == nil || target == p || target.Skills() == nil ||
		target.Skills().CurrentLevel(game.SkillHitpoints) <= 0 {
		return playerRefused
	}

	// Route intercept (lines 22-24): a plugin that gave slot 1
	// its own label handles the click. A claimed route clears
	// following (degenerate: no following system) and returns
	// without attacking. The autocast-conditional clear (line
	// 25) runs in the combat seam, before the attack (see the
	// file header).
	if deps.PlayerHooks.EmitOptionRoute != nil {
		route := &PlayerOptionRouteEvent{Player: p, Target: target, Option: msg.Option}
		if deps.PlayerHooks.EmitOptionRoute(route) || route.Handled {
			return playerRefused
		}
	}

	// Attack (line 26) through the combat seam, then the
	// player-attack emit (lines 27-28). An unwired seam drops
	// named: combat initiation is unavailable.
	if deps.CombatAttack == nil {
		return playerCombatUnwired
	}
	deps.CombatAttack(p, &target.Mobile)
	if deps.PlayerHooks.EmitPlayerAttack != nil {
		deps.PlayerHooks.EmitPlayerAttack(&PlayerAttackEvent{Player: p, Target: target})
	}
	return playerAttacked
}

// Spell on player (MagicOnPlayerPacketListener.cast,
// MagicOnPlayerPacketListener.ts:5-43, plus the NetworkBuilder
// spell_on_player case, NetworkBuilder.ts:786-806: the
// magic:spell-on-player custom event fires first when the target
// exists, then LunarSpells.handlePlayerTarget, then the cast).
// The dispatch case sets the click-run override before routing
// (spell_on_player is in INTERACTION_CLICKS,
// NetworkBuilder.ts:246-260), exactly as the option clicks do.
//
// Gaps, documented at their sites:
//   - Lunar routing (NetworkBuilder.ts:786-806): Lunar is out
//     of scope, so a Lunar-named spell falls through to
//     resolution and refuses silently when no table holds it —
//     the same observable outcome as TS with no Lunar handler
//     installed.
//   - No distance, plane, Wilderness, or multi-combat check
//     here, because TS has none here either: CombatFactory
//     owns them (see the file header).

// routeSpellOnPlayer routes one decoded spell_on_player message.
// A nil player or an unwired player lookup or combat seam drops
// with ReasonMagicOnPlayer; a refused packet returns noAction,
// the silent return TS gives; a started cast counts as acted.
func routeSpellOnPlayer(p *game.Player, msg protocol.SpellOnPlayer, deps Deps, drop func(DropReason) Outcome, acted, noAction func() Outcome) Outcome {
	if p == nil {
		return drop(ReasonMagicOnPlayer)
	}
	if !playerReady(p) || hitpoints(p) <= 0 {
		return noAction()
	}
	// Index bounds (lines 9-11): a negative index refuses
	// without the queue reset. The upper bound collapses
	// into the lookup miss below — PlayerLookup has no
	// capacity to check against, and the reset difference
	// is unobservable.
	if msg.TargetIndex < 0 {
		return noAction()
	}
	if deps.Players == nil {
		return drop(ReasonMagicOnPlayer)
	}
	target := deps.Players.Find(msg.TargetIndex)
	if target == nil || target == p || hitpoints(target) <= 0 {
		if q := p.MovementQueue(); q != nil {
			q.Reset(true)
		}
		return noAction()
	}
	// magic:spell-on-player (NetworkBuilder.ts:790-798): the
	// target exists, so the custom event fires before the
	// Lunar routing and the cast. A handled event claims the
	// click.
	name, _ := spellNameOf(deps, int(msg.SpellWidget), msg.SpellItemID)
	ev := &SpellOnPlayerEvent{
		Player:      p,
		Target:      target,
		SpellName:   name,
		SpellWidget: int(msg.SpellWidget),
		SpellChild:  msg.SpellChild,
		SpellItemID: msg.SpellItemID,
	}
	emitCustom(p, "magic:spell-on-player", ev)
	if ev.Handled {
		return acted()
	}
	// Lunar routing (NetworkBuilder.ts:799-801): no handler —
	// the spell falls through to resolution (see the file
	// header).
	//
	// Spell resolution and gates (lines 12-38). The spellId
	// < 0 check sits after the target lookup here, where TS
	// has it before: the event above must fire first when
	// the target exists, and the only observable difference
	// is the queue reset on a self-target with a bad spell
	// id, a hand-built packet.
	spellID := resolveSpellID(deps, int(msg.SpellWidget), msg.SpellChild, msg.SpellItemID)
	if spellID < 0 {
		return noAction()
	}
	spell := game.GetCombatSpell(spellID)
	if spell == nil {
		if q := p.MovementQueue(); q != nil {
			q.Reset(true)
		}
		return noAction()
	}
	if !spell.CanCastOnTarget(p, &target.Mobile) {
		if q := p.MovementQueue(); q != nil {
			q.Reset(true)
		}
		return noAction()
	}
	if deps.SpellHooks.CastSpell == nil {
		return drop(ReasonMagicOnPlayer)
	}
	deps.SpellHooks.CastSpell(p, &target.Mobile, spell)
	return acted()
}
