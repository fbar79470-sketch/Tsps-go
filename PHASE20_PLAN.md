# Phase 20 plan (skills shared handlers + XP sources — migration plan milestone 10)

Status: COMPLETED 2026-11-15 (dispatched under Tyler's
unattended-progression authorization; coordinator + 3 workers;
coordinator-verified).

## Scope (from the brief + migration plan §2 milestone 10)

Migration plan milestone 10, verbatim: "Skills shared handlers +
XP sources (the `SkillActions.js` pattern, skilling frameworks,
XP grants). Unlocks: `skills/` piece by piece; each skill still
ports separately." The plan text is thin; the scope below is
grounded in TS at `bfd81e3` (read-only checkout at
`~/tsps-bfd81e3`). Every behavior cites a TS file:line.

## Grounding: what TS core actually provides (verified by grep)

### SkillManager.ts is already mostly ported — Phase 20 closes its gaps

`internal/game/skills.go` already ports `SkillManager.ts`
(723 lines): EXP_ARRAY, combat formula, drains/boosts/level-ups,
caps, resting-level modifiers, `ensureCombatBaseline`, `setSkills`
normalization. Its header lists deliberate gaps; this phase
closes the ones whose systems now exist (hub since Phase 10,
interfaces since Phase 18, areas/Wilderness since Phase 15):

1. **The XP gate + world rate** (`SkillManager.ts:130-151`).
   `addExperience` first runs
   `PluginManager.emitCanGainExperience` (`PluginManager.ts:
   1665-1674`: every hook runs; a deny returns immediately with
   multiplier reset to 1; the combined multiplier is sanitized
   finite-and-positive else 1; no hooks → `{allow: null,
   multiplier: 1}`), then multiplies by
   `GameConstants.EXPERIENCE_MULTIPLIER * gate.multiplier`
   when `multipliers` is true (`GameConstants.ts:41`, from
   `world.json` `experienceMultiplier`; `setExperienceRate`
   validates finite-and-positive). Go currently hardcodes rate
   1 on both branches. Wire the real rate (Go config already
   parses it: `config/world.go:412-418`, default 1) and the
   real gate.
2. **Tenths + exp-drop position** (`SkillManager.ts:143-156`).
   After rates: `floor(xp*10 + 1e-6)/10`; then
   `sendExpDrop` (`PacketSender.ts:1759-1767`: opcode 116,
   `put(skill.getClientId())`, `putInt(exp)`); then the XP-lock
   early return (`skills:xp-locked` strictly true); then the
   200M cap check. CORRECTION (found during implementation,
   verified by two workers + the coordinator): `sendExpDrop`
   is a NO-OP in TS at `bfd81e3` — it hands the builder to
   the empty `PlayerSession.write()`
   (`PlayerSession.ts:228-230`), no native exp-drop encoder
   exists, and legacy opcode 116 collides with
   `WIDGET_SET_ROTATE_SPEED` (fixed length 8), so the frame
   never reaches a client and must never be framed. Go
   mirrors this: the hook fires at the TS position, the
   encoder is layout-only, and the session sender discards.
   (An earlier draft of this plan claimed the drop "is sent
   even for locked/capped gains"; that sentence was wrong
   and is superseded by this correction.)
3. **Level-up + experience-gained emits**
   (`SkillManager.ts:180-202`). On level-up: current level
   rises by levels gained, `emitPlayerLevelUp({player, skill,
   oldLevel, newLevel})`, APPEARANCE flag. On every landed gain
   (not refused/locked/capped): `emitExperienceGained({player,
   skill, experience: <landed post-cap delta>})` plus custom
   event `skills:experience-gained` with the post-rate tenths
   amount, `multiplied`, and `rate`. The two experience values
   differ by design; preserve both. Go emits none of these.
   Event types: `PluginTypes.ts:94-108, 344-354`.
4. **Custom events from setters** (`SkillManager.ts:530-535,
   589-594`). `setCurrentLevel` emits
   `skills:current-level-changed` when previous !== current
   (before refresh). `increaseCurrentLevel` emits
   `skills:restore` and reads back an adjusted amount (only
   when changed, finite, >= 0, floored). Go emits neither.
