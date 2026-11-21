package woodcutting

import (
	"io"
	"log"
	"testing"

	"tsps-go/internal/game"
	"tsps-go/internal/plugins"
)

// CrystalAxe tests: charge read/write/clamp, the signet save,
// shard recharge math + cap, empty → inactive transitions,
// and the Check messages (CrystalAxe.Woodcutting.js).
// Fixtures only; ids are the ItemIdentifiers.ts values
// verified against the real rev-242 seed cache (probe,
// 2026-11-20).

// testSession captures game messages and sounds for
// woodcutting tests (a SessionPort with the chat and sound
// extensions).
type testSession struct {
	gameMessages []string
	sounds       []int
}

func (s *testSession) SendPacket(packet []byte) {}
func (s *testSession) SendRunEnergy(units int, running bool, weightKg int, staminaTicks int) {
}
func (s *testSession) SendDestination(x, y int)      {}
func (s *testSession) SendVarp(id, value int)        {}
func (s *testSession) FlushTick(cycle int64) error   { return nil }
func (s *testSession) Close(code int, reason string) {}
func (s *testSession) IsOpen() bool                  { return true }
func (s *testSession) SendGameMessage(message string) {
	s.gameMessages = append(s.gameMessages, message)
}
func (s *testSession) SendTypedGameMessage(chatType int, message, from string) {}
func (s *testSession) SendSoundEffect(soundID, loopType, delay, volume int) {
	s.sounds = append(s.sounds, soundID)
}
func (s *testSession) SendAreaSound(soundID, x, y, level, loops, delay, radius, attenuation int) {
}

var (
	_ game.SessionPort       = (*testSession)(nil)
	_ game.ChatMessageSender = (*testSession)(nil)
	_ game.SoundSender       = (*testSession)(nil)
)

// chargedCrystal returns a charged crystal axe item with the
// given saved charges, always writing the meta (even 0 or
// negative, so clamp cases are expressible).
func chargedCrystal(id, charges int) game.Item {
	item := game.NewItem(id, 1)
	item.SetMetaValue(crystalChargesMetaKey, chargesMetaValue(charges))
	return item
}

// stackableDefs is a minimal game.ItemDefinitionSource
// marking the given ids stackable: crystal shards stack in
// the cache, and with no source installed the container
// treats every item as non-stackable, so DeleteNumber would
// clear a whole shard stack instead of subtracting.
type stackableDefs struct{ stackable map[int]bool }

func (s stackableDefs) IsStackable(id int) bool      { return s.stackable[id] }
func (s stackableDefs) IsNoted(id int) bool          { return false }
func (s stackableDefs) WeightGrams(id int) int       { return 0 }
func (s stackableDefs) EquipmentSlot(id int) int     { return -1 }
func (s stackableDefs) IsDoubleHanded(id int) bool   { return false }
func (s stackableDefs) Requirements(id int) []int    { return nil }
func (s stackableDefs) CombatRequirement(id int) int { return 0 }
func (s stackableDefs) IsTradeable(id int) bool      { return false }
func (s stackableDefs) IsSellable(id int) bool       { return false }
func (s stackableDefs) IsDropable(id int) bool       { return false }
func (s stackableDefs) UnNote(id int) int            { return id }
func (s stackableDefs) PlaceholderID(id int) int     { return -1 }

func TestCrystalCharges(t *testing.T) {
	cases := []struct {
		name string
		item game.Item
		want int
	}{
		{"saved 5000", chargedCrystal(23673, 5000), 5000},
		{"saved 1", chargedCrystal(23673, 1), 1},
		{"saved 0", chargedCrystal(23673, 0), 0},
		{"saved over max clamps", chargedCrystal(23673, 25000), 20000},
		{"saved negative clamps", chargedCrystal(23673, -5), 0},
		{"no meta charged is START", game.NewItem(23673, 1), 10000},
		{"no meta felling is START", game.NewItem(28220, 1), 10000},
		{"no meta uncharged is 0", game.NewItem(23675, 1), 0},
		{"no meta other item is 0", game.NewItem(995, 1), 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := crystalCharges(c.item); got != c.want {
				t.Fatalf("crystalCharges = %d, want %d", got, c.want)
			}
		})
	}
	// Meta present but without a charges field falls back to
	// the id default (TS Number(undefined) is NaN).
	item := game.NewItem(23673, 1)
	item.SetMetaValue(crystalChargesMetaKey, []byte(`{"other": 1}`))
	if got := crystalCharges(item); got != 10000 {
		t.Fatalf("meta without charges = %d, want 10000", got)
	}
}

