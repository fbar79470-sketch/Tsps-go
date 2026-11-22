// Spell gates and rune sources: the Go form of the spell
// families of PluginApi (PluginTypes.ts) that the spell system
// can fire since phase 27: onSpellDisabled, onSpellRuneBypass,
// and registerSpellRuneSource.
//
// Every emit below ports its PluginManager.emit* body,
// including the entry guards, the registration wrappers, the
// hook ordering, and the first-answer shape (runFirstAnswerGate,
// gates.go):
//
//   - emitSpellDisabled (PluginManager.ts:1824-1849): the first
//     non-null "disabled" answer wins and later hooks never
//     run; no answer yields null. The registration wrapper
//     skips events without a player or a non-integer spellId
//     (PluginManager.ts:3978-3995).
//   - emitSpellRuneBypass (PluginManager.ts:1851-1876): the
//     first non-null "bypass" answer wins; no answer yields
//     null. The registration wrapper is the same shape
//     (PluginManager.ts:3996-4013).
//   - emitSpellOnItem (PluginManager.ts:1975-1984): the slot/id
//     guard claims a stale slot first, then the hooks in order
//     with the TS loop's `if (event.handled) break`.
//   - emitSpellOnObject (PluginManager.ts:1960-1966): the loop
//     runs ALL hooks with no break; the per-hook registration
//     wrapper (PluginManager.ts:4201-4213) still skips once the
//     event is handled, so the first claim wins at handler
//     level. No entry guard.
//   - emitSpellCast (PluginManager.ts:1987-1993): the hooks in
//     order with the TS loop's `if (event.handled) break`.
//     The call site only emits when a spell name resolves
//     (NetworkBuilder.ts:1804-1812).
//   - checkSpellRuneSource (PluginManager.ts:5056-5074): the
//     first source that can supply every missing item answers
//     true, or false when none can; an empty missing list
//     answers false. A throwing source is logged and skipped.
//   - consumeSpellRuneSource (PluginManager.ts:5076-5092):
//     deducts the shortfall from the first source that can
//     supply it, leaving the rest alone.
//
// The Go emits return plain bools, not tri-state: both TS call
// sites compare `=== true` (Spell.ts: canCast's disabled check
// and itemsToConsume's whole-cost and per-rune bypass checks),
// so null and false are the same "not disabled / no bypass"
// reading to every caller. The first-answer gate still runs
// behind the bool: a hook that answers false stops later hooks
// exactly as TS's non-null answer does.
//
// EMIT-SITE SEAM (game never imports plugins): the spell
// CanCast path lives in internal/game (Worker A) and cannot
// call these emits. It calls the game-side CombatHooks seam
// (internal/game/combat_hooks.go: SpellDisabled,
// SpellRuneBypass, CheckSpellRuneSource, ConsumeSpellRuneSource),
// whose func fields the cmd/server adapter populates from
// these Hub methods (see cmd/server/combatwiring.go).
//
// RUNE SOURCES. No rune pouch (or any other rune source) exists
// in Go: a grep over the repo finds no rune-pouch concept
// outside the cache facade's gameval-name comment
// (internal/cache/facade.go:893, `bankside:runepouch_top`).
// CheckSpellRuneSource therefore returns false and
// ConsumeSpellRuneSource is a no-op until a source registers
// through Api.RegisterSpellRuneSource — the no-source default,
// documented, not invented. A source is content: e.g. a rune
// pouch plugin registers { check, consume } and the seam starts
// answering.

package plugins

import (
	"log"

	"tsps-go/internal/game"
)

// SpellDisabledEvent is the payload of the onSpellDisabled
// hooks. It mirrors PluginSpellDisabledEvent
// (PluginTypes.ts:663-670): the player, the spellbook, the
// spell id, the Spell being cast when the caller has it (e.g.
// spell.isMembers()), and the disabled answer. Spell is a
// game.CombatSpell: nil when the caller has none. Disabled is
// a *bool: nil is TS null (undecided), non-nil is the hook's
// answer. A hook sets Disabled through BoolPtr (gates.go).
type SpellDisabledEvent struct {
	Player    *game.Player
	Spellbook game.Spellbook
	SpellID   int
	Spell     game.CombatSpell
	Disabled  *bool
}

