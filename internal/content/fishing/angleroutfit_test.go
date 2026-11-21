package fishing

import (
	"testing"

	"tsps-go/internal/game"
	"tsps-go/internal/plugins"
)

// AnglerOutfit tests: the XP multiplier per piece + full
// set, the full-outfit check, and the set text
// (AnglerOutfit.Fishing.js). Fixtures only.

func equipAngler(t *testing.T, p *game.Player, hat, top, waders, boots int) {
	t.Helper()
	p.Equipment().SetItem(game.HeadSlot, game.NewItem(hat, 1))
	p.Equipment().SetItem(game.BodySlot, game.NewItem(top, 1))
	p.Equipment().SetItem(game.LegSlot, game.NewItem(waders, 1))
	p.Equipment().SetItem(game.FeetSlot, game.NewItem(boots, 1))
}

func TestAnglerXPMultiplier(t *testing.T) {
	f, _ := newTestFishing(t)
	ids := f.ids
	empty := game.NewItem(-1, 0)
	cases := []struct {
		name                    string
		hat, top, waders, boots int
		want                    float64
	}{
		{"nothing worn", -1, -1, -1, -1, 1},
		{"hat only", ids.AnglerHat, -1, -1, -1, 1.004},
		{"top only", -1, ids.AnglerTop, -1, -1, 1.008},
		{"waders only", -1, -1, ids.AnglerWaders, -1, 1.006},
		{"boots only", -1, -1, -1, ids.AnglerBoots, 1.002},
		{"three pieces, no set bonus", ids.AnglerHat, ids.AnglerTop, ids.AnglerWaders, -1, 1.018},
		{"full angler", ids.AnglerHat, ids.AnglerTop, ids.AnglerWaders, ids.AnglerBoots, 1.025},
		{"full spirit", ids.SpiritAnglerHeadband, ids.SpiritAnglerTop, ids.SpiritAnglerWaders, ids.SpiritAnglerBoots, 1.025},
		{"mixed full set", ids.SpiritAnglerHeadband, ids.AnglerTop, ids.SpiritAnglerWaders, ids.AnglerBoots, 1.025},
		{"wrong hat, rest full", 995, ids.AnglerTop, ids.AnglerWaders, ids.AnglerBoots, 1.016},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p := newTestPlayer("alice", 3000, 3000)
			// Start from empty slots: SetItem with an empty
			// item clears the slot.
			p.Equipment().SetItem(game.HeadSlot, empty)
			p.Equipment().SetItem(game.BodySlot, empty)
			p.Equipment().SetItem(game.LegSlot, empty)
			p.Equipment().SetItem(game.FeetSlot, empty)
			if c.hat >= 0 {
				p.Equipment().SetItem(game.HeadSlot, game.NewItem(c.hat, 1))
			}
			if c.top >= 0 {
				p.Equipment().SetItem(game.BodySlot, game.NewItem(c.top, 1))
			}
			if c.waders >= 0 {
				p.Equipment().SetItem(game.LegSlot, game.NewItem(c.waders, 1))
			}
			if c.boots >= 0 {
				p.Equipment().SetItem(game.FeetSlot, game.NewItem(c.boots, 1))
			}
			if got := f.anglerXPMultiplier(p); got != c.want {
				t.Fatalf("anglerXPMultiplier = %v, want %v", got, c.want)
			}
		})
	}
	// Nil safety: no ids, no player.
	if got := (&fishing{}).anglerXPMultiplier(nil); got != 1 {
		t.Errorf("no-ids anglerXPMultiplier(nil) = %v, want 1", got)
	}
}

