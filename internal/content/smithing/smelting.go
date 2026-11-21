// Smelting.Smithing.js: smelting at a furnace — the Smelt
// menu (the shared skillmulti CreationMenu), ore used on a
// furnace, and the smelt loop.
//
// Captured (rsprox, docs/smithing.md): choosing a bar plays
// human_furnace (899) with sound 2725 and the "You smelt ..."
// message on the same tick; the bar, "You retrieve a bar of
// ..." and the XP come 4 ticks later with human_furnace again,
// and the next ore starts on the 5th. Iron fails half the time.
// With no ore at all the furnace says "You don't have any ore
// to smelt." in a message box.
package smithing

import (
	"math"

	"tsps-go/internal/game"
	"tsps-go/internal/plugins"
)

// ---- Smelt menu / loop constants (Smelting.Smithing.js) ----

const (
	smeltMenuMode = 13 // skillmulti Smelt, cache enum 1809
	smeltTitle    = "What would you like to smelt?"
	// smeltMaxQuantity is the skillmulti list's quantity cap.
	smeltMaxQuantity = 28
	// smeltIntervalTicks is the smelt loop interval (5 ticks).
	smeltIntervalTicks = 5
	// smeltFinishTicks is when the bar + XP land (4th tick).
	smeltFinishTicks = 4
	// goldsmithBonusXP is the goldsmith gauntlets / Smithing
	// cape bonus on gold bars (+33.7 XP). The cape half never
	// applies in Go: "skillcape:has-perk" is unanswered
	// (gap 26d); the gauntlets half does.
	goldsmithBonusXP = 33.7
	lovakiteFurnace  = "Lovakite furnace"
	noOreMessage     = "You don't have any ore to smelt."
)

// smeltingSkillmultiOpFlags is the list's op flags (Make
// 1/5/10/X/All, bits 1-5).
const smeltingSkillmultiOpFlags = (1 << 1) | (1 << 2) | (1 << 3) | (1 << 4) | (1 << 5)

// smeltMenuEntry is one Smelt-menu choice: the item shown,
// the most that can be made, and the starter
// (Smelting.Smithing.js openMenu entries).
type smeltMenuEntry struct {
	itemID int
	most   int
	start  func(player *game.Player, amount int)
}

// smeltChoice is a recipe with its ingredients resolved for
// this bar: the catalyst's recipe when the player has its
// ingredients, otherwise the recipe itself
// (Smelting.Smithing.js resolveCatalyst).
type smeltChoice struct {
	recipe      *SmeltRecipe
	ingredients []Ingredient
	catalysed   bool
}

// storedAmount is what a carried container (the coal bag)
// holds of an ingredient ("storage:amount"). Unanswered the
// container holds nothing (gap 26d).
func (s *smithing) storedAmount(player *game.Player, itemID int) int {
	req := &storageAmountRequest{Player: player, ItemID: itemID}
	s.emitEvent("storage:amount", req)
	if req.Amount < 0 {
		return 0
	}
	return req.Amount
}

// available is the inventory's count of an item plus what a
// carried container holds.
func (s *smithing) available(player *game.Player, itemID int) int {
	if player == nil || player.Inventory() == nil {
		return 0
	}
	return player.Inventory().GetAmount(itemID) + s.storedAmount(player, itemID)
}

// hasIngredients reports whether the player covers a choice's
// ingredients (inventory + coal bag).
func (s *smithing) hasIngredients(player *game.Player, choice smeltChoice) bool {
	for _, ing := range choice.ingredients {
		if s.available(player, ing.ItemID) < ing.Amount {
			return false
		}
	}
	return true
}

// withCatalyst is the recipe with a catalyst: half the coal,
// one catalyst, double XP. Unchanged for bars without coal,
// or when the player carries no catalyst
// (Smelting.Smithing.js withCatalyst).
func (s *smithing) withCatalyst(player *game.Player, recipe *SmeltRecipe) smeltChoice {
	plain := smeltChoice{recipe: recipe, ingredients: recipe.Ingredients}
	if player == nil || player.Inventory() == nil {
		return plain
	}
	hasCoal := false
	for _, ing := range recipe.Ingredients {
		if ing.ItemID == coalItemID {
			hasCoal = true
			break
		}
	}
	if !hasCoal || !player.Inventory().Contains(smithingCatalystItemID) {
		return plain
	}
	ingredients := make([]Ingredient, 0, len(recipe.Ingredients)+1)
	for _, ing := range recipe.Ingredients {
		if ing.ItemID == coalItemID {
			ing.Amount = int(math.Ceil(float64(ing.Amount) / 2))
		}
		ingredients = append(ingredients, ing)
	}
	ingredients = append(ingredients, Ingredient{ItemID: smithingCatalystItemID, Amount: 1})
	return smeltChoice{recipe: recipe, ingredients: ingredients, catalysed: true}
}

