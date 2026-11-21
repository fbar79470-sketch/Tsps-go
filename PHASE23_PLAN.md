# Phase 23 plan — Lumbridge Guide dialogue (migration plan §3 item 3)

Status: COMPLETED (dispatched 2026-11-16 under Tyler's "keep going
without confirming" instruction; no per-phase approval sought;
verified 2026-11-16).

## Scope

Migration plan §3 item 3, the smallest dialogue content in the tree:
port `server/plugins/npcs/LumbridgeGuide.plugin.js` (7 lines) +
`server/plugins/npcs/lumbridgeguide/Dialogue.LumbridgeGuide.js`
(18 lines) to one Go content package under `internal/content/`,
following the `dateminutes`/`gracefuloutfit` precedent (compile-time
registration via one blank import in `cmd/server/pluginwiring.go`).
Porting rules are migration plan §4 (read before implementing):
faithful port with file:line citations, fixtures-only unit tests,
live verification before claiming behaviour, honest gaps in
PORT_STATUS.md, no copied `*.plugin.js` files, hub registration via
`internal/plugins`.

Out of scope: every other content port (Seeking Arrows, quests,
skills), dialogue data-file imports beyond what this piece strictly
needs (and never by copying upstream JSON — document the need), the
far-WALK bug, the y=3521–3522 map wall, the real production
scrypt-hash check, perf work. No scope expansion without TS
grounding. No Google Drive upload: the parent handles the
post-phase archive.

## Upstream grounding (verified by the coordinator, not trusted from summaries)

### LumbridgeGuide.plugin.js (entry shim, all 7 lines)

- `name: "LumbridgeGuide"` (line 2), `description: "Picks the
  Lumbridge Guide's dialogue depending on whether the player has
  lost their Amulet of the Eye."` (line 3). No `members` flag (F2P
  content), no `dependsOn`, no config declarations.
- `register` (lines 4–6) requires
  `./lumbridgeguide/Dialogue.LumbridgeGuide` and calls it with the
  api: the shim carries no behaviour of its own (plan §4: follow
  the require chain; never port an entry shim alone).

### Dialogue.LumbridgeGuide.js (all 18 lines)

- `PAGE = "Lumbridge Guide"` (line 3). `NORMAL =
  "standard-dialogue-if-the-player-still-has-the-amulet-of-the-eye-or-has-not-unlocked-it"`
  (line 4). `LOST =
  "standard-dialogue-if-the-player-lost-the-amulet-of-the-eye"`
  (line 5). `let pluginApi` (line 6), captured by `attach`
  (lines 15–18).
- `chooseVariant` (lines 8–13): if no entry of `event.pages` has
  `page === PAGE`, return `null` (line 9 — the `?.` means a
  missing pages array also falls through). Otherwise build
  `request = { player: event.player, reclaim: false }` (line 10),
  emit the custom event `"lumbridge-guide:amulet-status"` with the
  mutable request (line 11), and return the page-qualified object
  `{ page: PAGE, variant: request.reclaim ? LOST : NORMAL }`
  (line 12).
- `attach` (lines 15–18) registers exactly one hook:
  `api.onNpcDialogueVariant(chooseVariant)` (line 17).
- Header comment (lines 1–2): "The normal F2P conversation must
  remain available when members' recovery content is absent. The
  owning reward plugin answers whether recovery applies." The
  no-answerer default (NORMAL) is the designed behaviour, not a
  degradation to work around.

### The answerer (NOT ported — grounding for the boundary)

The sole listener of `lumbridge-guide:amulet-status` upstream is
`plugins/minigames/guardiansoftherift/Amulet.GuardiansOfTheRift.js:114`
(`status`, lines 50–52): `request.reclaim =
canReclaim(request.player)`, where `canReclaim` (lines 47–49) is
`Records.setting(player, UNLOCK) === 1 && questDone(player) &&
!owns(player)`:

- `UNLOCK = "gotr_amulet_of_the_eye_obtained"` (line 10).
  `Records.setting` (Records.GuardiansOfTheRift.js:133–134) reads
  the `guardians-of-the-rift:settings` player attribute (line 22:
  `SETTINGS_ATTRIBUTE`) — this is the attribute-surface read the
  task brief names. It lives in the GOTR answerer, not in
  LumbridgeGuide.
- `questDone` (lines 42–45) emits the `quest:is-complete` query
  for `temple_of_the_eye` (line 9).
- `owns` (lines 29–37) checks the four amulet variants in
  inventory, equipment, and banks, and records the last colour in
  the `guardians-of-the-rift:amulet-colour` attribute (line 27)
  when found.

None of that is in scope: no GOTR content, no quest system, no
invented answerer. In Go the event has no answerer, so `reclaim`
stays false and the guide always plays NORMAL in production until
GOTR content ports — the honest no-hooks default, exactly the
Phase 22 `skillcape:has-perk` precedent. The LOST branch is proven
by unit tests (fake emit) and by the live smoke test's temporary
probe answerer (removed afterward, zero references remain).

### TS engine grounding

