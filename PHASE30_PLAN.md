# Phase 30 plan — combat leftovers, first slice

Dispatched 2026-11-17 under Tyler's "keep going without
confirming". Tyler decided 2026-11-17: combat leftovers
first, then adding all skills. Doric's Quest remains picked
but deferred — NO quest work in this phase.

Baseline: 654 `.go` files, 17 packages, 4,092 test cases
passing incl. subtests, race clean, hub 53/108 firable,
29/95 members, 12/191 Core. Combat is finished as a planned
engine slice (phases 26–29); this phase closes the residual
gaps named in PHASE26/27/28/29 plans and PORT_STATUS gaps
14/15. No new verbatim data import in this phase:
`npc-combat-sounds.json` import needs Tyler's approval
(parent asks at phase completion); `item-alch.json` +
`item-gameplay.json` are already shipped (Tyler-approved,
SHA-256 verified) — deathComponents splitting is now
unblocked.

## Scope (prioritized; land in order, stop cleanly if the phase fills)

1. **WeaponProfiles profile layers** (phase 29 gap):
   port the profile registry + `attackAnimation`
   selection (18 `attackAnimation` entries in
   `WeaponProfile.ts` —
   `~/tsps-bfd81e3/server/src/main/typescript/elvarg/game/content/combat/WeaponProfile.ts`).
   Attack anims currently come from the definition
   loader's family/overlay fallback (`GetAttackAnim`'s
   item-def layer, `combatant.go:455-462`, citing
   `WeaponProfiles.attackAnimation`). Ground the layer
   order in `WeaponProfile.ts` and the combat methods
   that read it; keep the fallback for weapons with no
   profile entry.
2. **Dark-bow second-hit delay** (phase 26 gap):
   ground in TS (dark bow fires two projectiles in
   phase 26; the second hit's delay has no home in Go
   — see PHASE26_PLAN.md:218) and land it.
3. **Projectile heights** (phase 26 gap): reconcile
   Go's integer heights to TS exactly (currently within
   2 of TS — PHASE26_PLAN.md:220). Ground in the TS
   projectile creation sites; change the heights, not
   the shape of the API, unless TS forces it.
4. **NPC-with-spell magic casts** (phase 29 gap):
   open `StartCast` (or its equivalent path) for NPC
   casters; NPCs currently cast via the no-spell branch
   (`combat_magic.go:323`). `StartCast` takes `*Player`
   today — ground what TS does for NPC casters
   (`CombatSpell.ts`, `NpcCombatMethod` / magic method)
   and open the path without breaking the player path.
5. **Special-attack fire sounds** (phase 29 gap):
   build the missing seam so special attacks play
   their fire sound (`combat_sounds.go` pickers exist
   since phase 29: `AttackSoundFor`, `FireSoundFor`,
   `CastSoundFor`, `ImpactSoundFor`; the special-attack
   path has no call site).
6. **Queued-graphic priority** (phase 29 gap): match
   the TS priority check instead of Go's approximation
   (Go marks queued whenever the call was made;
   `mobile_animation.go:286-291`, `combat_spells.go:277-300`;
   TS only when the delayed graphic wins the priority
   check).
7. **Spec-orb sends** (phase 26 gap): weapon-meter
   tail, drain-path, and login sends for the
   special-attack orb (varps 300/301 exist on restore
   only — `special_restore.go:33,50,56,189`). Ground
   in TS `CombatSpecial.updateBar` and the drain/login
   paths; send in TS order (orb first, state second).
8. **deathComponents splitting** (phase 28 gap 15b):
   now unblocked — `item-gameplay.json` IS shipped.
   Port the splitting the death item-loss pipeline
   drops whole today (`playerdeath.go:26-27,496`).
   Ground in `PlayerDeath.ts` + the gameplay data's
   `deathComponents` field.
9. **fightType save field dead data + onCanAttack
   legacy fallback cleanup** (phase 26 gaps): wire the
   field if TS grounding supports a live consumer, or
   document precisely why it stays dead; remove the
   legacy `onCanAttack` fallback if TS grounding
   supports it (do not remove on a guess).
10. **Plugin ammo / crystal-bow charges / ammo
    strength** (phase 26 gap): land ONLY the
    engine-grounded part; content-owned parts stay
    documented as gaps, not invented.

