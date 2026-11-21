package fishing

import (
	"testing"

	"tsps-go/internal/game"
	"tsps-go/internal/plugins"
	"tsps-go/internal/plugins/skillactions"
)

// Fishing loop tests: the core loop in fishing.go against
// Worker A's fixture tables (tables_test.go): every gameval
// name maps to a deterministic fake id, no live cache.
// Fixtures come from helpers_test.go (Worker C's shared
// helpers: newTestFishing over fixture tables + ids, a
// capturing emit, newTestPlayer, setLevel); tests that need
// deterministic rolls set the instance's randFloat / randInt
// fields themselves after construction. Worker C's sub-unit
// methods run through the loop (angler multiplier, flake
// consumption, crystal charge, clue bottles, guild boost,
// karambwan readiness); rollExtraFish and rollClueBottles
// read the package-global rand, not the instance fields, so
// they are exercised but not steered here — with no flakes
// carried and no blessing worn, rollExtraFish always answers
// "" and clue bottles only ever roll their denominators.

// ---- helpers ----

func findEmitted(emitted []emittedEvent, name string) []any {
	var out []any
	for _, e := range emitted {
		if e.name == name {
			out = append(out, e.payload)
		}
	}
	return out
}

func TestCatchChance(t *testing.T) {
	tables := loadFixture(t)
	shrimp := tables.Fish["SHRIMP"]
	// Level 1, bonus 100: floor(48*98/98 + 0 + 0.5) = 48 →
	// (1+48)/256.
	if got := catchChance(1, shrimp, 100, 0); got != 49.0/256 {
		t.Errorf("shrimp L1 = %v, want %v", got, 49.0/256)
	}
	// Level 99: floor(0 + 256*98/98 + 0.5) = 256 → 257/256,
	// clamped to 1.
	if got := catchChance(99, shrimp, 100, 0); got != 1 {
		t.Errorf("shrimp L99 = %v, want 1", got)
	}
	// Bonus 135 (crystal): low floor(48*135/100) = 64, high
	// floor(256*135/100) = 345; level 1: floor(64 + 0.5) = 64
	// → 65/256.
	if got := catchChance(1, shrimp, 135, 0); got != 65.0/256 {
		t.Errorf("shrimp L1 bonus 135 = %v, want %v", got, 65.0/256)
	}
	// No rolls / bad roll index is no chance.
	if got := catchChance(1, tables.Fish["PARK_CASKET"], 100, 0); got != 0 {
		t.Errorf("park find = %v, want 0 (no chart)", got)
	}
}

func TestGetSpotTool(t *testing.T) {
	f, _ := newTestFishing(t)
	tables := f.tables
	// By NPC: Lumbridge Swamp 1530 labels its net "Net".
	if got := f.getSpotTool(1530, []string{"Net", "Bait"}, 1, nil); got != tables.Tools["NET"] {
		t.Errorf("1530 Net = %v, want NET", got)
	}
	if got := f.getSpotTool(1530, []string{"Net", "Bait"}, 2, nil); got != tables.Tools["FISHING_ROD"] {
		t.Errorf("1530 Bait = %v, want FISHING_ROD", got)
	}
	// By options: "Bait" means sea fish on a "Small Net/Bait"
	// spot but pike on a river "Lure/Bait" spot — the joined
	// list disambiguates.
	if got := f.getSpotTool(9999, []string{"Small Net", "Bait"}, 2, nil); got != tables.Tools["FISHING_ROD"] {
		t.Errorf("Small Net/Bait Bait = %v, want FISHING_ROD", got)
	}
	if got := f.getSpotTool(9999, []string{"Lure", "Bait"}, 2, nil); got != tables.Tools["PIKE_ROD"] {
		t.Errorf("Lure/Bait Bait = %v, want PIKE_ROD", got)
	}
	if got := f.getSpotTool(9999, []string{"Lure", "Bait"}, 1, nil); got != tables.Tools["FLY_FISHING_ROD"] {
		t.Errorf("Lure/Bait Lure = %v, want FLY_FISHING_ROD", got)
	}
	// A generic Cage/Harpoon spot harpoons tuna/swordfish.
	if got := f.getSpotTool(9999, []string{"Cage", "Harpoon"}, 2, nil); got != tables.Tools["HARPOON"] {
		t.Errorf("Cage/Harpoon Harpoon = %v, want HARPOON", got)
	}
	// Unknown spot / option: nil, leaving it to others.
	if got := f.getSpotTool(9999, []string{"Talk-to"}, 1, nil); got != nil {
		t.Errorf("Talk-to = %v, want nil", got)
	}
	// Barehand fallback stays locked with no answerer: the
	// harpoon tool is returned, and its own refusal plays.
	player := newTestPlayer("alice", 3241, 3146)
	if got := f.getSpotTool(9999, []string{"Cage", "Harpoon"}, 2, player); got != tables.Tools["HARPOON"] {
		t.Errorf("locked barehand Harpoon = %v, want HARPOON", got)
	}
}

