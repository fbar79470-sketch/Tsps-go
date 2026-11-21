package fishing

import (
	"strings"
	"testing"

	"tsps-go/internal/game"
)

// fixtureResolver is a hand-built IDResolver: every gameval
// name the source defs use maps to a deterministic fake id.
// No live cache is involved.
type fixtureResolver struct {
	obj map[string]int
}

func (f fixtureResolver) ObjID(name string) (int, bool, error) {
	id, ok := f.obj[name]
	return id, ok, nil
}

// newFixtureResolver collects every gameval name the source
// defs reference and assigns deterministic fake ids, one per
// distinct name (shared names share an id, as in the cache).
func newFixtureResolver() fixtureResolver {
	names := map[string]bool{}
	add := func(ns ...string) {
		for _, n := range ns {
			names[n] = true
		}
	}
	for _, def := range FishDefs {
		add(def.Item)
		if def.Big != nil {
			add(def.Big.Item)
		}
	}
	for _, def := range camdozaalFishDefs {
		add(def.Item)
	}
	for _, def := range ToolDefs {
		add(def.Item)
		add(def.Bait...)
		add(def.Worn...)
		for _, v := range def.Variants {
			add(v.Items...)
		}
	}
	for _, def := range camdozaalToolDefs {
		add(def.Item)
	}
	for _, def := range HarpoonVariantDefs {
		add(def.Items...)
	}
	obj := make(map[string]int, len(names))
	next := 1000
	for name := range names {
		obj[name] = next
		next++
	}
	return fixtureResolver{obj: obj}
}

func loadFixture(t *testing.T) *Tables {
	t.Helper()
	tables, err := Load(newFixtureResolver())
	if err != nil {
		t.Fatalf("Load(fixture) = %v", err)
	}
	return tables
}

func TestLoadCounts(t *testing.T) {
	tables := loadFixture(t)
	if got := len(tables.Fish); got != 50 {
		t.Errorf("fish = %d, want 50 (46 FISH + 4 Camdozaal)", got)
	}
	if got := len(tables.Tools); got != 24 {
		t.Errorf("tools = %d, want 24", got)
	}
	if got := len(tables.SpotToolsByOptions); got != 7 {
		t.Errorf("spot option keys = %d, want 7", got)
	}
	if got := len(tables.SpotToolsByNPC); got != 29 {
		t.Errorf("spot npcs = %d, want 29", got)
	}
	if got := len(tables.ClueBottleFish); got != 29 {
		t.Errorf("clue-bottle fish = %d, want 29", got)
	}
	if got := len(tables.Harpoons); got != 7 {
		t.Errorf("harpoon variants = %d, want 7", got)
	}
	if tables.FallbackItemNames != 0 {
		t.Errorf("FallbackItemNames = %d, want 0 (every name resolves by gameval)", tables.FallbackItemNames)
	}
	if tables.DistinctItemNames != tables.GamevalItemNames || tables.DistinctItemNames == 0 {
		t.Errorf("DistinctItemNames = %d, GamevalItemNames = %d, want equal and > 0",
			tables.DistinctItemNames, tables.GamevalItemNames)
	}
}