func TestFindChargedCrystalAxe(t *testing.T) {
	p := newTestPlayer("alice", 3000, 3000)
	// Wielded first: a wielded 23862 wins over a carried 23673.
	p.Equipment().SetItem(game.WeaponSlot, chargedCrystal(23862, 5000))
	p.Inventory().SetItem(0, chargedCrystal(23673, 4000))
	loc, ok := findChargedCrystalAxe(p, 0)
	if !ok || !loc.wielded || loc.item.ID() != 23862 {
		t.Fatalf("find = %+v %v, want wielded 23862", loc, ok)
	}
	// The axeId filter narrows the search to that id.
	loc, ok = findChargedCrystalAxe(p, 23673)
	if !ok || loc.wielded || loc.item.ID() != 23673 || loc.slot != 0 {
		t.Fatalf("find(23673) = %+v %v, want inventory slot 0", loc, ok)
	}
	// Inventory only, no wielded axe.
	p2 := newTestPlayer("bob", 3000, 3000)
	p2.Inventory().SetItem(3, chargedCrystal(28220, 7000))
	loc, ok = findChargedCrystalAxe(p2, 0)
	if !ok || loc.wielded || loc.item.ID() != 28220 || loc.slot != 3 {
		t.Fatalf("find = %+v %v, want inventory 28220 slot 3", loc, ok)
	}
	// No charged axe: an inactive axe does not match.
	p3 := newTestPlayer("carol", 3000, 3000)
	p3.Inventory().SetItem(0, game.NewItem(23675, 1))
	if _, ok := findChargedCrystalAxe(p3, 0); ok {
		t.Fatal("an inactive axe must not match findCharged")
	}
	if _, ok := findChargedCrystalAxe(nil, 0); ok {
		t.Fatal("nil player must not match")
	}
}

func TestTryUseCrystalChargeDrain(t *testing.T) {
	w, _ := newTestWoodcutting(t, loadFixtureTables(t))
	p := newTestPlayer("alice", 3000, 3000)
	session := &testSession{}
	p.Session = session
	p.Inventory().SetItem(0, chargedCrystal(23673, 2))
	if !w.tryUseCrystalCharge(p, 23673) {
		t.Fatal("drain must return true")
	}
	if got := crystalCharges(p.Inventory().ItemAt(0)); got != 1 {
		t.Fatalf("charges after drain = %d, want 1", got)
	}
	// The last charge turns the axe inactive and messages.
	if !w.tryUseCrystalCharge(p, 23673) {
		t.Fatal("last-charge drain must return true")
	}
	got := p.Inventory().ItemAt(0)
	if got.ID() != 23675 {
		t.Fatalf("axe id after depletion = %d, want 23675", got.ID())
	}
	if _, ok := got.GetMetaValue(crystalChargesMetaKey); ok {
		t.Fatal("depleted axe must have its meta cleared")
	}
	if len(session.gameMessages) != 1 || session.gameMessages[0] != "Your crystal axe has run out of charges." {
		t.Fatalf("messages = %q, want the run-out line", session.gameMessages)
	}
	// No charged axe held: nothing drains, nothing messages.
	if w.tryUseCrystalCharge(p, 23673) {
		t.Fatal("drain with no charged axe must return false")
	}
	if len(session.gameMessages) != 1 {
		t.Fatalf("messages = %q, want no further message", session.gameMessages)
	}
	// A carried crystal axe beside the chopping axe is left
	// alone: the drain takes the axeId.
	p2 := newTestPlayer("bob", 3000, 3000)
	p2.Inventory().SetItem(0, chargedCrystal(23673, 9))
	p2.Inventory().SetItem(1, chargedCrystal(28220, 8))
	if !w.tryUseCrystalCharge(p2, 28220) {
		t.Fatal("felling drain must return true")
	}
	if got := crystalCharges(p2.Inventory().ItemAt(0)); got != 9 {
		t.Fatalf("bystander axe charges = %d, want 9", got)
	}
	if got := crystalCharges(p2.Inventory().ItemAt(1)); got != 7 {
		t.Fatalf("felling axe charges = %d, want 7", got)
	}
}

