# Phase 41 Plan — Shopkeepers plugin

Port `server/plugins/npcs/Shopkeepers.plugin.js` (231 lines): the NPC-click
bindings that open shops, plus the item/point shop-currency registrations.
Tyler asked for it 2026-11-21 03:08 PST ("Add shopkeeper piece") right after
Phase 39 landed the shop data with nothing able to open a shop; he ordered
it right after the ditch, before Mining (widget tap 03:09 PST).

## Upstream (read in full)

`~/tsps-bfd81e3/server/plugins/npcs/Shopkeepers.plugin.js`:

- `CLICK_FIELDS` (line 6): `firstClick`..`fifthClick` for option slots 1-5.
- `ITEM_CURRENCIES` (lines 21-40): 19 item ids. `ITEM_CURRENCY_ALIASES`
  (lines 8-12): 2996 Agility arena ticket → `Agility arena ticket
  (discontinued)`; 12746 Archaic emblem tier 1 → `Archaic emblem`; 4278
  Ecto-token → `ecto-tokens`. `ITEM_CURRENCY_UNITS` (lines 18-20): 6306
  Trading sticks → `trade sticks`.
- `POINT_CURRENCIES` (lines 42-56): 13 names (Bounty Hunter Points,
  Deadman Points, `event points`, Honour points, Last Man Standing Points,
  League Points, Mahogany Homes Points, NMZ, PvP Arena Points, Speedrun
  Points, Tithe, Volcanic Mine points, Zeal Tokens).
- `registerPointCurrency` (lines 60-80): attribute `shop-currency:<name>`;
  amount = finite && > 0 ? floor : 0; add/remove clamp at 0; registers under
  each of `[name, ...aliases]` trimmed + uppercased. Exported for other
  plugins (BountyHunter `dependsOn: ["Shopkeepers", ...]`; its Points file
  replaces the generic attribute-backed handler).
- `traderKeys` (lines 86-96): per shop, per `npcInteractions` trader:
  slot must be an integer 1-5, npcIds integers >= 0 → `npcId:slot` keys.
- `defaultShopkeepers` (lines 99-108): bindings map over `shops.json`; a
  key claimed by two different shops becomes `null` (ambiguous).
