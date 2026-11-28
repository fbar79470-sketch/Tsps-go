package definition

import (
	"testing"
)

// loadNpcDropFixture loads a store from minimal fixtures with the
// given drop files overlaid. The subtables and overrides files are
// absent unless the caller provides them.
func loadNpcDropFixture(t *testing.T, drops, subtables, overrides string) *Store {
	t.Helper()
	extra := map[string]string{"npc-drops.json": drops}
	if subtables != "" {
		extra["npc-drop-subtables.json"] = subtables
	}
	if overrides != "" {
		extra["npc-drop-overrides.json"] = overrides
	}
	dir := fixtureDir(t, withFiles(extra))
	var logs logCapture
	store, err := Load(dir, LoadOptions{Logf: logs.logf})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	return store
}

func TestLoadNpcDropsMinimal(t *testing.T) {
	store := loadNpcDropFixture(t, `{"tables": {}, "npcs": {}}`, "", "")
	stats := store.NpcDropStats()
	if stats.Tables != 0 || stats.NPCs != 0 || stats.SharedTables != 0 {
		t.Fatalf("stats = %+v", stats)
	}
	if got := store.NpcDropTable(3036); got != nil {
		t.Fatalf("NpcDropTable(3036) = %v", got)
	}
	if got := store.NpcDropSharedTableNames(); len(got) != 0 {
		t.Fatalf("shared names = %v", got)
	}
}

func TestLoadNpcDropsMissingCoreFileFails(t *testing.T) {
	dir := fixtureDir(t, withoutFiles("npc-drops.json"))
	if _, err := Load(dir, LoadOptions{}); err == nil {
		t.Fatal("missing npc-drops.json must fail")
	}
}

func TestLoadNpcDropsNonObjectCoreFileFails(t *testing.T) {
	dir := fixtureDir(t, withFiles(map[string]string{"npc-drops.json": `[]`}))
	if _, err := Load(dir, LoadOptions{}); err == nil {
		t.Fatal("npc-drops.json holding an array must fail")
	}
}

func TestLoadNpcDropsMalformedSubtablesFails(t *testing.T) {
	dir := fixtureDir(t, withFiles(map[string]string{
		"npc-drops.json":          `{"tables": {}, "npcs": {}}`,
		"npc-drop-subtables.json": `{oops`,
	}))
	if _, err := Load(dir, LoadOptions{}); err == nil {
		t.Fatal("malformed npc-drop-subtables.json must fail")
	}
}

