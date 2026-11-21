package game

import (
	"encoding/json"
	"math"
)

// NPC aggression: the port of game/entity/impl/npc/NpcAggression.ts
// and AggressionTolerance.ts.
//
// WHEN IT RUNS. TS runs it from Player.process, once per tick for
// every non-bot player, after movement and before area processing
// (Player.ts:539-542): first the tolerance update, then
// NpcAggression.process. The Go call site is NOT wired here —
// Player.Process lives in player.go, owned by another worker —
// so this file exposes ProcessPlayerAggression, the exact
// Player.ts:539-542 block, for the wiring to call. Until it is
// wired, no NPC aggresses on its own; the decision logic below
// is complete and tested, awaiting its call site.
//
// WHOM NPCs AGGRESS ON. An NPC aggresses when, in TS check order
// (NpcAggression.ts:26-100): the player is not already in
// single-combat; the NPC is alive, shares the player's private
// area, and is not owner-scoped to someone else; no area rule
// forbids it and the definition is aggressive (or an area rule
// forces it); the player has not finished tolerance for it (or
// an area/plugin override keeps it aggressive); the NPC is not
// already in combat; the player is weak enough or in the
// Wilderness (isAggressiveTo); the player is within the
// aggression range measured from the spawn footprint; and the
// player is within the hunt range measured from the NPC's
// current tile. In multi-combat every hunter in range engages
// on the same tick; in single-combat only the first hunter
// takes the player per tick (always-multi NPCs still join).
//
// COMBAT BRIDGE. The engage/canAttack/inCombat calls belong to
// the combat core (Worker 2). They cross through the
// AggressionCombat bridge (see SetAggressionCombatBridge): with
// no bridge installed, CanAttack answers false and Attack is a
// no-op, so aggression can decide but never engages — the
// honest stub state. InCombat falls back to the HasPendingWork
// proxy (see npcInCombat) with no bridge installed.
//
// DEGENERATIONS, each documented again at its site:
//   - Private areas do not exist in Go: the private-area
//     equality check always passes, and the second pass over
//     the player's PrivateArea NPCs never runs.
//   - The plugin npcAggressionTolerance emit has no hub yet:
//     AggressionEnv.EmitNpcAggressionTolerance carries it when
//     the hub installs one; nil means no opinion.
//   - The attack distance comes from the definition's attack
//     type (see attackDistance); a provider-supplied combat
//     method's reach is a combat-factory gap.
//   - Multi-combat resolution needs the world's AreaManager
//     (InMulti): AggressionEnv.InMulti carries it; nil means
//     the area's own isMulti only, with no Wilderness fallback.

// AggressionToleranceAttribute is the persisted tolerance key
// (AggressionTolerance.ts:33, ATTRIBUTE). It is registered in
// the persistent-key registry (Phase 11); the value shape is
// the TS state object (see ToleranceState).
const AggressionToleranceAttribute = "npc-aggression:tolerance"

// ToleranceTicks is TOLERANCE_TICKS (AggressionTolerance.ts:35):
// ten minutes of game ticks spent inside the tolerance regions.
const ToleranceTicks = 1000

// ToleranceRegionRadius is REGION_RADIUS
// (AggressionTolerance.ts:37): a tolerance region reaches this
// many tiles each way from its centre.
const ToleranceRegionRadius = 10

// ToleranceCentre is one tolerance region centre
// (AggressionTolerance.ts:9-13, ToleranceCentre).
type ToleranceCentre struct {
	X, Y, Z int
}

// ToleranceState is the persisted tolerance state
// (AggressionTolerance.ts:15-20, ToleranceState): the older and
// newer region centres plus the ticks spent inside them, up to
// ToleranceTicks.
type ToleranceState struct {
	Older ToleranceCentre
	Newer ToleranceCentre
	Ticks int
}

// AttributeHolder is the get/setAttribute surface
// AggressionTolerance reads and writes (AggressionTolerance
// .ts:29-32, AttributeHolder). *Mobile satisfies it, and *Player
// through its embedded Mobile.
type AttributeHolder interface {
	GetAttribute(key string) any
	SetAttribute(key string, value any)
}

var _ AttributeHolder = (*Mobile)(nil)
var _ AttributeHolder = (*Player)(nil)

// AggressionTolerance is the port of AggressionTolerance.ts:
// OSRS aggression tolerance — two 21x21 regions per player, ten
// minutes inside them making monsters tolerant. Logging out is
// not leaving, so the state is a persisted player attribute.
type AggressionTolerance struct {
	holder AttributeHolder
}

