package cache

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
)

// CacheDefinitions is the definition facade: the decoded NPC, item,
// object, animation, graphic, varbit and identity-kit loaders over
// the configs index, the archived-definition overlay, the database
// rows, the struct/enum tables, the gameval name maps, and the
// spell metadata derived from item params. It ports
// CacheDefinitions.ts.
//
// Adaptation: TypeScript CacheDefinitions is a static class whose
// getState() builds the loaders lazily on first use from the
// CachePipeline globals. Here it is a struct built once by
// NewCacheDefinitions (or NewCacheDefinitionsFromStore for the
// real Store), which loads the config archives eagerly — the
// equivalent of the first getState() call — while the database,
// struct, enum, gamevals and sprites archives stay lazy, as in
// TypeScript. Methods are safe for concurrent use.
//
// Two TypeScript subsystems are deliberately NOT ported here, and
// their absence is visible rather than stubbed:
//   - Custom definition overlays and custom items
//     (registerCustomDefinitions, registerCustomItems, and the
//     applyCustomDefinition path inside getNpc/getItem/getObject):
//     they depend on the JSON definition pipelines and on
//     encodeContentData from ClientProtocol, which live outside
//     game/cache. With no custom content registered, the
//     TypeScript getters fall through to the loaders, which is
//     exactly what GetNpc/GetItem/GetObject do here.
//   - Enum and struct decoders as separate codec types: none
//     exist in the codec tree. The inline decoders from
//     CacheDefinitions.ts are ported as DecodeEnumValues and
//     DecodeStructParams.
//
// Error handling: TypeScript throws on corrupt gamevals, struct,
// enum, sprite and database archives; the Go methods that read
// those archives return the error instead. Loaders never fail
// (see TypeLoader), so GetNpc/GetItem/GetObject/GetSeq and friends
// keep their total signatures.

// structParamsOpcode is STRUCT_PARAMS_OPCODE
// (CacheDefinitions.ts:32): the only struct opcode.
const structParamsOpcode = 249

// DecodeStructParams decodes a struct file's params. It is
// CacheDefinitions.decodeStructParams (CacheDefinitions.ts:462-470):
// opcode 249 blocks accumulate into one map; any other opcode
// stops decoding. Truncated data is an error, as the TypeScript
// ByteBuffer throws.
func DecodeStructParams(data []byte) (ParamsMap, error) {
	params := make(ParamsMap)
	buf := NewByteBuffer(data)
	for {
		opcode, err := buf.ReadUnsignedByte()
		if err != nil {
			return params, err
		}
		if opcode == 0 {
			break
		}
		if opcode != structParamsOpcode {
			break // the only struct opcode
		}
		params, err = ReadParamsMap(buf, params)
		if err != nil {
			return params, err
		}
	}
	return params, nil
}

// DecodeEnumValues decodes an enum file's key -> value entries. It
// is CacheDefinitions.decodeEnumValues
// (CacheDefinitions.ts:492-513). The default value (opcodes 3/4)
// is read past and not included. Truncated data is an error.
func DecodeEnumValues(data []byte) (ParamsMap, error) {
	values := make(ParamsMap)
	buf := NewByteBuffer(data)
	for {
		opcode, err := buf.ReadUnsignedByte()
		if err != nil {
			return values, err
		}
		if opcode == 0 {
			break
		}
		switch {
		case opcode == 1 || opcode == 2:
			if _, err := buf.ReadUnsignedByte(); err != nil { // key / value type
				return values, err
			}
		case opcode == 3:
			if _, err := buf.ReadString(); err != nil { // default string
				return values, err
			}
		case opcode == 4:
			if _, err := buf.ReadInt(); err != nil { // default int
				return values, err
			}
		case opcode == 5 || opcode == 6:
			size, err := buf.ReadUnsignedShort()
			if err != nil {
				return values, err
			}
			for i := 0; i < size; i++ {
				key, err := buf.ReadInt()
				if err != nil {
					return values, err
				}
				if opcode == 5 {
					s, err := buf.ReadString()
					if err != nil {
						return values, err
					}
					values[int(key)] = s
				} else {
					v, err := buf.ReadInt()
					if err != nil {
						return values, err
					}
					values[int(key)] = v
				}
			}
		case opcode == 7 || opcode == 8:
			if _, err := buf.ReadUnsignedShort(); err != nil { // the dense table's size
				return values, err
			}
			size, err := buf.ReadUnsignedShort()
			if err != nil {
				return values, err
			}
			for i := 0; i < size; i++ {
				key, err := buf.ReadUnsignedShort()
				if err != nil {
					return values, err
				}
				if opcode == 7 {
					s, err := buf.ReadString()
					if err != nil {
						return values, err
					}
					values[key] = s
				} else {
					v, err := buf.ReadInt()
					if err != nil {
						return values, err
					}
					values[key] = v
				}
			}
		default:
			return values, nil
		}
	}
	return values, nil
}

// DefinitionCounts are the definition counts GetCounts and
// GetBaseCounts return: npcs, items, objects and seqs in
// TypeScript's field order.
type DefinitionCounts struct {
	NPCs    int
	Items   int
	Objects int
	Seqs    int
}

