# Phase 26 plan — DRAFT: finish combat, first slice (ranged data + combat styles)

Status: COMPLETED (2026-11-16, verified 2026-11-17 UTC; three
workers + coordinator integration). The first slice landed as
scoped below, plus a grounded fix for a Phase 16 gap the live
smoke found (dead NPCs never left the world — see Outcome).
Quests stay postponed; Doric's Quest stays picked-but-deferred.

## Why this phase exists

Tyler picked Doric's Quest on 2026-11-16 (15:25 UTC), then minutes
later instructed (15:27 UTC): "Postpone quests finish combat".
Doric's Quest therefore remains the picked first quest but is
deferred, and combat is the immediate priority. The tracking item
still says "paused awaiting Tyler's quest pick" — that is stale;
the quest pick happened and was then superseded by the combat
instruction.

"Finish combat" is not one phase. Phase 16 ported the combat core
(attack cycles, melee/ranged/magic methods, exact accuracy/max-hit
formulas, prayer, specials, hit queue, death state machines, combat
XP, attack listeners, hub combat families). What remains is the
set of systems Phase 16 explicitly left out, plus the content-data
tables the core's seams were built to read. This plan measures
those remainders from the upstream source at
`~/workspace/tsps-bfd81e3` (commit bfd81e3) and proposes the first
slice, in an order where each slice unlocks the next.

## What combat already does (grounding, not a claim to re-prove)

- Phase 16, verified, 2,757 tests then; repo now at 3,799 tests,
  17 packages, 45 of 108 hub hook families firable, 26 of 95
  non-hook PluginApi members ported (phases 22–25).
- Melee works end to end; NPCs wander, aggress, fight, die,
  respawn; hitsplat/health-bar sync live-verified.
- Ranged and magic *methods* exist in Go
  (`internal/game/combat_ranged.go`, `combat_magic.go`), but:
  - Ranged: "with no resolver registered, no ranged weapon is
    ever detected and every fighter attacks in melee"
    (`combat_ranged.go` header). The resolver seam
    (`RegisterRangedDataResolver`) exists; nothing registers it
    in production. Consequence, already documented in Phase 24:
    Seeking Arrows cannot fire in production until ranged
    weapon/ammunition data is ported.
  - Magic: `CombatSpell` interface exists; no spell is
    registered. Spell-on-NPC (opcode 75), spell-on-player, and
    spell-on-item stay decode-drop; the spell system is NOT
    STARTED.
  - Combat styles: only `UnarmedPunch` exists in Go
    (`combat_types.go`); the full data-driven registry
    (weapon interfaces, style selection, the
    `combat:style-by-category` attribute) is NOT STARTED.
    Varp 43 (combat style) stays decode-drop; varp 172
    (auto-retaliate) likewise.

## Measured remainder (upstream sizes, all at bfd81e3)

| Piece | Upstream source | Size | Go status |
|---|---|---|---|
| Ranged weapon/ammo data | `game/content/combat/ranged/RangedData.ts` | 617 lines, 260 `public static readonly` entries (weapons + ammunition) | NOT STARTED; resolver seam exists, unset |
| Weapon profiles | `game/content/combat/WeaponProfile.ts` | 450 lines | NOT STARTED (profile fields partly mirrored in `RangedWeaponData`) |
| Combat styles registry | `FightType.ts` (203) + `CombatStyleDefinitions.ts` (50) + `data/definitions/item-combat-styles.json` | 253 lines + data file | PARTIAL: only `UnarmedPunch`; registry NOT STARTED |
| Weapon interfaces | `WeaponInterfaces.ts` (87) + `WeaponInterfaceManager.ts` (98) | 185 lines | NOT STARTED |
| Combat bonuses | `game/model/equipment/BonusManager.ts` | 100 lines | NOT STARTED; named as a gap at 5+ sites (`skills.go`, `combat_types.go`) |
| `onCanAttack` gate | `PluginTypes.ts:1109`, `PluginManager.ts:3581` | hook family, emit site missing | Closest follow-up per `internal/plugins/combat.go`: allow-veto gate helper exists, emit site inside `canAttackPermission` needs a `CombatHooks` field not yet contracted |
| Special-attack orb | spec restore (`special_restore.go`) | — | No spec-orb update is sent on restore (documented gap); exact varp/interface send to be grounded in TS before committing |
| Spells | `magic/CombatSpells.ts` (2,116 lines) + `CombatSpell.ts` (152) + `Spell.ts` (293) + `Autocasting.ts` (333) + spellbook files (Arceuus 453, Lunar 855, ancients/effects) | ~4,500+ lines | NOT STARTED; interface only; listeners decode-drop |
| Poison / venom | `PoisonClock.ts` (159) + `task/impl/CombatPoisonEffect.ts` (526) | 685 lines | NOT STARTED; save fields exist (`poisonDamage`, `venomed`, `poisonClock`, immunity timers) but no live system |
| Skulls + player death loss | `CombatFactory.ts` skull handling via `Hooks` seam + `task/impl/PlayerDeath.ts` (441 lines) | 441 lines + seam | NOT STARTED; skull seam exists (`ApplySkull`/`IsSkulled`/`ExtendSkull`), no system; death respawns in place with no teleport and no item loss |
| Combat presentation | `NpcAnimationScanner.ts` (245) + animation/graphic/projectile/sound producers in combat methods | 245 lines + call sites | NOT STARTED; phase-21 senders exist, combat never calls them; `performAnimation`/`performGraphic` absent |
| Duel Arena gameplay | duelling state exists (phase 17); arena gameplay/staking | — | NOT STARTED — plugin content, not engine |
| NPC drops | `server/plugins/npcs/NpcDrops.plugin.js` (wiki-dump tables) via `onNpcDeath` | — | NOT STARTED — plugin content; core only emits the death event and rolls nothing |

