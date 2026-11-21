package fishing

import (
	"testing"

	"tsps-go/internal/game"
	"tsps-go/internal/plugins"
)

// CrystalHarpoon tests: charge read/write/clamp, the signet
// save, shard recharge math + cap, empty → inactive
// transitions, and the Check messages
// (CrystalHarpoon.Fishing.js). Fixtures only; harpoon ids
// are the ItemIdentifiers.ts values verified against the
// real rev-242 seed cache (probe, 2026-11-20).

// chargedCrystalHarpoon returns a charged crystal harpoon
// item with the given saved charges, always writing the
// meta (even 0 or negative, so clamp cases are
// expressible).
func chargedCrystalHarpoon(id, charges int) game.Item {
	item := game.NewItem(id, 1)
	item.SetMetaValue(crystalHarpoonChargesMetaKey, chargesMetaValue(charges))
	return item
}

func TestCrystalHarpoonCharges(t *testing.T) {
	cases := []struct {
		name string
		item game.Item
		want int
	}{
		{"saved 5000", chargedCrystalHarpoon(23762, 5000), 5000},
		{"saved 1", chargedCrystalHarpoon(23762, 1), 1},
		{"saved 0", chargedCrystalHarpoon(23762, 0), 0},
		{"saved over max clamps", chargedCrystalHarpoon(23762, 25000), 20000},
		{"saved negative clamps", chargedCrystalHarpoon(23762, -5), 0},
		{"no meta charged is START", game.NewItem(23762, 1), 10000},
		{"no meta gauntlet is START", game.NewItem(23864, 1), 10000},
		{"no meta uncharged is 0", game.NewItem(23764, 1), 0},
		{"no meta other item is 0", game.NewItem(995, 1), 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := crystalHarpoonCharges(c.item); got != c.want {
				t.Fatalf("crystalHarpoonCharges = %d, want %d", got, c.want)
			}
		})
	}
	// Meta present but without a charges field falls back to
	// the id default (TS Number(undefined) is NaN).
	item := game.NewItem(23762, 1)
	item.SetMetaValue(crystalHarpoonChargesMetaKey, []byte(`{"other": 1}`))
	if got := crystalHarpoonCharges(item); got != 10000 {
		t.Fatalf("meta without charges = %d, want 10000", got)
	}
}

func TestFindChargedCrystalHarpoon(t *testing.T) {
	p := newTestPlayer("alice", 3000, 3000)
	// Wielded first: a wielded 23864 wins over a carried 23762.
	p.Equipment().SetItem(game.WeaponSlot, chargedCrystalHarpoon(23864, 5000))
	p.Inventory().SetItem(0, chargedCrystalHarpoon(23762, 4000))
	loc, ok := findChargedCrystalHarpoon(p)
	if !ok || !loc.wielded || loc.item.ID() != 23864 {
		t.Fatalf("find = %+v %v, want wielded 23864", loc, ok)
	}
	// Inventory only, no wielded harpoon.
	p2 := newTestPlayer("bob", 3000, 3000)
	p2.Inventory().SetItem(3, chargedCrystalHarpoon(23762, 7000))
	loc, ok = findChargedCrystalHarpoon(p2)
	if !ok || loc.wielded || loc.item.ID() != 23762 || loc.slot != 3 {
		t.Fatalf("find = %+v %v, want inventory 23762 slot 3", loc, ok)
	}
	// No charged harpoon: an inactive harpoon does not match.
	p3 := newTestPlayer("carol", 3000, 3000)
	p3.Inventory().SetItem(0, game.NewItem(23764, 1))
	if _, ok := findChargedCrystalHarpoon(p3); ok {
		t.Fatal("an inactive harpoon must not match findCharged")
	}
	if _, ok := findChargedCrystalHarpoon(nil); ok {
		t.Fatal("nil player must not match")
	}
}