func TestGetSpotToolBarehandUnlocked(t *testing.T) {
	f, _ := newTestFishing(t)
	f.emit = func(name string, payload any) {
		if req, ok := payload.(*BarehandUnlockedRequest); ok && req != nil {
			req.Allowed = true
		}
	}
	player := newTestPlayer("alice", 3241, 3146)
	// No harpoon carried, Otto taught: the Harpoon option
	// falls back to barehanded tuna/swordfish.
	if got := f.getSpotTool(9999, []string{"Cage", "Harpoon"}, 2, player); got != f.tables.Tools["BAREHAND_HARPOON"] {
		t.Errorf("unlocked barehand Harpoon = %v, want BAREHAND_HARPOON", got)
	}
	// A carried harpoon keeps the harpoon tool.
	harpoon := f.tables.Tools["HARPOON"].ID
	player.Inventory().Adds(harpoon, 1)
	if got := f.getSpotTool(9999, []string{"Cage", "Harpoon"}, 2, player); got != f.tables.Tools["HARPOON"] {
		t.Errorf("harpoon carried = %v, want HARPOON", got)
	}
}

func TestHasToolRequirements(t *testing.T) {
	f, _ := newTestFishing(t)
	tables := f.tables
	player := newTestPlayer("alice", 3241, 3146)
	setLevel(player, game.SkillFishing, 1)

	// Level gate: a level-1 player cannot harpoon sharks.
	if got := f.hasToolRequirements(player, tables.Tools["SHARK_HARPOON"]); got != nil {
		t.Errorf("shark harpoon L1 = %v, want nil (level gate)", got)
	}
	// No tool: a level-1 player with no net cannot net.
	if got := f.hasToolRequirements(player, tables.Tools["NET"]); got != nil {
		t.Errorf("net without net = %v, want nil", got)
	}
	// With the net, the plain variant answers.
	player.Inventory().Adds(tables.Tools["NET"].ID, 1)
	if got := f.hasToolRequirements(player, tables.Tools["NET"]); got == nil || got.Bonus != 100 {
		t.Errorf("net with net = %v, want plain variant (bonus 100)", got)
	}
	// Bait gate: a rod without bait refuses.
	if got := f.hasToolRequirements(player, tables.Tools["FISHING_ROD"]); got != nil {
		t.Errorf("rod without bait = %v, want nil", got)
	}
	// With a rod and bait, the rod answers.
	player.Inventory().Adds(tables.Tools["FISHING_ROD"].ID, 1)
	player.Inventory().Adds(tables.Tools["FISHING_ROD"].Bait[0], 5)
	setLevel(player, game.SkillFishing, 5)
	if got := f.hasToolRequirements(player, tables.Tools["FISHING_ROD"]); got == nil {
		t.Error("rod with bait = nil, want a variant")
	}
	// Unboostable: the minnow net reads the MAX level — a
	// boosted 82 with a real 81 cannot use it.
	setLevel(player, game.SkillFishing, 82)
	player.Skills().SetMaxLevel(game.SkillFishing, 81)
	player.Inventory().Adds(tables.Tools["MINNOW_NET"].ID, 1)
	if got := f.hasToolRequirements(player, tables.Tools["MINNOW_NET"]); got != nil {
		t.Errorf("minnow net boosted 82/max 81 = %v, want nil (unboostable)", got)
	}
}

