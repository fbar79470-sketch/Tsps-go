# Phase 29 plan — combat presentation

Status: COMPLETED 2026-11-17 (three workers + coordinator
integration + coordinator live re-verification). Combat is
finished as a planned engine slice — see Outcome below.

## Why this phase exists

Phase 26's next-slice brief, repeated in Phase 27 and Phase 28:
"Phase 29 — combat presentation: `NpcAnimationScanner`,
animation/graphic producers, melee/magic projectile producers,
equipment/NPC-combat sound pickers — `StartCast` is a deliberate
no-op until then." This is the last of the planned combat slices.
After it, content ports resume piece by piece per
`CONTENT_MIGRATION_PLAN.md` §3, with Tyler's direction.

## Grounding correction (read first)

The brief's shorthand "the scanner that picks NPC combat
animations (attack/block/death) from the NPC combat
definitions" conflates two separate TS mechanisms. The
grounded facts at `bfd81e3`:

1. `NpcAnimationScanner.ts` does NOT pick NPC animations. It
   decodes the seq (animation) config archive into
   `SequenceDefinition{priority (opcode 10), forcedPriority
   (opcode 5), clientCycles}` and exposes three runtime
   getters — `getSequencePriority` (seq priority; 1 means
   the client drops the seq on a step), `getSequenceForcedPriority`
   (-1 → 5; attacks are 6, blocks 5, so a block never cuts a
   swing short), `getSequenceStall` (ceil(clientCycles / 30)
   ticks) — plus `preloadSequences`, `getLastSequenceId`, and
   `findNpcRigAnimations`, an async rig scan by skeleton
   that has NO callers in the server tree (verify against
   `scripts/` before porting it; if it is tooling-only it is
   NOT a runtime port).
2. NPC attack/block/death animations come from the NPC
   definition fields (`NpcDefinition.ts`: `attackAnim`,
   `defenceAnim`, `deathAnim`, `spawnAnim`, `deathSound`,
   `deathTicks`), read via `NPC.getAttackAnim()` /
   `getBlockAnim()` (`NPC.ts:677-685`). Player attack/block
   animations come from the weapon definition (`ItemDefinition.ts:253,308`)
   and the fight-type registry (`WeaponProfile.ts:111-116`
   `attackAnimation`, `FightType` animation).
3. The animation state machine is `Mobile.performAnimation`
   (`Mobile.ts:369-392`): priority rules (a higher-priority
   existing animation wins; equal priority + new id ≥ 0 +
   not the reset anim (65535) + existing id ≥ 0 → compare
   `getSequenceForcedPriority`), `Animation{id, delay,
   priority LOW/MED/HIGH}` (`model/Animation.ts`), and
   `performGraphic` / `performGraphicInSlot` (`Mobile.ts:394-419`).
   `getSequenceStall` is consumed at `Combat.ts:198`,
   `getSequencePriority` at `Player.ts:450`,
   `getSequenceForcedPriority` at `Mobile.ts:385`.

## Grounded shape (all at bfd81e3; do not re-derive, verify)

### Foundations (Worker A)

- **Seq decoder** — port `NpcAnimationScanner.ts`
  `decodeSequence` + `getSequenceFrameIds` + the three
  getters + `preloadSequences` (called from
  `GameBuilder.ts:2`, so Go wires it at boot). Opcode
  walk: 1 = frames (clientCycles = sum of frame lengths;
  skeletal sequences use opcodes 13/14/15/16 per revision
  — rev 226/220 branches, target rev is 242 so rev226 =
  true, rev220 = true), 5 = forcedPriority, 10 = priority,
  unknown opcode ends the walk at defaults. One tick =
  30 client cycles. The decoder reads the seq config
  archive from the cache — ground the Go cache codec's
  config-archive access first (`internal/cache`); if the
  codec cannot serve seq files, that is a finding, not an
  invention. Wire the stall source: Go already has
  `SetSequenceStallSource` (`combat.go:1281`, nil until
  wired) feeding `OnAnimationStarted` /
  `ExpectAttackAnimation` (Phase 16) — install
  `getSequenceStall` as the source.
