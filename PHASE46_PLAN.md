# Phase 46 — Bank (player-facing banking)

Tyler: "Add bank" (2026-11-21 20:51 PST), then "Pause magic add bank" (20:52 PST) — bank jumps the queue ahead of Magic (Phase 45, paused mid-port; its files stay untouched).

## 1. Scope

**In scope — the bank interaction layer.** Bank *storage* already exists (`internal/game/bank.go`, Phase 9: tab containers, `Rearrange`, placeholder support, save format `banks` Map). What is missing is everything the player touches:

1. **Open**: Bank booth (`Bank`/`Use`), Bank chest (`Use`, chest 29321 `Bank`), Banker NPC (`Bank` option), debounce, BANKING status, interface mount sequence, close.
2. **Deposit**: side-inventory item ops (Deposit-1/5/10/X/All), Deposit inventory (button 47 / 50004 / opcode 243), Deposit worn (49 / 50007 / 244), worn-slot Bank op, deposit drag (native IF_BUTTOND + opcode 245 bank_move), un-noting on the way in, full-bank message.
3. **Withdraw**: bank-grid item ops (Withdraw-1/5/10/X/All/All-but-1), note mode + "cannot be withdrawn as a note" message, placeholders (left on full withdraw, release, release-all, toggle), full/partial-inventory messages, max-stack clamp.
4. **Quantity mode + X prompt**: modes 1/5/10/X/All (varbit 6590), remembered custom quantity (varbit 3960), "Enter amount:" prompt.
5. **Tabs**: view/select, collapse, new-tab message, tab-count varbits (4171–4179), move-to-tab via drag, reorder within tab (50300 containers, already routed in switchslot), tab-bar collapse/release-placeholder options.
6. **Search**: legacy search button 50010 (server prompt + search-tab fill), exitSearch, closeSearch, client search child 42 ack.
7. **Settings interface 32500**: show menu (50013), close (32503), re-open bank (32512), placeholder toggle (32513 + message).
8. **Buttons**: legacy `handleButton` route via a new firable `onButtonClick` emit + tab-select buttons (50070 + 4*tab).
9. **Capacity**: default `MAX_SLOTS` (1410) — the no-plugin default in TS itself (`Bank.getCapacity` returns MAX_SLOTS when no plugin answers `bank:capacity`).
10. **Sounds**: CONTAINER_OPEN on open, PICK_UP_ITEM on withdraw, DROP_ITEM on deposit, CONTAINER_CLOSE on close.
11. **Banker dialogue action**: `npc-dialogue:action` handler (`action == "open_interface" && target == "Bank"`) ported and unit-tested; its live leg defers to the NPC-dialogs phase (see §6).

**Out of scope (deferred, documented as gaps):**
- Bank PIN (`emitCanBank` gate, `BankPin.plugin.js` + `bank-pin.json`) — the bank opens without a PIN by default.
- BankSpace capacity system (F2P 400 / members 900 + 50/block + 20 PIN + 40 Jagex, buy-slots dialogue, capacity tooltip) — needs `bank-space.json` import decision (§5).
- BankDepositBooth (deposit-box interface 192) — separate deposit-only piece.
- Platinum-token exchange, rune-pouch-note exchange, banknote un-noting on banker — extras in `BankBooths.plugin.js`.
- Incinerator (drag target only; `bank:incinerate` has no core consumer).
- Configure-Charges op 9 (AshSanctifier / VenatorBow item plugins).
- "Collect" option (Grand Exchange collection box — GE not ported).
- Tab drag-reorder (tab bar drag) — include only if it fits; else defer with a gap note.
- `emitCanBank` / `emitCanBankItem` gates — no plugin answers them in Go; open/deposit proceed un-gated (documented gap).

## 2. Survey (grounded)

### 2a. Upstream: two parallel button systems (do not conflate)

`Bank.ts` (1,567 lines, SHA-256 `2891222ec3da444fcdecf7d5ad36a66fef47a6432d75a5b10dbe2dca42d77400`):

