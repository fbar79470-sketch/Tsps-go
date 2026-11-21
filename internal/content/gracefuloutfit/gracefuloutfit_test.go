package gracefuloutfit

import (
	"math"
	"testing"

	"tsps-go/internal/game"
	"tsps-go/internal/plugins"
)

// Fixture item ids and names. The ids are arbitrary: only the
// names matter (prefix matching), plus one non-graceful name per
// slot shape.
const (
	hoodID      = 101 // "Graceful hood"
	topID       = 102 // "Graceful top"
	legsID      = 103 // "Graceful legs"
	glovesID    = 104 // "Graceful gloves"
	bootsID     = 105 // "Graceful boots"
	capeID      = 106 // "Graceful cape"
	agilityID   = 107 // "Agility cape" (substitute holder, not a graceful name)
	variantID   = 108 // "Graceful hood (blue)" (colour variant: prefix match)
	platebodyID = 109 // "Iron platebody" (no match)
)

// fixtureDefs is a minimal game.ItemDefinitionSource double
// carrying names (the optional ItemNameSource capability) and
// weights. It mirrors the fakes in internal/game tests; this
// package cannot reuse them (they are test-only in another
// package).
type fixtureDefs struct {
	names   map[int]string
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
func (s fixtureDefs) ItemName(id int) string    { return s.names[id] }

func newTestPlayer() *game.Player {
	p := game.NewPlayer("testplayer", game.Location{X: 3200, Y: 3200, Z: 0})
	p.SetItemDefinitions(fixtureDefs{
		names: map[int]string{
			hoodID:      "Graceful hood",
			topID:       "Graceful top",
			legsID:      "Graceful legs",
			glovesID:    "Graceful gloves",
			bootsID:     "Graceful boots",
			capeID:      "Graceful cape",
			agilityID:   "Agility cape",
			variantID:   "Graceful hood (blue)",
			platebodyID: "Iron platebody",
		},
	})
	return p
}

func equip(p *game.Player, slot, id int) {
	p.Equipment().Set(slot, game.NewItem(id, 1))
}

func equipFullSet(p *game.Player) {
	equip(p, game.HeadSlot, hoodID)
	equip(p, game.BodySlot, topID)
	equip(p, game.LegSlot, legsID)
	equip(p, game.HandsSlot, glovesID)
	equip(p, game.FeetSlot, bootsID)
	equip(p, game.CapeSlot, capeID)
}

func equipFiveWithoutCape(p *game.Player) {
	equip(p, game.HeadSlot, hoodID)
	equip(p, game.BodySlot, topID)
	equip(p, game.LegSlot, legsID)
	equip(p, game.HandsSlot, glovesID)
	equip(p, game.FeetSlot, bootsID)
}

func almostEqual(t *testing.T, what string, got, want float64) {
	t.Helper()
	if math.Abs(got-want) > 1e-9 {
		t.Errorf("%s = %v, want %v", what, got, want)
	}
}

func TestGracefulRestoreBonusFromSlots(t *testing.T) {
	// Pure-function tests: no player, no emit, fixture maps only.
	names := func(pairs ...any) map[int]string {
		m := map[int]string{}
		for i := 0; i < len(pairs); i += 2 {
			m[pairs[i].(int)] = pairs[i+1].(string)
		}
		return m
	}
	full := names(
		game.HeadSlot, "Graceful hood",
		game.BodySlot, "Graceful top",
		game.LegSlot, "Graceful legs",
		game.HandsSlot, "Graceful gloves",
		game.FeetSlot, "Graceful boots",
		game.CapeSlot, "Graceful cape",
	)
	five := names(
		game.HeadSlot, "Graceful hood",
		game.BodySlot, "Graceful top",
		game.LegSlot, "Graceful legs",
		game.HandsSlot, "Graceful gloves",
		game.FeetSlot, "Graceful boots",
	)
	tests := []struct {
		name       string
		nameBySlot map[int]string
		substitute bool
		want       float64
	}{
		{"empty, no substitute", map[int]string{}, false, 0},
		{"nil map, no substitute", nil, false, 0},
		{"hood only", names(game.HeadSlot, "Graceful hood"), false, 0.03},
		{"top only", names(game.BodySlot, "Graceful top"), false, 0.04},
		{"hood and top", names(game.HeadSlot, "Graceful hood", game.BodySlot, "Graceful top"), false, 0.07},
		{"colour variant prefix", names(game.HeadSlot, "Graceful hood (blue)"), false, 0.03},
		{"non-graceful name", names(game.BodySlot, "Iron platebody"), false, 0},
		{"bare Graceful matches nothing", names(game.HeadSlot, "Graceful"), false, 0},
		{"empty name matches nothing", names(game.HeadSlot, ""), false, 0},
		{"full set", full, false, 0.30},
		{"full set with substitute is not double-counted", full, true, 0.30},
		{"five plus substitute completes the set", five, true, 0.30},
		{"five without substitute", five, false, 0.17},
		{"substitute alone is the cape piece", map[int]string{}, true, 0.03},
		{"substitute with a non-graceful cape worn", names(game.CapeSlot, "Agility cape"), true, 0.03},
		{"substitute with a graceful cape worn adds nothing", names(game.CapeSlot, "Graceful cape"), true, 0.03},
		{"wrong slot does not count", names(game.RingSlot, "Graceful hood"), false, 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			almostEqual(t, "bonus", gracefulRestoreBonusFromSlots(tc.nameBySlot, tc.substitute), tc.want)
		})
	}
}

