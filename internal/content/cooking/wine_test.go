package cooking

import (
	"testing"

	"tsps-go/internal/game"
	"tsps-go/internal/plugins"
)

// Wine unit tests (Wine.Cooking.js): hand-built fixtures only;
// no live cache. Fixture ids come from helpers_test.go
// (fxGrapes 300, fxUnfermented 301, fxWine 302, fxBadWine 303,
// fxJugWater 214, fxKnife 946).

func wcWineResolver() wcMapResolver {
	return wcMapResolver{
		seqs:      map[string]int{"human_make_wine": 9101},
		spotanims: map[string]int{"cooking_make_wine_spotanim": 9201},
	}
}

func TestWineFor(t *testing.T) {
	c, _, _ := newTestCooking()
	w := newWineUnit(c, wcWineResolver())
	if got := w.wineFor(fxJugWater, fxGrapes); got == nil || got.GrapesID != fxGrapes {
		t.Fatalf("wineFor(water, grapes) = %v, want the grapes wine", got)
	}
	if got := w.wineFor(fxGrapes, fxJugWater); got == nil || got.GrapesID != fxGrapes {
		t.Fatalf("wineFor(grapes, water) = %v, want the grapes wine", got)
	}
	if got := w.wineFor(fxGrapes, fxWine); got != nil {
		t.Fatalf("wineFor(grapes, wine) = %v, want nil", got)
	}
	if got := w.wineFor(fxKnife, fxGrapes); got != nil {
		t.Fatalf("wineFor(knife, grapes) = %v, want nil", got)
	}
}

func TestWineRecipe(t *testing.T) {
	c, _, _ := newTestCooking()
	w := newWineUnit(c, wcWineResolver())
	wine := c.tables.Wines[0]
	recipe := w.wineRecipe(wine)
	if recipe.ProductID != fxUnfermented || recipe.Level != 35 || recipe.XP != 0 {
		t.Fatalf("wineRecipe = product %d level %d xp %v, want 301/35/0", recipe.ProductID, recipe.Level, recipe.XP)
	}
	if len(recipe.Ingredients) != 2 || recipe.Ingredients[0].ID != fxGrapes || recipe.Ingredients[0].Count != 1 ||
		recipe.Ingredients[1].ID != fxJugWater || recipe.Ingredients[1].Count != 1 {
		t.Fatalf("wineRecipe ingredients = %v, want [grapes 1, jug water 1]", recipe.Ingredients)
	}
	if len(recipe.Returns) != 0 || len(recipe.Timing) != 2 || recipe.Timing[0] != 2 || recipe.Timing[1] != 2 {
		t.Fatalf("wineRecipe returns/timing = %v/%v, want []/[2 2]", recipe.Returns, recipe.Timing)
	}
}

func TestWineSuccessChance(t *testing.T) {
	c, _, _ := newTestCooking()
	w := newWineUnit(c, wcWineResolver())
	wine := c.tables.Wines[0] // chance [48, 352]
	if got := w.wineSuccessChance(newTestPlayer("alice"), wine); got != 49.0/256 {
		t.Fatalf("successChance at level 1 = %v, want %v", got, 49.0/256)
	}
	p := newTestPlayer("alice")
	setCooking(p, 99)
	if got := w.wineSuccessChance(p, wine); got != 1 {
		t.Fatalf("successChance at level 99 = %v, want 1", got)
	}
	setCooking(p, 50)
	if got := w.wineSuccessChance(p, wine); got != 201.0/256 {
		t.Fatalf("successChance at level 50 = %v, want %v", got, 201.0/256)
	}
	// The level caps at 99: a boosted level above 99 reads as 99.
	setCooking(p, 99)
	if got := w.wineSuccessChance(p, wine); got != 1 {
		t.Fatalf("successChance at level 99 (capped) = %v, want 1", got)
	}
}

