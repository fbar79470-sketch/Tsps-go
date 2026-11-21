package cache

import (
	"strings"
	"sync"
)

// Gamevals: the names Jagex ships in the OSRS cache for its own ids
// (index 24), the names rsprox prints in captures. One archive per
// kind; each file is one id. This file ports Gamevals.ts.
//
// Adaptation: the TypeScript Gamevals reads through the
// CachePipeline's current store (or a pinned FileStore) and
// constructs a fresh CacheIndexDat2 per lookup. Here Gamevals is
// built over one ArchiveSource for index 24, constructed once; the
// decoded names, interfaces and tables are cached per kind exactly
// as in TypeScript, so the repeated table reads change nothing
// observable.

// GamevalKind is one gameval archive id (Gamevals.ts:77-91). Note
// the gaps: 5, 11, 12 and 13 have no kind.
type GamevalKind int

const (
	// GamevalObj (0) names items.
	GamevalObj GamevalKind = 0
	// GamevalNPC (1) names NPCs.
	GamevalNPC GamevalKind = 1
	// GamevalInv (2) names inventories.
	GamevalInv GamevalKind = 2
	// GamevalVarp (3) names varps.
	GamevalVarp GamevalKind = 3
	// GamevalVarbit (4) names varbits.
	GamevalVarbit GamevalKind = 4
	// GamevalLoc (6) names locs.
	GamevalLoc GamevalKind = 6
	// GamevalSeq (7) names animations.
	GamevalSeq GamevalKind = 7
	// GamevalSpotAnim (8) names graphics.
	GamevalSpotAnim GamevalKind = 8
	// GamevalDbRow (9) names database rows.
	GamevalDbRow GamevalKind = 9
	// GamevalDbTable (10) names DB tables with their column names.
	GamevalDbTable GamevalKind = 10
	// GamevalInterface (14) names interfaces with their
	// components, in the format used from revision 232.
	GamevalInterface GamevalKind = 14
)

// GamevalTable is a DB table's gameval record: its name and its
// column names by column id (Gamevals.ts:93-97).
type GamevalTable struct {
	Name    string
	Columns []string
}

// readGamevalString reads a NUL-terminated latin1 string starting
// at offset, returning the string and the offset past the
// terminator. It is readString (Gamevals.ts:117-121): a missing
// terminator reads to the end of the data without error.
func readGamevalString(data []byte, offset int) (string, int) {
	end := offset
	for end < len(data) && data[end] != 0 {
		end++
	}
	runes := make([]rune, 0, end-offset)
	for _, b := range data[offset:end] {
		runes = append(runes, rune(b))
	}
	return string(runes), end + 1
}

// DecodeGamevalTable decodes a DB table file: a byte, its name,
// then (non-zero byte, column name) pairs until a zero byte. It is
// decodeGamevalTable (Gamevals.ts:100-109).
func DecodeGamevalTable(data []byte) GamevalTable {
	name, offset := readGamevalString(data, 1)
	var columns []string
	for offset < len(data) {
		marker := data[offset]
		offset++
		if marker == 0 {
			break
		}
		var column string
		column, offset = readGamevalString(data, offset)
		columns = append(columns, column)
	}
	if columns == nil {
		columns = []string{}
	}
	return GamevalTable{Name: name, Columns: columns}
}

// GamevalInterfaceDef is an interface's gameval record: its name and
// its component id -> name map (Gamevals.ts:111-115). The Def
// suffix avoids the GamevalInterface kind constant; TypeScript
// spells both GamevalInterface.
type GamevalInterfaceDef struct {
	Name       string
	Components map[int]string
}

// DecodeGamevalInterface decodes an interface file: its name, then
// (u16 component, string) pairs until 0xFFFF. It is
// decodeGamevalInterface (Gamevals.ts:124-136).
func DecodeGamevalInterface(data []byte) GamevalInterfaceDef {
	name, offset := readGamevalString(data, 0)
	components := make(map[int]string)
	for offset+1 < len(data) {
		id := int(data[offset])<<8 | int(data[offset+1])
		offset += 2
		if id == 0xffff {
			break
		}
		var component string
		component, offset = readGamevalString(data, offset)
		components[id] = component
	}
	return GamevalInterfaceDef{Name: name, Components: components}
}

// Gamevals reads the gameval name tables of index 24. Methods are
// safe for concurrent use; results are cached after the first
// successful load of each kind, as in TypeScript. A failed load is
// not cached, so a later call retries, matching the TypeScript
// behavior of throwing before caching.
type Gamevals struct {
	source ArchiveSource

	mu         sync.Mutex
	interfaces map[int]GamevalInterfaceDef
	tables     map[int]GamevalTable
	names      map[GamevalKind]map[int]string
}