func TestFishEntries(t *testing.T) {
	tables := loadFixture(t)

	shrimp := tables.Fish["SHRIMP"]
	if shrimp == nil {
		t.Fatal("SHRIMP missing")
	}
	if shrimp.Level != 1 || shrimp.XP != 10 || shrimp.PetBase != 870330 {
		t.Errorf("SHRIMP = level %d xp %v pet %d, want 1/10/870330", shrimp.Level, shrimp.XP, shrimp.PetBase)
	}
	if len(shrimp.Rolls) != 1 || shrimp.Rolls[0] != [2]int{48, 256} {
		t.Errorf("SHRIMP rolls = %v, want [[48 256]]", shrimp.Rolls)
	}
	if got := shrimp.MessageFor(1); got != "You catch some raw shrimps." {
		t.Errorf("SHRIMP message = %q, want %q", got, "You catch some raw shrimps.")
	}

	// The fly rod's catch lines have no full stop.
	if got := tables.Fish["TROUT"].MessageFor(1); got != "You catch a raw trout" {
		t.Errorf("TROUT message = %q, want no full stop", got)
	}
	if got := tables.Fish["SALMON"].MessageFor(1); got != "You catch a raw salmon" {
		t.Errorf("SALMON message = %q, want no full stop", got)
	}
	if got := tables.Fish["SHARK"].MessageFor(1); got != "You catch a shark!" {
		t.Errorf("SHARK message = %q", got)
	}
	if got := tables.Fish["KARAMBWAN"].MessageFor(1); got != "You catch a Karambwan!" {
		t.Errorf("KARAMBWAN message = %q", got)
	}

	// Mackerel has two charts, so one cast can net two.
	if got := len(tables.Fish["MACKEREL"].Rolls); got != 2 {
		t.Errorf("MACKEREL rolls = %d, want 2", got)
	}

	// Big fish: 1/1000 bass, 1/2500 swordfish, 1/5000 shark.
	for name, want := range map[string]int{"BASS": 1000, "SWORDFISH": 2500, "SHARK": 5000} {
		fish := tables.Fish[name]
		if fish.Big == nil || fish.Big.Chance != want {
			t.Errorf("%s big chance = %v, want 1/%d", name, fish.Big, want)
		}
		if got := fish.Big.BigMessage(); got != "You catch an enormous "+strings.ToLower(name)+"!" {
			t.Errorf("%s big message = %q", name, got)
		}
	}
	// The barehand shark shares the shark's big fish.
	if tables.Fish["SHARK_BAREHAND"].Big.Chance != 5000 {
		t.Error("SHARK_BAREHAND big chance != 5000")
	}

	// XP stays float64 here; truncation happens at the grant
	// site (the core loop), not in the table.
	if tables.Fish["MINNOW"].XP != 26.1 {
		t.Errorf("MINNOW xp = %v, want 26.1", tables.Fish["MINNOW"].XP)
	}
	if tables.Fish["PARK_JADE"].XP != 7.5 {
		t.Errorf("PARK_JADE xp = %v, want 7.5", tables.Fish["PARK_JADE"].XP)
	}
	if tables.Fish["PARK_LOOP_HALF"].XP != 0 {
		t.Errorf("PARK_LOOP_HALF xp = %v, want 0", tables.Fish["PARK_LOOP_HALF"].XP)
	}

	// Dark crab diary rolls.
	crab := tables.Fish["DARK_CRAB"]
	if crab.DiaryRolls == nil || crab.DiaryRolls.Diary != "wilderness" || crab.DiaryRolls.Tier != "elite" {
		t.Errorf("DARK_CRAB diary rolls = %+v, want wilderness/elite", crab.DiaryRolls)
	}
	if len(crab.DiaryRolls.Rolls) != 2 || crab.DiaryRolls.Rolls[0] != [2]int{13, 55} {
		t.Errorf("DARK_CRAB diary rolls = %v, want [[13 55] [3 40]]", crab.DiaryRolls.Rolls)
	}

	// Junk: no pet roll, no fishing:success.
	for _, name := range []string{"LEATHER_BOOTS", "SEAWEED", "LEATHER_GLOVES", "OYSTER", "CASKET", "PARK_OLD_BOOT"} {
		if !tables.Fish[name].Junk {
			t.Errorf("%s Junk = false, want true", name)
		}
	}
	if tables.Fish["MACKEREL"].Junk {
		t.Error("MACKEREL Junk = true, want false")
	}
	// Park finds have no charts: they land off the table.
	if len(tables.Fish["PARK_JADE"].Rolls) != 0 {
		t.Errorf("PARK_JADE rolls = %v, want none", tables.Fish["PARK_JADE"].Rolls)
	}

	// Leaping fish need and train Strength/Agility.
	trout := tables.Fish["LEAPING_TROUT"]
	if len(trout.Requires) != 2 || trout.Requires[0] != (SkillRequirement{Skill: game.SkillStrength, Level: 15}) ||
		trout.Requires[1] != (SkillRequirement{Skill: game.SkillAgility, Level: 15}) {
		t.Errorf("LEAPING_TROUT requires = %+v", trout.Requires)
	}
	if len(trout.ExtraXP) != 2 || trout.ExtraXP[0] != (SkillXP{Skill: game.SkillStrength, XP: 5}) {
		t.Errorf("LEAPING_TROUT extraXp = %+v", trout.ExtraXP)
	}
	// Barehand tuna: 35 Strength to catch, 8 Strength XP.
	if got := tables.Fish["TUNA_BAREHAND"].Requires; len(got) != 1 || got[0].Level != 35 {
		t.Errorf("TUNA_BAREHAND requires = %+v", got)
	}
	if tables.Fish["TUNA_BAREHAND"].CatchAnimation != AnimBarehandTunaCatch {
		t.Errorf("TUNA_BAREHAND catch anim = %d, want %d", tables.Fish["TUNA_BAREHAND"].CatchAnimation, AnimBarehandTunaCatch)
	}
	if tables.Fish["SWORDFISH_BAREHAND"].CatchAnimation != AnimBarehandSwordfishCatch {
		t.Errorf("SWORDFISH_BAREHAND catch anim = %d, want %d", tables.Fish["SWORDFISH_BAREHAND"].CatchAnimation, AnimBarehandSwordfishCatch)
	}
}

