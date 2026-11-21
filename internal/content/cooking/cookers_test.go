package cooking

// Cookers unit tests (Cookers.Cooking.js): the core loop, gates,
// timing, burns, karambwan, menus. Hand-built fixtures only —
// the shared helpers (helpers_test.go, Worker A) supply the
// shrimp/guppy tables, the fake session and the test cooking
// instance; the extra cookables here live in the 1000s so they
// never collide with A's 100–946 ids.

import (
	"fmt"
	"math"
	"testing"

	"tsps-go/internal/game"
	"tsps-go/internal/plugins"
	"tsps-go/internal/plugins/skillactions"
)

const (
	fxRawBeef           = 1001
	fxCookedBeef        = 1002
	fxBurntBeef         = 1003
	fxRawChicken        = 1011
	fxBurntChicken      = 1012
	fxRawKarambwan      = 1021
	fxCookedKarambwan   = 1022
	fxBurntKarambwan    = 1023
	fxPoisonKarambwan   = 1024
	fxRawKarambwanji    = 1031
	fxCookedKarambwanji = 1032
	fxBurntKarambwanji  = 1033
	fxRawGraahk         = 1041
	fxCookedGraahk      = 1042
	fxBurntGraahk       = 1043
	fxRawStopBurn       = 1051
	fxCookedStopBurn    = 1052
	fxBurntStopBurn     = 1053
	fxUncookedCake      = 1061
	fxCake              = 1062
	fxBurntCake         = 1063
	fxCakeTin           = 1064
	fxRawDragon         = 1071
	fxCookedDragon      = 1072
	fxBurntDragon       = 1073
	fxRawGnomebowl      = 1081
	fxCookedGnomebowl   = 1082
	fxBurntGnomebowl    = 1083
	fxCookingGauntlets  = 1091

	fxCookingFireSeq = 9001
	fxCookingSeq     = 9002

	fxCookingFireObj  = 2001
	fxCookingRangeObj = 2002
	fxStoveObj        = 2003

	// Varbit ids from Worker A's fixtureTables.
	fxSuccessCountVarbit = 7001
	fxBusyVarbit         = 7002
)

// newCookersTest builds a cooking instance over the shared
// fixture tables, extended with the extra cookables this file
// needs, a deterministic always-succeed roll, resolved cooker
// ids, and an object-info lookup over the fake cookers.
func newCookersTest() (*cooking, *[]emittedEvent, *game.TaskManager) {
	c, emitted, tm := newTestCooking()
	add := func(cb *Cookable) { c.tables.CookablesByRaw[cb.RawID] = cb }
	add(&Cookable{RawID: fxRawBeef, CookedID: fxCookedBeef, BurntID: fxBurntBeef, Level: 1, XP: 30,
		Chance: map[string][2]int{"fire": {128, 512}, "range": {128, 512}}, Name: "beef"})
	add(&Cookable{RawID: fxRawChicken, CookedID: fxCookedChicken, BurntID: fxBurntChicken, Level: 1, XP: 30,
		Chance: map[string][2]int{"fire": {128, 512}, "range": {128, 512}}, Name: "chicken"})
	add(&Cookable{RawID: fxRawKarambwan, CookedID: fxCookedKarambwan, BurntID: fxBurntKarambwan, Level: 30, XP: 190,
		Chance: map[string][2]int{"fire": {128, 512}, "range": {128, 512}}, Name: "karambwan"})
	add(&Cookable{RawID: fxRawKarambwanji, CookedID: fxCookedKarambwanji, BurntID: fxBurntKarambwanji, Level: 1, XP: 10,
		Chance: map[string][2]int{"fire": {128, 512}, "range": {128, 512}}, Name: "karambwanji",
		DuringQuest: "tai_bwo_wannai_trio"})
	add(&Cookable{RawID: fxRawGraahk, CookedID: fxCookedGraahk, BurntID: fxBurntGraahk, Level: 15, XP: 0,
		Chance: map[string][2]int{"fire": {128, 512}, "range": {128, 512}}, Name: "graahk",
		Rumours: 25, HasRumours: true})
	add(&Cookable{RawID: fxRawStopBurn, CookedID: fxCookedStopBurn, BurntID: fxBurntStopBurn, Level: 1, XP: 30,
		StopBurn: 42, HasStopBurn: true, Name: "stop-burn fish"})
	add(&Cookable{RawID: fxUncookedCake, CookedID: fxCake, BurntID: fxBurntCake, Level: 40, XP: 180,
		Chance: map[string][2]int{"range": {128, 512}}, Name: "cake",
		ReturnItemID: fxCakeTin, HasReturnItem: true, ReturnOnBurn: true})
	add(&Cookable{RawID: fxRawDragon, CookedID: fxCookedDragon, BurntID: fxBurntDragon, Level: 1, XP: 25,
		Chance: map[string][2]int{"fire": {128, 512}, "range": {128, 512}}, Name: "drunk dragon",
		FailsWithoutBurning: true})
	add(&Cookable{RawID: fxRawGnomebowl, CookedID: fxCookedGnomebowl, BurntID: fxBurntGnomebowl, Level: 1, XP: 0,
		Chance: map[string][2]int{"range": {128, 512}}, Name: "gnomebowl",
		FirstCookTicks: 4, HasFirstCookTicks: true, GnomeTutorial: true})
	c.tables.ItemIDs["tbwt_raw_karambwan"] = fxRawKarambwan
	c.tables.ItemIDs["cooked_meat"] = fxCookedMeat
	c.tables.ItemIDs["burnt_meat"] = fxBurntBeef
	c.tables.ItemIDs["cooked_chicken"] = fxCookedChicken
	c.tables.ItemIDs["burnt_chicken"] = fxBurntChicken
	// A deterministic always-succeed roll (isSuccess: roll >=
	// 1 - chance; 1.0 succeeds whenever the chance is under 1).
	c.randFloat = func() float64 { return 1.0 }
	c.cookerIDs = fixtureCookerIDs()
	c.objectInfo = fixtureObjectInfo()
	return c, emitted, tm
}

func fixtureCookerIDs() CookerIDs {
	return CookerIDs{
		PoisonKarambwanID:  fxPoisonKarambwan,
		CookingGauntletsID: fxCookingGauntlets,
		FireCookingSeqID:   fxCookingFireSeq,
		CookingSeqID:       fxCookingSeq,
	}
}

// fixtureObjectInfo names the fake cookers: a fire, a range,
// and a stove (a range name without the word "range").
func fixtureObjectInfo() func(id int) (ObjectInfo, bool) {
	return func(id int) (ObjectInfo, bool) {
		switch id {
		case fxCookingFireObj:
			return ObjectInfo{Name: "Fire", Interactions: []string{"Cook"}, SizeX: 1, SizeY: 1}, true
		case fxCookingRangeObj:
			return ObjectInfo{Name: "Cooking range", Interactions: []string{"Cook"}, SizeX: 1, SizeY: 1}, true
		case fxStoveObj:
			return ObjectInfo{Name: "Stove", Interactions: []string{"Cook"}, SizeX: 1, SizeY: 1}, true
		}
		return ObjectInfo{}, false
	}
}

// cookTestPlayer builds a registered player with a capturing
// session, at the shared test tile.
func cookTestPlayer(name string) (*game.Player, *fakeSession) {
	p := newTestPlayer(name)
	session := &fakeSession{}
	p.Session = session
	return p, session
}

// cookGameObject builds a scenery object at loc.
func cookGameObject(id int, loc game.Location) *game.GameObject {
	return game.NewGameObject(id, loc, 10, 0, nil)
}

