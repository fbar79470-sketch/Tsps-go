// NPC interaction: the onNpcClick family, the onNpcInteraction
// family, and the emitNpcInteraction member (PluginTypes.ts:1167,
// 1403).
//
// TS splits NPC clicks in two: onNpcClick registers a plugin
// interaction method in NpcInteractionManager
// (registerNpcClickHook, PluginManager.ts:2944-2980, via
// registerPluginInteraction, NpcInteractionManager.ts:46-79),
// keyed by (npcId, clickType); the NPC-option dispatch runs
// NpcInteractionManager.handle FIRST (NPCOptionPacketListener
// .ts:121-126, dispatch), before emitNpcInteraction, so a
// click handler runs before the generic onNpcInteraction /
// onBeforeNpcInteraction / onAnyNpcInteraction lists
// (emitNpcInteraction, PluginManager.ts:1246-1263). Go runs the
// same order: Hub.EmitNpcInteraction runs the onNpcClick
// bindings first, then the onNpcInteraction name hooks. Go has
// no before/any lists (onBeforeNpcInteraction,
// onNpcsInteraction, onAnyNpcInteraction stay table-only in
// hooks.go); the name hooks are the "specific" list TS runs
// between them.
// The listener calls the one seam (listeners/npc.go
// npcOperate), after the static-shop step and the dialogue
// start, which is the TS handle() position for a plugin
// method (after core.shops.open / core.dialogues.start).
//
// The event carries the CONTENT npc id (npc.getContentId):
// a transformed NPC (an Ent trunk, 9474) answers under its
// transform id, not its spawned id. DefinitionName + Action
// carry what the name match needs (the definition's name and
// the clicked option text, actions[clickType-1]); the hub
// never needs the whole Definition. The per-event hook
// methods TS hangs on the event (onAnimationStall,
// onAttackSpeed, onCombatHitResolved, onCombatAttackDistance,
// onSpellDisabled, onSpellRuneBypass,
// onNpcAggressionTolerance, onPlayerDefeated —
// PluginTypes.ts:170-189) are NOT ported: their families
// stay table-only in hooks.go.
package plugins

import (
	"tsps-go/internal/game"
)

// NpcInteractionEvent is the payload of the onNpcClick and
// onNpcInteraction hooks and the emitNpcInteraction member. It
// mirrors PluginNpcInteractionEvent (PluginTypes.ts:160-168)
// minus the per-event hook-registration methods (see the file
// header) and minus Definition, which the listener carries
// but the hub never needs: the player, the NPC, the content
// NPC id, the NPC index, the click type (1-5), the NPC's
// tile, and the handled flag. DefinitionName is the NPC
// definition's name and Action the clicked option text
// (definition.getActions()[clickType-1]), filled by the
// cmd/server adapter; the onNpcInteraction name match reads
// them, never the Definition itself. Actions is the
// definition's full action list, filled by the same adapter
// for spot lookups that need the joined option list. A hook
// claims the click by returning true (or setting Handled);
// the emit stops at the first claim.
type NpcInteractionEvent struct {
	Player         *game.Player
	NPC            *game.NPC
	NpcID          int
	NpcIndex       int
	ClickType      int
	Location       game.Location
	DefinitionName string
	Action         string
	// Actions is the definition's full menu-action list, in
	// click order. The spot lookup (Fishing's getSpotTool)
	// needs the joined non-empty list
	// (SPOT_TOOLS_BY_OPTIONS, Fishing.plugin.js:461-471):
	// the clicked option alone is ambiguous — "Bait" means
	// sea fish on a "Small Net/Bait" spot but pike on a
	// river "Lure/Bait" spot. Filled by the cmd/server
	// adapter from Definition.Actions().
	Actions []string
	Handled bool
}

// NpcClickHandler is an onNpcClick handler: false falls
// through, any other result claims the click (the emit sets
// Handled). It mirrors the handler shape of the onNpcClick
// overload (PluginTypes.ts:1167;
// PluginManager.ts:4042-4044), whose TS handler returns
// void | boolean — the Go bool is the claim, as for
// ObjectInteractionHandler.
type NpcClickHandler func(*NpcInteractionEvent) bool

// npcClickHook is one onNpcClick registration: the hook
// entry plus the NPC ids and click type it covers, in
// registration order (the Go form of one
// NpcInteractionManager plugin binding, keyed
// npcId:clickType:method).
type npcClickHook struct {
	entry     hookEntry[*NpcInteractionEvent]
	npcIDs    map[int]struct{}
	clickType int
}

// --- hookRegistry storage for the onNpcClick family (see hooks.go) ---

