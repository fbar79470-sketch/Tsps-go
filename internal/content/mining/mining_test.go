package mining

import (
	"testing"

	"tsps-go/internal/game"
	"tsps-go/internal/plugins"
)

// ---- fixtures ----

// fixtureTables returns DefaultTables with the loc gamevals,
// item names and varp id the loop tests need.
func fixtureTables() *Tables {
	t := DefaultTables()
	t.LocGamevalByID = map[int]string{
		10943: "copperrock1", 11161: "copperrock2",
		11360: "tinrock1", 11361: "tinrock2",
		11364: "ironrock1", 11365: "ironrock2",
		11366: "coalrock1", 11367: "coalrock2",
		11380: "gemrock1", 11381: "gemrock",
		11386: "sandstone", 11387: "granite",
		11388: "amethystrock1", 11393: "amethystrock_empty",
		58917: "leadrock1", 58918: "leadrock1_empty",
		17962: "area_sanguine_mine_minerocks_01",
		17963: "area_sanguine_mine_minerocks_02",
		17964: "area_sanguine_mine_minerocks_03",
		17965: "area_sanguine_mine_minerocks_04",
		11390: "rocks1", 11391: "rocks2",
		15410: "lunar_runestone_top", 16687: "big_essence_rock", 34773: "blankrunestone",
		51485: "varlamore_mining_rock", 51486: "varlamore_mining_rock_empty",
		51487: "varlamore_mining_rock02", 51488: "varlamore_mining_rock_empty02",
		51489: "varlamore_mining_rock03", 51490: "varlamore_mining_rock_empty03",
		51491: "varlamore_mining_rock04", 51492: "varlamore_mining_rock_empty04",
		51493: "varlamore_mining_special_rock",
		20410: "lotr_mine_wall_coal_l", 20440: "lotr_mine_wall_depleted_mid",
		20441: "lotr_mine_wall_depleted_l",
	}
	t.LocIDByGameval = map[string]int{}
	for id, name := range t.LocGamevalByID {
		t.LocIDByGameval[name] = id
	}
	t.ItemNameByID = map[int]string{
		434: "Clay", 436: "Copper ore", 438: "Tin ore", 440: "Iron ore",
		442: "Silver ore", 444: "Gold ore", 447: "Mithril ore",
		449: "Adamantite ore", 451: "Runite ore", 453: "Coal",
		668: "Blurite ore", 6971: "Sandstone (1kg)", 6973: "Sandstone (2kg)",
		6975: "Sandstone (5kg)", 6977: "Sandstone (10kg)",
		6979: "Granite (500g)", 6981: "Granite (2kg)", 6983: "Granite (5kg)",
		1625: "Uncut opal", 1627: "Uncut jade", 1629: "Uncut red topaz",
		1623: "Uncut sapphire", 1621: "Uncut emerald", 1619: "Uncut ruby",
		1617: "Uncut diamond", 21347: "Amethyst", 9632: "Daeyalt ore",
		7936: "Pure essence", 1436: "Rune essence", 27616: "Ancient essence",
		1761: "Soft clay", 31716: "Lead ore", 13356: "Lovakite ore",
		13571: "Volcanic sulphur", 31719: "Nickel ore", 31722: "Rubium splinters",
		34020: "Sunstone", 30846: "Infernal shale",
		25676: "Barronite shards", 25684: "Barronite deposit",
		29088: "Calcified deposit", 29381: "Blessed bone shards",
		1265: "Bronze pickaxe", 1267: "Iron pickaxe", 1269: "Steel pickaxe",
		1275: "Rune pickaxe", 11920: "Dragon pickaxe", 23680: "Crystal pickaxe",
		13243: "Infernal pickaxe", 23962: "Crystal shard",
		13104: "Varrock armour 1", 13105: "Varrock armour 2",
		13106: "Varrock armour 3", 13107: "Varrock armour 4",
		23943: "Elven signet", 25545: "Celestial signet",
		13421: "Saltpetre", 23442: "Clue geode (beginner)",
		23182: "Clue scroll (beginner)",
		20358: "Clue geode (easy)", 2677: "Clue scroll (easy)",
		20360: "Clue geode (medium)", 2801: "Clue scroll (medium)",
		20362: "Clue geode (hard)", 2722: "Clue scroll (hard)",
		20364: "Clue geode (elite)", 12073: "Clue scroll (elite)",
		21343: "Mining gloves", 21345: "Superior mining gloves", 21392: "Expert mining gloves",
		2568: "Amulet of glory (4)", 2572: "Amulet of glory",
		12253: "Gas mask", 3125: "Spade",
	}
	// Fix the colliding ids above (gas mask / prospector legs
	// share 12253 in the literal map — the last wins; set them
	// explicitly).
	t.ItemNameByID[12253] = "Gas mask"
	t.ItemNameByID[12013] = "Prospector helmet"
	t.ItemNameByID[12014] = "Prospector jacket"
	t.ItemNameByID[12015] = "Prospector legs"
	t.ItemNameByID[12016] = "Prospector boots"
	t.OresMinedVarpID = 4526
	return t
}

