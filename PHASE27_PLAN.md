# Phase 27 plan — DRAFT: combat spells (standard spellbook + listeners + autocast)

Status: IN PROGRESS (dispatched 2026-11-17 under Tyler's "keep going
without confirming"; quests stay postponed, Doric's Quest stays
picked-but-deferred). Three workers + coordinator integration.

## Why this phase exists

Phase 26's next-slice brief (PHASE26_PLAN.md, "Proposed order after
Phase 26"): "Phase 27 — spell front-end: spell-id resolution +
`CombatSpells` standard spellbook first ..., MagicOnNpc /
MagicOnPlayer / MagicOnItem listeners out of decode-drop,
autocast (`Autocasting.ts`, 333 lines)." The magic combat method
already exists in Go (`internal/game/combat_magic.go`, Phase 16):
it reads `Combat.SelectedSpell()`, calls `CanCast` /
`CanCastOnTarget` / `StartCast` / `FinishCast` / `OnHitCalc` /
`SpellRadius`, pays base XP once per cast, and handles barrage
secondaries — but no spell is registered, so no player can cast
anything. The listeners exist as decode-drops (opcodes 75, 32,
94/90, 2). This phase supplies the spells, the resolution, the
listeners, and autocast.

## Grounded shape (all at bfd81e3; do not re-derive, verify)

### The spell table — `CombatSpells.ts` (2,116 lines)

- 62 statics: 36 standard-book spells + Charge (private static,
  `spellId()` 0, cast via `CombatSpells.handleSelf`) + 10 Arceuus
  + 16 Ancients (from line 1720, via `CombatAncientSpellExtend`).
  Standard book only this phase; Ancients/Lunar/Arceuus are
  separate books and split to follow-ups — say so in the docs.
- Standard spells are `CombatNormalSpell` (direct damage, options
  object: spellId, maximumHit, castAnimation, startGraphic,
  castProjectile, endGraphic, baseExperience, equipmentRequired,
  itemsRequired, levelRequired, optional finishCast/spellEffect/
  canCastOnTarget/castSound/impactSound/missingEquipmentMessage)
  or `CombatEffectSpell` (max hit 0 or -1, same options minus
  equipment; `finishCast` runs `spellEffect` on accurate hits).
- `CombatSpells.getCombatSpell(id)` / `getCombatSpellByName(name)`:
  linear scan of the statics (`CombatSpells.ts:2068-2116`).
- **Extraction must be scripted, brace-counted, verbatim.**
  A windowed regex bleeds across statics (found during
  planning: WATER_STRIKE read as 1156, BIND as 1163 — both
  wrong; BIND is 1572). Phase 26's script-extracted ranged
  tables are the precedent: extract id, level, max hit, base
  XP, splash XP, runes, equipment, animation/projectile/
  graphic ids per spell, and verify counts against the file
  (36 standard statics + Charge). Animation/projectile/
  graphic ids are recorded as data; producers are Phase 29 —
  do not send them from spell code.
- Spell ids are the client's widget-ish ids (1152–1190 range
  plus 1539/1542/1562/1572/1582/1592 for late-book spells,
  12037 Tele Block, 21876–21879 surges — verify by extraction,
  never by hand or by OSRS memory).

### The `Spell` base — `Spell.ts` (293 lines)