func TestLoadNpcDropsOverridesAndSlayerStrip(t *testing.T) {
	drops := `{"tables": {
		"goblin": {"table_id": "goblin", "label": "Drops", "main_max_roll": 128,
			"entries": [
				{"item_id": 526, "name": "Bones", "quantity": [1, 1], "always": true, "rarity_raw": "Always", "section": "100%"},
				{"item_id": 23083, "name": "Brimstone key", "quantity": [1, 1], "always": true, "rarity_raw": "Always", "section": "100%"},
				{"item_id": 995, "name": "Coins", "quantity": [3, 3], "weight": 38, "out_of": 128, "section": "Coins"}],
			"tertiary": [
				{"item_id": 23490, "name": "Larran's key", "quantity": [1, 1], "weight": 1, "out_of": 100, "section": "Tertiary"}]},
		"yama": {"table_id": "yama", "label": "Drops",
			"entries": [
				{"item_id": 1, "name": "One", "quantity": [1, 1], "weight": 1, "out_of": 2, "section": "Contract"},
				{"item_id": 2, "name": "Two", "quantity": [1, 1], "weight": 1, "out_of": 2, "section": "Other"}],
			"tertiary": []}
	}, "npcs": {
		"3036": {"npc_id": 3036, "name": "Goblin", "tables": ["goblin"]},
		"7234": {"npc_id": 7234, "name": "Ent", "tables": ["goblin"]},
		"13489": {"npc_id": 13489, "name": "Zombie pirate", "tables": ["goblin"]}
	}}`
	overrides := `{"removeNpcs": {"7234": "Ent"},
		"addNpcs": {"15741": {"npc_id": 15741, "name": "Maggot King", "tables": ["goblin"]},
			"13489": {"npc_id": 13489, "name": "Zombie pirate", "tables": ["yama"]}},
		"bossNpcs": {"124": "Yama"},
		"tables": {"yama": {"removeSections": ["Contract"]}},
		"subtables": {}}`
	store := loadNpcDropFixture(t, drops, "", overrides)
	stats := store.NpcDropStats()
	if stats.Tables != 2 {
		t.Fatalf("Tables = %d", stats.Tables)
	}
	// 3 dump NPCs - Ent 7234 removed + Maggot King added = 3.
	if stats.NPCs != 3 {
		t.Fatalf("NPCs = %d, want 3", stats.NPCs)
	}
	if stats.RemovedNPCs != 1 || stats.AddedNPCs != 1 {
		t.Fatalf("RemovedNPCs=%d AddedNPCs=%d", stats.RemovedNPCs, stats.AddedNPCs)
	}
	// Slayer strip: Brimstone key (main) + Larran's key (tertiary).
	if stats.SlayerKeysStripped != 2 {
		t.Fatalf("SlayerKeysStripped = %d, want 2", stats.SlayerKeysStripped)
	}
	if _, ok := store.NpcDropNPC(7234); ok {
		t.Fatal("Ent 7234 must be removed")
	}
	if got := store.NpcDropTable(7234); got != nil {
		t.Fatalf("NpcDropTable(7234) = %v", got)
	}
	// Zombie pirate keeps the dump table first, override appended.
	tables := store.NpcDropTable(13489)
	if len(tables) != 2 || tables[0].ID != "goblin" || tables[1].ID != "yama" {
		t.Fatalf("zombie pirate tables = %v", tableIDs(tables))
	}
	if got := store.NpcDropTable(15741); len(got) != 1 || got[0].ID != "goblin" {
		t.Fatalf("Maggot King tables = %v", tableIDs(got))
	}
	// Goblin table: Bones always + Coins; both Slayer keys gone.
	goblin := store.NpcDropTableByID("goblin")
	if goblin == nil {
		t.Fatal("goblin table missing")
	}
	if len(goblin.Entries) != 2 || len(goblin.Tertiary) != 0 {
		t.Fatalf("goblin entries=%d tertiary=%d", len(goblin.Entries), len(goblin.Tertiary))
	}
	if !goblin.Entries[0].Always || goblin.Entries[0].ItemID != 526 {
		t.Fatalf("first goblin entry = %+v", goblin.Entries[0])
	}
	// Yama: the Contract section is removed, the Other row stays.
	yama := store.NpcDropTableByID("yama")
	if len(yama.Entries) != 1 || yama.Entries[0].ItemID != 2 {
		t.Fatalf("yama entries = %+v", yama.Entries)
	}
	if !store.NpcDropIsBoss(124) || store.NpcDropIsBoss(3036) {
		t.Fatal("boss set wrong")
	}
}

func tableIDs(tables []*NpcDropTable) []string {
	ids := make([]string, len(tables))
	for i, table := range tables {
		ids[i] = table.ID
	}
	return ids
}

func TestLoadNpcDropsInvalidEntriesSkipped(t *testing.T) {
	drops := `{"tables": {
		"mixed": {"table_id": "mixed", "label": "Drops",
			"entries": [
				{"item_id": 526, "name": "Bones", "quantity": [1, 1], "always": true, "section": "100%"},
				{"name": "No id, no shared table, no nothing", "quantity": [1, 1], "weight": 1, "out_of": 2},
				{"item_id": -1, "name": "Negative id", "quantity": [1, 1], "weight": 1, "out_of": 2},
				{"item_id": 995, "name": "Bad quantity", "quantity": [0, 5], "weight": 1, "out_of": 2},
				{"item_id": 995, "name": "Missing weight", "quantity": [1, 1]},
				{"shared_table": "herb", "weight": 2, "out_of": 128, "section": "Herbs"},
				{"nothing": true, "weight": 8, "out_of": 128, "section": "Nothing"}],
			"tertiary": []}
	}, "npcs": {"1": {"npc_id": 1, "name": "One", "tables": ["mixed"]}}}`
	store := loadNpcDropFixture(t, drops, "", "")
	stats := store.NpcDropStats()
	if stats.SkippedEntries != 4 {
		t.Fatalf("SkippedEntries = %d, want 4", stats.SkippedEntries)
	}
	table := store.NpcDropTableByID("mixed")
	if len(table.Entries) != 3 {
		t.Fatalf("entries = %d, want 3 (Bones, herb ref, nothing)", len(table.Entries))
	}
	if table.Entries[1].SharedTable != "herb" || table.Entries[1].HasItemID {
		t.Fatalf("herb entry = %+v", table.Entries[1])
	}
	if !table.Entries[2].Nothing {
		t.Fatalf("nothing entry = %+v", table.Entries[2])
	}
}

