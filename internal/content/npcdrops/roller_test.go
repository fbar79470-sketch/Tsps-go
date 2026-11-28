package npcdrops

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"strconv"
	"strings"
	"testing"
)

// Tests for the drop-table roller.
//
// Two kinds of tables are used:
//
//   - Real tables, loaded from data/definitions/npc-drops.json and
//     npc-drop-subtables.json by the test-only parse below
//     (loadRealData). That parse is scaffolding for these tests only —
//     the production loader is Worker A's internal/definition code,
//     which must produce equivalent shapes (see types.go). Using the
//     real files means the live-slice tables (goblin, chicken, cow,
//     man, rat, corporeal beast, duke_sucellus) are tested exactly as
//     dumped, with no transcription risk.
//   - Synthetic tables, for exact scripted-RNG tests where the draw
//     order and consumption must be pinned (pre-roll first-hit-wins,
//     sigil group weights, depth cap, seed banding edges).
//
// Seeded RNG (math/rand with a fixed seed) makes every distribution
// test deterministic.

// scriptedRNG is an RNG driven by fixed sequences, for exact tests.
// Float64 values are returned in order (0 once exhausted); Intn
// values are taken modulo n (0 once exhausted). fi/ii count the draws
// consumed, which pins the algorithm's draw order.
type scriptedRNG struct {
	floats []float64
	ints   []int
	fi     int
	ii     int
}

func (s *scriptedRNG) Float64() float64 {
	if s.fi >= len(s.floats) {
		return 0
	}
	v := s.floats[s.fi]
	s.fi++
	return v
}

func (s *scriptedRNG) Intn(n int) int {
	if s.ii >= len(s.ints) || n <= 0 {
		return 0
	}
	v := s.ints[s.ii]
	s.ii++
	return v % n
}

// seededRoller returns a Roller over shared with a seeded RNG and a
// log recorder. The recorder lets tests assert the ordered fail-closed
// log line.
func seededRoller(shared map[string]*SharedTable, seed int64) (*Roller, *[]string) {
	var logs []string
	r := NewRoller(shared)
	r.RNG = rand.New(rand.NewSource(seed))
	r.Log = func(format string, args ...any) {
		logs = append(logs, fmt.Sprintf(format, args...))
	}
	return r, &logs
}

// --- test-only real-file parsing (NOT the loader) --------------------

// rawSharedEntry / rawSubtable mirror the dump's raw string shapes.
type rawSharedEntry struct {
	ItemID      int    `json:"item_id"`
	Name        string `json:"name"`
	QuantityRaw string `json:"quantity_raw"`
	RarityRaw   string `json:"rarity_raw"`
	SharedTable string `json:"shared_table"`
	Nothing     bool   `json:"nothing"`
}

type rawSeedCategory struct {
	Minimum          int              `json:"minimum"`
	MaximumExclusive *int             `json:"maximum_exclusive"`
	Entries          []rawSharedEntry `json:"entries"`
}

type rawSubtable struct {
	Name                  string            `json:"name"`
	Entries               []rawSharedEntry  `json:"entries"`
	CombatLevelMultiplier int               `json:"combat_level_multiplier"`
	Categories            []rawSeedCategory `json:"categories"`
}

// parseSharedRarity is the test copy of NpcDrops.plugin.js:130-141
// ("N/D" -> {weight, outOf}).
func parseSharedRarity(t *testing.T, raw string) (float64, float64) {
	t.Helper()
	parts := strings.Split(raw, "/")
	if len(parts) != 2 {
		t.Fatalf("bad shared rarity %q", raw)
	}
	w, err1 := strconv.Atoi(strings.TrimSpace(parts[0]))
	o, err2 := strconv.Atoi(strings.TrimSpace(parts[1]))
	if err1 != nil || err2 != nil || o <= 0 || w <= 0 {
		t.Fatalf("bad shared rarity %q", raw)
	}
	return float64(w), float64(o)
}

// parseQuantityRaw is the test copy of NpcDrops.plugin.js:143-157.
// Only a bare integer or a bare range parses; anything else (notably
// "100 (noted)") yields [1,1], exactly as TS falls back with
// `parseQuantityRaw(...) || [1, 1]` (loadDrops sharedRows).
func parseQuantityRaw(t *testing.T, raw string) ([2]int, bool) {
	t.Helper()
	noted := strings.Contains(strings.ToLower(raw), "(noted)")
	clean := strings.TrimSpace(raw)
	clean = strings.ReplaceAll(clean, "–", "-")
	if i := strings.Index(clean, "-"); i >= 0 {
		lo, err1 := strconv.Atoi(strings.TrimSpace(clean[:i]))
		hi, err2 := strconv.Atoi(strings.TrimSpace(clean[i+1:]))
		if err1 != nil || err2 != nil {
			t.Fatalf("bad quantity_raw %q", raw)
		}
		return [2]int{lo, hi}, noted
	}
	if v, err := strconv.Atoi(clean); err == nil {
		return [2]int{v, v}, noted
	}
	return [2]int{1, 1}, noted
}

func convertSharedEntry(t *testing.T, raw rawSharedEntry) SharedEntry {
	t.Helper()
	e := SharedEntry{
		ItemID:      raw.ItemID,
		Name:        raw.Name,
		Nothing:     raw.Nothing,
		SharedTable: raw.SharedTable,
	}
	if raw.RarityRaw != "" {
		w, o := parseSharedRarity(t, raw.RarityRaw)
		e.Weight, e.OutOf = w, o
	}
	if raw.QuantityRaw != "" {
		q, noted := parseQuantityRaw(t, raw.QuantityRaw)
		e.Quantity, e.Noted = q, noted
	}
	return e
}

// loadRealData parses the real drop files into the roller's types.
// It is test scaffolding, not the production loader.
func loadRealData(t *testing.T) (map[string]*Table, map[string]*SharedTable) {
	t.Helper()
	dropsJSON, err := os.ReadFile("../../../data/definitions/npc-drops.json")
	if err != nil {
		t.Fatalf("read npc-drops.json: %v", err)
	}
	var dropFile struct {
		Tables map[string]*Table          `json:"tables"`
		NPCs   map[string]json.RawMessage `json:"npcs"`
	}
	if err := json.Unmarshal(dropsJSON, &dropFile); err != nil {
		t.Fatalf("parse npc-drops.json: %v", err)
	}
	subJSON, err := os.ReadFile("../../../data/definitions/npc-drop-subtables.json")
	if err != nil {
		t.Fatalf("read npc-drop-subtables.json: %v", err)
	}
	var raw map[string]rawSubtable
	if err := json.Unmarshal(subJSON, &raw); err != nil {
		t.Fatalf("parse npc-drop-subtables.json: %v", err)
	}
	shared := make(map[string]*SharedTable, len(raw))
	for name, rs := range raw {
		st := &SharedTable{Name: name, CombatLevelMultiplier: rs.CombatLevelMultiplier}
		for _, re := range rs.Entries {
			st.Entries = append(st.Entries, convertSharedEntry(t, re))
		}
		for _, rc := range rs.Categories {
			cat := SeedCategory{Minimum: rc.Minimum, MaximumExclusive: rc.MaximumExclusive}
			for _, re := range rc.Entries {
				cat.Entries = append(cat.Entries, convertSharedEntry(t, re))
			}
			st.Categories = append(st.Categories, cat)
		}
		shared[name] = st
	}
	return dropFile.Tables, shared
}

