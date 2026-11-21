package cache

import (
	"testing"
)

func TestDecodeGamevalTable(t *testing.T) {
	data := []byte{0x01, 't', 'a', 'b', 0, 1, 'c', 'o', 'l', '1', 0, 1, 'c', 'o', 'l', '2', 0, 0}
	table := DecodeGamevalTable(data)
	if table.Name != "tab" || len(table.Columns) != 2 || table.Columns[0] != "col1" || table.Columns[1] != "col2" {
		t.Errorf("table = %+v", table)
	}
	// No columns: columns is an empty slice, not nil.
	empty := DecodeGamevalTable([]byte{0x01, 't', 0, 0})
	if empty.Name != "t" || empty.Columns == nil || len(empty.Columns) != 0 {
		t.Errorf("empty table = %+v", empty)
	}
	// Unterminated name reads to the end without error.
	unterminated := DecodeGamevalTable([]byte{0x01, 'a', 'b'})
	if unterminated.Name != "ab" || len(unterminated.Columns) != 0 {
		t.Errorf("unterminated = %+v", unterminated)
	}
	// Latin1 bytes map to runes directly.
	latin := DecodeGamevalTable([]byte{0x01, 0xe9, 0, 0})
	if latin.Name != "é" {
		t.Errorf("latin1 name = %q", latin.Name)
	}
}

func TestDecodeGamevalInterface(t *testing.T) {
	data := []byte{'i', 'f', 'a', 'c', 'e', 0, 0, 1, 'c', 'o', 'm', 'p', 0, 0, 2, 'o', 't', 'h', 'e', 'r', 0, 0xff, 0xff}
	iface := DecodeGamevalInterface(data)
	if iface.Name != "iface" || len(iface.Components) != 2 || iface.Components[1] != "comp" || iface.Components[2] != "other" {
		t.Errorf("interface = %+v", iface)
	}
	empty := DecodeGamevalInterface([]byte{'x', 0, 0xff, 0xff})
	if empty.Name != "x" || len(empty.Components) != 0 {
		t.Errorf("empty interface = %+v", empty)
	}
}

func gamevalSource(t *testing.T) *fakeSource {
	t.Helper()
	objArchive := buildTestArchive(t, 0, map[int][]byte{
		0: []byte("bronze_sword"),
		5: []byte("iron_sword"),
		7: []byte{}, // empty file -> empty name
	}, nil)
	tableData := []byte{0x01, 't', 'a', 'b', 0, 1, 'i', 'd', 0, 1, 'v', 'a', 'l', 0, 0}
	tableArchive := buildTestArchive(t, 10, map[int][]byte{3: tableData}, nil)
	ifaceData := []byte{'b', 'a', 'n', 'k', 0, 0, 1, 't', 'i', 't', 'l', 'e', 0, 0, 2, 'g', 'r', 'i', 'd', 0, 0xff, 0xff}
	ifaceArchive := buildTestArchive(t, 14, map[int][]byte{100: ifaceData}, nil)
	return newFakeSource(map[int]*Archive{0: objArchive, 10: tableArchive, 14: ifaceArchive})
}

func TestGamevalsNamesOf(t *testing.T) {
	g := NewGamevals(gamevalSource(t))
	names, err := g.NamesOf(GamevalObj)
	if err != nil {
		t.Fatal(err)
	}
	if names[0] != "bronze_sword" || names[5] != "iron_sword" {
		t.Errorf("names = %v", names)
	}
	if name, ok := names[7]; !ok || name != "" {
		t.Errorf("empty file name = %q %v", name, ok)
	}
	// Second call is cached: no further GetArchive.
	source := gamevalSource(t)
	g2 := NewGamevals(source)
	if _, err := g2.NamesOf(GamevalObj); err != nil {
		t.Fatal(err)
	}
	if _, err := g2.NamesOf(GamevalObj); err != nil {
		t.Fatal(err)
	}
	if source.calls[0] != 1 {
		t.Errorf("GetArchive calls = %d, want 1 (cached)", source.calls[0])
	}
}