// stubObjectManager is a minimal ObjectManagerAPI over a map,
// for currentCooker tests.
type stubObjectManager struct {
	byKey map[string]plugins.GameObject
}

func (m *stubObjectManager) key(id int, loc game.Location) string {
	return fmt.Sprintf("%d,%d,%d/%d", loc.X, loc.Y, loc.Z, id)
}

func (m *stubObjectManager) Register(obj plugins.GameObject, _ bool) {
	if m.byKey == nil {
		m.byKey = make(map[string]plugins.GameObject)
	}
	m.byKey[m.key(obj.ID(), obj.Location())] = obj
}
func (m *stubObjectManager) Deregister(obj plugins.GameObject, _ bool) {
	delete(m.byKey, m.key(obj.ID(), obj.Location()))
}
func (m *stubObjectManager) ReplaceInPlace(outgoing, incoming plugins.GameObject) {
	m.Deregister(outgoing, false)
	m.Register(incoming, false)
}
func (m *stubObjectManager) ObjectsAt(loc game.Location) []plugins.GameObject {
	var out []plugins.GameObject
	for _, obj := range m.byKey {
		if obj.Location().Equals(loc) {
			out = append(out, obj)
		}
	}
	return out
}
func (m *stubObjectManager) ObjectsInRegion(_, _ int) []plugins.GameObject { return nil }
func (m *stubObjectManager) Exists(_ int, loc game.Location) bool {
	return len(m.ObjectsAt(loc)) > 0
}
func (m *stubObjectManager) Get(id int, loc game.Location) plugins.GameObject {
	return m.byKey[m.key(id, loc)]
}
func (m *stubObjectManager) Find(_ *game.Player, id int, loc game.Location) plugins.GameObject {
	return m.Get(id, loc)
}
func (m *stubObjectManager) StillStands(_ *game.Player, id int, loc game.Location, _ plugins.GameObject) bool {
	return m.Get(id, loc) != nil
}
func (m *stubObjectManager) RemovePluginObjects() int { return 0 }
func (m *stubObjectManager) RemovedObjects() []plugins.GameObject {
	return nil
}

// cookTestDef is a hand-built ObjectDefinition for IsCooker
// tests.
type cookTestDef struct {
	id           int
	name         string
	interactions []string
}

func (d cookTestDef) ID() int                { return d.id }
func (d cookTestDef) Name() string           { return d.name }
func (d cookTestDef) SizeX() int             { return 1 }
func (d cookTestDef) SizeY() int             { return 1 }
func (d cookTestDef) BlockingMask() int      { return 0 }
func (d cookTestDef) Interactions() []string { return d.interactions }

// notedDefs wraps the shared fake item defs with a noted set,
// for the noted-item tests (the shared defs never note).
type notedDefs struct {
	fakeItemDefs
	noted map[int]bool
}

func (d notedDefs) IsNoted(id int) bool { return d.noted[id] }

func hasMessage(session *fakeSession, want string) bool {
	for _, m := range session.messages {
		if m == want {
			return true
		}
	}
	return false
}

func hasSpam(session *fakeSession, want string) bool {
	for _, m := range session.spam {
		if m == want {
			return true
		}
	}
	return false
}

func countEmitted(emitted *[]emittedEvent, name string) int {
	n := 0
	for _, e := range *emitted {
		if e.name == name {
			n++
		}
	}
	return n
}

// ---- gates (cookableForUse / canCook) ----

func TestCookableForUseDeliberateBurn(t *testing.T) {
	c, _, _ := newCookersTest()
	// Cooked meat cooks into burnt meat, no XP (the deliberate
	// burn); cooked chicken likewise.
	for _, tc := range []struct {
		in, burnt int
	}{
		{fxCookedMeat, fxBurntBeef},
		{fxCookedChicken, fxBurntChicken},
	} {
		cookable, deliberate := c.cookableForUse(tc.in)
		if cookable == nil || !deliberate {
			t.Fatalf("cookableForUse(%d) = %v/%v, want a deliberate burn", tc.in, cookable, deliberate)
		}
		if cookable.CookedID != tc.burnt || cookable.XP != 0 {
			t.Errorf("deliberate burn for %d: cooked=%d xp=%v, want %d/0", tc.in, cookable.CookedID, cookable.XP, tc.burnt)
		}
	}
	// A facility food never answers (the Camdozaal guppy cooks
	// at the preparation table, not on a fire or range).
	if cookable, _ := c.cookableForUse(fxRawGuppy); cookable != nil {
		t.Errorf("cookableForUse(guppy) = %+v, want nil (facility)", cookable)
	}
	// An unknown item never answers.
	if cookable, _ := c.cookableForUse(9999); cookable != nil {
		t.Errorf("cookableForUse(9999) = %+v, want nil", cookable)
	}
}

func TestCookableForUseMembersGate(t *testing.T) {
	c, _, _ := newCookersTest()
	c.membersWorld = false
	// The shrimp is non-members (no isMembersItem installed, so
	// every item counts as non-members): F2P cooks it.
	if cookable, _ := c.cookableForUse(fxRawShrimp); cookable == nil {
		t.Error("cookableForUse(shrimp) on F2P = nil, want the row")
	}
	// A members item is refused on F2P.
	c.isMembersItem = func(id int) bool { return id == fxRawBeef }
	if cookable, _ := c.cookableForUse(fxRawBeef); cookable != nil {
		t.Errorf("cookableForUse(members beef) on F2P = %+v, want nil", cookable)
	}
	// ...and allowed on a members world.
	c.membersWorld = true
	if cookable, _ := c.cookableForUse(fxRawBeef); cookable == nil {
		t.Error("cookableForUse(members beef) on members world = nil, want the row")
	}
}

func TestCanCookGateOrder(t *testing.T) {
	c, _, _ := newCookersTest()
	shrimp := c.tables.CookablesByRaw[fxRawShrimp]
	p, session := cookTestPlayer("alice")

	// Level gate: level 1 cannot cook the level-40 cake; the
	// refusal is a mesbox (unobservable through the fake
	// session — only the false is asserted here).
	setCooking(p, 1)
	cake := c.tables.CookablesByRaw[fxUncookedCake]
	if c.canCook(p, fxCookingRangeObj, "Cooking range", cake) {
		t.Error("canCook(cake at level 1) = true, want false")
	}

	// Lumbridge Castle range (object id 114) refuses
	// fail-closed: no quest:is-complete answerer exists.
	setCooking(p, 99)
	if c.canCook(p, cookingRangeObjectID, "Cooking range", shrimp) {
		t.Error("canCook(Lumbridge range) = true, want false (fail-closed)")
	}
	if !hasMessage(session, lumbridgeRangeMessage) {
		t.Errorf("messages = %v, want the Lumbridge range line", session.messages)
	}

	// Gnome-tutorial food on a fire refuses (mesbox).
	bowl := c.tables.CookablesByRaw[fxRawGnomebowl]
	if c.canCook(p, fxCookingFireObj, "Fire", bowl) {
		t.Error("canCook(gnomebowl on fire) = true, want false")
	}
	// ...but on a range it passes the fire gate.
	if !c.canCook(p, fxCookingRangeObj, "Cooking range", bowl) {
		t.Error("canCook(gnomebowl on range) = false, want true")
	}

	// During-quest food (karambwanji) refuses outside the
	// quest: no quest:is-started answerer, so fail-closed.
	ji := c.tables.CookablesByRaw[fxRawKarambwanji]
	if c.canCook(p, fxCookingRangeObj, "Cooking range", ji) {
		t.Error("canCook(karambwanji) = true, want false (fail-closed)")
	}
	if !hasMessage(session, notUsefulMessage) {
		t.Errorf("messages = %v, want the not-useful line", session.messages)
	}

	// Rumours food (graahk, 25) refuses: no
	// hunter:rumours-completed answerer, so 0 completed.
	graahk := c.tables.CookablesByRaw[fxRawGraahk]
	if c.canCook(p, fxCookingRangeObj, "Cooking range", graahk) {
		t.Error("canCook(graahk) = true, want false (fail-closed)")
	}
}

