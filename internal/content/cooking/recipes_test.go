package cooking

import (
	"testing"

	"tsps-go/internal/game"
	"tsps-go/internal/plugins"
)

func TestHasIngredientsAndMakeable(t *testing.T) {
	c, _, _ := newTestCooking()
	p := newTestPlayer("alice")
	recipe := c.recipeForProduct(fxRawSwampPaste)
	if recipe == nil {
		t.Fatal("no swamp paste recipe in fixture tables")
	}
	if c.hasIngredients(p, recipe) {
		t.Error("hasIngredients with empty inventory = true, want false")
	}
	give(p, fxFlour, 1)
	if c.hasIngredients(p, recipe) {
		t.Error("hasIngredients with flour only = true, want false")
	}
	give(p, fxSwampTar, 3)
	if !c.hasIngredients(p, recipe) {
		t.Error("hasIngredients with flour + tar = false, want true")
	}
	// makeable is the min over ingredients: 1 flour caps it.
	if got := c.makeable(p, recipe); got != 1 {
		t.Errorf("makeable = %d, want 1 (flour caps)", got)
	}
	give(p, fxFlour, 2)
	if got := c.makeable(p, recipe); got != 3 {
		t.Errorf("makeable = %d, want 3 (tar caps)", got)
	}
}

func TestFreedSlotsStackAware(t *testing.T) {
	c, _, _ := newTestCooking()
	p := newTestPlayer("alice")
	// Swamp tar stacks (fake defs): taking the whole stack frees
	// its slot; taking part of it frees none (the swamp-tar
	// case, Recipes.Cooking.js:26-35).
	give(p, fxSwampTar, 3)
	if got := c.freedSlots(p, Ingredient{ID: fxSwampTar, Count: 3}); got != 1 {
		t.Errorf("freedSlots(tar 3/3) = %d, want 1 (whole stack)", got)
	}
	if got := c.freedSlots(p, Ingredient{ID: fxSwampTar, Count: 1}); got != 0 {
		t.Errorf("freedSlots(tar 1/3) = %d, want 0 (stack keeps its slot)", got)
	}
	// Flour does not stack: each unit taken frees a slot.
	give(p, fxFlour, 2)
	if got := c.freedSlots(p, Ingredient{ID: fxFlour, Count: 1}); got != 1 {
		t.Errorf("freedSlots(flour 1/2) = %d, want 1", got)
	}
	if got := c.freedSlots(p, Ingredient{ID: fxFlour, Count: 5}); got != 2 {
		t.Errorf("freedSlots(flour 5/2) = %d, want 2 (capped at held)", got)
	}
	if got := c.freedSlots(p, Ingredient{ID: 99999, Count: 1}); got != 0 {
		t.Errorf("freedSlots(absent) = %d, want 0", got)
	}
}

func TestHasRoom(t *testing.T) {
	c, _, _ := newTestCooking()
	p := newTestPlayer("alice")
	recipe := c.recipeForProduct(fxRawSwampPaste)
	// Paste: 1 product + 1 return, 2 ingredients freed (flour
	// non-stackable 1, tar whole stack 1) → needs 0 free slots.
	give(p, fxFlour, 1)
	give(p, fxSwampTar, 1)
	if !c.hasRoom(p, recipe) {
		t.Error("hasRoom(paste, full-ish inventory) = false, want true (ingredients free 2, need 0)")
	}
	// Fill the inventory with junk, keeping the ingredients:
	// free slots 0, freed 2 → still room.
	give(p, fxPastryDough, 1)
	give(p, fxPieDish, 1)
	for i := 0; i < 24; i++ {
		give(p, 5000+i, 1)
	}
	if p.Inventory().GetFreeSlots() != 0 {
		t.Fatalf("free slots = %d, want 0 (test setup)", p.Inventory().GetFreeSlots())
	}
	if !c.hasRoom(p, recipe) {
		t.Error("hasRoom(paste, 0 free) = false, want true (freed slots cover it)")
	}
	// The pie shell returns nothing and frees 2 slots for 1
	// product: also fine at 0 free slots.
	shell := c.recipeForProduct(fxPieShell)
	if !c.hasRoom(p, shell) {
		t.Error("hasRoom(shell, 0 free) = false, want true")
	}
}