- **Cache-native widget route** — `Bank.handleWidgetAction` (`Bank.ts:877-1032`, dispatched from `NetworkBuilder.ts:1022`), packet `{groupId, childId, buttonNum, option?, slot?, itemId?}`:
  - Bank item grid (group 12, child 12): slots 1419–1427 are tab headings → `viewTab`; op 11 = Examine; amount-0 (placeholder): op 8 / option "release" → `releasePlaceholder`; else emit `bank:item-op`, then amount-prompt or `Bank.withdraw`.
  - Side worn view (group 15, child 4): slot = inventory slot; op 1 equip, op 9 deposit stack, op 10 examine.
  - Side inventory (group 15, child 3 or ≥ 0x8000): emit `bank:inventory-op`; op 9 equip; else amount-prompt or `Bank.deposits`.
  - Tab bar (group 12, child 10): tab = slot − 10; op 6 / "collapse" → `collapseTab`; op 7 / "placeholder" → `releasePlaceholders`; else `viewTab` + `closeSearch`.
  - Search (group 12, child 42): switch to main tab if needed; clears pending entered-amount action.
  - Worn slots (group 12, child 68–78): op 2 deposit just that worn item, op 10 examine.
  - Frame/controls: child 2 → `sendInterfaceRemoval()`; 23 swap/insert toggle (varbit 3959); 25 note toggle (varbit 3958); quantity child 35 + button 2 → `promptAmount` only; any of children [29,31,33,35,37] → `setBankQuantityMode(index)` (varbit 6590); 40 placeholder toggle (varbit 3755); 45 emit `bank:empty-containers` (plugin-owned, no core behaviour); 47 `depositItems(inventory)`; 49 `depositItems(equipment)`.
  - Incinerator child 54 is a drag target only, not a button.
- **Legacy button route** — `Bank.handleButton` (`Bank.ts:1098-1196`), reached only via `BankBooths.plugin.js` `onButtonClick` (buttonId, action 0) / `onInterfaceActionClick` (buttonId, action). Returns true whenever the bank is open.
  - Settings interface 32500: 32503 close, 32512 re-open bank, 32513 placeholder toggle (`sendConfig(118, …)` + message).
  - Tab-select: button `50070 + 4*tab`, tab 0–10; action 1 = collapse, else select (empty non-zero tab → `NEW_TAB_MESSAGE` = "To create a new tab, drag items from your bank onto this tab.").
  - 50013 show menu (close + `sendInterface(32500)`); 5386 note on; 5387 note off; 8130 swap; 8131 insert; 50004 deposit inventory; 50007 deposit worn; 5384/50001 close; 50010 search (or exitSearch if searching).

`Bank.open()` (`Bank.ts:1354-1414`): gate `emitCanBank(player, "open")` (PIN hook; no plugin → allow); `opening = !isOpen`; if opening, note-withdrawal forced off; `setStatus(BANKING)`; `setEnteredSyntaxAction(null)`; `sortItems()`; `setInterfaceId(12)`; sends in order: script 917 `[-1,-2]`; sub-interface (161:16) main 12 type 0 with varps {548:1, 262:sideWearOpsMask, 4611:MAX_SLOTS−capacity} and varbits {3755 placeholders, 3958 note, 3959 insert, 3960 custom qty, 4150 current tab, 4170:0, 5450:1, 6590 qty mode, 4171–4179 tab sizes}; sub-interface (161:74) side 15 type 3 same varps/varbits; flags ranges: (12:12) slots 0–1409 flags `0x7fe|(2<<17)|(1<<20)|(1<<21)` ops `0x7ff`; (12:12) drop slots 1428–1437 flag `1<<20`; (12:10) tab bar slots 10–19 flags `0x7fe|(1<<17)|(1<<20)`; (15:3) slots 0–27 flags 3278846; (15:4) worn slots 0–27 flags `(1<<1)|(1<<9)|(1<<10)|(1<<17)|(1<<20)`; varbit 12393 = 1; string "Bank of \<world name\>" at (12:3); view-container reset if opening (varbit 9727 → 0, varp 4385 → −1); `refreshItems()`; if opening: sound CONTAINER_OPEN + emit `bank:opened`. No `Bank.close()` method — closing is `sendInterfaceRemoval()` (`resetInterfaceState`: varp 548 → 0, varbit 12393 → 0, status NONE, interface −1, searching false, abort prompt, dialogue reset, `endBankSearch` → `closeSearch`).