func TestRollCatchCascade(t *testing.T) {
	f, _ := newTestFishing(t)
	f.randFloat = func() float64 { return 0 }
	player := newTestPlayer("alice", 3241, 3146)
	setLevel(player, game.SkillFishing, 1)
	// Level 1, always-succeeding rolls: the anchovy (15) is
	// filtered, the shrimp lands.
	caught := f.rollCatch(player, f.tables.Tools["NET"], 100)
	if len(caught) != 1 || caught[0].Name != "SHRIMP" {
		t.Fatalf("net roll = %v, want [SHRIMP]", caught)
	}
	// Always-failing rolls: a miss.
	f.randFloat = func() float64 { return 1 }
	if caught := f.rollCatch(player, f.tables.Tools["NET"], 100); len(caught) != 0 {
		t.Errorf("net roll rand 1 = %v, want a miss", caught)
	}
}

func TestRollCatchTable(t *testing.T) {
	f, _ := newTestFishing(t)
	f.randFloat = func() float64 { return 0 }
	player := newTestPlayer("alice", 3241, 3146)
	setLevel(player, game.SkillFishing, 1)
	// Always-succeeding rolls: the park table's jade pre-roll
	// (1/100) lands first, and the loop-half tertiary (1/1014)
	// lands on top.
	caught := f.rollCatch(player, f.tables.Tools["PARK_NET"], 100)
	if len(caught) != 2 || caught[0].Name != "PARK_JADE" || caught[1].Name != "PARK_LOOP_HALF" {
		t.Fatalf("park roll = %v, want [PARK_JADE PARK_LOOP_HALF]", caught)
	}
}

func TestRollCatchMulti(t *testing.T) {
	f, _ := newTestFishing(t)
	f.randFloat = func() float64 { return 0 }
	player := newTestPlayer("alice", 3241, 3146)
	setLevel(player, game.SkillFishing, 16)
	// Big net, always-succeeding rolls: every level-16 entry
	// lands, the mackerel twice (two chart rows), cod (23)
	// and bass (46) do not.
	caught := f.rollCatch(player, f.tables.Tools["BIG_NET"], 100)
	names := map[string]int{}
	for _, fish := range caught {
		names[fish.Name]++
	}
	if len(caught) != 7 || names["MACKEREL"] != 2 || names["CASKET"] != 1 || names["COD"] != 0 {
		t.Errorf("big net roll = %v, want 7 incl. MACKEREL x2 and CASKET, no COD", caught)
	}
}

func TestLandCatch(t *testing.T) {
	f, emitted := newTestFishing(t)
	player := newTestPlayer("alice", 3241, 3146)
	setLevel(player, game.SkillFishing, 1)
	tool := f.tables.Tools["NET"]
	variant := f.hasToolRequirements(player, tool)
	if variant != nil {
		t.Fatal("landCatch fixture: net without net must refuse first")
	}
	player.Inventory().Adds(tool.ID, 1)
	variant = f.hasToolRequirements(player, tool)
	if variant == nil {
		t.Fatal("landCatch fixture: net with net must answer")
	}
	shrimp := f.tables.Fish["SHRIMP"]
	if stop := f.landCatch(player, tool, variant, []*Fish{shrimp}, 1530); stop {
		t.Error("landCatch shrimp: stop = true, want false")
	}
	if got := player.Inventory().GetAmount(shrimp.ID); got != 1 {
		t.Errorf("shrimp in inventory = %d, want 1", got)
	}
	if got := player.Skills().Experience(game.SkillFishing); got != 10 {
		t.Errorf("Fishing XP = %d, want 10", got)
	}
	success := findEmitted(*emitted, fishingSuccessEventName)
	if len(success) != 1 {
		t.Fatalf("fishing:success emits = %d, want 1", len(success))
	}
	event, ok := success[0].(*FishingSuccessEvent)
	if !ok || event.ItemID != shrimp.ID || event.PetBase != shrimp.PetBase {
		t.Errorf("fishing:success = %+v, want shrimp/petBase %d", success[0], shrimp.PetBase)
	}
}

