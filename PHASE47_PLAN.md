# Phase 47 Plan — NPC dialogs: Talk-to bindings

Tyler's order: "Add npc dialogs" (2026-11-21 20:55 PST), queued after Bank
(Phase 46, done) and Magic (Phase 45, done). This is Phase 47.

## §1 Survey (done 2026-11-22, three workers + coordinator read-through)

### §1.1 What upstream does

`plugins/npcs/NpcDialogues.plugin.js` (1,406 lines) makes exactly **2 hook
registrations** (`register`, line 1209):

1. `api.onCustomEvent("npc-dialogue:start", …)` (:1378) — on-demand playback
   of a named variant outside Talk-to.
2. `api.onAnyNpcInteraction({ "Talk-to": … })` (:1393–1404) — the Talk-to
   binding. **One generic any-NPC handler keyed on the action string
   `"Talk-to"`; no `onNpcClick` hook, no per-NPC registration.**

It does not register `onNpcDialogueVariant` / `onNpcDialogueCondition`
itself (those names appear only in its header comment, :6–7). It *calls*
the core emitters `PluginManager.emitNpcDialogueVariant` (~:1330) and
`emitNpcDialogueCondition` (~:899), and *emits* 9 distinct custom events
(`npc-dialogue:shop` :572, `:menu` :782, `:hook` :827/:1056, `:choice` :832,
`:condition` :899, `:line` :928, `:action` :1071/:1120, plus
`slayer:assignment` :1168 and `slayer:task-tip` :1173).

Talk-to handler (:1393–1404): if the NPC name is in `SPECIAL_NPC_DIALOGUES`
(`"Skully"`, `"Estate agent"`, `"Estate Agent"`, `"Alwyn"`, :129) it returns
`false` so those plugins' own scripted conversations run instead. Otherwise
`resolveTranscript(event)`; if nothing playable resolves it logs a
once-per-name warning (`reportDialogueGap`, :207–218) and plays the
placeholder `{ npc: "Sorry, i've nothing interesting to talk about yet" }`
(:131), then `startDialogue(...)` and returns `true` (handled).

**Resolution key: cache NPC id → index → transcript page title**
(`resolveTranscript`, :1262–1345):

1. Primary: `event.npcId` (`npc.getContentId(player)`) looked up in
   `npc-dialogue-index.json` (:1270–1272), trying `[npcId,
   event.npc?.getId?.(), PREDECESSOR.get(npcId)]` — the second covers
   transforms the index doesn't know (Dr Harlow 16277 → parent 3480), the
   third covers successors of retired NPCs via
   `plugins/npcs/data/retired-npcs.json` (Captain Harlan 16821 → 3216,
   :27–28). Each index entry gives `{page, variants[]}`; the page title
   (minus the `Transcript:` prefix) is the key into `npc-dialogues.json`.
2. Variant choice: a quest plugin's `emitNpcDialogueVariant` answer wins
   (first answer, `PluginManager.ts:1289`); otherwise `pickVariant`
   (:~190–200): the record's `default`, else first `"standard*"` variant,
   else first non-`"overhead*"` variant. `activation: "explicit"` records
   never auto-play.
3. Fallback: cache NPC name — `data[event.definition.getName()]`, then
   `aliasKeys` (base-name map stripping a parenthetical suffix, so `"Hops"`
   reaches `"Hops (Biohazard)"`, :239–247). Flat pages (953) hold `steps`
   directly; 2,421 pages hold `variants`.

Dispatch path: `NPCOptionPacketListener.ts` `executeOption` → walk to NPC
(`walkToEntity`, range 1) → `operate` → `dispatch` → first
`NpcInteractionManager.handle`, else
`PluginManager.emitNpcInteraction` (`PluginManager.ts:1246`: before-hooks,
name hooks, any-hooks, stopping when handled). `registerNpcActions`
(`~:3160–3185`) re-derives the action string from
`definition.getActions()[clickType - 1]`; the event carries no option
string.

Speaker name → NPC id happens only for chatheads, via `speakerIdIndex`
(:222–236): pass 1 maps cache NPC names → ids (first wins); pass 2 maps
transcript page titles → the first id indexed under them. Used at exactly
one place (~:1005): a typed line spoken by someone other than the talked-to
NPC picks that speaker's chathead id.