type emittedEvent struct {
	name    string
	payload any
}

func newTestMining(t *testing.T) (*mining, *[]emittedEvent) {
	t.Helper()
	var emitted []emittedEvent
	m := newMining(fixtureTables())
	m.emit = func(name string, payload any) {
		emitted = append(emitted, emittedEvent{name: name, payload: payload})
	}
	// Deterministic randomness by default; tests override.
	m.randFloat = func() float64 { return 0.5 }
	m.randInt = func(min, max int) int { return min }
	return m, &emitted
}

func newTestPlayer(name string, x, y int) *game.Player {
	p := game.NewPlayer(name, game.Location{X: x, Y: y, Z: 0})
	p.SetRegistered(true)
	return p
}

func setLevel(p *game.Player, skill, level int) {
	p.Skills().SetCurrentLevel(skill, level, true)
	p.Skills().SetMaxLevel(skill, level)
}

func rockByName(tables *Tables, name string) *Rock {
	return tables.RockByName(name)
}

// fakeObjectDef is a plugins.ObjectDefinition naming one object.
type fakeObjectDef struct {
	name string
}

func (d fakeObjectDef) ID() int                { return 0 }
func (d fakeObjectDef) Name() string           { return d.name }
func (d fakeObjectDef) SizeX() int             { return 1 }
func (d fakeObjectDef) SizeY() int             { return 1 }
func (d fakeObjectDef) BlockingMask() int      { return 0 }
func (d fakeObjectDef) Interactions() []string { return []string{"Mine"} }

// fakeObjects is a minimal in-memory plugins.ObjectManagerAPI.
type fakeObjects struct {
	byTile       map[game.Location][]plugins.GameObject
	mapOriginals map[game.Location][]plugins.GameObject
	removed      []plugins.GameObject
}

func newFakeObjects() *fakeObjects {
	return &fakeObjects{
		byTile:       make(map[game.Location][]plugins.GameObject),
		mapOriginals: make(map[game.Location][]plugins.GameObject),
	}
}

func (f *fakeObjects) placeMapOriginal(obj plugins.GameObject) {
	if obj == nil {
		return
	}
	f.mapOriginals[obj.Location()] = append(f.mapOriginals[obj.Location()], obj)
}

func (f *fakeObjects) removeFrom(list map[game.Location][]plugins.GameObject, obj plugins.GameObject) {
	loc := obj.Location()
	kept := list[loc][:0]
	for _, o := range list[loc] {
		if o != obj {
			kept = append(kept, o)
		}
	}
	list[loc] = kept
}

func (f *fakeObjects) Register(obj plugins.GameObject, playerUpdate bool) {
	if obj == nil {
		return
	}
	f.byTile[obj.Location()] = append(f.byTile[obj.Location()], obj)
}