// resolveCatalyst is the catalyst's recipe when the player has
// its ingredients, otherwise the recipe itself
// (Smelting.Smithing.js resolveCatalyst).
func (s *smithing) resolveCatalyst(player *game.Player, recipe *SmeltRecipe) smeltChoice {
	if recipe == nil {
		return smeltChoice{}
	}
	catalysed := s.withCatalyst(player, recipe)
	if catalysed.catalysed && s.hasIngredients(player, catalysed) {
		return catalysed
	}
	return smeltChoice{recipe: recipe, ingredients: recipe.Ingredients}
}

// batches is how many of a recipe the player's ore (and coal
// bag) covers: the catalysed bars first, then the rest
// (Smelting.Smithing.js batches).
func (s *smithing) batches(player *game.Player, recipe *SmeltRecipe) int {
	if recipe == nil {
		return 0
	}
	covered := func(choice smeltChoice) int {
		most := -1
		for _, ing := range choice.ingredients {
			if ing.Amount <= 0 {
				continue
			}
			if n := s.available(player, ing.ItemID) / ing.Amount; most < 0 || n < most {
				most = n
			}
		}
		if most < 0 {
			return 0
		}
		return most
	}
	catalysed := s.withCatalyst(player, recipe)
	if !catalysed.catalysed {
		return covered(catalysed)
	}
	first := covered(catalysed)
	left := map[int]int{}
	for _, ing := range catalysed.ingredients {
		left[ing.ItemID] = s.available(player, ing.ItemID) - ing.Amount*first
	}
	plain := smeltChoice{recipe: recipe, ingredients: recipe.Ingredients}
	rest := -1
	for _, ing := range plain.ingredients {
		if ing.Amount <= 0 {
			continue
		}
		have, ok := left[ing.ItemID]
		if !ok {
			have = s.available(player, ing.ItemID)
		}
		if n := have / ing.Amount; rest < 0 || n < rest {
			rest = n
		}
	}
	if rest < 0 {
		rest = 0
	}
	return first + rest
}

// consumeIngredients takes a choice's ingredients: the
// inventory's coal goes first, then the bag's ("storage:take")
// (Smelting.Smithing.js consumeIngredients). An unanswered
// storage:take takes nothing (gap 26d).
func (s *smithing) consumeIngredients(player *game.Player, choice smeltChoice) {
	if player == nil || player.Inventory() == nil {
		return
	}
	inv := player.Inventory()
	for _, ing := range choice.ingredients {
		held := ing.Amount
		if have := inv.GetAmount(ing.ItemID); have < held {
			held = have
		}
		if held > 0 {
			inv.DeleteNumber(ing.ItemID, held)
		}
		if ing.Amount > held {
			s.emitEvent("storage:take", &storageTakeRequest{
				Player: player, ItemID: ing.ItemID, Amount: ing.Amount - held,
			})
		}
	}
}

// smeltingXP is a bar's XP: doubled with a catalyst; gold bars
// gain the goldsmith bonus with goldsmith gauntlets worn (or a
// Smithing/max cape — never in Go, gap 26d)
// (Smelting.Smithing.js smeltingXp).
func (s *smithing) smeltingXP(player *game.Player, choice smeltChoice) float64 {
	if choice.recipe == nil {
		return 0
	}
	if choice.catalysed {
		return choice.recipe.XP * 2
	}
	if choice.recipe.BarID != goldBarItemID {
		return choice.recipe.XP
	}
	boosted := false
	if player != nil && player.Equipment() != nil {
		hands := player.Equipment().ItemAt(game.HandsSlot)
		boosted = hands.IsValid() && hands.ID() == goldsmithGauntletsID
	}
	if !boosted {
		req := &game.SkillcapePerkRequest{Player: player, Skill: "SMITHING"}
		s.emitEvent(game.SkillcapeHasPerkEventName, req)
		boosted = req.Result
	}
	if boosted {
		return choice.recipe.XP + goldsmithBonusXP
	}
	return choice.recipe.XP
}

