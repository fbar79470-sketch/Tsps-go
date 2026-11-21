package mining

import (
	"math"
	"testing"

	"tsps-go/internal/game"
	"tsps-go/internal/plugins"
)

// testSession captures game messages and sounds for mining
// tests (a SessionPort with the chat and sound extensions).
type testSession struct {
	gameMessages []string
	sounds       []int
}

func (s *testSession) SendPacket(packet []byte) {}
func (s *testSession) SendRunEnergy(units int, running bool, weightKg int, staminaTicks int) {
}
func (s *testSession) SendDestination(x, y int)      {}
func (s *testSession) SendVarp(id, value int)        {}
func (s *testSession) FlushTick(cycle int64) error   { return nil }
func (s *testSession) Close(code int, reason string) {}
func (s *testSession) IsOpen() bool                  { return true }
func (s *testSession) SendGameMessage(message string) {
	s.gameMessages = append(s.gameMessages, message)
}
func (s *testSession) SendTypedGameMessage(chatType int, message, from string) {}
func (s *testSession) SendSoundEffect(soundID, loopType, delay, volume int) {
	s.sounds = append(s.sounds, soundID)
}
func (s *testSession) SendAreaSound(soundID, x, y, level, loops, delay, radius, attenuation int) {
}

var (
	_ game.SessionPort       = (*testSession)(nil)
	_ game.ChatMessageSender = (*testSession)(nil)
	_ game.SoundSender       = (*testSession)(nil)
)

// chargedCrystalPickaxe returns a crystal pickaxe item with the
// given saved charges.
func chargedCrystalPickaxe(id, charges int) game.Item {
	item := game.NewItem(id, 1)
	item.SetMetaValue(crystalPickaxeMetaKey, chargesMetaValue(charges))
	return item
}

// ---- crystal pickaxe ----

func TestCrystalPickaxeCharges(t *testing.T) {
	cases := []struct {
		name string
		item game.Item
		want int
	}{
		{"saved 5000", chargedCrystalPickaxe(23680, 5000), 5000},
		{"saved 1", chargedCrystalPickaxe(23680, 1), 1},
		{"saved 0", chargedCrystalPickaxe(23680, 0), 0},
		{"saved over max clamps", chargedCrystalPickaxe(23680, 25000), 20000},
		{"saved negative clamps", chargedCrystalPickaxe(23680, -5), 0},
		{"no meta charged is START", game.NewItem(23680, 1), 10000},
		{"no meta gauntlet is START", game.NewItem(23863, 1), 10000},
		{"no meta uncharged is 0", game.NewItem(23682, 1), 0},
		{"no meta other item is 0", game.NewItem(995, 1), 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := crystalPickaxeCharges(c.item); got != c.want {
				t.Fatalf("crystalPickaxeCharges = %d, want %d", got, c.want)
			}
		})
	}
}

func TestUseCrystalPickaxeCharge(t *testing.T) {
	p := newTestPlayer("miner", 3200, 3200)
	session := &testSession{}
	p.Session = session
	p.Inventory().Add(chargedCrystalPickaxe(23680, 5), true)
	loc, ok := findChargedCrystalPickaxe(p)
	if !ok {
		t.Fatal("charged crystal pickaxe not found")
	}
	useCrystalPickaxeCharge(p, loc)
	if got := crystalPickaxeCharges(p.Inventory().ItemAt(0)); got != 4 {
		t.Errorf("after one use: charges = %d, want 4", got)
	}
	// The last charge turns it into the inactive form.
	p.Inventory().SetItem(0, chargedCrystalPickaxe(23680, 1))
	loc, _ = findChargedCrystalPickaxe(p)
	useCrystalPickaxeCharge(p, loc)
	if got := p.Inventory().ItemAt(0).ID(); got != 23682 {
		t.Errorf("after last charge: id = %d, want inactive 23682", got)
	}
	found := false
	for _, msg := range session.gameMessages {
		if msg == "Your crystal pickaxe has run out of charges." {
			found = true
		}
	}
	if !found {
		t.Errorf("run-out message missing; got %v", session.gameMessages)
	}
}

