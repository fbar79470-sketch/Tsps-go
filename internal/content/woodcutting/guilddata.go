// Guild data for the Woodcutting Guild sub-units: the `ents` and
// `shrine` sections of
// server/plugins/skills/data/woodcutting-guild.json (upstream
// commit bfd81e3), transcribed into Go tables with JSON line
// citations, exactly as Phase 32 transcribed the plugin.js tables
// into tables.go. The JSON file itself is NEVER copied into the
// repo (see PHASE33_PLAN.md, "Guild-data decision"): verbatim
// file imports need Tyler's approval, while ported values in Go
// source with citations are the normal porting method. The
// `gates`, `cave`, `vine`, `roots`, and `diary` sections of that
// file belong to the Guild unit and are NOT transcribed here.
// Every id below was verified against the real rev-242 seed
// cache (see guilddata_test.go / the temporary probe, deleted
// after use).
//
// Line numbers cite woodcutting-guild.json unless another file
// is named. The tables are read-only after init; EntTrunk and
// Shrine consume them.
package woodcutting

import "tsps-go/internal/game"

// EntNPC is one Ent spawn (woodcutting-guild.json:25-26): where
// it stands, how many logs each success yields there, and how
// many ticks after its remains expire it respawns.
type EntNPC struct {
	// Where is the capture's location label ("Woodcutting
	// Guild" / "Wilderness"); it is also the `where` the
	// woodcutting:ent-logs emit carries.
	Where string
	// LogsPerSuccess is 1 in the guild, 2 in the Wilderness.
	LogsPerSuccess int
	// RespawnTicks is 50 in the guild, 15 in the Wilderness.
	RespawnTicks int
}

// EntLog is one log an Ent trunk can yield
// (woodcutting-guild.json:35-40): the id, the name the yield
// message prints, the base Woodcutting levels it is open to
// (MaxLevel 99 when the JSON omits it — the TS `?? 99`),
// and the roll weight, plus the per-axe-tier bonus weight
// (magic logs only).
type EntLog struct {
	ID               int
	Name             string
	MinLevel         int
	MaxLevel         int
	Weight           int
	WeightPerAxeTier int
}

// EntMessages are the Ent trunk's chat lines
// (woodcutting-guild.json:43-46).
type EntMessages struct {
	Swing string
	NoAxe string
	Full  string
	Yield string
}

// EntsTable is the `ents` section
// (woodcutting-guild.json:23-48).
type EntsTable struct {
	// NPCs maps the Ent NPC id to its spawn (7234 guild,
	// 6594 wilderness).
	NPCs map[int]EntNPC
	// TrunkID is the trunk NPC id (9474).
	TrunkID int
	// TransformTicks is 4: the Ent becomes a trunk four
	// ticks after it dies (two after the death event).
	TransformTicks int
	// TrunkTicks is 101: how long the trunk lasts.
	TrunkTicks int
	// ChopTicks is 3: one chop attempt per session every
	// three ticks.
	ChopTicks int
	// XP is 25 per success.
	XP int
	// CutChance is the eight-row success chart, one
	// [low, high] row per axe tier bronze..dragon.
	CutChance CutChart
	// Logs are the six yieldable logs, in JSON order.
	Logs []EntLog
	// Messages are the trunk's chat lines.
	Messages EntMessages
}

// Ents is the transcribed `ents` section
// (woodcutting-guild.json:23-48). The cutChance rows are
// [12, 37], [18, 56], [24, 75], [27, 84], [30, 94],
// [36, 112], [42, 131], [45, 140] (line 33).
var Ents = EntsTable{
	NPCs: map[int]EntNPC{
		7234: {Where: "Woodcutting Guild", LogsPerSuccess: 1, RespawnTicks: 50},
		6594: {Where: "Wilderness", LogsPerSuccess: 2, RespawnTicks: 15},
	},
	TrunkID:        9474,
	TransformTicks: 4,
	TrunkTicks:     101,
	ChopTicks:      3,
	XP:             25,
	CutChance: CutChart{
		{Low: 12, High: 37, Has: true},
		{Low: 18, High: 56, Has: true},
		{Low: 24, High: 75, Has: true},
		{Low: 27, High: 84, Has: true},
		{Low: 30, High: 94, Has: true},
		{Low: 36, High: 112, Has: true},
		{Low: 42, High: 131, Has: true},
		{Low: 45, High: 140, Has: true},
	},
	Logs: []EntLog{
		{ID: 1511, Name: "Logs", MinLevel: 1, MaxLevel: 44, Weight: 4},
		{ID: 1521, Name: "Oak logs", MinLevel: 15, MaxLevel: 99, Weight: 4},
		{ID: 1519, Name: "Willow logs", MinLevel: 30, MaxLevel: 99, Weight: 1},
		{ID: 1517, Name: "Maple logs", MinLevel: 45, MaxLevel: 99, Weight: 1},
		{ID: 1515, Name: "Yew logs", MinLevel: 61, MaxLevel: 99, Weight: 1},
		{ID: 1513, Name: "Magic logs", MinLevel: 75, MaxLevel: 99, Weight: 1, WeightPerAxeTier: 1},
	},
	Messages: EntMessages{
		Swing: "You swing your axe at the Ent trunk.",
		NoAxe: "You don't have an axe which you can use.",
		Full:  "Your inventory is too full to carry any more logs.",
		Yield: "The ent carcass yields: {amount} x {log}",
	},
}

