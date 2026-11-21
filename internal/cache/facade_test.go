package cache

import (
	"testing"
)

func spellItem(widgetID int, name string, level int, runes ...[2]int) []byte {
	e := (&enc{}).u8(2).str(name).u8(249)
	count := 3 + len(runes)*2
	e.u8(count)
	e.u8(0).medium(596).i32(widgetID)
	e.u8(1).medium(601).str(name)
	e.u8(0).medium(604).i32(level)
	for i, r := range runes {
		e.u8(0).medium(365 + i*2).i32(r[0])
		e.u8(0).medium(366 + i*2).i32(r[1])
	}
	return e.u8(0).done()
}

func facadeConfigs(t *testing.T) *fakeSource {
	t.Helper()
	npcs := buildTestArchive(t, ConfigNPCs, map[int][]byte{
		0: npcData("Goblin", nil),
		1: npcData("Morpher", func(e *enc) { e.u8(106).u16(5).u16(65535).u8(0).u16(2) }),
		2: npcData("Morphed", nil),
		3: npcData("Cycler A", func(e *enc) { e.u8(106).u16(5).u16(65535).u8(0).u16(4) }),
		4: npcData("Cycler B", func(e *enc) { e.u8(106).u16(5).u16(65535).u8(0).u16(3) }),
	}, nil)
	locs := buildTestArchive(t, ConfigLocs, map[int][]byte{
		0: (&enc{}).u8(2).str("Tree").u8(0).done(),
		1: (&enc{}).u8(2).str("Rock").u8(0).done(),
	}, nil)
	objs := buildTestArchive(t, ConfigObjs, map[int][]byte{
		0: []byte{0},
		1: (&enc{}).u8(2).str("Bronze sword").u8(0).done(),
		2: spellItem((161<<16)|5, "Wind Strike", 1, [2]int{556, 1}, [2]int{558, 1}),
		3: spellItem((161<<16)|6, "Water Bolt", 7, [2]int{556, 2}),
		5: (&enc{}).u8(2).str("Logs").u8(0).done(),
		6: (&enc{}).u8(2).str("null").u8(0).done(),
	}, nil)
	seqs := buildTestArchive(t, ConfigSeqs, map[int][]byte{
		0: (&enc{}).u8(1).u16(1).u16(2).u16(10).u16(0).u8(0).done(),
		1: (&enc{}).u8(0).done(),
	}, nil)
	varbits := buildTestArchive(t, ConfigVarbits, map[int][]byte{
		0: (&enc{}).u8(1).u16(100).u8(0).u8(1).u8(0).done(),
		2: (&enc{}).u8(1).u16(101).u8(2).u8(3).u8(0).done(),
	}, nil)
	spotanims := buildTestArchive(t, ConfigSpotAnims, map[int][]byte{
		0: (&enc{}).u8(1).u16(500).u8(0).done(),
	}, nil)
	idks := buildTestArchive(t, ConfigIdentkits, map[int][]byte{
		0: (&enc{}).u8(1).u8(0).u8(0).done(),
		1: (&enc{}).u8(1).u8(7).u8(3).u8(0).done(),
	}, nil)
	dbrows := buildTestArchive(t, ConfigDBRow, map[int][]byte{
		10: (&enc{}).u8(3).u8(1).u8(0).u8(1).smart(1).smart(1).i32(42).u8(255).u8(4).varInt2(300).u8(0).done(),
		11: (&enc{}).u8(3).u8(1).u8(0).u8(1).smart(36).smart(1).str("row11").u8(255).u8(4).varInt2(300).u8(0).done(),
		12: (&enc{}).u8(4).varInt2(301).u8(0).done(),
	}, nil)
	structs := buildTestArchive(t, ConfigStruct, map[int][]byte{
		0: (&enc{}).u8(249).u8(1).u8(0).medium(10).i32(77).u8(0).done(),
		1: (&enc{}).u8(249).u8(1).u8(1).medium(11).str("s").u8(7).u8(249).u8(1).u8(0).medium(12).i32(1).u8(0).done(),
	}, nil)
	enums := buildTestArchive(t, ConfigEnums, map[int][]byte{
		0: (&enc{}).u8(1).u8(0).u8(2).u8(1).u8(5).u16(2).i32(10).str("ten").i32(20).str("twenty").u8(0).done(),
		1: (&enc{}).u8(3).str("default").u8(4).i32(99).u8(6).u16(1).i32(5).i32(50).u8(0).done(),
		2: (&enc{}).u8(7).u16(99).u16(2).u16(1).str("a").u16(2).str("b").u8(8).u16(99).u16(1).u16(3).i32(30).u8(0).done(),
	}, nil)
	return newFakeSource(map[int]*Archive{
		ConfigNPCs: npcs, ConfigLocs: locs, ConfigObjs: objs, ConfigSeqs: seqs,
		ConfigVarbits: varbits, ConfigSpotAnims: spotanims, ConfigIdentkits: idks,
		ConfigDBRow: dbrows, ConfigStruct: structs, ConfigEnums: enums,
	})
}

