package fishing

import (
	"testing"

	"tsps-go/internal/game"
	"tsps-go/internal/plugins"
)

// InfernalHarpoon tests: charge read, the cook that never
// fires without a Cooking answerer (pinned), the cook that
// fires with one, the kept-cook roll, Check, making and
// recharging (InfernalHarpoon.Fishing.js). Fixtures only;
// harpoon ids are the ItemIdentifiers.ts values verified
// against the real rev-242 seed cache (probe, 2026-11-20).

// chargedInfernalHarpoon returns a charged infernal
// harpoon item with the given saved charges.
func chargedInfernalHarpoon(id, charges int) game.Item {
	item := game.NewItem(id, 1)
	item.SetMetaValue(infernalHarpoonChargesMetaKey, chargesMetaValue(charges))
	return item
}

func TestInfernalHarpoonCharges(t *testing.T) {
	cases := []struct {
		name string
		item game.Item
		want int
	}{
		{"saved 2500", chargedInfernalHarpoon(21031, 2500), 2500},
		{"saved 1", chargedInfernalHarpoon(21031, 1), 1},
		{"saved 0", chargedInfernalHarpoon(21031, 0), 0},
		{"saved over max clamps", chargedInfernalHarpoon(21031, 9000), 5000},
		{"saved negative clamps", chargedInfernalHarpoon(21031, -5), 0},
		{"no meta charged is MAX", game.NewItem(21031, 1), 5000},
		{"no meta or is MAX", game.NewItem(25059, 1), 5000},
		{"no meta uncharged is MAX too", game.NewItem(21033, 1), 5000},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := infernalHarpoonCharges(c.item); got != c.want {
				t.Fatalf("infernalHarpoonCharges = %d, want %d", got, c.want)
			}
		})
	}
	// Meta present but without a charges field falls back to
	// MAX (TS Number(undefined) is NaN).
	item := game.NewItem(21031, 1)
	item.SetMetaValue(infernalHarpoonChargesMetaKey, []byte(`{"other": 1}`))
	if got := infernalHarpoonCharges(item); got != 5000 {
		t.Fatalf("meta without charges = %d, want 5000", got)
	}
}

func TestFindChargedInfernalHarpoon(t *testing.T) {
	p := newTestPlayer("alice", 3000, 3000)
	// Wielded first: a wielded (or) wins over a carried plain.
	p.Equipment().SetItem(game.WeaponSlot, chargedInfernalHarpoon(25059, 5000))
	p.Inventory().SetItem(0, chargedInfernalHarpoon(21031, 4000))
	loc, ok := findChargedInfernalHarpoon(p)
	if !ok || !loc.wielded || loc.item.ID() != 25059 {
		t.Fatalf("find = %+v %v, want wielded 25059", loc, ok)
	}
	// Inventory only.
	p2 := newTestPlayer("bob", 3000, 3000)
	p2.Inventory().SetItem(3, chargedInfernalHarpoon(30342, 7000))
	loc, ok = findChargedInfernalHarpoon(p2)
	if !ok || loc.wielded || loc.item.ID() != 30342 || loc.slot != 3 {
		t.Fatalf("find = %+v %v, want inventory 30342 slot 3", loc, ok)
	}
	// No charged harpoon: an uncharged harpoon does not match.
	p3 := newTestPlayer("carol", 3000, 3000)
	p3.Inventory().SetItem(0, game.NewItem(21033, 1))
	if _, ok := findChargedInfernalHarpoon(p3); ok {
		t.Fatal("an uncharged harpoon must not match findCharged")
	}
	if _, ok := findChargedInfernalHarpoon(nil); ok {
		t.Fatal("nil player must not match")
	}
}

func TestTryCookFishNeverFiresUnanswered(t *testing.T) {
	// HONEST DEGRADATION (infernalharpoon.go header): with
	// no "cooking:raw-xp" answerer the cook NEVER fires, and
	// no charge is spent. 200 attempts pin that: the 1/3
	// roll would otherwise fire ~67 times.
	f, emitted := newTestFishing(t)
	p := newTestPlayer("alice", 3000, 3000)
	setLevel(p, game.SkillCooking, 99)
	p.Inventory().SetItem(0, chargedInfernalHarpoon(21031, 5000))
	for i := 0; i < 200; i++ {
		if f.tryCookFish(p, 317) {
			t.Fatal("tryCookFish fired with no cooking:raw-xp answerer")
		}
	}
	if got := infernalHarpoonCharges(p.Inventory().ItemAt(0)); got != 5000 {
		t.Fatalf("charges after 200 unanswered attempts = %d, want 5000", got)
	}
	if got := p.Skills().Experience(game.SkillCooking); got != 0 {
		t.Fatalf("Cooking XP after 200 unanswered attempts = %d, want 0", got)
	}
	// The query was still emitted — only on the attempts
	// whose 1/3 roll hit (TS emits after the roll), so the
	// count is ~67, not 200 — and every emit had the TS
	// shape.
	seen := 0
	for _, e := range *emitted {
		if e.name == cookingRawXPEventName {
			seen++
			req, ok := e.payload.(*CookingRawXPRequest)
			if !ok || req.RawID != 317 || req.Player != p {
				t.Fatalf("cooking:raw-xp payload = %+v, want raw 317 for the player", e.payload)
			}
		}
	}
	if seen == 0 {
		t.Fatal("cooking:raw-xp was never emitted in 200 attempts")
	}
}

