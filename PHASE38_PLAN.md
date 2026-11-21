# Phase 38 Plan — `::item` admin command

## Decision
Tyler asked 2026-11-21 01:46 PST: "Add item command". This phase ports ONLY
the `::item` command from upstream `AdminCommands.plugin.js`. It is the FIRST
command ever registered in the Go port: the registry and dispatch exist since
Phase 12, but no `RegisterCommand(` call exists anywhere in the repo yet, so
this phase also establishes the registration + boot wiring pattern later
commands follow.

## Scope
Port `api.registerCommand("item", ...)` (`server/plugins/commands/
AdminCommands.plugin.js:1623-1635`) into the Go server:

- `::item id [amount]` — default amount 1; amount capped at
  `Number.MAX_SAFE_INTEGER` (9007199254740991); invalid args print exactly
  `"Usage: ::item id [amount]"`; success prints exactly
  `"Spawned item {id} x{amount}."`; rights gate ADMINISTRATOR.
- Home: new package `internal/content/admincommands/`, porting the
  `AdminCommands` plugin (name + description from the plugin file,
  lines 969-970) and starting with only `::item`. Later admin commands
  (`::give`, `::clearinv`, ...) land in the same package. Registered via
  the established content pattern: package `init()` calls
  `plugins.Register(Plugin)`; one blank import line in
  `cmd/server/pluginwiring.go` compiles it in; `hub.Load()` in
  `newPluginHub` runs its `Register` during plugin Load.
- No change to the registry, the chat listener, or the rights seams —
  all already exist and are already wired (`wireCommandRegistry` in
  `cmd/server/commandwiring.go`, messenger = `player.SendMessage`,
  rights source = the carried save's `Rights.ID`).

## Grounding (parent-verified 2026-11-21)
- Upstream (`AdminCommands.plugin.js:1623-1635`, read in full):
  `parseIntArg(parts[1])` for the id, `parts.length > 2 ?
  parseIntArg(parts[2]) : 1` for the amount; `id === null ||
  amount === null || id < 0 || amount <= 0` → `"Usage: ::item id
  [amount]"`; `cappedAmount = Math.min(amount, Number.MAX_SAFE_INTEGER)`;
  `player.getInventory().adds(id, cappedAmount)`; `"Spawned item ${id}
  x${cappedAmount}."`; registered with `PlayerRights.ADMINISTRATOR`,
  `"Spawn an item"`. Upstream does NOT validate the id against item
  definitions in this command.
- `parseIntArg` (`AdminCommands.plugin.js:172-175`): `Number.parseInt(value,
  10)`, NaN → null. So: leading whitespace skipped, optional sign, leading
  decimal digits only (`"12abc"` → 12, `"0x10"` → 0, `"1.5"` → 1,
  `"1e3"` → 1, `"abc"`/empty → null); no octal, no hex.
- `adds` (`ItemContainer.ts:474-476`): `addItem(new Item(id, amount))` —
  no validation. Unknown ids therefore behave like any other id through
  the container's `add` (stackable per the definition source, one slot
  per item otherwise).
- Go side (verified in-repo):
  - `Api.RegisterCommand(command, handler, minimumRights, description)`
    (`internal/plugins/api.go:347`); hub registry created lazily
    (`internal/plugins/commands.go`, `Hub.Commands()`); registration is
    silently ignored after Load (`Api.canRegister`), so content registers
    during Load only.
  - `game.CommandRegistry.Emit` runs onCommand hooks → base lookup →
    rights gate (denial text `"You do not have permission to use this
    command."`) → handlers in order (`internal/game/commands.go`).
  - Chat listener routes `"::"` text to `hooks.Commands.DispatchCommand`
    before any chat gate (`internal/listeners/chat.go:315-323`); the
    registry is already wired as the dispatcher at boot
    (`cmd/server/server.go:386`).
  - `Player.Inventory()` (`internal/game/player.go:561`) returns the
    28-slot `*Inventory`; `Inventory.Adds(id, amount)`
    (`internal/game/container.go:937`) → `Add(item, true)`, which ports
    `ItemContainer.add` faithfully: id < 0 or (amount <= 0 and not a
    bank) is dropped; stackable ids merge into one slot capped at
    `MaxStackAmount` (0x7fffffff); non-stackable ids fan out one per
    empty slot; a full inventory sends `"You couldn't hold all those
    items."` and refreshes. `stackable(id)` consults the definition
    source (`storeItemDefinitions.IsStackable`, `cmd/server/
    containerswiring.go`); `Store.Item(id)` never returns nil — unknown
    ids get the DEFAULT shape, stackable false. So the Go unknown-id
    behaviour matches TS exactly: the item lands one-per-slot, at most
    28, then the couldn't-hold line, and the success message still
    prints (TS prints it unconditionally after `adds`).
  - `Player.SendMessage` (`internal/game/message.go:102`) drops the
    message with no session — unit tests need a `ChatMessageSender`
    double (the `capturingSession` precedent,
    `internal/listeners/chat_test.go:42-70`).
  - Rights: `game.RightsAdministrator = 2`; the boot rights source reads
    the carried save's `Rights.ID` (`cmd/server/server.go:380-386`); a
    player with no carried save reads as `RightsNone`. Saves are JSON
    (`internal/persist/store.go:154`: `<savesDir>/<username>.json`,
    profile main); `Rights` serializes as `{"id", "spriteId", "yellTag"}`
    (`internal/persist/playersave.go:314-319`, admin = id 2, spriteId
    619). A live admin account is made by logging in once, editing the
    save's `rights` to id 2, and logging in again (rights are read from
    the carried save at login).
  - Incoming chat packet: opcode 190, payload = kind byte 0 (public) +
    NUL-terminated latin1 text (`internal/protocol/decode_listeners.go:
    DecodeChat`); outgoing game messages arrive on opcode 120
    (`ServerChatMessage`), already collected into `s.chats` by the
    smoke drivers.
  - `AdminCommands` has no members gate and no switch in its plugin
    definition (name + description only, lines 969-970), so it loads on
    every world.

