package cache

import (
	"container/list"
	"errors"
	"fmt"
	"sync"
)

// This file ports the generic definition machinery: Type.ts (the
// opcode-decode loop, params maps, cache-type detection),
// TypeLoader.ts (the loader family), CacheInfo.ts and CacheType.ts.
//
// Upstream sources (paths relative to
// server/src/main/typescript/elvarg/game/cache unless noted):
//   - codec/rs/config/Type.ts
//   - codec/rs/config/TypeLoader.ts
//   - codec/rs/cache/CacheInfo.ts, CacheType.ts
//   - codec/rs/MathConstants.ts (BIT_MASKS, for IndexTypeLoader)

// GameType identifies the game a cache belongs to. It is the GameType
// union in CacheInfo.ts.
type GameType string

const (
	// GameClassic is a RuneScape Classic cache.
	GameClassic GameType = "classic"
	// GameRunescape is a RuneScape 2/3 cache.
	GameRunescape GameType = "runescape"
	// GameOldschool is an Old School RuneScape cache.
	GameOldschool GameType = "oldschool"
)

// CacheInfo describes the cache a definition was decoded from. It
// mirrors the CacheInfo type in CacheInfo.ts. (The unexported
// cacheInfo in pipeline.go is the unrelated info.json subset.)
type CacheInfo struct {
	Name        string   `json:"name"`
	Game        GameType `json:"game"`
	Environment string   `json:"environment"`
	Revision    int      `json:"revision"`
	Timestamp   string   `json:"timestamp"`
	Size        int      `json:"size"`
}

// CacheInfoForActive builds the CacheInfo CacheDefinitions.getState
// uses (CacheDefinitions.ts:108-115): the active cache's name,
// revision and timestamp, game "oldschool", environment "live", size
// 0.
func CacheInfoForActive(active *ActiveCache) CacheInfo {
	return CacheInfo{
		Name:        active.Name,
		Game:        GameOldschool,
		Environment: "live",
		Revision:    active.Revision,
		Timestamp:   active.Timestamp,
		Size:        0,
	}
}

// CacheType is the on-disk layout family of a cache. It is the
// CacheType union in CacheType.ts.
type CacheType string

const (
	// CacheTypeClassic is a RuneScape Classic cache layout.
	CacheTypeClassic CacheType = "classic"
	// CacheTypeLegacy is a pre-234 RuneScape cache layout.
	CacheTypeLegacy CacheType = "legacy"
	// CacheTypeDat is a 234-409 RuneScape cache layout.
	CacheTypeDat CacheType = "dat"
	// CacheTypeDat2 is the layout every Old School cache uses.
	CacheTypeDat2 CacheType = "dat2"
)

// DetectCacheType returns the layout family for a cache, mirroring
// detectCacheType (CacheType.ts:464-481). An unknown game is an
// error, as in TypeScript (which throws).
func DetectCacheType(info CacheInfo) (CacheType, error) {
	switch info.Game {
	case GameClassic:
		return CacheTypeClassic, nil
	case GameRunescape:
		if info.Revision < 234 {
			return CacheTypeLegacy, nil
		} else if info.Revision < 410 {
			return CacheTypeDat, nil
		}
		return CacheTypeDat2, nil
	case GameOldschool:
		return CacheTypeDat2, nil
	default:
		return "", fmt.Errorf("Unknown game type: %s", info.Game)
	}
}

// detectCacheTypeOrDat2 is DetectCacheType with the unknown-game
// error collapsed to dat2. Definition constructors cannot fail in
// TypeScript either (the throw would escape the constructor); the
// facade and the archived registry only ever construct types for
// "oldschool" caches, so the fallback is unreachable in practice and
// exists only to keep constructors total.
func detectCacheTypeOrDat2(info CacheInfo) CacheType {
	if ct, err := DetectCacheType(info); err == nil {
		return ct
	}
	return CacheTypeDat2
}