STRETCH (only if items 1–10 land cleanly with room in
the phase; otherwise document as the Phase 31 brief, do
not half-land): gap 14 spellbook residuals — Ancients/
Lunar/Arceuus books, bind/curse/freeze application,
Vengeance, spell-on-object, `ApplyChargeMaxHit`, NPC god
drains, TeleBlock timer frame, Magic Dart Slayer's-staff
rules, ElementalStaves content, rune pouch, alchemy
confirm dialogue.

## Worker split (3 workers, parallel)

- **Worker A — profiles + save/cleanup:** items 1, 9.
  Owns: new `internal/game/weapon_profile*.go`
  (registry + selection), `combatant.go` (GetAttackAnim
  layering), `combat.go` (ExpectAttackAnimation only
  if the profile layer needs it), persist save wiring
  only if fightType gets a live consumer. Does NOT
  edit the melee/ranged call sites (`combat_melee.go:47`,
  `combat_ranged.go:430` already call `GetAttackAnim`).
- **Worker B — projectiles + NPC casts + queued
  graphics:** items 2, 3, 4, 6. Owns:
  `combat_ranged.go` (dark bow + heights),
  `combat_magic.go` + `combat_spells.go` (NPC cast
  path + queued-graphic priority),
  `mobile_animation.go` (queued-flag semantics only),
  projectile creation sites. Reports exact TS file:line
  for every height/delay/priority value changed.
- **Worker C — orb + death splitting + spec sounds +
  ammo engine part:** items 5, 7, 8, 10. Owns:
  `special_restore.go` + special-attack orb sends,
  `playerdeath.go` (deathComponents only),
  `combat_sounds.go` + the special-attack fire-sound
  call site(s), `ranged_data.go` (ammo/charges engine
  part only). Does NOT touch `combat_melee.go` anim
  call sites owned by the phase 29 shape; if a fire-
  sound call must land in a melee/ranged method, it
  reports the exact hunk to the coordinator instead
  of editing across Worker A's/B's files.

## Cross-worker contracts (do not deviate without telling the coordinator)

- **P1 — Attack anim layering (A owns, nobody else
  touches):** `GetAttackAnim(target)` stays the single
  call site; A adds the profile layer above the
  item-def layer and reports the layer order + the
  fallback rule. B and C never set attack animations
  directly.
- **Q1 — Queued graphics (B owns):** the queued flag
  semantics live in `mobile_animation.go` + the spell
  cast context; C's spec-sound work and A's profile
  work never touch the queued flag.
- **O1 — Spec orb (C owns):** varp 300/301 sends stay
  in TS order (orb first, state second); no other
  worker sends those varps.
- **D1 — deathComponents (C owns):** the splitting
  lives in `playerdeath.go` behind the existing
  kept-items pipeline; A/B never touch death.
- **C5 — No new data imports.** Approved verbatim
  imports are music-areas, item-combat-styles,
  item-alch, item-gameplay. `npc-combat-sounds.json`
  is OUT — flag as pending decision, do not import.
- **Docs:** workers do NOT edit the docs files — the
  coordinator integrates them from worker reports to
  avoid merge conflicts.

## Out of scope (do not start, document as gaps)