func TestCanCookFoodWithoutMessages(t *testing.T) {
	c, _, _ := newCookersTest()
	shrimp := c.tables.CookablesByRaw[fxRawShrimp]
	p, session := cookTestPlayer("alice")
	setCooking(p, 1)
	if !c.canCookFood(p, shrimp) {
		t.Error("canCookFood(shrimp at level 1) = false, want true")
	}
	if len(session.messages) != 0 || len(session.spam) != 0 {
		t.Errorf("canCookFood sent messages %v / spam %v, want none", session.messages, session.spam)
	}
	// The level gate still applies, silently.
	cake := c.tables.CookablesByRaw[fxUncookedCake]
	if c.canCookFood(p, cake) {
		t.Error("canCookFood(cake at level 1) = true, want false")
	}
	if len(session.messages) != 0 || len(session.spam) != 0 {
		t.Errorf("canCookFood(cake) sent messages %v / spam %v, want none", session.messages, session.spam)
	}
}

// ---- chances (chanceValues / stopBurnLevel / successChanceFor) ----

func TestChanceValuesGauntletsAndHosidius(t *testing.T) {
	c, _, _ := newCookersTest()
	p, _ := cookTestPlayer("alice")
	setCooking(p, 50)
	shrimp := c.tables.CookablesByRaw[fxRawShrimp]
	loc := game.Location{X: 3200, Y: 3200, Z: 0}

	// Plain range: the row's range chart.
	got := c.chanceValues(p, shrimp, fxCookingRangeObj, "range", loc)
	if got == nil || *got != [2]int{128, 512} {
		t.Fatalf("chanceValues(range) = %v, want [128 512]", got)
	}
	// Cooking gauntlets (hands slot) switch to the gauntlets
	// chart when the row has one.
	shrimp.Chance["gauntlets"] = [2]int{200, 600}
	p.Equipment().Set(game.HandsSlot, game.NewItem(fxCookingGauntlets, 1))
	got = c.chanceValues(p, shrimp, fxCookingRangeObj, "range", loc)
	if got == nil || *got != [2]int{200, 600} {
		t.Fatalf("chanceValues(gauntlets) = %v, want [200 600]", got)
	}
	// Without the gauntlets the range chart applies again.
	p2, _ := cookTestPlayer("bob")
	setCooking(p2, 50)
	got = c.chanceValues(p2, shrimp, fxCookingRangeObj, "range", loc)
	if got == nil || *got != [2]int{128, 512} {
		t.Fatalf("chanceValues(range, no gauntlets) = %v, want [128 512]", got)
	}

	// Hosidius Kitchen (1674–1684 x 3616–3626): +12 on the
	// kitchen's clay oven (CLAY_OVEN_3, 21302 — the bonus is
	// tied to that object id, not to any range).
	kitchen := game.Location{X: 1679, Y: 3621, Z: 0}
	got = c.chanceValues(p2, shrimp, clayOven3ObjectID, "range", kitchen)
	if got == nil || *got != [2]int{128 + 12, 512 + 12} {
		t.Fatalf("chanceValues(Hosidius range) = %v, want [140 524]", got)
	}
	// The bonus never applies on a fire, and never outside the
	// rectangle.
	got = c.chanceValues(p2, shrimp, fxCookingFireObj, "fire", kitchen)
	if got == nil || *got != [2]int{128, 512} {
		t.Fatalf("chanceValues(Hosidius fire) = %v, want [128 512]", got)
	}
	got = c.chanceValues(p2, shrimp, clayOven3ObjectID, "range", game.Location{X: 3200, Y: 3200, Z: 0})
	if got == nil || *got != [2]int{128, 512} {
		t.Fatalf("chanceValues(outside Hosidius) = %v, want [128 512]", got)
	}
	// The elite +25 never applies: no diary:is-complete
	// answerer exists, so the bonus stays 12.
	if bonus := c.hosidiusBonus(p2, clayOven3ObjectID, kitchen); bonus != 12 {
		t.Errorf("hosidiusBonus(Hosidius) = %d, want 12 (never 25)", bonus)
	}
}

func TestStopBurnLevelAndFallbackChance(t *testing.T) {
	c, _, _ := newCookersTest()
	p, _ := cookTestPlayer("alice")
	loc := game.Location{X: 3200, Y: 3200, Z: 0}
	shrimp := c.tables.CookablesByRaw[fxRawShrimp]

	// Shrimp interp [128 512]: the chance reaches 1 first at
	// level 34 (successChance floors to 258/256 there).
	if got := c.stopBurnLevel(p, shrimp, fxCookingRangeObj, "range", loc); got != 34 {
		t.Errorf("stopBurnLevel(shrimp) = %d, want 34", got)
	}
	// The stopBurn-only fish (stopBurn 42, no chart): the
	// fallback line reaches 1 at its stopBurn.
	stop := c.tables.CookablesByRaw[fxRawStopBurn]
	if got := c.stopBurnLevel(p, stop, fxCookingRangeObj, "range", loc); got != 42 {
		t.Errorf("stopBurnLevel(stopBurn fish) = %d, want 42", got)
	}
	// Fallback line: 58% at the food's level, 100% at
	// stopBurn (Wiki: the 42% burn chance falls linearly).
	// The 0.58 reads back as 0.5800000000000001 in float64,
	// so compare with a tolerance.
	setCooking(p, 1)
	if got := c.successChanceFor(p, stop, fxCookingRangeObj, "range", loc); math.Abs(got-0.58) > 1e-9 {
		t.Errorf("successChance(stopBurn fish, level 1) = %v, want 0.58", got)
	}
	setCooking(p, 42)
	if got := c.successChanceFor(p, stop, fxCookingRangeObj, "range", loc); got != 1 {
		t.Errorf("successChance(stopBurn fish, level 42) = %v, want 1", got)
	}
	// Interp midpoint: shrimp at level 65 — the formula reads
	// successChance(128, 512, 65) directly (it saturates to 1
	// well before 65: stopBurn is 34).
	setCooking(p, 65)
	want := successChance(128, 512, 65)
	if got := c.successChanceFor(p, shrimp, fxCookingRangeObj, "range", loc); got != want {
		t.Errorf("successChance(shrimp, level 65) = %v, want %v", got, want)
	}
}

// ---- IsCooker ----

func TestIsCooker(t *testing.T) {
	// TS isCooker (Cookers.Cooking.js:610-615): the name must
	// be cookable, and either no interaction is set, Cook is
	// set, or the name is a fire. A Forester's Campfire has
	// no Cook action — food is used on it directly.
	cases := []struct {
		def  cookTestDef
		want bool
	}{
		{cookTestDef{id: 1, name: "Fire", interactions: []string{""}}, true},
		{cookTestDef{id: 2, name: "Cooking range", interactions: []string{"Cook"}}, true},
		{cookTestDef{id: 3, name: "Stove", interactions: []string{"Cook"}}, true},
		{cookTestDef{id: 4, name: "Cooking range", interactions: []string{""}}, true},
		{cookTestDef{id: 5, name: "Oak tree", interactions: []string{"Chop down"}}, false},
		{cookTestDef{id: 6, name: "Cooking range", interactions: []string{"Search"}}, false},
		{cookTestDef{id: 7, name: "Forester's Campfire", interactions: []string{""}}, true},
	}
	for _, tc := range cases {
		if got := IsCooker(tc.def); got != tc.want {
			t.Errorf("IsCooker(%q %v) = %v, want %v", tc.def.name, tc.def.interactions, got, tc.want)
		}
	}
	if IsCooker(nil) {
		t.Error("IsCooker(nil) = true, want false")
	}
}

