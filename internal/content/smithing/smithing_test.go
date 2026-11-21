// Smithing core-loop tests (fixtures only, no live server).
// The tables are hand-built fixtures over the frozen Tables
// API; item ids below are the real rev-242 ids where the
// comment says so, fixture ids (9000s) otherwise.
package smithing

import (
	"strings"
	"testing"

	"tsps-go/internal/game"
	"tsps-go/internal/plugins"
	"tsps-go/internal/session"
)

// ---- fixture ids (real rev-242 ids unless marked fixture) ----

const (
	fxCopperOre    = 436
	fxTinOre       = 438
	fxIronOre      = 440
	fxSilverOre    = 442
	fxGoldOre      = 444
	fxCoal         = 453
	fxBluriteOre   = 668
	fxSodaAsh      = 1781
	fxBucketSand   = 1783
	fxMoltenGlass  = 1775
	fxBucket       = 1925
	fxBronzeBar    = 2349
	fxIronBar      = 2351
	fxSteelBar     = 2353
	fxSilverBar    = 2355
	fxGoldBar      = 2357
	fxLovakiteOre  = 13356
	fxLovakiteBar  = 13354
	fxPerfectOre   = 2364 // fixture-adjacent: perfect gold ore
	fxPerfectBar   = 2365
	fxBluriteBar   = 9467
	fxHammer       = 2347
	fxLogs         = 1511
	fxBronzeBall   = 886
	fxSteelBall    = 2
	fxBronzeSpear  = 9001 // fixture ids
	fxBronzeHasta  = 9002 // fixture ids
	fxBronzeDagger = 1205
	fxBronzeAxe    = 1351
	fxBronzePlate  = 1121
	fxIronDagger   = 1203
)

