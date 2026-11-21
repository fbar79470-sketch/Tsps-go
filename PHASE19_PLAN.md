# Phase 19 plan (clans/social + friends/ignores — migration plan milestone 9)

Status: COMPLETED 2026-11-15 (dispatched under Tyler's
unattended-progression authorization; coordinator + 3 workers).

## Scope (from the brief + migration plan §2 milestone 9)

Migration plan milestone 9, verbatim: "Clans/social +
friends/ignores. Unlocks: clan content, social packets,
ignore-list behaviour." The plan text is thin; the scope below
is grounded in TS at `bfd81e3` (read-only checkout at
`~/tsps-bfd81e3`). Every behavior cites a TS file:line.

### The core/plugin boundary (verified by grep, decisive for scope)

TS core provides the relations engine + the packet plumbing and
NOTHING ELSE social:

- `PlayerRelations.ts` (383 lines, whole file): friend/ignore
  lists as bigint name-hashes (caps 200/100), friend ranks
  0..6, friends-chat channel name / last owner / entry-talk-kick
  ranks, private-chat status (ON/FRIENDS_ONLY/OFF) + public
  (0..4) / trade (0..2) modes, the gates
  (`canReceivePublicChatFrom` :90-97, `isVisibleTo`,
  `canReceivePrivateMessageFrom`), `updateLists` presence
  broadcast, `addFriend`/`deleteFriend`/`addIgnore`/
  `deleteIgnore` with exact refusal text, `message()` PM
  delivery, `onLogin` sends, `getPrivateMessageId`.
- `PacketSender.ts` legacy social sends: `sendFriendStatus`
  (opcode 221), `sendFriend` (50, world+9 rule),
  `sendDeleteFriend` (51), `sendAddIgnore` (214),
  `sendDeleteIgnore` (215), `sendChatOptions` (122),
  `sendPrivateMessage` (via `encodeChatMessage` kind
  `private_in`), `sendSpecialMessage` (:322-326, kinds
  `clan`/`channel`), `sendShowClanChatOptions` (115).
- Core encoders in `ClientProtocol.ts`: `encodeFriendsChatSnapshot`
  (opcode 121, :1218-1250), `encodeClanSettingsFull` (124),
  `encodeClanChannelFull` (125), `encodeClanChannelDelta` (126),
  `encodeClanSettingsDelta` (127), `encodeClanVar` (128)
  (:1312-1400) — all exposed via `PluginCoreApi`
  (`PluginTypes.ts:1653-1658`).
- Decode (`ClientProtocol.ts:849-899`) + dispatch
  (`NetworkBuilder.ts:894-911`): CHAT kinds, `CLAN_CHANNEL_ACTION`
  (181), `FRIENDS_CHAT_ACTION` (196), `PRIVATE_MESSAGE` (197),
  `CHAT_FILTER` (198), `REPORT_ABUSE` (199). Dispatch routes
  EVERY social packet to `PluginManager.emitSocialPacket` and
  takes no other action (guarded on `this.player`).
- `Player.onLogout` calls `updateLists(false)` after the logout
  hooks, before the save (`Player.ts:725-765`).
- `PlayerSave.ts` persists friends/ignores/friendRanks/channel
  name/last owner/entry-talk-kick ranks (:175-182, apply
  :699-725, snapshot :819-830), normalized to positive unique
  stringified bigints capped at 200/100 (:482-516). It has NO
  chat-mode or privateMessageId fields: modes persist only via
  the FriendsList plugin's `chat.filters` attribute.

Plugin content (NOT ported — the engine unlocks it, later
content pieces port it):

- `plugins/interface/FriendsList.plugin.js` +
  `FriendsChatManager.js` (914 lines): friends-chat channels
  (join/leave/kick, ranks, bans), snapshot sending, PM
  friend-gate + delivery, chat-filter attribute persistence,
  widget actions, panel restore. Its methods
  (`handleRelationAction` :433-470, `handlePrivateMessage`
  :242-268, `deliverPrivateMessage` :270-277, `setChatFilters`
  :278-283, `sendSnapshot` :386-431, `onLogin` :193-210) are
  the CONSUMER CONTRACT for the engine API: they show exactly
  which relations/encoder calls content makes. Cite them;
  do not port them.
- `plugins/interface/clans/` (`Clans.plugin.js` + 10 files,
  ~5,082 lines): the whole clan system. NOT ported; only its
  core encoders (above) land here.

### Scope items

1. **PlayerRelations port** (`internal/game`): the whole
   `PlayerRelations.ts` surface, int64 name-hashes
   (37^12 < 2^63, per the `session.NameToLong` proof).
   `Misc` name functions needed by relations and absent in Go:
   `longToString` (`Misc.ts:814-837`), `formatName`/`fixName`
   (:801-812, :840+). (`stringToLongBigInt` already exists as
   `session.NameToLong`; the game layer takes local copies per
   the `grounditems.go` precedent — game must not import
   session.) `shownName` needs DisplayNames: a seam, wired by
   the coordinator to `persist.DisplayNames` (Go API exists).
   `Player.getLongUsername` (`Player.ts:976-978`): field +
   accessor, set at entry from `session.NameToLong`
   (TS sets it at enterWorld).
