package crafting

import (
	"testing"

	"tsps-go/internal/game"
	"tsps-go/internal/plugins"
	"tsps-go/internal/session"
)

// ---- fake ids (hand-built fixtures only; no live cache) ----

const (
	fxNeedle            = 1001
	fxCostumeNeedle     = 1002
	fxThread            = 1003
	fxLeather           = 1010
	fxHardLeather       = 1011
	fxLeatherGloves     = 1012
	fxLeatherBoots      = 1013
	fxCoif              = 1014
	fxFoxFur            = 1020
	fxSmallMeatPouch    = 1021
	fxLargeMeatPouch    = 1022
	fxAntelopeFur       = 1023
	fxSteelStuds        = 1030
	fxLeatherBody       = 1031
	fxStuddedBody       = 1032
	fxKebbitClaws       = 1033
	fxLeatherVambraces  = 1034
	fxSpikyVambraces    = 1035
	fxHammer            = 1036
	fxImcandoHammer     = 1037
	fxImcandoOffHand    = 1038
	fxHardLeatherShield = 1039
	fxOakShield         = 1040
	fxBronzeNails       = 1041

	fxChisel            = 1101
	fxJewellersChisel   = 1102
	fxUncutSapphire     = 1103
	fxSapphire          = 1104
	fxUncutOpal         = 1105
	fxOpal              = 1106
	fxCrushedGem        = 1107
	fxAmethyst          = 1108
	fxAmethystBoltTips  = 1109
	fxAmethystArrowtips = 1110

	fxGoldBar           = 1201
	fxSilverBar         = 1202
	fxRingMould         = 1203
	fxNecklaceMould     = 1204
	fxAmuletMould       = 1205
	fxBraceletMould     = 1206
	fxGoldRing          = 1207
	fxSapphireRing      = 1208
	fxGoldNecklace      = 1209
	fxGoldAmuletU       = 1210
	fxGoldAmulet        = 1211
	fxOpalRing          = 1212
	fxOpalGem           = 1213
	fxPerfectGoldBar    = 1214
	fxRuby              = 1215
	fxPerfectRing       = 1216
	fxPerfectNecklace   = 1217
	fxBallOfWool        = 1218
	fxSapphireAmuletU   = 1227
	fxSapphireAmulet    = 1228
	fxUnstrungSymbol    = 1229
	fxUnblessedSymbol   = 1230
	fxSapphireNecklace  = 1231
	fxEternalGem        = 1220
	fxEnchantedGem      = 1221
	fxSlayerRing8       = 1222
	fxSlayerRingEternal = 1223
	fxSilverSickle      = 1224
	fxJadeRing          = 1225
	fxJade              = 1226

	fxGlassblowingPipe = 1301
	fxMoltenGlass      = 1302
	fxBeerGlass        = 1303
	fxVial             = 1304

	fxClay          = 1401
	fxSoftClay      = 1402
	fxBucketOfWater = 1403
	fxBucket        = 1404
	fxUnfiredPot    = 1405
	fxPot           = 1406
	fxUnfiredBowl   = 1407
	fxBowl          = 1408
	fxUnfiredCup    = 1409
	fxEmptyCup      = 1410
	fxUnfiredPotLid = 1411
	fxPotLid        = 1412
	fxBowlOfWater   = 1413
	fxJugOfWater    = 1414
	fxJug           = 1415

	fxWool           = 1501
	fxFlax           = 1502
	fxBowString      = 1503
	fxSinew          = 1504
	fxCrossbowString = 1505
	fxJuteFibre      = 1506
	fxStripOfCloth   = 1507
	fxMagicString    = 1508

	fxWaterOrb         = 1601
	fxBattlestaff      = 1602
	fxWaterBattlestaff = 1603

	fxMyreShell = 1701
	fxMyreSnelm = 1702

	fxLimestone      = 1801
	fxLimestoneBrick = 1802

	fxEmptyLightOrb  = 1901
	fxCaveGoblinWire = 1902
	fxLightOrb       = 1903

	fxButtons         = 2001
	fxPolishedButtons = 2002

	fxSilverSickleB  = 2101
	fxEmerald        = 2102
	fxEmeraldSickleB = 2103
	fxRubySickleB    = 2104
	fxDiamond        = 2105
	fxDiamondSickleB = 2106

	fxSilverBolts = 1232

	fxFurnaceObj = 3001
	fxWheelObj   = 3002
	fxOvenObj    = 3003
	fxLoomObj    = 3004

	fxSeqLeather     = 9001
	fxSeqSapphire    = 9002
	fxSeqOpal        = 9003
	fxSeqAmethyst    = 9004
	fxSeqGlass       = 9005
	fxSeqWheel       = 9006
	fxSeqOven        = 9007
	fxSeqSpinning    = 9008
	fxSeqWheelObj    = 9009
	fxSeqLoom        = 9010
	fxSeqBattlestaff = 9011
	fxSeqFurnace     = 9012
	fxSeqLimestone   = 9013
	fxSeqEmerald     = 9014
	fxSeqRuby        = 9015
	fxSeqDiamond     = 9016
	fxSeqSpiky       = 9017
)

