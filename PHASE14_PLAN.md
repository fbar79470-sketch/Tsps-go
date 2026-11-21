# Phase 14 plan (COMPLETED 2026-11-15 — coordinator: see Outcome at the end)

Migration plan §2 milestone 4, dispatched 2026-11-15 under Tyler's
unattended-progression authorization (2026-11-14, overnight). No
separate per-phase approval was sought.

## Scope

1. **DialogueManager core** — port `game/model/dialogues/` as core
   defines it: `DialogueManager` (state machine: indexed entries,
   `advance`/`startDialog`/`startDialogues`/`startStaticDialogue`,
   `handleResume`/`handleOption`, owner scoping), all entry types
   (`NpcDialogue`, `PlayerDialogue`, `OptionDialogue`,
   `StatementDialogue`, `ItemStatementDialogue`,
   `DoubleItemStatementDialogue`, `ActionDialogue`, `EndDialogue`),
   builders (`DialogueBuilder`, `DialogueChainBuilder`,
   `DynamicDialogueBuilder`), `DialogueOption`/`DialogueOptionAction`,
   `DialogueExpression`, `DialogueIdentifiers`. Ground the exact
   widget/interface frames each entry sends; port ONLY the
   interface-sending surface dialogue itself needs.
2. **Dialogue data loading** — port the loaders in
   `plugins/npcs/NpcDialogues.plugin.js` (~line 1210): reads
   `npc-dialogues.json`, `npc-dialogue-index.json`, and
   `event-dialogues.json` from the definitions directory, in TS
   order, with TS validation/error behavior. Validate read-only
   against the real definitions dir (counts parsed, nothing copied).
   Repo keeps only tiny hand-written starter files (Phase 6
   precedent). `dialogue-types.json`: ground who consumes it before
   deciding loader vs gap.
3. **Transcript playback + NPC dialogue dispatch** — port the
   `NpcDialogues` playback engine (steps: line/choice/random/
   condition/jump/message/shop; variant + condition selection via
   `onNpcDialogueVariant` / `onNpcDialogueCondition`; `npc-dialogue:*`
   custom events) as an engine-adjacent built-in system, following
   the Phase 6 NpcSpawns precedent — it is a plugin in TS, but
   milestone 4 explicitly names it. Port the NPC-click path
   (`NPCOptionPacketListener`, walk-to-NPC, then dialogue start)
   insofar as TS routes Talk-to through it. Shop steps: emit the
   `npc-dialogue:shop` event; no ShopManager exists yet (milestone
   7) — document as gap, do not build shops.
4. **Hub exposure** — dialogue hook families with registration
   methods + production emit sites, `getDialogueManager` (or actual
   TS member name), dialogue-starting members as `PluginApi`
   defines them. Update hub table counts. `game` never imports
   `plugins`.
5. **Save/state implications** — verify from TS whether dialogue
   state persists; handle through the owned/preserved split, no
   owned fields without grounding. Document explicitly if nothing
   persists.

## Out of scope

Timers + areas (milestone 5), NPC drivers + combat (6), shops/
trading (7), interface runtime (8) beyond the dialogue-needing
surface, clans/social (9), skill handlers + XP (10), sailing/
template regions/sounds (11), content ports beyond proof
(LumbridgeGuide is NOT this phase), the pre-existing far-WALK
routing bug (use staged clicks in the smoke test), embedded JS/
sidecar, scrypt check, MobileList.All perf work.

## Verification

`go build ./...`, `go vet ./...`, `gofmt -l .` clean,
`go test ./... -p 1 -count=1` + `-race`, CGO_ENABLED=0 build, and a
live smoke test: click spawned NPC → dialogue opens (correct widget
frames) → lines advance → choice branches → dialogue ends → state
cleared; hub-hook-started dialogue behaves the same. Claim "NPCs can
talk" only if verified live end to end.

## Outcome

COMPLETED 2026-11-15. All five scope items landed, verified
(build/vet/gofmt clean incl. CGO_ENABLED=0, 2,356 test cases
incl. subtests across 12 packages, race clean, live smoke
test 3 runs stable — see PORT_STATUS.md):

1. DialogueManager core: the state machine (indexed entries,
   `Advance`/`StartDialogues`/owner scoping, `HandleResume`/
   `HandleOption` with the 1-based child-index quirk), all 8
   entry types, builders, `DialogueOption`, the 30-value
   expression set, and the chatbox encoders (mount/close,
   chat-text, mesbox) through the `DialogueSender` seam,
   implemented session-side. `StatementLineHeight` uses the
   no-cache branch only; no FrameUpdater; no
   `closeChatboxFor` equivalent.
2. Dialogue data loading: `LoadDialogueData` with TS
   required/optional behavior (npc-dialogues.json REQUIRED,
   event-dialogues.json missing→`{}`, index any-failure→
   empty), `PickVariant`, aliases, speaker-id index,
   order-preserving decode. Validated read-only against the
   real dir (3,374 records, 74,970 steps, zero parse
   failures; nothing copied). `dialogue-types.json` has no
   loader — grounded gap, pet content only. Retired-NPC
   successors stay empty (plugin-private data, no approved
   read path).
3. Transcript playback + NPC dialogue dispatch: the
   NpcDialogues `startDialogue`/`run`/`presentMenu` engine
   as an engine-adjacent built-in (conditions, variants,
   jumps, random, overhead, menus with pagination,
   substitutions, `npc-dialogue:*` events), with menus
   presenting through the new `MultiChatboxPrompt` registry
   (the `sendMultiChatboxPrompt` port — a coordinator
   integration addition after the first smoke run showed
   choices hitting `unavailable()` with the seam unset).
   The NPC-click listener is implemented (all 5 click
   types, walk-to-NPC, Talk-to dispatch, `npc:operated`
   emit); combat/spell/item-on-NPC stay decode-drop. Shop
   steps deferred to milestone 7.
4. Hub exposure: 2 more families firable (21 of 108:
   variant, condition) and 1 more non-hook member ported
   (12 of 95: `sendMultiChatboxPrompt`), plus the 8
   `npc-dialogue:*` custom-event names on the existing
   surface. Verified: `PluginApi` has NO dialogue members —
   `getDialogueManager` is a `Player` method. `game` never
   imports `plugins`.
5. Save/state: nothing about dialogue persists
   (session-only; zero `PlayerSave` references). No new
   owned fields.

Live-verified end to end: click → lines advance → choice
branches → dialogue ends → state cleared; hub-hook-started
dialogue behaves the same. Gaps carried forward: shop
steps (milestone 7), static-binding registry, NPC
transforms, ChatTextLayout font paging, FrameUpdater dedup,
`closeChatboxFor`, far-WALK bug (pre-existing, still out
of scope), scrypt check, MobileList.All perf note.