// fixtureTables builds the tables the loop tests need.
func fixtureTables() *Tables {
	return &Tables{
		Smelting: []SmeltRecipe{
			{
				Name: "Bronze bar", BarID: fxBronzeBar, Bar: fxBronzeBar, Level: 1, XP: 6.2,
				Ingredients:     []Ingredient{{ItemID: fxCopperOre, Amount: 1}, {ItemID: fxTinOre, Amount: 1}},
				BarPhrase:       "a bronze bar",
				SmeltMessage:    "You smelt the copper and tin together in the furnace.",
				RetrieveMessage: "You retrieve a bar of bronze.",
				MissingMessage:  "You need at least one copper ore and one tin ore to smelt a bronze bar.",
			},
			{
				Name: "Blurite bar", BarID: fxBluriteBar, Bar: fxBluriteBar, Level: 13, XP: 8,
				Ingredients:     []Ingredient{{ItemID: fxBluriteOre, Amount: 1}},
				BarPhrase:       "a blurite bar",
				SmeltMessage:    "You smelt the blurite in the furnace.",
				RetrieveMessage: "You retrieve a bar of blurite.",
				MissingMessage:  "You need at least one blurite ore to smelt a blurite bar.",
				Quest:           "the_knights_sword",
			},
			{
				Name: "Iron bar", BarID: fxIronBar, Bar: fxIronBar, Level: 15, XP: 12.5,
				Ingredients:     []Ingredient{{ItemID: fxIronOre, Amount: 1}},
				BarPhrase:       "an iron bar",
				SmeltMessage:    "You smelt the iron in the furnace.",
				RetrieveMessage: "You retrieve a bar of iron.",
				MissingMessage:  "You need at least one iron ore to smelt an iron bar.",
				SuccessChance:   0.5, HasSuccessChance: true,
				FailMessage: "The ore is too impure and you fail to refine it.",
			},
			{
				Name: "Silver bar", BarID: fxSilverBar, Bar: fxSilverBar, Level: 20, XP: 13.7,
				Ingredients:     []Ingredient{{ItemID: fxSilverOre, Amount: 1}},
				BarPhrase:       "a silver bar",
				SmeltMessage:    "You smelt the silver in the furnace.",
				RetrieveMessage: "You retrieve a bar of silver.",
				MissingMessage:  "You need at least one silver ore to smelt a silver bar.",
			},
			{
				Name: "Steel bar", BarID: fxSteelBar, Bar: fxSteelBar, Level: 30, XP: 17.5,
				Ingredients:     []Ingredient{{ItemID: fxIronOre, Amount: 1}, {ItemID: fxCoal, Amount: 2}},
				BarPhrase:       "a steel bar",
				SmeltMessage:    "You smelt the iron and coal together in the furnace.",
				RetrieveMessage: "You retrieve a bar of steel.",
				MissingMessage:  "You need at least one iron ore and two coal to smelt a steel bar.",
			},
			{
				Name: "Gold bar", BarID: fxGoldBar, Bar: fxGoldBar, Level: 40, XP: 22.5,
				Ingredients:     []Ingredient{{ItemID: fxGoldOre, Amount: 1}},
				BarPhrase:       "a gold bar",
				SmeltMessage:    "You smelt the gold in the furnace.",
				RetrieveMessage: "You retrieve a bar of gold.",
				MissingMessage:  "You need at least one gold ore to smelt a gold bar.",
			},
			{
				Name: "Lovakite bar", BarID: fxLovakiteBar, Bar: fxLovakiteBar, Level: 45, XP: 20,
				Ingredients:     []Ingredient{{ItemID: fxLovakiteOre, Amount: 1}, {ItemID: fxCoal, Amount: 2}},
				BarPhrase:       "a lovakite bar",
				SmeltMessage:    "You smelt the lovakite in the furnace.",
				RetrieveMessage: "You retrieve a bar of lovakite.",
				MissingMessage:  "You need at least one lovakite ore and two coal to smelt a lovakite bar.",
				Furnace:         "Lovakite furnace", NotInMenu: true,
			},
			{
				Name: "'perfect' gold bar", BarID: fxPerfectBar, Bar: fxPerfectBar, Level: 40, XP: 22.5,
				Ingredients:     []Ingredient{{ItemID: fxPerfectOre, Amount: 1}},
				BarPhrase:       "a 'perfect' gold bar",
				SmeltMessage:    "You smelt the gold in the furnace.",
				RetrieveMessage: "You retrieve a bar of 'perfect' gold.",
				MissingMessage:  "You need at least one 'perfect' gold ore to smelt a 'perfect' gold bar.",
				NotInMenu:       true,
			},
			{
				Name: "Molten glass", BarID: fxMoltenGlass, Bar: fxMoltenGlass, Level: 1, XP: 20,
				Ingredients:    []Ingredient{{ItemID: fxBucketSand, Amount: 1}, {ItemID: fxSodaAsh, Amount: 1}},
				Byproducts:     []Ingredient{{ItemID: fxBucket, Amount: 1}},
				BarPhrase:      "molten glass",
				SmeltMessage:   "You heat the sand and soda ash in the furnace.",
				MissingMessage: "You need a bucket of sand and soda ash to make molten glass.",
				Skill:          "CRAFTING", Members: true, NotInMenu: true,
				Interval: 2, FirstDelay: 2, FinishDelay: 0, HasTiming: true,
			},
		},
		Cannonballs: []CannonballRecipe{
			{BallID: fxBronzeBall, BarID: fxBronzeBar, Level: 5, XPPerBar: 8.8},
			{BallID: fxSteelBall, BarID: fxSteelBar, Level: 35, XPPerBar: 25.6},
		},
		Blurite: []BluriteProduct{
			{ItemID: 9376, Level: 8, XP: 17.5, Amount: 10, Phrase: "ten blurite crossbow bolts"},
			{ItemID: 9422, Level: 13, XP: 17.5, Amount: 1, Phrase: "a pair of crossbow limbs"},
		},
		Spears: []SpearRecipe{
			{BarID: fxBronzeBar, LogID: fxLogs, Level: 5, XP: 25, SpearID: fxBronzeSpear, HastaID: fxBronzeHasta},
		},
		Anvil: []AnvilProduct{
			{BarID: fxBronzeBar, BarType: 1, Component: 9, SlotIndex: 1, ItemID: fxBronzeDagger, Level: 1, Bars: 1, Amount: 1, Metal: "bronze", Obj: "bronze_dagger", Label: "Dagger", Phrase: "a dagger", XP: 12.5},
			{BarID: fxBronzeBar, BarType: 1, Component: 10, SlotIndex: 2, ItemID: fxBronzeAxe, Level: 1, Bars: 1, Amount: 1, Metal: "bronze", Obj: "bronze_axe", Label: "Axe", Phrase: "an axe", XP: 12.5},
			{BarID: fxBronzeBar, BarType: 1, Component: 30, SlotIndex: 22, ItemID: fxBronzePlate, Level: 18, Bars: 5, Amount: 1, Metal: "bronze", Obj: "bronze_platebody", Label: "Platebody", Phrase: "a bronze platebody", XP: 62.5},
			{BarID: fxIronBar, BarType: 2, Component: 9, SlotIndex: 1, ItemID: fxIronDagger, Level: 15, Bars: 1, Amount: 1, Metal: "iron", Obj: "iron_dagger", Label: "Dagger", Phrase: "a dagger", XP: 25},
		},
		QuestNames: map[string]string{
			"the_knights_sword": "The Knight's Sword",
			"dwarf_cannon":      "Dwarf Cannon",
		},
		XPPerBar: map[int]float64{fxBronzeBar: 12.5, fxIronBar: 25},
	}
}