// resolveRecipe is the recipe a gold-bar choice really smelts:
// the Family Crest 'perfect' gold ore when that is all the
// gold ore the player has (Smelting.Smithing.js resolveRecipe).
func (s *smithing) resolveRecipe(player *game.Player, recipe *SmeltRecipe) *SmeltRecipe {
	if recipe == nil || tables == nil {
		return recipe
	}
	if recipe.BarID != goldBarItemID || s.hasIngredients(player, smeltChoice{recipe: recipe, ingredients: recipe.Ingredients}) {
		return recipe
	}
	perfect := tables.SmeltByBar(perfectGoldBarItemID)
	if perfect != nil && s.hasIngredients(player, smeltChoice{recipe: perfect, ingredients: perfect.Ingredients}) {
		return perfect
	}
	return recipe
}

// beginSmelt opens the cycle: the message, animation and
// sound; the ore goes in on the finish tick
// (Smelting.Smithing.js beginSmelt).
func (s *smithing) beginSmelt(player *game.Player, recipe *SmeltRecipe) bool {
	if recipe == nil {
		return false
	}
	if !s.hasIngredients(player, s.resolveCatalyst(player, recipe)) {
		player.SendMessage(recipe.MissingMessage)
		return false
	}
	if recipe.SmeltMessage != "" {
		spam(player, recipe.SmeltMessage)
	}
	finishDelay := smeltFinishTicks
	if recipe.HasTiming {
		finishDelay = recipe.FinishDelay
	}
	// Immediate recipes animate at the exchange below, once
	// rather than twice.
	if finishDelay > 0 {
		player.PerformAnimation(humanFurnaceSeq, 0)
	}
	game.SendSoundEffect(player, smeltingSoundID, 1, 0, 1)
	return true
}

// finishSmelt makes the bar, finishDelay ticks after begin
// (Smelting.Smithing.js finishSmelt).
func (s *smithing) finishSmelt(player *game.Player, chosen *SmeltRecipe, furnaceID int) bool {
	if chosen == nil {
		return false
	}
	recipe := s.resolveCatalyst(player, chosen)
	if !s.hasIngredients(player, recipe) {
		player.SendMessage(chosen.MissingMessage)
		return false
	}
	s.consumeIngredients(player, recipe)
	player.PerformAnimation(humanFurnaceSeq, 0)
	inv := player.Inventory()
	for _, by := range chosen.Byproducts {
		if inv != nil {
			inv.Add(game.NewItem(by.ItemID, by.Amount), true)
		}
	}
	// A second bar (Varrock armour) sets Second, the recipe
	// its ingredients were taken by.
	outcome := &smeltOutcome{
		Player: player, Recipe: recipe.recipe, Base: chosen,
		FurnaceID: furnaceID, Bars: 1,
		Success: true,
	}
	if recipe.recipe != nil && recipe.recipe.HasSuccessChance {
		outcome.Success = s.randFloat() < recipe.recipe.SuccessChance
	}
	s.emitEvent("smithing:smelt-outcome", outcome)
	if !outcome.Success {
		spam(player, chosen.FailMessage)
		return true
	}
	skill := skillIDForRecipe(chosen)
	if inv != nil {
		inv.Add(game.NewItem(chosen.BarID, outcome.Bars), true)
	}
	xp := s.smeltingXP(player, recipe)
	if outcome.Bars > 1 {
		second := recipe
		if outcome.Second != nil {
			second = *outcome.Second
		}
		xp += float64(outcome.Bars-1) * s.smeltingXP(player, second)
	}
	addXP(player, skill, xp)
	if chosen.RetrieveMessage != "" {
		spam(player, chosen.RetrieveMessage)
	}
	s.emitEvent("smelting:success", &smeltingSuccessEvent{
		Player: player, Skill: skill, ItemID: chosen.BarID, FurnaceID: furnaceID,
	})
	s.produced(player, skill, chosen.BarID, outcome.Bars, xp, "smelt")
	return true
}

