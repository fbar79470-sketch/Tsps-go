package woodcutting

import (
	"testing"

	"tsps-go/internal/game"
	"tsps-go/internal/plugins"
)

// ---- Shrine fixtures ----

// newTestShrine builds the shared woodcutting instance's shrine
// unit over fixture tables, with deterministic randomness (every
// reward roll picks the nest: 0.5 * 1200 is never < 1).
func newTestShrine(t *testing.T) (*woodcutting, *entShrine) {
	t.Helper()
	tables := loadFixtureTables(t)
	w, _ := newTestWoodcutting(t, tables)
	w.randFloat = func() float64 { return 0.5 }
	return w, w.shrine
}

// eggPlayer puts one egg of id in inventory slot 0.
func eggPlayer(t *testing.T, eggID int) *game.Player {
	t.Helper()
	p := newTestPlayer("alice", 3200, 3200)
	p.Inventory().SetItem(0, game.NewItem(eggID, 1))
	return p
}

// shrineEvent builds the item-on-object event for an egg used
// on the shrine at the shrine's tile.
func shrineEvent(p *game.Player, eggID, slot int) *plugins.ItemOnObjectEvent {
	return &plugins.ItemOnObjectEvent{
		Player:   p,
		ObjectID: Shrine.ID,
		ItemID:   eggID,
		ItemSlot: slot,
		Location: game.Location{X: 3201, Y: 3200, Z: 0},
	}
}

// TestShrineRewardDistribution pins reward
// (Shrine.Woodcutting.js:12-16): each evil chicken piece rolls
// 1/1200 in JSON order with a fresh roll per piece, otherwise
// the seed nest.
func TestShrineRewardDistribution(t *testing.T) {
	seq := func(values ...float64) func() float64 {
		i := 0
		return func() float64 {
			v := values[i%len(values)]
			i++
			return v
		}
	}
	cases := []struct {
		name string
		rand func() float64
		want int
	}{
		{"first piece", seq(0), 20433},
		{"second piece", seq(0.5, 0), 20436},
		{"third piece", seq(0.5, 0.5, 0), 20439},
		{"fourth piece", seq(0.5, 0.5, 0.5, 0), 20442},
		{"nest when all miss", seq(0.5), 22798},
		{"a 1/1200 hit is below 1", seq(0.0008), 20433},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := shrineReward(tc.rand); got != tc.want {
				t.Errorf("shrineReward = %d, want %d", got, tc.want)
			}
		})
	}
}

// TestShrineOfferEgg pins offerEgg
// (Shrine.Woodcutting.js:18-48): the slot item is replaced
// with the nest, 100 Prayer XP lands, the offerings count
// increments, and a second offering counts two.
func TestShrineOfferEgg(t *testing.T) {
	_, s := newTestShrine(t)
	p := eggPlayer(t, 5076)
	if !s.offerEgg(shrineEvent(p, 5076, 0)) {
		t.Fatal("red egg on the shrine: want handled (true)")
	}
	if got := p.Inventory().ItemAt(0).ID(); got != Shrine.Nest {
		t.Errorf("slot 0 after offering = %d, want nest %d", got, Shrine.Nest)
	}
	if got := p.Skills().Experience(game.SkillPrayer); got != 100 {
		t.Errorf("Prayer XP = %d, want 100", got)
	}
	if got := shrineOfferingsCount(p); got != 1 {
		t.Errorf("offerings = %d, want 1", got)
	}
	if got := p.GetAttribute(ShrineOfferingsAttribute); got != 1 {
		t.Errorf("offerings attribute = %v, want 1", got)
	}
	// A second offering (blue egg) counts two and still pays.
	p.Inventory().SetItem(0, game.NewItem(5077, 1))
	if !s.offerEgg(shrineEvent(p, 5077, 0)) {
		t.Fatal("blue egg on the shrine: want handled (true)")
	}
	if got := shrineOfferingsCount(p); got != 2 {
		t.Errorf("offerings = %d, want 2", got)
	}
	if got := p.Skills().Experience(game.SkillPrayer); got != 200 {
		t.Errorf("Prayer XP = %d, want 200", got)
	}
}