func (f *fakeObjects) Deregister(obj plugins.GameObject, playerUpdate bool) {
	if obj == nil {
		return
	}
	f.removeFrom(f.byTile, obj)
	f.removeFrom(f.mapOriginals, obj)
}

func (f *fakeObjects) ReplaceInPlace(outgoing, incoming plugins.GameObject) {
	if outgoing == nil || incoming == nil {
		return
	}
	loc := outgoing.Location()
	f.removeFrom(f.mapOriginals, outgoing)
	for i, o := range f.byTile[loc] {
		if o == outgoing {
			f.byTile[loc][i] = incoming
			return
		}
	}
	f.byTile[loc] = append(f.byTile[loc], incoming)
}

func (f *fakeObjects) ObjectsAt(loc game.Location) []plugins.GameObject { return f.byTile[loc] }
func (f *fakeObjects) ObjectsInRegion(regionX, regionY int) []plugins.GameObject {
	return nil
}
func (f *fakeObjects) Exists(id int, loc game.Location) bool { return len(f.byTile[loc]) > 0 }

func (f *fakeObjects) Get(id int, loc game.Location) plugins.GameObject {
	for _, o := range f.mapOriginals[loc] {
		if o.ID() == id {
			return o
		}
	}
	for _, o := range f.byTile[loc] {
		if o.ID() == id {
			return o
		}
	}
	return nil
}

func (f *fakeObjects) Find(player *game.Player, id int, loc game.Location) plugins.GameObject {
	for _, o := range f.byTile[loc] {
		if o.ID() == id {
			return o
		}
	}
	return nil
}

func (f *fakeObjects) StillStands(player *game.Player, id int, loc game.Location, obj plugins.GameObject) bool {
	if f.Find(player, id, loc) == obj {
		return true
	}
	for _, o := range f.mapOriginals[loc] {
		if o == obj {
			return true
		}
	}
	return false
}

func (f *fakeObjects) RemovePluginObjects() int { return 0 }
func (f *fakeObjects) RemovedObjects() []plugins.GameObject {
	return f.removed
}

// placeRock stands a rock as a map original at (3201, 3200, 0),
// next to the test player's spawn (3200, 3200, 0).
func placeRock(objects *fakeObjects, rockID int) *game.GameObject {
	obj := game.NewGameObject(rockID, game.Location{X: 3201, Y: 3200, Z: 0}, 10, 0, nil)
	objects.placeMapOriginal(obj)
	return obj
}

// givePickaxe puts a bronze pickaxe in the player's inventory.
func givePickaxe(p *game.Player, id int) {
	p.Inventory().Add(game.NewItem(id, 1), true)
}

// ---- success formula ----

func TestSuccessNumerator(t *testing.T) {
	cases := []struct {
		chart [2]int
		level int
		want  int
	}{
		// Copper [100, 350]: L1 → 1+floor(100+0+0.5) = 101;
		// L99 → 1+floor(0+350+0.5) = 351 → capped 256.
		{[2]int{100, 350}, 1, 101},
		{[2]int{100, 350}, 99, 256},
		// Iron [96, 350] at 15: 1+floor(82.28+50+0.5) = 133.
		{[2]int{96, 350}, 15, 133},
		// Runite [1, 18] at 85: 1+floor(0.14+15.55+0.5) = 17.
		{[2]int{1, 18}, 85, 17},
		// Amethyst [-18, 10] at 92: 1+floor(-1.28+9.39+0.5) = 9.
		{[2]int{-18, 10}, 92, 9},
		// Amethyst at 99: 1+floor(0+10+0.5) = 11.
		{[2]int{-18, 10}, 99, 11},
	}
	for _, c := range cases {
		if got := successNumerator(c.chart, c.level); got != c.want {
			t.Errorf("successNumerator(%v, %d) = %d, want %d", c.chart, c.level, got, c.want)
		}
	}
}

// ---- pickaxes ----