// SpellRuneBypassEvent is the payload of the
// onSpellRuneBypass hooks. It mirrors
// PluginSpellRuneBypassEvent (PluginTypes.ts:672-679): the
// player, the spellbook, the spell id, the rune id when the
// query is per-rune, and the bypass answer. RuneID is 0 for
// the whole-cost query (TS passes runeId undefined there —
// Spell.ts itemsToConsume calls the emit with no rune id
// first); 0 is never a real rune id, so the two forms stay
// distinct. Bypass is a *bool, as Disabled is above.
type SpellRuneBypassEvent struct {
	Player    *game.Player
	Spellbook game.Spellbook
	SpellID   int
	RuneID    int
	Bypass    *bool
}

// SpellOnItemEvent is the payload of the onSpellOnItem
// hooks. It mirrors PluginSpellOnItemEvent
// (PluginTypes.ts:801-810): the player, the spellbook name of
// the cast spell ("" when the cache has none), the spell's
// widget and item ids, the inventory item the spell was cast
// on and its slot, and the handled flag. A hook sets Handled
// to claim the cast (the listener's own cast route is then
// skipped, as TS skips MAGIC_ITEMS.castOnItem when the emit
// claims the click).
type SpellOnItemEvent struct {
	Player      *game.Player
	SpellName   string
	SpellWidget int
	SpellItemID int
	ItemID      int
	Slot        int
	// Cycle is the world cycle the click arrived on (the
	// listener's pacing clock; handlers that pace a cast,
	// like Superheat's 3-tick pace, read it). Go-only: TS
	// paces through the spell's wall-clock cooldown instead.
	// 0 when the caller has none.
	Cycle   int64
	Handled bool
}

// SpellOnObjectEvent is the payload of the onSpellOnObject
// hooks. It mirrors PluginSpellOnObjectEvent
// (PluginTypes.ts:763-774): the player, the object (the
// GameObject type from ObjectInteractionEvent, objects.go:57),
// its id, the spell's widget/child/item ids, the resolved
// combat spell id (-1 when the selection resolves to no
// spell), the object's tile, and the handled flag. A hook sets
// Handled to claim the cast; the emit runs ALL hooks with no
// break, so a later hook still sees the event.
type SpellOnObjectEvent struct {
	Player      *game.Player
	Object      GameObject
	ObjectID    int
	SpellWidget int
	SpellChild  int
	SpellItemID int
	SpellID     int
	Location    game.Location
	Handled     bool
}

// SpellCastEvent is the payload of the onSpellCast hooks. It
// mirrors PluginSpellCastEvent (PluginTypes.ts:790-800): the
// player, the spellbook name of the cast spell, the spell's
// widget and item ids, the spellbook option clicked (1 for
// "Cast"; 2 is Varrock Teleport's "Grand Exchange"), and the
// handled flag. A hook sets Handled to claim the cast; the
// first claim wins and later hooks never run.
type SpellCastEvent struct {
	Player      *game.Player
	SpellName   string
	SpellWidget int
	SpellItemID int
	OpID        int
	Handled     bool
}

// SpellRuneSource is a container that can supply runes the
// inventory cannot, e.g. a rune pouch. It mirrors
// PluginSpellRuneSource (PluginTypes.ts:981-988): Check is
// true when this source holds every item in missingItems in
// full; Consume deducts missingItems from this source, and is
// only called after Check returned true. Items are game.Item
// values; handlers read their id/amount, never mutate the
// player's containers through them.
type SpellRuneSource interface {
	Check(player *game.Player, items []game.Item) bool
	Consume(player *game.Player, items []game.Item)
}

// spellRuneSourceEntry is one registered rune source: the
// owning plugin's name (for error labelling) plus the source.
// It mirrors the spellRuneSources entries
// (PluginManager.ts:416).
type spellRuneSourceEntry struct {
	pluginName string
	source     SpellRuneSource
}

// --- hookRegistry storage for the spell families (see hooks.go) ---

func (r *hookRegistry) addSpellDisabled(e hookEntry[*SpellDisabledEvent]) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.spellDisabled = append(r.spellDisabled, e)
}

func (r *hookRegistry) snapshotSpellDisabled() []hookEntry[*SpellDisabledEvent] {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return append([]hookEntry[*SpellDisabledEvent](nil), r.spellDisabled...)
}

func (r *hookRegistry) addSpellRuneBypass(e hookEntry[*SpellRuneBypassEvent]) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.spellRuneBypass = append(r.spellRuneBypass, e)
}

func (r *hookRegistry) snapshotSpellRuneBypass() []hookEntry[*SpellRuneBypassEvent] {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return append([]hookEntry[*SpellRuneBypassEvent](nil), r.spellRuneBypass...)
}