`DialogueManager.ts`: `startDialogues(builder, owner?)` (:115) →
`startDialogueMap` (:126) → entries send on the chatbox interface group
231. `NpcDialogue` (:12): head keyed by NPC **id**; visible name is the
explicit speaker name when supplied, else the cache definition name.

**Dialogue-hook registrations across `plugins/npcs/`** (real `api.…(` calls,
not comment mentions): `onNpcDialogueVariant` — 4 files (Entrana, Silk
Merchant, Tutors, LumbridgeGuide); `onNpcDialogueCondition` — 17 files;
union 18 files. `npc-dialogue:*` custom-event listeners: 23 files, 38 call
sites.

### §1.2 The index file (NOT imported — needs Tyler's approval)

`~/tsps-bfd81e3/server/data/definitions/npc-dialogue-index.json`:

- **4,782,584 bytes; SHA-256
  `0e66e4c4f5e7b779b595d936dab347f55180d08b2686d4cea64e508e538952d7`.**
- Top-level keys are NPC ids as strings (7,438 distinct ids, range 0–16764);
  values are arrays of `{page, variants}` — 12,450 page entries total
  (1 page: 4,368 ids; 2: 2,042; 3: 549; 4: 200; 5: 209; 6: 35; 7: 16; 8: 7;
  3,070 ids map to more than one page).
- Mapping direction: **NPC id → list of transcript pages + applicable
  variant names**. Sample: id `0` → `[{page:
  "Transcript:Tool Leprechaun", variants: ["standard-dialogue",
  "using-an-item-on-the-tool-leprechaun", "treasure-trails-easy"]},
  {page: "Transcript:Lost City", variants:
  ["talking-to-a-tool-leprechaun"]}]`.
- Cross-checks: 3,293 distinct pages, **all exist as keys in
  `npc-dialogues.json`**; 81 dialogue keys are never referenced by any
  index entry. 56,959 variant refs; all 11,735 distinct index variant
  names occur somewhere in `npc-dialogues.json`. **1,523 refs (2.7%) point
  at steps-only pages** with no `variants` dict (e.g. `Hudo` /
  `standard-dialogue`) — the variant name is a label there and the whole
  `steps` array plays.
- Lumbridge spot-checks: 306 (Lumbridge Guide) → `Transcript:Lumbridge
  Guide` (all 7 variants) + more pages; 3105 (Hans) → `Transcript:Hans` +
  `2025 Christmas event` + `Death to the Dorgeshuun` + …; 815 (Duke
  Horacio) → `Transcript:Duke Horacio` (`standard-dialogue`) + quest pages;
  2813 (Shop keeper) → `Transcript:Shop keeper (Lumbridge)`; 766 (a
  Banker) → `Transcript:Banker (Etceteria)` — the generic `Banker` page
  exists, but the index maps banker ids to location-specific pages;
  **226 (Cook) is absent from the index entirely**.

### §1.3 The imported dialogue file (already in the repo)

`data/definitions/npc-dialogues.json` (verbatim, Tyler-approved 2026-11-21,
SHA-256 `f50b9906…`, 45,911,508 bytes): 3,374 top-level keys = transcript
page titles. Record shapes: `{default, variants}` 1,764; `{steps}` 858;
`{branches, default, variants}` 638; `{branches, steps}` 94; `{activation,
default, variants}` 14; `{activation, branches, default, variants}` 5;
`{activation, branches, steps}` 1. 15,064 distinct variant names;
`standard-dialogue` most common (957 records). `default` is a string in
1,484 records, `null` in 937, absent in 953 — `null`/absent means the file
does not say which variant plays (resolved via index / quest state).
`activation` is only ever `"explicit"` (20 records). 722 records contain a
`random` step somewhere; 626 are pure-linear; 2,471 contain a typed step.
By default playback the first step is `random` in 230 records, a plain line
in 1,975, another typed step in 232; 937 records have no default steps at
all (`default: null` + variants only).

