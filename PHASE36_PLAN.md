# Phase 36 Plan — Cooking (fourth skill, F2P)

Tyler picked Cooking (text 2026-11-20 21:39 PST; not on the options widget, no
conflict). F2P, entry `Cooking.plugin.js` 39 lines (thin registrar), true size
2,045 lines. This file was written by the coordinator after reading every
upstream file in full (`plugins/skills/Cooking.plugin.js`,
`plugins/skills/cooking/*.js`, `plugins/skills/data/cooking.json`) and checking
every Go seam cited below against the repo at Phase 35 state (735 .go files,
20 packages, 4,534 tests passing, race clean, hub 57/108 firable — 61 `Api.On*`
methods, 4 named overloads).

## 0. Data decision — SETTLED, do not revisit

`data/definitions/cooking.json` was imported verbatim by the parent on
2026-11-20 with Tyler's approval (widget tap 21:41 PST). SHA-256 identical on
both sides: `5760fe60c7d58d140ed5fb6ea3168c72c2ae02640702d2a66a369156b4ff5788`.
Full suite still 4,534 passing after the import. **Use the file verbatim; do
not re-import it, do not transcribe it instead, do not copy it anywhere else.**
This phase builds the Go loader for it and load-proves it against the real
rev-242 cache (temporary test, then delete the test — the Phase 26–28 method).
PORT_STATUS.md's data-import paragraph already has the entry (line 469) —
extend it with the loader + load-prove numbers. The Woodcutting Guild import
(`woodcutting-guild.json`) is still unanswered — do NOT import it, do NOT fold
the Guild into this phase.

## 1. What the upstream skill is (all read in full)

Registrar (`Cooking.plugin.js:1-39`): registers Cookers, Recipes, Wine,
SacredEel, LeapingFish, Gnome, Churn. Exports `isCookingActive`, `isCooker`,
`canCookFood`, `COOKABLE_BY_RAW`, `COOKABLE_OBJECT_NAMES`, `FIRE_OBJECT_NAMES`.

### 1.1 cooking.json (29,762 B; verified by parse, not by the survey)

Top-level keys: `cookables`, `doughs`, `recipes`, `wines`, `churn`.

- `cookables`: 122 rows. Row keys: `article, burnMessage, burnt, chance,
  cooked, duringQuest, facility, failsWithoutBurning, firstCookTicks,
  gnomeTutorial, level, message, name, preparationStopBurn, rangeOnly, raw,
  returnItem, returnOnBurn, rumours, stopBurn, xp`. 100 rows have `chance`,
  22 are stopBurn-only. `facility` (Cookers must NOT cook these):
  `raw_guppy, raw_cavefish, raw_tetra, raw_catfish` (preparationStopBurn 36 /
  45 / 54 / 70). `rangeOnly`: 24 rows (all uncooked pies/pizza/cake/potato
  plus numeric gnome ids 9576, 9581, 9577, 9482, 9483). `gnomeTutorial`: 7
  rows (9576, 9581, 9577, 9482, 9483, `raw_crunchies`, `raw_batta`).
  `duringQuest`: `tbwt_raw_karambwanji` → `tai_bwo_wannai_trio`.
  `rumours`: 6 hunter meats (graahk/kyatt/fennecfox 25, antelopesun/
  speedy2/antelopemoon 50). `failsWithoutBurning`: [9576] (drunk dragon).
  `returnItem`: 5 rows (9581/9577 → `crunchy_tray`, 9482/9483 → `batta_tin`,
  `uncooked_cake` → `cake_tin`; `returnOnBurn` true only for uncooked_cake).
  `firstCookTicks`: 9576 → 1, `raw_gnomebowl` → 4, 9563 → 4. `chance` key
  sets: `(fire,gauntlets,range)` ×6, `(fire,lumbridge,range)`,
  `(fire,range)`, `(lumbridge,range)`, `(range,)` alone. 32 rows have a
  `message`, 1 has a `burnMessage`. No row has `lumbridgeStopBurn`.
- `recipes`: 47 rows. Keys: `ingredients, level, message, product, returns,
  timing, xp`. Only 3 rows have `timing`, 7 have `xp`, 7 have `message`, 0
  have `members`. Defaults (Common.Cooking.js:52-53): `xp ?? 0`,
  `timing ?? [1, 2]`. Ingredients are gameval names or `[name, count]` pairs;
  `returns` likewise.
- `wines`: 2 rows. Grapes: level 35, xp 200, chance [48, 352],
  `pendingLimit` 60000. Zamorak grapes: level 65, xp 200, chance [-125, 260].
- `doughs`: flour `[pot_flour, pot_empty]`; waters 3 pairs
  (`bucket_water`/`bucket_empty`, `bowl_water`/`bowl_empty`,
  `jug_water`/`jug_empty`); products 4 (bread_dough 1, pastry_dough 1,
  pizza_base 35, uncooked_pitta_bread 58) each with a `message`.
- `churn`: inputs 4 (`pot_of_not_garlic_butter` → butter, `pot_of_butter` →
  butter, `pot_of_cream` → cream, `bucket_milk` → milk, returns
  `bucket_empty`); products 7 (milk→cream 21/18xp/10 ticks,
  milk→butter 38/40.5/19, milk→cheese 48/64/26, cream→butter 38/22.5/10,
  cream→cheese 48/46/19, butter→cheese 48/23.5/10, not-garlic-butter→cheese
  48/23.5/10; seqs `player_churns_milk_short/medium/long`); labels
  (`pot_of_cream` → Cream, `pot_of_butter` → Butter, `cheese` → Cheese).

### 1.2 Common.Cooking.js (192 lines) — shared helpers

- `DATA = require("../data/cooking.json")`. `index()` builds:
  `cookablesByRaw` (first row wins per raw id), `recipesByIngredient`,
  `recipesByProduct`, `doughs`, `wines`. Resolution is by cache gameval:
  `gameval(kind, name)` throws on unknown (`obj`/`seq`/`spotanim`); `obj()`
  also accepts a numeric id (for ambiguously-named items). Ingredient entries
  are `[name, count]` or bare names (count 1).
- `successChance(low, high, level)` (lines 115–118): the interp formula —
  `floor((low*(99-level))/98 + (high*(level-1))/98 + 0.5) + 1`, divided by
  256, clamped to [0,1]. NOTE: uncapped at 99 — levels above 99 keep raising
  it. Same formula family as Fishing `catchChance`.
- `level(player)` (121–125): current Cooking level + `skills:level-boost`
  query (`{player, skill, boost: 0}`; boost clamped ≥ 0). No Go answerer for
  the boost — default 0.
- `actionsPerCycle(player, kind)` (128–132): `skills:actions-per-cycle`
  query (`{player, skill, kind, actions: 1}`), clamped ≥ 1. No Go answerer —
  default 1.
- `wearsCookingCape(player)` (135–139): `skillcape:has-perk` query
  (`{player, skill: "COOKING", result: false}`). The Go query type already
  exists (`game.SkillcapeHasPerkEventName` /
  `game.SkillcapePerkRequest` in `internal/game/equipment_effects.go`) and
  has NO answerer in Go — `Result` stays false. Reuse those types; do NOT
  redefine them.
- `withArticle(name, article)`, `spam` (SPAM chat), `mesbox` (statement
  dialogue), `sendVarbit` (by gameval name, skipped when unresolvable).
- `KNIFE_NAMES = ["KNIFE","KNIFE_2","KNIFE_3","KNIFE_4","KNIFE_5"]` resolved
  via `ItemIds` (missing names filtered out).
- `addXp(player, xp)`: no-op when xp ≤ 0.
- `run(player, step)` (177–185): a 1-tick keyed task; `step(++tick)` returns
  false to stop; stops when the player unregisters or dies. Go equivalent:
  the Woodcutting/Fletching 1-tick task pattern — `game.NewTask(1,
  func(cycle int64) bool)` + lazily-resolved TaskManager (see §3).

### 1.3 Recipes.Cooking.js (229 lines) — item-on-item recipes + doughs

- Constants: `DOUGH_TITLE = "What sort of dough do you wish to make?"`,
  `MAKE_TITLE = "How many do you wish to make?"`, `MAKE_MENU_MODE = 0`,
  `DOUGH_TIMING = [1, 1, 2]`, `CHOCOLATE_BOMB_BOWL = 9563`,
  `NO_SPACE_FOR_DOUGH_MESSAGE = "You don't have enough inventory space to
  make any dough."`
