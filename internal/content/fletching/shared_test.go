package fletching

import (
	"encoding/json"
	"testing"

	"tsps-go/internal/game"
	"tsps-go/internal/plugins"
	"tsps-go/internal/plugins/skillactions"
)

// ---- fake ids (hand-built fixtures only; no live cache) ----

const (
	fxKnife           = 100
	fxFletchingKnife  = 101
	fxJewellersChisel = 102
	fxChisel          = 103
	fxHammer          = 104

	fxLogs          = 200
	fxOakLogs       = 201
	fxAcheyLogs     = 202
	fxArrowShaft    = 300
	fxJavelinShaft  = 301
	fxOgreShaft     = 302
	fxHeadlessArrow = 310
	fxBroadArrow    = 311

	fxBowString        = 400
	fxNotedBowString   = 401
	fxUnstrungShortbow = 500
	fxShortbow         = 501

	fxSpool = 600

	fxFeather         = 700
	fxRedFeather      = 701
	fxGryphonFeather  = 702
	fxBroadArrowheads = 710

	fxBronzeDartTip = 800
	fxBronzeDart    = 801

	fxCelastrusBark = 900
	fxBattlestaff   = 901
	fxTicksZeroOut  = 902
	fxTicksZeroIn   = 903

	fxInstantVarbit = 5000

	fxAnimFletching = 9001
	fxAnimStringing = 9002
)

func intPtr(n int) *int { return &n }

// fixtureTables builds hand-built resolved tables over the fake
// ids above: the logs recipes, stringing, headless arrows,
// bronze darts (instant), an ogre-shaft random-yield recipe
// with a quest gate, a battlestaff (Celastrus bark), a
// Slayer-gated broad-arrow recipe, and a ticks-0 recipe.
func fixtureTables() *Tables {
	makeMenu := Menu{Title: "What would you like to make?"}
	stringMenu := Menu{Title: "How many would you like to string?", Mode: intPtr(10)}
	featherMenu := Menu{Title: "How many sets of 15 do you wish to feather?", Mode: intPtr(3)}
	setsMenu := Menu{Title: "How many sets would you like to make?", Mode: intPtr(3)}
	return &Tables{
		ToolIDs: map[string][]int{
			"knife":      {fxKnife, fxFletchingKnife},
			"plainKnife": {fxKnife},
			"chisel":     {fxChisel, fxJewellersChisel},
			"hammer":     {fxHammer},
		},
		FletchingKnifeID:     fxFletchingKnife,
		JewellersChiselID:    fxJewellersChisel,
		ArrowShaftID:         fxArrowShaft,
		KnifeID:              fxKnife,
		CelastrusBarkID:      fxCelastrusBark,
		SpoolID:              fxSpool,
		BowStringID:          fxBowString,
		SpoolChargesVarbitID: 6000,
		SpoolSizeVarbitID:    6001,
		Recipes: []*Recipe{
			{
				Group: "logs", OutputID: fxArrowShaft, Amount: 15, AmountMax: 15,
				Level: 1, XP: 5, Ticks: 3, FirstTick: 2, StartTick: intPtr(0),
				KnifeBonus: true, Tool: "knife",
				Materials:   []Material{{IDs: []int{fxLogs}, Amount: 1}},
				AnimationID: fxAnimFletching, HasAnimation: true,
				Message: "You carefully cut the wood into {amount} arrow shafts.",
				MaxSets: creationAllSentinel, Menu: makeMenu,
			},
			{
				Group: "logs", OutputID: fxJavelinShaft, Amount: 15, AmountMax: 15,
				Level: 3, XP: 5, Ticks: 3, FirstTick: 2, StartTick: intPtr(0),
				KnifeBonus: true, Tool: "knife",
				Materials:   []Material{{IDs: []int{fxLogs}, Amount: 1}},
				AnimationID: fxAnimFletching, HasAnimation: true,
				Message: "You carefully cut the wood into some javelin shafts.",
				MaxSets: creationAllSentinel, Menu: makeMenu,
			},
			{
				Group: "stringing", OutputID: fxShortbow, Amount: 1, AmountMax: 1,
				Level: 5, XP: 5, Ticks: 2, FirstTick: 1, StartTick: nil,
				Materials: []Material{
					{IDs: []int{fxUnstrungShortbow}, Amount: 1},
					{IDs: []int{fxBowString}, Amount: 1},
				},
				AnimationID: fxAnimStringing, HasAnimation: true,
				Message: "You add a string to the bow.",
				MaxSets: creationAllSentinel, Menu: stringMenu,
			},
			{
				Group: "arrows", OutputID: fxHeadlessArrow, Amount: 15, AmountMax: 15,
				Level: 1, XP: 15, Ticks: 2, FirstTick: 1, StartTick: intPtr(0),
				Materials: []Material{
					{IDs: []int{fxArrowShaft}, Amount: 15},
					{IDs: []int{fxFeather, fxRedFeather, fxGryphonFeather}, UsedOnlyIDs: []int{fxGryphonFeather}, Amount: 15},
				},
				AnimationID: fxAnimFletching, HasAnimation: true,
				Message: "You attach feathers to {amount} arrow shafts.",
				MaxSets: creationAllSentinel, Menu: featherMenu,
			},
			{
				Group: "darts", OutputID: fxBronzeDart, Amount: 10, AmountMax: 10,
				Level: 10, XP: 18, Ticks: 2, FirstTick: 1, StartTick: intPtr(0),
				Materials: []Material{
					{IDs: []int{fxBronzeDartTip}, Amount: 10},
					{IDs: []int{fxFeather}, Amount: 10},
				},
				AnimationID: fxAnimFletching, HasAnimation: true,
				Message:         "You finish making {amount} darts.",
				InstantVarbitID: fxInstantVarbit, HasInstantVarbit: true,
				MaxSets: 10, Menu: setsMenu,
			},
			{
				Group: "logs", OutputID: fxOgreShaft, Amount: 2, AmountMax: 6,
				Level: 5, XP: 6.4, XPEach: 1.6, HasXPEach: true,
				Ticks: 3, FirstTick: 1, StartTick: intPtr(0),
				KnifeBonus: true, Tool: "knife",
				Materials:   []Material{{IDs: []int{fxAcheyLogs}, Amount: 1}},
				AnimationID: fxAnimFletching, HasAnimation: true,
				Quest:   &QuestGate{Key: "big_chompy_bird_hunting", State: "started"},
				MaxSets: creationAllSentinel, Menu: makeMenu,
			},
			{
				Group: "logs", OutputID: fxBattlestaff, Amount: 1, AmountMax: 1,
				Level: 40, XP: 80, Ticks: 4, FirstTick: 1, StartTick: intPtr(0),
				Tool:        "knife",
				Materials:   []Material{{IDs: []int{fxCelastrusBark}, Amount: 1}},
				AnimationID: fxAnimFletching, HasAnimation: true,
				MaxSets: creationAllSentinel, Menu: makeMenu,
			},
			{
				Group: "arrows", OutputID: fxBroadArrow, Amount: 15, AmountMax: 15,
				Level: 52, XP: 150, Ticks: 2, FirstTick: 1, StartTick: intPtr(0),
				Materials: []Material{
					{IDs: []int{fxHeadlessArrow}, Amount: 15},
					{IDs: []int{fxBroadArrowheads}, Amount: 15},
				},
				AnimationID: fxAnimFletching, HasAnimation: true,
				Unlock:  "Broader Fletching",
				MaxSets: creationAllSentinel, Menu: featherMenu,
			},
			{
				Group: "parts", OutputID: fxTicksZeroOut, Amount: 1, AmountMax: 1,
				Level: 78, XP: 120, Ticks: 0, FirstTick: 1, StartTick: intPtr(0),
				Materials:   []Material{{IDs: []int{fxTicksZeroIn}, Amount: 1}},
				AnimationID: fxAnimFletching, HasAnimation: true,
				MaxSets: creationAllSentinel, Menu: makeMenu,
			},
		},
	}
}