func facadeGamevals(t *testing.T) *fakeSource {
	t.Helper()
	names := func(entries map[int]string) *Archive {
		files := make(map[int][]byte, len(entries))
		for id, name := range entries {
			files[id] = []byte(name)
		}
		return buildTestArchive(t, 0, files, nil)
	}
	tableData := []byte{0x01, 's', 'l', 'a', 'y', 'e', 'r', '_', 't', 'a', 's', 'k', 0, 1, 'i', 'd', 0, 1, 'n', 'a', 'm', 'e', 0, 0}
	ifaceData := []byte{'s', 'p', 'e', 'l', 'l', 'b', 'o', 'o', 'k', 0, 0, 5, 'w', 'i', 'n', 'd', '_', 's', 't', 'r', 'i', 'k', 'e', 0, 0xff, 0xff}
	return newFakeSource(map[int]*Archive{
		int(GamevalObj):       names(map[int]string{1: "bronze_sword", 5: "logs"}),
		int(GamevalNPC):       names(map[int]string{0: "goblin"}),
		int(GamevalInv):       names(map[int]string{0: "inventory"}),
		int(GamevalVarp):      names(map[int]string{0: "combat_style"}),
		int(GamevalVarbit):    names(map[int]string{0: "morph_varbit"}),
		int(GamevalLoc):       names(map[int]string{0: "tree"}),
		int(GamevalSeq):       names(map[int]string{0: "walk"}),
		int(GamevalSpotAnim):  names(map[int]string{0: "poof"}),
		int(GamevalDbRow):     names(map[int]string{10: "slayer_row"}),
		int(GamevalDbTable):   buildTestArchive(t, 10, map[int][]byte{3: tableData}, nil),
		int(GamevalInterface): buildTestArchive(t, 14, map[int][]byte{161: ifaceData}, nil),
	})
}

func facadeSprites(t *testing.T) *fakeSource {
	t.Helper()
	source := newFakeSource(map[int]*Archive{5: buildTestArchive(t, 5, map[int][]byte{0: []byte("sprite")}, nil)})
	source.named = true
	source.names["logo"] = 5
	return source
}

func testFacade(t *testing.T) (*CacheDefinitions, *fakeFactory, *fakeSource) {
	t.Helper()
	configs := facadeConfigs(t)
	factory := &fakeFactory{
		sources: map[int]ArchiveSource{
			IndexConfigs: configs,
			GamevalIndex: facadeGamevals(t),
			IndexSprites: facadeSprites(t),
		},
		asked: map[int]int{},
	}
	defs, err := NewCacheDefinitions(testInfo(242), factory.factory)
	if err != nil {
		t.Fatal(err)
	}
	return defs, factory, configs
}

func TestFacadeConstructionAndGetters(t *testing.T) {
	defs, factory, configs := testFacade(t)
	if defs.CacheInfo().Revision != 242 {
		t.Errorf("CacheInfo = %+v", defs.CacheInfo())
	}
	counts := defs.GetCounts()
	if counts != (DefinitionCounts{NPCs: 5, Items: 7, Objects: 2, Seqs: 2}) {
		t.Errorf("counts = %+v", counts)
	}
	if defs.GetBaseCounts() != counts {
		t.Error("GetBaseCounts differs from GetCounts")
	}
	if got := defs.GetNpc(0); got.Name != "Goblin" {
		t.Errorf("GetNpc(0) = %q", got.Name)
	}
	if got := defs.GetItem(1); got.Name != "Bronze sword" {
		t.Errorf("GetItem(1) = %q", got.Name)
	}
	if got := defs.GetObject(0); got.Name != "Tree" {
		t.Errorf("GetObject(0) = %q", got.Name)
	}
	if got := defs.GetSeq(0); len(got.FrameIds) != 1 || got.FrameIds[0] != 10 {
		t.Errorf("GetSeq(0) = %+v", got)
	}
	if defs.GetSeqCount() != 2 || defs.GetSpotanimCount() != 1 {
		t.Errorf("seq/spot counts = %d %d", defs.GetSeqCount(), defs.GetSpotanimCount())
	}
	if got := defs.GetSpotanim(0); got.ModelId != 500 {
		t.Errorf("GetSpotanim(0) = %+v", got)
	}
	if got := defs.GetVarbit(0); !got.Defined() || got.BaseVar != 100 {
		t.Errorf("GetVarbit(0) = %+v", got)
	}
	kits := defs.GetIdentKits()
	if len(kits) != 2 || kits[1].BodyPartId != 7 || !kits[1].NonSelectable {
		t.Errorf("GetIdentKits = %+v", kits)
	}
	// Only the configs index is touched at construction, and only
	// the seven eager archives are read.
	if factory.asked[GamevalIndex] != 0 || factory.asked[IndexSprites] != 0 {
		t.Errorf("lazy indexes asked at construction: %v", factory.asked)
	}
	for _, id := range []int{ConfigDBRow, ConfigStruct, ConfigEnums} {
		if configs.calls[id] != 0 {
			t.Errorf("lazy archive %d read at construction", id)
		}
	}
}

