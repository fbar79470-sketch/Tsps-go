package crafting

import (
	"testing"

	"tsps-go/internal/content/fletching"
	"tsps-go/internal/game"
	"tsps-go/internal/plugins"
)

func wheelEvent(p *game.Player) *plugins.ObjectInteractionEvent {
	return &plugins.ObjectInteractionEvent{
		Player: p, Object: testGameObject(fxWheelObj, p.Location()), ObjectID: fxWheelObj,
		Definition: testObjectDef{id: fxWheelObj, name: "Spinning wheel", interactions: []string{"Spin"}},
		ClickType:  1,
	}
}

func loomEvent(p *game.Player) *plugins.ObjectInteractionEvent {
	return &plugins.ObjectInteractionEvent{
		Player: p, Object: testGameObject(fxLoomObj, p.Location()), ObjectID: fxLoomObj,
		Definition: testObjectDef{id: fxLoomObj, name: "Loom", interactions: []string{"Weave"}},
		ClickType:  1,
	}
}

func TestSpinMenuMode16OneMaterial(t *testing.T) {
	c, _, _ := newTestCrafting()
	p, session := testPlayer("alice")
	setCrafting(p, 10)
	give(p, fxFlax, 3)
	c.spin(wheelEvent(p))
	if len(session.menus) != 1 {
		t.Fatalf("menus = %d, want 1", len(session.menus))
	}
	menu := session.menus[0]
	if got := menu.Title(); got != "How many would you like to spin?" {
		t.Errorf("menu title = %q, want the one-material spin title", got)
	}
	if got := menu.Options().Mode; got != 16 {
		t.Errorf("menu mode = %d, want 16", got)
	}
}

func TestSpinNothingMessage(t *testing.T) {
	c, _, _ := newTestCrafting()
	p, session := testPlayer("alice")
	setCrafting(p, 10)
	c.spin(wheelEvent(p))
	if !session.hasMessage("You have nothing to spin.") {
		t.Errorf("messages = %v, want the nothing-to-spin line", session.messages)
	}
	if len(session.menus) != 0 {
		t.Errorf("menus = %d, want 0", len(session.menus))
	}
}

func TestSpinBowStringFirstTick2Interval3(t *testing.T) {
	c, emitted, _ := newTestCrafting()
	p, session := testPlayer("alice")
	setCrafting(p, 10)
	give(p, fxFlax, 3)
	c.spin(wheelEvent(p))
	session.menus[0].Execute(fxBowString, 3)
	driveCraftingTicks(c, 1)
	if got := countOf(p, fxBowString); got != 0 {
		t.Fatalf("bow strings after tick 1 = %d, want 0", got)
	}
	driveCraftingTicks(c, 1)
	if got := countOf(p, fxBowString); got != 1 {
		t.Fatalf("bow strings after tick 2 = %d, want 1", got)
	}
	if !hasEvent(emitted, "crafting:spun") {
		t.Error("crafting:spun was not emitted")
	}
	// 15 XP per bow string.
	if got := p.Skills().Experience(game.SkillCrafting); got != 15 {
		t.Errorf("crafting XP = %d, want 15", got)
	}
	driveCraftingTicks(c, 2)
	if got := countOf(p, fxBowString); got != 1 {
		t.Fatalf("bow strings after tick 4 = %d, want 1 (every 3 ticks)", got)
	}
	driveCraftingTicks(c, 1)
	if got := countOf(p, fxBowString); got != 2 {
		t.Fatalf("bow strings after tick 5 = %d, want 2", got)
	}
}

func TestSpunEventFiresAfterAddAndXP(t *testing.T) {
	c, _, _ := newTestCrafting()
	p, _ := testPlayer("alice")
	setCrafting(p, 10)
	give(p, fxFlax, 1)
	// Record the inventory and XP at the moment the spun
	// event fires: the bow string and its XP must already be
	// there (TS emits after add + XP).
	var spunSeen bool
	var bowStringsAtEmit, xpAtEmit int
	c.emit = func(name string, payload any) {
		if name == "crafting:spun" {
			spunSeen = true
			bowStringsAtEmit = countOf(p, fxBowString)
			xpAtEmit = p.Skills().Experience(game.SkillCrafting)
			if event, ok := payload.(*fletching.CraftingSpunEvent); ok {
				if event.Player != p || event.ItemID != fxBowString || event.Amount != 1 {
					t.Errorf("spun payload = %+v, want {alice, bow string, 1}", event)
				}
			} else {
				t.Errorf("spun payload type = %T, want *fletching.CraftingSpunEvent", payload)
			}
		}
	}
	c.spin(wheelEvent(p))
	p.Session.(*fakeSession).menus[0].Execute(fxBowString, 1)
	driveCraftingTicks(c, 2)
	if !spunSeen {
		t.Fatal("crafting:spun was not emitted")
	}
	if bowStringsAtEmit != 1 {
		t.Errorf("bow strings at emit = %d, want 1 (added before the emit)", bowStringsAtEmit)
	}
	if xpAtEmit != 15 {
		t.Errorf("crafting XP at emit = %d, want 15 (granted before the emit)", xpAtEmit)
	}
}