// TestShrineOfferEggGuards pins offerEgg's fall-throughs:
// an unknown item and a wrong object fall through (false);
// a slot that no longer holds the egg claims the use
// silently (true) and changes nothing.
func TestShrineOfferEggGuards(t *testing.T) {
	_, s := newTestShrine(t)
	p := eggPlayer(t, 5076)
	// Unknown item id.
	if s.offerEgg(shrineEvent(p, 1234, 0)) {
		t.Error("unknown item: want fall-through (false)")
	}
	if got := p.Inventory().ItemAt(0).ID(); got != 5076 {
		t.Errorf("unknown item: slot 0 = %d, want the egg still there", got)
	}
	// Wrong object id.
	wrongObject := shrineEvent(p, 5076, 0)
	wrongObject.ObjectID = 12345
	if s.offerEgg(wrongObject) {
		t.Error("wrong object: want fall-through (false)")
	}
	// Slot identity: the egg sits in slot 1, the event names
	// slot 0 (which holds nothing).
	p.Inventory().SetItem(0, game.NewItem(-1, 0))
	p.Inventory().SetItem(1, game.NewItem(5076, 1))
	if !s.offerEgg(shrineEvent(p, 5076, 0)) {
		t.Error("slot mismatch: want claimed (true)")
	}
	if got := p.Inventory().ItemAt(1).ID(); got != 5076 {
		t.Errorf("slot mismatch: slot 1 = %d, want the egg untouched", got)
	}
	if got := shrineOfferingsCount(p); got != 0 {
		t.Errorf("slot mismatch: offerings = %d, want 0", got)
	}
}

// TestShrineOfferEggEvilChicken pins the rare reward: with
// a 0 roll the first evil chicken piece lands in the slot,
// and the XP still pays.
func TestShrineOfferEggEvilChicken(t *testing.T) {
	tables := loadFixtureTables(t)
	w, _ := newTestWoodcutting(t, tables)
	w.randFloat = func() float64 { return 0 }
	s := w.shrine
	p := eggPlayer(t, 5078)
	if !s.offerEgg(shrineEvent(p, 5078, 0)) {
		t.Fatal("green egg on the shrine: want handled (true)")
	}
	if got := p.Inventory().ItemAt(0).ID(); got != 20433 {
		t.Errorf("slot 0 after offering = %d, want evil chicken piece 20433", got)
	}
	if got := p.Skills().Experience(game.SkillPrayer); got != 100 {
		t.Errorf("Prayer XP = %d, want 100", got)
	}
}

// TestShrineHubRegistration fires the shrine through the hub:
// the named item-on-object hook matches "Bird's egg" on
// "Shrine" (via the installed name resolvers) and the
// offering lands.
func TestShrineHubRegistration(t *testing.T) {
	tables := loadFixtureTables(t)
	SetTables(tables)
	defer SetTables(nil)
	hub := plugins.NewHub([]plugins.Plugin{Plugin}, plugins.HubOptions{MembersWorld: true})
	hub.SetItemResolver(func(id int) (string, bool) {
		if id == 5076 || id == 5077 || id == 5078 {
			return "Bird's egg", false
		}
		return "", false
	})
	hub.SetObjectDefinitionResolver(func(id int, player *game.Player) plugins.ObjectDefinition {
		if id == Shrine.ID {
			return fakeObjectDef{name: "Shrine"}
		}
		return nil
	})
	hub.Load()
	origRandFloat := shared.randFloat
	shared.randFloat = func() float64 { return 0.5 }
	defer func() { shared.randFloat = origRandFloat }()
	p := eggPlayer(t, 5076)
	event := &plugins.ItemOnObjectEvent{
		Player:   p,
		Object:   fakeShrineObject{},
		ObjectID: Shrine.ID,
		Item:     game.NewItem(5076, 1),
		ItemID:   5076,
		ItemSlot: 0,
		Location: game.Location{X: 3201, Y: 3200, Z: 0},
	}
	if !hub.EmitItemOnObject(event) {
		t.Fatal("hub egg-on-shrine: want handled")
	}
	if got := p.Inventory().ItemAt(0).ID(); got != Shrine.Nest {
		t.Errorf("hub offering: slot 0 = %d, want nest %d", got, Shrine.Nest)
	}
	if got := shrineOfferingsCount(p); got != 1 {
		t.Errorf("hub offering: offerings = %d, want 1", got)
	}
}

// fakeShrineObject is a minimal plugins.GameObject for the
// hub shrine test: only the id, tile, type, and face the
// event guard reads.
type fakeShrineObject struct{}

func (fakeShrineObject) ID() int                 { return Shrine.ID }
func (fakeShrineObject) Location() game.Location { return game.Location{X: 3201, Y: 3200, Z: 0} }
func (fakeShrineObject) Type() int               { return 10 }
func (fakeShrineObject) Face() int               { return 0 }