// ParamsMap is the key -> value record block opcode 249 carries. It
// mirrors ParamsMap (Type.ts:5): keys are ints, values are int32
// (from readInt) or string. A nil map means the definition carries
// no params block, as an undefined ParamsMap does in TypeScript.
type ParamsMap map[int]any

// ParamInt returns a params value as an int, reporting false when
// the key is absent or the value is a string. It is the Go form of
// the `typeof value === "number"` checks callers apply to
// ParamsMap.get results in TypeScript.
func ParamInt(params ParamsMap, key int) (int, bool) {
	v, ok := params[key]
	if !ok {
		return 0, false
	}
	switch n := v.(type) {
	case int32:
		return int(n), true
	case int:
		return n, true
	default:
		return 0, false
	}
}

// ParamString returns a params value as a string, reporting false
// when the key is absent or the value is numeric.
func ParamString(params ParamsMap, key int) (string, bool) {
	v, ok := params[key]
	if !ok {
		return "", false
	}
	s, ok := v.(string)
	return s, ok
}

// ReadParamsMap reads a params block: a count byte, then per entry a
// string-flag byte, a medium key, and a string or int value. It
// mirrors Type.readParamsMap (Type.ts:14-30), merging into params
// when non-nil and allocating a new map otherwise. ByteBuffer errors
// are returned unwrapped so the decode loop can recognize
// ErrBufferOverflow.
func ReadParamsMap(buf *ByteBuffer, params ParamsMap) (ParamsMap, error) {
	count, err := buf.ReadUnsignedByte()
	if err != nil {
		return params, err
	}
	if params == nil {
		params = make(ParamsMap)
	}
	for i := 0; i < count; i++ {
		flag, err := buf.ReadUnsignedByte()
		if err != nil {
			return params, err
		}
		key, err := buf.ReadMedium()
		if err != nil {
			return params, err
		}
		if flag == 1 {
			s, err := buf.ReadString()
			if err != nil {
				return params, err
			}
			params[key] = s
		} else {
			v, err := buf.ReadInt()
			if err != nil {
				return params, err
			}
			params[key] = v
		}
	}
	return params, nil
}

// baseType holds the fields every definition type inherits from
// Type (Type.ts:7-36): its id, the cache it came from, and the
// derived cache type. Definition structs embed it.
type baseType struct {
	id        int
	cacheInfo CacheInfo
	cacheType CacheType
}

func newBaseType(id int, info CacheInfo) baseType {
	return baseType{id: id, cacheInfo: info, cacheType: detectCacheTypeOrDat2(info)}
}

// ID returns the definition's id.
func (t *baseType) ID() int { return t.id }

// CacheInfo returns the cache the definition was decoded from.
func (t *baseType) CacheInfo() CacheInfo { return t.cacheInfo }

// CacheType returns the layout family of the definition's cache.
func (t *baseType) CacheType() CacheType { return t.cacheType }

// isLargeModelID reports whether model ids in this cache are big
// smarts rather than unsigned shorts. Every type that reads model
// ids repeats this predicate (ObjType.ts:162-164,
// NpcType.ts:159-161, LocType.ts:186-188).
func (t *baseType) isLargeModelID() bool {
	return t.cacheInfo.Game == GameRunescape && t.cacheInfo.Revision >= 670
}

// readString reads a definition string: NUL-terminated for dat2
// caches, newline-terminated (0xa) otherwise. It is Type.readString
// (Type.ts:38-41).
func (t *baseType) readString(buf *ByteBuffer) (string, error) {
	stop := byte(0)
	if t.cacheType != CacheTypeDat2 {
		stop = 0xa
	}
	return buf.ReadStringTerminated(stop)
}

// opcodeDecoder is the part of a definition type the shared decode
// loop needs: one opcode's payload reader plus the cache type that
// decides EOF tolerance.
type opcodeDecoder interface {
	DecodeOpcode(opcode int, buf *ByteBuffer) error
	CacheType() CacheType
}

