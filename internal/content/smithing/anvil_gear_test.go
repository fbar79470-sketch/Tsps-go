// Anvil, cannonballs, spears and gear tests (fixtures only).
package smithing

import (
	"strings"
	"testing"

	"tsps-go/internal/game"
	"tsps-go/internal/plugins"
)

// ---- anvil ----

func anvilObject() *fakeObject {
	return &fakeObject{id: 1001, loc: game.Location{X: 3200, Y: 3200, Z: 0}}
}

func TestAnvilOpenInterface(t *testing.T) {
	s, _ := newTestSmithing(t)
	p, sess := newTestPlayer("smith")
	setLevel(p, game.SkillSmithing, 1)
	give(p, fxBronzeBar, 5)
	give(p, fxHammer, 1)
	if !s.openInterface(p, 0, anvilObject()) {
		t.Fatal("openInterface failed")
	}
	if got := p.GetInterfaceId(); got != smithingInterfaceID {
		t.Errorf("interface id = %d, want 312", got)
	}
	if got := sess.varbits[smithingBarTypeVarbit]; got != 1 {
		t.Errorf("smithing_bar_type = %d, want 1 (bronze)", got)
	}
	// All is remembered by default: the varp highlights what
	// the bars cover (5).
	if got := sess.varps[makexcraftingVarp]; got != 5 {
		t.Errorf("makexcrafting = %d, want 5", got)
	}
	if len(sess.mounts) != 1 || sess.mounts[0].groupID != smithingInterfaceID {
		t.Errorf("mounts = %v, want one mount of 312", sess.mounts)
	}
	if len(sess.flags) != lastProductComponent-firstProductComponent+1 {
		t.Errorf("interface flags = %d, want %d (components 9-38)", len(sess.flags), lastProductComponent-firstProductComponent+1)
	}
	for _, f := range sess.flags {
		if f.flags != 2 {
			t.Errorf("flags %v, want 2", f)
		}
	}
}

func TestAnvilChooseBarBestWorkable(t *testing.T) {
	s, _ := newTestSmithing(t)
	p, _ := newTestPlayer("smith")
	setLevel(p, game.SkillSmithing, 15)
	give(p, fxBronzeBar, 1)
	give(p, fxIronBar, 1)
	// Level 15 can work iron: the best workable bar wins.
	if got := s.chooseBar(p, 0); got != fxIronBar {
		t.Errorf("chooseBar = %d, want iron", got)
	}
	// Level 1 cannot work iron: bronze wins.
	setLevel(p, game.SkillSmithing, 1)
	if got := s.chooseBar(p, 0); got != fxBronzeBar {
		t.Errorf("chooseBar = %d, want bronze", got)
	}
	// A preferred bar the player carries wins outright.
	if got := s.chooseBar(p, fxIronBar); got != fxIronBar {
		t.Errorf("chooseBar(preferred iron) = %d, want iron", got)
	}
}