- `hasIngredients`, `freedSlots` (stack-aware: a stackable ingredient frees
  1 slot only when the recipe takes the whole stack — the swamp-tar case),
  `hasRoom` (free slots ≥ 1 + returns − freed), `makeable` (min over
  ingredients of floor(amount/count)), `productName` (cache name, lowercased).
- `isMembersRecipe`: row `members` override, else the cache item's
  `isMembers` (no recipe row sets `members` — cache only in practice).
  `availableOnWorld`: members world or not a members recipe.
- `canMake`: F2P refusal sends `FreeToPlay.ITEM_MESSAGE` =
  `"To use this item please login to a members' world."` (verified in
  `FreeToPlay.plugin.js:8` — use this exact string; no Go constant exists
  yet, define it once in the cooking package); level refusal is a mesbox
  `You need a Cooking level of {level} to make {article+name}.`
- `makeOne` (77–93): available + ingredients + room (else inventory.full());
  consume ingredients, add product, add returns, spam `message`, addXp,
  emit `skills:produced {player, skill: COOKING, itemId: productId, amount:
  1, xp, kind: "make"}`. No Go consumer for `skills:produced` except
  Fletching Diaries (fletching-specific tasks) — emit it anyway.
- `start` (101–128): `TaskManager.cancelTasks(player index)` first;
  `hooks.canMake() === false` aborts; timing loop via `Common.run` with
  `wait = recipe.timing[0]`, `hooks.cycle(remaining)` at each item start,
  `hooks.made()` after each, `hooks.done()` at the end; `actionsPerCycle`
  per cycle, capped by remaining; `wait =
  recipe.timing[min(made, len-1)]` (last value repeats).
- `offer` (134–153): `canMake` first, room first, `most = makeable`; most
  ≤ 0 → return; most == 1 → `start` directly (no menu); else CreationMenu
  (`MAKE_TITLE`, `[productId]`, execute → `start(player, recipe,
  min(amount, makeable), hooks)`, `{mode: 0, maxAmount: most, lastAmount:
  most}`).
- `doughRecipe` (156–167): product with 0 XP, ingredients flour 1 + water 1,
  returns pot 1 + empty container 1, timing `DOUGH_TIMING`.
- `offerDough` (173–195): free-slot check first (else
  NO_SPACE_FOR_DOUGH_MESSAGE); menu `DOUGH_TITLE` with all 4 dough product
  ids, `maxAmount`/`lastAmount` = min(flour, water); execute finds the
  product, builds the dough recipe, `canMake` then `start`.
- `waterFor` (197–202): either item is flour → the other must be a dough
  water.
- `useItemOnItem` (204–216): `onItemOnItem` (`{noted: false}`); water →
  `offerDough` (handled); else `recipeWith(first, second)` (data order,
  lowest level first, `firstId !== secondId`) → `offer` (handled).
- `prepareGnomebowl` (219–224): `onItemAction("Half baked bowl", {Prepare:
  ...})`; recipe for product 9563, only when `itemId` is gameval
  `half_baked_bowl`; `offer`.

### 1.4 Cookers.Cooking.js (717 lines) — the core loop

- Timing: `FIRST_COOK_TICKS = 2`, `SECOND_COOK_TICKS = 3`,
  `COOK_INTERVAL_TICKS = 4`, `KARAMBWAN_RECLICK_TICKS = 1`.
- Titles: `ONE_TYPE_TITLE = "How many would you like to cook?"`,
  `MANY_TYPES_TITLE = "What would you like to cook?"`, `COOK_MENU_MODE = 6`.
- Messages: `NOTHING_TO_COOK_MESSAGE = "You haven't got anything to
  cook."`, `CANNOT_COOK_MESSAGE = "You can't cook that."`,
  `NOT_USEFUL_MESSAGE = "You don't feel that would be useful now."`
- `SUCCESS_COUNT_VARBIT = "cooking_success_count"`,
  `SUCCESS_COUNT_ATTRIBUTE = "cooking:success-count"`, `BUSY_VARBIT =
  "busy"`. Persist the attribute with default 0 (`api.persistAttribute`).
- `DELIBERATE_BURN`: cooked meat/chicken → burnt, xp 0, `stopBurn: 1`,
  message `You deliberately burn the nicely cooked meat.` (chicken variant
  likewise). `deliberateBurnFor` maps `COOKED_MEAT`/`COOKED_CHICKEN` by
  `ItemIds`.
- `availableOnWorld(cookable)`: members world, or the raw item is not
  members AND the cookable is not `gnomeTutorial`.
- `cookableFor(itemId)`: cookables map entry or deliberate burn; excludes
  `facility` cookables; excludes unavailable-on-world.
- Karambwan: `KARAMBWAN_TAUGHT_ATTRIBUTE =
  "quest.tai_bwo_wannai_trio.tinsay"`, value 8. Untrained raw karambwan cooks
  into `POISON_KARAMBWAN`, xp 80, message `You cook the karambwan. It looks
  very dodgy.` — and its level gate drops to 1 (`cookedResult`).
- `successMessage` default `You successfully cook {article+name}.`;
  `burnMessage` default `You accidentally burn the {name}.`
- Object names: `FIRE_OBJECT_NAMES = {Fire, Forester's Campfire, Firepit,
  Firepit with hook, Firepit with pot}`; `HOUSE_RANGE_NAMES = {Small oven,
  Large oven, Steel range, Fancy range}`; `RANGE_OBJECT_NAMES = {Cooking
  range, Range, Stove, Oven, Clay oven, Clay Oven}` (both spellings);
  `COOKABLE_OBJECT_NAMES` = union. `cookAnimationFor`: fire/house →
  gameval `human_firecooking`, else `human_cooking`.
- Hosidius Kitchen: rect x 1674–1684, y 3616–3626, z 0; object must be
  `ObjectIdentifiers.CLAY_OVEN_3`; bonus 12, or 25 with elite Kourend &
  Kebos Diary (`diary:is-complete {player, diary: "kourend", tier:
  "elite", complete: false}` — NO Go answerer, so the bonus is 12; document
  it, do not invent an answerer).
- `wearsCookingGauntlets`: hands slot id == `ItemIds.COOKING_GAUNTLETS`.
- `chanceValues(player, cookable, objectId, cooker, location)`: gauntlets
  chart first (when worn and the food has one), else `lumbridge` chart when
  `objectId == ObjectIdentifiers.COOKING_RANGE`, else `fire`/`range` chart
  per cooker kind (each falling back to the other); Hosidius bonus added
  only on a range. Returns null for a food without Wiki values.
- `stopBurnLevel`: search levels 1..200 (`HIGHEST_STOP_LEVEL`) for
  `successChance >= 1`; without values, Lumbridge-range
  `lumbridgeStopBurn` (no row has one) else `cookable.stopBurn`; Infinity
  when never.
- `successChance(player, ...)`: cooking cape → 1; else Wiki values via
  `Common.successChance`; else the fallback line — 58% at the food's level
  rising to 100% at `stopBurn` (`burnChance = 42 - ((level -
  cookable.level) * 42) / (stopBurn - cookable.level)`, chance = `1 -
  burnChance/100`; 1 when `level >= stopBurn` or `stopBurn <=
  cookable.level`).
- `isSuccess`: `random() >= 1 - successChance` (a roll of 0 burns anything
  that can burn).
- Sessions: `activeSessions` map (player → session). `stopCooking(player,
  resetAnimation=true)`: delete; if `cooked == 0` send busy varbit 0;
  reset animation when asked. `countCook(player, burnt)`: count 0 on burn
  else +1, set attribute + varbit. `countCookEvent` answers
  `cooking:count {player, burnt}`. `sendSuccessCount` on login (only when
  count > 0).
- `canCook(player, object, cookable)` — gate order, all with messages:
  members (`"To use this item please login to a members' world."`);
  `gnomeTutorial` on a fire → mesbox CANNOT_COOK; `COOKING_RANGE` object
  requires `quest:is-complete {player, key: "cooks_assistant", complete:
  null}` — NO Go answerer, so the Lumbridge Castle range is refused with
  `You need to complete Cook's Assistant to use this range.` (fail-closed,
  documented); level gate (untrained karambwan → 1) mesbox `You need a
  Cooking level of {level} to cook {article+name}.`; `duringQuestMet`
  (`quest:is-started` + `quest:is-complete`, both unanswered in Go → a
  `duringQuest` food never cooks; fail-closed, documented);
  `rumoursMet` (`hunter:rumours-completed` unanswered → 0; a `rumours` food
  never cooks; fail-closed, documented).