// NewAggressionTolerance returns the tolerance tracker for
// holder (usually a *Player). A nil holder reads as no state
// and drops updates: the Mobile attribute accessors are
// nil-safe, and a nil holder must not panic the tick.
func NewAggressionTolerance(holder AttributeHolder) *AggressionTolerance {
	return &AggressionTolerance{holder: holder}
}

// State returns the stored tolerance state, or false when none
// is stored or the stored value is not a valid state
// (AggressionTolerance.ts:40-43, state()): the state must carry
// both centres and finite ticks. The stored form is the
// map[string]any the TS JSON shape implies (lowercase keys:
// older/newer/ticks, x/y/z), which survives the save/load
// JSON round-trip unchanged; a ToleranceState value or pointer
// (as a test or content might store directly) parses too.
// Numbers parse from int, int64, float64, and json.Number; a
// non-finite tick count invalidates the state, as in TS.
func (t *AggressionTolerance) State() (ToleranceState, bool) {
	if t == nil || t.holder == nil {
		return ToleranceState{}, false
	}
	return parseToleranceState(t.holder.GetAttribute(AggressionToleranceAttribute))
}

// Update advances the tolerance state for tile, once per tick
// (AggressionTolerance.ts:46-60, update): with no valid state
// it seeds both centres on the tile at 0 ticks; inside either
// region it counts up to ToleranceTicks; outside both it drops
// the older region, keeps the newer one, centres a new region
// on the tile, and restarts at 0.
//
// TS mutates the stored state object in place after seeding;
// Go writes back a fresh map holding the same content. The
// difference is unobservable: nothing else holds the stored
// map, and the attribute store swaps the pointer under its
// lock.
func (t *AggressionTolerance) Update(tile Location) {
	if t == nil || t.holder == nil {
		return
	}
	here := ToleranceCentre{X: tile.X, Y: tile.Y, Z: tile.Z}
	state, ok := t.State()
	if !ok {
		t.holder.SetAttribute(AggressionToleranceAttribute, toleranceStateMap(here, here, 0))
		return
	}
	if withinToleranceRegion(state.Older, here) || withinToleranceRegion(state.Newer, here) {
		if state.Ticks < ToleranceTicks {
			state.Ticks++
			t.holder.SetAttribute(AggressionToleranceAttribute, toleranceStateMap(state.Older, state.Newer, state.Ticks))
		}
		return
	}
	state.Older = state.Newer
	state.Newer = here
	state.Ticks = 0
	t.holder.SetAttribute(AggressionToleranceAttribute, toleranceStateMap(state.Older, state.Newer, state.Ticks))
}

// Finished reports whether monsters that build tolerance have
// stopped being aggressive towards this player
// (AggressionTolerance.ts:63-66, finished).
func (t *AggressionTolerance) Finished() bool {
	state, ok := t.State()
	return ok && state.Ticks >= ToleranceTicks
}

// withinToleranceRegion reports whether tile lies within a
// tolerance region centred on centre (AggressionTolerance
// .ts:68-72, within): same plane, at most REGION_RADIUS each way.
func withinToleranceRegion(centre, tile ToleranceCentre) bool {
	dx := centre.X - tile.X
	if dx < 0 {
		dx = -dx
	}
	dy := centre.Y - tile.Y
	if dy < 0 {
		dy = -dy
	}
	return centre.Z == tile.Z && dx <= ToleranceRegionRadius && dy <= ToleranceRegionRadius
}

// toleranceStateMap renders a state in the stored form: the TS
// JSON shape (AggressionTolerance.ts:48, 55-57), lowercase
// keys, so a save/load round-trip returns the same map.
func toleranceStateMap(older, newer ToleranceCentre, ticks int) map[string]any {
	centre := func(c ToleranceCentre) map[string]any {
		return map[string]any{"x": c.X, "y": c.Y, "z": c.Z}
	}
	return map[string]any{"older": centre(older), "newer": centre(newer), "ticks": ticks}
}

// parseToleranceState parses the stored forms State accepts.
// It returns false for anything else, including a state with a
// missing centre or a non-finite tick count.
func parseToleranceState(value any) (ToleranceState, bool) {
	switch v := value.(type) {
	case ToleranceState:
		return v, true
	case *ToleranceState:
		if v == nil {
			return ToleranceState{}, false
		}
		return *v, true
	case map[string]any:
		older, okOlder := parseToleranceCentre(v["older"])
		newer, okNewer := parseToleranceCentre(v["newer"])
		ticks, okTicks := parseToleranceTicks(v["ticks"])
		if !okOlder || !okNewer || !okTicks {
			return ToleranceState{}, false
		}
		return ToleranceState{Older: older, Newer: newer, Ticks: ticks}, true
	default:
		return ToleranceState{}, false
	}
}