// decodeLoop runs the Type.decode loop (Type.ts:43-68) over buf:
// opcodes until 0, EOF, or error. For dat2 caches, reaching EOF is
// an implicit terminator and a mid-opcode buffer overflow stops the
// loop cleanly, leaving whatever fields were already read; for other
// cache types both are errors. Unknown-opcode errors always
// propagate. Callers run Post separately, as TypeLoader does.
func decodeLoop(d opcodeDecoder, buf *ByteBuffer) error {
	dat2 := d.CacheType() == CacheTypeDat2
	for {
		// For DAT2 (per-file) config entries, reaching EOF can be
		// treated as an implicit terminator to tolerate empty or
		// truncated files.
		if buf.Offset >= buf.Len() {
			if dat2 {
				return nil
			}
			return ErrBufferOverflow
		}
		opcode, err := buf.ReadUnsignedByte()
		if err != nil {
			return err
		}
		if opcode == 0 {
			return nil
		}
		if err := d.DecodeOpcode(opcode, buf); err != nil {
			// Gracefully stop on mid-opcode EOF for DAT2 entries.
			if dat2 && errors.Is(err, ErrBufferOverflow) {
				return nil
			}
			return err
		}
	}
}

// DefinitionType is a decodable definition: the constraint on the
// generic loaders. Decode runs the opcode loop over one file's
// bytes; Post finalizes derived defaults.
type DefinitionType interface {
	Decode(data []byte) error
	Post()
}

// VarReader is the Pick<VarManager, "getVarp" | "getVarbit"> view
// NpcType.transform and LocType.transform need (NpcType.ts:612,
// LocType.ts:607). The full VarManager lives outside game/cache and
// is not ported here.
type VarReader interface {
	GetVarp(id int) int
	GetVarbit(id int) int
}

// TypeLoader loads definitions of one kind by id, caching them. It
// mirrors the TypeLoader interface (TypeLoader.ts:81-87). Load never
// fails: an unknown or undecodable id yields a default-initialized
// type, as BaseTypeLoader.load does (it catches decode errors and
// caches the partial type). Count is one past the highest id the
// loader can serve.
type TypeLoader[T any] interface {
	Load(id int) T
	Count() int
	ClearCache()
}

// typeCache is an LRU-bounded id -> value cache, the Go form of the
// insertion-ordered Map BaseTypeLoader keeps (TypeLoader.ts:109-112,
// 150-157): reads touch the entry, and the oldest entry is evicted
// while the cache exceeds maxCacheSize (default 4096). It is not
// safe for concurrent use; loaders guard it with a mutex.
type typeCache[T any] struct {
	max   int
	order *list.List // front = oldest, back = newest; elements are int ids
	items map[int]*list.Element
	vals  map[int]T
}

func newTypeCache[T any](max int) *typeCache[T] {
	return &typeCache[T]{max: max, order: list.New(), items: make(map[int]*list.Element), vals: make(map[int]T)}
}

func (c *typeCache[T]) get(id int) (T, bool) {
	var zero T
	el, ok := c.items[id]
	if !ok {
		return zero, false
	}
	c.order.MoveToBack(el)
	return c.vals[id], true
}

func (c *typeCache[T]) put(id int, v T) {
	if el, ok := c.items[id]; ok {
		c.order.MoveToBack(el)
		c.vals[id] = v
		return
	}
	c.items[id] = c.order.PushBack(id)
	c.vals[id] = v
	for c.order.Len() > c.max {
		oldest := c.order.Front()
		if oldest == nil {
			break
		}
		oldID := oldest.Value.(int)
		c.order.Remove(oldest)
		delete(c.items, oldID)
		delete(c.vals, oldID)
	}
}

func (c *typeCache[T]) clear() {
	c.order.Init()
	c.items = make(map[int]*list.Element)
	c.vals = make(map[int]T)
}

// defaultTypeCacheSize is BaseTypeLoader.maxCacheSize
// (TypeLoader.ts:112).
const defaultTypeCacheSize = 4096

// defaultArchiveCacheSize is IndexTypeLoader.maxArchiveCacheSize
// (TypeLoader.ts:187).
const defaultArchiveCacheSize = 256