func TestLoadNpcDropsSubtables(t *testing.T) {
	drops := `{"tables": {}, "npcs": {}}`
	subtables := `{
		"gem": {"entries": [
			{"nothing": true, "rarity_raw": "63/128"},
			{"item_id": 1623, "name": "Uncut sapphire", "quantity_raw": "1", "rarity_raw": "32/128"},
			{"item_id": 442, "name": "Silver ore", "quantity_raw": "100 (noted)", "rarity_raw": "2/128"},
			{"item_id": 995, "name": "Coins", "quantity_raw": "19500-20000", "rarity_raw": "1/128"},
			{"item_id": 1, "name": "Text rarity", "quantity_raw": "1", "rarity_raw": "Common"},
			{"shared_table": "megaRare", "name": "Mega-rare", "rarity_raw": "1/128"},
			{"name": "No id at all", "rarity_raw": "1/128"}]},
		"seed": {"combat_level_multiplier": 10, "categories": [
			{"minimum": 0, "maximum_exclusive": 485, "entries": [
				{"item_id": 5318, "name": "Potato seed", "quantity_raw": "4", "rarity_raw": "368/1008"}]},
			{"minimum": 485, "entries": [
				{"item_id": 5305, "name": "Barley seed", "quantity_raw": "3", "rarity_raw": "229/1000"}]}]},
		"broken_seed": {"combat_level_multiplier": 10, "categories": [
			{"minimum": 5, "maximum_exclusive": 485, "entries": []}]},
		"empty": {"entries": []}
	}`
	store := loadNpcDropFixture(t, drops, subtables, "")
	stats := store.NpcDropStats()
	if stats.SharedTables != 2 {
		t.Fatalf("SharedTables = %d, want 2 (gem + seed; empty has no rows, broken_seed is not valid)", stats.SharedTables)
	}
	if stats.SeedTables != 1 {
		t.Fatalf("SeedTables = %d, want 1", stats.SeedTables)
	}
	// Text rarity + no-id rows are unusable.
	if stats.UnusableSubtableRows != 2 {
		t.Fatalf("UnusableSubtableRows = %d, want 2", stats.UnusableSubtableRows)
	}
	gem := store.NpcDropSharedTable("gem")
	if len(gem) != 5 {
		t.Fatalf("gem rows = %d, want 5", len(gem))
	}
	if !gem[0].Nothing || gem[0].Weight != 63 || gem[0].OutOf != 128 {
		t.Fatalf("gem nothing = %+v", gem[0])
	}
	if gem[1].ItemID != 1623 || gem[1].Quantity != [2]int{1, 1} {
		t.Fatalf("gem sapphire = %+v", gem[1])
	}
	// "100 (noted)" parses to quantity [1,1] with Noted true, as
	// in TS (parseQuantityRaw matches only a bare int or range).
	if gem[2].ItemID != 442 || !gem[2].Noted || gem[2].Quantity != [2]int{1, 1} {
		t.Fatalf("gem silver ore = %+v", gem[2])
	}
	if gem[3].Quantity != [2]int{19500, 20000} {
		t.Fatalf("gem coins = %+v", gem[3])
	}
	if gem[4].SharedTable != "megaRare" {
		t.Fatalf("gem mega-rare ref = %+v", gem[4])
	}
	seed, ok := store.NpcDropSeedTable("seed")
	if !ok {
		t.Fatal("seed table missing")
	}
	if seed.Multiplier != 10 || len(seed.Categories) != 2 {
		t.Fatalf("seed = %+v", seed)
	}
	if seed.Categories[0].MaximumExclusive != 485 || !seed.Categories[0].HasMaximumExclusive {
		t.Fatalf("seed cat 0 = %+v", seed.Categories[0])
	}
	if seed.Categories[1].HasMaximumExclusive {
		t.Fatalf("seed last category must have no maximum: %+v", seed.Categories[1])
	}
	if seed.Categories[0].Entries[0].ItemID != 5318 || seed.Categories[0].Entries[0].Quantity != [2]int{4, 4} {
		t.Fatalf("seed potato = %+v", seed.Categories[0].Entries[0])
	}
	// The broken seed table (categories do not start at 0) is not
	// a seed table, and with no flat entries it is not shared.
	if _, ok := store.NpcDropSeedTable("broken_seed"); ok {
		t.Fatal("broken_seed must not parse as a seed table")
	}
	if got := store.NpcDropSharedTable("broken_seed"); got != nil {
		t.Fatalf("broken_seed shared = %v", got)
	}
	if got := store.NpcDropSharedTable("empty"); got != nil {
		t.Fatalf("empty shared = %v", got)
	}
}