// ---- fake item definitions ----

type fakeItemDefs struct {
	names map[int]string
}

func (f fakeItemDefs) IsStackable(id int) bool {
	// Silver bolts (unf) are the fixture's non-stackable
	// output, for the jewellery space-preview refusal test.
	return id != fxSilverBolts
}
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
func (f fakeItemDefs) NoteID(id int) int            { return -1 }
func (f fakeItemDefs) ItemName(id int) string       { return f.names[id] }
func (f fakeItemDefs) IsMembersItem(id int) bool    { return false }

func testItemDefs() fakeItemDefs {
	return fakeItemDefs{names: map[int]string{
		fxNeedle: "Needle", fxThread: "Thread",
		fxLeather: "Leather", fxHardLeather: "Hard leather",
		fxHammer: "Hammer", fxBronzeNails: "Bronze nails",
		fxSteelStuds: "Steel studs", fxLeatherBody: "Leather body",
		fxKebbitClaws: "Kebbit claws", fxLeatherVambraces: "Leather vambraces",
		fxOakShield: "Oak shield",
		fxChisel:    "Chisel", fxJewellersChisel: "Jeweller's chisel",
		fxUncutSapphire: "Uncut sapphire", fxUncutOpal: "Uncut opal",
		fxAmethyst:      "Amethyst",
		fxLeatherGloves: "Leather gloves", fxLeatherBoots: "Leather boots",
		fxCoif: "Coif", fxSmallMeatPouch: "Small meat pouch",
		fxLargeMeatPouch: "Large meat pouch", fxFoxFur: "Fox fur",
		fxAntelopeFur: "Sunlight antelope fur", fxStuddedBody: "Studded body",
		fxSpikyVambraces: "Spiky vambraces", fxHardLeatherShield: "Hard leather shield",
		fxSapphire: "Sapphire", fxOpal: "Opal", fxAmethystBoltTips: "Amethyst bolt tips",
		fxAmethystArrowtips: "Amethyst arrowtips",
		fxGoldRing:          "Gold ring", fxSapphireRing: "Sapphire ring",
		fxGoldNecklace: "Gold necklace", fxOpalRing: "Opal ring",
		fxWaterBattlestaff: "Water battlestaff", fxMyreSnelm: "Myre snelm",
		fxBowString: "Bow string", fxBallOfWool: "Ball of wool",
		fxCrossbowString: "Crossbow string", fxStripOfCloth: "Strip of cloth",
		fxLimestoneBrick: "Limestone brick", fxLightOrb: "Light orb",
		fxButtons: "Buttons", fxPolishedButtons: "Polished buttons",
		fxEmeraldSickleB: "Emerald sickle (b)", fxRubySickleB: "Ruby sickle (b)",
		fxSilverBolts:    "Silver bolts (unf)",
		fxDiamondSickleB: "Diamond sickle (b)", fxSilverSickleB: "Silver sickle (b)",
	}}
}

