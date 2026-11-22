// InterfaceActionClickOpcode + the interface-action dispatch
// paths: the port of InterfaceActionClickOpcode.handle
// (InterfaceActionClickOpcode.ts:7-51), the widget_action
// fallthrough (NetworkBuilder.ts:1121-1136), the widget_target
// fallback (NetworkBuilder.ts:1173-1216), the local_trigger
// cases (NetworkBuilder.ts:1261-1281), the dialogue_continue
// emit pre-branch (NetworkBuilder.ts:939-951), the
// player:widget-action custom event (NetworkBuilder.ts:977-979,
// 1219-1222), the WORLD_INTERACTIONS preamble
// (NetworkBuilder.ts:680-699), the widget close case
// (NetworkBuilder.ts:1170-1171), and the interface_close case
// (NetworkBuilder.ts:1229-1259). The entered-amount/syntax
// prompt dispatch (NetworkBuilder.ts:954-975) lives in
// prompts.go.
//
// SHAPE. The opcode handle runs its gates, then the emit.
// handleInterfaceActionClick ports handle() exactly: the
// null/dead gate, the refusesWhileBusy + isLocked BUSY_MESSAGE
// gate, the teleporting gate (which degenerates to pass: no
// teleport system exists), then emitInterfaceActionClick.
// emitInterfaceActionClick ports the emit call itself: the
// MultiChatboxPrompt check runs FIRST, inside the hub emit
// (PluginManager.ts:2089-2092), then the onInterfaceActionClick
// / onInterfaceActionButton hooks in registration order until
// one claims the click. The dialogue_continue case calls the
// emit directly, NOT the handle: TS calls
// PluginManager.emitInterfaceActionClick there with no gates
// (NetworkBuilder.ts:939-945), so a dead player's resume can
// still answer a prompt.
//
// The listeners package never imports the plugins package: the
// hub emit and the busy-refusal set arrive as func fields on
// InterfaceHooks (the NpcHooks/ObjectHooks precedent), and the
// coordinator wires them over Hub.EmitInterfaceActionClick /
// Hub.RefusesWhileBusy with a translation adapter (the
// listener-facing event mirrors the hub event field for
// field). A nil seam means no hooks / refuses nothing: the
// no-hooks defaults.
//
// ROUTING. The widget_action case (routeWidgetAction in
// listeners.go) runs the player:widget-action emit first, then
// the equipment, bank (gap), shop, prayer/combat/spell (gaps),
// and main-inventory branches in TS order; only the final else
// reaches the opcode handle. A handled fallthrough SKIPS the
// dialogue-option tail (TS `continue`, NetworkBuilder.ts:1132),
// while an unhandled one runs it — after the simple-click
// button-click fallback (emitButtonClick, NetworkBuilder.ts:
// 1133-1137), which fires the onButtonClick / onButton hooks
// (phase 46) for simple IF_BUTTON clicks only; a claimed
// button click also skips the tail. The shop branch is
// untouched: group 300/301 clicks still route to the shop
// handler first (ShopManager.handleWidgetAction sits before
// the fallthrough in the TS chain, NetworkBuilder.ts:1025),
// and only unhandled shop clicks fall through.
//
// The widget_target case closes interruptible interfaces when
// the target is the inventory group (NetworkBuilder.ts:1181),
// then runs the spell-on-item routes (LunarSpells reads false
// — a gap — emitSpellOnItem fires since phase 43, and
// MAGIC_ITEMS.castOnItem handles the alchemy spells) and falls
// through to the opcode handle with the target widget as the
// button id and the source slot as the action. The local_trigger cases map the opcode
// parameter 1..10 to the action with the child index as both
// child id and slot, and every other parameter to a script
// trigger with action 0 (NetworkBuilder.ts:1261-1281).
//
// WORLD-INPUT PREAMBLE. worldInputPreamble ports the preamble
// TS runs for every WORLD_INTERACTIONS packet type before
// dispatch (NetworkBuilder.ts:680-699): the animation-stall
// hold (GAP: no stalled-input queue exists in Go, so a stalled
// click proceeds — the movement queue already honors the stall
// for movement), the TRADING-status skip (the whole packet is
// dropped), the player:world-input custom event (a handled
// event drops the packet), and closeInterruptibleInterfaces
// with the move-when-immobile exception (move packets never
// reach Handle — the coordinator's walk path owns that call
// site, see the report) and the inventory_action
// preservesInterfaceOnEquip exception. Widget-action,
// widget-target, dialogue, varp, and close packets are NOT in
// WORLD_INTERACTIONS and get no preamble. The preamble runs
// only for ready players (playerReady): TS has no unready
// players, and the downstream routes refuse them anyway.
//
// CLOSE SITES. The :697 preamble close and the :1181 inventory
// close call the Player form (Player.CloseInterruptibleInterfaces,
// the status-gated form). The :1251 IF_CLOSE close calls the
// sender form through the InterfaceSends seam (TS calls
// PacketSender.closeInterruptibleInterfaces directly,
// bypassing the Player form's TRADING/DUELING exclusion); a
// nil seam falls back to the Player form, which diverges only
// while trading or duelling. The widget close case and the
// close-on-interface-close overlay branch call closeInterface
// through the same seam.