func TestLoadNpcDropsRealFiles(t *testing.T) {
	var logs logCapture
	store, err := Load("../../data/definitions", LoadOptions{Logf: logs.logf})
	if err != nil {
		t.Fatalf("Load real definitions: %v", err)
	}
	stats := store.NpcDropStats()
	// Counts computed over the shipped files (see the test's
	// comments): the plan's "3,086 NPCs after overrides" is wrong
	// — 3,086 dump NPCs, minus Ent 7234 removed, plus 108 new
	// addNpcs ids = 3,193 records, of which 3,190 resolve to a
	// table (Vorkath 8058/8060 and Amoxliatl 13686 have none).
	want := NpcDropStats{
		Tables: 1106, NPCs: 3193, NPCsWithTables: 3190, SharedTables: 13, SeedTables: 1,
		BossNPCs: 3, Entries: 14737, Tertiary: 1592, UnusableSubtableRows: 0,
		UnresolvedItems: 2, SlayerKeysStripped: 27, AssumedQuantityRows: 12,
		SkippedEntries: 0, SkippedTables: 0, SkippedNPCs: 0, RemovedNPCs: 1, AddedNPCs: 108,
	}
	if stats != want {
		t.Fatalf("stats = %+v\nwant %+v", stats, want)
	}
	if !logs.contains("[npc-drops] Loaded 1106 tables for 3193 NPCs") {
		t.Fatalf("logs = %q", logs.joined())
	}

	// No Slayer key remains in any table.
	for _, table := range store.NpcDropTables() {
		for _, entry := range append(append([]NpcDropEntry(nil), table.Entries...), table.Tertiary...) {
			if entry.HasItemID && (entry.ItemID == 23083 || entry.ItemID == 23490) {
				t.Fatalf("table %s still has Slayer key %d", table.ID, entry.ItemID)
			}
		}
	}

	// Goblin 3036: table 1, 20 entries + 4 tertiary, Bones always.
	goblin := store.NpcDropTable(3036)
	if len(goblin) != 1 || goblin[0].ID != "goblin_drop_table1" {
		t.Fatalf("goblin tables = %v", tableIDs(goblin))
	}
	if len(goblin[0].Entries) != 20 || len(goblin[0].Tertiary) != 4 {
		t.Fatalf("goblin entries=%d tertiary=%d", len(goblin[0].Entries), len(goblin[0].Tertiary))
	}
	if !goblin[0].HasMainMaxRoll || goblin[0].MainMaxRoll != 128 {
		t.Fatalf("goblin main_max_roll = %d (has=%v)", goblin[0].MainMaxRoll, goblin[0].HasMainMaxRoll)
	}
	if !goblin[0].Entries[0].Always || goblin[0].Entries[0].ItemID != 526 {
		t.Fatalf("goblin first entry = %+v", goblin[0].Entries[0])
	}

	// Chicken 1174, Cow 2790 (all-always, no main_max_roll), Man 3106.
	chicken := store.NpcDropTable(1174)
	if len(chicken) != 1 || chicken[0].ID != "chicken" {
		t.Fatalf("chicken tables = %v", tableIDs(chicken))
	}
	cow := store.NpcDropTable(2790)
	if len(cow) != 1 || cow[0].ID != "cow" || cow[0].HasMainMaxRoll {
		t.Fatalf("cow tables = %v (hasMainMaxRoll=%v)", tableIDs(cow), cow[0].HasMainMaxRoll)
	}
	if len(cow[0].Entries) != 3 {
		t.Fatalf("cow entries = %d", len(cow[0].Entries))
	}
	man := store.NpcDropTable(3106)
	if len(man) != 1 || man[0].ID != "man" {
		t.Fatalf("man tables = %v", tableIDs(man))
	}
	var herb, energy *NpcDropEntry
	for i := range man[0].Entries {
		if man[0].Entries[i].SharedTable == "herb" {
			herb = &man[0].Entries[i]
		}
		if man[0].Entries[i].ItemID == 3010 {
			energy = &man[0].Entries[i]
		}
	}
	if herb == nil || herb.Weight != 23 || herb.OutOf != 128 {
		t.Fatalf("man herb entry = %+v", herb)
	}
	if energy == nil || !energy.SeparateRoll || energy.Weight != 1 || energy.OutOf != 8 {
		t.Fatalf("man energy potion = %+v", energy)
	}

	// Rat 2854: the empty main table — no crash, no main drops.
	rat := store.NpcDropTable(2854)
	if len(rat) != 1 || rat[0].ID != "rat" || len(rat[0].Entries) != 0 || len(rat[0].Tertiary) != 3 {
		t.Fatalf("rat tables = %v entries=%d tertiary=%d", tableIDs(rat), len(rat[0].Entries), len(rat[0].Tertiary))
	}

	// Overrides: zombie pirate keeps the dump table first and
	// gains the second; Ent 7234 is gone; Maggot King is added.
	zombie := store.NpcDropTable(13489)
	if len(zombie) != 2 || zombie[0].ID != "zombie_pirate_2" || zombie[1].ID != "zombie_pirate" {
		t.Fatalf("zombie pirate tables = %v", tableIDs(zombie))
	}
	if _, ok := store.NpcDropNPC(7234); ok {
		t.Fatal("Ent 7234 must be removed by overrides")
	}
	maggot := store.NpcDropTable(15741)
	if len(maggot) != 1 || maggot[0].ID != "maggot_king_2" {
		t.Fatalf("maggot king tables = %v", tableIDs(maggot))
	}
	// Abyssal orphan pet added to abyssal_sire tertiary, 1/2560.
	sire := store.NpcDropTableByID("abyssal_sire")
	var orphan *NpcDropEntry
	for i := range sire.Tertiary {
		if sire.Tertiary[i].ItemID == 13262 {
			orphan = &sire.Tertiary[i]
		}
	}
	if orphan == nil || orphan.Weight != 1 || orphan.OutOf != 2560 {
		t.Fatalf("abyssal orphan = %+v", orphan)
	}
	// Yama: no Contract section remains anywhere.
	yama := store.NpcDropTableByID("yama")
	for _, entry := range append(append([]NpcDropEntry(nil), yama.Entries...), yama.Tertiary...) {
		if entry.Section == "Contract" {
			t.Fatalf("yama still has Contract entry %+v", entry)
		}
	}
	// Boss NPCs and their tables.
	for _, id := range []int{124, 13668, 14860} {
		if !store.NpcDropIsBoss(id) {
			t.Fatalf("NpcDropIsBoss(%d) = false", id)
		}
	}
	if !yama.Boss {
		t.Fatal("yama table must be marked boss")
	}
	if store.NpcDropTableByID("goblin_drop_table1").Boss {
		t.Fatal("goblin table must not be marked boss")
	}
	// Zero-table NPCs resolve to nothing.
	for _, id := range []int{8058, 8060, 13686} {
		if got := store.NpcDropTable(id); got != nil {
			t.Fatalf("NpcDropTable(%d) = %v, want nil", id, tableIDs(got))
		}
	}

	// Lowercase "always" is still an always drop (flag decides).
	vanguard := store.NpcDropTableByID("vanguard")
	var aid *NpcDropEntry
	for i := range vanguard.Entries {
		if vanguard.Entries[i].ItemID == 20984 {
			aid = &vanguard.Entries[i]
		}
	}
	if aid == nil || !aid.Always || aid.RarityRaw != "always" {
		t.Fatalf("vanguard Xeric's aid = %+v", aid)
	}
	// Unresolved "Brimstone key" tables still load their real row.
	dagannoth := store.NpcDropTableByID("reanimated_dagannoth")
	if len(dagannoth.Entries) != 1 || dagannoth.Entries[0].ItemID != 7857 {
		t.Fatalf("reanimated_dagannoth = %+v", dagannoth.Entries)
	}

	// Reward chains: the 4 Forgotten Four tables only, 5 stages each.
	chained := 0
	for _, table := range store.NpcDropTables() {
		if table.RewardChain != nil {
			chained++
			if len(table.RewardChain.Stages) != 5 || table.RewardChain.StateDomain != "forgotten_four" {
				t.Fatalf("table %s chain = %+v", table.ID, table.RewardChain)
			}
			if len(table.TertiaryGroups) != 1 {
				t.Fatalf("table %s tertiary groups = %d", table.ID, len(table.TertiaryGroups))
			}
		}
	}
	if chained != 4 {
		t.Fatalf("chained tables = %d, want 4", chained)
	}
	// Ent rolls 25 times per kill.
	if ent := store.NpcDropTableByID("ent"); ent.Rolls != 25 {
		t.Fatalf("ent rolls = %d, want 25", ent.Rolls)
	}

	// Shared subtables: 13 names; rareDrop nests gem + megaRare.
	names := store.NpcDropSharedTableNames()
	if len(names) != 13 {
		t.Fatalf("shared names = %v", names)
	}
	rare := store.NpcDropSharedTable("rareDrop")
	var gemRef, megaRef, silver *NpcDropSharedEntry
	for i := range rare {
		switch {
		case rare[i].SharedTable == "gem":
			gemRef = &rare[i]
		case rare[i].SharedTable == "megaRare":
			megaRef = &rare[i]
		case rare[i].ItemID == 442:
			silver = &rare[i]
		}
	}
	if gemRef == nil || gemRef.Weight != 20 || gemRef.OutOf != 128 {
		t.Fatalf("rareDrop gem ref = %+v", gemRef)
	}
	if megaRef == nil || megaRef.Weight != 15 || megaRef.OutOf != 128 {
		t.Fatalf("rareDrop megaRare ref = %+v", megaRef)
	}
	if silver == nil || !silver.Noted || silver.Quantity != [2]int{1, 1} {
		t.Fatalf("rareDrop silver ore = %+v (TS: [1,1] noted)", silver)
	}
	// Seed categories: multiplier 10, 6 bands, maxima as dumped.
	seed, ok := store.NpcDropSeedTable("seed")
	if !ok {
		t.Fatal("seed table missing")
	}
	if seed.Multiplier != 10 || len(seed.Categories) != 6 {
		t.Fatalf("seed multiplier=%d categories=%d", seed.Multiplier, len(seed.Categories))
	}
	maxima := []int{485, 728, 850, 947, 995}
	for i, max := range maxima {
		if !seed.Categories[i].HasMaximumExclusive || seed.Categories[i].MaximumExclusive != max {
			t.Fatalf("seed category %d = %+v", i, seed.Categories[i])
		}
	}
	if seed.Categories[5].HasMaximumExclusive {
		t.Fatalf("seed last category must have no maximum: %+v", seed.Categories[5])
	}
	total := 0
	for _, category := range seed.Categories {
		total += len(category.Entries)
	}
	if total != 44 {
		t.Fatalf("seed entries = %d, want 44", total)
	}
	if seed.Categories[0].Entries[0].ItemID != 5318 || seed.Categories[0].Entries[0].Quantity != [2]int{4, 4} {
		t.Fatalf("seed first entry = %+v", seed.Categories[0].Entries[0])
	}
	// The flattened seed pool matches the category total.
	if got := store.NpcDropSharedTable("seed"); len(got) != 44 {
		t.Fatalf("flattened seed rows = %d, want 44", len(got))
	}
	// Every shared-table reference from a main table resolves.
	for _, table := range store.NpcDropTables() {
		for _, entry := range table.Entries {
			if entry.SharedTable != "" && store.NpcDropSharedTable(entry.SharedTable) == nil {
				t.Fatalf("table %s references unknown shared table %q", table.ID, entry.SharedTable)
			}
		}
	}
}