// ---- fake session ----

// fakeSession implements game.SessionPort plus the chat,
// varbit, CreationMenu and anvil sender extensions,
// capturing what content sends.
type fakeSession struct {
	messages []string
	spam     []string
	varbits  map[int]int
	varps    map[int]int
	menus    []*game.CreationMenu
	mounts   []mountedInterface
	flags    []flagRange
	packets  [][]byte
	closed   []int
}

type mountedInterface struct {
	targetUID int
	groupID   int
	typ       int
}

type flagRange struct {
	uid      int
	fromSlot int
	toSlot   int
	flags    int
}

func (s *fakeSession) SendPacket(packet []byte) { s.packets = append(s.packets, packet) }
func (s *fakeSession) SendRunEnergy(units int, running bool, weightKg int, staminaTicks int) {
}
func (s *fakeSession) SendDestination(x, y int) {}
func (s *fakeSession) SendVarp(id, value int) {
	if s.varps == nil {
		s.varps = make(map[int]int)
	}
	s.varps[id] = value
}
func (s *fakeSession) FlushTick(cycle int64) error   { return nil }
func (s *fakeSession) Close(code int, reason string) {}
func (s *fakeSession) IsOpen() bool                  { return true }
func (s *fakeSession) SendGameMessage(message string) {
	s.messages = append(s.messages, message)
}
func (s *fakeSession) SendTypedGameMessage(chatType int, message, from string) {
	s.spam = append(s.spam, message)
}
func (s *fakeSession) SendVarbit(id, value int) {
	if s.varbits == nil {
		s.varbits = make(map[int]int)
	}
	s.varbits[id] = value
}
func (s *fakeSession) GetVarbit(id int) int { return s.varbits[id] }
func (s *fakeSession) SendCreationMenu(menu *game.CreationMenu, itemName func(id int) string, lastItem int, hasLastItem bool) {
	s.menus = append(s.menus, menu)
}
func (s *fakeSession) CloseCreationMenu() {}
func (s *fakeSession) SendInterfaceFlagsRange(uid, fromSlot, toSlot, flags int) {
	s.flags = append(s.flags, flagRange{uid, fromSlot, toSlot, flags})
}
func (s *fakeSession) MountSubInterface(targetUID, groupID, typ int) *session.SubInterfaceMount {
	s.mounts = append(s.mounts, mountedInterface{targetUID, groupID, typ})
	return &session.SubInterfaceMount{TargetUID: targetUID, GroupID: groupID, Type: typ}
}
func (s *fakeSession) CloseSubInterface(targetUID int) { s.closed = append(s.closed, targetUID) }
func (s *fakeSession) CloseSubInterfaceMount(mount *session.SubInterfaceMount) bool {
	if mount == nil {
		return false
	}
	s.closed = append(s.closed, mount.TargetUID)
	return true
}

// fakeItemDefs is the item-definition source for tests.
type fakeItemDefs struct {
	names     map[int]string
	stackable map[int]bool
}

func (f fakeItemDefs) IsStackable(id int) bool      { return f.stackable[id] }
func (f fakeItemDefs) IsNoted(id int) bool          { return false }
func (f fakeItemDefs) WeightGrams(id int) int       { return 0 }
func (f fakeItemDefs) EquipmentSlot(id int) int     { return -1 }
func (f fakeItemDefs) IsDoubleHanded(id int) bool   { return false }
func (f fakeItemDefs) Requirements(id int) []int    { return nil }
func (f fakeItemDefs) CombatRequirement(id int) int { return 0 }
func (f fakeItemDefs) IsTradeable(id int) bool      { return false }
func (f fakeItemDefs) IsSellable(id int) bool       { return false }
func (f fakeItemDefs) IsDropable(id int) bool       { return false }
func (f fakeItemDefs) UnNote(id int) int            { return id }
func (f fakeItemDefs) PlaceholderID(id int) int     { return -1 }
func (f fakeItemDefs) ItemName(id int) string       { return f.names[id] }
func (f fakeItemDefs) IsMembersItem(id int) bool    { return false }