// parseToleranceCentre parses one stored centre: a
// ToleranceCentre value or pointer, or a map with x/y/z (either
// case — the stored form uses lowercase, but a hand-built map
// may use the Go field names).
func parseToleranceCentre(value any) (ToleranceCentre, bool) {
	switch v := value.(type) {
	case ToleranceCentre:
		return v, true
	case *ToleranceCentre:
		if v == nil {
			return ToleranceCentre{}, false
		}
		return *v, true
	case map[string]any:
		x, okX := parseToleranceTicks(v["x"])
		if !okX {
			x, okX = parseToleranceTicks(v["X"])
		}
		y, okY := parseToleranceTicks(v["y"])
		if !okY {
			y, okY = parseToleranceTicks(v["Y"])
		}
		z, okZ := parseToleranceTicks(v["z"])
		if !okZ {
			z, okZ = parseToleranceTicks(v["Z"])
		}
		if !okX || !okY || !okZ {
			return ToleranceCentre{}, false
		}
		return ToleranceCentre{X: x, Y: y, Z: z}, true
	default:
		return ToleranceCentre{}, false
	}
}

// parseToleranceTicks parses one stored integer: int, int64,
// float64 (truncated, as a JSON round-trip yields whole floats),
// or json.Number. A NaN or infinite float is invalid, matching
// the TS Number.isFinite check.
func parseToleranceTicks(value any) (int, bool) {
	switch v := value.(type) {
	case int:
		return v, true
	case int64:
		return int(v), true
	case float64:
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return 0, false
		}
		return int(v), true
	case json.Number:
		if n, err := v.Int64(); err == nil {
			return int(n), true
		}
		if f, err := v.Float64(); err == nil {
			return parseToleranceTicks(f)
		}
		return 0, false
	default:
		return 0, false
	}
}

// WildernessChecker is the Wilderness surface isAggressiveTo
// reads (NPC.ts:263-267): IsIn plus the wilderness level at a
// tile. *Wilderness satisfies it. A nil checker reads as "no
// wilderness": the level term is false.
type WildernessChecker interface {
	IsIn(m Located) bool
	LevelAt(x, y int) int
}

// IsAggressiveTo reports whether this NPC can be aggressive to
// player (NPC.ts:263-267, isAggressiveTo): the player's combat
// level is at most twice the NPC's, or the player is in the
// Wilderness above level 0. A player with no SkillManager (a
// zero-value Player) reads combat level 0; NewPlayer always
// installs one.
func (n *NPC) IsAggressiveTo(player *Player, wild WildernessChecker) bool {
	combatLevel := 0
	if player != nil {
		if skills := player.Skills(); skills != nil {
			combatLevel = skills.CombatLevel()
		}
	}
	if combatLevel <= n.Definition().CombatLevel()*2 {
		return true
	}
	if wild == nil || player == nil {
		return false
	}
	loc := player.Location()
	return wild.IsIn(player) && wild.LevelAt(loc.X, loc.Y) > 0
}

// footprintDistance is CombatRange.footprintDistance
// (CombatRange.ts:235-241): the Chebyshev distance from a
// footprint (origin is the south-west corner, extending
// size-1 tiles north and east) to a point. A cross-plane pair
// reads as unreachableDistance, where TS reads
// MAX_SAFE_INTEGER. The size must not be added as a symmetric
// radius: the footprint already extends north and east.
func footprintDistance(origin Location, size int, point Location) int {
	if origin.Z != point.Z {
		return unreachableDistance
	}
	extent := max(size, 1) - 1
	dx := max(origin.X-point.X, 0, point.X-(origin.X+extent))
	dy := max(origin.Y-point.Y, 0, point.Y-(origin.Y+extent))
	return max(dx, dy)
}

// AggressionCombat is the combat-system bridge NpcAggression
// engages through: in-combat checks, the can-attack gate, and
// starting the attack (CombatFactory.inCombat / canAttack and
// Combat.attack). The combat core installs the real bridge;
// with none installed, the default bridge answers in-combat
// through the HasPendingWork proxy, refuses every attack, and
// drops Attack calls (see defaultAggressionCombat).
type AggressionCombat interface {
	// InCombat reports whether m is attacking or being
	// attacked (CombatFactory.ts:1237-1239).
	InCombat(m *Mobile) bool
	// CanAttack reports whether npc may attack target: the
	// CAN_ATTACK answer of CombatFactory.canAttack
	// (CombatFactory.ts:665-681), collapsed to a boolean.
	CanAttack(npc *NPC, target *Player) bool
	// Attack starts npc attacking target (Combat.ts:269,
	// attack).
	Attack(npc *NPC, target *Player)
}

