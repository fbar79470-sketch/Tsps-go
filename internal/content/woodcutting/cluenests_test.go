package woodcutting

import (
	"testing"

	"tsps-go/internal/game"
	"tsps-go/internal/plugins"
)

// namedDefs is an ItemDefinitionSource test double naming items by
// id (the gracefuloutfit fixtureDefs pattern): ownsClue scans
// inventory + bank by display name, so the test names the clue
// forms it plants.
type namedDefs struct {
	stackableDefs
	names map[int]string
}

func (s namedDefs) ItemName(id int) string { return s.names[id] }

// TestBuildClueTiers pins the tier table (buildTiers): five tiers,
// modifiers 0.2/1.7/2/3.3/10, nest + scroll ids in tier order.
func TestBuildClueTiers(t *testing.T) {
	tiers := buildClueTiers()
	if len(tiers) != 5 {
		t.Fatalf("buildClueTiers = %d tiers, want 5", len(tiers))
	}
	want := []clueTier{
		{"beginner", 0.2, 23127, 23182},
		{"easy", 1.7, 19712, 2677},
		{"medium", 2, 19714, 2801},
		{"hard", 3.3, 19716, 2722},
		{"elite", 10, 19718, 12073},
	}
	for i, w := range want {
		if tiers[i] != w {
			t.Errorf("tier %d = %+v, want %+v", i, tiers[i], w)
		}
	}
	if got, ok := clueTierByNestID(19714); !ok || got.name != "medium" {
		t.Errorf("clueTierByNestID(19714) = %+v, %v, want medium, true", got, ok)
	}
	if _, ok := clueTierByNestID(995); ok {
		t.Error("clueTierByNestID(995) matched, want no match")
	}
}

// TestClueChance pins the tier chance math (rollClueNests). Values
// are hand-computed from the formula, not copied from the
// implementation: base = floor(petBase / (100 + level) x modifier),
// chance = max(1, floor(base / multiplier)) when base >= 1.
func TestClueChance(t *testing.T) {
	const normalPetBase = 317647 // normal tree
	cases := []struct {
		name                 string
		petBase, level       int
		modifier             float64
		thresholdMet         bool
		multiplier           float64
		wantBase, wantChance int
	}{
		// Normal L99: 317647/199 = 1596.216…; x0.2 = 319.24 → 319.
		{"normal L99 beginner", normalPetBase, 99, 0.2, false, 1, 319, 319},
		// x1.7 = 2713.56 → 2713.
		{"normal L99 easy", normalPetBase, 99, 1.7, false, 1, 2713, 2713},
		// x2 = 3192.43 → 3192.
		{"normal L99 medium", normalPetBase, 99, 2, false, 1, 3192, 3192},
		// x3.3 = 5267.53 → 5267.
		{"normal L99 hard", normalPetBase, 99, 3.3, false, 1, 5267, 5267},
		// x10 = 15962.16 → 15962.
		{"normal L99 elite", normalPetBase, 99, 10, false, 1, 15962, 15962},
		// CA met: floor(317647 x 95/100) = 301764; /199 = 1516.40;
		// x0.2 = 303.28 → 303.
		{"normal L99 beginner CA", normalPetBase, 99, 0.2, true, 1, 303, 303},
		// L1: 317647/101 = 3145.01; x0.2 = 629.00 → 629.
		{"normal L1 beginner", normalPetBase, 1, 0.2, false, 1, 629, 629},
		// Multiplier 2 halves the chance: floor(319/2) = 159.
		{"multiplier 2", normalPetBase, 99, 0.2, false, 2, 319, 159},
		// A multiplier <= 1 reads as 1.
		{"multiplier 0.5", normalPetBase, 99, 0.2, false, 0.5, 319, 319},
		// Base 1 with a big multiplier floors to 0, then max(1, 0).
		{"base 1 multiplier 10", 1000, 99, 0.2, false, 10, 1, 1},
		// Base 0 never rolls: chance stays 0.
		{"base 0", 100, 99, 0.2, false, 1, 0, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			base, chance := clueChance(tc.petBase, tc.level, tc.modifier, tc.thresholdMet, tc.multiplier)
			if base != tc.wantBase || chance != tc.wantChance {
				t.Errorf("clueChance = base %d chance %d, want base %d chance %d", base, chance, tc.wantBase, tc.wantChance)
			}
		})
	}
}