func testItemDefs() fakeItemDefs {
	return fakeItemDefs{
		names: map[int]string{
			fxCopperOre: "Copper ore", fxTinOre: "Tin ore", fxIronOre: "Iron ore",
			fxSilverOre: "Silver ore", fxGoldOre: "Gold ore", fxCoal: "Coal",
			fxBluriteOre: "Blurite ore", fxLovakiteOre: "Lovakite ore",
			fxBucketSand: "Bucket of sand", fxSodaAsh: "Soda ash", fxBucket: "Bucket",
			fxMoltenGlass: "Molten glass", fxBronzeBar: "Bronze bar", fxIronBar: "Iron bar",
			fxSteelBar: "Steel bar", fxSilverBar: "Silver bar", fxGoldBar: "Gold bar",
			fxLovakiteBar: "Lovakite bar", fxPerfectBar: "'perfect' gold bar",
			fxBluriteBar: "Blurite bar", fxHammer: "Hammer", fxLogs: "Logs",
			fxBronzeBall: "Bronze cannonball", fxSteelBall: "Steel cannonball",
			fxBronzeDagger: "Bronze dagger", fxBronzeAxe: "Bronze axe",
			fxBronzePlate: "Bronze platebody", fxIronDagger: "Iron dagger",
			9376: "Blurite bolts (unf)", 9422: "Blurite limbs",
			fxBronzeSpear: "Bronze spear", fxBronzeHasta: "Bronze hasta",
			4: "Ammo mould", 27012: "Double ammo mould",
			2568: "Ring of forging", 13104: "Varrock armour 1",
			776: "Goldsmith gauntlets", 27017: "Smithing catalyst",
		},
		stackable: map[int]bool{fxBronzeBall: true, fxSteelBall: true},
	}
}

// fakeObject is a plugins.GameObject naming one object.
type fakeObject struct {
	id  int
	loc game.Location
}

func (o *fakeObject) ID() int                 { return o.id }
func (o *fakeObject) Location() game.Location { return o.loc }
func (o *fakeObject) Type() int               { return 10 }
func (o *fakeObject) Face() int               { return 0 }

// fakeObjectDef is a plugins.ObjectDefinition naming one object.
type fakeObjectDef struct{ name string }

func (d *fakeObjectDef) ID() int                { return 0 }
func (d *fakeObjectDef) Name() string           { return d.name }
func (d *fakeObjectDef) SizeX() int             { return 1 }
func (d *fakeObjectDef) SizeY() int             { return 1 }
func (d *fakeObjectDef) BlockingMask() int      { return 0 }
func (d *fakeObjectDef) Interactions() []string { return nil }

// ---- harness ----

type emittedEvent struct {
	name    string
	payload any
}

// newTestSmithing builds a smithing instance over the fixture
// tables with a capturing emitter. The emitter answers
// nothing by itself; tests that need a live answerer (Gear's
// smelt-outcome) install one through emitHook.
func newTestSmithing(t *testing.T) (*smithing, *[]emittedEvent) {
	t.Helper()
	SetTables(fixtureTables())
	t.Cleanup(func() { SetTables(nil) })
	var emitted []emittedEvent
	s := newSmithing()
	s.emit = func(name string, payload any) {
		emitted = append(emitted, emittedEvent{name: name, payload: payload})
	}
	s.randFloat = func() float64 { return 0.5 }
	return s, &emitted
}

func newTestPlayer(name string) (*game.Player, *fakeSession) {
	p := game.NewPlayer(name, game.Location{X: 3200, Y: 3200, Z: 0})
	p.SetRegistered(true)
	p.SetItemDefinitions(testItemDefs())
	setLevel(p, game.SkillHitpoints, 10)
	session := &fakeSession{}
	p.Session = session
	return p, session
}

func setLevel(p *game.Player, skill, level int) {
	p.Skills().SetCurrentLevel(skill, level, true)
	p.Skills().SetMaxLevel(skill, level)
}

func give(p *game.Player, id, amount int) {
	p.Inventory().Add(game.NewItem(id, amount), true)
}

func wear(p *game.Player, slot, id int) {
	p.Equipment().Set(slot, game.NewItem(id, 1))
}

func hasMessage(sess *fakeSession, want string) bool {
	for _, m := range sess.messages {
		if strings.Contains(m, want) {
			return true
		}
	}
	return false
}

func hasSpam(sess *fakeSession, want string) bool {
	for _, m := range sess.spam {
		if strings.Contains(m, want) {
			return true
		}
	}
	return false
}

func hasEvent(emitted *[]emittedEvent, name string) bool {
	for _, e := range *emitted {
		if e.name == name {
			return true
		}
	}
	return false
}

func statementText(p *game.Player) (string, bool) {
	dm := p.GetDialogueManager()
	if dm == nil || !dm.IsActive() {
		return "", false
	}
	if stmt, ok := dm.GetCurrent().(*game.StatementDialogue); ok {
		return stmt.GetText(), true
	}
	return "", false
}

// driveTicks runs processSmithingTick for the next n cycles,
// advancing the session clock (the mining precedent: tests
// set the clock, then drive).
func driveTicks(s *smithing, n int) {
	for i := 0; i < n; i++ {
		s.tick++
		s.processSmithingTick(s.tick)
	}
}