func TestFishAmounts(t *testing.T) {
	tables := loadFixture(t)

	// Karambwanji: floor(level/5)+1 on the CURRENT level.
	karambwanji := tables.Fish["KARAMBWANJI"]
	for level, want := range map[int]int{5: 2, 6: 2, 25: 6, 99: 20} {
		if got := karambwanji.AmountFor(level, level); got != want {
			t.Errorf("KARAMBWANJI amount at %d = %d, want %d", level, got, want)
		}
	}
	if got := karambwanji.MessageFor(3); got != "You catch 3 Karambwanji." {
		t.Errorf("KARAMBWANJI message = %q", got)
	}

	// Minnows: 10@82 … 14@99 on the MAX level.
	minnow := tables.Fish["MINNOW"]
	for max, want := range map[int]int{82: 10, 85: 11, 90: 12, 95: 13, 99: 14} {
		if got := minnow.AmountFor(99, max); got != want {
			t.Errorf("MINNOW amount at max %d = %d, want %d", max, got, want)
		}
	}

	// Default amount is 1.
	if got := tables.Fish["SHRIMP"].AmountFor(99, 99); got != 1 {
		t.Errorf("SHRIMP amount = %d, want 1", got)
	}
}

func TestToolEntries(t *testing.T) {
	tables := loadFixture(t)

	net := tables.Tools["NET"]
	if net.AnimationID != AnimNet || net.Interval != NetCageHarpoonIntervalTicks ||
		!net.HasFirstRoll || net.FirstRoll != NetCageFirstRollTicks || net.StartMessage != CastNetMessage {
		t.Errorf("NET = anim %d interval %d firstRoll %d/%v start %q",
			net.AnimationID, net.Interval, net.FirstRoll, net.HasFirstRoll, net.StartMessage)
	}
	// Cascade order is highest level first.
	if len(net.Fish) != 2 || net.Fish[0].Name != "ANCHOVY" || net.Fish[1].Name != "SHRIMP" {
		t.Errorf("NET fish cascade = %v, want [ANCHOVY SHRIMP]", fishNames(net.Fish))
	}
	// Default variant: the tool itself, level 1, bonus 100.
	if len(net.Variants) != 1 || net.Variants[0].Bonus != 100 || net.Variants[0].Level != 1 ||
		len(net.Variants[0].IDs) != 1 || net.Variants[0].IDs[0] != net.ID {
		t.Errorf("NET default variant = %+v, want the net itself at bonus 100", net.Variants)
	}

	rod := tables.Tools["FISHING_ROD"]
	if rod.AnimationID != AnimRod || rod.LoopAnimationID != AnimRodOnSpot ||
		rod.FollowMessage != "You attempt to catch a fish." ||
		rod.NoBaitStatement != NoFishingBaitStatement {
		t.Errorf("FISHING_ROD = anim %d/%d follow %q noBait %q",
			rod.AnimationID, rod.LoopAnimationID, rod.FollowMessage, rod.NoBaitStatement)
	}
	if rod.LevelStatement == nil || rod.LevelStatement(5) != "You need at least level 5 Fishing to bait these fish." {
		t.Error("FISHING_ROD levelStatement != baitLevel(5)")
	}

	// Harpoon tools share the HARPOONS variants and the
	// harpoon level box.
	for _, name := range []string{"HARPOON", "SHARK_HARPOON"} {
		tool := tables.Tools[name]
		if len(tool.Variants) != 7 {
			t.Errorf("%s variants = %d, want 7", name, len(tool.Variants))
		}
		if tool.LevelStatement == nil ||
			tool.LevelStatement(35) != "You need to be at least level 35 Fishing to harpoon these fish." {
			t.Errorf("%s levelStatement != harpoonLevel(35)", name)
		}
	}

	// The dark-crab pot relies on the defaults: start line
	// "You begin to fish.." and the 5-tick interval.
	crab := tables.Tools["DARK_CRAB_POT"]
	if crab.StartMessage != DefaultStartMessage || crab.Interval != FishingActionIntervalTicks {
		t.Errorf("DARK_CRAB_POT start %q interval %d, want default/%d",
			crab.StartMessage, crab.Interval, FishingActionIntervalTicks)
	}
	if len(crab.Bait) != 1 {
		t.Errorf("DARK_CRAB_POT bait = %d entries, want 1 (dark fishing bait)", len(crab.Bait))
	}

	// Infernal-eel rod: the cast animation repeats (loop ==
	// cast, not on-spot), and ice gloves must be worn.
	infernal := tables.Tools["INFERNAL_EEL_ROD"]
	if infernal.LoopAnimationID != AnimRod {
		t.Errorf("INFERNAL_EEL_ROD loop anim = %d, want %d (cast repeats)", infernal.LoopAnimationID, AnimRod)
	}
	if len(infernal.Worn) != 2 {
		t.Errorf("INFERNAL_EEL_ROD worn = %d, want 2 (ice gloves / smiths gloves (i))", len(infernal.Worn))
	}

	// Anglerfish rod: noBaitStatement null — no box.
	if tables.Tools["ANGLERFISH_ROD"].NoBaitStatement != "" {
		t.Errorf("ANGLERFISH_ROD noBaitStatement = %q, want empty (null in TS)", tables.Tools["ANGLERFISH_ROD"].NoBaitStatement)
	}

	// Minnow net: unboostable, 2-tick rolls, no animation
	// restart on roll, MinnowPlatform spot (deferred).
	minnow := tables.Tools["MINNOW_NET"]
	if !minnow.Unboostable || minnow.Interval != MinnowAttemptIntervalTicks || minnow.AnimateOnRoll {
		t.Errorf("MINNOW_NET unboostable %v interval %d animateOnRoll %v, want true/2/false",
			minnow.Unboostable, minnow.Interval, minnow.AnimateOnRoll)
	}
	if minnow.Spot != "MinnowPlatform" {
		t.Errorf("MINNOW_NET spot = %q, want MinnowPlatform", minnow.Spot)
	}

	// Karambwan vessel: stationary, vessel bait, 4-tick
	// interval, no first roll (first roll at the interval).
	vessel := tables.Tools["KARAMBWAN_VESSEL"]
	if !vessel.Stationary || !vessel.Vessel || vessel.Interval != KarambwanIntervalTicks || vessel.HasFirstRoll {
		t.Errorf("KARAMBWAN_VESSEL stationary %v vessel %v interval %d hasFirstRoll %v",
			vessel.Stationary, vessel.Vessel, vessel.Interval, vessel.HasFirstRoll)
	}
	if vessel.MissMessage != "A Karambwan deftly snatches the Karambwanji from your vessel!" {
		t.Errorf("KARAMBWAN_VESSEL miss = %q", vessel.MissMessage)
	}
	if vessel.NoToolStatement != "You need a Karambwan vessel to fish here." {
		t.Errorf("KARAMBWAN_VESSEL noTool = %q", vessel.NoToolStatement)
	}

	// Big net: multi, junk first in Mod Ash's order.
	bigNet := tables.Tools["BIG_NET"]
	if !bigNet.Multi || len(bigNet.Fish) != 8 || bigNet.Fish[0].Name != "LEATHER_BOOTS" {
		t.Errorf("BIG_NET multi %v fish %v", bigNet.Multi, fishNames(bigNet.Fish))
	}

	// Barbarian rod: 15/15 requirements, five baits, quest.
	barb := tables.Tools["BARBARIAN_ROD"]
	if barb.Quest != "Barbarian Training" || len(barb.Bait) != 5 || len(barb.Requires) != 2 {
		t.Errorf("BARBARIAN_ROD quest %q bait %d requires %v", barb.Quest, len(barb.Bait), barb.Requires)
	}
	if barb.AnimationID != AnimBarbarianRod || barb.LoopAnimationID != AnimBarbarianRodOnSpot {
		t.Errorf("BARBARIAN_ROD anim %d/%d, want %d/%d",
			barb.AnimationID, barb.LoopAnimationID, AnimBarbarianRod, AnimBarbarianRodOnSpot)
	}

	// Barehand tools: empty-ids variants, barehand flag.
	for _, name := range []string{"BAREHAND_SHARK", "BAREHAND_HARPOON"} {
		tool := tables.Tools[name]
		if !tool.Barehand || len(tool.Variants) != 1 || len(tool.Variants[0].IDs) != 0 || !tool.Variants[0].Barehand {
			t.Errorf("%s barehand %v variants %+v", name, tool.Barehand, tool.Variants)
		}
		if tool.AnimationID != AnimBarehandStart || tool.LoopAnimationID != AnimBarehandLoop {
			t.Errorf("%s anim %d/%d, want %d/%d", name, tool.AnimationID, tool.LoopAnimationID,
				AnimBarehandStart, AnimBarehandLoop)
		}
	}
	if tables.Tools["BAREHAND_SHARK"].CatchAnimationID != AnimBarehandSharkCatch {
		t.Errorf("BAREHAND_SHARK catch anim = %d, want %d",
			tables.Tools["BAREHAND_SHARK"].CatchAnimationID, AnimBarehandSharkCatch)
	}
}