## Requirements
1. `internal/content/admincommands/admincommands.go`: package doc (grounding
   + the one deliberate deviation below), `Plugin` value (name
   `"AdminCommands"`, upstream description), `init()` registering it,
   handler calling `api.RegisterCommand("item", handler,
   game.RightsAdministrator, "Spawn an item")`.
2. `parseIntArg`: faithful JS `Number.parseInt(value, 10)` — leading
   JS-whitespace skipped, optional `+`/`-`, leading ASCII digits;
   no digits → invalid. Accumulate into int64, saturating at
   `math.MaxInt64` on overflow (see deviation).
3. Handler, in exact TS order: id = parseIntArg(parts[1]) (missing →
   invalid); amount = len(parts) > 2 ? parseIntArg(parts[2]) : 1;
   id invalid / amount invalid / id < 0 / amount <= 0 → send
   `"Usage: ::item id [amount]"`, return true; else
   `capped = min(amount, 9007199254740991)`;
   `player.Inventory().Adds(id, capped)`; send
   `"Spawned item {id} x{capped}."`, return true. Extra parts beyond
   the amount are ignored, as in TS.
4. Blank import in `cmd/server/pluginwiring.go` (alphabetical position:
   first, before `cooking` — the list is alphabetical).
5. Tests (`admincommands_test.go`):
   - `parseIntArg` table: default/valid forms, `"12abc"` → 12, `"0x10"`
     → 0, `"1.5"` → 1, `"1e3"` → 1, `" 42"` → 42, `"+7"` → 7, `"-5"` →
     -5, `""`/`"abc"`/`"  "` → invalid, huge digit string → saturates.
   - Handler through a real hub (`plugins.NewHub` with only this plugin
     + `Load()`): registration present with floor ADMINISTRATOR and
     description `"Spawn an item"`; admin `Execute("item 882")` →
     `"Spawned item 882 x1."` + 1 bronze arrow in the inventory;
     explicit amount; `item 995 1000` stacks coins into one slot (fake
     definition source: 995 stackable); invalid id/amount/zero/negative
     → usage message and an unchanged inventory; cap case (amount >
     MAX_SAFE_INTEGER) → message names 9007199254740991 and the stack
     holds `MaxStackAmount`; unknown id → message prints, items land
     one-per-slot; full inventory → both `"You couldn't hold all those
     items."` and the spawned message, in that order; non-admin
     `Execute` → denial text, no item; `RegisteredCommands` lists `item`
     for admin, hides it for a non-admin.
   - `Player.SendMessage` needs a `ChatMessageSender` test double;
     `NewPlayer` gives a live inventory; install the fake definition
     source with `Player.SetItemDefinitions` (the `container_test.go`
     precedent).
6. Live verification (fresh temp root `/home/hatch/smoke38/`, real
   rev-242 cache, as in prior phases; do NOT touch `/home/hatch/live36`
   or `~/tsps-full`): small dedicated driver (copy the login/session
   helpers from the smoke36 driver, opcode 190 chat send = kind byte 0
   + NUL-terminated latin1 text, opcode 120 game messages collected):
   seed an admin account (login once, set the save's `rights.id` to 2,
   relogin), `::item 882 5` → `"Spawned item 882 x5."` on the wire and
   5 bronze arrows in the save's inventory; `::item 995 1000` → coins
   stacked; a non-admin `::item 882` → the denial line on the wire and
   no item. Run legs singly (the full-driver pass exceeds the
   background-exec wall clock, per the smoke36 lesson).
7. Docs: PORT_STATUS.md — the CommandPacketListener row ("NO core commands
   exist") updated to `::item` shipped; the P3 content row 9 → 10 of 577
   with the AdminCommands entry; a gap note for the ≥2^63 id divergence
   (below) and for `::give` / `::items` / other admin commands still
   NOT STARTED.

## Deliberate deviation (document in code)
JS `parseInt` returns a float, so `::item 99999999999999999999` gives TS
an id of 1e20 and prints it back as `100000000000000000000`. Go has no
float ids: `parseIntArg` saturates at `math.MaxInt64`, so the same input
prints `9223372036854775807`. The divergence is observable only for ids
≥ 2^63 — no real item id is near that range — and the inventory
behaviour (non-stackable, one per slot) is identical either way. Pinned
by test, not claimed as parity.

## Explicitly out of scope
- `::give`, `::items` (search UI), and every other admin command.
  Only `::item` registers.
- Any change to the registry, the chat listener, the rights seams, the
  container, the save format, the client, or the Android app.
- No new dependencies.

## Environment notes
- Go 1.25.1 at `~/workspace/tools/go/bin/go`. No new dependencies.
- The live36 servers (Go :43594, client :3000) are running for the
  parent's browser test — do NOT touch `/home/hatch/live36` or
  `~/tsps-full`'s running client. Use a fresh temp root for the live
  check, as prior phases did.
- Verification: `go build ./...`, `go vet ./...`, `gofmt -l .`,
  `CGO_ENABLED=0 go build ./...`, full suite `go test ./... -p 1
  -count=1 -v` with exact pass counts, `go test -race ./...`.