func (r *hookRegistry) addSpellRuneSource(e spellRuneSourceEntry) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.spellRuneSources = append(r.spellRuneSources, e)
}

func (r *hookRegistry) snapshotSpellRuneSources() []spellRuneSourceEntry {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return append([]spellRuneSourceEntry(nil), r.spellRuneSources...)
}

func (r *hookRegistry) addSpellOnItem(e hookEntry[*SpellOnItemEvent]) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.spellOnItem = append(r.spellOnItem, e)
}

func (r *hookRegistry) snapshotSpellOnItem() []hookEntry[*SpellOnItemEvent] {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return append([]hookEntry[*SpellOnItemEvent](nil), r.spellOnItem...)
}

func (r *hookRegistry) addSpellOnObject(e hookEntry[*SpellOnObjectEvent]) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.spellOnObject = append(r.spellOnObject, e)
}

func (r *hookRegistry) snapshotSpellOnObject() []hookEntry[*SpellOnObjectEvent] {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return append([]hookEntry[*SpellOnObjectEvent](nil), r.spellOnObject...)
}

func (r *hookRegistry) addSpellCast(e hookEntry[*SpellCastEvent]) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.spellCast = append(r.spellCast, e)
}

func (r *hookRegistry) snapshotSpellCast() []hookEntry[*SpellCastEvent] {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return append([]hookEntry[*SpellCastEvent](nil), r.spellCast...)
}

// --- registration methods ---

// OnSpellDisabled registers a spell-disabled hook, mirroring
// onSpellDisabled (PluginTypes.ts:1161). Hooks run in
// registration order through the first-answer gate
// (EmitSpellDisabled): the first non-null Disabled wins and
// later hooks never run. A nil handler is ignored silently,
// mirroring the createApi guard (PluginManager.ts:3978-3995).
// The stored handler wraps the plugin's with the TS guard: an
// event without a player never reaches the plugin's handler.
// The TS guard's non-integer-spellId term has no Go form (Go
// ints are always integers); the wrapper checks the player
// only.
func (a *Api) OnSpellDisabled(handler func(*SpellDisabledEvent)) {
	if handler == nil {
		return
	}
	if !a.canRegister("onSpellDisabled") {
		return
	}
	a.hub.hooks.addSpellDisabled(hookEntry[*SpellDisabledEvent]{pluginName: a.name, handler: func(event *SpellDisabledEvent) {
		if event == nil || event.Player == nil {
			return
		}
		handler(event)
	}})
}

// OnSpellRuneBypass registers a spell-rune-bypass hook,
// mirroring onSpellRuneBypass (PluginTypes.ts:1162). Hooks run
// in registration order through the first-answer gate
// (EmitSpellRuneBypass). A nil handler is ignored silently,
// mirroring the createApi guard (PluginManager.ts:3996-4013).
// The stored handler wraps the plugin's with the TS guard, as
// OnSpellDisabled does.
func (a *Api) OnSpellRuneBypass(handler func(*SpellRuneBypassEvent)) {
	if handler == nil {
		return
	}
	if !a.canRegister("onSpellRuneBypass") {
		return
	}
	a.hub.hooks.addSpellRuneBypass(hookEntry[*SpellRuneBypassEvent]{pluginName: a.name, handler: func(event *SpellRuneBypassEvent) {
		if event == nil || event.Player == nil {
			return
		}
		handler(event)
	}})
}

// OnSpellOnItem registers a spell-on-item hook, mirroring
// onSpellOnItem (PluginTypes.ts:1210). Hooks run in
// registration order through EmitSpellOnItem: each hook's
// registration wrapper skips once the event is handled, so
// the first claim wins and later hooks never run — the TS
// loop's `if (event.handled) break`
// (PluginManager.ts:1975-1984). A nil handler is ignored
// silently, mirroring the createApi guard
// (PluginManager.ts:4225-4235). The stored handler wraps the
// plugin's with the TS guard: an event without a player, or
// one already handled, never reaches the plugin's handler.
func (a *Api) OnSpellOnItem(handler func(*SpellOnItemEvent)) {
	if handler == nil {
		return
	}
	if !a.canRegister("onSpellOnItem") {
		return
	}
	a.hub.hooks.addSpellOnItem(hookEntry[*SpellOnItemEvent]{pluginName: a.name, handler: func(event *SpellOnItemEvent) {
		if event == nil || event.Handled || event.Player == nil {
			return
		}
		handler(event)
	}})
}

