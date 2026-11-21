package game

import (
	"math"
	"sync"
	"testing"
)

// --- EquipmentEffects registry ---

func TestEquipmentEffectsApplyOrderAndGuards(t *testing.T) {
	effects := NewEquipmentEffects()
	p := NewPlayer("alice", Location{X: 3200, Y: 3200, Z: 0})
	// No modifiers: identity.
	if got := effects.ApplyRestoreModifiers(p, 20); got != 20 {
		t.Fatalf("empty restore apply = %v, want 20", got)
	}
	if got := effects.ApplyDrainModifiers(p, 59); got != 59 {
		t.Fatalf("empty drain apply = %v, want 59", got)
	}
	// Modifiers run in registration order, chained: 20 -> 24 -> 26.
	effects.RegisterRestoreModifier(func(pl *Player, units float64) float64 {
		if pl != p {
			t.Errorf("restore modifier got player %v, want p", pl)
		}
		return units * 1.2
	})
	effects.RegisterRestoreModifier(func(_ *Player, units float64) float64 { return units + 2 })
	if got := effects.ApplyRestoreModifiers(p, 20); got != 26 {
		t.Fatalf("chained restore = %v, want 26", got)
	}
	// A modifier returning NaN, +Inf, -Inf, or a negative keeps
	// the previous value (EquipmentEffects.ts:303-308); a zero
	// return is kept (the drain-to-zero case).
	effects.RegisterRestoreModifier(func(_ *Player, _ float64) float64 { return math.NaN() })
	effects.RegisterRestoreModifier(func(_ *Player, _ float64) float64 { return math.Inf(1) })
	effects.RegisterRestoreModifier(func(_ *Player, _ float64) float64 { return math.Inf(-1) })
	effects.RegisterRestoreModifier(func(_ *Player, _ float64) float64 { return -5 })
	if got := effects.ApplyRestoreModifiers(p, 20); got != 26 {
		t.Fatalf("guarded restore = %v, want 26", got)
	}
	drain := NewEquipmentEffects()
	drain.RegisterDrainModifier(func(_ *Player, _ float64) float64 { return 0 })
	if got := drain.ApplyDrainModifiers(p, 59); got != 0 {
		t.Fatalf("zero drain = %v, want 0", got)
	}
	drain.RegisterDrainModifier(func(_ *Player, _ float64) float64 { return math.NaN() })
	if got := drain.ApplyDrainModifiers(p, 59); got != 0 {
		t.Fatalf("NaN after zero drain = %v, want 0", got)
	}
}

func TestEquipmentEffectsNilHandling(t *testing.T) {
	p := NewPlayer("alice", Location{X: 3200, Y: 3200, Z: 0})
	// Nil registrations are ignored (the TS non-function guard).
	effects := NewEquipmentEffects()
	effects.RegisterRestoreModifier(nil)
	effects.RegisterDrainModifier(nil)
	if got := effects.ApplyRestoreModifiers(p, 20); got != 20 {
		t.Fatalf("after nil restore registration = %v, want 20", got)
	}
	if got := effects.ApplyDrainModifiers(p, 59); got != 59 {
		t.Fatalf("after nil drain registration = %v, want 59", got)
	}
	// A nil registry applies identity and accepts registration
	// silently (a standalone player reads through a nil field).
	var nilEffects *EquipmentEffects
	nilEffects.RegisterRestoreModifier(func(_ *Player, u float64) float64 { return u * 2 })
	nilEffects.RegisterDrainModifier(func(_ *Player, u float64) float64 { return u * 2 })
	if got := nilEffects.ApplyRestoreModifiers(p, 20); got != 20 {
		t.Fatalf("nil registry restore = %v, want 20", got)
	}
	if got := nilEffects.ApplyDrainModifiers(p, 59); got != 59 {
		t.Fatalf("nil registry drain = %v, want 59", got)
	}
	// The formula-level apply is identity for a nil player and
	// for a player with no registry installed.
	if got := applyRunEnergyRestoreModifiers(nil, 20); got != 20 {
		t.Fatalf("nil-player restore apply = %v, want 20", got)
	}
	if got := applyRunEnergyDrainModifiers(nil, 59); got != 59 {
		t.Fatalf("nil-player drain apply = %v, want 59", got)
	}
	if got := applyRunEnergyRestoreModifiers(p, 20); got != 20 {
		t.Fatalf("uninstalled restore apply = %v, want 20", got)
	}
}

func TestEquipmentEffectsConcurrentRegisterApply(t *testing.T) {
	// Registration concurrent with application must not race:
	// Apply snapshots under a read lock. Run under -race.
	effects := NewEquipmentEffects()
	p := NewPlayer("alice", Location{X: 3200, Y: 3200, Z: 0})
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				effects.RegisterRestoreModifier(func(_ *Player, u float64) float64 { return u })
			}
		}()
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				if got := effects.ApplyRestoreModifiers(p, 20); got != 20 {
					t.Errorf("concurrent apply = %v, want 20", got)
					return
				}
			}
		}()
	}
	wg.Wait()
}