func recipeByOutput(tables *Tables, outputID int) *Recipe {
	for _, recipe := range tables.Recipes {
		if recipe.OutputID == outputID {
			return recipe
		}
	}
	return nil
}

// ---- fake session ----
// Implements game.SessionPort plus the ChatMessageSender,
// varbit, and DialogueSender extensions the loop reads, so
// messages, varbits, and interface removals are observable.

// fakeSession is a capturing session for content tests.
type fakeSession struct {
	messages      []string
	typedMessages []string
	varbitsSent   map[int]int
	varbitValues  map[int]int
	varpsSent     map[int]int
	removals      int
	chatboxGroups []int
	menus         []*game.CreationMenu
	menusClosed   int
}

func newFakeSession() *fakeSession {
	return &fakeSession{
		varbitsSent:  make(map[int]int),
		varbitValues: make(map[int]int),
		varpsSent:    make(map[int]int),
	}
}

// SessionPort.
func (s *fakeSession) SendPacket(packet []byte) {}
func (s *fakeSession) SendRunEnergy(units int, running bool, weightKg int, staminaTicks int) {
}
func (s *fakeSession) SendDestination(x, y int)      {}
func (s *fakeSession) SendVarp(id, value int)        { s.varpsSent[id] = value }
func (s *fakeSession) FlushTick(cycle int64) error   { return nil }
func (s *fakeSession) Close(code int, reason string) {}
func (s *fakeSession) IsOpen() bool                  { return true }

// ChatMessageSender.
func (s *fakeSession) SendGameMessage(message string) { s.messages = append(s.messages, message) }
func (s *fakeSession) SendTypedGameMessage(chatType int, message, from string) {
	s.typedMessages = append(s.typedMessages, message)
}

// Varbit access.
func (s *fakeSession) GetVarbit(id int) int { return s.varbitValues[id] }
func (s *fakeSession) SendVarbit(id, value int) {
	s.varbitsSent[id] = value
	s.varbitValues[id] = value
}

// CreationMenuSender.
func (s *fakeSession) SendCreationMenu(menu *game.CreationMenu, itemName func(id int) string, lastItem int, hasLastItem bool) {
	s.menus = append(s.menus, menu)
}
func (s *fakeSession) CloseCreationMenu() { s.menusClosed++ }

// DialogueSender.
func (s *fakeSession) SendChatboxInterface(groupID int, unclamp bool) {
	s.chatboxGroups = append(s.chatboxGroups, groupID)
}
func (s *fakeSession) SendNpcHeadOnInterface(npcID, uid int)                       {}
func (s *fakeSession) SendPlayerHeadOnInterface(uid int)                           {}
func (s *fakeSession) SendInterfaceAnimation(uid, animationID int)                 {}
func (s *fakeSession) SendString(text string, uid int)                             {}
func (s *fakeSession) SendClientScript(scriptID int, args ...game.ClientScriptArg) {}
func (s *fakeSession) SendInterfaceFlagsRange(uid, fromSlot, toSlot, flags int)    {}
func (s *fakeSession) SendItemOnInterfaces(uid, itemID, amount int)                {}
func (s *fakeSession) SendInterfaceScript(scriptID int)                            {}
func (s *fakeSession) SendInterfaceRemoval()                                       { s.removals++ }
func (s *fakeSession) IsChatboxInterface(groupID int) bool                         { return false }
func (s *fakeSession) WithChatboxOwner(owner any, present func()) {
	if present != nil {
		present()
	}
}

func (s *fakeSession) hasMessage(text string) bool {
	for _, m := range s.messages {
		if m == text {
			return true
		}
	}
	return false
}

func (s *fakeSession) hasTypedMessage(text string) bool {
	for _, m := range s.typedMessages {
		if m == text {
			return true
		}
	}
	return false
}

// ---- player fixture ----

// fakeItemDefs is a minimal ItemDefinitionSource for the
// fixtures: every fake item stacks, as the real items do
// (logs, shafts, bow strings, feathers, arrowheads — all
// stackable in the cache). Without it every fixture item is
// non-stackable (nil-source default), so 20 feathers fill 20
// of the 28 inventory slots and the loop's space check
// refuses batches the real game would make. Two deliberate
// exceptions: fxOgreShaft stays non-stackable, because the
// space-refusal test needs a non-stackable output, and the
// 9000-series junk ids stay non-stackable, because the
// spool withdraw tests fill the inventory with them.
type fakeItemDefs struct{}