// loadDecoded builds a fresh type, decodes data into it when
// present, and runs Post only when decoding succeeded, mirroring
// the try block of BaseTypeLoader.load (TypeLoader.ts:129-141).
// Decode errors are swallowed — the partial type is returned — as
// TypeScript logs and continues; callers that need the error use the
// Decode* functions directly.
func loadDecoded[T DefinitionType](newFunc func(id int, info CacheInfo) T, info CacheInfo, id int, data []byte, present bool) T {
	t := newFunc(id, info)
	if present {
		if err := t.Decode(data); err == nil {
			t.Post()
		}
	}
	return t
}

// ArchiveTypeLoader loads one definition per file of a single
// archive. It mirrors ArchiveTypeLoader (TypeLoader.ts:160-180):
// getDataBuffer reads file id from the archive, and Count is one
// past the highest file id (not the file count, which is smaller
// when ids have gaps). Load is safe for concurrent use; the
// TypeScript loader is single-threaded, so the mutex is a
// Go-necessary addition.
type ArchiveTypeLoader[T DefinitionType] struct {
	newFunc func(id int, info CacheInfo) T
	info    CacheInfo
	archive *Archive

	mu    sync.Mutex
	cache *typeCache[T]
}

// NewArchiveTypeLoader builds an ArchiveTypeLoader over archive.
// newFunc constructs a default-initialized type of the loader's
// kind, e.g. NewObjType.
func NewArchiveTypeLoader[T DefinitionType](newFunc func(id int, info CacheInfo) T, info CacheInfo, archive *Archive) *ArchiveTypeLoader[T] {
	return &ArchiveTypeLoader[T]{newFunc: newFunc, info: info, archive: archive, cache: newTypeCache[T](defaultTypeCacheSize)}
}

// Archive returns the archive this loader reads files from. It is
// the archive field ArchiveTypeLoader exposes in TypeScript, which
// CacheDefinitions uses for file-existence checks.
func (l *ArchiveTypeLoader[T]) Archive() *Archive { return l.archive }

// Load returns the decoded type for id, caching it. Unknown ids
// yield a default type, as in TypeScript.
func (l *ArchiveTypeLoader[T]) Load(id int) T {
	l.mu.Lock()
	defer l.mu.Unlock()
	if cached, ok := l.cache.get(id); ok {
		return cached
	}
	data, present := l.archive.GetFile(id)
	t := loadDecoded(l.newFunc, l.info, id, data, present)
	l.cache.put(id, t)
	return t
}

// Count returns one past the highest file id in the archive.
func (l *ArchiveTypeLoader[T]) Count() int {
	if l.archive == nil {
		return 0
	}
	return l.archive.LastFileID + 1
}

// ClearCache drops all cached types.
func (l *ArchiveTypeLoader[T]) ClearCache() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.cache.clear()
}

// IndexTypeLoader loads definitions spread across an index's
// archives: archive id = id >> fileIdBits, file id = the low
// fileIdBits bits. It mirrors IndexTypeLoader
// (TypeLoader.ts:182-235), including the count formula
// (max((archiveCount-1) * filesPerArchive, 0) + fileCount of the
// last archive) and the 256-entry LRU archive cache. Archives that
// fail to load are treated as absent, matching the catch in
// BaseTypeLoader.load.
type IndexTypeLoader[T DefinitionType] struct {
	newFunc    func(id int, info CacheInfo) T
	info       CacheInfo
	index      ArchiveSource
	fileIDBits int
	count      int

	mu       sync.Mutex
	cache    *typeCache[T]
	archives *typeCache[*Archive]
}