- **Mobile animation/graphic state** — Go has NO
  `performAnimation` / `performGraphic` and NO Mobile
  animation/graphic fields (verified: no matches in
  `internal/game`). Port `Mobile.performAnimation` /
  `performGraphic` / `performGraphicInSlot` /
  `delayedAnimation` with the TS priority rules, using
  the seq decoder for forced priority. The sync encoders
  ALREADY support the output: `protocol/sync.go`
  Animation mask (player 0x08, NPC 0x10) + `AnimationView`
  and `Graphics []GraphicView` — but NOTHING populates
  them today (no `AnimationView{` constructions outside
  tests; `flush.go` `createPlayerView`/`createNpcView`
  never set them). Add the animation/graphic fields to
  the sync snapshots and wire them in `flush.go`. Ground
  the per-tick clearing (TS `Mobile.ts:272-273` resets
  animation/graphic — read the site for the exact
  phase; the sync snapshot must clear after flush).
- **NPC definition anim fields** — verify whether Go's
  NPC definition decoder already carries `attackAnim` /
  `defenceAnim` / `deathAnim` / `spawnAnim` /
  `deathSound` / `deathTicks` (Go's `ItemDefinition`
  already has `AttackAnim(attackType)` at
  `definition/item.go:369`; the NPC side was NOT found
  by grep — check `internal/definition/npc*.go` and
  extend the decoder if the fields are missing, grounded
  in `NpcDefinitionLoader.ts`).
- **Accessors** — `Mobile.GetAttackAnim(target)` /
  `GetBlockAnim()` / `GetDeathAnim()` mirroring
  `Player.ts:333-348` / `NPC.ts:677-685` (player: weapon
  def / fight-type via `WeaponProfile.attackAnimation`;
  NPC: current definition). Go's `FightType` already
  carries `Animation` / `AnimationAgainst` /
  `Sound` (`combat_types.go:136-149`) — reuse, do not
  re-derive.

### Animation/graphic producers (Worker B)

- **Melee start** (`MeleeCombatMethod.ts:9-14`):
  `character.getAttackAnim(target)` → performAnimation
  (skip when -1) + attack sound (Worker C's picker,
  called via contract S1 — B inserts the call, C owns
  the picker).
- **Ranged start** (`RangedCombatMethod.ts:133-183`):
  attack anim (:136), launch graphic (:143),
  start/double-start graphic (:145), draw graphic
  (:158, `GraphicHeight.ARCHER_DRAW`), projectile
  (already wired in Go, Phase 26 — do not rewire),
  fire sound (contract S2). Ground the exact graphic
  ids/heights from `RangedData.ts` / `WeaponProfile.ts`
  — Go's `RangedProjectileProfile` already carries the
  profile shape (`combat_ranged.go:45-84`).
- **Magic start** (`MagicCombatMethod.ts:268-282` +
  `CombatSpell.startCast`, `CombatSpell.ts:50-101`):
  attack anim (:268), cast projectile (:274: delay 0,
  speed 20, heights 43/31 — reuse Go's
  `CreateProjectile`, the Phase 26 ranged pattern),
  cast sound (contract S3). `StartCast` (all three
  spell kinds are no-ops today at
  `combat_spells.go:769,826,1108`) goes live:
  `castAnimationFor` (staff-variant map
  `STAFF_CAST_ANIMATIONS`), start graphic (NPC ids
  2000/109/3580/2007 excluded — ground the list, do
  not extend it), `castProjectile` + `sendProjectile`,
  queued end-graphic via `PROJECTILE_VISUAL_CONTEXT`
  for legacy-magic timing (player caster, NORMAL
  spellbook, `hitDelay() == null`, radius 0).
- **Magic impact** (`MagicCombatMethod.ts:196-210`,
  `:358-372`): end graphic on the target, else the
  splash graphic; impact sound (accurate) or
  `SPELL_FAIL_SPLASH` (227) — sounds via contract S3,
  graphics by B. NPC casts use the same path
  (default projectile 91, `DefaultNPCMagicProjectile`,
  already a Go constant).
- **Block animations** — `Combat.ts:1054-1055` (melee
  hits, `method.playsBlockAnimation() !== false`) and
  `CombatFactory.ts:1036-1041`: target performs its
  block anim when `getBlockAnim() >= 0`. Go's
  `BaseCombatMethod.PlaysBlockAnimation()` already
  returns true (`combat_method.go:98`). Find Go's
  equivalent hit-application sites and wire both.
- **Death animations** — NPC: `NPCDeathTask.ts:44`
  plays the definition death anim. Player: death
  animation 836, recorded in Phase 28 but never sent
  (gap 15g) — send it from the case-4 death path in
  `combat_death.go` (the `PlayerDeathTask` case-4
  order: combat reset → animation 836 → `player:dying`
  event). Ground the exact insertion point in
  `PlayerDeath.ts` first.

### Sounds (Worker C)