func TestTryUseCrystalChargeSignet(t *testing.T) {
	m, _ := newTestMining(t)
	p := newTestPlayer("miner", 3200, 3200)
	p.Inventory().Add(chargedCrystalPickaxe(23680, 100), true)
	// No signet: always drains.
	m.randFloat = func() float64 { return 0.0 }
	if !m.tryUseCrystalCharge(p) {
		t.Error("no signet: want charge used")
	}
	if got := crystalPickaxeCharges(p.Inventory().ItemAt(0)); got != 99 {
		t.Errorf("no signet: charges = %d, want 99", got)
	}
	// Signet worn, rand 0.0 < 0.10: saved.
	p.Equipment().Set(game.RingSlot, game.NewItem(23943, 1))
	if m.tryUseCrystalCharge(p) {
		t.Error("signet save: want false")
	}
	if got := crystalPickaxeCharges(p.Inventory().ItemAt(0)); got != 99 {
		t.Errorf("signet save: charges = %d, want 99", got)
	}
	// Signet worn, rand 0.5: drains.
	m.randFloat = func() float64 { return 0.5 }
	if !m.tryUseCrystalCharge(p) {
		t.Error("signet no save: want charge used")
	}
	if got := crystalPickaxeCharges(p.Inventory().ItemAt(0)); got != 98 {
		t.Errorf("signet no save: charges = %d, want 98", got)
	}
}

func TestCheckCrystalPickaxeCharges(t *testing.T) {
	m, _ := newTestMining(t)
	p := newTestPlayer("miner", 3200, 3200)
	session := &testSession{}
	p.Session = session
	p.Inventory().Add(chargedCrystalPickaxe(23680, 5000), true)
	event := &plugins.PluginItemActionEvent{
		Player: p, ItemID: 23680, Slot: 0, Option: "Check",
	}
	if !m.checkCrystalPickaxeCharges(event) {
		t.Fatal("Check not handled")
	}
	if len(session.gameMessages) != 1 || session.gameMessages[0] != "Your crystal pickaxe has 5,000 charges left." {
		t.Errorf("check message = %v, want the 5,000-charges line", session.gameMessages)
	}
}

func TestAddCrystalShardCharges(t *testing.T) {
	m, _ := newTestMining(t)
	p := newTestPlayer("miner", 3200, 3200)
	session := &testSession{}
	p.Session = session
	// Uncharged pickaxe + 10 shards → charged, 1,000 charges.
	p.Inventory().Add(game.NewItem(23682, 1), true)
	p.Inventory().Add(game.NewItem(23962, 10), true)
	event := &plugins.PluginItemOnItemEvent{
		Player:   p,
		UsedItem: game.NewItem(23962, 1), UsedItemID: 23962, UsedItemSlot: 1,
		UsedWithItem: game.NewItem(23682, 1), UsedWithItemID: 23682, UsedWithItemSlot: 0,
	}
	if !m.addCrystalShardCharges(event) {
		t.Fatal("shard recharge not handled")
	}
	if got := p.Inventory().ItemAt(0).ID(); got != 23680 {
		t.Errorf("after recharge: id = %d, want charged 23680", got)
	}
	if got := crystalPickaxeCharges(p.Inventory().ItemAt(0)); got != 1000 {
		t.Errorf("after recharge: charges = %d, want 1000", got)
	}
	if got := p.Inventory().GetAmount(23962); got != 0 {
		t.Errorf("shards left = %d, want 0", got)
	}
	// A full pickaxe refuses more shards.
	p.Inventory().SetItem(0, chargedCrystalPickaxe(23680, 20000))
	p.Inventory().Add(game.NewItem(23962, 5), true)
	event2 := &plugins.PluginItemOnItemEvent{
		Player:   p,
		UsedItem: game.NewItem(23962, 1), UsedItemID: 23962, UsedItemSlot: 1,
		UsedWithItem: chargedCrystalPickaxe(23680, 20000), UsedWithItemID: 23680, UsedWithItemSlot: 0,
	}
	if !m.addCrystalShardCharges(event2) {
		t.Fatal("full-pickaxe recharge not handled")
	}
	if got := p.Inventory().GetAmount(23962); got != 5 {
		t.Errorf("full pickaxe: shards = %d, want 5 (untouched)", got)
	}
}

// ---- infernal pickaxe ----