func TestTryUseCrystalChargeDrain(t *testing.T) {
	f, _ := newTestFishing(t)
	p := newTestPlayer("alice", 3000, 3000)
	session := &testSession{}
	p.Session = session
	p.Inventory().SetItem(0, chargedCrystalHarpoon(23762, 2))
	if !f.tryUseCrystalCharge(p) {
		t.Fatal("drain must return true")
	}
	if got := crystalHarpoonCharges(p.Inventory().ItemAt(0)); got != 1 {
		t.Fatalf("charges after drain = %d, want 1", got)
	}
	// The last charge turns the harpoon inactive and messages.
	if !f.tryUseCrystalCharge(p) {
		t.Fatal("last-charge drain must return true")
	}
	got := p.Inventory().ItemAt(0)
	if got.ID() != 23764 {
		t.Fatalf("harpoon id after depletion = %d, want 23764", got.ID())
	}
	if _, ok := got.GetMetaValue(crystalHarpoonChargesMetaKey); ok {
		t.Fatal("depleted harpoon must have its meta cleared")
	}
	if len(session.gameMessages) != 1 || session.gameMessages[0] != "Your crystal harpoon has run out of charges." {
		t.Fatalf("messages = %q, want the run-out line", session.gameMessages)
	}
	// No charged harpoon held: nothing drains, nothing messages.
	if f.tryUseCrystalCharge(p) {
		t.Fatal("drain with no charged harpoon must return false")
	}
	if len(session.gameMessages) != 1 {
		t.Fatalf("messages = %q, want no further message", session.gameMessages)
	}
	// Nil ids: the signet check degrades, the drain still runs.
	p2 := newTestPlayer("bob", 3000, 3000)
	p2.Inventory().SetItem(0, chargedCrystalHarpoon(23762, 5))
	if !(&fishing{}).tryUseCrystalCharge(p2) {
		t.Fatal("no-ids drain must still return true")
	}
	if got := crystalHarpoonCharges(p2.Inventory().ItemAt(0)); got != 4 {
		t.Fatalf("no-ids charges after drain = %d, want 4", got)
	}
}

func TestTryUseCrystalChargeSignet(t *testing.T) {
	f, _ := newTestFishing(t)
	// Without a signet, 200 drains from 5000 cost exactly 200.
	plain := newTestPlayer("alice", 3000, 3000)
	plain.Inventory().SetItem(0, chargedCrystalHarpoon(23762, 5000))
	for i := 0; i < 200; i++ {
		if !f.tryUseCrystalCharge(plain) {
			t.Fatal("drain must return true while charges remain")
		}
	}
	if got := crystalHarpoonCharges(plain.Inventory().ItemAt(0)); got != 4800 {
		t.Fatalf("no-signet charges after 200 drains = %d, want 4800", got)
	}
	// With a signet, at least one of 200 drains is saved:
	// P(no save) = 0.9^200, negligible. A saved drain
	// returns false (no charge spent), so the return value
	// is not asserted per drain.
	saved := newTestPlayer("bob", 3000, 3000)
	saved.Equipment().SetItem(game.RingSlot, game.NewItem(f.ids.ElvenSignet, 1))
	saved.Inventory().SetItem(0, chargedCrystalHarpoon(23762, 5000))
	for i := 0; i < 200; i++ {
		f.tryUseCrystalCharge(saved)
	}
	got := crystalHarpoonCharges(saved.Inventory().ItemAt(0))
	if got <= 4800 || got > 5000 {
		t.Fatalf("signet charges after 200 drains = %d, want (4800, 5000]", got)
	}
}