- `canCookFood(player, cookable)`: same gates without messages (bots use
  it; also the alternates check).
- `validSelectedInput(player, cookable, selected)`: integer slot, the slot
  still holds the same item, id == raw, amount == 1, not preset /
  untradeable / unbankable.
- `startCooking(player, object, cookable, sessions, amount=Infinity,
  selected=null, alternates=[])`: `canCook`, `validSelectedInput`,
  inventory contains raw; karambwan re-use (previous session, same raw,
  `RAW_KARAMBWAN`): next tick is the previous one when `cooked == 0`, else
  `cycle + KARAMBWAN_RECLICK_TICKS`; otherwise `cycle +
  (cookable.firstCookTicks ?? FIRST_COOK_TICKS)`. Session: `{cookable,
  selected, alternates, animation, cooker: "fire"|"range", objectId,
  objectType, objectFace, location (clone), privateArea, nextCookTick,
  cooked, remaining}`. BUSY varbit 1 unless reusing with `cooked == 0`.
- `carriedCookables(player, onFire)`: inventory order, skip noted items,
  skip `facility`, skip unavailable-on-world, skip `rangeOnly` on fire,
  dedup by raw id.
- `offerCook(player, object, cookables, singleClick=false, selected=null)`:
  one cookable → `canCook`; carried ≤ 1 → `startCooking(..., 1, selected)`
  and when `singleClick && cookable.firstCookTicks === undefined`, cook
  immediately (2-tick cooking: `cookStep` at once, then `stopCooking`);
  carried > 1 → menu `ONE_TYPE_TITLE` with `[raw]`, most = carried.
  Many cookables → group by cooked product (`cookedResult`, so karambwan
  groups by poison-vs-cooked); `most` = max group total; menu
  `MANY_TYPES_TITLE` with product ids; choose → first cookable of the group
  + the rest as `alternates`.
- `openCookMenu`: CreationMenu(title, items, {execute: choose}, `{mode: 6,
  maxAmount: most, lastAmount: most}`).
- `currentCooker(session)`: `MapObjects.get(objectId, location,
  privateArea) ?? ObjectManager.objectsAt(location).find(matches)`; template
  fallback `privateArea.resolveObject`; null when a same-slot object was
  replaced or the private area is destroyed. Go has the `MapObjects.get`
  seam (Phase 32) and `World.getRemovedObjects` (Phase 33) — verify the exact
  Go method names at implementation time.
- `cookTick(task)` — ONE global 1-tick task for all cooking players
  (submitted by `ensureCookTask` on first cook): `cycle++`; unregistered →
  delete; HP ≤ 0 → `stopCooking(player, false)`; movement queue non-empty
  or force movement → `stopCooking(player)` (animation reset); `levelUpPaused`
  → skip; `remaining <= 0` or no `carriedRaw` → stop; cooker gone or out of
  interaction distance → stop; `cycle < nextCookTick` → wait; else
  `cookStep`.
- `carriedRaw(player, session)`: contains raw, else move to the first
  alternate the inventory contains and `canCookFood` passes.
- `cookStep(player, session, cycle)`: pace = `cooked == 0 ? 3 : 4`;
  `nextCookTick = cycle + actionInterval(api, player, COOKING, "cook",
  pace)`; BUSY varbit 0 once the first food starts; `cooked++`;
  `remaining--`; `Sound.COOKING_COOK` (Go: `game.SoundCookingCook`, id 2577,
  already ported); animation; `actions = min(actionsPerCycle(player,
  "cook"), remaining + 1)`; loop `cookOne`, `done > 0` decrements remaining
  again; `cookOne === false` stops cooking.
- `cookOne(player, session)` — in order: deliberate burn (delete raw, add
  cooked, spam message, return); selected-input revalidation; gnomeTutorial
  slot logic (find the single non-preset slot; `Preset items cannot be used
  for this.`; stackable guards; `returnItem` needs a free slot);
  non-gnome `returnItem` needs a free slot BEFORE anything changes, then
  delete raw; emit `cooking:burn {player, rawId, itemId: cooked, burn}` —
  NO Go consumers; the emit may flip `burn`, so read it after; gnome
  revalidation + `deleteAtSlot`; success: addable check (0 → restore the
  raw, return); add cooked; `returnItem`; spam success; `countCook(false)`;
  emit `items:obtained {player, itemId: cooked, rawId, amount, source:
  "cooking", location}` (no Go consumers); emit `cooking:success {player,
  skill, rawId, itemId, amount, location, objectId, objectLocation}`;
  addXp; emit `skills:produced {kind: "cook"}`. `failsWithoutBurning`
  (drunk dragon 9576): add the burnt item, `countCook(false)`, NO XP, NO
  message. Burn: add burnt, `returnOnBurn` → returnItem, emit
  `cooking:burnt {player, rawId, itemId: burnt}`, spam burn message,
  `countCook(true)`.
- `isCooker(definition)`: name in `COOKABLE_OBJECT_NAMES` AND (no actions
  listed, or actions include "Cook", or a fire name). Note: a Forester's
  Campfire has no Cook action — food is used on it directly.
- `handleCook` (`onItemOnObject`, `{noted: false}`): isCooker; `cookableFor`
  (rangeOnly on fire → return); `event.handled = true`; gnomeTutorial →
  `selected = {item, slot}` + validation; `offerCook(player, object,
  [cookable], false, selected)`.
- `refuseCook` (`onItemOnObject`, `{noted: false, fallback: true}`):
  isCooker → `handled`, mesbox CANNOT_COOK. Registration order matters:
  `handleCook` first, `refuseCook` as the fallback. Verify the Go
  `OnItemOnObject` fallback semantics (Phase 33 `OnItemOnItemNamed`
  claim-on-`handler() !== false`; check the object variant the same way).
- `handleCookClick` (`onObjectInteraction("Fire"|"Range"|"Cooking range"|
  "Stove"|"Oven"|"Clay oven"|"Clay Oven"|"Small oven"|"Large oven"|"Steel
  range"|"Fancy range"`, `{Cook: ...})`): `carriedCookables`; empty →
  mesbox NOTHING_TO_COOK; `offerCook(..., singleClick=true)`.
- `answerCooked` (`cooking:cooked {rawId}` → sets `cookedId`, `xp` when the
  cookable exists and is not `facility`). No Go emitter exists — define the
  request type in the cooking package, document the TS shape.
- `answerRawXp` (`cooking:raw-xp {rawId}` → sets `xp` when the cookable
  exists and is not `facility`). **This is the Phase 35 unblock**: Fishing's
  `tryCookFish` (`internal/content/fishing/infernalharpoon.go:195-215`)
  emits `&CookingRawXPRequest{Player, RawID}` and reads `HasXP`/`XP` after.
  The cooking answerer MUST type-assert to `*fishing.CookingRawXPRequest`
  — do NOT define a second request type for this event (two types for one
  event name silently never match). `cooking` importing `fishing` is
  one-directional (fishing does not import cooking); no cycle. Set `XP` and
  `HasXP = true` on a hit. After this lands, update the fishing
  infernal-harpoon test's no-answerer premise ONLY if the test breaks — it
  builds its own harness without Cooking registered, so it should still pass;
  run the full suite and confirm.
- `pauseCooking`: `pauseForLevelUp(event, activeSessions, stopCooking)`.
  Go signature (`internal/plugins/skillactions/skillactions.go:81`):
  `PauseForLevelUp(event *LevelUpPopupOpenedPayload, sessions
  map[*game.Player]Session, stop func(*game.Player), onResume
  func(Session))`. Follow Fletching's call shape
  (`internal/content/fletching/shared.go:1385`): sessions must implement
  `skillactions.Session` (pointer identity matters).
- `stopOnDisconnect`, `ensureCookTask` (global 1-tick `game.Task`,
  submitted once).
- `attachCookers` registrations, in order: `persistAttribute(
  "cooking:success-count", 0)`; `onPlayerLogin(sendSuccessCount)`;
  `onPlayerDisconnect(stopOnDisconnect)`; `onCustomEvent(
  "level-up:popup-opened", pauseCooking)`; `onCustomEvent("cooking:count",
  countCookEvent)`; `onItemOnObject(handleCook, {noted: false})`;
  `onItemOnObject(refuseCook, {noted: false, fallback: true})`; 11×
  `onObjectInteraction` Cook; `onCustomEvent("cooking:raw-xp",
  answerRawXp)`; `onCustomEvent("cooking:cooked", answerCooked)`.