func TestInfernalCharges(t *testing.T) {
	cases := []struct {
		name string
		item game.Item
		want int
	}{
		{"saved 100", func() game.Item {
			it := game.NewItem(13243, 1)
			it.SetMetaValue(infernalMetaKey, chargesMetaValue(100))
			return it
		}(), 100},
		{"saved over max clamps", func() game.Item {
			it := game.NewItem(13243, 1)
			it.SetMetaValue(infernalMetaKey, chargesMetaValue(9000))
			return it
		}(), 5000},
		{"no meta charged is MAX", game.NewItem(13243, 1), 5000},
		{"no meta uncharged is MAX (no save yet)", game.NewItem(13244, 1), 5000},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := infernalCharges(c.item); got != c.want {
				t.Fatalf("infernalCharges = %d, want %d", got, c.want)
			}
		})
	}
}

func TestTryCombustOre(t *testing.T) {
	m, _ := newTestMining(t)
	p := newTestPlayer("miner", 3200, 3200)
	setLevel(p, game.SkillMining, 1)
	// Wield a charged infernal pickaxe.
	it := game.NewItem(13243, 1)
	it.SetMetaValue(infernalMetaKey, chargesMetaValue(5000))
	p.Equipment().Set(game.WeaponSlot, it)
	// rand 0.0 < 1/3: combusts copper (3.6 XP → 3 int).
	m.randFloat = func() float64 { return 0.0 }
	if !m.tryCombustOre(p, 13243, 436) {
		t.Fatal("copper combust: want true")
	}
	if got := p.Skills().Experience(game.SkillSmithing); got != 3 {
		t.Errorf("smithing XP = %d, want 3 (3.6 truncated)", got)
	}
	loc, _ := findChargedInfernalPickaxe(p, 13243)
	if got := infernalCharges(loc.item); got != 4999 {
		t.Errorf("charges after combust = %d, want 4999", got)
	}
	// rand 0.5: no combust.
	m.randFloat = func() float64 { return 0.5 }
	if m.tryCombustOre(p, 13243, 436) {
		t.Error("rand 0.5: want no combust")
	}
	// A non-infernal pickaxe never combusts.
	if m.tryCombustOre(p, 1275, 436) {
		t.Error("rune pickaxe: want no combust")
	}
	// Sandstone 2kg has no entry: never combusts (1kg/5kg/
	// 10kg do — 10.2/19.2/28.2).
	m.randFloat = func() float64 { return 0.0 }
	if m.tryCombustOre(p, 13243, 6973) {
		t.Error("sandstone 2kg: want no combust (no XP entry)")
	}
}

func TestHandleInfernalCreation(t *testing.T) {
	m, _ := newTestMining(t)
	p := newTestPlayer("miner", 3200, 3200)
	session := &testSession{}
	p.Session = session
	// Below 85 Smithing: refused, nothing changes.
	setLevel(p, game.SkillSmithing, 84)
	p.Inventory().Add(game.NewItem(13233, 1), true) // smouldering stone
	p.Inventory().Add(game.NewItem(11920, 1), true) // dragon pickaxe
	stone := p.Inventory().ItemAt(0)
	dragon := p.Inventory().ItemAt(1)
	if !m.handleInfernalCreation(p, stone, dragon, 1) {
		t.Error("below 85: want handled (refusal message)")
	}
	if got := p.Inventory().ItemAt(1).ID(); got != 11920 {
		t.Errorf("below 85: pickaxe = %d, want dragon 11920", got)
	}
	// At 85: creation consumes the stone, transforms the
	// pickaxe, grants +200 Mining / +350 Smithing XP.
	setLevel(p, game.SkillSmithing, 85)
	setLevel(p, game.SkillMining, 1)
	stone = p.Inventory().ItemAt(0)
	dragon = p.Inventory().ItemAt(1)
	if !m.handleInfernalCreation(p, stone, dragon, 1) {
		t.Error("creation: want handled")
	}
	if got := p.Inventory().GetAmount(13233); got != 0 {
		t.Errorf("stone left = %d, want 0", got)
	}
	if got := p.Inventory().ItemAt(1).ID(); got != 13243 {
		t.Errorf("pickaxe = %d, want infernal 13243", got)
	}
	if got := infernalCharges(p.Inventory().ItemAt(1)); got != 5000 {
		t.Errorf("charges = %d, want 5000", got)
	}
	if got := p.Skills().Experience(game.SkillMining); got != 200 {
		t.Errorf("mining XP = %d, want 200", got)
	}
	if got := p.Skills().Experience(game.SkillSmithing); got != 350 {
		t.Errorf("smithing XP = %d, want 350", got)
	}
}

