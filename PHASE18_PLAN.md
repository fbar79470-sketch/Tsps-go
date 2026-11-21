# Phase 18 plan (interface runtime — migration plan milestone 8)

Status: IN PROGRESS (dispatched 2026-11-15 under Tyler's
unattended-progression authorization; coordinator + 3 workers).

## Scope (from the brief + migration plan §2 milestone 8)

Migration plan milestone 8, verbatim: "Interface runtime
(interface-action dispatch, open/close, widget state). Unlocks:
`interface/` (58 plugins), bank/shop interface flows, minigame
panels." Grounded in TS at `bfd81e3` (read-only checkout at
`~/tsps-bfd81e3`); every behavior cites a TS file:line.

1. **Interface runtime core.** `Player.interfaceId` (-1 default)
   + `setInterfaceId` emitting `interface:closed` /
   `interface:changed` (`Player.ts:156,1136-1151`);
   `closeInterruptibleInterfaces` on Player (status-gated) and
   PacketSender (`Player.ts:812-822`, `PacketSender.ts:976-1010`);
   the sender-side sub-interface tracking (`subInterfaceTargets`,
   `chatboxGroupId`/`chatboxOwner`, `sendSubInterface` /
   `closeSubInterface`, `sendInterface`, `sendChatboxInterface`,
   `sendTabInterface`, `sendSidebarInterface`,
   `sendInterfaceSet`, `sendWalkableInterface`,
   `sendInterfaceRemoval`, `closeInterface`);
   `InterfaceLayoutRegistry.open` surface dispatch
   (`InterfaceLayoutDefinition.ts:75-127`) + `sendConfiguredInterface`
   (`PacketSender.ts:420-423`) on top of the Phase 6 layouts
   loader (already in `internal/definition/layouts.go`); close
   triggers per `NetworkBuilder.ts:697,1181,1251`. Unify dialogue
   chatbox groups / shop groups 300/301 / trade screens onto the
   runtime only where TS grounds a shared mechanism.
2. **Interface action dispatch.** `InterfaceActionClickOpcode.handle`
   (`InterfaceActionClickOpcode.ts`: dead gate, `refusesWhileBusy` +
   `isLocked` BUSY_MESSAGE gate, teleporting gate, then
   `emitInterfaceActionClick` with the MultiChatboxPrompt check
   first — `PluginManager.ts:2076-2105`); the inbound routing
   branches in `NetworkBuilder.ts:1121,1206,1261,1272`; the
   entered-amount / entered-syntax prompt actions
   (`EnteredAmountAction`, `Player.ts:207,1626-1631`,
   `NetworkBuilder.ts:954-975`, `PacketSender.ts:446-475`,
   runClientScript 108/110) including the shop buy/sell-X path
   (Phase 17 gap) and slot-form prompt picks (Phase 14 gap).
   Hub: `onInterfaceActionClick` + `onInterfaceActionButton`
   (`PluginTypes.ts:1242-1247`, `PluginManager.ts:3120,4379-4400`).
3. **Varp/varbit store — DECISION: IN SCOPE, PacketSender form.**
   The TS live store is the per-player `PacketSender.varps` map
   (`PacketSender.ts:221`) with `getVarp` (default 0), `getVarbit`
   (cache-def packing), `sendConfig` (set + send), `sendVarbit`
   (read-modify-write), and a resend-all loop (`:337`). It is
   the "widget state" half of milestone 8's parenthetical and
   unblocks the Phase 12/17 gaps (examine price line settings
   gate, trade accept-delay varbit 13130). Session-only:
   `PlayerSave.ts` has no varp/varc fields. `VarManager`
   (`vartype/VarManager.ts`) is NOT the live store — transform
   plumbing + varc/array machinery with no core instantiation —
   and stays NOT STARTED.
4. **Bank interface flows — DECISION: NOT IN SCOPE as flows.**
   Bank interface behavior is plugin-driven (BankPin/BankSpace
   plugins; no core bank interface-action listener; `Bank.ts`
   core holds container logic + interface constants + events).
   The runtime unlocks bank flows; the flows themselves are
   content. `endBankSearch` (close of interface 12 ends bank
   search, `PacketSender.ts:902-907`) is core close behavior:
   wire it iff Go's Bank holds search state, else record the gap.