// aggressionCombatBridge is the installed bridge, or nil for
// the default. It is set once at boot via
// SetAggressionCombatBridge, before the world starts ticking,
// and never changed afterwards; like the other
// coordinator-installed wiring in this package it is
// deliberately unsynchronized.
var aggressionCombatBridge AggressionCombat

// SetAggressionCombatBridge installs the combat bridge the
// aggression loop engages through. A nil bridge restores the
// default (decide, never engage). Call once at boot, before
// ticking starts; see AggressionCombat.
func SetAggressionCombatBridge(bridge AggressionCombat) {
	aggressionCombatBridge = bridge
}

// aggressionCombat returns the installed bridge, or the default
// when none is installed.
func aggressionCombat() AggressionCombat {
	if aggressionCombatBridge != nil {
		return aggressionCombatBridge
	}
	return defaultAggressionCombat{}
}

// defaultAggressionCombat is the no-combat bridge: in-combat
// through the HasPendingWork proxy, every attack refused, and
// Attack dropped. Aggression decisions run and are tested
// against it; engagement awaits the combat core.
type defaultAggressionCombat struct{}

func (defaultAggressionCombat) InCombat(m *Mobile) bool {
	return m != nil && m.GetCombat().HasPendingWork()
}
func (defaultAggressionCombat) CanAttack(npc *NPC, target *Player) bool { return false }
func (defaultAggressionCombat) Attack(npc *NPC, target *Player)         {}

// AggressionEnv carries the world surfaces NpcAggression reads
// that live outside the NPC and the player. The zero value is
// usable: nil fields degenerate as documented on each field,
// and the coordinator installs the real surfaces at boot.
type AggressionEnv struct {
	// Wilderness answers the isAggressiveTo Wilderness term.
	// Nil reads as no wilderness.
	Wilderness WildernessChecker
	// InMulti answers AreaManager.inMulti for an actor
	// (AreaManager.ts:109-115). Nil answers the actor's
	// area's isMulti only, with no Wilderness zone fallback.
	InMulti func(actor AreaActor) bool
	// EmitNpcAggressionTolerance is the plugin
	// emitNpcAggressionTolerance hook (NpcAggression.ts:53):
	// true keeps the NPC aggressive despite finished
	// tolerance. Nil means no opinion. The hub installs it.
	EmitNpcAggressionTolerance func(player *Player, npc *NPC) *bool
}

// inMulti resolves env.InMulti, defaulting to the area-only
// check.
func (env AggressionEnv) inMulti(actor AreaActor) bool {
	if env.InMulti != nil {
		return env.InMulti(actor)
	}
	if area := actor.Area(); area != nil {
		return area.IsMulti()
	}
	return false
}

// multiCombatBetween reports whether npc and player fight under
// multi-combat rules (CombatFactory.ts:688-691,
// multiCombatBetween): an always-multi NPC on either side, or
// both actors in multi. The target here is always a player, so
// only the NPC's always-multi flag is read.
func (env AggressionEnv) multiCombatBetween(npc *NPC, player *Player) bool {
	return npc.IsMultiCombat() || (env.inMulti(npc) && env.inMulti(player))
}

// ProcessPlayerAggression runs the Player.ts:539-542 aggression
// block for player: bots skip it; otherwise the tolerance
// update runs, then the aggression pass over the player's local
// NPCs. The call-site wiring (Player.Process) calls this once
// per tick, after movement and before area processing. A nil
// player is a no-op.
func ProcessPlayerAggression(player *Player, env AggressionEnv) {
	if player == nil || player.IsBot {
		return
	}
	NewAggressionTolerance(player).Update(player.Location())
	ProcessNpcAggression(player, env)
}

// ProcessNpcAggression runs the NpcAggression pass for player
// (NpcAggression.ts:15-27, process): a player already in
// single-combat takes no new hunters, and the local NPCs are
// scanned once. The PrivateArea second pass (NpcAggression
// .ts:24-26) never runs: no private areas exist in Go, so the
// player's area is never a PrivateArea and its NPCs are exactly
// the local NPCs. A nil player is a no-op.
func ProcessNpcAggression(player *Player, env AggressionEnv) {
	if player == nil {
		return
	}
	combat := aggressionCombat()
	if combat.InCombat(&player.Mobile) && !env.inMulti(player) {
		return
	}
	runAggression(player, player.LocalNPCs(), env, false)
}