func TestCheckCrystalHarpoonCharges(t *testing.T) {
	f, _ := newTestFishing(t)
	p := newTestPlayer("alice", 3000, 3000)
	session := &testSession{}
	p.Session = session
	p.Inventory().SetItem(0, game.NewItem(23762, 1))
	event := &plugins.PluginItemActionEvent{Player: p, ItemID: 23762, Slot: 0}
	if !f.checkCrystalHarpoonCharges(event) {
		t.Fatal("Check must claim the click")
	}
	if len(session.gameMessages) != 1 || session.gameMessages[0] != "Your crystal harpoon has 10,000 charges left." {
		t.Fatalf("messages = %q, want the grouped 10,000 line", session.gameMessages)
	}
	p.Inventory().SetItem(0, chargedCrystalHarpoon(23762, 1))
	if !f.checkCrystalHarpoonCharges(event) {
		t.Fatal("Check must claim the click")
	}
	if len(session.gameMessages) != 2 || session.gameMessages[1] != "Your crystal harpoon has 1 charge left." {
		t.Fatalf("messages = %q, want the singular line", session.gameMessages)
	}
	if f.checkCrystalHarpoonCharges(nil) {
		t.Error("nil event must not claim")
	}
}

func TestAddShardCharges(t *testing.T) {
	f, _ := newTestFishing(t)
	shard := f.ids.CrystalShard
	mkPlayer := func() (*game.Player, *testSession) {
		p := newTestPlayer("alice", 3000, 3000)
		session := &testSession{}
		p.Session = session
		p.SetItemDefinitions(stackableDefs{stackable: map[int]bool{shard: true}})
		return p, session
	}
	onItem := func(p *game.Player, usedID, usedSlot, withID, withSlot int) *plugins.PluginItemOnItemEvent {
		return &plugins.PluginItemOnItemEvent{
			Player:           p,
			UsedItem:         p.Inventory().ItemAt(usedSlot),
			UsedItemID:       usedID,
			UsedItemSlot:     usedSlot,
			UsedWithItem:     p.Inventory().ItemAt(withSlot),
			UsedWithItemID:   withID,
			UsedWithItemSlot: withSlot,
		}
	}
	// Charged harpoon at 19900 + 5 shards: 1 shard to the cap.
	p, session := mkPlayer()
	p.Inventory().SetItem(0, chargedCrystalHarpoon(23762, 19900))
	p.Inventory().SetItem(1, game.NewItem(shard, 5))
	if !f.addShardCharges(onItem(p, shard, 1, 23762, 0)) {
		t.Fatal("shard recharge must handle the use")
	}
	if got := crystalHarpoonCharges(p.Inventory().ItemAt(0)); got != 20000 {
		t.Fatalf("charges after recharge = %d, want 20000", got)
	}
	if got := p.Inventory().GetAmount(shard); got != 4 {
		t.Fatalf("shards left = %d, want 4", got)
	}
	if len(session.gameMessages) != 1 || session.gameMessages[0] != "You add 1 crystal shard to the harpoon." {
		t.Fatalf("messages = %q, want the singular shard line", session.gameMessages)
	}
	// Inactive harpoon + 10 shards: reactivates at 1000 charges.
	p, session = mkPlayer()
	p.Inventory().SetItem(0, game.NewItem(23764, 1))
	p.Inventory().SetItem(1, game.NewItem(shard, 10))
	if !f.addShardCharges(onItem(p, 23764, 0, shard, 1)) {
		t.Fatal("inactive recharge must handle the use")
	}
	got := p.Inventory().ItemAt(0)
	if got.ID() != 23762 {
		t.Fatalf("harpoon id after recharge = %d, want 23762", got.ID())
	}
	if c := crystalHarpoonCharges(got); c != 1000 {
		t.Fatalf("charges after inactive recharge = %d, want 1000", c)
	}
	if n := p.Inventory().GetAmount(shard); n != 0 {
		t.Fatalf("shards left = %d, want 0", n)
	}
	if len(session.gameMessages) != 1 || session.gameMessages[0] != "You add 10 crystal shards to the harpoon." {
		t.Fatalf("messages = %q, want the plural shard line", session.gameMessages)
	}
	// Full harpoon: message, handled, shards untouched.
	p, session = mkPlayer()
	p.Inventory().SetItem(0, chargedCrystalHarpoon(23762, 20000))
	p.Inventory().SetItem(1, game.NewItem(shard, 3))
	if !f.addShardCharges(onItem(p, shard, 1, 23762, 0)) {
		t.Fatal("full-harpoon recharge must handle the use")
	}
	if n := p.Inventory().GetAmount(shard); n != 3 {
		t.Fatalf("shards left = %d, want 3 (untouched)", n)
	}
	if len(session.gameMessages) != 1 || session.gameMessages[0] != "Your crystal harpoon cannot hold any more charges." {
		t.Fatalf("messages = %q, want the full line", session.gameMessages)
	}
	// No shards: falls through.
	p, _ = mkPlayer()
	p.Inventory().SetItem(0, chargedCrystalHarpoon(23762, 5000))
	if f.addShardCharges(onItem(p, shard, 1, 23762, 0)) {
		t.Error("recharge with no shards must fall through")
	}
	// Wrong pair: falls through.
	p, _ = mkPlayer()
	p.Inventory().SetItem(0, game.NewItem(995, 1))
	p.Inventory().SetItem(1, game.NewItem(shard, 5))
	if f.addShardCharges(onItem(p, shard, 1, 995, 0)) {
		t.Error("shard on a non-harpoon must fall through")
	}
	if f.addShardCharges(nil) {
		t.Error("nil event must fall through")
	}
}