`Bank.refreshItems()` (`Bank.ts:1417-1433`): `reconfigureTabs` first (may re-open a tab and return early); varbits 4171–4179 (tab sizes; deliberately NOT varbit 4150); `sendBankSnapshot()` (layout tabs 1–9 then 0, display ids, `PacketSender.ts:858-870`); `sendItemContainer(inventory, 5064)`; emit `bank:refreshed`.

`Bank.withdraw` (`Bank.ts:290-375`) / `deposits`/`deposit` (`Bank.ts:376-425`) / `switchsItem` (`Bank.ts:1438-1558`): see worker-2 report for the full amount math. Op mapping (`actionAmount`, `Bank.ts:792-821`): withdraw — op 1 default (qty mode), 2 → 1, 3 → 5, 4 → 10, 5 → custom, 6 → X (prompt), 7 → All, 8 → All-but-1, 9/10 → 0; deposit — op 1 → 1, 2 → default, 3 → 1, 4 → 5, 5 → 10, 6 → custom, 7 → X (prompt), 8 → All. Option suffixes override: `-1/-5/-10/-x/-all` (withdraw also `-all-but-1`). Deposit un-notes on the way in (`ItemContainer.switchItem`); full bank → `bank:full` ("Not enough space in bank."); withdraw full inventory → "You don't have enough inventory space." (spam), partial → "You don't have enough inventory space to withdraw that many." (spam), max-stack → "You cannot withdraw that entire amount into your inventory."

Tabs (`Bank.ts:554-604, 1248-1284`): `getTabCount` counts consecutive non-empty custom tabs from 1; `reconfigureTabs` compacts (runs at every refresh); `collapseTab` (main tab must have room); `releasePlaceholders` (tab 0 = all tabs); `viewTab` (tab > count → NEW_TAB_MESSAGE, else set tab + varbit 4150 + refresh). Search (`Bank.ts:453-516`): `search` fills tab 10 with clones of name-matching items, sets current tab 0, opens tab 10; `exitSearch` clears and re-opens previous tab; `closeSearch` sends client script 101 mode 11.

`BankBooths.plugin.js`: registers `onObjectInteraction("Bank booth", {Bank, Use})`, `("Bank chest"/"Bank Chest", {Use})`, `onNpcInteraction("Banker", {Talk-to → dialogue, Bank})`, `npc-dialogue:action` → `open_interface`/`Bank`, `onButtonClick` → `handleBankButton`, `onInterfaceActionClick` → `handleBankInterfaceAction`. 1-tick open debounce. Bots get status + interface id only. Button sets: main {50013, 5386, 5387, 8130, 8131, 50004, 50007, 5384, 50001, 50010}, settings {32503, 32512, 32513}, tab-select start 50070 (tab = (buttonId−50070)/4, only when % 4 == 0).

Capacity (`BankSpace.plugin.js` + `data/bank-space.json`): base 400 F2P / 900 members, +50/block × 9, +20 PIN, +40 Jagex = 1410 max; `LOCKED_SLOTS_VARP` 4611 = MAX_SLOTS − capacity.

### 2b. Go side — what exists, what is missing

**Exists and reusable:** full IF_BUTTON-family + widget-target + local-trigger decode/dispatch (`internal/protocol/decode_listeners.go`, `internal/listeners/interfaceaction.go`); firable `onInterfaceActionClick`/`onInterfaceActionButton` (`internal/plugins/interfaces.go`); `SendInterface`/`MountSubInterface`/`CloseSubInterface`/`SendInterfaceRemoval` (`internal/session/interface_sender.go`, `dialogue_sender.go`); varp/varbit sends with recording store (`internal/session/runstatus.go`); bank tab containers + persistence (`banks` Map in saves); `Rearrange` with live insert mode; adapter `sendBankRefresh` (`cmd/server/containerswiring.go:392-408`: tab-count varbits + bank snapshot opcode 52 + inventory snapshot); `BankMainInterfaceID = 12`, `BankInventoryInterfaceID = 5064` constants; `PlayerStatusBanking` constant (not producible); attribute keys for note/insert/quantity/custom-qty/placeholders (`internal/game/bank.go`).