// runAggression scans npcs for hunters that take player this
// tick (NpcAggression.ts:30-100), returning whether a
// single-combat hunter has taken the player (including one
// taken by an earlier pass, via engaged). The checks run in TS
// order; see the file header for the full rule list. Each
// skipped NPC is skipped alone: the rest of the list may still
// engage.
func runAggression(player *Player, npcs []*NPC, env AggressionEnv, engaged bool) bool {
	combat := aggressionCombat()
	area := player.Area()
	singleCombatEngaged := engaged
	tolerance := NewAggressionTolerance(player)
	for _, npc := range npcs {
		if npc == nil {
			continue
		}
		def := npc.Definition()
		// The NPC must be available to attack the player.
		// The hitpoints term reads the Dying flag instead of
		// hitpoints <= 0: Go hitpoints of 0 mean "unset"
		// (see npc.go), and death handling sets Dying when
		// hitpoints reach 0, so a live NPC is never skipped
		// for unset hitpoints. The private-area term always
		// passes: no private areas exist in Go.
		if npc.Dying || npc.Hitpoints < 0 {
			continue
		}
		if npc.IsOwnerOnly() && npc.Owner() != player {
			continue
		}
		// An area rule of false forbids; true forces, and
		// overrides the aggressive flag, the tolerance
		// check, the level check, and the spawn-range check
		// below. A nil rule means no opinion. The Go area
		// methods are called directly, as TS rule-hook
		// callers ask the area directly (see area.go); the
		// callArea isolation seam covers only enter/leave/
		// process/music.
		var rule *bool
		if area != nil {
			rule = area.NpcAggression(player, npc)
		}
		ruleFalse := rule != nil && !*rule
		ruleTrue := rule != nil && *rule
		if ruleFalse || (!ruleTrue && !def.IsAggressive()) {
			continue
		}
		if !ruleTrue && def.BuildsAggressionTolerance() && tolerance.Finished() {
			// Tolerance is per-NPC: skip just this one.
			// The area override wins over the plugin emit
			// (the ?? chain, NpcAggression.ts:52-53); a
			// nil emit means no opinion, which is not
			// "keeps aggressive".
			var override *bool
			if area != nil {
				override = area.NpcAggressionTolerance(player, npc)
			}
			if override == nil && env.EmitNpcAggressionTolerance != nil {
				override = env.EmitNpcAggressionTolerance(player, npc)
			}
			if override == nil || !*override {
				continue
			}
		}
		if combat.InCombat(&npc.Mobile) {
			// An NPC keeps the target it acquired until
			// combat ends: it never re-hunts while it
			// has one, multi or not.
			continue
		}
		if !ruleTrue && !npc.IsAggressiveTo(player, env.Wilderness) {
			continue
		}
		// The NPC's authority to start this fight is its
		// aggression range from the spawn, measured to the
		// spawn footprint (NpcAggression.ts:62-69).
		attackDistance := npc.attackDistance()
		spawnDistance := footprintDistance(npc.Spawn, def.Size(), player.Location())
		if !ruleTrue && spawnDistance > def.AggressionRange(float64(attackDistance)) {
			continue
		}
		// The hunt range starts at the NPC's current tile
		// (NpcAggression.ts:71-74). A cross-plane player
		// reads distance -1 in Go, which must NOT satisfy
		// the range: TS getDistance reads MAX_SAFE_INTEGER
		// across planes.
		huntDistance := npc.Location().Distance(player.Location())
		huntRange := max(def.HuntRange(), attackDistance)
		if huntDistance < 0 || huntDistance > huntRange {
			continue
		}
		if !combat.CanAttack(npc, player) {
			continue
		}
		// In multi-combat every hunter in range engages on
		// the same tick. In single-combat only the first
		// one takes the player: setting its target does not
		// mark the player as under attack until a hit
		// lands, so canAttack cannot tell the others the
		// player is taken. Always-multi NPCs still join,
		// whatever the list order — but an ordinary NPC
		// that engages counts even under multi rules, so a
		// hunter on a single tile next to the multi area
		// must not join it this tick.
		multi := env.multiCombatBetween(npc, player)
		if !multi && singleCombatEngaged {
			continue
		}
		combat.Attack(npc, player)
		if !npc.IsMultiCombat() {
			singleCombatEngaged = true
		}
	}
	return singleCombatEngaged
}