// ---- the tick loop (startCooking / cookTick / cookStep) ----

// startRangeCook starts a shrimp session on the fake range and
// returns the player, session capture and object manager.
func startRangeCook(t *testing.T, c *cooking, tm *game.TaskManager, rawID, amount int, level int) (*game.Player, *fakeSession, *stubObjectManager) {
	t.Helper()
	p, session := cookTestPlayer("alice")
	setCooking(p, level)
	give(p, rawID, amount)
	obj := cookGameObject(fxCookingRangeObj, p.Location())
	objects := &stubObjectManager{}
	objects.Register(obj, false)
	c.objects = objects
	cookable := c.tables.CookablesByRaw[rawID]
	if !c.startCooking(p, obj, cookable, amount, nil, nil, false) {
		t.Fatal("startCooking = false, want true")
	}
	return p, session, objects
}

func TestCookLoopTiming(t *testing.T) {
	c, emitted, tm := newCookersTest()
	p, session, _ := startRangeCook(t, c, tm, fxRawShrimp, 5, 1)

	// First cook at +2 ticks, second at +3, then every 4.
	if session.varbits[fxBusyVarbit] != 1 {
		t.Errorf("busy varbit after start = %d, want 1", session.varbits[fxBusyVarbit])
	}
	driveTicks(tm, 1) // cycle 1: wait
	if got := countOf(p, fxShrimp); got != 0 {
		t.Errorf("cooked after 1 tick = %d, want 0", got)
	}
	driveTicks(tm, 1) // cycle 2: first cook
	if got := countOf(p, fxShrimp); got != 1 {
		t.Errorf("cooked after 2 ticks = %d, want 1", got)
	}
	if got := p.Skills().Experience(game.SkillCooking); got != 30 {
		t.Errorf("XP after first cook = %d, want 30", got)
	}
	if session.varbits[fxBusyVarbit] != 0 {
		t.Errorf("busy varbit after first cook = %d, want 0", session.varbits[fxBusyVarbit])
	}
	driveTicks(tm, 2) // cycles 3-4: wait
	if got := countOf(p, fxShrimp); got != 1 {
		t.Errorf("cooked after 4 ticks = %d, want 1", got)
	}
	driveTicks(tm, 1) // cycle 5: second cook
	if got := countOf(p, fxShrimp); got != 2 {
		t.Errorf("cooked after 5 ticks = %d, want 2", got)
	}
	driveTicks(tm, 8) // cycles 6-13: cooks at 9 and 13
	if got := countOf(p, fxShrimp); got != 4 {
		t.Errorf("cooked after 13 ticks = %d, want 4", got)
	}
	driveTicks(tm, 4) // cycle 17: fifth cook
	if got := countOf(p, fxShrimp); got != 5 {
		t.Errorf("cooked after 17 ticks = %d, want 5", got)
	}
	if got := countOf(p, fxRawShrimp); got != 0 {
		t.Errorf("raw left = %d, want 0", got)
	}
	if got := p.Skills().Experience(game.SkillCooking); got != 150 {
		t.Errorf("XP after 5 cooks = %d, want 150", got)
	}
	// The next tick stops the session (nothing left).
	driveTicks(tm, 1)
	if c.isCookingActive(p) {
		t.Error("still cooking after the last food, want stopped")
	}
	if got := session.varbits[fxSuccessCountVarbit]; got != 5 {
		t.Errorf("success-count varbit = %d, want 5", got)
	}
	// Five successes, five produced events, five success
	// messages.
	if got := countEmitted(emitted, SkillsProducedEventName); got != 5 {
		t.Errorf("produced emits = %d, want 5", got)
	}
	if got := countEmitted(emitted, CookingSuccessEventName); got != 5 {
		t.Errorf("success emits = %d, want 5", got)
	}
	if !hasSpam(session, "You successfully cook some shrimps.") {
		t.Errorf("spam = %v, want the shrimp success line", session.spam)
	}
}

func TestCookLoopStops(t *testing.T) {
	// Movement stops the session (animation reset).
	c, _, tm := newCookersTest()
	p, _, _ := startRangeCook(t, c, tm, fxRawShrimp, 5, 1)
	driveTicks(tm, 2) // first cook lands
	p.MovementQueue().AddCheckpoint(game.Location{X: 3201, Y: 3200, Z: 0})
	driveTicks(tm, 1)
	if c.isCookingActive(p) {
		t.Error("still cooking after moving, want stopped")
	}
	if got := countOf(p, fxShrimp); got != 1 {
		t.Errorf("cooked after movement stop = %d, want 1", got)
	}

	// Death stops the session before the first cook (busy
	// cleared).
	c, _, tm = newCookersTest()
	p, session, _ := startRangeCook(t, c, tm, fxRawShrimp, 5, 1)
	setTestLevel(p, game.SkillHitpoints, 0)
	driveTicks(tm, 1)
	if c.isCookingActive(p) {
		t.Error("still cooking at 0 HP, want stopped")
	}
	if session.varbits[fxBusyVarbit] != 0 {
		t.Errorf("busy varbit after death stop = %d, want 0", session.varbits[fxBusyVarbit])
	}
	if got := countOf(p, fxShrimp); got != 0 {
		t.Errorf("cooked after death stop = %d, want 0", got)
	}

	// The cooker being replaced stops the session.
	c, _, tm = newCookersTest()
	p, _, objects := startRangeCook(t, c, tm, fxRawShrimp, 5, 1)
	driveTicks(tm, 2) // first cook lands
	objects.Deregister(cookGameObject(fxCookingRangeObj, p.Location()), false)
	objects.Register(cookGameObject(fxStoveObj, p.Location()), false)
	driveTicks(tm, 3) // cycle 5: cooker gone
	if c.isCookingActive(p) {
		t.Error("still cooking after the cooker was replaced, want stopped")
	}
	if got := countOf(p, fxShrimp); got != 1 {
		t.Errorf("cooked after replacement stop = %d, want 1", got)
	}

	// Running out of raw stops the session.
	c, _, tm = newCookersTest()
	p, _, _ = startRangeCook(t, c, tm, fxRawShrimp, 2, 1)
	driveTicks(tm, 6) // cooks at 2 and 5, then out of raw
	if c.isCookingActive(p) {
		t.Error("still cooking with no raw left, want stopped")
	}
	if got := countOf(p, fxShrimp); got != 2 {
		t.Errorf("cooked after raw runs out = %d, want 2", got)
	}
}

func TestFirstCookTicksOverride(t *testing.T) {
	c, _, tm := newCookersTest()
	p, _, _ := startRangeCook(t, c, tm, fxRawGnomebowl, 2, 1)
	// The gnomebowl row's firstCookTicks is 4, not 2.
	driveTicks(tm, 3)
	if got := countOf(p, fxCookedGnomebowl); got != 0 {
		t.Errorf("cooked after 3 ticks = %d, want 0 (firstCookTicks 4)", got)
	}
	driveTicks(tm, 1) // cycle 4: first cook
	if got := countOf(p, fxCookedGnomebowl); got != 1 {
		t.Errorf("cooked after 4 ticks = %d, want 1", got)
	}
}