func TestLandCatchTruncatesXP(t *testing.T) {
	f, _ := newTestFishing(t)
	player := newTestPlayer("alice", 3241, 3146)
	setLevel(player, game.SkillFishing, 99)
	tool := f.tables.Tools["MINNOW_NET"]
	player.Inventory().Adds(tool.ID, 1)
	variant := f.hasToolRequirements(player, tool)
	if variant == nil {
		t.Fatal("minnow fixture: variant must answer at 99")
	}
	minnow := f.tables.Fish["MINNOW"]
	f.landCatch(player, tool, variant, []*Fish{minnow}, 6488)
	// 26.1 XP truncates to 26 (the structural gap, same as
	// Woodcutting gap 16); 14 minnows land at max level 99.
	if got := player.Skills().Experience(game.SkillFishing); got != 26 {
		t.Errorf("minnow XP = %d, want 26 (26.1 truncated)", got)
	}
	if got := player.Inventory().GetAmount(minnow.ID); got != 14 {
		t.Errorf("minnows in inventory = %d, want 14", got)
	}
}

func TestLandCatchJunk(t *testing.T) {
	f, emitted := newTestFishing(t)
	player := newTestPlayer("alice", 3241, 3146)
	setLevel(player, game.SkillFishing, 16)
	tool := f.tables.Tools["BIG_NET"]
	player.Inventory().Adds(tool.ID, 1)
	variant := f.hasToolRequirements(player, tool)
	if variant == nil {
		t.Fatal("big net fixture: variant must answer at 16")
	}
	casket := f.tables.Fish["CASKET"]
	f.landCatch(player, tool, variant, []*Fish{casket}, 1530)
	// Junk lands, but emits fishing:junk and no
	// fishing:success.
	if len(findEmitted(*emitted, fishingJunkEventName)) != 1 {
		t.Errorf("fishing:junk emits = %d, want 1", len(findEmitted(*emitted, fishingJunkEventName)))
	}
	if len(findEmitted(*emitted, fishingSuccessEventName)) != 0 {
		t.Errorf("fishing:success emits = %d, want 0 (junk)", len(findEmitted(*emitted, fishingSuccessEventName)))
	}
	if got := player.Inventory().GetAmount(casket.ID); got != 1 {
		t.Errorf("casket in inventory = %d, want 1", got)
	}
}

func TestStorageHelpersUnanswered(t *testing.T) {
	f, _ := newTestFishing(t)
	player := newTestPlayer("alice", 3241, 3146)
	bait := f.tables.Tools["FISHING_ROD"].Bait[0]
	player.Inventory().Adds(bait, 3)
	// No storage content answers: room false, gather 0,
	// amount the inventory count, take 0.
	if f.storageRoom(player, bait) {
		t.Error("storageRoom unanswered = true, want false")
	}
	if got := f.storageGather(player, bait, 2, 0); got != 0 {
		t.Errorf("storageGather unanswered = %d, want 0", got)
	}
	if got := f.storageAmount(player, bait); got != 3 {
		t.Errorf("storageAmount unanswered = %d, want 3 (inventory count)", got)
	}
	if got := f.storageTake(player, bait, 1); got != 0 {
		t.Errorf("storageTake unanswered = %d, want 0", got)
	}
}