func TestFindBestPickaxe(t *testing.T) {
	m, _ := newTestMining(t)
	p := newTestPlayer("miner", 3200, 3200)
	setLevel(p, game.SkillMining, 1)
	if got := m.findBestPickaxe(p); got != nil {
		t.Errorf("no pickaxe: got %v", got)
	}
	givePickaxe(p, 1265) // bronze
	givePickaxe(p, 1267) // iron
	if got := m.findBestPickaxe(p); got == nil || got.ID != 1267 {
		t.Errorf("bronze+iron: got %v, want iron 1267", got)
	}
	// A rune pickaxe the player cannot use is skipped.
	givePickaxe(p, 1275)
	if got := m.findBestPickaxe(p); got == nil || got.ID != 1267 {
		t.Errorf("rune too high: got %v, want iron 1267", got)
	}
	// At 41 the rune pickaxe wins.
	setLevel(p, game.SkillMining, 41)
	if got := m.findBestPickaxe(p); got == nil || got.ID != 1275 {
		t.Errorf("level 41: got %v, want rune 1275", got)
	}
	// A wielded rune pickaxe beats a carried dragon pickaxe the
	// player cannot use (level 41 < 61).
	setLevel(p, game.SkillMining, 41)
	p.Equipment().Set(game.WeaponSlot, game.NewItem(1275, 1))
	givePickaxe(p, 11920)
	if got := m.findBestPickaxe(p); got == nil || got.ID != 1275 {
		t.Errorf("wielded rune: got %v, want rune 1275", got)
	}
	// Crystal needs 50 Agility as well as 71 Mining. The
	// dragon pickaxe (given earlier, level 61) is the best
	// usable one without it.
	setLevel(p, game.SkillMining, 71)
	setLevel(p, game.SkillAgility, 1)
	givePickaxe(p, 23680)
	if got := m.findBestPickaxe(p); got == nil || got.ID != 11920 {
		t.Errorf("crystal without agility: got %v, want dragon 11920", got)
	}
	setLevel(p, game.SkillAgility, 50)
	if got := m.findBestPickaxe(p); got == nil || got.ID != 23680 {
		t.Errorf("crystal with agility: got %v, want crystal 23680", got)
	}
}

func TestRollIntervalFastRoll(t *testing.T) {
	m, _ := newTestMining(t)
	var dragon Pickaxe
	for _, p := range m.tables.Pickaxes {
		if p.ID == 11920 {
			dragon = p
		}
	}
	// With rand 0.5 the fast roll never fires (1/6).
	if got := m.rollInterval(dragon); got != 3 {
		t.Errorf("dragon interval (no fast) = %d, want 3", got)
	}
	// With rand 0.0 it always fires.
	m.randFloat = func() float64 { return 0.0 }
	if got := m.rollInterval(dragon); got != 2 {
		t.Errorf("dragon interval (fast) = %d, want 2", got)
	}
	// Bronze has no fast roll.
	bronze := m.tables.Pickaxes[0]
	if got := m.rollInterval(bronze); got != 8 {
		t.Errorf("bronze interval = %d, want 8", got)
	}
}

// ---- outcomes / cascade / yield ----

func TestRollOutcomeWeights(t *testing.T) {
	m, _ := newTestMining(t)
	barronite := rockByName(m.tables, "Barronite rocks")
	// rand 0.5 * 100 = 50 < 76 → shards.
	m.randFloat = func() float64 { return 0.5 }
	if got := m.rollOutcome(barronite); got == nil || got.ItemID != 25676 {
		t.Errorf("barronite roll 0.5: got %v, want shards 25676", got)
	}
	// rand 0.9 * 100 = 90 >= 76 → deposit.
	m.randFloat = func() float64 { return 0.9 }
	if got := m.rollOutcome(barronite); got == nil || got.ItemID != 25684 {
		t.Errorf("barronite roll 0.9: got %v, want deposit 25684", got)
	}
}