// --- small assertion helpers -----------------------------------------

func countItem(drops []Drop, id int) int {
	n := 0
	for _, d := range drops {
		if d.ItemID == id {
			n++
		}
	}
	return n
}

func hasItem(drops []Drop, id int) bool { return countItem(drops, id) > 0 }

// reachableRanges returns every item id a table can produce (main,
// bonus, tertiary, tertiary groups, and transitively referenced
// shared tables, plus the Legends-gate talismans when gem is
// reachable) with the quantity ranges each can roll in. Quantity
// [0,0] (absent) normalises to [1,1], matching rollRange.
func reachableRanges(table *Table, shared map[string]*SharedTable) map[int][][2]int {
	out := map[int][][2]int{}
	gemReachable := false
	add := func(id int, q [2]int) {
		if id <= 0 {
			return
		}
		if q == [2]int{0, 0} {
			q = [2]int{1, 1}
		}
		out[id] = append(out[id], q)
	}
	var walkShared func(name string, depth int)
	walkShared = func(name string, depth int) {
		if depth > MaxSharedDepth {
			return
		}
		if name == "gem" {
			gemReachable = true
		}
		st := shared[name]
		if st == nil {
			return
		}
		var rows []SharedEntry
		if len(st.Categories) > 0 {
			for _, c := range st.Categories {
				rows = append(rows, c.Entries...)
			}
		} else {
			rows = st.Entries
		}
		for _, e := range rows {
			if e.SharedTable != "" {
				walkShared(e.SharedTable, depth+1)
				continue
			}
			add(e.ItemID, e.Quantity)
		}
	}
	var walkEntry func(e *Entry)
	walkEntry = func(e *Entry) {
		if e.SharedTable != "" {
			walkShared(e.SharedTable, 0)
			return
		}
		add(e.ItemID, e.Quantity)
		for _, alt := range e.QuantityAlternatives {
			add(e.ItemID, alt.Quantity)
		}
		for i := range e.BonusDrops {
			b := &e.BonusDrops[i]
			add(b.ItemID, b.Quantity)
			for _, alt := range b.QuantityAlternatives {
				add(b.ItemID, alt.Quantity)
			}
		}
	}
	for i := range table.Entries {
		walkEntry(&table.Entries[i])
	}
	for i := range table.Tertiary {
		walkEntry(&table.Tertiary[i])
	}
	for gi := range table.TertiaryGroups {
		for i := range table.TertiaryGroups[gi].Entries {
			walkEntry(&table.TertiaryGroups[gi].Entries[i])
		}
	}
	if gemReachable {
		add(NatureTalismanID, [2]int{1, 1})
		add(ChaosTalismanID, [2]int{1, 1})
	}
	return out
}

func assertValidDrops(t *testing.T, drops []Drop, allowed map[int][][2]int) {
	t.Helper()
	for _, d := range drops {
		if d.ItemID <= 0 {
			t.Errorf("drop with invalid item id %d: %+v", d.ItemID, d)
			continue
		}
		if d.Amount <= 0 {
			t.Errorf("drop with non-positive amount: %+v", d)
			continue
		}
		ranges, ok := allowed[d.ItemID]
		if !ok {
			t.Errorf("drop of item %d not reachable from this table: %+v", d.ItemID, d)
			continue
		}
		inRange := false
		for _, q := range ranges {
			if d.Amount >= q[0] && d.Amount <= q[1] {
				inRange = true
				break
			}
		}
		if !inRange {
			t.Errorf("drop of item %d amount %d outside ranges %v", d.ItemID, d.Amount, ranges)
		}
	}
}

// --- real-file shape -------------------------------------------------

func TestLoadRealFiles(t *testing.T) {
	tables, shared := loadRealData(t)
	if len(tables) != 1106 {
		t.Errorf("tables = %d, want 1106", len(tables))
	}
	if len(shared) != 13 {
		t.Errorf("shared tables = %d, want 13", len(shared))
	}
	weightSum := func(rows []SharedEntry) float64 {
		sum := 0.0
		for _, e := range rows {
			sum += e.Weight
		}
		return sum
	}
	for _, name := range []string{"herb", "gem", "rareDrop", "megaRare", "gwdRareDrop", "gwdGem", "gwdMegaRare"} {
		if got := weightSum(shared[name].Entries); got != 128 {
			t.Errorf("subtable %s weight sum = %v, want 128", name, got)
		}
	}
	if got := weightSum(shared["talisman"].Entries); got != 70 {
		t.Errorf("talisman weight sum = %v, want 70", got)
	}
	seed := shared["seed"]
	if seed.CombatLevelMultiplier != 10 {
		t.Errorf("seed multiplier = %d, want 10", seed.CombatLevelMultiplier)
	}
	wantSeedSums := []float64{1008, 1000, 1000, 1000, 1000, 1100}
	if len(seed.Categories) != len(wantSeedSums) {
		t.Fatalf("seed categories = %d, want %d", len(seed.Categories), len(wantSeedSums))
	}
	for i, want := range wantSeedSums {
		if got := weightSum(seed.Categories[i].Entries); got != want {
			t.Errorf("seed category %d weight sum = %v, want %v", i, got, want)
		}
	}
	// The 4 DT2 tables are the only ones carrying a reward chain and
	// the only ones carrying tertiary groups.
	chained := []string{}
	for id, tb := range tables {
		if tb.RewardChain != nil {
			chained = append(chained, id)
		}
		if len(tb.TertiaryGroups) != 0 && tb.RewardChain == nil {
			t.Errorf("table %s has tertiary groups but no reward chain", id)
		}
	}
	if len(chained) != 4 {
		t.Errorf("reward_chain tables = %v, want 4", chained)
	}
}

// --- live slice: goblin / chicken / cow / man / rat ------------------

func TestGoblinTable1AlwaysAndMain(t *testing.T) {
	tables, shared := loadRealData(t)
	table := tables["goblin_drop_table1"]
	if table.MainMaxRoll != 128 {
		t.Fatalf("goblin table 1 main_max_roll = %v, want 128", table.MainMaxRoll)
	}
	r, _ := seededRoller(shared, 7)
	allowed := reachableRanges(table, shared)
	// Main item ids (non-always, non-tertiary) for the at-most-one
	// main drop per kill check.
	mainIDs := map[int]bool{}
	for _, e := range table.Entries {
		if !e.Always && e.ItemID > 0 {
			mainIDs[e.ItemID] = true
		}
	}
	tertIDs := map[int]bool{}
	for _, e := range table.Tertiary {
		tertIDs[e.ItemID] = true
	}
	const kills = 2000
	onlyBones := 0
	coins15 := 0
	ensouled := 0
	for k := 0; k < kills; k++ {
		drops := r.RollTable(table, KillContext{NPCName: "Goblin"})
		assertValidDrops(t, drops, allowed)
		if countItem(drops, 526) != 1 {
			t.Fatalf("kill %d: Bones count = %d, want exactly 1 (drops %+v)", k, countItem(drops, 526), drops)
		}
		mainDrops := 0
		for _, d := range drops {
			if mainIDs[d.ItemID] {
				mainDrops++
			}
			if d.ItemID == 995 && d.Amount == 15 {
				coins15++
			}
			if d.ItemID == 13448 {
				ensouled++
			}
		}
		if mainDrops > 1 {
			t.Fatalf("kill %d: %d main drops, want at most 1: %+v", k, mainDrops, drops)
		}
		if len(drops) == 1 {
			onlyBones++
		}
		// The 1/1 conditional tertiary (Goblin skull) never drops
		// without a quest answer.
		if hasItem(drops, 7812) {
			t.Fatalf("kill %d: Goblin skull dropped without a quest verdict: %+v", k, drops)
		}
	}
	if onlyBones < 100 {
		t.Errorf("kills with only Bones = %d, want > 100 (Nothing slot 38/128)", onlyBones)
	}
	// Coins x15 at 28/128: expect ~437 over 2000 kills.
	if coins15 < 350 || coins15 > 525 {
		t.Errorf("Coins x15 count = %d, want 350..525", coins15)
	}
	if ensouled == 0 {
		t.Errorf("Ensouled goblin head never dropped over %d kills (1/35)", kills)
	}
}