// NewGamevals builds a Gamevals over the index-24 source.
func NewGamevals(source ArchiveSource) *Gamevals {
	return &Gamevals{source: source, names: make(map[GamevalKind]map[int]string)}
}

// NewGamevalsFromStore builds a Gamevals over the real Store,
// loading the index-24 reference table.
func NewGamevalsFromStore(store *Store) (*Gamevals, error) {
	index, err := NewCacheIndexFromStore(GamevalIndex, store)
	if err != nil {
		return nil, err
	}
	return NewGamevals(index), nil
}

// NamesOf returns every named id of one kind (not interfaces):
// id -> name. It mirrors Gamevals.namesOf (Gamevals.ts:151-162).
// Every file present in the archive yields a name, including an
// empty file (an empty name), as the TypeScript truthiness check
// on the data object does.
func (g *Gamevals) NamesOf(kind GamevalKind) (map[int]string, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if cached, ok := g.names[kind]; ok {
		return cached, nil
	}
	archive, err := g.source.GetArchive(int(kind), nil)
	if err != nil {
		return nil, err
	}
	names := make(map[int]string)
	for _, id := range archive.FileIDs {
		data, ok := archive.GetFile(id)
		if ok {
			names[id] = string(data)
		}
	}
	g.names[kind] = names
	return names, nil
}

// AllInterfaces returns every interface by group id. It mirrors
// Gamevals.allInterfaces (Gamevals.ts:165-174).
func (g *Gamevals) AllInterfaces() (map[int]GamevalInterfaceDef, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.interfaces != nil {
		return g.interfaces, nil
	}
	archive, err := g.source.GetArchive(int(GamevalInterface), nil)
	if err != nil {
		return nil, err
	}
	interfaces := make(map[int]GamevalInterfaceDef)
	for _, group := range archive.FileIDs {
		data, ok := archive.GetFile(group)
		if ok {
			interfaces[group] = DecodeGamevalInterface(data)
		}
	}
	g.interfaces = interfaces
	return interfaces, nil
}

// AllTables returns every DB table by id, with column names. It
// mirrors Gamevals.allTables (Gamevals.ts:177-186).
func (g *Gamevals) AllTables() (map[int]GamevalTable, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.tables != nil {
		return g.tables, nil
	}
	archive, err := g.source.GetArchive(int(GamevalDbTable), nil)
	if err != nil {
		return nil, err
	}
	tables := make(map[int]GamevalTable)
	for _, id := range archive.FileIDs {
		data, ok := archive.GetFile(id)
		if ok {
			tables[id] = DecodeGamevalTable(data)
		}
	}
	g.tables = tables
	return tables, nil
}

// TableColumn returns a DB table column's id by name, or false
// when the table has no such column. It mirrors
// Gamevals.tableColumn (Gamevals.ts:189-192).
func (g *Gamevals) TableColumn(table int, column string) (int, bool, error) {
	tables, err := g.AllTables()
	if err != nil {
		return 0, false, err
	}
	t, ok := tables[table]
	if !ok {
		return 0, false, nil
	}
	for i, name := range t.Columns {
		if name == column {
			return i, true, nil
		}
	}
	return 0, false, nil
}

// ComponentName returns `interface:component` for a packed
// (group << 16) | component id, or false when the cache has no
// such component. It mirrors Gamevals.componentName
// (Gamevals.ts:195-199).
func (g *Gamevals) ComponentName(uid int) (string, bool, error) {
	interfaces, err := g.AllInterfaces()
	if err != nil {
		return "", false, err
	}
	iface, ok := interfaces[int(uint32(uid)>>16)]
	if !ok {
		return "", false, nil
	}
	component, ok := iface.Components[uid&0xffff]
	if !ok {
		return "", false, nil
	}
	return iface.Name + ":" + component, true, nil
}

// ComponentID returns the packed id of `interface:component`, or
// false when the cache has no such component. It mirrors
// Gamevals.componentId (Gamevals.ts:202-211): only the first two
// colon-separated parts are considered.
func (g *Gamevals) ComponentID(name string) (int, bool, error) {
	parts := strings.Split(name, ":")
	if len(parts) < 2 {
		return 0, false, nil
	}
	ifaceName, componentName := parts[0], parts[1]
	interfaces, err := g.AllInterfaces()
	if err != nil {
		return 0, false, err
	}
	for group, iface := range interfaces {
		if iface.Name != ifaceName {
			continue
		}
		for id, component := range iface.Components {
			if component == componentName {
				return (group << 16) | id, true, nil
			}
		}
	}
	return 0, false, nil
}