func TestEnergyRestoreBonusQueriesSkillcape(t *testing.T) {
	queries := 0
	var last *game.SkillcapePerkRequest
	s := newOutfit(func(name string, payload any) {
		queries++
		if name != game.SkillcapeHasPerkEventName {
			t.Errorf("emit name = %q, want %q", name, game.SkillcapeHasPerkEventName)
		}
		last, _ = payload.(*game.SkillcapePerkRequest)
	})
	p := newTestPlayer()
	equipFullSet(p)
	almostEqual(t, "full-set bonus", s.energyRestoreBonus(p), 0.30)
	if queries != 1 {
		t.Fatalf("queries = %d, want 1", queries)
	}
	if last == nil || last.Player != p || last.Skill != "AGILITY" || last.Result {
		t.Fatalf("request = %+v, want {player, AGILITY, false}", last)
	}
	if last.Inventory {
		t.Errorf("request.Inventory = true, want false (TS request carries no inventory flag)")
	}
}

func TestEnergyRestoreBonusWithAnswerer(t *testing.T) {
	s := newOutfit(func(name string, payload any) {
		if req, ok := payload.(*game.SkillcapePerkRequest); ok {
			req.Result = true
		}
	})
	p := newTestPlayer()
	equipFiveWithoutCape(p)
	equip(p, game.CapeSlot, agilityID)
	almostEqual(t, "five plus substitute", s.energyRestoreBonus(p), 0.30)
}

func TestEnergyRestoreBonusNilPlayer(t *testing.T) {
	s := newOutfit(func(string, any) { t.Error("emit called for a nil player") })
	if got := s.energyRestoreBonus(nil); got != 0 {
		t.Errorf("energyRestoreBonus(nil) = %v, want 0", got)
	}
}

func TestApplyGracefulRunEnergy(t *testing.T) {
	s := newOutfit(nil) // No answerers: substitute never triggers.
	p := newTestPlayer()
	if got := s.applyGracefulRunEnergy(p, 100); got != 100 {
		t.Errorf("naked apply = %v, want 100 unchanged", got)
	}
	equipFullSet(p)
	if got := s.applyGracefulRunEnergy(p, 100); math.Abs(got-130) > 1e-9 {
		t.Errorf("full-set apply = %v, want 130", got)
	}
	// One piece: 3% on 100 units.
	p2 := newTestPlayer()
	equip(p2, game.HeadSlot, hoodID)
	if got := s.applyGracefulRunEnergy(p2, 100); math.Abs(got-103) > 1e-9 {
		t.Errorf("hood-only apply = %v, want 103", got)
	}
}

