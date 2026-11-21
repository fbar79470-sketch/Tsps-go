package cooking

import (
	"testing"

	"tsps-go/internal/game"
	"tsps-go/internal/plugins"
)

// Gnome unit tests (Gnome.Cooking.js): hand-built fixtures
// only. The unit resolves every name through the gameval
// resolver, so the test resolver carries the whole Gnome name
// set over fixture ids (600-series), plus the real numeric
// mixed/result ids from the TS tables. The ui ids are fixture
// ids too: group 700, click/create packed under it, main
// (800<<16)|1, varbits 701/702.

func wcGnomeResolver() wcMapResolver {
	click := (700 << 16) | 5
	create := (700 << 16) | 6
	main := (800 << 16) | 1
	return wcMapResolver{
		objs: map[string]int{
			"cocktail_shaker":      600,
			"pineapple":            630,
			"lemon":                631,
			"orange":               632,
			"lime":                 633,
			"vodka":                634,
			"gin":                  635,
			"dwellberries":         636,
			"lemon_slices":         637,
			"lime_chunks":          638,
			"pineapple_chunks":     639,
			"orange_slices":        640,
			"lime_slices":          641,
			"pineapple_ring":       648,
			"lemon_chunks":         649,
			"orange_chunks":        650,
			"equa_leaves":          fxEquaLeaves,
			"toads_legs":           642,
			"cheese":               643,
			"tomato":               644,
			"chocolate_bar":        fxChocolateBar,
			"chocolate_dust":       645,
			"cocktail_glass_empty": 646,
			"gnome_spice":          651,
			"gianne_dough":         652,
			"crunchy_tray":         612,
			"batta_tin":            613,
			"raw_crunchies":        614,
			"raw_batta":            615,
			"toad_crunchies":       616,
			"chocchip_crunchies":   617,
			"cheese+tom_batta":     618,
			"fruit_blast":          619,
			"pineapple_punch":      620,
			"wizard_blizzard":      621,
			"sgg":                  622,
			"drunk_dragon":         623,
			"burnt_crunchies":      610,
			"burnt_batta":          611,
			"pot_of_cream":         244,
		},
		interfaces: map[string]int{"aluft_cooking": 700},
		components: map[string]int{
			"aluft_cooking:cuisine_clickzone": click,
			"aluft_cooking:confirmation":      create,
			"toplevel_osrs_stretch:mainmodal": main,
		},
		varbits: map[string]int{
			"aluft_cooking_type":   701,
			"gnome_chosen_cuisine": 702,
		},
	}
}

// wcGnomeClickUID / wcGnomeCreateUID / wcGnomeMainUID are the
// fixture component uids in wcGnomeResolver.
const (
	wcGnomeClickUID  = (700 << 16) | 5
	wcGnomeCreateUID = (700 << 16) | 6
	wcGnomeMainUID   = (800 << 16) | 1
)

func wcGnomePlayer(t *testing.T, c *cooking) (*game.Player, *wcTestSession) {
	t.Helper()
	p := newTestPlayer("alice")
	sess := newWCTestSession()
	p.Session = sess
	setCooking(p, 99)
	return p, sess
}

func TestGnomeOpenMenu(t *testing.T) {
	c, _, _ := newTestCooking()
	u := newGnomeUnit(c, wcGnomeResolver())
	if !u.uiOK {
		t.Fatal("uiOK = false, want true (every fixture id resolves)")
	}
	p, sess := wcGnomePlayer(t, c)
	give(p, 600, 1) // cocktail shaker, slot 0
	if !u.openCocktails(&plugins.PluginItemActionEvent{Player: p, ItemID: 600, Slot: 0}) {
		t.Fatal("openCocktails = false, want true")
	}
	if got := sess.varbits[701]; got != 0 {
		t.Fatalf("type varbit = %d, want 0 (cocktails)", got)
	}
	if got := sess.varbits[702]; got != 0 {
		t.Fatalf("chosen varbit = %d, want the cocktails default 0", got)
	}
	if got := p.GetInterfaceId(); got != 700 {
		t.Fatalf("interface id = %d, want 700", got)
	}
	if _, ok := sess.mounts[wcGnomeMainUID]; !ok {
		t.Fatal("no mount at the main modal, want one")
	}
	if len(sess.flags) != 1 || sess.flags[0].uid != wcGnomeClickUID ||
		sess.flags[0].fromSlot != 0 || sess.flags[0].toSlot != 6 || sess.flags[0].flags != 2 {
		t.Fatalf("flags = %+v, want one range on the clickzone 0..6 flags 2", sess.flags)
	}
	if _, ok := u.sessions[p]; !ok {
		t.Fatal("no session stored, want one")
	}
}