5. **The skill slot + start/stopSkillable**
   (`SkillManager.ts:622-649`, `Player.ts:1590-1597`). The
   player holds one current skill action (`skill: any`).
   `startSkillable` stops the previous action, sends interface
   removal, checks `hasRequirements(player)`, sets the slot,
   calls `start(player)`. `stopSkillable` calls `cancel(player)`
   on the slot, clears it, clears the creation menu. Content
   also sets the slot directly with bare `{cancel}` objects
   (`Prayer.plugin.js:296`: burying task; `DaddysHome.Quest.js`:
   `FurnitureBuild` Task with `hasRequirements`/`start`/
   `cancel`). Go has no slot; `StopSkillable` is a faithful
   no-op and `startSkillable` is unported. Callers of
   `stopSkillable` in TS core: `MovementQueue.walkToReset`
   (`MovementQueue.ts:1334-1344`), Emotes, Gambling,
   TeleportHandler, Drop/Equip/MagicOnItem/SecondGroundItem
   listeners. In Go, Drop/Equip already call `StopSkillable`
   (become live automatically); the walk-to-reset site names
   the gap in a comment (`movement_queue.go`, walkToReset
   section); Emotes/TeleportHandler/MagicOnItem/
   SecondGroundItemOption have no Go equivalent call site and
   stay gaps — do not invent them.
6. **`setLevel` guards** (`SkillManager.ts:236-282`).
   Wilderness guard (rights bypass), equipment-requirements
   guard (DEVELOPER bypass, "Please unequip your ..." refusal),
   Hitpoints floor (already ported), then assignment +
   `setHasVengeance(false)` + `BonusManager.update` +
   `WeaponInterfaceManager.assign` + `PrayerHandler.
   deactivatePrayers` + APPEARANCE flag. Go has Wilderness
   (`Wilderness.IsIn`), the requirements seam
   (`definition.ItemDefinition.Requirements`), and
   `DeactivatePrayers`; vengeance/BonusManager/
   WeaponInterfaceManager do not exist. Port what is
   groundable, document the rest. (No production caller exists
   today — see pressedSkill below — but the guards are core
   behavior, not content.)
7. **`pressedSkill` is DEAD CODE — do NOT port.**
   `SkillManager.ts:205-234` has zero callers in all of
   `server/src` and `server/plugins` (grep-verified). The
   skills-tab click is owned by content: `SkillGuide.plugin.js`
   offers `skills:stat-clicked`, `SetSkillLevel.plugin.js`
   claims it. Porting `pressedSkill` would port behavior TS
   never runs. Record the finding; do not port it.

### SkillActions.js is 30 lines — port the pattern, not a framework

`plugins/skills/SkillActions.js` exports exactly two functions:

- `pauseForLevelUp(event, sessions, stop, onResume)`: holds a
  skill's session through a level-up dialogue. `event` is the
  `level-up:popup-opened` custom-event payload (NOT the
  `onPlayerLevelUp` event — `PluginPlayerLevelUpEvent` has no
  `pause` field): `{player, skill, pause}` where `pause` is
  supplied by the SkillLevelUp plugin
  (`SkillLevelUp.plugin.js`, `openSkillPopup`: `pause: (action,
  resume, cancel) => state.actions.set(...)`). Semantics: no
  session → return; no `event.pause` → `stop(player)`; else
  set `session.levelUpPaused = true` and register resume/cancel
  closures that no-op if the session was replaced. 15+ skill
  files consume it via `onCustomEvent("level-up:popup-opened",
  ...)`, and every session tick loop skips `levelUpPaused`
  sessions.
- `actionInterval(emitter, player, skill, kind, ticks, saving
  = 0)`: emits `skills:action-interval {player, skill, kind,
  ticks}` (content may lower `ticks`; a league relic's 2-tick
  pace), then returns `max(1, (ticks|0) - saving)` — the
  tool saving applies after content's answer so they stack.
  The only CORE emit site of this event is
  `MagicOnItemPacketListener.ts:316` (alchemy pace), in a
  listener that is decode-drop in Go — so the event fires in
  Go only when content calls the helper. Document that.