package listeners

import (
	"tsps-go/internal/game"
	"tsps-go/internal/protocol"
)

// busyMessage is InterfaceActionClickOpcode.BUSY_MESSAGE
// (InterfaceActionClickOpcode.ts:5): a refuseWhileBusy button's
// answer to a locked player.
const busyMessage = "Please finish what you are doing first."

// agilityObstacleAttribute is the agility runner's lock
// attribute (Player.isLocked, Player.ts:827-829).
const agilityObstacleAttribute = "agility.obstacle"

// preserveInterfaceOnEquipAttribute is
// EquipPacketListener.PRESERVE_INTERFACE_ON_EQUIP_ATTRIBUTE
// (EquipPacketListener.ts:33): when it equals the open
// interface id, an inventory_action packet preserves the
// interface instead of closing it (NetworkBuilder.ts:697).
const preserveInterfaceOnEquipAttribute = "interface:preserve-on-equip"

// closeOnInterfaceCloseAttribute is
// CLOSE_ON_INTERFACE_CLOSE_ATTRIBUTE (NetworkBuilder.ts:184):
// an overlay group id the interface_close case closes instead
// of the main close path.
const closeOnInterfaceCloseAttribute = "interface:close-on-interface-close"

// InterfaceActionClickEvent is the listener-facing form of the
// PluginInterfaceActionClickEvent (PluginTypes.ts:873-895).
// It mirrors the hub event field for field (see
// plugins.InterfaceActionClickEvent); the coordinator's
// adapter translates between them. Optional fields carry Has*
// presence flags: TS distinguishes an absent field (undefined)
// from a present zero, and the prompt check reads Slot only
// when present.
type InterfaceActionClickEvent struct {
	Player *game.Player
	// ButtonID is the clicked button id (event.buttonId).
	ButtonID int
	// Action is the click action (event.action).
	Action int

	OpID       int
	HasOpID    bool
	GroupID    int
	HasGroupID bool
	ChildID    int
	HasChildID bool
	ItemID     int
	HasItemID  bool
	Slot       int
	HasSlot    bool
	Option     string
	HasOption  bool
	SubOpID    int
	HasSubOpID bool

	TargetWidgetID    int32
	HasTargetWidgetID bool
	TargetSlot        int
	HasTargetSlot     bool
	TargetItemID      int
	HasTargetItemID   bool
	SourceWidgetID    int32
	HasSourceWidgetID bool
	SourceSlot        int
	HasSourceSlot     bool
	SourceItemID      int
	HasSourceItemID   bool

	ArgsData []byte
	// ScriptTrigger marks a cache-script trigger
	// (if_triggeroplocal from a cache script).
	ScriptTrigger bool

	Handled bool
}