// ShrineEgg is one bird's egg the shrine accepts
// (woodcutting-guild.json:57-59): its colour, the
// projectile id the offering flies, and the sound id.
type ShrineEgg struct {
	Colour     string
	Projectile int
	Sound      int
}

// ShrineProjectile is the offering's flight
// (woodcutting-guild.json:55): offsets from the shrine tile,
// the start/end cycles, the arc angle, and the heights.
type ShrineProjectile struct {
	FromX, FromY int
	ToX, ToY     int
	StartCycle   int
	EndCycle     int
	Angle        int
	StartHeight  int
	EndHeight    int
}

// ShrineMessages are the shrine's chat lines
// (woodcutting-guild.json:63-65). Count carries a {count}
// placeholder.
type ShrineMessages struct {
	Offer    string
	CountOne string
	Count    string
}

// ShrineTable is the `shrine` section
// (woodcutting-guild.json:49-67).
type ShrineTable struct {
	// ID is the shrine object id (29088).
	ID int
	// Anim is the offering animation (3705).
	Anim int
	// XP is the Prayer XP per offering (100).
	XP int
	// Nest is the seed-nest reward id (22798).
	Nest int
	// Projectile is the offering's flight.
	Projectile ShrineProjectile
	// Eggs maps the egg item id to its offering (5076 red,
	// 5077 blue, 5078 green).
	Eggs map[int]ShrineEgg
	// EvilChickenChance is 1200: each evil chicken piece
	// rolls 1/1200.
	EvilChickenChance int
	// EvilChickenPieces are the four evil chicken piece ids,
	// in JSON order.
	EvilChickenPieces []int
	// Messages are the shrine's chat lines.
	Messages ShrineMessages
}

// Shrine is the transcribed `shrine` section
// (woodcutting-guild.json:49-67). The `fullPrayer` line
// ("You already have full Prayer Points.", line 51) is NOT
// transcribed: Shrine.Woodcutting.js never reads it —
// offering at full Prayer still grants the XP (the Prayer
// cap is the SkillManager's) — so transcribing it would
// invent a use.
var Shrine = ShrineTable{
	ID:   29088,
	Anim: 3705,
	XP:   100,
	Nest: 22798,
	Projectile: ShrineProjectile{
		FromX: 1, FromY: 1, ToX: 2, ToY: 1,
		StartCycle: 0, EndCycle: 140, Angle: 50,
		StartHeight: 25, EndHeight: 112,
	},
	Eggs: map[int]ShrineEgg{
		5076: {Colour: "red", Projectile: 1308, Sound: 3047},
		5077: {Colour: "blue", Projectile: 1307, Sound: 1984},
		5078: {Colour: "green", Projectile: 1309, Sound: 1986},
	},
	EvilChickenChance: 1200,
	EvilChickenPieces: []int{20433, 20436, 20439, 20442},
	Messages: ShrineMessages{
		Offer:    "You offer your bird's egg to the shrine and receive a reward.",
		CountOne: "You have made <col=ff0000>one</col> offering.",
		Count:    "You have made <col=ff0000>{count}</col> offerings.",
	},
}

// ShrineOfferingsAttribute is the persisted offerings count
// (Shrine.Woodcutting.js:5, OFFERINGS_ATTRIBUTE):
// "woodcutting-guild:shrine-offerings", starting at 0.
const ShrineOfferingsAttribute = "woodcutting-guild:shrine-offerings"

// EntLogsEventName is the "woodcutting:ent-logs" emit
// (EntTrunk.Woodcutting.js: giveLogs).
const EntLogsEventName = "woodcutting:ent-logs"

// EntLogsEvent is the "woodcutting:ent-logs" payload: the
// player, the UNNOTED log id (the emit carries log.id, not
// the noted id the inventory received), and the Ent's
// location label ("Woodcutting Guild" / "Wilderness").
type EntLogsEvent struct {
	Player *game.Player
	ItemID int
	Where  string
}
