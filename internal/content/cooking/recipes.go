// Recipes.Cooking.js (229 lines, commit bfd81e3): using one
// ingredient on another — flour and water into the four doughs,
// pastry dough into a pie dish, the pie fillings and part pies,
// and the pizza steps and toppings. Recipes, levels and XP are
// the Wiki's (data/definitions/cooking.json); the menus, lines
// and timing are from rsprox captures (recordings 75, 564, 1858).
// None of these steps plays an animation or sound.
//
// The make-many menu reuses the Phase 34 CreationMenu seam
// (game.NewCreationMenu + sendCreationMenu); it is NOT rebuilt
// here.
package cooking

import (
	"fmt"
	"strings"

	"tsps-go/internal/game"
	"tsps-go/internal/plugins"
)

// ---- constants (Recipes.Cooking.js:8-19) ----

const (
	// DoughTitle is DOUGH_TITLE: "What sort of dough do you wish
	// to make?"
	DoughTitle = "What sort of dough do you wish to make?"
	// MakeTitle is MAKE_TITLE: "How many do you wish to make?"
	MakeTitle = "How many do you wish to make?"
	// MakeMenuMode is MAKE_MENU_MODE (0): clientscript
	// skillmulti_setup's first argument on every captured dough,
	// pie and wine menu.
	MakeMenuMode = 0
	// NoSpaceForDoughMessage is NO_SPACE_FOR_DOUGH_MESSAGE.
	NoSpaceForDoughMessage = "You don't have enough inventory space to make any dough."
	// ChocolateBombBowl is CHOCOLATE_BOMB_BOWL (9563): the half
	// made bowl whose cache gameval name is shared by other
	// bowls, so prepareGnomebowl checks the item id, not the
	// product lookup alone.
	ChocolateBombBowl = 9563
)

// DoughTiming is DOUGH_TIMING ([1, 1, 2]): ticks before each
// item — a dough a tick after the choice, the next a tick after
// that and the rest every 2; a pie shell, filling or pizza step
// a tick after the choice and then every 2. The last value
// repeats (Recipes.Cooking.js start).
var DoughTiming = [3]int{1, 1, 2}

// RecipeHooks are the start() hooks (Recipes.Cooking.js:101-128):
// CanMake runs before spending each item's ingredients (false
// aborts), Cycle runs as each item is started (the choice, then
// each item that has another after it), Made runs after each
// item, Done runs after the last one made. Every hook is
// optional (nil means no hook), matching TS's `hooks.x?.()`.
type RecipeHooks struct {
	CanMake func() bool
	Cycle   func(remaining int)
	Made    func()
	Done    func()
}

// ---- inventory helpers (Recipes.Cooking.js:21-53) ----

// hasIngredients reports whether the inventory holds every
// ingredient of the recipe in the needed amounts
// (Recipes.Cooking.js:21-23).
func (c *cooking) hasIngredients(player *game.Player, recipe *Recipe) bool {
	if player == nil || recipe == nil {
		return false
	}
	inventory := player.Inventory()
	if inventory == nil {
		return false
	}
	for _, ing := range recipe.Ingredients {
		if inventory.GetAmount(ing.ID) < ing.Count {
			return false
		}
	}
	return true
}

// freedSlots is the slots one ingredient frees when the recipe
// takes it (Recipes.Cooking.js:26-35): a stack keeps its slot
// unless the recipe takes the whole stack — treating every
// ingredient as freeing one slot loses the returned pot when
// swamp tar is stacked. A non-stackable ingredient frees one
// slot per unit taken, up to what is held.
func (c *cooking) freedSlots(player *game.Player, ingredient Ingredient) int {
	if player == nil {
		return 0
	}
	inventory := player.Inventory()
	if inventory == nil {
		return 0
	}
	held := inventory.GetAmount(ingredient.ID)
	if held <= 0 {
		return 0
	}
	stackable := false
	if src := player.ItemDefinitions(); src != nil {
		stackable = src.IsStackable(ingredient.ID)
	}
	if stackable {
		if held <= ingredient.Count {
			return 1
		}
		return 0
	}
	return min(ingredient.Count, held)
}