func TestFacadeMissingArchive(t *testing.T) {
	configs := facadeConfigs(t)
	delete(configs.archives, ConfigSeqs)
	factory := &fakeFactory{sources: map[int]ArchiveSource{IndexConfigs: configs}, asked: map[int]int{}}
	if _, err := NewCacheDefinitions(testInfo(242), factory.factory); err == nil {
		t.Error("missing seq archive accepted")
	}
	if _, err := NewCacheDefinitions(testInfo(242), func(int) (ArchiveSource, error) {
		return nil, errTestFailure
	}); err == nil {
		t.Error("factory error accepted")
	}
}

func TestFacadeHasDefinition(t *testing.T) {
	defs, _, _ := testFacade(t)
	cases := []struct {
		kind    string
		id      int
		defined bool
		known   bool
	}{
		{"npc", 0, true, true}, {"npc", 4, true, true}, {"npc", 5, false, true}, {"npc", -1, false, true},
		{"loc", 1, true, true}, {"loc", 2, false, true},
		{"item", 6, true, true}, {"item", 7, false, true},
		{"varbit", 0, true, true}, {"varbit", 1, false, true}, {"varbit", 2, true, true},
		{"seq", 1, true, true}, {"seq", 2, false, true},
		{"spotanim", 0, true, true}, {"spotanim", 1, false, true},
		{"dbro", 10, true, true}, {"dbro", 99, false, true},
		{"dbrow", 10, false, false}, // TS spells the case "dbro"
		{"struct", 0, false, false}, {"enum", 0, false, false}, {"bogus", 0, false, false},
	}
	for _, tc := range cases {
		defined, known := defs.HasDefinition(tc.kind, tc.id)
		if defined != tc.defined || known != tc.known {
			t.Errorf("HasDefinition(%q,%d) = %v %v, want %v %v", tc.kind, tc.id, defined, known, tc.defined, tc.known)
		}
	}
}

func TestFacadeHasItem(t *testing.T) {
	defs, _, _ := testFacade(t)
	if defs.HasItem(0) || defs.HasItem(-1) {
		t.Error("HasItem(0/-1)")
	}
	if !defs.HasItem(1) || !defs.HasItem(5) {
		t.Error("HasItem(1/5) false")
	}
	if defs.HasItem(6) {
		t.Error("HasItem(6) with name null")
	}
	if defs.HasItem(7) || defs.HasItem(99) {
		t.Error("HasItem out of range")
	}
}

func TestFacadeResolveNpc(t *testing.T) {
	defs, _, _ := testFacade(t)
	vars := fakeVars{varbits: map[int]int{5: 0}}
	if got := defs.ResolveNpc(1, vars); got == nil || got.ID() != 2 || got.Name != "Morphed" {
		t.Errorf("ResolveNpc(1) = %v", got)
	}
	if got := defs.ResolveNpc(0, vars); got == nil || got.ID() != 0 {
		t.Errorf("ResolveNpc(0) = %v", got)
	}
	if got := defs.ResolveNpc(3, vars); got != nil {
		t.Errorf("cyclic ResolveNpc = %v, want nil", got)
	}
}