The session pattern around these helpers (verified in
`Mining.plugin.js`, `Woodcutting.plugin.js`,
`Fishing.plugin.js`, `Prayer.plugin.js`): a `Map` of sessions
keyed by player, ticked by a submitted `Task`
(`api.getTaskManager().submit(new MiningTask(sessions))`) or
by a per-player Task, with `levelUpPaused` skips and
`player:interaction-cleared` / emote interrupts. Go already
has `game.TaskManager` (`internal/game/task.go`, `Schedule`
== TS `submit`) and `OnPlayerProcess` — what is missing is
content access to the engine TaskManager (`PluginApi.
getTaskManager`, `PluginTypes.ts:1377`, a non-hook member
currently unported) and the Go form of the two helpers.

### SkillLevelUp is CONTENT — do not port it

`plugins/interface/SkillLevelUp.plugin.js` (chat message,
graphic, sounds, jingle, group-233 pop-up, unlock boxes from
`data/level-up-*.json`) owns all level-up feedback, including
emitting `level-up:popup-opened`. It needs sounds/graphics/
jingles (milestone 11) and its data files. Phase 20 ports the
engine side only: the `onPlayerLevelUp` hook family firing
from `addExperience`, the `level-up:popup-opened` payload
contract as a Go type, and the `pauseForLevelUp` helper. The
pop-up itself stays content. Likewise `skills:produced`,
`skills:actions-per-cycle`, `skills:level-boost`,
`skills:xp-locked`, `skills:combat-xp-locked` are
content-to-content events (league/diary consumers); core
emits none of them — no engine work, no invented producers.

### Skill identity (Skill.ts, 187 lines)

24 skills in declaration order (index = `nextIndex` counter);
names; tab button ids; chatbox interfaces; client ids
(Construction 21→22, Hunter 22→21, Sailing 23; Sailing has
button/chatbox -1). Go has indices + the session-side client-id
table only. Port the name/button/chatbox/client-id table as
Go functions in the game package — future content (level-up
feedback, skill guides) needs names, and the proof needs them
for readable assertions.

### What is NOT in scope

Individual skill content (no Woodcutting/Fishing/Mining/...),
SkillLevelUp, SkillGuide/SetSkillLevel, modes (league rate
consumers of the gate stay content; the gate itself is
engine), sailing/template regions/sounds/projectiles
(milestone 11), quests/minigames/bosses/achievements, the
far-WALK bug, the y=3521 wall, the scrypt check, perf work.
`CreationMenu` (`model/menu/CreationMenu.ts`): no Go type
exists; `stopSkillable`'s menu clear stays a documented gap.

## Seam contracts (fixed — workers implement exactly these)

### Game-side seam (Worker A builds, Worker C wires)

`internal/game/skill_hooks.go` (new), mirroring the
`CombatHooks` pattern in `combat_hooks.go` (func fields only;
game never imports plugins):

```go
type PlayerLevelUpEvent struct { Player *Player; Skill, OldLevel, NewLevel int }
type ExperienceGainedEvent struct { Player *Player; Skill, Experience int }
type SkillsHooks struct {
    CanGainExperience func(player *Player, skill, experience int) (allow *bool, multiplier float64)
    EmitPlayerLevelUp func(*PlayerLevelUpEvent)
    EmitExperienceGained func(*ExperienceGainedEvent)
}
var SkillHooks SkillsHooks
```

Nil fields are inert: gate open with multiplier 1, emits
no-ops. `AddExperienceWithMultipliers` consults the gate
first (deny → return before any send, per TS order), applies
`rate = worldRate * gateMultiplier` when `multipliers`, else
1. World rate: package-level `SetExperienceRate(rate
float64)` / `ExperienceRate() float64`, default 1.0, setter
ignoring non-finite/<=0 values (mirroring
`GameConstants.setExperienceRate`).

Exp-drop: `SkillManager.SetExpDropHook(func(skill, exp int))`,
invoked at the TS position with the post-rate tenths value
(int; TS `putInt` truncates the float — Worker B verifies
`PacketBuilder.putInt` semantics).