func (fakeItemDefs) IsStackable(id int) bool {
	switch id {
	case fxOgreShaft:
		return false
	}
	return id < 9000
}
func (fakeItemDefs) IsNoted(id int) bool          { return false }
func (fakeItemDefs) WeightGrams(id int) int       { return 0 }
func (fakeItemDefs) EquipmentSlot(id int) int     { return -1 }
func (fakeItemDefs) IsDoubleHanded(id int) bool   { return false }
func (fakeItemDefs) Requirements(id int) []int    { return nil }
func (fakeItemDefs) CombatRequirement(id int) int { return 0 }
func (fakeItemDefs) IsTradeable(id int) bool      { return false }
func (fakeItemDefs) IsSellable(id int) bool       { return false }
func (fakeItemDefs) IsDropable(id int) bool       { return false }
func (fakeItemDefs) UnNote(id int) int            { return id }
func (fakeItemDefs) PlaceholderID(id int) int     { return -1 }
func (fakeItemDefs) NoteID(id int) int {
	// The fixture note: bow string 400 notes to 401.
	if id == fxBowString {
		return fxNotedBowString
	}
	return -1
}

func newTestPlayer(name string) *game.Player {
	p := game.NewPlayer(name, game.Location{X: 3200, Y: 3200, Z: 0})
	p.SetRegistered(true)
	p.SetItemDefinitions(fakeItemDefs{})
	setTestLevel(p, game.SkillHitpoints, 10)
	return p
}

func setTestLevel(p *game.Player, skill, level int) {
	p.Skills().SetCurrentLevel(skill, level, true)
	p.Skills().SetMaxLevel(skill, level)
}

func setFletching(p *game.Player, level int) {
	setTestLevel(p, game.SkillFletching, level)
}

// newTestFletching builds an instance over fixture tables with
// a capturing emit func and deterministic randomness.
func newTestFletching(t *testing.T) (*fletching, *[]emittedEvent, *game.TaskManager) {
	t.Helper()
	var emitted []emittedEvent
	tm := game.NewTaskManager()
	f := newFletching(fixtureTables())
	f.emit = func(name string, payload any) {
		emitted = append(emitted, emittedEvent{name: name, payload: payload})
	}
	f.tasks = tm
	f.randInt = func(min, max int) int { return min }
	return f, &emitted, tm
}

type emittedEvent struct {
	name    string
	payload any
}

func (f *fletching) emittedByName(emitted *[]emittedEvent, name string) []any {
	var out []any
	for _, e := range *emitted {
		if e.name == name {
			out = append(out, e.payload)
		}
	}
	return out
}

// driveTicks runs the task manager n cycles.
func driveTicks(tm *game.TaskManager, n int) {
	for i := 1; i <= n; i++ {
		tm.Process(int64(i))
	}
}

// ---- pair index ----

func TestPairIndexKnifeOnLogs(t *testing.T) {
	f, _, _ := newTestFletching(t)
	found := f.recipesFor(fxKnife, fxLogs)
	if len(found) != 2 {
		t.Fatalf("recipesFor(knife, logs) = %d recipes, want 2", len(found))
	}
	if found[0].OutputID != fxArrowShaft || found[1].OutputID != fxJavelinShaft {
		t.Fatalf("recipesFor order = %v, want [shafts javelin]", found)
	}
	// Fletching knife is the same tool entry: same pairs.
	if got := f.recipesFor(fxFletchingKnife, fxLogs); len(got) != 2 {
		t.Fatalf("recipesFor(fletching knife, logs) = %d, want 2", len(got))
	}
	// Bow string on unstrung bow: the stringing recipe.
	if got := f.recipesFor(fxBowString, fxUnstrungShortbow); len(got) != 1 || got[0].OutputID != fxShortbow {
		t.Fatalf("recipesFor(bow string, unstrung) = %v, want [shortbow]", got)
	}
	// Spool stands in for the bow string both ways.
	if got := f.recipesFor(fxSpool, fxUnstrungShortbow); len(got) != 1 || got[0].OutputID != fxShortbow {
		t.Fatalf("recipesFor(spool, unstrung) = %v, want [shortbow]", got)
	}
	if got := f.recipesFor(fxUnstrungShortbow, fxSpool); len(got) != 1 || got[0].OutputID != fxShortbow {
		t.Fatalf("recipesFor(unstrung, spool) = %v, want [shortbow]", got)
	}
}

func TestPairIndexUsedOnlyFeatherExcludedAsTarget(t *testing.T) {
	f, _, _ := newTestFletching(t)
	// Gryphon feather used ON arrow shafts: the headless-arrow
	// recipe starts (usedOnly only bars it as the target).
	if got := f.recipesFor(fxGryphonFeather, fxArrowShaft); len(got) != 1 {
		t.Fatalf("recipesFor(gryphon, shafts) = %d, want 1", len(got))
	}
	// Arrow shafts used ON a gryphon feather: the pair is known
	// (indexed) but recipesFor drops it — the feather is
	// usedOnly as the target.
	if got := f.recipesFor(fxArrowShaft, fxGryphonFeather); len(got) != 0 {
		t.Fatalf("recipesFor(shafts, gryphon) = %d, want 0", len(got))
	}
	if !f.hasPair(fxArrowShaft, fxGryphonFeather) {
		t.Fatal("hasPair(shafts, gryphon) = false, want true (known pair, filtered recipes)")
	}
}

func TestRecipesForUnknownPairEmpty(t *testing.T) {
	f, _, _ := newTestFletching(t)
	if got := f.recipesFor(fxLogs, fxBowString); len(got) != 0 {
		t.Fatalf("recipesFor(logs, bow string) = %d, want 0", len(got))
	}
	if f.hasPair(fxLogs, fxBowString) {
		t.Fatal("hasPair(logs, bow string) = true, want false")
	}
}

// ---- handlePair: fall-through and refusal paths ----

func TestHandlePairUnknownPairReturnsFalse(t *testing.T) {
	f, _, _ := newTestFletching(t)
	p := newTestPlayer("alice")
	p.Session = newFakeSession()
	if f.handlePair(p, fxLogs, fxBowString) {
		t.Fatal("handlePair(unknown pair) = true, want false (falls through)")
	}
}