// ---- cookOne paths ----

func TestCookOneBurn(t *testing.T) {
	c, emitted, tm := newCookersTest()
	// A roll of 0 burns anything that can burn.
	c.randFloat = func() float64 { return 0 }
	p, session, _ := startRangeCook(t, c, tm, fxRawShrimp, 2, 1)
	driveTicks(tm, 2) // first cook: burn
	if got := countOf(p, fxBurntShrimp); got != 1 {
		t.Errorf("burnt after burn = %d, want 1", got)
	}
	if got := countOf(p, fxShrimp); got != 0 {
		t.Errorf("cooked after burn = %d, want 0", got)
	}
	if got := p.Skills().Experience(game.SkillCooking); got != 0 {
		t.Errorf("XP after burn = %d, want 0", got)
	}
	if !hasSpam(session, "You accidentally burn the shrimps.") {
		t.Errorf("spam = %v, want the burn line", session.spam)
	}
	if got := countEmitted(emitted, CookingBurntEventName); got != 1 {
		t.Errorf("burnt emits = %d, want 1", got)
	}
	// A burn resets the success count to 0.
	if got := session.varbits[fxSuccessCountVarbit]; got != 0 {
		t.Errorf("success-count varbit after burn = %d, want 0", got)
	}
}

func TestKarambwanUntrainedAndTrained(t *testing.T) {
	// Untrained: level gate 1, cooks into poison karambwan,
	// xp 80, with the dodgy line.
	c, _, tm := newCookersTest()
	p, session, _ := startRangeCook(t, c, tm, fxRawKarambwan, 1, 1)
	driveTicks(tm, 2)
	if got := countOf(p, fxPoisonKarambwan); got != 1 {
		t.Errorf("poison karambwan = %d, want 1", got)
	}
	if got := countOf(p, fxCookedKarambwan); got != 0 {
		t.Errorf("cooked karambwan (untrained) = %d, want 0", got)
	}
	if got := p.Skills().Experience(game.SkillCooking); got != 80 {
		t.Errorf("XP untrained = %d, want 80", got)
	}
	if !hasSpam(session, "You cook the karambwan. It looks very dodgy.") {
		t.Errorf("spam = %v, want the dodgy line", session.spam)
	}

	// Trained (attribute 8): the level-30 gate applies at
	// start — a trained level-1 player cannot start.
	c, _, tm = newCookersTest()
	p, _ = cookTestPlayer("alice")
	setCooking(p, 1)
	p.SetAttribute(karambwanTaughtAttribute, 8)
	give(p, fxRawKarambwan, 1)
	obj := cookGameObject(fxCookingRangeObj, p.Location())
	objects := &stubObjectManager{}
	objects.Register(obj, false)
	c.objects = objects
	if c.startCooking(p, obj, c.tables.CookablesByRaw[fxRawKarambwan], 1, nil, nil, false) {
		t.Error("startCooking(trained karambwan at level 1) = true, want false")
	}
	// At level 30 it cooks the real fish (xp 190).
	c, _, tm = newCookersTest()
	p, _, _ = startRangeCook(t, c, tm, fxRawKarambwan, 1, 30)
	p.SetAttribute(karambwanTaughtAttribute, 8)
	driveTicks(tm, 2)
	if got := countOf(p, fxCookedKarambwan); got != 1 {
		t.Errorf("cooked karambwan at level 30 (trained) = %d, want 1", got)
	}
	if got := p.Skills().Experience(game.SkillCooking); got != 190 {
		t.Errorf("XP trained = %d, want 190", got)
	}
	if got := countOf(p, fxPoisonKarambwan); got != 0 {
		t.Errorf("poison karambwan (trained) = %d, want 0", got)
	}
}

func TestKarambwanReclickTiming(t *testing.T) {
	c, _, tm := newCookersTest()
	p, _, _ := startRangeCook(t, c, tm, fxRawKarambwan, 3, 1)
	// Untrained, so the first cook lands at +2.
	driveTicks(tm, 2)
	if got := countOf(p, fxPoisonKarambwan); got != 1 {
		t.Fatalf("poison after first cook = %d, want 1", got)
	}
	// Re-clicking after a cook restarts with +1 tick, not +2.
	cookable := c.tables.CookablesByRaw[fxRawKarambwan]
	obj := cookGameObject(fxCookingRangeObj, p.Location())
	if !c.startCooking(p, obj, cookable, 2, nil, nil, false) {
		t.Fatal("re-click startCooking = false, want true")
	}
	if got := c.sessions[p].nextCookTick; got != c.cookCycle+int64(karambwanReclickTicks) {
		t.Errorf("nextCookTick after re-click = %d, want cookCycle+1 (%d)", got, c.cookCycle+1)
	}
	driveTicks(tm, 1) // +1: second cook
	if got := countOf(p, fxPoisonKarambwan); got != 2 {
		t.Errorf("poison after re-click +1 = %d, want 2", got)
	}

	// Re-clicking before anything cooked keeps the previous
	// nextCookTick.
	c, _, tm = newCookersTest()
	p, _, _ = startRangeCook(t, c, tm, fxRawKarambwan, 3, 1)
	driveTicks(tm, 1) // cycle 1: nothing cooked yet
	cookable = c.tables.CookablesByRaw[fxRawKarambwan]
	obj = cookGameObject(fxCookingRangeObj, p.Location())
	previous := c.sessions[p].nextCookTick
	if !c.startCooking(p, obj, cookable, 3, nil, nil, false) {
		t.Fatal("early re-click startCooking = false, want true")
	}
	if got := c.sessions[p].nextCookTick; got != previous {
		t.Errorf("nextCookTick after early re-click = %d, want previous %d", got, previous)
	}
}

func TestDeliberateBurn(t *testing.T) {
	c, _, tm := newCookersTest()
	p, session := cookTestPlayer("alice")
	setCooking(p, 1)
	give(p, fxCookedMeat, 1)
	objects := &stubObjectManager{}
	obj := cookGameObject(fxCookingRangeObj, p.Location())
	objects.Register(obj, false)
	c.objects = objects
	// Using cooked meat on the range deliberately burns it:
	// no XP, the burnt meat, the deliberate line.
	c.handleCook(&plugins.ItemOnObjectEvent{
		Player: p, Object: obj, ObjectID: fxCookingRangeObj,
		ItemID: fxCookedMeat, Item: game.NewItem(fxCookedMeat, 1), ItemSlot: 0,
	})
	if !c.isCookingActive(p) {
		t.Fatal("no session after using cooked meat on the range")
	}
	driveTicks(tm, 2)
	if got := countOf(p, fxBurntBeef); got != 1 {
		t.Errorf("burnt meat = %d, want 1", got)
	}
	if got := countOf(p, fxCookedMeat); got != 0 {
		t.Errorf("cooked meat left = %d, want 0", got)
	}
	if got := p.Skills().Experience(game.SkillCooking); got != 0 {
		t.Errorf("XP after deliberate burn = %d, want 0", got)
	}
	if !hasSpam(session, "You deliberately burn the nicely cooked meat.") {
		t.Errorf("spam = %v, want the deliberate-burn line", session.spam)
	}
}