5. **Hub exposure.** Interface hook families + `interface:closed` /
   `interface:changed` custom events; `registerCustomInterface`
   stays NOT STARTED (needs the HTTP ContentApi endpoint;
   transport is WS-only). Update hub table counts.
6. **Save/state.** Nothing persists (varps, interface id, and
   prompt actions are all session-only) — verify against
   `PlayerSave.ts` and document explicitly; no owned fields.

## Out of scope

Clans/social (milestone 9), skill handlers + XP (10),
sailing/template regions/sounds (11), content ports beyond
proof, the far-WALK bug, the y=3521 map wall, embedded
JS/sidecar, scrypt check, MobileList.All perf work.

## Workers (file ownership — no worker edits another's files)

- Worker 1, runtime core: `internal/game/player.go` (struct
  fields only), new `internal/game/interface_state.go` +
  `internal/game/enteredaction.go`, new `internal/session/`
  interface files + `session.go` struct/init edits (adds BOTH
  the `iface` and the `varps` fields), new
  `internal/definition/layouts_open.go`, new
  `cmd/server/interfacewiring.go`. Never touches `server.go`,
  `internal/listeners`, `internal/plugins`, `varps.go`,
  `runstatus.go`, `shopwiring.go`.
- Worker 2, dispatch + prompts + hub: `internal/listeners`,
  `internal/protocol` decoder additions, `internal/plugins`.
  Never touches `server.go`, `internal/game`,
  `internal/session`, `internal/definition`, `cmd/server`.
  Uses Worker 1's `EnteredAmountAction`/`EnteredSyntaxAction`
  contract and `CloseInterruptibleInterfaces` seam; reports
  exact `handleTurnPacket` Deps lines to the coordinator.
- Worker 3, varp store + examine/trade wiring + audit: owns
  `internal/session/varps.go` (expand freely, keep the type
  name + the `&varpStore{values: ...}` construction),
  `internal/session/runstatus.go`,
  `cmd/server/shopwiring.go` (GetVarbit wiring only),
  `internal/session/bootstrap.go` / `music.go` only if
  unifying varp sends wire-identically. Never touches
  `session.go`, `server.go`, `internal/game`,
  `internal/listeners`, `internal/plugins`.

Coordinator integrates: all `server.go` / `main.go` edits,
`PORT_STATUS.md`, `CONTENT_MIGRATION_PLAN.md`, this plan's
outcome, final verification + live smoke test.

## Verification (coordinator, before reporting)

`go build ./...`, `go vet ./...`, `gofmt -l .` (clean),
`CGO_ENABLED=0 go build ./...`, `go test ./... -p 1 -count=1`,
`go test ./... -p 1 -count=1 -race`, live smoke test (open via
hub hook + widget click dispatches + close clears state +
entered-amount round-trip + varp frame via the store), 3 runs
stable, temp root outside the repo, cleaned up.

## Outcome

COMPLETED 2026-11-15. Verified: build/vet/gofmt clean incl.
CGO_ENABLED=0, 3,180 test cases passing incl. subtests across
12 packages (0 failures, 0 skips), race clean, live smoke
test 3 runs stable (see PORT_STATUS.md). The interface
runtime core (interface id + close gate, mount/sub-interface/
chatbox/tab layering, layout open dispatch, close on
movement/logout/death), the interface-action dispatch (gates,
prompt-first hub emit, shop branch untouched), the entered-
amount/syntax prompt listeners, the live varp/varbit store
(PacketSender form, with the varbit resolver + audio-settings
replay wiring), and the interface hub families (41/108
firable, 18/95 members) all landed. The phase 12/17 gaps the
store unblocks are closed: the examine price line is live,
the trade accept-delay triggers, and buy/sell-X prompts
round-trip. Two findings from verification: (1) the smoke
test's first run stalled at 2 packets because the driver
skipped the post-login HANDSHAKE that enters the world —
a driver gap, not a port bug; (2) the boot NOTE in main.go
still said "no general interface runtime yet" and was
updated. `VarManager`, `registerCustomInterface`, bank
interface flows, and the per-player vars seam stay NOT
STARTED/gaps with grounding (see PORT_STATUS.md); the
far-WALK bug and the y=3521 map wall stay untouched.
