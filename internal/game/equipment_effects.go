package game

import (
	"math"
	"sync"
)

// Run-energy equipment effects: the port of the run-energy half of
// EquipmentEffects.ts (elvarg/game/content/combat/EquipmentEffects.ts)
// — the RunEnergyRestoreModifier / RunEnergyDrainModifier registries,
// their register functions, and the apply reduce. The graceful set
// raises the restore rate through a restore modifier; the endurance
// ring scales the drain through a drain modifier. All other modifier
// families in the file (melee/ranged/magic hit, prayer, special
// energy, status, incoming damage) are NOT STARTED: no content port
// needs them yet, and each lands with the system that consumes it.
//
// NARROWING (documented): TS types the modifiers over Mobile
// (EquipmentEffects.ts:21-22). Go narrows them to *Player: only
// players have run energy in Go, both call sites resolve to a
// player (the restore site has p; the drain site resolves the queue
// owner — see movementOwnerPlayer), and the TS drain call site
// already casts its character to Player (MovementQueue.ts:818).
// A modifier therefore always receives a non-nil *Player.
//
// WORLD SCOPING: TS keeps the registries in module-level arrays.
// Go keeps one EquipmentEffects per World (installed on players at
// add, like the AreaManager): no process-wide globals, following
// the Phase 21 race-fix precedent (a tick in flight past
// Engine.Stop must never read another world's registry). The
// registry guards its slices with an RWMutex: registration happens
// at boot (flushed from the hub queue before ticking starts) but
// late registration is legitimate — the TS closures accept pushes
// at any time — so Apply snapshots under a read lock and runs the
// modifiers without holding it, mirroring the hookRegistry
// snapshot discipline. A modifier that registers during an apply
// therefore cannot deadlock, and only affects later ticks.
//
// NO ERROR ISOLATION: modifiers run inline in the reduce, exactly
// as TS runs them (EquipmentEffects.ts:303-308, 321-325). A
// modifier that panics propagates into the tick, as a TS throw
// propagates into Player.process; the hub's per-hook isolation
// does not apply here because the apply sites are engine code,
// not hook emits.

// RunEnergyRestoreModifier is the Go form of RunEnergyRestoreModifier
// (EquipmentEffects.ts:21): it maps the energy units regained this
// tick (the graceful set raises them). It receives the player the
// units belong to; see the narrowing note in the file header.
type RunEnergyRestoreModifier func(*Player, float64) float64

// RunEnergyDrainModifier is the Go form of RunEnergyDrainModifier
// (EquipmentEffects.ts:22): it maps the run energy spent for one
// running tick (the endurance ring scales it to 85%; returning 0
// skips the drain — PluginTypes.ts:1495). It receives the player
// the amount belongs to; see the narrowing note in the file header.
type RunEnergyDrainModifier func(*Player, float64) float64

// EquipmentEffects is the world-scoped run-energy modifier
// registry: the Go form of the runEnergyRestoreModifiers /
// runEnergyDrainModifiers module arrays (EquipmentEffects.ts:
// 61-62). Build one with NewEquipmentEffects; the zero value is
// usable (an empty registry applies identity) but the World
// always installs a constructed one. It is safe for concurrent
// Register/Apply use; see the file header.
type EquipmentEffects struct {
	mu      sync.RWMutex
	restore []RunEnergyRestoreModifier
	drain   []RunEnergyDrainModifier
}

// NewEquipmentEffects returns an empty run-energy modifier
// registry.
func NewEquipmentEffects() *EquipmentEffects {
	return &EquipmentEffects{}
}

