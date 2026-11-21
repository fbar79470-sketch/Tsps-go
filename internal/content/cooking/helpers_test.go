package cooking

import (
	"tsps-go/internal/game"
)

// ---- fake ids (hand-built fixtures only; no live cache) ----

const (
	fxRawShrimp   = 100
	fxShrimp      = 101
	fxBurntShrimp = 102
	fxRawGuppy    = 120
	fxGuppy       = 121
	fxRuinedGuppy = 122

	fxFlour       = 200
	fxPot         = 201
	fxBucketWater = 210
	fxBucketEmpty = 211
	fxBowlWater   = 212
	fxBowlEmpty   = 213
	fxJugWater    = 214
	fxJugEmpty    = 215

	fxBreadDough  = 220
	fxPastryDough = 221
	fxPizzaBase   = 222
	fxPittaDough  = 223

	fxSwampTar        = 230
	fxRawSwampPaste   = 231
	fxPieDish         = 232
	fxPieShell        = 233
	fxUncookedMeatPie = 234
	fxCookedMeat      = 111
	fxCookedChicken   = 114

	fxHalfBakedBowl  = 240
	fxChocolateBar   = 241
	fxEquaLeaves     = 242
	fxGnomebowlMould = 245
	fxChocolateBomb  = 246

	fxGrapes      = 300
	fxUnfermented = 301
	fxWine        = 302
	fxBadWine     = 303

	fxKnife = 946
)

// ---- fake item definitions ----