// hasRoom reports whether the inventory has room for what one
// item gives back (Recipes.Cooking.js:37-40): the product, its
// returns, less the slots the ingredients free.
func (c *cooking) hasRoom(player *game.Player, recipe *Recipe) bool {
	if player == nil || recipe == nil {
		return false
	}
	inventory := player.Inventory()
	if inventory == nil {
		return false
	}
	freed := 0
	for _, ingredient := range recipe.Ingredients {
		freed += c.freedSlots(player, ingredient)
	}
	return inventory.GetFreeSlots() >= 1+len(recipe.Returns)-freed
}

// makeable is how many of the recipe the inventory holds the
// ingredients for (Recipes.Cooking.js:43-45): the min over
// ingredients of floor(amount/count). A recipe with no
// ingredients makes 0 (TS Math.min over an empty list is
// Infinity; no row has no ingredients, so 0 is the safe Go
// form).
func (c *cooking) makeable(player *game.Player, recipe *Recipe) int {
	if player == nil || recipe == nil || len(recipe.Ingredients) == 0 {
		return 0
	}
	inventory := player.Inventory()
	if inventory == nil {
		return 0
	}
	most := -1
	for _, ing := range recipe.Ingredients {
		if ing.Count <= 0 {
			return 0
		}
		n := inventory.GetAmount(ing.ID) / ing.Count
		if most < 0 || n < most {
			most = n
		}
	}
	return most
}

// productName is the product's cache name, lowercased
// (Recipes.Cooking.js:47-49).
func (c *cooking) productName(player *game.Player, recipe *Recipe) string {
	if player == nil || recipe == nil {
		return ""
	}
	return strings.ToLower(player.ItemName(recipe.ProductID))
}

// isMembersRecipe reports whether a recipe is members-only
// (Recipes.Cooking.js:56-58): a row's own `members` field
// overrides the cache when the two disagree; otherwise the
// cache item's isMembers flag (resolved at load into
// Recipe.Members). No recipe row sets `members` — cache only
// in practice.
func (c *cooking) isMembersRecipe(recipe *Recipe) bool {
	if recipe == nil {
		return false
	}
	if recipe.MembersOverride != nil {
		return *recipe.MembersOverride
	}
	return recipe.Members
}

// availableOnWorld reports whether the recipe may be made on
// this world (Recipes.Cooking.js:60-62): a members world allows
// everything; a free world refuses members recipes.
func (c *cooking) availableOnWorld(recipe *Recipe) bool {
	membersWorld := true
	if c != nil {
		membersWorld = c.membersWorld
	}
	return membersWorld || !c.isMembersRecipe(recipe)
}

// canMake reports whether the player can make the recipe, and
// says why not when it is members-only here or the level is too
// low (Recipes.Cooking.js:65-74): the members refusal is the
// FreeToPlay item message; the level refusal is a mesbox
// "You need a Cooking level of {level} to make {article+name}."
func (c *cooking) canMake(player *game.Player, recipe *Recipe) bool {
	if player == nil || recipe == nil {
		return false
	}
	if !c.availableOnWorld(recipe) {
		player.SendMessage(FreeToPlayItemMessage)
		return false
	}
	if c.cookingLevel(player) >= recipe.Level {
		return true
	}
	c.mesbox(player, fmt.Sprintf("You need a Cooking level of %d to make %s.",
		recipe.Level, withArticle(c.productName(player, recipe), "")))
	return false
}

// ---- making (Recipes.Cooking.js:77-128) ----