func TestFailsWithoutBurning(t *testing.T) {
	c, emitted, tm := newCookersTest()
	c.randFloat = func() float64 { return 0 } // burns
	p, session, _ := startRangeCook(t, c, tm, fxRawDragon, 1, 1)
	driveTicks(tm, 2)
	// The drunk dragon still arrives (its burnt id), with no
	// XP, no burn line, no burnt event — but the count moves.
	if got := countOf(p, fxBurntDragon); got != 1 {
		t.Errorf("burnt dragon = %d, want 1", got)
	}
	if got := p.Skills().Experience(game.SkillCooking); got != 0 {
		t.Errorf("XP after failed dragon = %d, want 0", got)
	}
	if hasSpam(session, "You accidentally burn the drunk dragon.") {
		t.Errorf("spam = %v, want no burn line for the dragon", session.spam)
	}
	if got := countEmitted(emitted, CookingBurntEventName); got != 0 {
		t.Errorf("burnt emits = %d, want 0", got)
	}
	if got := session.varbits[fxSuccessCountVarbit]; got != 1 {
		t.Errorf("success-count varbit = %d, want 1", got)
	}
}

func TestReturnItemCake(t *testing.T) {
	// Success returns the cake tin as well as the cake.
	c, _, tm := newCookersTest()
	p, _, _ := startRangeCook(t, c, tm, fxUncookedCake, 1, 40)
	driveTicks(tm, 2)
	if got := countOf(p, fxCake); got != 1 {
		t.Errorf("cake = %d, want 1", got)
	}
	if got := countOf(p, fxCakeTin); got != 1 {
		t.Errorf("cake tin = %d, want 1", got)
	}
	if got := p.Skills().Experience(game.SkillCooking); got != 180 {
		t.Errorf("XP = %d, want 180", got)
	}

	// A burn still returns the tin (returnOnBurn). The cake's
	// real range chart [128 512] saturates to chance 1 at
	// level 34 — below the level-40 gate, so a level-40 cook
	// can never burn with it; the chart is overridden here
	// (test-local row) to exercise the burn branch.
	c, _, tm = newCookersTest()
	c.randFloat = func() float64 { return 0 }
	c.tables.CookablesByRaw[fxUncookedCake].Chance["range"] = [2]int{0, 100}
	p, _, _ = startRangeCook(t, c, tm, fxUncookedCake, 1, 40)
	driveTicks(tm, 2)
	if got := countOf(p, fxBurntCake); got != 1 {
		t.Errorf("burnt cake = %d, want 1", got)
	}
	if got := countOf(p, fxCakeTin); got != 1 {
		t.Errorf("cake tin after burn = %d, want 1 (returnOnBurn)", got)
	}

	// No free slot: the cook refuses before anything changes.
	c, _, tm = newCookersTest()
	p, _ = cookTestPlayer("alice")
	setCooking(p, 40)
	give(p, fxUncookedCake, 1)
	for i := 0; i < 27; i++ {
		give(p, 6000+i, 1)
	}
	obj := cookGameObject(fxCookingRangeObj, p.Location())
	objects := &stubObjectManager{}
	objects.Register(obj, false)
	c.objects = objects
	cake := c.tables.CookablesByRaw[fxUncookedCake]
	if !c.startCooking(p, obj, cake, 1, nil, nil, false) {
		t.Fatal("startCooking(full inventory) = false, want true (the room check is in cookOne)")
	}
	driveTicks(tm, 2)
	if got := countOf(p, fxUncookedCake); got != 1 {
		t.Errorf("uncooked cake after refused cook = %d, want 1 (kept)", got)
	}
	if got := countOf(p, fxCake); got != 0 {
		t.Errorf("cake after refused cook = %d, want 0", got)
	}
	if c.isCookingActive(p) {
		t.Error("still cooking after the refused cook, want stopped")
	}
}

func TestGnomeSelectedInput(t *testing.T) {
	// The selected slot cooks (first cook at +4).
	c, _, tm := newCookersTest()
	p, _ := cookTestPlayer("alice")
	setCooking(p, 1)
	give(p, fxRawGnomebowl, 1)
	objects := &stubObjectManager{}
	obj := cookGameObject(fxCookingRangeObj, p.Location())
	objects.Register(obj, false)
	c.objects = objects
	c.handleCook(&plugins.ItemOnObjectEvent{
		Player: p, Object: obj, ObjectID: fxCookingRangeObj,
		ItemID: fxRawGnomebowl, Item: game.NewItem(fxRawGnomebowl, 1), ItemSlot: 0,
	})
	if !c.isCookingActive(p) {
		t.Fatal("no session after using the gnomebowl on the range")
	}
	driveTicks(tm, 4)
	if got := countOf(p, fxCookedGnomebowl); got != 1 {
		t.Errorf("cooked gnomebowl = %d, want 1", got)
	}

	// A stale slot (the item moved away) never starts.
	c, _, tm = newCookersTest()
	p, _ = cookTestPlayer("alice")
	setCooking(p, 1)
	give(p, fxRawGnomebowl, 1)
	objects = &stubObjectManager{}
	obj = cookGameObject(fxCookingRangeObj, p.Location())
	objects.Register(obj, false)
	c.objects = objects
	c.handleCook(&plugins.ItemOnObjectEvent{
		Player: p, Object: obj, ObjectID: fxCookingRangeObj,
		ItemID: fxRawGnomebowl, Item: game.NewItem(fxRawGnomebowl, 1), ItemSlot: 5,
	})
	if c.isCookingActive(p) {
		t.Error("session started from a stale slot, want none")
	}

	// A selected input that no longer matches the slot (the
	// event carries amount 2, the slot holds amount 1) is
	// stale and never starts.
	c, _, tm = newCookersTest()
	p, _ = cookTestPlayer("alice")
	setCooking(p, 1)
	give(p, fxRawGnomebowl, 2)
	objects = &stubObjectManager{}
	obj = cookGameObject(fxCookingRangeObj, p.Location())
	objects.Register(obj, false)
	c.objects = objects
	c.handleCook(&plugins.ItemOnObjectEvent{
		Player: p, Object: obj, ObjectID: fxCookingRangeObj,
		ItemID: fxRawGnomebowl, Item: game.NewItem(fxRawGnomebowl, 2), ItemSlot: 0,
	})
	if c.isCookingActive(p) {
		t.Error("session started from a stack of 2, want none")
	}
}

// ---- menus (offerCook / openCookMenu) ----

func TestOfferCookMenus(t *testing.T) {
	// One food, several carried: the "how many" menu, mode 6,
	// max and last amount the carried count.
	c, _, tm := newCookersTest()
	p, session := cookTestPlayer("alice")
	setCooking(p, 1)
	give(p, fxRawShrimp, 3)
	obj := cookGameObject(fxCookingRangeObj, p.Location())
	objects := &stubObjectManager{}
	objects.Register(obj, false)
	c.objects = objects
	claimed := c.handleCookClick(&plugins.ObjectInteractionEvent{
		Player: p, Object: obj, ObjectID: fxCookingRangeObj,
		Definition: cookTestDef{id: fxCookingRangeObj, name: "Cooking range", interactions: []string{"Cook"}},
		Option:     "Cook",
	})
	if !claimed {
		t.Fatal("handleCookClick = false, want true (always claims)")
	}
	if len(session.menus) != 1 {
		t.Fatalf("menus = %d, want 1", len(session.menus))
	}
	menu := session.menus[0]
	if menu.Title() != "How many would you like to cook?" {
		t.Errorf("menu title = %q, want the one-type title", menu.Title())
	}
	if items := menu.Items(); len(items) != 1 || items[0] != fxRawShrimp {
		t.Errorf("menu items = %v, want [%d]", items, fxRawShrimp)
	}
	if opts := menu.Options(); opts.Mode != cookMenuMode || opts.MaxAmount == nil || *opts.MaxAmount != 3 ||
		opts.LastAmount == nil || *opts.LastAmount != 3 {
		t.Errorf("menu options = %+v, want mode 6, max/last 3", opts)
	}
	// Choosing 2 cooks 2.
	menu.Execute(fxRawShrimp, 2)
	if !c.isCookingActive(p) {
		t.Fatal("no session after choosing from the menu")
	}
	driveTicks(tm, 6) // cooks at 2 and 5
	if got := countOf(p, fxShrimp); got != 2 {
		t.Errorf("cooked after menu choice = %d, want 2", got)
	}
	if got := countOf(p, fxRawShrimp); got != 1 {
		t.Errorf("raw left after menu choice = %d, want 1", got)
	}
}