func TestMakeOne(t *testing.T) {
	c, emitted, _ := newTestCooking()
	p := newTestPlayer("alice")
	setCooking(p, 1)
	session := &fakeSession{}
	p.Session = session
	recipe := c.recipeForProduct(fxRawSwampPaste)
	give(p, fxFlour, 2)
	give(p, fxSwampTar, 2)
	if !c.makeOne(p, recipe) {
		t.Fatal("makeOne = false, want true")
	}
	if got := countOf(p, fxFlour); got != 1 {
		t.Errorf("flour = %d, want 1", got)
	}
	if got := countOf(p, fxSwampTar); got != 1 {
		t.Errorf("tar = %d, want 1", got)
	}
	if got := countOf(p, fxRawSwampPaste); got != 1 {
		t.Errorf("paste = %d, want 1", got)
	}
	if got := countOf(p, fxPot); got != 1 {
		t.Errorf("returned pot = %d, want 1", got)
	}
	// skills:produced {kind: "make"} emitted once.
	produced := 0
	for _, e := range *emitted {
		if e.name == SkillsProducedEventName {
			produced++
			ev, ok := e.payload.(*ProducedEvent)
			if !ok || ev.ItemID != fxRawSwampPaste || ev.Kind != "make" || ev.Amount != 1 {
				t.Errorf("produced event = %+v, want paste/make/1", e.payload)
			}
		}
	}
	if produced != 1 {
		t.Errorf("produced emits = %d, want 1", produced)
	}
	// No room: a full inventory with no freed slots refuses and
	// keeps the ingredients.
	p2 := newTestPlayer("bob")
	setCooking(p2, 1)
	give(p2, fxFlour, 1)
	give(p2, fxSwampTar, 1)
	for i := 0; i < 26; i++ {
		give(p2, 6000+i, 1)
	}
	// 28 slots: flour + tar + 26 junk = full, but the paste
	// frees both ingredient slots, so it fits — instead use a
	// recipe whose ingredients free nothing: make the tar
	// partial (3 held, 1 taken) so freed = 1 (flour) + 0 (tar)
	// and the paste needs 2 - 1 = 1 free slot... it has 0.
	p2.Inventory().DeleteNumber(fxSwampTar, 1)
	give(p2, fxSwampTar, 3)
	if c.makeOne(p2, recipe) {
		t.Error("makeOne with no room = true, want false")
	}
	if got := countOf(p2, fxFlour); got != 1 {
		t.Errorf("flour after refused makeOne = %d, want 1 (kept)", got)
	}
}

func TestMakeOneXPAndMessage(t *testing.T) {
	c, _, _ := newTestCooking()
	p := newTestPlayer("alice")
	setCooking(p, 42)
	session := &fakeSession{}
	p.Session = session
	// Chocolate bomb: xp 50, no message; returns the mould.
	recipe := c.recipeForProduct(ChocolateBombBowl)
	give(p, fxHalfBakedBowl, 1)
	give(p, fxChocolateBar, 4)
	give(p, fxEquaLeaves, 1)
	if !c.makeOne(p, recipe) {
		t.Fatal("makeOne(bomb) = false, want true")
	}
	if got := p.Skills().Experience(game.SkillCooking); got != 50 {
		t.Errorf("Experience = %d, want 50", got)
	}
	if got := countOf(p, fxGnomebowlMould); got != 1 {
		t.Errorf("returned mould = %d, want 1", got)
	}
	// The pie shell has no message and no XP: silent, XP-free.
	p2 := newTestPlayer("bob")
	setCooking(p2, 1)
	shell := c.recipeForProduct(fxPieShell)
	give(p2, fxPastryDough, 1)
	give(p2, fxPieDish, 1)
	if !c.makeOne(p2, shell) {
		t.Fatal("makeOne(shell) = false, want true")
	}
	if got := p2.Skills().Experience(game.SkillCooking); got != 0 {
		t.Errorf("Experience after shell = %d, want 0", got)
	}
}