// makeOne makes one item (Recipes.Cooking.js:77-93): the
// ingredients go, the product (and any emptied containers)
// come, with any XP. It returns false when nothing was made:
// unavailable on this world, missing ingredients, or no room
// (the room refusal sends inventory.full()). It emits
// skills:produced {kind: "make"}; no Go consumer exists yet
// except Fletching Diaries (fletching-specific tasks), so the
// emit is registered but unanswered.
func (c *cooking) makeOne(player *game.Player, recipe *Recipe) bool {
	if player == nil || recipe == nil {
		return false
	}
	if !c.availableOnWorld(recipe) || !c.hasIngredients(player, recipe) {
		return false
	}
	inventory := player.Inventory()
	if inventory == nil {
		return false
	}
	if !c.hasRoom(player, recipe) {
		inventory.Full()
		return false
	}
	for _, ing := range recipe.Ingredients {
		inventory.DeleteNumber(ing.ID, ing.Count)
	}
	inventory.AddItem(game.NewItem(recipe.ProductID, 1))
	for _, ret := range recipe.Returns {
		inventory.AddItem(game.NewItem(ret.ID, ret.Count))
	}
	if recipe.Message != "" {
		c.spam(player, recipe.Message)
	}
	c.addXP(player, recipe.XP)
	c.emitEvent(SkillsProducedEventName, &ProducedEvent{
		Player: player,
		Skill:  game.SkillCooking,
		ItemID: recipe.ProductID,
		Amount: 1,
		XP:     recipe.XP,
		Kind:   "make",
	})
	return true
}

// startRecipe makes amount on the recipe's timing
// (Recipes.Cooking.js:101-128 start): TaskManager.cancelTasks
// (player index) first — a new make cancels any running one —
// then the timing loop via Common.run, with hooks.cycle at each
// item start, hooks.made after each, hooks.done at the end, and
// actionsPerCycle per cycle capped by remaining. wait is
// recipe.timing[0] first, then timing[min(made, len-1)] (the
// last value repeats). A canMake refusal or a failed makeOne
// stops the loop (hooks.done only when something was made).
func (c *cooking) startRecipe(player *game.Player, recipe *Recipe, amount int, hooks RecipeHooks) {
	if player == nil || recipe == nil || amount <= 0 {
		return
	}
	if tm := c.taskManager(); tm != nil {
		tm.CancelTasks(player.Index)
	}
	if hooks.CanMake != nil && !hooks.CanMake() {
		return
	}
	remaining := amount
	made := 0
	wait := timingAt(recipe, 0)
	if hooks.Cycle != nil {
		hooks.Cycle(remaining)
	}
	c.run(player, func(tick int64) bool {
		wait--
		if wait > 0 {
			return true
		}
		actions := c.actionsPerCycle(player, "make")
		for done := 0; done < actions && remaining > 0; done++ {
			if (hooks.CanMake != nil && !hooks.CanMake()) || !c.makeOne(player, recipe) {
				if made > 0 && hooks.Done != nil {
					hooks.Done()
				}
				return false
			}
			made++
			remaining--
			if hooks.Made != nil {
				hooks.Made()
			}
		}
		if remaining <= 0 || !c.hasIngredients(player, recipe) || (hooks.CanMake != nil && !hooks.CanMake()) {
			if hooks.Done != nil {
				hooks.Done()
			}
			return false
		}
		if hooks.Cycle != nil {
			hooks.Cycle(remaining)
		}
		wait = timingAt(recipe, made)
		return true
	})
}

// timingAt is the ticks before the (made+1)-th item:
// recipe.timing[min(made, len-1)] (Recipes.Cooking.js start) —
// the last value repeats. A recipe with no timing reads 1.
func timingAt(recipe *Recipe, made int) int {
	if recipe == nil || len(recipe.Timing) == 0 {
		return 1
	}
	if made >= len(recipe.Timing) {
		return recipe.Timing[len(recipe.Timing)-1]
	}
	return recipe.Timing[made]
}