func TestFacadeDbRows(t *testing.T) {
	defs, _, configs := testFacade(t)
	row, err := defs.GetDbRow(10)
	if err != nil {
		t.Fatal(err)
	}
	if row == nil || row.TableID != 300 || row.Int(0) != 42 {
		t.Errorf("GetDbRow(10) = %+v", row)
	}
	if configs.calls[ConfigDBRow] != 1 {
		t.Errorf("db archive reads = %d, want 1", configs.calls[ConfigDBRow])
	}
	// Cached: a second lookup does not re-read the archive.
	if row2, _ := defs.GetDbRow(10); row2 != row {
		t.Error("db row not cached")
	}
	if configs.calls[ConfigDBRow] != 1 {
		t.Error("db archive re-read")
	}
	row11, err := defs.GetDbRow(11)
	if err != nil || row11.String(0) != "row11" {
		t.Errorf("GetDbRow(11) = %+v %v", row11, err)
	}
	missing, err := defs.GetDbRow(99)
	if err != nil || missing != nil {
		t.Errorf("GetDbRow(99) = %v %v", missing, err)
	}
	rows, err := defs.GetDbTableRows(300)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || rows[0].ID() != 10 || rows[1].ID() != 11 {
		t.Errorf("table 300 rows = %v", rows)
	}
	empty, err := defs.GetDbTableRows(999)
	if err != nil || len(empty) != 0 {
		t.Errorf("table 999 rows = %v %v", empty, err)
	}
	id, ok, err := defs.GetDbTableId("slayer_task")
	if err != nil || !ok || id != 3 {
		t.Errorf("GetDbTableId = %d %v %v", id, ok, err)
	}
	if _, ok, err := defs.GetDbTableId("nope"); err != nil || ok {
		t.Errorf("missing table id = %v %v", ok, err)
	}
	col, ok, err := defs.GetDbColumn(3, "name")
	if err != nil || !ok || col != 1 {
		t.Errorf("GetDbColumn = %d %v %v", col, ok, err)
	}
	cols, err := defs.GetDbColumns(3)
	if err != nil || len(cols) != 2 || cols[0] != "id" || cols[1] != "name" {
		t.Errorf("GetDbColumns = %v %v", cols, err)
	}
	cols, err = defs.GetDbColumns(99)
	if err != nil || len(cols) != 0 {
		t.Errorf("missing table columns = %v %v", cols, err)
	}
}