// ---- fake session ----
// Implements game.SessionPort plus the ChatMessageSender,
// varbit, CreationMenuSender, DialogueSender and sub-interface
// extensions, capturing what content sends (the smithing /
// fletching fakeSession precedent).

type mountedInterface struct {
	targetUID int
	groupID   int
	typ       int
}

type fakeSession struct {
	messages      []string
	spam          []string
	varbits       map[int]int
	varps         map[int]int
	menus         []*game.CreationMenu
	mounts        []mountedInterface
	closed        []int
	packets       [][]byte
	chatboxGroups []int
}

func newFakeSession() *fakeSession {
	return &fakeSession{
		varbits: make(map[int]int),
		varps:   make(map[int]int),
	}
}

func (s *fakeSession) SendPacket(packet []byte) { s.packets = append(s.packets, packet) }
func (s *fakeSession) SendRunEnergy(units int, running bool, weightKg int, staminaTicks int) {
}
func (s *fakeSession) SendDestination(x, y int) {}
func (s *fakeSession) SendVarp(id, value int)   { s.varps[id] = value }
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
func (s *fakeSession) GetVarbit(id int) int { return s.varbits[id] }
func (s *fakeSession) SendVarbit(id, value int) {
	s.varbits[id] = value
}
func (s *fakeSession) SendCreationMenu(menu *game.CreationMenu, itemName func(id int) string, lastItem int, hasLastItem bool) {
	s.menus = append(s.menus, menu)
}
func (s *fakeSession) CloseCreationMenu() {}
func (s *fakeSession) SendChatboxInterface(groupID int, unclamp bool) {
	s.chatboxGroups = append(s.chatboxGroups, groupID)
}
func (s *fakeSession) SendNpcHeadOnInterface(npcID, uid int)       {}
func (s *fakeSession) SendPlayerHeadOnInterface(uid int)           {}
func (s *fakeSession) SendInterfaceAnimation(uid, animationID int) {}
func (s *fakeSession) SendString(text string, uid int)             {}
func (s *fakeSession) SendClientScript(scriptID int, args ...game.ClientScriptArg) {
}
func (s *fakeSession) SendInterfaceFlagsRange(uid, fromSlot, toSlot, flags int) {}
func (s *fakeSession) SendItemOnInterfaces(uid, itemID, amount int)             {}
func (s *fakeSession) SendInterfaceScript(scriptID int)                         {}
func (s *fakeSession) SendInterfaceRemoval()                                    {}
func (s *fakeSession) IsChatboxInterface(groupID int) bool {
	for _, g := range s.chatboxGroups {
		if g == groupID {
			return true
		}
	}
	return false
}
func (s *fakeSession) WithChatboxOwner(owner any, present func()) {
	if present != nil {
		present()
	}
}
func (s *fakeSession) MountSubInterface(targetUID, groupID, typ int) *session.SubInterfaceMount {
	s.mounts = append(s.mounts, mountedInterface{targetUID, groupID, typ})
	return &session.SubInterfaceMount{TargetUID: targetUID, GroupID: groupID, Type: typ}
}
func (s *fakeSession) CloseSubInterface(targetUID int) {
	s.closed = append(s.closed, targetUID)
}

func (s *fakeSession) hasMessage(text string) bool {
	for _, m := range s.messages {
		if m == text {
			return true
		}
	}
	return false
}

func (s *fakeSession) hasSpam(text string) bool {
	for _, m := range s.spam {
		if m == text {
			return true
		}
	}
	return false
}

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

func setCrafting(p *game.Player, level int) { setTestLevel(p, game.SkillCrafting, level) }

func testPlayer(name string) (*game.Player, *fakeSession) {
	p := newTestPlayer(name)
	session := newFakeSession()
	p.Session = session
	return p, session
}

type emittedEvent struct {
	name    string
	payload any
}

// newTestCrafting builds an instance over fixture tables with
// a capturing emit func and a fresh TaskManager.
func newTestCrafting() (*crafting, *[]emittedEvent, *game.TaskManager) {
	var emitted []emittedEvent
	tm := game.NewTaskManager()
	c := newCrafting(fixtureTables())
	c.emit = func(name string, payload any) {
		emitted = append(emitted, emittedEvent{name: name, payload: payload})
	}
	c.tasks = tm
	return c, &emitted, tm
}