2. **Relations gate live**: `CanReceivePublicChatFrom`
   (`internal/game/chat.go`) reads live relations, closing the
   Phase 12 degeneration (exact only for a fresh player).
3. **Protocol**: decoders for 181/196/197/198/199 (exact
   layouts incl. the signed rank byte and the unknown-action
   → raw behavior); encoders for 221/50/51/214/215/122/121/
   124/125/126/127/128/115. The snapshot (121) and clan
   (124-128) encoders have NO producers — like Phase 13's
   merge/animation encoders, they exist for content.
4. **Session senders**: the `PacketSender` social sends above,
   through the optional-interface seam precedent
   (`ChatMessageSender` in `internal/game/message.go`).
5. **Dispatch**: `internal/listeners` cases for the five
   opcodes + the CHAT clan/friends_chat kinds: decode, guard
   on player, emit to the hub, no further action. This
   replaces the `ReasonPluginSocial` drops. The `chat_filter`
   decode-drop and the game-kind no-action behavior stay as
   TS has them (game kind: no action, `NetworkBuilder.ts`
   :894-903).
6. **Persistence**: friends/ignores/ranks/channel fields move
   from preserved-verbatim to OWNED in `persist.OwnedState`
   (Phase 11 attributes precedent: nil = preserve, non-nil =
   overwrite). Entry loading fills live relations (apply
   :699-725); snapshots rebuild from live relations (snapshot
   :819-830). Save JSON shape is unchanged (fields exist).
7. **Login/logout wiring** (`cmd/server`): entry sets
   `LongUsername` + loads relations; logout calls
   `updateLists(false)` at the TS-ordered seam (after logout
   hooks, before save). Relations `OnLogin` sends are EXPOSED
   but NOT wired by core (TS: the FriendsList plugin calls
   them, `FriendsChatManager.onLogin`).
8. **Hub**: `SocialPacket` gains all 8 union variants
   (`PluginTypes.ts:67-80`); the existing public_chat emit
   site keeps working. `onFriendAdd`/`onFriendRemove` stay
   TABLE-ONLY: zero emit call sites exist in TS core or
   plugins (verified by grep) — inventing emits would
   deviate. Core encoder exposure (the 5 clan encoders +
   snapshot encoder) only if the existing `Core` pattern
   admits function fields cheaply; else table-only with the
   reason stated.

## Decisions (coordinator's, grounded above)

- **D1. No core default social-packet handling.** TS core
  never acts on a social packet beyond the hub emit, so Go
  core doesn't either. Production add-friend / PM / filter
  behavior awaits the FriendsList content port; Phase 19
  proves the mechanism end to end with a temporary probe
  content plugin in the live smoke test (removed after).
- **D2. Chat modes + privateMessageId are session-only.**
  No `PlayerSave` fields exist; the `chat.filters` attribute
  is content's `persistAttribute` call, for the content port.
- **D3. Friends-chat channels + clans are content, encoders
  are engine.** See the boundary above.
- **D4. `PlayerRelations.message()` is ported** although no
  live TS caller uses it in the snapshot flow (the plugin's
  `deliverPrivateMessage` supersedes it): it is core API
  content may call. Document the no-caller fact at the site.
- **D5. Single packet struct.** `game.SocialPacket` (new,
  plain data, kind + all variant fields) is the one packet
  shape: dispatch builds it, `plugins.SocialPacketEvent.Packet`
  holds it, the adapter passes it through. Kind values are
  the TS `type` strings. The `plugins` package keeps alias
  constants (e.g. `SocialPacketPublicChat`) so existing code
  keeps compiling.

## Fixed interface contracts (workers code against these)

- **C1 `game.SocialPacket`**: `Kind game.SocialPacketKind`
  (`"public_chat"`, `"clan_chat"`, `"clan_channel_action"`,
  `"friends_chat_action"`, `"private_message"`, `"chat_filter"`,
  `"report_abuse"`, `"chat"`), `Text string`, `AutoChat bool`,
  `MessageType string` (`"friends_chat"` for the chat variant),
  `ClanType int`, `Action string` (clan: kick/ban/mute;
  friends: join/leave/kick/add_friend/remove_friend/
  set_friend_rank/add_ignore/remove_ignore), `Name string`,
  `Value int`, `Rank int`, `Recipient string`,
  `PublicMode, PrivateMode, TradeMode int`, `RuleID int`,
  `Mute bool`.