func TestCanMakeGates(t *testing.T) {
	c, _, _ := newTestCooking()
	p := newTestPlayer("alice")
	session := &fakeSession{}
	p.Session = session
	// Level gate: level 1 cannot make the level-42 bomb.
	setCooking(p, 1)
	recipe := c.recipeForProduct(ChocolateBombBowl)
	if c.canMake(p, recipe) {
		t.Error("canMake(bomb at level 1) = true, want false")
	}
	setCooking(p, 42)
	if !c.canMake(p, recipe) {
		t.Error("canMake(bomb at level 42) = false, want true")
	}
	// Members gate: a members recipe on a free world refuses
	// with the FreeToPlay item message.
	members := &Recipe{ProductID: fxChocolateBomb, Level: 1, Members: true, Timing: []int{1, 2}}
	c.membersWorld = false
	if c.canMake(p, members) {
		t.Error("canMake(members recipe, free world) = true, want false")
	}
	if len(session.messages) != 1 || session.messages[0] != FreeToPlayItemMessage {
		t.Errorf("messages = %v, want [%q]", session.messages, FreeToPlayItemMessage)
	}
	// A members world allows it.
	c.membersWorld = true
	if !c.canMake(p, members) {
		t.Error("canMake(members recipe, members world) = false, want true")
	}
	// A row `members` override wins over the cache flag.
	no := false
	overridden := &Recipe{ProductID: fxChocolateBomb, Level: 1, Members: true, MembersOverride: &no, Timing: []int{1, 2}}
	c.membersWorld = false
	if c.isMembersRecipe(overridden) {
		t.Error("isMembersRecipe with members=false override = true, want false")
	}
}

func TestStartRecipeTimingAndHooks(t *testing.T) {
	c, _, tm := newTestCooking()
	p := newTestPlayer("alice")
	p.SetIndex(11)
	setCooking(p, 1)
	recipe := c.recipeForProduct(fxRawSwampPaste)
	give(p, fxFlour, 3)
	give(p, fxSwampTar, 3)
	var cycles []int
	made, done := 0, 0
	hooks := RecipeHooks{
		Cycle: func(remaining int) { cycles = append(cycles, remaining) },
		Made:  func() { made++ },
		Done:  func() { done++ },
	}
	c.startRecipe(p, recipe, 3, hooks)
	// Timing [1, 2]: first item on tick 1, second on tick 3,
	// third on tick 5; done after the third.
	driveTicks(tm, 1)
	if made != 1 {
		t.Errorf("made after tick 1 = %d, want 1", made)
	}
	driveTicks(tm, 1)
	if made != 1 {
		t.Errorf("made after tick 2 = %d, want 1 (waiting)", made)
	}
	driveTicks(tm, 1)
	if made != 2 {
		t.Errorf("made after tick 3 = %d, want 2", made)
	}
	driveTicks(tm, 2)
	if made != 3 || done != 1 {
		t.Errorf("made/done after tick 5 = %d/%d, want 3/1", made, done)
	}
	if got := countOf(p, fxRawSwampPaste); got != 3 {
		t.Errorf("paste = %d, want 3", got)
	}
	// Cycle ran at the choice (3 remaining) and before each
	// later item (2, then 1).
	if len(cycles) != 3 || cycles[0] != 3 || cycles[1] != 2 || cycles[2] != 1 {
		t.Errorf("cycles = %v, want [3 2 1]", cycles)
	}
}

func TestStartRecipeCanMakeAbort(t *testing.T) {
	c, _, tm := newTestCooking()
	p := newTestPlayer("alice")
	p.SetIndex(12)
	setCooking(p, 1)
	recipe := c.recipeForProduct(fxRawSwampPaste)
	give(p, fxFlour, 2)
	give(p, fxSwampTar, 2)
	// A CanMake hook refusing before the first item aborts with
	// nothing made and no Done.
	hooks := RecipeHooks{CanMake: func() bool { return false }}
	c.startRecipe(p, recipe, 2, hooks)
	driveTicks(tm, 3)
	if got := countOf(p, fxRawSwampPaste); got != 0 {
		t.Errorf("paste after aborted start = %d, want 0", got)
	}
	// A CanMake hook refusing after one item stops the loop and
	// calls Done (made > 0).
	done := 0
	calls := 0
	hooks = RecipeHooks{
		CanMake: func() bool {
			calls++
			return calls <= 2 // start check + first item check pass; second fails
		},
		Done: func() { done++ },
	}
	c.startRecipe(p, recipe, 2, hooks)
	driveTicks(tm, 3)
	if got := countOf(p, fxRawSwampPaste); got != 1 {
		t.Errorf("paste after mid-loop abort = %d, want 1", got)
	}
	if done != 1 {
		t.Errorf("done = %d, want 1", done)
	}
}