// ---- clue geodes ----

func TestGeodeChance(t *testing.T) {
	// petBase 1000, level 1: floor(700) * round(mod*10) / 1010.
	cases := []struct {
		modifier float64
		want     int
	}{
		{0.2, 1},   // beginner: 700*2/1010 = 1
		{1.7, 11},  // easy: 700*17/1010 = 11
		{2.0, 13},  // medium: 700*20/1010 = 13
		{3.3, 22},  // hard: 700*33/1010 = 22
		{10.0, 69}, // elite: 700*100/1010 = 69
	}
	for _, c := range cases {
		if got := geodeChance(1000, 1, c.modifier, false); got != c.want {
			t.Errorf("geodeChance(1000, 1, %v) = %d, want %d", c.modifier, got, c.want)
		}
	}
	// The combat-achievements factor (0.95) applies to the
	// petBase term first: 700 -> 665, then 665*17/1010 = 11.
	if got := geodeChance(1000, 1, 1.7, true); got != 11 {
		t.Errorf("geodeChance with CA = %d, want 11", got)
	}
}

func TestOwnsClue(t *testing.T) {
	m, _ := newTestMining(t)
	p := newTestPlayer("miner", 3200, 3200)
	tier := clueGeodeTiers[0] // beginner
	if m.ownsClue(p, tier) {
		t.Error("no clue: want false")
	}
	p.Inventory().Add(game.NewItem(23182, 1), true) // beginner scroll
	if !m.ownsClue(p, tier) {
		t.Error("beginner scroll in inventory: want true")
	}
	// An easy scroll does not count for the beginner tier.
	p2 := newTestPlayer("miner2", 3200, 3200)
	p2.Inventory().Add(game.NewItem(2677, 1), true) // easy scroll
	if m.ownsClue(p2, tier) {
		t.Error("easy scroll: want false for beginner tier")
	}
}

func TestOpenClueGeode(t *testing.T) {
	m, _ := newTestMining(t)
	p := newTestPlayer("miner", 3200, 3200)
	session := &testSession{}
	p.Session = session
	p.Inventory().Add(game.NewItem(23442, 1), true) // beginner geode
	event := &plugins.PluginItemActionEvent{
		Player: p, ItemID: 23442, Slot: 0, Option: "Open",
	}
	if !m.openClueGeode(event) {
		t.Fatal("Open not handled")
	}
	if got := p.Inventory().GetAmount(23442); got != 0 {
		t.Errorf("geode left = %d, want 0", got)
	}
	if got := p.Inventory().GetAmount(23182); got != 1 {
		t.Errorf("beginner scroll = %d, want 1", got)
	}
}

func TestRollClueGeodes(t *testing.T) {
	m, _ := newTestMining(t)
	p := newTestPlayer("miner", 3200, 3200)
	setLevel(p, game.SkillMining, 1)
	// rand 0.0: every tier's 1-in-N roll hits, and the player
	// owns no clue — all five geodes arrive.
	m.randFloat = func() float64 { return 0.0 }
	m.rollClueGeodes(&successEvent{Player: p, Skill: game.SkillMining, PetBase: 1000})
	for _, id := range []int{23442, 20358, 20360, 20362, 20364} {
		if got := p.Inventory().GetAmount(id); got != 1 {
			t.Errorf("geode %d = %d, want 1", id, got)
		}
	}
	// A non-mining success event rolls nothing.
	p2 := newTestPlayer("miner2", 3200, 3200)
	m.rollClueGeodes(&successEvent{Player: p2, Skill: game.SkillWoodcutting, PetBase: 1000})
	if got := p2.Inventory().GetAmount(23442); got != 0 {
		t.Errorf("woodcutting event: geode = %d, want 0", got)
	}
}

// ---- saltpetre ----

func TestSaltpetreAmount(t *testing.T) {
	m, _ := newTestMining(t)
	p := newTestPlayer("miner", 3200, 3200)
	setLevel(p, game.SkillMining, 50)
	m.randFloat = func() float64 { return 0.0 }
	if got := m.saltpetreAmount(p); got != 2 {
		t.Errorf("rand 0.0 at 50: got %d, want 2", got)
	}
	m.randFloat = func() float64 { return 0.99 }
	if got := m.saltpetreAmount(p); got != 1 {
		t.Errorf("rand 0.99 at 50: got %d, want 1", got)
	}
}