- `onNpcDialogueVariant` (PluginTypes.ts:1091, handler shape
  1108–1114): return a variant name, a `{page?, variant}` object,
  or null/undefined to fall through. `emitNpcDialogueVariant`
  (PluginManager.ts:1289–1304): hooks run in registration order,
  the first truthy result wins (an object is truthy even with an
  empty variant), later hooks never run, no answer yields null.
- The event's `pages` array (PluginNpcDialogueContext,
  PluginTypes.ts:206–213) carries page names WITHOUT the
  `Transcript:` prefix: the Go loader strips it
  (`definition/dialogue.go`, `PageName`), and the playback's
  `TranscriptPageRef.Page` is prefix-free, so the Go handler's
  `page === "Lumbridge Guide"` check matches exactly as in TS.
- Variant choice routing (Go `ResolveTranscript`,
  `internal/game/transcript.go`): a page-qualified choice routes
  via `findTranscriptPage(pages, wantedPage)` and plays the named
  variant's steps when they exist; a missing variant falls back
  to `PickVariant` per page.

### Data entries the piece needs (verified read-only; NOT copied)

Upstream `data/definitions/npc-dialogue-index.json` maps NPC ids
306, 1179, 1181, 3393 to page `"Transcript:Lumbridge Guide"` with
7 variants (both target variants first, then
`contacting-through-astral-contacts-random-option`,
`treasure-trails-hard`,
`treasure-trails-hard-while-holding-the-puzzle-box`,
`giving-him-the-tarnished-locket-first-time`,
`giving-him-the-tarnished-locket-after-not-giving-him-the-amulet`).
Upstream `data/definitions/npc-dialogues.json` holds the
`"Lumbridge Guide"` record: `default: null`, the same 7 variants,
14 named branches. The repo's `{}` starters lack all of it, so
with starter data Talk-to yields no pages and no dialogue (the
existing Phase 14 behaviour, not a regression). With a real data
directory the port works as upstream does. No upstream JSON is
copied into the repo; the live smoke test uses hand-built fixture
data in a temp root (see Verification).

## Go current state (verified)

- `Api.OnNpcDialogueVariant` / `Hub.EmitNpcDialogueVariant` exist
  (`internal/plugins/dialogue.go`, Phase 14) with the
  first-answer-wins contract, `NpcDialogueVariantChoice`, and the
  `DialogueVariant` / `DialogueVariantOnPage` builders. The
  playback consults the selector through the `cmd/server`
  adapter; game never imports plugins.
- `Api.EmitCustomEvent` exists and runs handlers synchronously,
  so a mutable request payload reads back answers after the emit
  returns (the `gracefuloutfit` `capeIsSubstitute` precedent).
- Player attributes exist (Phase 11, lazy map on `Mobile`).
  Nothing in this port reads or writes an attribute: the
  attribute read belongs to the unported answerer (see above).
- The Talk-to click path requires a real NPC definition whose
  first action is "Talk-to": `StartDialogue` answers ONLY
  Talk-to clicks (`cmd/server/dialoguewiring.go`), and
  `npcOptionAt` reports no option for a nil definition
  (`internal/listeners/npc.go`). The `npcOverlays` machinery
  carries no name/actions fields, so a `--no-cache` smoke test
  CANNOT drive Talk-to through the click path. The live smoke
  test therefore boots with the real rev-242 cache (downloaded
  once from OpenRS2 into the temp root, reused across runs).
- Hub counts before this phase: 45 of 108 families firable, 26 of
  95 non-hook members, 12 of 191 Core. This port needs NO new hub
  surface: the variant family and the custom-event emit already
  exist, so the counts do not change.

## Implementation (coordinator-direct; no workers — the surface is one small package)

1. `internal/game/lumbridge_guide.go` (new, shared vocabulary
   following the `SkillcapeHasPerk` precedent in
   `equipment_effects.go`): `LumbridgeGuideAmuletStatusEventName
   = "lumbridge-guide:amulet-status"` and
   `LumbridgeGuideAmuletStatusRequest{Player *Player, Reclaim
   bool}`, with the answerer grounding and the no-answerer
   default documented at the site.
2. `internal/content/lumbridgeguide/lumbridgeguide.go` (new):
   package doc citing both upstream files, the event inventory
   (emits the query; listens to nothing else), the page/variant
   constants verbatim, a `guide` piece holding the emit func
   (captured at Register, as `gracefuloutfit` does; the JS
   module-level `pluginApi` global becomes a struct field —
   the only deviation, forced by Go having no per-plugin
   module state), `chooseVariant` mirroring lines 8–13 exactly
   (nil/empty/mismatched pages → nil; else emit and return
   `DialogueVariantOnPage`), and the `plugins.Plugin` value
   (name + description verbatim, no members flag) with
   attach-only Register + `init` + `var Plugin`.
3. `cmd/server/pluginwiring.go`: one blank import line.
4. `internal/content/lumbridgeguide/lumbridgeguide_test.go`
   (new, fixtures only): nil context → nil; nil/empty pages →
   nil; pages without the guide → nil; guide page + nil emit →
   NORMAL page-qualified choice; guide page + fake emit leaving
   reclaim false → NORMAL; guide page + fake emit setting
   reclaim true → LOST; the request carries the event player;
   plugin name/description verbatim.