func TestWineSuccesses(t *testing.T) {
	c, _, _ := newTestCooking()
	w := newWineUnit(c, nil)
	w.randFloat = func() float64 { return 0 }
	if got := w.successes(30, 1); got != 30 {
		t.Fatalf("successes(30, 1.0) = %d, want 30", got)
	}
	w.randFloat = func() float64 { return 0.999 }
	if got := w.successes(30, 0.5); got != 0 {
		t.Fatalf("successes(30, 0.5, rolls 0.999) = %d, want 0", got)
	}
	// 16 jugs: one batch of 1 (15 rolls) + 1 leftover roll.
	rolls := []float64{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0.999}
	i := 0
	w.randFloat = func() float64 {
		v := rolls[i]
		i++
		return v
	}
	if got := w.successes(16, 0.5); got != 15 {
		t.Fatalf("successes(16, 0.5, 15 good + 1 bad) = %d, want 15", got)
	}
}

func TestWineMakeOneJug(t *testing.T) {
	c, emitted, tm := newTestCooking()
	w := newWineUnit(c, wcWineResolver())
	p := newTestPlayer("alice")
	sess := newWCTestSession()
	p.Session = sess
	setCooking(p, 99)
	give(p, fxGrapes, 1)
	give(p, fxJugWater, 1)
	w.makeWine(wcItemOnItem(p, fxGrapes, fxJugWater))
	// One jug to make: start directly, no menu.
	if len(sess.menus) != 0 {
		t.Fatalf("menus = %d, want 0 (one jug starts directly)", len(sess.menus))
	}
	// The cycle hook plays the animation as the menu closes.
	if anim := p.Animation(); anim == nil || anim.ID != 9101 {
		t.Fatalf("animation = %v, want 9101", anim)
	}
	if g := p.Graphic(); g == nil || g.ID != 9201 {
		t.Fatalf("graphic = %v, want 9201", g)
	}
	driveTicks(tm, 2)
	if got := countOf(p, fxUnfermented); got != 1 {
		t.Fatalf("unfermented = %d, want 1", got)
	}
	if got := countOf(p, fxGrapes); got != 0 {
		t.Fatalf("grapes = %d, want 0", got)
	}
	if len(sess.sounds) != 1 || sess.sounds[0] != makeWineSound {
		t.Fatalf("sounds = %v, want [2489]", sess.sounds)
	}
	if len(sess.spam) != 1 || sess.spam[0] != fermentMessage {
		t.Fatalf("spam = %v, want the ferment line once", sess.spam)
	}
	if got := w.fermenting[p]; got != fermentTicks {
		t.Fatalf("fermenting timer = %d, want %d", got, fermentTicks)
	}
	if got := wcXP(p); got != 0 {
		t.Fatalf("xp = %d, want 0 (unfermented pays none)", got)
	}
	_ = emitted
}

func TestWineMakeTwoJugsViaMenu(t *testing.T) {
	c, _, tm := newTestCooking()
	w := newWineUnit(c, wcWineResolver())
	p := newTestPlayer("alice")
	sess := newWCTestSession()
	p.Session = sess
	setCooking(p, 99)
	give(p, fxGrapes, 2)
	give(p, fxJugWater, 2)
	w.makeWine(wcItemOnItem(p, fxGrapes, fxJugWater))
	if len(sess.menus) != 1 {
		t.Fatalf("menus = %d, want 1", len(sess.menus))
	}
	menu := sess.menus[0]
	if menu.Title() != MakeTitle || len(menu.Items()) != 1 || menu.Items()[0] != fxUnfermented {
		t.Fatalf("menu = %q %v, want %q [301]", menu.Title(), menu.Items(), MakeTitle)
	}
	menu.Execute(fxUnfermented, 2)
	driveTicks(tm, 2)
	if got := countOf(p, fxUnfermented); got != 1 {
		t.Fatalf("unfermented after 2 ticks = %d, want 1", got)
	}
	driveTicks(tm, 2)
	if got := countOf(p, fxUnfermented); got != 2 {
		t.Fatalf("unfermented after 4 ticks = %d, want 2", got)
	}
	if len(sess.sounds) != 2 {
		t.Fatalf("sounds = %d, want 2 (one per jug)", len(sess.sounds))
	}
}