func TestHandlePairQuestGatedOnlySaysNothingInteresting(t *testing.T) {
	f, _, _ := newTestFletching(t)
	p := newTestPlayer("alice")
	session := newFakeSession()
	p.Session = session
	setFletching(p, 99)
	p.Inventory().Adds(fxKnife, 1)
	p.Inventory().Adds(fxAcheyLogs, 5)
	// The ogre-shaft recipe is the only recipe knife+achey logs
	// starts, and its quest gate is unanswered → fail-closed.
	if !f.handlePair(p, fxKnife, fxAcheyLogs) {
		t.Fatal("handlePair(quest-gated pair) = false, want true (took the click)")
	}
	if !session.hasMessage("Nothing interesting happens.") {
		t.Fatalf("messages = %v, want 'Nothing interesting happens.'", session.messages)
	}
	if p.Inventory().GetAmount(fxOgreShaft) != 0 {
		t.Fatal("ogre shafts made despite the locked quest gate")
	}
}

func TestHandlePairSlayerGatedOnlySaysNothingInteresting(t *testing.T) {
	f, _, _ := newTestFletching(t)
	p := newTestPlayer("alice")
	session := newFakeSession()
	p.Session = session
	setFletching(p, 99)
	p.Inventory().Adds(fxHeadlessArrow, 15)
	p.Inventory().Adds(fxBroadArrowheads, 15)
	if !f.handlePair(p, fxHeadlessArrow, fxBroadArrowheads) {
		t.Fatal("handlePair(slayer-gated pair) = false, want true")
	}
	if !session.hasMessage("Nothing interesting happens.") {
		t.Fatalf("messages = %v, want 'Nothing interesting happens.'", session.messages)
	}
}

func TestHandlePairMissingToolMessageWhenAllChoicesLackTools(t *testing.T) {
	f, _, _ := newTestFletching(t)
	p := newTestPlayer("alice")
	session := newFakeSession()
	p.Session = session
	setFletching(p, 99)
	p.Inventory().Adds(fxLogs, 5)
	// No knife: both log recipes lack tools.
	if !f.handlePair(p, fxKnife, fxLogs) {
		t.Fatal("handlePair = false, want true")
	}
	if !session.hasMessage("You'll need a knife to do that.") {
		t.Fatalf("messages = %v, want the knife line", session.messages)
	}
}

func TestHandlePairPresetOnlySaysPresetText(t *testing.T) {
	f, _, _ := newTestFletching(t)
	p := newTestPlayer("alice")
	session := newFakeSession()
	p.Session = session
	setFletching(p, 99)
	p.Inventory().Adds(fxKnife, 1)
	// Only a preset copy of the logs: held (raw count) covers
	// the recipe, but plainAmount sees nothing to take.
	presetLogs := game.NewItemWithMeta(fxLogs, 5, map[string]json.RawMessage{game.ItemPresetMeta: json.RawMessage("true")})
	p.Inventory().AddItem(presetLogs)
	if !f.handlePair(p, fxKnife, fxLogs) {
		t.Fatal("handlePair = false, want true")
	}
	if !session.hasMessage(presetText) {
		t.Fatalf("messages = %v, want %q", session.messages, presetText)
	}
	if p.Inventory().GetAmount(fxArrowShaft) != 0 {
		t.Fatal("preset logs converted into shafts")
	}
}

func TestHandlePairEmptySpoolSilent(t *testing.T) {
	f, _, _ := newTestFletching(t)
	p := newTestPlayer("alice")
	session := newFakeSession()
	p.Session = session
	setFletching(p, 99)
	p.Inventory().Adds(fxSpool, 1)
	p.Inventory().Adds(fxUnstrungShortbow, 1)
	// The spool is the only bow-string source and it holds 0:
	// usable is empty, nothing is held, so silence — not even
	// "Nothing interesting happens."
	if !f.handlePair(p, fxSpool, fxUnstrungShortbow) {
		t.Fatal("handlePair(spool, unstrung) = false, want true")
	}
	if len(session.messages) != 0 {
		t.Fatalf("messages = %v, want none (empty spool is silent)", session.messages)
	}
}

func TestHandlePairLevelGateMessage(t *testing.T) {
	f, _, _ := newTestFletching(t)
	p := newTestPlayer("alice")
	session := newFakeSession()
	p.Session = session
	setFletching(p, 1)
	p.Inventory().Adds(fxKnife, 1)
	p.Inventory().Adds(fxCelastrusBark, 1)
	// Single recipe, 1 batch available: the fast path calls
	// startFletching, whose level gate speaks first.
	if !f.handlePair(p, fxKnife, fxCelastrusBark) {
		t.Fatal("handlePair = false, want true")
	}
	if !session.hasMessage("You need a Fletching level of at least 40 to do this.") {
		t.Fatalf("messages = %v, want the level line", session.messages)
	}
}

// ---- makeBatch ----

func TestMakeBatchStringingConsumesAndPaysXP(t *testing.T) {
	f, emitted, _ := newTestFletching(t)
	p := newTestPlayer("alice")
	session := newFakeSession()
	p.Session = session
	setFletching(p, 99)
	p.Inventory().Adds(fxUnstrungShortbow, 1)
	p.Inventory().Adds(fxBowString, 1)
	recipe := recipeByOutput(f.tables, fxShortbow)
	if !f.makeBatch(p, recipe, fxBowString, nil) {
		t.Fatal("makeBatch = false, want true")
	}
	if p.Inventory().GetAmount(fxShortbow) != 1 {
		t.Fatalf("shortbows = %d, want 1", p.Inventory().GetAmount(fxShortbow))
	}
	if p.Inventory().GetAmount(fxUnstrungShortbow) != 0 || p.Inventory().GetAmount(fxBowString) != 0 {
		t.Fatal("materials were not consumed")
	}
	if got := p.Skills().Experience(game.SkillFletching); got != 5 {
		t.Fatalf("Fletching XP = %d, want 5", got)
	}
	if !session.hasTypedMessage("You add a string to the bow.") {
		t.Fatalf("typed messages = %v, want the stringing line", session.typedMessages)
	}
	made := f.emittedByName(emitted, "fletching:made")
	if len(made) != 1 {
		t.Fatalf("fletching:made emits = %d, want 1", len(made))
	}
	produced := f.emittedByName(emitted, "skills:produced")
	if len(produced) != 1 {
		t.Fatalf("skills:produced emits = %d, want 1", len(produced))
	}
	ev := produced[0].(*SkillsProducedEvent)
	if ev.ItemID != fxShortbow || ev.Amount != 1 || ev.XP != 5 || ev.Kind != "stringing" {
		t.Fatalf("skills:produced = %+v, want shortbow x1 xp 5 stringing", ev)
	}
	if ev.StationObjectID != nil {
		t.Fatalf("stationObjectId = %v, want nil", ev.StationObjectID)
	}
}