Out of scope for the whole "finish combat" arc (unchanged):
far-WALK bug and y=3521–3522 map wall (gap 11, needs TS/cache
grounding first), real production scrypt-hash check, Duel Arena
as a first slice, wholesale spellbook ports in one phase.

## Proposed Phase 26 scope (recommended first slice)

Ranged data + combat styles — the slice that makes the combat
that already exists actually usable, and unblocks Phase 24:

1. Ranged data: port `RangedData.ts` (weapons + ammunition
   tables) and `WeaponProfile.ts` behind the existing
   `RegisterRangedDataResolver` / `RegisterAmmunitionData`
   seams, register the resolver in production wiring
   (`cmd/server/combatwiring.go`), and verify a bow actually
   fires as RANGED (not melee) in a live smoke test — which
   also makes Seeking Arrows fire in production for the first
   time.
2. Combat styles: port the `item-combat-styles.json`-driven
   registry (`CombatStyleDefinitions.ts` + `FightType.ts`
   rows beyond `UnarmedPunch`), weapon-interface selection
   (`WeaponInterfaces.ts` + `WeaponInterfaceManager.ts`),
   and `BonusManager.ts` (attack/defence/other bonus
   computation from equipment), so equipping a weapon changes
   the fight type, the weapon interface, and the bonuses the
   formulas already read.
3. Wire varp 43 (combat style) out of decode-drop through the
   live varp store (phase 18), with per-category style memory
   and autocast config send as TS does them; varp 172
   (auto-retaliate) only if it falls out of the same wiring —
   otherwise it stays a named decode-drop.
4. `onCanAttack`: add the `CombatHooks` field and the emit
   site in `canAttackPermission` (hub side already exists;
   gate helper `runAllowVetoGate` already exists).
5. Spec-orb update on special restore — only if the TS send
   is grounded during the phase; if grounding fails, it stays
   a documented gap rather than an invented send.
6. Ranged projectile send via the phase-21 projectile sender
   — only the ranged shot's projectile, grounded in
   `RangedCombatMethod.ts`; animations/graphics/sounds stay
   out (they belong to the presentation slice below).

Out of scope for Phase 26: spells of any book, poison/venom,
skulls, player-death item loss/teleport, Duel Arena, NPC drops,
Barrows set checks, Crystal Bow / ether-bow special cases
(item-specific content), full animation system.

## Proposed order after Phase 26

- Phase 27 — spell front-end: spell-id resolution +
  `CombatSpells` standard spellbook first (the 2,116-line file
  is mostly spell definitions; ancients/Lunar/Arceuus are
  separate books and may split further), MagicOnNpc /
  MagicOnPlayer / MagicOnItem listeners out of decode-drop,
  autocast (`Autocasting.ts`, 333 lines).
- Phase 28 — poison/venom + skulls + player death:
  `PoisonClock` + `CombatPoisonEffect`, skull system through
  the existing seam, `PlayerDeath.ts` item-loss/kept-items/
  grave flow (441 lines, heavy interface dependencies — may
  split).
- Phase 29 — combat presentation: `NpcAnimationScanner`,
  animation/graphic/projectile/sound producers at the combat
  call sites, equipment/NPC-combat sound pickers.
- Then, only with Tyler's direction: Duel Arena gameplay,
  NPC drops plugin, Barrows, boss/special content.

## Verification (same pattern as phases 16–25)