func smeltRecipe(tables *Tables, barID int) *SmeltRecipe {
	return tables.SmeltByBar(barID)
}

// TestObjectNameResolverInstalled proves the item-on-object
// router can resolve object names once wiring installs the
// resolver (gap 26f): the Lumbridge furnace 24009 resolves to
// "Furnace" and the Rusted anvil 39620 to "Rusted anvil", an
// unknown id resolves to "", and clearing the resolver (nil)
// restores the "" degradation. The ids are the cache-probed
// live ids the smoke43 harness uses.
func TestObjectNameResolverInstalled(t *testing.T) {
	if got := shared.objectNameOf(24009); got != "" {
		t.Fatalf("objectNameOf(24009) without a resolver = %q, want \"\"", got)
	}
	names := map[int]string{24009: "Furnace", 39620: "Rusted anvil"}
	SetObjectNameResolver(func(id int) string { return names[id] })
	t.Cleanup(func() { SetObjectNameResolver(nil) })
	if got := shared.objectNameOf(24009); got != "Furnace" {
		t.Errorf("objectNameOf(24009) = %q, want %q", got, "Furnace")
	}
	if got := shared.objectNameOf(39620); got != "Rusted anvil" {
		t.Errorf("objectNameOf(39620) = %q, want %q", got, "Rusted anvil")
	}
	if got := shared.objectNameOf(99999); got != "" {
		t.Errorf("objectNameOf(99999) = %q, want \"\"", got)
	}
	SetObjectNameResolver(nil)
	if got := shared.objectNameOf(24009); got != "" {
		t.Errorf("objectNameOf(24009) after clearing = %q, want \"\"", got)
	}
}

// ---- smelting ----

func TestSmeltBronzeLoopTimingAndXP(t *testing.T) {
	s, _ := newTestSmithing(t)
	p, sess := newTestPlayer("smith")
	setLevel(p, game.SkillSmithing, 1)
	give(p, fxCopperOre, 2)
	give(p, fxTinOre, 2)
	recipe := smeltRecipe(tables, fxBronzeBar)
	if !s.startSmelting(p, recipe, 2, -1) {
		t.Fatal("startSmelting failed")
	}
	// Tick 1: begin (message + anim); no bar yet.
	driveTicks(s, 1)
	if has := p.Inventory().GetAmount(fxBronzeBar); has != 0 {
		t.Errorf("after tick 1: bronze bars = %d, want 0", has)
	}
	if !hasSpam(sess, "You smelt the copper and tin together in the furnace.") {
		t.Errorf("begin spam = %v, want the smelt line", sess.spam)
	}
	// Tick 5: first bar + XP (6.2 truncates to 6).
	driveTicks(s, 4)
	if got := p.Inventory().GetAmount(fxBronzeBar); got != 1 {
		t.Errorf("after tick 5: bronze bars = %d, want 1", got)
	}
	if got := p.Skills().Experience(game.SkillSmithing); got != 6 {
		t.Errorf("smithing XP = %d, want 6 (6.2 truncates)", got)
	}
	if !hasSpam(sess, "You retrieve a bar of bronze.") {
		t.Errorf("retrieve spam = %v, want the retrieve line", sess.spam)
	}
	// Tick 6: second begin; tick 10: second bar, session ends.
	driveTicks(s, 5)
	if got := p.Inventory().GetAmount(fxBronzeBar); got != 2 {
		t.Errorf("after tick 10: bronze bars = %d, want 2", got)
	}
	if got := p.Skills().Experience(game.SkillSmithing); got != 12 {
		t.Errorf("smithing XP = %d, want 12", got)
	}
	if _, ok := s.sessions[p]; ok {
		t.Error("session still active after the last bar")
	}
}

