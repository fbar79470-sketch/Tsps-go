# Phase 16 plan

Migration plan §2 milestone 6 (NPC drivers + combat core),
dispatched 2026-11-15 under Tyler's unattended-progression
authorization (2026-11-14, overnight). No separate per-phase
approval was sought.

## Scope

1. **NPC drivers** — port the TS NPC process drivers as core
   defines them: `NPCMovementCoordinator`
   (`game/entity/impl/npc/NPCMovementCoordinator.ts`, wandering
   + walking home + the radius-0 stationary rule),
   `NpcAggression` / `AggressionTolerance`
   (`NpcAggression.ts`, `AggressionTolerance.ts`, incl. the
   `npc-aggression:tolerance` attribute registered in phase
   11), following, facing/interaction clearing, and the
   `NPC.process()` phase order (`NPC.ts:305-380+`: hit queue,
   timers, movement coordinator, combat pre/post movement,
   interaction-range clearing, `AreaManager.process`). The
   `scriptedMovement` / dialogue-hold guards are part of the
   port. Respawn timing as core defines it. NPCs must stop
   being static. No invented AI.
2. **Combat core** — port `game/content/combat/*` as core
   defines it: `Combat` (target/attacker/hit queue/attack
   cycles, `preMovementProcess`/`postMovementProcess`,
   `hasPendingWork`), `CombatFactory` (damage rolls, accuracy
   / max-hit formulas, protection-prayer terms, special
   traits as core computes them), `CombatMethod` + melee /
   ranged / magic methods, `CombatRange` / `CombatPosition`,
   the hit queue + hitsplat sync through the existing
   player/NPC sync path, death handling (player + NPC death,
   respawn, drop dispatch to hooks), and the combat mobility
   gates (documented gaps since phase 7 — implement exactly
   what TS grounds). Content-delegated parts (NPC combat
   methods via providers) port the provider mechanism, not
   the content. Drops: ground whether core rolls drops or
   plugins do (`npc-drops:roll`); port what core does, hook
   dispatch for the rest. Prayer: port the combat-facing
   prayer surface (protection-prayer checks in the damage
   pipeline, `PrayerHandler` state/drain as core defines it);
   if the prayer-book UI/drain loop depends on the interface
   runtime, document it as a milestone-8 gap with grounding.
3. **Combat listeners** — port the combat-related packet
   listeners from decode-drop to full implementations per
   the TS listeners: the NPC attack option and the player
   attack option. Spell-on-NPC (75) / spell-on-player (32)
   ONLY if TS core routes them through the combat system;
   if spellcasting belongs to a later milestone, leave them
   decode-drop and document the boundary with grounding.
   Item-on-NPC (36) stays as the plan directs: it was a
   phase 14 gap pending `walkToEntity` — if this phase ports
   walk-to-entity as part of combat targeting, wire
   item-on-NPC too; otherwise leave it and document why.
4. **Hub exposure** — the combat/NPC hook families from the
   108-family table (`onNpcDeath`, `onPlayerDeath`,
   `onCombatHitPrepare/Roll/Damage/Resolved`,
   `onCombatAttackDistance`, player-death drop events —
   ground the actual TS names in `PluginTypes.ts`) with
   registration methods + production emit sites, and
   `registerNpcCombatMethodProvider` (PluginTypes.ts:1548,
   PluginManager.ts:4926/5241) with the provider registry
   the combat factory consults (CombatFactory.ts:174).
   Update the hub table counts. `game` never imports
   `plugins`: providers and event hooks cross the boundary
   through game-side registries/seams populated by cmd/server
   wiring, following the phase 15 area queue/flush pattern.
5. **XP decision (decided with grounding, see below)** —
   combat grants XP through the existing `SkillManager` in
   this phase. `CombatFactory.ts:1181-1225` grants XP
   directly in hit resolution (`addExperience` for Magic /
   Hitpoints / Defence / offensive skills, `OSRS_DAMAGE_XP`
   scaling) — combat XP is part of the combat core, not of
   milestone 10, which covers skilling shared handlers and
   skilling XP sources (`SkillActions.js` pattern).
   Consequence: killing things grants combat XP; skilling XP
   sources remain a milestone-10 gap.
6. **Save/state implications** — verify from TS whether any
   combat/NPC-driver state persists; handle through the
   owned/preserved split, no owned fields without grounding.
   Document explicitly if nothing persists (expected:
   combat/driver state is session-only; drops become ground
   items, which already persist only as runtime state).

## Out of scope

Shops/trading (milestone 7), interface runtime (8),
clans/social (9), skill handlers + XP sources (10) except
the item-5 combat-XP decision above, sailing/template
regions/sounds (11), content ports beyond proof (a test-only
combat-method provider hook is fine; SeekingArrows or any
other real content port is NOT this phase), the pre-existing
far-WALK routing bug (recorded in PHASE12_PLAN.md — use
staged clicks in the smoke test), embedded JS/sidecar,
scrypt check, MobileList.All perf work.

## Verification

`go build ./...`, `go vet ./...`, `gofmt -l .` clean,
`go test ./... -p 1 -count=1` + `-race`, CGO_ENABLED=0 build,
and a live smoke test: a spawned NPC wanders from its spawn
tile over several ticks; an aggressive NPC approaches/attacks
a nearby player (hitsplat frames observed); a player
attacking an NPC deals damage, the NPC dies, and respawns; a
hub hook observes the death event. Claim "NPCs move and
fight" only if verified live end to end.

## Outcome

COMPLETED 2026-11-15, verified: build/vet/gofmt clean incl.
CGO_ENABLED=0, 2,757 test cases pass incl. subtests across 12
packages, race clean, live smoke test 3 runs stable (see
PORT_STATUS.md). NPC drivers (wander/aggression/follow/retreat/
respawn), the combat core (attack cycles, melee/ranged/magic
methods, exact damage formulas, hit queue + hitsplat sync,
death/respawn, prayer, specials registry, combat XP), the NPC
+ player attack listeners, and 11 hub combat families
(34/108 firable, 15/95 members) all landed. Two findings from
verification: (1) eager per-NPC RouteFinder allocation
(~337KB/NPC, ~10GB for the 30k spawns) OOM-killed the server
at boot — fixed by lazy allocation, with regression tests;
(2) a pre-existing map-data wall (two solid clipped rows at
y=3521-3522 spanning x=3038..3347, no gate) blocks walks from
the spawn side to the spider area — documented, not fixed
(out of scope; needs TS/cache grounding, same family as the
far-WALK bug). Spell listeners, item-on-NPC, and combat-style
varp 43 stay decode-drop with grounding; drops stay content's
job (no `npc-drops:roll` in TS core).