**Gaps (all must be closed by this phase except where noted):**
1. Opcodes 243 (`ClientBankDepositInventory`), 244 (`ClientBankDepositEquipment`), 245 (`ClientBankMove`) are **decode-drops** — in the opcode table only, no decoder, no dispatch case (`listeners.go:1271` default). TS decodes them in `ClientProtocol.ts:934-936` and handles them in `NetworkBuilder.ts:860-878`.
2. `onButton`/`onButtonClick` **table-only** — no Api method, no Hub emit; the IF_BUTTON (13) simple-click fallback is a documented no-op (`internal/listeners/interfaceaction.go:44-49`, `internal/plugins/hooks.go:262-267`).
3. No bank open sequence — no mount of main 12 / side 15, no varps/varbits bundle, no script 917, no flags ranges, no `sendSideOps` (varp 262 mask).
4. No deposit/withdraw/note/quantity/search/tab-view flows.
5. No BANKING status — no status field; `SetStatusNone` dep nil.
6. No bank UI state — no current-bank-tab, no searching flag/syntax, no `isOpen`; `EndBankSearch` dep nil; `Bank.handleDrag` always false.
7. No options-form sub-interface mounts — `MountSubInterface` never passes varps/varbits/hidden/scripts (TS `sendSubInterface` options).
8. No `releaseInterfaceInventories` (interface-close paths skip inventory stop-transmit).

**Open paths (Go hubs, all firable):** `Api.OnObjectInteractionNamed` (`internal/plugins/objects.go:486`, exact case-sensitive name match) — register `("Bank booth", {Bank, Use})`, `("Bank chest", {Use})`, `("Bank Chest", {Use})`. `Api.OnNpcInteraction` (`internal/plugins/npcinteraction.go:294`) — register `("Banker", {Talk-to, Bank})`. The upstream "bank" fallbacks do NOT exist in Go (`internal/listeners/objectaction.go:409-415`, `internal/listeners/npc.go:769-775` — silent gaps today); everything goes through plugin hooks. `onItemOnNpc` is table-only (currency/unnote/rune-pouch legs cannot port). `npc-dialogue:start` and `npc-dialogue:action` (`*game.NpcDialogueActionEvent`, `Action`/`Target`/`StepID`/`Handled`) exist and are firable via `Api.OnCustomEvent`.

**Real world data (probed from the real rev-242 cache):**
- Bank booth 10355, actions `["", "Bank", "Collect", "", ""]` (Bank = clickType 2): Lumbridge (3091,3242), (3091,3243), (3091,3245); Edgeville (3095,3489), (3095,3491), (3096,3493), (3098,3493).
- Bank booth 10583, same actions: Varrock East (3252–3256,3419); Varrock West (3186,3438), (3186,3440), (3186,3442), (3186,3444).
- Bank chest 29321, actions `["Bank", "", "Collect", "", ""]` (Bank = clickType 1); most chests `["Use", "", "Collect", …]`; Bank Chest 30796 `["Use", …]`.
- Banker NPCs: 101 defs named "Banker", almost all actions `["Talk-to", "", "Bank", "Collect", ""]` (Talk-to = 1, Bank = 3). Lumbridge: 1613 at (3088,3242), 1618 at (3090,3242/3243), 1613 at (3090,3245). Varrock East: 2897/2898 at (3252–3256,3418). Varrock West: 2898/2897 at (3187,3438/3440/3442/3444). Avoid 8666 at (3187,3446) (no actions).
- Smoke-test setups (plane 0): Lumbridge spawn (3093,3243,0) → booth 10355 at (3091,3243) Bank (clickType 2); banker 1618 at (3090,3243) Bank (clickType 3). Varrock East spawn (3254,3421,0) → booth 10583 at (3254,3419); banker 2898 at (3254,3418). Varrock West spawn (3184,3440,0) → booth 10583 at (3186,3440); banker 2897 at (3187,3440).

## 3. Worker split (file ownership is disjoint — do not cross it)