func TestAnvilProductLoopTiming(t *testing.T) {
	s, _ := newTestSmithing(t)
	p, sess := newTestPlayer("smith")
	setLevel(p, game.SkillSmithing, 1)
	give(p, fxBronzeBar, 3)
	give(p, fxHammer, 1)
	s.randFloat = func() float64 { return 0.9 } // no uniform roll
	if !s.openInterface(p, 0, anvilObject()) {
		t.Fatal("openInterface failed")
	}
	// Click the bronze dagger (component 9), All (3 bars).
	if !s.chooseProduct(&plugins.InterfaceActionClickEvent{
		Player: p, ButtonID: (smithingInterfaceID << 16) | 9,
	}) {
		t.Fatal("chooseProduct failed")
	}
	// The click sets the lasttype varbits and closes the
	// interface.
	if got := sess.varbits[smithingBarLastTypeVarbit]; got != 1 {
		t.Errorf("smithing_bar_lasttype = %d, want 1", got)
	}
	if got := sess.varbits[smithingItemLastTypeVarbit]; got != 1 {
		t.Errorf("smithing_item_lasttype = %d, want slotIndex 1", got)
	}
	if got := p.GetInterfaceId(); got != -1 {
		t.Errorf("interface id = %d, want -1 (closed)", got)
	}
	// First item 3 ticks after the click tick (driven tick 4:
	// nextCycle is tick+1+firstDelay), then every 5.
	driveTicks(s, 3)
	if got := p.Inventory().GetAmount(fxBronzeDagger); got != 0 {
		t.Errorf("after tick 3: daggers = %d, want 0", got)
	}
	driveTicks(s, 1)
	if got := p.Inventory().GetAmount(fxBronzeDagger); got != 1 {
		t.Errorf("after tick 4: daggers = %d, want 1", got)
	}
	if got := p.Inventory().GetAmount(fxBronzeBar); got != 2 {
		t.Errorf("after tick 4: bars = %d, want 2", got)
	}
	if got := p.Skills().Experience(game.SkillSmithing); got != 12 {
		t.Errorf("smithing XP = %d, want 12 (12.5 truncates)", got)
	}
	if !hasSpam(sess, "You hammer the bronze and make a dagger.") {
		t.Errorf("spam = %v, want the hammer line", sess.spam)
	}
	driveTicks(s, 5)
	if got := p.Inventory().GetAmount(fxBronzeDagger); got != 2 {
		t.Errorf("after tick 9: daggers = %d, want 2", got)
	}
	driveTicks(s, 5)
	if got := p.Inventory().GetAmount(fxBronzeDagger); got != 3 {
		t.Errorf("after tick 14: daggers = %d, want 3", got)
	}
	if _, ok := s.sessions[p]; ok {
		t.Error("session still active after the last dagger")
	}
}

func TestAnvilUniformRollShortensInterval(t *testing.T) {
	s, _ := newTestSmithing(t)
	p, _ := newTestPlayer("smith")
	setLevel(p, game.SkillSmithing, 1)
	give(p, fxBronzeBar, 2)
	give(p, fxHammer, 1)
	// Full uniform chance would need the uniform; force the
	// roll directly: randFloat 0.0 always rolls under any
	// non-zero chance — but with no pieces the chance is 0,
	// so equip the tunic (20%) and roll 0.1.
	wear(p, game.BodySlot, smithsTunicItemID)
	s.randFloat = func() float64 { return 0.1 }
	if got := s.smithingInterval(p); got != uniformIntervalTicks {
		t.Errorf("smithingInterval = %d, want 4 (uniform roll)", got)
	}
	s.randFloat = func() float64 { return 0.9 }
	if got := s.smithingInterval(p); got != smithIntervalTicks {
		t.Errorf("smithingInterval = %d, want 5", got)
	}
	// Full set is 100%: any roll shortens.
	if got := smithsUniformChance(p); got != 0.2 {
		t.Errorf("uniform chance (tunic) = %v, want 0.2", got)
	}
}

func TestAnvilQuantityHandling(t *testing.T) {
	s, _ := newTestSmithing(t)
	p, _ := newTestPlayer("smith")
	setLevel(p, game.SkillSmithing, 1)
	give(p, fxBronzeBar, 10)
	give(p, fxHammer, 1)
	if !s.openInterface(p, 0, anvilObject()) {
		t.Fatal("openInterface failed")
	}
	// Quantity 5 (component 4), then the dagger: 5 made.
	if !s.selectQuantity(&plugins.InterfaceActionClickEvent{
		Player: p, ButtonID: (smithingInterfaceID << 16) | 4,
	}) {
		t.Fatal("selectQuantity(5) failed")
	}
	if got := s.selectedQuantityOf(p); got != 5 {
		t.Fatalf("selected quantity = %d, want 5", got)
	}
	if !s.chooseProduct(&plugins.InterfaceActionClickEvent{
		Player: p, ButtonID: (smithingInterfaceID << 16) | 9,
	}) {
		t.Fatal("chooseProduct failed")
	}
	driveTicks(s, 24) // daggers at ticks 4, 9, 14, 19, 24
	if got := p.Inventory().GetAmount(fxBronzeDagger); got != 5 {
		t.Errorf("daggers = %d, want 5", got)
	}
	if _, ok := s.sessions[p]; ok {
		t.Error("session still active after 5 daggers")
	}

	// Quantity X (component 6) arms the entered-amount
	// prompt; answering 3 remembers 3.
	s2, _ := newTestSmithing(t)
	p2, sess2 := newTestPlayer("smith2")
	setLevel(p2, game.SkillSmithing, 1)
	give(p2, fxBronzeBar, 10)
	give(p2, fxHammer, 1)
	if !s2.openInterface(p2, 0, anvilObject()) {
		t.Fatal("openInterface failed")
	}
	if !s2.selectQuantity(&plugins.InterfaceActionClickEvent{
		Player: p2, ButtonID: (smithingInterfaceID << 16) | 6,
	}) {
		t.Fatal("selectQuantity(X) failed")
	}
	action := p2.GetEnteredAmountAction()
	if action == nil || action.Execute == nil {
		t.Fatal("no entered-amount action armed for X")
	}
	if len(sess2.packets) == 0 {
		t.Error("no prompt packet sent for X")
	}
	action.Execute(3)
	if got := s2.selectedQuantityOf(p2); got != 3 {
		t.Errorf("selected quantity = %d, want 3", got)
	}
}