Lumbridge presence (exact, case-sensitive): Lumbridge Guide ✅, Banker ✅,
Hans ✅, Bob ✅, Man ✅, Father Aereck ✅, Duke Horacio ✅; Shop keeper ❌,
Shopkeeper ❌, Cook ❌, Fishing spot ❌. Near-misses exist as separate keys
(`Cook (Lumbridge)`, `Shop keeper (Lumbridge)`, `Shop assistant
(Lumbridge)`; 9 `Cook*` keys, 40 `Shop*` keys total). First NPC lines of
the default variants: Lumbridge Guide — "Greetings, adventurer. I am
Phileas, the Lumbridge Guide…"; Banker — "Good day, how may I help you?";
Hans — "Hello. What are you doing here?"; Bob — "Get yer own!"; Man —
"Not too bad, but I'm a little worried about the increase of goblins
these days."; Father Aereck — "Welcome to the church of holy Saradomin.";
Duke Horacio — "Greetings. Welcome to my castle." (Lumbridge Guide has
`default: null` — the line quoted is from the standard variant; the first
variant in file order is the lost-amulet one.)

2,249 / 3,374 (66.6%) dialogue keys appear as spawn names in
`npc-spawns.json`; the other 1,125 are quest/event/object/overhead-only
pages. `event-dialogues.json` in the repo is the 3-byte `{}` starter —
merged into the same catalogue at load, contributes zero records today.

### §1.4 What the Go side already has (the surprising finding)

The engine-to-Talk-to chain is **complete and wired end to end** — there is
no missing dispatch path and no missing engine:

- `internal/definition/dialogue.go:374` `LoadDialogueData`: loads
  `npc-dialogues.json` (required), `event-dialogues.json` (optional when
  missing), `npc-dialogue-index.json` (any failure silently yields an empty
  index — `loadDialogueIndex`, :665–726, already parses the upstream
  `{page, variants}` shape). `Record(name)` (:341), `IndexEntries(id)`
  (:349), `TranscriptRecord.PickVariant()` (:235, ports TS `pickVariant`),
  `computeDialogueAliases` (:728, ports `aliasKeys`),
  `computeSpeakerIDIndex` (:753, ports `speakerIdIndex`, fed by
  `DialogueLoadOptions.NPCName` wired to `store.NPC(id).Name()` in
  `cmd/server/dialoguewiring.go:221–232`).
- `internal/game/transcript.go` + `transcript_steps.go`: complete playback
  system (`DialoguePlayback` over `TranscriptDeps`); `ResolveTranscript`
  (:773) implements the TS order — id index first (NPCID, live NPC id,
  `Predecessors[NPCID]`), explicit activation, then name/alias fallback,
  then variant selection via `variantChoice` (:954) and `PickVariant()`
  per page; the steps-only-with-variant-label case falls through to the
  `PickVariant()` loop, which plays the flat `steps` (so the 1,523
  steps-only index refs are handled). `StartTalkTo` (:1041) returns false
  only for the four `SpecialNPCDialogues` (Skully, Estate agent/Estate
  Agent, Alwyn) or nil player; otherwise resolves and plays the resolved
  steps or the placeholder line `"Sorry, i've nothing interesting to talk
  about yet"` (`PlaceholderLine`, :87), returning `true` (click claimed).
  Step types: line, choice, random, condition, jump, message, action, end,
  call, unavailable, reference, bare speech. `StartPage` (:1107) and
  `StartNamedVariant` (:1191) cover explicit/item playback.
- Wiring: `cmd/server/dialoguewiring.go` (`dialoguePlaybackDeps` :63,
  `loadDialoguePlayback` :221, `SubscribeDialogueStart` :242 for
  `npc-dialogue:start`, `NpcHooks` :405 with the `StartDialogue` adapter
  :408–421 answering only Talk-to clicks, `InstallDialogueListeners` :473);
  called from `cmd/server/server.go:323/:327/:504/:960/:1110`. Hub dialogue
  families in `internal/plugins/dialogue.go`: exactly two, both with emit
  sites — `onNpcDialogueVariant` (:245, first truthy answer wins) and
  `onNpcDialogueCondition` (:261, first boolean wins), bridged to the
  game-side registries by the adapter in `dialoguewiring.go:150–170`.
- Content: Lumbridge Guide is the only registrant
  (`internal/content/lumbridgeguide/lumbridgeguide.go:120`); no
  `OnNpcDialogueCondition` caller exists anywhere outside the hub.