Custom-event payloads (game package, emitted via the existing
`Player.EmitCustomEvent` seam, nil-safe):

```go
type SkillsExperienceGainedPayload struct { Player *Player; Skill, Experience int; Multiplied bool; Rate float64 }
type SkillsCurrentLevelChangedPayload struct { Player *Player; Skill, Previous, Current int }
type SkillsRestorePayload struct { Player *Player; Skill, Amount, Max int } // Amount mutable by handlers
```

Names: `skills:experience-gained`, `skills:current-level-
changed`, `skills:restore`. The restore read-back applies
the TS rule in `IncreaseCurrentLevel`.

Skill slot (Worker A): `Player.GetSkill() Skillable` /
`Player.SetSkill(Skillable)`, where `Skillable` is
`interface { Cancel(player *Player) }`, and
`StartableSkill` is `interface { Skillable;
HasRequirements(player *Player) bool; Start(player *Player)
}`. `SkillManager.StartSkillable(StartableSkill)` /
`StopSkillable()` per `SkillManager.ts:626-649` (interface
removal via the existing sender seam if one fits without new
coupling, else documented gap; creation-menu clear is a
documented gap).

Skill metadata (Worker A, game package): `SkillName(int)
string`, `SkillButton(int) int`, `SkillChatboxInterface(int)
int`, `SkillClientID(int) int`, grounded in `Skill.ts`;
out-of-range → `""`/`-1` per the existing skills.go guard
policy.

### Hub API (Worker B builds, Worker C wires)

New `internal/plugins/skills.go`: `Api.OnPlayerLevelUp`,
`Api.OnExperienceGained`, `Api.OnCanGainExperience` +
`Hub.EmitPlayerLevelUp`, `Hub.EmitExperienceGained` (run-all,
error-isolated, mirroring `PluginManager.ts:1101-1116`),
`Hub.EmitCanGainExperience(player, skill, experience)
(allow *bool, multiplier float64)` (veto-plus-accumulator,
mirroring `PluginManager.ts:1665-1674`). Event types carry
`*game.Player` + skill int. Update the unfired-family table
in `hooks.go` (skills paragraph → ported) and the firable
count 41 → 44.

TaskManager exposure: `Hub.SetTaskManager(tm
*game.TaskManager)` + `Api.GetTaskManager() *game.TaskManager`
(nil until the adapter installs it). Counted as a ported
non-hook member (18 → 19) only if it lands honestly.

### SkillActions helper (Worker C builds)

New package `internal/plugins/skillactions` (TS location:
`plugins/skills/SkillActions.js`, required by skill plugins;
Go form: helper package under the hub tree, importable by
future content; it must NOT import the plugins package —
the emitter is a plain func):

```go
type LevelUpPopupOpenedPayload struct {
    Player *Player // *game.Player
    Skill int
    Pause func(action any, resume, cancel func())
}
type Session interface { SetLevelUpPaused(bool) }
func PauseForLevelUp(event *LevelUpPopupOpenedPayload, sessions map[*Player]Session, stop func(*Player), onResume func(Session))
func ActionInterval(emit func(name string, payload any), player *Player, skill int, kind string, ticks, saving int) int
type ActionIntervalPayload struct { Player *Player; Skill int; Kind string; Ticks int }
```

`PauseForLevelUp` semantics exactly per `SkillActions.js`:
no session → return; nil `Pause` → `stop(player)`; else
mark paused and register identity-checked resume/cancel.
`ActionInterval`: nil-safe emitter, `max(1, ticks - saving)`
after the emit. Sessions hold pointers in practice; the
identity check compares the stored session with `==` and
must be documented as requiring comparable (pointer)
sessions.

## Worker split (disjoint file ownership)

- **Worker A — XP engine + skill slot** (`internal/game`
  only): seam + rate + gate wiring in `AddExperience`,
  exp-drop hook, level-up/gained/custom-event emits, restore
  read-back, skill metadata, skill slot + start/stop, the
  walk-to-reset `StopSkillable` call + stale-comment updates,
  `setLevel` guards (grounded subset), skills.go header gap
  updates, unit tests. Must NOT touch plugins/protocol/
  session/cmd/server/docs.