func (r *hookRegistry) addNpcClick(e npcClickHook) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.npcClick = append(r.npcClick, e)
}

func (r *hookRegistry) snapshotNpcClick(npcID, clickType int) []hookEntry[*NpcInteractionEvent] {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var matched []hookEntry[*NpcInteractionEvent]
	for _, hook := range r.npcClick {
		if hook.clickType != clickType {
			continue
		}
		if _, ok := hook.npcIDs[npcID]; !ok {
			continue
		}
		matched = append(matched, hook.entry)
	}
	return matched
}

// NpcInteractionHandler is an onNpcInteraction handler: false
// falls through, any other result claims the click (the emit
// sets Handled). It reuses the NpcClickHandler shape: TS's
// registerNpcActions handler returns void | boolean
// (PluginManager.ts:3160-3180) and `action(event) !== false`
// claims the click — a void return claims too, but Go has no
// void-returning handler shape, so true is the claim, as for
// NpcClickHandler.
type NpcInteractionHandler func(*NpcInteractionEvent) bool

// npcInteractionHook is one onNpcInteraction registration: the
// hook entry plus the NPC definition name and the
// option-text-keyed actions it covers, in registration order
// (the Go form of one registerNpcActions hook on
// PluginManager.npcInteractionHooks, PluginManager.ts:3155-
// 3180).
type npcInteractionHook struct {
	entry   hookEntry[*NpcInteractionEvent]
	name    string
	actions map[string]NpcInteractionHandler
}

// --- hookRegistry storage for the onNpcInteraction family (see hooks.go) ---

func (r *hookRegistry) addNpcInteraction(e npcInteractionHook) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.npcInteraction = append(r.npcInteraction, e)
}

func (r *hookRegistry) snapshotNpcInteraction(name string) []hookEntry[*NpcInteractionEvent] {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var matched []hookEntry[*NpcInteractionEvent]
	for _, hook := range r.npcInteraction {
		if hook.name != name {
			continue
		}
		matched = append(matched, hook.entry)
	}
	return matched
}

// OnNpcClick registers an NPC-click hook, mirroring
// onNpcClick (PluginTypes.ts:1167) and its createApi
// registration (PluginManager.ts:4042-4044) through
// registerNpcClickHook (PluginManager.ts:2944-2980): the
// handler runs when the clicked NPC's content id is one of
// npcIDs and the click type matches. Invalid registrations
// are warned about and dropped, mirroring the TS guard: no
// valid ids, a click type outside 1-5, or a nil handler.
// (TS also warns when some ids are invalid and keeps the
// valid ones; Go does the same.) A nil handler is ignored
// silently, mirroring the createApi guard.
func (a *Api) OnNpcClick(npcIDs []int, clickType int, handler NpcClickHandler) {
	if handler == nil {
		return
	}
	if !a.canRegister("onNpcClick") {
		return
	}
	valid := make([]int, 0, len(npcIDs))
	for _, id := range npcIDs {
		if id >= 0 {
			valid = append(valid, id)
		}
	}
	if len(valid) == 0 || clickType < 1 || clickType > 5 {
		if a != nil && a.hub != nil {
			if logger := a.hub.Logger(); logger != nil {
				logger.Printf("[plugins] %s attempted invalid npc click hook registration npcIds=%s clickType=%d", a.name, jsonString(npcIDs), clickType)
			}
		}
		return
	}
	if len(valid) != len(npcIDs) {
		if a != nil && a.hub != nil {
			if logger := a.hub.Logger(); logger != nil {
				logger.Printf("[plugins] %s npc click hook registration dropped invalid ids (kept %d/%d): npcIds=%s", a.name, len(valid), len(npcIDs), jsonString(npcIDs))
			}
		}
	}
	ids := make(map[int]struct{}, len(valid))
	for _, id := range valid {
		ids[id] = struct{}{}
	}
	a.hub.hooks.addNpcClick(npcClickHook{
		entry: hookEntry[*NpcInteractionEvent]{pluginName: a.name, handler: func(event *NpcInteractionEvent) {
			if event == nil {
				return
			}
			if handler(event) {
				event.Handled = true
			}
		}},
		npcIDs:    ids,
		clickType: clickType,
	})
}