- Talk-to today (`internal/listeners/npc.go`: `routeNpcAction` :515 →
  `npcAction` :545 → `npcOperate` :717): static shop step, then
  `StartDialogue` on `"talk-to"`, then `EmitNpcInteraction`, then `"bank"`
  fallback, else `"Nothing interesting happens."` So Talk-to **does**
  dispatch to dialogue today; an NPC with no reachable transcript plays the
  placeholder line and the click is claimed.

**The exact gap is data + resolution reach, not machinery:**

1. The id index is the empty `{}` starter — every NPC whose transcript
   page name differs from its definition name (disambiguated pages,
   multi-page NPCs, location-specific pages like `Banker (Etceteria)`)
   is unreachable; only the name/alias fallback can reach a transcript.
   With it, `SpeakerIDByName` is empty, so paired/cutscene speakers never
   get a chathead id.
2. `Predecessors` is always an empty map as loaded; `ParseRetiredNPCSuccessors`
   (`dialogue.go:780+`) exists but no non-test caller fills it. Upstream
   `plugins/npcs/data/retired-npcs.json` (1,577 bytes) holds **12** successor
   mappings (e.g. 3216 → 16821 Captain Harlan, was Melee combat tutor).
3. Content coverage is one NPC (Lumbridge Guide variant hook); no condition
   hook is registered anywhere.

**No new hub family is needed.** Variant selection, condition answers, and
line/choice/action observation are all covered by the existing families and
the eight `npc-dialogue:*` custom events on the existing custom-event
surface.

**Ordering deviation (documented, NOT in scope):** in TS the transcript runs
inside `emitNpcInteraction` as an any-hook (last, after specific handlers);
in Go `StartDialogue` runs *before* `EmitNpcInteraction` (`npc.go`
header :56–71). A specific handler claiming Talk-to cannot pre-empt the
transcript in Go. No shipped handler claims Talk-to, so it is unobservable
today; moving it is follow-up work, not this phase.

## §2 Scope (sized honestly)

The binding machinery already exists, so this phase is NOT a from-scratch
port. It is:

1. **Fill `Predecessors`** — transcribe the 12 successor mappings from
   upstream `plugins/npcs/data/retired-npcs.json` (1,577 bytes) into Go
   with file:line citations (Phase 33 transcription precedent: values, not
   the file). Wire the map into the load path so `ResolveTranscript`'s
   predecessor fallback is live. Unit-test with the real mappings.
2. **Prove the id-index path end to end** with a fixture index (in-memory
   in unit tests; a small hand-written `npc-dialogue-index.json` overlay
   in the *temp smoke root only*, never in the repo): multi-page NPC,
   steps-only page with a variant label (the Hudo case), first-page-wins,
   `SpeakerIDByName` population, predecessor fallback through the loaded
   map. The real 7,438-id index is NOT imported in this phase — it needs
   Tyler's approval (see §7); the machinery must be proven ready so the
   import lights it up with no code change.
3. **Live-verify the name-fallback slice** (the path that works today
   without the index): talk to real NPCs in Lumbridge and observe the
   transcripts playing on the wire — Hans (3105), Man, Banker (Lumbridge
   1613/1618), Duke Horacio (815), Father Aereck (2812), Lumbridge Guide
   (306, variant hook → standard variant), plus the negative case Cook
   (Lumbridge) → placeholder line. Also live-verify the fixture-index leg
   (Shop keeper 2813 → `Shop keeper (Lumbridge)` page) in the smoke root
   with the overlay index.
4. **No new hub family, no dispatch changes, no engine changes** unless a
   live leg proves one is needed. The ordering deviation stays a residual.