// RegisterRestoreModifier appends a restore modifier, in
// registration order (the reduce consults modifiers in order).
// It is the Go form of registerRunEnergyRestoreModifier
// (EquipmentEffects.ts:199-203): a nil modifier is ignored, the
// Go form of the `typeof modifier !== "function"` guard.
func (e *EquipmentEffects) RegisterRestoreModifier(modifier RunEnergyRestoreModifier) {
	if e == nil || modifier == nil {
		return
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	e.restore = append(e.restore, modifier)
}

// RegisterDrainModifier appends a drain modifier, in registration
// order. It is the Go form of registerRunEnergyDrainModifier
// (EquipmentEffects.ts:205-209): a nil modifier is ignored, the
// Go form of the `typeof modifier === "function"` guard.
func (e *EquipmentEffects) RegisterDrainModifier(modifier RunEnergyDrainModifier) {
	if e == nil || modifier == nil {
		return
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	e.drain = append(e.drain, modifier)
}

// ApplyRestoreModifiers runs the restore modifiers over units, in
// registration order: the Go form of
// applyRunEnergyRestoreModifiers (EquipmentEffects.ts:303-308).
// The reduce keeps the previous value when a modifier returns a
// non-finite or negative result. A nil registry applies identity.
// The player passed to each modifier is p, unchanged.
func (e *EquipmentEffects) ApplyRestoreModifiers(p *Player, units float64) float64 {
	if e == nil {
		return units
	}
	e.mu.RLock()
	modifiers := append([]RunEnergyRestoreModifier(nil), e.restore...)
	e.mu.RUnlock()
	value := units
	for _, modifier := range modifiers {
		if next := modifier(p, value); isFinite(next) && next >= 0 {
			value = next
		}
	}
	return value
}

// ApplyDrainModifiers runs the drain modifiers over amount, in
// registration order: the Go form of applyRunEnergyDrainModifiers
// (EquipmentEffects.ts:321-325). The reduce keeps the previous
// value when a modifier returns a non-finite or negative result.
// A nil registry applies identity. The player passed to each
// modifier is p, unchanged.
func (e *EquipmentEffects) ApplyDrainModifiers(p *Player, amount float64) float64 {
	if e == nil {
		return amount
	}
	e.mu.RLock()
	modifiers := append([]RunEnergyDrainModifier(nil), e.drain...)
	e.mu.RUnlock()
	value := amount
	for _, modifier := range modifiers {
		if next := modifier(p, value); isFinite(next) && next >= 0 {
			value = next
		}
	}
	return value
}

// isFinite reports whether f is a finite float64: the Go form of
// Number.isFinite (math has no IsFinite; NaN and both infinities
// are excluded). The modifier reduce and the weights override
// rule both read finiteness through it.
func isFinite(f float64) bool {
	return !math.IsNaN(f) && !math.IsInf(f, 0)
}

// applyRunEnergyRestoreModifiers is the formula's view of
// applyRunEnergyRestoreModifiers (EquipmentEffects.ts:303-308):
// the modifiers of p's installed world registry, or identity
// when p is nil or no registry is installed (a standalone
// player, or a world without the wiring). The formula call sites
// pass the player; the exported pure formula functions pass nil
// and therefore always read the unmodified base.
func applyRunEnergyRestoreModifiers(p *Player, units float64) float64 {
	if p == nil {
		return units
	}
	return p.equipmentEffects.ApplyRestoreModifiers(p, units)
}

// applyRunEnergyDrainModifiers is the formula's view of
// applyRunEnergyDrainModifiers (EquipmentEffects.ts:321-325),
// with the same nil-identity rule as
// applyRunEnergyRestoreModifiers.
func applyRunEnergyDrainModifiers(p *Player, units float64) float64 {
	if p == nil {
		return units
	}
	return p.equipmentEffects.ApplyDrainModifiers(p, units)
}

// EquipmentWeightsEventName is the custom-event name for the
// equipment-weights query (Player.ts:1289: PluginManager
// .emitCustomEvent("equipment:weights", request)). Content
// subscribes through Api.OnCustomEvent; the producer is
// Player.CarriedWeightGrams.
const EquipmentWeightsEventName = "equipment:weights"

// EquipmentWeightsRequest is the payload of the
// "equipment:weights" custom event: the player whose weight is
// being computed and the per-slot kg overrides content fills in
// (Player.ts:1288: `{player, slotWeights: {}}`). Handlers
// receive a pointer so their writes are visible to the emitter,
// following the ObjectExamineEvent precedent. SlotWeights maps
// equipment slot index to kilograms (NOT grams): graceful writes
// whole negative kg, worn-weights content writes
// WeightGrams/1000. CarriedWeightGrams applies the TS override
// rule per slot; see weight.go.
type EquipmentWeightsRequest struct {
	Player      *Player
	SlotWeights map[int]float64
}

// EquipmentEffectsEventName is the custom-event name for the
// equipment-effects description query ("equipment:effects",
// EquipmentStats.plugin.js:126-133, describeEffects). Content
// subscribes through Api.OnCustomEvent. There is NO producer in
// Go: EquipmentStats is not ported, so the event never fires and
// a subscriber's handler never runs (a documented gap, not a
// silent one).
const EquipmentEffectsEventName = "equipment:effects"

// EquipmentEffectsRequest is the payload of the
// "equipment:effects" custom event: the player, the description
// lines content appends, the per-slot kg weights content fills
// in (shared with the weights query's shape), and the set-text
// enum ids content appends (EquipmentStats.plugin.js:126-133:
// `{player, lines: [], slotWeights: {}, setTexts: []}`).
// Handlers receive a pointer so their writes are visible to the
// emitter. It has no producer in Go; see
// EquipmentEffectsEventName.
type EquipmentEffectsRequest struct {
	Player      *Player
	Lines       []string
	SlotWeights map[int]float64
	SetTexts    []int
}

// SkillcapeHasPerkEventName is the custom-event name for the
// skillcape-perk query ("skillcape:has-perk",
// skillcapes/Perks.SkillCapes.js:10, answered by
// Common.SkillCapes.js:124-130). Content emits the query through
// Api.EmitCustomEvent and reads Result after the emit returns.
// There are NO handlers in Go: SkillCapes is not ported, so
// Result stays false — the honest no-hooks default. Do not
// invent skillcape content to answer it.
const SkillcapeHasPerkEventName = "skillcape:has-perk"

// SkillcapePerkRequest is the payload of the "skillcape:has-perk"
// custom event: the player, the skill being queried (a Skill or
// its key string in TS — GracefulOutfit passes "AGILITY", so Go
// carries the key string), whether a carried cape counts too
// (Inventory: `inventory: true` counts a carried cape, the
// Prayer cape's holy wrench — Perks.SkillCapes.js:3-6), and the
// answer, which starts false and a handler sets true when the
// worn (or carried, when Inventory) cape gives the skill's perk
// (Common.SkillCapes.js:124-130). The querier reads Result after
// the emit returns; with no handlers it stays false.
type SkillcapePerkRequest struct {
	Player    *Player
	Skill     string
	Inventory bool
	Result    bool
}