func TestChickenAlwaysDrops(t *testing.T) {
	tables, shared := loadRealData(t)
	table := tables["chicken"]
	r, _ := seededRoller(shared, 11)
	allowed := reachableRanges(table, shared)
	const kills = 500
	for k := 0; k < kills; k++ {
		drops := r.RollTable(table, KillContext{NPCName: "Chicken"})
		assertValidDrops(t, drops, allowed)
		if countItem(drops, 526) != 1 || countItem(drops, 2138) != 1 {
			t.Fatalf("kill %d: want Bones x1 + Raw chicken x1 exactly, got %+v", k, drops)
		}
		for _, d := range drops {
			switch d.ItemID {
			case 526, 2138, 23182:
			case 314:
				if d.Amount != 5 && d.Amount != 15 {
					t.Fatalf("kill %d: Feather amount %d, want 5 or 15", k, d.Amount)
				}
			default:
				t.Fatalf("kill %d: unexpected drop %+v", k, d)
			}
		}
		if hasItem(drops, 2832) {
			t.Fatalf("kill %d: Key (medium) dropped without a quest verdict", k)
		}
	}
}

func TestCowAllAlwaysNoMainRoll(t *testing.T) {
	tables, shared := loadRealData(t)
	table := tables["cow"]
	if table.MainMaxRoll != 0 {
		t.Fatalf("cow main_max_roll = %v, want absent (0)", table.MainMaxRoll)
	}
	r, _ := seededRoller(shared, 13)
	const kills = 500
	for k := 0; k < kills; k++ {
		drops := r.RollTable(table, KillContext{NPCName: "Cow"})
		if len(drops) < 3 {
			t.Fatalf("kill %d: %d drops, want at least the 3 always drops: %+v", k, len(drops), drops)
		}
		if countItem(drops, 1739) != 1 || countItem(drops, 2132) != 1 || countItem(drops, 526) != 1 {
			t.Fatalf("kill %d: want Cowhide + Raw beef + Bones exactly once each, got %+v", k, drops)
		}
		for _, d := range drops {
			if d.ItemID != 1739 && d.ItemID != 2132 && d.ItemID != 526 && d.ItemID != 23182 {
				t.Fatalf("kill %d: cow dropped non-always, non-tertiary item %+v (no main roll exists)", k, d)
			}
			if d.Amount != 1 {
				t.Fatalf("kill %d: amount %d, want 1: %+v", k, d.Amount, d)
			}
		}
	}
}

func TestManSeparateRollAndHerb(t *testing.T) {
	tables, shared := loadRealData(t)
	table := tables["man"]
	r, _ := seededRoller(shared, 17)
	allowed := reachableRanges(table, shared)
	herbIDs := map[int]bool{}
	for _, e := range shared["herb"].Entries {
		herbIDs[e.ItemID] = true
	}
	mainIDs := map[int]bool{}
	for _, e := range table.Entries {
		if !e.Always && e.ItemID > 0 {
			mainIDs[e.ItemID] = true
		}
	}
	const kills = 5000
	potions := 0
	herbs := 0
	both := 0
	for k := 0; k < kills; k++ {
		drops := r.RollTable(table, KillContext{NPCName: "Man"})
		assertValidDrops(t, drops, allowed)
		if countItem(drops, 526) != 1 {
			t.Fatalf("kill %d: Bones count = %d, want 1: %+v", k, countItem(drops, 526), drops)
		}
		gotPotion := hasItem(drops, 3010)
		gotMain := false
		for _, d := range drops {
			if mainIDs[d.ItemID] {
				gotMain = true
			}
			if herbIDs[d.ItemID] {
				herbs++
			}
			if d.ItemID == 877 && (d.Amount < 2 || d.Amount > 12) {
				t.Fatalf("kill %d: Bronze bolts amount %d outside [2,12]", k, d.Amount)
			}
		}
		if gotPotion {
			potions++
		}
		// Separate roll fires independently of the main roll: a
		// kill can carry both the potion and a main drop.
		if gotPotion && gotMain {
			both++
		}
		if hasItem(drops, 2832) {
			t.Fatalf("kill %d: Key (medium) dropped without a quest verdict", k)
		}
		if hasItem(drops, 11941) {
			t.Fatalf("kill %d: Looting bag dropped outside the Wilderness", k)
		}
	}
	// Energy potion(3) at 1/8: expect ~625 over 5000 kills.
	if potions < 500 || potions > 750 {
		t.Errorf("Energy potion(3) count = %d, want 500..750", potions)
	}
	// Herb subtable at 23/128: expect ~900 herb drops.
	if herbs < 500 {
		t.Errorf("herb drops = %d, want > 500", herbs)
	}
	if both == 0 {
		t.Errorf("no kill carried both the separate-roll potion and a main drop over %d kills", kills)
	}
}

func TestRatEmptyTableNoCrash(t *testing.T) {
	tables, shared := loadRealData(t)
	table := tables["rat"]
	if len(table.Entries) != 0 {
		t.Fatalf("rat table has %d main entries, want 0", len(table.Entries))
	}
	r, _ := seededRoller(shared, 19)
	const kills = 500
	bones := 0
	for k := 0; k < kills; k++ {
		drops := r.RollTable(table, KillContext{NPCName: "Rat"})
		for _, d := range drops {
			if d.ItemID != 7836 {
				t.Fatalf("kill %d: rat dropped %+v, want only Rat bone (1/4 tertiary)", k, d)
			}
			bones++
		}
	}
	// Rat bone at 1/4: expect ~125 over 500 kills.
	if bones < 50 {
		t.Errorf("Rat bone count = %d, want > 50", bones)
	}
}

// --- pre-roll / separate-roll exactness (scripted RNG) --------------