Out of scope (residuals, §6): the real `npc-dialogue-index.json` import
(needs Tyler's approval); per-NPC variant/condition content registrations
beyond Lumbridge Guide (the 18 upstream files); `npc-dialogue:start`
explicit playback beyond what exists; event-dialogues content (file is
empty); the ordering deviation.

## §3 Port plan

New/changed files (all under the repo; no new package is expected — the
work is small and lives in the existing definition/game/wiring files):

- `internal/definition/dialogue.go` — `Predecessors` filled at load from
  the transcribed successor table (new small table + wiring in
  `LoadDialogueData`); keep `ParseRetiredNPCSuccessors` as the tested
  parser for the file shape.
- `internal/definition/dialogue_test.go` — loader test asserting the real
  12 mappings land in `Predecessors`; keep `TestDialogueFilesValid`
  (3,374 records, 0 index ids) unchanged — the index is still the starter
  in the repo.
- `internal/game/transcript_test.go` (or a new test file) — id-index
  resolution with a fixture `DialogueData` (multi-page, steps-only +
  variant label, first-page-wins, speaker-id lookup via
  `SpeakerIDByName`); predecessor fallback through the *loaded* map
  (the existing `TestResolveTranscriptPredecessorFallback` hand-fills the
  map — extend or add a loaded-map variant).
- Alias check: verify `Shop keeper` → `Shop keeper (Lumbridge)` is what
  `computeDialogueAliases` produces (first key in catalogue order with a
  playable variant) and test it if so; if the alias resolves elsewhere,
  record the actual target in Results and do not force it.

Do not touch: `data/definitions/npc-dialogue-index.json` (stays `{}`),
`npc-dialogues.json`, `event-dialogues.json`, `internal/listeners/npc.go`
dispatch order, hub families.

## §4 Live verification (harness /home/hatch/smoke47/)

Fresh temp root, real server binary + real rev-242 cache (symlink to
`/home/hatch/smoke27/seed-root/caches/osrs-242_2026-11-03`), per-mode
`npc-spawns.json` in the temp root (Phase 35 lesson: the driver must write
it or the world has zero NPCs). Legs run singly if a full pass exceeds the
background-exec wall clock. Legs (each: exact NPC id, spawn tile, observed
chatbox lines in order, speaker name shown):

- `guide` — Lumbridge Guide 306: standard variant plays (amulet-status
  unanswered → NormalVariant), first line "Greetings, adventurer. I am
  Phileas, the Lumbridge Guide…".
- `hans` — Hans 3105: "Hello. What are you doing here?"
- `banker` — Banker 1613/1618: "Good day, how may I help you?" (name
  fallback to the generic `Banker` page).
- `duke` — Duke Horacio 815: "Greetings. Welcome to my castle." + the
  choice menu appears.
- `aereck` — Father Aereck 2812: "Welcome to the church of holy
  Saradomin."
- `man` — a Man spawn: goblin-worry line.
- `cook-negative` — Cook (Lumbridge): placeholder line "Sorry, i've
  nothing interesting to talk about yet" and the gap log.
- `shopkeeper-index` — Shop keeper 2813 with the fixture index overlay in
  the smoke root: the `Shop keeper (Lumbridge)` page plays via the id
  index (proves the index path live without the real import).

## §5 Verification

- `go build ./...`, `go vet ./...`, `gofmt -l .`,
  `CGO_ENABLED=0 go build ./...` — all clean.
- Full suite `-p 1 -count=1 -v` (baseline at HEAD a41843d: 5,129 tests
  per the Phase 45 commit message — recount, do not trust it blindly).
- `go test -race ./...` with the known-flake protocol: known
  `TestEngineTicksAtRate` 5 ms timing flake in `internal/game`, known §20
  `cmd/server` race-load timing issues, and the known
  `TestReloginRoundTripOverWebSocket` disconnect-save timeout are NOT data
  races — if the race run fails only on one of those with no DATA RACE
  warning, re-run that package once before reporting; any DATA RACE
  warning is a real finding.
- No commits by the coordinator; no touching live36/tsps-full; no broad
  pkill; no `.js` files; clean up temp probe files.

## §6 Residuals (gap 30)

- Real `npc-dialogue-index.json` import (needs Tyler's approval — see §7):
  unlocks id-index resolution for 7,438 NPC ids / 12,450 page entries.
- Ordering deviation: transcript runs before `EmitNpcInteraction` in Go,
  inside it in TS (`npc.go:56–71`) — unobservable today.
- Per-NPC variant/condition content: 18 upstream files register variant
  or condition hooks; only Lumbridge Guide is ported. Quest-gated variants
  play their default/standard variant today.
- `npc-dialogue:start` explicit playback exists but is unit-covered only.
- Event-dialogues: file is empty; no event transcripts exist.
- 81 dialogue keys are never referenced by any index entry (quest/event/
  object pages — reachable only by name or explicit activation).

## §7 Data file needing Tyler's import approval (NOT imported)

- `server/data/definitions/npc-dialogue-index.json` — **4,782,584 bytes,
  SHA-256
  `0e66e4c4f5e7b779b595d936dab347f55180d08b2686d4cea64e508e538952d7`**
  (NPC id → transcript pages, 7,438 ids / 12,450 entries). Required for
  full Talk-to coverage: without it, only NPCs whose definition name (or
  alias base) matches a transcript page title resolve; with it, the
  id-index path this phase proves (fixture-tested + live-verified with the
  overlay) lights up with no code change.

## §8 Results

**Port (worker, coordinator-verified):** 3 files changed, no new package, no
new hub family, no dispatch changes.

- `internal/definition/dialogue.go` — transcribed the 12 successor mappings
  from upstream `plugins/npcs/data/retired-npcs.json` into
  `retiredNPCSuccessors` (successor → retired, each line cited), wired
  `LoadDialogueData` to fill `DialogueData.Predecessors` from a fresh copy
  per load; updated the two stale comments that said the map stays empty.
  `ParseRetiredNPCSuccessors` kept as the tested file-shape parser.
- `internal/definition/dialogue_test.go` — new `TestLoadDialoguePredecessors`:
  asserts all 12 mappings land via `LoadDialogueData`, cross-checks the
  transcribed table against `ParseRetiredNPCSuccessors` run over a verbatim
  copy of the upstream successors object, asserts per-load map independence.
  `TestDialogueFilesValid` untouched (3,374 records, 0 index ids).
- `internal/game/transcript_test.go` — 4 new tests:
  `TestResolveTranscriptPredecessorFallbackLoadedMap` (16821 → 3216 → Bob
  page through the loaded map), `TestResolveTranscriptMultiPageFirstPlayableWins`,
  `TestResolveTranscriptStepsOnlyPageWithVariantLabel` (the Hudo case: flat
  steps play with no hook, with a hook answering `standard-dialogue`, and
  with a hook answering an unrelated label),
  `TestSpeakerIDByNameFromFixtureIndex` (loader populates
  `SpeakerIDByName["Alice"] = 456` from a fixture index; a paired-speaker
  typed line resolves that chathead id in playback).
- Verify-don't-change: `ResolveTranscript` + `PickVariant` + `wantedSteps`
  read and confirmed — the steps-only fall-through and name/alias fallback
  work exactly as the plan claims; no fix needed there.
- Alias finding (not forced): `computeDialogueAliases` resolves
  "Shop keeper" → "Shop keeper (Al Kharid)" (first playable key in
  catalogue order), not Lumbridge; likewise "Cook" → "Cook (Blue Moon
  Inn)", "Banker" → "Banker (Al Kharid)", "Shopkeeper" → "Shopkeeper
  (Aldarin)". The Lumbridge pages are reachable only via the id index.

**Verification (coordinator):** `go build ./...`, `go vet ./...`,
`gofmt -l .`, `CGO_ENABLED=0 go build ./...` — all clean. Full suite
`-p 1 -count=1 -v`: **5,134 tests (3,915 top-level + 1,219 subtests),
0 failures, 0 skips, 30 packages** (5,129 at HEAD a41843d + 5 new,
reconciled from the full log). Race: see the phase-47 paragraph in
PORT_STATUS.md.

**Live legs (harness `/home/hatch/smoke47/`, real server binary + real
rev-242 cache, legs run singly) — 7 of 8 PASS:**

- `guide` PASS — Lumbridge Guide 306 at (3238,3220), player (3238,3219):
  13 lines, 7 menus. Line 1 [Lumbridge Guide]: "Greetings, adventurer. I
  am Phileas, the Lumbridge Guide. I am here to give information and
  directions to new players. Is there anything I can help you with?"
  Menu 1: "Where can I find a quest to go on?" / "What monsters should I
  fight?" / "Where can I make money?" / "Where can I find more
  information?" / "More options...". Chain loops via "Yes please" →
  jump-to-initial (capped at 20 advances).
- `hans` PASS — Hans 3105 at (3207,3233), player (3207,3232): 3 lines,
  1 menu. Line 1 [Hans]: "Hello. What are you doing here?" → [alice]
  "I'm looking for whoever is in charge of this place." → [Hans] "Who,
  the Duke? He's in his study, on the first floor."
- `banker` PASS — Banker 1618 at (3090,3242), player (3092,3242) (counter
  branch, radius 2): 1 line, 1 menu. Line 1 [Banker]: "Good day, how may I
  help you?" Menu: "I'd like to access my bank account, please." / "I'd
  like to check my PIN settings." / "I'd like to collect items." / "I'd
  like to buy more bank slots." / "What is this place?"
- `duke` PASS — Duke Horacio 815 at (3212,3220), level 1, player
  (3211,3220), level 1: 9 lines, 3 menus. Line 1 [Duke Horacio]:
  "Greetings. Welcome to my castle." Menu 1 includes "I seek a shield
  that will protect me from dragonbreath." — picking it played the Dragon
  Slayer opening ("Elvarg, the dragon of Crandor island!" …). Gotcha: the
  leg seeds z=1 (per-plane NPC sync; the Duke never appears on level 0).
- `aereck` PASS — Father Aereck 2812 at (3244,3205), player (3245,3206):
  9 lines, 2 menus. Line 1 [Father Aereck]: "Welcome to the church of
  holy Saradomin." The "Who's Saradomin?" branch played in full.
- `man` PASS — Man 3106 (spawn (3216,3219), wanders; player ended
  (3209,3228)): the goblin line arrived on attempt 17 of 60 — [alice]
  "Hello, how's it going?" → [Man] "Not too bad, but I'm a little worried
  about the increase of goblins these days." → [alice] "Don't worry, I'll
  kill them." The Man page starts with a `random` step with 23 options,
  so the goblin branch is ~1/23 per Talk-to; attempts 1–16 played other
  branches.
- `cook-negative` — **corrected leg premise, not a port bug:** Cook 4626
  at (3210,3214) played a real transcript, not the placeholder — line 1
  [Cook]: "What do you want? I'm a little busy here, so make it quick!"
  with a 3-option menu. Diagnosis: the alias fallback maps "Cook" to
  "Cook (Blue Moon Inn)" (first playable `Cook (...)` key in catalogue
  order), so every NPC named "Cook" resolves there and the placeholder
  cannot play for Cook. A true no-transcript case needs an NPC whose name
  *and* alias base match no page — not observed live in this phase.
- `shopkeeper-index` PASS — Shop keeper 2813 at (3210,3246), player
  (3209,3247), via the fixture index overlay in the smoke root only:
  line 1 [Shop keeper]: "Can I help you at all?" with the exact 2-option
  Lumbridge menu ("Yes please. What are you selling?", "No thanks.").
  This proves the id-index path: the alias fallback would have played
  "Shop keeper (Al Kharid)", whose menu has 4 options. Repo
  `data/definitions/npc-dialogue-index.json` verified still the 3-byte
  `{}` starter afterwards.

**Harness notes:** Talk-to is clickType 1 = opcode 76 (`NativeOpNpc1Alt`).
Continue = opcode 62 `RESUME_PAUSEBUTTON` with widget `(group<<16)|5`
(group 231 NPC / 217 player); option pick = opcode 62 with widget
`(219<<16)|1` + 1-based index. Dialogue lines are `WIDGET_SET_TEXT`
(105) on group 231 child 6 (speaker: child 4); **option menus arrive on
opcode 170 (`RUN_CLIENT_SCRIPT`, u16 script id), not opcode 110** — the
harness initially missed menus for this reason. Wire-text caveat: the
server wraps dialogue text at 53 chars and joins lines with a space
(`CharacterPages`), which doubles the space at every wrap point on the
wire; leg checks compared whitespace-normalized text.

**Supplementary finding (not fixed, not this phase's scope):** Talk-to on
Banker 1613 at (3088,3242) failed with "I can't reach that!" — the player
walked around to adjacent (3087,3242) and the walk-to task still gave up;
the counter branch (`canInteractWithUnreachableNpc`, radius 2) did not
fire from that west-side tile, most likely the projectile-ray/clip check
reports the banker unreachable from there. 1618 on the customer side
works. Recorded in gap 30.

**Unit-covered only, not observed live:** the placeholder line for a
truly transcript-less NPC; `npc-dialogue:start` explicit playback;
paired-speaker chathead resolution beyond the unit test; quest-gated
variant selection (no condition hooks registered).