func TestGamevalsInterfacesAndTables(t *testing.T) {
	g := NewGamevals(gamevalSource(t))
	ifaces, err := g.AllInterfaces()
	if err != nil {
		t.Fatal(err)
	}
	iface := ifaces[100]
	if iface.Name != "bank" || iface.Components[1] != "title" || iface.Components[2] != "grid" {
		t.Errorf("interfaces = %+v", ifaces)
	}
	tables, err := g.AllTables()
	if err != nil {
		t.Fatal(err)
	}
	if tables[3].Name != "tab" || len(tables[3].Columns) != 2 {
		t.Errorf("tables = %+v", tables)
	}
	col, ok, err := g.TableColumn(3, "val")
	if err != nil || !ok || col != 1 {
		t.Errorf("TableColumn = %d %v %v", col, ok, err)
	}
	if _, ok, err := g.TableColumn(3, "nope"); err != nil || ok {
		t.Errorf("missing column = %v %v", ok, err)
	}
	if _, ok, err := g.TableColumn(99, "val"); err != nil || ok {
		t.Errorf("missing table = %v %v", ok, err)
	}
	name, ok, err := g.ComponentName((100 << 16) | 2)
	if err != nil || !ok || name != "bank:grid" {
		t.Errorf("ComponentName = %q %v %v", name, ok, err)
	}
	if _, ok, err := g.ComponentName((100 << 16) | 9); err != nil || ok {
		t.Errorf("missing component = %v %v", ok, err)
	}
	if _, ok, err := g.ComponentName((7 << 16) | 1); err != nil || ok {
		t.Errorf("missing group = %v %v", ok, err)
	}
	id, ok, err := g.ComponentID("bank:grid")
	if err != nil || !ok || id != (100<<16)|2 {
		t.Errorf("ComponentID = %d %v %v", id, ok, err)
	}
	// Only the first two colon parts are considered.
	id2, ok, err := g.ComponentID("bank:grid:extra")
	if err != nil || !ok || id2 != id {
		t.Errorf("ComponentID extra part = %d %v %v", id2, ok, err)
	}
	if _, ok, err := g.ComponentID("nocolon"); err != nil || ok {
		t.Errorf("no-colon ComponentID = %v %v", ok, err)
	}
	if _, ok, err := g.ComponentID("bank:nope"); err != nil || ok {
		t.Errorf("missing ComponentID = %v %v", ok, err)
	}
}

func TestGamevalsMissingArchiveRetries(t *testing.T) {
	source := newFakeSource(map[int]*Archive{})
	g := NewGamevals(source)
	if _, err := g.NamesOf(GamevalNPC); err == nil {
		t.Error("missing archive loaded, want error")
	}
	// Failures are not cached: adding the archive makes the
	// retry succeed.
	source.archives[1] = buildTestArchive(t, 1, map[int][]byte{0: []byte("goblin")}, nil)
	names, err := g.NamesOf(GamevalNPC)
	if err != nil {
		t.Fatal(err)
	}
	if names[0] != "goblin" {
		t.Errorf("retry names = %v", names)
	}
	if _, err := g.AllInterfaces(); err == nil {
		t.Error("missing interface archive loaded, want error")
	}
}

func TestGamevalsFromStore(t *testing.T) {
	dir := t.TempDir()
	table := buildRefTable(6, 1, 0, []refArchive{
		{idDelta: 0, crc: 1, revision: 1, fileDeltas: []int{0}},
	})
	writeStoreFixture(t, dir,
		map[int][]byte{GamevalIndex: plainContainer(table)},
		map[int]map[int][]byte{GamevalIndex: {0: plainContainer([]byte("sword"))}})
	store, err := OpenStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	g, err := NewGamevalsFromStore(store)
	if err != nil {
		t.Fatal(err)
	}
	names, err := g.NamesOf(GamevalObj)
	if err != nil {
		t.Fatal(err)
	}
	if names[0] != "sword" {
		t.Errorf("store names = %v", names)
	}
}