// WidgetActionEvent is the listener-facing form of the
// player:widget-action custom-event payload
// (NetworkBuilder.ts:977-979, 1219-1222): the player, the
// decoded packet, and the handled flag. Packet holds the
// decoded protocol.WidgetAction (widget_action) or
// protocol.WidgetDrag (widget_drag). The coordinator's adapter
// copies it into the hub payload (plugins.WidgetActionEvent)
// and reads Handled back. A handled event skips the whole
// dispatch case.
type WidgetActionEvent struct {
	Player  *game.Player
	Packet  any
	Handled bool
}

// WorldInputEvent is the listener-facing form of the
// player:world-input custom-event payload
// (NetworkBuilder.ts:688-691): the player, the TS packet type
// string, the decoded packet, and the handled flag. A handled
// event skips the packet entirely: no close, no dispatch. The
// coordinator's adapter copies it into the hub payload
// (plugins.WorldInputEvent) and reads Handled back.
type WorldInputEvent struct {
	Player     *game.Player
	PacketType string
	Packet     any
	Handled    bool
}

// InterfaceCloseEvent is the listener-facing form of the
// interface:close custom-event payload
// (NetworkBuilder.ts:1230-1236): the player and the handled
// flag. A handled event still aborts any armed input prompt,
// then skips the rest of the close. The coordinator's adapter
// copies it into the hub payload
// (plugins.InterfaceCloseEvent) and reads Handled back.
type InterfaceCloseEvent struct {
	Player  *game.Player
	Handled bool
}

// ButtonClickEvent is the listener-facing form of the
// simple-click button event (PluginTypes.ts): the player, the
// clicked button id, and the handled flag. It mirrors the hub
// event field for field (see plugins.ButtonClickEvent); the
// coordinator's adapter translates between them. A simple
// IF_BUTTON (opcode 13) click unclaimed by the
// interface-action hub fires it (NetworkBuilder.ts:1133-1137).
type ButtonClickEvent struct {
	Player   *game.Player
	ButtonID int
	Handled  bool
}

// InterfaceHooks carries the interface-action hub emits and
// the busy-refusal set as func fields, following the NpcHooks
// / ObjectHooks precedent. Every field is optional: nil means
// no hooks / refuses nothing, which degenerates to the
// no-hooks defaults (unhandled clicks, no busy refusals, no
// custom-event interceptions). The coordinator wires these in
// cmd/server; the emit adapters translate the listener-facing
// events to the hub events.
type InterfaceHooks struct {
	// RefusesWhileBusy reports whether buttonID was
	// registered with refuseWhileBusy (the hub's
	// busy-refusal set). A nil seam refuses nothing.
	RefusesWhileBusy func(buttonID int) bool
	// EmitInterfaceActionClick fires the
	// onInterfaceActionClick / onInterfaceActionButton
	// hooks (after the hub's prompt check) and reports
	// whether the click was handled. The event's Handled
	// flag carries the same claim; either one claiming is
	// enough (the NpcHooks dual-claim pattern). A nil seam
	// means unhandled.
	EmitInterfaceActionClick func(*InterfaceActionClickEvent) bool
	// EmitWidgetAction fires the player:widget-action
	// custom event and reports whether a handler marked it
	// handled (return value or the Handled flag). A nil
	// seam means unhandled.
	EmitWidgetAction func(*WidgetActionEvent) bool
	// EmitWorldInput fires the player:world-input custom
	// event and reports whether a handler marked it
	// handled (return value or the Handled flag). A nil
	// seam means unhandled.
	EmitWorldInput func(*WorldInputEvent) bool
	// EmitInterfaceClose fires the interface:close custom
	// event and reports whether a handler marked it
	// handled (return value or the Handled flag). A nil
	// seam means unhandled.
	EmitInterfaceClose func(*InterfaceCloseEvent) bool
	// EmitButtonClick fires the onButtonClick / onButton
	// hooks (after the interface-action dispatch, for simple
	// IF_BUTTON clicks only) and reports whether a hook
	// claimed the click (return value or the Handled flag).
	// A nil seam means unhandled.
	EmitButtonClick func(*ButtonClickEvent) bool
}