func TestOfferCookManyTypesMenu(t *testing.T) {
	c, _, tm := newCookersTest()
	p, session := cookTestPlayer("alice")
	setCooking(p, 1)
	give(p, fxRawShrimp, 2)
	give(p, fxRawBeef, 3)
	obj := cookGameObject(fxCookingRangeObj, p.Location())
	objects := &stubObjectManager{}
	objects.Register(obj, false)
	c.objects = objects
	c.handleCookClick(&plugins.ObjectInteractionEvent{
		Player: p, Object: obj, ObjectID: fxCookingRangeObj,
		Definition: cookTestDef{id: fxCookingRangeObj, name: "Cooking range", interactions: []string{"Cook"}},
		Option:     "Cook",
	})
	if len(session.menus) != 1 {
		t.Fatalf("menus = %d, want 1", len(session.menus))
	}
	menu := session.menus[0]
	if menu.Title() != "What would you like to cook?" {
		t.Errorf("menu title = %q, want the many-types title", menu.Title())
	}
	// The menu shows what the foods cook into, grouped by
	// product: shrimps and beef.
	items := menu.Items()
	if len(items) != 2 || items[0] != fxShrimp || items[1] != fxCookedBeef {
		t.Errorf("menu items = %v, want [%d %d]", items, fxShrimp, fxCookedBeef)
	}
	if opts := menu.Options(); opts.MaxAmount == nil || *opts.MaxAmount != 3 {
		t.Errorf("menu max amount = %v, want 3 (most carried)", opts.MaxAmount)
	}
	// Choosing beef cooks beef.
	menu.Execute(fxCookedBeef, 2)
	driveTicks(tm, 6)
	if got := countOf(p, fxCookedBeef); got != 2 {
		t.Errorf("cooked beef after menu choice = %d, want 2", got)
	}
	if got := countOf(p, fxShrimp); got != 0 {
		t.Errorf("cooked shrimp after beef choice = %d, want 0", got)
	}
}

func TestOfferCookSingleClickImmediate(t *testing.T) {
	// One raw item, left-clicking the range: cooks at once
	// (2-tick cooking), no menu, session stopped after.
	c, _, tm := newCookersTest()
	p, session := cookTestPlayer("alice")
	setCooking(p, 1)
	give(p, fxRawShrimp, 1)
	obj := cookGameObject(fxCookingRangeObj, p.Location())
	objects := &stubObjectManager{}
	objects.Register(obj, false)
	c.objects = objects
	c.handleCookClick(&plugins.ObjectInteractionEvent{
		Player: p, Object: obj, ObjectID: fxCookingRangeObj,
		Definition: cookTestDef{id: fxCookingRangeObj, name: "Cooking range", interactions: []string{"Cook"}},
		Option:     "Cook",
	})
	if len(session.menus) != 0 {
		t.Fatalf("menus = %d, want 0 (single item cooks at once)", len(session.menus))
	}
	if got := countOf(p, fxShrimp); got != 1 {
		t.Errorf("cooked after single click = %d, want 1 (immediate)", got)
	}
	if c.isCookingActive(p) {
		t.Error("still cooking after the single-click cook, want stopped")
	}
	_ = tm
}

func TestHandleCookClickNothingToCook(t *testing.T) {
	c, _, _ := newCookersTest()
	p, _ := cookTestPlayer("alice")
	setCooking(p, 1)
	obj := cookGameObject(fxCookingRangeObj, p.Location())
	// Empty inventory: the click still claims, and no session
	// starts. The mesbox line is not observable through the
	// fake session (no DialogueSender), so only the claim and
	// the missing session are asserted.
	if !c.handleCookClick(&plugins.ObjectInteractionEvent{
		Player: p, Object: obj, ObjectID: fxCookingRangeObj,
		Definition: cookTestDef{id: fxCookingRangeObj, name: "Cooking range", interactions: []string{"Cook"}},
		Option:     "Cook",
	}) {
		t.Error("handleCookClick(empty) = false, want true (always claims)")
	}
	if c.isCookingActive(p) {
		t.Error("session started with nothing to cook, want none")
	}
}

func TestCarriedCookables(t *testing.T) {
	c, _, _ := newCookersTest()
	p, _ := cookTestPlayer("alice")
	setCooking(p, 1)
	give(p, fxRawShrimp, 3)
	give(p, fxRawBeef, 2)
	give(p, fxRawGuppy, 1) // facility: skipped
	give(p, 9999, 1)       // not a cookable: skipped
	got := c.carriedCookables(p, false)
	if len(got) != 2 || got[0].RawID != fxRawShrimp || got[1].RawID != fxRawBeef {
		t.Errorf("carriedCookables = %v, want [shrimp beef]", rawIDs(got))
	}
	// Noted shrimp is skipped.
	p2, _ := cookTestPlayer("bob")
	setCooking(p2, 1)
	p2.SetItemDefinitions(notedDefs{fakeItemDefs: testItemDefs(), noted: map[int]bool{fxRawShrimp: true}})
	give(p2, fxRawShrimp, 1)
	give(p2, fxRawBeef, 1)
	got = c.carriedCookables(p2, false)
	if len(got) != 1 || got[0].RawID != fxRawBeef {
		t.Errorf("carriedCookables(noted shrimp) = %v, want [beef]", rawIDs(got))
	}
	// A rangeOnly food is skipped on a fire.
	cake := c.tables.CookablesByRaw[fxUncookedCake]
	cake.RangeOnly = true
	p3, _ := cookTestPlayer("carol")
	setCooking(p3, 40)
	give(p3, fxUncookedCake, 1)
	give(p3, fxRawShrimp, 1)
	got = c.carriedCookables(p3, true)
	if len(got) != 1 || got[0].RawID != fxRawShrimp {
		t.Errorf("carriedCookables(rangeOnly on fire) = %v, want [shrimp]", rawIDs(got))
	}
}

func rawIDs(cookables []*Cookable) []int {
	var out []int
	for _, cb := range cookables {
		out = append(out, cb.RawID)
	}
	return out
}

// ---- count / login / pause / disconnect ----