// CacheDefinitions is the facade described in the package-level
// comment above. The zero value is unusable; build one with
// NewCacheDefinitions or NewCacheDefinitionsFromStore.
type CacheDefinitions struct {
	info    CacheInfo
	factory IndexFactory
	configs ArchiveSource

	npcs        *ArchivedTypeLoader[*NpcType]
	varbits     *ArchiveTypeLoader[*VarBitType]
	identKits   *ArchiveTypeLoader[*IdkType]
	items       *PostProcessedObjTypeLoader
	objects     *ArchivedTypeLoader[*LocType]
	sequences   *ArchivedTypeLoader[*SeqType]
	spotAnims   *ArchiveTypeLoader[*SpotAnimType]
	objArchives *ArchivedTypeLoader[*ObjType]
	archives    *ArchivedDefinitionRegistry

	archiveContext ArchivedDefinitionContext

	// mu guards the lazy caches below. Loaders and the registry
	// have their own locks; helpers suffixed Locked run with mu
	// held and must not call public methods that take mu.
	mu              sync.Mutex
	gamevals        *Gamevals
	idsByName       map[GamevalKind]map[string]int
	dbArchive       *Archive
	dbRowIDsByTable map[int][]int
	dbRows          map[int]*DbRowType
	dbTables        map[int][]*DbRowType
	structParams    map[int]ParamsMap
	enumValues      map[int]ParamsMap
	structArchive   *Archive
	enumArchive     *Archive
	sprites         namedArchiveSource

	spellNamesByWidget map[int]string
	spellsByName       map[string]SpellRef
	spellNamesOrder    []string
	spellWidgetGroups  map[int]bool
	spellsReady        bool
}

// namedArchiveSource is an ArchiveSource that can also resolve
// archive names, for sprite lookups. *CacheIndex satisfies it.
type namedArchiveSource interface {
	ArchiveSource
	ArchiveID(name string) int
}

// NewCacheDefinitions builds the facade over the configs index
// supplied by factory, mirroring getState()
// (CacheDefinitions.ts:104-160): the native archive loaders, the
// archived registry over their counts, the archived wrappers, and
// the post-processed item loader. A missing config archive is an
// error, as getArchive throwing is in TypeScript.
func NewCacheDefinitions(info CacheInfo, factory IndexFactory) (*CacheDefinitions, error) {
	configs, err := factory(IndexConfigs)
	if err != nil {
		return nil, err
	}
	loadArchive := func(archiveID int, what string) (*Archive, error) {
		archive, err := configs.GetArchive(archiveID, nil)
		if err != nil {
			return nil, err
		}
		if archive == nil {
			return nil, errMissingConfigArchive(what, archiveID)
		}
		return archive, nil
	}
	npcArchive, err := loadArchive(ConfigNPCs, "npcs")
	if err != nil {
		return nil, err
	}
	locArchive, err := loadArchive(ConfigLocs, "locs")
	if err != nil {
		return nil, err
	}
	objArchive, err := loadArchive(ConfigObjs, "objs")
	if err != nil {
		return nil, err
	}
	seqArchive, err := loadArchive(ConfigSeqs, "seqs")
	if err != nil {
		return nil, err
	}
	varbitArchive, err := loadArchive(ConfigVarbits, "varbits")
	if err != nil {
		return nil, err
	}
	spotAnimArchive, err := loadArchive(ConfigSpotAnims, "spotAnims")
	if err != nil {
		return nil, err
	}
	idkArchive, err := loadArchive(ConfigIdentkits, "identkits")
	if err != nil {
		return nil, err
	}

	nativeNPC := NewArchiveNpcLoader(info, npcArchive)
	nativeLoc := NewArchiveLocLoader(info, locArchive)
	nativeObj := NewArchiveObjLoader(info, objArchive)
	// Only archived animations are decoded here (validation);
	// playback is client-side.
	nativeSeq := NewArchiveTypeLoader(NewSeqType, info, seqArchive)

	archiveContext := ArchivedDefinitionContext{
		CacheInfo: info,
		Counts: ArchivedNativeCounts{
			NPC:          nativeNPC.Count(),
			Loc:          nativeLoc.Count(),
			Obj:          nativeObj.Count(),
			Seq:          nativeSeq.Count(),
			SeqSupported: true,
		},
		CurrentDefinition: func(kind ArchivedDefinitionKind, id int) (string, []byte, bool) {
			switch kind {
			case ArchivedKindNPC:
				data, ok := npcArchive.GetFile(id)
				return nativeNPC.Load(id).Name, data, ok
			case ArchivedKindLoc:
				data, ok := locArchive.GetFile(id)
				return nativeLoc.Load(id).Name, data, ok
			case ArchivedKindObj:
				data, ok := objArchive.GetFile(id)
				return nativeObj.Load(id).Name, data, ok
			case ArchivedKindSeq:
				// Sequences have no name; only retired items
				// are ever replaced in place.
				data, ok := seqArchive.GetFile(id)
				return "", data, ok
			default:
				return "", nil, false
			}
		},
	}
	archives, err := NewArchivedDefinitionRegistry(archiveContext)
	if err != nil {
		return nil, err
	}
	objArchives := NewArchivedTypeLoader(nativeObj, NewObjType,
		func(id int) (ArchivedTypeSource, bool) { return archives.GetSource(ArchivedKindObj, id) },
		func() int { return archives.GetCount(ArchivedKindObj) })
	return &CacheDefinitions{
		info:    info,
		factory: factory,
		configs: configs,
		varbits: NewArchiveVarBitLoader(info, varbitArchive),
		sequences: NewArchivedTypeLoader(nativeSeq, NewSeqType,
			func(id int) (ArchivedTypeSource, bool) { return archives.GetSource(ArchivedKindSeq, id) },
			func() int { return archives.GetCount(ArchivedKindSeq) }),
		spotAnims: NewArchiveSpotAnimLoader(info, spotAnimArchive),
		identKits: NewArchiveIdkLoader(info, idkArchive),
		npcs: NewArchivedTypeLoader(nativeNPC, NewNpcType,
			func(id int) (ArchivedTypeSource, bool) { return archives.GetSource(ArchivedKindNPC, id) },
			func() int { return archives.GetCount(ArchivedKindNPC) }),
		items: NewPostProcessedObjTypeLoader(objArchives),
		objects: NewArchivedTypeLoader(nativeLoc, NewLocType,
			func(id int) (ArchivedTypeSource, bool) { return archives.GetSource(ArchivedKindLoc, id) },
			func() int { return archives.GetCount(ArchivedKindLoc) }),
		objArchives:    objArchives,
		archives:       archives,
		archiveContext: archiveContext,
		idsByName:      make(map[GamevalKind]map[string]int),
		dbRows:         make(map[int]*DbRowType),
		dbTables:       make(map[int][]*DbRowType),
		structParams:   make(map[int]ParamsMap),
		enumValues:     make(map[int]ParamsMap),
	}, nil
}