### 1.5 Wine.Cooking.js (234 lines)

- `MAKE_WINE_ANIMATION = "human_make_wine"`,
  `MAKE_WINE_SPOTANIM = "cooking_make_wine_spotanim"`, `MAKE_WINE_SOUND =
  2489`, delay 2 ticks. `FERMENT_MESSAGE = "You squeeze the grapes into
  the jug. The wine begins to ferment."` (once, when the last jug of a batch
  is made). `WINE_TIMING = [2, 2]`. `FERMENT_TICKS = 20`.
  `FERMENT_BATCHES = 15`. `WATER = "jug_water"`.
- `wineFor(first, second)`: one side must be jug water, the other a
  `grapesId` from `wineData()`.
- `wineRecipe(wine)`: product = unfermented, xp 0, ingredients grapes 1 +
  jug water 1, no returns, timing [2, 2].
- `playMakeWine`: anim + graphic + sound (delay 2).
- `makeWine` (`onItemOnItem`, `{noted: false}`): `Recipes.offer(player,
  wineRecipe, {cycle: playMakeWine, canMake: pendingWine < pendingLimit
  (when the wine has one), made: queueFerment, done: spam
  FERMENT_MESSAGE})`.
- `successChance(player, wine)`: level capped at 99; the Common interp
  formula on `wine.chance`.
- `pendingWine`: inventory amount + all bank tabs (skip
  `BankSearchTabIndex`, Go: `game.BankSearchTabIndex = 10`; tabs via
  `Player.Bank(tab)`, `BankCount()` 11).
- `successes(count, chance, random)`: `batch = floor(count / 15)`; 15 rolls
  each worth `batch`; plus one roll per leftover jug.
- `bankFits` / `fermentBanked`: banked jugs become wine/bad wine; a new
  output stack takes the unfermented stack's own slot first; outputs join
  existing stacks; one refresh. Bank surface needed: per-tab `getAmount`,
  `getItems`, `setItem`, `add`, `getAddableAmount`, `sortItems`,
  `refreshItems`, free-slot count. `Bank` embeds `*itemContainer`
  (`internal/game/bank.go:107`) — verify the exact method names against
  the Container interface at implementation time.
- `fermentWine(player, wine)`: carried + banked; if the bank can't fit,
  banked stays unfermented for the next timer (recompute with carried
  only); carried jugs convert in place slot by slot; XP `good * wine.xp`
  (only when good > 0); `skills:produced {kind: "ferment"}`.
- `queueFerment(player)`: `fermenting.set(player, 20)`; one global 1-tick
  task; `fermentTick` decrements, unregistered players are dropped,
  `ferment(player)` at 0. `resumeFermenting` on login (any unfermented jug
  carried or banked → `queueFerment`). `forget` on disconnect.
- Registrations: `onItemOnItem(makeWine, {noted: false})`,
  `onPlayerLogin(resumeFermenting)`, `onPlayerDisconnect(forget)`.

### 1.6 Churn.Cooking.js (181 lines)

- `MENU_TITLE = "What would you like to make?"`, `MENU_MODE = 0`,
  `NOTHING_TO_CHURN_MESSAGE = "You have nothing to churn."`
- `churnData()`: inputs (id, returnId or null), products (input, productId,
  seqId, level, xp, ticks, name), `productOrder` from the labels keys,
  `labels` map. Resolved by gameval at load.
- `choices(player, onlyInput)`: carried inputs (or only the used one),
  most-processed first (butter before cream before milk — i.e. product
  order), moving to the next input when one runs out. Returns product →
  rows.
- `churnOne(player, row)`: product takes the input's slot; the milk's
  bucket takes a free slot, else the product drops to the ground via
  `ItemOnGroundManager.registers` and the bucket takes the slot. Go: no
  `forceAdd` exists (Phase 35 gap, documented); use the ground-item seam
  (`game.Hooks.DropGroundItem`, the Woodcutting precedent — verify the
  exact name). Spam `You churn your {input name} to make {name}.` (only the
  butter line is captured; the rest follow it — "Ours"). addXp, `skills:produced
  {kind: "churn"}`.
- `start(player, rows, amount)`: `TaskManager.cancelTasks(player index)`;
  first item comes `row.ticks - 1` ticks after the choice; animation plays
  on the choice and again with each item that has another after it; each
  next item a full `row.ticks` after; `actionsPerCycle(player, "make")`.
- `hasLevel`: mesbox `You need a Cooking level of {level} to make
  {article+name}.` (same line as the item-on-item recipes).
- `offer(player, onlyInput)`: empty → `You have nothing to churn.`
  (plain message); menu with product ids, `labels` as the menu labels,
  `maxAmount`/`lastAmount` = most carried; execute → `hasLevel` →
  `start(min(amount, carried))`.
- `isChurn`: object definition name == "Dairy churn". Churning is
  members-only: on a free world, Churn gets the members' item line
  (`FreeToPlay.ITEM_MESSAGE`, same string as §1.3).
- Registrations: `onObjectInteraction("Dairy churn", {Churn: churnClick})`,
  `onItemOnObject(useOnChurn, {noted: false})` (only when the item is a
  churn input AND a members world; `handled`, `offer(player, itemId)`).

### 1.7 SacredEel.Cooking.js (54 lines)

- `MIN_LEVEL = 72`. Tables: eel `SACRED_EEL`, scales `ZULRAHS_SCALES`,
  knives = `Common.knives()`.
- `dissect` (`onItemOnItem`, `{noted: false}`): knife+eel in either order;
  members world only (silent return otherwise); `handled`; level < 72 →
  `You need a Cooking level of 72 to dissect sacred eels.`; band =
  `min(4, floor((level - 72) / 8))`; scales = `3 + band + floor(random *
  3)` (3–9); xp = `100 + 3 * scales`; delete 1 eel, add scales, addXp;
  emit `cooking:sacred-eel-dissected {player, itemId: eel, scales}`; emit
  `skills:produced {kind: "sacred-eel"}`. No capture exists for a message
  or animation — implement none, and say so.

### 1.8 LeapingFish.Cooking.js (102 lines)

- `LAST_CUT_TICK_ATTRIBUTE = "cooking:leaping-fish-cut-tick"`,
  `AUTO_CUT_TICKS = 3`.
- `FISH` (by gameval name): `brut_spawning_trout` → roe `brut_roe`,
  per 150, offcuts [1,2], xp 10; `brut_spawning_salmon` → roe, per 80,
  offcuts [3,4], xp 10; `brut_sturgeon` → `brut_caviar`, per 80, offcuts
  [5,6], xp 15.
- `roeChance(fish, level) = min(1, max(1, level) / fish.per)`.
- `cutOne(player, fish)`: needs the fish + a knife still carried (dropping
  the knife does NOT stop automatic cutting — the check is at cut time);
  sets the last-cut-tick attribute to the current process cycle (Go: ground
  the current tick — Woodcutting keeps its own tick counter in its 1-tick
  task; `Engine.TickCount()` exists; verify the right source at
  implementation time); delete 1 fish; roe roll misses → return true (fish
  consumed, nothing given); roe/caviar added; offcuts roll → `forceAdd`
  `brut_fish_cuts` — **no Go `forceAdd`** (Phase 35 gap): try inventory
  Add, else ground-drop via the DropGroundItem seam; document the
  deviation; addXp; `skills:produced {kind: "leaping-fish"}`.