`go build ./...`, `go vet ./...`, `gofmt -l .`,
`CGO_ENABLED=0 go build ./...`, `go test ./... -p 1 -count=1`
(3,799 tests before this phase — must not drop), `go test -race`,
plus a live smoke test against the real cache: equip a bow,
attack an NPC, confirm RANGED method + ammo decrement +
Seeking Arrows floor where applicable; equip a weapon, confirm
fight-type/interface/bonus change and varp 43 traffic.
Update `PORT_STATUS.md` (combat row + known gaps) and this plan;
upload the per-phase archive to Drive per the standing
instruction (folder `tsps-go`, id
`1a8dzseX8f1vESLPm28X0xQGwA6uuU9qX`).

## Decision needed from Tyler

None for this phase — the scope was carried by the parent
brief (keep-going instruction + "postpone quests, finish
combat"), which superseded the draft's confirmation request.
"Finish combat" will take several phases either way; the order
below stands, with the next slice being spells (Phase 27).
Doric's Quest stays picked-but-deferred until combat is done,
per his 15:27 UTC instruction.

## Outcome (verified 2026-11-17 UTC)

- `go build ./...`, `go vet ./...`, `gofmt -l .`,
  `CGO_ENABLED=0 go build ./...`: clean. 620 `.go` files,
  17 packages. `go test ./... -p 1 -count=1`: all 17 pass,
  **3,862 passing cases incl. subtests, 0 failures**
  (3,799 + 63: Worker A 11 + Worker B 28 + Worker C 23 +
  coordinator regression test 1). `go test -race`: all 17
  pass. Hub: **46/108 families firable** (+onCanAttack),
  26/95 members, 12/191 Core.
- Ranged data (Worker A): `RangedData.ts` /
  `WeaponProfile.ts` tables ported script-extracted
  (`internal/game/ranged_data_tables.go`, 193 ammunition
  kinds / 253 item ids, 102 weapon entries / 229 item ids,
  counts exact vs upstream), resolver registered in
  production (`InstallRangedData`, `cmd/server/server.go:392`),
  ranged projectile sends wired (NPC + player, dark bow
  fires two). Live smoke (15 runs): seeking-arrow
  non-final hits all exactly 3, rune-arrow controls show
  sub-3 hits — Seeking Arrows fires in production for the
  first time.
- Combat styles (Worker B): `item-combat-styles.json`
  imported verbatim + SHA-256 verified (161 fight types,
  53 weapon interfaces), registry + weapon interfaces +
  `BonusManager` wired into the Phase 16 formulas, varp 43
  / varp 172 / varbit 357 out of decode-drop, login + death
  assign hookups (coordinator). Live smoke 3 runs, 17/17
  checks each.
- onCanAttack (Worker C): `OnCanAttack` seam + hub emit
  (TS duel branch + `canAttackByPolicy` order, legacy
  `CanAttack` kept as fallback), wiring added by the
  coordinator. Spec orb: varps 300/301 on restore (weapon
  bar/meter tail, drain-path and login-time sends remain
  gaps). Projectile-flight divergence RESOLVED: Go was
  wrong for even sizes (`west+offset*2` vs TS
  `west+size-1`); fixed, unit-covered.
- NPC corpse removal (coordinator, found by Worker A's
  smoke): dead NPCs never left the world — `dispatchNpcDeath`
  scheduled the respawn but never queued the removal, so
  corpses stood at HP 0 forever. Grounded in
  `NPCDeathTask.remove` (NPCDeathTask.ts:101-117); fixed
  with a `SetNpcRemover` seam installed by
  `InstallNpcRespawnWiring`, plus the dying-flag clear.
  Regression test
  `TestNPCKillRespawnsCloneAndRemovesCorpse` (kill through
  the real pipeline in a ticking world; clone arrives, corpse
  leaves). Correction to Worker A's report: the clone DID
  arrive in the reproduction — only the removal was missing.
  Remains-path deviation documented: removal is immediate
  at dispatch (Go has no remains system; TS waits for the
  remains task).
- NOT landed (next slices): spells of any book (Phase 27),
  poison/venom + skulls + player death loss/teleport
  (Phase 28), combat presentation (Phase 29). Gaps inside
  this slice: dark-bow second hit delay has no home in the
  single-triple core shape (both hits land on STANDARD
  delay); projectile heights are ints (within 2 of TS);
  Longrange +2 now triggers via Worker B's fight types;
  plugin ammunition, crystal-bow charges, ammo strength
  NOT STARTED; `fightType` save field is dead data (style
  restores from the `combat:style-by-category` attribute);
  spec-orb weapon-meter tail / drain-path / login-time
  sends; `onCanAttack` legacy `CanAttack` fallback remains
  until its test assignments migrate.