func TestMakeBatchTruncatesFloatXP(t *testing.T) {
	f, _, _ := newTestFletching(t)
	p := newTestPlayer("alice")
	p.Session = newFakeSession()
	setFletching(p, 99)
	p.Inventory().Adds(fxKnife, 1)
	p.Inventory().Adds(fxAcheyLogs, 1)
	recipe := recipeByOutput(f.tables, fxOgreShaft)
	// Answer the quest gate so the level check is the only gate
	// in play here — makeBatch does not check quests (handlePair
	// filters them), so this is a pure XP check.
	f.emit = func(name string, payload any) {}
	// Random roll: randInt returns min (0) → amount 2, xpEach
	// 1.6 * 2 = 3.2 → truncated to 3.
	if !f.makeBatch(p, recipe, fxKnife, nil) {
		t.Fatal("makeBatch = false, want true")
	}
	if got := p.Inventory().GetAmount(fxOgreShaft); got != 2 {
		t.Fatalf("ogre shafts = %d, want 2 (min roll)", got)
	}
	if got := p.Skills().Experience(game.SkillFletching); got != 3 {
		t.Fatalf("Fletching XP = %d, want 3 (3.2 truncated)", got)
	}
}

func TestMakeBatchSpaceRefusalConsumesNothing(t *testing.T) {
	f, _, _ := newTestFletching(t)
	p := newTestPlayer("alice")
	session := newFakeSession()
	p.Session = session
	setFletching(p, 99)
	// Fill the inventory with unstackable junk, leaving the
	// stringing materials in two of the slots: after consuming
	// them only 2 slots free, but the batch must fit at
	// amountMax — here amountMax is 1, so force the refusal
	// differently: use the dart recipe (amountMax 10) with a
	// full inventory of junk + the materials. After consuming
	// 10 tips + 10 feathers (20 slots freed as stacks? tips and
	// feathers are separate stacks...) — simplest honest case:
	// fill 27 slots with junk, 1 slot with the unstrung bow,
	// and no bow string in inventory (spool empty) — then
	// batchesAvailable is 0 and makeBatch refuses silently.
	// Instead: put the bow string in last, fill the rest, and
	// check the amountMax rule with the ogre recipe (amountMax
	// 6, non-stackable output): 27 junk + 1 achey log, knife
	// worn. Consuming the log frees 1 slot; 6 shafts need 6
	// slots → refuse.
	for i := 0; i < 27; i++ {
		p.Inventory().Adds(9000+i, 1)
	}
	p.Inventory().Adds(fxAcheyLogs, 1)
	p.Equipment().Adds(fxKnife, 1)
	recipe := recipeByOutput(f.tables, fxOgreShaft)
	if f.makeBatch(p, recipe, fxKnife, nil) {
		t.Fatal("makeBatch = true, want false (no room at amountMax 6)")
	}
	if !session.hasMessage("You don't have enough inventory space.") {
		t.Fatalf("messages = %v, want the space refusal", session.messages)
	}
	if p.Inventory().GetAmount(fxAcheyLogs) != 1 {
		t.Fatal("achey log was consumed despite the refusal")
	}
}

func TestMakeBatchPartialScaling(t *testing.T) {
	f, _, _ := newTestFletching(t)
	p := newTestPlayer("alice")
	p.Session = newFakeSession()
	setFletching(p, 99)
	// 7 shafts + 7 feathers: no full set of 15, one partial
	// batch of 7.
	p.Inventory().Adds(fxArrowShaft, 7)
	p.Inventory().Adds(fxFeather, 7)
	recipe := recipeByOutput(f.tables, fxHeadlessArrow)
	if got := f.fullBatches(p, recipe); got != 0 {
		t.Fatalf("fullBatches = %d, want 0", got)
	}
	if got := f.partialSize(p, recipe); got != 7 {
		t.Fatalf("partialSize = %d, want 7", got)
	}
	if !f.makeBatch(p, recipe, fxFeather, nil) {
		t.Fatal("makeBatch = false, want true (partial batch)")
	}
	if got := p.Inventory().GetAmount(fxHeadlessArrow); got != 7 {
		t.Fatalf("headless arrows = %d, want 7", got)
	}
	// XP scales with the partial size: 15 xp * 7/15 = 7.0.
	if got := p.Skills().Experience(game.SkillFletching); got != 7 {
		t.Fatalf("Fletching XP = %d, want 7 (scaled 15*7/15)", got)
	}
}

// ---- material planning ----

func TestMaterialPlanVirtualFirstWhenViaIsAlias(t *testing.T) {
	f, _, _ := newTestFletching(t)
	p := newTestPlayer("alice")
	p.Session = newFakeSession()
	setFletching(p, 99)
	p.Inventory().Adds(fxUnstrungShortbow, 1)
	p.Inventory().Adds(fxBowString, 1)
	p.Inventory().Adds(fxSpool, 1)
	f.setSpoolCharges(p, 5)
	recipe := recipeByOutput(f.tables, fxShortbow)
	plan := f.materialPlan(p, recipe, fxSpool)
	if !plan.complete {
		t.Fatal("plan.complete = false, want true")
	}
	// Virtual-first: the bow string comes from the spool, and
	// the inventory string is untouched.
	if len(plan.entries) != 2 {
		t.Fatalf("plan entries = %v, want 2", plan.entries)
	}
	if plan.entries[1].itemID != fxBowString || !plan.entries[1].virtual {
		t.Fatalf("plan[1] = %+v, want bow string virtual", plan.entries[1])
	}
	if !f.makeBatch(p, recipe, fxSpool, nil) {
		t.Fatal("makeBatch = false, want true")
	}
	if got := spoolCharges(p); got != 4 {
		t.Fatalf("spool charges = %d, want 4", got)
	}
	if got := p.Inventory().GetAmount(fxBowString); got != 1 {
		t.Fatalf("inventory bow strings = %d, want 1 (untouched)", got)
	}
}