// offer offers the make-many menu for a recipe
// (Recipes.Cooking.js:134-153): canMake first, room first
// (inventory.full()), most = makeable; most <= 0 returns; most
// == 1 starts directly (no menu); else the CreationMenu
// (MAKE_TITLE, [productId], execute → start(min(amount,
// makeable)), {mode: 0, maxAmount: most, lastAmount: most}).
func (c *cooking) offer(player *game.Player, recipe *Recipe, hooks RecipeHooks) {
	if player == nil || recipe == nil {
		return
	}
	if !c.canMake(player, recipe) {
		return
	}
	inventory := player.Inventory()
	if inventory == nil {
		return
	}
	if !c.hasRoom(player, recipe) {
		inventory.Full()
		return
	}
	most := c.makeable(player, recipe)
	if most <= 0 {
		return
	}
	if most == 1 {
		c.startRecipe(player, recipe, 1, hooks)
		return
	}
	menu := game.NewCreationMenu(
		MakeTitle,
		[]int{recipe.ProductID},
		func(itemID, amount int) {
			c.startRecipe(player, recipe, min(amount, c.makeable(player, recipe)), hooks)
		},
		game.CreationMenuOptions{Mode: MakeMenuMode, MaxAmount: &most, LastAmount: &most},
	)
	c.sendCreationMenu(player, menu)
}

// ---- doughs (Recipes.Cooking.js:156-202) ----

// doughRecipe builds the dough recipe for this product with
// this water (Recipes.Cooking.js:156-167): product with 0 XP,
// ingredients flour 1 + water 1, returns pot 1 + empty
// container 1, timing DOUGH_TIMING.
func (c *cooking) doughRecipe(dough *DoughData, water DoughWater, product DoughProduct) *Recipe {
	if dough == nil {
		return nil
	}
	return &Recipe{
		ProductID: product.ProductID,
		Level:     product.Level,
		Message:   product.Message,
		Ingredients: []Ingredient{
			{ID: dough.FlourID, Count: 1},
			{ID: water.ID, Count: 1},
		},
		Returns: []Ingredient{
			{ID: dough.PotID, Count: 1},
			{ID: water.EmptyID, Count: 1},
		},
		Timing:  append([]int(nil), DoughTiming[:]...),
		Members: product.Members,
	}
}

// offerDough opens the dough menu (Recipes.Cooking.js:173-195):
// flour on water (or water on flour) always opens it, but not
// without a free slot (NO_SPACE_FOR_DOUGH_MESSAGE). The menu
// shows all four dough product ids with maxAmount/lastAmount =
// min(flour, water); execute finds the product, builds the
// dough recipe, and starts it when canMake passes (min(amount,
// makeable)).
func (c *cooking) offerDough(player *game.Player, water DoughWater) {
	if player == nil || c == nil || c.tables == nil || c.tables.Doughs == nil {
		return
	}
	dough := c.tables.Doughs
	inventory := player.Inventory()
	if inventory == nil {
		return
	}
	if inventory.GetFreeSlots() <= 0 {
		player.SendMessage(NoSpaceForDoughMessage)
		return
	}
	most := min(inventory.GetAmount(dough.FlourID), inventory.GetAmount(water.ID))
	var items []int
	byProduct := make(map[int]DoughProduct, len(dough.Products))
	for _, product := range dough.Products {
		items = append(items, product.ProductID)
		byProduct[product.ProductID] = product
	}
	menu := game.NewCreationMenu(
		DoughTitle,
		items,
		func(itemID, amount int) {
			product, ok := byProduct[itemID]
			if !ok {
				return
			}
			recipe := c.doughRecipe(dough, water, product)
			if c.canMake(player, recipe) {
				c.startRecipe(player, recipe, min(amount, c.makeable(player, recipe)), RecipeHooks{})
			}
		},
		game.CreationMenuOptions{Mode: MakeMenuMode, MaxAmount: &most, LastAmount: &most},
	)
	c.sendCreationMenu(player, menu)
}