func TestRollCascade(t *testing.T) {
	m, _ := newTestMining(t)
	sandstone := rockByName(m.tables, "Sandstone rocks")
	// Level 99 with rand 0.0: every cascade step succeeds; the
	// first (10kg) wins.
	m.randFloat = func() float64 { return 0.0 }
	if got := m.rollCascade(sandstone, 99); got == nil || got.ItemID != 6977 {
		t.Errorf("sandstone L99 cascade: got %v, want 6977", got)
	}
	// Level 35 with rand 0.99: the 10kg roll (numerator
	// 1+floor(4*64/98+50*34/98+0.5) = 1+floor(2.61+17.35+0.5) =
	// 20) fails (0.99*256=253 >= 20); 5kg (1+floor(8*64/98+
	// 75*34/98+0.5) = 1+floor(5.22+26.02+0.5) = 32) fails;
	// 2kg (1+floor(16*64/98+100*34/98+0.5) = 1+floor(10.45+
	// 34.69+0.5) = 46) fails; 1kg (1+floor(25*64/98+200*34/98
	// +0.5) = 1+floor(16.33+69.39+0.5) = 87) fails → nothing.
	m.randFloat = func() float64 { return 0.99 }
	if got := m.rollCascade(sandstone, 35); got != nil {
		t.Errorf("sandstone L35 rand 0.99: got %v, want nil", got)
	}
}

func TestRockHoldsYield(t *testing.T) {
	m, _ := newTestMining(t)
	objects := newFakeObjects()
	rock := placeRock(objects, 58917) // lead rock stand-in
	nickel := rockByName(m.tables, "Nickel rocks")
	// Nickel draws min 2, max 16; the fixture randInt returns
	// min, so total = 2: the first ore holds, the second goes.
	if !m.rockHolds(rock, nickel) {
		t.Error("nickel first ore: want holds")
	}
	if m.rockHolds(rock, nickel) {
		t.Error("nickel second ore: want deplete")
	}
	// Rubium: min 2, then 1/6 per ore. With rand 0.5 the chance
	// never fires, so it holds past min.
	rubium := rockByName(m.tables, "Rubium rocks")
	rock2 := placeRock(objects, 58919)
	if !m.rockHolds(rock2, rubium) || !m.rockHolds(rock2, rubium) || !m.rockHolds(rock2, rubium) {
		t.Error("rubium: want holds while the 1/6 roll misses")
	}
	m.randFloat = func() float64 { return 0.0 }
	if m.rockHolds(rock2, rubium) {
		t.Error("rubium: want deplete when the 1/6 roll hits")
	}
}

func TestGloveSaveCount(t *testing.T) {
	m, _ := newTestMining(t)
	p := newTestPlayer("miner", 3200, 3200)
	copper := rockByName(m.tables, "Copper rocks")
	mithril := rockByName(m.tables, "Mithril rocks")
	runite := rockByName(m.tables, "Runite rocks")
	if got := m.gloveSaveCount(p, copper); got != 0 {
		t.Errorf("no gloves: got %d, want 0", got)
	}
	p.Equipment().Set(game.HandsSlot, game.NewItem(21343, 1)) // mining gloves
	if got := m.gloveSaveCount(p, copper); got != 1 {
		t.Errorf("mining gloves on copper: got %d, want 1", got)
	}
	if got := m.gloveSaveCount(p, mithril); got != 0 {
		t.Errorf("mining gloves on mithril: got %d, want 0", got)
	}
	p.Equipment().Set(game.HandsSlot, game.NewItem(21345, 1)) // superior
	if got := m.gloveSaveCount(p, mithril); got != 1 {
		t.Errorf("superior gloves on mithril: got %d, want 1", got)
	}
	if got := m.gloveSaveCount(p, runite); got != 0 {
		t.Errorf("superior gloves on runite: got %d, want 0", got)
	}
	p.Equipment().Set(game.HandsSlot, game.NewItem(21392, 1)) // expert
	if got := m.gloveSaveCount(p, runite); got != 1 {
		t.Errorf("expert gloves on runite: got %d, want 1", got)
	}
	if got := m.gloveSaveCount(p, copper); got != 3 {
		t.Errorf("expert gloves on copper: got %d, want 3", got)
	}
	// Free worlds get no glove saves.
	m.membersWorld = false
	if got := m.gloveSaveCount(p, copper); got != 0 {
		t.Errorf("free world: got %d, want 0", got)
	}
}