func TestMaterialPlanInventoryFirstOtherwise(t *testing.T) {
	f, _, _ := newTestFletching(t)
	p := newTestPlayer("alice")
	p.Session = newFakeSession()
	setFletching(p, 99)
	p.Inventory().Adds(fxUnstrungShortbow, 1)
	p.Inventory().Adds(fxBowString, 1)
	p.Inventory().Adds(fxSpool, 1)
	f.setSpoolCharges(p, 5)
	recipe := recipeByOutput(f.tables, fxShortbow)
	plan := f.materialPlan(p, recipe, fxBowString)
	if !plan.complete {
		t.Fatal("plan.complete = false, want true")
	}
	if plan.entries[1].virtual {
		t.Fatalf("plan[1] = %+v, want inventory-first (not virtual)", plan.entries[1])
	}
	if !f.makeBatch(p, recipe, fxBowString, nil) {
		t.Fatal("makeBatch = false, want true")
	}
	if got := spoolCharges(p); got != 5 {
		t.Fatalf("spool charges = %d, want 5 (untouched)", got)
	}
	if got := p.Inventory().GetAmount(fxBowString); got != 0 {
		t.Fatalf("inventory bow strings = %d, want 0", got)
	}
}

func TestTakeOrderClickedIdFirst(t *testing.T) {
	f, _, _ := newTestFletching(t)
	material := Material{IDs: []int{fxFeather, fxRedFeather, fxGryphonFeather}, Amount: 15}
	got := f.takeOrder(material, fxRedFeather)
	if len(got) != 3 || got[0] != fxRedFeather || got[1] != fxFeather || got[2] != fxGryphonFeather {
		t.Fatalf("takeOrder = %v, want [red feather ...]", got)
	}
	// An id outside the material keeps data order.
	if got := f.takeOrder(material, fxLogs); len(got) != 3 || got[0] != fxFeather {
		t.Fatalf("takeOrder(other) = %v, want data order", got)
	}
}

func TestTakePlainSkipsRestrictedInSlotOrder(t *testing.T) {
	p := newTestPlayer("alice")
	p.Session = newFakeSession()
	// Slot 0: preset logs x3 (skipped). Slot 1: plain logs x2.
	preset := game.NewItemWithMeta(fxLogs, 3, map[string]json.RawMessage{game.ItemPresetMeta: json.RawMessage("true")})
	p.Inventory().AddItem(preset)
	p.Inventory().Adds(fxLogs, 2)
	if got := takePlain(p.Inventory(), fxLogs, 2); got != 2 {
		t.Fatalf("takePlain = %d, want 2", got)
	}
	if got := p.Inventory().GetAmount(fxLogs); got != 3 {
		t.Fatalf("logs left = %d, want 3 (the preset copies)", got)
	}
}

// ---- level / actions queries ----

func TestFletchingLevelBoostQuery(t *testing.T) {
	f, _, _ := newTestFletching(t)
	p := newTestPlayer("alice")
	p.Session = newFakeSession()
	setFletching(p, 39)
	// No answerer: level is the current level.
	if got := f.fletchingLevel(p); got != 39 {
		t.Fatalf("fletchingLevel = %d, want 39", got)
	}
	// An answerer raising the boost by 12 lets a level-39
	// player meet a level-40 gate.
	f.emit = func(name string, payload any) {
		if req, ok := payload.(*LevelBoostRequest); ok {
			req.Boost = 12
		}
	}
	if got := f.fletchingLevel(p); got != 51 {
		t.Fatalf("fletchingLevel = %d, want 51 (39+12)", got)
	}
}

func TestActionsPerCycleQuery(t *testing.T) {
	f, _, _ := newTestFletching(t)
	p := newTestPlayer("alice")
	p.Session = newFakeSession()
	recipe := recipeByOutput(f.tables, fxBronzeDart)
	if got := f.actionsPerCycle(p, recipe); got != 1 {
		t.Fatalf("actionsPerCycle = %d, want 1 (unanswered)", got)
	}
	f.emit = func(name string, payload any) {
		if req, ok := payload.(*ActionsPerCycleRequest); ok {
			req.Actions = 3
		}
	}
	if got := f.actionsPerCycle(p, recipe); got != 3 {
		t.Fatalf("actionsPerCycle = %d, want 3", got)
	}
}

// ---- instant path ----

func TestHandlePairInstantDartsOneSetPerClick(t *testing.T) {
	f, _, _ := newTestFletching(t)
	p := newTestPlayer("alice")
	session := newFakeSession()
	p.Session = session
	setFletching(p, 99)
	// Instant varbit reads 0 by default (no session value).
	p.Inventory().Adds(fxBronzeDartTip, 20)
	p.Inventory().Adds(fxFeather, 20)
	if !f.handlePair(p, fxBronzeDartTip, fxFeather) {
		t.Fatal("handlePair = false, want true")
	}
	// One set of 10, no task, no menu.
	if got := p.Inventory().GetAmount(fxBronzeDart); got != 10 {
		t.Fatalf("bronze darts = %d, want 10", got)
	}
	if len(f.running) != 0 {
		t.Fatal("a task is running after the instant path")
	}
	if got := p.Skills().Experience(game.SkillFletching); got != 18 {
		t.Fatalf("Fletching XP = %d, want 18", got)
	}
}

func TestHandlePairInstantVarbitSetOpensMenuInstead(t *testing.T) {
	f, _, _ := newTestFletching(t)
	p := newTestPlayer("alice")
	session := newFakeSession()
	p.Session = session
	session.varbitValues[fxInstantVarbit] = 1
	setFletching(p, 99)
	p.Inventory().Adds(fxBronzeDartTip, 20)
	p.Inventory().Adds(fxFeather, 20)
	if !f.handlePair(p, fxBronzeDartTip, fxFeather) {
		t.Fatal("handlePair = false, want true")
	}
	if p.Inventory().GetAmount(fxBronzeDart) != 0 {
		t.Fatal("darts were made immediately despite the varbit being set")
	}
	if len(session.menus) != 1 {
		t.Fatalf("menus opened = %d, want 1", len(session.menus))
	}
	menu := session.menus[0]
	if got := menu.Options().MaxAmount; got == nil || *got != 2 {
		t.Fatalf("menu maxAmount = %v, want 2 (20 tips / 10 per set)", got)
	}
	if got := menu.Options().Mode; got != 3 {
		t.Fatalf("menu mode = %d, want 3 (sets menu)", got)
	}
	if got := menu.Items(); len(got) != 1 || got[0] != fxBronzeDart {
		t.Fatalf("menu items = %v, want [bronze dart]", got)
	}
}

// ---- task ----