func TestWineFermentsAfter20Ticks(t *testing.T) {
	c, emitted, tm := newTestCooking()
	w := newWineUnit(c, wcWineResolver())
	w.randFloat = func() float64 { return 0 } // every jug good
	p := newTestPlayer("alice")
	setCooking(p, 99)
	give(p, fxUnfermented, 1)
	w.queueFerment(p)
	driveTicks(tm, fermentTicks-1)
	if got := countOf(p, fxWine); got != 0 {
		t.Fatalf("wine after 19 ticks = %d, want 0", got)
	}
	driveTicks(tm, 1)
	if got := countOf(p, fxWine); got != 1 {
		t.Fatalf("wine after 20 ticks = %d, want 1", got)
	}
	if got := countOf(p, fxUnfermented); got != 0 {
		t.Fatalf("unfermented after 20 ticks = %d, want 0", got)
	}
	if got := wcXP(p); got != 200 {
		t.Fatalf("xp = %d, want 200", got)
	}
	produced := wcProduced(emitted)
	if len(produced) != 1 || produced[0].Kind != "ferment" || produced[0].ItemID != fxWine ||
		produced[0].Amount != 1 || produced[0].XP != 200 {
		t.Fatalf("produced = %+v, want one ferment event for 1 wine / 200 xp", produced)
	}
}

func TestWineFermentCarriedAndBanked(t *testing.T) {
	c, _, tm := newTestCooking()
	// The unfermented stack must stack in the bank: give the
	// player a defs source that stacks it.
	p := newTestPlayer("alice")
	p.SetItemDefinitions(fakeItemDefs{stackable: map[int]bool{fxUnfermented: true}})
	w := newWineUnit(c, wcWineResolver())
	w.randFloat = func() float64 { return 0 }
	setCooking(p, 99)
	give(p, fxUnfermented, 1)
	p.Bank(0).AddItem(game.NewItem(fxUnfermented, 40))
	w.queueFerment(p)
	driveTicks(tm, fermentTicks)
	if got := countOf(p, fxWine); got != 1 {
		t.Fatalf("carried wine = %d, want 1", got)
	}
	if got := p.Bank(0).GetAmount(fxWine); got != 40 {
		t.Fatalf("banked wine = %d, want 40", got)
	}
	if got := p.Bank(0).GetAmount(fxUnfermented); got != 0 {
		t.Fatalf("banked unfermented = %d, want 0", got)
	}
	if got := wcXP(p); got != 41*200 {
		t.Fatalf("xp = %d, want %d", got, 41*200)
	}
}

func TestWineFermentBankNoRoomKeepsBanked(t *testing.T) {
	c, _, tm := newTestCooking()
	p := newTestPlayer("alice")
	p.SetItemDefinitions(fakeItemDefs{stackable: map[int]bool{fxUnfermented: true}})
	w := newWineUnit(c, wcWineResolver())
	// Rolls: 6 total jugs at chance ~0.606 → 3 good, 3 bad, so
	// the banked outcome needs TWO new stacks (wine + bad)
	// while only the unfermented stack's slot is free.
	rolls := []float64{0, 0, 0, 1, 1, 1, 0}
	i := 0
	w.randFloat = func() float64 {
		v := rolls[i]
		i++
		return v
	}
	setCooking(p, 35) // chance [48,352] at 35 ≈ 0.606
	give(p, fxUnfermented, 1)
	bank := p.Bank(0)
	bank.AddItem(game.NewItem(fxUnfermented, 5))
	// Fill the tab to MaxBankSlots: GetFreeSlots counts the
	// growth headroom up to 1410, so 352 items still leave
	// room.
	for j := 0; j < game.MaxBankSlots-1; j++ {
		bank.AddItem(game.NewItem(1000+j, 1))
	}
	if got := bank.GetFreeSlots(); got != 0 {
		t.Fatalf("bank free slots = %d, want 0", got)
	}
	w.queueFerment(p)
	driveTicks(tm, fermentTicks)
	// Carried converts (1 wine); the bank keeps its jugs.
	if got := countOf(p, fxWine); got != 1 {
		t.Fatalf("carried wine = %d, want 1", got)
	}
	if got := bank.GetAmount(fxUnfermented); got != 5 {
		t.Fatalf("banked unfermented = %d, want 5 (kept for the next timer)", got)
	}
	if got := bank.GetAmount(fxWine); got != 0 {
		t.Fatalf("banked wine = %d, want 0", got)
	}
	if got := wcXP(p); got != 200 {
		t.Fatalf("xp = %d, want 200 (carried only)", got)
	}
}