// NewIndexTypeLoader builds an IndexTypeLoader over index with the
// given file-id width (8 by default in TypeScript; NPC indexes use
// 7, varbit indexes 10).
func NewIndexTypeLoader[T DefinitionType](newFunc func(id int, info CacheInfo) T, info CacheInfo, index ArchiveSource, fileIDBits int) *IndexTypeLoader[T] {
	filesPerArchive := 1 << fileIDBits
	count := (index.ArchiveCount()-1)*filesPerArchive + index.FileCount(index.LastArchiveID())
	if count < 0 {
		count = 0
	}
	return &IndexTypeLoader[T]{
		newFunc: newFunc, info: info, index: index, fileIDBits: fileIDBits, count: count,
		cache: newTypeCache[T](defaultTypeCacheSize), archives: newTypeCache[*Archive](defaultArchiveCacheSize),
	}
}

// Load returns the decoded type for id, caching it.
func (l *IndexTypeLoader[T]) Load(id int) T {
	l.mu.Lock()
	defer l.mu.Unlock()
	if cached, ok := l.cache.get(id); ok {
		return cached
	}
	archiveID := id >> l.fileIDBits
	// BIT_MASKS[fileIdBits-1] is (1<<fileIdBits)-1
	// (MathConstants.ts:592-605).
	fileID := id & ((1 << l.fileIDBits) - 1)
	var data []byte
	present := false
	archive, ok := l.archives.get(archiveID)
	if !ok {
		loaded, err := l.index.GetArchive(archiveID, nil)
		if err == nil {
			archive = loaded
			l.archives.put(archiveID, archive)
			ok = true
		}
	}
	if ok && archive != nil {
		data, present = archive.GetFile(fileID)
	}
	t := loadDecoded(l.newFunc, l.info, id, data, present)
	l.cache.put(id, t)
	return t
}

// Count returns the count computed at construction.
func (l *IndexTypeLoader[T]) Count() int { return l.count }

// ClearCache drops cached types and archives.
func (l *IndexTypeLoader[T]) ClearCache() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.cache.clear()
	l.archives.clear()
}

// DummyTypeLoader always returns a fresh default type and reports
// count 0. It mirrors DummyTypeLoader (TypeLoader.ts:91-106).
type DummyTypeLoader[T DefinitionType] struct {
	newFunc func(id int, info CacheInfo) T
	info    CacheInfo
}

// NewDummyTypeLoader builds a DummyTypeLoader.
func NewDummyTypeLoader[T DefinitionType](newFunc func(id int, info CacheInfo) T, info CacheInfo) *DummyTypeLoader[T] {
	return &DummyTypeLoader[T]{newFunc: newFunc, info: info}
}

// Load returns a fresh default type.
func (l *DummyTypeLoader[T]) Load(id int) T { return l.newFunc(id, l.info) }

// Count returns 0.
func (l *DummyTypeLoader[T]) Count() int { return 0 }

// ClearCache is a no-op.
func (l *DummyTypeLoader[T]) ClearCache() {}

// DatTypeLoader holds types decoded eagerly from one "<name>.dat"
// file in a config archive: a u16 count followed by count sequential
// decodes. It mirrors DatTypeLoader (TypeLoader.ts:237-272).
type DatTypeLoader[T DefinitionType] struct {
	types []T
}

// LoadDatTypes decodes a DatTypeLoader from configArchive's
// "<name>.dat" file, mirroring DatTypeLoader.load
// (TypeLoader.ts:238-259). A missing file or a decode failure is an
// error, as the TypeScript static loader throws.
func LoadDatTypes[T DefinitionType](newFunc func(id int, info CacheInfo) T, info CacheInfo, configArchive *Archive, name string) (*DatTypeLoader[T], error) {
	data, ok := configArchive.GetFileNamed(name + ".dat")
	if !ok {
		return nil, fmt.Errorf("%s.dat not found", name)
	}
	buf := NewByteBuffer(data)
	count, err := buf.ReadUnsignedShort()
	if err != nil {
		return nil, err
	}
	types := make([]T, count)
	for i := 0; i < count; i++ {
		t := newFunc(i, info)
		if err := decodeLoop(opcodeDecoderOf(t), buf); err != nil {
			return nil, err
		}
		t.Post()
		types[i] = t
	}
	return &DatTypeLoader[T]{types: types}, nil
}