func TestTryCookFishFiresAnswered(t *testing.T) {
	// With a Cooking answerer (the shape a future Cooking
	// port answers), the cook fires on the 1/3 roll, grants
	// half the raw XP, and spends one charge.
	f, _ := newTestFishing(t)
	f.emit = func(name string, payload any) {
		if name == cookingRawXPEventName {
			if req, ok := payload.(*CookingRawXPRequest); ok {
				req.XP = 100
				req.HasXP = true
			}
		}
	}
	p := newTestPlayer("alice", 3000, 3000)
	setLevel(p, game.SkillCooking, 99)
	p.Inventory().SetItem(0, chargedInfernalHarpoon(21031, 5000))
	fired := false
	for i := 0; i < 100 && !fired; i++ {
		fired = f.tryCookFish(p, 317)
	}
	if !fired {
		t.Fatal("tryCookFish never fired in 100 answered attempts (P ≈ (2/3)^100)")
	}
	if got := p.Skills().Experience(game.SkillCooking); got != 50 {
		t.Fatalf("Cooking XP after one cook = %d, want 50 (half of 100)", got)
	}
	if got := infernalHarpoonCharges(p.Inventory().ItemAt(0)); got != 4999 {
		t.Fatalf("charges after one cook = %d, want 4999", got)
	}
}

func TestTryCookKept(t *testing.T) {
	f, _ := newTestFishing(t)
	// No harpoon: never fires.
	p := newTestPlayer("alice", 3000, 3000)
	if f.tryCookKept(p) {
		t.Fatal("tryCookKept with no harpoon must return false")
	}
	// With a harpoon: the 1/3 roll fires sometimes and
	// spends a charge per fire; 300 attempts make an
	// all-miss or all-hit run negligible.
	p.Inventory().SetItem(0, chargedInfernalHarpoon(21031, 5000))
	fires := 0
	for i := 0; i < 300; i++ {
		if f.tryCookKept(p) {
			fires++
		}
	}
	if fires == 0 || fires == 300 {
		t.Fatalf("tryCookKept fired %d/300 times, want a mix (1/3 roll)", fires)
	}
	if got := infernalHarpoonCharges(p.Inventory().ItemAt(0)); got != 5000-fires {
		t.Fatalf("charges after %d cooks = %d, want %d", fires, got, 5000-fires)
	}
	// The last charge turns the harpoon uncharged.
	p2 := newTestPlayer("bob", 3000, 3000)
	session := &testSession{}
	p2.Session = session
	p2.Inventory().SetItem(0, chargedInfernalHarpoon(21031, 1))
	for i := 0; i < 100; i++ {
		if f.tryCookKept(p2) {
			break
		}
	}
	got := p2.Inventory().ItemAt(0)
	if got.ID() != 21033 {
		t.Fatalf("harpoon id after last-charge cook = %d, want 21033", got.ID())
	}
	if _, ok := got.GetMetaValue(infernalHarpoonChargesMetaKey); ok {
		t.Fatal("depleted harpoon must have its meta cleared")
	}
	if len(session.gameMessages) != 1 || session.gameMessages[0] != "Your infernal harpoon has run out of charges." {
		t.Fatalf("messages = %q, want the run-out line", session.gameMessages)
	}
}

