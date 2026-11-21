# Phase 24 plan — Seeking Arrows combat content (migration plan §3 item 4)

Status: COMPLETED (verified 2026-11-16; dispatched under Tyler's
"keep going without confirming" instruction; no per-phase
approval sought).

## Scope

Migration plan §3 item 4, the smallest real combat-modifier
content: port `server/plugins/items/SeekingArrows.plugin.js`
(21 lines, self-contained) to one Go content package under
`internal/content/`, following the
`dateminutes`/`gracefuloutfit`/`lumbridgeguide` precedent
(compile-time registration via one blank import in
`cmd/server/pluginwiring.go`). Porting rules are migration plan
§4 (read before implementing): faithful port with file:line
citations, fixtures-only unit tests, live verification before
claiming behaviour, honest gaps in PORT_STATUS.md, no copied
`*.plugin.js` files, hub registration via `internal/plugins`.

Out of scope: every other content port (quests, skills, bosses,
specials), the `combat_ranged.go` projectile divergence fix
(grounding only if encountered), the far-WALK bug, the
y=3521–3522 map wall, the real production scrypt-hash check,
perf work. No scope expansion without TS grounding. No Google
Drive upload: the parent handles the post-phase archive.

## Upstream grounding (verified by the coordinator, not trusted from summaries)

### SeekingArrows.plugin.js (all 21 lines)

- Header comment (line 1): "Seeking dragon arrow raises
  successful bow hits to at least three; special minima do not
  stack."
- `seekingMinimum` (lines 4–11): destructures `attacker`,
  `damage`, `accurate` from the event (line 5). Guard (line 6):
  return unless accurate, the attacker is a player
  (`attacker?.isPlayer?.()`, null-safe), and
  `event.combatType === core.CombatType.RANGED`. `getAsPlayer`
  (line 7). Return when a special is activated (line 8,
  `player.isSpecialActivated()` — the "special minima do not
  stack" half). Read the ammunition (line 9,
  `player.getCombat().getAmmunition()`); when it is exactly
  `core.Ammunition.SEEKING_DRAGON_ARROW`, raise the damage
  (line 10, `damage.setDamage(Math.max(3, damage.getDamage()))`).
- `module.exports` (lines 13–21): `name: "SeekingArrows"`
  (line 14), `description: "Makes successful bow hits with
  seeking dragon arrows deal at least three damage."`
  (line 15), `members: true` (line 16), `register` (lines
  17–20): `core = api.core` (line 18), one attach-only line
  `api.onCombatHitDamage(seekingMinimum)` (line 19). No
  `dependsOn`, no config declarations, no custom events.
- No weapon/bow check exists upstream: the only gates are
  accuracy, player attacker, RANGED combat type, no active
  special, and the ammunition identity. The "bow hits" wording
  in the description is descriptive, not a check — crossbows
  cannot fire seeking arrows because the ammunition data does
  not list them, not because this handler tests the weapon.

### Supporting TS grounding

- `Ammunition.SEEKING_DRAGON_ARROW` is built from
  `ItemIdentifiers.SEEKING_DRAGON_ARROW` (RangedData.ts:43),
  which is item id 33595 (ItemIdentifiers.ts:31169). A second
  constant, `SEEKING_DRAGON_ARROW_2 = 33596` (the noted form),
  exists but the Ammunition singleton uses 33595 only.
- `Combat.ammunition: Ammunition | null = null`
  (Combat.ts:265); `getAmmunition()` (Combat.ts:801) returns
  null when no ammunition is resolved.
- `PluginCombatHitDamageEvent` (PluginTypes.ts:647–655):
  `attacker`, `target`, `combatType`, `hit`, `damage`,
  `accurate`. `onCombatHitDamage` (PluginTypes.ts:1152).
- `emitCombatHitDamage` (PluginManager.ts:1803–1808): guards
  attacker/target/damage (no hit guard); runs ALL hooks in
  registration order.
- `PendingHit.emitHitDamage` (PendingHit.ts:527–541) fires
  BEFORE `capToRemaining` (emit call sites 449 and 511, caps
  at 453 and 515): the floor applies first, then the splat
  caps to the target's remaining hitpoints. `accurate` is
  written back after the emit; `damage` is mutated in place.