func TestStartRecipeActionsPerCycle(t *testing.T) {
	c, _, tm := newTestCooking()
	p := newTestPlayer("alice")
	p.SetIndex(13)
	setCooking(p, 1)
	// An actions-per-cycle answer of 2 makes two per cycle.
	c.emit = func(name string, payload any) {
		if req, ok := payload.(*ActionsPerCycleRequest); ok {
			req.Actions = 2
		}
	}
	recipe := c.recipeForProduct(fxRawSwampPaste)
	give(p, fxFlour, 4)
	give(p, fxSwampTar, 4)
	c.startRecipe(p, recipe, 4, RecipeHooks{})
	driveTicks(tm, 1)
	if got := countOf(p, fxRawSwampPaste); got != 2 {
		t.Errorf("paste after tick 1 = %d, want 2 (actions per cycle)", got)
	}
	driveTicks(tm, 2)
	if got := countOf(p, fxRawSwampPaste); got != 4 {
		t.Errorf("paste after tick 3 = %d, want 4", got)
	}
}

func TestOfferSingleStartsDirectly(t *testing.T) {
	c, _, tm := newTestCooking()
	p := newTestPlayer("alice")
	p.SetIndex(14)
	setCooking(p, 1)
	session := &fakeSession{}
	p.Session = session
	recipe := c.recipeForProduct(fxRawSwampPaste)
	give(p, fxFlour, 1)
	give(p, fxSwampTar, 1)
	// most == 1: no menu, the make starts directly.
	c.offer(p, recipe, RecipeHooks{})
	if len(session.menus) != 0 {
		t.Errorf("menus = %d, want 0 (single make, no menu)", len(session.menus))
	}
	driveTicks(tm, 1)
	if got := countOf(p, fxRawSwampPaste); got != 1 {
		t.Errorf("paste after tick 1 = %d, want 1", got)
	}
}

func TestOfferMenu(t *testing.T) {
	c, _, tm := newTestCooking()
	p := newTestPlayer("alice")
	p.SetIndex(15)
	setCooking(p, 1)
	session := &fakeSession{}
	p.Session = session
	recipe := c.recipeForProduct(fxRawSwampPaste)
	give(p, fxFlour, 3)
	give(p, fxSwampTar, 3)
	c.offer(p, recipe, RecipeHooks{})
	if len(session.menus) != 1 {
		t.Fatalf("menus = %d, want 1", len(session.menus))
	}
	menu := session.menus[0]
	if menu.Title() != MakeTitle {
		t.Errorf("menu title = %q, want %q", menu.Title(), MakeTitle)
	}
	if got := menu.Items(); len(got) != 1 || got[0] != fxRawSwampPaste {
		t.Errorf("menu items = %v, want [%d]", got, fxRawSwampPaste)
	}
	if menu.Options().Mode != MakeMenuMode {
		t.Errorf("menu mode = %d, want %d", menu.Options().Mode, MakeMenuMode)
	}
	if menu.EffectiveMaxAmount() != 3 || menu.EffectiveLastAmount() != 3 {
		t.Errorf("menu max/last = %d/%d, want 3/3", menu.EffectiveMaxAmount(), menu.EffectiveLastAmount())
	}
	// Nothing is made until the menu is used.
	driveTicks(tm, 2)
	if got := countOf(p, fxRawSwampPaste); got != 0 {
		t.Errorf("paste before choosing = %d, want 0", got)
	}
	// Choosing 2 makes 2 (timing [1,2]: ticks 1 and 3).
	menu.Execute(fxRawSwampPaste, 2)
	driveTicks(tm, 3)
	if got := countOf(p, fxRawSwampPaste); got != 2 {
		t.Errorf("paste after choosing 2 = %d, want 2", got)
	}
}