// isLocked mirrors Player.isLocked (Player.ts:827-829): held
// by a teleport (its cast, until the move) or an agility
// obstacle (the "agility.obstacle" attribute). The teleport
// term degenerates to false: no teleport system exists in Go,
// so no player is ever mid-teleport-cast. The attribute term
// reads the live attribute store (phase 11): presence of the
// key, whatever value it holds, is TS `getAttribute(...) !=
// null`.
func isLocked(p *game.Player) bool {
	return p.HasAttribute(agilityObstacleAttribute)
}

// handleInterfaceActionClick ports InterfaceActionClickOpcode
// .handle (InterfaceActionClickOpcode.ts:22-50): the null/dead
// gate, the refusesWhileBusy + isLocked BUSY_MESSAGE gate
// (which claims the click), the teleporting gate, then the
// emit. It reports whether the click was handled (the message
// sent or the emit claimed). A zero-value player (no skills
// installed) fails the dead gate unevaluably and returns
// false: TS has no such player, and the hitpoints read would
// panic the tick goroutine. The teleporting gate degenerates
// to pass (see isLocked).
func handleInterfaceActionClick(ev *InterfaceActionClickEvent, deps Deps) bool {
	if ev == nil {
		return false
	}
	p := ev.Player
	if p == nil || !playerReady(p) || hitpoints(p) <= 0 {
		return false
	}
	if deps.InterfaceHooks.RefusesWhileBusy != nil &&
		deps.InterfaceHooks.RefusesWhileBusy(ev.ButtonID) && isLocked(p) {
		p.SendMessage(busyMessage)
		return true
	}
	// Teleporting gate (line 43): degenerates to pass — no
	// teleport system exists (see isLocked).
	return emitInterfaceActionClick(ev, deps)
}

// emitInterfaceActionClick fires the interface-action emit
// for ev: the MultiChatboxPrompt check first, then the hooks
// (PluginManager.emitInterfaceActionClick,
// PluginManager.ts:2076-2105). Both live inside the hub emit,
// reached through the InterfaceHooks seam; a nil seam means
// no prompt and no hooks, so the click is unhandled. It
// reports whether the emit claimed the click (the seam's
// return value or the event's Handled flag). Unlike the
// handle, it applies NO gates: the dialogue_continue case
// calls the emit directly (NetworkBuilder.ts:939-945).
func emitInterfaceActionClick(ev *InterfaceActionClickEvent, deps Deps) bool {
	if ev == nil || ev.Player == nil {
		return false
	}
	if deps.InterfaceHooks.EmitInterfaceActionClick == nil {
		return false
	}
	if deps.InterfaceHooks.EmitInterfaceActionClick(ev) || ev.Handled {
		ev.Handled = true
		return true
	}
	return false
}

// emitWidgetAction fires the player:widget-action custom event
// for one widget_action or widget_drag packet
// (NetworkBuilder.ts:977-979, 1219-1222) and reports whether
// a handler marked it handled. A nil seam means unhandled.
func emitWidgetAction(p *game.Player, packet any, deps Deps) bool {
	if deps.InterfaceHooks.EmitWidgetAction == nil {
		return false
	}
	ev := &WidgetActionEvent{Player: p, Packet: packet}
	if deps.InterfaceHooks.EmitWidgetAction(ev) || ev.Handled {
		return true
	}
	return false
}

// emitButtonClick fires the onButtonClick / onButton hooks for
// one simple IF_BUTTON click (NetworkBuilder.ts:1133-1137)
// and reports whether a hook claimed the click. A nil seam
// means unhandled. It runs only for simple clicks: the caller
// (the widget-action fallthrough) checks msg.Simple.
func emitButtonClick(p *game.Player, buttonID int, deps Deps) bool {
	if deps.InterfaceHooks.EmitButtonClick == nil {
		return false
	}
	ev := &ButtonClickEvent{Player: p, ButtonID: buttonID}
	if deps.InterfaceHooks.EmitButtonClick(ev) || ev.Handled {
		return true
	}
	return false
}