5. Docs: this plan (Outcome), PORT_STATUS.md (header numbers,
   content inventory row, gaps, next-modules order),
   CONTENT_MIGRATION_PLAN.md (§3 item 3 DONE + boundary +
   residual gaps).

## Event inventory (document in code + PORT_STATUS.md)

- Emits `lumbridge-guide:amulet-status`
  (`game.LumbridgeGuideAmuletStatusEventName`) as a query with a
  mutable request: NO answerer in Go — the GOTR Amulet plugin
  that answers it is not ported, so `Reclaim` stays false and
  NORMAL always plays in production. Documented gap, not a
  wiring bug.
- Listens to `onNpcDialogueVariant` (existing firable family):
  produced by the transcript playback through the cmd/server
  adapter on every Talk-to resolution.
- Emits/listens to nothing else. No attributes read or written.
  No hub count change.

## Verification (coordinator-measured)

- `go build ./...`, `go vet ./...`, `gofmt -l .`,
  `CGO_ENABLED=0 go build ./...`: clean. `go test ./... -p 1
  -count=1` (serialized per the `TestEngineTicksAtRate` flake;
  no tolerance change) with the exact passing-case count incl.
  subtests. `go test ./... -p 1 -count=1 -race`: clean.
- Live smoke test, 3 runs stable, throwaway test + temporary
  flag-gated probe answerer (registered via `plugins.Register`
  from the test file only — no pluginwiring change), both
  removed afterward with grep-verified zero probe references
  references: temp root OUTSIDE the repo on the main
  filesystem (cache download needs it; /tmp is a 512 MB
  tmpfs), real rev-242 cache, real WebSockets, deadline-free
  reader goroutine, ClientHandshake after LOGIN, 10 ms ticks,
  memory saves. Fixture data in the temp root only: one NPC
  spawn (id 306 at 3090,3524, adjacent to the default spawn,
  wanderRadius 0), index entry 306 → `Transcript:Lumbridge
  Guide`, dialogue record with both variants carrying single
  obviously-fixture npc lines (`FIXTURE-NORMAL` /
  `FIXTURE-LOST` — no transcript content invented or copied).
  Each run: login → handshake → parse NPC_INFO (opcode 21)
  additions to find the guide's world index (type id 306) →
  NPC_OPTION_1 click (opcode 76) → observe the NORMAL fixture
  line on the wire with the probe disabled, then the LOST
  fixture line with the probe enabled (fresh login per
  branch), and the absence of the other branch's line.
- No stray processes, no leftover temp roots, no probe
  references, `game` never imports `plugins`.

## Outcome

Landed 2026-11-16, exactly as scoped. `internal/content/lumbridgeguide`
ports the 7-line shim + the 18-line dialogue unit (one
`onNpcDialogueVariant` registration, page/variant constants verbatim,
the `lumbridge-guide:amulet-status` query emitted through the hub's
synchronous custom-event path); the shared request type lives in
`internal/game/lumbridge_guide.go`; one blank import in
`cmd/server/pluginwiring.go` loads it. No new hub surface was needed,
so hub counts are unchanged (45 of 108 families firable, 26 of 95
non-hook members, 12 of 191 Core). The GOTR answerer was NOT ported
(boundary, grounded in the header comment of the upstream unit): in
production the query goes unanswered and the NORMAL variant always
plays, the honest no-hooks default; the LOST branch is proven by
fake-emit unit tests and was observed live only through the temporary
probe answerer, since removed.

Verification (coordinator-measured): `go build ./...`, `go vet ./...`,
`gofmt -l .`, `CGO_ENABLED=0 go build ./...` all clean; 601 `.go`
files, 16 packages; `go test ./... -p 1 -count=1` all pass (3,780
passing cases incl. subtests, 0 failures, 0 skips — 3,773 + 7 new
lumbridgeguide tests; the 3 plugin-wiring content-list expectations
were updated for the 4th compiled-in piece); `go test -race` passes;
zero JS files; `game` never imports `plugins`. Live smoke test
(throwaway, real rev-242 cache, temp root outside the repo, 2
invocations × 3 runs, test + temp root since removed): a real
NPC-option-1 click on the spawned guide (id 306, found by parsing
NPC_INFO frames) played the NORMAL fixture line with the probe
disabled and the LOST fixture line with the probe answering, with the
other branch's line and the no-handler fallback absent in both — the
NORMAL observation proves the hook overrode the loader's default
(LOST is first in file order). The guide's dialogue entries are NOT in
the repo's `{}` starters (documented need, nothing copied): with
starter data Talk-to yields no pages, existing Phase 14 behavior.

Residual gaps: no Go answerer for the amulet-status query (GOTR
content not ported); starter data lacks the guide's index + record
entries; the LOST branch was observed live only via the removed
probe. Far-WALK bug, y=3521 wall, scrypt check untouched.