func fishNames(fish []*Fish) []string {
	out := make([]string, len(fish))
	for i, f := range fish {
		out[i] = f.Name
	}
	return out
}

func TestHarpoonVariants(t *testing.T) {
	tables := loadFixture(t)
	want := []struct {
		bonus    int
		agility  int
		crystal  bool
		infernal bool
	}{
		{135, 0, false, false},  // echo
		{135, 50, true, false},  // charged crystal
		{120, 0, false, true},   // charged infernal
		{120, 0, false, false},  // uncharged infernal
		{120, 50, false, false}, // inactive crystal
		{120, 0, false, false},  // dragon
		{100, 0, false, false},  // plain / barb-tail
	}
	for i, w := range want {
		v := tables.Harpoons[i]
		if v.Bonus != w.bonus || v.Agility != w.agility || v.Crystal != w.crystal || v.Infernal != w.infernal {
			t.Errorf("harpoon variant %d = bonus %d agility %d crystal %v infernal %v, want %+v",
				i, v.Bonus, v.Agility, v.Crystal, v.Infernal, w)
		}
	}
	// Pearl rods fish exactly as their plain versions: the
	// variant sets hold both ids at bonus 100.
	if len(tables.FishingRods) != 1 || len(tables.FishingRods[0].IDs) != 2 {
		t.Errorf("FishingRods = %+v, want one variant with 2 ids", tables.FishingRods)
	}
	if len(tables.BarbarianRods[0].IDs) != 2 {
		t.Errorf("BarbarianRods ids = %d, want 2", len(tables.BarbarianRods[0].IDs))
	}
}