func TestCheckInfernalHarpoon(t *testing.T) {
	f, _ := newTestFishing(t)
	p := newTestPlayer("alice", 3000, 3000)
	session := &testSession{}
	p.Session = session
	p.Inventory().SetItem(0, game.NewItem(21031, 1))
	event := &plugins.PluginItemActionEvent{Player: p, ItemID: 21031, Slot: 0}
	if !f.checkInfernalHarpoon(event) {
		t.Fatal("Check must claim the click")
	}
	if len(session.gameMessages) != 1 || session.gameMessages[0] != "Infernal harpoon: 100.0% remaining." {
		t.Fatalf("messages = %q, want the 100.0%% line", session.gameMessages)
	}
	p.Inventory().SetItem(0, chargedInfernalHarpoon(21031, 2500))
	if !f.checkInfernalHarpoon(event) {
		t.Fatal("Check must claim the click")
	}
	if len(session.gameMessages) != 2 || session.gameMessages[1] != "Infernal harpoon: 50.0% remaining." {
		t.Fatalf("messages = %q, want the 50.0%% line", session.gameMessages)
	}
	// An uncharged harpoon falls through.
	p.Inventory().SetItem(0, game.NewItem(21033, 1))
	if f.checkInfernalHarpoon(event) {
		t.Error("Check on an uncharged harpoon must fall through")
	}
	if f.checkInfernalHarpoon(nil) {
		t.Error("nil event must not claim")
	}
}

func TestMakeInfernalHarpoon(t *testing.T) {
	f, _ := newTestFishing(t)
	stone, dragon := f.ids.SmoulderingStone, f.ids.DragonHarpoon
	mkEvent := func(p *game.Player) *plugins.PluginItemOnItemEvent {
		return &plugins.PluginItemOnItemEvent{
			Player:           p,
			UsedItem:         p.Inventory().ItemAt(0),
			UsedItemID:       stone,
			UsedItemSlot:     0,
			UsedWithItem:     p.Inventory().ItemAt(1),
			UsedWithItemID:   dragon,
			UsedWithItemSlot: 1,
		}
	}
	// Levels met: the stone is consumed, the dragon harpoon
	// becomes an infernal harpoon, XP is granted.
	p := newTestPlayer("alice", 3000, 3000)
	session := &testSession{}
	p.Session = session
	setLevel(p, game.SkillFishing, 75)
	setLevel(p, game.SkillCooking, 85)
	p.Inventory().SetItem(0, game.NewItem(stone, 1))
	p.Inventory().SetItem(1, game.NewItem(dragon, 1))
	if !f.makeInfernalHarpoon(mkEvent(p)) {
		t.Fatal("make must claim the use")
	}
	if got := p.Inventory().ItemAt(1).ID(); got != infernalHarpoonID {
		t.Fatalf("harpoon id after make = %d, want %d", got, infernalHarpoonID)
	}
	if n := p.Inventory().GetAmount(stone); n != 0 {
		t.Fatalf("stones left = %d, want 0", n)
	}
	if got := p.Skills().Experience(game.SkillFishing); got != 200 {
		t.Fatalf("Fishing XP after make = %d, want 200", got)
	}
	if got := p.Skills().Experience(game.SkillCooking); got != 350 {
		t.Fatalf("Cooking XP after make = %d, want 350", got)
	}
	if len(session.gameMessages) != 1 || session.gameMessages[0] != "You infuse the dragon harpoon with the smouldering stone." {
		t.Fatalf("messages = %q, want the infuse line", session.gameMessages)
	}
	// Levels short: the refusal line, nothing consumed, but
	// the use is still claimed (TS returns undefined).
	p2 := newTestPlayer("bob", 3000, 3000)
	session2 := &testSession{}
	p2.Session = session2
	setLevel(p2, game.SkillFishing, 74)
	setLevel(p2, game.SkillCooking, 85)
	p2.Inventory().SetItem(0, game.NewItem(stone, 1))
	p2.Inventory().SetItem(1, game.NewItem(dragon, 1))
	if !f.makeInfernalHarpoon(mkEvent(p2)) {
		t.Fatal("make must claim the use even on the level refusal")
	}
	if got := p2.Inventory().ItemAt(1).ID(); got != dragon {
		t.Fatalf("harpoon id after refused make = %d, want the dragon harpoon untouched", got)
	}
	if n := p2.Inventory().GetAmount(stone); n != 1 {
		t.Fatalf("stones left = %d, want 1 (untouched)", n)
	}
	if len(session2.gameMessages) != 1 || session2.gameMessages[0] != "You need a Fishing level of 75 and a Cooking level of 85 to do that." {
		t.Fatalf("messages = %q, want the level refusal", session2.gameMessages)
	}
}