func TestAnvilNoHammerAndOverLevel(t *testing.T) {
	s, _ := newTestSmithing(t)
	p, sess := newTestPlayer("smith")
	setLevel(p, game.SkillSmithing, 1)
	give(p, fxBronzeBar, 5)
	// No hammer: the smith cannot start.
	product := tables.AnvilByComponent(1, 9)
	if s.startSmithing(p, product, 1, nil) {
		t.Error("startSmithing succeeded without a hammer")
	}
	if !hasMessage(sess, "You need a hammer to work metal bars.") {
		t.Errorf("messages = %v, want the hammer line", sess.messages)
	}

	// Over-level product: the interface closes with a level
	// message box.
	p2, _ := newTestPlayer("smith2")
	setLevel(p2, game.SkillSmithing, 1)
	give(p2, fxBronzeBar, 5)
	give(p2, fxHammer, 1)
	if !s.openInterface(p2, 0, anvilObject()) {
		t.Fatal("openInterface failed")
	}
	platebody := tables.AnvilByComponent(1, 30)
	if platebody == nil || platebody.Level != 18 {
		t.Fatalf("platebody product = %v, want level 18", platebody)
	}
	if s.startSmithing(p2, platebody, 1, nil) {
		t.Error("startSmithing succeeded over level")
	}
	if got := p2.GetInterfaceId(); got != -1 {
		t.Errorf("interface id = %d, want -1 (closed on over-level)", got)
	}
	text, ok := statementText(p2)
	if !ok || !strings.Contains(text, "You need a Smithing level of 18 to make a Bronze Platebody.") {
		t.Errorf("statement = %q (active %v), want the level 18 line", text, ok)
	}
}

func TestAnvilRustedBronzeOnly(t *testing.T) {
	s, _ := newTestSmithing(t)
	s.objectNameResolver = func(id int) string { return "Rusted anvil" }
	p, sess := newTestPlayer("smith")
	setLevel(p, game.SkillSmithing, 15)
	give(p, fxIronBar, 1)
	give(p, fxHammer, 1)
	// An iron bar on the rusted anvil is refused.
	event := &plugins.ItemOnObjectEvent{
		Player: p, Object: &fakeObject{id: 200, loc: p.Location()},
		ObjectID: 200, ItemID: fxIronBar,
	}
	s.anvilUseOnAnvil(event)
	if !event.Handled {
		t.Fatal("iron-on-rusted-anvil was not handled")
	}
	if !hasMessage(sess, rustedBronzeOnlyMessage) {
		t.Errorf("messages = %v, want the bronze-only line", sess.messages)
	}
	if p.GetInterfaceId() == smithingInterfaceID {
		t.Error("interface opened for iron on the rusted anvil")
	}
	// A bronze bar opens the interface on bronze.
	give(p, fxBronzeBar, 1)
	event2 := &plugins.ItemOnObjectEvent{
		Player: p, Object: &fakeObject{id: 200, loc: p.Location()},
		ObjectID: 200, ItemID: fxBronzeBar,
	}
	s.anvilUseOnAnvil(event2)
	if got := p.GetInterfaceId(); got != smithingInterfaceID {
		t.Errorf("interface id = %d, want 312 for bronze on the rusted anvil", got)
	}
	if got := sess.varbits[smithingBarTypeVarbit]; got != 1 {
		t.Errorf("smithing_bar_type = %d, want 1 (bronze)", got)
	}
}

