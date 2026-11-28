// Package npcdrops is the Go port of the NPC drop-table roller
// (server/plugins/npcs/NpcDrops.plugin.js, Phase 50).
//
// It is the pure roll function only: given a loaded drop table and a
// kill context it returns the drops, with no placement and no hub
// registration. The loader (internal/definition, Worker A) owns parsing
// the three data files; the types in this package mirror the JSON
// shapes so the loader can unmarshal into them (or convert
// field-for-field — see the mapping notes on each type).
//
// Rules are grounded in NpcDrops.plugin.js; the line numbers cited
// below are from that file at upstream HEAD bfd81e3.
package npcdrops

// Drop is one rolled drop: an item id, an amount and whether it drops
// noted. Placement (ground items, note-id swap) is Worker C's job;
// the roller only reports the resolved drop.
type Drop struct {
	ItemID int
	Amount int
	Noted  bool
}

// KillContext is everything about one kill the roller needs. All zero
// values mean the empty context: no Wilderness, unknown combat level,
// above ground, no kill conditions, nothing pre-handled — which is the
// behaviour with no context/quest providers in Go (see the plan §2).
//
// The fields the TS plugin fills by emitting npc-drops:context
// (NpcDrops.plugin.js:962-970: perfectKill, bountyTask,
// preferredMethodLocation, handledItemIds, combatLevel) are present so
// a future provider can fill them; until one exists they stay zero.
type KillContext struct {
	// NPCName is the killed NPC's name, as the drop file's npcs map
	// carries it. Used only by the Looting-bag exclusion
	// (NpcDrops.plugin.js:55-56).
	NPCName string
	// NPCCombatLevel picks the general seed table's band
	// (SeedCategories.NpcDrops.js:31-38). Zero or negative means
	// unknown: the first (lowest) category rolls.
	NPCCombatLevel int
	// InWilderness gates the Looting-bag tertiary rows
	// (NpcDrops.plugin.js:55-56, :1043).
	InWilderness bool
	// MapY is the killer's map y with instancing resolved by the
	// caller (NpcDrops.plugin.js:753-765, mapY). MapY >=
	// UndergroundMapY counts as underground for the Legends' Quest
	// talisman substitution.
	MapY int
	// PerfectKill, BountyTask and PreferredMethodLocation feed
	// quantity_alternatives conditions
	// (NpcDrops.plugin.js:565-598, conditionHolds).
	PerfectKill             bool
	BountyTask              string
	PreferredMethodLocation string
	// HandledItemIDs are item ids a quest/encounter handler already
	// owns for this kill; drops with those ids are removed after the
	// roll (NpcDrops.plugin.js:1085-1087).
	HandledItemIDs map[int]bool
}

// UndergroundMapY is the map-y band at/above which a tile counts as
// underground (NpcDrops.plugin.js:744, UNDERGROUND_Y = 6400).
const UndergroundMapY = 6400

// Talisman ids for the Legends' Quest gem-table gate
// (NpcDrops.plugin.js:743-770, locationTalisman; ids grounded in the
// talisman subtable of npc-drop-subtables.json: Chaos 1452, Nature
// 1462).
const (
	ChaosTalismanID  = 1452
	NatureTalismanID = 1462
)

// LootingBagName is the tertiary row gated on the Wilderness
// (NpcDrops.plugin.js:54-56). LootingBagExcludedNPCs are the NPC names
// it never drops from, matched exactly as TS matches them.
const LootingBagName = "Looting bag"

var lootingBagExcludedNPCs = map[string]bool{
	"Aviansie":         true,
	"Abyssal demon":    true,
	"Black chinchompa": true,
}

// MaxSharedDepth caps shared-table recursion
// (NpcDrops.plugin.js:772-776: depth > 4 returns no drops; the
// top-level call starts at depth 0, so five chained tables roll).
const MaxSharedDepth = 4