func TestTryUseCrystalChargeSignetSave(t *testing.T) {
	w, _ := newTestWoodcutting(t, loadFixtureTables(t))
	p := newTestPlayer("alice", 3000, 3000)
	p.Equipment().SetItem(game.WeaponSlot, chargedCrystal(23673, 100))
	// The signet must be worn, not carried, to save the charge.
	p.Equipment().SetItem(game.RingSlot, game.NewItem(23943, 1))
	w.randFloat = func() float64 { return 0.05 } // under 10%: saved.
	if w.tryUseCrystalCharge(p, 23673) {
		t.Fatal("a signet save must return false")
	}
	if got := crystalCharges(p.Equipment().ItemAt(game.WeaponSlot)); got != 100 {
		t.Fatalf("saved charges = %d, want 100", got)
	}
	w.randFloat = func() float64 { return 0.5 } // over 10%: spent.
	if !w.tryUseCrystalCharge(p, 23673) {
		t.Fatal("a missed save must drain")
	}
	if got := crystalCharges(p.Equipment().ItemAt(game.WeaponSlot)); got != 99 {
		t.Fatalf("charges after drain = %d, want 99", got)
	}
	// A carried (unworn) signet saves nothing.
	p2 := newTestPlayer("bob", 3000, 3000)
	p2.Equipment().SetItem(game.WeaponSlot, chargedCrystal(23673, 100))
	p2.Inventory().SetItem(0, game.NewItem(23943, 1))
	w.randFloat = func() float64 { return 0.05 }
	if !w.tryUseCrystalCharge(p2, 23673) {
		t.Fatal("a carried signet must not save the charge")
	}
	if got := crystalCharges(p2.Equipment().ItemAt(game.WeaponSlot)); got != 99 {
		t.Fatalf("charges after drain = %d, want 99", got)
	}
}

func TestCheckCrystalCharges(t *testing.T) {
	w, _ := newTestWoodcutting(t, loadFixtureTables(t))
	p := newTestPlayer("alice", 3000, 3000)
	session := &testSession{}
	p.Session = session
	p.Inventory().SetItem(0, chargedCrystal(23673, 10000))
	ev := &plugins.PluginItemActionEvent{
		Player: p, InterfaceID: 3214, ItemID: 23673, Slot: 0, ClickType: 3,
	}
	if !w.checkCrystalCharges(ev) {
		t.Fatal("Check must claim the click")
	}
	if len(session.gameMessages) != 1 || session.gameMessages[0] != "Your crystal axe has 10,000 charges left." {
		t.Fatalf("messages = %q, want the grouped 10,000 line", session.gameMessages)
	}
	// Singular: one charge left.
	p.Inventory().SetItem(0, chargedCrystal(23673, 1))
	if !w.checkCrystalCharges(ev) {
		t.Fatal("Check must claim the click")
	}
	if len(session.gameMessages) != 2 || session.gameMessages[1] != "Your crystal axe has 1 charge left." {
		t.Fatalf("messages = %q, want the singular line", session.gameMessages)
	}
	// A wielded axe reports through the equipment interface.
	p2 := newTestPlayer("bob", 3000, 3000)
	withShardDefs(p2)
	session2 := &testSession{}
	p2.Session = session2
	p2.Equipment().SetItem(game.WeaponSlot, chargedCrystal(28220, 2500))
	ev2 := &plugins.PluginItemActionEvent{
		Player: p2, InterfaceID: game.EquipmentInventoryInterfaceID,
		ItemID: 28220, Slot: game.WeaponSlot, ClickType: 3,
	}
	if !w.checkCrystalCharges(ev2) {
		t.Fatal("wielded Check must claim the click")
	}
	if len(session2.gameMessages) != 1 || session2.gameMessages[0] != "Your crystal axe has 2,500 charges left." {
		t.Fatalf("messages = %q, want the 2,500 line", session2.gameMessages)
	}
}

