# Phase 44 — Crafting

## Scope

Port the Crafting skill (Tyler's 7th skill pick, 2026-11-21 14:35 PST, after Smithing).
Upstream: `server/plugins/skills/Crafting.plugin.js` (31 lines) + 13 units under
`server/plugins/skills/crafting/` + `SkillActions.js` (already ported, Phase 20) =
15 true files, 2,088 lines, F2P. This is the first full read of Crafting
(SKILLS_SURVEY.md §11: the survey sweep-screened it only).

Units (all under `~/tsps-bfd81e3/server/plugins/skills/`):

| File | Lines | Role |
|---|---|---|
| `Crafting.plugin.js` | 31 | Entry registrar: Shared.attach + 12 unit attaches + `player:interaction-cleared` stop |
| `crafting/Shared.Crafting.js` | 234 | Shared api/core, `startMaking` 1-tick loop, `openMakeMenu` CreationMenu, XP/level helpers |
| `crafting/Leather.Crafting.js` | 354 | Needle+leather sewing, thread, hunters' pouches, combine (studs/claws/hides) |
| `crafting/Gems.Crafting.js` | 194 | Chisel gem cutting (crush), amethyst, `crafting:cut-gem` answerer |
| `crafting/Jewellery.Crafting.js` | 344 | Gold/silver jewellery at furnaces (interfaces 446/6), perfect jewellery, stringing |
| `crafting/Glassblowing.Crafting.js` | 80 | Pipe + molten glass |
| `crafting/Pottery.Crafting.js` | 229 | Soft clay, potter's wheel shaping, pottery oven firing |
| `crafting/Spinning.Crafting.js` | 200 | Spinning wheels + looms; emits `crafting:spun` |
| `crafting/Battlestaves.Crafting.js` | 79 | Charged orb + battlestaff |
| `crafting/Snelms.Crafting.js` | 99 | Chisel on blamish shells |
| `crafting/Limestone.Crafting.js` | 53 | Chisel on limestone, instant carve |
| `crafting/LightOrbs.Crafting.js` | 37 | Empty light orb + cave goblin wire |
| `crafting/Buttons.Crafting.js` | 23 | `onItemAction` Polish buttons |
| `crafting/GemSickles.Crafting.js` | 101 | Gem on silver sickle (b), quest-gated |

Out of scope: **CraftingSpells** (`CraftingSpells.plugin.js`, 346 lines, members,
separate entry, own data file `crafting-spells.json`) — blocked on `onSpellCast`
(Lunar) and `onSpellOnObject` (ChargeOrbs, table-only), plus `inventory.forceAdd`
and `diary:is-complete` (SKILLS_SURVEY.md:883). It is a separate skill entry, not
a Crafting sub-unit; do not expand into it. Quests stay deferred (Doric's Quest
picked but postponed 2026-11-16).

## Grounding — upstream behavior

### Entry router (`Crafting.plugin.js:14-29`)

`Shared.attach(api)` then attaches the 12 units in order: Leather, Gems, Snelms,
Limestone, Buttons, Jewellery, Glassblowing, LightOrbs, Pottery, Spinning,
Battlestaves, GemSickles. Plus `onCustomEvent("player:interaction-cleared",
stopCraftingOnInteractionCleared)` — walking away / interaction cleared stops
the active make-many loop. No other entry hooks.

### Shared (`Shared.Crafting.js`) — the two primitives every unit uses

- `DATA` is `crafting.json` parsed at require time; `itemId(name)` resolves an
  ItemIdentifiers name via `core.ItemIds` and **throws on unknown** — the Go
  loader must resolve every name at load (gameval first, fallback constants
  second; see §"Grounding — data files and cache").
- `craftingLevel(player)`: current level + `skills:level-boost` request (no Go
  answerer — boost stays 0; record as a gap, do not invent).
- `actionsPerCycle(player, kind)` / `actionInterval(player, kind, ticks)`:
  `skills:actions-per-cycle` / `skills:action-interval` queries (no Go
  answerers — 1 action per cycle, interval as tabled; record as gaps).
- `produced(player, itemId, amount, xp, kind, materials)`: emits
  `skills:produced` (no Go consumer — emit anyway, harmless; record as dead end).
- `available(recipes)`: members recipes only on members worlds
  (`core.WorldDefinition.isMembersWorld()`). Go: the port needs the same
  members-world flag the Cooking/Fletching wiring installs before hub Load
  (see §"Grounding — Go seams").
- `spam(player, text)`: `sendTypedMessage(ChatType.SPAM, ...)` — the channel
  live uses for Crafting's action lines.
- `nextTick(player, fn)`: runs `fn` on the tick after the click (single
  item-on-item actions: chisel-on-gem too low, ball of wool on amulet).
- `openMakeMenu(player, {title, mode, items, maxAmount, choose})`: builds a
  `CreationMenu` (title, item ids, labels defaulting to cache item names) and
  sends it. `choose(item, amount)` runs on the choice.
- `startMaking(player, {amount, startTick=0, firstTick, interval, animation,
  sound, onAnimate, has, make, anchor, resetAnimation, kind})` — THE loop
  shape, counted from the tick the player picks the item (Shared header
  comment): a 1-tick `Task`; `stopCrafting` first; `has()` must hold to start;
  the task stops when the player is unregistered, dead, moving, force-moved,
  off the start location, has an interface open, is >2 tiles from `anchor`,
  or `has()` fails. Tick counting: tick N plays the anim+sound at
  `startTick` (and replays it on every make tick except when it equals
  `startTick`); the first item lands at `firstTick`, then one every
  `interval` ticks (`(tick - firstTick) % pace === 0`); each cycle makes
  `actionsPerCycle` items via `make()` until `remaining` hits 0 or `make()`
  returns false. `stop()` clears the `crafting:task` attribute when it is
  this task and optionally resets the animation. The task is stored under
  persistAttribute-adjacent attribute `crafting:task` (not a persisted key —
  it holds a live task, cleared on stop).

### Leather (`Leather.Crafting.js`)

- Needle (or costume needle) on leather/hide, either order
  (`onItemOnItem`, noted:false): opens the skillmulti menu, mode **0**,
  title "How many do you wish to make?", max = leather carried. Items at
  ticks 2, 5, 8, … (firstTick 2, interval 3), anim 1249
  (`human_leather_crafting`), reset on stop, "You make …" (spam, from the
  recipe's `made` field). Above-level items stay in the menu and answer with
  a message box on choice: "You need a Crafting level of X to make …"
  (`StatementDialogue`). No thread → "You need some thread to sew leather."
  (plain message; ponytail: not captured). A reel of thread lasts 5 items;
  the 5th consumes it with "You use up one of your reels of thread." (spam);
  a costume needle needs no thread. State: `persistAttribute(
  "crafting:thread-uses", 0)`. Emits `crafting:made` {player, itemId}.
- Materials (from `crafting.json` `leather`, 9 materials, 32 recipes):
  LEATHER 7, HARD_LEATHER 1, GREEN/BLUE/RED/BLACK_DRAGON_LEATHER 3 each,
  SNAKESKIN 5, XERICIAN_FABRIC 3, HUEYCOATL_HIDE 4. Plain leather F2P except
  the coif (members); all dragon/snakeskin/xerician/hueycoatl members.
- Hunters' pouches: needle on fur, no menu — one pouch, anim at the click,
  pouch + XP at tick 2 (firstTick 2, interval 3, amount 1). No thread →
  message box "You'll need a needle and some thread to craft a small meat
  pouch." (text varies by pouch); upgrade without the base pouch →
  item-box "You need a small meat pouch to upgrade into a large pouch."
  (5 recipes: small/large meat, small/medium/large fur).
- Combine (13 recipes): studs/claws/hide on a finished piece, either order
  (`onItemOnItem`): opens the leather menu with the single recipe, max =
  min plain counts; ticks/anim from the recipe (`ticks` 3, seq 1249, except
  spiky vambraces seq 5243). Pieces count only as **plain** copies
  (`plainAmount`/`takePlain` from `items/PoweredStaff.js`) — a preset piece
  can't come out as a plain tradeable one. Go has no presets, so plain ==
  total; record as a simplification, not a gap. Missing tool/material →
  "You need [a/an] X to make this." (amount named when >1); preset-only →
  the PoweredStaff preset text. Tools: hammer, with Imcando hammer
  (either hand, inventory or wielded weapon/shield slot) as substitute.

### Gems (`Gems.Crafting.js`)

- Chisel (or Jeweller's chisel) on an uncut gem (`onItemOnItem`,
  noted:false): menu mode **12**, title "How many gems would you like to
  cut?", max = gems carried; first cut tick 1, then every 2 ticks, the
  gem's cutting anim (seq from the recipe), sound 2586, "You cut the X."
  (spam). Below level → next-tick item box with the uncut gem: "You need a
  Crafting level of X to cut Y." (plural). 10 recipes: opal 1/15, jade
  13/20, red topaz 16/25, sapphire 20/50, emerald 27/67.5, ruby 34/85,
  diamond 43/107.5, dragonstone 55/137.5, onyx 67/167.5, zenyte 89/200.
  Sapphire/emerald/ruby/diamond are F2P; opal/jade/red topaz/dragonstone/
  onyx/zenyte members.
- Crush: semi-precious gems (opal/jade/red topaz) carry `crush: {low, high,
  xp}`; on a failed roll the gem is smashed: "You mis-hit the chisel and
  smash the X to pieces!" (spam), a crushed gem + crush XP, sound 2589.
  Chance is the OSRS skilling curve
  `(1 + floor(low*(99-lvl)/98 + high*(lvl-1)/98 + 0.5)) / 256` (`cutChance`,
  exported for tests); a Jeweller's chisel adds +0.2 success and has a
  10% chance to cut a second gem at once.
- Amethyst (members): chisel on amethyst → menu mode **0**, title "What
  would you like to make?", 4 products (15 bolt tips / 15 arrowtips /
  5 javelin tips / 8 dart tips, 60 XP each, levels 83/85/87/89), first
  tick 1, every 2, anim 6295 (`human_amethystcutting`). Below level →
  "You need level X Crafting to make Y."
- `crafting:cut-gem` answerer: `{uncutId}` → `{cutId, xp}` (for content
  that cuts as it mines — a league relic; no Go asker exists, but port
  the answerer: cheap, mirrors the `cooking:raw-xp` precedent).

### Jewellery (`Jewellery.Crafting.js`)

- Furnace Smelt (`onObjectInteraction("Furnace"|"Small furnace", Smelt)`):
  when the player carries a gold bar + a ring/necklace/amulet/bracelet
  mould → interface **446** (gold); a silver bar + a silver mould →
  interface **6** (silver); gold first, else silver, else **false** (falls
  through to Smithing's bars menu — see §"Grounding — Go seams" for the
  ordering). Bar on furnace (`onItemOnObject`, noted:false) opens the same
  menus directly. The menus are client-drawn (cache scripts); the server
  only opens the sub-interface (main modal UID `(161<<16)|16`), sets varp
  **2224** (`makexcrafting`, remembered quantity) and, on a product click,
  the last-item varbit (`crafting_gold_item_lasttype` / 
  `crafting_silver_item_lasttype`, enums **4318**/**4319** — probe-verified
  below: 33 / 20 entries, varbits 13892 / 13893). Quantity buttons come
  from the JSON `quantities` tables (gold components 60–64 = 1/5/10/x/28;
  silver 32–36); "x" asks "How many would you like to make?" (entered
  amount, cap 28). Choosing a product closes the menu, plays anim 899
  (`human_furnace`) + sound 2725, first item 2 ticks later, then every 3
  ticks, no message. Gold products: 4 types × 8 gem slots (plain + 7
  gems), level/XP per product (gold ring 5/15 … zenyte ring 89/150); the
  gem-set product is made when the matching cut gem is carried, else the
  plain piece; bar + gem consumed, mould kept. Silver: 20 products by
  component (opal ring 1/10 … silvthrill rod etc.), mould + `extra` (cut
  gem) consumed. Level message: "You need a Crafting level of at least X
  to craft this." Slayer ring (component 16 on 446, level 75, 15 XP):
  needs the `slayer_ring_unlocked` varbit (=1, probe: varbit 3207) else
  "You need to unlock the ability to craft slayer rings from a Slayer
  master."; eternal gem → eternal ring, else enchanted-gem ring.
  Inventory-space preview: after deleting inputs, the output must fit
  (`getAddableAmount`); else `inventory.full()` and nothing happens.
  Emits `crafting:made` {player, itemId, furnace:true}.
- Perfect jewellery: 'perfect' gold bar + ruby + mould on a furnace
  (`onItemOnObject`): one mould carried → makes it at once ("You craft
  a 'perfect' ring." / necklace, 40/70 or 40/75); both moulds →
  `sendMultiChatboxPrompt` "What would you like to make?" — **no Go
  seam** (same gap as Smithing's Dismantle, gap 26h): defer the
  two-mould choice, record as a gap; single-mould works.
- Stringing amulets (`onItemOnItem`, either order): ball of wool (or the
  recipe's material) on an unstrung amulet/symbol/emblem → next tick,
  "You put some string on your amulet." (spam; noun from the pair),
  4 XP, no animation. Pairs from `strungAmulets` (default material
  BALL_OF_WOOL).

### Glassblowing (`Glassblowing.Crafting.js`)

- Glassblowing pipe on molten glass, either order (`onItemOnItem`,
  noted:false), members-only: menu mode **0**, "How many do you wish to
  make?", all 8 items whatever the player's level; above level →
  "You need level X Crafting to make a/an Y." and nothing is made. First
  item tick 2, then every 3, anim 884 (`human_glassblowing`) + sound 2724
  (delay 10), "You make a beer glass." (spam; `made` field or
  withArticle(name)). 8 recipes: beer glass 1/17.5 … unpowered orb 87/52.5
  (light orb uses the LightOrbs unit, not this one).

### Pottery (`Pottery.Crafting.js`)

- Water container on clay (`onItemOnItem`, either order): clay + bucket/
  bowl/jug/vial of water → soft clay + empty container, spam "You mix
  the clay and water.<br>You now have some soft workable clay." (the
  `<br>` is in the TS string; send it as-is — check how the Go chat
  path renders it and record if it differs).
- Potter's wheel Use, or soft clay on a wheel (`onObjectInteraction`
  "Potter's Wheel"/"Potter's wheel" Use; `onItemOnObject`): shape menu
  mode **0**, "How many do you wish to make?". Shaping: result at ticks
  2, 5, 8, … (interval 3); one soft clay shapes `shaped ?? 1` pieces
  (cups: 4). Firing: pottery oven Fire, or unfired piece on an oven
  (`onObjectInteraction` "Pottery Oven"/"Pottery oven" Fire): fire menu
  mode **9**, "What would you like to fire in the oven?"; put at ticks
  0, 7, 14, … and remove at 6, 13, 20, … (interval 7, delay 6); firing
  has **no** level requirement. Crack chance is the firing curve
  `min(1, (1 + floor(180*(99-lvl)/98 + 800*(lvl-1)/98 + 0.5)) / 256)`
  (guaranteed from level 13); a cracked piece is destroyed with no
  product, no XP, no message (except cups, `cracks: false`). Pot lids
  are locked behind One Small Favour (`unlock: {quest:
  "one_small_favour", stage: 34}`) via `quest:get-stage` — no Go
  answerer, so **fail-closed: locked**; record as a gap. Emits
  `pottery:fired` {player, objectId, itemId} on a successful firing.
  Station anchor: within 2 tiles of the wheel/oven. Recipes: pot 1
  (6.3/6.3), pie dish 7 (15/10), bowl 8 (18/15), plant pot 19 (20/17.5),
  cup 3 (8.5/8.5, ×4), pot lid 25 (20/20, quest-locked). Pottery uses
  its own task shape (not `startMaking`): implement `PotteryTask`
  following the TS class, including the `putMessage`/`shapeMessage`/
  `removeMessage` spam lines from the JSON.

### Spinning (`Spinning.Crafting.js`)

- Spinning wheel Spin (`onObjectInteraction` "Spinning wheel" /
  "Spinning Wheel", Spin) or material on wheel/loom (`onItemOnObject`,
  noted:false): menu mode **16**, title "What would you like to spin?"
  (one product: "How many would you like to spin?"), max = most carried
  material / amount. First item tick 2, then every 3 ticks (2 ticks at
  the Seers' Village wheel (2710,3471,1) for flax with Kandarin medium
  diary complete — `diary:is-complete` has no Go answerer, so **3 ticks
  always**; record as a gap). Anim 13139 (`human_spinningwheel_60`) +
  wheel anim 466 + sound 2590, replayed per item. No success message.
  8 spinning recipes (ball of wool 1/2.5 F2P; flax→bow string 10/15,
  flax→linen yarn 12/16, sinew/roots→crossbow string 10/15, magic
  roots→magic string 19/30, hair→rope 30/25, hemp→hemp yarn 39/60,
  cotton→cotton yarn 73/105 — all but wool members). Loom Weave
  (`onObjectInteraction` "Loom", Weave): menu mode **35**, title "How
  many would you like to weave?", first item tick 3, then every 3,
  anim 2270 (`farming_useloom`) + sound 2587 (delay 2). 7 weaving
  recipes (strip of cloth 10/12 … bolt of cotton 73/132, all members).
- **The bonus**: `makeOne` emits `crafting:spun` {player, itemId,
  amount:1, inputItemIds, inputs, outputItemId, stationObjectId?,
  location?} **after** adding the item and the XP, for spinning only
  (not weaving). The Fletching spool listens (see §"Grounding — Go
  seams"): the Go emit must be `&fletching.CraftingSpunEvent{Player,
  ItemID, Amount: 1}` — same order (item in inventory first, then
  emit), so the spool's inventory check sees the bow string.
- Inventory-space preview: if the output cannot fit after consuming
  the material, `makeOne` returns false (loop stops, no message).

### Battlestaves (`Battlestaves.Crafting.js`)

- Charged orb on a battlestaff, either order (`onItemOnItem`,
  noted:false), members-only: menu mode **0**, "How many do you wish to
  make?", single recipe per orb; first staff tick 3, then every 2,
  anim 7531 (`human_battlestaff_crafting`), no message. 4 recipes
  (water/air/earth/fire orbs; levels/XP in the JSON). Level message
  follows the glassblowing form.

### Snelms (`Snelms.Crafting.js`)

- Chisel on a blamish snail shell (`onItemOnItem`, noted:false),
  members-only: level 15, 32.5 XP, interval 2 ticks (from
  `snelms.ticks`), first tick 1, anim 890 (`human_opalcutting`, the
  gem-cutting seq reused), "You make a X." (spam). Emits
  `crafting:made`. Below level → next-tick item box with the shell:
  "You need a Crafting level of 15 to craft this." One shell → no menu,
  straight into the loop. 9 recipes (round + pointed snelms; no pointed
  bark variant in the cache).

### Limestone (`Limestone.Crafting.js`)

- Chisel on limestone (`onItemOnItem`, noted:false), members-only: one
  brick per use, **on the tick of the use, no menu, no sound**; anim is
  `human_limestonecutting` (seq by name); "You use the chisel on the
  limestone and carve it into a building block." (spam); 6 XP, level 12;
  below level → "You need a Crafting level of 12 to carve limestone."
  The brick replaces the limestone **in place** (first limestone's slot,
  via `setId` + `refreshItems`). Carving can fail below level 40 on
  live (rock, 1.5 XP) but no source gives the rate or the line, so
  **every carve succeeds** — record as a gap, do not invent a rate.

### LightOrbs (`LightOrbs.Crafting.js`)

- Empty light orb + cave goblin wire (`onItemOnItem`, noted:false),
  members-only: at once, no menu, no animation, no message; level 87,
  104 XP. Emits `crafting:light-orb-made` {player, itemId} (Sherlock's
  Dorgesh-Kaan task listens; no Go consumer).

### Buttons (`Buttons.Crafting.js`)

- `onItemAction("Buttons", {Polish: polish})`, members-only: level 3,
  5 XP, one use polishes the pair **in place** (setId + refreshItems).
  Below level → spam "You rub the buttons on your clothes but they
  aren't improved by the process."; success → spam "You rub the buttons
  on your clothes and they become more shiny."

### GemSickles (`GemSickles.Crafting.js`)

- Gem (emerald/ruby/diamond) on a silver sickle (b)
  (`onItemOnItem(gemName, "Silver sickle (b)")`, noted:false),
  members-only: asks `crafting:gem-sickle-allowed` {player, product,
  allowed} — no Go answerer, so **allowed stays false and the use does
  nothing** (fail-closed); record as a gap. When allowed (future quest
  work): Yes/No prompt "Craft a ruby sickle?" via
  `sendMultiChatboxPrompt` — **no Go seam** (same gap as perfect
  jewellery): defer the whole unit's live path, record as a gap; the
  handler + gate must still be ported so a future answerer + prompt
  seam can switch it on. On Yes: gem + sickle consumed, new sickle,
  gem-cutting anim + sound 2586, XP 20/26/32, item-box dialogue "You
  embed the ruby into the sickle."; emits `crafting:gem-sickle-made`
  {player, product}. No `skills:produced` (quest item).

## Grounding — data files and cache (probe-verified 2026-11-21)

`data/definitions/crafting.json` — 38,296 bytes, SHA-256
`bdeeb5b364c348b9565a425520630cd509a1d1d46aea7447f5ea86b040ae79eb`,
imported verbatim 2026-11-21 on Tyler's approval (widget tap "Import the
Crafting file"). Top-level keys (16 sections): `leather` (9 materials, 32
recipes), `huntersPouches` (5), `combine` (13), `gems` (10), `amethyst`
(4), `glassblowing` (8), `pottery` (6 + `waterContainers` 4 pairs),
`spinning` (8), `weaving` (7), `gold` (`gems` 8, `types` 4, `slayerRing`,
`quantities` 5), `silver` (`products` 20, `quantities` 6), `perfect` (2),
`strungAmulets` (`xp` 4, `pairs`), `battlestaves` (4), `snelms` (9),
`gemSickles` (3). Item keys are `ItemIdentifiers` UPPER_SNAKE names,
resolved at load (a bad name throws in TS — the Go loader must fail the
boot on an unresolvable name that has no fallback, matching
`loadDefinitions`).

Cache probe (temporary test vs real rev-242 cache, since deleted):

- `crafting.json` item names: **271 distinct, 112 resolved / 159 missing**
  via the lowercase-name → `GetObjId` gameval lookup (the Fletching /
  Cooking / Smithing pattern). The 159 are NOT missing items — spot
  checks prove it (`BASKET` = 5376, `BUTTONS` = 688, `SOFT_CLAY` = 1761,
  `LIMESTONE_BRICK` = 3420, `POLISHED_BUTTONS` = 10496, `LIGHT_ORB` =
  10973, `EMPTY_LIGHT_ORB` = 10980, `CAVE_GOBLIN_WIRE` = 10981, all from
  `ItemIdentifiers.ts`): their gamevals simply aren't the lowercase
  name. **Loader decision (Fletching precedent, 176-entry fallback
  table): transcribe the ~159 fallback constants from
  `ItemIdentifiers.ts`, verifying each id's cache display name, and
  record the transcription with line citations in `item_ids.go`.**
  Never fail the boot on a name the fallback covers; fail it on a name
  neither source resolves (with the name in the error).
- Spot checks resolved with correct display names: FLAX→1779 "Flax",
  BOW_STRING→1777 "Bow string", GOLD_BAR→2357 "Gold bar", RING_MOULD→
  1592 "Ring mould", NEEDLE→1733 "Needle", THREAD→1734 "Thread",
  CHISEL→1755 "Chisel", MOLTEN_GLASS→1775 "Molten glass",
  AMETHYST→21347 "Amethyst", LIMESTONE→3211 "Limestone".
- Sequences all present with the right names: 13139
  `human_spinningwheel_60`, 466 `spinningwheel`, 2270
  `farming_useloom`, 6295 `human_amethystcutting`, 890
  `human_opalcutting`, 884 `human_glassblowing`, 7531
  `human_battlestaff_crafting`, 1249 `human_leather_crafting`, 899
  `human_furnace`, pottery wheel 883 `human_potterywheel`, pottery oven
  1317 `potteryoven_quick`. Pottery sounds 2588 / 2725 from the JSON.
- Enums **4318** (33 entries) and **4319** (20 entries) exist — the
  jewellery last-item tables. Varbits resolve:
  `crafting_gold_item_lasttype` → 13892, `crafting_silver_item_lasttype`
  → 13893, `slayer_ring_unlocked` → 3207.

## Grounding — Go seams (verified read-only)

- **CreationMenu** (`internal/game/creationmenu.go:92`):
  `game.NewCreationMenu(title string, items []int, action func(itemID,
  amount int), options CreationMenuOptions)`; sent through the session's
  `CreationMenuSender` (the Fletching `openMenu` pattern,
  `internal/content/fletching/shared.go:1582-1605`; Smithing's
  `smelting.go:440` / `anvil.go:381` / `spears.go:157`). Script 2046
  travels on opcode **110** `WIDGET_RUN_SCRIPT`, NOT 170; menu mount
  must NOT use `SendChatboxInterface` (Phase 34 corrections, verified
  against TS encoders). CreationMenu modes are per-menu (leather 0,
  gems 12, spinning 16, weaving 35, pottery shape 0 / fire 9,
  glassblowing/battlestaves/snelms 0, amethyst 0) — pass them through
  `CreationMenuOptions` exactly as TS passes them to skillmulti.
- **Make-many loop**: no shared Go `startMaking` exists. Smithing's
  `sessions.go` is the closest reusable shape (1-tick task, begin/step,
  firstDelay/finishDelay, stop on move/death/logout, station snapshot);
  Fletching has its own 1-tick task. Crafting's loop differs from both
  in its tick counting (startTick/firstTick/interval with the anim
  replayed per make tick) and its stop set (interface open, force
  movement, anchor >2 tiles, start-location change). Implement a
  Crafting-local task in `shared.go` following `Shared.startMaking`
  exactly; do NOT bend Smithing's sessions to fit it. Pottery needs
  its own task shape (interval 7 / delay 6 firing, interval 3 shaping).
- **Hub families** (59/108 firable after Phase 43): all Crafting hooks
  are already firable — `OnObjectInteraction` / `OnObjectInteractionNamed`,
  `OnItemOnObject` / `OnItemOnObjectNamed`, `OnItemOnItem` /
  `OnItemOnItemNamed`, `OnItemAction` (named form carries `Slot`,
  `internal/plugins/itemaction.go:87`), `OnInterfaceActionClick`
  (Phase 43: the Button form; Crafting needs Click for interfaces
  446/6), `OnCustomEvent` / `Api.EmitCustomEvent` (`internal/plugins/
  events.go:32`, `api.go:249`). **No new hub family in this phase; the
  hub count stays 59/108.**
- **First-handled-wins ordering** (`internal/plugins/objects.go:731`):
  `EmitObjectInteraction` runs named handlers in registration order
  until one marks `Handled`. Crafting's blank import sorts before
  Smithing's (alphabetical, `cmd/server/pluginwiring.go`), so
  Crafting's Furnace/Smelt handler runs first and must leave the
  event unhandled when no jewellery menu applies, letting Smithing's
  bars menu open. Item-on-object runs main hooks then fallbacks,
  skipping once handled (`objects.go:815`); Crafting's bar-on-furnace
  and Smithing's ore-on-furnace handlers match disjoint items.
- **`crafting:spun` consumer already exists**: Fletching's spool
  registers `api.OnCustomEvent("crafting:spun", f.absorbSpun)`
  (`internal/content/fletching/spool.go:58`); `absorbSpun` (`:401`)
  type-asserts to `*fletching.CraftingSpunEvent{Player, ItemID,
  Amount}` and moves `min(amount, capacity-charges, inventory
  bow strings)` onto a carried spool. Precedent for the cross-package
  type: Cooking's `answerRawXP` type-asserts to
  `*fishing.CookingRawXPRequest` (`internal/content/cooking/
  common.go:261`). Crafting emits `&fletching.CraftingSpunEvent{...}`
  — item added + XP granted **before** the emit, matching TS and the
  spool's inventory check.
- **Members world**: Cooking/Fletching install a package-level flag
  before hub Load (`SetMembersWorld`, `internal/content/cooking/
  cooking.go:205-207`); Crafting needs the same in
  `installCraftingTables` (worldDef.MembersWorld), because
  `Shared.available()` filters members recipes at use time.
- **Object-name resolver** (Phase 43 live-found gap, do NOT repeat
  it): the item-on-object router matches by object *name* (furnace,
  wheel, loom, oven), so `installCraftingTables` MUST install
  Crafting's own `SetObjectNameResolver` (smithing's is
  package-local, `internal/content/smithing/smithing.go:88`) from the
  loaded definitions — with a unit test that fails when it is missing.
- **Dialogue**: `game.NewStatementDialogue` /
  `NewItemStatementDialogue` exist (`internal/game/
  dialogue_entries.go:567/658`) for the level/item-box lines.
- **Inventory**: `Items()`, `SetItem(slot, item)`, `Item.SetID()`,
  `RefreshItems()`, `GetAddableAmount(item)`, `GetFreeSlots()`,
  `DeleteNumber`, `AddItem`, `Contains`, `GetAmount`
  (`internal/game/container.go`) — in-place transforms (limestone,
  buttons) are `items[slot].SetID(new)` + `RefreshItems()`; the
  space-preview pattern is a copied-inventory preview
  (`new Inventory(player).SetItems(...)` in TS — the Go equivalent is
  a copy via `SetItems` on a fresh container; check how Smithing or
  Fletching previews and follow it, or port the preview with
  `GetAddableAmount` on a copied container).
- **XP**: grants go through SkillManager with the known integer
  truncation (gaps 16–19/25/26 — e.g. leather gloves 13.8 → 13,
  snelm 32.5 → 32, pot 6.3 → 6). Do NOT change XP math.
- **No Go seam for `sendMultiChatboxPrompt`** (Smithing gap 26h):
  perfect jewellery (two moulds) and gem sickles cannot prompt. Defer
  both prompts; record as gaps. Do NOT build the seam in this phase.
- **Custom events with no Go consumer** (emit anyway, harmless):
  `skills:produced`, `skills:level-boost` (query, boost 0),
  `skills:actions-per-cycle` (1), `skills:action-interval` (tabled),
  `crafting:made`, `pottery:fired`, `crafting:light-orb-made`,
  `crafting:gem-sickle-made`. `crafting:cut-gem` HAS a Go-shaped
  answerer to port (no asker yet). `quest:get-stage`,
  `diary:is-complete`, `crafting:gem-sickle-allowed` have no answerers
  — their gates fail closed as documented above.

## Plan

1. `internal/content/crafting/tables.go` — `CraftingData` (JSON structs
   for all 16 sections) + `Load(data, r IDResolver)` /
   `LoadFile(path, r)` / `LoadFromCache(defs)` (Smithing
   `tables.go:431-462` shape; `IDResolver` is `ObjID` / `EnumValues`
   / `ObjNames` / `InterfaceID`, smithing `tables.go:334`). Resolve
   every ItemIdentifiers name via `ObjID(lowercase)` first, then the
   fallback table; fail the boot (named error) on a name neither
   source resolves. Resolve and store: recipe item/material/tool ids,
   mould ids, enum 4318/4319 values, varbit ids (13892/13893/3207),
   sequence ids (from the JSON `seq` fields + TS constants 1249/899/
   13139/466/2270/6295/890/884/7531/883/1317).
2. `internal/content/crafting/item_ids.go` — the ~159 fallback
   constants transcribed from `ItemIdentifiers.ts` (cite the file line
   per constant group), each verified against the cache display name
   by the loader test. Include at least: SOFT_CLAY 1761, BUTTONS 688,
   POLISHED_BUTTONS 10496, BASKET 5376, LIMESTONE_BRICK 3420,
   LIGHT_ORB 10973, EMPTY_LIGHT_ORB 10980, CAVE_GOBLIN_WIRE 10981.
3. `internal/content/crafting/shared.go` — port of
   `Shared.Crafting.js`: `craftingLevel` (boost 0), `available`
   (members filter), `withArticle`, `spam`, `nextTick`, `openMakeMenu`
   (CreationMenu wrapper), `startMaking` (the 1-tick task, exact tick
   counting + stop set above), `stopCrafting` (`crafting:task`
   attribute), `produced` (emits `skills:produced`), `addCraftingXp`.
4. `internal/content/crafting/crafting.go` — `Plugin` + `attach`:
   `Shared.attach` equivalent, `player:interaction-cleared` stop,
   the 12 unit attaches, `persistAttribute("crafting:thread-uses", 0)`,
   `crafting:cut-gem` answerer. Package `init()` →
   `plugins.Register(Plugin)`; blank import in
   `cmd/server/pluginwiring.go` between `cooking` and `dateminutes`
   (alphabetical first); `cmd/server/craftingwiring.go` with
   `installCraftingTables` (LoadFromCache + SetTables +
   SetObjectNameResolver + SetMembersWorld, before hub Load).
   Update the three `pluginwiring_test.go` load-order expectations:
   measure where Crafting lands in `Loaded()` first (Go init order,
   not import order — Phases 42/43 updated these tests the same way).
5. One file per unit (`leather.go`, `gems.go`, `jewellery.go`,
   `glassblowing.go`, `pottery.go`, `spinning.go`, `battlestaves.go`,
   `snelms.go`, `limestone.go`, `lightorbs.go`, `buttons.go`,
   `gemsickles.go`), each registering exactly the hooks its TS unit
   registers, with TS messages/timings/levels/XP verbatim.
6. `crafting:spun`: `spinning.go` emits
   `&fletching.CraftingSpunEvent{Player, ItemID, Amount: 1}` after
   the item + XP land (import `tsps-go/internal/content/fletching`
   for the type only — the cooking→fishing precedent). Add a unit
   test proving the spool absorbs a spun bow string (seed a spool
   via the fletching package's own test helpers or a direct
   `absorbSpun`-equivalent assertion on the event path; if the
   spool's internals are unexported, test through
   `EmitCustomEvent("crafting:spun", ...)` on a hub with both
   plugins registered and assert the spool charges varbit moves).
7. Jewellery interfaces: open 446/6 as sub-interfaces (main modal UID
   `(161<<16)|16`), varp 2224, last-item varbits via enums 4318/4319;
   handle quantity + product clicks via `OnInterfaceActionClick`;
   verify against the Phase 43 anvil pattern (`internal/content/
   smithing/anvil.go`) and the `OnInterfaceActionClick` seam. The
   client draws the menu from cache scripts — the server must NOT
   invent menu contents; it only opens, tracks quantity, and handles
   clicks. If the click routing needs a seam that does not exist,
   stop and report it rather than inventing one.
8. Pottery: bespoke `PotteryTask` (interval 7 / delay 6 firing,
   interval 3 shaping), station anchor ≤2 tiles, crack roll, quest
   lock (fail-closed), `pottery:fired` emit.
9. Unit tests: loader (all 271 names resolve; enum/varbit/seq ids;
   fallback display names), each unit's menu/level/message/timing
   paths, thread accounting (5 uses consume a reel; costume needle
   exempt), crush curve (`cutChance` boundaries), firing curve
   (level 13 guarantees), spool absorb via `crafting:spun`, furnace
   fall-through (Crafting unhandled → Smithing menu), jewellery
   space-preview refusal, walk-away/member filtering.
10. Live-verify in fresh `/home/hatch/smoke44/` (real rev-242 cache,
    temp root outside the repo; copy target.txt + data/, symlink
    caches; confirm the smoke root's world def is members, as the
    Fletching live legs required). Station coordinates, probe-verified
    2026-11-21 against the real rev-242 cache (temp test in cmd/server,
    since deleted — note: the probe first returned "null" names because
    it built CacheInfo without Game=oldschool; with
    CacheInfoForActive's shape the names decode correctly): Furnace
    24009 "Furnace" at (3226,3256,0) face 1 (Lumbridge; also Falador
    (2976,3368,0), Al Kharid (3272,3185,0)); Potter's Wheel 14887 at
    (3087,3409,0)/(3087,3410,0) face 2 and Pottery Oven 11601 at
    (3084,3407,0) face 3 (Barbarian Village); Spinning wheel 14889 at
    (3081,3430,0) face 3 (Barbarian Village) and (3209,3212,1) face 2
    (Lumbridge Castle 2nd floor), plus Seers' 25824 at (2710,3471,1)
    face 3. No loom was found in the scanned regions (Lumbridge,
    Barbarian Village, Seers', Edgeville, Falador, Al Kharid,
    Rellekka, Varrock) — weaving stays unit-covered unless the live
    worker locates a loom. Legs (run singly with a RUNS
    override if needed): jewellery (gold bar + ring mould on the
    Edgeville furnace → interface 446, gold ring, 15 XP, anim 899,
    sound 2725), spinning (flax on a spinning wheel → menu mode 16,
    bow string, 15 XP each, `crafting:spun` fires and a seeded spool
    absorbs — assert the spool charges varbit moves), gem cutting
    (chisel on uncut sapphire → menu mode 12, cut sapphire, 50 XP),
    leather (needle + thread + leather → gloves, 13 XP — truncation
    evidence — "You make a pair of leather gloves."), pottery
    (soft clay on a wheel → unfired pot), full-inventory refusal,
    walk-away stops. Harness traps: gorilla readErr is sticky
    (deadline-free reader); ClientHandshake after LOGIN; per-mode
    npc-spawns.json in the temp root; eager spawns at 150/tick (~2
    min for the world to fill); station coordinates are in the
    paragraph above (all probe-verified vs the real cache).

## Verification criteria

- `go build ./...`, `go vet ./...`, `gofmt -l .` clean;
  `CGO_ENABLED=0 go build ./...` clean.
- Full suite `-p 1 -count=1 -v`: all packages pass, 0 failures,
  0 skips; report exact counts (baseline 4,863 / 26 packages from
  Phase 43) and the new-test breakdown by area.
- `go test -race ./...`: clean; ANY `DATA RACE` warning is a stop
  and a report, not a flake (Phase 43 fixed the last real one).
  The known `TestEngineTicksAtRate` timing flake alone → re-run
  that package once, and say so.
- Live: every leg above PASS with check counts; sub-units not
  observed live listed as unit-covered only.
- `crafting:spun` → spool absorb proven live (spool varbit moves)
  or, failing that, at unit level with the live gap stated plainly.
- Docs: PORT_STATUS.md (header, "What was verified", data-import
  paragraph completed, P3 content row 15/577, §27 gaps, gap 17(c)
  annotated as closed for `crafting:spun`), PHASE44_PLAN.md Results.
- No stray files, no `.js`, probe tests deleted, `live36` and
  `~/tsps-full` untouched, no commits (parent handles git), no new
  dependencies, no hand-edited JSON.

## Gaps to record (§27, all documented at the port sites, none silent)

(a) XP integer truncation (leather gloves 13.8 → 13, snelm 32.5 → 32,
    pot 6.3 → 6, etc.) — structural, same gap as 16–19/25/26.
(b) No `SkillLevelUp` emitter; `level-up:popup-opened` never emitted
    (session pause dead, as in Smithing).
(c) `skills:level-boost` / `skills:actions-per-cycle` /
    `skills:action-interval` / `skills:produced` unanswered (defaults:
    boost 0, 1 action, tabled interval, emit with no consumer).
(d) Quest gates fail-closed: pot lids (One Small Favour, stage 34)
    locked; gem sickles (`crafting:gem-sickle-allowed`) never allowed.
(e) `sendMultiChatboxPrompt` has no Go seam: perfect jewellery with
    both moulds carried cannot prompt (single mould works); gem
    sickles cannot prompt even if allowed.
(f) `diary:is-complete` unanswered: Seers' Village 2-tick flax spin
    never applies; always 3 ticks.
(g) Limestone carving never fails (live failure below 40 has no known
    rate or line — every carve succeeds).
(h) Combine counts only plain pieces; Go has no presets, so plain ==
    total (simplification, not a divergence in this codebase).
(i) `crafting:made`, `pottery:fired`, `crafting:light-orb-made`,
    `crafting:gem-sickle-made` emitted with no Go consumer.
(j) CraftingSpells NOT STARTED (separate entry, blocked on
    `onSpellCast` / `onSpellOnObject`).
(k) Sub-units unit-covered only, not observed live: (to be filled
    after the live run — everything not covered by a smoke44 leg).

## Results (2026-11-21, verified)

- Foundation (worker A): `internal/content/crafting/tables.go`
  (loader for all 16 `crafting.json` sections — all 271 names
  resolve: 112 gameval + 159 fallback), `item_ids.go` (170
  fallback constants, each display-name-verified),
  `shared.go` (Shared.Crafting.js port — sessions map + one
  global 1-tick task, mining/smithing shape, NOT
  StartSkillable), `crafting.go` (Plugin + cut-gem answerer),
  `types.go`, `cmd/server/craftingwiring.go` (own
  `SetObjectNameResolver` + `SetMembersWorld`, before hub
  Load). 4,868 tests / 27 packages, race clean.
- Units (worker B): all 12 units implemented, 78 new tests.
  **4,946 tests (3,764 top-level + 1,182 subtests), 0 fail,
  0 skip, 27 packages**; race clean (coordinator-verified
  independently — see the PORT_STATUS phase-44 paragraph
  for the race line). **Spool absorb passes** (unit +
  live): spinning flax moves bow strings onto a carried
  spool; `crafting:spun` fires per item, after item + XP
  land. Gap 17(c) CLOSED for `crafting:spun`.
- Corrections found in-phase: the real hub load order is
  `… Fishing, Mining, Woodcutting, Cooking, Crafting,
  Smithing` (worker A mis-placed Crafting; test
  expectations corrected); `sendMultiChatboxPrompt` DOES
  have a Go seam (`Api.SendMultiChatboxPrompt`) — perfect
  jewellery (both moulds) and gem sickles DO prompt, so
  plan gap 27(e) is not a gap; zenyte gem-cut XP is 50 in
  `crafting.json` (OSRS 200) — ported as-is.
- Live (smoke44, 8 legs, 68/71 checks): jewellery 13/13,
  spinning 11/11 (spool absorbed live: varbit 17291 = 5),
  cutting 9/9, leather 9/9 (13.8→13 truncation live),
  pottery 16/16, walk-away 5/5, fall-through 3/3,
  full-inventory 2/5 — the 3 failures are a corrected leg
  premise (1:1 crafts always fit in the freed slot,
  TS-faithful), not a port bug. No port bugs found live.
  Unit-covered only: weaving, battlestaves, snelm,
  limestone, light orbs, buttons, gem sickles, perfect
  jewellery (two-mould prompt), amethyst, glassblowing,
  silver jewellery, combine, hunters' pouches.
- Docs: PORT_STATUS.md (header, "What was verified",
  data-import paragraph completed, P3 row 15/577, §27
  gaps, gap 17(c) annotated). Hub unchanged: 59/108
  firable, 63 methods. CraftingSpells deferred (separate
  entry, blocked on `onSpellCast`/`onSpellOnObject`).
- Residual gaps: §27 (a)–(n) in PORT_STATUS.md.