// testZoneSource is a ZoneBoundarySource test double answering one
// tag with one boundary.
type testZoneSource struct {
	tag      string
	boundary game.Boundary
}

func (s testZoneSource) ZoneBoundaries(tag string) []game.Boundary {
	if tag == s.tag {
		return []game.Boundary{s.boundary}
	}
	return nil
}

// TestHasWildernessRing pins the ring check: an imbued ring of
// wealth equipped while standing in levelled Wilderness counts;
// the same ring outside the Wilderness, or no ring, does not.
func TestHasWildernessRing(t *testing.T) {
	tables := loadFixtureTables(t)
	w, _ := newTestWoodcutting(t, tables)
	wild := game.NewWilderness(testZoneSource{
		tag:      "pvp",
		boundary: game.NewBoundary(3190, 3210, 3590, 3610, 0),
	}, nil)
	w.wilderness = wild
	// (3200, 3600) is inside the pvp boundary and inside the
	// levelled strip, so LevelAt = (3600-3520)/8+1 = 11.
	inWild := newTestPlayer("alice", 3200, 3600)
	if w.hasWildernessRing(inWild) {
		t.Error("no ring in the Wilderness: hasWildernessRing = true")
	}
	inWild.Equipment().Set(game.RingSlot, game.NewItem(12785, 1))
	if !w.hasWildernessRing(inWild) {
		t.Error("ring of wealth (i) in the Wilderness: hasWildernessRing = false")
	}
	// An inventory ring does not count.
	inWild2 := newTestPlayer("bob", 3200, 3600)
	inWild2.Inventory().Adds(12785, 1)
	if w.hasWildernessRing(inWild2) {
		t.Error("ring in inventory: hasWildernessRing = true")
	}
	// Outside the Wilderness the ring does not count.
	outside := newTestPlayer("carol", 3200, 3200)
	outside.Equipment().Set(game.RingSlot, game.NewItem(20786, 1))
	if w.hasWildernessRing(outside) {
		t.Error("ring outside the Wilderness: hasWildernessRing = true")
	}
	// No Wilderness installed: the ring never doubles.
	w2, _ := newTestWoodcutting(t, tables)
	if w2.hasWildernessRing(inWild) {
		t.Error("no Wilderness installed: hasWildernessRing = true")
	}
}

// TestRollClueNestsWildernessRing pins the ring doubling in the
// roll: with randFloat 0.0005 on a normal tree at level 99,
// beginner (chance 319) rolls 0.1595 < 1 either way, easy (2713)
// rolls 1.3565 — skipped without the ring, doubled to < 2 with
// it — medium (3192) rolls 1.596 the same way, and hard (5267)
// rolls 2.6335, skipped even doubled.
func TestRollClueNestsWildernessRing(t *testing.T) {
	tables := loadFixtureTables(t)
	wild := game.NewWilderness(testZoneSource{
		tag:      "pvp",
		boundary: game.NewBoundary(3190, 3210, 3590, 3610, 0),
	}, nil)

	roll := func(inWild bool) []int {
		w, emitted := newTestWoodcutting(t, tables)
		w.randFloat = func() float64 { return 0.0005 }
		w.wilderness = wild
		x, y := 3200, 3200
		if inWild {
			x, y = 3200, 3600
		}
		p := newTestPlayer("alice", x, y)
		p.Session = &capturingSession{}
		if inWild {
			p.Equipment().Set(game.RingSlot, game.NewItem(12785, 1))
		}
		setLevel(p, game.SkillWoodcutting, 99)
		w.rollClueNests(p, treeByName(tables, "normal tree"))
		var ids []int
		for _, e := range *emitted {
			if req, ok := e.payload.(*ClueItemRequest); ok {
				ids = append(ids, req.ItemID)
			}
		}
		return ids
	}
	// Outside the Wilderness (no ring effect): beginner only.
	if got := roll(false); len(got) != 1 || got[0] != 23127 {
		t.Errorf("no ring: clue-item ids = %v, want [23127]", got)
	}
	// Ring in the Wilderness: beginner, easy, medium.
	if got := roll(true); len(got) != 3 || got[0] != 23127 || got[1] != 19712 || got[2] != 19714 {
		t.Errorf("ring in Wilderness: clue-item ids = %v, want [23127 19712 19714]", got)
	}
}