// OnSpellOnObject registers a spell-on-object hook,
// mirroring onSpellOnObject (PluginTypes.ts:1208). Hooks run
// in registration order through EmitSpellOnObject, whose loop
// has no break (PluginManager.ts:1960-1966) — but each hook's
// registration wrapper skips once the event is handled, so
// the first claim still wins at handler level, exactly as
// OnSpellOnItem's wrapper does for the breaking emit. A nil
// handler is ignored silently, mirroring the createApi guard
// (PluginManager.ts:4201-4213). The stored handler wraps the
// plugin's with the TS guard: an event without a player or
// object, or one already handled, never reaches the plugin's
// handler.
func (a *Api) OnSpellOnObject(handler func(*SpellOnObjectEvent)) {
	if handler == nil {
		return
	}
	if !a.canRegister("onSpellOnObject") {
		return
	}
	a.hub.hooks.addSpellOnObject(hookEntry[*SpellOnObjectEvent]{pluginName: a.name, handler: func(event *SpellOnObjectEvent) {
		if event == nil || event.Handled || event.Player == nil || event.Object == nil {
			return
		}
		handler(event)
	}})
}

// OnSpellCast registers a spell-cast hook, mirroring
// onSpellCast (PluginTypes.ts:1212). Hooks run in
// registration order through EmitSpellCast: each hook's
// registration wrapper skips once the event is handled, so
// the first claim wins and later hooks never run — the TS
// loop's `if (event.handled) break`
// (PluginManager.ts:1987-1993). A nil handler is ignored
// silently, mirroring the createApi guard
// (PluginManager.ts:4239-4251). The stored handler wraps the
// plugin's with the TS guard: an event without a player, or
// one already handled, never reaches the plugin's handler.
func (a *Api) OnSpellCast(handler func(*SpellCastEvent)) {
	if handler == nil {
		return
	}
	if !a.canRegister("onSpellCast") {
		return
	}
	a.hub.hooks.addSpellCast(hookEntry[*SpellCastEvent]{pluginName: a.name, handler: func(event *SpellCastEvent) {
		if event == nil || event.Handled || event.Player == nil {
			return
		}
		handler(event)
	}})
}

// RegisterSpellRuneSource registers a rune source (e.g. a rune
// pouch), mirroring the PluginApi registerSpellRuneSource
// member (PluginTypes.ts:1527) and its createApi implementation
// (PluginManager.ts:4849-4861): the source must carry check and
// consume, else the registration is warned about and dropped.
// In Go the interface guarantees both methods, so only a nil
// source is invalid. Sources are consulted in registration
// order by CheckSpellRuneSource / ConsumeSpellRuneSource. A nil
// Api, or an Api with a nil hub, is a no-op. Registration has
// NO load-phase gate, like RegisterArea: the TS closure has no
// phase check, and both load-time and later registration are
// legitimate.
func (a *Api) RegisterSpellRuneSource(source SpellRuneSource) {
	if a == nil || a.hub == nil || a.hub.hooks == nil {
		return
	}
	if source == nil {
		logger := a.hub.Logger()
		if logger == nil {
			logger = log.Default()
		}
		logger.Printf("[plugins] %s attempted invalid spell rune source registration", a.name)
		return
	}
	a.hub.hooks.addSpellRuneSource(spellRuneSourceEntry{pluginName: a.name, source: source})
}

// --- Hub emits ---

// EmitSpellDisabled asks the registered hooks whether a spell
// is disabled for a player, mirroring
// PluginManager.emitSpellDisabled (PluginManager.ts:1824-1849)
// with label "spell_disabled": the first non-null Disabled
// wins, later hooks never run, and no answer yields false
// (TS null; every caller compares `=== true`, so false is the
// faithful collapse). A nil Hub or a Hub without a registry
// yields false. There is no emit-level player guard: the
// registration wrappers skip player-less events, exactly as
// TS's do, so the outcome is identical (false) — following the
// EmitCanShop precedent.
//
// The production call site is Spell.canCast (Worker A),
// through the game-side CombatHooks.SpellDisabled seam (see
// the file header).
func (h *Hub) EmitSpellDisabled(player *game.Player, spellbook game.Spellbook, spellID int, spell game.CombatSpell) bool {
	if h == nil || h.hooks == nil {
		return false
	}
	event := &SpellDisabledEvent{Player: player, Spellbook: spellbook, SpellID: spellID, Spell: spell}
	answer := runFirstAnswerGate(h.Logger(), "spell_disabled", h.hooks.snapshotSpellDisabled(), event,
		func(e *SpellDisabledEvent) *bool { return e.Disabled })
	return answer != nil && *answer
}