func TestPikeInterval(t *testing.T) {
	// 6 one time in 5, else 5: only those two values, and
	// both occur over many draws.
	seen := map[int]bool{}
	for i := 0; i < 500; i++ {
		v := PikeInterval()
		if v != 5 && v != 6 {
			t.Fatalf("PikeInterval() = %d, want 5 or 6", v)
		}
		seen[v] = true
	}
	if len(seen) != 2 {
		t.Errorf("PikeInterval() only produced %v over 500 draws", seen)
	}
	// The tool wires it, with the fixed interval unused.
	tool := loadFixture(t).Tools["PIKE_ROD"]
	if tool.IntervalFunc == nil {
		t.Fatal("PIKE_ROD IntervalFunc = nil")
	}
	if got := tool.IntervalTicks(); got != 5 && got != 6 {
		t.Errorf("PIKE_ROD IntervalTicks() = %d", got)
	}
}

func TestParkTable(t *testing.T) {
	tables := loadFixture(t)
	park := tables.Tools["PARK_NET"]
	if park.Table == nil {
		t.Fatal("PARK_NET table = nil")
	}
	if len(park.Table.Rolls) != 1 || park.Table.Rolls[0] != [2]int{20, 20} {
		t.Errorf("PARK_NET table rolls = %v, want [[20 20]]", park.Table.Rolls)
	}
	if len(park.Table.PreRoll) != 2 ||
		park.Table.PreRoll[0].Fish.Name != "PARK_JADE" || park.Table.PreRoll[0].Chance != 1.0/100 ||
		park.Table.PreRoll[1].Fish.Name != "PARK_RED_TOPAZ" || park.Table.PreRoll[1].Chance != 1.0/505 {
		t.Errorf("PARK_NET pre-roll = %+v", park.Table.PreRoll)
	}
	weights := 0
	for _, w := range park.Table.Weighted {
		weights += w.Weight
	}
	if len(park.Table.Weighted) != 5 || weights != 10 {
		t.Errorf("PARK_NET weighted = %d entries totalling %d, want 5/10", len(park.Table.Weighted), weights)
	}
	if len(park.Table.Tertiary) != 1 || park.Table.Tertiary[0].Fish.Name != "PARK_LOOP_HALF" ||
		park.Table.Tertiary[0].Chance != 1.0/1014 {
		t.Errorf("PARK_NET tertiary = %+v", park.Table.Tertiary)
	}
	if !park.Stationary || park.Interval != 1 || !park.HasFirstRoll || park.FirstRoll != 1 {
		t.Errorf("PARK_NET stationary %v interval %d firstRoll %d/%v",
			park.Stationary, park.Interval, park.FirstRoll, park.HasFirstRoll)
	}
	if park.NoToolMessage != "This is the wrong size net for fishing here." {
		t.Errorf("PARK_NET noToolMessage = %q", park.NoToolMessage)
	}
}