// --- Formula wiring ---

func TestRestoreModifierAppliesThroughProcess(t *testing.T) {
	// Agility 50 restores a base of 20/tick; a +20% modifier
	// (the graceful shape) restores 24.
	p, session := restorePlayer(t, 9900, 50, 0)
	p.equipmentEffects = NewEquipmentEffects()
	p.equipmentEffects.RegisterRestoreModifier(func(_ *Player, units float64) float64 {
		return units * 1.2
	})
	p.Process(1)
	if got := p.GetRunEnergyUnits(); got != 9924 {
		t.Fatalf("units = %d, want 9924 (20 * 1.2)", got)
	}
	want := [][4]any{{9924, true, 0, 0}}
	if len(session.runEnergy) != 1 || session.runEnergy[0] != want[0] {
		t.Fatalf("runEnergy sends = %v, want %v", session.runEnergy, want)
	}
}

func TestRestoreModifierGuardKeepsBaseThroughProcess(t *testing.T) {
	// A modifier returning NaN keeps the base: agility 50
	// restores 20, as with no modifier.
	p, _ := restorePlayer(t, 9900, 50, 0)
	p.equipmentEffects = NewEquipmentEffects()
	p.equipmentEffects.RegisterRestoreModifier(func(_ *Player, _ float64) float64 {
		return math.NaN()
	})
	p.Process(1)
	if got := p.GetRunEnergyUnits(); got != 9920 {
		t.Fatalf("units = %d, want 9920 (NaN modifier ignored)", got)
	}
}

func TestDrainModifierAppliesThroughPlayerQueue(t *testing.T) {
	// Weight 0, agility 1: base drain 59; a 50% drain modifier
	// (the endurance-ring shape) drains floor(59 * 0.5) = 29.
	p, session := restorePlayer(t, 10000, 1, 0)
	p.equipmentEffects = NewEquipmentEffects()
	p.equipmentEffects.RegisterDrainModifier(func(_ *Player, amount float64) float64 {
		return amount * 0.5
	})
	p.MovementQueue().RequestWalk(Location{X: 3091, Y: 3524, Z: 0}, nil)
	p.Process(1)
	if got := p.GetRunEnergyUnits(); got != 10000-29 {
		t.Fatalf("units = %d, want %d", got, 10000-29)
	}
	want := [][4]any{{9971, true, 0, 0}}
	if len(session.runEnergy) != 1 || session.runEnergy[0] != want[0] {
		t.Fatalf("runEnergy sends = %v, want %v", session.runEnergy, want)
	}
}

func TestMovementOwnerPlayerResolution(t *testing.T) {
	p := NewPlayer("alice", Location{X: 3200, Y: 3200, Z: 0})
	if got := movementOwnerPlayer(p); got != p {
		t.Fatal("a player owner must resolve to itself")
	}
	n := NewNPC(1, Location{X: 3200, Y: 3200, Z: 0})
	if got := movementOwnerPlayer(n); got != nil {
		t.Fatal("an NPC owner must resolve to nil (NPC queues never drain)")
	}
	owner := newFakeOwner(Location{X: 100, Y: 100, Z: 0})
	if got := movementOwnerPlayer(owner); got != nil {
		t.Fatal("an owner without AsPlayer must resolve to nil (identity drain)")
	}
	if got := movementOwnerPlayer(nil); got != nil {
		t.Fatal("a nil owner must resolve to nil")
	}
}

func TestWorldInstallsEquipmentEffectsOnAdd(t *testing.T) {
	w := NewWorld(Location{X: 3200, Y: 3200, Z: 0})
	p := NewPlayer("alice", Location{X: 3200, Y: 3200, Z: 0})
	if p.equipmentEffects != nil {
		t.Fatal("a standalone player must start without a registry")
	}
	w.QueueAddPlayer(p)
	w.Process()
	if p.equipmentEffects == nil {
		t.Fatal("a world-added player must have the world's registry installed")
	}
	if p.equipmentEffects != w.EquipmentEffects() {
		t.Fatal("the installed registry must be the world's registry")
	}
	// A second world has a different registry instance: no
	// process-wide sharing.
	w2 := NewWorld(Location{X: 3200, Y: 3200, Z: 0})
	if w.EquipmentEffects() == w2.EquipmentEffects() {
		t.Fatal("two worlds must not share a registry instance")
	}
}

// --- equipment:weights emit site ---