func TestPreRollFirstHitWins(t *testing.T) {
	table := &Table{
		ID:          "pre-test",
		MainMaxRoll: 2,
		Entries: []Entry{
			{ItemID: 1001, Name: "Pre A", Quantity: [2]int{1, 1}, Weight: 1, OutOf: 2, PreRoll: true},
			{ItemID: 1002, Name: "Pre B", Quantity: [2]int{1, 1}, Weight: 1, OutOf: 2, PreRoll: true},
			{ItemID: 1003, Name: "Main A", Quantity: [2]int{1, 1}, Weight: 1, OutOf: 2},
			{ItemID: 1004, Name: "Main B", Quantity: [2]int{1, 1}, Weight: 1, OutOf: 2},
		},
	}
	// First pre-roll hits: only it drops; B is never rolled and the
	// main roll never happens (1 float consumed).
	rng := &scriptedRNG{floats: []float64{0.1}, ints: []int{0}}
	r := NewRoller(nil)
	r.RNG = rng
	drops := r.RollTable(table, KillContext{})
	if len(drops) != 1 || drops[0].ItemID != 1001 {
		t.Fatalf("first-hit case: drops = %+v, want [{1001 1}]", drops)
	}
	if rng.fi != 1 || rng.ii != 1 {
		t.Fatalf("first-hit case: floats=%d ints=%d, want 1 and 1", rng.fi, rng.ii)
	}
	// First misses, second hits.
	rng = &scriptedRNG{floats: []float64{0.9, 0.1}, ints: []int{0}}
	r.RNG = rng
	drops = r.RollTable(table, KillContext{})
	if len(drops) != 1 || drops[0].ItemID != 1002 {
		t.Fatalf("second-hit case: drops = %+v, want [{1002 1}]", drops)
	}
	if rng.fi != 2 {
		t.Fatalf("second-hit case: floats=%d, want 2", rng.fi)
	}
	// Both miss: the main roll happens.
	rng = &scriptedRNG{floats: []float64{0.9, 0.9, 0.0}, ints: []int{0}}
	r.RNG = rng
	drops = r.RollTable(table, KillContext{})
	if len(drops) != 1 || drops[0].ItemID != 1003 {
		t.Fatalf("both-miss case: drops = %+v, want [{1003 1}]", drops)
	}
	if rng.fi != 3 {
		t.Fatalf("both-miss case: floats=%d, want 3", rng.fi)
	}
}

func TestSeparateRollIndependentOfMain(t *testing.T) {
	table := &Table{
		ID:          "sep-test",
		MainMaxRoll: 2,
		Entries: []Entry{
			{ItemID: 2001, Name: "Separate", Quantity: [2]int{1, 1}, Weight: 1, OutOf: 8, SeparateRoll: true},
			{ItemID: 2002, Name: "Main A", Quantity: [2]int{1, 1}, Weight: 1, OutOf: 2},
			{ItemID: 2003, Name: "Main B", Quantity: [2]int{1, 1}, Weight: 1, OutOf: 2},
		},
	}
	r := NewRoller(nil)
	// Separate hits (0.0*8 < 1) and the main roll still happens.
	rng := &scriptedRNG{floats: []float64{0.0, 0.0}, ints: []int{0, 0}}
	r.RNG = rng
	drops := r.RollTable(table, KillContext{})
	if len(drops) != 2 || drops[0].ItemID != 2001 || drops[1].ItemID != 2002 {
		t.Fatalf("hit case: drops = %+v, want [{2001 1} {2002 1}]", drops)
	}
	// Separate misses (0.9*8 = 7.2 >= 1); main roll alone.
	rng = &scriptedRNG{floats: []float64{0.9, 0.0}, ints: []int{0}}
	r.RNG = rng
	drops = r.RollTable(table, KillContext{})
	if len(drops) != 1 || drops[0].ItemID != 2002 {
		t.Fatalf("miss case: drops = %+v, want [{2002 1}]", drops)
	}
}

func TestRollsRepetitionAndOncePerKill(t *testing.T) {
	table := &Table{
		ID:          "rolls-test",
		MainMaxRoll: 2,
		Rolls:       3,
		Entries: []Entry{
			{ItemID: 3000, Name: "Always", Quantity: [2]int{1, 1}, Always: true},
			{ItemID: 3001, Name: "Pre", Quantity: [2]int{1, 1}, Weight: 1, OutOf: 1, PreRoll: true, OncePerKill: true},
			{ItemID: 3002, Name: "Sep", Quantity: [2]int{1, 1}, Weight: 1, OutOf: 1, SeparateRoll: true, OncePerKill: true},
			{ItemID: 3003, Name: "Main A", Quantity: [2]int{1, 1}, Weight: 1, OutOf: 2},
			{ItemID: 3004, Name: "Main B", Quantity: [2]int{1, 1}, Weight: 1, OutOf: 2},
		},
	}
	// Roll 1: pre hits (replaces main) + separate hits. Rolls 2-3:
	// once_per_kill rows are skipped; each rolls the main table.
	rng := &scriptedRNG{floats: []float64{0.0, 0.0, 0.0, 0.5}, ints: []int{0, 0, 0, 0, 0}}
	r := NewRoller(nil)
	r.RNG = rng
	drops := r.RollTable(table, KillContext{})
	want := []Drop{
		{ItemID: 3000, Amount: 1},
		{ItemID: 3001, Amount: 1},
		{ItemID: 3002, Amount: 1},
		{ItemID: 3003, Amount: 1},
		{ItemID: 3004, Amount: 1},
	}
	if len(drops) != len(want) {
		t.Fatalf("drops = %+v, want %+v", drops, want)
	}
	for i, w := range want {
		if drops[i] != w {
			t.Fatalf("drop %d = %+v, want %+v (all %+v)", i, drops[i], w, drops)
		}
	}
	if rng.fi != 4 || rng.ii != 5 {
		t.Fatalf("draws: floats=%d ints=%d, want 4 and 5", rng.fi, rng.ii)
	}
}

// --- Corporeal Beast sigils: rarity is informational -----------------