func TestSpotMaps(t *testing.T) {
	tables := loadFixture(t)

	// By options: every id with the same options behaves alike.
	cage := tables.SpotToolsByOptions["Cage/Harpoon"]
	if cage["Cage"].Name != "LOBSTER_POT" || cage["Harpoon"].Name != "HARPOON" {
		t.Errorf("Cage/Harpoon = %v", cage)
	}
	if tables.SpotToolsByOptions["Big Net/Harpoon"]["Harpoon"].Name != "SHARK_HARPOON" {
		t.Error("Big Net/Harpoon Harpoon != SHARK_HARPOON")
	}
	if tables.SpotToolsByOptions["Net/Harpoon"]["Net"].Name != "BIG_NET" {
		t.Error("Net/Harpoon Net != BIG_NET")
	}
	if tables.SpotToolsByOptions["Lure/Bait"]["Bait"].Name != "PIKE_ROD" {
		t.Error("Lure/Bait Bait != PIKE_ROD (river pike, not sea fish)")
	}
	if tables.SpotToolsByOptions["Cage"]["Cage"].Name != "DARK_CRAB_POT" {
		t.Error("Cage Cage != DARK_CRAB_POT")
	}

	// By NPC: tutorial pond, Lumbridge Swamp, Piscatoris,
	// frog/eel spots, karambwan, park.
	spot := func(id int, option string) string {
		tools := tables.SpotToolsByNPC[id]
		if tools == nil {
			t.Errorf("spot %d has no entry", id)
			return ""
		}
		tool := tools[option]
		if tool == nil {
			t.Errorf("spot %d has no %q option", id, option)
			return ""
		}
		return tool.Name
	}
	if got := spot(NPCFishingSpot43, "Net"); got != "NET" {
		t.Errorf("tutorial spot Net = %s, want NET", got)
	}
	if got := spot(NPCFishingSpot30, "Bait"); got != "FISHING_ROD" {
		t.Errorf("Lumbridge Swamp Bait = %s, want FISHING_ROD", got)
	}
	if got := spot(NPCFishingSpot55, "Net"); got != "MONKFISH_NET" {
		t.Errorf("Piscatoris Net = %s, want MONKFISH_NET", got)
	}
	for _, id := range []int{NPCFishingSpot2, NPCFishingSpot3, NPCFishingSpot4, NPCFishingSpot5, NPCFishingSpot92} {
		if got := spot(id, "Bait"); got != "CAVE_EEL_ROD" {
			t.Errorf("frog/eel spot %d Bait = %s, want CAVE_EEL_ROD", id, got)
		}
	}
	if got := spot(NPCFishingSpot58, "Net"); got != "KARAMBWANJI_NET" {
		t.Errorf("Holy Lake Net = %s, want KARAMBWANJI_NET", got)
	}
	if got := spot(NPCFishingSpot60, "Fish"); got != "KARAMBWAN_VESSEL" {
		t.Errorf("karambwan spot Fish = %s, want KARAMBWAN_VESSEL", got)
	}
	if got := spot(NPCFishingSpot101, "Cast"); got != "PARK_NET" {
		t.Errorf("park pond Cast = %s, want PARK_NET", got)
	}
	if got := spot(NPCFishingSpot87, "Small Net"); got != "MINNOW_NET" {
		t.Errorf("minnow spot Small Net = %s, want MINNOW_NET", got)
	}
	if got := spot(NPCRodFishingSpot20, "Bait"); got != "INFERNAL_EEL_ROD" {
		t.Errorf("Mor Ul Rek Bait = %s, want INFERNAL_EEL_ROD", got)
	}
}