- **C2 Relations seams**: `(*Player).Relations()` returns the
  non-nil `*PlayerRelations` (lazy allocation if the Player
  constructor path can't guarantee it). Relations sends go
  through a game-defined optional session interface
  (`RelationsSender`, worker 1 defines, worker 2 implements
  in session); world iteration/lookup goes through a
  coordinator-installed seam (worker 1 defines, worker 2
  wires; nil world degrades `updateLists` to self-only
  sends, documented). `GetLongUsername() int64` returns the
  field; entry wiring sets it.
- **C3 Relations save API**: worker 1 provides list/rank/
  channel load + snapshot methods on `PlayerRelations`
  (names: `LoadLists`, `LoadFriendRanks`,
  `SnapshotRelations` or equivalents — worker 1 fixes the
  names and reports them; worker 2 adapts at the seam and
  the coordinator reconciles any drift). TS apply/snapshot
  order and normalization apply.
- **C4 Protocol names**: `DecodeClanChannelAction`,
  `DecodeFriendsChatAction`, `DecodePrivateMessage`,
  `DecodeChatFilter`, `DecodeReportAbuse`;
  `EncodeFriendStatus`, `EncodeFriend`, `EncodeDeleteFriend`,
  `EncodeAddIgnore`, `EncodeDeleteIgnore`,
  `EncodeChatFilterSettings`, `EncodeFriendsChatSnapshot`,
  `EncodeClanSettingsFull`, `EncodeClanChannelFull`,
  `EncodeClanChannelDelta`, `EncodeClanSettingsDelta`,
  `EncodeClanVar`, `EncodeShowClanChatOptions`. Decoded
  message structs mirror the TS decode branches.
- **C5 Dispatch hook**: `listeners.Deps` gains
  `SocialEmit func(p *game.Player, pkt game.SocialPacket) bool`
  (nil = no hooks, TS no-hooks default). The cmd/server
  adapter wires it to `Hub.EmitSocialPacket`.
- **C6 Persist**: `OwnedState` gains relations fields with
  nil-means-preserve semantics; `WithOwned` writes them.

## Out of scope

Skills/XP (milestone 10), sailing/template regions/sounds/
projectiles (milestone 11), content ports beyond the smoke-test
probe (FriendsList/Clans content), the far-WALK bug, the
y=3521 wall, the scrypt check, perf work. No new dependencies.
No scope expansion without TS grounding.

## Verification (coordinator)

`go build ./...`, `go vet ./...`, `gofmt -l .` clean;
`CGO_ENABLED=0 go build ./...` clean;
`go test ./... -p 1 -count=1` (serialized: the
`TestEngineTicksAtRate` flake; no tolerance change) with the
exact passing-case count; `-race` clean; live smoke test, 3
runs stable, real binary + temp probe content plugin, temp
root outside the repo, deadline-free reader, handshake after
LOGIN: add-friend → friend frames + presence, PM delivery,
chat_filter → modes + gate behavior, ignore blocks public
chat, logout/relogin preserves the friends list. Probe
removed after; zero probe references remain. Docs:
`PORT_STATUS.md`, `CONTENT_MIGRATION_PLAN.md` (milestone 9
DONE + boundary), this file's Outcome section.

## Worker split (package-disjoint; docs owned by coordinator)

- Worker 1: `internal/game` only (relations, names, chat
  gate, `SocialPacket`, `LongUsername`, seams).
- Worker 2: `internal/protocol`, `internal/session`,
  `internal/listeners`, `internal/persist`,
  `cmd/server` (wiring only; no game/plugin edits).
- Worker 3: `internal/plugins` only (packet variants, hub
  counts/table, Core encoder assessment).

Workers: if a contract above proves wrong against TS, report
it to the coordinator with the citation instead of
unilaterally changing another worker's interface.

## Outcome

COMPLETED 2026-11-15. Verified: build/vet/gofmt clean incl.
CGO_ENABLED=0, 3,335 test cases passing incl. subtests across
12 packages (0 failures), race clean, live smoke test 3 runs
stable (see PORT_STATUS.md), real-binary boot check clean.
All 8 scope items landed: the whole `PlayerRelations` port,
the live chat gate, all 5 social decoders + 13 social/clan/
snapshot encoders, the session senders, hub-only dispatch
for all 8 social-packet variants, relations as owned save
state, TS-ordered logout presence-offline wiring, and the
hub variant extension (counts unchanged: 41/108 families,
18/95 members, 4/191 Core — the social-packet family gained
kinds, not a new family). Decisions D1–D5 held as written.
Two findings from verification: (1) the legacy presence
sends are no-ops in TS (`PlayerSession.write` is empty) —
Go mirrors that, and live presence awaits content snapshot
sending; (2) TS `Misc.textUnpack` is broken (returns NULs) —
Go ports the intended algorithm, a documented deviation
with no live TS caller. Coordinator reconciliation: plugins
kind constants switched to true `game.X` aliases, and the
legacy-save channel-rank defaults (−1/−1/2) fixed at the
JSON decode boundary. Gaps: friends-chat channels + clan
system (content), snapshot sending, PM sender echo,
`chat.filters` persistence, Core clan-encoder exposure.
Far-WALK bug + y=3521 wall untouched. Next: milestone 10
(skills/XP).