- `Mobile.isPlayer()` (Mobile.ts:967, `instanceof Player`),
  `getAsPlayer()` (Mobile.ts:981, null when not a player).
- `CombatType` enum order MELEE, RANGED, MAGIC
  (CombatType.ts:1–5).
- `HitDamage.setDamage` (HitDamage.ts:53–56): assign, then
  `update()` (which floors at 0 and fixes the mask).

## Go seam analysis (verified, not assumed)

- The `onCombatHitDamage` family is firable since phase 16.
  Game seam `Hooks.OnCombatHitDamage` (combat_hooks.go) →
  cmd/server adapter (combatwiring.go:454) →
  `Hub.EmitCombatHitDamage` (plugins/combat.go:857) with
  TS-identical entry guards. The game emit points
  (combat_hitqueue.go:758/842) fire before `capToRemaining`
  (762/851), matching the TS order above. The plugin event's
  `Damage` is `any` holding the shared `*game.HitDamage`;
  `HitDamage.SetDamage`/`Damage()` mirror TS
  (combat_hitqueue.go:67–75).
- `CombatType` mapping is a direct cast; game and plugin
  orders both ground CombatType.ts (combatwiring.go:300–306).
  `CombatTypeRanged` exists (plugins/combat.go:133).
- `Player.GetCombat().IsSpecialActivated()`
  (combat.go:1156) and `Ammunition()` (combat.go:1113) exist;
  `AmmunitionData` carries `ItemID` (combat_ranged.go:50).
- GAP 1 — seam to build (minimal, grounded): `Mobile` has no
  kind predicate or back-pointer (stated in the
  `findCombatNPC` comment, combatwiring.go:308–315), so
  content cannot ask "is this Mobile a player, and which
  `*Player`". Build `Mobile.IsPlayer()`/`AsPlayer()` in
  `internal/game`, backed by the existing `combatantFor`
  identity mechanism (combatant.go:408), mirroring TS
  `Mobile.isPlayer()`/`getAsPlayer()`. Fail-closed for
  unbound or nil mobiles (the `unknownCombatant` adapter).
  `isNpc`/`getAsNpc` stay unported; nothing in this phase
  needs them.
- GAP 2 — document, do not build: no ranged-data resolver is
  registered in production (`RegisterRangedDataResolver` has
  zero callers), so no ranged weapon is ever detected and
  every fighter attacks in melee (combat_ranged.go header).
  The handler therefore cannot fire in production until
  ranged weapon/ammunition data is ported. The live smoke
  test registers a test-only resolver in the throwaway
  harness to prove the pipeline plus handler end to end. No
  production resolver is invented here: the ranged data
  tables are a separate content-data port.
- No new hub surface: `OnCombatHitDamage` is already
  firable. Hub counts stay 45 of 108 families firable, 26
  of 95 non-hook members, 12 of 191 Core members.

## Implementation

1. `internal/game/combatant.go`: `Mobile.IsPlayer()` /
   `AsPlayer()` + unit tests (player-bound, NPC-bound,
   unbound, nil). Update the stale `findCombatNPC` comment
   to stay accurate (Mobile now carries a player predicate;
   still no NPC back-pointer).
2. `internal/content/seekingarrows/seekingarrows.go`: the
   `Plugin` value (name, description, members flag verbatim),
   attach-only register, and the `seekingMinimum` handler in
   the TS check order. Package comment carries the event
   inventory (listens to `onCombatHitDamage` only, emits
   nothing), the resolver gap, and the two deviations forced
   by the language change: `Damage` arrives as `any` and is
   type-asserted to `*game.HitDamage` (fail-closed otherwise),
   and the ammunition identity check compares `ItemID`
   against 33595 (the TS singleton has no Go form;
   `AmmunitionData` identity is pointer-based and content
   does not hold the singleton).
3. `internal/content/seekingarrows/seekingarrows_test.go`:
   fixtures-only. Guard table (inaccurate, NPC attacker,
   nil attacker, melee/magic combat type, special active,
   wrong ammunition, nil ammunition, nil damage, non-
   `HitDamage` damage) plus floor cases (1→3, 2→3, 3→3,
   5→5), plugin identity/members, hub registration through a
   members world, and free-world members gating.