- **Sound table** — `Sound.ts` is a 153-entry enum of
  `new Sound(id, loopType, delay, volume)` (spot-verified
  `SHOOT_ARROW` 2702, `WEAPON_WHIP` 2720,
  `SPELL_FAIL_SPLASH` 227). Script-extract it verbatim
  (Phase 26 ranged-table precedent); ids from the
  file, never by hand. Named constants for the sounds
  combat uses; the full table is the lookup.
- **Send semantics** — `Sounds.ts`: `sendSound`
  (player → private `sendSoundEffect`; NPC → area
  sound at its tile) and `playAreaSoundAt` (radius;
  melee specials use range 5 per the `Sounds.ts`
  comment). Go already has the send path
  (`internal/game/sounds.go`, `session/sound_sender.go`
  `SendSound`/`SendSoundEffect`, `EncodeSound` 131) —
  verify what `sendSound` vs `playAreaSoundAt`
  maps to in Go and port only the missing seam.
- **Pickers (C owns, B calls)** — melee attack sound
  (`Mobile.getAttackSound`: player =
  `WeaponProfiles.get()?.attackSound ?? fightType.getAttackSound()`
  (`Player.ts:339-341`); NPC = `NPC.ts:681` — ground
  it), ranged fire sound (profile `singleFireSound` /
  `fireSound`, else `SHOOT_ARROW`), magic cast sound
  (`spell.castSound() ?? resolveCastSound(spellId)`)
  and impact sound (`spell.impactSound() ??
  resolveImpactSound(spellId)`) with the TS
  resolve maps, NPC death sound
  (`NpcDefinition.getDeathSound` — ground where TS
  plays it before wiring it).
- **Call sites** — B inserts the calls (contracts S1–S3
  below); C implements the pickers and verifies each
  site fires in the live smoke.

### Melee projectiles

Melee has NO projectile in TS (`MeleeCombatMethod.ts`
has no `createProjectile` call — verified by grep).
Do NOT invent one; document it as verified-absent.
Magic projectiles are the only new producers (B);
ranged already exists (Phase 26).

## Scope

1. **Scanner + foundations (Worker A).** Seq decoder
   (`decodeSequence`, three getters, `preloadSequences`,
   boot wiring), `SetSequenceStallSource` install,
   Mobile animation/graphic state + `performAnimation` /
   `performGraphic` (+ slot + delayed) with TS priority
   rules, snapshot + `flush.go` wiring, NPC definition
   anim fields (extend decoder if missing), attack/block/
   death accessors.
2. **Producers (Worker B).** Melee/ranged/magic start
   animations + graphics, block animations at both call
   sites, NPC + player death animations (incl. 836),
   `StartCast` live (all three spell kinds), magic
   projectile producer, magic impact graphics.
3. **Sounds (Worker C).** 153-entry Sound table,
   send/area semantics, pickers (attack / fire / cast /
   impact / death), verified at every combat call site.
4. **Hub exposure** only as the port requires it,
   grounded in TS. Update hub counts honestly if they
   change.
5. **Docs:** this plan (outcome at the end),
   `PORT_STATUS.md` (header verification numbers,
   inventory, hub counts if changed, gaps,
   next-modules order), `CONTENT_MIGRATION_PLAN.md`
   (combat-completion progress + residual gaps).
   Workers do NOT edit the docs files — the
   coordinator integrates them from worker reports to
   avoid merge conflicts.

## Cross-worker contracts (do not deviate without telling the coordinator)

- **A1 — Animation/graphic state (A owns, B calls):**
  `Mobile.PerformAnimation(id, delay)` /
  `PerformGraphic(id, height, delay)` (or the exact
  signatures A defines in its report — B and C code
  against A's reported signatures; A reports them
  first via the coordinator if the shapes change) +
  `GetAttackAnim(target)` / `GetBlockAnim()` /
  `GetDeathAnim()` on Mobile. B never sets animation
  state directly. A wires the seq decoder behind
  `SetSequenceStallSource` and the sync views.
- **S1/S2/S3 — Sound pickers (C owns, B calls):**
  `AttackSound(attacker) Sound`,
  `FireSound(attacker, profile) Sound`,
  `CastSound(spell) Sound` / `ImpactSound(spell) Sound`
  in `internal/game` (exact signatures in C's report;
  B inserts one call per site, C implements). C owns
  the Sound table + send semantics; B owns the
  call-site insertion points. If C's pickers are not
  ready when B integrates, B lands the animation
  work with the sound calls as clearly-marked TODOs
  and tells the coordinator — do not stub sounds
  silently.