func TestRechargeInfernalHarpoon(t *testing.T) {
	f, _ := newTestFishing(t)
	stone, dragon := f.ids.SmoulderingStone, f.ids.DragonHarpoon
	// Stone on an uncharged harpoon.
	p := newTestPlayer("alice", 3000, 3000)
	session := &testSession{}
	p.Session = session
	p.Inventory().SetItem(0, game.NewItem(21033, 1))
	p.Inventory().SetItem(1, game.NewItem(stone, 1))
	event := &plugins.PluginItemOnItemEvent{
		Player: p, UsedItem: p.Inventory().ItemAt(1), UsedItemID: stone, UsedItemSlot: 1,
		UsedWithItem: p.Inventory().ItemAt(0), UsedWithItemID: 21033, UsedWithItemSlot: 0,
	}
	if !f.rechargeInfernalHarpoon(event) {
		t.Fatal("recharge must claim the use")
	}
	if got := p.Inventory().ItemAt(0).ID(); got != 21031 {
		t.Fatalf("harpoon id after recharge = %d, want 21031", got)
	}
	if n := p.Inventory().GetAmount(stone); n != 0 {
		t.Fatalf("stones left = %d, want 0", n)
	}
	if len(session.gameMessages) != 1 || session.gameMessages[0] != "Your infernal harpoon has been recharged with 5,000 charges." {
		t.Fatalf("messages = %q, want the recharge line", session.gameMessages)
	}
	// Dragon harpoon on an uncharged (or) recharges it to (or).
	p2 := newTestPlayer("bob", 3000, 3000)
	p2.Inventory().SetItem(0, game.NewItem(25367, 1))
	p2.Inventory().SetItem(1, game.NewItem(dragon, 1))
	event2 := &plugins.PluginItemOnItemEvent{
		Player: p2, UsedItem: p2.Inventory().ItemAt(1), UsedItemID: dragon, UsedItemSlot: 1,
		UsedWithItem: p2.Inventory().ItemAt(0), UsedWithItemID: 25367, UsedWithItemSlot: 0,
	}
	if !f.rechargeInfernalHarpoon(event2) {
		t.Fatal("recharge must claim the use")
	}
	if got := p2.Inventory().ItemAt(0).ID(); got != 25059 {
		t.Fatalf("harpoon id after recharge = %d, want 25059", got)
	}
	if f.rechargeInfernalHarpoon(nil) {
		t.Error("nil event must not claim")
	}
}

func TestRegisterInfernalHarpoon(t *testing.T) {
	f, _ := newTestFishing(t)
	stone, dragon := f.ids.SmoulderingStone, f.ids.DragonHarpoon
	hub := hubWith(t, func(api *plugins.Api) { f.registerInfernalHarpoon(api) })
	hub.SetItemResolver(func(id int) (string, bool) {
		switch id {
		case 21031:
			return "Infernal harpoon", false
		case 25059:
			return "Infernal harpoon (or)", false
		case 21033:
			return "Infernal harpoon (uncharged)", false
		case stone:
			return "Smouldering stone", false
		case dragon:
			return "Dragon harpoon", false
		}
		return "", false
	})
	hub.SetItemActionsResolver(func(id int) []string {
		if id == 21031 {
			return []string{"Wield", "Check", "", "", "Drop"}
		}
		return game.DefaultInventoryActions()
	})
	// Check via the hub.
	p := newTestPlayer("alice", 3000, 3000)
	session := &testSession{}
	p.Session = session
	p.Inventory().SetItem(0, game.NewItem(21031, 1))
	if !hub.EmitItemAction(&plugins.PluginItemActionEvent{
		Player: p, InterfaceID: game.InventoryInterfaceID, ItemID: 21031, Slot: 0, ClickType: 2,
	}) {
		t.Fatal("hub Check emit must be handled")
	}
	if len(session.gameMessages) != 1 || session.gameMessages[0] != "Infernal harpoon: 100.0% remaining." {
		t.Fatalf("messages = %q, want the Check line", session.gameMessages)
	}
	// Make via the hub: stone on a dragon harpoon.
	setLevel(p, game.SkillFishing, 75)
	setLevel(p, game.SkillCooking, 85)
	p.Inventory().SetItem(0, game.NewItem(stone, 1))
	p.Inventory().SetItem(1, game.NewItem(dragon, 1))
	if !hub.EmitItemOnItem(&plugins.PluginItemOnItemEvent{
		Player:           p,
		UsedItem:         p.Inventory().ItemAt(0),
		UsedItemID:       stone,
		UsedItemSlot:     0,
		UsedWithItem:     p.Inventory().ItemAt(1),
		UsedWithItemID:   dragon,
		UsedWithItemSlot: 1,
	}) {
		t.Fatal("hub make emit must be handled")
	}
	if got := p.Inventory().ItemAt(1).ID(); got != infernalHarpoonID {
		t.Fatalf("hub make harpoon id = %d, want %d", got, infernalHarpoonID)
	}
	// Nil api is a no-op, not a panic.
	f.registerInfernalHarpoon(nil)
}