4. `cmd/server/pluginwiring.go`: one blank import
   (alphabetical, between lumbridgeguide and wornweights).
   Update the members-world expectation in
   `pluginwiring_test.go`; free-world and switch expectations
   are unchanged (members-gated out).
5. Docs: PORT_STATUS.md (header, verification numbers,
   content inventory row + paragraph, gaps, next-modules
   order), CONTENT_MIGRATION_PLAN.md (§3 item 4 DONE +
   boundary + residual gaps).

## Verification

- `go build ./...`, `go vet ./...`, `gofmt -l .`,
  `CGO_ENABLED=0 go build ./...`, `go test ./... -p 1
  -count=1` (exact passing-case count incl. subtests),
  `go test ./... -p 1 -count=1 -race`, all measured by the
  coordinator.
- Live smoke test, 3 runs stable: temp root outside the
  repo, real rev-242 cache, throwaway test + test-only
  ranged resolver, deadline-free readers, handshake after
  login. A player with a bow and seeking dragon arrows
  attacks an NPC: accurate sub-3 hits land at the 3-damage
  floor on the wire; a control run with non-seeking arrows
  shows 1–2 hits as before. Throwaway files removed
  afterwards; zero probe references remain (grep-verified).

## Outcome

COMPLETED 2026-11-16, all scope items landed:

1. `internal/content/seekingarrows/seekingarrows.go`: the
   `Plugin` value (name, description, members flag verbatim),
   attach-only register, and `seekingMinimum` in the TS guard
   order. The two language-change deviations are documented
   at the site (opaque `any` damage assertion, fail-closed;
   `ItemID` comparison against 33595).
2. Seam: `Mobile.IsPlayer()`/`AsPlayer()` in
   `internal/game/combatant.go`, backed by `combatantFor`;
   the `findCombatNPC` comment updated. The
   `onCombatHitDamage` pipeline needed nothing else: the
   Phase 16 emit points fire before `capToRemaining`, as in
   TS, and the live runs confirm the floor-then-cap order
   (life-final hits cap to remaining HP).
3. Event inventory: listens to `onCombatHitDamage` only
   (firable since Phase 16, producer: the combat hit
   pipeline), emits nothing. Documented in the package
   comment.
4. No new hub surface. Counts unchanged: 45 of 108 families
   firable, 26 of 95 non-hook members, 12 of 191 Core.
5. Docs: this plan, PORT_STATUS.md (header, verification
   numbers, P3 row now 5 of 577, Phase 24 inventory
   paragraph, next-modules order), CONTENT_MIGRATION_PLAN.md
   (§3 item 4 DONE + boundary + residual gaps).

Verification (all measured by the coordinator): `go build
./...`, `go vet ./...`, `gofmt -l .`, `CGO_ENABLED=0 go
build ./...` clean; 604 `.go` files, 17 packages;
`go test ./... -p 1 -count=1`: 3,799 passing cases incl.
subtests (3,780 + 19 new), 0 failures, 0 skips;
`go test ./... -p 1 -count=1 -race` passes. Live smoke
test, 3 runs stable over 2 invocations (throwaway harness,
real cache, test-only resolver, NPC-sync decoder, removed
afterwards, zero probe references by grep): seeking-arrow
lives `[3 2]` throughout, rune-arrow control lives all
1s/2s, each life summing to the goblin's 5 HP.

Two harness findings worth keeping: the goblin's cache
actions are `["", "Attack", ...]`, so the Attack click is
option 2 (opcode 12), not option 1 — an option-1 click
hits the empty slot and never fights. And the killing
frame's health entry shows FULL health (death resets HP
before the flush snapshot), so goblin lives bounded by
respawn additions — not the health entry — classify
killing blows. The latter is an ungrounded Phase 16 combat
observation, noted here only; no fix attempted.

Residual gaps (unchanged from the plan): no ranged-data
resolver in production (the handler cannot fire until
ranged weapon/ammunition data ports); `isNpc`/`getAsNpc`
unported. Out-of-scope items untouched: the
`combat_ranged.go` divergence, the far-WALK bug, the
y=3521–3522 wall, the scrypt-hash check, perf work.