func TestDoughRecipeAndWaterFor(t *testing.T) {
	c, _, _ := newTestCooking()
	dough := c.tables.Doughs
	water, ok := c.waterFor(fxFlour, fxBucketWater)
	if !ok || water.ID != fxBucketWater || water.EmptyID != fxBucketEmpty {
		t.Errorf("waterFor(flour, bucket) = %+v/%v, want bucket water", water, ok)
	}
	// Either order works.
	if _, ok := c.waterFor(fxJugWater, fxFlour); !ok {
		t.Error("waterFor(jug, flour) = false, want true")
	}
	// Neither item flour: no dough.
	if _, ok := c.waterFor(fxBucketWater, fxJugWater); ok {
		t.Error("waterFor(bucket, jug) = true, want false")
	}
	// Flour + a non-water: no dough.
	if _, ok := c.waterFor(fxFlour, fxPieDish); ok {
		t.Error("waterFor(flour, pie dish) = true, want false")
	}
	recipe := c.doughRecipe(dough, water, dough.Products[0])
	if recipe.ProductID != fxBreadDough || recipe.XP != 0 || recipe.Level != 1 {
		t.Errorf("dough recipe = %+v, want bread dough level 1 xp 0", recipe)
	}
	if len(recipe.Timing) != 3 || recipe.Timing[2] != 2 {
		t.Errorf("dough timing = %v, want [1 1 2]", recipe.Timing)
	}
	if len(recipe.Ingredients) != 2 || recipe.Ingredients[0].ID != fxFlour || recipe.Ingredients[1].ID != fxBucketWater {
		t.Errorf("dough ingredients = %v, want flour + bucket water", recipe.Ingredients)
	}
	if len(recipe.Returns) != 2 || recipe.Returns[0].ID != fxPot || recipe.Returns[1].ID != fxBucketEmpty {
		t.Errorf("dough returns = %v, want pot + bucket empty", recipe.Returns)
	}
}

func TestOfferDough(t *testing.T) {
	c, _, tm := newTestCooking()
	p := newTestPlayer("alice")
	p.SetIndex(16)
	setCooking(p, 1)
	session := &fakeSession{}
	p.Session = session
	give(p, fxFlour, 2)
	give(p, fxBucketWater, 2)
	water, _ := c.waterFor(fxFlour, fxBucketWater)
	c.offerDough(p, water)
	if len(session.menus) != 1 {
		t.Fatalf("menus = %d, want 1", len(session.menus))
	}
	menu := session.menus[0]
	if menu.Title() != DoughTitle {
		t.Errorf("menu title = %q, want %q", menu.Title(), DoughTitle)
	}
	if got := menu.Items(); len(got) != 4 {
		t.Errorf("menu items = %v, want the 4 doughs", got)
	}
	if menu.EffectiveMaxAmount() != 2 {
		t.Errorf("menu max = %d, want 2 (min flour/water)", menu.EffectiveMaxAmount())
	}
	// Choose bread dough x2: dough timing [1,1,2] makes one on
	// tick 1 and one on tick 2.
	menu.Execute(fxBreadDough, 2)
	driveTicks(tm, 1)
	if got := countOf(p, fxBreadDough); got != 1 {
		t.Errorf("bread dough after tick 1 = %d, want 1", got)
	}
	driveTicks(tm, 1)
	if got := countOf(p, fxBreadDough); got != 2 {
		t.Errorf("bread dough after tick 2 = %d, want 2", got)
	}
	if got := countOf(p, fxPot); got != 2 {
		t.Errorf("returned pots = %d, want 2", got)
	}
	if got := countOf(p, fxBucketEmpty); got != 2 {
		t.Errorf("returned buckets = %d, want 2", got)
	}
	// No free slot: the refusal message, no menu.
	p2 := newTestPlayer("bob")
	setCooking(p2, 1)
	session2 := &fakeSession{}
	p2.Session = session2
	give(p2, fxFlour, 1)
	give(p2, fxBucketWater, 1)
	for i := 0; i < 26; i++ {
		give(p2, 7000+i, 1)
	}
	c.offerDough(p2, water)
	if len(session2.menus) != 0 {
		t.Errorf("menus with no free slot = %d, want 0", len(session2.menus))
	}
	if len(session2.messages) != 1 || session2.messages[0] != NoSpaceForDoughMessage {
		t.Errorf("messages = %v, want [%q]", session2.messages, NoSpaceForDoughMessage)
	}
}

func TestRecipeWithAndForProduct(t *testing.T) {
	c, _, _ := newTestCooking()
	// Data order: pastry dough on pie dish finds the shell.
	if got := c.recipeWith(fxPastryDough, fxPieDish); got == nil || got.ProductID != fxPieShell {
		t.Errorf("recipeWith(pastry, dish) = %v, want pie shell", got)
	}
	// Same id twice: no recipe (firstId !== secondId).
	if got := c.recipeWith(fxFlour, fxFlour); got != nil {
		t.Errorf("recipeWith(flour, flour) = %v, want nil", got)
	}
	// Unknown pair: nil.
	if got := c.recipeWith(fxFlour, fxPieDish); got != nil {
		t.Errorf("recipeWith(flour, dish) = %v, want nil", got)
	}
	if got := c.recipeForProduct(99999); got != nil {
		t.Errorf("recipeForProduct(unknown) = %v, want nil", got)
	}
}