func TestCorporealSigilsUseWeightNotRarity(t *testing.T) {
	tables, shared := loadRealData(t)
	// Ground the test data in the real file: the three sigils carry
	// weight 3/7, 3/7, 1/7 but a published rarity of 1/1365
	// (0.0007326). The draw must use weight/out_of.
	sigils := map[int]Entry{}
	for _, e := range tables["corporeal_beast"].Entries {
		if e.RollGroup == "Sigils" {
			sigils[e.ItemID] = e
		}
	}
	if len(sigils) != 3 {
		t.Fatalf("sigil entries = %d, want 3", len(sigils))
	}
	if got := sigils[12823]; got.Weight != 3 || got.OutOf != 7 || got.Rarity == got.Weight/got.OutOf {
		t.Fatalf("spectral sigil = %+v, want weight 3 out_of 7 with rarity != 3/7", got)
	}
	if sigils[12823].GroupAccessWeight != 1 || sigils[12823].GroupAccessOutOf != 585 {
		t.Fatalf("spectral sigil access = %v/%v, want 1/585",
			sigils[12823].GroupAccessWeight, sigils[12823].GroupAccessOutOf)
	}

	table := &Table{
		ID:          "sigils-test",
		MainMaxRoll: 128,
		Entries: []Entry{
			{ItemID: 12823, Name: "Spectral sigil", Quantity: [2]int{1, 1}, Weight: 3, OutOf: 7, Rarity: 0.0007326, PreRoll: true, SeparateRoll: true, RollGroup: "Sigils", GroupAccessWeight: 1, GroupAccessOutOf: 585},
			{ItemID: 12827, Name: "Arcane sigil", Quantity: [2]int{1, 1}, Weight: 3, OutOf: 7, Rarity: 0.0007326, PreRoll: true, SeparateRoll: true, RollGroup: "Sigils", GroupAccessWeight: 1, GroupAccessOutOf: 585},
			{ItemID: 12819, Name: "Elysian sigil", Quantity: [2]int{1, 1}, Weight: 1, OutOf: 7, Rarity: 0.0007326, PreRoll: true, SeparateRoll: true, RollGroup: "Sigils", GroupAccessWeight: 1, GroupAccessOutOf: 585},
			{ItemID: 4001, Name: "Main", Quantity: [2]int{1, 1}, Weight: 1, OutOf: 128},
		},
	}
	r := NewRoller(nil)
	roll := func(floats []float64) []Drop {
		rng := &scriptedRNG{floats: floats, ints: []int{0}}
		r.RNG = rng
		return r.RollTable(table, KillContext{})
	}
	// Access hit (f*585 < 1) then weighted pick over 3/3/1 out of 7.
	// f=0.4 -> roll 2.8 -> spectral (3/7 slot); rarity-equal weights
	// would pick arcane here, so this pins weight-based picking.
	if drops := roll([]float64{0.0, 0.4}); len(drops) != 1 || drops[0].ItemID != 12823 {
		t.Fatalf("f=0.4: drops = %+v, want spectral sigil 12823", drops)
	}
	if drops := roll([]float64{0.0, 0.5}); len(drops) != 1 || drops[0].ItemID != 12827 {
		t.Fatalf("f=0.5: drops = %+v, want arcane sigil 12827", drops)
	}
	if drops := roll([]float64{0.0, 0.99}); len(drops) != 1 || drops[0].ItemID != 12819 {
		t.Fatalf("f=0.99: drops = %+v, want elysian sigil 12819", drops)
	}
	// Access boundary: 1/585 = 0.001709...; 0.001 hits, 0.00171 misses.
	if drops := roll([]float64{0.001, 0.0}); len(drops) != 1 || drops[0].ItemID != 12823 {
		t.Fatalf("access f=0.001: drops = %+v, want spectral sigil", drops)
	}
	// On an access miss the main roll proceeds and no sigil appears
	// — the sigils' separate_roll flag does NOT create a separate
	// group roll (pre_roll wins the classification).
	if drops := roll([]float64{0.00171, 0.0}); len(drops) != 1 || drops[0].ItemID != 4001 {
		t.Fatalf("access f=0.00171: drops = %+v, want main item 4001 only", drops)
	}
	// Real-table smoke: sigils are rare (~1/585 per kill).
	real := tables["corporeal_beast"]
	rr, _ := seededRoller(shared, 23)
	sigilDrops := 0
	for k := 0; k < 2000; k++ {
		drops := rr.RollTable(real, KillContext{NPCName: "Corporeal Beast"})
		assertValidDrops(t, drops, reachableRanges(real, shared))
		for _, d := range drops {
			if d.ItemID == 12823 || d.ItemID == 12827 || d.ItemID == 12819 {
				sigilDrops++
			}
		}
	}
	if sigilDrops > 20 {
		t.Errorf("sigil drops = %d over 2000 kills, want <= 20 (rate 1/585)", sigilDrops)
	}
}

// --- tertiary verdicts / 1-1 rule / post-filter ----------------------

func TestTertiaryConditionalVerdicts(t *testing.T) {
	// An always entry and a 1/1 conditional tertiary sharing one
	// item id, so the post-filter's effect on always drops is visible.
	table := &Table{
		ID: "verdict-test",
		Entries: []Entry{
			{ItemID: 7812, Name: "Goblin skull", Quantity: [2]int{1, 1}, Always: true},
		},
		Tertiary: []Entry{
			{ItemID: 7812, Name: "Goblin skull", Quantity: [2]int{1, 1}, Weight: 1, OutOf: 1, QuestKey: "quest_ragandboneman1", QuestMode: "RequiresDuring"},
			{ItemID: 13448, Name: "Ensouled goblin head", Quantity: [2]int{1, 1}, Weight: 1, OutOf: 35},
		},
	}
	var seenKey, seenMode string
	verdict := func(allowed bool) QuestVerdictFunc {
		return func(itemID int, questKey, questMode string) (bool, bool) {
			seenKey, seenMode = questKey, questMode
			if itemID == 7812 {
				return allowed, true
			}
			return false, false
		}
	}
	// No answer: the always drop stays, the 1/1 tertiary is skipped.
	r := NewRoller(nil)
	r.RNG = rand.New(rand.NewSource(5))
	for k := 0; k < 50; k++ {
		if got := countItem(r.RollTable(table, KillContext{}), 7812); got != 1 {
			t.Fatalf("unanswered kill %d: skull count = %d, want 1 (always only)", k, got)
		}
	}
	// Allowed: the tertiary hits on every kill (1/1), alongside the
	// always drop.
	r.QuestVerdict = verdict(true)
	for k := 0; k < 50; k++ {
		if got := countItem(r.RollTable(table, KillContext{}), 7812); got != 2 {
			t.Fatalf("allowed kill %d: skull count = %d, want 2 (always + tertiary)", k, got)
		}
	}
	if seenKey != "quest_ragandboneman1" || seenMode != "RequiresDuring" {
		t.Fatalf("verdict asked with key=%q mode=%q, want quest_ragandboneman1 / RequiresDuring", seenKey, seenMode)
	}
	// Refused: the tertiary is skipped AND the always drop is removed
	// by the post-filter.
	r.QuestVerdict = verdict(false)
	for k := 0; k < 50; k++ {
		if drops := r.RollTable(table, KillContext{}); hasItem(drops, 7812) {
			t.Fatalf("refused kill %d: skull still dropped: %+v", k, drops)
		}
	}
}

// --- Looting bag ------------------------------------------------------

func TestLootingBagWildernessGate(t *testing.T) {
	tables, shared := loadRealData(t)
	man := tables["man"]
	rat := tables["rat"]
	r, _ := seededRoller(shared, 29)
	count := func(table *Table, ctx KillContext, kills int) int {
		n := 0
		for k := 0; k < kills; k++ {
			n += countItem(r.RollTable(table, ctx), 11941)
		}
		return n
	}
	if got := count(man, KillContext{NPCName: "Man"}, 1000); got != 0 {
		t.Errorf("Looting bag dropped %d times outside the Wilderness, want 0", got)
	}
	if got := count(man, KillContext{NPCName: "Man", InWilderness: true}, 1000); got < 35 || got > 100 {
		t.Errorf("Looting bag (wilderness Man) = %d, want 35..100 (1/15)", got)
	}
	for _, name := range []string{"Aviansie", "Abyssal demon", "Black chinchompa"} {
		if got := count(man, KillContext{NPCName: name, InWilderness: true}, 1000); got != 0 {
			t.Errorf("Looting bag dropped %d times from %s in the Wilderness, want 0", got, name)
		}
	}
	if got := count(rat, KillContext{NPCName: "Rat", InWilderness: true}, 1000); got < 10 {
		t.Errorf("Looting bag (wilderness Rat) = %d, want > 10 (1/15)", got)
	}
}