// worldInputPreamble runs the WORLD_INTERACTIONS preamble for
// one decoded packet (NetworkBuilder.ts:680-699) and reports
// whether Handle must skip the packet (return without
// routing). packetType is the TS packet type string;
// packet is the decoded message, carried opaquely for the
// custom event. The preamble runs only for ready players
// with a non-nil player; anything else proceeds to routing,
// which refuses it with its own reason.
//
// Order is TS order: the animation-stall hold (GAP — no
// stalled-input queue; the click proceeds), the TRADING skip,
// the player:world-input emit (handled skips the packet),
// then closeInterruptibleInterfaces unless the packet is an
// inventory_action that preserves the interface on equip.
// The move-when-immobile exception never applies here: move
// packets never reach Handle.
func worldInputPreamble(p *game.Player, packetType string, packet any, deps Deps) (skip bool) {
	if p == nil || !playerReady(p) {
		return false
	}
	// Animation-stall hold (lines 682-688): GAP — TS holds
	// the click until the stall ends (stalledInput), but Go
	// has no stalled-input queue in the session layer, so
	// the click proceeds. The movement queue already honors
	// the stall for movement (movement_queue.go).
	// TRADING skip (line 689): a trading player drops the
	// whole packet. Status() derives TRADING from the trade
	// state (trading.go).
	if p.Trading().Status() == game.PlayerStatusTrading {
		return true
	}
	// player:world-input (lines 690-691): handled skips the
	// packet — no close, no dispatch.
	if deps.InterfaceHooks.EmitWorldInput != nil {
		ev := &WorldInputEvent{Player: p, PacketType: packetType, Packet: packet}
		if deps.InterfaceHooks.EmitWorldInput(ev) || ev.Handled {
			return true
		}
	}
	// Close (lines 692-697), unless this inventory_action
	// preserves the interface on equip.
	if packetType == "inventory_action" && preservesInterfaceOnEquip(p) {
		return false
	}
	p.CloseInterruptibleInterfaces()
	return false
}

// MovePreamble runs the WORLD_INTERACTIONS preamble for a move
// packet (NetworkBuilder.ts:680-699) and reports whether the
// caller must skip the packet. Move packets never reach Handle:
// handleTurnPacketWithDeps routes them to OnMovementRequest, so
// the coordinator calls this from the move cases with the
// decoded movement request as packet. Order is TS order: the
// TRADING skip, the player:world-input emit (handled skips the
// packet), then closeInterruptibleInterfaces only when the
// player can move (the move-when-immobile exception — an
// immobile player's click leaves the interface open). The
// animation-stall hold is the same documented gap as in
// worldInputPreamble. A nil or not-ready player proceeds: the
// movement path refuses it with its own rules.
func MovePreamble(p *game.Player, packet any, deps Deps) (skip bool) {
	if p == nil || !playerReady(p) {
		return false
	}
	if p.Trading().Status() == game.PlayerStatusTrading {
		return true
	}
	if deps.InterfaceHooks.EmitWorldInput != nil {
		ev := &WorldInputEvent{Player: p, PacketType: "move", Packet: packet}
		if deps.InterfaceHooks.EmitWorldInput(ev) || ev.Handled {
			return true
		}
	}
	if p.MovementQueue().CanMove() {
		p.CloseInterruptibleInterfaces()
	}
	return false
}