func TestCarriedWeightEmitsWeightsEvent(t *testing.T) {
	src := &testItemDefs{weights: map[int]int{4151: 2500}}
	p := testPlayerWithDefs(src)
	p.Equipment().Set(WeaponSlot, NewItem(4151, 1))
	var gotName string
	var gotRequest *EquipmentWeightsRequest
	p.EmitCustomEvent = func(name string, payload any) {
		gotName = name
		gotRequest, _ = payload.(*EquipmentWeightsRequest)
	}
	if got := p.CarriedWeightGrams(); got != 2500 {
		t.Fatalf("weight = %d, want 2500 (no overrides written)", got)
	}
	if gotName != EquipmentWeightsEventName {
		t.Fatalf("event name = %q, want %q", gotName, EquipmentWeightsEventName)
	}
	if gotRequest == nil {
		t.Fatal("payload must be a *EquipmentWeightsRequest")
	}
	if gotRequest.Player != p {
		t.Fatal("request player must be the weighed player")
	}
	if gotRequest.SlotWeights == nil || len(gotRequest.SlotWeights) != 0 {
		t.Fatalf("request slotWeights must start as an empty map, got %v", gotRequest.SlotWeights)
	}
}

func TestCarriedWeightOverrides(t *testing.T) {
	src := &testItemDefs{
		stackable: map[int]bool{995: true},
		noted:     map[int]bool{996: true},
		weights:   map[int]int{4151: 2500, 1127: 8000, 995: 5000, 996: 5000},
	}
	p := testPlayerWithDefs(src)
	p.Equipment().Set(WeaponSlot, NewItem(4151, 1)) // 2500g
	p.Equipment().Set(BodySlot, NewItem(1127, 1))   // 8000g
	p.Equipment().Set(ShieldSlot, NewItem(995, 10)) // stackable: 0g
	p.Inventory().SetItem(0, NewItem(1127, 1))      // 8000g, inventory
	// Graceful-style negative whole-kg override on the weapon
	// slot; a fractional override on the body slot; an override
	// on the stackable shield slot (wins over the 0g rule); an
	// override on the empty head slot (ignored — the slot holds
	// nothing). The inventory item always weighs its definition
	// weight: the inventory loop never consults overrides.
	p.EmitCustomEvent = func(_ string, payload any) {
		req := payload.(*EquipmentWeightsRequest)
		req.SlotWeights[WeaponSlot] = -3
		req.SlotWeights[BodySlot] = 1.5
		req.SlotWeights[ShieldSlot] = 2
		req.SlotWeights[HeadSlot] = -6
	}
	// -3000 + 1500 + 2000 + 8000 (inventory) = 8500.
	if got := p.CarriedWeightGrams(); got != 8500 {
		t.Fatalf("weight = %d, want 8500", got)
	}
	if got := p.CarriedWeightKg(); got != 8 {
		t.Fatalf("kg = %d, want 8", got)
	}
}

func TestCarriedWeightOverrideGuards(t *testing.T) {
	src := &testItemDefs{weights: map[int]int{4151: 2500, 1127: 8000}}
	newPlayer := func() *Player {
		p := testPlayerWithDefs(src)
		p.Equipment().Set(WeaponSlot, NewItem(4151, 1))
		p.Equipment().Set(BodySlot, NewItem(1127, 0)) // amount 0: unoccupied
		return p
	}
	// NaN and infinite overrides fall back to definition weight.
	for _, bad := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
		p := newPlayer()
		p.EmitCustomEvent = func(_ string, payload any) {
			payload.(*EquipmentWeightsRequest).SlotWeights[WeaponSlot] = bad
		}
		if got := p.CarriedWeightGrams(); got != 2500 {
			t.Fatalf("override %v: weight = %d, want 2500 (fallback)", bad, got)
		}
	}
	// An override on an amount-0 slot falls back (0g for the
	// unoccupied slot), not the override.
	p := newPlayer()
	p.EmitCustomEvent = func(_ string, payload any) {
		payload.(*EquipmentWeightsRequest).SlotWeights[BodySlot] = -6
	}
	if got := p.CarriedWeightGrams(); got != 2500 {
		t.Fatalf("amount-0 override: weight = %d, want 2500", got)
	}
	// No sink installed: definition weights, no emit.
	p = newPlayer()
	if got := p.CarriedWeightGrams(); got != 2500 {
		t.Fatalf("no sink: weight = %d, want 2500", got)
	}
}