func TestOpenMenuMaxAmountTitleAndExecute(t *testing.T) {
	f, _, _ := newTestFletching(t)
	p := newTestPlayer("alice")
	session := newFakeSession()
	p.Session = session
	setFletching(p, 99)
	p.Inventory().Adds(fxKnife, 1)
	p.Inventory().Adds(fxLogs, 3)
	if !f.handlePair(p, fxKnife, fxLogs) {
		t.Fatal("handlePair = false, want true")
	}
	if len(session.menus) != 1 {
		t.Fatalf("menus opened = %d, want 1", len(session.menus))
	}
	menu := session.menus[0]
	// 3 logs → 3 batches of each; the make menu's title has
	// no {amount}, so it reads verbatim.
	if menu.Title() != "What would you like to make?" {
		t.Fatalf("menu title = %q", menu.Title())
	}
	if got := menu.Options().MaxAmount; got == nil || *got != 3 {
		t.Fatalf("menu maxAmount = %v, want 3", got)
	}
	if got := menu.Items(); len(got) != 2 || got[0] != fxArrowShaft || got[1] != fxJavelinShaft {
		t.Fatalf("menu items = %v, want [shafts javelin]", got)
	}
	if got := menu.Options().Mode; got != 0 {
		t.Fatalf("menu mode = %d, want 0 (make menu sets none)", got)
	}
	// Choosing arrow shafts for 2 starts the task.
	menu.Execute(fxArrowShaft, 2)
	task, ok := f.running[p]
	if !ok || task == nil {
		t.Fatal("no task running after choosing from the menu")
	}
	if task.remaining != 2 || task.batches != 2 {
		t.Fatalf("task batches = %d remaining = %d, want 2/2", task.batches, task.remaining)
	}
	// Choosing an item the menu does not offer runs nothing
	// (CreationMenu.execute's includes-check).
	menu.Execute(fxShortbow, 1)
	if task.remaining != 2 {
		t.Fatal("executing an unoffered item changed the task")
	}
}

func TestStartFletchingTicksZeroMakesImmediately(t *testing.T) {
	f, _, _ := newTestFletching(t)
	p := newTestPlayer("alice")
	p.Session = newFakeSession()
	setFletching(p, 99)
	p.Inventory().Adds(fxTicksZeroIn, 1)
	recipe := recipeByOutput(f.tables, fxTicksZeroOut)
	f.startFletching(p, recipe, 1, fxTicksZeroIn, nil)
	if got := p.Inventory().GetAmount(fxTicksZeroOut); got != 1 {
		t.Fatalf("output = %d, want 1 (immediate, no task)", got)
	}
	if len(f.running) != 0 {
		t.Fatal("a task is running for a ticks-0 recipe")
	}
}

func TestTaskMakesBatchesOnFirstTickPacing(t *testing.T) {
	f, _, tm := newTestFletching(t)
	p := newTestPlayer("alice")
	p.Session = newFakeSession()
	setFletching(p, 99)
	p.Inventory().Adds(fxKnife, 1)
	p.Inventory().Adds(fxLogs, 5)
	recipe := recipeByOutput(f.tables, fxArrowShaft)
	// firstTick 2, ticks 3, no fletching knife carried: batches
	// on task ticks 2 and 5.
	f.startFletching(p, recipe, 2, fxKnife, nil)
	if len(f.running) != 1 {
		t.Fatal("no task running after startFletching")
	}
	driveTicks(tm, 1)
	if got := p.Inventory().GetAmount(fxArrowShaft); got != 0 {
		t.Fatalf("shafts after tick 1 = %d, want 0 (firstTick 2)", got)
	}
	driveTicks(tm, 1) // tick 2
	if got := p.Inventory().GetAmount(fxArrowShaft); got != 15 {
		t.Fatalf("shafts after tick 2 = %d, want 15", got)
	}
	driveTicks(tm, 2) // ticks 3, 4: pacing
	if got := p.Inventory().GetAmount(fxArrowShaft); got != 15 {
		t.Fatalf("shafts after tick 4 = %d, want 15 (next batch on tick 5)", got)
	}
	driveTicks(tm, 1) // tick 5
	if got := p.Inventory().GetAmount(fxArrowShaft); got != 30 {
		t.Fatalf("shafts after tick 5 = %d, want 30", got)
	}
	if len(f.running) != 0 {
		t.Fatal("task still running after the last batch")
	}
}

func TestTaskKnifeSavingAfterFirstTwoShaftLogs(t *testing.T) {
	f, _, tm := newTestFletching(t)
	p := newTestPlayer("alice")
	p.Session = newFakeSession()
	setFletching(p, 99)
	p.Inventory().Adds(fxKnife, 1)
	p.Inventory().Adds(fxFletchingKnife, 1)
	p.Inventory().Adds(fxLogs, 5)
	recipe := recipeByOutput(f.tables, fxArrowShaft)
	// Arrow shafts with a fletching knife: the first two logs
	// use the normal pace (ticks 2, 5), the third takes the
	// knife saving (tick 7, not 8).
	f.startFletching(p, recipe, 3, fxKnife, nil)
	driveTicks(tm, 2) // tick 2: batch 1
	if got := p.Inventory().GetAmount(fxArrowShaft); got != 15 {
		t.Fatalf("shafts after tick 2 = %d, want 15", got)
	}
	driveTicks(tm, 3) // tick 5: batch 2 (no saving yet)
	if got := p.Inventory().GetAmount(fxArrowShaft); got != 30 {
		t.Fatalf("shafts after tick 5 = %d, want 30", got)
	}
	driveTicks(tm, 1) // tick 6: no batch (saving applies from here)
	if got := p.Inventory().GetAmount(fxArrowShaft); got != 30 {
		t.Fatalf("shafts after tick 6 = %d, want 30", got)
	}
	driveTicks(tm, 1) // tick 7: batch 3 (interval 3-1=2 after tick 5)
	if got := p.Inventory().GetAmount(fxArrowShaft); got != 45 {
		t.Fatalf("shafts after tick 7 = %d, want 45 (knife saving)", got)
	}
	if len(f.running) != 0 {
		t.Fatal("task still running after the last batch")
	}
}

