# Phase 12 plan (COMPLETED 2026-11-15 — coordinator: see Outcome at the end)

Chat + commands, milestone 2 of `CONTENT_MIGRATION_PLAN.md` §2
("Chat/message encoders + chat broadcast + command registry";
`player.sendMessage` in 1,416 of 2,907 plugin files, 103
`registerCommand` calls in `commands/`; it gates nearly
everything and closes the phase 9 gap where refusal/examine/
level-up text was dropped silently). This is PORT_STATUS.md
next-module #1. It does NOT add the object registry
(milestone 3), dialogue (4), timers + areas (5), NPC drivers
+ combat (6), shops/trading (7), interface runtime (8),
clans/social incl. friends/ignores/private messaging (9),
skill handlers + XP sources (10), sailing/template regions/
sounds (11), any content port beyond what proves chat/
commands work, the real-production scrypt-hash check,
MobileList.All perf work, or HTTP extras/WebRTC. No embedded
JS runtime, no Node sidecar — both ruled out by the decided
strategy; do not build, prototype, or spike them. Do NOT copy
any `*.plugin.js` file into the repo.

Upstream reference: RSPSApp/tsps commit `bfd81e3`, read-only
checkout at `~/tsps-bfd81e3` (verified; TS root
`server/src/main/typescript/elvarg/`). Grounding entry
points (workers: cite file:line for every behavior):
`net/packet/impl/ChatPacketListener.ts` (opcode 190),
`net/packet/impl/CommandPacketListener.ts` (sub-dispatch of
190 on `::`), `net/NetworkBuilder.ts` `dispatch()` (the typed
switch, examine handling, routing), `Player.sendMessage` +
the `PacketSender`/`ClientProtocol` chat encoders (message
types/variants, formatting, wire encoding), the player-sync
chat mechanism (how TS surfaces chat text to nearby players),
and the command registry (`registerCommand` semantics: names,
aliases, permission/staff gating, argument parsing, `::`
handling) plus which commands, if any, TS core defines
vs `server/plugins/commands/` content (content stays NOT
STARTED). Chat-related hook families come from the 108-family
table in `internal/plugins/hooks.go`. Private messaging /
friends / ignore lists belong to milestone 9 unless the
milestone 2 text explicitly includes them — it does not, so
they stay NOT STARTED with the boundary documented.

## Scope, in order

1. Game-message sending: port `Player.sendMessage` and its
   chat encoders exactly as TS defines them (types/variants,
   formatting/color handling, wire encoding). Audit every
   site that currently drops text (listeners, skills,
   containers) and wire it through `sendMessage` ONLY where
   TS grounds the exact text and call site — no invented
   messages. Ungroundable sites stay gaps with comments.
2. Public chat: implement the chat packet listener per the
   TS listener (inbound decode incl. effects/colors/mode as
   TS defines them, chat text on the player, visibility to
   nearby players through the existing player-sync/
   appearance path, core rate/mute gates TS itself enforces).
   No invented moderation behavior (plugin-owned).
3. Commands: port the command registry + dispatch core
   (`registerCommand` semantics, gating, argument parsing,
   `::` handling), dispatch from chat input, and the hub
   `Api.RegisterCommand` surface. Port built-in/core commands
   ONLY as TS core defines them; plugin commands are content
   and stay NOT STARTED. If TS core has no built-in commands,
   port registry + dispatch + one test-only command (fixture
   only, never shipped as content).
4. Hub exposure: chat/command surface content needs
   (`sendMessage` on the player object, `Api.RegisterCommand`,
   chat-related hook families with registration methods +
   emit sites ONLY where this phase's engine produces those
   events). Keep `game`-never-imports-`plugins` intact.
   Update the hub member tables (hook families + non-hook
   members, with counts).
5. Save/state implications: determine from TS whether any
   chat/command state persists; handle through the existing
   owned/preserved split, no new owned fields without TS
   grounding. If nothing persists, document that explicitly.

## Verification

Build/vet/gofmt clean, CGO_ENABLED=0 build clean,
`go test ./... -p 1 -count=1` + `-race` (serialized for the
known `TestEngineTicksAtRate` flake; no tolerance change),
fixtures-only unit tests, plus a live smoke test (built
binary, temp data root outside the repo, `--no-cache`,
deadline-free reader, two clients for visibility): a
`sendMessage` from a hub hook reaches the client as the
correct wire frame; a chat packet from one client is visible
to a nearby second client per TS's gating; a test command
registered through the hub dispatches and replies; a
previously silent refusal now delivers its text. Claim each
behavior only if verified live.

## Outcome

COMPLETED 2026-11-15. All five scope items landed, verified
(build/vet/gofmt clean incl. CGO_ENABLED=0, 1,982 test cases
incl. subtests across 12 packages, race clean, live smoke
test 3 runs stable — see PORT_STATUS.md):

1. Game-message sending: `EncodeChatMessage` (opcode 120,
   `ClientProtocol.ts:1185-1217`), `Player.SendMessage` /
   `SendTypedMessage`, session senders via the optional
   `ChatMessageSender` extension interface. 12 text sites
   wired with exact TS text; ungroundable sites stay silent
   with comments (incl. the drop DEVELOPER message, the
   equip slot −1 server-log case, and level-up feedback,
   which is SkillLevelUp plugin content).
2. Public chat: full `ChatPacketListener.ts` port (normalize,
   `::` first, `allowChat` gates, social-packet emit point,
   `forceChat`, broadcast loop with dedup + relations gate),
   overhead text through the forced-chat sync update blocks,
   chat icons, mute/blocked-word gates from the already-
   ported `PlayerPunishment` lists. No invented moderation.
3. Commands: registry + dispatch + `::` parsing per
   `CommandPacketListener.ts`/`PluginManager.ts`, rights
   gating from the carried save. Finding: TS core defines
   NO built-in commands — all 103+ `registerCommand` calls
   live in `server/plugins/`; only registry + dispatch +
   a test-only fixture shipped (the live probe plugin was
   removed after verification).
4. Hub exposure: `Api.RegisterCommand` /
   `GetRegisteredCommands` / `SetCommandRights`,
   `OnCommand`, `OnSocialPacket`; 9 of 108 hook families
   firable, 8 of 95 non-hook members ported. The
   object-examine intercept is the `world:object-examine`
   custom event. `game` never imports `plugins`.
5. Save/state: nothing about chat/commands persists
   (registrations/overrides are process state, permissions
   are world.json config, mutes are txt lists); `Rights`
   is preserved verbatim. No new owned fields.

Examine (dispatch-inline in TS `dispatch()`) was
implemented as part of the chat-encoder work: opcodes 9/85
+ inventory/ground/worn routes, with the item-examine price
line as the documented gap. Live-verified end to end: hook
`sendMessage`, near/far chat visibility, overhead sync
text, command dispatch + replies, NPC examine, and the mute
refusal + command bypass. Gaps carried forward: price line,
relations-gate degeneration (milestone 9), no core
commands, rights from the carried save, single-click walks
over 64 tiles route nowhere (pre-existing Phase 4
search-map bound — staged clicks work).