func TestGnomeOpenMenuFreeWorld(t *testing.T) {
	c, _, _ := newTestCooking()
	u := newGnomeUnit(c, wcGnomeResolver())
	p, _ := wcGnomePlayer(t, c)
	c.membersWorld = false
	give(p, 600, 1)
	if u.openCocktails(&plugins.PluginItemActionEvent{Player: p, ItemID: 600, Slot: 0}) {
		t.Fatal("openCocktails on a free world = true, want false")
	}
	if _, ok := u.sessions[p]; ok {
		t.Fatal("session stored on a free world, want none")
	}
}

func TestGnomeSelectAndCreateCocktail(t *testing.T) {
	c, emitted, _ := newTestCooking()
	u := newGnomeUnit(c, wcGnomeResolver())
	p, sess := wcGnomePlayer(t, c)
	setCooking(p, 6)
	give(p, 600, 1) // shaker, slot 0
	give(p, 630, 1) // pineapple
	give(p, 631, 1) // lemon
	give(p, 632, 1) // orange
	if !u.openCocktails(&plugins.PluginItemActionEvent{Player: p, ItemID: 600, Slot: 0}) {
		t.Fatal("openCocktails = false, want true")
	}
	u.interfaceAction(&plugins.InterfaceActionClickEvent{Player: p, ButtonID: wcGnomeClickUID, Action: 1, Slot: 0, HasSlot: true})
	if got := sess.varbits[702]; got != 0 {
		t.Fatalf("chosen varbit after select = %d, want 0 (fruit blast selector)", got)
	}
	u.interfaceAction(&plugins.InterfaceActionClickEvent{Player: p, ButtonID: wcGnomeCreateUID, Action: 1})
	if got := countOf(p, 9568); got != 1 {
		t.Fatalf("mixed blast = %d, want 1", got)
	}
	if got := countOf(p, 600); got != 0 {
		t.Fatalf("shaker = %d, want 0 (spent)", got)
	}
	if got := countOf(p, 630); got != 0 {
		t.Fatalf("pineapple = %d, want 0 (spent)", got)
	}
	if got := wcXP(p); got != 30 {
		t.Fatalf("xp = %d, want 30", got)
	}
	if _, ok := u.sessions[p]; ok {
		t.Fatal("session still stored after create, want none")
	}
	if got := p.GetInterfaceId(); got != -1 {
		t.Fatalf("interface id = %d, want -1 (menu closed)", got)
	}
	if _, ok := sess.mounts[wcGnomeMainUID]; ok {
		t.Fatal("mount still present after create, want it closed")
	}
	produced := wcProduced(emitted)
	if len(produced) != 1 || produced[0].Kind != "make" || produced[0].ItemID != 9568 || produced[0].XP != 30 {
		t.Fatalf("produced = %+v, want one make event for mixed blast / 30 xp", produced)
	}
}

func TestGnomeSelectNullClears(t *testing.T) {
	c, _, _ := newTestCooking()
	u := newGnomeUnit(c, wcGnomeResolver())
	p, _ := wcGnomePlayer(t, c)
	setCooking(p, 99)
	give(p, 600, 1)
	give(p, 630, 1)
	give(p, 631, 1)
	give(p, 632, 1)
	if !u.openCocktails(&plugins.PluginItemActionEvent{Player: p, ItemID: 600, Slot: 0}) {
		t.Fatal("openCocktails = false, want true")
	}
	// Slot 5 is a null row in the cocktails menu: selecting it
	// clears server state.
	u.interfaceAction(&plugins.InterfaceActionClickEvent{Player: p, ButtonID: wcGnomeClickUID, Action: 1, Slot: 5, HasSlot: true})
	u.interfaceAction(&plugins.InterfaceActionClickEvent{Player: p, ButtonID: wcGnomeCreateUID, Action: 1})
	if got := countOf(p, 9568); got != 0 {
		t.Fatalf("mixed blast = %d, want 0 (no selection)", got)
	}
	if got := countOf(p, 600); got != 1 {
		t.Fatalf("shaker = %d, want 1 (kept)", got)
	}
	if _, ok := u.sessions[p]; !ok {
		t.Fatal("session dropped after a null create, want it open")
	}
}

