package definition

// Fake cache construction for tests, using only the exported
// internal/cache API (its test helpers are same-package and not
// importable). Archives are built with cache.DecodeArchive over
// hand-encoded opcode streams; gameval names come from a fake
// index-24 source. No real cache data is involved.

import (
	"encoding/binary"
	"sort"
	"testing"

	"tsps-go/internal/cache"
)

// opStream builds one definition file's opcode stream.
type opStream struct {
	buf []byte
}

func (s *opStream) u8(v int) *opStream {
	s.buf = append(s.buf, byte(v))
	return s
}

func (s *opStream) u16(v int) *opStream {
	s.buf = append(s.buf, byte(v>>8), byte(v))
	return s
}

func (s *opStream) short(v int) *opStream {
	return s.u16(v & 0xffff)
}

func (s *opStream) i32(v int) *opStream {
	var b [4]byte
	binary.BigEndian.PutUint32(b[:], uint32(int32(v)))
	s.buf = append(s.buf, b[:]...)
	return s
}

func (s *opStream) str(text string) *opStream {
	s.buf = append(s.buf, text...)
	s.buf = append(s.buf, 0)
	return s
}

// op appends an opcode byte.
func (s *opStream) op(code int) *opStream { return s.u8(code) }

// bytes terminates the stream with opcode 0.
func (s *opStream) bytes() []byte {
	return append(append([]byte{}, s.buf...), 0)
}

// npcEntry builds a minimal NPC definition file: opcode 2 (name),
// then the given extra opcodes.
func npcEntry(name string, extra ...func(*opStream)) []byte {
	s := &opStream{}
	s.op(2).str(name)
	for _, fn := range extra {
		fn(s)
	}
	return s.bytes()
}

func npcDesc(desc string) func(*opStream) {
	return func(s *opStream) { s.op(3).str(desc) }
}

func npcSize(size int) func(*opStream) {
	return func(s *opStream) { s.op(12).u8(size) }
}

func npcIdleSeq(id int) func(*opStream) {
	return func(s *opStream) { s.op(13).u16(id) }
}

func npcAction(slot int, action string) func(*opStream) {
	return func(s *opStream) { s.op(30 + slot).str(action) }
}

func npcLevel(opcode, value int) func(*opStream) {
	return func(s *opStream) { s.op(opcode).u16(value) }
}

func npcCombatLevel(level int) func(*opStream) {
	return func(s *opStream) { s.op(95).u16(level) }
}

func npcSpawnDirection(dir int) func(*opStream) {
	return func(s *opStream) { s.op(125).u8(dir & 0xff) }
}

// objEntry builds a minimal item definition file: opcode 2 (name),
// then the given extra opcodes.
func objEntry(name string, extra ...func(*opStream)) []byte {
	s := &opStream{}
	s.op(2).str(name)
	for _, fn := range extra {
		fn(s)
	}
	return s.bytes()
}

func objExamine(text string) func(*opStream) {
	return func(s *opStream) { s.op(3).str(text) }
}

func objStackable() func(*opStream) {
	return func(s *opStream) { s.op(11) }
}

func objPrice(price int) func(*opStream) {
	return func(s *opStream) { s.op(12).i32(price) }
}

func objWearPos(slot int) func(*opStream) {
	return func(s *opStream) { s.op(13).u8(slot) }
}

func objWearPos2(slot int) func(*opStream) {
	return func(s *opStream) { s.op(14).u8(slot) }
}

func objTradable() func(*opStream) {
	return func(s *opStream) { s.op(65) }
}

func objWeight(weight int) func(*opStream) {
	return func(s *opStream) { s.op(75).short(weight) }
}

func objNote(noteID int) func(*opStream) {
	return func(s *opStream) { s.op(97).u16(noteID) }
}

func objNoteTemplate(template int) func(*opStream) {
	return func(s *opStream) { s.op(98).u16(template) }
}

func objPlaceholder(placeholderID int) func(*opStream) {
	return func(s *opStream) { s.op(148).u16(placeholderID) }
}

func objPlaceholderTemplate(template int) func(*opStream) {
	return func(s *opStream) { s.op(149).u16(template) }
}

func objInventoryAction(slot int, action string) func(*opStream) {
	return func(s *opStream) { s.op(35 + slot).str(action) }
}

// objParam adds one opcode-249 params block with a single integer
// param.
func objParams(params ...[2]int) func(*opStream) {
	return func(s *opStream) {
		s.op(249).u8(len(params))
		for _, p := range params {
			s.u8(0)
			s.buf = append(s.buf, byte(p[0]>>16), byte(p[0]>>8), byte(p[0]))
			s.i32(p[1])
		}
	}
}

// locEntry builds a minimal object definition file: opcode 2
// (name), then the given extra opcodes.
func locEntry(name string, extra ...func(*opStream)) []byte {
	s := &opStream{}
	s.op(2).str(name)
	for _, fn := range extra {
		fn(s)
	}
	return s.bytes()
}

func locDesc(desc string) func(*opStream) {
	return func(s *opStream) { s.op(3).str(desc) }
}

func locSize(x, y int) func(*opStream) {
	return func(s *opStream) { s.op(14).u8(x).op(15).u8(y) }
}

func locAction(slot int, action string) func(*opStream) {
	return func(s *opStream) { s.op(30 + slot).str(action) }
}

func locClipTypeNone() func(*opStream) {
	return func(s *opStream) { s.op(17) }
}

func locClipTypeDecoration() func(*opStream) {
	return func(s *opStream) { s.op(27) }
}

func locInteractive(v int) func(*opStream) {
	return func(s *opStream) { s.op(19).u8(v) }
}

func locMapFunction(id int) func(*opStream) {
	return func(s *opStream) { s.op(60).u16(id) }
}