func TestApplyGracefulRunEnergyCachesUntilSignatureChanges(t *testing.T) {
	queries := 0
	s := newOutfit(func(string, any) { queries++ })
	p := newTestPlayer()
	equipFullSet(p)
	s.applyGracefulRunEnergy(p, 100)
	s.applyGracefulRunEnergy(p, 100)
	s.applyGracefulRunEnergy(p, 100)
	if queries != 1 {
		t.Fatalf("queries after 3 applies = %d, want 1 (cached)", queries)
	}
	// Changing one worn id recomputes.
	equip(p, game.HeadSlot, platebodyID)
	s.applyGracefulRunEnergy(p, 100)
	if queries != 2 {
		t.Fatalf("queries after equipment change = %d, want 2", queries)
	}
	// Disconnect cleanup drops the entry: the next apply recomputes.
	s.onPlayerDisconnect(&plugins.PlayerDisconnectEvent{Player: p})
	s.applyGracefulRunEnergy(p, 100)
	if queries != 3 {
		t.Fatalf("queries after disconnect = %d, want 3", queries)
	}
}

func TestApplyGracefulRunEnergyNilPlayer(t *testing.T) {
	s := newOutfit(nil)
	if got := s.applyGracefulRunEnergy(nil, 42); got != 42 {
		t.Errorf("apply(nil, 42) = %v, want 42", got)
	}
}

func TestAddGracefulWeights(t *testing.T) {
	p := newTestPlayer()
	equipFullSet(p)
	slotWeights := map[int]float64{}
	if worn := addGracefulWeights(p, slotWeights); worn != 6 {
		t.Fatalf("worn = %d, want 6", worn)
	}
	want := map[int]float64{
		game.HeadSlot:  -3,
		game.BodySlot:  -5,
		game.LegSlot:   -6,
		game.HandsSlot: -3,
		game.FeetSlot:  -4,
		game.CapeSlot:  -4,
	}
	if len(slotWeights) != len(want) {
		t.Fatalf("slotWeights = %v, want %v", slotWeights, want)
	}
	for slot, kg := range want {
		if slotWeights[slot] != kg {
			t.Errorf("slotWeights[%d] = %v, want %v", slot, slotWeights[slot], kg)
		}
	}
}

func TestAddGracefulWeightsPartialAndVariant(t *testing.T) {
	p := newTestPlayer()
	equip(p, game.HeadSlot, variantID) // Colour variant: prefix match.
	equip(p, game.BodySlot, platebodyID)
	slotWeights := map[int]float64{}
	if worn := addGracefulWeights(p, slotWeights); worn != 1 {
		t.Fatalf("worn = %d, want 1", worn)
	}
	if slotWeights[game.HeadSlot] != -3 {
		t.Errorf("slotWeights[head] = %v, want -3", slotWeights[game.HeadSlot])
	}
	if _, ok := slotWeights[game.BodySlot]; ok {
		t.Errorf("slotWeights has a body entry %v, want none", slotWeights)
	}
}

func TestAddGracefulWeightsOverwritesUnconditionally(t *testing.T) {
	// Graceful overwrites whatever is there (unlike WornWeights,
	// which keeps an already-finite override).
	p := newTestPlayer()
	equip(p, game.HeadSlot, hoodID)
	slotWeights := map[int]float64{game.HeadSlot: 99}
	addGracefulWeights(p, slotWeights)
	if slotWeights[game.HeadSlot] != -3 {
		t.Errorf("slotWeights[head] = %v, want -3 (overwrite)", slotWeights[game.HeadSlot])
	}
}

func TestAddGracefulWeightsNilGuards(t *testing.T) {
	if worn := addGracefulWeights(nil, map[int]float64{}); worn != 0 {
		t.Errorf("nil player worn = %d, want 0", worn)
	}
	if worn := addGracefulWeights(newTestPlayer(), nil); worn != 0 {
		t.Errorf("nil map worn = %d, want 0", worn)
	}
	// A zero-value player has no equipment container.
	if worn := addGracefulWeights(&game.Player{}, map[int]float64{}); worn != 0 {
		t.Errorf("zero player worn = %d, want 0", worn)
	}
}