func TestGnomeCreateLevelGate(t *testing.T) {
	c, _, _ := newTestCooking()
	u := newGnomeUnit(c, wcGnomeResolver())
	p, _ := wcGnomePlayer(t, c)
	setCooking(p, 5) // fruit blast needs 6
	give(p, 600, 1)
	give(p, 630, 1)
	give(p, 631, 1)
	give(p, 632, 1)
	if !u.openCocktails(&plugins.PluginItemActionEvent{Player: p, ItemID: 600, Slot: 0}) {
		t.Fatal("openCocktails = false, want true")
	}
	u.interfaceAction(&plugins.InterfaceActionClickEvent{Player: p, ButtonID: wcGnomeClickUID, Action: 1, Slot: 0, HasSlot: true})
	u.interfaceAction(&plugins.InterfaceActionClickEvent{Player: p, ButtonID: wcGnomeCreateUID, Action: 1})
	if got := countOf(p, 9568); got != 0 {
		t.Fatalf("mixed blast = %d, want 0 (level gate, silent)", got)
	}
	if got := countOf(p, 600); got != 1 {
		t.Fatalf("shaker = %d, want 1 (kept)", got)
	}
}

func TestGnomeBattaSpiceGate(t *testing.T) {
	c, _, _ := newTestCooking()
	u := newGnomeUnit(c, wcGnomeResolver())
	p, sess := wcGnomePlayer(t, c)
	setCooking(p, 26)
	give(p, 601, 1) // half baked batta, slot 0
	give(p, fxEquaLeaves, 1)
	give(p, 642, 1) // toads legs
	give(p, 643, 1) // cheese
	if !u.openBatta(&plugins.PluginItemActionEvent{Player: p, ItemID: 601, Slot: 0}) {
		t.Fatal("openBatta = false, want true")
	}
	if got := sess.varbits[701]; got != 1 {
		t.Fatalf("type varbit = %d, want 1 (batta)", got)
	}
	if got := sess.varbits[702]; got != 7 {
		t.Fatalf("chosen varbit = %d, want the batta default 7", got)
	}
	// Slot 1 is the spice-requiring 9482 row.
	u.interfaceAction(&plugins.InterfaceActionClickEvent{Player: p, ButtonID: wcGnomeClickUID, Action: 1, Slot: 1, HasSlot: true})
	u.interfaceAction(&plugins.InterfaceActionClickEvent{Player: p, ButtonID: wcGnomeCreateUID, Action: 1})
	if got := countOf(p, 9482); got != 0 {
		t.Fatalf("batta = %d, want 0 (no gnome spice)", got)
	}
	give(p, 651, 1) // gnome spice
	u.interfaceAction(&plugins.InterfaceActionClickEvent{Player: p, ButtonID: wcGnomeCreateUID, Action: 1})
	if got := countOf(p, 9482); got != 1 {
		t.Fatalf("batta = %d, want 1 (with spice)", got)
	}
	if got := wcXP(p); got != 40 {
		t.Fatalf("xp = %d, want 40", got)
	}
}

func TestGnomePour(t *testing.T) {
	c, emitted, _ := newTestCooking()
	u := newGnomeUnit(c, wcGnomeResolver())
	p, _ := wcGnomePlayer(t, c)
	give(p, 9568, 1) // mixed blast, slot 0
	give(p, 646, 1)  // empty cocktail glass
	give(p, 637, 1)  // lemon slices
	if !u.pour(&plugins.PluginItemActionEvent{Player: p, ItemID: 9568, Slot: 0}) {
		t.Fatal("pour = false, want true")
	}
	if got := countOf(p, 619); got != 1 {
		t.Fatalf("fruit blast = %d, want 1", got)
	}
	if got := countOf(p, 600); got != 1 {
		t.Fatalf("shaker = %d, want 1 (returned)", got)
	}
	if got := countOf(p, 9568); got != 0 {
		t.Fatalf("mixed blast = %d, want 0 (spent)", got)
	}
	if got := wcXP(p); got != 20 {
		t.Fatalf("xp = %d, want 20 (finish xp)", got)
	}
	produced := wcProduced(emitted)
	if len(produced) != 1 || produced[0].ItemID != 619 || produced[0].XP != 20 {
		t.Fatalf("produced = %+v, want one make event for fruit blast / 20 xp", produced)
	}
}