// preservesInterfaceOnEquip mirrors
// EquipPacketListener.preservesInterfaceOnEquip
// (EquipPacketListener.ts:35-37): the
// "interface:preserve-on-equip" attribute equals the open
// interface id. TS compares with === over JS numbers; Go
// compares numerically across the int and float kinds a
// JSON-decoded attribute value can hold (an integral float
// equals its int), and any other kind never preserves.
func preservesInterfaceOnEquip(p *game.Player) bool {
	raw := p.GetAttribute(preserveInterfaceOnEquipAttribute)
	if raw == nil {
		return false
	}
	var value int64
	switch v := raw.(type) {
	case int:
		value = int64(v)
	case int8:
		value = int64(v)
	case int16:
		value = int64(v)
	case int32:
		value = int64(v)
	case int64:
		value = v
	case uint:
		value = int64(v)
	case uint8:
		value = int64(v)
	case uint16:
		value = int64(v)
	case uint32:
		value = int64(v)
	case uint64:
		value = int64(v)
	case float32:
		if float32(int64(v)) != v {
			return false
		}
		value = int64(v)
	case float64:
		if float64(int64(v)) != v {
			return false
		}
		value = int64(v)
	default:
		return false
	}
	return value == int64(p.GetInterfaceId())
}

// routeWidgetTarget routes one decoded widget_target message:
// the widget_target dispatch case (NetworkBuilder.ts:1173-
// 1216). An inventory-group target closes interruptible
// interfaces first (line 1181); the spell-on-item routes
// (LunarSpells.handleItemTarget, emitSpellOnItem,
// MAGIC_ITEMS.castOnItem) run next for inventory targets, and
// a claimed spell click counts as acted; otherwise the opcode
// handle runs with the target widget as the button id and the
// source slot as the action. A handled click counts as acted;
// an unhandled one drops with ReasonWidgetTarget (the spell
// half of the case is still missing). A nil player drops with
// ReasonNoPlayer: TS guards the case on `if (this.player)`.
func routeWidgetTarget(p *game.Player, msg protocol.WidgetTarget, cycle int64, deps Deps, drop func(DropReason) Outcome, acted func() Outcome) Outcome {
	if p == nil {
		return drop(ReasonNoPlayer)
	}
	// Inventory-group close (lines 1178-1181): the target
	// widget's group is the high 16 bits of the uid.
	onInventory := int(uint32(msg.TargetWidgetID)>>16) == mainInventoryGroupID
	if onInventory {
		p.CloseInterruptibleInterfaces()
	}
	// Spell routes (lines 1182-1204): LunarSpells reads false
	// (a gap — see castSpellOnItem in magicitem.go);
	// emitSpellOnItem fires since phase 43 and a claimed
	// spell click counts as acted and skips the handle below,
	// as does a claimed alchemy cast. cycle is the current
	// world process cycle, for the alchemy pacing stamps.
	if onInventory && castSpellOnItem(p, msg, cycle, deps) {
		return acted()
	}
	ev := &InterfaceActionClickEvent{
		Player:            p,
		ButtonID:          int(msg.TargetWidgetID),
		Action:            msg.SourceSlot,
		TargetWidgetID:    msg.TargetWidgetID,
		HasTargetWidgetID: true,
		TargetSlot:        msg.TargetSlot,
		HasTargetSlot:     true,
		TargetItemID:      msg.TargetItemID,
		HasTargetItemID:   true,
		SourceWidgetID:    msg.SourceWidgetID,
		HasSourceWidgetID: true,
		SourceSlot:        msg.SourceSlot,
		HasSourceSlot:     true,
		SourceItemID:      msg.SourceItemID,
		HasSourceItemID:   true,
	}
	if handleInterfaceActionClick(ev, deps) {
		return acted()
	}
	return drop(ReasonWidgetTarget)
}