func TestClueBottleFish(t *testing.T) {
	tables := loadFixture(t)
	for _, name := range []string{"SHRIMP", "SHARK_BAREHAND", "MINNOW", "LEAPING_STURGEON", "DARK_CRAB"} {
		if !tables.IsClueBottleFish(tables.Fish[name]) {
			t.Errorf("%s should be a clue-bottle fish", name)
		}
	}
	for _, name := range []string{"LEATHER_BOOTS", "CASKET", "PARK_JADE", "FROG_SPAWN", "SLIMY_EEL", "GUPPY"} {
		if tables.IsClueBottleFish(tables.Fish[name]) {
			t.Errorf("%s should NOT be a clue-bottle fish", name)
		}
	}
	if tables.IsClueBottleFish(nil) {
		t.Error("IsClueBottleFish(nil) = true")
	}
}

func TestCamdozaal(t *testing.T) {
	tables := loadFixture(t)
	small := tables.CamdozaalTools["Small Net"]
	big := tables.CamdozaalTools["Big Net"]
	if small == nil || big == nil {
		t.Fatal("Camdozaal tools missing")
	}
	if got := fishNames(small.Fish); len(got) != 3 || got[0] != "TETRA" || got[1] != "CAVEFISH" || got[2] != "GUPPY" {
		t.Errorf("Camdozaal small net fish = %v, want [TETRA CAVEFISH GUPPY]", got)
	}
	if got := fishNames(big.Fish); len(got) != 2 || got[0] != "CATFISH" || got[1] != "TETRA" {
		t.Errorf("Camdozaal big net fish = %v, want [CATFISH TETRA]", got)
	}
	if big.ClueBase != 100000 {
		t.Errorf("Camdozaal big net clueBase = %d, want 100000", big.ClueBase)
	}
	for _, tool := range []*FishingTool{small, big} {
		if !tool.Stationary || tool.Interval != NetCageHarpoonIntervalTicks ||
			!tool.HasFirstRoll || tool.FirstRoll != NetCageFirstRollTicks ||
			tool.StartMessage != CastNetMessage {
			t.Errorf("Camdozaal tool %v: stationary %v interval %d firstRoll %d/%v start %q",
				tool, tool.Stationary, tool.Interval, tool.FirstRoll, tool.HasFirstRoll, tool.StartMessage)
		}
	}
	// Registered under all three Camdozaal NPC ids.
	for _, id := range []int{NPCCamdozaalFishingSpot1, NPCCamdozaalFishingSpot2, NPCCamdozaalFishingSpot3} {
		tools := tables.SpotToolsByNPC[id]
		if tools == nil || tools["Small Net"] != small || tools["Big Net"] != big {
			t.Errorf("Camdozaal spot %d tools = %v", id, tools)
		}
	}
	// Fish entries: levels and XP from the file.
	if tables.Fish["GUPPY"].Level != 7 || tables.Fish["GUPPY"].XP != 8 {
		t.Errorf("GUPPY = level %d xp %v, want 7/8", tables.Fish["GUPPY"].Level, tables.Fish["GUPPY"].XP)
	}
	if tables.Fish["CATFISH"].Level != 46 || tables.Fish["CATFISH"].XP != 33 {
		t.Errorf("CATFISH = level %d xp %v, want 46/33", tables.Fish["CATFISH"].Level, tables.Fish["CATFISH"].XP)
	}
	// The rolls are the file's estimate, not a measured curve.
	if len(tables.Fish["TETRA"].Rolls) != 1 || tables.Fish["TETRA"].Rolls[0] != [2]int{32, 192} {
		t.Errorf("TETRA rolls = %v, want [[32 192]] (estimate)", tables.Fish["TETRA"].Rolls)
	}
}

func TestLoadFailsClosed(t *testing.T) {
	if _, err := Load(nil); err == nil {
		t.Error("Load(nil) = nil error, want an error")
	}
	if _, err := LoadFromCache(nil); err == nil {
		t.Error("LoadFromCache(nil) = nil error, want an error")
	}
	// A resolver that knows nothing fails listing the names.
	if _, err := Load(fixtureResolver{obj: map[string]int{}}); err == nil {
		t.Error("Load(empty resolver) = nil error, want an error")
	} else if !strings.Contains(err.Error(), "raw_shrimp") {
		t.Errorf("Load(empty resolver) error = %v, want it to name a missing item", err)
	}
	// A resolver missing one name fails on that name.
	r := newFixtureResolver()
	delete(r.obj, "raw_shark")
	if _, err := Load(r); err == nil {
		t.Error("Load(missing raw_shark) = nil error, want an error")
	} else if !strings.Contains(err.Error(), "raw_shark") {
		t.Errorf("Load(missing raw_shark) error = %v, want it to name raw_shark", err)
	}
}