func TestAnvilBluriteMenu(t *testing.T) {
	s, _ := newTestSmithing(t)
	p, sess := newTestPlayer("smith")
	setLevel(p, game.SkillSmithing, 13)
	give(p, fxBluriteBar, 2)
	give(p, fxHammer, 1)
	// Blurite is not on interface 312: a CreationMenu opens
	// instead, offering bolts and limbs.
	if !s.openInterface(p, 0, anvilObject()) {
		t.Fatal("openInterface failed")
	}
	if got := p.GetInterfaceId(); got == smithingInterfaceID {
		t.Error("interface 312 opened for blurite")
	}
	if len(sess.menus) != 1 {
		t.Fatalf("menus = %d, want 1 (blurite menu)", len(sess.menus))
	}
	items := sess.menus[0].Items()
	if len(items) != 2 || items[0] != 9376 || items[1] != 9422 {
		t.Errorf("blurite menu items = %v, want [9376 9422]", items)
	}
}

func TestAnvilNoBarMessageBox(t *testing.T) {
	s, _ := newTestSmithing(t)
	p, _ := newTestPlayer("smith")
	setLevel(p, game.SkillSmithing, 1)
	if s.openInterface(p, 0, anvilObject()) {
		t.Error("openInterface succeeded with no bar")
	}
	text, ok := statementText(p)
	if !ok || text != noBarMessage {
		t.Errorf("statement = %q (active %v), want the no-bar line", text, ok)
	}
}

// ---- cannonballs ----

func TestCannonballPourAndMoulds(t *testing.T) {
	s, _ := newTestSmithing(t)
	p, sess := newTestPlayer("smith")
	setLevel(p, game.SkillSmithing, 35)
	give(p, fxSteelBar, 2)
	give(p, ammoMouldItemID, 1)
	cb := cannonballFor(fxSteelBar)
	if cb == nil {
		t.Fatal("no cannonball recipe for steel")
	}
	if !s.pour(p, cb) {
		t.Fatal("pour failed")
	}
	if got := p.Inventory().GetAmount(fxSteelBall); got != 4 {
		t.Errorf("balls = %d, want 4 (one bar)", got)
	}
	if got := p.Skills().Experience(game.SkillSmithing); got != 25 {
		t.Errorf("XP = %d, want 25 (25.6 truncates)", got)
	}
	if !hasMessage(sess, "The molten metal cools slowly to form 4 cannonballs.") {
		t.Errorf("messages = %v, want the pour line", sess.messages)
	}

	// Double mould: two bars at once, 8 balls.
	p2, _ := newTestPlayer("smith2")
	setLevel(p2, game.SkillSmithing, 35)
	give(p2, fxSteelBar, 2)
	give(p2, doubleAmmoMouldItemID, 1)
	if !s.pour(p2, cb) {
		t.Fatal("double pour failed")
	}
	if got := p2.Inventory().GetAmount(fxSteelBall); got != 8 {
		t.Errorf("double balls = %d, want 8", got)
	}
	if got := p2.Inventory().GetAmount(fxSteelBar); got != 0 {
		t.Errorf("double bars left = %d, want 0", got)
	}

	// No mould: refused.
	p3, sess3 := newTestPlayer("smith3")
	setLevel(p3, game.SkillSmithing, 35)
	give(p3, fxSteelBar, 1)
	if s.pour(p3, cb) {
		t.Error("pour succeeded without a mould")
	}
	if !hasMessage(sess3, "You need an ammo mould to make cannonballs.") {
		t.Errorf("messages = %v, want the mould line", sess3.messages)
	}
}