func TestFacadeNameLookups(t *testing.T) {
	defs, factory, _ := testFacade(t)
	lookups := []struct {
		fn   func(string) (int, bool, error)
		name string
		want int
	}{
		{defs.GetObjId, "bronze_sword", 1},
		{defs.GetNpcId, "goblin", 0},
		{defs.GetLocId, "tree", 0},
		{defs.GetSeqId, "walk", 0},
		{defs.GetSpotanimId, "poof", 0},
		{defs.GetVarbitId, "morph_varbit", 0},
		{defs.GetVarpId, "combat_style", 0},
		{defs.GetInventoryId, "inventory", 0},
		{defs.GetDbRowId, "slayer_row", 10},
	}
	for _, tc := range lookups {
		id, ok, err := tc.fn(tc.name)
		if err != nil || !ok || id != tc.want {
			t.Errorf("lookup(%q) = %d %v %v, want %d", tc.name, id, ok, err, tc.want)
		}
		if _, ok, err := tc.fn("no_such_name"); err != nil || ok {
			t.Errorf("lookup(missing) = %v %v", ok, err)
		}
	}
	if factory.asked[GamevalIndex] == 0 {
		t.Error("gamevals index never requested")
	}
	name, ok, err := defs.GetSeqName(0)
	if err != nil || !ok || name != "walk" {
		t.Errorf("GetSeqName = %q %v %v", name, ok, err)
	}
	if _, ok, err := defs.GetSeqName(99); err != nil || ok {
		t.Errorf("GetSeqName(99) = %v %v", ok, err)
	}
	name, ok, err = defs.GetDbRowName(10)
	if err != nil || !ok || name != "slayer_row" {
		t.Errorf("GetDbRowName = %q %v %v", name, ok, err)
	}
	names, err := defs.GetObjNames()
	if err != nil || names[1] != "bronze_sword" || names[5] != "logs" {
		t.Errorf("GetObjNames = %v %v", names, err)
	}
	names, err = defs.GetNpcNames()
	if err != nil || names[0] != "goblin" {
		t.Errorf("GetNpcNames = %v %v", names, err)
	}
	names, err = defs.GetSeqNames()
	if err != nil || names[0] != "walk" {
		t.Errorf("GetSeqNames = %v %v", names, err)
	}
	names, err = defs.GetLocNames()
	if err != nil || names[0] != "tree" {
		t.Errorf("GetLocNames = %v %v", names, err)
	}
	names, err = defs.GetSpotanimNames()
	if err != nil || names[0] != "poof" {
		t.Errorf("GetSpotanimNames = %v %v", names, err)
	}
	names, err = defs.GetVarbitNames()
	if err != nil || names[0] != "morph_varbit" {
		t.Errorf("GetVarbitNames = %v %v", names, err)
	}
	names, err = defs.GetDbRowNames()
	if err != nil || names[10] != "slayer_row" {
		t.Errorf("GetDbRowNames = %v %v", names, err)
	}
	// GetDbTableNames is namesOf(DBTABLE): the raw file bytes as a
	// string, exactly as in TypeScript (the structured form is
	// AllTables/GetDbColumns).
	names, err = defs.GetDbTableNames()
	wantRaw := string([]byte{0x01, 's', 'l', 'a', 'y', 'e', 'r', '_', 't', 'a', 's', 'k', 0, 1, 'i', 'd', 0, 1, 'n', 'a', 'm', 'e', 0, 0})
	if err != nil || names[3] != wantRaw {
		t.Errorf("GetDbTableNames = %q %v", names[3], err)
	}
	names, err = defs.GetVarpNames()
	if err != nil || names[0] != "combat_style" {
		t.Errorf("GetVarpNames = %v %v", names, err)
	}
	names, err = defs.GetInventoryNames()
	if err != nil || names[0] != "inventory" {
		t.Errorf("GetInventoryNames = %v %v", names, err)
	}
	comp, ok, err := defs.GetComponentId("spellbook:wind_strike")
	if err != nil || !ok || comp != (161<<16)|5 {
		t.Errorf("GetComponentId = %d %v %v", comp, ok, err)
	}
	if _, ok, err := defs.GetComponentId("spellbook:nope"); err != nil || ok {
		t.Errorf("missing component = %v %v", ok, err)
	}
	group, ok, err := defs.GetInterfaceId("spellbook")
	if err != nil || !ok || group != 161 {
		t.Errorf("GetInterfaceId = %d %v %v", group, ok, err)
	}
	if _, ok, err := defs.GetInterfaceId("nope"); err != nil || ok {
		t.Errorf("missing interface = %v %v", ok, err)
	}
	sprite, ok, err := defs.GetSpriteId("logo")
	if err != nil || !ok || sprite != 5 {
		t.Errorf("GetSpriteId = %d %v %v", sprite, ok, err)
	}
	if _, ok, err := defs.GetSpriteId("nope"); err != nil || ok {
		t.Errorf("missing sprite = %v %v", ok, err)
	}
	if factory.asked[IndexSprites] == 0 {
		t.Error("sprites index never requested")
	}
}

func TestFacadeSpriteNoNames(t *testing.T) {
	configs := facadeConfigs(t)
	// A minimal source without ArchiveID: GetSpriteId errors.
	plain := &plainSource{source: facadeSprites(t)}
	factory := &fakeFactory{
		sources: map[int]ArchiveSource{IndexConfigs: configs, IndexSprites: plain},
		asked:   map[int]int{},
	}
	defs, err := NewCacheDefinitions(testInfo(242), factory.factory)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := defs.GetSpriteId("logo"); err == nil {
		t.Error("nameless sprites source accepted")
	}
}

// plainSource exposes only the ArchiveSource methods, hiding
// ArchiveID, to exercise the errSpritesNoNames path.
type plainSource struct {
	source ArchiveSource
}

func (p *plainSource) GetArchive(id int, key *[4]int32) (*Archive, error) {
	return p.source.GetArchive(id, key)
}
func (p *plainSource) ArchiveCount() int    { return p.source.ArchiveCount() }
func (p *plainSource) LastArchiveID() int   { return p.source.LastArchiveID() }
func (p *plainSource) FileCount(id int) int { return p.source.FileCount(id) }