func TestUseItemOnItem(t *testing.T) {
	c, _, tm := newTestCooking()
	p := newTestPlayer("alice")
	p.SetIndex(17)
	setCooking(p, 1)
	session := &fakeSession{}
	p.Session = session
	// Flour on water opens the dough menu and claims the use.
	give(p, fxFlour, 1)
	give(p, fxBucketWater, 1)
	event := &plugins.PluginItemOnItemEvent{Player: p, UsedItemID: fxFlour, UsedWithItemID: fxBucketWater}
	c.useItemOnItem(event)
	if !event.Handled {
		t.Error("useItemOnItem(flour, water) did not claim the use")
	}
	if len(session.menus) != 1 || session.menus[0].Title() != DoughTitle {
		t.Errorf("menus = %d, want 1 dough menu", len(session.menus))
	}
	// Pastry dough on pie dish offers the shell (most == 1:
	// starts directly, no menu).
	p2 := newTestPlayer("bob")
	p2.SetIndex(18)
	setCooking(p2, 1)
	session2 := &fakeSession{}
	p2.Session = session2
	give(p2, fxPastryDough, 1)
	give(p2, fxPieDish, 1)
	event2 := &plugins.PluginItemOnItemEvent{Player: p2, UsedItemID: fxPastryDough, UsedWithItemID: fxPieDish}
	c.useItemOnItem(event2)
	if !event2.Handled {
		t.Error("useItemOnItem(pastry, dish) did not claim the use")
	}
	if len(session2.menus) != 0 {
		t.Errorf("menus = %d, want 0 (single shell, direct start)", len(session2.menus))
	}
	driveTicks(tm, 1)
	if got := countOf(p2, fxPieShell); got != 1 {
		t.Errorf("pie shell after tick 1 = %d, want 1", got)
	}
	// Unknown pair falls through unclaimed.
	event3 := &plugins.PluginItemOnItemEvent{Player: p2, UsedItemID: fxFlour, UsedWithItemID: fxPieDish}
	c.useItemOnItem(event3)
	if event3.Handled {
		t.Error("useItemOnItem(flour, dish) claimed the use, want fall-through")
	}
	c.useItemOnItem(nil)
}

func TestPrepareGnomebowl(t *testing.T) {
	c, _, tm := newTestCooking()
	p := newTestPlayer("alice")
	p.SetIndex(19)
	setCooking(p, 42)
	session := &fakeSession{}
	p.Session = session
	give(p, fxHalfBakedBowl, 1)
	give(p, fxChocolateBar, 4)
	give(p, fxEquaLeaves, 1)
	// The half baked bowl's Prepare opens the chocolate-bomb
	// recipe (most == 1: direct start).
	event := &plugins.PluginItemActionEvent{Player: p, ItemID: fxHalfBakedBowl}
	if !c.prepareGnomebowl(event) {
		t.Error("prepareGnomebowl(half baked bowl) = false, want true")
	}
	driveTicks(tm, 1)
	if got := countOf(p, ChocolateBombBowl); got != 1 {
		t.Errorf("half made bowl after tick 1 = %d, want 1", got)
	}
	// Another bowl id does not open it (the cache name is
	// shared by other bowls — the id check matters).
	event2 := &plugins.PluginItemActionEvent{Player: p, ItemID: 99999}
	if c.prepareGnomebowl(event2) {
		t.Error("prepareGnomebowl(other bowl) = true, want false")
	}
	if c.prepareGnomebowl(nil) {
		t.Error("prepareGnomebowl(nil) = true, want false")
	}
}

func TestRegisterNilApi(t *testing.T) {
	// Register wiring is nil-safe: a nil Api wires nothing
	// (the hub never passes one, but the guard is cheap).
	// Live registration is exercised by the coordinator's
	// integration (cmd/server wiring + hub Load).
	c := newCooking(fixtureTables())
	c.register(nil)
	c.registerRecipes(nil)
	c.registerAnswerers(nil)
}