func TestGnomeGarnishDragon(t *testing.T) {
	c, emitted, _ := newTestCooking()
	u := newGnomeUnit(c, wcGnomeResolver())
	p, _ := wcGnomePlayer(t, c)
	give(p, 9575, 1) // mixed dragon, poured
	give(p, 639, 1)  // pineapple chunks
	give(p, 244, 1)  // pot of cream
	if !u.garnishDragon(&plugins.PluginItemActionEvent{Player: p, ItemID: 9575, Slot: 0}) {
		t.Fatal("garnishDragon = false, want true")
	}
	if got := countOf(p, 9576); got != 1 {
		t.Fatalf("garnished dragon = %d, want 1", got)
	}
	if got := countOf(p, 9575); got != 0 {
		t.Fatalf("poured dragon = %d, want 0", got)
	}
	if got := wcXP(p); got != 0 {
		t.Fatalf("xp = %d, want 0 (dragon finish pays none)", got)
	}
	produced := wcProduced(emitted)
	if len(produced) != 1 || produced[0].ItemID != 9576 || produced[0].XP != 0 {
		t.Fatalf("produced = %+v, want one make event for 9576 / 0 xp", produced)
	}
}

func TestGnomeEmptyBurnt(t *testing.T) {
	c, emitted, _ := newTestCooking()
	u := newGnomeUnit(c, wcGnomeResolver())
	p, _ := wcGnomePlayer(t, c)
	give(p, 610, 1) // burnt crunchies
	if !u.emptyBurnt(&plugins.PluginItemActionEvent{Player: p, ItemID: 610, Slot: 0}) {
		t.Fatal("emptyBurnt(crunchies) = false, want true")
	}
	if got := countOf(p, 612); got != 1 {
		t.Fatalf("crunchy tray = %d, want 1", got)
	}
	give(p, 611, 1) // burnt batta: lands in slot 1 (the tray took slot 0)
	if !u.emptyBurnt(&plugins.PluginItemActionEvent{Player: p, ItemID: 611, Slot: 1}) {
		t.Fatal("emptyBurnt(batta) = false, want true")
	}
	if got := countOf(p, 613); got != 1 {
		t.Fatalf("batta tin = %d, want 1", got)
	}
	// kind "" → no skills:produced emit.
	if got := len(wcProduced(emitted)); got != 0 {
		t.Fatalf("produced = %d events, want none", got)
	}
}

func TestGnomeUseItemsDoughAndFinish(t *testing.T) {
	c, emitted, _ := newTestCooking()
	u := newGnomeUnit(c, wcGnomeResolver())
	p, _ := wcGnomePlayer(t, c)
	give(p, 652, 1) // gianne dough
	give(p, 612, 1) // crunchy tray
	u.useItems(wcItemOnItem(p, 652, 612))
	if got := countOf(p, 614); got != 1 {
		t.Fatalf("raw crunchies = %d, want 1", got)
	}
	give(p, 9582, 1)         // lands in slot 1 (raw crunchies took slot 0)
	give(p, fxEquaLeaves, 1) // slot 2
	u.useItems(wcItemOnItemSlots(p, 9582, 1, fxEquaLeaves, 2))
	if got := countOf(p, 616); got != 1 {
		t.Fatalf("toad crunchies = %d, want 1", got)
	}
	if got := wcXP(p); got != 40 {
		t.Fatalf("xp = %d, want 40 (finish xp)", got)
	}
	produced := wcProduced(emitted)
	if len(produced) != 2 {
		t.Fatalf("produced = %d events, want 2", len(produced))
	}
}