func locClipMask(mask int) func(*opStream) {
	return func(s *opStream) { s.op(69).u8(mask) }
}

// locTransforms adds an opcode-77 transform block: varbit, varp,
// and the transform ids (the last entry is the default; -1 is
// encoded as 65535). The decoder appends a trailing -1 of its own.
func locTransforms(varbit, varp int, ids ...int) func(*opStream) {
	return func(s *opStream) {
		s.op(77)
		if varbit < 0 {
			s.u16(65535)
		} else {
			s.u16(varbit)
		}
		if varp < 0 {
			s.u16(65535)
		} else {
			s.u16(varp)
		}
		s.u8(len(ids) - 1)
		for _, id := range ids {
			if id < 0 {
				s.u16(65535)
			} else {
				s.u16(id)
			}
		}
	}
}

// buildArchive packs files into a *cache.Archive: a single file is
// stored raw; multiple files use one chunk with the delta-encoded
// int32 size trailer and a trailing chunk-count byte
// (DecodeArchive's multi-file layout).
func buildArchive(t *testing.T, id int, files map[int][]byte) *cache.Archive {
	t.Helper()
	ids := make([]int, 0, len(files))
	for fileID := range files {
		ids = append(ids, fileID)
	}
	sort.Ints(ids)
	lastFileID := 0
	if len(ids) > 0 {
		lastFileID = ids[len(ids)-1]
	}
	var data []byte
	if len(ids) == 1 {
		data = files[ids[0]]
	} else {
		var payload []byte
		for _, fileID := range ids {
			payload = append(payload, files[fileID]...)
		}
		data = append(data, payload...)
		prev := 0
		for _, fileID := range ids {
			size := len(files[fileID])
			var b [4]byte
			binary.BigEndian.PutUint32(b[:], uint32(int32(size-prev)))
			data = append(data, b[:]...)
			prev = size
		}
		data = append(data, 1)
	}
	archive, err := cache.DecodeArchive(id, lastFileID, len(ids), ids, nil, data)
	if err != nil {
		t.Fatalf("DecodeArchive(%d): %v", id, err)
	}
	return archive
}

type fakeArchiveSource struct {
	archives map[int]*cache.Archive
}

func (f *fakeArchiveSource) GetArchive(archiveID int, _ *[4]int32) (*cache.Archive, error) {
	archive, ok := f.archives[archiveID]
	if !ok {
		return nil, nil
	}
	return archive, nil
}

func (f *fakeArchiveSource) ArchiveCount() int { return len(f.archives) }

func (f *fakeArchiveSource) LastArchiveID() int {
	max := -1
	for id := range f.archives {
		if id > max {
			max = id
		}
	}
	return max
}

func (f *fakeArchiveSource) FileCount(archiveID int) int {
	if archive, ok := f.archives[archiveID]; ok {
		return archive.FileCount
	}
	return 0
}

// fakeCacheSpec describes a fake cache: definition files by id
// per kind, plus gameval sequence names.
type fakeCacheSpec struct {
	npcs     map[int][]byte
	locs     map[int][]byte
	objs     map[int][]byte
	seqNames map[int]string
}

// newFakeCache builds a CacheDefinitions + Gamevals pair over the
// spec. Kinds with no files get a single default entry so the
// facade's required-archive checks pass.
func newFakeCache(t *testing.T, spec fakeCacheSpec) (*cache.CacheDefinitions, *cache.Gamevals) {
	t.Helper()
	ensure := func(files map[int][]byte) map[int][]byte {
		if len(files) == 0 {
			return map[int][]byte{0: {0}}
		}
		return files
	}
	configs := &fakeArchiveSource{archives: map[int]*cache.Archive{
		cache.ConfigNPCs:      buildArchive(t, cache.ConfigNPCs, ensure(spec.npcs)),
		cache.ConfigLocs:      buildArchive(t, cache.ConfigLocs, ensure(spec.locs)),
		cache.ConfigObjs:      buildArchive(t, cache.ConfigObjs, ensure(spec.objs)),
		cache.ConfigSeqs:      buildArchive(t, cache.ConfigSeqs, map[int][]byte{0: {0}}),
		cache.ConfigVarbits:   buildArchive(t, cache.ConfigVarbits, map[int][]byte{0: {0}}),
		cache.ConfigSpotAnims: buildArchive(t, cache.ConfigSpotAnims, map[int][]byte{0: {0}}),
		cache.ConfigIdentkits: buildArchive(t, cache.ConfigIdentkits, map[int][]byte{0: {0}}),
	}}
	seqFiles := make(map[int][]byte, len(spec.seqNames))
	for id, name := range spec.seqNames {
		seqFiles[id] = []byte(name)
	}
	if len(seqFiles) == 0 {
		// An archive needs a non-empty file; a blank name yields
		// no animation role.
		seqFiles[0] = []byte(" ")
	}
	gamevalsSource := &fakeArchiveSource{archives: map[int]*cache.Archive{
		int(cache.GamevalSeq): buildArchive(t, int(cache.GamevalSeq), seqFiles),
	}}
	factory := func(indexID int) (cache.ArchiveSource, error) {
		switch indexID {
		case cache.IndexConfigs:
			return configs, nil
		case cache.GamevalIndex:
			return gamevalsSource, nil
		default:
			return &fakeArchiveSource{archives: map[int]*cache.Archive{}}, nil
		}
	}
	info := cache.CacheInfo{Name: "test", Game: cache.GameOldschool, Environment: "live", Revision: 242}
	defs, err := cache.NewCacheDefinitions(info, factory)
	if err != nil {
		t.Fatalf("NewCacheDefinitions: %v", err)
	}
	return defs, cache.NewGamevals(gamevalsSource)
}