// TestRollClueNestsNoPetBase pins the skip: trees with no pet base
// never roll, emit nothing, and message nothing.
func TestRollClueNestsNoPetBase(t *testing.T) {
	tables := loadFixtureTables(t)
	w, emitted := newTestWoodcutting(t, tables)
	p, session := newVinePlayer("alice")
	setLevel(p, game.SkillWoodcutting, 99)
	dramen := treeByName(tables, "dramen")
	if dramen == nil || dramen.PetBase != 0 {
		t.Fatal("fixture dramen must have PetBase 0")
	}
	w.rollClueNests(p, dramen)
	if len(*emitted) != 0 {
		t.Errorf("emitted = %v, want none", *emitted)
	}
	if len(session.messages) != 0 {
		t.Errorf("messages = %v, want none", session.messages)
	}
}

// TestRollClueNestsAllTiers pins the roll at its most generous:
// with randFloat 0 every tier's roll succeeds (0 x chance < 1),
// so all five nests drop — five red messages, one per tier.
func TestRollClueNestsAllTiers(t *testing.T) {
	tables := loadFixtureTables(t)
	w, emitted := newTestWoodcutting(t, tables)
	w.randFloat = func() float64 { return 0 }
	p, session := newVinePlayer("alice")
	setLevel(p, game.SkillWoodcutting, 99)
	w.rollClueNests(p, treeByName(tables, "normal tree"))
	if len(session.messages) != 5 {
		t.Fatalf("messages = %v, want 5 nest messages", session.messages)
	}
	for i, m := range session.messages {
		if m != "<col=ff0000>A bird's nest falls out of the tree." {
			t.Errorf("message %d = %q, want the red nest line", i, m)
		}
	}
	// Five vessel-chance queries (one per tier) plus five clue-item
	// requests; the CA query fires for the four non-beginner tiers.
	var vessel, clueItem, ca int
	for _, e := range *emitted {
		switch e.name {
		case treasureTrailsVesselChanceEventName:
			vessel++
		case treasureTrailsClueItemEventName:
			clueItem++
		case combatAchievementsTierEventName:
			ca++
		}
	}
	if vessel != 5 || clueItem != 5 || ca != 4 {
		t.Errorf("emitted vessel=%d clue-item=%d ca=%d, want 5/5/4", vessel, clueItem, ca)
	}
}

// TestRollClueNestsHighRoll pins the roll at its stingiest: with
// randFloat 0.999 no tier rolls (0.999 x chance >= 1 for every
// chance >= 2), so nothing drops.
func TestRollClueNestsHighRoll(t *testing.T) {
	tables := loadFixtureTables(t)
	w, _ := newTestWoodcutting(t, tables)
	w.randFloat = func() float64 { return 0.999 }
	p, session := newVinePlayer("alice")
	setLevel(p, game.SkillWoodcutting, 99)
	w.rollClueNests(p, treeByName(tables, "normal tree"))
	if len(session.messages) != 0 {
		t.Errorf("messages = %v, want none", session.messages)
	}
}