// Entry is one row of a drop table's entries[] or tertiary[].
// Field names and JSON tags match the dump in
// data/definitions/npc-drops.json. Weight/OutOf are the draw inputs;
// Rarity is informational only and is NEVER used for a draw —
// notably the 3 Corporeal Beast sigils, whose published rarity
// (1/1365) differs from weight/out_of (3/7, 3/7, 1/7 inside their
// group); the roller draws on weight/out_of
// (NpcDrops.plugin.js:847-865, rollWeighted; plan §1.2).
//
// QuestKey/QuestMode are NOT in the dump: the loader parses them from
// the entry's notes
// ("Quest requirement: WikiQuestDropRequirement(questKey=..., mode=...)",
// NpcDrops.plugin.js:73). An empty QuestKey means the row carries no
// quest requirement.
type Entry struct {
	ItemID   int     `json:"item_id,omitempty"`
	Name     string  `json:"name,omitempty"`
	Quantity [2]int  `json:"quantity,omitempty"`
	Weight   float64 `json:"weight,omitempty"`
	OutOf    float64 `json:"out_of,omitempty"`
	Rarity   float64 `json:"rarity,omitempty"`
	Section  string  `json:"section,omitempty"`
	Always   bool    `json:"always,omitempty"`
	Noted    bool    `json:"noted,omitempty"`
	// SeparateRoll / PreRoll / OncePerKill drive rollOnce
	// (NpcDrops.plugin.js:867-955).
	SeparateRoll bool `json:"separate_roll,omitempty"`
	PreRoll      bool `json:"pre_roll,omitempty"`
	OncePerKill  bool `json:"once_per_kill,omitempty"`
	// Nothing consumes weight in a pool and drops nothing.
	Nothing bool `json:"nothing,omitempty"`
	// SharedTable names a shared subtable to roll instead of an item
	// (NpcDrops.plugin.js:816-845, resolveEntry).
	SharedTable string `json:"shared_table,omitempty"`
	// RollGroup, GroupAccessWeight and GroupAccessOutOf describe a
	// pre-roll group (the Corporeal Beast "Sigils" group:
	// 1/585 access, then one weighted sigil;
	// NpcDrops.plugin.js:916-935).
	RollGroup         string  `json:"roll_group,omitempty"`
	GroupAccessWeight float64 `json:"group_access_weight,omitempty"`
	GroupAccessOutOf  float64 `json:"group_access_out_of,omitempty"`
	// RollCountWeights repeats a shared-table roll
	// (RollCounts.NpcDrops.js:14-22).
	RollCountWeights []RollCount `json:"roll_count_weights,omitempty"`
	// QuantityAlternatives / QuantitySelection drive rollQuantity
	// (NpcDrops.plugin.js:600-614).
	QuantityAlternatives []QuantityAlternative `json:"quantity_alternatives,omitempty"`
	QuantitySelection    string                `json:"quantity_selection,omitempty"`
	// BonusDrops resolve alongside the entry itself
	// (NpcDrops.plugin.js:836-842).
	BonusDrops []Entry `json:"bonus_drops,omitempty"`
	// QuestKey / QuestMode: parsed by the loader from notes (see the
	// type doc); empty means no quest requirement.
	QuestKey  string `json:"quest_key,omitempty"`
	QuestMode string `json:"quest_mode,omitempty"`
	// ClueScrollBox is informational (no clue system consumes it yet).
	ClueScrollBox bool `json:"clue_scroll_box,omitempty"`
}

// RollCount is one {rolls, weight} step of a roll_count_weights list
// (RollCounts.NpcDrops.js:14-22).
type RollCount struct {
	Rolls  int `json:"rolls,omitempty"`
	Weight int `json:"weight,omitempty"`
}

// QuantityAlternative is one quantity_alternatives entry. The first
// alternative is the default; later ones carry a Condition
// (NpcDrops.plugin.js:589-598, chooseAlternative).
type QuantityAlternative struct {
	Quantity  [2]int             `json:"quantity,omitempty"`
	Condition *QuantityCondition `json:"condition,omitempty"`
}

// QuantityCondition is the condition on a quantity alternative.
// Types in the data: "perfect_kill", "bounty_task" (Monster), and
// "preferred_method" (Location) (NpcDrops.plugin.js:565-586).
type QuantityCondition struct {
	Type     string `json:"type,omitempty"`
	Monster  string `json:"monster,omitempty"`
	Location string `json:"location,omitempty"`
}