func TestAddGracefulEffectsFullSet(t *testing.T) {
	s := newOutfit(nil)
	p := newTestPlayer()
	equipFullSet(p)
	req := &game.EquipmentEffectsRequest{Player: p, SlotWeights: map[int]float64{}}
	s.addGracefulEffects(req)
	if len(req.Lines) != 1 || req.Lines[0] != "Graceful set: +30% run energy restore" {
		t.Errorf("Lines = %v, want [Graceful set: +30%% run energy restore]", req.Lines)
	}
	if len(req.SetTexts) != 1 || req.SetTexts[0] != setText {
		t.Errorf("SetTexts = %v, want [%d]", req.SetTexts, setText)
	}
	if req.SlotWeights[game.HeadSlot] != -3 || len(req.SlotWeights) != 6 {
		t.Errorf("SlotWeights = %v, want all six weights filled", req.SlotWeights)
	}
}

func TestAddGracefulEffectsFivePlusSubstitute(t *testing.T) {
	s := newOutfit(func(_ string, payload any) {
		if req, ok := payload.(*game.SkillcapePerkRequest); ok {
			req.Result = true
		}
	})
	p := newTestPlayer()
	equipFiveWithoutCape(p)
	equip(p, game.CapeSlot, agilityID)
	req := &game.EquipmentEffectsRequest{Player: p, SlotWeights: map[int]float64{}}
	s.addGracefulEffects(req)
	if len(req.Lines) != 1 || req.Lines[0] != "Graceful set: +30% run energy restore" {
		t.Errorf("Lines = %v, want the set line", req.Lines)
	}
	if len(req.SetTexts) != 1 || req.SetTexts[0] != setText {
		t.Errorf("SetTexts = %v, want [%d]", req.SetTexts, setText)
	}
}

func TestAddGracefulEffectsFiveWithoutSubstitute(t *testing.T) {
	s := newOutfit(nil) // No answerers: substitute never triggers.
	p := newTestPlayer()
	equipFiveWithoutCape(p)
	equip(p, game.CapeSlot, agilityID)
	req := &game.EquipmentEffectsRequest{Player: p, SlotWeights: map[int]float64{}}
	s.addGracefulEffects(req)
	if len(req.Lines) != 0 || len(req.SetTexts) != 0 {
		t.Errorf("Lines/SetTexts = %v/%v, want none", req.Lines, req.SetTexts)
	}
	// The weights step still ran.
	if len(req.SlotWeights) != 5 {
		t.Errorf("SlotWeights = %v, want five weights", req.SlotWeights)
	}
}

func TestAddGracefulEffectsNilGuards(t *testing.T) {
	s := newOutfit(nil)
	s.addGracefulEffects(nil) // Must not panic.
	s.addGracefulEffects(&game.EquipmentEffectsRequest{SlotWeights: map[int]float64{}})
	// No slotWeights map: the weights step no-ops, so no line.
	p := newTestPlayer()
	equipFullSet(p)
	req := &game.EquipmentEffectsRequest{Player: p}
	s.addGracefulEffects(req)
	if len(req.Lines) != 0 {
		t.Errorf("Lines = %v, want none without a slotWeights map", req.Lines)
	}
}

func TestCustomEventHandlersIgnoreWrongPayloads(t *testing.T) {
	s := newOutfit(nil)
	s.onEquipmentEffects(nil)
	s.onEquipmentEffects("not a request")
	s.onEquipmentEffects(&game.EquipmentWeightsRequest{})
	s.onEquipmentWeights(nil)
	s.onEquipmentWeights(42)
	s.onEquipmentWeights(&game.EquipmentEffectsRequest{})
	s.onPlayerDisconnect(nil)
}