// --- seed banding ------------------------------------------------------

func TestSeedTableBanding(t *testing.T) {
	_, shared := loadRealData(t)
	seed := shared["seed"]
	catIDs := func(i int) map[int]bool {
		ids := map[int]bool{}
		for _, e := range seed.Categories[i].Entries {
			ids[e.ItemID] = true
		}
		return ids
	}
	table := &Table{
		ID:          "seed-test",
		MainMaxRoll: 128,
		Entries: []Entry{
			{SharedTable: "seed", Weight: 128, OutOf: 128, Quantity: [2]int{1, 1}},
		},
	}
	r, _ := seededRoller(shared, 31)
	rollMany := func(ctx KillContext, kills int) map[int]bool {
		seen := map[int]bool{}
		for k := 0; k < kills; k++ {
			for _, d := range r.RollTable(table, ctx) {
				seen[d.ItemID] = true
			}
		}
		return seen
	}
	// Unknown combat level: first (lowest) category only.
	seen := rollMany(KillContext{}, 200)
	for id := range seen {
		if !catIDs(0)[id] {
			t.Fatalf("unknown level: dropped seed %d outside category 0", id)
		}
	}
	if len(seen) == 0 {
		t.Fatalf("unknown level: no seed drops over 200 kills")
	}
	// Combat level 10: bound 100 < 485, still category 0 only.
	seen = rollMany(KillContext{NPCCombatLevel: 10}, 200)
	for id := range seen {
		if !catIDs(0)[id] {
			t.Fatalf("level 10: dropped seed %d outside category 0", id)
		}
	}
	// Combat level 50: bound 500; category 1 (485..728) is reachable.
	seen = rollMany(KillContext{NPCCombatLevel: 50}, 500)
	for id := range seen {
		if !catIDs(0)[id] && !catIDs(1)[id] {
			t.Fatalf("level 50: dropped seed %d outside categories 0-1", id)
		}
	}
	hitCat1 := false
	for id := range seen {
		if catIDs(1)[id] {
			hitCat1 = true
		}
	}
	if !hitCat1 {
		t.Errorf("level 50: no category-1 seed over 500 kills (expect ~3%% of rolls)")
	}
	// Combat level 200: bound 2000; every band is reachable.
	seen = rollMany(KillContext{NPCCombatLevel: 200}, 2000)
	for i := 0; i < 6; i++ {
		hit := false
		for id := range seen {
			if catIDs(i)[id] {
				hit = true
			}
		}
		if !hit {
			t.Errorf("level 200: no category-%d seed over 2000 kills", i)
		}
	}

	// Exact band edges with a scripted RNG: level 50 rolls
	// Intn(500); 484 stays in category 0, 485 enters category 1.
	// Draw order: main-roll float, band int, pool int, quantity int.
	rr := NewRoller(shared)
	roll := func(band int) []Drop {
		rng := &scriptedRNG{floats: []float64{0.0}, ints: []int{band, 0, 0}}
		rr.RNG = rng
		drops := rr.RollTable(table, KillContext{NPCCombatLevel: 50})
		if rng.fi != 1 || rng.ii != 3 {
			t.Fatalf("band %d: draws floats=%d ints=%d, want 1 and 3", band, rng.fi, rng.ii)
		}
		return drops
	}
	if drops := roll(484); len(drops) != 1 || drops[0].ItemID != 5318 || drops[0].Amount != 4 {
		t.Fatalf("band 484: drops = %+v, want [{5318 4}] (potato seed)", drops)
	}
	if drops := roll(485); len(drops) != 1 || drops[0].ItemID != 5305 || drops[0].Amount != 4 {
		t.Fatalf("band 485: drops = %+v, want [{5305 4}] (barley seed)", drops)
	}
}

// --- shared-table depth cap -------------------------------------------

func TestSharedTableDepthCap(t *testing.T) {
	ref := func(to string) SharedEntry {
		return SharedEntry{SharedTable: to, Weight: 1, OutOf: 1}
	}
	shared := map[string]*SharedTable{
		"a": {Name: "a", Entries: []SharedEntry{ref("b")}},
		"b": {Name: "b", Entries: []SharedEntry{ref("c")}},
		"c": {Name: "c", Entries: []SharedEntry{ref("d")}},
		"d": {Name: "d", Entries: []SharedEntry{ref("e")}},
		"e": {Name: "e", Entries: []SharedEntry{ref("f")}},
		"f": {Name: "f", Entries: []SharedEntry{{ItemID: 5001, Quantity: [2]int{1, 1}, Weight: 1, OutOf: 1}}},
		"g": {Name: "g", Entries: []SharedEntry{ref("g")}},
		"h": {Name: "h", Entries: []SharedEntry{ref("i")}},
		"i": {Name: "i", Entries: []SharedEntry{{ItemID: 5002, Quantity: [2]int{1, 1}, Weight: 1, OutOf: 1}}},
	}
	table := func(name string) *Table {
		return &Table{
			ID:          "depth-test",
			MainMaxRoll: 1,
			Entries:     []Entry{{SharedTable: name, Weight: 1, OutOf: 1, Quantity: [2]int{1, 1}}},
		}
	}
	r := NewRoller(shared)
	r.RNG = &scriptedRNG{floats: []float64{0.0}, ints: []int{0, 0, 0, 0, 0, 0, 0}}
	// a->b->c->d->e->f is six tables deep; the depth-5 call returns
	// nothing (cap 4), so the item never drops.
	if drops := r.RollTable(table("a"), KillContext{}); len(drops) != 0 {
		t.Fatalf("6-deep chain: drops = %+v, want none (depth cap 4)", drops)
	}
	// A short chain resolves.
	r.RNG = &scriptedRNG{floats: []float64{0.0}, ints: []int{0, 0}}
	if drops := r.RollTable(table("h"), KillContext{}); len(drops) != 1 || drops[0].ItemID != 5002 {
		t.Fatalf("2-deep chain: drops = %+v, want [{5002 1}]", drops)
	}
	// A self-referential table terminates instead of looping.
	r.RNG = &scriptedRNG{floats: []float64{0.0}, ints: []int{0, 0, 0, 0, 0, 0}}
	if drops := r.RollTable(table("g"), KillContext{}); len(drops) != 0 {
		t.Fatalf("self-referential chain: drops = %+v, want none", drops)
	}
}

// --- gem -> mega-rare Legends' Quest gate ------------------------------