func TestGnomeFruitCutPrompt(t *testing.T) {
	c, emitted, _ := newTestCooking()
	u := newGnomeUnit(c, wcGnomeResolver())
	p, _ := wcGnomePlayer(t, c)
	var gotPairs []any
	u.prompt = func(player *game.Player, title string, pairs ...any) bool {
		gotPairs = pairs
		return true
	}
	give(p, fxKnife, 1) // slot 0
	give(p, 630, 1)     // pineapple, slot 1
	u.useItems(wcItemOnItem(p, fxKnife, 630))
	if len(gotPairs) != 4 {
		t.Fatalf("prompt pairs = %d, want 4 (Dice/callback, Slice/callback)", len(gotPairs))
	}
	if gotPairs[0] != "Dice" || gotPairs[2] != "Slice" {
		t.Fatalf("prompt labels = %v, want [Dice _ Slice _]", gotPairs)
	}
	if _, ok := u.cuts[p]; !ok {
		t.Fatal("no cut token stored, want one")
	}
	// Take the Dice callback: pineapple → pineapple_chunks ×1.
	dice, ok := gotPairs[1].(plugins.PromptCallback)
	if !ok {
		t.Fatalf("dice callback = %T, want plugins.PromptCallback", gotPairs[1])
	}
	dice(p, 0, "Dice")
	if got := countOf(p, 639); got != 1 {
		t.Fatalf("pineapple chunks = %d, want 1", got)
	}
	if got := countOf(p, 630); got != 0 {
		t.Fatalf("pineapple = %d, want 0 (cut)", got)
	}
	if _, ok := u.cuts[p]; ok {
		t.Fatal("cut token still stored after the cut, want none")
	}
	produced := wcProduced(emitted)
	if len(produced) != 1 || produced[0].ItemID != 639 {
		t.Fatalf("produced = %+v, want one make event for pineapple chunks", produced)
	}
}

func TestGnomeFruitCutPromptDeclined(t *testing.T) {
	c, _, _ := newTestCooking()
	u := newGnomeUnit(c, wcGnomeResolver())
	p, _ := wcGnomePlayer(t, c)
	u.prompt = func(player *game.Player, title string, pairs ...any) bool { return false }
	give(p, fxKnife, 1)
	give(p, 630, 1)
	u.useItems(wcItemOnItem(p, fxKnife, 630))
	if _, ok := u.cuts[p]; ok {
		t.Fatal("cut token stored after a declined prompt, want none")
	}
	if got := countOf(p, 630); got != 1 {
		t.Fatalf("pineapple = %d, want 1 (kept)", got)
	}
}

func TestGnomeInterfaceClosed(t *testing.T) {
	c, _, _ := newTestCooking()
	u := newGnomeUnit(c, wcGnomeResolver())
	p, sess := wcGnomePlayer(t, c)
	give(p, 600, 1)
	if !u.openCocktails(&plugins.PluginItemActionEvent{Player: p, ItemID: 600, Slot: 0}) {
		t.Fatal("openCocktails = false, want true")
	}
	// The mount is still live: a close event for the group
	// keeps the session (another listener may have reopened).
	u.interfaceClosed(game.InterfaceClosedEvent{Player: p, InterfaceID: 700})
	if _, ok := u.sessions[p]; !ok {
		t.Fatal("session dropped while the mount is live, want it kept")
	}
	// The mount is gone: the close drops the session.
	delete(sess.mounts, wcGnomeMainUID)
	u.interfaceClosed(game.InterfaceClosedEvent{Player: p, InterfaceID: 700})
	if _, ok := u.sessions[p]; ok {
		t.Fatal("session kept after the mount closed, want it dropped")
	}
}

func TestGnomeClearPlayer(t *testing.T) {
	c, _, _ := newTestCooking()
	u := newGnomeUnit(c, wcGnomeResolver())
	p, _ := wcGnomePlayer(t, c)
	give(p, 600, 1)
	if !u.openCocktails(&plugins.PluginItemActionEvent{Player: p, ItemID: 600, Slot: 0}) {
		t.Fatal("openCocktails = false, want true")
	}
	u.clearPlayer(p)
	if _, ok := u.sessions[p]; ok {
		t.Fatal("session kept after clearPlayer, want none")
	}
}

func TestGnomeInterfaceActionWrongGroup(t *testing.T) {
	c, _, _ := newTestCooking()
	u := newGnomeUnit(c, wcGnomeResolver())
	p, _ := wcGnomePlayer(t, c)
	give(p, 600, 1)
	give(p, 630, 1)
	give(p, 631, 1)
	give(p, 632, 1)
	if !u.openCocktails(&plugins.PluginItemActionEvent{Player: p, ItemID: 600, Slot: 0}) {
		t.Fatal("openCocktails = false, want true")
	}
	// A click on another group (701<<16) must not select.
	u.interfaceAction(&plugins.InterfaceActionClickEvent{Player: p, ButtonID: (701 << 16) | 5, Action: 1, Slot: 0, HasSlot: true})
	u.interfaceAction(&plugins.InterfaceActionClickEvent{Player: p, ButtonID: wcGnomeCreateUID, Action: 1})
	if got := countOf(p, 9568); got != 0 {
		t.Fatalf("mixed blast = %d, want 0 (no selection made)", got)
	}
}