func TestPluginRegistersThroughHub(t *testing.T) {
	s := newOutfit(nil)
	hub := plugins.NewHub([]plugins.Plugin{s.plugin()}, plugins.HubOptions{MembersWorld: true})
	hub.Load()
	if got := hub.Loaded(); len(got) != 1 || got[0] != "GracefulOutfit" {
		t.Fatalf("Loaded() = %v, want [GracefulOutfit]", got)
	}
	p := newTestPlayer()
	equipFullSet(p)
	// The weights event delivered through the hub fills the map.
	weights := &game.EquipmentWeightsRequest{Player: p, SlotWeights: map[int]float64{}}
	hub.EmitCustomEvent(game.EquipmentWeightsEventName, weights)
	if len(weights.SlotWeights) != 6 || weights.SlotWeights[game.LegSlot] != -6 {
		t.Fatalf("SlotWeights = %v, want six graceful weights", weights.SlotWeights)
	}
	// The effects event delivered through the hub pushes the line
	// (in production no engine producer emits it; the wiring is
	// still verified here).
	effects := &game.EquipmentEffectsRequest{Player: p, SlotWeights: map[int]float64{}}
	hub.EmitCustomEvent(game.EquipmentEffectsEventName, effects)
	if len(effects.Lines) != 1 {
		t.Fatalf("Lines = %v, want one set line", effects.Lines)
	}
	// Disconnect through the hub is a no-panic smoke; the
	// cache-clear assertion lives in
	// TestApplyGracefulRunEnergyCachesUntilSignatureChanges.
	hub.EmitPlayerDisconnect(&plugins.PlayerDisconnectEvent{Player: p, Username: p.Username, Source: "removed"})
}

func TestPluginSkippedOnFreeToPlayWorld(t *testing.T) {
	s := newOutfit(nil)
	hub := plugins.NewHub([]plugins.Plugin{s.plugin()}, plugins.HubOptions{})
	hub.Load()
	if got := hub.Loaded(); len(got) != 0 {
		t.Fatalf("Loaded() = %v, want none (members plugin on a free world)", got)
	}
}

func TestPluginThroughHubWithSkillcapeAnswerer(t *testing.T) {
	// A stand-in SkillCapes answerer proves the query/answer
	// contract through the real hub bus: the substitute path
	// fires only when an answerer sets Result.
	answerer := plugins.Plugin{
		Name: "SkillcapeAnswerer",
		Register: func(api *plugins.Api) {
			api.OnCustomEvent(game.SkillcapeHasPerkEventName, func(payload any) {
				if req, ok := payload.(*game.SkillcapePerkRequest); ok && req.Skill == "AGILITY" {
					req.Result = true
				}
			})
		},
	}
	s := newOutfit(nil)
	hub := plugins.NewHub([]plugins.Plugin{s.plugin(), answerer}, plugins.HubOptions{MembersWorld: true})
	hub.Load()
	p := newTestPlayer()
	equipFiveWithoutCape(p)
	equip(p, game.CapeSlot, agilityID)
	effects := &game.EquipmentEffectsRequest{Player: p, SlotWeights: map[int]float64{}}
	hub.EmitCustomEvent(game.EquipmentEffectsEventName, effects)
	if len(effects.Lines) != 1 || effects.Lines[0] != "Graceful set: +30% run energy restore" {
		t.Fatalf("Lines = %v, want the set line via the answerer", effects.Lines)
	}
}

func TestPluginValueShape(t *testing.T) {
	if Plugin.Name != "GracefulOutfit" {
		t.Errorf("Plugin.Name = %q, want GracefulOutfit", Plugin.Name)
	}
	wantDesc := "Gives each graceful piece its run-energy restoration bonus, plus the full-set bonus, and counts an Agility or max cape as the graceful cape."
	if Plugin.Description != wantDesc {
		t.Errorf("Plugin.Description = %q, want verbatim TS description", Plugin.Description)
	}
	if !Plugin.Members {
		t.Error("Plugin.Members is false; the TS plugin sets members: true")
	}
	if Plugin.Register == nil {
		t.Error("Plugin.Register is nil")
	}
	if len(Plugin.DependsOn) != 0 {
		t.Errorf("Plugin.DependsOn = %v, want none", Plugin.DependsOn)
	}
}