func TestCarriedWeightWornWeightsRoundTrip(t *testing.T) {
	// The WornWeights shape: the worn-variant definition weight
	// in grams, divided by 1000 to kg, written as the override;
	// CarriedWeightGrams converts back with int(kg * 1000).
	// Fixture weights are the real worn-variant cache weights
	// from the WornWeights header comment (WornWeights.plugin.js
	// :4-6): 89 boots -4.535kg, 10554 penance gloves -4.535kg,
	// 10073 spotted cape -2.267kg, 10074 spottier cape -4.535kg.
	// The worn ids (88, 10553, 10069, 10071) carry inventory
	// weights the override replaces.
	wornVariant := map[int]int{88: 89, 10553: 10554, 10069: 10073, 10071: 10074}
	src := &testItemDefs{weights: map[int]int{
		88: 340, 89: -4535,
		10553: 340, 10554: -4535,
		10069: 453, 10073: -2267,
		10071: 453, 10074: -4535,
	}}
	cases := []struct {
		wornID, slot, wantGrams int
	}{
		{88, FeetSlot, -4535},
		{10553, HandsSlot, -4535},
		{10069, CapeSlot, -2267},
		{10071, CapeSlot, -4535},
	}
	for _, c := range cases {
		p := testPlayerWithDefs(src)
		p.Equipment().Set(c.slot, NewItem(c.wornID, 1))
		p.EmitCustomEvent = func(_ string, payload any) {
			req := payload.(*EquipmentWeightsRequest)
			for slot := range 14 {
				if variant, ok := wornVariant[p.Equipment().ItemAt(slot).ID()]; ok {
					req.SlotWeights[slot] = float64(src.WeightGrams(variant)) / 1000
				}
			}
		}
		if got := p.CarriedWeightGrams(); got != c.wantGrams {
			t.Errorf("worn %d: weight = %d, want %d", c.wornID, got, c.wantGrams)
		}
	}
}

func TestWeightOverrideFloatConversion(t *testing.T) {
	// Pin the exact int(override * 1000) conversion, including
	// the first gram value whose float round-trip truncates:
	// 1001g -> 1.001kg -> 1000g (measured; see weight.go).
	cases := []struct {
		grams, want int
	}{
		{0, 0}, {1, 1}, {250, 250}, {300, 300}, {340, 340},
		{453, 453}, {500, 500}, {1000, 1000},
		{-4535, -4535}, {-2267, -2267}, {-3000, -3000},
		{1001, 1000}, {-1001, -1000},
	}
	for _, c := range cases {
		kg := float64(c.grams) / 1000
		if got := int(kg * 1000); got != c.want {
			t.Errorf("int(%d/1000*1000) = %d, want %d", c.grams, got, c.want)
		}
	}
	// And through the slot rule itself: the 1001g truncation is
	// observable in the total, the kg total is unaffected.
	src := &testItemDefs{weights: map[int]int{4151: 2500}}
	p := testPlayerWithDefs(src)
	p.Equipment().Set(WeaponSlot, NewItem(4151, 1))
	p.EmitCustomEvent = func(_ string, payload any) {
		payload.(*EquipmentWeightsRequest).SlotWeights[WeaponSlot] = 1.001
	}
	if got := p.CarriedWeightGrams(); got != 1000 {
		t.Fatalf("1.001kg override: grams = %d, want 1000", got)
	}
	if got := p.CarriedWeightKg(); got != 1 {
		t.Fatalf("1.001kg override: kg = %d, want 1", got)
	}
}

// --- ItemName seam ---

// namedTestItemDefs is a testItemDefs with the optional
// ItemNameSource capability.
type namedTestItemDefs struct {
	*testItemDefs
	names map[int]string
}

func (s *namedTestItemDefs) ItemName(id int) string { return s.names[id] }

func TestPlayerItemName(t *testing.T) {
	src := &namedTestItemDefs{
		testItemDefs: &testItemDefs{},
		names:        map[int]string{4151: "Abyssal whip", 1052: "Cape"},
	}
	p := testPlayerWithDefs(src)
	if got := p.ItemName(4151); got != "Abyssal whip" {
		t.Fatalf("ItemName(4151) = %q, want %q", got, "Abyssal whip")
	}
	// An id the source has no name for reads "".
	if got := p.ItemName(999999); got != "" {
		t.Fatalf("ItemName(unknown) = %q, want empty", got)
	}
	// ids <= 0 read "" without consulting the source (TS null).
	if got := p.ItemName(0); got != "" {
		t.Fatalf("ItemName(0) = %q, want empty", got)
	}
	if got := p.ItemName(-1); got != "" {
		t.Fatalf("ItemName(-1) = %q, want empty", got)
	}
	// No source installed reads "".
	unsourced := testPlayerWithDefs(nil)
	if got := unsourced.ItemName(4151); got != "" {
		t.Fatalf("unsourced ItemName = %q, want empty", got)
	}
	// A source without the capability reads "".
	plain := testPlayerWithDefs(&testItemDefs{})
	if got := plain.ItemName(4151); got != "" {
		t.Fatalf("capability-less ItemName = %q, want empty", got)
	}
	// A nil player reads "".
	var nilPlayer *Player
	if got := nilPlayer.ItemName(4151); got != "" {
		t.Fatalf("nil-player ItemName = %q, want empty", got)
	}
}