// EmitSpellRuneBypass asks the registered hooks whether a rune
// cost is bypassed for a player, mirroring
// PluginManager.emitSpellRuneBypass (PluginManager.ts:1851-1876)
// with label "spell_rune_bypass": the first non-null Bypass
// wins, later hooks never run, and no answer yields false.
// RuneID is the rune being queried, or 0 for the whole-cost
// query (TS passes runeId undefined there). A nil Hub or a Hub
// without a registry yields false.
//
// The production call sites are Spell.itemsToConsume (Worker
// A: the whole-cost query, then one per-rune query) and the
// MagicOnItem listener (Worker B), through the game-side
// CombatHooks.SpellRuneBypass seam (see the file header).
func (h *Hub) EmitSpellRuneBypass(player *game.Player, spellbook game.Spellbook, spellID, runeID int) bool {
	if h == nil || h.hooks == nil {
		return false
	}
	event := &SpellRuneBypassEvent{Player: player, Spellbook: spellbook, SpellID: spellID, RuneID: runeID}
	answer := runFirstAnswerGate(h.Logger(), "spell_rune_bypass", h.hooks.snapshotSpellRuneBypass(), event,
		func(e *SpellRuneBypassEvent) *bool { return e.Bypass })
	return answer != nil && *answer
}

// EmitSpellOnItem runs the onSpellOnItem hooks for a spell
// cast on an inventory item, mirroring
// PluginManager.emitSpellOnItem (PluginManager.ts:1975-1984)
// with label "spell_on_item": the slot/id guard first —
// when the slot no longer holds the item the click is
// claimed (true) and no hook runs, exactly as TS returns
// true there — then the hooks in registration order, each
// skipped once the event is handled (the TS loop's break),
// and the result is whether a hook claimed the cast. The
// emitCanUseItem term reads as allowed: no can-use seam
// exists in the listeners (the same documented gap
// castOnItem carries). A nil Hub, a Hub without a registry,
// or a nil event yields false (Go-only guards; TS has no
// nil-event equivalent). A nil player yields true: the slot
// guard cannot pass without one, and TS's guard would throw
// on the inventory read — claiming the click is the
// fail-closed reading.
//
// The production call site is castSpellOnItem
// (internal/listeners/magicitem.go), through the
// listeners' SpellHooks.EmitSpellOnItem seam wired in
// cmd/server/spellwiring.go. Superheat Item is its first
// consumer (phase 43).
func (h *Hub) EmitSpellOnItem(event *SpellOnItemEvent) bool {
	if h == nil || h.hooks == nil || event == nil {
		return false
	}
	player := event.Player
	if player == nil || player.Inventory() == nil {
		return true
	}
	if event.Slot < 0 || event.Slot >= player.Inventory().Capacity() {
		return true
	}
	if held := player.Inventory().ItemAt(event.Slot); !held.IsValid() || held.ID() != event.ItemID {
		return true
	}
	emitHooks(h.Logger(), "spell_on_item", h.hooks.snapshotSpellOnItem(), event)
	return event.Handled
}

// EmitSpellOnObject runs the onSpellOnObject hooks for a spell
// cast on an object, mirroring
// PluginManager.emitSpellOnObject (PluginManager.ts:1960-1966)
// with label "spell_on_object": the loop runs ALL hooks with
// no break, and the result is whether a hook claimed the cast.
// The per-hook registration wrappers still skip once the event
// is handled (PluginManager.ts:4201-4213), so the first claim
// wins at handler level, exactly as EmitSpellOnItem's
// breaking loop does — the run-all shape is the emit's, not
// the handlers'. A nil Hub, a Hub without a registry, or a nil
// event yields false (Go-only guards; TS has no nil-event
// equivalent). Unlike EmitSpellOnItem there is no slot guard:
// TS's emit has no entry guard at all, and the listener only
// emits for an object the lookup found.
//
// The production call site is spellOnObject
// (internal/listeners/useitem.go), through the listeners'
// SpellHooks.EmitSpellOnObject seam wired in
// cmd/server/spellwiring.go. Charge Orbs is its first
// consumer (phase 45, slice 3).
func (h *Hub) EmitSpellOnObject(event *SpellOnObjectEvent) bool {
	if h == nil || h.hooks == nil || event == nil {
		return false
	}
	emitHooks(h.Logger(), "spell_on_object", h.hooks.snapshotSpellOnObject(), event)
	return event.Handled
}