// startSmelting starts a smelt loop (Smelting.Smithing.js
// startSmelting). False when the recipe cannot be smelted.
func (s *smithing) startSmelting(player *game.Player, chosen *SmeltRecipe, amount, furnaceID int) bool {
	if player == nil || chosen == nil || amount <= 0 || tables == nil {
		return false
	}
	if chosen.Members && !s.membersWorld {
		player.SendMessage("You need to be on a members world to smelt that bar.")
		return false
	}
	recipe := s.resolveRecipe(player, chosen)
	if recipe.Quest != "" && !s.questComplete(player, recipe.Quest) {
		player.SendMessage("You need to complete " + questName(recipe.Quest) + " to smelt " + recipe.BarPhrase + ".")
		return false
	}
	if s.levelForRecipe(player, recipe) < recipe.Level {
		messageBox(player, "You need a Smithing level of "+itoa(recipe.Level)+" to smelt "+recipe.BarPhrase+".")
		return false
	}
	if !s.hasIngredients(player, s.resolveCatalyst(player, recipe)) {
		player.SendMessage(recipe.MissingMessage)
		return false
	}
	interval, firstDelay, finishDelay := smeltIntervalTicks, 0, smeltFinishTicks
	if recipe.HasTiming {
		interval, firstDelay, finishDelay = recipe.Interval, recipe.FirstDelay, recipe.FinishDelay
	}
	s.startSession(player, sessionOptions{
		kind: "smelt",
		// Molten glass is a Crafting recipe at the same
		// furnace: its batching is Crafting's question.
		skill:       skillIDForRecipe(recipe),
		remaining:   amount,
		firstDelay:  firstDelay,
		interval:    constInterval(interval),
		finishDelay: finishDelay,
		begin:       func(smelter *game.Player) bool { return s.beginSmelt(smelter, recipe) },
		step:        func(smelter *game.Player) bool { return s.finishSmelt(smelter, recipe, furnaceID) },
	})
	return true
}

// isSmeltingActive reports whether the player has a smelt
// session (isSmeltingActive).
func (s *smithing) isSmeltingActive(player *game.Player) bool {
	return s.sessionTypeOf(player) == "smelt"
}

// ---- the Smelt menu ----

// addMenuExtra registers another furnace product offered
// beside the bars (Smelting.Smithing.js addMenuEntries);
// Cannonballs is the only provider.
func (s *smithing) addMenuExtra(provider func(*game.Player) []smeltMenuEntry) {
	if provider != nil {
		s.menuExtras = append(s.menuExtras, provider)
	}
}

// extraEntries is every menu extra for the player.
func (s *smithing) extraEntries(player *game.Player) []smeltMenuEntry {
	var out []smeltMenuEntry
	for _, provider := range s.menuExtras {
		out = append(out, provider(player)...)
	}
	return out
}

// openMenu opens the Smelt menu for the entries, in order
// (Smelting.Smithing.js openMenu). The menu is built with
// game.NewCreationMenu and sent through the session's
// CreationMenuSender, the fletching precedent: script 2046
// travels on opcode 110, NOT 170, and the mount must NOT use
// SendChatboxInterface.
func (s *smithing) openMenu(player *game.Player, entries []smeltMenuEntry) {
	if player == nil || len(entries) == 0 {
		return
	}
	byItem := make(map[int]smeltMenuEntry, len(entries))
	var itemIDs []int
	most := 1
	for _, entry := range entries {
		if _, ok := byItem[entry.itemID]; !ok {
			itemIDs = append(itemIDs, entry.itemID)
		}
		byItem[entry.itemID] = entry
		if entry.most > most {
			most = entry.most
		}
	}
	if most > smeltMaxQuantity {
		most = smeltMaxQuantity
	}
	menu := game.NewCreationMenu(
		smeltTitle,
		itemIDs,
		func(itemID, amount int) {
			if entry, ok := byItem[itemID]; ok && entry.start != nil {
				entry.start(player, amount)
			}
		},
		game.CreationMenuOptions{Mode: smeltMenuMode, MaxAmount: &most},
	)
	if player.Session == nil {
		return
	}
	sender, ok := player.Session.(game.CreationMenuSender)
	if !ok {
		return
	}
	lastItem, hasLastItem := player.CreationMenuLastItem()
	sender.SendCreationMenu(menu, player.ItemName, lastItem, hasLastItem)
}