type fakeItemDefs struct {
	stackable map[int]bool
	names     map[int]string
	members   map[int]bool
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
func (f fakeItemDefs) IsMembersItem(id int) bool    { return f.members[id] }

func testItemDefs() fakeItemDefs {
	return fakeItemDefs{
		stackable: map[int]bool{fxSwampTar: true},
		names: map[int]string{
			fxRawShrimp: "Raw shrimps", fxShrimp: "Shrimps",
			fxRawSwampPaste: "Raw swamp paste", fxPieShell: "Pie shell",
			fxUncookedMeatPie: "Uncooked meat pie",
			fxBreadDough:      "Bread dough", fxPastryDough: "Pastry dough",
			fxPizzaBase: "Pizza base", fxPittaDough: "Uncooked pitta bread",
			ChocolateBombBowl: "Half made bowl", fxChocolateBomb: "Chocolate bomb",
			fxHalfBakedBowl: "Half baked bowl",
		},
	}
}

// ---- fake session ----
// Implements game.SessionPort plus the ChatMessageSender,
// varbit and CreationMenuSender extensions, capturing what
// content sends. A nil session drops everything; this fake
// lets tests observe menus, messages and varbits.

type fakeSession struct {
	messages []string
	spam     []string
	varbits  map[int]int
	menus    []*game.CreationMenu
}

func (s *fakeSession) SendPacket(packet []byte) {}
func (s *fakeSession) SendRunEnergy(units int, running bool, weightKg int, staminaTicks int) {
}
func (s *fakeSession) SendDestination(x, y int) {}
func (s *fakeSession) SendVarp(id, value int)   {}
func (s *fakeSession) FlushTick(cycle int64) error {
	return nil
}
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

// ---- players, instances, ticks ----

func newTestPlayer(name string) *game.Player {
	p := game.NewPlayer(name, game.Location{X: 3200, Y: 3200, Z: 0})
	p.SetRegistered(true)
	p.SetItemDefinitions(testItemDefs())
	setTestLevel(p, game.SkillHitpoints, 10)
	return p
}

func setTestLevel(p *game.Player, skill, level int) {
	p.Skills().SetCurrentLevel(skill, level, true)
	p.Skills().SetMaxLevel(skill, level)
}

func setCooking(p *game.Player, level int) { setTestLevel(p, game.SkillCooking, level) }

// fixtureTables builds hand-built tables over the fake ids: two
// cookables (shrimps, facility guppy), the swamp-paste / pie /
// chocolate-bomb recipes, the doughs, one wine, and the churn
// data the loader tests already cover.
func fixtureTables() *Tables {
	paste := &Recipe{
		ProductID: fxRawSwampPaste, Level: 1,
		Ingredients: []Ingredient{{ID: fxFlour, Count: 1}, {ID: fxSwampTar, Count: 1}},
		Returns:     []Ingredient{{ID: fxPot, Count: 1}},
		Timing:      []int{1, 2},
	}
	shell := &Recipe{
		ProductID: fxPieShell, Level: 1,
		Ingredients: []Ingredient{{ID: fxPastryDough, Count: 1}, {ID: fxPieDish, Count: 1}},
		Timing:      []int{1, 2},
	}
	meatPie := &Recipe{
		ProductID: fxUncookedMeatPie, Level: 20, XP: 0,
		Ingredients: []Ingredient{{ID: fxCookedMeat, Count: 1}, {ID: fxPieShell, Count: 1}},
		Timing:      []int{1, 2},
	}
	bomb := &Recipe{
		ProductID: ChocolateBombBowl, Level: 42, XP: 50,
		Ingredients: []Ingredient{{ID: fxHalfBakedBowl, Count: 1}, {ID: fxChocolateBar, Count: 4}, {ID: fxEquaLeaves, Count: 1}},
		Returns:     []Ingredient{{ID: fxGnomebowlMould, Count: 1}},
		Timing:      []int{1, 2},
	}
	doughs := &DoughData{
		FlourID: fxFlour, PotID: fxPot,
		Waters: []DoughWater{
			{ID: fxBucketWater, EmptyID: fxBucketEmpty},
			{ID: fxBowlWater, EmptyID: fxBowlEmpty},
			{ID: fxJugWater, EmptyID: fxJugEmpty},
		},
		Products: []DoughProduct{
			{ProductID: fxBreadDough, Level: 1, Message: "You mix the water and flour to make some bread dough."},
			{ProductID: fxPastryDough, Level: 1, Message: "You mix the water and flour to make some pastry dough."},
			{ProductID: fxPizzaBase, Level: 35, Message: "You mix the water and flour to make a pizza base."},
			{ProductID: fxPittaDough, Level: 58, Message: "You mix the water and flour to make some pitta dough."},
		},
	}
	return &Tables{
		CookablesByRaw: map[int]*Cookable{
			fxRawShrimp: {RawID: fxRawShrimp, CookedID: fxShrimp, BurntID: fxBurntShrimp, Level: 1, XP: 30,
				Chance: map[string][2]int{"fire": {128, 512}, "range": {128, 512}}, Name: "shrimps", Article: "some"},
			fxRawGuppy: {RawID: fxRawGuppy, CookedID: fxGuppy, BurntID: fxRuinedGuppy, Level: 7, XP: 12,
				StopBurn: 42, HasStopBurn: true, Facility: "camdozaal_preparation_table", Name: "guppy"},
		},
		RecipesByIngredient: map[int][]*Recipe{
			fxFlour:         {paste},
			fxSwampTar:      {paste},
			fxPastryDough:   {shell},
			fxPieDish:       {shell},
			fxCookedMeat:    {meatPie},
			fxPieShell:      {meatPie},
			fxHalfBakedBowl: {bomb},
			fxChocolateBar:  {bomb},
			fxEquaLeaves:    {bomb},
		},
		RecipesByProduct: map[int]*Recipe{
			fxRawSwampPaste:   paste,
			fxPieShell:        shell,
			fxUncookedMeatPie: meatPie,
			ChocolateBombBowl: bomb,
		},
		Doughs: doughs,
		Wines: []*Wine{
			{GrapesID: fxGrapes, UnfermentedID: fxUnfermented, WineID: fxWine, BadID: fxBadWine,
				Level: 35, XP: 200, Chance: [2]int{48, 352}, PendingLimit: 60000, HasPendingLimit: true},
		},
		Churn: &ChurnData{
			Labels:            map[string]string{"pot_of_cream": "Cream"},
			LabelsByProductID: map[int]string{244: "Cream"},
			Inputs:            []ChurnInput{{InputID: 322, Name: "milk", ReturnID: fxBucketEmpty, HasReturn: true}},
			Products:          []ChurnProduct{{InputID: 322, ProductID: 244, Name: "cream", Level: 21, XP: 18, Ticks: 10, SeqID: 9001}},
		},
		ItemIDs: map[string]int{
			"raw_shrimp": fxRawShrimp, "shrimp": fxShrimp, "half_baked_bowl": fxHalfBakedBowl,
			"pot_flour": fxFlour, "pot_empty": fxPot,
			"bucket_water": fxBucketWater, "bucket_empty": fxBucketEmpty,
			"bowl_water": fxBowlWater, "bowl_empty": fxBowlEmpty,
			"jug_water": fxJugWater, "jug_empty": fxJugEmpty,
			"bread_dough": fxBreadDough, "pastry_dough": fxPastryDough,
			"pizza_base": fxPizzaBase, "uncooked_pitta_bread": fxPittaDough,
		},
		SeqIDs:    map[string]int{},
		VarbitIDs: map[string]int{"cooking_success_count": 7001, "busy": 7002},
		KnifeIDs:  []int{fxKnife},
	}
}

type emittedEvent struct {
	name    string
	payload any
}

// newTestCooking builds an instance over fixture tables with a
// capturing emit func and a fresh TaskManager.
func newTestCooking() (*cooking, *[]emittedEvent, *game.TaskManager) {
	var emitted []emittedEvent
	tm := game.NewTaskManager()
	c := newCooking(fixtureTables())
	c.emit = func(name string, payload any) {
		emitted = append(emitted, emittedEvent{name: name, payload: payload})
	}
	c.tasks = tm
	return c, &emitted, tm
}

// driveTicks runs the task manager n cycles.
func driveTicks(tm *game.TaskManager, n int) {
	for i := 1; i <= n; i++ {
		tm.Process(int64(i))
	}
}

// give adds amount of id to the player's inventory.
func give(p *game.Player, id, amount int) {
	p.Inventory().AddItem(game.NewItem(id, amount))
}

func countOf(p *game.Player, id int) int {
	if inv := p.Inventory(); inv != nil {
		return inv.GetAmount(id)
	}
	return 0
}