// Load returns the type for id, or the zero value when id is out of
// range (TypeScript returns undefined for an out-of-range index).
func (l *DatTypeLoader[T]) Load(id int) T {
	var zero T
	if id < 0 || id >= len(l.types) {
		return zero
	}
	return l.types[id]
}

// Count returns the number of decoded types.
func (l *DatTypeLoader[T]) Count() int { return len(l.types) }

// ClearCache is a no-op.
func (l *DatTypeLoader[T]) ClearCache() {}

// opcodeDecoderOf adapts a DefinitionType to the decode loop. Every
// DefinitionType in this package also implements DecodeOpcode and
// CacheType via its embedded baseType; the assertion documents that
// coupling. (It cannot fail for types constructed here.)
func opcodeDecoderOf[T DefinitionType](t T) opcodeDecoder {
	return any(t).(opcodeDecoder)
}

// IndexedDatTypeLoader loads types lazily from a "<name>.dat" file
// using offsets from a "<name>.idx" file. It mirrors
// IndexedDatTypeLoader (TypeLoader.ts:274-332): the .idx file holds
// a u16 count then per-type u16 lengths, and each type decodes from
// its offset in the shared .dat buffer. Unlike TypeScript, which
// mutates one shared buffer's offset per load, each load here
// decodes from a fresh view at the type's offset, which is
// observably identical under single-threaded use and safe under
// concurrent loads.
type IndexedDatTypeLoader[T DefinitionType] struct {
	newFunc     func(id int, info CacheInfo) T
	info        CacheInfo
	count       int
	data        []byte
	dataOffsets []int32

	mu    sync.Mutex
	cache *typeCache[T]
}

// LoadIndexedDatTypes builds an IndexedDatTypeLoader from
// configArchive's "<name>.dat" and "<name>.idx" files, mirroring
// IndexedDatTypeLoader.load (TypeLoader.ts:275-307).
func LoadIndexedDatTypes[T DefinitionType](newFunc func(id int, info CacheInfo) T, info CacheInfo, configArchive *Archive, name string) (*IndexedDatTypeLoader[T], error) {
	dataFile, ok := configArchive.GetFileNamed(name + ".dat")
	if !ok {
		return nil, fmt.Errorf("%s.dat not found", name)
	}
	indexFile, ok := configArchive.GetFileNamed(name + ".idx")
	if !ok {
		return nil, fmt.Errorf("%s.idx not found", name)
	}
	indexBuf := NewByteBuffer(indexFile)
	count, err := indexBuf.ReadUnsignedShort()
	if err != nil {
		return nil, err
	}
	offsets := make([]int32, count)
	offset := int32(indexBuf.Offset)
	for i := 0; i < count; i++ {
		offsets[i] = offset
		length, err := indexBuf.ReadUnsignedShort()
		if err != nil {
			return nil, err
		}
		offset += int32(length)
	}
	return &IndexedDatTypeLoader[T]{
		newFunc: newFunc, info: info, count: count, data: dataFile, dataOffsets: offsets,
		cache: newTypeCache[T](defaultTypeCacheSize),
	}, nil
}

// Load returns the decoded type for id. Out-of-range ids yield a
// default type, as getDataBuffer returning undefined does in
// TypeScript.
func (l *IndexedDatTypeLoader[T]) Load(id int) T {
	l.mu.Lock()
	defer l.mu.Unlock()
	if cached, ok := l.cache.get(id); ok {
		return cached
	}
	t := l.newFunc(id, l.info)
	if id >= 0 && id < l.count {
		buf := NewByteBuffer(l.data)
		buf.Offset = int(l.dataOffsets[id])
		if err := decodeLoop(opcodeDecoderOf(t), buf); err == nil {
			t.Post()
		}
	}
	l.cache.put(id, t)
	return t
}

// Count returns the number of indexed types.
func (l *IndexedDatTypeLoader[T]) Count() int { return l.count }

// ClearCache is a no-op, matching the TypeScript override
// (TypeLoader.ts:331), which deliberately does not clear the base
// cache.
func (l *IndexedDatTypeLoader[T]) ClearCache() {}