func TestStorageAmountAnswered(t *testing.T) {
	f, _ := newTestFishing(t)
	f.emit = func(name string, payload any) {
		if req, ok := payload.(*FishingStorageAmountRequest); ok && req != nil {
			req.Amount += 5 // a tackle box holding 5 more
		}
	}
	player := newTestPlayer("alice", 3241, 3146)
	bait := f.tables.Tools["FISHING_ROD"].Bait[0]
	player.Inventory().Adds(bait, 3)
	if got := f.storageAmount(player, bait); got != 8 {
		t.Errorf("storageAmount answered = %d, want 8 (3 inventory + 5 storage)", got)
	}
}

func TestEmitCustomEventNilSafe(t *testing.T) {
	var f *fishing
	f.emitCustomEvent("fishing:continue", nil) // nil receiver: no-op, no panic
	empty := newFishing(nil)
	empty.emitCustomEvent("fishing:continue", nil) // nil emit + nil api: no-op
}

// TestStartAndTickFishing drives one small-net session end to
// end: click the Lumbridge Swamp spot, wait for the first roll
// (tick+3), catch a shrimp, and keep fishing.
func TestStartAndTickFishing(t *testing.T) {
	f, _ := newTestFishing(t)
	f.randFloat = func() float64 { return 0 }
	f.randInt = func(min, max int) int { return 2 }
	player := newTestPlayer("alice", 3241, 3146)
	setLevel(player, game.SkillFishing, 1)
	player.Inventory().Adds(f.tables.Tools["NET"].ID, 1)
	npc := game.NewNPC(1530, game.Location{X: 3240, Y: 3146, Z: 0})
	npc.SetRegistered(true)
	event := &plugins.NpcInteractionEvent{
		Player: player, NPC: npc, NpcID: 1530, ClickType: 1, Location: npc.Location(),
		DefinitionName: "Fishing spot", Action: "Net", Actions: []string{"Net", "Bait"},
	}
	if !f.startFishingInteraction(event) {
		t.Fatal("startFishingInteraction: want the click claimed")
	}
	if _, ok := f.sessions[player]; !ok {
		t.Fatal("no session after the click")
	}
	// Ticks 1-2: no roll yet (first roll at tick+3).
	f.processFishingTick(1)
	f.processFishingTick(2)
	if got := player.Inventory().GetAmount(f.tables.Fish["SHRIMP"].ID); got != 0 {
		t.Errorf("shrimp after tick 2 = %d, want 0", got)
	}
	// Tick 3: the first roll lands a shrimp (rand 0 always
	// succeeds), 10 XP, and the session continues.
	f.processFishingTick(3)
	if got := player.Inventory().GetAmount(f.tables.Fish["SHRIMP"].ID); got != 1 {
		t.Errorf("shrimp after tick 3 = %d, want 1", got)
	}
	if got := player.Skills().Experience(game.SkillFishing); got != 10 {
		t.Errorf("Fishing XP after tick 3 = %d, want 10", got)
	}
	if _, ok := f.sessions[player]; !ok {
		t.Error("session ended after the first catch, want it to continue")
	}
	// Walking away stops fishing on the next tick.
	player.SetLocation(game.Location{X: 3300, Y: 3200, Z: 0})
	f.processFishingTick(4)
	if _, ok := f.sessions[player]; ok {
		t.Error("session survives walking away, want it stopped")
	}
}