func TestCannonballQuestGateFailClosed(t *testing.T) {
	s, _ := newTestSmithing(t)
	p, sess := newTestPlayer("smith")
	setLevel(p, game.SkillSmithing, 35)
	give(p, fxSteelBar, 1)
	give(p, ammoMouldItemID, 1)
	cb := cannonballFor(fxSteelBar)
	// quest:is-complete is unanswered, so Dwarf Cannon fails
	// closed (gap 26c).
	if s.startCannonballs(p, cb, 1) {
		t.Error("cannonballs started without Dwarf Cannon")
	}
	if !hasMessage(sess, "You need to complete Dwarf Cannon to make cannonballs.") {
		t.Errorf("messages = %v, want the Dwarf Cannon line", sess.messages)
	}
}

// ---- spears ----

func TestSpearLockFailOpenAndMake(t *testing.T) {
	s, _ := newTestSmithing(t)
	// trained() is fail-open: no Barbarian Training quest is
	// registered in Go, so the anvil is usable.
	if !spearTrained(nil) {
		t.Error("spearTrained is not fail-open")
	}
	s.objectNameResolver = func(id int) string { return barbarianAnvilName }
	p, sess := newTestPlayer("smith")
	setLevel(p, game.SkillSmithing, 5)
	give(p, fxBronzeBar, 2)
	give(p, fxLogs, 2)
	give(p, fxHammer, 1)
	event := &plugins.ItemOnObjectEvent{
		Player: p, Object: &fakeObject{id: 300, loc: p.Location()},
		ObjectID: 300, ItemID: fxBronzeBar,
	}
	if !s.spearUseBar(event) {
		t.Fatal("bar on barbarian anvil was not handled")
	}
	if len(sess.menus) != 1 {
		t.Fatalf("menus = %d, want 1 (spear/hasta choice)", len(sess.menus))
	}
	items := sess.menus[0].Items()
	if len(items) != 2 || items[0] != fxBronzeSpear || items[1] != fxBronzeHasta {
		t.Errorf("spear menu items = %v, want [9001 9002]", items)
	}
	// Choose the spear through the menu action: 2 made.
	menu := sess.menus[0]
	menu.Execute(fxBronzeSpear, 2)
	driveTicks(s, 9) // first at tick 1 (finishDelay 0), then every 4
	if got := p.Inventory().GetAmount(fxBronzeSpear); got != 2 {
		t.Errorf("spears = %d, want 2", got)
	}
	if got := p.Inventory().GetAmount(fxBronzeBar); got != 0 {
		t.Errorf("bars left = %d, want 0", got)
	}
	if got := p.Inventory().GetAmount(fxLogs); got != 0 {
		t.Errorf("logs left = %d, want 0", got)
	}
	if got := p.Skills().Experience(game.SkillSmithing); got != 50 {
		t.Errorf("XP = %d, want 50 (25 x 2)", got)
	}

	// Without a hammer the anvil refuses.
	p2, sess2 := newTestPlayer("smith2")
	setLevel(p2, game.SkillSmithing, 5)
	give(p2, fxBronzeBar, 1)
	give(p2, fxLogs, 1)
	event2 := &plugins.ItemOnObjectEvent{
		Player: p2, Object: &fakeObject{id: 300, loc: p2.Location()},
		ObjectID: 300, ItemID: fxBronzeBar,
	}
	if !s.spearUseBar(event2) {
		t.Fatal("bar on barbarian anvil was not handled")
	}
	if !hasMessage(sess2, noHammerBarbarian) {
		t.Errorf("messages = %v, want the hammer line", sess2.messages)
	}
}

// ---- gear ----