func TestCountCookAndLoginResend(t *testing.T) {
	c, _, _ := newCookersTest()
	p, session := cookTestPlayer("alice")
	setCooking(p, 1)
	// A count from elsewhere (the Gauntlet's range) moves the
	// same attribute and varbit.
	c.countCookEvent(&CountEvent{Player: p, Burnt: false})
	if got := session.varbits[fxSuccessCountVarbit]; got != 1 {
		t.Errorf("success-count varbit after count event = %d, want 1", got)
	}
	c.countCookEvent(&CountEvent{Player: p, Burnt: true})
	if got := session.varbits[fxSuccessCountVarbit]; got != 0 {
		t.Errorf("success-count varbit after burn = %d, want 0", got)
	}
	// Login resends the count when it is above 0, and sends
	// nothing at 0.
	c.countCookEvent(&CountEvent{Player: p, Burnt: false})
	c.countCookEvent(&CountEvent{Player: p, Burnt: false})
	delete(session.varbits, fxSuccessCountVarbit)
	c.sendSuccessCount(&plugins.PlayerLoginEvent{Player: p})
	if got := session.varbits[fxSuccessCountVarbit]; got != 2 {
		t.Errorf("success-count varbit after login = %d, want 2", got)
	}
	c.countCookEvent(&CountEvent{Player: p, Burnt: true})
	delete(session.varbits, fxSuccessCountVarbit)
	c.sendSuccessCount(&plugins.PlayerLoginEvent{Player: p})
	if _, ok := session.varbits[fxSuccessCountVarbit]; ok {
		t.Errorf("success-count varbit sent at 0 = %v, want nothing", session.varbits)
	}
}

func TestPauseCooking(t *testing.T) {
	c, _, tm := newCookersTest()
	p, _, _ := startRangeCook(t, c, tm, fxRawShrimp, 3, 1)
	driveTicks(tm, 2) // first cook lands
	// With no pop-up holding the action, the level-up event
	// stops it.
	c.pauseCooking(&skillactions.LevelUpPopupOpenedPayload{Player: p, Skill: game.SkillCooking})
	if c.isCookingActive(p) {
		t.Error("still cooking after level-up with no pop-up, want stopped")
	}

	// With a pop-up holding it, the session pauses: ticks skip
	// it, and resuming clears the flag.
	c, _, tm = newCookersTest()
	p, _, _ = startRangeCook(t, c, tm, fxRawShrimp, 3, 1)
	driveTicks(tm, 2) // first cook lands
	var resume func()
	c.pauseCooking(&skillactions.LevelUpPopupOpenedPayload{
		Player: p, Skill: game.SkillCooking,
		Pause: func(_ any, r, _ func()) { resume = r },
	})
	if s := c.sessions[p]; s == nil || !s.levelUpPaused {
		t.Fatal("session not paused after level-up with a pop-up")
	}
	driveTicks(tm, 4) // cycle 6 would cook; paused, so nothing
	if got := countOf(p, fxShrimp); got != 1 {
		t.Errorf("cooked while paused = %d, want 1", got)
	}
	resume()
	if s := c.sessions[p]; s == nil || s.levelUpPaused {
		t.Error("session still paused after resume, want cleared")
	}
	driveTicks(tm, 1) // cycle 7: past nextCookTick, cooks
	if got := countOf(p, fxShrimp); got != 2 {
		t.Errorf("cooked after resume = %d, want 2", got)
	}
}

func TestStopOnDisconnect(t *testing.T) {
	c, _, tm := newCookersTest()
	p, _, _ := startRangeCook(t, c, tm, fxRawShrimp, 3, 1)
	c.stopOnDisconnect(&plugins.PlayerDisconnectEvent{Player: p})
	if c.isCookingActive(p) {
		t.Error("still cooking after disconnect, want stopped")
	}
}

// ---- hub-level: OnItemOnObject fallback semantics ----

func TestHubItemOnObjectFallback(t *testing.T) {
	// The fallback finding (plan §2): EmitItemOnObject runs
	// ALL main hooks first, then ALL fallback hooks, and each
	// handler's wrapper skips once the event is Handled — so a
	// fallback-registered refuseCook runs only when no earlier
	// handler claimed the use.
	c, _, _ := newCookersTest()
	hub := plugins.NewHub([]plugins.Plugin{c.plugin()}, plugins.HubOptions{MembersWorld: true})
	hub.Load()
	p, session := cookTestPlayer("alice")
	setCooking(p, 1)
	give(p, fxRawShrimp, 3)
	give(p, 9999, 1)
	obj := cookGameObject(fxCookingRangeObj, p.Location())
	objects := &stubObjectManager{}
	objects.Register(obj, false)
	c.objects = objects

	// Shrimp on the range: handleCook claims it (menu opens);
	// the fallback refuseCook never runs.
	handled := hub.EmitItemOnObject(&plugins.ItemOnObjectEvent{
		Player: p, Object: obj, ObjectID: fxCookingRangeObj,
		ItemID: fxRawShrimp, Item: game.NewItem(fxRawShrimp, 1), ItemSlot: 0,
	})
	if !handled {
		t.Fatal("EmitItemOnObject(shrimp) = false, want true (handleCook claims)")
	}
	if len(session.menus) != 1 {
		t.Fatalf("menus after shrimp use = %d, want 1 (handleCook's menu)", len(session.menus))
	}

	// Junk on the range: nothing claims it, so the fallback
	// refuseCook claims it (no menu, Handled true).
	p2, session2 := cookTestPlayer("bob")
	setCooking(p2, 1)
	give(p2, 9999, 1)
	handled = hub.EmitItemOnObject(&plugins.ItemOnObjectEvent{
		Player: p2, Object: obj, ObjectID: fxCookingRangeObj,
		ItemID: 9999, Item: game.NewItem(9999, 1), ItemSlot: 0,
	})
	if !handled {
		t.Fatal("EmitItemOnObject(junk) = false, want true (fallback refuseCook claims)")
	}
	if len(session2.menus) != 0 {
		t.Errorf("menus after junk use = %d, want 0", len(session2.menus))
	}
	if c.isCookingActive(p2) {
		t.Error("session started from junk, want none")
	}

	// A use on a non-cooker claims nothing at all.
	handled = hub.EmitItemOnObject(&plugins.ItemOnObjectEvent{
		Player: p2, Object: obj, ObjectID: 9998,
		ItemID: 9999, Item: game.NewItem(9999, 1), ItemSlot: 0,
	})
	if handled {
		t.Error("EmitItemOnObject(non-cooker) = true, want false")
	}
}

// ---- registration / wiring ----

func TestRegisterWiresPlugin(t *testing.T) {
	c, _, _ := newCookersTest()
	plugin := c.plugin()
	if plugin.Name != "Cooking" {
		t.Errorf("plugin name = %q, want Cooking", plugin.Name)
	}
	if plugin.Members {
		t.Error("plugin Members = true, want false (Cooking is F2P)")
	}
	hub := plugins.NewHub([]plugins.Plugin{plugin}, plugins.HubOptions{MembersWorld: true})
	hub.Load()
	if loaded := hub.Loaded(); len(loaded) != 1 || loaded[0] != "Cooking" {
		t.Errorf("hub loaded = %v, want [Cooking]", loaded)
	}
	// The success-count attribute is persisted at register.
	// (PersistAttribute is hub-side; the load above proves the
	// registration path runs without panicking.)
}

func TestLoadCookerIDsFromCacheNil(t *testing.T) {
	if _, err := LoadCookerIDsFromCache(nil); err == nil {
		t.Error("LoadCookerIDsFromCache(nil) = nil error, want an error")
	}
}

func TestIsCookingActiveExport(t *testing.T) {
	c, _, tm := newCookersTest()
	p, _, _ := startRangeCook(t, c, tm, fxRawShrimp, 1, 1)
	// The package-level export reads the shared instance, not
	// this test instance, so it stays false here — the
	// instance method is the tested surface.
	if IsCookingActive(p) {
		t.Error("IsCookingActive(shared) = true for a test-instance session, want false")
	}
	if !c.isCookingActive(p) {
		t.Error("isCookingActive(test instance) = false, want true")
	}
}
