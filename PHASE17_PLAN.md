# Phase 17 plan (COMPLETED — shops/trading, migration milestone 7)

Scope: migration plan §2 milestone 7, "Shops + trading/duelling +
Grand Exchange" — the CORE systems content needs, grounded in TS at
`bfd81e3`. Dispatched under Tyler's unattended-progression
authorization (2026-11-14, overnight).

## Boundary decisions (grounded before dispatch)

- ShopManager (`game/model/container/shop/ShopManager.ts`, 1339
  lines) and Trading (`game/content/Trading.ts`, 656 lines) are
  core: PORT.
- Duelling (`game/content/Duelling.ts`, 57 lines) is shared state
  only — its own header says "DuelArena.plugin.js owns the
  gameplay": port the `Dueling` state class + `DuelRule`/`DuelState`
  surface; Duel Arena gameplay + staking are plugin content, NOT
  STARTED.
- Grand Exchange (`plugins/interface/GrandExchange.plugin.js`) is
  plugin content needing the interface runtime (milestone 8): the
  marketplace is NOT STARTED. The definition GE-value accessors
  (`hasGrandExchangeValue`/`getGrandExchangeValue`) ARE ported if
  shop pricing / the examine price line need them.
- `shop:*` custom events (10 found, not 7: `shop:opened`,
  `shop:stock-visible`, `shop:buy-limit`, `shop:delivery`,
  `shop:purchase`, `shop:keep-stock`, `shop:sell-price`,
  `shop:buy-price`, `shop:buys-back`, `shop:can-sell`) fire
  through the hub's
  custom-event mechanism at the same sites TS emits them.
- Hub families new this phase: `onCanTrade`, `onTradeRequest`,
  `onTradeCompleted`, `onCanShop`, `onCanStockItem`. Non-hook
  members: `registerShopCurrency`, `registerItemShopCurrency`, plus
  the scoped ShopManager/ShopDefinition/TradeState core surface
  (following the `getObjectManager` precedent).

## Scope

1. ShopManager: definitions + loading (TS load order, validation),
   stock/restock/destock timing, player-sold stock, pricing
   (buy/sell rules, value/GE accessors), currencies, access rules.
   Examine price line (`ExaminePrice.ts`) wired if its accessors
   land here. Shop stock persistence determined from TS, not
   assumed.
2. Shop interface flows: NPC Trade-option dispatch to shop opening
   (`core.shops.open` / `registerNpcInteraction` shopId actions),
   dialogue shop steps (Phase 14 `FindShops`/`OpenShop` seams),
   shop widget + container-action handling, buy/sell transactions
   with TS-exact refusal text, close behavior. ONLY the
   interface-sending surface shops need; the general interface
   runtime stays milestone 8.
3. Player-to-player trading: request/acceptance, offer flow,
   confirmation stages, accept/decline, transfer on completion,
   trade widget frames, trade-related mobility/combat gating
   (Phase 7 gaps), trade escrow in saves (ground in
   `PlayerSave.ts`).
4. Hub exposure: the 5 families above + currency/shop members,
   with production emit sites. `game` never imports `plugins`;
   seams follow the phase 16 combat-hooks pattern.
5. Save/state: shop/trade persistence per TS grounding only.

## Out of scope

Interface runtime beyond shop/trade needs, clans/social,
skills/XP, sailing/template regions/sounds, content ports beyond
proof, the far-WALK bug, the y=3521 map wall, scrypt check,
MobileList.All perf work, Duel Arena / GE marketplace content.

## Worker split (file ownership — no overlaps)

- Worker A (shop core): NEW `internal/game/shop*.go`,
  `internal/protocol/shop.go`, `internal/definition/*` (only if the
  existing shop loader needs extension), `internal/game/examine_price.go`,
  `internal/listeners/examine.go`. Reports ShopManager API + boot
  wiring needs.
- Worker B (trading core): NEW `internal/game/trading.go`,
  `internal/game/duelling.go`, `internal/protocol/trade.go`,
  `internal/listeners/player.go` (option 2) + NEW
  `internal/listeners/trade*.go`, trade gates in
  `internal/game/movement_queue.go` / combat files,
  `internal/persist/*` (escrow only). Reports `listeners.go`
  dispatch-insertion snippets.
- Worker C (flows + hub): `internal/listeners/npc.go` (Trade
  dispatch), NEW `internal/game/shop_dialogue.go` (dialogue seams
  via a local `ShopOpener` interface — coordinator binds it to
  Worker A's ShopManager), NEW `internal/listeners/shopwidget.go`
  (shop widget/container-action handlers), ALL of
  `internal/plugins/*` (hub families + members + table updates).
  Reports wiring snippets.
- Coordinator owns: `internal/listeners/listeners.go` (dispatch +
  table), `internal/listeners/doc.go`, `cmd/server/*` wiring,
  `PORT_STATUS.md`, `CONTENT_MIGRATION_PLAN.md`, this file, final
  verification + live smoke test. Workers do NOT edit those files.

## Verification

`go build ./...`, `go vet ./...`, `gofmt -l .` clean,
`go test ./... -p 1 -count=1` + `-race`, `CGO_ENABLED=0` build,
live smoke test: shop open via NPC Trade + buy + sell + two-player
trade completion + decline safety (temp data root outside repo,
`--no-cache`, deadline-free reader, staged clicks / south-of-wall
spawns as needed).

## Outcome

COMPLETED 2026-11-15, verified: build/vet/gofmt clean incl.
CGO_ENABLED=0, 3,027 test cases pass incl. subtests across 12
packages (0 failures, 0 skips), race clean, live smoke test 3
runs stable (see PORT_STATUS.md). ShopManager (definitions in
TS load order, stock/restock, pricing, currencies, access
rules), shop flows (NPC Trade dispatch, dialogue shop steps,
widget buy/sell, close), Trading (requests, offer/confirm
stages, transfer, gates, escrow save fold), Duelling state +
the duel-order shuffle exception, the shop/trade hub families
(39/108 firable, 17/95 members) + currency registration, and
the examine price-line accessors all landed. Two findings
from verification: (1) the smoke sell leg first refused until
the fixture carried the item-gameplay `sellable` flag — TS
`canSell` requires it (or specialist buyback) for every item,
and the Go port matches exactly (fixture gap, not a port
bug); (2) the confirm screen arrives as a trade re-OPEN
(stage 1), not a TRADE_UPDATE — a driver-parser fix, not a
port bug. Duel Arena gameplay/staking and the Grand Exchange
marketplace stay NOT STARTED with grounding (plugin content);
the examine price line is wired but silent (no varp/varbit
store); buy/sell-X prompts drop (no entered-amount listener);
the far-WALK bug and the y=3521 map wall stay untouched.