- **Worker A — engine seams.** Owns: `internal/plugins/` (new firable `onButtonClick`/`onButton`: `Api.OnButtonClick`, `Hub.EmitButtonClick`, event struct `{Player, ButtonID, Handled}`; the listener-side IF_BUTTON-13 simple-click fallback in `internal/listeners/interfaceaction.go` + `listeners.go`), `internal/protocol/` + `internal/listeners/` (decoders + dispatch for opcodes 243/244/245, following the existing `DecodeWidgetAction` shape; 245 resolves display slots and calls the bank move path), `internal/game/` player bank UI state (BANKING status field + `Status()` derivation; current-bank-tab, searching flag + syntax, custom quantity, quantity mode, note mode — read live from the existing attribute keys where they exist), `internal/session/` (options-form sub-interface mount: varps/varbits on `MountSubInterface`; `sendSideOps` varp-262 mask helper if it fits — else flag it). Unit tests in the packages touched. Must NOT touch `internal/content/`, `cmd/server/`, or any Magic file.
- **Worker B — bank core flows.** Owns: `internal/game/` bank flow functions (open/refresh/withdraw/deposit/deposits/switchsItem/handleButton/handleBankWidget/actionAmount/asksAmount/promptAmount/viewTab/collapseTab/releasePlaceholders/search/exitSearch/closeSearch/reconfigureTabs/layout/resolveDisplaySlot/moveToTab/moveItem/rearrange/handleDrag — port the TS semantics from §2a against the seams Worker A defines; if a seam is not yet landed, code against the contract in §4 and flag it) + `cmd/server/` adapter additions (open-sequence sends: script 917, flags ranges, side mount, "Bank of \<world\>" string; reuse `sendBankRefresh`). Must NOT touch `internal/plugins/`, `internal/protocol/`, `internal/listeners/`, `internal/content/`, or any Magic file.
- **Worker C — content port (BankBooths).** Owns: `internal/content/bank/` (new package: open paths, bankerDialogueAction handler, button routing, interface-action routing delegating to Worker B's flows, settings interface, debounce) + `cmd/server/bankwiring.go` + the blank import in `cmd/server/pluginwiring.go` (first-alphabetical) + `pluginwiring_test.go` load-order expectations. Must NOT touch `internal/game/`, `internal/plugins/`, `internal/protocol/`, `internal/listeners/`, or any Magic file.

**Magic files — DO NOT TOUCH, EVER:** `PHASE45_PLAN.md`, `cmd/server/spellwiring.go`, `internal/listeners/spellhooks.go`, `internal/listeners/useitem.go`, `internal/plugins/hooks.go`, `internal/plugins/spells.go`. (Worker A's `internal/plugins/` work must not edit `hooks.go` or `spells.go` — add the button-click emit in `interfaces.go` or a new file instead. Verify with `git status` before finishing: those six paths must be byte-identical.)

**Shared contracts (§4) — if a contract is wrong, say so in your report; do not silently redefine it.**

## 4. Shared contracts

- **Bank-open state**: `game.Player` gains a bank UI state block: current tab (default 0), searching bool + syntax string, and a BANKING status producible via a real status field (Worker A). `BankIsOpen(p)` = status == BANKING && interface id == 12 (`Bank.ts:285-288`).
- **Button-click emit**: `Hub.EmitButtonClick(ev *ButtonClickEvent) bool` runs `onButtonClick` hooks then `onButton` hooks (TS order, `PluginManager.ts:3083-3110`); `ButtonClickEvent{Player, ButtonID, Handled}`. The listener fires it for simple IF_BUTTON (opcode 13) clicks not claimed by the interface-action hub (TS `NetworkBuilder.ts:1133-1137`).
- **Bank opcodes**: 243 deposit-inventory → `Bank.depositItems(inventory)` if open; 244 deposit-equipment → same for equipment; 245 bank-move `{from, to, mode (1=insert else swap), tab?}` → resolve display slots, same-tab check, `setInsertMode(mode == insert)`, `Bank.rearrange` (TS `NetworkBuilder.ts:860-878`). All three require the bank open.
- **Interface-action routing**: the bank content package registers `onInterfaceActionClick` and routes by (group, child): (12,12) bank-grid ops, (15,3)/(15,≥0x8000) side-inventory ops, (15,4) side-worn ops, (12,10) tab bar, (12,42) search, (12,2)/(12,23)/(12,25)/(12,29,31,33,35,37)/(12,40)/(12,45)/(12,47)/(12,49)/(12,68–78) frame controls. The `onInterfaceActionButton` form may be used for the legacy button ids instead — pick one, document it.
- **Open sequence**: `Bank.open` sends script 917, mounts 12 + 15 with the varp/varbit bundle (§2a), flags ranges, varbit 12393 = 1, "Bank of \<world\>" string, view-container reset when opening, `sendBankRefresh`, CONTAINER_OPEN sound, `bank:opened` custom event. Close: `sendInterfaceRemoval()` resets varp 548 → 0, varbit 12393 → 0, status NONE, interface −1, searching false (existing `interface_sender.go:558-559` behaviour — extend it, don't duplicate it).
- **Amounts**: implement `actionAmount`/`asksAmount` exactly per §2a (withdraw ops 1–8, deposit ops 1–8, option suffixes `-1/-5/-10/-x/-all` (+ `-all-but-1` withdraw)). X-prompt via the existing entered-amount prompt seam (check `internal/listeners/prompts.go`); the answer is stored as the remembered custom quantity (varbit 3960) and the op re-runs.
- **Deposit un-notes**: noted items deposit as their un-noted id (`ItemContainer.switchItem` semantics); a 0-fit deposit says "You cannot deposit that entire amount into your bank."
- **Placeholders**: withdraw-to-inventory leaves a placeholder when placeholders are on and the item has a cache placeholder id; full-stack withdraw of a placeholder-less item sends the spam "This item cannot leave a placeholder."; release via op 8 / "release" option on amount-0 stacks; toggle via child 40 / button 32513.

## 5. Data-file decision (for the parent — do NOT import)

| File | Size / SHA-256 | Verdict |
|---|---|---|
| `server/plugins/interface/data/bank-space.json` | 3,353 bytes / `614910dfd299271bb29991bdedfbe7b7652fa1e4335ae4ede587dcb7cfcc8ddf` | **Flag for Tyler.** Needed only for real capacity (400 F2P / 900 members, +50/block × 9 costs, PIN +20, Jagex +40) and the buy-slots dialogue. Core slice uses the TS no-plugin default 1410. |
| `server/data/definitions/npc-dialogues.json` | ~45 MB (3,374 speaker keys) | **Flag for Tyler — already queued.** Needed for banker Talk-to → bank (and the whole NPC-dialogs phase after Bank). The `npc-dialogue:action` handler is ported in this phase; its live leg waits for the import. |
| *(none for bank core)* | — | `Bank.ts` uses no data file; ids/varps/messages are constants; placeholder identity comes from the cache. |

## 6. Notes / decisions

- **Talk-to → bank**: the banker transcript is absent from Go (`data/definitions/npc-dialogues.json` etc. are 3-byte `{}` stubs; NPC dialogs are queued after Bank). Phase 46 ships booth + Banker `Bank`-option live; Talk-to → bank is handler-only (unit-tested) until the dialogues import lands.
- **BankDepositBooth** (interface 192) is a separate deposit-only piece — out of scope.
- **"Collect"** (Grand Exchange) is out of scope.
- **Save format**: banks already persist as the `banks` Map (tabs 0–10). No save-format change in this phase; current-tab/search/preferences are attributes (insert-mode + placeholders already read live; note/quantity/custom-qty gain live consumers in this phase).
- **XP**: none — banking grants no XP. The cross-skill XP-truncation gap does not apply.

## 7. Verification (coordinator)

- `go build ./...`, `go vet ./...`, `gofmt -l .`, `CGO_ENABLED=0 go build ./...` clean.
- Full suite `-p 1 -count=1 -v`: exact pass/fail/skip counts + package count.
- `go test -race ./...`: clean; known flake protocol (TestEngineTicksAtRate in internal/game; §20 cmd/server savewiring timeouts under race load — re-run the package once, say so plainly). ANY DATA RACE is reported exactly.
- Live in fresh `/home/hatch/smoke46/`: open at Lumbridge booth (3091,3243) + Varrock East booth (3254,3419) + banker Bank option (1618 at 3090,3243; 2898 at 3254,3418); deposit 1/5/all; withdraw 1/5/all; bank persists across relogin; full-inventory withdraw refusal; walk-away/close. Legs may run singly (RUNS override). Harness traps: deadline-free reader goroutine; ClientHandshake after LOGIN; per-mode npc-spawns.json in the temp root; ~2 min for eager NPC spawn fill.
- Docs: PORT_STATUS.md content row (count the entries — bank is the next content entry after Crafting's 15 of 577), new residual-gaps section, "What was verified" entry. PHASE46_PLAN.md Results section.

## 8. Results

Phase 46 complete (coordinator-verified, 2026-11-22). Player-facing
banking works end to end against the real server + real rev-242 cache.

- Survey (§2) grounded the upstream open-bank path with file:line
  citations (Bank.ts, BankPin/BankSpace plugins, BankBooths list);
  the two parallel button systems, the interface-12 open sequence,
  and the 243/244/245 decode-drops are all in §2. Data files
  flagged for Tyler, NOT imported (§5): `bank-space.json`
  (3,353 bytes, SHA-256 `614910df…`) and `npc-dialogues.json`
  (~45 MB, banker Talk-to transcript).
- Port (workers A/B/C, disjoint files): engine seams (firable
  `onButtonClick` + `onButton` via `Hub.EmitButtonClick`,
  decoders for 243/244/245, BANKING status, bank UI state,
  options-form mount); bank core flows (`internal/game/bankflows.go`,
  open/refresh/deposit/withdraw/buttons/tabs/search/drag) +
  `cmd/server` adapter; content port (`internal/content/bank/`,
  booth + banker bindings). Full suite 5,048 tests (3,845
  top-level + 1,203 subtests), 0 fail, 0 skip, 28 packages;
  race clean (first full run hit only the known §20 cmd/server
  savewiring timing flake, no DATA RACE; package re-run clean).
- Live (harness `/home/hatch/smoke46/`, 7 legs, 44 of 44 checks):
  open-booth 7/7, deposit 10/10, withdraw 10/10, persist 3/3,
  full 5/5, close 2/2, open-banker 7/7. Bank contents persist
  across relogin; full-inventory withdraw refused with the
  TS message. Tabs/search/drag unit-covered only.
- Live-found engine fixes (both TS-grounded): (1)
  `canInteractWithUnreachableNpc` (MovementQueue.ts:1923-1928)
  ported in `internal/game/movement_queue.go` — counter NPCs
  interact from the route end within radius 2 with a clear
  projectile ray (4 unit tests in
  `internal/game/movement_counter_test.go`); (2) the NPC click
  path (`internal/listeners/npc.go`) switched from `WalkToTile`
  + `ReachedObj` to the queue's `WalkToEntity` at range 1
  (NPCOptionPacketListener.ts:78-93) — the old shape could never
  arrive at a counter NPC, so fix (1) alone left the banker leg
  failing. Three listener tests needed fixture updates for the
  switch (registered + hitpoints NPCs, as the spawn path makes
  them; the transformed-NPC test now walks 4 tiles so the
  transform lands mid-walk). The frozen `isOnEntityRouteEnd`
  dry run (MovementQueue.ts:1542) is deliberately still
  degenerate. The temporary real-cache probe test was deleted
  after use.
- Hub: 63 of 108 families firable (67 `Api.On*` methods) — Bank
  adds `onButtonClick` + `onButton`; the paused Magic work in
  the same tree adds `onSpellCast` + `onSpellOnObject` (wired,
  not live-verified). Content: 16 of 577 entries.
- Residual gaps: gap 28 in PORT_STATUS.md (Bank PIN, BankSpace
  capacity, DepositBooth, token/pouch/Collect exchanges,
  op 9, save hints, frozen dry run, Talk-to transcript,
  tabs/search/drag live-observation open).
- Magic files untouched (mtimes 04:48–04:52, verified at
  completion). Nothing committed (parent handles git).