func errMissingConfigArchive(what string, archiveID int) error {
	return &configArchiveError{what: what, archiveID: archiveID}
}

type configArchiveError struct {
	what      string
	archiveID int
}

func (e *configArchiveError) Error() string {
	return fmt.Sprintf("missing config archive %s (%d)", e.what, e.archiveID)
}

// errSpritesNoNames is returned when the sprites index source
// cannot resolve archive names. *CacheIndex always can; only a
// minimal fake source triggers this.
var errSpritesNoNames = errors.New("sprites index source does not support archive name lookup")

// NewCacheDefinitionsFromStore builds the facade over the real
// Store, loading each index's reference table on demand through
// the store. It is the production constructor: the equivalent of
// getState() reading CachePipeline.getStore().
func NewCacheDefinitionsFromStore(info CacheInfo, store *Store) (*CacheDefinitions, error) {
	factory := func(indexID int) (ArchiveSource, error) {
		return NewCacheIndexFromStore(indexID, store)
	}
	return NewCacheDefinitions(info, factory)
}

// CacheInfo returns the cache the facade's definitions were
// decoded from.
func (d *CacheDefinitions) CacheInfo() CacheInfo { return d.info }

// GetNpc returns the NPC definition for id, archived overlay
// included. (The custom-definition overlay path in TypeScript is
// not ported; see the type comment.)
func (d *CacheDefinitions) GetNpc(id int) *NpcType { return d.npcs.Load(id) }

// GetItem returns the post-processed item definition for id,
// archived overlay included.
func (d *CacheDefinitions) GetItem(id int) *ObjType { return d.items.Load(id) }

// GetObject returns the object definition for id, archived overlay
// included.
func (d *CacheDefinitions) GetObject(id int) *LocType { return d.objects.Load(id) }

// GetSeq returns the animation definition for id, archived overlay
// included.
func (d *CacheDefinitions) GetSeq(id int) *SeqType { return d.sequences.Load(id) }

// GetSeqCount returns the animation count, archived overlay
// included.
func (d *CacheDefinitions) GetSeqCount() int { return d.sequences.Count() }

// GetSpotanim returns the graphic definition for id.
func (d *CacheDefinitions) GetSpotanim(id int) *SpotAnimType { return d.spotAnims.Load(id) }

// GetSpotanimCount returns the graphic count.
func (d *CacheDefinitions) GetSpotanimCount() int { return d.spotAnims.Count() }

// GetVarbit returns the varbit definition for id.
func (d *CacheDefinitions) GetVarbit(id int) *VarBitType { return d.varbits.Load(id) }

// GetIdentKits returns every identity kit, in id order.
func (d *CacheDefinitions) GetIdentKits() []*IdkType {
	loader := d.identKits
	kits := make([]*IdkType, loader.Count())
	for id := range kits {
		kits[id] = loader.Load(id)
	}
	return kits
}

// GetBaseCounts returns the native-plus-archived counts. The seq
// count is the registry count: native sequences plus appended
// archived ones.
func (d *CacheDefinitions) GetBaseCounts() DefinitionCounts {
	return DefinitionCounts{
		NPCs:    d.npcs.Count(),
		Items:   d.items.Count(),
		Objects: d.objects.Count(),
		Seqs:    d.archives.GetCount(ArchivedKindSeq),
	}
}

// GetCounts returns GetBaseCounts extended by custom definitions
// in TypeScript. This port has no custom definitions, so it
// returns GetBaseCounts.
func (d *CacheDefinitions) GetCounts() DefinitionCounts { return d.GetBaseCounts() }