- **C3 — Death (B owns the path, C grounds the
  sound):** death animation 836 + NPC definition death
  anim go through B's death-path work in
  `combat_death.go` / the NPC death task; C reports
  whether TS plays a death sound and where, and B
  inserts the call if it does.
- **C4 — File ownership:** A: `internal/cache/seq*.go`
  (new), `internal/game/mobile_animation.go` (new),
  `internal/definition/npc*.go` (decoder only),
  `internal/session/flush.go` (view wiring only),
  `internal/game/combat.go` (stall-source install
  only). B: `internal/game/combat_melee.go`,
  `combat_ranged.go` (anim/graphic additions only —
  projectiles already exist),
  `combat_magic.go`, `combat_spells.go` (StartCast),
  `combat_factory.go` / `combat.go` (block sites),
  `combat_death.go` (death anims). C:
  `internal/game/sound_table.go` (new),
  `internal/game/sounds.go` (semantics only),
  `internal/game/combat_sounds.go` (new pickers).
  Anything else: ask first.
- **C5 — No new data imports.** The approved verbatim
  imports are music-areas, item-combat-styles,
  item-alch, item-gameplay. Any further data import
  needs Tyler's approval; do not extend the precedent.

## Out of scope (do not start, document as gaps)

Ancients/Lunar/Arceuus spellbooks; all quest work;
graves; the far-WALK bug; the y=3521–3522 map wall;
the real production scrypt-hash check; perf work;
`findNpcRigAnimations` if it proves tooling-only
(document, do not port as runtime); full animation
stalls (content, per the scanner's own comment —
only the partial stall via `getSequenceStall` is
core); `emitAnimationStall` hook (not ported,
`combat.go:1292` documents it). If the scope exceeds
one phase, land a coherent verified subset and
document the remainder precisely as the next phase's
brief. This is the last planned combat slice — if
combat is fully finished, say so with the evidence;
if anything remains, name it precisely instead of
claiming completion.

## Verification (same pattern as phases 16–28)