// recipeEntries is the menu entries for recipes.
func (s *smithing) recipeEntries(player *game.Player, recipes []*SmeltRecipe, furnaceID int) []smeltMenuEntry {
	entries := make([]smeltMenuEntry, 0, len(recipes))
	for _, recipe := range recipes {
		recipe := recipe
		entries = append(entries, smeltMenuEntry{
			itemID: recipe.BarID,
			most:   s.batches(player, s.resolveRecipe(player, recipe)),
			start:  func(smelter *game.Player, amount int) { s.startSmelting(smelter, recipe, amount, furnaceID) },
		})
	}
	return entries
}

// smeltMenuRecipes is the bars the furnace's Smelt menu lists,
// in order (Data.Smithing.js smeltMenuRecipes).
func (s *smithing) smeltMenuRecipes() []*SmeltRecipe {
	if tables == nil {
		return nil
	}
	var out []*SmeltRecipe
	for i := range tables.Smelting {
		recipe := &tables.Smelting[i]
		if recipe.NotInMenu || (recipe.Members && !s.membersWorld) {
			continue
		}
		out = append(out, recipe)
	}
	return out
}

// smelt is "Smelt" on a furnace: the menu lists every bar;
// with nothing to smelt, a message box instead
// (Smelting.Smithing.js smelt).
func (s *smithing) smelt(player *game.Player, furnaceID int) {
	if player == nil || tables == nil {
		return
	}
	hasOre := false
	for i := range tables.Smelting {
		recipe := &tables.Smelting[i]
		if recipe.Skill != "" || recipe.Furnace != "" {
			continue
		}
		for _, ing := range recipe.Ingredients {
			if ing.ItemID != coalItemID && s.available(player, ing.ItemID) > 0 {
				hasOre = true
				break
			}
		}
		if hasOre {
			break
		}
	}
	extras := s.extraEntries(player)
	if !hasOre && len(extras) == 0 {
		var crafting []*SmeltRecipe
		for i := range tables.Smelting {
			recipe := &tables.Smelting[i]
			if recipe.Skill == "" || recipe.Furnace != "" ||
				(recipe.Members && !s.membersWorld) || !s.hasIngredients(player, smeltChoice{recipe: recipe, ingredients: recipe.Ingredients}) {
				continue
			}
			crafting = append(crafting, recipe)
		}
		if len(crafting) > 0 {
			entries := s.recipeEntries(player, crafting, furnaceID)
			// Recording 4283: with one pair, Smelt makes glass
			// directly.
			if len(entries) == 1 && entries[0].most == 1 {
				entries[0].start(player, 1)
			} else {
				s.openMenu(player, entries)
			}
			return
		}
		messageBox(player, noOreMessage)
		return
	}
	s.openMenu(player, append(s.recipeEntries(player, s.smeltMenuRecipes(), furnaceID), extras...))
}

// smeltingUseOnFurnace is an item used on a furnace: a lone,
// unambiguous ore smelts once; otherwise ore opens the menu
// (Smelting.Smithing.js useOnFurnace).
func (s *smithing) smeltingUseOnFurnace(event *plugins.ItemOnObjectEvent) {
	if event == nil || event.Player == nil || tables == nil {
		return
	}
	if s.objectNameOf(event.ObjectID) == lovakiteFurnace {
		s.smeltLovakiteUse(event)
		return
	}
	var recipes []*SmeltRecipe
	for i := range tables.Smelting {
		recipe := &tables.Smelting[i]
		if recipe.Furnace != "" || (recipe.Members && !s.membersWorld) {
			continue
		}
		for _, ing := range recipe.Ingredients {
			if ing.ItemID == event.ItemID {
				recipes = append(recipes, recipe)
				break
			}
		}
	}
	if len(recipes) == 0 {
		return
	}
	event.Handled = true
	var ready []*SmeltRecipe
	for _, recipe := range recipes {
		if recipe.Skill == "" && !recipe.NotInMenu &&
			s.hasIngredients(event.Player, smeltChoice{recipe: recipe, ingredients: recipe.Ingredients}) {
			ready = append(ready, recipe)
		}
	}
	singleOre := true
	for i := range tables.Smelting {
		recipe := &tables.Smelting[i]
		if recipe.Skill != "" || recipe.Furnace != "" {
			continue
		}
		for _, ing := range recipe.Ingredients {
			want := 0
			if ing.ItemID == event.ItemID {
				want = 1
			}
			if s.available(event.Player, ing.ItemID) != want {
				singleOre = false
				break
			}
		}
		if !singleOre {
			break
		}
	}
	// A single ore used on the furnace needs no quantity
	// choice; Smelt keeps its menu.
	if singleOre && len(ready) == 1 && len(s.extraEntries(event.Player)) == 0 {
		s.startSmelting(event.Player, ready[0], 1, event.ObjectID)
		return
	}
	var offMenu []*SmeltRecipe
	for _, recipe := range recipes {
		if recipe.NotInMenu {
			offMenu = append(offMenu, recipe)
		}
	}
	if len(offMenu) > 0 {
		s.openMenu(event.Player, s.recipeEntries(event.Player, offMenu, event.ObjectID))
		return
	}
	s.smelt(event.Player, event.ObjectID)
}