func TestSmeltIronFailsHalf(t *testing.T) {
	s, _ := newTestSmithing(t)
	p, sess := newTestPlayer("smith")
	setLevel(p, game.SkillSmithing, 15)
	give(p, fxIronOre, 1)
	recipe := smeltRecipe(tables, fxIronBar)
	// randFloat 0.9 >= 0.5 success chance: the smelt fails.
	s.randFloat = func() float64 { return 0.9 }
	if !s.startSmelting(p, recipe, 1, -1) {
		t.Fatal("startSmelting failed")
	}
	driveTicks(s, 5)
	if got := p.Inventory().GetAmount(fxIronBar); got != 0 {
		t.Errorf("failed iron: bars = %d, want 0", got)
	}
	if got := p.Inventory().GetAmount(fxIronOre); got != 0 {
		t.Errorf("failed iron: ore left = %d, want 0 (ore is consumed)", got)
	}
	if got := p.Skills().Experience(game.SkillSmithing); got != 0 {
		t.Errorf("failed iron: XP = %d, want 0", got)
	}
	if !hasSpam(sess, "The ore is too impure and you fail to refine it.") {
		t.Errorf("fail spam = %v, want the impure line", sess.spam)
	}
	if _, ok := s.sessions[p]; ok {
		t.Error("session still active after the failed bar")
	}

	// randFloat 0.1 < 0.5: the smelt succeeds.
	s2, _ := newTestSmithing(t)
	p2, _ := newTestPlayer("smith2")
	setLevel(p2, game.SkillSmithing, 15)
	give(p2, fxIronOre, 1)
	s2.randFloat = func() float64 { return 0.1 }
	if !s2.startSmelting(p2, smeltRecipe(tables, fxIronBar), 1, -1) {
		t.Fatal("startSmelting failed")
	}
	driveTicks(s2, 5)
	if got := p2.Inventory().GetAmount(fxIronBar); got != 1 {
		t.Errorf("good iron: bars = %d, want 1", got)
	}
	if got := p2.Skills().Experience(game.SkillSmithing); got != 12 {
		t.Errorf("good iron: XP = %d, want 12 (12.5 truncates)", got)
	}
}

func TestSmeltCatalystHalvesCoalDoublesXP(t *testing.T) {
	s, _ := newTestSmithing(t)
	p, _ := newTestPlayer("smith")
	setLevel(p, game.SkillSmithing, 30)
	give(p, fxIronOre, 1)
	give(p, fxCoal, 1)
	give(p, 27017, 1) // Smithing catalyst
	recipe := smeltRecipe(tables, fxSteelBar)
	if !s.startSmelting(p, recipe, 1, -1) {
		t.Fatal("startSmelting failed (catalyst should cover 1 coal)")
	}
	driveTicks(s, 5)
	if got := p.Inventory().GetAmount(fxSteelBar); got != 1 {
		t.Fatalf("catalysed steel: bars = %d, want 1", got)
	}
	if got := p.Inventory().GetAmount(fxCoal); got != 0 {
		t.Errorf("catalysed steel: coal left = %d, want 0 (half of 2)", got)
	}
	if got := p.Inventory().GetAmount(27017); got != 0 {
		t.Errorf("catalysed steel: catalyst left = %d, want 0", got)
	}
	if got := p.Skills().Experience(game.SkillSmithing); got != 35 {
		t.Errorf("catalysed steel: XP = %d, want 35 (17.5 x 2)", got)
	}
}

func TestSmeltGoldsmithGauntlets(t *testing.T) {
	s, _ := newTestSmithing(t)
	p, _ := newTestPlayer("smith")
	setLevel(p, game.SkillSmithing, 40)
	give(p, fxGoldOre, 1)
	wear(p, game.HandsSlot, 776) // Goldsmith gauntlets
	if !s.startSmelting(p, smeltRecipe(tables, fxGoldBar), 1, -1) {
		t.Fatal("startSmelting failed")
	}
	driveTicks(s, 5)
	if got := p.Skills().Experience(game.SkillSmithing); got != 56 {
		t.Errorf("gauntlet gold: XP = %d, want 56 (22.5 + 33.7 truncates)", got)
	}

	s2, _ := newTestSmithing(t)
	p2, _ := newTestPlayer("smith2")
	setLevel(p2, game.SkillSmithing, 40)
	give(p2, fxGoldOre, 1)
	if !s2.startSmelting(p2, smeltRecipe(tables, fxGoldBar), 1, -1) {
		t.Fatal("startSmelting failed")
	}
	driveTicks(s2, 5)
	if got := p2.Skills().Experience(game.SkillSmithing); got != 22 {
		t.Errorf("bare-handed gold: XP = %d, want 22 (22.5 truncates)", got)
	}
}

func TestSmeltMenuOpensMode13(t *testing.T) {
	s, _ := newTestSmithing(t)
	p, sess := newTestPlayer("smith")
	setLevel(p, game.SkillSmithing, 1)
	give(p, fxCopperOre, 3)
	give(p, fxTinOre, 3)
	s.smelt(p, -1)
	if len(sess.menus) != 1 {
		t.Fatalf("menus = %d, want 1", len(sess.menus))
	}
	menu := sess.menus[0]
	if menu.Title() != smeltTitle {
		t.Errorf("menu title = %q, want %q", menu.Title(), smeltTitle)
	}
	if menu.Options().Mode != smeltMenuMode {
		t.Errorf("menu mode = %d, want %d", menu.Options().Mode, smeltMenuMode)
	}
	found := false
	for _, id := range menu.Items() {
		if id == fxBronzeBar {
			found = true
		}
	}
	if !found {
		t.Errorf("menu items = %v, want bronze bar listed", menu.Items())
	}
	if menu.Options().MaxAmount == nil || *menu.Options().MaxAmount != 3 {
		t.Errorf("menu maxAmount = %v, want 3 (ore covers 3)", menu.Options().MaxAmount)
	}
}