`go build ./...`, `go vet ./...`, `gofmt -l .`,
`CGO_ENABLED=0 go build ./...`,
`go test ./... -p 1 -count=1` (4,032 tests before this
phase — must not drop), `go test -race`, plus a live
smoke test against the real cache (reuse
`/home/hatch/smoke27/` + `/home/hatch/smoke28/`,
seed cache at
`/home/hatch/smoke27/seed-root/caches/osrs-242_2026-11-03`),
3 runs stable: a melee attack playing the weapon's
attack animation + block animation on the target, a
ranged shot with animation + projectile, a Wind Strike
with cast animation + projectile + impact graphic,
a death playing animation 836 (player) / the
definition death anim (NPC), and the attack/cast/
impact sounds on the wire. Update this plan and
`PORT_STATUS.md`; no Drive upload (parent's job).

## Outcome (coordinator, 2026-11-17)

All six scope items landed; combat is finished as a
planned engine slice (four finish-combat slices: 26
ranged, 27 spells, 28 poison/skulls/death, 29
presentation). What remains is residual gaps (below),
not a fifth slice.

1. **NpcAnimationScanner** (Worker A): `internal/cache/
   seq_scanner.go` — `SequenceScanner` decodes the seq
   config archive (`Priority`/`ForcedPriority`/`StallTicks`
   = ceil(clientCycles/30), `Preload`, `LastSequenceID`,
   `SequenceFrameIDs`) — real-cache spot checks: last seq
   15173; 422 punch forced 6 / stall 2; 424 block forced 5
   / stall 3; 836 death forced 10 / stall 669 (capped to 5
   by `OnAnimationStarted`); 1658 whip forced 6 / stall 2.
   `findNpcRigAnimations` NOT ported as runtime (tooling-only;
   only caller is the AdminCommands rig-scan command).
   `Mobile.performAnimation`/`PerformGraphic` + slot-0
   flush wiring (`internal/session/flush.go`,
   `cmd/server/syncsource.go`), NPC def anim accessors,
   `InstallSequenceAnimations` at boot
   (`cmd/server/combatwiring.go`).
2. **Animation/graphic producers** (Worker B): melee start
   (attack anim + sound), ranged start (attack anim, NPC
   draw graphic 96, player launch/start graphics, fire
   sound), magic start (`StartCast` live on all three spell
   kinds: staff-variant anims, start graphic, cast
   projectile, queued end-graphic via
   `PROJECTILE_VISUAL_CONTEXT`; NPC no-spell branch live;
   TeleBlock hit/miss projectiles 1299/1300; Charge
   presentation anim 811 / graphic 111 / sound 1651),
   magic impact (end graphic or splash 85, area sound
   radius 10, primary hits only, no replay), block anims
   (melee + `ExecuteHit`), death anims (player 836 +
   `player:dying` custom event; NPC definition death anim).
   Melee projectiles verified ABSENT in TS — none invented.
3. **Melee/magic projectile producers**: magic live (see
   above); melee has none in TS (verified, not a gap).
   Ranged launch graphics were the one open item at worker
   handoff — the coordinator closed it: all 193 ammunition
   kinds now carry script-extracted start/double-start
   graphics (82 with a start, 10 with a double; extractor
   `/tmp/extract_ranged_graphics.py`, verified by name/item/
   projectile match + spot-checks incl. bronze 19/1104,
   rune 24/1109, dragon 1116/1111), plus the three
   WeaponProfile.ts flags (venator launchGraphic 2289, dark
   bow doubleStartGraphic, blowpipes suppress).
4. **Sound pickers** (Worker C): the 153-entry `Sound.ts`
   table was already ported in phase 21 (pinned by a
   regression test); `internal/game/combat_sounds.go`
   pickers (`AttackSoundFor`, `FireSoundFor`,
   `CastSoundFor`, `ImpactSoundFor`) + a zero-sound guard in
   `SendSound`. `DeathSoundFor` is definition-only in
   practice — TS plays NPC death sounds from
   `npc-combat-sounds.json`, absent from the repo (import
   needs Tyler's approval). `sendCombatSound` (Worker B)
   fixes a real bug found by its own tests: a `*Mobile`
   player attacker would have broadcast an area sound
   instead of hearing its own synth.
5. **Hub**: no new hook families, no new members — counts
   unchanged (53/108 firable, 29/95 members, 12/191 Core).
   One new custom event: `player:dying`
   (`PlayerDyingEvent{Player, Killer}`).
6. **Docs**: this plan, `PORT_STATUS.md`, and
   `CONTENT_MIGRATION_PLAN.md` updated by the coordinator.

Verification (coordinator, final tree): `go build ./...`,
`go vet ./...`, `gofmt -l .` clean; `CGO_ENABLED=0 go
build ./...` clean; 654 `.go` files, 17 packages;
`go test ./... -p 1 -count=1`: **4,092 passing, 0
failures** (4,032 baseline + 59 worker tests + 1
coordinator test); `go test -race`: 17/17 clean. Live
smoke (`/home/hatch/smoke29/`, real server + real rev-242
cache, 3 runs × 4 modes, coordinator re-ran with a new
ranged start-graphic check): melee 3/3 (anims 390/388,
goblin 6184/836), ranged 3/3 (anim 426, projectile 10,
sound 2702, start graphic 19 at height 100, goblin 836,
ammo 499→497/shot), magic 3/3 (cast anim 1162, start
graphic 90@100, projectile 91, impact graphic 92@100
delay 56, sounds 220/221, goblin 836, mind runes paid),
death 2/3 in the driver + 1/1 clean re-run (anims
[422 424 836], "Oh dear, you are dead!", respawn
(3089,3524,0), HP 99/99) — the single driver failure was
run 1's death leg timing out at HP 2/99 (harness 240s
boundary, not a defect; no code in this phase touches
death timing or damage).

Residual gaps (next-phase brief material, NOT a fifth
combat slice unless Tyler directs it):
- WeaponProfiles **profile layers** NOT ported (18
  `attackAnimation` entries in `WeaponProfile.ts`; no
  registry in Go) — attack anims come from the definition
  loader's family/overlay fallback instead (goblin attack
  is 6184, not the 422 fallback — proven on the wire).
- `npc-combat-sounds.json` absent: NPC death sounds and
  the `NpcCombatSounds` hit/attack captured sounds are
  NoSound; import needs Tyler's approval.
- NPC-with-spell magic casts not opened (`StartCast`
  takes `*Player`); NPCs cast via the no-spell branch.
- Special-attack fire sounds have no seam; content spells
  with queued visuals skip the `!queuedSound` path.
- Queued-graphic priority is approximated (Go marks
  queued whenever the call was made; TS only when the
  delayed graphic wins the priority check).
- Dark-bow second-hit delay, projectile heights within 2
  of TS, plugin ammo/crystal-bow charges/ammo strength
  (phase 26 gaps, untouched).
- Pre-existing, untouched: far-WALK bug, y=3521 wall +
  gap 11c, gap 12c bootstrap weight, scrypt check,
  Wilderness `canAttack` content gate (live PvP still
  blocked — skull trigger unit-covered only).