All quest work (Doric's Quest deferred);
`npc-combat-sounds.json` import or `NpcCombatSounds`
wiring (needs Tyler's approval — flag as the pending
decision in the coordinator report); the far-WALK bug;
the y=3521–3522 map wall; the real production
scrypt-hash check; perf work; any new verbatim data
import. No scope expansion without TS grounding. If
the scope exceeds one phase, land a coherent verified
subset and document the remainder precisely as the
next phase's brief — the next work after leftovers is
adding all skills, per Tyler's order, so keep the
handoff crisp about what is left.

## Verification (same pattern as phases 26–29)

`go build ./...`, `go vet ./...`, `gofmt -l .`,
`CGO_ENABLED=0 go build ./...`,
`go test ./... -p 1 -count=1` (4,092 tests before this
phase — must not drop), `go test -race`, plus a live
smoke test against the real cache (reuse
`/home/hatch/smoke27/` + `/home/hatch/smoke28/` +
`/home/hatch/smoke29/`, seed cache at
`/home/hatch/smoke27/seed-root/caches/osrs-242_2026-11-03`),
3 runs stable: the observable subset of what landed
(e.g. a weapon whose attack anim comes from its
profile entry rather than the fallback, a dark-bow
double hit with the correct delay, an NPC casting a
spell with cast anim + projectile, spec-orb varps on
drain and login, a death splitting a component item).
Update this plan and `PORT_STATUS.md`; no Drive upload
(parent's job).

## Outcome (coordinator, 2026-11-18)

All ten scope items landed; the stretch (gap 14
spellbook residuals) was NOT started — it is the
Phase 31 brief below, not half-landed. No new data
import; `npc-combat-sounds.json` stays absent (import
needs Tyler's approval — flagged as the pending
decision in the coordinator report, parent asks at
phase completion).

1. **WeaponProfiles profile layers** (Worker A):
   `internal/game/weapon_profile.go` — the
   attack-animation layer of `WeaponProfile.ts` only
   (the rest of the profile stays in the ranged
   tables / combat styles where phases 26–27 mirror
   it). `GetAttackAnim` layer order (TS
   `WeaponProfiles.attackAnimation`,
   `WeaponProfile.ts:111-119`, via
   `Player.getAttackAnim`, `Player.ts:333-338`):
   (1) profile by wielded item id, (2) the item's own
   data (`ItemDefinition.getAttackAnim`), (3) profile
   by weapon type via `CurrentWeaponInterface`,
   (4) the fight style's animation against the target
   kind. All 18 literal `attackAnimation` entries
   (9 by weapon, 47 item ids, resolved through
   `ItemIdentifiers.ts` / `CrystalBow.ts`). Known
   limits, documented in code: no `ItemAliases` port
   in Go, so layer 1 is exact-id only; javelins are
   `equipmentType: ARROWS` with no `weaponInterface`
   in `item-gameplay.json`, so the JAVELIN 929 entry
   is unreachable until the interface seam covers
   thrown ammo (same pre-existing gap that makes
   `AssignWeaponInterface` read UNARMED for a javelin
   wielder). Live deltas vs the old fallback: ascension
   crossbows 4230/7552 → 15113/15114, black chinchompa
   vs players 2779 → 7618, blisterwood stake 806 →
   7617.
2. **Dark-bow second-hit delay** (Worker B):
   `HitDelayProfile{Base, DistanceOffset, Divisor}`
   + `RangedWeaponData.HitDelayProfiles`
   (`WeaponProfile.ts:295,166-171`;
   `RangedCombatMethod.ts` `hits()` slicing).
   DARK_BOW now carries `[{1,3,6}, {1,2,3}]`; at
   edge distance 9 the shots land on delays 3 and 4
   (was both 3); one-arrow shots keep only delay 3.
3. **Projectile heights exact** (Worker B): heights
   are TS `number`s, so Go heights became `float64`
   (`WeaponProfile.ts:58-62,287,296,337`; the sender
   multiplies by 4 — `PacketSender.ts:1278-1279`).
   Bow/thrown 40.75/36.5 [163/146], crossbow
   38.75/36.5 [155/146], dart 35.5/36.5 [142/146],
   ballista 38.75/44.25 [155/177], venator 39/36.5
   [156/146], default/NPC/magic 43/31 [172/124]
   unchanged, dark-bow second 48/31 [192/124]
   unchanged. Touched `projectile.go`,
   `combat_ranged.go`, `ranged_data.go`,
   `internal/session/projectile_sender.go` (the
   delivery half — `int(math.Round(h*4))`),
   `internal/plugins/projectiles.go`.
4. **NPC-with-spell magic casts** (Worker B):
   `CombatSpell.StartCast` / `SpellCaster.StartCast`
   now take `caster *Mobile`
   (`CombatSpell.ts:51`; `MagicCombatMethod.ts:261-300`).
   Spell branch runs for any caster (cast sound for
   any caster, base XP owed only for players);
   staff variant player-only; the four NPC ids
   2000/109/3580/2007 skip the start graphic;
   impact visual queued only for player casters.
   `CanCast`/`CanCastOnTarget` stay player-only
   (TS `canAttack` returns true for NPCs before any
   spell check). **Honest limit:** no production
   path sets an NPC's cast spell — the only
   `CastSpellOn` caller is the player spell listener
   (`cmd/server/spellwiring.go:91`) — so the NPC
   spell branch is unit-covered only and cannot fire
   live until NPC spell selection from NPC combat
   defs lands (content). The NPC no-spell branch
   (default wind-strike projectile 91) is unchanged
   in shape and covered by the magic-mode regression.
5. **Special-attack fire sounds** (Worker C +
   coordinator call sites): `CombatSpecial.FireSounds`
   + `SpecialFireSoundsFor` / `PlaySpecialFireSound`
   (`combat_sounds.go`; TS grounding
   `RangedCombatMethod.ts:38-40,182-183`,
   `MeleeCombatMethod.ts:9-14`; dragon dagger 2537,
   dragon claws 4138/4140/4141/4141 with delays
   10/30/42/60). Coordinator applied the two call-site
   hunks Worker C deliberately left out (its files):
   melee + ranged `Start` now play the special sound
   INSTEAD of the ordinary sound when one exists.
6. **Queued-graphic priority** (Worker B):
   `PerformGraphic` now returns `bool` (whether
   accepted — the Go form of the TS identity check,
   since every play allocates a fresh `Graphic`);
   `queueMagicImpactGraphic` sets `visual.queued`
   from that return (`MagicCombatMethod.ts:211`;
   `Mobile.ts:394-402`). Sound half still runs
   first, as in TS.
7. **Spec-orb sends** (Worker C):
   `updateSpecialAttackOrb` now called from
   `DrainSpecial` (after `EnsureSpecialRestore`),
   `AssignWeaponInterface` (after `assignSpecialBar`
   — both the login and the equip-change send),
   and the death reset (double-send matches TS).
   Varps 300/301 in TS order (orb first, state
   second). **Key finding: the weapon-meter tail is
   dead in TS itself** — `updateBar`'s tail
   (per-segment `sendInterfaceComponentMoval` +
   "Special Attack (X%)" string,
   `CombatSpecial.ts:240-259`) goes through
   `PlayerSession.write`, a no-op at bfd81e3
   (`PlayerSession.ts:228-230`); Go correctly sends
   nothing and no native `WIDGET_SET_POSITION`
   tail was built (it would be a deviation).
8. **deathComponents splitting** (Worker C):
   `splitIntoComponents` ported
   (`PlayerDeath.ts:353-360`,
   `ItemDefinition.ts:110,318-320`; definition
   layer already carried it from shipped
   `item-gameplay.json` — 10 entries: Nightmare
   staves 24423–24425, Avernic treads variants
   31091–31097). Wired into `DropDeathItems` before
   the per-item loop (gates: killer nil / self-kill /
   not-Wilderness → whole; kept and lost-on-death
   items stay whole; each component gets the whole
   item's amount). TS `keepsEverything` guard
   deliberately omitted — Go has no rights field, so
   it is always false (commented at the site).
   **Honest limit:** live PvP is still blocked
   (Wilderness `canAttack` unported), so splitting
   is unit-covered only — no live PvP kill exists to
   observe it on.
9. **fightType save field + onCanAttack legacy
   fallback** (Worker A): fightType is LIVE, so it
   was wired, not documented dead
   (`PlayerSave.ts:642,754`, `Player.ts:1654-1683,1678`;
   `resolveSavedFightType`, `PlayerSave.ts:255-260`).
   New `RestoreSavedFightType` + `FightTypeSaveJSON`
   (`fighttype_save.go`), wired at login by
   `applyEntryFightType` (`combatstylewiring.go`,
   after equipment restore, before the login
   `AssignWeaponInterface`); `FightType` added to
   `persist.OwnedState` (nil = preserve) and
   `buildSnapshot` writes the live fight type.
   Observable exactly when no per-category style was
   ever remembered and the saved value is one of the
   equipped weapon's offered styles. The legacy
   `onCanAttack` fallback was REMOVED — TS grounding
   supports it (`PluginManager.emitCanAttack`,
   `PluginManager.ts:1360-1377`, is the only form;
   no method-less emit exists in TS; production never
   assigned the field). `CombatHooks.CanAttack`
   deleted; all 5 test files migrated to
   `OnCanAttack`.
10. **Plugin ammo / crystal-bow charges / ammo
    strength** (Worker C, engine part only):
    all 193/193 ammo kinds now carry `strength`
    (script-extracted from `RangedData.ts:300-308,380-382`;
    0 missing/extra/duplicate/conflict; bronze arrow
    7, rune arrow 50, dragon arrow 65, seeking
    dragon arrow 60, onyx bolt 120, dragon bolt 117,
    ascension bolt 53, morrigans javelin 145,
    crystal bow 0). Crystal-bow charge data from
    `CrystalBow.ts` (250 shots/stage, stage order,
    bonuses, strengths, static/empty ids) +
    predicates. **Bug found and fixed:**
    `DecrementAmmo` never consulted the existing
    `IsRangedChargeBow` set, so the generic path ate
    a crystal bow (1→0→removed) on its first shot —
    now charge bows return early (TS order: plugin
    handlers → webweaver/craw early return →
    crystal-bow branch degrades instead of
    decrementing, `CombatFactory.ts:1799-1811`).
    Deliberately left out: plugin ammo resolution
    (no Go form, content-owned); crystal-bow
    degradation loop + per-shot meta counting +
    `combat:consume-charge` event + BonusManager
    refresh (charge system NOT STARTED — charge bows
    fire without consuming anything until it lands);
    ammo strength has no formula consumer (TS
    `getStrength()` itself is never called by the
    engine at bfd81e3; ranged max hit reads the worn
    equipment's ranged-strength bonus).

Verification (coordinator, final tree): `go build
./...`, `go vet ./...`, `gofmt -l .` clean;
`CGO_ENABLED=0 go build ./...` clean; 660 `.go`
files, 17 packages; `go test ./... -p 1 -count=1`:
**4,115 passing, 0 failures** (4,092 baseline + 23:
Worker A 8, Worker B 5, Worker C 10 — plus the
coordinator's two call-site hunks, covered by
Worker C's tests); `go test -race`: 17/17 clean, no
data races. Hub counts unchanged (53/108 firable,
29/95 members, 12/191 Core) — no new hook families.
Live smoke (`/home/hatch/smoke30/`, real server +
real rev-242 cache, 3 runs × 8 modes, 34 checks per
run, 0 failures): melee 3/3, ranged 3/3 (bronze
arrow projectile heights 163/146 on the wire),
magic 3/3, profile 3/3 (ascension crossbow attack
anim 15114 — the profile's npcAttackAnimation vs an
NPC target — and never the 4230 style fallback;
ascension bolt projectile 4437), darkbow 3/3 (dragon
arrow projectile 1120; the two hitsplats of one shot
landed 597–601ms apart — exactly one tick — in every
run), orb 3/3 (varp 300 == 1000 and varp 301 == 0 at
login), crystalbow 3/3 (projectile 249; bow still
equipped, slot 4214, after firing), death 3/3
(anims [422 424 836], "Oh dear, you are dead!",
respawn (3089,3524,0), HP 99/99). Harness notes:
the profile check first expected 15113 (the
vs-player anim) and failed twice — the expectation
was wrong, the code is TS-faithful (npcAttackAnimation
15114 vs NPCs); the darkbow check first failed because
a 5-HP goblin died on the first hit — fixed by staging
a 50-HP goblin (hitpoints only; smoke28's max_hit 8 /
attack 99 staging made the goblin lethal across the
long fights, killing alice mid-ranged and starving the
magic mode of runes — corrected to hitpoints-only).
A full 3-run driver pass exceeds the background-exec
wall clock (~520s SIGTERM mid-run-2), so runs were
executed singly via a `RUNS` override: run 1 8/8,
run 2 8/8, run 3 8/8, each from a fresh temp root.

## Phase 31 brief (next phase: gap 14 spellbook residuals)

The stretch was not started. Per Tyler's order
(combat leftovers, then adding all skills), the
next work after this phase is adding all skills —
the gap-14 residuals below are NOT the next phase
unless Tyler redirects; they remain as documented
residuals: Ancients/Lunar/Arceuus spellbooks,
bind/curse/freeze application, Vengeance,
spell-on-object, `ApplyChargeMaxHit`, NPC god
drains, TeleBlock timer frame, Magic Dart
Slayer's-staff rules, ElementalStaves content, rune
pouch, alchemy confirm dialogue. Also still open:
`npc-combat-sounds.json` import (needs Tyler's
approval — parent asks at phase completion),
NPC spell selection from NPC combat defs (no
producer sets an NPC's cast spell), crystal-bow
degradation loop + charge system, plugin ammo
resolution, ItemAliases (profile layer 1 is
exact-id only), javelin weapon-interface seam.
Pre-existing, untouched: far-WALK bug, y=3521 wall
+ gap 11c, gap 12c bootstrap weight, real production
scrypt-hash check, Wilderness `canAttack` content
gate (live PvP still blocked).