func shardEvent(p *game.Player, usedID, usedSlot, withID, withSlot int) *plugins.PluginItemOnItemEvent {
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

// withShardDefs installs the stackable defs source on p, so
// shard stacks subtract instead of clearing whole.
func withShardDefs(p *game.Player) {
	p.SetItemDefinitions(stackableDefs{stackable: map[int]bool{crystalShardID: true}})
}

func TestAddShardCharges(t *testing.T) {
	w, _ := newTestWoodcutting(t, loadFixtureTables(t))
	// Shard on a nearly-full charged axe: only the shards
	// needed to reach the cap are consumed.
	p := newTestPlayer("alice", 3000, 3000)
	withShardDefs(p)
	session := &testSession{}
	p.Session = session
	p.Inventory().SetItem(0, chargedCrystal(23673, 19900))
	p.Inventory().SetItem(1, game.NewItem(crystalShardID, 5))
	if !w.addShardCharges(shardEvent(p, crystalShardID, 1, 23673, 0)) {
		t.Fatal("shard recharge must handle the use")
	}
	if got := crystalCharges(p.Inventory().ItemAt(0)); got != 20000 {
		t.Fatalf("charges = %d, want 20000", got)
	}
	if got := p.Inventory().GetAmount(crystalShardID); got != 4 {
		t.Fatalf("shards left = %d, want 4", got)
	}
	if len(session.gameMessages) != 1 || session.gameMessages[0] != "You add 1 crystal shard to the axe." {
		t.Fatalf("messages = %q, want the singular add line", session.gameMessages)
	}
	// Either direction works: axe on shard.
	p.Inventory().SetItem(0, chargedCrystal(23673, 19000))
	p.Inventory().SetItem(1, game.NewItem(crystalShardID, 3))
	if !w.addShardCharges(shardEvent(p, 23673, 0, crystalShardID, 1)) {
		t.Fatal("axe-on-shard must handle the use")
	}
	if got := crystalCharges(p.Inventory().ItemAt(0)); got != 19300 {
		t.Fatalf("charges = %d, want 19300", got)
	}
	if got := p.Inventory().GetAmount(crystalShardID); got != 0 {
		t.Fatalf("shards left = %d, want 0", got)
	}
	if len(session.gameMessages) != 2 || session.gameMessages[1] != "You add 3 crystal shards to the axe." {
		t.Fatalf("messages = %q, want the plural add line", session.gameMessages)
	}
	// Shard on an inactive axe: it becomes charged.
	p2 := newTestPlayer("bob", 3000, 3000)
	withShardDefs(p2)
	session2 := &testSession{}
	p2.Session = session2
	p2.Inventory().SetItem(0, game.NewItem(23675, 1))
	p2.Inventory().SetItem(1, game.NewItem(crystalShardID, 10))
	if !w.addShardCharges(shardEvent(p2, crystalShardID, 1, 23675, 0)) {
		t.Fatal("inactive recharge must handle the use")
	}
	axe := p2.Inventory().ItemAt(0)
	if axe.ID() != 23673 {
		t.Fatalf("axe id = %d, want 23673", axe.ID())
	}
	if got := crystalCharges(axe); got != 1000 {
		t.Fatalf("charges = %d, want 1000", got)
	}
	if got := p2.Inventory().GetAmount(crystalShardID); got != 0 {
		t.Fatalf("shards left = %d, want 0", got)
	}
	// A full axe: message + handled, no shards consumed.
	p3 := newTestPlayer("carol", 3000, 3000)
	withShardDefs(p3)
	session3 := &testSession{}
	p3.Session = session3
	p3.Inventory().SetItem(0, chargedCrystal(23673, 20000))
	p3.Inventory().SetItem(1, game.NewItem(crystalShardID, 7))
	if !w.addShardCharges(shardEvent(p3, crystalShardID, 1, 23673, 0)) {
		t.Fatal("full-axe use must still handle")
	}
	if got := p3.Inventory().GetAmount(crystalShardID); got != 7 {
		t.Fatalf("shards left = %d, want 7", got)
	}
	if len(session3.gameMessages) != 1 || session3.gameMessages[0] != "Your crystal axe cannot hold any more charges." {
		t.Fatalf("messages = %q, want the full line", session3.gameMessages)
	}
	// No shards carried: the use falls through.
	p4 := newTestPlayer("dave", 3000, 3000)
	p4.Inventory().SetItem(0, chargedCrystal(23673, 5000))
	if w.addShardCharges(shardEvent(p4, crystalShardID, 1, 23673, 0)) {
		t.Fatal("a use with no shards must fall through")
	}
	// Neither item is a crystal axe: the use falls through.
	p5 := newTestPlayer("erin", 3000, 3000)
	withShardDefs(p5)
	p5.Inventory().SetItem(0, game.NewItem(995, 1))
	p5.Inventory().SetItem(1, game.NewItem(crystalShardID, 5))
	if w.addShardCharges(shardEvent(p5, crystalShardID, 1, 995, 0)) {
		t.Fatal("a non-axe pair must fall through")
	}
	if got := p5.Inventory().GetAmount(crystalShardID); got != 5 {
		t.Fatalf("shards left = %d, want 5", got)
	}
}

func TestChopSuccessDrainsCrystalAxe(t *testing.T) {
	// The core-loop wiring: a successful chop with the
	// charged crystal axe spends one charge (chopSuccess,
	// after the log line, before XP — plugin.js:1416-1420).
	// The session axe carries the real id (23673); the
	// fixture tables cannot resolve it, so the session is
	// built directly instead of going through startChopping.
	w, _ := newTestWoodcutting(t, loadFixtureTables(t))
	w.randFloat = func() float64 { return 0.5 }
	p := newTestPlayer("alice", 3200, 3200)
	setLevel(p, game.SkillWoodcutting, 99)
	p.Inventory().SetItem(0, chargedCrystal(23673, 100))
	normal := treeByName(loadFixtureTables(t), "normal tree")
	state := &session{
		tree:     normal,
		axe:      Axe{ID: 23673, Item: "crystal_axe"},
		objectID: normal.ObjectIDs[0],
		location: game.Location{X: 3201, Y: 3200, Z: 0},
	}
	w.chopSuccess(p, state)
	if got := crystalCharges(p.Inventory().ItemAt(0)); got != 99 {
		t.Fatalf("charges after chop = %d, want 99", got)
	}
	if !p.Inventory().Contains(normal.LogID) {
		t.Fatal("want the chopped log in the inventory")
	}
	if got := p.Skills().Experience(game.SkillWoodcutting); got != 25 {
		t.Errorf("Woodcutting XP = %d, want 25", got)
	}
}

func TestCrystalAxeRegistration(t *testing.T) {
	w, _ := newTestWoodcutting(t, loadFixtureTables(t))
	hub := plugins.NewHub([]plugins.Plugin{{
		Name:     "Woodcutting",
		Register: func(api *plugins.Api) { w.registerCrystalAxe(api) },
	}}, plugins.HubOptions{Logger: log.New(io.Discard, "", 0)})
	hub.SetItemResolver(func(id int) (string, bool) {
		switch id {
		case 23962:
			return "Crystal shard", false
		case 23963:
			return "Crystal shard", true
		case 23673:
			return "Crystal axe", false
		case 23675:
			return "Crystal axe (inactive)", false
		case 28220:
			return "Crystal felling axe", false
		case 28223:
			return "Crystal felling axe (inactive)", false
		default:
			return "", false
		}
	})
	hub.SetItemActionsResolver(func(id int) []string {
		if id == 23673 || id == 28220 {
			return []string{"", "Wield", "Check", "Revert", "Drop"}
		}
		return []string{"", "", "", "", "Drop"}
	})
	hub.Load()

	// Check via the hub: the named item-action handler fires.
	p := newTestPlayer("alice", 3000, 3000)
	withShardDefs(p)
	session := &testSession{}
	p.Session = session
	p.Inventory().SetItem(0, chargedCrystal(23673, 1234))
	ev := &plugins.PluginItemActionEvent{
		Player: p, InterfaceID: 3214, ItemID: 23673, Slot: 0, ClickType: 3,
	}
	if !hub.EmitItemAction(ev) {
		t.Fatal("Check must claim through the hub")
	}
	if len(session.gameMessages) != 1 || session.gameMessages[0] != "Your crystal axe has 1,234 charges left." {
		t.Fatalf("messages = %q, want the 1,234 line", session.gameMessages)
	}
	// Shard recharge via the hub, both directions.
	p.Inventory().SetItem(1, game.NewItem(crystalShardID, 2))
	if !hub.EmitItemOnItem(shardEvent(p, crystalShardID, 1, 23673, 0)) {
		t.Fatal("shard recharge must claim through the hub")
	}
	if got := crystalCharges(p.Inventory().ItemAt(0)); got != 1434 {
		t.Fatalf("charges = %d, want 1434", got)
	}
	// A noted shard does not recharge (the {noted: false} filter).
	p.Inventory().SetItem(1, game.NewItem(23963, 2))
	before := crystalCharges(p.Inventory().ItemAt(0))
	ev2 := &plugins.PluginItemOnItemEvent{
		Player:           p,
		UsedItem:         p.Inventory().ItemAt(1),
		UsedItemID:       23963,
		UsedItemSlot:     1,
		UsedWithItem:     p.Inventory().ItemAt(0),
		UsedWithItemID:   23673,
		UsedWithItemSlot: 0,
	}
	if hub.EmitItemOnItem(ev2) {
		t.Fatal("a noted shard must not recharge through the hub")
	}
	if got := crystalCharges(p.Inventory().ItemAt(0)); got != before {
		t.Fatalf("charges = %d, want unchanged %d", got, before)
	}
}