func TestRingOfForgingChargesAndMelt(t *testing.T) {
	s, emitted := newTestSmithing(t)
	// Wire Gear's answerer into the test emitter, the way the
	// hub would: smelt-outcome events run smeltOutcome.
	s.emit = func(name string, payload any) {
		*emitted = append(*emitted, emittedEvent{name, payload})
		if name == "smithing:smelt-outcome" {
			s.smeltOutcome(payload)
		}
	}
	p, sess := newTestPlayer("smith")
	setLevel(p, game.SkillSmithing, 15)
	give(p, fxIronOre, 2)
	wear(p, game.RingSlot, ringOfForgingItemID)
	// randFloat 0.9 would fail iron; the ring forces success.
	s.randFloat = func() float64 { return 0.9 }
	if !s.startSmelting(p, smeltRecipe(tables, fxIronBar), 1, -1) {
		t.Fatal("startSmelting failed")
	}
	driveTicks(s, 5)
	if got := p.Inventory().GetAmount(fxIronBar); got != 1 {
		t.Errorf("ring iron: bars = %d, want 1 (never fails)", got)
	}
	if got := ringCharges(p); got != 139 {
		t.Errorf("ring charges = %d, want 139", got)
	}

	// Check reports the charges.
	if !s.checkRing(&plugins.PluginItemActionEvent{Player: p}) {
		t.Fatal("checkRing failed")
	}
	if !hasMessage(sess, "You can smelt 139 more pieces of iron ore") {
		t.Errorf("messages = %v, want the 139-pieces line", sess.messages)
	}

	// One charge left: the next iron bar melts the ring and
	// stops the session.
	p.SetAttribute(ringChargesAttribute, 1)
	give(p, fxIronOre, 3)
	if !s.startSmelting(p, smeltRecipe(tables, fxIronBar), 3, -1) {
		t.Fatal("startSmelting failed")
	}
	driveTicks(s, 5)
	if ring := p.Equipment().ItemAt(game.RingSlot); ring.IsValid() {
		t.Errorf("ring slot = %v, want empty (melted)", ring)
	}
	if !hasMessage(sess, "Your Ring of Forging has melted.") {
		t.Errorf("messages = %v, want the melted line", sess.messages)
	}
	if _, ok := s.sessions[p]; ok {
		t.Error("session still active after the ring melted")
	}
	// The melt resets the charges for the next ring.
	if got := ringCharges(p); got != ringChargesTotal {
		t.Errorf("ring charges after melt = %d, want 140", got)
	}
}

func TestVarrockArmourDoubleBarEdgevilleOnly(t *testing.T) {
	s, emitted := newTestSmithing(t)
	s.emit = func(name string, payload any) {
		*emitted = append(*emitted, emittedEvent{name, payload})
		if name == "smithing:smelt-outcome" {
			s.smeltOutcome(payload)
		}
	}
	// At the Edgeville furnace (16469) with Varrock armour 1
	// and a 0.05 roll (< 0.1): two bars from two ores.
	p, _ := newTestPlayer("smith")
	setLevel(p, game.SkillSmithing, 15)
	give(p, fxIronOre, 2)
	wear(p, game.BodySlot, varrockArmour1ItemID)
	s.randFloat = func() float64 { return 0.05 }
	if !s.startSmelting(p, smeltRecipe(tables, fxIronBar), 1, edgevilleFurnaceLocID) {
		t.Fatal("startSmelting failed")
	}
	driveTicks(s, 5)
	if got := p.Inventory().GetAmount(fxIronBar); got != 2 {
		t.Errorf("Edgeville bars = %d, want 2", got)
	}
	if got := p.Inventory().GetAmount(fxIronOre); got != 0 {
		t.Errorf("Edgeville ore left = %d, want 0", got)
	}
	if got := p.Skills().Experience(game.SkillSmithing); got != 25 {
		t.Errorf("Edgeville XP = %d, want 25 (12.5 x 2)", got)
	}

	// Elsewhere the same roll smelts one bar.
	s2, emitted2 := newTestSmithing(t)
	s2.emit = func(name string, payload any) {
		*emitted2 = append(*emitted2, emittedEvent{name, payload})
		if name == "smithing:smelt-outcome" {
			s2.smeltOutcome(payload)
		}
	}
	p2, _ := newTestPlayer("smith2")
	setLevel(p2, game.SkillSmithing, 15)
	give(p2, fxIronOre, 2)
	wear(p2, game.BodySlot, varrockArmour1ItemID)
	s2.randFloat = func() float64 { return 0.05 }
	if !s2.startSmelting(p2, smeltRecipe(tables, fxIronBar), 1, -1) {
		t.Fatal("startSmelting failed")
	}
	driveTicks(s2, 5)
	if got := p2.Inventory().GetAmount(fxIronBar); got != 1 {
		t.Errorf("elsewhere bars = %d, want 1", got)
	}
}