func TestHasSpade(t *testing.T) {
	m, _ := newTestMining(t)
	p := newTestPlayer("miner", 3200, 3200)
	if m.hasSpade(p) {
		t.Error("no spade: want false")
	}
	p.Inventory().Add(game.NewItem(3125, 1), true)
	if !m.hasSpade(p) {
		t.Error("spade in inventory: want true")
	}
}

// ---- varrock armour ----

func TestVarrockExtraOre(t *testing.T) {
	m, _ := newTestMining(t)
	m.membersWorld = true
	p := newTestPlayer("miner", 3200, 3200)
	// Tier 4 + runite + rand 0.0: bonus.
	p.Equipment().Set(game.BodySlot, game.NewItem(13107, 1))
	m.randFloat = func() float64 { return 0.0 }
	req := &extraOreRequest{Player: p, OreID: 451}
	m.varrockExtraOre(req)
	if len(req.Bonuses) != 1 {
		t.Errorf("tier 4 runite: bonuses = %d, want 1", len(req.Bonuses))
	}
	// Tier 1 cannot double runite.
	p.Equipment().Set(game.BodySlot, game.NewItem(13104, 1))
	req = &extraOreRequest{Player: p, OreID: 451}
	m.varrockExtraOre(req)
	if len(req.Bonuses) != 0 {
		t.Errorf("tier 1 runite: bonuses = %d, want 0", len(req.Bonuses))
	}
	// Tier 1 doubles copper.
	req = &extraOreRequest{Player: p, OreID: 436}
	m.varrockExtraOre(req)
	if len(req.Bonuses) != 1 {
		t.Errorf("tier 1 copper: bonuses = %d, want 1", len(req.Bonuses))
	}
	// rand 0.5: the 10% roll misses.
	m.randFloat = func() float64 { return 0.5 }
	req = &extraOreRequest{Player: p, OreID: 436}
	m.varrockExtraOre(req)
	if len(req.Bonuses) != 0 {
		t.Errorf("rand 0.5: bonuses = %d, want 0", len(req.Bonuses))
	}
	// Free world: no bonus.
	m.membersWorld = false
	m.randFloat = func() float64 { return 0.0 }
	req = &extraOreRequest{Player: p, OreID: 436}
	m.varrockExtraOre(req)
	if len(req.Bonuses) != 0 {
		t.Errorf("free world: bonuses = %d, want 0", len(req.Bonuses))
	}
}

// ---- prospector outfit ----

func TestProspectorMultiplier(t *testing.T) {
	m, _ := newTestMining(t)
	p := newTestPlayer("miner", 3200, 3200)
	if got := m.prospectorMultiplier(p); got != 1.0 {
		t.Errorf("nothing worn: got %v, want 1.0", got)
	}
	// Float sums are compared with a tolerance: 0.004+0.008
	// is not exactly 1.012 in binary.
	near := func(got, want float64) bool { return math.Abs(got-want) < 1e-9 }
	p.Equipment().Set(game.HeadSlot, game.NewItem(12013, 1))
	p.Equipment().Set(game.BodySlot, game.NewItem(12014, 1))
	if got := m.prospectorMultiplier(p); !near(got, 1.012) {
		t.Errorf("helmet+jacket: got %v, want 1.012", got)
	}
	// Varrock armour 4 stands in for the jacket.
	p.Equipment().Set(game.BodySlot, game.NewItem(13107, 1))
	if got := m.prospectorMultiplier(p); !near(got, 1.012) {
		t.Errorf("helmet+varrock4: got %v, want 1.012", got)
	}
	// Full set: 0.4+0.8+0.6+0.2+0.5 = 2.5%.
	p.Equipment().Set(game.BodySlot, game.NewItem(12014, 1))
	p.Equipment().Set(game.LegSlot, game.NewItem(12015, 1))
	p.Equipment().Set(game.FeetSlot, game.NewItem(12016, 1))
	if got := m.prospectorMultiplier(p); !near(got, 1.025) {
		t.Errorf("full set: got %v, want 1.025", got)
	}
	// A golden helmet counts the same as a plain one.
	m.tables.ItemNameByID[12017] = "Golden prospector helmet"
	p.Equipment().Set(game.HeadSlot, game.NewItem(12017, 1))
	if got := m.prospectorMultiplier(p); !near(got, 1.025) {
		t.Errorf("golden helmet in full set: got %v, want 1.025", got)
	}
}