func TestSmeltNoOreMessageBox(t *testing.T) {
	s, _ := newTestSmithing(t)
	p, _ := newTestPlayer("smith")
	setLevel(p, game.SkillSmithing, 1)
	s.smelt(p, -1)
	text, ok := statementText(p)
	if !ok || text != noOreMessage {
		t.Errorf("statement = %q (active %v), want %q", text, ok, noOreMessage)
	}
}

func TestSmeltLoneIronOreSmeltsOnce(t *testing.T) {
	s, _ := newTestSmithing(t)
	p, sess := newTestPlayer("smith")
	setLevel(p, game.SkillSmithing, 15)
	give(p, fxIronOre, 1)
	s.objectNameResolver = func(id int) string { return "Furnace" }
	s.randFloat = func() float64 { return 0.1 }
	event := &plugins.ItemOnObjectEvent{
		Player: p, Object: &fakeObject{id: 100, loc: p.Location()},
		ObjectID: 100, ItemID: fxIronOre,
	}
	s.useItemOnObject(event)
	if !event.Handled {
		t.Fatal("iron-on-furnace was not handled")
	}
	if len(sess.menus) != 0 {
		t.Errorf("menus = %d, want 0 (lone ore smelts directly)", len(sess.menus))
	}
	if _, ok := s.sessions[p]; !ok {
		t.Fatal("no smelt session started for the lone ore")
	}
	driveTicks(s, 5)
	if got := p.Inventory().GetAmount(fxIronBar); got != 1 {
		t.Errorf("lone iron: bars = %d, want 1", got)
	}
}

func TestSmeltMoltenGlassTiming(t *testing.T) {
	s, _ := newTestSmithing(t)
	p, _ := newTestPlayer("smith")
	setLevel(p, game.SkillCrafting, 1)
	give(p, fxBucketSand, 2)
	give(p, fxSodaAsh, 2)
	recipe := smeltRecipe(tables, fxMoltenGlass)
	if !s.startSmelting(p, recipe, 2, -1) {
		t.Fatal("startSmelting failed")
	}
	// firstDelay 2: the first glass lands on tick 3
	// (nextCycle 3, finishDelay 0), the second on tick 5.
	driveTicks(s, 2)
	if got := p.Inventory().GetAmount(fxMoltenGlass); got != 0 {
		t.Errorf("after tick 2: glass = %d, want 0", got)
	}
	driveTicks(s, 1)
	if got := p.Inventory().GetAmount(fxMoltenGlass); got != 1 {
		t.Errorf("after tick 3: glass = %d, want 1", got)
	}
	if got := p.Inventory().GetAmount(fxBucket); got != 1 {
		t.Errorf("after tick 3: buckets = %d, want 1 (byproduct)", got)
	}
	if got := p.Skills().Experience(game.SkillCrafting); got != 20 {
		t.Errorf("crafting XP = %d, want 20", got)
	}
	if got := p.Skills().Experience(game.SkillSmithing); got != 0 {
		t.Errorf("smithing XP = %d, want 0 (glass is Crafting)", got)
	}
	driveTicks(s, 2)
	if got := p.Inventory().GetAmount(fxMoltenGlass); got != 2 {
		t.Errorf("after tick 5: glass = %d, want 2", got)
	}
	if _, ok := s.sessions[p]; ok {
		t.Error("session still active after the last glass")
	}
}

func TestSmeltLevelAndQuestGates(t *testing.T) {
	s, _ := newTestSmithing(t)
	p, _ := newTestPlayer("smith")
	setLevel(p, game.SkillSmithing, 1)
	give(p, fxIronOre, 1)
	if s.startSmelting(p, smeltRecipe(tables, fxIronBar), 1, -1) {
		t.Error("startSmelting succeeded below the level")
	}
	text, ok := statementText(p)
	if !ok || !strings.Contains(text, "You need a Smithing level of 15") {
		t.Errorf("statement = %q (active %v), want the level 15 line", text, ok)
	}

	// Blurite needs The Knight's Sword; quest:is-complete is
	// unanswered, so it fails closed.
	p2, _ := newTestPlayer("smith2")
	setLevel(p2, game.SkillSmithing, 13)
	give(p2, fxBluriteOre, 1)
	if s.startSmelting(p2, smeltRecipe(tables, fxBluriteBar), 1, -1) {
		t.Error("blurite smelt succeeded without the quest")
	}
}