- `customShopkeepers` (lines 111-118): same over `custom-shops.json`.
- `shopkeepers` (lines 120-126): customs overwrite defaults.
- `sharedTraders` (lines 134-141): keys bound to > 1 `shops.json` shop,
  with the shop list (Daga's Scimitar Smithy versions etc.).
- `openShop` (lines 144-148): `core.ShopManager.open(player, shopId,
  true)`; false → unhandled; true → `event.handled = true`.
- `tradeVersion` (lines 155-169): for a shared key, emit
  `npc-dialogue:shop` with `{player, npcId, target: null, shops: names,
  shop: null}`; the chosen shop is `request.shop`, else the first name
  ending ` (Default)`; neither → unhandled.
- `openDefaultShop` (lines 177-186): handled → false; a custom binding
  for the key → false (hard definitions win); a shared key →
  tradeVersion; else the unique binding → openShop; unknown → false.
- `actionLabel` (lines 189-194): the cache's exact action label for
  (npcId, slot) — `NpcDefinition.forId(npcId).getActions()[slot-1]`,
  null when empty.
- `attachShopkeepers` (lines 196-210): customs → hard definitions via
  `api.registerNpcInteraction(npcId, {[CLICK_FIELDS[slot-1]]: {shopId}})`;
  defaults (minus customs) → `api.onAnyNpcInteraction(actions)` with
  `actions[label] = openDefaultShop` per distinct label.
- `register` (lines 212-226): item currencies, point currencies, attach.

TS dispatch order that matters
(`NPCOptionPacketListener.ts:121-132`, `PluginManager.ts:1246-1263`):
`NpcInteractionManager.handle` (static definitions, incl. the custom
hard bindings → `ShopManager.open` directly) runs BEFORE
`emitNpcInteraction`, which runs the before list, the specific
(onNpcInteraction name hooks), then the any list (onAnyNpcInteraction).
The generated bindings are deliberately the any-list fallback "behind
named owners such as Castle Wars' Lanthus".

## Measured ground truth (probe vs real rev-242 cache + shipped shops.json)

Temporary probe (deleted after use; `TestPhase41ProbeLabels` in
`cmd/server/phase41_probe_test.go`, run from `/home/hatch/smoke40/run1`):

- 487 shops; 669 distinct `npcId:slot` keys; slots: 1×1, 3×405, 4×9, 5×5
  (trader rows; keys expand over npcIds lists).
- 26 ambiguous keys (same key → 2 shops).
- Distinct action labels (9): `Trade` 675, `Rewards` 8, `Trade-Co-op` 4,
  `Exchange` 2, `Shop` 2, `Buy` 1, `Get-rewards` 1, `Trade-Builders-Store`
  1, `Trade-General-Store` 1. Every trader npcId resolves in the cache
  (0 missing).
- The single slot-1 binding is shop 1461 Ranging Guild Ticket Exchange,
  NPC 6071 — its label is not `Talk-to`, so the Go Talk-to-only dialogue
  seam never claims it.
- Shared example: NPC 1943 slot 3 → shops 1020 `Armour Shop (Jatizso)`
  and 1497 `Armour Shop` — neither ends ` (Default)`, so with no
  `npc-dialogue:shop` subscriber tradeVersion returns false (unhandled),
  exactly as in TS.
- `custom-shops.json` is `[]` (empty): zero custom bindings in
  production today. The custom mechanism is still ported + unit-tested.
- Live-leg targets: shop 1102 Edgeville General Store (COINS, NPCs
  2821/2822 slot 3, 14 stock rows: 1931×5, 1935×2, 20742×5, 1735×2,
  1925×3, 22660×15, 1923×2, 1887×2, 590×2, 1755×2, 2347×5, 946×1,
  550×5, 9003×5); Tokkul shop 1468 TzHaar-Hur-Lek's Ore and Gem Store
  (Default), NPC 2184 slot 3, spawns (2464,5149,0) wander 1.

## Go state (verified, don't re-derive)

- ShopManager is a full port (`internal/game/shop_manager.go`, Phase 17):
  `Open(player, shopID, resetScroll, targetUID)` (CanShop denial returns
  true, mirroring TS), `Close`, `HasShop`, `GetOpenShop`,
  `RegisterCurrency` / `RegisterItemCurrency`, `currencyHandlers`.
  `ShopSender.SendShopOpen` is wired in `cmd/server/shopwiring.go:107-116`,
  so opens reach the wire in production.
- Hub currency members exist (`internal/plugins/shops.go`, Phase 17):
  `Api.RegisterShopCurrency` / `Api.RegisterItemShopCurrency` with
  queue-until-`SetShopCurrencyRegistry` semantics; `Core.ShopManager()`
  returns `ShopManagerAPI` (`Open(player, shopID, resetScroll)` with the
  default target UID); `Core.ShopDefinition()` returns
  `ShopDefinitionRegistry` (`ForID` / `All`). Both are nil until the
  coordinator installs them (after hub Load — see below).
- Hub NPC families (`internal/plugins/npcinteraction.go`): `OnNpcClick`
  (Phase 33) and `OnNpcInteraction` name hooks (Phase 35) exist;
  `Hub.EmitNpcInteraction` runs click bindings first, then name hooks.
  `onAnyNpcInteraction` / `onBeforeNpcInteraction` / `onNpcsInteraction`
  are still table-only in `hooks.go`. There is NO `registerNpcInteraction`
  surface (definition contributions), and `Store.Interaction()` is empty
  (0 definitions) because the definition loader only merges contributed
  sources and none exist.
- `Api.PersistAttribute(key)` exists (`internal/plugins/api.go:304`);
  player attributes are `Mobile.GetAttribute` / `SetAttribute`
  (`internal/game/attributes.go:97,129`), nil-safe (nil store → nil / no-op).
- `Api.EmitCustomEvent(name, payload)` exists (`internal/plugins/api.go:249`,
  no load-phase gate); payloads pass by pointer, so a
  `*game.NpcDialogueShopEvent` (`internal/game/transcript.go:682`,
  `{Player, NPCID, Target, Shops, Shop *string}`) mutated by a subscriber
  is visible to the emitter — the same shape the dialogue runtime's
  `executeShopStep` / `shopName` use (`transcript_steps.go:1379-1440`).
- Item ids (TS `ItemIdentifiers.ts`, line cited each):
  Abyssal pearls 26792 (:24745), Agility arena ticket 2996 (:2869),
  Archaic emblem tier 1 12746 (:11210), Archery ticket 1464 (:1398),
  Brimhaven voucher 29482 (:27335), Castle wars ticket 4067 (:3666),
  Ecto-token 4278 (:3869), Golden nugget 12012 (:10477), Hallowed mark
  24711 (:22789), Mark of grace 11849 (:10314), Mermaid's tear 21656
  (:19960), Molch pearl 22820 (:21066), Pieces of eight 8951 (:7957),
  Spirit flakes 25588 (:23608), Stardust 25527 (:23551), Termites 30038
  (:27859), Tokkul 6529 (:5773), Trading sticks 6306 (:5601),
  Unidentified minerals 21341 (:19688).

## The boot-order problem (decides the design)

`cmd/server/server.go`: `loadDefinitions` (cache facades, :168) →
`newPluginHub` → `hub.Load()` (:185) → `loadGameDefinitions`
(`data/definitions` Store incl. shops.json, ~:285) →
`newShopManager` + `installShopHooks` + `installShopHubSurfaces` (:300-301,
`SetShopManager` / `SetShopDefinitionRegistry` /
`SetShopCurrencyRegistry`). So at plugin Register time NEITHER the shop
data NOR the NPC action labels NOR the Core shop surfaces exist yet, and
`Api` registration is sealed after Load (`canRegister`). The plugin
therefore cannot attach its bindings during Register.

Design (two halves, mirroring TS's own split):

1. **Currencies at Register** (no data needed): item currencies via
   `Api.RegisterItemShopCurrency` (queued until the registry installs —
   the existing seam); point currencies via a new exported
   `shopkeepers.RegisterPointCurrency(api, name, aliases...)` porting
   `registerPointCurrency` (PersistAttribute + RegisterShopCurrency under
   uppercased keys). The export mirrors TS (BountyHunter depends on
   Shopkeepers; its Points file replaces the generic handler).
2. **Bindings installed post-Load by the coordinator**: new Hub engine
   methods `Hub.AddNpcClickHook` / `Hub.AddAnyNpcInteractionHook`
   (bypassing the load seal, the `SubscribeCustomEvent` precedent in
   `events.go`) + a new `Api.OnAnyNpcInteraction` family for future
   content with hardcoded labels. The content package exposes
   `shopkeepers.InstallBindings(hub, defaults, customs []plugins.ShopDefinition,
   npcActions func(npcID int) []string) int`, called from
   `cmd/server/shopwiring.go` after `installShopHubSurfaces`. Handlers
   close over the package-level Api set in Register (mirroring TS's
   module-level `let pluginApi`); the ShopManager is resolved lazily at
   emit time through `api.Core().ShopManager()` (hub-contract lesson:
   never capture a post-Load surface in Register).

Custom bindings go through `AddNpcClickHook` (not the Store's interaction
table): TS runs them in `handle()` before the emit, and Go click hooks
run first inside the emit, before name/any hooks — the same claim order
relative to every other hook. The one ordering difference (a slot-1
custom binding would reach Go's Talk-to-only StartDialogue before the
emit; TS handle() runs first) is unobservable today — no custom bindings
exist, and no default binding's label is `Talk-to` — and is documented
as a gap. A general `registerNpcInteraction` definition-contribution
surface stays out of scope (it would need post-load Store contributions;
the Store loads interactions before hub Load).

## New seams needed

1. `internal/definition/shop.go`: parse `npcInteractions` in `toShop`
   (trader rows: `npcIds` ints >= 0, `optionSlot` int 1-5 — the
   `traderKeys` validity rules, applied at load; invalid rows/ids
   dropped, pinned by a loader test). New `ShopNpcInteraction{NPCIDs
   []int, OptionSlot int}` + `ShopDefinition.NpcInteractions()`. Also
   record which shop ids came from `custom-shops.json`:
   `readCoreShops` already concatenates both files under source `core`
   (exactly as TS `ShopDefinitionLoader.readCoreDefinitions` does — TS
   also reports source `core` for custom shops), so keep `Source()`
   unchanged and add `Store.IsCustomShop(id int) bool` fed by the
   custom-file rows. The install splits defaults/customs on it.
2. `internal/plugins/shops.go`: `ShopNpcInteraction` (plugins-local type)
   + `NpcInteractions() []ShopNpcInteraction` on the `ShopDefinition`
   interface; `cmd/server/shopwiring.go` `definitionShopAdapter` maps it.
3. `internal/plugins/npcinteraction.go`: `OnAnyNpcInteraction` family —
   `anyNpcInteractionHook{entry, actions}`, `Api.OnAnyNpcInteraction`
   (filter nil handlers; empty map warns + drops, mirroring
   `OnNpcInteraction`), `Hub.AddAnyNpcInteractionHook` (engine install,
   same validation), `Hub.AddNpcClickHook` (engine install, same
   validation as `Api.OnNpcClick`), emit step after name hooks in
   `Hub.EmitNpcInteraction` (TS order before → specific → any; Go has no
   before list). Hub count becomes 58/108 firable, 62 methods.

## Content package (`internal/content/shopkeepers/`)

- `shopkeepers.go`: Plugin value (name/description verbatim from the
  plugin file), `init()` → `plugins.Register`, `Register(api)`:
  currencies only + stash `api` package-level. Item-id const block with
  `ItemIdentifiers.ts` line citations; alias/unit tables.
- `bindings.go` (pure, unit-testable): `TraderKey{NPCID, Slot}`,
  `ComputeBindings(defaults, customs []plugins.ShopDefinition) Bindings`
  (ports `defaultShopkeepers` + `customShopkeepers` + `shopkeepers` +
  `sharedTraders`: `Custom map[TraderKey]int`, `Unique map[TraderKey]int`
  with ambiguous → absent, `Shared map[TraderKey][]ShopRef{ID, Name}`),
  `LabelsFor(b Bindings, npcActions func(int) []string) map[string]bool`
  (ports the actions-map build: label = actions[slot-1], skip empty).
- `handlers.go`: `openShop(event, shopID)` (nil manager or failed open →
  false; else `Handled=true`), `openDefaultShop(event)` (handled →
  false; custom key → false; shared → tradeVersion; unique → openShop;
  unknown → false), `tradeVersion(event)` (emit `npc-dialogue:shop`
  with `*game.NpcDialogueShopEvent`; chosen = `request.Shop`, else first
  name ending ` (Default)`; neither → false), `RegisterPointCurrency`,
  `InstallBindings` (registers customs via `AddNpcClickHook`, defaults
  via `AddAnyNpcInteractionHook` keyed by label; no-op when hub or the
  stashed api is nil; returns bindings installed; coordinator logs it).
- Point-currency handler semantics (exact): amount = stored number
  finite && > 0 ? floor : 0 (stored values are int/float64; non-numeric
  → 0); add = amount + max(0, value); remove = max(0, amount - value)
  — note TS does NOT clamp a negative remove value, mirror it.
- `shopkeepers_test.go`: traderKeys/ComputeBindings (invalid slots,
  ambiguous → null, custom precedence over ambiguous defaults, shared
  detection incl. the 1943/1020/1497 shape), label mapping (skip empty
  labels), point currency amount/add/remove/clamp + uppercased keys +
  persist key, item-currency registration (no panic, queued + flushed),
  openShop/openDefaultShop/tradeVersion with a fake `ShopManagerAPI`
  and a test hub (follow the wildernessditch fake-session/test-hub
  pattern), incl. the no-Default shared case → false and the
  subscriber-picks-a-version case → opens it.

## Wiring (`cmd/server`)

- `pluginwiring.go`: blank import `_ "tsps-go/internal/content/shopkeepers"`
  (alphabetical: after seekingarrows, before wildernessditch);
  `pluginwiring_test.go` pinned plugin-list tests updated.
- `shopwiring.go`: `installShopkeepers(hub, store, defs)` after
  `installShopHubSurfaces` in `server.go` (~:301): split
  `store.Shops()` by `store.IsCustomShop`, NPC actions from
  `defs.GetNpc(id).Actions` (nil def → no actions → label skipped, as
  TS `actionLabel` returns null for a missing definition), call
  `shopkeepers.InstallBindings`, log the count. Nil-safe throughout
  (NoCache: no store/defs → skip).
- `definitionShopAdapter.NpcInteractions()` mapping.

## Live verification (`/home/hatch/smoke41/`, fresh temp root)

Real rev-242 cache from `/home/hatch/smoke27/seed-root/caches/osrs-242_2026-11-03`
(copy `target.txt` + `data/`, symlink `caches`; never run from the repo
root). Legs run singly if the full driver exceeds the exec wall clock
(`RUNS` override). Harness traps: gorilla readErr is sticky
(deadline-free reader goroutine); client must send ClientHandshake
after LOGIN; driver must write per-mode `npc-spawns.json` into the temp
root; NPC spawning is eager at 150/tick — allow ~2 min after boot for
the world to fill. Legs (each: checks counted, 0 failures to pass):

- **shop-open**: walk to Edgeville shop keeper 2821, option 3 (opcode
  34) → shop 1102 opens on the wire (shop-open frame, title `Edgeville
  General Store`, currency COINS, the 14 stock rows with the amounts
  above).
- **shop-buy**: buy 1× 1931 (pot) with coins → coins decrease by the
  shown price, pot in inventory, stock 5→4.
- **shop-sell**: sell a bought item back → coins increase, stock grows.
- **shop-shared**: click NPC 1943 option 3 → NO shop opens (no Default
  version, no subscriber) and the click falls through unhandled.
- **shop-tokkul** (attempt): spawn near TzHaar-Hur-Lek (2464,5149),
  grant Tokkul via `::item 6529`, click NPC 2184 option 3 → shop 1468
  opens with TOKKUL currency; buy 1 item with Tokkul. If the NPC is not
  reachable in the harness, fall back to unit-covered and say so.

## Docs

- `PORT_STATUS.md`: content row 12 of 577; new §24 residual gaps;
  "What was verified" entry; P3 hub row (58/108, 62 methods) + the
  unfired-family table (`onAnyNpcInteraction` leaves table-only;
  `onBeforeNpcInteraction` / `onNpcsInteraction` stay).
- `internal/plugins/hooks.go` header: update the "other 65" count and
  the `onAnyNpcInteraction` table-only mention; `npcinteraction.go`
  header: any-list now firable; `dialoguewiring.go` + `listeners/npc.go`
  comments where they say no any-hooks exist.
- `PHASE41_PLAN.md`: Results section when done.

## Verification

`go build ./...`, `go vet ./...`, `gofmt -l .`,
`CGO_ENABLED=0 go build ./...` clean; full suite `-p 1 -count=1 -v`
with exact pass/fail/skip counts + package count; `go test -race ./...`
clean. Report anything unit-covered-only vs live-observed plainly.

## Out of scope

Mining, Smithing, other NPC content, Grand Exchange, trading,
`registerNpcInteraction` as a general surface, `registerPointCurrency`
consumers (BountyHunter etc. — future phases), client, Android, gateway.
No commits (parent handles git). Do NOT touch `/home/hatch/live36` or
`~/tsps-full`. No broad process cleanup. No new dependencies. No stray
artifacts, probe files, or `.js` files in the repo; delete temporary
probe tests after use.

## Residual gaps (expected, to be recorded in §24)

- General `registerNpcInteraction` (definition contributions incl.
  teleport/dialogue actions) stays table-only.
- `onBeforeNpcInteraction` / `onNpcsInteraction` stay table-only.
- The slot-1 custom-binding ordering note (above).
- `registerPointCurrency` consumers (BountyHunter, LMS, etc.) not
  ported — the export exists for them.
- Shared-trader version picking with no subscriber and no Default
  version leaves the click unhandled (TS-faithful, live-verified on
  NPC 1943).

## Results (2026-11-21, verified)

Done. Shopkeepers is ported and live-verified; gap 22(d) is
RESOLVED. Hub: **58 of 108 families firable, 62 `Api.On*`
methods** (4 named overloads). Content: **12 of 577**.

### Files changed / added

- New package `internal/content/shopkeepers/` (4 files):
  `shopkeepers.go` (Plugin, Register — 19 item + 13 point
  currencies), `bindings.go` (`ComputeBindings` /
  `LabelsFor`), `handlers.go` (`RegisterPointCurrency`
  export, `openShop` / `openDefaultShop` / `tradeVersion`,
  `InstallBindings`), `shopkeepers_test.go` (10 tests).
- `internal/definition/shop.go` + `loader.go`:
  `ShopNpcInteraction` + `ShopDefinition.NpcInteractions()`,
  `parseShopNpcInteractions` (TS `traderKeys` validity
  rules at load), `readCoreShops` returns core + custom
  separately, `Store.IsCustomShop(id)`.
- `internal/plugins/shops.go`: `ShopNpcInteraction` type +
  `NpcInteractions()` on the `ShopDefinition` interface.
- `internal/plugins/hooks.go` + `npcinteraction.go`:
  `onAnyNpcInteraction` registry + `Api.
  OnAnyNpcInteraction` + `Hub.AddNpcClickHook` /
  `Hub.AddAnyNpcInteractionHook` (engine installs bypassing
  the load seal); `EmitNpcInteraction` runs the any list
  LAST (TS order before → specific → any, minus the
  unported before list).
- `cmd/server/pluginwiring.go` (blank import),
  `shopwiring.go` (`installShopkeepers`, called from
  `server.go` after `installShopHubSurfaces`),
  `pluginwiring_test.go` (load-order lists).
- `internal/listeners/npc.go`: arrival-closure fix (below)
  + header comment; `internal/listeners/npc_test.go`:
  fixtures carry HP 10 (live-NPC convention) + 2 new
  arrival tests.
- `internal/definition/shop_test.go`: `TestShopNpcInteractions`.
- Docs: `PORT_STATUS.md` (header, "What was verified",
  P3 row, content row 12/577, gap 22(d) RESOLVED, new §24);
  this plan's Results section.

### Verification numbers

- `go build ./...`, `go vet ./...`, `gofmt -l .`,
  `CGO_ENABLED=0 go build ./...`: all clean.
- Full suite `go test ./... -p 1 -count=1 -v`: **4,713
  passing (3,565 top-level + 1,148 subtests), 0 fail,
  0 skip, 24 packages** (was 4,696 / 23 packages in
  phase 40; +17 tests).
- `go test -race ./...`: **clean, 24/24 packages, no data
  race** (the phase-37-suspect `combatant.go`
  `SetCombatCycleSource` race still does not reproduce).
- Boot log: `[shopkeepers] installed 669 shopkeeper
  bindings (487 default shops, 0 custom shops)`.

### Live legs (`/home/hatch/smoke41/`, real rev-242 cache, run singly) — 32 checks, 0 failures

- **shop-open PASS 7/7**: Trade on Shop keeper 2821 →
  shop 1102 Edgeville General Store on the wire (id,
  title, currency 995, the 14 file stock rows in order).
- **shop-buy PASS 7/7**: 1 pot 1931 for 1 coin; coins
  1000→999, pot in inventory, re-open frame shows stock
  5→4.
- **shop-sell PASS 7/7**: pot sold for sellPrice 0; pot
  gone, coins 1000, re-open frame shows stock 5→6.
- **shop-shared PASS 3/3**: Trade on Raum Urda-Stein 1943
  → NO shop opens (shared trader, no subscriber, no
  Default version) and the player walks to the NPC.
- **shop-tokkul PASS 8/8**: Trade on TzHaar-Hur-Lek 2184
  → Default shop 1468 (16 file rows, currency item -1 as
  TS sends it); bought 1 copper ore 438 for 4 Tokkul
  (100→96).

### Two live-found bugs, both TS-grounded, both fixed in-phase

1. **Wandering-NPC arrival drop** (`listeners/npc.go`):
   the arrival closure re-validated with
   `current.Location() != npcLoc` and dropped the
   interaction whenever the NPC stepped off its
   click-time tile mid-walk. TS does not do this:
   `MovementQueue.ts:1525` + `isInteractionTargetValid`
   (lines 1696–1704) checks registered + hp > 0 + same
   private area + same plane + owner — no tile check —
   and `npcType` (1690–1694) is transform id else npc id.
   Fix: arrival checks identity / `ContentID() ==
   clickedType` / HP > 0 / same plane. Unit tests: a
   wandering NPC still interacts; a transform mid-walk
   drops silently. Side effect: listener test fixtures
   built with `game.NewNPC` carry HP 0 (Go's zero-means-
   unset), which the new check reads as dead — fixed by
   giving the fixtures HP 10, the live-NPC convention
   (the spawn path gives every world NPC HP > 0).
2. **`tradeVersion` Default fallback** (`handlers.go`):
   the first port gated on `request.Shop != nil` only.
   TS picks `request.shop ?? first name ending
   " (Default)"` itself (`Shopkeepers.plugin.js:155-171`)
   — the Default fallback lives in tradeVersion, not in a
   subscriber, and the emit's handled flag is not
   consulted. Fixed + unit-tested (no-subscriber Default
   opens 1468; subscriber pick beats Default → 1469);
   live-confirmed by shop-tokkul.

### Findings recorded, not bugs

- The shop-open frame's currency item is 65535 (-1) for
  Tokkul: TS `currencyItemId` (`ShopManager.ts:1299`) is
  the same hardcoded COINS/BLOOD_MONEY switch, so the Go
  port matches it exactly; the Tokkul payment is
  live-proven by the buy.
- The TzHaar gem-store NPC cannot be routed to from 3
  tiles south (2464,5152) — the counter/wall blocks the
  route — so the tokkul leg seeds adjacent (2464,5150);
  a routing observation, not a shop bug.
- `custom-shops.json` is `[]` (empty): zero custom
  bindings in production today; the custom mechanism is
  still ported + unit-tested.

### Residual gaps (see PORT_STATUS.md §24)

General `registerNpcInteraction` table-only;
`onBeforeNpcInteraction` / `onNpcsInteraction`
table-only; slot-1 custom-binding ordering note;
`registerPointCurrency` consumers not ported;
shared-trader with no subscriber and no Default
unhandled (TS-faithful); transcript still not moved
into the emit, StartDialogue not narrowed to static
bindings only.