func TestGemMegaRareLegendsGate(t *testing.T) {
	_, shared := loadRealData(t)
	table := func(name string) *Table {
		return &Table{
			ID:          "gem-test",
			MainMaxRoll: 1,
			Entries:     []Entry{{SharedTable: name, Weight: 1, OutOf: 1, Quantity: [2]int{1, 1}}},
		}
	}
	r := NewRoller(shared)
	// Gem pool order: nothing 63, sapphire 32, emerald 16, ruby 8,
	// talisman 3, diamond 2, javelin 1, loop 1, tooth 1, megaRare 1.
	// Intn(128) = 127 lands on the mega-rare ref.
	roll := func(floats []float64, ints []int, ctx KillContext) []Drop {
		rng := &scriptedRNG{floats: floats, ints: ints}
		r.RNG = rng
		return r.RollTable(table("gem"), ctx)
	}
	// Legends' Quest not complete (default): talisman instead.
	if drops := roll([]float64{0.0}, []int{127}, KillContext{MapY: 0}); len(drops) != 1 || drops[0].ItemID != NatureTalismanID {
		t.Fatalf("gate, above ground: drops = %+v, want nature talisman %d", drops, NatureTalismanID)
	}
	if drops := roll([]float64{0.0}, []int{127}, KillContext{MapY: 6400}); len(drops) != 1 || drops[0].ItemID != ChaosTalismanID {
		t.Fatalf("gate, underground (y=6400): drops = %+v, want chaos talisman %d", drops, ChaosTalismanID)
	}
	if drops := roll([]float64{0.0}, []int{127}, KillContext{MapY: 6399}); len(drops) != 1 || drops[0].ItemID != NatureTalismanID {
		t.Fatalf("gate, y=6399: drops = %+v, want nature talisman %d", drops, NatureTalismanID)
	}
	// Legends' Quest complete: the mega-rare table rolls normally.
	// Mega-rare order: nothing 113, rune spear 8, shield half 4,
	// dragon spear 3. Intn(128) = 113 lands on the rune spear.
	r.LegendsQuestComplete = func() bool { return true }
	if drops := roll([]float64{0.0}, []int{127, 113, 0}, KillContext{}); len(drops) != 1 || drops[0].ItemID != 1247 {
		t.Fatalf("quest complete: drops = %+v, want rune spear 1247", drops)
	}
	if drops := roll([]float64{0.0}, []int{127, 0}, KillContext{}); len(drops) != 0 {
		t.Fatalf("quest complete, mega-rare nothing: drops = %+v, want none", drops)
	}
	r.LegendsQuestComplete = nil

	// Nested: rareDrop -> gem -> megaRare is gated too. RareDrop
	// order puts the gem ref at [93,113); Intn(128) = 100 hits it.
	rng := &scriptedRNG{floats: []float64{0.0}, ints: []int{100, 127}}
	r.RNG = rng
	if drops := r.RollTable(table("rareDrop"), KillContext{}); len(drops) != 1 || drops[0].ItemID != NatureTalismanID {
		t.Fatalf("nested RDT->gem->megaRare: drops = %+v, want nature talisman %d", drops, NatureTalismanID)
	}
	// ...while rareDrop's OWN mega-rare slot ([113,128)) needs no
	// quest: Intn(128) = 120 hits it, then 120 in megaRare hits the
	// rune spear.
	rng = &scriptedRNG{floats: []float64{0.0}, ints: []int{120, 120, 0}}
	r.RNG = rng
	if drops := r.RollTable(table("rareDrop"), KillContext{}); len(drops) != 1 || drops[0].ItemID != 1247 {
		t.Fatalf("rareDrop own megaRare: drops = %+v, want rune spear 1247", drops)
	}
}

// --- quantity alternatives / roll counts / bonus drops -----------------

func TestQuantityAlternatives(t *testing.T) {
	r := NewRoller(nil)
	roll := func(table *Table, ctx KillContext, ints []int) []Drop {
		rng := &scriptedRNG{floats: []float64{0.0}, ints: ints}
		r.RNG = rng
		return r.RollTable(table, ctx)
	}
	// Conditional: base quantity against the empty context, the
	// perfect_kill alternative when it holds.
	condTable := &Table{
		ID:          "qa-cond",
		MainMaxRoll: 1,
		Entries: []Entry{{
			ItemID: 1103, Name: "Bronze chainbody", Quantity: [2]int{11, 11}, Weight: 1, OutOf: 1,
			QuantitySelection: "conditional",
			QuantityAlternatives: []QuantityAlternative{
				{Quantity: [2]int{11, 11}},
				{Quantity: [2]int{17, 17}, Condition: &QuantityCondition{Type: "perfect_kill"}},
			},
		}},
	}
	if drops := roll(condTable, KillContext{}, []int{0}); len(drops) != 1 || drops[0].Amount != 11 {
		t.Fatalf("conditional, empty context: drops = %+v, want amount 11", drops)
	}
	if drops := roll(condTable, KillContext{PerfectKill: true}, []int{0}); len(drops) != 1 || drops[0].Amount != 17 {
		t.Fatalf("conditional, perfect kill: drops = %+v, want amount 17", drops)
	}
	// Unknown selection: uniform pick among the listed quantities.
	unknownTable := &Table{
		ID:          "qa-unknown",
		MainMaxRoll: 1,
		Entries: []Entry{{
			ItemID: 23872, Name: "Raw paddlefish", Quantity: [2]int{1, 1}, Weight: 1, OutOf: 1,
			QuantitySelection: "unknown",
			QuantityAlternatives: []QuantityAlternative{
				{Quantity: [2]int{1, 1}},
				{Quantity: [2]int{3, 3}},
			},
		}},
	}
	if drops := roll(unknownTable, KillContext{}, []int{0, 0}); len(drops) != 1 || drops[0].Amount != 1 {
		t.Fatalf("unknown, pick 0: drops = %+v, want amount 1", drops)
	}
	if drops := roll(unknownTable, KillContext{}, []int{1, 0}); len(drops) != 1 || drops[0].Amount != 3 {
		t.Fatalf("unknown, pick 1: drops = %+v, want amount 3", drops)
	}
}

func TestRollCountWeights(t *testing.T) {
	_, shared := loadRealData(t)
	// Aberrant spectre shape: one herb roll, 1/2/3 herbs at 11/26,
	// 11/26, 4/26. Draw order: main-roll float, RollCounts int,
	// then per herb roll a pool int + a quantity int.
	table := &Table{
		ID:          "rcw-test",
		MainMaxRoll: 128,
		Entries: []Entry{{
			SharedTable: "herb", Weight: 78, OutOf: 128,
			RollCountWeights: []RollCount{{Rolls: 1, Weight: 11}, {Rolls: 2, Weight: 11}, {Rolls: 3, Weight: 4}},
		}},
	}
	r := NewRoller(shared)
	roll := func(ints []int) []Drop {
		rng := &scriptedRNG{floats: []float64{0.0}, ints: ints}
		r.RNG = rng
		return r.RollTable(table, KillContext{})
	}
	// Pick int 0 -> 1 roll; pool int 0 -> grimy guam leaf (199).
	if drops := roll([]int{0, 0, 0}); len(drops) != 1 || drops[0].ItemID != 199 {
		t.Fatalf("1 roll: drops = %+v, want [{199 1}]", drops)
	}
	if drops := roll([]int{11, 0, 0, 0, 0}); len(drops) != 2 {
		t.Fatalf("2 rolls: drops = %+v, want 2 herb drops", drops)
	}
	if drops := roll([]int{22, 0, 0, 0, 0, 0, 0}); len(drops) != 3 {
		t.Fatalf("3 rolls: drops = %+v, want 3 herb drops", drops)
	}
	// Invalid weights roll once and consume no RollCounts draw.
	badTable := &Table{
		ID:          "rcw-bad",
		MainMaxRoll: 128,
		Entries: []Entry{{
			SharedTable: "herb", Weight: 78, OutOf: 128,
			RollCountWeights: []RollCount{{Rolls: 0, Weight: 5}},
		}},
	}
	rng := &scriptedRNG{floats: []float64{0.0}, ints: []int{0, 0}}
	r.RNG = rng
	if drops := r.RollTable(badTable, KillContext{}); len(drops) != 1 {
		t.Fatalf("invalid roll counts: drops = %+v, want 1 herb drop", drops)
	}
	if rng.ii != 2 {
		t.Fatalf("invalid roll counts: ints consumed = %d, want 2 (pool + quantity only)", rng.ii)
	}
}