// waterFor returns the dough water of the pair, when either
// item is flour (Recipes.Cooking.js:197-202): the other must be
// a dough water, else no dough.
func (c *cooking) waterFor(firstID, secondID int) (DoughWater, bool) {
	if c == nil || c.tables == nil || c.tables.Doughs == nil {
		return DoughWater{}, false
	}
	dough := c.tables.Doughs
	if firstID != dough.FlourID && secondID != dough.FlourID {
		return DoughWater{}, false
	}
	otherID := secondID
	if firstID != dough.FlourID {
		otherID = firstID
	}
	for _, water := range dough.Waters {
		if water.ID == otherID {
			return water, true
		}
	}
	return DoughWater{}, false
}

// ---- lookups (Common.Cooking.js:84-98) ----

// recipeWith returns the recipe these two items make, in data
// order (lowest level first), or nil (Common.Cooking.js:84-88
// recipeWith): the first recipe that takes firstId as an
// ingredient and also takes secondId, with firstId != secondId.
func (c *cooking) recipeWith(firstID, secondID int) *Recipe {
	if c == nil || c.tables == nil || firstID == secondID {
		return nil
	}
	for _, recipe := range c.tables.RecipesForIngredient(firstID) {
		for _, ing := range recipe.Ingredients {
			if ing.ID == secondID {
				return recipe
			}
		}
	}
	return nil
}

// recipeForProduct returns the item-on-item recipe that makes
// productID, or nil (Common.Cooking.js:91-94 recipeForProduct).
func (c *cooking) recipeForProduct(productID int) *Recipe {
	if c == nil || c.tables == nil {
		return nil
	}
	return c.tables.RecipeForProduct(productID)
}

// ---- hooks (Recipes.Cooking.js:204-224) ----

// useItemOnItem handles item on item (Recipes.Cooking.js:
// 204-216): water → offerDough (handled); else the recipe the
// pair makes (data order) → offer (handled). An unknown pair
// falls through unclaimed.
func (c *cooking) useItemOnItem(event *plugins.PluginItemOnItemEvent) {
	if event == nil || event.Player == nil {
		return
	}
	if water, ok := c.waterFor(event.UsedItemID, event.UsedWithItemID); ok {
		event.Handled = true
		c.offerDough(event.Player, water)
		return
	}
	recipe := c.recipeWith(event.UsedItemID, event.UsedWithItemID)
	if recipe == nil {
		return
	}
	event.Handled = true
	c.offer(event.Player, recipe, RecipeHooks{})
}

// prepareGnomebowl handles the "Half baked bowl" Prepare action
// (Recipes.Cooking.js:219-224): the chocolate-bomb recipe for
// product 9563, only when the clicked item is gameval
// half_baked_bowl (the cache name is shared by other bowls, so
// the id check matters). It returns false (falls through) when
// there is no such recipe or the item is not the half baked
// bowl; otherwise it offers the recipe and claims the click.
func (c *cooking) prepareGnomebowl(event *plugins.PluginItemActionEvent) bool {
	if event == nil || event.Player == nil {
		return false
	}
	recipe := c.recipeForProduct(ChocolateBombBowl)
	if recipe == nil {
		return false
	}
	halfBakedBowl, ok := c.itemID("half_baked_bowl")
	if !ok || event.ItemID != halfBakedBowl {
		return false
	}
	c.offer(event.Player, recipe, RecipeHooks{})
	return true
}

// registerRecipes wires the Recipes unit's hooks, in TS attach
// order (Recipes.Cooking.js:226-229): the generic onItemOnItem
// (noted:false), then the "Half baked bowl" Prepare action.
func (c *cooking) registerRecipes(api *plugins.Api) {
	if api == nil {
		return
	}
	api.OnItemOnItem(c.useItemOnItem, &plugins.ItemUseFilter{Noted: plugins.BoolPtr(false)})
	api.OnItemActionNamed("Half baked bowl", map[string]plugins.ItemActionHandler{"Prepare": c.prepareGnomebowl}, nil)
}