// TertiaryGroup is one tertiary_groups entry. Only type "exclusive"
// exists in the data (the 4 DT2 tables); its entries' weight/out_of
// chances sum to the published access rate, and at most one outcome
// is picked (OrderedRewards.NpcDrops.js, group). SourceAccess is the
// published total access rate (1/40 for the DT2 clue groups); the
// roller derives the access rate from the entries, exactly as TS
// does, and never reads SourceAccess for the draw.
type TertiaryGroup struct {
	Type         string     `json:"type,omitempty"`
	SourceAccess AccessRate `json:"source_access,omitempty"`
	Entries      []Entry    `json:"entries,omitempty"`
}

// AccessRate is a {weight, out_of} rate, used for a tertiary group's
// source_access.
type AccessRate struct {
	Weight float64 `json:"weight,omitempty"`
	OutOf  float64 `json:"out_of,omitempty"`
}

// RewardChainInfo marks a table carrying an ordered reward_chain (the
// 4 Forgotten Four boss tables; NpcDrops.plugin.js reward_chain,
// OrderedRewards.NpcDrops.js select). It is presence-only on purpose:
// this phase never resolves an ordered chain — the main roll fails
// closed when it is non-nil — and the stage shape (stages, scaling,
// progression) belongs to the follow-up that builds the state
// provider (plan §7 residual 1).
type RewardChainInfo struct {
	Type        string `json:"type,omitempty"`
	StateDomain string `json:"state_domain,omitempty"`
}

// Table is one drop table from npc-drops.json. MainMaxRoll is the
// denominator of the weighted main roll (absent — 0 — on all-always
// and empty tables); Rolls repeats rollOnce (absent — 0 — means 1).
// RewardChain is non-nil only on the 4 DT2 tables.
type Table struct {
	ID             string           `json:"table_id,omitempty"`
	Label          string           `json:"label,omitempty"`
	MainMaxRoll    float64          `json:"main_max_roll,omitempty"`
	Rolls          int              `json:"rolls,omitempty"`
	Entries        []Entry          `json:"entries,omitempty"`
	Tertiary       []Entry          `json:"tertiary,omitempty"`
	TertiaryGroups []TertiaryGroup  `json:"tertiary_groups,omitempty"`
	RewardChain    *RewardChainInfo `json:"reward_chain,omitempty"`
}

// SharedEntry is one row of a shared subtable AFTER the loader has
// parsed it. The dump (npc-drop-subtables.json) carries raw strings —
// quantity_raw ("67", "19500-20000", "100 (noted)") and rarity_raw
// ("N/D") — which the loader parses into Quantity, Weight, OutOf and
// Noted (NpcDrops.plugin.js:130-161, parseSharedRarity /
// parseQuantityRaw / isNotedQuantity). Quantity [0,0] means absent
// (a ref row); Weight/OutOf are the parsed "N/D" pair.
type SharedEntry struct {
	ItemID   int     `json:"item_id,omitempty"`
	Name     string  `json:"name,omitempty"`
	Quantity [2]int  `json:"quantity,omitempty"`
	Weight   float64 `json:"weight,omitempty"`
	OutOf    float64 `json:"out_of,omitempty"`
	Noted    bool    `json:"noted,omitempty"`
	// Nothing consumes weight in the pool and drops nothing.
	Nothing bool `json:"nothing,omitempty"`
	// SharedTable names a nested shared table (a "ref" row:
	// rareDrop -> gem -> megaRare).
	SharedTable string `json:"shared_table,omitempty"`
}

// SeedCategory is one combat-level band of the general seed table
// (SeedCategories.NpcDrops.js:31-38). MaximumExclusive is nil on the
// last (unbounded) category.
type SeedCategory struct {
	Minimum          int           `json:"minimum,omitempty"`
	MaximumExclusive *int          `json:"maximum_exclusive,omitempty"`
	Entries          []SharedEntry `json:"entries,omitempty"`
}

// SharedTable is one parsed shared subtable. Either Entries (flat
// tables: rareDrop, gem, herb, ...) or Categories (the seed table
// only) is populated. CombatLevelMultiplier is the seed table's 10;
// it is 0 on flat tables.
type SharedTable struct {
	Name                  string         `json:"name,omitempty"`
	Entries               []SharedEntry  `json:"entries,omitempty"`
	Categories            []SeedCategory `json:"categories,omitempty"`
	CombatLevelMultiplier int            `json:"combat_level_multiplier,omitempty"`
}