// HasDefinition reports whether a definition id exists, by asking
// the archive or the registry rather than by decoding: a loader
// answers an unknown id with a default object, which is
// indistinguishable from a real definition that happens to hold
// default values. It mirrors CacheDefinitions.hasDefinition. The
// second result is false ("unknown") for kinds whose absence
// cannot be proven from an empty result — structs, enums, db
// tables, varps, inventories, sprites, sounds and params — which
// TypeScript answers with undefined. Note the kind spelling
// "dbro": TypeScript spells the case "dbro"
// (CacheDefinitions.ts), and this matches it exactly.
func (d *CacheDefinitions) HasDefinition(kind string, id int) (defined, known bool) {
	if id < 0 {
		return false, true
	}
	counts := d.archiveContext.Counts
	contains := func(ids []int, id int) bool {
		for _, v := range ids {
			if v == id {
				return true
			}
		}
		return false
	}
	switch kind {
	case "loc":
		return id < counts.Loc || contains(d.archives.IDs(ArchivedKindLoc), id), true
	case "npc":
		return id < counts.NPC || contains(d.archives.IDs(ArchivedKindNPC), id), true
	case "item":
		return id < counts.Obj || contains(d.archives.IDs(ArchivedKindObj), id), true
	case "varbit":
		_, ok := d.varbits.Archive().GetFile(id)
		return ok, true
	case "seq":
		if contains(d.archives.IDs(ArchivedKindSeq), id) {
			return true, true
		}
		_, _, ok := d.archiveContext.CurrentDefinition(ArchivedKindSeq, id)
		return ok, true
	case "spotanim":
		_, ok := d.spotAnims.Archive().GetFile(id)
		return ok, true
	case "dbro":
		row, err := d.GetDbRow(id)
		return err == nil && row != nil, true
	default:
		// Structs, enums, db tables, varps, inventories, sprites,
		// sounds and params cannot be proven absent from an empty
		// result, so they answer "unknown" rather than a guess.
		return false, false
	}
}

// HasItem reports whether id is a real item: in range, with a
// non-empty name other than "null".
func (d *CacheDefinitions) HasItem(id int) bool {
	if id <= 0 {
		return false
	}
	if id >= d.GetCounts().Items {
		return false
	}
	item := d.GetItem(id)
	return item != nil && item.Name != "" && item.Name != "null"
}

// npcLoaderAdapter adapts the facade to NpcTypeLoader for
// ResolveNpc, mirroring the inline loader object in
// CacheDefinitions.resolveNpc.
type npcLoaderAdapter struct{ d *CacheDefinitions }

func (a npcLoaderAdapter) Load(id int) *NpcType { return a.d.GetNpc(id) }
func (a npcLoaderAdapter) Count() int           { return a.d.GetCounts().NPCs }
func (a npcLoaderAdapter) ClearCache()          {}

// ResolveNpc follows morph transforms to the NPC visible for the
// current vars, or nil on a transform cycle or a -1 target. It
// mirrors CacheDefinitions.resolveNpc.
func (d *CacheDefinitions) ResolveNpc(id int, vars VarReader) *NpcType {
	npc := d.GetNpc(id)
	seen := make(map[int]bool)
	loader := npcLoaderAdapter{d}
	for npc != nil && len(npc.Transforms) > 0 {
		if seen[npc.ID()] {
			return nil
		}
		seen[npc.ID()] = true
		npc = npc.Transform(vars, loader)
	}
	return npc
}

// dbArchiveLocked returns the dbRow archive, loading it once. It
// mirrors getDbArchive (CacheDefinitions.ts:230-236), which reads
// the configs index's dbRow archive (ConfigType.OSRS.dbRow).
func (d *CacheDefinitions) dbArchiveLocked() (*Archive, error) {
	if d.dbArchive != nil {
		return d.dbArchive, nil
	}
	archive, err := d.configs.GetArchive(ConfigDBRow, nil)
	if err != nil {
		return nil, err
	}
	d.dbArchive = archive
	return archive, nil
}

// dbRowIDsByTableLocked indexes table membership only; unused rows
// retain no decoded column data. It mirrors getDbRowIdsByTable
// (CacheDefinitions.ts:239-253).
func (d *CacheDefinitions) dbRowIDsByTableLocked() (map[int][]int, error) {
	if d.dbRowIDsByTable != nil {
		return d.dbRowIDsByTable, nil
	}
	archive, err := d.dbArchiveLocked()
	if err != nil {
		return nil, err
	}
	byTable := make(map[int][]int)
	for _, id := range archive.FileIDs {
		data, ok := archive.GetFile(id)
		if !ok {
			continue
		}
		tableID, err := ReadDbRowTableID(data)
		if err != nil {
			return nil, err
		}
		byTable[tableID] = append(byTable[tableID], id)
	}
	d.dbRowIDsByTable = byTable
	return byTable, nil
}

// dbRowLocked decodes and caches one row. Callers hold d.mu.
func (d *CacheDefinitions) dbRowLocked(id int) (*DbRowType, error) {
	if cached, ok := d.dbRows[id]; ok {
		return cached, nil
	}
	archive, err := d.dbArchiveLocked()
	if err != nil {
		return nil, err
	}
	data, ok := archive.GetFile(id)
	if !ok {
		return nil, nil
	}
	row, err := DecodeDbRow(id, data)
	if err != nil {
		return nil, err
	}
	d.dbRows[id] = row
	return row, nil
}

// GetDbRow returns a cache database row by id, decoded and
// retained only when requested, or (nil, nil) when the archive
// has no such file.
func (d *CacheDefinitions) GetDbRow(id int) (*DbRowType, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.dbRowLocked(id)
}