func TestRegisterCrystalHarpoon(t *testing.T) {
	f, _ := newTestFishing(t)
	shard := f.ids.CrystalShard
	hub := hubWith(t, func(api *plugins.Api) { f.registerCrystalHarpoon(api) })
	hub.SetItemResolver(func(id int) (string, bool) {
		switch id {
		case 23762:
			return "Crystal harpoon", false
		case 23764:
			return "Crystal harpoon (inactive)", false
		case shard:
			return "Crystal shard", false
		}
		return "", false
	})
	hub.SetItemActionsResolver(func(id int) []string {
		if id == 23762 {
			return []string{"Wield", "Check", "", "", "Drop"}
		}
		return game.DefaultInventoryActions()
	})
	// Check via the hub: click type 2 is the Check option.
	p := newTestPlayer("alice", 3000, 3000)
	session := &testSession{}
	p.Session = session
	p.Inventory().SetItem(0, game.NewItem(23762, 1))
	if !hub.EmitItemAction(&plugins.PluginItemActionEvent{
		Player: p, InterfaceID: game.InventoryInterfaceID, ItemID: 23762, Slot: 0, ClickType: 2,
	}) {
		t.Fatal("hub Check emit must be handled")
	}
	if len(session.gameMessages) != 1 || session.gameMessages[0] != "Your crystal harpoon has 10,000 charges left." {
		t.Fatalf("messages = %q, want the Check line", session.gameMessages)
	}
	// Shard recharge via the hub: shard on the harpoon.
	p.SetItemDefinitions(stackableDefs{stackable: map[int]bool{shard: true}})
	p.Inventory().SetItem(0, chargedCrystalHarpoon(23762, 19900))
	p.Inventory().SetItem(1, game.NewItem(shard, 5))
	if !hub.EmitItemOnItem(&plugins.PluginItemOnItemEvent{
		Player:           p,
		UsedItem:         p.Inventory().ItemAt(1),
		UsedItemID:       shard,
		UsedItemSlot:     1,
		UsedWithItem:     p.Inventory().ItemAt(0),
		UsedWithItemID:   23762,
		UsedWithItemSlot: 0,
	}) {
		t.Fatal("hub shard emit must be handled")
	}
	if got := crystalHarpoonCharges(p.Inventory().ItemAt(0)); got != 20000 {
		t.Fatalf("hub recharge charges = %d, want 20000", got)
	}
	// Nil api is a no-op, not a panic.
	f.registerCrystalHarpoon(nil)
}