func TestTaskStopsWhenMaterialsRunOut(t *testing.T) {
	f, _, tm := newTestFletching(t)
	p := newTestPlayer("alice")
	p.Session = newFakeSession()
	setFletching(p, 99)
	p.Inventory().Adds(fxKnife, 1)
	p.Inventory().Adds(fxLogs, 1)
	recipe := recipeByOutput(f.tables, fxArrowShaft)
	f.startFletching(p, recipe, 5, fxKnife, nil)
	driveTicks(tm, 2)
	if got := p.Inventory().GetAmount(fxArrowShaft); got != 15 {
		t.Fatalf("shafts = %d, want 15", got)
	}
	if len(f.running) != 0 {
		t.Fatal("task still running after materials ran out")
	}
}

func TestStopFletchingOnDisconnect(t *testing.T) {
	f, _, _ := newTestFletching(t)
	p := newTestPlayer("alice")
	p.Session = newFakeSession()
	setFletching(p, 99)
	p.Inventory().Adds(fxKnife, 1)
	p.Inventory().Adds(fxLogs, 5)
	recipe := recipeByOutput(f.tables, fxArrowShaft)
	f.startFletching(p, recipe, 5, fxKnife, nil)
	if len(f.running) != 1 {
		t.Fatal("no task running before disconnect")
	}
	f.onPlayerDisconnect(&plugins.PlayerDisconnectEvent{Player: p, Username: "alice"})
	if len(f.running) != 0 {
		t.Fatal("task still running after disconnect")
	}
}

func TestPauseFletchingHoldsAndResumes(t *testing.T) {
	f, _, tm := newTestFletching(t)
	p := newTestPlayer("alice")
	p.Session = newFakeSession()
	setFletching(p, 99)
	p.Inventory().Adds(fxKnife, 1)
	p.Inventory().Adds(fxLogs, 5)
	recipe := recipeByOutput(f.tables, fxArrowShaft)
	f.startFletching(p, recipe, 2, fxKnife, nil)
	task := f.running[p]
	// No Pause on the event: the action stops instead of
	// pausing (SkillActions.js pauseForLevelUp).
	f.pauseFletching(&skillactions.LevelUpPopupOpenedPayload{Player: p, Skill: game.SkillFletching})
	if len(f.running) != 0 {
		t.Fatal("task still running after a pauseless level-up event")
	}
	// With a Pause: the session is held, ticks skip it, and
	// resume clears the flag.
	f.startFletching(p, recipe, 2, fxKnife, nil)
	task = f.running[p]
	var resume func()
	var cancel func()
	f.pauseFletching(&skillactions.LevelUpPopupOpenedPayload{
		Player: p,
		Skill:  game.SkillFletching,
		Pause: func(action any, r, c func()) {
			resume, cancel = r, c
		},
	})
	_ = cancel
	if !task.levelUpPaused {
		t.Fatal("levelUpPaused = false, want true while held")
	}
	driveTicks(tm, 3)
	if got := p.Inventory().GetAmount(fxArrowShaft); got != 0 {
		t.Fatalf("shafts while paused = %d, want 0", got)
	}
	resume()
	if task.levelUpPaused {
		t.Fatal("levelUpPaused still true after resume")
	}
	driveTicks(tm, 2)
	if got := p.Inventory().GetAmount(fxArrowShaft); got != 15 {
		t.Fatalf("shafts after resume = %d, want 15", got)
	}
}

func TestAnswerShafts(t *testing.T) {
	f, _, _ := newTestFletching(t)
	req := &ShaftsRequest{LogID: fxLogs}
	f.answerShafts(req)
	if req.ItemID != fxArrowShaft || req.Amount != 15 || req.XP != 5 {
		t.Fatalf("shafts answer = %+v, want arrow shaft x15 xp 5", req)
	}
	// A log with no arrow-shaft recipe answers nothing.
	req = &ShaftsRequest{LogID: fxOakLogs}
	f.answerShafts(req)
	if req.ItemID != 0 || req.Amount != 0 || req.XP != 0 {
		t.Fatalf("shafts answer for oak = %+v, want zeros", req)
	}
	// A non-pointer payload is ignored, not a panic.
	f.answerShafts("not a request")
}

func TestNilTablesDegradeToNoop(t *testing.T) {
	f := newFletching(nil)
	p := newTestPlayer("alice")
	p.Session = newFakeSession()
	if f.handlePair(p, fxKnife, fxLogs) {
		t.Fatal("handlePair with nil tables claimed the click")
	}
	if f.makeBatch(p, nil, fxKnife, nil) {
		t.Fatal("makeBatch(nil recipe) = true")
	}
	f.startFletching(p, nil, 1, fxKnife, nil)
	f.stopFletching(p, true)
	f.pauseFletching(nil)
	f.answerShafts(&ShaftsRequest{LogID: fxLogs})
}

func TestIntervalActionIntervalQuery(t *testing.T) {
	f, _, _ := newTestFletching(t)
	p := newTestPlayer("alice")
	p.Session = newFakeSession()
	recipe := recipeByOutput(f.tables, fxArrowShaft)
	// Unanswered: the recipe pace (3), no knife saving (no
	// fletching knife carried).
	if got := f.interval(p, recipe, false); got != 3 {
		t.Fatalf("interval = %d, want 3", got)
	}
	// Content lowers the pace to 2; the knife saving still
	// applies after it.
	f.emit = func(name string, payload any) {
		if req, ok := payload.(*skillactions.ActionIntervalPayload); ok {
			req.Ticks = 2
		}
	}
	p.Inventory().Adds(fxFletchingKnife, 1)
	if got := f.interval(p, recipe, false); got != 1 {
		t.Fatalf("interval = %d, want 1 (2 lowered, minus knife saving, floor 1)", got)
	}
	// delayKnifeSaving suppresses the knife tick.
	if got := f.interval(p, recipe, true); got != 2 {
		t.Fatalf("interval(delayKnifeSaving) = %d, want 2", got)
	}
}

func TestToolIntervalJewellersChisel(t *testing.T) {
	f, _, _ := newTestFletching(t)
	p := newTestPlayer("alice")
	p.Session = newFakeSession()
	recipe := &Recipe{Ticks: 5, JewellersTicks: intPtr(2)}
	if got := f.toolInterval(p, recipe); got != 5 {
		t.Fatalf("toolInterval without chisel = %d, want 5", got)
	}
	p.Inventory().Adds(fxJewellersChisel, 1)
	if got := f.toolInterval(p, recipe); got != 2 {
		t.Fatalf("toolInterval with jeweller's chisel = %d, want 2", got)
	}
}