`Spell.canCast(player, del)`: Magic level check (exact line
"You need a Magic level of N to cast this spell." + combat
reset) → `onSpellDisabled` first-answer gate (spellbook,
spellId, spell; refusal clears cast + resets) → spellbook
match (mismatch clears autocast + cast + resets) → rune
shortfall via `itemsToConsume` (`onSpellRuneBypass` per item,
`magic:spell-cost` event with paid/covered split) → rune-source
check (`checkSpellRuneSource`, e.g. rune pouch) → equipment
check (exact `missingEquipmentMessage`) → on `del=true`:
shared 1-tick cast delay (`magic:next-cast-at` attribute) +
per-spell cooldown attribute, `magic:rune-saving` event
(staff of the dead / Kodai wand may negate cost), rune
deletion / rune-source consumption, `endSpellbookSwap`,
cast context + `magic:cast-committed` event. `canCastOnTarget`
defaults true; combat spells override it.
`missingItemsMessage`: first shortfall item, rune name title
case + "s", else lower case + "s" ("You do not have enough
Law Runes to cast this spell."), cache rune order when known.

### `CombatSpell` — `CombatSpell.ts` (152 lines)

`usesSharedCastDelay()` true. `startCast`: animation with
staff-variant map (710→1161, 711→1162, 716→1163, 717→1164,
718→1165, 724→1166, 727→1167, 728→1168, 729→1169,
1819→1820 — record, do not play: no `performAnimation` yet),
start graphic, projectile send + projectile-visual context
for standard spells with no fixed hit delay. `finishCast`
via `onHitCalc`: accurate hits emit `magic:hit-effects`
(allowed + effectMultiplier) then `spellEffectOnHitCalc`,
then `magic:hit-effects-applied`. Effect spells:
`finishCast` → `spellEffect` → `magic:utility-effect-check`
then `magic:utility-effect` (allowed/handled) → options
`spellEffect` if unhandled. **Bind/curse/freeze effects are
plugin content** (`plugins/combat/StandardBindings.plugin.js`,
`StandardCurses.plugin.js`, `Freezes.plugin.js` listen on
`magic:utility-effect`): this phase emits the events with the
TS payload shape; the plugins themselves are a content
follow-up, documented as such. A bind cast therefore spends
runes, rolls the hit, and emits — but applies no freeze until
its plugin ports.

### Spell-id resolution — `NetworkBuilder.ts:1786-1800`

`resolveClientCombatSpellId(spellWidget, spellChild,
spellItemId)`: tries `[spellChild, spellWidget & 0xffff,
spellItemId]` through `getCombatSpell`, then cache spell name
(`CacheDefinitions.getSpellName`) → `getCombatSpellByName`,
then `MAGIC_ITEMS.resolveSpellId(name)` (Low Alchemy 1162,
High Alchemy 1178, Telekinetic Grab 1168, reanimations).
Go decoders already exist (`DecodeSpellOnNpc` opcode 75,
`DecodeSpellOnPlayer`, `DecodeSpellOnGround`,
`DecodeSpellOnObject` opcode 2); the Go cache spell-name
lookup must be grounded against `internal/cache` /
`internal/definition` before use — if no Go equivalent
exists, resolve by id only and document the name fallback
as a gap.

### Listeners (TS)

- MagicOnNpc = `NPCOptionPacketListener.castSpell`
  (`NPCOptionPacketListener.ts:161-198`): dead/busy gates,
  NPC lookup, `CombatSpells.getCombatSpell(spellId)` OR
  `EffectSpells.forSpellId(spellId)` (+ `magic:resolve-spell`
  custom event for plugin-supplied utility spells),
  CrossFloor visibility 24 tiles, area `canCastOnNpc`,
  combat spells → `Combat.castSpellOn` iff attackable
  (else scripted route `castSpellOnScriptedNpc`, which also
  rejects when no plugin takes the cast), `canCastOnTarget`
  check, movement reset on failure. NetworkBuilder
  (spell_on_npc, 808-822) checks `LunarSpells.handleNpcTarget`
  first.
- MagicOnPlayer = `MagicOnPlayerPacketListener.cast`
  (5-43): dead gates, index bounds, self-target reset,
  spell lookup, `canCastOnTarget`, `Combat.castSpellOn`.
  NetworkBuilder (786-806) routes Lunar spells first.
- MagicOnItem = `MagicOnItemPacketListener.ts`:
  `castOnItem` (116-164) and `castGroundItem` (481-503,
  telegrab). Alchemy: LOW 1162 lvl 21 xp 31, 3 fire runes,
  cast every 3 ticks, anim 712, graphic 112; HIGH 1178 lvl
  55 xp 65, 5 fire runes, every 5 ticks, anim 713, graphic
  113. Telegrab: 1168 lvl 33 xp 43, range 10, anim 723,
  graphics 142/144, projectile 143, 80 client cycles to land.
  Rune bypass: `onSpellRuneBypass` + infinite air/fire staves
  (id sets in the listener). Spellbook check by interface id.
  Ground-item lookup + walk-to approach + inventory capacity
  + `magic:telekinetic-grab-cast` event. The item comes away
  when the projectile lands, not on the cast.
- `EffectSpells.ts` (268 lines): utility spells — Bones to
  Bananas, Low/High Alchemy, Telekinetic Grab, Superheat Item,
  Bones to Peaches, Bake Pie, Vengeance, Vengeance Other.
  Vengeance is Lunar: table it, handler is a Lunar follow-up.
- Spell-on-object (opcode 2, `UseItemPacketListener.
  spellOnObject`) is adjacent: include only if it falls out
  of the same resolution work, else document as a gap.

### Autocasting — `Autocasting.ts` (333 lines)

- `AUTOCAST_SPELLS` map, slots → spell: 1–16 strikes/bolts/
  blasts/waves, 48–51 surges, 17 Crumble Undead, 18 Magic
  Dart, 19/20/52 god spells, 47 Iban Blast, 31–46 ancients,
  53–58 arceuus. This phase ports the map with the standard
  subset resolvable; ancient/arceuus slots resolve to nil
  until their books land (documented, not invented).
- `handleWidgetAction`: combat interface 593 child 26 clears
  autocast; children 23/28 open the selector; autocast
  interface 201 child 1 slot pick (slot 0 = back to combat
  interface). God slots 19/20/52 only for the matching god's
  staff; only a slot the open list places, from the player's
  own book, can be chosen.
- `openSelector`: Lunar refusal ("You cannot autocast Lunar
  Magicks."), staff check ("You need to equip a staff to
  autocast spells."), per-book `weaponRefusal` (ancient staff
  family only for Ancients; ancient staff autocasts nothing
  else on Normal; Arceuus by weapon name list), fight type
  Focus/Pound, sends: fight-type config + varp 664 selector
  list + sub-interface 201 + interface flags.
- `isActive` = autocast spell set AND staff equipped (Go's
  `autocastingActive` currently checks spell only — the
  weapon-capability term is the documented Phase 16
  deviation; close it here if the staff check lands).
- `setAutocast`: clears the spell if the weapon can't cast
  it; "Default spell set. Please equip a staff to use
  autocast." when set without a staff; `refreshIndicators` +
  `BonusManager.update`. `clearForStyle` on style pick
  (autocast_set and autocast_spell go to 0). `clearForRefusal`
  restores the pre-autocast style slot.
- `refreshIndicators`: varbit 275 (on/off), varbit 276
  (spell index), varbit 2668 (defensive), varp 43 = 4 while
  autocasting. Varp 664 list values: -1 standard, 4675
  ancient staff, 4170 slayer's staff, 9013 skull sceptre,
  plus per-weapon own lists (sotd/sol/sob, void mace, iban,
  thammaron/accursed sceptres).
- `autocastSpellId` is a preserved save field (-1 = none;
  `persist/playersave.go:132-133`): restore on login through
  `setAutocast` (ground the TS login-restore call site;
  a staff that can't cast the saved spell clears it).

### Spellbook — `MagicSpellbook.ts`

NORMAL iface 1151, ANCIENT 12855, LUNAR 29999, ARCEUUS 39999.
`changeSpellbook`: same-book refreshes tab 6; Lunar needs
40 Defence; autocast reset; tab-6 send; `magic:spellbook-
changed` event with the book key ("lunar" etc.). Go persist
already holds `SpellBook` + interface ids
(`playersave.go:480-484`); bootstrap sends varbit 4070.
Go `internal/game` has NO spellbook field yet — this phase
adds the session-only state (default NORMAL from the save).

### Go seams already in place (do not rebuild)

- `CombatSpell` interface (`internal/game/combat_method.go:
  158`): MaximumHit, BaseExperience, SplashExperience,
  AttackSpeed, HitDelay, CanCast(caster, consume),
  CanCastOnTarget, StartCast, FinishCast, SpellRadius. **No
  `SpellID()` — add it; the only test implementer is
  `fakeSpell` (`combat_test_helpers_test.go:204`).**
- Magic method (`combat_magic.go`) already drives the
  interface; `Combat.CastSpellOn` exists (`combat.go:373`).
- `Player.EmitCustomEvent func(name string, payload any)`
  (`player.go:384`) is the custom-event seam (nil-safe);
  `CombatHooks` (`combat_hooks.go`) is the policy-gate seam
  (func fields, game never imports plugins).
- `runFirstAnswerGate` (`internal/plugins/gates.go`) is the
  contract for `onSpellDisabled` / `onSpellRuneBypass`
  (PluginManager.ts:1824-1876); both families are currently
  table-only (comments only in `gates.go`/`hooks.go`).
- Containers: `GetAmount`, `Deletes`/`DeleteNumber`,
  `ContainsAllItem` (`container.go`). Skills:
  `CurrentLevel`, `AddExperienceWithMultipliers`
  (`skills.go`). Equipment: `GetWeapon`,
  `hasStaffEquipped` (`weapon_interface.go:353`). Timers:
  FREEZE/STUN keys exist (`timers.go:130-133`); no
  TELEBLOCK key — ground `TeleBlock.ts` before adding one.
- NPC attackable check via the `IsAttackable() bool`
  definition assertion (`combat_factory.go:744,1083`).

## Scope

1. **Standard spellbook data + implementations (Worker A).**
   Script-extract the 36-spell table (ids, levels, max hits,
   base/splash XP, runes, equipment, anim/projectile/graphic
   ids as data) into `internal/game` (static table, Phase 26
   ranged precedent — `CombatSpells.ts` is core engine code,
   not a plugin). Implement the `Spell` base `CanCast` per
   `Spell.ts` (level, disabled gate, spellbook match, rune
   shortfall + rune-source, equipment, shared delay +
   cooldown, rune-saving, deletion, cast context), plus
   `CombatNormalSpell` / `CombatEffectSpell` Go types with
   `FinishCast`/`OnHitCalc` event emission, `GetCombatSpell`
   / `GetCombatSpellByName`, Charge `handleSelf`, god-spell
   rules (equipment + `missingEquipmentMessage` + the
   `applyChargeMaxHit` cape rule — ground its call site in
   TS first; if no Go seam exists, document it), TeleBlock
   (ground `TeleBlock.ts`), Crumble Undead `canCastOnTarget`
   (ground how TS decides "undead"), Iban Blast / Magic Dart
   equipment rules (ground Iban staff charges, if any).
   Add `SpellID()` to the `CombatSpell` interface and fix
   `fakeSpell`. Effect-spell payloads per §"CombatSpell"
   above; bind/curse/freeze application stays a content
   follow-up (events only).
2. **Listeners + resolution (Worker B).** Port
   `resolveClientCombatSpellId` (id tries, then cache-name
   fallback if a Go equivalent exists, then the
   alchemy/telegrab/reanimation ids). Wire opcode 75
   (spell-on-NPC, `castSpell` incl. scripted route +
   `magic:resolve-spell` + area `canCastOnNpc` if a Go Area
   seam exists, else document), opcode 32 (spell-on-player,
   `cast`, Lunar routing as a documented drop), and
   MagicOnItem (castOnItem alchemy + castGroundItem
   telegrab; other `EffectSpells` table entries route to
   documented NOT STARTED handlers). Keep the click-run
   overrides. Spell-on-object only if it falls out of the
   same work.
3. **Autocast + spellbook + hub (Worker C).** `MagicSpellbook`
   Go type + session-only `Player` spellbook state (default
   NORMAL from save) + `changeSpellbook` (40-Defence Lunar
   gate, tab-6 send, event, autocast reset). Hub families
   `onSpellDisabled` / `onSpellRuneBypass` (first-answer
   gate) + `checkSpellRuneSource` / `consumeSpellRuneSource`
   (ground whether a Go rune-pouch source exists; if none,
   the no-source default is documented, not invented).
   Autocasting: map (standard subset), widget actions
   593/201, selector (varp 664, standard + slayer's-staff
   lists; ancient/arceuus lists present only as far as
   their spells resolve), indicators (varbits 275/276/2668,
   varp 43 = 4), set/clear/refusal paths,
   `autocastSpellId` login restore. Wire the hub → game
   seam in `cmd/server` (populate the `CombatHooks` spell
   fields). Close the `autocastingActive` weapon-capability
   deviation if the staff check lands.
4. **Hub exposure** only as the port requires it, grounded
   in TS. Update hub counts honestly if they change.
5. **Docs:** this plan (outcome at the end), `PORT_STATUS.md`
   (header verification numbers, inventory, hub counts if
   changed, gaps, next-modules order),
   `CONTENT_MIGRATION_PLAN.md` (combat-completion progress
   + residual gaps). Workers do NOT edit the docs files —
   the coordinator integrates them from worker reports to
   avoid merge conflicts.

## Cross-worker contracts (do not deviate without telling the coordinator)

- **C1 — Registry (A owns, B/C call):**
  `game.GetCombatSpell(id int) CombatSpell` (nil if unknown),
  `game.GetCombatSpellByName(name string) CombatSpell`,
  `CombatSpell.SpellID() int` (new interface method).
- **C2 — Spellbook (C owns, A calls):** `game.Spellbook`
  type + `SpellbookNormal/Ancient/Lunar/Arceuus` constants
  + `Player.Spellbook() Spellbook` + `Player.SetSpellbook(Spellbook)`
  (session-only; C decides the login-restore call site).
- **C3 — Gates (C owns hub side, A owns call sites):**
  `Api.OnSpellDisabled` / `Api.OnSpellRuneBypass`
  registration; `Hub.EmitSpellDisabled(player *game.Player,
  spellbook game.Spellbook, spellID int, spell game.CombatSpell) bool`;
  `Hub.EmitSpellRuneBypass(player, spellbook, spellID,
  itemID int) bool`; `Hub.CheckSpellRuneSource(player,
  items []game.Item) bool`;
  `Hub.ConsumeSpellRuneSource(player, items []game.Item)`.
  Game side: new func fields on `CombatHooks`
  (`combat_hooks.go`, A adds): `SpellDisabled`,
  `SpellRuneBypass`, `CheckSpellRuneSource`,
  `ConsumeSpellRuneSource` — nil-safe (nil = not disabled /
  no bypass / no source). C wires them in `cmd/server`.
- **C4 — Custom events (A owns payload types + emit sites,
  via `Player.EmitCustomEvent`):** names verbatim —
  `magic:spell-cost`, `magic:rune-saving`,
  `magic:cast-committed`, `magic:hit-effects`,
  `magic:hit-effects-applied`, `magic:utility-effect-check`,
  `magic:utility-effect`, `magic:resolve-spell`,
  `magic:spell-on-player`, `magic:telekinetic-grab-cast`,
  `magic:spellbook-changed`. Payload types in
  `internal/game` (precedent: `PrivateAreaEnteredPayload`).
- **C5 — Autocast (C owns):** `game.Autocasting` (or
  equivalent) with `SetAutocast(player, spell)`,
  `ClearForStyle(player)`, `IsActive(player) bool`,
  `AutocastSpell(index int) CombatSpell`; widget dispatch
  through the existing interface-action path (C owns
  `interfaceaction.go` branches; B owns `listeners.go`
  opcode cases — do not cross-edit).
- **C6 — File ownership:** A: `internal/game/combat_spells*.go`,
  `combat_method.go` (interface + comment only),
  `combat_hooks.go` (spell fields only),
  `combat_test_helpers_test.go` (fakeSpell only). B:
  `internal/listeners/*` + `internal/game/spell_resolve.go`
  (if the resolver lives in game). C: `internal/game/
  autocasting.go`, `internal/game/spellbook.go`,
  `internal/plugins/*` (spell families),
  `cmd/server/*` (wiring). Anything else: ask first.

## Out of scope (do not start, document as gaps)

Ancients/Lunar/Arceuus spellbooks (tables, handlers,
`LunarSpells.handleNpcTarget/handlePlayerTarget` routing
beyond a documented drop); bind/curse/freeze plugin content
(events only here); Vengeance and other Lunar utility
spells; spell-on-object unless it falls out of the same
work; player death / poison/venom / skulls (Phase 28);
combat presentation — `NpcAnimationScanner`, animation/
graphic/projectile/sound producers (Phase 29); all quest
work; the far-WALK bug; the y=3521–3522 map wall; the real
production scrypt-hash check; perf work. If the scope
exceeds one phase, land a coherent verified subset
(standard spellbook + one listener path first) and document
the remainder precisely as the next phase's brief.

## Verification (same pattern as phases 16–26)

`go build ./...`, `go vet ./...`, `gofmt -l .`,
`CGO_ENABLED=0 go build ./...`,
`go test ./... -p 1 -count=1` (3,862 tests before this phase —
must not drop), `go test -race`, plus a live smoke test
against the real cache, 3 runs stable: a real spell cast on
an NPC landing damage (runes consumed, XP granted), and
autocast selection persisting across relogin. Update this
plan and `PORT_STATUS.md`; no Drive upload (parent's job).

## Outcome

COMPLETED 2026-11-17 (coordinator-verified; parent verification +
Drive archive pending). Standard spellbook + listeners + autocast,
all three scope items landed; two coordinator-found integration
bugs fixed in-phase (the save split never wrote the live autocast
spell id; the listener spell-id resolution needed the real
client's widget-child/item values, not spell ids).

### What landed

1. **Standard spellbook data + implementations (Worker A).**
   `internal/game/combat_spells_data.go` (script-extracted,
   brace-counted, verbatim: 36 standard spells — 26 normal + 10
   effect — ids/levels/max hits/base+splash XP/runes/equipment/
   anim+projectile+graphic ids; spot-verified WIND_STRIKE 1152,
   BIND 1572, WATER_STRIKE 1154, CRUMBLE_UNDEAD 1171, MAGIC_DART
   12037, TELEBLOCK 12445, surges 21876–21879, god spells
   1190/1191/1192) and `internal/game/combat_spells.go`
   (1,503 lines): `canCastSpell` in exact TS order (level →
   `onSpellDisabled` first-answer gate → spellbook match → rune
   shortfall via `itemsToConsume` with the `magic:spell-cost`
   paid/covered split → rune-source check → equipment check →
   shared 1-tick delay + per-spell cooldown → `magic:rune-saving`
   → rune deletion/source consumption → cast context +
   `magic:cast-committed`), `missingItemsMessage` per TS,
   `CombatNormalSpell`/`CombatEffectSpell` with `FinishCast`/
   `OnHitCalc` event emission (`magic:hit-effects`,
   `magic:hit-effects-applied`, `magic:utility-effect-check`,
   `magic:utility-effect`), Charge `HandleSelfSpell` (180 XP,
   `charge:until` cycle+700, 100-tick cooldown), god-spell
   equipment rules + player-target drains, TeleBlock as a
   cycle-stamped attribute (`tele-block:until`, +500/300s or
   +250/150s with Protect from Magic — TS uses a wall-clock
   `SecondsTimer`, no TimerKey invented), Crumble Undead via
   `NpcDefinition.IsUndead()`/`IsAttackable()`, Iban Blast /
   Magic Dart equipment rules. `SpellID()` added to the
   `CombatSpell` interface (contract C1). XP conversion is
   `math.Round` (halves away from zero; per-cast error ≤0.5 XP
   — Go stores integer XP, the persist shape is `[]int`).
   EffectSpells table: 9 entries (Bones to Bananas 1159, Low
   Alchemy 1162, Telekinetic Grab 1168, Superheat Item 1173,
   High Alchemy 1178, Bones to Peaches 15877, Bake Pie 9107,
   Vengeance Other 9135, Vengeance 9136; aliases 3280/6884).
2. **Listeners + resolution (Worker B).**
   `internal/game/spell_resolve.go` (registry + resolver:
   `GetCombatSpell`/`GetCombatSpellByName`, `GetUtilitySpell`,
   `ResolveSpell`, `ResolveClientCombatSpellID`,
   `ResolveMagicItemSpellID` — Low Alchemy 1162, High Alchemy
   1178, Telekinetic Grab 1168, reanimations; −1 default) and
   `internal/listeners/spellhooks.go` + `magicitem.go` + the
   opcode cases in `listeners.go`/`npc.go`/`player.go`: opcode
   75 spell-on-NPC (dead/busy gates, NPC lookup,
   `magic:resolve-spell`, scripted route with `WalkToEntity`
   + `onSpellOnNpc` emit, CrossFloor 24 tiles documented as
   a gap — no CrossFloor policy seam), opcode 32
   spell-on-player (`magic:spell-on-player` event), MagicOnItem
   (alchemy LOW/HIGH with 3/5-tick pacing, slot re-validation,
   XP 31/65; telegrab with the ~3-tick landing delay, XP 43,
   "Too late - it's gone!"). All seams go through nil-safe
   `Deps.SpellHooks` func fields, wired per player in
   `cmd/server/spellwiring.go` (`InstallSpellListeners`,
   coordinator-added: cache spell-name lookup, spellbook
   interface read, `Combat.CastSpellOn`, the four gate
   adapters reading `game.Hooks` at call time, world task
   scheduling; `EmitSpellOnNpc` left nil — no hub family).
   Spell-on-object (opcode 2) deliberately NOT implemented
   (needs a hub family no worker owns this phase).
3. **Autocast + spellbook + hub (Worker C).**
   `internal/game/spellbook.go` (`Spellbook` + constants,
   `InterfaceID()` 1151/12855/29999/39999, `ChangeSpellbook`
   per `MagicSpellbook.ts`, entry restore via
   `applyEntrySpellbook`), `internal/game/autocasting.go`
   (full `Autocasting.ts` port: 58-slot map, selector lists
   + varp 664, god slots, `weaponRefusal`, `openAutocastSelector`,
   `SetAutocast`/`clearForStyle`/`clearForRefusal`,
   `RefreshAutocastIndicators` — varbits 275/276/2668, varp 43
   = 4 — `HandleAutocastWidgetAction` for 593:26/23/28 and
   201:1, `RestoreAutocastOnLogin`; the Phase 16
   `autocastingActive` deviation is closed — it now routes
   through `IsActive`, spell set AND staff equipped),
   `internal/plugins/spells.go` (`Api.OnSpellDisabled` /
   `OnSpellRuneBypass` first-answer gates,
   `RegisterSpellRuneSource` + `Hub.Check/ConsumeSpellRuneSource`;
   no Go rune-pouch exists, so with no source registered
   Check = false / Consume = no-op, documented no-source
   default), wired in `cmd/server/combatwiring.go`
   (`InstallCombatHooks` populates the four `CombatHooks`
   spell fields) and `cmd/server/server.go`
   (`InstallStandardSpells()` at boot, `InstallSpellListeners`
   per entering player, autocast widget dispatch via the
   one-line `HandleAutocastWidgetAction` call in
   `routeWidgetAction`, coordinator-added). Hub counts:
   **48 of 108 hook families firable** (+onSpellDisabled,
   +onSpellRuneBypass), **29 of 95 non-hook members**
   (+OnSpellDisabled, +OnSpellRuneBypass,
   +RegisterSpellRuneSource), 12 of 191 Core unchanged.

### Coordinator integration fixes (found by the live smoke test)

- **Save split: `autocastSpellId` was preserved, never owned.**
  `persist.OwnedState` carried the comment "nothing in Go
  sets a fight type or an autocast spell yet", so snapshots
  preserved the carried value and a live pick was lost on
  the next snapshot (the smoke showed `autocast=-1` after a
  successful pick). Fixed per `PlayerSave.ts:756` (live spell
  id, else −1): `OwnedState.AutocastSpellID` (always written,
  SpecPercentage precedent), `OwnedState()` extraction,
  `WithOwned` overwrite, `buildSnapshot` reads the live
  combat spell via `liveAutocastSpellID` (nil spell or a
  spell without `SpellID()` reads −1, the TS optional-chain
  case). Tests: `TestWithOwnedAutocastClears` + the
  set-path in `TestWithOwned` + `TestSnapshotWritesLiveAutocastSpell`
  (pick snapshots to 1152, clear snapshots back to −1).
  `fightType` stays preserved (nothing sets one yet).
- **Listener resolution needs the real client's values.**
  The smoke first sent spell ids as the widget child/item
  (e.g. High Alchemy as child 1178, item −1) and every
  utility cast refused: `GetCombatSpell(1178)` is nil (it is
  an EffectSpells utility spell, not a combat-registry
  spell) and the cache name lookup keyed by `(218<<16)|1178`
  has no entry. The real client sends the spellbook widget
  child and spell item id — High Alchemy child 47 / item
  3303, Telekinetic Grab child 30 / item 3290, Wind Strike
  child 11 / item 3273 (verified live against the cache:
  `GetSpellByName("High Level Alchemy")` = widget
  `(218<<16)|47`, item 3303) — and the resolver reaches the
  spell through the cache spell-name fallback, exactly as
  production does. The harness now sends the real values.
- **Alchemy XP discrepancy (31/65 vs 4000/20000): resolved,
  no fix needed.** TS `EffectSpells.baseExperience()` holds
  4000 (Low) / 20000 (High) verbatim, but TS never pays
  them: `MagicOnItemPacketListener.castAlchemy` pays its own
  constants LOW_ALCH_XP = 31 / HIGH_ALCH_XP = 65
  (`MagicOnItemPacketListener.ts:35-36, 296-297`). The
  4000/20000 are dead data in TS. Go mirrors this exactly:
  the listener pays 31/65 from its own constants
  (`magicitem.go:73-74`); the table holds 4000/20000
  verbatim and nothing in Go reads them for alchemy XP.

### Harness findings (all grounded, all in the smoke notes)

- **Staff rune coverage is plugin content, not engine.**
  `Spell.itemsToConsume` covers runes only through the
  `onSpellRuneBypass` hook — in TS the ElementalStaves
  plugin answers it (`plugins/items/ElementalStaves.plugin.js`).
  With no such plugin registered, a staff of air does NOT
  cover the air rune on combat spells (live: Wind Strike
  paid 1 air + 1 mind). The listener's own infinite air/fire
  staff sets (`runeCosts`, `MagicOnItemPacketListener.ts:105-110`)
  DO cover telegrab's air (live: law paid, air untouched).
  Both are TS-faithful without the plugin; the plugin is a
  content follow-up.
- **The repo ships no `item-alch.json` / `item-gameplay.json`.**
  Both are part of the 81 MB of upstream JSON data,
  deliberately not copied. Without `item-alch.json` every
  alchemy cast is refused (`policy.Allowed` needs highAlch >
  0 && lowAlch > 0 — the values come only from that file);
  without `item-gameplay.json` no weapon has a weapon-interface
  name, so every weapon reads UNARMED and no staff is ever
  recognised (`hasStaffEquipped` false, autocast never
  active). The smoke staged minimal fixtures in its temp
  roots (rune scimitar high 15360 / low 10240 verbatim from
  upstream `item-alch.json`; staff of air `STAFF` verbatim
  from upstream `item-gameplay.json`) and both paths then
  work. A production server run from the repo root hits
  both gaps; whether to import either file verbatim (the
  `music-areas.json` precedent) is a decision for Tyler/the
  parent, not taken here.
- **A blocked projectile line SW of spawn (smoke observation,
  same family as gap 11b).** From spawn (3089,3524) the line
  to a goblin at (3086,3528) is blocked: the magic attack
  selects, `CanReach` fails, the combat routes east around
  the obstacle, the route ends with no clear line, and the
  post-movement check cancels with the TS-faithful "I can't
  reach that!". The NE line to (3092,3520) in the same map
  region is clear (the manual Wind Strike landed there), so
  the region is loaded — the SW line hits a real map
  obstacle, not unloaded data. The smoke moved the second
  goblin due east (3095,3524) and the autocast attack fired
  there. No port bug found; the obstacle's identity is map
  trivia, not investigated further.

### Verification (measured)

- `go build ./...`, `go vet ./...`, `gofmt -l .`: clean.
  `CGO_ENABLED=0 go build ./...`: clean. 636 `.go` files
  (620 + 16), 17 packages.
- `go test ./... -p 1 -count=1`: all 17 packages pass
  (3,959 passing test cases including subtests, 0 failures —
  3,862 from phase 26 + 97 new cases. The workers added 23
  (A) + 39 (B) + 34 (C) test functions and the coordinator
  added 3 (1 wiring, 2 save-split); function counts and case
  counts differ because test functions carry subtests, and
  the workers each measured on the live tree, so the
  coordinator re-measured the total at the end.)
  `go test ./... -p 1 -count=1 -race`: passes 17/17. (The
  pre-existing `TestEngineTicksAtRate` timing test passed
  serialized; no tolerance change was made. Two earlier
  `-race` runs failed intermittently with NO `DATA RACE`
  reported — prime suspect the same flaky timing test under
  race-detector slowness, unproven.)
- Live phase-27 smoke test (throwaway harness in
  `/home/hatch/smoke27/`, driving a real WebSocket client
  against the real server binary + real rev-242 cache from
  temp roots outside the repo; staged fixtures — two
  goblin spawns, one ground-item coins stack, one
  `item-alch.json` entry, one `item-gameplay.json` entry —
  all in the temp roots, nothing copied into the repo;
  harness + roots kept at `/home/hatch/smoke27/` for
  inspection): 3 runs stable, 9 checks each, all passing.
  Session 1 (one login): a real High Alchemy on a rune
  scimitar paid 15,360 coins (1000 telegrab coins + 15,360
  = 16,360 in inventory), consumed 5 fire + 1 nature, and
  the scimitar left the inventory; a real Telekinetic Grab
  returned the staged 1,000 coins and consumed 1 law rune
  (air covered by the staff-of-air set); a real Wind Strike
  on goblin A (child 11 / item 3273, the cache name path)
  consumed 1 air + 1 mind and granted XP (Magic 210,000 →
  210,116/210,118 = wind strike + 65 alchemy + 43 telegrab);
  the autocast pick (593:23 → 201 slot 1) persisted as
  `autocastSpellId` 1152 in the logout save. Session 2
  (relogin): a plain Attack click on goblin B fired the
  restored autocast spell — 3–4 Wind Strikes in 10s, air +
  mind consumed one pair per cast (99 → 96/95), Magic XP
  +28/+30, autocast still 1152. No projectile frame is
  asserted anywhere: `StartCast` is a deliberate no-op this
  phase (Phase 29 owns the producers), so a cast sends no
  opcode-84 frame; casts are proven by runes + XP in the
  save. The repo holds no cache directory — the rev-242
  cache was downloaded once into the smoke seed root
  (242 MB, OpenRS2) and symlinked per run; the earlier
  MEMORY claim that it lived at `~/workspace/tsps-go/caches/`
  was wrong (no such directory exists).

### Residual gaps (documented, not built)

Ancients/Lunar/Arceuus spellbooks (tables, handlers,
`LunarSpells.handleNpcTarget/handlePlayerTarget` routing
beyond a documented drop); bind/curse/freeze plugin content
(events only — `magic:utility-effect` is emitted, no freeze
is applied until its plugin ports); Vengeance and other
Lunar utility spells; spell-on-object (opcode 2, needs a
hub family); `ApplyChargeMaxHit` has no call site (needs the
`GetMagicHitBounds` seam — do NOT wire via
`ModifyMagicMaxHit`, wrong order); NPC-target god drains (no
NPC level setter); TeleBlock effect-timer frame (no sender
seam); Magic Dart Slayer's-staff rules (plugin content);
ElementalStaves plugin (staff rune coverage);
animation/graphic/projectile/sound production (Phase 29);
Barrows Ahrim's autocast exception; `ItemAliases`;
rune-pouch sources (no-source default); alchemy confirm
dialogue (fail-closed); no click-delay system;
`Player.SendTab` missing; spell-on-object NOT STARTED.
Production data gaps: `item-alch.json` and
`item-gameplay.json` are not shipped (see above) — alchemy
refuses everything and no staff is recognised without them.
Next slices per plan order: Phase 28 (poison/venom + skulls
+ player death), Phase 29 (combat presentation). After
combat, content ports resume piece by piece only with
Tyler's direction.