func TestFacadeStructEnum(t *testing.T) {
	defs, _, configs := testFacade(t)
	params, err := defs.GetStructParams(0)
	if err != nil {
		t.Fatal(err)
	}
	if v, ok := ParamInt(params, 10); !ok || v != 77 {
		t.Errorf("struct 0 = %v", params)
	}
	// An unknown opcode stops struct decoding: the second 249
	// block is never read.
	params, err = defs.GetStructParams(1)
	if err != nil {
		t.Fatal(err)
	}
	if s, ok := ParamString(params, 11); !ok || s != "s" {
		t.Errorf("struct 1 = %v", params)
	}
	if _, ok := ParamInt(params, 12); ok {
		t.Error("struct param after unknown opcode decoded")
	}
	// Missing struct -> empty map, cached.
	empty, err := defs.GetStructParams(99)
	if err != nil || len(empty) != 0 {
		t.Errorf("missing struct = %v %v", empty, err)
	}
	if configs.calls[ConfigStruct] != 1 {
		t.Errorf("struct archive reads = %d, want 1", configs.calls[ConfigStruct])
	}
	// Enums: string table, int table with defaults skipped, dense tables.
	values, err := defs.GetEnumValues(0)
	if err != nil {
		t.Fatal(err)
	}
	if values[10] != "ten" || values[20] != "twenty" || len(values) != 2 {
		t.Errorf("enum 0 = %v", values)
	}
	values, err = defs.GetEnumValues(1)
	if err != nil {
		t.Fatal(err)
	}
	if values[5] != int32(50) || len(values) != 1 {
		t.Errorf("enum 1 = %v (defaults must be skipped)", values)
	}
	values, err = defs.GetEnumValues(2)
	if err != nil {
		t.Fatal(err)
	}
	if values[1] != "a" || values[2] != "b" || values[3] != int32(30) || len(values) != 3 {
		t.Errorf("enum 2 = %v", values)
	}
	empty, err = defs.GetEnumValues(99)
	if err != nil || len(empty) != 0 {
		t.Errorf("missing enum = %v %v", empty, err)
	}
	if configs.calls[ConfigEnums] != 1 {
		t.Errorf("enum archive reads = %d, want 1", configs.calls[ConfigEnums])
	}
	// PreloadEnums reads the archive eagerly and ignores errors.
	defs2, _, configs2 := testFacade(t)
	defs2.PreloadEnums()
	if configs2.calls[ConfigEnums] != 1 {
		t.Error("PreloadEnums did not read the archive")
	}
	configs3 := facadeConfigs(t)
	delete(configs3.archives, ConfigEnums)
	factory := &fakeFactory{sources: map[int]ArchiveSource{IndexConfigs: configs3}, asked: map[int]int{}}
	defs3, err := NewCacheDefinitions(testInfo(242), factory.factory)
	if err != nil {
		t.Fatal(err)
	}
	defs3.PreloadEnums() // must not panic
	if _, err := defs3.GetEnumValues(0); err == nil {
		t.Error("missing enum archive loaded, want error")
	}
}

func TestDecodeStructParamsAndEnumValues(t *testing.T) {
	// Two 249 blocks accumulate.
	params, err := DecodeStructParams((&enc{}).u8(249).u8(1).u8(0).medium(1).i32(1).u8(249).u8(1).u8(0).medium(2).i32(2).u8(0).done())
	if err != nil {
		t.Fatal(err)
	}
	if len(params) != 2 {
		t.Errorf("accumulated struct = %v", params)
	}
	// Truncated data errors.
	if _, err := DecodeStructParams([]byte{249, 1}); err == nil {
		t.Error("truncated struct decoded")
	}
	if _, err := DecodeStructParams(nil); err == nil {
		t.Error("empty struct decoded")
	}
	// Unknown enum opcode stops, returning values so far.
	values, err := DecodeEnumValues((&enc{}).u8(6).u16(1).i32(1).i32(10).u8(9).u8(6).u16(1).i32(2).i32(20).u8(0).done())
	if err != nil {
		t.Fatal(err)
	}
	if len(values) != 1 || values[1] != int32(10) {
		t.Errorf("enum stop = %v", values)
	}
	if _, err := DecodeEnumValues([]byte{5, 0}); err == nil {
		t.Error("truncated enum decoded")
	}
	// Empty enum file (terminator only) is empty, not an error.
	values, err = DecodeEnumValues([]byte{0})
	if err != nil || len(values) != 0 {
		t.Errorf("empty enum = %v %v", values, err)
	}
}