func TestWinePendingLimit(t *testing.T) {
	c, _, tm := newTestCooking()
	p := newTestPlayer("alice")
	p.SetItemDefinitions(fakeItemDefs{stackable: map[int]bool{fxUnfermented: true}})
	w := newWineUnit(c, wcWineResolver())
	setCooking(p, 99)
	sess := newWCTestSession()
	p.Session = sess
	// 59998 banked + 1 carried = 59999 pending; 3 grapes + 3 water.
	p.Bank(0).AddItem(game.NewItem(fxUnfermented, 59998))
	give(p, fxUnfermented, 1)
	give(p, fxGrapes, 3)
	give(p, fxJugWater, 3)
	w.makeWine(wcItemOnItem(p, fxGrapes, fxJugWater))
	if len(sess.menus) != 1 {
		t.Fatalf("menus = %d, want 1", len(sess.menus))
	}
	sess.menus[0].Execute(fxUnfermented, 3)
	driveTicks(tm, 2)
	if got := w.pendingWine(p, c.tables.Wines[0]); got != 60000 {
		t.Fatalf("pending after first jug = %d, want 60000", got)
	}
	if got := countOf(p, fxGrapes); got != 2 {
		t.Fatalf("grapes after first jug = %d, want 2", got)
	}
	driveTicks(tm, 4)
	if got := w.pendingWine(p, c.tables.Wines[0]); got != 60000 {
		t.Fatalf("pending after 6 ticks = %d, want 60000 (refused jugs spend nothing)", got)
	}
	if got := countOf(p, fxGrapes); got != 2 {
		t.Fatalf("grapes after 6 ticks = %d, want 2", got)
	}
}

func TestWineResumeAndForget(t *testing.T) {
	c, _, tm := newTestCooking()
	w := newWineUnit(c, wcWineResolver())
	w.randFloat = func() float64 { return 0 }
	p := newTestPlayer("alice")
	setCooking(p, 99)
	give(p, fxUnfermented, 1)
	// Login with an unfermented jug resumes the timer.
	w.resumeFermenting(&plugins.PlayerLoginEvent{Player: p, Username: "alice"})
	if _, ok := w.fermenting[p]; !ok {
		t.Fatal("resumeFermenting did not queue a timer")
	}
	// Disconnect forgets it: nothing ferments.
	w.forget(&plugins.PlayerDisconnectEvent{Player: p, Username: "alice"})
	driveTicks(tm, fermentTicks)
	if got := countOf(p, fxWine); got != 0 {
		t.Fatalf("wine after forgotten timer = %d, want 0", got)
	}
	// Login again: it ferments 20 ticks later.
	w.resumeFermenting(&plugins.PlayerLoginEvent{Player: p, Username: "alice"})
	driveTicks(tm, fermentTicks)
	if got := countOf(p, fxWine); got != 1 {
		t.Fatalf("wine after resumed timer = %d, want 1", got)
	}
}

func TestWineBadWineOnFailedRoll(t *testing.T) {
	c, _, tm := newTestCooking()
	w := newWineUnit(c, wcWineResolver())
	w.randFloat = func() float64 { return 0.999 } // every roll fails
	p := newTestPlayer("alice")
	setCooking(p, 35) // chance ≈ 0.606, so 0.999 always fails
	give(p, fxUnfermented, 1)
	w.queueFerment(p)
	driveTicks(tm, fermentTicks)
	if got := countOf(p, fxBadWine); got != 1 {
		t.Fatalf("bad wine = %d, want 1", got)
	}
	if got := wcXP(p); got != 0 {
		t.Fatalf("xp = %d, want 0 (no good jugs)", got)
	}
}