// routeLocalTrigger routes one decoded local_trigger message:
// the local_trigger dispatch cases (NetworkBuilder.ts:1261-
// 1281). An opcode parameter in 1..10 is an op click: the
// action is the parameter, the child id is the child index,
// and the slot is the child index when non-negative. Any
// other parameter is a script trigger: action 0, the child
// id is the widget's low 16 bits, and scriptTrigger is set.
// Both carry the item id when non-negative and the script
// argument bytes. A handled click counts as acted; an
// unhandled one is ReasonNoAction (the trigger's only purpose
// is the emit, which ran). A nil player drops with
// ReasonNoPlayer: TS guards both branches on `if
// (this.player)`.
func routeLocalTrigger(p *game.Player, msg protocol.LocalTrigger, deps Deps, drop func(DropReason) Outcome, acted, noAction func() Outcome) Outcome {
	if p == nil {
		return drop(ReasonNoPlayer)
	}
	ev := &InterfaceActionClickEvent{
		Player:     p,
		ButtonID:   int(msg.WidgetID),
		GroupID:    int(uint32(msg.WidgetID) >> 16),
		HasGroupID: true,
		ArgsData:   msg.ArgsData,
	}
	if msg.ItemID >= 0 {
		ev.ItemID, ev.HasItemID = msg.ItemID, true
	}
	if msg.ChildIndex >= 0 {
		ev.Slot, ev.HasSlot = msg.ChildIndex, true
	}
	if msg.OpcodeParam >= 1 && msg.OpcodeParam <= 10 {
		ev.Action = int(msg.OpcodeParam)
		ev.ChildID, ev.HasChildID = msg.ChildIndex, true
	} else {
		ev.Action = 0
		ev.ChildID, ev.HasChildID = int(uint32(msg.WidgetID)&0xffff), true
		ev.ScriptTrigger = true
	}
	if handleInterfaceActionClick(ev, deps) {
		return acted()
	}
	return noAction()
}

// routeWidgetClose routes one decoded widget message: the
// widget dispatch case (NetworkBuilder.ts:1170-1171). Only
// the close action acts, closing the named group through the
// session's closeInterface; an open report is routed and
// ignored. A nil player drops with ReasonNoPlayer: TS guards
// the case on `if (this.player)`. A close with no close seam
// wired is ReasonNoAction (nothing open can be named without
// the session state); a wired close counts as acted.
func routeWidgetClose(p *game.Player, msg protocol.Widget, deps Deps, drop func(DropReason) Outcome, acted, noAction func() Outcome) Outcome {
	if p == nil {
		return drop(ReasonNoPlayer)
	}
	if msg.Action != protocol.WidgetClosed {
		return noAction()
	}
	if deps.InterfaceSends.CloseInterface == nil {
		return noAction()
	}
	deps.InterfaceSends.CloseInterface(p, msg.GroupID)
	return acted()
}

// routeInterfaceClose routes an interface_close packet: the
// interface_close dispatch case (NetworkBuilder.ts:1229-
// 1259). Order is TS order: the interface:close custom event
// (a handled event still aborts any armed input prompt, then
// skips the rest), the close-on-interface-close overlay
// branch (an integer attribute closes that overlay instead,
// after aborting the prompt), the had-something-open check
// (status, interface id, active dialogue, interruptible
// interface, or armed prompt — IF_CLOSE is explicit, so the
// sender form of the close runs, bypassing the Player form's
// TRADING/DUELING exclusion), and the nothing-open branch,
// which stays a gap: real OSRS opens the logout tab here,
// and the exact tab-switch mechanism is unconfirmed (the TS
// TODO, lines 1256-1258). A nil player drops with
// ReasonNoPlayer: TS guards the case on `if (this.player)`.
// A close that ran (handled, overlay, or had-something-open)
// counts as acted; the nothing-open branch is ReasonNoAction.
func routeInterfaceClose(p *game.Player, deps Deps, drop func(DropReason) Outcome, acted, noAction func() Outcome) Outcome {
	if p == nil {
		return drop(ReasonNoPlayer)
	}
	// interface:close (lines 1230-1236): a handled event
	// still aborts the chatbox prompt whatever closes.
	if deps.InterfaceHooks.EmitInterfaceClose != nil {
		ev := &InterfaceCloseEvent{Player: p}
		if deps.InterfaceHooks.EmitInterfaceClose(ev) || ev.Handled {
			abortInputPrompt(p, deps)
			return acted()
		}
	}
	// Close-on-interface-close overlay (lines 1237-1241):
	// an integer attribute names the overlay to close. The
	// attribute is cleared first (TS sets it to null; Go
	// SetAttribute(nil) deletes, which is the same
	// observable state).
	if overlayID, ok := closeOnInterfaceCloseID(p); ok {
		p.SetAttribute(closeOnInterfaceCloseAttribute, nil)
		abortInputPrompt(p, deps)
		if deps.InterfaceSends.CloseInterface != nil {
			deps.InterfaceSends.CloseInterface(p, overlayID)
		}
		return acted()
	}
	// Had-something-open (lines 1242-1254).
	if interfaceCloseHadSomethingOpen(p, deps) {
		closeInterruptibleInterfacesSender(p, deps)
		return acted()
	}
	// Nothing was open (lines 1255-1258): GAP — real OSRS
	// opens the logout tab here; the TS TODO stays a gap
	// comment, and the packet takes no action.
	return noAction()
}