- **Worker B — wire format + hub** (`internal/protocol`,
  `internal/session`, `internal/plugins` except the
  `skillactions` subpackage): `EncodeExpDrop` (opcode 116,
  byte-verified against `PacketBuilder` semantics),
  `Session.SendExpDrop`, the three hub families + emits +
  table/count updates, `GetTaskManager` exposure, Core
  Skill/SkillManager table-only verification, unit tests.
  Must NOT touch game/cmd/server/docs.
- **Worker C — wiring + helper + proof + docs**:
  `cmd/server/skillswiring.go` (seam installation, exp-drop
  hook at entry, world rate at boot from
  `worldDef.ExperienceMultiplier`, TaskManager handle),
  `internal/plugins/skillactions`, the temporary probe
  exercise + live smoke (3 runs stable, probe removed
  afterwards, zero references remain), and docs
  (`PORT_STATUS.md`, `CONTENT_MIGRATION_PLAN.md` milestone 10,
  this file's Outcome section draft). Must NOT touch
  game/protocol/session/plugins (except `skillactions`).

## Verification (coordinator re-measures everything)

`go build`, `go vet`, `gofmt -l` clean; `CGO_ENABLED=0`
build clean; `go test ./... -p 1 -count=1` (serialized —
`TestEngineTicksAtRate` flakes in parallel; no tolerance
change) with exact passing-case count incl. subtests;
`-race` clean; live smoke 3 runs stable (probe drives the
framework end to end over real WebSockets: no opcode-116
frame (the exp-drop send is a TS no-op — see the grounding
correction), skill delta, gate multiplier/refusal, level-up +
experience-gained hooks, action-interval answer,
popup pause/resume); zero probe references; zero JS files;
game-never-imports-plugins intact; docs updated.

## Outcome

COMPLETED 2026-11-15. Coordinator-finalized after
integration and independent re-verification (all numbers
below were re-measured by the coordinator; Worker C's
draft, which they matched, is preserved in substance).

LANDED. All three worker contracts integrated: the
game-side `SkillsHooks` seam + world rate + exp-drop hook +
skill slot + metadata + `setLevel` guards (Worker A), the
layout-only exp-drop encoder + discarding session sender +
three hub skills families + `getTaskManager` (Worker B),
and the cmd/server wiring + `SkillActions` helper + live
proof + docs (Worker C). Hub counts verified by reading
the hub code: 44 firable families (41 + 3 skills), 19
ported non-hook members (18 + `getTaskManager`).

WIRING (`cmd/server/skillswiring.go`, new; hookup lines in
`server.go` added by Worker C — see Deviations):
`InstallSkillHooks` populates `game.SkillHooks` from the hub
(gate forwarded verbatim as `(*bool, float64)`; level-up /
gained events translated field-for-field) and installs the
engine `TaskManager` (`world.Tasks()`, the single instance
the world ticks) via `Hub.SetTaskManager`;
`installSkillExperienceRate` sets the rate once at boot
from `worldDef.ExperienceMultiplier`;
`installSkillExpDropWiring` installs the per-entry exp-drop
hook calling `Session.SendExpDrop`, as a parallel install
next to `installSkillsAppearanceWiring`. All nil-safe; no
behavior change with no content hooks registered.

HELPER (`internal/plugins/skillactions`, new, 10 unit
tests): exact port of `SkillActions.js` — `PauseForLevelUp`
(no session / no pause / identity-checked resume-cancel)
and `ActionInterval` (nil-safe emitter, post-content tool
saving, floor 1). Does not import `internal/plugins`.

PROOF (temporary probe content + throwaway live test, both
REMOVED afterwards; zero references remain, grep-verified):
a stub Mining action (engine `TaskManager` task +
`ActionInterval` + session map + `AddExperience`) over real
WebSockets, deadline-free reader, temp root outside the
repo, `--no-cache`. Observed, 6 runs stable across two
`-count=3` batches: gate hook doubled 50 XP to 100 (Mining
1 -> 2) and refused 100 XP (0 landed); skills-delta frames
carried Mining XP 100 then 110, never 200; level-up +
experience-gained hooks fired (chat marker on the wire);
the action-interval hook lowered the pace 5 -> 2 ticks
(grant cycles spaced exactly 2 apart); a simulated
`level-up:popup-opened` emit paused the session (no grants
for ~30 ticks) and resume restarted it; NO opcode-116
packet appeared on the wire and every message decoded
cleanly (see the grounding correction below).

VERIFICATION (re-measured by the coordinator):
`go build`, `go vet`, `gofmt -l` clean;
`CGO_ENABLED=0` build clean; 516 `.go` files, 13 packages,
0 JS files, 0 probe references; `go test ./... -p 1
-count=1`: 3,406 passing cases incl. subtests, 0 failures,
0 skips (3,335 + 71 new); `-race` passes all 13 packages;
real-binary boot check from a temp root (`--no-cache`,
rev 242, 600 ms tick, listening, HTTP 404, clean SIGTERM
shutdown, temp files removed). The coordinator also
re-verified the exp-drop finding independently (empty
`write()`, opcode-116 collision, zero native encoders),
read all three workers' code, and fixed the plan's
grounding sentence plus the two stale listener comments
at integration.

GROUNDING CORRECTION (applied at integration — the
plan text above now states the corrected finding): the
original draft's §"Tenths + exp-drop position" sentence
claiming "the exp-drop is sent even for locked/capped
gains" was WRONG. `sendExpDrop`
(`PacketSender.ts:1759-1767`) hands a `PacketBuilder(116)`
to `PlayerSession.write()`, which is EMPTY at `bfd81e3`
(`PlayerSession.ts:228-230`); no native exp-drop encoder
exists in `ClientProtocol.ts`; and legacy opcode 116
collides with `WIDGET_SET_ROTATE_SPEED` (fixed length 8,
`ServerPackets.ts:92,251`), so framing it would desync the
stream. Worker B verified all three claims; Worker C
re-verified them by independent reading. Go mirrors TS:
layout-only `EncodeExpDrop` + discarding `SendExpDrop`;
the hook seam still fires at the TS position. The proof
was adjusted accordingly (no-116 assertion, delta frames
as the wire proof) and the finding is recorded in
`PORT_STATUS.md` and the migration-plan milestone 10
boundary.

PAYLOAD CONVENTION (Worker A, recorded in `PORT_STATUS.md`
notes): the three skill custom events use a mixed
convention — `skills:experience-gained` and
`skills:current-level-changed` emit VALUE payloads (the
interface-state precedent), while `skills:restore` emits a
POINTER (`*SkillsRestorePayload`) so handler mutation is
visible to the read-back. Handlers for `skills:restore`
must type-assert the pointer form.

DEVIATIONS FROM THE PLAN (all within Worker C scope):
(1) Worker C added the three `server.go` hookup lines
itself (boot install + rate + per-entry install) rather
than leaving them for the coordinator, because the live
proof requires the hookup; the lines are minimal and
marked. (2) The temporary probe content used explicit
`plugins.Register` (once) + an env gate — no blank import
was ever added to `pluginwiring.go`, so parallel workers'
suites were never affected; the pluginwiring `Loaded()`
assertion stayed green throughout. (3) The throwaway
test's first version snapshotted delta frames too early
and flaked (2/3 runs); it was fixed to poll for the later
delta, then passed 6/6.

REMAINING GAPS (all recorded in `PORT_STATUS.md` /
migration plan): SkillLevelUp content, `CreationMenu`,
`pressedSkill` dead code (deliberately unported), the core
`skills:action-interval` emit site riding the decode-drop
MagicOnItem listener, `setLevel` vengeance/`BonusManager`/
weapon-interface gaps + unapplied rights bypasses, the
Emotes/TeleportHandler/MagicOnItem/SecondGroundItem
`stopSkillable` call sites, and all individual skill
content. The far-WALK bug, the y=3521 wall, and the scrypt
check remain untouched. Next: milestone 11
(sailing/template regions/sounds).