// GetDbTableRows returns every row of a table, in id order,
// sharing the individual row cache.
func (d *CacheDefinitions) GetDbTableRows(tableID int) ([]*DbRowType, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if cached, ok := d.dbTables[tableID]; ok {
		return cached, nil
	}
	byTable, err := d.dbRowIDsByTableLocked()
	if err != nil {
		return nil, err
	}
	ids, ok := byTable[tableID]
	if !ok {
		return []*DbRowType{}, nil
	}
	rows := make([]*DbRowType, 0, len(ids))
	for _, id := range ids {
		row, err := d.dbRowLocked(id)
		if err != nil {
			return nil, err
		}
		rows = append(rows, row)
	}
	d.dbTables[tableID] = rows
	return rows, nil
}

// gamevalsLocked returns the Gamevals, building its index once.
// TypeScript constructs a fresh index per lookup; the decoded
// results are cached either way, so one index is equivalent.
func (d *CacheDefinitions) gamevalsLocked() (*Gamevals, error) {
	if d.gamevals != nil {
		return d.gamevals, nil
	}
	source, err := d.factory(GamevalIndex)
	if err != nil {
		return nil, err
	}
	d.gamevals = NewGamevals(source)
	return d.gamevals, nil
}

// GetDbTableId returns a database table's id by its gameval name
// (`slayer_task`), or false.
func (d *CacheDefinitions) GetDbTableId(name string) (int, bool, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	gamevals, err := d.gamevalsLocked()
	if err != nil {
		return 0, false, err
	}
	tables, err := gamevals.AllTables()
	if err != nil {
		return 0, false, err
	}
	for id, table := range tables {
		if table.Name == name {
			return id, true, nil
		}
	}
	return 0, false, nil
}

// GetDbColumn returns a database table column's id by its gameval
// name, or false; columns can move between revisions.
func (d *CacheDefinitions) GetDbColumn(tableID int, column string) (int, bool, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	gamevals, err := d.gamevalsLocked()
	if err != nil {
		return 0, false, err
	}
	return gamevals.TableColumn(tableID, column)
}

// GetDbColumns returns a database table's column names by column
// id (empty when the cache has no names for it).
func (d *CacheDefinitions) GetDbColumns(tableID int) ([]string, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	gamevals, err := d.gamevalsLocked()
	if err != nil {
		return nil, err
	}
	tables, err := gamevals.AllTables()
	if err != nil {
		return nil, err
	}
	if table, ok := tables[tableID]; ok {
		return table.Columns, nil
	}
	return []string{}, nil
}

// idByNameLocked returns the cached name -> id map for one kind,
// building it from the gameval names on first use. It mirrors
// CacheDefinitions.idByName.
func (d *CacheDefinitions) idByNameLocked(kind GamevalKind) (map[string]int, error) {
	if ids, ok := d.idsByName[kind]; ok {
		return ids, nil
	}
	gamevals, err := d.gamevalsLocked()
	if err != nil {
		return nil, err
	}
	names, err := gamevals.NamesOf(kind)
	if err != nil {
		return nil, err
	}
	ids := make(map[string]int, len(names))
	for id, name := range names {
		ids[name] = id
	}
	d.idsByName[kind] = ids
	return ids, nil
}

// idForName is the shared body of the Get*Id lookups.
func (d *CacheDefinitions) idForName(kind GamevalKind, name string) (int, bool, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	ids, err := d.idByNameLocked(kind)
	if err != nil {
		return 0, false, err
	}
	id, ok := ids[name]
	return id, ok, nil
}

// namesForKind is the shared body of the Get*Names lookups. The
// returned map is the cached live map and must not be mutated.
func (d *CacheDefinitions) namesForKind(kind GamevalKind) (map[int]string, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	gamevals, err := d.gamevalsLocked()
	if err != nil {
		return nil, err
	}
	return gamevals.NamesOf(kind)
}

// GetVarbitId returns a varbit's id by its gameval name, or false.
func (d *CacheDefinitions) GetVarbitId(name string) (int, bool, error) {
	return d.idForName(GamevalVarbit, name)
}

// GetVarpId returns a varp's id by its gameval name, or false.
func (d *CacheDefinitions) GetVarpId(name string) (int, bool, error) {
	return d.idForName(GamevalVarp, name)
}

// GetInventoryId returns an inventory's id by its gameval name, or
// false.
func (d *CacheDefinitions) GetInventoryId(name string) (int, bool, error) {
	return d.idForName(GamevalInv, name)
}

// GetLocId returns a loc's id by its gameval name, or false.
func (d *CacheDefinitions) GetLocId(name string) (int, bool, error) {
	return d.idForName(GamevalLoc, name)
}

// GetNpcId returns an NPC's id by its gameval name, or false.
func (d *CacheDefinitions) GetNpcId(name string) (int, bool, error) {
	return d.idForName(GamevalNPC, name)
}

// GetObjId returns an obj's id by its gameval name, or false.
func (d *CacheDefinitions) GetObjId(name string) (int, bool, error) {
	return d.idForName(GamevalObj, name)
}

// GetSeqId returns an animation's id by its gameval name, or
// false.
func (d *CacheDefinitions) GetSeqId(name string) (int, bool, error) {
	return d.idForName(GamevalSeq, name)
}

// GetDbRowId returns a database row's id by its gameval name, or
// false.
func (d *CacheDefinitions) GetDbRowId(name string) (int, bool, error) {
	return d.idForName(GamevalDbRow, name)
}