// closeOnInterfaceCloseID reads the close-on-interface-close
// overlay id: the attribute value when it is an integer
// (NetworkBuilder.ts:1237-1238, `typeof ... === "number" &&
// Number.isInteger`). The numeric reading matches
// preservesInterfaceOnEquip: JSON-decoded integral floats
// count, anything else does not.
func closeOnInterfaceCloseID(p *game.Player) (int, bool) {
	raw := p.GetAttribute(closeOnInterfaceCloseAttribute)
	if raw == nil {
		return 0, false
	}
	switch v := raw.(type) {
	case int:
		return v, true
	case int8:
		return int(v), true
	case int16:
		return int(v), true
	case int32:
		return int(v), true
	case int64:
		return int(v), true
	case uint:
		return int(v), true
	case uint8:
		return int(v), true
	case uint16:
		return int(v), true
	case uint32:
		return int(v), true
	case uint64:
		return int(v), true
	case float32:
		if float32(int64(v)) != v {
			return 0, false
		}
		return int(v), true
	case float64:
		if float64(int64(v)) != v {
			return 0, false
		}
		return int(v), true
	default:
		return 0, false
	}
}

// interfaceCloseHadSomethingOpen mirrors the
// hadSomethingOpen computation (NetworkBuilder.ts:1242-1247):
// a non-NONE status, an open main interface, an active
// dialogue, a mounted interruptible interface, or an armed
// input prompt. Status reads the Player status field
// (BANKING via the field, TRADING via the trade derivation,
// status.go); SHOPPING/DUELING/PRICE_CHECKING still have no
// producer (their screens set the interface id, which the
// interface-id disjunct covers). The
// interruptible-interface term reads the InterfaceSends seam
// (nil reads false: no session state is visible). A
// zero-value player has no dialogue manager; the dialogue
// term guards it.
func interfaceCloseHadSomethingOpen(p *game.Player, deps Deps) bool {
	if p.Status() != game.PlayerStatusNone {
		return true
	}
	if dueling := p.Dueling(); dueling != nil && dueling.State() != game.DuelStateNone {
		return true
	}
	if p.GetInterfaceId() >= 0 {
		return true
	}
	if manager := p.GetDialogueManager(); manager != nil && manager.IsActive() {
		return true
	}
	if deps.InterfaceSends.HasInterruptibleInterface != nil && deps.InterfaceSends.HasInterruptibleInterface(p) {
		return true
	}
	return p.HasInputPrompt()
}

// closeInterruptibleInterfacesSender runs the SENDER form of
// closeInterruptibleInterfaces
// (PacketSender.closeInterruptibleInterfaces,
// PacketSender.ts:981-995) through the InterfaceSends seam:
// the IF_CLOSE close bypasses the Player form's
// TRADING/DUELING exclusion, as TS calls the sender directly.
// A nil seam falls back to the Player form, which diverges
// only while trading or duelling.
func closeInterruptibleInterfacesSender(p *game.Player, deps Deps) {
	if deps.InterfaceSends.CloseInterruptibleInterfaces != nil {
		deps.InterfaceSends.CloseInterruptibleInterfaces(p)
		return
	}
	p.CloseInterruptibleInterfaces()
}