func TestWeavingNoSpunEvent(t *testing.T) {
	c, emitted, _ := newTestCrafting()
	p, session := testPlayer("alice")
	setCrafting(p, 10)
	give(p, fxBallOfWool, 8)
	c.weave(loomEvent(p))
	if len(session.menus) != 1 {
		t.Fatalf("menus = %d, want 1", len(session.menus))
	}
	if got := session.menus[0].Options().Mode; got != 35 {
		t.Errorf("menu mode = %d, want 35", got)
	}
	session.menus[0].Execute(fxStripOfCloth, 1)
	driveCraftingTicks(c, 3)
	if got := countOf(p, fxStripOfCloth); got != 1 {
		t.Fatalf("strips of cloth after tick 3 = %d, want 1", got)
	}
	if hasEvent(emitted, "crafting:spun") {
		t.Error("crafting:spun emitted for weaving, want wheel-only")
	}
}

func TestSpinWalkAwayStops(t *testing.T) {
	c, _, _ := newTestCrafting()
	p, session := testPlayer("alice")
	setCrafting(p, 10)
	give(p, fxFlax, 5)
	c.spin(wheelEvent(p))
	session.menus[0].Execute(fxBowString, 5)
	driveCraftingTicks(c, 2)
	if got := countOf(p, fxBowString); got != 1 {
		t.Fatalf("bow strings = %d, want 1", got)
	}
	// Walk away: the loop must stop.
	p.SetLocation(game.Location{X: 3210, Y: 3210, Z: 0})
	driveCraftingTicks(c, 6)
	if got := countOf(p, fxBowString); got != 1 {
		t.Errorf("bow strings after walking away = %d, want 1 (loop stopped)", got)
	}
	if c.sessionFor(p) != nil {
		t.Error("spinning session still active after walking away")
	}
}

func TestSpinTicksAlwaysThree(t *testing.T) {
	c, _, _ := newTestCrafting()
	p, _ := testPlayer("alice")
	setCrafting(p, 10)
	// No diary:is-complete answerer: the Seers' 2-tick rule
	// never applies.
	if got := c.spinTicks(p, nil, []int{fxFlax}); got != 3 {
		t.Errorf("spinTicks = %d, want 3", got)
	}
}

func TestSpoolAbsorbsSpunBowString(t *testing.T) {
	craftingTables, defs := loadRealTables(t)
	fletchingTables, err := fletching.LoadFromCache(defs, "../../../data/definitions/fletching.json")
	if err != nil {
		t.Fatalf("fletching.LoadFromCache: %v", err)
	}
	SetTables(craftingTables)
	fletching.SetTables(fletchingTables)
	hub := plugins.NewHub([]plugins.Plugin{Plugin, fletching.Plugin}, plugins.HubOptions{MembersWorld: true})
	hub.Load()

	p, _ := testPlayer("alice")
	setCrafting(p, 10)
	flaxID, _ := craftingTables.ItemID("FLAX")
	spoolID := fletchingTables.SpoolID
	bowStringID := fletchingTables.BowStringID
	if flaxID == 0 || spoolID == 0 || bowStringID == 0 {
		t.Fatalf("real ids unresolved: flax=%d spool=%d bow string=%d", flaxID, spoolID, bowStringID)
	}
	give(p, flaxID, 3)
	give(p, spoolID, 1)
	event := &plugins.ObjectInteractionEvent{
		Player: p, Object: testGameObject(2644, p.Location()), ObjectID: 2644,
		Definition: testObjectDef{id: 2644, name: "Spinning wheel", interactions: []string{"Spin"}},
		ClickType:  1,
	}
	if !hub.EmitObjectInteraction(event) {
		t.Fatal("EmitObjectInteraction = false for the spinning wheel")
	}
	session := p.Session.(*fakeSession)
	if len(session.menus) != 1 {
		t.Fatalf("menus = %d, want 1", len(session.menus))
	}
	session.menus[0].Execute(bowStringID, 3)
	driveCraftingTicks(shared, 2)
	charges, _ := p.GetAttribute(fletching.SpoolChargesAttribute).(int)
	if charges != 1 {
		t.Fatalf("spool charges = %d, want 1 (one spun bow string absorbed)", charges)
	}
	if got := countOf(p, bowStringID); got != 0 {
		t.Errorf("bow strings in inventory = %d, want 0 (absorbed onto the spool)", got)
	}
	driveCraftingTicks(shared, 3)
	charges, _ = p.GetAttribute(fletching.SpoolChargesAttribute).(int)
	if charges != 2 {
		t.Fatalf("spool charges after tick 5 = %d, want 2", charges)
	}
}