// TestRollClueNestsVesselChanceBlocked pins the vessel-chance
// answer: an answerer that blocks the easy tier (Allowed false)
// skips it while the other tiers still roll.
func TestRollClueNestsVesselChanceBlocked(t *testing.T) {
	tables := loadFixtureTables(t)
	w, emitted := newTestWoodcutting(t, tables)
	blocked := false
	w.emit = func(name string, payload any) {
		*emitted = append(*emitted, emittedEvent{name: name, payload: payload})
		if req, ok := payload.(*VesselChanceRequest); ok && req.Tier == "easy" {
			req.Allowed = &blocked
		}
	}
	w.randFloat = func() float64 { return 0 }
	p, session := newVinePlayer("alice")
	setLevel(p, game.SkillWoodcutting, 99)
	w.rollClueNests(p, treeByName(tables, "normal tree"))
	if len(session.messages) != 4 {
		t.Fatalf("messages = %v, want 4 (easy blocked)", session.messages)
	}
	for _, e := range *emitted {
		if req, ok := e.payload.(*ClueItemRequest); ok && req.ItemID == 19712 {
			t.Error("blocked easy tier still emitted a clue-item request")
		}
	}
}

// TestRollClueNestsOwnedClueSkips pins the ownsClue path via the
// can-receive answer: an answerer that refuses the medium tier
// (Allowed false = owned) emits missed-clue and drops nothing for
// it, while the other tiers still roll.
func TestRollClueNestsOwnedClueSkips(t *testing.T) {
	tables := loadFixtureTables(t)
	w, emitted := newTestWoodcutting(t, tables)
	owned := false
	w.emit = func(name string, payload any) {
		*emitted = append(*emitted, emittedEvent{name: name, payload: payload})
		if req, ok := payload.(*CanReceiveClueRequest); ok && req.Tier == "medium" {
			req.Allowed = &owned
		}
	}
	w.randFloat = func() float64 { return 0 }
	p, session := newVinePlayer("alice")
	setLevel(p, game.SkillWoodcutting, 99)
	w.rollClueNests(p, treeByName(tables, "normal tree"))
	if len(session.messages) != 4 {
		t.Fatalf("messages = %v, want 4 (medium owned)", session.messages)
	}
	missed := 0
	for _, e := range *emitted {
		if ev, ok := e.payload.(*MissedClueEvent); ok && ev.Tier == "medium" {
			missed++
		}
		if req, ok := e.payload.(*ClueItemRequest); ok && req.ItemID == 19714 {
			t.Error("owned medium tier still emitted a clue-item request")
		}
	}
	if missed != 1 {
		t.Errorf("missed-clue emits for medium = %d, want 1", missed)
	}
}

// TestOwnsClueInventoryAndBank pins the fallback name scan: a clue
// form in the inventory or in any of the 11 bank tabs counts as
// owned; other items do not.
func TestOwnsClueInventoryAndBank(t *testing.T) {
	tables := loadFixtureTables(t)
	w, _ := newTestWoodcutting(t, tables)
	const scrollEasyID = 9001
	const scrollHardID = 9002
	const otherID = 9003
	easy := clueTierByNestIDMust(t, 19712)
	p := newTestPlayer("alice", 3200, 3200)
	p.SetItemDefinitions(namedDefs{names: map[int]string{
		scrollEasyID: "Clue scroll (easy)",
		scrollHardID: "Clue scroll (hard)",
		otherID:      "Logs",
	}})
	// Empty hands: not owned.
	if w.ownsClue(p, easy) {
		t.Error("empty player owns the easy clue")
	}
	// A different tier's scroll does not count.
	p.Inventory().Adds(scrollHardID, 1)
	if w.ownsClue(p, easy) {
		t.Error("hard scroll counts as owning easy")
	}
	// An unrelated item does not count.
	p.Inventory().Adds(otherID, 1)
	if w.ownsClue(p, easy) {
		t.Error("logs count as owning easy")
	}
	// The matching scroll in the inventory counts.
	p.Inventory().Adds(scrollEasyID, 1)
	if !w.ownsClue(p, easy) {
		t.Error("easy scroll in inventory not owned")
	}
	// The bank scan: clear the inventory, plant the scroll in the
	// last tab (tab 10).
	p2 := newTestPlayer("bob", 3200, 3200)
	p2.SetItemDefinitions(namedDefs{names: map[int]string{scrollEasyID: "Clue nest (easy)"}})
	p2.Bank(10).Adds(scrollEasyID, 1)
	if !w.ownsClue(p2, easy) {
		t.Error("easy nest in bank tab 10 not owned")
	}
	// A zero-amount slot does not count (the TS getAmount() > 0
	// guard).
	p3 := newTestPlayer("carol", 3200, 3200)
	p3.SetItemDefinitions(namedDefs{names: map[int]string{scrollEasyID: "Scroll box (easy)"}})
	p3.Inventory().SetItem(0, game.NewItem(scrollEasyID, 0))
	if w.ownsClue(p3, easy) {
		t.Error("zero-amount scroll counts as owned")
	}
}