// ---- startMining gates ----

func TestStartMiningGates(t *testing.T) {
	m, _ := newTestMining(t)
	objects := newFakeObjects()
	m.objects = objects
	p := newTestPlayer("miner", 3200, 3200)
	setLevel(p, game.SkillMining, 1)
	copper := rockByName(m.tables, "Copper rocks")
	rock := placeRock(objects, 10943)
	// No pickaxe → false.
	if m.startMining(p, rock, copper) {
		t.Error("no pickaxe: want false")
	}
	// Bronze pickaxe → true.
	givePickaxe(p, 1265)
	if !m.startMining(p, rock, copper) {
		t.Error("bronze pickaxe: want true")
	}
	// Iron needs 15.
	m.stopMining(p, false)
	iron := rockByName(m.tables, "Iron rocks")
	ironRock := placeRock(objects, 11364)
	if m.startMining(p, ironRock, iron) {
		t.Error("iron at level 1: want false")
	}
	setLevel(p, game.SkillMining, 15)
	if !m.startMining(p, ironRock, iron) {
		t.Error("iron at level 15: want true")
	}
	// Sulphur needs a breathing filter.
	m.stopMining(p, false)
	setLevel(p, game.SkillMining, 42)
	sulphur := rockByName(m.tables, "Volcanic sulphur")
	sulphurRock := placeRock(objects, 28496)
	if m.startMining(p, sulphurRock, sulphur) {
		t.Error("sulphur without filter: want false")
	}
	p.Equipment().Set(game.HeadSlot, game.NewItem(12253, 1)) // gas mask
	if !m.startMining(p, sulphurRock, sulphur) {
		t.Error("sulphur with gas mask: want true")
	}
}

// ---- the tick loop: copper mine, deplete, respawn ----

func TestCopperMineDepleteRespawn(t *testing.T) {
	m, emitted := newTestMining(t)
	objects := newFakeObjects()
	m.objects = objects
	tm := game.NewTaskManager()
	m.tasks = tm
	p := newTestPlayer("miner", 3200, 3200)
	setLevel(p, game.SkillMining, 1)
	givePickaxe(p, 1265)
	copper := rockByName(m.tables, "Copper rocks")
	rock := placeRock(objects, 10943)
	// Force every ore roll to succeed while missing the 1/256
	// gem pre-roll: 0.1*256 = 25.6 >= 1 (no gem) and < 101
	// (copper success at level 1).
	m.randFloat = func() float64 { return 0.1 }
	if !m.startMining(p, rock, copper) {
		t.Fatal("startMining failed")
	}
	// Tick 1: swing (tick+1). Tick 2: first roll → copper ore.
	m.tick = 1
	m.processMiningTick(1)
	m.tick = 2
	m.processMiningTick(2)
	if got := p.Inventory().GetAmount(436); got != 1 {
		t.Fatalf("after first roll: copper ore = %d, want 1", got)
	}
	if got := p.Skills().CurrentLevel(game.SkillMining); got != 1 {
		t.Fatalf("mining level = %d, want 1 (XP truncates 17.5 → 17, no level)", got)
	}
	// The counter advanced and the success event fired.
	if v, ok := p.GetAttribute(OreMinedAttribute).(int); !ok || v != 1 {
		t.Errorf("ores-mined = %v, want 1", p.GetAttribute(OreMinedAttribute))
	}
	foundSuccess := false
	for _, e := range *emitted {
		if e.name == "mining:success" {
			foundSuccess = true
		}
	}
	if !foundSuccess {
		t.Error("mining:success was not emitted")
	}
	// A copper rock holds one ore: the next roll (tick 2 +
	// bronze interval 8) depletes it.
	m.tick = 10
	m.processMiningTick(10)
	if _, ok := m.sessions[p]; ok {
		t.Error("session still active after depletion")
	}
	if got := objects.Get(11390, rock.Location()); got == nil {
		t.Error("depleted rock rocks1 not in place")
	}
	if got := objects.Get(10943, rock.Location()); got != nil {
		t.Error("live copper rock still stands after depletion")
	}
	// The respawn task restores it after 4 ticks.
	for cycle := int64(1); cycle <= 4; cycle++ {
		tm.Process(cycle)
	}
	if got := objects.Get(10943, rock.Location()); got == nil {
		t.Error("copper rock did not respawn after 4 ticks")
	}
	if got := objects.Get(11390, rock.Location()); got != nil {
		t.Error("depleted rock still stands after respawn")
	}
}

