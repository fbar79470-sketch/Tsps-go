package wornweights

import (
	"math"
	"testing"

	"tsps-go/internal/game"
	"tsps-go/internal/plugins"
)

// fixtureDefs is a minimal game.ItemDefinitionSource double
// carrying weights (the only capability addWornWeights reads).
// It mirrors the fakes in internal/game tests; this package
// cannot reuse them (they are test-only in another package).
type fixtureDefs struct {
	weights map[int]int
}

func (s fixtureDefs) IsStackable(int) bool      { return false }
func (s fixtureDefs) IsNoted(int) bool          { return false }
func (s fixtureDefs) WeightGrams(id int) int    { return s.weights[id] }
func (s fixtureDefs) EquipmentSlot(int) int     { return -1 }
func (s fixtureDefs) IsDoubleHanded(int) bool   { return false }
func (s fixtureDefs) Requirements(int) []int    { return nil }
func (s fixtureDefs) CombatRequirement(int) int { return 0 }
func (s fixtureDefs) IsTradeable(int) bool      { return true }
func (s fixtureDefs) IsSellable(int) bool       { return true }
func (s fixtureDefs) IsDropable(int) bool       { return true }
func (s fixtureDefs) UnNote(id int) int         { return id }
func (s fixtureDefs) PlaceholderID(int) int     { return -1 }

func newTestPlayer() *game.Player {
	p := game.NewPlayer("testplayer", game.Location{X: 3200, Y: 3200, Z: 0})
	// Worn weights from the TS header comment
	// (WornWeights.plugin.js:5-8): boots 89 -4.535 kg, penance
	// gloves 10554 the same, spotted cape 10073 -2.267 kg,
	// spottier cape 10074 -4.535 kg.
	p.SetItemDefinitions(fixtureDefs{weights: map[int]int{
		89:    -4535,
		10554: -4535,
		10073: -2267,
		10074: -4535,
	}})
	return p
}

func equip(p *game.Player, slot, id int) {
	p.Equipment().Set(slot, game.NewItem(id, 1))
}

func TestWornWeightIDMap(t *testing.T) {
	want := map[int]int{88: 89, 10553: 10554, 10069: 10073, 10071: 10074}
	if len(wornWeightID) != len(want) {
		t.Fatalf("wornWeightID = %v, want %v", wornWeightID, want)
	}
	for from, to := range want {
		if wornWeightID[from] != to {
			t.Errorf("wornWeightID[%d] = %d, want %d", from, wornWeightID[from], to)
		}
	}
}

func TestAddWornWeights(t *testing.T) {
	tests := []struct {
		name string
		slot int
		id   int
		want float64
	}{
		{"boots of lightness", game.FeetSlot, 88, -4.535},
		{"penance gloves", game.HandsSlot, 10553, -4.535},
		{"spotted cape", game.CapeSlot, 10069, -2.267},
		{"spottier cape", game.CapeSlot, 10071, -4.535},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := newTestPlayer()
			equip(p, tc.slot, tc.id)
			req := &game.EquipmentWeightsRequest{Player: p, SlotWeights: map[int]float64{}}
			addWornWeights(req)
			got, ok := req.SlotWeights[tc.slot]
			if !ok {
				t.Fatalf("SlotWeights = %v, want an entry for slot %d", req.SlotWeights, tc.slot)
			}
			if math.Abs(got-tc.want) > 1e-9 {
				t.Errorf("SlotWeights[%d] = %v, want %v", tc.slot, got, tc.want)
			}
			if len(req.SlotWeights) != 1 {
				t.Errorf("SlotWeights = %v, want exactly one entry", req.SlotWeights)
			}
		})
	}
}

func TestAddWornWeightsAllSlotsAtOnce(t *testing.T) {
	p := newTestPlayer()
	equip(p, game.FeetSlot, 88)
	equip(p, game.HandsSlot, 10553)
	equip(p, game.CapeSlot, 10069)
	req := &game.EquipmentWeightsRequest{Player: p, SlotWeights: map[int]float64{}}
	addWornWeights(req)
	want := map[int]float64{game.FeetSlot: -4.535, game.HandsSlot: -4.535, game.CapeSlot: -2.267}
	if len(req.SlotWeights) != len(want) {
		t.Fatalf("SlotWeights = %v, want %v", req.SlotWeights, want)
	}
	for slot, kg := range want {
		if math.Abs(req.SlotWeights[slot]-kg) > 1e-9 {
			t.Errorf("SlotWeights[%d] = %v, want %v", slot, req.SlotWeights[slot], kg)
		}
	}
}