func TestFacadeArchivedOverlay(t *testing.T) {
	defs, _, _ := testFacade(t)
	bundle := archivedBundle("b1",
		archivedRow(ArchivedKindNPC, "old_goblin", 50, 5, npcData("Archived Goblin", nil)))
	changes, err := defs.RegisterDefinitionArchives([]ArchivedDefinitionBundle{bundle})
	if err != nil {
		t.Fatal(err)
	}
	if len(changes.NPC) != 1 || changes.NPC[0] != 5 {
		t.Errorf("changes = %+v", changes)
	}
	if got := defs.GetNpc(5); got.Name != "Archived Goblin" {
		t.Errorf("overlay npc = %q", got.Name)
	}
	if defs.GetCounts().NPCs != 6 {
		t.Errorf("overlay counts = %+v", defs.GetCounts())
	}
	if defined, known := defs.HasDefinition("npc", 5); !defined || !known {
		t.Errorf("HasDefinition overlay = %v %v", defined, known)
	}
	if id, ok := defs.GetArchivedID("b1", ArchivedKindNPC, "old_goblin"); !ok || id != 5 {
		t.Errorf("GetArchivedID = %d %v", id, ok)
	}
	if bundles := defs.GetDefinitionArchiveBundles(); len(bundles) != 1 || bundles[0].ID != "b1" {
		t.Errorf("bundles = %v", bundles)
	}
	// Same bundles: no semantic changes.
	same, err := defs.GetDefinitionArchiveChanges([]ArchivedDefinitionBundle{bundle})
	if err != nil {
		t.Fatal(err)
	}
	if len(same.NPC) != 0 || len(same.Obj) != 0 {
		t.Errorf("same-bundle changes = %+v", same)
	}
	// Different payload for the same id: changed.
	changed := archivedBundle("b1",
		archivedRow(ArchivedKindNPC, "old_goblin", 50, 5, npcData("Archived Goblin 2", nil)))
	diff, err := defs.GetDefinitionArchiveChanges([]ArchivedDefinitionBundle{changed})
	if err != nil {
		t.Fatal(err)
	}
	if len(diff.NPC) != 1 || diff.NPC[0] != 5 {
		t.Errorf("changed-bundle diff = %+v", diff)
	}
	// Removing everything: the published id counts as changed.
	removed, err := defs.GetDefinitionArchiveChanges(nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(removed.NPC) != 1 {
		t.Errorf("removal diff = %+v", removed)
	}
	// An invalid registration is rejected and leaves the view intact.
	bad := archivedBundle("b2", archivedRow(ArchivedKindNPC, "bad", 51, 6, npcData("Bad", nil)))
	bad.Definitions[0].CRC++
	if _, err := defs.RegisterDefinitionArchives([]ArchivedDefinitionBundle{bad}); err == nil {
		t.Error("invalid registration accepted")
	}
	if got := defs.GetNpc(5); got.Name != "Archived Goblin" {
		t.Errorf("view after failed registration = %q", got.Name)
	}
}

func TestFacadeArchivedObjInvalidatesSpells(t *testing.T) {
	defs, _, _ := testFacade(t)
	if names := defs.GetSpellNames(); len(names) != 2 {
		t.Fatalf("initial spells = %v", names)
	}
	spell := spellItem((161<<16)|7, "Fire Blast", 59, [2]int{556, 4})
	bundle := archivedBundle("b1", archivedRow(ArchivedKindObj, "fire_blast", 70, 7, spell))
	if _, err := defs.RegisterDefinitionArchives([]ArchivedDefinitionBundle{bundle}); err != nil {
		t.Fatal(err)
	}
	names := defs.GetSpellNames()
	if len(names) != 3 || names[2] != "fire blast" {
		t.Errorf("spells after obj overlay = %v", names)
	}
	ref, ok := defs.GetSpellByName("Fire Blast")
	if !ok || ref.ItemID != 7 {
		t.Errorf("overlay spell ref = %+v %v", ref, ok)
	}
}

func TestFacadeSpells(t *testing.T) {
	defs, _, _ := testFacade(t)
	// Item param preferred over the widget scan.
	name, ok := defs.GetSpellName((161<<16)|5, 2)
	if !ok || name != "Wind Strike" {
		t.Errorf("GetSpellName(item) = %q %v", name, ok)
	}
	// Widget fallback when the item id is out of range.
	name, ok = defs.GetSpellName((161<<16)|6, 0)
	if !ok || name != "Water Bolt" {
		t.Errorf("GetSpellName(widget) = %q %v", name, ok)
	}
	if _, ok := defs.GetSpellName((161<<16)|9, 0); ok {
		t.Error("unknown widget spell found")
	}
	if !defs.IsSpellWidgetGroup(161) || defs.IsSpellWidgetGroup(999) {
		t.Error("IsSpellWidgetGroup")
	}
	ref, ok := defs.GetSpellByName("  WIND strike ")
	if !ok || ref.WidgetID != (161<<16)|5 || ref.ItemID != 2 {
		t.Errorf("GetSpellByName = %+v %v", ref, ok)
	}
	if _, ok := defs.GetSpellByName("Nope"); ok {
		t.Error("unknown spell found")
	}
	names := defs.GetSpellNames()
	if len(names) != 2 || names[0] != "wind strike" || names[1] != "water bolt" {
		t.Errorf("GetSpellNames = %v", names)
	}
	reqs, ok := defs.GetSpellRequirements("wind strike")
	if !ok || reqs.Level != 1 || len(reqs.Runes) != 2 || reqs.Runes[0] != (SpellRune{ID: 556, Amount: 1}) || reqs.Runes[1] != (SpellRune{ID: 558, Amount: 1}) {
		t.Errorf("requirements = %+v %v", reqs, ok)
	}
	reqs, ok = defs.GetSpellRequirements("water bolt")
	if !ok || reqs.Level != 7 || len(reqs.Runes) != 1 || reqs.Runes[0].Amount != 2 {
		t.Errorf("water bolt requirements = %+v %v", reqs, ok)
	}
	if _, ok := defs.GetSpellRequirements("nope"); ok {
		t.Error("unknown spell requirements found")
	}
}

func TestFacadeFromStore(t *testing.T) {
	dir := t.TempDir()
	// Configs index: one single-file archive per config kind.
	configTable := buildRefTable(6, 1, 0, []refArchive{
		{idDelta: 3, crc: 1, revision: 1, fileDeltas: []int{0}},
		{idDelta: 3, crc: 1, revision: 1, fileDeltas: []int{0}},
		{idDelta: 2, crc: 1, revision: 1, fileDeltas: []int{0}},
		{idDelta: 1, crc: 1, revision: 1, fileDeltas: []int{0}},
		{idDelta: 1, crc: 1, revision: 1, fileDeltas: []int{0}},
		{idDelta: 2, crc: 1, revision: 1, fileDeltas: []int{0}},
		{idDelta: 1, crc: 1, revision: 1, fileDeltas: []int{0}},
		{idDelta: 1, crc: 1, revision: 1, fileDeltas: []int{0}},
		{idDelta: 20, crc: 1, revision: 1, fileDeltas: []int{0}},
		{idDelta: 4, crc: 1, revision: 1, fileDeltas: []int{0}},
	})
	configArchives := map[int][]byte{
		ConfigIdentkits: {0},
		ConfigLocs:      (&enc{}).u8(2).str("Tree").u8(0).done(),
		ConfigEnums:     {0},
		ConfigNPCs:      npcData("Goblin", nil),
		ConfigObjs:      (&enc{}).u8(2).str("Sword").u8(0).done(),
		ConfigSeqs:      {0},
		ConfigSpotAnims: {0},
		ConfigVarbits:   {0},
		ConfigStruct:    {0},
		ConfigDBRow:     {0},
	}
	configContainers := make(map[int][]byte, len(configArchives))
	for id, data := range configArchives {
		configContainers[id] = plainContainer(data)
	}
	gamevalTable := buildRefTable(6, 1, 0, []refArchive{
		{idDelta: 0, crc: 1, revision: 1, fileDeltas: []int{0}},
	})
	spriteTable := buildRefTable(6, 1, 0x1, []refArchive{
		{idDelta: 5, nameHash: HashDjb2("logo"), crc: 1, revision: 1, fileDeltas: []int{0}, fileHashes: []int32{1}},
	})
	writeStoreFixture(t, dir,
		map[int][]byte{
			IndexConfigs: plainContainer(configTable),
			GamevalIndex: plainContainer(gamevalTable),
			IndexSprites: plainContainer(spriteTable),
		},
		map[int]map[int][]byte{
			IndexConfigs: configContainers,
			GamevalIndex: {0: plainContainer([]byte("sword"))},
			IndexSprites: {5: plainContainer([]byte("sprite"))},
		})
	store, err := OpenStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	defs, err := NewCacheDefinitionsFromStore(testInfo(242), store)
	if err != nil {
		t.Fatal(err)
	}
	if got := defs.GetNpc(0); got.Name != "Goblin" {
		t.Errorf("store GetNpc = %q", got.Name)
	}
	if got := defs.GetObject(0); got.Name != "Tree" {
		t.Errorf("store GetObject = %q", got.Name)
	}
	id, ok, err := defs.GetObjId("sword")
	if err != nil || !ok || id != 0 {
		t.Errorf("store GetObjId = %d %v %v", id, ok, err)
	}
	sprite, ok, err := defs.GetSpriteId("logo")
	if err != nil || !ok || sprite != 5 {
		t.Errorf("store GetSpriteId = %d %v %v", sprite, ok, err)
	}
}