func TestAnglerWearsFullOutfit(t *testing.T) {
	f, _ := newTestFishing(t)
	ids := f.ids
	full := newTestPlayer("alice", 3000, 3000)
	equipAngler(t, full, ids.AnglerHat, ids.AnglerTop, ids.AnglerWaders, ids.AnglerBoots)
	if !f.anglerWearsFullOutfit(full) {
		t.Error("full angler outfit must count as full")
	}
	spirit := newTestPlayer("bob", 3000, 3000)
	equipAngler(t, spirit, ids.SpiritAnglerHeadband, ids.SpiritAnglerTop, ids.SpiritAnglerWaders, ids.SpiritAnglerBoots)
	if !f.anglerWearsFullOutfit(spirit) {
		t.Error("full spirit outfit must count as full")
	}
	partial := newTestPlayer("carol", 3000, 3000)
	equipAngler(t, partial, ids.AnglerHat, ids.AnglerTop, ids.AnglerWaders, -1)
	// The -1 boots slot holds an empty item, not boots.
	if f.anglerWearsFullOutfit(partial) {
		t.Error("three pieces must not count as full")
	}
	if f.anglerWearsFullOutfit(nil) {
		t.Error("nil player must not count as full")
	}
	if (&fishing{}).anglerWearsFullOutfit(full) {
		t.Error("no-ids instance must not count any outfit as full")
	}
}

func TestAnglerSetText(t *testing.T) {
	f, _ := newTestFishing(t)
	ids := f.ids
	full := newTestPlayer("alice", 3000, 3000)
	equipAngler(t, full, ids.AnglerHat, ids.AnglerTop, ids.AnglerWaders, ids.AnglerBoots)
	req := &game.EquipmentEffectsRequest{Player: full}
	f.addAnglerSetText(req)
	if len(req.SetTexts) != 1 || req.SetTexts[0] != anglerSetText {
		t.Fatalf("full angler SetTexts = %v, want [%d]", req.SetTexts, anglerSetText)
	}
	spirit := newTestPlayer("bob", 3000, 3000)
	equipAngler(t, spirit, ids.SpiritAnglerHeadband, ids.SpiritAnglerTop, ids.SpiritAnglerWaders, ids.SpiritAnglerBoots)
	req = &game.EquipmentEffectsRequest{Player: spirit}
	f.addAnglerSetText(req)
	if len(req.SetTexts) != 1 || req.SetTexts[0] != spiritAnglerSetText {
		t.Fatalf("full spirit SetTexts = %v, want [%d]", req.SetTexts, spiritAnglerSetText)
	}
	mixed := newTestPlayer("carol", 3000, 3000)
	equipAngler(t, mixed, ids.SpiritAnglerHeadband, ids.AnglerTop, ids.AnglerWaders, ids.AnglerBoots)
	req = &game.EquipmentEffectsRequest{Player: mixed}
	f.addAnglerSetText(req)
	if len(req.SetTexts) != 1 || req.SetTexts[0] != anglerSetText {
		t.Fatalf("mixed full SetTexts = %v, want the angler text [%d]", req.SetTexts, anglerSetText)
	}
	partial := newTestPlayer("dave", 3000, 3000)
	equipAngler(t, partial, ids.AnglerHat, ids.AnglerTop, -1, -1)
	req = &game.EquipmentEffectsRequest{Player: partial}
	f.addAnglerSetText(req)
	if len(req.SetTexts) != 0 {
		t.Fatalf("partial outfit SetTexts = %v, want none", req.SetTexts)
	}
	// Wrong payload types and nils are no-ops, not panics.
	f.addAnglerSetText("not a request")
	f.addAnglerSetText(nil)
	f.addAnglerSetText(&game.EquipmentEffectsRequest{})
}

func TestRegisterAnglerOutfit(t *testing.T) {
	f, _ := newTestFishing(t)
	ids := f.ids
	hub := hubWith(t, func(api *plugins.Api) { f.registerAnglerOutfit(api) })
	full := newTestPlayer("alice", 3000, 3000)
	equipAngler(t, full, ids.AnglerHat, ids.AnglerTop, ids.AnglerWaders, ids.AnglerBoots)
	req := &game.EquipmentEffectsRequest{Player: full}
	hub.EmitCustomEvent(game.EquipmentEffectsEventName, req)
	if len(req.SetTexts) != 1 || req.SetTexts[0] != anglerSetText {
		t.Fatalf("hub round-trip SetTexts = %v, want [%d]", req.SetTexts, anglerSetText)
	}
	// Nil api is a no-op, not a panic.
	f.registerAnglerOutfit(nil)
}