func TestSmeltPerfectGoldFallback(t *testing.T) {
	s, _ := newTestSmithing(t)
	p, _ := newTestPlayer("smith")
	setLevel(p, game.SkillSmithing, 40)
	give(p, fxPerfectOre, 1) // no plain gold ore at all
	recipe := s.resolveRecipe(p, smeltRecipe(tables, fxGoldBar))
	if recipe == nil || recipe.BarID != fxPerfectBar {
		t.Fatalf("resolveRecipe = %v, want the perfect gold recipe", recipe)
	}
	if !s.startSmelting(p, smeltRecipe(tables, fxGoldBar), 1, -1) {
		t.Fatal("startSmelting failed")
	}
	driveTicks(s, 5)
	if got := p.Inventory().GetAmount(fxPerfectBar); got != 1 {
		t.Errorf("perfect gold bars = %d, want 1", got)
	}
}

// ---- sessions ----

func TestSessionStopsOnMoveDeathLogout(t *testing.T) {
	s, _ := newTestSmithing(t)
	p, _ := newTestPlayer("smith")
	setLevel(p, game.SkillSmithing, 1)
	give(p, fxCopperOre, 5)
	give(p, fxTinOre, 5)
	if !s.startSmelting(p, smeltRecipe(tables, fxBronzeBar), 5, -1) {
		t.Fatal("startSmelting failed")
	}
	// Walk away: the session stops, no bar lands.
	// Move the player one tile east of the tile the session
	// snapshotted.
	start := p.Location()
	p.SetLocation(game.Location{X: start.X + 1, Y: start.Y, Z: start.Z})
	s.setTick(1)
	s.processSmithingTick(1)
	if _, ok := s.sessions[p]; ok {
		t.Error("session still active after moving")
	}

	// Death stops the session.
	p2, _ := newTestPlayer("smith2")
	setLevel(p2, game.SkillSmithing, 1)
	setLevel(p2, game.SkillHitpoints, 10)
	give(p2, fxCopperOre, 5)
	give(p2, fxTinOre, 5)
	if !s.startSmelting(p2, smeltRecipe(tables, fxBronzeBar), 5, -1) {
		t.Fatal("startSmelting failed")
	}
	setLevel(p2, game.SkillHitpoints, 0)
	s.setTick(2)
	s.processSmithingTick(2)
	if _, ok := s.sessions[p2]; ok {
		t.Error("session still active after death")
	}

	// Disconnect stops the session and forgets the anvil.
	p3, _ := newTestPlayer("smith3")
	setLevel(p3, game.SkillSmithing, 1)
	give(p3, fxCopperOre, 5)
	give(p3, fxTinOre, 5)
	if !s.startSmelting(p3, smeltRecipe(tables, fxBronzeBar), 5, -1) {
		t.Fatal("startSmelting failed")
	}
	s.disconnect(&plugins.PlayerDisconnectEvent{Player: p3})
	if _, ok := s.sessions[p3]; ok {
		t.Error("session still active after disconnect")
	}
}

func TestBarForOre(t *testing.T) {
	s, _ := newTestSmithing(t)
	req := &barForOreRequest{OreID: fxIronOre}
	s.answerBarForOre(req)
	if req.BarID != fxIronBar || req.XP != 12.5 {
		t.Errorf("bar-for-ore iron = (%d, %v), want (2351, 12.5)", req.BarID, req.XP)
	}
	// Coal, a steel bar's second ingredient, smelts into
	// nothing.
	req = &barForOreRequest{OreID: fxCoal}
	s.answerBarForOre(req)
	if req.BarID != 0 {
		t.Errorf("bar-for-ore coal = %d, want 0", req.BarID)
	}
}

func TestQuantityForSmeltAction(t *testing.T) {
	if got := QuantityForSmeltAction(0); got != 1 {
		t.Errorf("quantity(0) = %d, want 1", got)
	}
	if got := QuantityForSmeltAction(5); got != 5 {
		t.Errorf("quantity(5) = %d, want 5", got)
	}
	if got := QuantityForSmeltAction(99); got != 28 {
		t.Errorf("quantity(99) = %d, want 28", got)
	}
}

func TestTablesNilNoops(t *testing.T) {
	s := newSmithing()
	SetTables(nil)
	p, _ := newTestPlayer("smith")
	setLevel(p, game.SkillSmithing, 99)
	give(p, fxCopperOre, 1)
	give(p, fxTinOre, 1)
	// With no tables installed, startSmelting must not panic
	// or start anything: the recipe lookup itself is nil.
	if s.startSmelting(p, nil, 1, -1) {
		t.Error("startSmelting with nil recipe succeeded")
	}
	if _, ok := s.sessions[p]; ok {
		t.Error("session started with no tables")
	}
}