- `autoCut`: every 3 ticks until no fish left; a walk or click stops it
  (via `Common.run`'s keyed task).
- `cut` (`onItemOnItem`, `{noted: false}`): knife+fish either order;
  members world only (silent return); `handled`; same-tick guard (attribute
  == current cycle → return); `TaskManager.cancelTasks(player index)`;
  `cutOne` at once, then `autoCut` while fish remain. Only one fish is cut
  per tick even when the knife is re-used.

### 1.9 Gnome.Cooking.js (267 lines) — the riskiest unit

A synchronous custom interface (`aluft_cooking`), NOT a CreationMenu. It
needs Go interface-surface checks BEFORE implementation — if the Go
`Player`/`PacketSender` surface lacks any of these, that is a new seam to
build grounded in TS `PacketSender`, not a reason to stub the unit.

- Constants: `MENU = "aluft_cooking"`, `MENU_TYPE =
  "aluft_cooking_type"`, `CHOSEN = "gnome_chosen_cuisine"`, `OP1 = 1 << 1`;
  `MIXED_DRAGON_SHAKER = 9574`, `POURED = 9575`, `GARNISHED = 9576`.
- `COCKTAILS` (5): selectors 0–4; levels 6/8/18/20/32; mixed ids
  9568/9569/9566/9567/9574; products `fruit_blast`, `pineapple_punch`,
  `wizard_blizzard`, `sgg`, `drunk_dragon`; mix lists (gameval names +
  counts); finish lists; finishXp 20/40/80/90 (dragon: no finishXp — pour
  gives 0, garnish gives 0; the mixed dragon itself is xp 0 at creation —
  see `createCuisine` XP below). Dragon has `poured: 9575`.
- `BATTA` (indices 1 and 4 used): selector 8 level 26 result 9482
  (equa_leaves, toads_legs, cheese; spice); selector 11 level 29 result
  9483 (tomato, cheese).
- `CRUNCHY` (indices 0 and 3 used): selector 16 level 10 result 9581
  (toads_legs ×2; spice); selector 19 level 16 result 9577 (chocolate_bar
  ×2; spice).
- `MENUS = [COCKTAILS + [null, null], BATTA, CRUNCHY]`.
- `FINISH` (3): 9582 + equa_leaves → `toad_crunchies` xp 40; 9578 +
  chocolate_dust → `chocchip_crunchies` xp 46; 9484 + equa_leaves →
  `cheese+tom_batta` xp 88.
- `plan(player, inputs, outputs, preferred)`: inputs must be plain
  non-stackable single items (not preset/untradeable/unbankable); preferred
  slots first; output capacity planned before any mutation
  (`freeSlots + slots.length >= needed`); returns null when unplannable.
  `apply(player, transaction, product, xp, kind)`: deleteAtSlot per input
  slot, add outputs, refresh, addXp, `skills:produced {kind: "make"}`
  (kind null → no emit).
- `ui()`: interface id `aluft_cooking`, component `cuisine_clickzone`,
  component `confirmation`, varbits `aluft_cooking_type` +
  `gnome_chosen_cuisine`, component `toplevel_osrs_stretch:mainmodal` —
  ALL resolved by cache gameval; throws when any is missing. Verify each
  resolves against the real rev-242 cache in the load-prove test.
- `valid(player, session)`: members world, registered, HP > 0, current
  interface id == group, sub-interface at main == session.mount, the slot
  still holds the same item, item id == source, item plain.
- `openMenu(event, type)`: members, not a bot (`player.isPlayerBot()` —
  Go: `combatant.IsPlayerBot()` exists), plain item, slot holds the item;
  send varbit type + chosen default `[0, 7, 16][type]`; `setInterfaceId`
  (group); `mountSubInterface(main, group, 0)`; send interface flags range
  on the clickzone (0..len-1, OP1). Two synchronous-replacement guards
  (interface-opened hook may have replaced the mount — return without
  storing a session).
- `selectCuisine` (interface click on the clickzone component, action 1):
  `session.selected = MENUS[type][slot] ?? null`; a null clears server
  state; a selection sends varbit chosen = selector.
- `createCuisine` (interface click on the confirmation component, action
  1): level gate (silent return); spice gate (`gnome_spice` ≥ 1 when
  `recipe.spice`); plan with `[[source, 1], ...ingredients]` → `[[product,
  1]]`; apply with XP 30 (cocktails) / 40 (batta) / 30 (crunchy), kind
  "make"; close the mount (`closeSubInterfaceMount`, clear interface id
  when nothing is mounted).
- `pour` (`onItemAction` "Mixed blast"/"Mixed punch"/"Mixed blizzard"/
  "Mixed sgg"/"Mixed dragon" `{Pour: ...}`): product = `poured ?? product`;
  plan `[[mixed, 1], [cocktail_glass_empty, 1], ...finish]` →
  `[[cocktail_shaker, 1], [product, 1]]`; apply with `finishXp ?? 0`.
- `garnishDragon` (`onItemAction` "Mixed dragon" `{"Add-ingreds": ...}`):
  9575 + `pineapple_chunks` + `pot_of_cream` → 9576, xp 0, kind "make".
- `emptyBurnt` (`onItemAction` "Burnt crunchies" `{"Empty Tray": ...}` /
  "Burnt batta" `{"Empty Tin": ...}`): burnt → `crunchy_tray` /
  `batta_tin`, xp 0, kind null (no `skills:produced` emit).
- `interfaceClosed` (`interface:closed` custom event — Go:
  `game.InterfaceClosedEventName`): only when `interfaceId == group`;
  drop the session when the mount no longer matches.
- `clearPlayer` on logout / disconnect / death.
- `FRUIT`: pineapple → Dice `pineapple_chunks` ×1 / Slice
  `pineapple_ring` ×4; lemon → `lemon_chunks` ×1 / `lemon_slices` ×1;
  orange → `orange_chunks` ×1 / `orange_slices` ×1; lime → `lime_chunks`
  ×1 / `lime_slices` ×1.
- `chooseCut(player, token, output, count)`: token must still match (the
  cuts map holds one token per player); plan fruit → output; apply xp 0.
- `useItems` (`onItemOnItem`, `{noted: false}`): `gianne_dough` +
  `crunchy_tray` → `raw_crunchies` xp 0; `gianne_dough` + `batta_tin` →
  `raw_batta` xp 0; FINISH pairs → product + xp; knife + fruit →
  `cuts.set(player, token)` + `sendMultiChatboxPrompt(player, "How would
  you like to cut the fruit?", "Dice", fn, "Slice", fn)` (Go:
  `Api.SendMultiChatboxPrompt(player, title, pairs ...any) bool` — pairs
  are text/callback alternates; false → drop the token).
- `interfaceAction` (`onInterfaceActionClick` — Go: firable,
  `Api.OnInterfaceActionClick`, event has `ButtonID`, `Action`,
  `Slot`/`HasSlot`, `GroupID`, `Handled`): ignore when `cachedUi` is nil
  or `(buttonId >>> 16) !== group`; click component → `selectCuisine`;
  create component → `createCuisine`.
- Registrations: 10× `onItemAction` (Cocktail shaker Mix-cocktail; Half
  baked batta Prepare; Half baked crunchy Prepare; Mixed blast/punch/
  blizzard/sgg/dragon Pour; Mixed dragon Add-ingreds; Burnt crunchies
  Empty Tray; Burnt batta Empty Tin); `onItemOnItem(useItems, {noted:
  false})`; `onInterfaceActionClick(interfaceAction)`;
  `onCustomEvent("interface:closed", interfaceClosed)`; `onPlayerLogout`,
  `onPlayerDisconnect`, `onPlayerDeath` (all → clearPlayer).

## 2. Go seams — what exists (verified) and what to check first

EXISTS — reuse, do not rebuild:
- CreationMenu seam (Phase 34): `game.CreationMenu`,
  `Session.SendCreationMenu`, `CloseCreationMenu`, click routing in
  `internal/listeners/npc.go:934` (item choice → menu `execute`).
- `onItemOnItem` / `OnItemOnItemNamed` (Phase 33), `onItemAction` /
  `OnItemActionNamed` / `OnItemFirstAction` / `OnCanUseItem` (Phase 32),
  `onItemOnObject` / `OnItemOnObjectNamed` (firable; check the `fallback`
  semantics for `refuseCook` — Phase 33 documented the item-on-item claim
  rule, verify the object variant the same way before relying on it),
  `onObjectInteraction` / `OnObjectInteractionNamed` (firable),
  `onNpcInteraction` (Phase 35 — NOT needed here; Cooking uses object
  interaction, not NPC interaction), `onPlayerLogin` / `onPlayerDisconnect`
  / `onPlayerDeath` / `onPlayerLogout` (`internal/plugins/api.go`),
  `onCustomEvent` / `EmitCustomEvent` (`internal/plugins/events.go:32,47`;
  `Api.EmitCustomEvent(name, payload any)`).
- `OnInterfaceActionClick` (`internal/plugins/interfaces.go:144`) with
  `InterfaceActionClickEvent{Player, ButtonID, Action, Slot, HasSlot,
  GroupID, HasGroupID, ChildID, ItemID, OpID, Option, Handled, ...}`.
- `Api.SendMultiChatboxPrompt(player, title, pairs ...any) bool`
  (`internal/plugins/dialogue.go:425`).
- `interface:closed` custom event (`internal/game/interface_state.go`).
- `game.SoundCookingCook` (id 2577, `internal/game/sounds.go:105`).
- `game.SkillCooking` (`internal/game/skills.go:176`).
- `game.SkillcapeHasPerkEventName` / `SkillcapePerkRequest`
  (`internal/game/equipment_effects.go:230-239`) — reuse, no answerer.
- `game.BankSearchTabIndex = 10`; `Player.Bank(tab)`, `Player.BankCount()`
  (11); `Bank` embeds `*itemContainer` (Container interface).
- `game.TaskManager.Schedule`, `game.NewTask(1, func(cycle int64) bool)`;
  `Api.GetTaskManager()` (`internal/plugins/skills.go:383`) — resolve
  LAZILY (Phase 32 hub-contract lesson: never capture TaskManager or any
  hub surface at Register time; it installs after Load).
- `skillactions.PauseForLevelUp(event, sessions, stop, onResume)` and
  `skillactions.ActionInterval(emit, player, skill, kind, ticks, saving)`
  (`internal/plugins/skillactions/skillactions.go:81,134`); sessions must
  implement `skillactions.Session` (pointer identity matters — follow
  Fletching `internal/content/fletching/shared.go:1385`).
- `Api.PersistAttribute` (`internal/plugins/api.go:304`).
- Ground-item seam: check the `game.Hooks.DropGroundItem` name against the
  Woodcutting usage before using it for Churn / LeapingFish offcuts.
- `combatant.IsPlayerBot()` exists (for Gnome's `alive` check — verify the
  `*game.Player` accessor at implementation time).

CHECK FIRST (do not assume):
- `OnItemOnObject` fallback semantics for `refuseCook` (see §1.4).
- Gnome's full interface surface: `Player` interface-id get/set,
  `PacketSender` sub-interface mount/get/close, `sendInterfaceFlagsRange`,
  `sendVarbit` — verify every method against `internal/session` +
  `internal/game` before writing Gnome; a missing method is a new seam to
  build grounded in TS `PacketSender`, documented as such.
- `ItemDefinition` cache access for names / `isMembers` / `isStackable`:
  follow the Fletching/Woodcutting loader pattern (`defs.GetItem(id)` —
  verify the exact Store API; `internal/definition/item.go`).
- `ObjectIdentifiers.CLAY_OVEN_3` / `COOKING_RANGE`: resolve by cache
  gameval at load (check how Woodcutting/Fishing resolve object gamevals).
- The fishing infernal-harpoon test premise
  (`internal/content/fishing/infernalharpoon_test.go:85-118`): it builds a
  harness WITHOUT Cooking registered; confirm it still passes with Cooking
  present (it should — run the full suite).

## 3. Work split (3 workers, in parallel, against this plan)

Shared contract — ALL workers follow it, no exceptions:
- Package `internal/content/cooking`. Files: `tables.go` (+ `tables_test.go`),
  `common.go`, `cooking.go` (struct + `Register`), `cookers.go`,
  `recipes.go`, `wine.go`, `churn.go`, `sacred_eel.go`, `leaping_fish.go`,
  `gnome.go`, plus per-file `_test.go` files.
- `Tables` type in `tables.go`: `CookablesByRaw map[int]*Cookable`,
  `RecipesByIngredient map[int][]*Recipe`, `RecipesByProduct
  map[int]*Recipe`, `Doughs *DoughData`, `Wines []*Wine`,
  `Churn *ChurnData`. `Load(data []byte, r IDResolver) (*Tables, error)`,
  `LoadFromCache(defs *cache.CacheDefinitions) (*Tables, error)`,
  `DefaultPath = "data/definitions/cooking.json"`. `IDResolver` follows the
  Fletching loader's resolver shape (gameval-name → id, fail-closed with a
  0-entry fallback table only where the cache genuinely mis-resolves, like
  Fletching's `item_ids.go` — document every fallback entry with its reason).
  First-wins rule for duplicate raw ids (Common.Cooking.js:39-41). Every
  table row cites its JSON source (`cooking.json` section + gameval names).
- `cooking` struct (in `cooking.go`): holds `tables *Tables`, `emit
  func(name string, payload any)`, lazily-resolved `tasks
  *game.TaskManager`, and the members-world flag from `worldDef` (follow the
  Woodcutting/Fletching struct + wiring pattern). `Register(api
  *plugins.Api)` wires all units. Wiring in `cmd/server/cookingwiring.go`
  (follow `fletchingwiring.go`): load tables from cache BEFORE hub Load,
  fail boot on load failure, blank import in `pluginwiring.go`.
- Shared request/event types live in the cooking package (one home, no
  duplicates): `LevelBoostRequest`, `ActionsPerCycleRequest`,
  `DiaryCompleteRequest`, `QuestStateRequest`, `RumoursCompletedRequest`,
  `BurnRequest`, `BurntEvent`, `SuccessEvent`, `ProducedEvent`,
  `CookedRequest`, `CountEvent`, `ObtainedEvent`, `SacredEelEvent`. EXCEPT:
  `cooking:raw-xp` uses `*fishing.CookingRawXPRequest` (see §1.4 — do NOT
  redefine it), and `skillcape:has-perk` uses the `game` package types.
- Unanswered queries (all fail-closed, documented at the emit site):
  `skills:level-boost` → boost 0; `skills:actions-per-cycle` → 1;
  `diary:is-complete` → complete false; `quest:is-complete` /
  `quest:is-started` → false; `hunter:rumours-completed` → completed 0;
  `skillcape:has-perk` → result false (cape never prevents burns).
- XP is granted as integers (the structural XP truncation shared with
  Woodcutting/Fishing — `addXp` takes the JSON xp as given; fractional xp
  like churn's 40.5/22.5/23.5 truncates at the grant site; document it,
  do not invent rounding).

### Worker A — Tables + loader + Common + answerers + Recipes

- `tables.go`: the loader + all table types for `cookables` (122),
  `recipes` (47), `doughs`, `wines` (2), `churn`. Gameval resolution via
  `LoadFromCache`; numeric ids pass through (Common.Cooking.js:30). Load-prove
  against the real rev-242 cache with a temporary test (counts + spot ids by
  display name + every gameval name resolving), then DELETE the test.
- `common.go`: `successChance`, `level`, `actionsPerCycle`,
  `wearsCookingCape`, `knives`, `addXp`, `run` (1-tick keyed task), `spam`,
  `mesbox`, `sendVarbit`, `gameval`, `withArticle`, `FreeToPlayItemMessage`
  (the exact string from §1.3).
- Answerers in `cookers.go` or `common.go` (A owns them): `cooking:raw-xp`
  (type-assert `*fishing.CookingRawXPRequest`, set XP + HasXP) and
  `cooking:cooked` (own `CookedRequest` type, set CookedID + XP). Register
  both in `Register`.
- `recipes.go`: the full Recipes unit (§1.3) — `makeOne`, `start`, `offer`,
  `offerDough`, `waterFor`, `useItemOnItem`, `prepareGnomebowl`, helpers
  (`hasIngredients`, `freedSlots`, `hasRoom`, `makeable`,
  `isMembersRecipe`, `canMake`). CreationMenu mode 0 (reuse Phase 34 seam).

### Worker B — Cookers core loop + wiring

- `cookers.go`: the full Cookers unit (§1.4) — sessions, global 1-tick
  task, `cookTick`/`cookStep`/`cookOne`, menus (mode 6), gates (members,
  gnomeTutorial, Lumbridge range, level, duringQuest, rumours — all
  fail-closed per §1.4), Hosidius bonus (12, never 25 without a diary
  answerer), gauntlets/cape, deliberate burn, karambwan (poison + re-click
  timing), `currentCooker`, `countCook` + `cooking:count` + login resend,
  `pauseCooking`, `stopOnDisconnect`, `ensureCookTask`, `isCooker`,
  `handleCook` / `refuseCook` / `handleCookClick`, `answerCooked` /
  `answerRawXp` (coordinate with A — ONE owner each; B owns the Cookers
  file, A owns the answerer registration — decide in integration, do not
  register twice), `isCookingActive`, `canCookFood`, `COOKABLE_OBJECT_NAMES`
  / `FIRE_OBJECT_NAMES` exports.
- `cooking.go` + `cmd/server/cookingwiring.go` + blank import: B owns the
  struct, `Register`, and boot wiring (tables load before hub Load, fail
  boot on load failure; members-world flag).

### Worker C — Wine + Churn + SacredEel + LeapingFish + Gnome

- `wine.go`: the full Wine unit (§1.5) — `makeWine`, `wineRecipe`,
  `playMakeWine`, `successChance`, `pendingWine`, `successes`,
  `bankFits`, `fermentBanked`, `fermentWine`, `ferment`, `queueFerment`,
  `fermentTick`, `resumeFermenting`, `forget`.
- `churn.go`: the full Churn unit (§1.6) — `churnData`, `choices`,
  `nextRow`, `carriedFor`, `churnMessage`, `churnOne` (ground-drop
  fallback via the DropGroundItem seam), `playChurn`, `start`, `hasLevel`,
  `offer`, `isChurn`, `churnClick`, `useOnChurn`.
- `sacred_eel.go`: the full SacredEel unit (§1.7).
- `leaping_fish.go`: the full LeapingFish unit (§1.8) — ground the current
  tick source and the offcut ground-drop fallback; document both.
- `gnome.go`: the full Gnome unit (§1.9) — verify the interface surface
  FIRST (§2); if a method is missing, build the seam grounded in TS
  `PacketSender` and document it. Do NOT stub Gnome; if the surface cannot
  be grounded in this phase, say so in the report and leave the unit
  documented as NOT STARTED with the exact missing seam named.

## 4. Out of scope

Any other skill; all quest work (Doric's Quest still picked but deferred);
the Woodcutting Guild; gap 14 spellbook residuals; the npc-combat-sounds
game-level seam; the far-WALK bug; the y=3521–3522 map wall; the real
production scrypt-hash check; perf work; any new verbatim data import. No
scope expansion without TS grounding.

## 5. Verification (coordinator, on the final integrated code)

- `go build ./...`, `go vet ./...`, `gofmt -l .`, `CGO_ENABLED=0 go build
  ./...`: all clean.
- `go test ./... -p 1 -count=1 -v`: exact passing-case count incl. subtests,
  0 failures, 0 skips, all packages. `go test -race ./... -p 1 -count=1`:
  clean (re-run if `TestEngineTicksAtRate` flakes; tolerance untouched).
- Load-prove: temporary test against the real rev-242 cache (122 cookables,
  47 recipes, 2 wines, doughs, churn; every gameval name resolves; spot ids
  by display name), then DELETE the test.
- Live smoke (new `/home/hatch/smoke36/` harness, real rev-242 cache, temp
  roots outside the repo, legs singly via a `RUNS` override): 3 runs stable,
  legs for — raw shrimps on a fire (cooked + XP, and a burn observed if the
  roll allows; at minimum one burnt result across the runs or honestly
  unit-covered), multiple raw types on a range (CreationMenu 2046 on the
  wire, choose + cook), grapes + jug of water → unfermented wine → ferment
  after 20 ticks (wine + 200 XP), infernal-harpoon cook firing through the
  new `cooking:raw-xp` answerer if the harness can reach it (charged
  infernal harpoon + a caught fish; otherwise unit-covered + the
  `tryCookFish`-with-answerer test). Describe exactly what was observed
  live; the unobserved legs are unit-covered only, stated plainly.
- No `zz_*` probe files, no `.js` files, no probe references in non-test
  code. No data file imported in this phase (the parent already imported
  cooking.json — verify it is the only new file in `data/definitions/`).

## 6. Docs

- This file: append the Outcome section (what landed, verification, smoke,
  deferred brief, open questions).
- `PORT_STATUS.md`: header (phase-36 numbers), "What was verified"
  (phase-36 bullet), P3 content row (9 of 577), Phase 36 paragraph, new gap
  19 (Cooking residuals), Next modules, data-import paragraph (extend the
  cooking.json entry with loader + load-prove numbers). Hub row only if the
  counts change (expect unchanged — Cooking rides firable families).
- `CONTENT_MIGRATION_PLAN.md`: skills progress paragraph (Phase 36).
- `SKILLS_SURVEY.md`: only if a survey claim proves wrong (note the
  correction). Known survey check: §5 "Cooking | data/cooking.json
  (29,762)" confirmed; §7 hook flags to verify against the full read
  (notably `onInterfaceAction` = Go's firable `OnInterfaceActionClick`).

## 7. Deferred brief (if the scope exceeds one phase)

Land the core loop first (Worker B + Worker A), then Wine/Churn/SacredEel/
LeapingFish (Worker C minus Gnome), then Gnome. If Gnome cannot be grounded
in this phase, it becomes the next phase's brief with the exact missing seam
named. Document anything not landed precisely — no silent stubs.

## 8. Open questions

- None known before implementation. The cooking.json import is settled.
  The Woodcutting Guild import is a separate pending question for Tyler.
  The next skill after Cooking is Tyler's pick.

## 9. Outcome (coordinator, 2026-11-21)

All three workers landed their full scope; nothing was deferred to a
next phase. Gnome grounded with no new seam (the interface surface
it needs already existed). The phase is COMPLETE: the whole Cooking
skill — registrar + Common + Cookers + Recipes + Wine + Churn +
SacredEel + LeapingFish + Gnome — is ported, unit-tested, and
live-verified on the fire, range, and wine legs.

### What landed

- **Worker A — tables + loader + Common + answerers + Recipes**
  (`internal/content/cooking/tables.go`, `common.go`,
  `recipes.go`, `types.go`, `item_ids.go`): loader for the
  verbatim `cooking.json` (122 cookables, 47 recipes, 2 wines,
  4 dough products, churn; first-wins cookables, last-wins
  recipes-by-product, `xp ?? 0` / `timing ?? [1, 2]` defaults);
  `successChance` (interp formula), `cookingLevel`,
  `actionsPerCycle`, `wearsCookingCape`, `knives()`, `addXP`,
  `run`, `spam`, `mesbox`, `sendVarbit`, `sendCreationMenu`;
  the `cooking:raw-xp` answerer (type-asserts to
  `*fishing.CookingRawXPRequest`, sets XP + HasXP — the Phase 35
  infernal-harpoon cook now has an answerer) and the
  `cooking:cooked` answerer (own `CookedRequest` type);
  `item_ids.go` knife fallbacks (KNIFE_2..5, transcribed from
  `ItemIdentifiers.ts`). Recipes: `makeOne`, `startRecipe`
  (timing loop, `timing[min(made, len-1)]` — `Timing` is `[]int`,
  not `[2]int`, because dough timing is `[1, 1, 2]`),
  `offer` / `offerDough` / `doughRecipe` / `waterFor` /
  `useItemOnItem` / `prepareGnomebowl`. 37 Worker A tests.
- **Worker B — Cookers core loop + struct/Register + boot wiring**
  (`cookers.go`, `cooking.go`, `cmd/server/cookingwiring.go`,
  `pluginwiring.go`): sessions map, ONE global 1-tick task,
  `cookTick`/`cookStep`/`cookOne` in exact TS order, CreationMenu
  mode 6, gates in exact TS order, Hosidius bonus (+12, CLAY_OVEN_3
  only — elite 25 never, no diary answerer), gauntlets, cape,
  deliberate burn, karambwan (untrained → poison, XP 80, level
  gate 1, re-click timing 1), `countCook` + `cooking:count` +
  login resend, `pauseCooking`, `stopOnDisconnect`, `isCooker` /
  `handleCook` / `refuseCook` (fallback) / `handleCookClick` (11
  object names). Timing FIRST=2, SECOND=3, INTERVAL=4; sound
  2577. 29 Worker B tests. `OnItemOnObject` fallback claim rule
  verified (main hooks first, then fallbacks, each wrapper
  skipping once Handled — the Phase 33 rule).
- **Worker C — Wine + Churn + SacredEel + LeapingFish + Gnome**
  (`wine.go`, `churn.go`, `sacred_eel.go`, `leaping_fish.go`,
  `gnome.go`, plus the shared `gamevalResolver` /
  `newGamevalResolver` cache adapter in `wine.go`): Wine
  (`wineFor`, `playMakeWine`, `makeWine` via `Recipes.offer`,
  `pendingWine`, `fermentWine` carried + banked, `queueFerment`
  FERMENT_TICKS=20, `resumeFermenting` on login, `forget` on
  disconnect); Churn (`churnData`, `choices`, `churnOne` with
  ground-drop via `game.Hooks.DropGroundItem` when the bucket
  has nowhere to go, `start`, `offer`, `churnClick`,
  `useOnChurn`); SacredEel (MIN_LEVEL 72, band
  `min(4, (level-72)/8)`, scales `3+band+rand(0..2)`, XP
  `100+3×scales`); LeapingFish (trout/salmon/sturgeon rows,
  `roeChance`, `cutOne`, `autoCut` every 3 ticks on
  `Api.Core().Cycle()`); Gnome (COCKTAILS/BATTA/CRUNCHY menus,
  FINISH, FRUIT, `plan`/`apply`, `openMenu`, `selectCuisine`,
  `createCuisine` XP 30/40/30, `pour`, `garnishDragon`,
  `emptyBurnt`, `interfaceClosed`, `chooseCut`, `useItems`
  incl. `SendMultiChatboxPrompt`, `interfaceAction`). 48 Worker C
  tests. Gnome needed no new seam: `GetInterfaceId` /
  `SetInterfaceId`, `MountSubInterface` / `GetSubInterfaceAt` /
  `CloseSubInterfaceMount`, `SendVarbit`,
  `SendInterfaceFlagsRange`, `IsPlayerBot` all exist.

### Coordinator fixes (integration)

- **Boot-blocking gameval bug (found by the load-prove, fixed by
  the coordinator):** `LoadCookerIDsFromCache` resolved
  `poison_karambwan` and `cooking_gauntlets` as cache gamevals,
  and SacredEel resolved `sacred_eel` / `zulrahs_scales` the same
  way — but the rev-242 cache has NO such gamevals (probe
  2026-11-21: all four answer not-found). TS reads all four
  through `core.ItemIds`, not gamevals. The loader errored on
  the first name, which would have failed the boot outright;
  SacredEel would silently never match its eel. Fixed with
  `itemIDConstants` in `item_ids.go` (POISON_KARAMBWAN 3146,
  COOKING_GAUNTLETS 775, SACRED_EEL 13339, ZULRAHS_SCALES 12934,
  transcribed from `ItemIdentifiers.ts:2989/:773/:11785/:11398`,
  display names verified in the load-prove); the loader takes
  the two items from the table and keeps the two anim seqs
  (`human_firecooking` 897, `human_cooking` 896) as gamevals;
  SacredEel tries the gameval first (so tests can inject fixture
  ids) and falls back to the constants (the production path).
  The temporary load-prove test was deleted after the re-run
  went green.
- Churn test identifier renames (`churnTitle` /
  `churnNothingMessage` → the existing `churnMenuTitle` /
  `nothingToChurnMessage`) — Worker B's minimal fix, kept.
- Temporary coordinator debug logging in `cookTick` (added to
  diagnose the first smoke failure, removed before close —
  verified by grep: no `cookdbg` remains).

### Verification (final integrated code)

- `go build ./...`, `go vet ./...`, `gofmt -l .`: clean.
  `CGO_ENABLED=0 go build ./...`: clean. 759 `.go` files, 21
  packages (new `internal/content/cooking`).
- `go test ./... -p 1 -count=1 -v`: 4,648 passing incl.
  subtests, 0 failures, 0 skips (4,534 baseline + 114 new,
  exactly the cooking package: 37 A + 29 B + 48 C).
  `go test -race ./... -p 1 -count=1`: 21/21 packages ok,
  race clean. `TestEngineTicksAtRate` passed in the final
  runs (one flake in an earlier run under load — the known
  pre-existing 5ms timing flake, tolerance untouched).
  Cooking package 5/5 consecutive green runs: the transient
  4-test `cookers_test.go` failure Worker C saw once was not
  reproduced — most likely a mid-edit artifact while B and C
  edited shared files concurrently, not a test defect.
- Load-prove (temporary test, deleted): `LoadFromCache` 122
  cookables; `LoadCookerIDsFromCache` {3146 775 897 896};
  seqs `human_make_wine` 7529, `human_firecooking` 897,
  `human_cooking` 896; spotanim `cooking_make_wine_spotanim`
  47; interface `aluft_cooking` 49; components
  `aluft_cooking:cuisine_clickzone` 3211268,
  `aluft_cooking:confirmation` 3211273,
  `toplevel_osrs_stretch:mainmodal` 10551312; varbits
  `aluft_cooking_type` 698, `gnome_chosen_cuisine` 2517; every
  Gnome menu/finish/fruit name, every leaping-fish name, and
  the four `ItemIds` display names (Poison karambwan,
  Cooking gauntlets, Sacred eel, Zulrah's scales) resolve.
- Hub unchanged: 57 of 108 families firable (61 `Api.On*`
  methods, 4 named overloads). Cooking rides firable
  families only (`OnItemOnObject`, `OnObjectInteractionNamed`,
  `OnItemOnItem`, `OnInterfaceActionClick`, `OnPlayerLogin` /
  `OnPlayerLogout` / `OnPlayerDisconnect` / `OnPlayerDeath`).
- Live smoke (`/home/hatch/smoke36/`, real rev-242 cache, 3
  runs stable, 0 failures): fire leg — 10 raw shrimps on a
  spawned Fire 3769: menu script 2046 + fire anim 897 on the
  wire every run; run 1: 5 cooked + 5 burnt, 150 XP; run 2:
  6 + 4, 180 XP; run 3: 3 + 7, 90 XP (burns observed live in
  all 3 runs; shrimp XP is 30, not 10 — the first smoke
  failure was the harness seeding 10 unstackable shrimps in
  one slot, which the container's non-stackable delete
  correctly clears wholesale, and asserting 10 XP; both
  fixed in the harness, server untouched). Range leg — 10
  shrimps + 10 sardines, left-click a spawned Range 7183
  (NOT 114: 114 is the Lumbridge Castle range, refused
  without Cook's Assistant — fail-closed by design): menu
  2046 + range anim 896 on the wire every run; shrimps
  chosen x5 — run 1: 4 + 1, 120 XP; run 2: 4 + 1, 120 XP;
  run 3: 1 + 4, 30 XP; sardines untouched (10 raw left)
  every run. Wine leg — Cooking 35, 5 grapes + 5 jugs of
  water: make menu 2046 + the ferment message on the wire
  every run; run 1: 2 wines, 400 XP; run 2: 4 wines, 800 XP;
  run 3: 3 wines, 600 XP (200 XP per good jug; the rest went
  bad at the ~60% level-35 chance, as TS predicts). The
  infernal-harpoon cook was NOT observed live (needs a
  harpoon spot + charged-harpoon seeding the harness cannot
  reach); it is unit-covered by `TestTryCookFishFiresAnswered`
  (100 answered attempts fire) + `TestAnswerRawXP`, and the
  `cooking:raw-xp` answerer is live in production. Gnome,
  Churn, SacredEel, LeapingFish, doughs, karambwan, and the
  other 167 recipes are unit-covered only, honestly stated.
- No `zz_*` probe files, no `.js` files, no probe references
  in the repo. No data file imported in this phase (the
  parent imported `cooking.json` before the phase began).

### Gaps (new gap 19, PORT_STATUS.md)

XP integer truncation (structural, shared); unanswered queries
all fail-closed (`skills:level-boost` → 0,
`skills:actions-per-cycle` → 1, `diary:is-complete` → false,
`quest:is-complete` / `quest:is-started` → false,
`hunter:rumours-completed` → 0, `skillcape:has-perk` →
false); Lumbridge Castle range refused without Cook's
Assistant; `duringQuest` / `rumours` foods never cook;
Hosidius bonus 12, never 25; infernal cook never observed
live (unit-covered); no SkillLevelUp emitter; no
ForceMovement check; LeapingFish/Churn offcuts ground-drop
via `DropGroundItem` (no `forceAdd` — the Wiki outcome,
unconfirmed in TS); wine banked leftovers go to the first
tab with a free slot (no Go `GetTabForItem`); Gnome's
`PluginItemActionEvent` carries no Item reference (read from
the inventory slot at handler time); `cooking:cooked` has no
Go emitter.

### Open questions

- None for this phase. The Woodcutting Guild import
  (`woodcutting-guild.json`) is still unanswered by Tyler —
  not folded in. The next skill after Cooking is Tyler's pick.