// TestFullInventoryRefusal pins the full-inventory gate: a full
// inventory with no stackable room refuses to start.
func TestFullInventoryRefusal(t *testing.T) {
	m, _ := newTestMining(t)
	objects := newFakeObjects()
	m.objects = objects
	p := newTestPlayer("miner", 3200, 3200)
	setLevel(p, game.SkillMining, 1)
	givePickaxe(p, 1265)
	// Fill the inventory with junk (28 slots).
	for i := 0; i < 28; i++ {
		p.Inventory().Add(game.NewItem(2000+i, 1), true)
	}
	if !p.Inventory().IsFull() {
		t.Skip("inventory did not fill (capacity differs)")
	}
	copper := rockByName(m.tables, "Copper rocks")
	rock := placeRock(objects, 10943)
	if m.startMining(p, rock, copper) {
		t.Error("full inventory: want false")
	}
}

// TestWalkAwayStops pins the movement stop: a session ends when
// the player starts moving.
func TestWalkAwayStops(t *testing.T) {
	m, _ := newTestMining(t)
	objects := newFakeObjects()
	m.objects = objects
	p := newTestPlayer("miner", 3200, 3200)
	setLevel(p, game.SkillMining, 1)
	givePickaxe(p, 1265)
	copper := rockByName(m.tables, "Copper rocks")
	rock := placeRock(objects, 10943)
	if !m.startMining(p, rock, copper) {
		t.Fatal("startMining failed")
	}
	// Queue a walk step: the movement queue is non-empty.
	p.MovementQueue().AddCheckpoint(game.Location{X: 3201, Y: 3200, Z: 0})
	m.tick = 3
	m.processMiningTick(3)
	if _, ok := m.sessions[p]; ok {
		t.Error("session still active while moving")
	}
}

// TestMineDepletedRock pins Mine on a mined-out rock: sound +
// the no-ore line, no session.
func TestMineDepletedRock(t *testing.T) {
	m, _ := newTestMining(t)
	objects := newFakeObjects()
	m.objects = objects
	p := newTestPlayer("miner", 3200, 3200)
	depleted := game.NewGameObject(11390, game.Location{X: 3201, Y: 3200, Z: 0}, 10, 0, nil)
	event := &plugins.ObjectInteractionEvent{
		Player: p, Object: depleted, Definition: fakeObjectDef{name: "Rocks"},
		ObjectID: 11390, Option: "Mine", HasOption: true,
		Location: depleted.Location(),
	}
	if !m.mineDepletedRock(event) {
		t.Error("Mine on rocks1: want handled")
	}
	if _, ok := m.sessions[p]; ok {
		t.Error("session started on a depleted rock")
	}
	// A live rock is not depleted.
	live := game.NewGameObject(10943, game.Location{X: 3201, Y: 3200, Z: 0}, 10, 0, nil)
	event2 := &plugins.ObjectInteractionEvent{
		Player: p, Object: live, Definition: fakeObjectDef{name: "Copper rocks"},
		ObjectID: 10943, Option: "Mine", HasOption: true,
		Location: live.Location(),
	}
	if m.mineDepletedRock(event2) {
		t.Error("Mine on a live rock: want not handled by the depleted handler")
	}
}