// GetSpotanimId returns a graphic's id by its gameval name, or
// false.
func (d *CacheDefinitions) GetSpotanimId(name string) (int, bool, error) {
	return d.idForName(GamevalSpotAnim, name)
}

// GetSeqName returns an animation's gameval name, or false when
// the cache has none for it.
func (d *CacheDefinitions) GetSeqName(id int) (string, bool, error) {
	names, err := d.namesForKind(GamevalSeq)
	if err != nil {
		return "", false, err
	}
	name, ok := names[id]
	return name, ok, nil
}

// GetDbRowName returns a database row's gameval name, or false.
func (d *CacheDefinitions) GetDbRowName(id int) (string, bool, error) {
	names, err := d.namesForKind(GamevalDbRow)
	if err != nil {
		return "", false, err
	}
	name, ok := names[id]
	return name, ok, nil
}

// GetSeqNames returns every named animation: id -> gameval name.
func (d *CacheDefinitions) GetSeqNames() (map[int]string, error) {
	return d.namesForKind(GamevalSeq)
}

// GetLocNames returns every named loc: id -> gameval name.
func (d *CacheDefinitions) GetLocNames() (map[int]string, error) {
	return d.namesForKind(GamevalLoc)
}

// GetObjNames returns every named obj: id -> gameval name.
func (d *CacheDefinitions) GetObjNames() (map[int]string, error) {
	return d.namesForKind(GamevalObj)
}

// GetSpotanimNames returns every named graphic: id -> gameval
// name.
func (d *CacheDefinitions) GetSpotanimNames() (map[int]string, error) {
	return d.namesForKind(GamevalSpotAnim)
}

// GetVarbitNames returns every named varbit: id -> gameval name.
func (d *CacheDefinitions) GetVarbitNames() (map[int]string, error) {
	return d.namesForKind(GamevalVarbit)
}

// GetDbRowNames returns every named db row: id -> gameval name.
func (d *CacheDefinitions) GetDbRowNames() (map[int]string, error) {
	return d.namesForKind(GamevalDbRow)
}

// GetDbTableNames returns every named db table: id -> gameval
// name.
func (d *CacheDefinitions) GetDbTableNames() (map[int]string, error) {
	return d.namesForKind(GamevalDbTable)
}

// GetVarpNames returns every named varp: id -> gameval name.
func (d *CacheDefinitions) GetVarpNames() (map[int]string, error) {
	return d.namesForKind(GamevalVarp)
}

// GetInventoryNames returns every named inventory: id -> gameval
// name.
func (d *CacheDefinitions) GetInventoryNames() (map[int]string, error) {
	return d.namesForKind(GamevalInv)
}

// GetNpcNames returns every named NPC: id -> gameval name.
func (d *CacheDefinitions) GetNpcNames() (map[int]string, error) {
	return d.namesForKind(GamevalNPC)
}

// GetSpriteId returns a sprite archive by its cache name,
// including multi-frame overhead icons, or false. The sprites
// index is built lazily on first use; TypeScript builds a fresh
// index per call, which re-reads the same table.
func (d *CacheDefinitions) GetSpriteId(name string) (int, bool, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.sprites == nil {
		source, err := d.factory(IndexSprites)
		if err != nil {
			return 0, false, err
		}
		named, ok := source.(namedArchiveSource)
		if !ok {
			return 0, false, errSpritesNoNames
		}
		d.sprites = named
	}
	id := d.sprites.ArchiveID(name)
	if id < 0 {
		return 0, false, nil
	}
	return id, true, nil
}

// GetComponentId returns an interface component's packed id by its
// gameval name (`bankside:runepouch_top`), or false.
func (d *CacheDefinitions) GetComponentId(name string) (int, bool, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	gamevals, err := d.gamevalsLocked()
	if err != nil {
		return 0, false, err
	}
	return gamevals.ComponentID(name)
}

// GetInterfaceId returns an interface's group id by its gameval
// name (`poh_jewellery_box`), or false.
func (d *CacheDefinitions) GetInterfaceId(name string) (int, bool, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	gamevals, err := d.gamevalsLocked()
	if err != nil {
		return 0, false, err
	}
	interfaces, err := gamevals.AllInterfaces()
	if err != nil {
		return 0, false, err
	}
	for group, iface := range interfaces {
		if iface.Name == name {
			return group, true, nil
		}
	}
	return 0, false, nil
}

// structArchiveLocked returns the struct archive, read once: the
// TypeScript comment notes re-reading it per struct took ~75 ms
// each.
func (d *CacheDefinitions) structArchiveLocked() (*Archive, error) {
	if d.structArchive != nil {
		return d.structArchive, nil
	}
	archive, err := d.configs.GetArchive(ConfigStruct, nil)
	if err != nil {
		return nil, err
	}
	d.structArchive = archive
	return archive, nil
}

// GetStructParams returns a cache struct's params (config archive
// 34), the key -> value records cache scripts read with
// struct_param. Empty when the struct does not exist.
func (d *CacheDefinitions) GetStructParams(id int) (ParamsMap, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if cached, ok := d.structParams[id]; ok {
		return cached, nil
	}
	archive, err := d.structArchiveLocked()
	if err != nil {
		return nil, err
	}
	data, ok := archive.GetFile(id)
	var params ParamsMap
	if ok {
		params, err = DecodeStructParams(data)
		if err != nil {
			return nil, err
		}
	} else {
		params = make(ParamsMap)
	}
	d.structParams[id] = params
	return params, nil
}