// driveCraftingTicks runs processCraftingTick for the next n
// cycles (the smithing driveTicks precedent).
func driveCraftingTicks(c *crafting, n int) {
	for i := 0; i < n; i++ {
		c.tick++
		c.processCraftingTick(c.tick)
	}
}

// driveTaskTicks runs the TaskManager n cycles (the cooking
// driveTicks precedent: nextTick and PotteryTask runs).
func driveTaskTicks(tm *game.TaskManager, n int) {
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

func itemStatementText(p *game.Player) (int, string, bool) {
	dm := p.GetDialogueManager()
	if dm == nil || !dm.IsActive() {
		return 0, "", false
	}
	if stmt, ok := dm.GetCurrent().(*game.ItemStatementDialogue); ok {
		return stmt.GetItemID(), stmt.GetText(), true
	}
	return 0, "", false
}

func testGameObject(id int, loc game.Location) *game.GameObject {
	return game.NewGameObject(id, loc, 10, 0, nil)
}

// testObjectDef is a hand-built ObjectDefinition for hub
// object-interaction tests.
type testObjectDef struct {
	id           int
	name         string
	interactions []string
}

func (d testObjectDef) ID() int                { return d.id }
func (d testObjectDef) Name() string           { return d.name }
func (d testObjectDef) SizeX() int             { return 1 }
func (d testObjectDef) SizeY() int             { return 1 }
func (d testObjectDef) BlockingMask() int      { return 0 }
func (d testObjectDef) Interactions() []string { return d.interactions }

// ---- fixture tables ----

func fixtureTables() *Tables {
	return &Tables{
		Leather: []LeatherMaterial{
			{MaterialID: fxLeather, MaterialName: "LEATHER", Recipes: []LeatherRecipe{
				{ItemID: fxLeatherGloves, Leather: 1, Level: 1, XP: 13.8, Made: "a pair of leather gloves"},
				{ItemID: fxLeatherBoots, Leather: 1, Level: 7, XP: 16.25, Made: "a pair of leather boots"},
				{ItemID: fxCoif, Leather: 1, Level: 38, XP: 37, Made: "a coif", Members: true},
			}},
			{MaterialID: fxHardLeather, MaterialName: "HARD_LEATHER", Recipes: []LeatherRecipe{
				{ItemID: fxLeatherBody, Leather: 1, Level: 14, XP: 25, Made: "a leather body"},
			}},
		},
		HuntersPouches: []HunterPouchRecipe{
			{FurID: fxFoxFur, Amount: 4, ItemID: fxSmallMeatPouch, Level: 35, XP: 30, Members: true},
			{FurID: fxAntelopeFur, Amount: 3, FromID: fxSmallMeatPouch, HasFrom: true, ItemID: fxLargeMeatPouch, Level: 65, XP: 60, Members: true},
		},
		Combine: []CombineRecipe{
			{Pair: [2]int{fxSteelStuds, fxLeatherBody}, PairAmounts: [2]int{1, 1}, ItemID: fxStuddedBody, Level: 41, XP: 40, Ticks: 3, SeqID: fxSeqLeather, SeqName: "human_leather_crafting", Members: true},
			{Pair: [2]int{fxKebbitClaws, fxLeatherVambraces}, PairAmounts: [2]int{1, 1}, ItemID: fxSpikyVambraces, Level: 32, XP: 6, Ticks: 3, SeqID: fxSeqSpiky, SeqName: "human_crafting_spikedvambraces", Members: true},
			{Pair: [2]int{fxHardLeather, fxOakShield}, PairAmounts: [2]int{2, 1}, Materials: []Ingredient{{ItemID: fxBronzeNails, Amount: 15}}, ToolIDs: []int{fxHammer}, ItemID: fxHardLeatherShield, Level: 41, XP: 70, Ticks: 5, SeqID: fxSeqLeather, SeqName: "human_leather_crafting", Members: true},
		},
		Gems: []GemRecipe{
			{UncutID: fxUncutSapphire, CutID: fxSapphire, Plural: "sapphires", Level: 20, XP: 50, SeqID: fxSeqSapphire, SeqName: "human_sapphirecutting"},
			{UncutID: fxUncutOpal, CutID: fxOpal, Plural: "opals", Level: 1, XP: 15, SeqID: fxSeqOpal, SeqName: "human_opalcutting", Members: true, Crush: CrushData{Low: 128, High: 250, XP: 3.8}, HasCrush: true},
		},
		Amethyst: AmethystData{SeqID: fxSeqAmethyst, SeqName: "human_amethystcutting", Recipes: []AmethystRecipe{
			{ItemID: fxAmethystBoltTips, Amount: 15, Level: 83, XP: 60, Members: true},
			{ItemID: fxAmethystArrowtips, Amount: 15, Level: 85, XP: 60, Members: true},
		}},
		Glassblowing: GlassblowingData{SeqID: fxSeqGlass, SeqName: "human_glassblowing", SoundID: 2724, SoundDelay: 10, Recipes: []GlassblowingRecipe{
			{ItemID: fxBeerGlass, Label: "Beer glass", Name: "beer glass", Level: 1, XP: 17.5, Members: true},
			{ItemID: fxVial, Label: "Vial", Name: "vial", Level: 33, XP: 35, Members: true},
		}},
		Pottery: PotteryData{
			SeqID: fxSeqWheel, SeqName: "human_potterywheel",
			OvenSeqID: fxSeqOven, OvenSeqName: "potteryoven_quick",
			SoundID: 2588, OvenSoundID: 2725,
			Recipes: []PotteryRecipe{
				{Name: "pot", Plural: "pots", UnfiredID: fxUnfiredPot, FiredID: fxPot, Level: 1, ShapingXP: 6.3, FiringXP: 6.3, Shaped: 1, Cracks: true},
				{Name: "bowl", Plural: "bowls", UnfiredID: fxUnfiredBowl, FiredID: fxBowl, Level: 8, ShapingXP: 18, FiringXP: 15, Shaped: 1, Cracks: true,
					ShapeMessage: "You make the clay into a bowl.", PutMessage: "You put a bowl in the oven.", RemoveMessage: "You remove the bowl from the oven."},
				{Name: "cup", Plural: "cups", UnfiredID: fxUnfiredCup, FiredID: fxEmptyCup, Level: 3, ShapingXP: 8.5, FiringXP: 8.5, Members: true, Shaped: 4, ShapeLabel: "Set of cups", Cracks: false,
					ShapeMessage: "You make the clay into a set of cups.", PutMessage: "You put an item in the oven.", RemoveMessage: "You remove the item from the oven."},
				{Name: "pot lid", Plural: "pot lids", UnfiredID: fxUnfiredPotLid, FiredID: fxPotLid, Level: 25, ShapingXP: 20, FiringXP: 20, Members: true, Shaped: 1, Cracks: true, UnlockQuest: "one_small_favour", UnlockStage: 34, HasUnlock: true},
			},
			WaterContainers: []Ingredient{
				{ItemID: fxBucketOfWater, Amount: fxBucket},
				{ItemID: fxBowlOfWater, Amount: fxBowl},
				{ItemID: fxJugOfWater, Amount: fxJug},
			},
		},
		Spinning: SpinningData{
			SeqID: fxSeqSpinning, SeqName: "human_spinningwheel_60",
			WheelSeqID: fxSeqWheelObj, WheelSeqName: "spinningwheel", SoundID: 2590,
			Recipes: []SpinningRecipe{
				{ItemID: fxBallOfWool, MaterialIDs: []int{fxWool}, Amount: 1, Level: 1, XP: 2.5},
				{ItemID: fxBowString, MaterialIDs: []int{fxFlax}, Amount: 1, Level: 10, XP: 15},
				{ItemID: fxCrossbowString, MaterialIDs: []int{fxSinew}, Amount: 1, Level: 10, XP: 15},
			},
		},
		Weaving: WeavingData{
			SeqID: fxSeqLoom, SeqName: "farming_useloom", SoundID: 2587, SoundDelay: 2,
			Recipes: []WeavingRecipe{
				{ItemID: fxStripOfCloth, MaterialIDs: []int{fxBallOfWool}, Amount: 4, Level: 10, XP: 12, Members: true},
			},
		},
		Gold: GoldData{
			GemIDs: []int{-1, fxSapphire},
			Types: []GoldType{
				{Name: "ring", Components: []int{8, 9}, MouldID: fxRingMould, Products: []GoldProduct{
					{ItemID: fxGoldRing, Level: 5, XP: 15},
					{ItemID: fxSapphireRing, Level: 20, XP: 50},
				}},
				{Name: "necklace", Components: []int{23, 24}, MouldID: fxNecklaceMould, Products: []GoldProduct{
					{ItemID: fxGoldNecklace, Level: 6, XP: 20},
					{ItemID: fxSapphireNecklace, Level: 22, XP: 55},
				}},
			},
			SlayerRing: GoldSlayerRing{
				Component: 16, MouldID: fxRingMould, Level: 75, XP: 15, UnlockVarbit: "slayer_ring_unlocked",
				Products: []Ingredient{
					{ItemID: fxEternalGem, Amount: fxSlayerRingEternal},
					{ItemID: fxEnchantedGem, Amount: fxSlayerRing8},
				},
			},
			Quantities: map[int]int{60: 1, 61: 5, 62: 10, 64: 28},
			XComponent: 63,
		},
		Silver: SilverData{
			Products: []SilverProduct{
				{Component: 7, ComponentName: "opal_ring", ItemID: fxOpalRing, Level: 1, XP: 10, MouldID: fxRingMould, ExtraIDs: []int{fxOpalGem}, Amount: 1},
				{Component: 8, ComponentName: "jade_ring", ItemID: fxJadeRing, Level: 13, XP: 32, MouldID: fxRingMould, ExtraIDs: []int{fxJade}, Amount: 1, Members: true},
				{Component: 27, ComponentName: "silver_bolts_(unf)", ItemID: fxSilverBolts, Level: 1, XP: 10, MouldID: fxRingMould, Amount: 10},
			},
			Quantities: map[int]int{32: 1, 33: 5, 34: 10, 36: 28},
			XComponent: 35,
		},
		Perfect: []PerfectRecipe{
			{Name: "'perfect' ring", ItemID: fxPerfectRing, MouldID: fxRingMould, Level: 40, XP: 70},
			{Name: "'perfect' necklace", ItemID: fxPerfectNecklace, MouldID: fxNecklaceMould, Level: 40, XP: 75},
		},
		StrungAmulets: StrungAmuletData{
			XP: 4,
			Pairs: []StrungAmuletPair{
				{UnstrungID: fxGoldAmuletU, StrungID: fxGoldAmulet, MaterialID: fxBallOfWool},
				{UnstrungID: fxSapphireAmuletU, StrungID: fxSapphireAmulet, MaterialID: fxBallOfWool},
				{UnstrungID: fxUnstrungSymbol, StrungID: fxUnblessedSymbol, Noun: "symbol", MaterialID: fxBallOfWool},
			},
		},
		Battlestaves: BattlestavesData{
			SeqID: fxSeqBattlestaff, SeqName: "human_battlestaff_crafting",
			Recipes: []BattlestavesRecipe{
				{OrbID: fxWaterOrb, ItemID: fxWaterBattlestaff, Level: 54, XP: 100},
			},
		},
		Snelms: SnelmsData{
			Level: 15, XP: 32.5, Ticks: 2, SeqID: fxSeqOpal, SeqName: "human_opalcutting",
			Recipes: []SnelmRecipe{{ShellID: fxMyreShell, ItemID: fxMyreSnelm}},
		},
		GemSickles: []GemSickleRecipe{
			{GemName: "Emerald", Name: "emerald", Article: "an", ProductID: fxEmeraldSickleB, XP: 20, SeqID: fxSeqEmerald, SeqName: "human_emeraldcutting"},
			{GemName: "Ruby", Name: "ruby", Article: "a", ProductID: fxRubySickleB, XP: 26, SeqID: fxSeqRuby, SeqName: "human_rubycutting"},
			{GemName: "Diamond", Name: "diamond", Article: "a", ProductID: fxDiamondSickleB, XP: 32, SeqID: fxSeqDiamond, SeqName: "human_diamondcutting"},
		},
		ItemIDs: map[string]int{
			"NEEDLE": fxNeedle, "COSTUME_NEEDLE": fxCostumeNeedle, "THREAD": fxThread,
			"LEATHER": fxLeather, "HARD_LEATHER": fxHardLeather,
			"LEATHER_GLOVES": fxLeatherGloves, "LEATHER_BOOTS": fxLeatherBoots, "COIF": fxCoif,
			"FOX_FUR": fxFoxFur, "SMALL_MEAT_POUCH": fxSmallMeatPouch, "LARGE_MEAT_POUCH": fxLargeMeatPouch,
			"SUNLIGHT_ANTELOPE_FUR": fxAntelopeFur, "STEEL_STUDS": fxSteelStuds, "LEATHER_BODY": fxLeatherBody,
			"STUDDED_BODY": fxStuddedBody, "KEBBIT_CLAWS": fxKebbitClaws, "LEATHER_VAMBRACES": fxLeatherVambraces,
			"SPIKY_VAMBRACES": fxSpikyVambraces, "HAMMER": fxHammer, "IMCANDO_HAMMER": fxImcandoHammer,
			"IMCANDO_HAMMER_OFF_HAND_": fxImcandoOffHand, "HARD_LEATHER_SHIELD": fxHardLeatherShield,
			"OAK_SHIELD": fxOakShield, "BRONZE_NAILS": fxBronzeNails,
			"CHISEL": fxChisel, "JEWELLERS_CHISEL": fxJewellersChisel,
			"UNCUT_SAPPHIRE": fxUncutSapphire, "SAPPHIRE": fxSapphire, "UNCUT_OPAL": fxUncutOpal, "OPAL": fxOpal,
			"CRUSHED_GEM": fxCrushedGem, "AMETHYST": fxAmethyst,
			"AMETHYST_BOLT_TIPS": fxAmethystBoltTips, "AMETHYST_ARROWTIPS": fxAmethystArrowtips,
			"GOLD_BAR": fxGoldBar, "SILVER_BAR": fxSilverBar, "RING_MOULD": fxRingMould,
			"NECKLACE_MOULD": fxNecklaceMould, "AMULET_MOULD": fxAmuletMould, "BRACELET_MOULD": fxBraceletMould,
			"GOLD_RING": fxGoldRing, "SAPPHIRE_RING": fxSapphireRing, "GOLD_NECKLACE": fxGoldNecklace,
			"GOLD_AMULET_U_": fxGoldAmuletU, "GOLD_AMULET": fxGoldAmulet, "OPAL_RING": fxOpalRing,
			"PERFECT_GOLD_BAR": fxPerfectGoldBar, "RUBY": fxRuby, "PERFECT_RING": fxPerfectRing,
			"PERFECT_NECKLACE": fxPerfectNecklace, "BALL_OF_WOOL": fxBallOfWool,
			"SAPPHIRE_AMULET_U_": fxSapphireAmuletU, "SAPPHIRE_AMULET": fxSapphireAmulet,
			"UNSTRUNG_SYMBOL": fxUnstrungSymbol, "UNBLESSED_SYMBOL": fxUnblessedSymbol,
			"SAPPHIRE_NECKLACE": fxSapphireNecklace, "ETERNAL_GEM": fxEternalGem, "ENCHANTED_GEM": fxEnchantedGem,
			"SLAYER_RING_8_": fxSlayerRing8, "SLAYER_RING_ETERNAL_": fxSlayerRingEternal,
			"SILVER_SICKLE": fxSilverSickle, "JADE_RING": fxJadeRing, "JADE": fxJade,
			"GLASSBLOWING_PIPE": fxGlassblowingPipe, "MOLTEN_GLASS": fxMoltenGlass,
			"BEER_GLASS": fxBeerGlass, "VIAL": fxVial,
			"CLAY": fxClay, "SOFT_CLAY": fxSoftClay, "BUCKET_OF_WATER": fxBucketOfWater, "BUCKET": fxBucket,
			"UNFIRED_POT": fxUnfiredPot, "POT": fxPot, "UNFIRED_BOWL": fxUnfiredBowl, "BOWL": fxBowl,
			"UNFIRED_CUP": fxUnfiredCup, "EMPTY_CUP": fxEmptyCup, "UNFIRED_POT_LID": fxUnfiredPotLid,
			"POT_LID": fxPotLid, "BOWL_OF_WATER": fxBowlOfWater, "JUG_OF_WATER": fxJugOfWater, "JUG": fxJug,
			"WOOL": fxWool, "FLAX": fxFlax, "BOW_STRING": fxBowString, "SINEW": fxSinew,
			"CROSSBOW_STRING": fxCrossbowString, "JUTE_FIBRE": fxJuteFibre, "STRIP_OF_CLOTH": fxStripOfCloth,
			"MAGIC_STRING": fxMagicString,
			"WATER_ORB":    fxWaterOrb, "BATTLESTAFF": fxBattlestaff, "WATER_BATTLESTAFF": fxWaterBattlestaff,
			"BLAMISH_MYRE_SHELL": fxMyreShell, "MYRE_SNELM": fxMyreSnelm,
			"LIMESTONE": fxLimestone, "LIMESTONE_BRICK": fxLimestoneBrick,
			"EMPTY_LIGHT_ORB": fxEmptyLightOrb, "CAVE_GOBLIN_WIRE": fxCaveGoblinWire, "LIGHT_ORB": fxLightOrb,
			"BUTTONS": fxButtons, "POLISHED_BUTTONS": fxPolishedButtons,
			"SILVER_SICKLE_B_": fxSilverSickleB, "EMERALD": fxEmerald, "EMERALD_SICKLE_B_": fxEmeraldSickleB,
			"RUBY_SICKLE_B_": fxRubySickleB, "DIAMOND": fxDiamond, "DIAMOND_SICKLE_B_": fxDiamondSickleB,
		},
		SeqIDs: map[string]int{
			"human_leather_crafting": fxSeqLeather, "human_sapphirecutting": fxSeqSapphire,
			"human_opalcutting": fxSeqOpal, "human_amethystcutting": fxSeqAmethyst,
			"human_glassblowing": fxSeqGlass, "human_potterywheel": fxSeqWheel,
			"potteryoven_quick": fxSeqOven, "human_spinningwheel_60": fxSeqSpinning,
			"spinningwheel": fxSeqWheelObj, "farming_useloom": fxSeqLoom,
			"human_battlestaff_crafting": fxSeqBattlestaff, "human_furnace": fxSeqFurnace,
			"human_limestonecutting": fxSeqLimestone, "human_emeraldcutting": fxSeqEmerald,
			"human_rubycutting": fxSeqRuby, "human_diamondcutting": fxSeqDiamond,
			"human_crafting_spikedvambraces": fxSeqSpiky,
		},
		VarbitIDs: map[string]int{
			"crafting_gold_item_lasttype":   13892,
			"crafting_silver_item_lasttype": 13893,
			"slayer_ring_unlocked":          3207,
		},
		GoldLastType:   map[int]int{(446 << 16) | 8: 1, (446 << 16) | 9: 2},
		SilverLastType: map[int]int{(6 << 16) | 7: 1},
	}
}

// hubWith registers the test instance's plugin on a hub and
// loads it, for emit-level tests.
func hubWith(t *testing.T, c *crafting) *plugins.Hub {
	t.Helper()
	hub := plugins.NewHub([]plugins.Plugin{c.plugin()}, plugins.HubOptions{MembersWorld: true})
	hub.Load()
	return hub
}