// ---- saltpetre dig ----

func TestDigSaltpetre(t *testing.T) {
	m, _ := newTestMining(t)
	objects := newFakeObjects()
	m.objects = objects
	tm := game.NewTaskManager()
	m.tasks = tm
	p := newTestPlayer("miner", 3200, 3200)
	setLevel(p, game.SkillMining, 1)
	session := &testSession{}
	p.Session = session
	p.Inventory().Add(game.NewItem(3125, 1), true) // spade
	deposit := game.NewGameObject(27433, game.Location{X: 3201, Y: 3200, Z: 0}, 10, 0, nil)
	event := &plugins.ObjectInteractionEvent{
		Player: p, Object: deposit, Definition: fakeObjectDef{name: "Saltpetre"},
		ObjectID: 27433, Option: "Dig", HasOption: true,
		Location: deposit.Location(),
	}
	// rand 0.99: yield 1 at level 1.
	m.randFloat = func() float64 { return 0.99 }
	if !m.digSaltpetre(event) {
		t.Fatal("Dig not handled")
	}
	// The 2-tick task completes on the second Process tick.
	tm.Process(1)
	if got := p.Inventory().GetAmount(13421); got != 0 {
		t.Fatalf("after 1 tick: saltpetre = %d, want 0", got)
	}
	tm.Process(2)
	if got := p.Inventory().GetAmount(13421); got != 1 {
		t.Fatalf("after 2 ticks: saltpetre = %d, want 1", got)
	}
	found := false
	for _, msg := range session.gameMessages {
		if msg == "You dig up some saltpetre." {
			found = true
		}
	}
	if !found {
		t.Errorf("dig message missing; got %v", session.gameMessages)
	}
}

func TestDigSaltpetreNoSpade(t *testing.T) {
	m, _ := newTestMining(t)
	m.objects = newFakeObjects()
	m.tasks = game.NewTaskManager()
	p := newTestPlayer("miner", 3200, 3200)
	session := &testSession{}
	p.Session = session
	deposit := game.NewGameObject(27433, game.Location{X: 3201, Y: 3200, Z: 0}, 10, 0, nil)
	event := &plugins.ObjectInteractionEvent{
		Player: p, Object: deposit, Definition: fakeObjectDef{name: "Saltpetre"},
		ObjectID: 27433, Option: "Dig", HasOption: true,
		Location: deposit.Location(),
	}
	if !m.digSaltpetre(event) {
		t.Fatal("Dig not handled")
	}
	if len(session.gameMessages) != 1 || session.gameMessages[0] != saltpetreNeedSpade {
		t.Errorf("no-spade message = %v, want %q", session.gameMessages, saltpetreNeedSpade)
	}
	if got := p.Inventory().GetAmount(13421); got != 0 {
		t.Errorf("saltpetre = %d, want 0", got)
	}
}

func TestDigSaltpetreSupersede(t *testing.T) {
	m, _ := newTestMining(t)
	m.objects = newFakeObjects()
	tm := game.NewTaskManager()
	m.tasks = tm
	p := newTestPlayer("miner", 3200, 3200)
	setLevel(p, game.SkillMining, 1)
	p.Inventory().Add(game.NewItem(3125, 1), true)
	deposit := game.NewGameObject(27433, game.Location{X: 3201, Y: 3200, Z: 0}, 10, 0, nil)
	event := &plugins.ObjectInteractionEvent{
		Player: p, Object: deposit, Definition: fakeObjectDef{name: "Saltpetre"},
		ObjectID: 27433, Option: "Dig", HasOption: true,
		Location: deposit.Location(),
	}
	m.randFloat = func() float64 { return 0.99 }
	m.digSaltpetre(event)
	m.digSaltpetre(event) // second dig retires the first
	tm.Process(1)
	tm.Process(2)
	if got := p.Inventory().GetAmount(13421); got != 1 {
		t.Errorf("after supersede: saltpetre = %d, want 1 (one dig only)", got)
	}
}