// enumArchiveLocked returns the enums archive, decoded once;
// decompressing it for each new enum caused a hitch in TypeScript.
func (d *CacheDefinitions) enumArchiveLocked() (*Archive, error) {
	if d.enumArchive != nil {
		return d.enumArchive, nil
	}
	archive, err := d.configs.GetArchive(ConfigEnums, nil)
	if err != nil {
		return nil, err
	}
	d.enumArchive = archive
	return archive, nil
}

// PreloadEnums decodes the enums archive at startup, so no enum's
// first lookup does it mid-tick. Failures are ignored, as in
// TypeScript: without a cache, enums are looked up lazily instead.
func (d *CacheDefinitions) PreloadEnums() {
	d.mu.Lock()
	defer d.mu.Unlock()
	_, _ = d.enumArchiveLocked()
}

// GetEnumValues returns a cache enum's key -> value entries
// (config archive 8), the tables cache scripts read with enum.
// Empty when the enum does not exist. The default value is not
// included.
func (d *CacheDefinitions) GetEnumValues(id int) (ParamsMap, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if cached, ok := d.enumValues[id]; ok {
		return cached, nil
	}
	archive, err := d.enumArchiveLocked()
	if err != nil {
		return nil, err
	}
	data, ok := archive.GetFile(id)
	var values ParamsMap
	if ok {
		values, err = DecodeEnumValues(data)
		if err != nil {
			return nil, err
		}
	} else {
		values = make(ParamsMap)
	}
	d.enumValues[id] = values
	return values, nil
}

// DefinitionArchiveChanges lists the ids whose archived source
// changed, per kind: the union of the ids before and after a
// registry replacement.
type DefinitionArchiveChanges struct {
	NPC []int
	Loc []int
	Obj []int
	Seq []int
}

