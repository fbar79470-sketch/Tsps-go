package definition

import (
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// NpcDropMaxStack is MAX_STACK (NpcDrops.plugin.js:37): the largest
// item stack, bounding every loaded quantity.
const NpcDropMaxStack = 2147483647

// slayerKeyItemIDs is SLAYER_KEYS (NpcDrops.plugin.js:45): Brimstone
// key 23083 and Larran's key 23490. applyWikiCorrections (:329-369)
// strips them from every table because they drop from Konar's and
// Krystilia's task kills at a chance set by the monster's combat
// level (plugins/skills/slayer/Keys.Slayer.js), not from the drop
// tables. The Slayer plugin rolls them when it lands; this phase
// implements only this strip of applyWikiCorrections, and the other
// corrections (Zulrah/Vorkath rolls=2, Mad Angel / Skotizo /
// Amoxliatl / Alchemical Hydra, animated armour, Kalphite Queen
// noted) are residuals — see PHASE50_PLAN.md §7.6.
var slayerKeyItemIDs = map[int]bool{23083: true, 23490: true}

// NpcDropBonusDrop is one bonus_drops row: an extra item that drops
// alongside its parent entry (e.g. Saradomin brew + Super restore
// from Commander Zilyana).
type NpcDropBonusDrop struct {
	ItemID   int
	Name     string
	Quantity [2]int
}

// NpcDropRollCountWeight is one roll_count_weights row
// (RollCounts.NpcDrops.js:14-22): a shared-table entry rolls that
// many times with this weight.
type NpcDropRollCountWeight struct {
	Rolls  int
	Weight int
}

// NpcDropQuantityCondition is the condition on a quantity alternative
// (NpcDrops.plugin.js:565-586, conditionHolds): "perfect_kill" (no
// extra fields), "bounty_task" (Monster), or "preferred_method"
// (Location). The first alternative is the default and carries no
// condition; later ones carry one.
type NpcDropQuantityCondition struct {
	Type     string
	Monster  string
	Location string
}

// NpcDropQuantityAlternative is one quantity_alternatives entry: a
// quantity plus the condition under which it applies (nil on the
// default alternative).
type NpcDropQuantityAlternative struct {
	Quantity  [2]int
	Condition *NpcDropQuantityCondition
}

// NpcDropEntry is one main or tertiary row of a drop table
// (npc-drops.json). ItemID is 0 with HasItemID false for the two
// item-less shapes: a shared-table reference (SharedTable set) or
// an explicit empty outcome (Nothing set) — the 1,106 such rows in
// the dump. Quantity is [1,1] when the row carries none (the
// shared/nothing shapes); for item rows it is always present in the
// dump. Always is identified by the flag, not by rarity: no main
// entry has numeric rarity 1.0, and one always row uses lowercase
// rarity_raw "always" (Vanguard, Xeric's aid (+)(4) 20984), so the
// flag — never the raw string — decides. Rarity is informational
// (the roller uses Weight/OutOf); RarityRaw preserves the dump's
// string ("Always" / a fraction) for tests and debugging.
type NpcDropEntry struct {
	ItemID    int
	HasItemID bool
	Name      string
	Quantity  [2]int
	// Weight/OutOf are the draw odds. Always entries carry none
	// (0/0); every other row in the dump carries both, including
	// shared-table and nothing rows.
	Weight int
	OutOf  int
	// Rarity is the dump's numeric rarity when present (absent on
	// always rows and on the 1,106 shared/nothing rows).
	Rarity    float64
	HasRarity bool
	RarityRaw string
	Always    bool
	Section   string
	Noted     bool
	// SharedTable names the shared subtable this row rolls into
	// ("" when the row is a plain item or a nothing outcome).
	SharedTable string
	Nothing     bool
	// SeparateRoll / PreRoll / OncePerKill are the rollOnce
	// classifications (NpcDrops.plugin.js:867-955).
	SeparateRoll bool
	PreRoll      bool
	OncePerKill  bool
	// RollGroup, GroupAccessWeight/OutOf and WeightBasis describe
	// a pre-roll group (Corporeal Beast sigils: one access roll,
	// then exactly one weighted item from the group).
	RollGroup            string
	GroupAccessWeight    int
	GroupAccessOutOf     int
	WeightBasis          string
	ClueScrollBox        bool
	Notes                []string
	AssumedRarity        bool
	PublishedRarity      string
	QuantityAlternatives []NpcDropQuantityAlternative
	QuantitySelection    string
	QuantityRaw          string
	RollCountWeights     []NpcDropRollCountWeight
	// UnresolvedRollCount marks a shared-table row whose roll
	// count the dumper could not resolve (5 rows).
	UnresolvedRollCount bool
	BonusDrops          []NpcDropBonusDrop
}

// NpcDropRewardChainStage is one stage of an ordered reward chain
// (OrderedRewards.NpcDrops.js, docs/npc-drops-ordered.md): the
// Forgotten Four tables only. Stage extras the roller does not
// need (record_first_received, source_scaling, eligibility,
// progression) are not parsed in this phase; the ordered path is
// fail-closed (PHASE50_PLAN.md §2.3), so the stage's entries and
// access odds are all the loader must carry.
type NpcDropRewardChainStage struct {
	Key         string
	Weight      int
	OutOf       int
	MainMaxRoll int
	Entries     []NpcDropEntry
}

// NpcDropRewardChain is a table's reward_chain (4 DT2 boss tables
// only): type "ordered_exclusive", state_domain "forgotten_four".
type NpcDropRewardChain struct {
	Type        string
	StateDomain string
	Stages      []NpcDropRewardChainStage
}

// NpcDropAccess is a {weight, out_of} access chance, e.g. a
// tertiary group's source_access.
type NpcDropAccess struct {
	Weight int
	OutOf  int
}

// NpcDropTertiaryGroup is one tertiary_groups entry (the 4 DT2 boss
// tables only): type "exclusive", one outcome picked at most.
type NpcDropTertiaryGroup struct {
	Type         string
	SourceAccess NpcDropAccess
	Entries      []NpcDropEntry
}

// NpcDropTable is one drop table (npc-drops.json tables[id]).
// Entries and Tertiary are the post-override, post-Slayer-strip
// rows. Rolls is 1 unless the dump sets rolls (47 tables; 3 on 39,
// 2 on 6, 9 on bloodthirst_rockslug, 25 on ent). HasMainMaxRoll is
// false on the 448 tables without main_max_roll (409 all-always +
// 39 empty). Boss is set by loadNpcDrops when a boss NPC resolves
// to the table (NpcDrops.plugin.js:509-510); a table shared by a
// boss and a non-boss NPC is marked boss either way, as in TS.
type NpcDropTable struct {
	ID             string
	Label          string
	TableName      string
	WikiPage       string
	MainMaxRoll    int
	HasMainMaxRoll bool
	Rolls          int
	Entries        []NpcDropEntry
	Tertiary       []NpcDropEntry
	RewardChain    *NpcDropRewardChain
	TertiaryGroups []NpcDropTertiaryGroup
	// BrimstoneKeyRoll / BrimstoneKonarBonus mark tables where
	// the Slayer plugin rolls Brimstone/Larran's keys on Konar
	// tasks (Keys.Slayer.js); not this loader's job, preserved
	// for the Slayer phase.
	BrimstoneKeyRoll    bool
	BrimstoneKonarBonus bool
	Boss                bool
}

// NpcDropNPC is one npc-drops.json npcs[id] record after overrides
// (npc-drop-overrides.json addNpcs/removeNpcs applied in memory;
// the files on disk are never modified — generated data is fixed
// in its dumper, never hand-edited). Tables lists the table ids in
// dump order first, override-appended tables after; the first is
// the default table.
type NpcDropNPC struct {
	ID        int
	Name      string
	CacheName string
	WikiPage  string
	Tables    []string
}

// NpcDropSharedEntry is one parsed row of a shared subtable
// (npc-drop-subtables.json). Rows are raw strings in the file, so
// rarity_raw is parsed to Weight/OutOf and quantity_raw to
// Quantity here (parseSharedRarity / parseQuantityRaw,
// NpcDrops.plugin.js:130-157). SharedTable names a nested subtable
// (rareDrop -> gem / megaRare); Nothing is an explicit empty slot.
// Quantity is [1,1] when quantity_raw is absent or unparseable —
// including "100 (noted)": TS's parseQuantityRaw matches only a
// bare integer or a bare range, so that row parses to [1,1] with
// Noted true, and the port reproduces that faithfully rather than
// inventing a 100 quantity.
type NpcDropSharedEntry struct {
	ItemID      int
	HasItemID   bool
	Name        string
	Quantity    [2]int
	Noted       bool
	Weight      int
	OutOf       int
	SharedTable string
	Nothing     bool
}

// NpcDropSeedCategory is one level band of the general seed table
// (SeedCategories.NpcDrops.js): combat level * multiplier is
// rolled and the first category whose MaximumExclusive exceeds the
// roll is used. The last category has no maximum
// (HasMaximumExclusive false).
type NpcDropSeedCategory struct {
	Minimum             int
	MaximumExclusive    int
	HasMaximumExclusive bool
	Entries             []NpcDropSharedEntry
}

// NpcDropSeedTable is a shared subtable with level-banded
// categories (only "seed" in the shipped file):
// combat_level_multiplier 10 + 6 categories.
type NpcDropSeedTable struct {
	Name       string
	Multiplier int
	Categories []NpcDropSeedCategory
}

// NpcDropStats is the load report for the three drop files.
type NpcDropStats struct {
	// Tables is the number of drop tables (1,106 in the shipped
	// file).
	Tables int
	// NPCs is the number of NPC records after overrides (3,193
	// in the shipped file: 3,086 in the dump, minus Ent 7234
	// removed, plus 108 new ids from addNpcs).
	NPCs int
	// NPCsWithTables is the number of NPCs resolving to at least
	// one table (3,190: the 3 zero-table NPCs — Vorkath 8058 /
	// 8060 and Amoxliatl 13686 — resolve to none, as in TS where
	// tablesByNpc only holds NPCs with a resolved table).
	NPCsWithTables int
	// SharedTables is the number of parsed shared subtables (13
	// in the shipped file, seed included).
	SharedTables int
	// SeedTables is the number of shared subtables with valid
	// level-banded categories (1: "seed").
	SeedTables int
	// BossNPCs is the number of override boss ids (3: Yama 124,
	// Araxxor 13668, Shellbane gryphon 14860).
	BossNPCs int
	// Entries / Tertiary are the loaded main / tertiary row
	// counts after overrides and the Slayer-key strip (14,737 /
	// 1,592 in the shipped file).
	Entries  int
	Tertiary int
	// UnusableSubtableRows counts shared-subtable rows skipped
	// because rarity_raw is not a "N/D" fraction (text rarities
	// such as "Common") or because a row has no item, nested
	// table, or nothing shape (NpcDrops.plugin.js:516-527).
	UnusableSubtableRows int
	// UnresolvedItems counts table-level unresolved_items names
	// skipped (1 "Brimstone key" in each of reanimated_dagannoth
	// and wyrmling: 2 total). The name has no item_id anywhere
	// in the dump, so there is no row to load.
	UnresolvedItems int
	// SlayerKeysStripped counts entries removed by the
	// Slayer-key strip (27 in the shipped file: 24 main, 3
	// tertiary).
	SlayerKeysStripped int
	// AssumedQuantityRows counts rows with
	// quantity_selection "unknown" (12 in the shipped file):
	// the roller rolls uniformly among their listed quantities,
	// an assumed distribution (NpcDrops.plugin.js:471-476).
	AssumedQuantityRows int
	// SkippedEntries / SkippedTables / SkippedNPCs count rows,
	// tables, and NPC records that failed validation and were
	// dropped (0 in the shipped files).
	SkippedEntries int
	SkippedTables  int
	SkippedNPCs    int
	// RemovedNPCs is the number of removeNpcs ids actually
	// present in the dump (1: Ent 7234; the other four are
	// no-ops). AddedNPCs is the number of addNpcs ids new to
	// the dump (108).
	RemovedNPCs int
	AddedNPCs   int
}

// NpcDropTable returns the resolved drop tables for an NPC id, in
// dump order first (the first table is the default), or nil when
// the NPC has no drop table. The returned table pointers are
// shared with the Store; callers must not mutate them.
func (s *Store) NpcDropTable(npcID int) []*NpcDropTable {
	tables := s.npcDropByNPC[npcID]
	return append([]*NpcDropTable(nil), tables...)
}

// NpcDropTableByID returns one drop table by its table id, or nil
// when unknown.
func (s *Store) NpcDropTableByID(id string) *NpcDropTable {
	return s.npcDropTables[id]
}

// NpcDropTables returns every loaded drop table, sorted by id.
func (s *Store) NpcDropTables() []*NpcDropTable {
	return append([]*NpcDropTable(nil), s.npcDropTableList...)
}

// NpcDropNPC returns one NPC's drop record after overrides, or
// false when the NPC has no record.
func (s *Store) NpcDropNPC(npcID int) (*NpcDropNPC, bool) {
	npc, ok := s.npcDropNPCs[npcID]
	return npc, ok
}

// NpcDropSharedTable returns one parsed shared subtable's rows, or
// nil when unknown. The seed table is flattened here too, as in TS
// (loadDrops :534 sets sharedTables to the categories' rows
// concatenated); use NpcDropSeedTable for the banded view.
func (s *Store) NpcDropSharedTable(name string) []NpcDropSharedEntry {
	rows := s.npcDropShared[name]
	return append([]NpcDropSharedEntry(nil), rows...)
}

// NpcDropSharedTableNames returns the parsed shared subtable
// names, sorted.
func (s *Store) NpcDropSharedTableNames() []string {
	names := make([]string, 0, len(s.npcDropShared))
	for name := range s.npcDropShared {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// NpcDropSeedTable returns one level-banded seed table, or false
// when the name is not a valid seed table.
func (s *Store) NpcDropSeedTable(name string) (*NpcDropSeedTable, bool) {
	table, ok := s.npcDropSeed[name]
	return table, ok
}

// NpcDropIsBoss reports whether an NPC id is a boss for drop
// purposes: listed in overrides bossNpcs (the cache lacks param
// 1361 for those three). The cache-param half of TS isBoss
// (:84-94) is not evaluated here — the multiplier it feeds is
// not implemented in this phase.
func (s *Store) NpcDropIsBoss(npcID int) bool { return s.npcDropBoss[npcID] }

// NpcDropStats returns the load report for the drop files.
func (s *Store) NpcDropStats() NpcDropStats { return s.npcDropStats }

// loadNpcDrops ports loadDrops (NpcDrops.plugin.js:440-552) as an
// eager boot load, following the loadGroundItems pattern
// (internal/definition/grounditems.go:66-106): the core file
// npc-drops.json is REQUIRED (readJSONValue: missing or malformed
// is an error, boot-fatal) and must hold an object with tables and
// npcs sections; the subtables and overrides files are optional
// when missing (TS readJson returns null for a missing file) but
// malformed when present fails. Per-entry validation lives in
// toNpcDropEntry; invalid rows are skipped and counted. The
// world's npc-drops-custom.json is NOT loaded (no custom file
// exists in Go — a residual, PHASE50_PLAN.md §7.8).
//
// Order (loadDrops :466-469, applyOverrides :168-209): removeNpcs
// -> addNpcs (existing NPC: only tables it lacks are appended,
// dump order first) -> tables (addEntries/addTertiary deduped by
// item_id, removeSections filters by section) -> subtables (dump
// wins, override only fills gaps) -> bossNpcs collected. Then the
// Slayer-key strip of applyWikiCorrections (:329-369) only. All
// transforms run in memory; the files on disk are never written.
func (s *Store) loadNpcDrops() error {
	dump, err := s.readCoreNpcDrops()
	if err != nil {
		return err
	}
	var stats NpcDropStats
	tables, npcs, err := s.parseNpcDropDump(dump, &stats)
	if err != nil {
		return err
	}
	stats.Tables = len(tables)
	stats.NPCs = len(npcs)
	subtables, overrides, err := s.readNpcDropSupportFiles()
	if err != nil {
		return err
	}
	s.applyNpcDropOverrides(tables, npcs, subtables, overrides, &stats)
	stats.NPCs = len(npcs)
	s.stripSlayerKeys(tables, &stats)
	s.buildNpcDropByNPC(tables, npcs, overrides, &stats)
	shared, seed, unusable := s.parseNpcDropSubtables(subtables)
	stats.SharedTables = len(shared)
	stats.SeedTables = len(seed)
	stats.UnusableSubtableRows = unusable

	s.npcDropTables = tables
	s.npcDropTableList = sortedNpcDropTables(tables)
	s.npcDropNPCs = npcs
	s.npcDropShared = shared
	s.npcDropSeed = seed
	s.npcDropStats = stats
	s.logf("[npc-drops] Loaded %d tables for %d NPCs (%d with tables), %d shared subtables (%d seed), %d entries + %d tertiary (skipped=%d entries, %d tables, %d NPCs; unusableSubtableRows=%d, unresolvedItems=%d, slayerKeysStripped=%d, assumedQuantityRows=%d)",
		stats.Tables, stats.NPCs, stats.NPCsWithTables, stats.SharedTables, stats.SeedTables,
		stats.Entries, stats.Tertiary, stats.SkippedEntries, stats.SkippedTables, stats.SkippedNPCs,
		stats.UnusableSubtableRows, stats.UnresolvedItems, stats.SlayerKeysStripped, stats.AssumedQuantityRows)
	return nil
}

// ReloadNpcDrops re-reads the three drop files and swaps in the new
// tables (the ::reloaddrops owner command, NpcDrops.plugin.js:1301-1315).
// loadNpcDrops rebuilds every map from scratch and only assigns the
// new state at the end, so a failed reload leaves the previously
// loaded tables in place. It returns the post-reload NPC and table
// counts.
func (s *Store) ReloadNpcDrops() (npcs, tables int, err error) {
	if err := s.loadNpcDrops(); err != nil {
		return 0, 0, err
	}
	return s.npcDropStats.NPCs, s.npcDropStats.Tables, nil
}

// readCoreNpcDrops reads npc-drops.json and validates its object
// shape: a JSON object ({meta, tables, npcs}), not an array. A
// missing tables or npcs section is malformed here — TS logs and
// returns zero counts instead, but the Go loader pattern makes a
// core file's malformed shape boot-fatal, as with the array check
// in readCoreGroundItems.
func (s *Store) readCoreNpcDrops() (map[string]any, error) {
	file := filepath.Join(s.dir, "npc-drops.json")
	parsed, err := readJSONValue(file)
	if err != nil {
		return nil, err
	}
	dump, ok := parsed.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%s must contain an object", file)
	}
	if _, ok := dump["tables"]; !ok {
		return nil, fmt.Errorf("%s has no `tables` section", file)
	}
	if _, ok := dump["npcs"]; !ok {
		return nil, fmt.Errorf("%s has no `npcs` section", file)
	}
	return dump, nil
}

// readNpcDropSupportFiles reads the subtables and overrides files:
// absent yields an empty object (TS readJson returns null, and
// loadDrops treats it as {}), present-but-malformed fails.
func (s *Store) readNpcDropSupportFiles() (subtables, overrides map[string]any, err error) {
	subtables = make(map[string]any)
	overrides = make(map[string]any)
	for _, tc := range []struct {
		name string
		out  *map[string]any
	}{
		{"npc-drop-subtables.json", &subtables},
		{"npc-drop-overrides.json", &overrides},
	} {
		value, err := readJSONValueOptional(filepath.Join(s.dir, tc.name))
		if err != nil {
			return nil, nil, err
		}
		if value == nil {
			continue
		}
		obj, ok := value.(map[string]any)
		if !ok {
			return nil, nil, fmt.Errorf("%s must contain an object", filepath.Join(s.dir, tc.name))
		}
		*tc.out = obj
	}
	return subtables, overrides, nil
}

// parseNpcDropDump converts the dump's tables and npcs sections
// into Go values. Table ids and NPC ids are the object keys;
// table_id / npc_id fields are not trusted over the key (they are
// equal in the shipped file).
func (s *Store) parseNpcDropDump(dump map[string]any, stats *NpcDropStats) (map[string]*NpcDropTable, map[int]*NpcDropNPC, error) {
	tablesRaw, ok := dump["tables"].(map[string]any)
	if !ok {
		return nil, nil, fmt.Errorf("npc-drops.json `tables` must contain an object")
	}
	npcsRaw, ok := dump["npcs"].(map[string]any)
	if !ok {
		return nil, nil, fmt.Errorf("npc-drops.json `npcs` must contain an object")
	}
	tables := make(map[string]*NpcDropTable, len(tablesRaw))
	for _, id := range sortedStringKeys(tablesRaw) {
		table, ok := s.toNpcDropTable(id, tablesRaw[id], stats)
		if !ok {
			stats.SkippedTables++
			continue
		}
		tables[id] = table
	}
	npcs := make(map[int]*NpcDropNPC, len(npcsRaw))
	for _, key := range sortedNumericKeys(npcsRaw) {
		id, err := strconv.Atoi(key)
		if err != nil || strconv.Itoa(id) != key {
			stats.SkippedNPCs++
			continue
		}
		npc, ok := toNpcDropNPC(id, npcsRaw[key])
		if !ok {
			stats.SkippedNPCs++
			continue
		}
		npcs[id] = npc
	}
	return tables, npcs, nil
}

// toNpcDropTable converts one tables[id] object. entries and
// tertiary default to empty when absent (TS `table.entries ||
// []`); a present-but-non-array field makes the whole table
// invalid, since its rows cannot be read at all.
func (s *Store) toNpcDropTable(id string, raw any, stats *NpcDropStats) (*NpcDropTable, bool) {
	obj, ok := raw.(map[string]any)
	if !ok {
		return nil, false
	}
	table := &NpcDropTable{ID: id, Rolls: 1}
	if v, present := field(obj, "label"); present {
		table.Label, _ = v.(string)
	}
	if v, present := field(obj, "table_name"); present {
		table.TableName, _ = v.(string)
	}
	if v, present := field(obj, "wiki_page"); present {
		table.WikiPage, _ = v.(string)
	}
	if _, present := field(obj, "main_max_roll"); present {
		n, ok := intField(obj, "main_max_roll")
		if !ok || n < 0 {
			return nil, false
		}
		table.MainMaxRoll, table.HasMainMaxRoll = n, true
	}
	if _, present := field(obj, "rolls"); present {
		n, ok := intField(obj, "rolls")
		if !ok || n < 1 {
			return nil, false
		}
		table.Rolls = n
	}
	table.BrimstoneKeyRoll = boolField(obj, "brimstone_key_roll")
	table.BrimstoneKonarBonus = boolField(obj, "brimstone_konar_bonus")
	for _, tc := range []struct {
		key  string
		dest *[]NpcDropEntry
	}{
		{"entries", &table.Entries},
		{"tertiary", &table.Tertiary},
	} {
		rows, ok := s.parseNpcDropEntries(obj, tc.key, stats)
		if !ok {
			return nil, false
		}
		*tc.dest = rows
	}
	if chain, ok := parseNpcDropRewardChain(objField(obj, "reward_chain")); ok {
		table.RewardChain = chain
	}
	if groups, ok := parseNpcDropTertiaryGroups(obj, stats); ok {
		table.TertiaryGroups = groups
	}
	if v, present := field(obj, "unresolved_items"); present {
		if items, ok := v.([]any); ok {
			stats.UnresolvedItems += len(items)
		}
	}
	return table, true
}

// parseNpcDropEntries converts one entries/tertiary array. A
// missing field is an empty list; a non-array field fails the
// table. Invalid rows are skipped and counted, never fatal.
func (s *Store) parseNpcDropEntries(obj map[string]any, key string, stats *NpcDropStats) ([]NpcDropEntry, bool) {
	v, present := field(obj, key)
	if !present || v == nil {
		return nil, true
	}
	rows, ok := v.([]any)
	if !ok {
		return nil, false
	}
	out := make([]NpcDropEntry, 0, len(rows))
	for _, raw := range rows {
		entry, ok := toNpcDropEntry(raw)
		if !ok {
			stats.SkippedEntries++
			continue
		}
		if entry.QuantitySelection == "unknown" {
			stats.AssumedQuantityRows++
		}
		out = append(out, entry)
	}
	return out, true
}

// toNpcDropEntry converts one drop-table row. Validation mirrors
// what the roller needs: an item row needs an integer item_id >= 0
// and a valid [min,max] quantity (1 <= min <= max <= MAX_STACK,
// isDropQuantity :210-215); a row with no item_id must be a
// shared-table reference or a nothing outcome, or it is invalid
// (the dump's only item-less rows are those two shapes). Always
// rows carry no weight/out_of; every other row must carry integer
// weight >= 0 and out_of >= 1 (weight 0 occurs in the dump —
// h_amguard's Grimy tarromin at 0/110 — and means "never drawn",
// not "invalid"). A non-object row is invalid.
func toNpcDropEntry(raw any) (NpcDropEntry, bool) {
	obj, ok := raw.(map[string]any)
	if !ok {
		return NpcDropEntry{}, false
	}
	var entry NpcDropEntry
	if _, present := field(obj, "item_id"); present {
		id, ok := intField(obj, "item_id")
		if !ok || id < 0 {
			return NpcDropEntry{}, false
		}
		entry.ItemID, entry.HasItemID = id, true
	}
	if v, present := field(obj, "name"); present {
		entry.Name, _ = v.(string)
	}
	entry.Quantity = [2]int{1, 1}
	if v, present := field(obj, "quantity"); present {
		q, ok := parseNpcDropQuantity(v)
		if !ok {
			return NpcDropEntry{}, false
		}
		entry.Quantity = q
	} else if entry.HasItemID {
		// Item rows in the dump always carry a quantity; a
		// missing one is malformed, not a default.
		return NpcDropEntry{}, false
	}
	if v, present := field(obj, "shared_table"); present {
		name, _ := v.(string)
		if name == "" {
			return NpcDropEntry{}, false
		}
		entry.SharedTable = name
	}
	entry.Nothing = boolField(obj, "nothing")
	if !entry.HasItemID && entry.SharedTable == "" && !entry.Nothing {
		return NpcDropEntry{}, false
	}
	if v, present := field(obj, "rarity"); present {
		if n, ok := numberValue(v); ok {
			entry.Rarity, entry.HasRarity = n, true
		}
	}
	if v, present := field(obj, "rarity_raw"); present {
		entry.RarityRaw, _ = v.(string)
	}
	entry.Always = boolField(obj, "always")
	if v, present := field(obj, "section"); present {
		entry.Section, _ = v.(string)
	}
	entry.Noted = boolField(obj, "noted")
	entry.SeparateRoll = boolField(obj, "separate_roll")
	entry.PreRoll = boolField(obj, "pre_roll")
	entry.OncePerKill = boolField(obj, "once_per_kill")
	if v, present := field(obj, "roll_group"); present {
		entry.RollGroup, _ = v.(string)
	}
	if _, present := field(obj, "group_access_weight"); present {
		n, ok := intField(obj, "group_access_weight")
		if !ok || n < 0 {
			return NpcDropEntry{}, false
		}
		entry.GroupAccessWeight = n
	}
	if _, present := field(obj, "group_access_out_of"); present {
		n, ok := intField(obj, "group_access_out_of")
		if !ok || n < 0 {
			return NpcDropEntry{}, false
		}
		entry.GroupAccessOutOf = n
	}
	if v, present := field(obj, "weight_basis"); present {
		entry.WeightBasis, _ = v.(string)
	}
	entry.ClueScrollBox = boolField(obj, "clue_scroll_box")
	if v, present := field(obj, "notes"); present {
		if notes, ok := v.([]any); ok {
			for _, n := range notes {
				if text, ok := n.(string); ok {
					entry.Notes = append(entry.Notes, text)
				}
			}
		}
	}
	entry.AssumedRarity = boolField(obj, "assumed_rarity")
	if v, present := field(obj, "published_rarity"); present {
		entry.PublishedRarity, _ = v.(string)
	}
	if v, present := field(obj, "quantity_alternatives"); present {
		if alts, ok := v.([]any); ok {
			for _, alt := range alts {
				altObj, ok := alt.(map[string]any)
				if !ok {
					continue
				}
				var entryAlt NpcDropQuantityAlternative
				if qv, present := field(altObj, "quantity"); present {
					if q, ok := parseNpcDropQuantity(qv); ok {
						entryAlt.Quantity = q
					} else {
						continue
					}
				} else {
					continue
				}
				if cv, present := field(altObj, "condition"); present {
					if condObj, ok := cv.(map[string]any); ok {
						cond := &NpcDropQuantityCondition{}
						cond.Type, _ = condObj["type"].(string)
						cond.Monster, _ = condObj["monster"].(string)
						cond.Location, _ = condObj["location"].(string)
						entryAlt.Condition = cond
					}
				}
				entry.QuantityAlternatives = append(entry.QuantityAlternatives, entryAlt)
			}
		}
	}
	if v, present := field(obj, "quantity_selection"); present {
		entry.QuantitySelection, _ = v.(string)
	}
	if v, present := field(obj, "quantity_raw"); present {
		entry.QuantityRaw, _ = v.(string)
	}
	if v, present := field(obj, "roll_count_weights"); present {
		if weights, ok := v.([]any); ok {
			for _, w := range weights {
				wObj, ok := w.(map[string]any)
				if !ok {
					continue
				}
				rolls, okR := intField(wObj, "rolls")
				weight, okW := intField(wObj, "weight")
				if okR && okW && rolls >= 1 && weight >= 0 {
					entry.RollCountWeights = append(entry.RollCountWeights, NpcDropRollCountWeight{Rolls: rolls, Weight: weight})
				}
			}
		}
	}
	entry.UnresolvedRollCount = boolField(obj, "unresolved_roll_count")
	if v, present := field(obj, "bonus_drops"); present {
		if bonus, ok := v.([]any); ok {
			for _, b := range bonus {
				bObj, ok := b.(map[string]any)
				if !ok {
					continue
				}
				id, ok := intField(bObj, "item_id")
				if !ok || id < 0 {
					continue
				}
				row := NpcDropBonusDrop{ItemID: id, Quantity: [2]int{1, 1}}
				if nv, present := field(bObj, "name"); present {
					row.Name, _ = nv.(string)
				}
				if qv, present := field(bObj, "quantity"); present {
					if q, ok := parseNpcDropQuantity(qv); ok {
						row.Quantity = q
					}
				}
				entry.BonusDrops = append(entry.BonusDrops, row)
			}
		}
	}
	if !entry.Always {
		w, okW := intField(obj, "weight")
		o, okO := intField(obj, "out_of")
		if !okW || !okO || w < 0 || o < 1 {
			return NpcDropEntry{}, false
		}
		entry.Weight, entry.OutOf = w, o
	}
	return entry, true
}

// parseNpcDropQuantity validates a [min,max] quantity pair
// (isDropQuantity, NpcDrops.plugin.js:210-215).
func parseNpcDropQuantity(v any) ([2]int, bool) {
	arr, ok := v.([]any)
	if !ok || len(arr) != 2 {
		return [2]int{}, false
	}
	min, ok := numberValue(arr[0])
	if !ok || !isJSInteger(min) || jsTrunc(min) < 1 {
		return [2]int{}, false
	}
	max, ok := numberValue(arr[1])
	if !ok || !isJSInteger(max) || jsTrunc(max) < jsTrunc(min) || jsTrunc(max) > NpcDropMaxStack {
		return [2]int{}, false
	}
	return [2]int{jsTrunc(min), jsTrunc(max)}, true
}

// parseNpcDropRewardChain converts a reward_chain object; a
// missing (nil) chain is absent. A malformed chain returns false
// and the table keeps RewardChain nil — the ordered path is
// fail-closed in this phase, and a chain that cannot be read must
// not silently become a flat table, so the coordinator's
// integration must treat a reward_chain-bearing table with a nil
// parsed chain as a load defect, not as a normal table. In
// practice the shipped file's 4 chains are all well-formed.
func parseNpcDropRewardChain(obj map[string]any) (*NpcDropRewardChain, bool) {
	if obj == nil {
		return nil, false
	}
	chain := &NpcDropRewardChain{}
	if v, present := field(obj, "type"); present {
		chain.Type, _ = v.(string)
	}
	if v, present := field(obj, "state_domain"); present {
		chain.StateDomain, _ = v.(string)
	}
	stagesRaw, ok := field(obj, "stages")
	if !ok {
		return nil, false
	}
	stages, ok := stagesRaw.([]any)
	if !ok {
		return nil, false
	}
	for _, raw := range stages {
		stageObj, ok := raw.(map[string]any)
		if !ok {
			return nil, false
		}
		stage := NpcDropRewardChainStage{}
		if v, present := field(stageObj, "key"); present {
			stage.Key, _ = v.(string)
		}
		w, okW := intField(stageObj, "weight")
		o, okO := intField(stageObj, "out_of")
		m, okM := intField(stageObj, "main_max_roll")
		if !okW || !okO || !okM || w < 0 || o < 1 || m < 0 {
			return nil, false
		}
		stage.Weight, stage.OutOf, stage.MainMaxRoll = w, o, m
		entriesRaw, ok := field(stageObj, "entries")
		if !ok {
			return nil, false
		}
		entries, ok := entriesRaw.([]any)
		if !ok {
			return nil, false
		}
		for _, rawEntry := range entries {
			entry, ok := toNpcDropEntry(rawEntry)
			if !ok {
				return nil, false
			}
			stage.Entries = append(stage.Entries, entry)
		}
		chain.Stages = append(chain.Stages, stage)
	}
	return chain, true
}

// parseNpcDropTertiaryGroups converts a tertiary_groups array;
// each group's entries are ordinary drop rows.
func parseNpcDropTertiaryGroups(obj map[string]any, stats *NpcDropStats) ([]NpcDropTertiaryGroup, bool) {
	v, present := field(obj, "tertiary_groups")
	if !present || v == nil {
		return nil, false
	}
	raw, ok := v.([]any)
	if !ok {
		return nil, false
	}
	groups := make([]NpcDropTertiaryGroup, 0, len(raw))
	for _, rawGroup := range raw {
		groupObj, ok := rawGroup.(map[string]any)
		if !ok {
			return nil, false
		}
		group := NpcDropTertiaryGroup{}
		if tv, present := field(groupObj, "type"); present {
			group.Type, _ = tv.(string)
		}
		if access, present := field(groupObj, "source_access"); present {
			accessObj, ok := access.(map[string]any)
			if !ok {
				return nil, false
			}
			w, okW := intField(accessObj, "weight")
			o, okO := intField(accessObj, "out_of")
			if !okW || !okO || w < 0 || o < 1 {
				return nil, false
			}
			group.SourceAccess = NpcDropAccess{Weight: w, OutOf: o}
		}
		entriesRaw, ok := field(groupObj, "entries")
		if !ok {
			return nil, false
		}
		entries, ok := entriesRaw.([]any)
		if !ok {
			return nil, false
		}
		for _, rawEntry := range entries {
			entry, ok := toNpcDropEntry(rawEntry)
			if !ok {
				stats.SkippedEntries++
				continue
			}
			group.Entries = append(group.Entries, entry)
		}
		groups = append(groups, group)
	}
	return groups, true
}

// toNpcDropNPC converts one npcs[id] record. The tables field
// must be an array of strings when present (TS skips an NPC with
// no array or an empty array when building tablesByNpc, but the
// record itself stays in the dump map for overrides to append
// to — so an absent tables field is an empty list here, not an
// error; a non-string element makes the record invalid).
func toNpcDropNPC(id int, raw any) (*NpcDropNPC, bool) {
	obj, ok := raw.(map[string]any)
	if !ok {
		return nil, false
	}
	npc := &NpcDropNPC{ID: id}
	if v, present := field(obj, "name"); present {
		npc.Name, _ = v.(string)
	}
	if v, present := field(obj, "cache_name"); present {
		npc.CacheName, _ = v.(string)
	}
	if v, present := field(obj, "wiki_page"); present {
		npc.WikiPage, _ = v.(string)
	}
	if v, present := field(obj, "tables"); present && v != nil {
		refs, ok := v.([]any)
		if !ok {
			return nil, false
		}
		for _, ref := range refs {
			name, ok := ref.(string)
			if !ok || name == "" {
				return nil, false
			}
			npc.Tables = append(npc.Tables, name)
		}
	}
	return npc, true
}

// applyNpcDropOverrides ports applyOverrides
// (NpcDrops.plugin.js:168-209) in TS order. The dump, subtables,
// and overrides maps are mutated in place (in memory only); the
// JSON files on disk are never written.
func (s *Store) applyNpcDropOverrides(tables map[string]*NpcDropTable, npcs map[int]*NpcDropNPC, subtables, overrides map[string]any, stats *NpcDropStats) {
	// removeNpcs: delete dump.npcs[id]. Only ids actually present
	// are counted; the rest are no-ops, as in TS.
	if remove, ok := overrides["removeNpcs"].(map[string]any); ok {
		for key := range remove {
			id, err := strconv.Atoi(key)
			if err != nil {
				continue
			}
			if _, present := npcs[id]; present {
				delete(npcs, id)
				stats.RemovedNPCs++
			}
		}
	}
	// addNpcs: unknown id inserts the whole record; an existing
	// NPC only gains the tables it lacks, dump order first.
	if add, ok := overrides["addNpcs"].(map[string]any); ok {
		for _, key := range sortedNumericKeys(add) {
			if key == "$comment" {
				continue
			}
			id, err := strconv.Atoi(key)
			if err != nil || strconv.Itoa(id) != key {
				continue
			}
			incoming, ok := toNpcDropNPC(id, add[key])
			if !ok {
				stats.SkippedNPCs++
				continue
			}
			existing, present := npcs[id]
			if !present {
				npcs[id] = incoming
				stats.AddedNPCs++
				continue
			}
			for _, ref := range incoming.Tables {
				if !containsString(existing.Tables, ref) {
					existing.Tables = append(existing.Tables, ref)
				}
			}
		}
	}
	// tables: addEntries/addTertiary appended only if no existing
	// entry has the same item_id (TS compares item_id with ===
	// — two item-less rows both compare undefined, so they dedup
	// against each other too); removeSections filters entries and
	// tertiary by section.
	if changes, ok := overrides["tables"].(map[string]any); ok {
		for _, tableID := range sortedStringKeys(changes) {
			change, ok := changes[tableID].(map[string]any)
			if !ok {
				continue
			}
			table, present := tables[tableID]
			if !present {
				s.logf("[npc-drops] npc-drop-overrides.json changes unknown table '%s'", tableID)
				continue
			}
			for _, tc := range []struct {
				key  string
				rows *[]NpcDropEntry
			}{
				{"addEntries", &table.Entries},
				{"addTertiary", &table.Tertiary},
			} {
				addRows, ok := change[tc.key].([]any)
				if !ok {
					continue
				}
				for _, raw := range addRows {
					entry, ok := toNpcDropEntry(raw)
					if !ok {
						stats.SkippedEntries++
						continue
					}
					if !npcDropEntryHasItemID(*tc.rows, entry) {
						*tc.rows = append(*tc.rows, entry)
					}
				}
			}
			if removedRaw, ok := change["removeSections"].([]any); ok && len(removedRaw) > 0 {
				removed := make(map[string]bool, len(removedRaw))
				for _, section := range removedRaw {
					if name, ok := section.(string); ok {
						removed[name] = true
					}
				}
				if len(removed) > 0 {
					table.Entries = filterNpcDropEntriesBySection(table.Entries, removed)
					table.Tertiary = filterNpcDropEntriesBySection(table.Tertiary, removed)
				}
			}
		}
	}
	// subtables: dump wins, the override only fills gaps.
	if subs, ok := overrides["subtables"].(map[string]any); ok {
		for name, table := range subs {
			if _, present := subtables[name]; !present {
				subtables[name] = table
			}
		}
	}
}

// npcDropEntryHasItemID reports whether any row in rows has the
// same item_id as entry, with TS === semantics: two rows both
// lacking an item_id count as equal.
func npcDropEntryHasItemID(rows []NpcDropEntry, entry NpcDropEntry) bool {
	for _, row := range rows {
		if row.HasItemID == entry.HasItemID && (!row.HasItemID || row.ItemID == entry.ItemID) {
			return true
		}
	}
	return false
}

func filterNpcDropEntriesBySection(rows []NpcDropEntry, removed map[string]bool) []NpcDropEntry {
	out := rows[:0]
	for _, row := range rows {
		if !removed[row.Section] {
			out = append(out, row)
		}
	}
	return out
}

// stripSlayerKeys is the Slayer-key strip of applyWikiCorrections
// (NpcDrops.plugin.js:329-336): remove entries with item_id 23083
// / 23490 from every table's entries and tertiary.
func (s *Store) stripSlayerKeys(tables map[string]*NpcDropTable, stats *NpcDropStats) {
	for _, table := range tables {
		for _, rows := range []*[]NpcDropEntry{&table.Entries, &table.Tertiary} {
			kept := (*rows)[:0]
			for _, entry := range *rows {
				if entry.HasItemID && slayerKeyItemIDs[entry.ItemID] {
					stats.SlayerKeysStripped++
					continue
				}
				kept = append(kept, entry)
			}
			*rows = kept
		}
	}
}

// buildNpcDropByNPC ports the tablesByNpc build (loadDrops
// :492-508): an NPC resolves to its tables in order; an unknown
// table reference is logged and skipped; an NPC with no resolved
// table gets no entry (Vorkath 8058/8060, Amoxliatl 13686). Boss
// NPCs mark their tables boss = true. It also fills the stats'
// entry counts after overrides and the strip.
func (s *Store) buildNpcDropByNPC(tables map[string]*NpcDropTable, npcs map[int]*NpcDropNPC, overrides map[string]any, stats *NpcDropStats) {
	byNPC := make(map[int][]*NpcDropTable, len(npcs))
	boss := make(map[int]bool)
	if bossRaw, ok := overrides["bossNpcs"].(map[string]any); ok {
		for key := range bossRaw {
			if key == "$comment" {
				continue
			}
			if id, err := strconv.Atoi(key); err == nil && strconv.Itoa(id) == key {
				boss[id] = true
			}
		}
	}
	stats.BossNPCs = len(boss)
	ids := make([]int, 0, len(npcs))
	for id := range npcs {
		ids = append(ids, id)
	}
	sort.Ints(ids)
	for _, id := range ids {
		var resolved []*NpcDropTable
		for _, ref := range npcs[id].Tables {
			table, ok := tables[ref]
			if !ok {
				s.logf("[npc-drops] npc %d references unknown drop table '%s'", id, ref)
				continue
			}
			resolved = append(resolved, table)
		}
		if len(resolved) == 0 {
			continue
		}
		byNPC[id] = resolved
		if boss[id] {
			for _, table := range resolved {
				table.Boss = true
			}
		}
	}
	stats.NPCsWithTables = len(byNPC)
	for _, table := range tables {
		stats.Entries += len(table.Entries)
		stats.Tertiary += len(table.Tertiary)
	}
	s.npcDropByNPC = byNPC
	s.npcDropBoss = boss
}

// parseNpcDropSubtables ports the shared-table parse (loadDrops
// :515-545): a subtable with valid level-banded categories is a
// seed table (its categories are parsed separately and its rows
// are also flattened into the shared pool, as in TS); any other
// subtable parses its entries flat, and is stored only when at
// least one row parses. Unusable rows are counted, never fatal.
func (s *Store) parseNpcDropSubtables(subtables map[string]any) (map[string][]NpcDropSharedEntry, map[string]*NpcDropSeedTable, int) {
	shared := make(map[string][]NpcDropSharedEntry, len(subtables))
	seed := make(map[string]*NpcDropSeedTable)
	unusable := 0
	for _, name := range sortedStringKeys(subtables) {
		obj, ok := subtables[name].(map[string]any)
		if !ok {
			continue
		}
		if seedTable, ok := parseNpcDropSeedTable(name, obj, &unusable); ok {
			seed[name] = seedTable
			var flat []NpcDropSharedEntry
			for _, category := range seedTable.Categories {
				flat = append(flat, category.Entries...)
			}
			shared[name] = flat
			continue
		}
		rows, ok := obj["entries"].([]any)
		if !ok {
			continue
		}
		entries := parseNpcDropSharedRows(rows, &unusable)
		if len(entries) > 0 {
			shared[name] = entries
		}
	}
	return shared, seed, unusable
}

// parseNpcDropSeedTable validates a seed table per
// SeedCategories.validCategories (SeedCategories.NpcDrops.js:10-24):
// categories non-empty, a safe-integer combat_level_multiplier >=
// 1, categories starting at minimum 0 and contiguous, the last
// with no maximum_exclusive. A table failing any check is not a
// seed table (the caller falls back to the flat parse).
func parseNpcDropSeedTable(name string, obj map[string]any, unusable *int) (*NpcDropSeedTable, bool) {
	categoriesRaw, ok := obj["categories"].([]any)
	if !ok || len(categoriesRaw) == 0 {
		return nil, false
	}
	multiplier, ok := intField(obj, "combat_level_multiplier")
	if !ok || multiplier < 1 {
		return nil, false
	}
	table := &NpcDropSeedTable{Name: name, Multiplier: multiplier}
	minimum := 0
	for i, raw := range categoriesRaw {
		categoryObj, ok := raw.(map[string]any)
		if !ok {
			return nil, false
		}
		min, ok := intField(categoryObj, "minimum")
		if !ok || min != minimum {
			return nil, false
		}
		category := NpcDropSeedCategory{Minimum: min}
		last := i == len(categoriesRaw)-1
		if maxRaw, present := field(categoryObj, "maximum_exclusive"); present && maxRaw != nil {
			max, ok := intField(categoryObj, "maximum_exclusive")
			if !ok || last || max <= minimum {
				return nil, false
			}
			category.MaximumExclusive, category.HasMaximumExclusive = max, true
		} else if !last {
			return nil, false
		}
		entriesRaw, ok := categoryObj["entries"].([]any)
		if !ok {
			return nil, false
		}
		category.Entries = parseNpcDropSharedRows(entriesRaw, unusable)
		table.Categories = append(table.Categories, category)
		if category.HasMaximumExclusive {
			minimum = category.MaximumExclusive
		}
	}
	return table, true
}

// parseNpcDropSharedRows ports sharedRows (loadDrops :515-527):
// rarity_raw "N/D" -> {weight, outOf}; a row without a parseable
// fraction is unusable, as is a row with a fraction but no item,
// nested table, or nothing shape. Nested-table rows are checked
// first, then nothing, then items — the same order as TS.
func parseNpcDropSharedRows(rows []any, unusable *int) []NpcDropSharedEntry {
	entries := make([]NpcDropSharedEntry, 0, len(rows))
	for _, raw := range rows {
		obj, ok := raw.(map[string]any)
		if !ok {
			*unusable++
			continue
		}
		rarityRaw, _ := field(obj, "rarity_raw")
		weight, outOf, ok := parseNpcDropSharedRarity(rarityRaw)
		if !ok {
			*unusable++
			continue
		}
		entry := NpcDropSharedEntry{Weight: weight, OutOf: outOf}
		if v, present := field(obj, "name"); present {
			entry.Name, _ = v.(string)
		}
		if ref, present := field(obj, "shared_table"); present {
			if name, ok := ref.(string); ok && name != "" {
				entry.SharedTable = name
				entries = append(entries, entry)
				continue
			}
		}
		if boolField(obj, "nothing") {
			entry.Nothing = true
			entries = append(entries, entry)
			continue
		}
		id, ok := intField(obj, "item_id")
		if !ok || id < 0 {
			*unusable++
			continue
		}
		entry.ItemID, entry.HasItemID = id, true
		entry.Quantity = [2]int{1, 1}
		if qv, present := field(obj, "quantity_raw"); present {
			if q, ok := parseNpcDropQuantityRaw(qv); ok {
				entry.Quantity = q
			}
			entry.Noted = isNpcDropNotedQuantity(qv)
		}
		entries = append(entries, entry)
	}
	return entries
}

// parseNpcDropSharedRarity ports parseSharedRarity
// (NpcDrops.plugin.js:130-141): "N/D" with both parts positive
// integers; anything else (including text rarities such as
// "Common") is unusable.
var npcDropSharedRarityRe = regexp.MustCompile(`^(\d+)\s*/\s*(\d+)$`)

func parseNpcDropSharedRarity(raw any) (weight, outOf int, ok bool) {
	text, isString := raw.(string)
	if !isString {
		return 0, 0, false
	}
	match := npcDropSharedRarityRe.FindStringSubmatch(strings.TrimSpace(text))
	if match == nil {
		return 0, 0, false
	}
	weight, errW := strconv.Atoi(match[1])
	outOf, errO := strconv.Atoi(match[2])
	if errW != nil || errO != nil || weight <= 0 || outOf <= 0 {
		return 0, 0, false
	}
	return weight, outOf, true
}

// parseNpcDropQuantityRaw ports parseQuantityRaw
// (NpcDrops.plugin.js:143-157): a bare integer, or a "min-max"
// range (hyphen or en dash). "(noted)" suffixes do not match
// either form and yield no quantity — see NpcDropSharedEntry.
var (
	npcDropQuantitySingleRe = regexp.MustCompile(`^(\d+)$`)
	npcDropQuantityRangeRe  = regexp.MustCompile(`^(\d+)\s*[-–]\s*(\d+)$`)
)

func parseNpcDropQuantityRaw(raw any) ([2]int, bool) {
	text, isString := raw.(string)
	if !isString {
		return [2]int{}, false
	}
	text = strings.TrimSpace(text)
	if match := npcDropQuantitySingleRe.FindStringSubmatch(text); match != nil {
		value, err := strconv.Atoi(match[1])
		if err != nil {
			return [2]int{}, false
		}
		return [2]int{value, value}, true
	}
	if match := npcDropQuantityRangeRe.FindStringSubmatch(text); match != nil {
		min, errMin := strconv.Atoi(match[1])
		max, errMax := strconv.Atoi(match[2])
		if errMin != nil || errMax != nil {
			return [2]int{}, false
		}
		return [2]int{min, max}, true
	}
	return [2]int{}, false
}

// isNpcDropNotedQuantity ports isNotedQuantity
// (NpcDrops.plugin.js:159-161): "(noted)" anywhere in the string,
// case-insensitive.
func isNpcDropNotedQuantity(raw any) bool {
	text, isString := raw.(string)
	return isString && strings.Contains(strings.ToLower(text), "(noted)")
}

func sortedStringKeys(obj map[string]any) []string {
	keys := make([]string, 0, len(obj))
	for key := range obj {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func sortedNpcDropTables(tables map[string]*NpcDropTable) []*NpcDropTable {
	ids := make([]string, 0, len(tables))
	for id := range tables {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	list := make([]*NpcDropTable, 0, len(tables))
	for _, id := range ids {
		list = append(list, tables[id])
	}
	return list
}

func containsString(list []string, want string) bool {
	for _, item := range list {
		if item == want {
			return true
		}
	}
	return false
}