func TestAddWornWeightsFirstWriterWins(t *testing.T) {
	// An already-finite override keeps its value; NaN and
	// infinities are not finite, so they are overwritten.
	tests := []struct {
		name     string
		existing float64
		want     float64
	}{
		{"finite override kept", -3, -3},
		{"zero override kept", 0, 0},
		{"NaN overwritten", math.NaN(), -4.535},
		{"+Inf overwritten", math.Inf(1), -4.535},
		{"-Inf overwritten", math.Inf(-1), -4.535},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := newTestPlayer()
			equip(p, game.FeetSlot, 88)
			req := &game.EquipmentWeightsRequest{
				Player:      p,
				SlotWeights: map[int]float64{game.FeetSlot: tc.existing},
			}
			addWornWeights(req)
			if math.Abs(req.SlotWeights[game.FeetSlot]-tc.want) > 1e-9 {
				t.Errorf("SlotWeights[feet] = %v, want %v", req.SlotWeights[game.FeetSlot], tc.want)
			}
		})
	}
}

func TestAddWornWeightsIgnoresUnmapped(t *testing.T) {
	p := newTestPlayer()
	equip(p, game.HeadSlot, 1169) // Unmapped id.
	equip(p, game.BodySlot, 89)   // The worn id itself is not a key.
	equip(p, game.WeaponSlot, 88) // Mapped id in any slot still counts (all slots iterate).
	req := &game.EquipmentWeightsRequest{Player: p, SlotWeights: map[int]float64{}}
	addWornWeights(req)
	if len(req.SlotWeights) != 1 || math.Abs(req.SlotWeights[game.WeaponSlot]+4.535) > 1e-9 {
		t.Errorf("SlotWeights = %v, want only the weapon-slot boots entry", req.SlotWeights)
	}
}

func TestAddWornWeightsNilGuards(t *testing.T) {
	addWornWeights(nil) // Must not panic.
	addWornWeights(&game.EquipmentWeightsRequest{SlotWeights: map[int]float64{}})
	addWornWeights(&game.EquipmentWeightsRequest{Player: newTestPlayer()})
	// A zero-value player has no equipment container.
	addWornWeights(&game.EquipmentWeightsRequest{Player: &game.Player{}, SlotWeights: map[int]float64{}})
	// A player with no definition source installed gets no overrides.
	p := game.NewPlayer("nodefs", game.Location{X: 3200, Y: 3200, Z: 0})
	equip(p, game.FeetSlot, 88)
	req := &game.EquipmentWeightsRequest{Player: p, SlotWeights: map[int]float64{}}
	addWornWeights(req)
	if len(req.SlotWeights) != 0 {
		t.Errorf("SlotWeights = %v, want none without a definition source", req.SlotWeights)
	}
}

func TestOnEquipmentWeightsIgnoresWrongPayloads(t *testing.T) {
	onEquipmentWeights(nil)
	onEquipmentWeights("not a request")
	onEquipmentWeights(&game.EquipmentEffectsRequest{})
}

func TestPluginRegistersThroughHub(t *testing.T) {
	// No members flag: loads on a free-to-play world too.
	hub := plugins.NewHub([]plugins.Plugin{Plugin}, plugins.HubOptions{})
	hub.Load()
	if got := hub.Loaded(); len(got) != 1 || got[0] != "WornWeights" {
		t.Fatalf("Loaded() = %v, want [WornWeights]", got)
	}
	p := newTestPlayer()
	equip(p, game.FeetSlot, 88)
	req := &game.EquipmentWeightsRequest{Player: p, SlotWeights: map[int]float64{}}
	hub.EmitCustomEvent(game.EquipmentWeightsEventName, req)
	if math.Abs(req.SlotWeights[game.FeetSlot]+4.535) > 1e-9 {
		t.Fatalf("SlotWeights = %v, want the boots worn weight", req.SlotWeights)
	}
}

func TestPluginValueShape(t *testing.T) {
	if Plugin.Name != "WornWeights" {
		t.Errorf("Plugin.Name = %q, want WornWeights", Plugin.Name)
	}
	wantDesc := "Makes boots of lightness, penance gloves and spotted and spottier capes weigh their lighter worn weight in the equipment panel and for run-energy drain."
	if Plugin.Description != wantDesc {
		t.Errorf("Plugin.Description = %q, want verbatim TS description", Plugin.Description)
	}
	if Plugin.Members {
		t.Error("Plugin.Members is true; the TS plugin sets no members flag")
	}
	if Plugin.Register == nil {
		t.Error("Plugin.Register is nil")
	}
	if len(Plugin.DependsOn) != 0 {
		t.Errorf("Plugin.DependsOn = %v, want none", Plugin.DependsOn)
	}
}