func TestBonusDrops(t *testing.T) {
	table := &Table{
		ID:          "bonus-test",
		MainMaxRoll: 1,
		Entries: []Entry{{
			ItemID: 21543, Name: "Calcite", Quantity: [2]int{1, 1}, Weight: 1, OutOf: 1,
			BonusDrops: []Entry{{ItemID: 21545, Name: "Pyrophosphite", Quantity: [2]int{1, 1}}},
		}},
	}
	r := NewRoller(nil)
	r.RNG = &scriptedRNG{floats: []float64{0.0}, ints: []int{0, 0}}
	drops := r.RollTable(table, KillContext{})
	if len(drops) != 2 || drops[0].ItemID != 21543 || drops[1].ItemID != 21545 {
		t.Fatalf("bonus drops = %+v, want calcite + pyrophosphite", drops)
	}
}

// --- hits / rollWeighted primitives ------------------------------------

func TestHitsFractionalWeights(t *testing.T) {
	r := NewRoller(nil)
	hit := func(e *Entry, f float64) bool {
		r.RNG = &scriptedRNG{floats: []float64{f}}
		return r.hits(e, 1)
	}
	// 2.5/400: f=0.005 -> 2.0 < 2.5 hits; f=0.01 -> 4.0 >= 2.5 misses.
	e := &Entry{Weight: 2.5, OutOf: 400}
	if !hit(e, 0.005) {
		t.Errorf("2.5/400 at f=0.005 should hit")
	}
	if hit(e, 0.01) {
		t.Errorf("2.5/400 at f=0.01 should miss")
	}
	// A non-positive weight counts as 1 (TS hits default).
	e = &Entry{Weight: 0, OutOf: 8}
	if !hit(e, 0.1) {
		t.Errorf("weight 0 (counts as 1) at f=0.1 should hit 1/8")
	}
	if hit(e, 0.2) {
		t.Errorf("weight 0 (counts as 1) at f=0.2 should miss 1/8")
	}
	// A non-integer out_of never hits.
	e = &Entry{Weight: 1, OutOf: 2.5}
	if hit(e, 0.0) {
		t.Errorf("non-integer out_of should never hit")
	}
}

func TestRollWeightedEmptySlotMiss(t *testing.T) {
	// a_corpse shape: main_max_roll 4, one entry at 1/4 — the
	// remaining 3/4 of the pool is an implicit nothing.
	table := &Table{
		ID:          "miss-test",
		MainMaxRoll: 4,
		Entries: []Entry{
			{ItemID: 6001, Name: "Only", Quantity: [2]int{1, 1}, Weight: 1, OutOf: 4},
		},
	}
	r := NewRoller(nil)
	r.RNG = &scriptedRNG{floats: []float64{0.9}, ints: []int{}}
	if drops := r.RollTable(table, KillContext{}); len(drops) != 0 {
		t.Fatalf("roll past the last slot: drops = %+v, want none", drops)
	}
	r.RNG = &scriptedRNG{floats: []float64{0.0}, ints: []int{0}}
	if drops := r.RollTable(table, KillContext{}); len(drops) != 1 || drops[0].ItemID != 6001 {
		t.Fatalf("roll inside the slot: drops = %+v, want [{6001 1}]", drops)
	}
	// Absent main_max_roll: the total is the weight sum.
	table2 := &Table{
		ID: "sum-test",
		Entries: []Entry{
			{ItemID: 6002, Name: "A", Quantity: [2]int{1, 1}, Weight: 1},
			{ItemID: 6003, Name: "B", Quantity: [2]int{1, 1}, Weight: 1},
		},
	}
	r.RNG = &scriptedRNG{floats: []float64{0.75}, ints: []int{0}}
	if drops := r.RollTable(table2, KillContext{}); len(drops) != 1 || drops[0].ItemID != 6003 {
		t.Fatalf("sum total f=0.75: drops = %+v, want [{6003 1}]", drops)
	}
}

// --- ordered path: fail closed ------------------------------------------

func TestOrderedTableFailsClosed(t *testing.T) {
	tables, shared := loadRealData(t)
	table := tables["duke_sucellus"]
	if table.RewardChain == nil {
		t.Fatalf("duke_sucellus has no reward_chain in the real file")
	}
	r, logs := seededRoller(shared, 37)
	mainIDs := map[int]bool{}
	for _, e := range table.Entries {
		if e.ItemID > 0 {
			mainIDs[e.ItemID] = true
		}
	}
	clueIDs := map[int]bool{2677: true, 2801: true, 2722: true, 12113: true}
	const kills = 2000
	clues := 0
	for k := 0; k < kills; k++ {
		drops := r.RollTable(table, KillContext{NPCName: "Duke Sucellus"})
		for _, d := range drops {
			if mainIDs[d.ItemID] {
				t.Fatalf("kill %d: main-table item %d dropped from an ordered table: %+v", k, d.ItemID, drops)
			}
			if clueIDs[d.ItemID] {
				clues++
			}
			// The 1/1 conditional tertiaries never drop unanswered.
			if d.ItemID == 28430 || d.ItemID == 28336 {
				t.Fatalf("kill %d: 1/1 tertiary %d dropped without a quest verdict", k, d.ItemID)
			}
		}
	}
	// One fail-closed log line per kill.
	if len(*logs) != kills {
		t.Errorf("ordered log lines = %d, want %d", len(*logs), kills)
	}
	for _, line := range *logs {
		if !strings.Contains(line, "duke_sucellus") || !strings.Contains(line, "main roll skipped") {
			t.Errorf("unexpected log line: %q", line)
			break
		}
	}
	// The exclusive tertiary group still resolves (~1/40 per kill).
	if clues < 10 {
		t.Errorf("tertiary-group clue scrolls = %d over %d kills, want > 10", clues, kills)
	}
}

// --- whole-file sweep ---------------------------------------------------

func TestAllRealTablesRoll(t *testing.T) {
	tables, shared := loadRealData(t)
	r, logs := seededRoller(shared, 41)
	orderedLines := 0
	for id, table := range tables {
		allowed := reachableRanges(table, shared)
		for k := 0; k < 3; k++ {
			drops := r.RollTable(table, KillContext{NPCName: "Sweep"})
			assertValidDrops(t, drops, allowed)
		}
		_ = id
	}
	for _, line := range *logs {
		if strings.Contains(line, "main roll skipped") {
			orderedLines++
		}
	}
	// 4 ordered tables x 3 kills.
	if orderedLines != 12 {
		t.Errorf("ordered fail-closed lines = %d, want 12", orderedLines)
	}
}