// smeltLovakiteUse is an item used on the Lovakengj furnace,
// which smelts lovakite only (Smelting.Smithing.js
// smeltLovakite).
func (s *smithing) smeltLovakiteUse(event *plugins.ItemOnObjectEvent) {
	if event == nil || event.Player == nil || tables == nil {
		return
	}
	var recipes []*SmeltRecipe
	for i := range tables.Smelting {
		if tables.Smelting[i].Furnace == lovakiteFurnace {
			recipes = append(recipes, &tables.Smelting[i])
		}
	}
	if event.ItemID != 0 {
		matched := false
		for _, recipe := range recipes {
			for _, ing := range recipe.Ingredients {
				if ing.ItemID == event.ItemID {
					matched = true
					break
				}
			}
		}
		if !matched {
			return
		}
		event.Handled = true
	}
	hasOre := false
	for _, recipe := range recipes {
		if len(recipe.Ingredients) > 0 && s.available(event.Player, recipe.Ingredients[0].ItemID) > 0 {
			hasOre = true
			break
		}
	}
	if !hasOre {
		messageBox(event.Player, noOreMessage)
		return
	}
	s.openMenu(event.Player, s.recipeEntries(event.Player, recipes, event.ObjectID))
}

// smeltLovakiteClick is "Smelt" on a Lovakite furnace.
func (s *smithing) smeltLovakiteClick(event *plugins.ObjectInteractionEvent) bool {
	if event == nil || event.Player == nil || tables == nil {
		return false
	}
	s.smeltLovakiteUse(&plugins.ItemOnObjectEvent{Player: event.Player, Object: event.Object, ObjectID: event.ObjectID})
	return true
}

// smeltClick is "Smelt" (or "Use") on a furnace.
func (s *smithing) smeltClick(event *plugins.ObjectInteractionEvent) bool {
	if event == nil || event.Player == nil || tables == nil {
		return false
	}
	s.smelt(event.Player, event.ObjectID)
	return true
}

// answerBarForOre answers "smithing:bar-for-ore": the bar an
// ore smelts into and its XP (Smelting.Smithing.js
// answerBarForOre). Coal, a steel bar's second ingredient,
// smelts into nothing.
func (s *smithing) answerBarForOre(payload any) {
	req, ok := payload.(*barForOreRequest)
	if !ok || req == nil || tables == nil {
		return
	}
	for i := range tables.Smelting {
		recipe := &tables.Smelting[i]
		if recipe.Furnace != "" || len(recipe.Ingredients) == 0 ||
			recipe.Ingredients[0].ItemID != req.OreID ||
			(req.OutputID != 0 && recipe.BarID != req.OutputID) {
			continue
		}
		req.BarID = recipe.BarID
		req.XP = recipe.XP
		return
	}
}

// registerSmelting wires the furnace hooks
// (Smelting.Smithing.js attach). The furnace id set
// (FURNACE_OBJECT_IDS) is a loader concern for the Blast
// Furnace's object catalog; this item needs only the named
// interactions.
func (s *smithing) registerSmelting(api *plugins.Api) {
	api.OnObjectInteractionNamed("Furnace", map[string]plugins.ObjectInteractionHandler{
		"Smelt": s.smeltClick, "Use": s.smeltClick,
	})
	api.OnObjectInteractionNamed("Small furnace", map[string]plugins.ObjectInteractionHandler{
		"Smelt": s.smeltClick,
	})
	api.OnObjectInteractionNamed(lovakiteFurnace, map[string]plugins.ObjectInteractionHandler{
		"Smelt": s.smeltLovakiteClick,
	})
}