func clueTierByNestIDMust(t *testing.T, nestID int) clueTier {
	t.Helper()
	tier, ok := clueTierByNestID(nestID)
	if !ok {
		t.Fatalf("no tier for nest %d", nestID)
	}
	return tier
}

// TestOpenClueNest pins the default open path: slot identity, the
// free-slot gate, then delete the nest and add an empty nest + the
// clue scroll, with the message.
func TestOpenClueNest(t *testing.T) {
	tables := loadFixtureTables(t)
	w, _ := newTestWoodcutting(t, tables)
	p, session := newVinePlayer("alice")
	// Slot 0 holds an easy nest; slot 1 holds a log (so the free
	// slot count is 26, not 27 — the gate still passes).
	p.Inventory().Adds(19712, 1)
	p.Inventory().Adds(treeByName(tables, "normal tree").LogID, 1)
	event := &plugins.PluginItemActionEvent{Player: p, ItemID: 19712, Slot: 0, Option: "Open"}
	if !w.openClueNest(event) {
		t.Fatal("openClueNest did not claim the nest")
	}
	if got := p.Inventory().GetAmount(19712); got != 0 {
		t.Errorf("nest amount = %d, want 0", got)
	}
	if got := p.Inventory().GetAmount(tables.BirdNests.EmptyNest); got != 1 {
		t.Errorf("empty nest amount = %d, want 1", got)
	}
	if got := p.Inventory().GetAmount(2677); got != 1 {
		t.Errorf("easy scroll amount = %d, want 1", got)
	}
	if len(session.messages) != 1 || session.messages[0] != "You take a clue scroll out of the bird's nest." {
		t.Errorf("messages = %v, want the take-out line", session.messages)
	}
	// An unknown item id falls through.
	if w.openClueNest(&plugins.PluginItemActionEvent{Player: p, ItemID: 995, Slot: 0, Option: "Open"}) {
		t.Error("openClueNest claimed an unknown item")
	}
	// A slot whose item is not the nest claims without changing
	// anything (the TS slot-identity return-true).
	p2, session2 := newVinePlayer("bob")
	p2.Inventory().Adds(treeByName(tables, "normal tree").LogID, 1)
	if !w.openClueNest(&plugins.PluginItemActionEvent{Player: p2, ItemID: 19712, Slot: 0, Option: "Open"}) {
		t.Error("openClueNest did not claim the mismatched slot")
	}
	if len(session2.messages) != 0 {
		t.Errorf("messages = %v, want none (slot mismatch is silent)", session2.messages)
	}
	if got := p2.Inventory().GetAmount(treeByName(tables, "normal tree").LogID); got != 1 {
		t.Errorf("log amount = %d, want 1 (nothing deleted)", got)
	}
}