func TestStartFishingFullInventory(t *testing.T) {
	f, _ := newTestFishing(t)
	player := newTestPlayer("alice", 3241, 3146)
	setLevel(player, game.SkillFishing, 1)
	player.Inventory().Adds(f.tables.Tools["NET"].ID, 1)
	// Fill the other 27 slots with distinct junk: a full
	// inventory that stacks none of the tool's fish.
	for i := 0; i < 27; i++ {
		player.Inventory().Adds(9000+i, 1)
	}
	if !player.Inventory().IsFull() {
		t.Fatal("fixture: inventory must be full")
	}
	npc := game.NewNPC(1530, game.Location{X: 3240, Y: 3146, Z: 0})
	npc.SetRegistered(true)
	event := &plugins.NpcInteractionEvent{
		Player: player, NPC: npc, NpcID: 1530, ClickType: 1, Location: npc.Location(),
		DefinitionName: "Fishing spot", Action: "Net", Actions: []string{"Net", "Bait"},
	}
	// The click is claimed (the full-inventory statement
	// plays) but no session starts.
	if !f.startFishingInteraction(event) {
		t.Fatal("full inventory: want the click claimed")
	}
	if _, ok := f.sessions[player]; ok {
		t.Error("full inventory: session started, want none")
	}
}

func TestStartFishingInteractionUnknownSpot(t *testing.T) {
	f, _ := newTestFishing(t)
	player := newTestPlayer("alice", 3241, 3146)
	setLevel(player, game.SkillFishing, 1)
	npc := game.NewNPC(9999, game.Location{X: 3240, Y: 3146, Z: 0})
	npc.SetRegistered(true)
	event := &plugins.NpcInteractionEvent{
		Player: player, NPC: npc, NpcID: 9999, ClickType: 1, Location: npc.Location(),
		DefinitionName: "Man", Action: "Talk-to", Actions: []string{"Talk-to"},
	}
	if f.startFishingInteraction(event) {
		t.Error("unknown spot: want the click left to others")
	}
}

func TestPauseFishing(t *testing.T) {
	f, _ := newTestFishing(t)
	player := newTestPlayer("alice", 3241, 3146)
	setLevel(player, game.SkillFishing, 1)
	player.Inventory().Adds(f.tables.Tools["NET"].ID, 1)
	npc := game.NewNPC(1530, game.Location{X: 3240, Y: 3146, Z: 0})
	npc.SetRegistered(true)
	if !f.startFishing(player, npc, f.tables.Tools["NET"]) {
		t.Fatal("fixture: startFishing must claim")
	}
	// No pop-up to hold the action: PauseForLevelUp stops it.
	f.pauseFishing(&skillactions.LevelUpPopupOpenedPayload{Player: player, Skill: game.SkillFishing, Pause: nil})
	if _, ok := f.sessions[player]; ok {
		t.Error("pause with nil Pause: session survives, want it stopped")
	}

	// With a pop-up: the session pauses, then resumes.
	if !f.startFishing(player, npc, f.tables.Tools["NET"]) {
		t.Fatal("fixture: restart must claim")
	}
	var resume func()
	var cancel func()
	f.pauseFishing(&skillactions.LevelUpPopupOpenedPayload{
		Player: player, Skill: game.SkillFishing,
		Pause: func(action any, r, c func()) { resume, cancel = r, c },
	})
	_ = cancel
	session := f.sessions[player]
	if session == nil || !session.levelUpPaused {
		t.Fatal("pause with pop-up: session not paused")
	}
	// A paused session skips its tick without catching.
	f.processFishingTick(3)
	if got := player.Inventory().GetAmount(f.tables.Fish["SHRIMP"].ID); got != 0 {
		t.Errorf("shrimp while paused = %d, want 0", got)
	}
	resume()
	if session.levelUpPaused {
		t.Error("resume: session still paused")
	}
}

func TestQuestAllowsAlwaysPasses(t *testing.T) {
	f, _ := newTestFishing(t)
	player := newTestPlayer("alice", 3241, 3146)
	// Go has no QuestRuntime: Swan Song / Barbarian Training
	// gates always pass (TS fails open for unregistered
	// quests, plugin.js:677-681).
	if !f.questAllows(player, "Swan Song") || !f.questAllows(player, "Barbarian Training") {
		t.Error("questAllows: want true for unregistered quests")
	}
}