// EmitSpellCast runs the onSpellCast hooks for a spell cast
// from the spellbook on nothing, mirroring
// PluginManager.emitSpellCast (PluginManager.ts:1987-1993)
// with label "spell_cast": the hooks run in registration
// order through the runHandledGate contract (gates.go), each
// skipped once the event is handled (the TS loop's break),
// and the result is whether a hook claimed the cast. A nil
// Hub, a Hub without a registry, or a nil event yields false
// (Go-only guards; TS has no nil-event equivalent). The
// listener only emits when a spell name resolves
// (NetworkBuilder.ts:1804-1812), so an unresolvable click
// never reaches this emit.
//
// The production call site is routeWidgetAction
// (internal/listeners/listeners.go), through the listeners'
// SpellHooks.EmitSpellCast seam wired in
// cmd/server/spellwiring.go. Bolt Enchanting and the Lunar
// crafting spells are its first consumers (phase 45).
func (h *Hub) EmitSpellCast(event *SpellCastEvent) bool {
	if h == nil || h.hooks == nil || event == nil {
		return false
	}
	return runHandledGate(h.Logger(), "spell_cast", h.hooks.snapshotSpellCast(), event,
		func(e *SpellCastEvent) bool { return e.Handled })
}

// CheckSpellRuneSource reports whether a registered rune source
// can supply every item in items, mirroring
// PluginManager.checkSpellRuneSource
// (PluginManager.ts:5056-5074): the first source whose Check
// returns true answers true; an empty list answers false; no
// source answers false. A throwing source is logged and
// skipped (executeHook's contract does not apply here —
// sources are not hooks, so the check recovers inline and
// continues, as TS's try/catch does). A nil Hub or a Hub
// without a registry answers false.
//
// The production call sites are Spell.canCast's shortfall check
// and missingItemsMessage (Worker A) and the MagicOnItem
// listener (Worker B), through the game-side
// CombatHooks.CheckSpellRuneSource seam (see the file header).
func (h *Hub) CheckSpellRuneSource(player *game.Player, items []game.Item) bool {
	if h == nil || h.hooks == nil || len(items) == 0 {
		return false
	}
	logger := h.Logger()
	if logger == nil {
		logger = log.Default()
	}
	for _, entry := range h.hooks.snapshotSpellRuneSources() {
		if entry.source == nil {
			continue
		}
		covered := func() (covered bool) {
			defer func() {
				if r := recover(); r != nil {
					logger.Printf("[plugins] spell rune source check failed (%s): %v", entry.pluginName, r)
					covered = false
				}
			}()
			return entry.source.Check(player, items)
		}()
		if covered {
			return true
		}
	}
	return false
}

// ConsumeSpellRuneSource deducts items from the first rune
// source that can supply them, mirroring
// PluginManager.consumeSpellRuneSource
// (PluginManager.ts:5076-5092): the first source whose Check
// returns true has Consume called on it, and the rest are left
// alone. An empty list or no covering source is a no-op. A
// throwing source is logged and skipped, as in
// CheckSpellRuneSource.
//
// The production call sites are Spell.canCast's rune-spend
// path (Worker A) and the MagicOnItem listener (Worker B),
// through the game-side CombatHooks.ConsumeSpellRuneSource seam
// (see the file header).
func (h *Hub) ConsumeSpellRuneSource(player *game.Player, items []game.Item) {
	if h == nil || h.hooks == nil || len(items) == 0 {
		return
	}
	logger := h.Logger()
	if logger == nil {
		logger = log.Default()
	}
	for _, entry := range h.hooks.snapshotSpellRuneSources() {
		if entry.source == nil {
			continue
		}
		covered := func() (covered bool) {
			defer func() {
				if r := recover(); r != nil {
					logger.Printf("[plugins] spell rune source check failed (%s): %v", entry.pluginName, r)
					covered = false
				}
			}()
			return entry.source.Check(player, items)
		}()
		if !covered {
			continue
		}
		func() {
			defer func() {
				if r := recover(); r != nil {
					logger.Printf("[plugins] spell rune source consume failed (%s): %v", entry.pluginName, r)
				}
			}()
			entry.source.Consume(player, items)
		}()
		return
	}
}