// TestOpenClueNestFullInventory pins the free-slot gate: with no
// free slot the nest stays and the message fires.
func TestOpenClueNestFullInventory(t *testing.T) {
	tables := loadFixtureTables(t)
	w, _ := newTestWoodcutting(t, tables)
	p, session := newVinePlayer("alice")
	for i := 0; i < 28; i++ {
		p.Inventory().Adds(1000+i, 1)
	}
	if got := p.Inventory().GetFreeSlots(); got != 0 {
		t.Fatalf("free slots = %d, want 0", got)
	}
	// Replace slot 0 with the nest (still 0 free).
	p.Inventory().SetItem(0, game.NewItem(19712, 1))
	if !w.openClueNest(&plugins.PluginItemActionEvent{Player: p, ItemID: 19712, Slot: 0, Option: "Open"}) {
		t.Fatal("openClueNest did not claim the full-inventory nest")
	}
	if got := p.Inventory().GetAmount(19712); got != 1 {
		t.Errorf("nest amount = %d, want 1 (gate kept it)", got)
	}
	if len(session.messages) != 1 || session.messages[0] != "You need a free inventory space to open the bird's nest." {
		t.Errorf("messages = %v, want the free-space line", session.messages)
	}
}

// TestOpenClueNestOpenVesselHandled pins the open-vessel answer:
// a handler that takes over (Handled true) leaves the inventory
// untouched.
func TestOpenClueNestOpenVesselHandled(t *testing.T) {
	tables := loadFixtureTables(t)
	w, _ := newTestWoodcutting(t, tables)
	w.emit = func(name string, payload any) {
		if req, ok := payload.(*OpenVesselRequest); ok {
			req.Handled = true
		}
	}
	p, session := newVinePlayer("alice")
	p.Inventory().Adds(19712, 1)
	if !w.openClueNest(&plugins.PluginItemActionEvent{Player: p, ItemID: 19712, Slot: 0, Option: "Open"}) {
		t.Fatal("openClueNest did not claim the handled nest")
	}
	if got := p.Inventory().GetAmount(19712); got != 1 {
		t.Errorf("nest amount = %d, want 1 (handler took over)", got)
	}
	if len(session.messages) != 0 {
		t.Errorf("messages = %v, want none", session.messages)
	}
}

// TestRollClueNestsWiredIntoChopSuccess pins the wiring: a
// successful chop on a normal tree rolls clue nests (randFloat 0
// drops all five), after the woodcutting:success emit.
func TestRollClueNestsWiredIntoChopSuccess(t *testing.T) {
	tables := loadFixtureTables(t)
	w, emitted := newTestWoodcutting(t, tables)
	w.randFloat = func() float64 { return 0 }
	p, cap := newVinePlayer("alice")
	setLevel(p, game.SkillWoodcutting, 99)
	tree := treeByName(tables, "normal tree")
	state := &session{tree: tree, axe: *axeByItem(tables, "bronze_axe")}
	// A felling-free, ration-free chop: no axe complications.
	w.chopSuccess(p, state)
	nestMessages := 0
	for _, m := range cap.messages {
		if m == "<col=ff0000>A bird's nest falls out of the tree." {
			nestMessages++
		}
	}
	if nestMessages != 5 {
		t.Errorf("nest messages = %d, want 5 (roll wired into chopSuccess)", nestMessages)
	}
	// The success emit precedes the first clue-item request.
	successIdx, clueIdx := -1, -1
	for i, e := range *emitted {
		if e.name == successEventName && successIdx == -1 {
			successIdx = i
		}
		if e.name == treasureTrailsClueItemEventName && clueIdx == -1 {
			clueIdx = i
		}
	}
	if successIdx == -1 || clueIdx == -1 || successIdx > clueIdx {
		t.Errorf("success emit at %d, first clue-item at %d, want success first", successIdx, clueIdx)
	}
}