func unionArchivedIDs(before, after []int) []int {
	seen := make(map[int]bool)
	var out []int
	for _, id := range append(append([]int{}, before...), after...) {
		if !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	return out
}

// RegisterDefinitionArchives validates bundles against a candidate
// registry and, on success, publishes them, returning the changed
// ids per kind. It mirrors
// CacheDefinitions.registerDefinitionArchives. The TypeScript
// collision check against custom definitions is vacuous here —
// this port registers no custom definitions — and dependent
// custom definitions cannot exist, so both steps are omitted.
func (d *CacheDefinitions) RegisterDefinitionArchives(bundles []ArchivedDefinitionBundle) (DefinitionArchiveChanges, error) {
	candidate, err := NewArchivedDefinitionRegistry(d.archiveContext)
	if err != nil {
		return DefinitionArchiveChanges{}, err
	}
	if err := candidate.Replace(bundles); err != nil {
		return DefinitionArchiveChanges{}, err
	}
	before := DefinitionArchiveChanges{
		NPC: d.archives.IDs(ArchivedKindNPC),
		Loc: d.archives.IDs(ArchivedKindLoc),
		Obj: d.archives.IDs(ArchivedKindObj),
		Seq: d.archives.IDs(ArchivedKindSeq),
	}
	if err := d.archives.Replace(bundles); err != nil {
		return DefinitionArchiveChanges{}, err
	}
	afterObj := d.archives.IDs(ArchivedKindObj)
	if len(before.Obj) > 0 || len(afterObj) > 0 {
		d.mu.Lock()
		d.spellsReady = false
		d.spellNamesByWidget = nil
		d.spellsByName = nil
		d.spellNamesOrder = nil
		d.spellWidgetGroups = nil
		d.mu.Unlock()
	}
	d.sequences.ClearArchivedCache()
	d.npcs.ClearArchivedCache()
	d.objects.ClearArchivedCache()
	d.objArchives.ClearArchivedCache()
	return DefinitionArchiveChanges{
		NPC: unionArchivedIDs(before.NPC, d.archives.IDs(ArchivedKindNPC)),
		Loc: unionArchivedIDs(before.Loc, d.archives.IDs(ArchivedKindLoc)),
		Obj: unionArchivedIDs(before.Obj, afterObj),
		Seq: unionArchivedIDs(before.Seq, d.archives.IDs(ArchivedKindSeq)),
	}, nil
}

// GetDefinitionArchiveBundles returns the published archived
// bundles.
func (d *CacheDefinitions) GetDefinitionArchiveBundles() []ArchivedDefinitionBundle {
	return d.archives.GetBundles()
}

// GetDefinitionArchiveChanges validates a candidate and reports
// semantic source changes without publishing it. It mirrors
// CacheDefinitions.getDefinitionArchiveChanges: an id counts as
// changed when its registered source differs (compared as JSON)
// between the published and candidate registries.
func (d *CacheDefinitions) GetDefinitionArchiveChanges(bundles []ArchivedDefinitionBundle) (DefinitionArchiveChanges, error) {
	candidate, err := NewArchivedDefinitionRegistry(d.archiveContext)
	if err != nil {
		return DefinitionArchiveChanges{}, err
	}
	if err := candidate.Replace(bundles); err != nil {
		return DefinitionArchiveChanges{}, err
	}
	sourceJSON := func(r *ArchivedDefinitionRegistry, kind ArchivedDefinitionKind, id int) string {
		source, ok := r.GetSource(kind, id)
		if !ok {
			return "null"
		}
		data, err := json.Marshal(source)
		if err != nil {
			return ""
		}
		return string(data)
	}
	changed := func(kind ArchivedDefinitionKind) []int {
		var out []int
		for _, id := range unionArchivedIDs(d.archives.IDs(kind), candidate.IDs(kind)) {
			if sourceJSON(d.archives, kind, id) != sourceJSON(candidate, kind, id) {
				out = append(out, id)
			}
		}
		return out
	}
	return DefinitionArchiveChanges{
		NPC: changed(ArchivedKindNPC),
		Loc: changed(ArchivedKindLoc),
		Obj: changed(ArchivedKindObj),
		Seq: changed(ArchivedKindSeq),
	}, nil
}

// GetArchivedID resolves a bundle symbol to its appended id, or
// false when the bundle has no such definition.
func (d *CacheDefinitions) GetArchivedID(bundle string, kind ArchivedDefinitionKind, symbol string) (int, bool) {
	return d.archives.Resolve(bundle, kind, symbol)
}

// Spell parameter ids (CacheDefinitions.ts:61-65).
const (
	spellWidgetParamID = 596
	spellNameParamID   = 601
	spellLevelParamID  = 604
)

// spellRuneParamIDs are a spell obj's runes as (obj, count) param
// pairs: 365/366, 367/368, 369/370, 371/372.
var spellRuneParamIDs = [][2]int{{365, 366}, {367, 368}, {369, 370}, {371, 372}}

// SpellRef is the spellbook widget and spell item a client sends
// for a spell.
type SpellRef struct {
	WidgetID int
	ItemID   int
}

// SpellRune is one rune requirement: an item id and an amount.
type SpellRune struct {
	ID     int
	Amount int
}

// SpellRequirements are the Magic level and runes a spell needs.
type SpellRequirements struct {
	Level int
	Runes []SpellRune
}

// ensureSpellMetadataLocked builds the spell indexes from item
// params on first use. Callers hold d.mu; the item scan goes
// through GetItem, which takes no facade lock.
func (d *CacheDefinitions) ensureSpellMetadataLocked() {
	if d.spellsReady {
		return
	}
	spellNamesByWidget := make(map[int]string)
	spellsByName := make(map[string]SpellRef)
	var spellNamesOrder []string
	spellWidgetGroups := make(map[int]bool)
	for id := 0; id < d.items.Count(); id++ {
		params := d.GetItem(id).Params
		widgetID, okWidget := ParamInt(params, spellWidgetParamID)
		spellName, okName := ParamString(params, spellNameParamID)
		if !okWidget || !okName {
			continue
		}
		spellNamesByWidget[widgetID] = spellName
		spellWidgetGroups[int(uint32(widgetID)>>16)] = true
		normalized := strings.ToLower(spellName)
		if _, ok := spellsByName[normalized]; !ok {
			spellsByName[normalized] = SpellRef{WidgetID: widgetID, ItemID: id}
			spellNamesOrder = append(spellNamesOrder, normalized)
		}
	}
	d.spellNamesByWidget = spellNamesByWidget
	d.spellsByName = spellsByName
	d.spellNamesOrder = spellNamesOrder
	d.spellWidgetGroups = spellWidgetGroups
	d.spellsReady = true
}

// GetSpellName returns the spell name for a widget id, preferring
// the named item's param when the item id is in range.
func (d *CacheDefinitions) GetSpellName(widgetID, itemID int) (string, bool) {
	if itemID > 0 && itemID < d.items.Count() {
		if name, ok := ParamString(d.GetItem(itemID).Params, spellNameParamID); ok {
			return name, true
		}
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	d.ensureSpellMetadataLocked()
	name, ok := d.spellNamesByWidget[widgetID]
	return name, ok
}

// IsSpellWidgetGroup reports whether a widget group id holds
// spells.
func (d *CacheDefinitions) IsSpellWidgetGroup(groupID int) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.ensureSpellMetadataLocked()
	return d.spellWidgetGroups[groupID]
}

// GetSpellByName returns the spellbook widget and spell item a
// client sends for the spell with this name.
func (d *CacheDefinitions) GetSpellByName(name string) (SpellRef, bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.ensureSpellMetadataLocked()
	ref, ok := d.spellsByName[strings.ToLower(strings.TrimSpace(name))]
	return ref, ok
}

// GetSpellNames returns every spell name the cache's spell objs
// carry (lower case), in first-seen order.
func (d *CacheDefinitions) GetSpellNames() []string {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.ensureSpellMetadataLocked()
	return append([]string{}, d.spellNamesOrder...)
}

// GetSpellRequirements returns the Magic level and runes the
// cache's spell obj lists for the spell with this name.
func (d *CacheDefinitions) GetSpellRequirements(name string) (SpellRequirements, bool) {
	spell, ok := d.GetSpellByName(name)
	if !ok {
		return SpellRequirements{}, false
	}
	params := d.GetItem(spell.ItemID).Params
	var runes []SpellRune
	for _, pair := range spellRuneParamIDs {
		id, okID := ParamInt(params, pair[0])
		amount, okAmount := ParamInt(params, pair[1])
		if okID && id > 0 && okAmount && amount > 0 {
			runes = append(runes, SpellRune{ID: id, Amount: amount})
		}
	}
	level, _ := ParamInt(params, spellLevelParamID)
	return SpellRequirements{Level: level, Runes: runes}, true
}