// OnNpcInteraction registers a name-based NPC-interaction
// hook, mirroring onNpcInteraction(name, actions)
// (PluginTypes.ts:167-176; PluginManager.ts:3488-3499) through
// registerNpcActions (PluginManager.ts:3155-3180): the handler
// for the clicked option runs when the NPC definition's name
// matches and the option text (definition.getActions()
// [clickType-1]) is one of the registered actions. The action
// claims the click by returning true (or setting Handled);
// false falls through to the next hook. Invalid registrations
// are warned about and dropped, mirroring the TS guards: an
// empty name, or no function-valued actions (TS filters the
// actions map to function values in registerNpcActions and
// the onNpcsInteraction guard warns on invalid names). A nil
// actions map registers nothing, silently — there is nothing
// to filter. The click-type guard (1-5) lives in the hook
// wrapper at emit time, exactly where TS checks it
// (PluginManager.ts:3175).
func (a *Api) OnNpcInteraction(name string, actions map[string]NpcInteractionHandler) {
	if !a.canRegister("onNpcInteraction") {
		return
	}
	if actions == nil {
		return
	}
	valid := make(map[string]NpcInteractionHandler, len(actions))
	for option, handler := range actions {
		if handler != nil {
			valid[option] = handler
		}
	}
	if name == "" || len(valid) == 0 {
		if a != nil && a.hub != nil {
			if logger := a.hub.Logger(); logger != nil {
				logger.Printf("[plugins] %s attempted invalid npc interaction hook registration name=%q actions=%d", a.name, name, len(valid))
			}
		}
		return
	}
	if len(valid) != len(actions) {
		if a != nil && a.hub != nil {
			if logger := a.hub.Logger(); logger != nil {
				logger.Printf("[plugins] %s npc interaction hook registration dropped invalid actions (kept %d/%d): name=%q", a.name, len(valid), len(actions), name)
			}
		}
	}
	a.hub.hooks.addNpcInteraction(npcInteractionHook{
		entry: hookEntry[*NpcInteractionEvent]{pluginName: a.name, handler: func(event *NpcInteractionEvent) {
			if event == nil || event.Handled || event.ClickType < 1 || event.ClickType > 5 {
				return
			}
			if event.DefinitionName != name {
				return
			}
			action, ok := valid[event.Action]
			if !ok || action == nil {
				return
			}
			if action(event) {
				event.Handled = true
			}
		}},
		name:    name,
		actions: valid,
	})
}

// EmitNpcInteraction runs an NPC click through the
// registered onNpcClick hooks and reports whether one
// claimed it, mirroring PluginManager.emitNpcInteraction
// (PluginManager.ts:1246-1263) with label
// "npc_interaction": the entry guards (a nil event, player,
// or NPC returns false), then the matching click hooks in
// registration order, breaking once the event is handled.
// The generic before/main/any lists TS runs in the same
// emit are NOT ported yet (table-only in hooks.go); when
// they land they run after the click hooks, as TS runs
// handle() before emitNpcInteraction. A pre-handled event
// runs no hooks and returns true, as the TS loop does. A
// nil Hub or a Hub without a registry returns false.
//
// The production call site is the NPC-option listener's
// npcOperate (through the cmd/server adapter);
// Api.EmitNpcInteraction exposes the same emit to content
// (PluginTypes.ts:1403).
func (h *Hub) EmitNpcInteraction(event *NpcInteractionEvent) bool {
	if h == nil || h.hooks == nil || event == nil || event.Player == nil || event.NPC == nil {
		return false
	}
	hooks := h.hooks.snapshotNpcClick(event.NpcID, event.ClickType)
	if runHandledGate(h.Logger(), "npc_interaction", hooks, event,
		func(e *NpcInteractionEvent) bool { return e.Handled }) {
		return true
	}
	// The onNpcInteraction name hooks run AFTER the click
	// bindings (TS: NpcInteractionManager.handle runs before
	// emitNpcInteraction, and inside the emit the specific
	// list runs before the any list; Go has no before/any
	// lists). A name hook matches on the definition name
	// and the clicked option text; a click already claimed
	// above never reaches them.
	nameHooks := h.hooks.snapshotNpcInteraction(event.DefinitionName)
	return runHandledGate(h.Logger(), "npc_interaction", nameHooks, event,
		func(e *NpcInteractionEvent) bool { return e.Handled })
}

// EmitNpcInteraction runs an NPC click through the
// registered onNpcClick hooks and then the onNpcInteraction
// name hooks, and reports whether one claimed it, mirroring
// the PluginApi emitNpcInteraction member (PluginTypes.ts:
// 1403) and its createApi passthrough (PluginManager.ts:
// 4695). It delegates to Hub.EmitNpcInteraction. A nil Api,
// a nil hub, or a nil event yields false. This is an emit,
// not a registration: it has no load-phase gate.
func (a *Api) EmitNpcInteraction(event *NpcInteractionEvent) bool {
	if a == nil || a.hub == nil {
		return false
	}
	return a.hub.EmitNpcInteraction(event)
}
