package cache

import (
	"encoding/base64"
	"strings"
	"testing"
)

func archivedRow(kind ArchivedDefinitionKind, symbol string, sourceID, id int, data []byte) ArchivedDefinitionRow {
	return ArchivedDefinitionRow{
		Kind: kind, Symbol: symbol, SourceID: sourceID, ID: id,
		DataBase64: base64.StdEncoding.EncodeToString(data), CRC: CRC32(data),
	}
}

func archivedSourceInfo(revision int) CacheInfo {
	return CacheInfo{Name: "osrs-200_2020-06-01", Game: GameOldschool, Environment: "live", Revision: revision, Timestamp: "2020-06-01", Size: 123}
}

func archivedBundle(id string, rows ...ArchivedDefinitionRow) ArchivedDefinitionBundle {
	return ArchivedDefinitionBundle{
		ID:          id,
		TargetCache: ArchivedTargetCache{Name: "osrs-242_2026-11-03", Game: GameOldschool, Revision: 242},
		SourceCache: archivedSourceInfo(200),
		Definitions: rows,
	}
}

func testRegistry(t *testing.T) *ArchivedDefinitionRegistry {
	t.Helper()
	retired := (&enc{}).u8(2).str("null").u8(0).done()
	reg, err := NewArchivedDefinitionRegistry(ArchivedDefinitionContext{
		CacheInfo: testInfo(242),
		Counts:    ArchivedNativeCounts{NPC: 100, Loc: 100, Obj: 100, Seq: 50, SeqSupported: true},
		CurrentDefinition: func(kind ArchivedDefinitionKind, id int) (string, []byte, bool) {
			if kind == ArchivedKindObj && id == 5 {
				return "null", retired, true
			}
			return "", nil, false
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	return reg
}

func npcData(name string, extra func(*enc)) []byte {
	e := (&enc{}).u8(2).str(name)
	if extra != nil {
		extra(e)
	}
	return e.u8(0).done()
}

func TestRegistryReplaceHappyPath(t *testing.T) {
	reg := testRegistry(t)
	// Seq archived from source 30 -> appended 50.
	seqRow := archivedRow(ArchivedKindSeq, "old_anim", 30, 50, (&enc{}).u8(1).u16(1).u16(2).u16(10).u16(0).u8(0).done())
	// NPC morph target: source 20 -> appended 101.
	targetRow := archivedRow(ArchivedKindNPC, "old_npc_b", 20, 101, npcData("Old B", nil))
	// NPC with idle seq 30 (rebound to 50), walk seq 7 (native,
	// untouched), and a transform naming source 20 (rebound to 101).
	morphRow := archivedRow(ArchivedKindNPC, "old_npc_a", 10, 100, npcData("Old A", func(e *enc) {
		e.u8(13).u16(30).u8(14).u16(7)
		e.u8(106).u16(5).u16(65535).u8(0).u16(20)
	}))
	// Loc with seqId 30 rebound to 50.
	locRow := archivedRow(ArchivedKindLoc, "old_loc", 60, 100, (&enc{}).u8(2).str("Old Loc").u8(24).u16(30).u8(0).done())
	// Obj chain: note/template/count targets in the same bundle.
	objRow := archivedRow(ArchivedKindObj, "old_item", 40, 100, (&enc{}).u8(2).str("Old Item").u8(97).u16(41).u8(98).u16(42).u8(100).u16(43).u16(5).u8(0).done())
	noteRow := archivedRow(ArchivedKindObj, "old_item_noted", 41, 101, npcData("Old Item (noted)", nil))
	tmplRow := archivedRow(ArchivedKindObj, "old_item_tmpl", 42, 102, npcData("Template", nil))
	countRow := archivedRow(ArchivedKindObj, "old_item_stack", 43, 103, npcData("Stacked", nil))
	err := reg.Replace([]ArchivedDefinitionBundle{archivedBundle("b1", seqRow, targetRow, morphRow, locRow, objRow, noteRow, tmplRow, countRow)})
	if err != nil {
		t.Fatal(err)
	}
	if got := reg.GetCount(ArchivedKindNPC); got != 102 {
		t.Errorf("npc count = %d, want 102", got)
	}
	if got := reg.GetCount(ArchivedKindSeq); got != 51 {
		t.Errorf("seq count = %d, want 51", got)
	}
	if got := reg.GetCount(ArchivedKindObj); got != 104 {
		t.Errorf("obj count = %d, want 104", got)
	}
	if id, ok := reg.Resolve("b1", ArchivedKindNPC, "old_npc_a"); !ok || id != 100 {
		t.Errorf("Resolve = %d %v", id, ok)
	}
	if _, ok := reg.Resolve("b1", ArchivedKindNPC, "nope"); ok {
		t.Error("unknown symbol resolved")
	}
	ids := reg.IDs(ArchivedKindNPC)
	if len(ids) != 2 || ids[0] != 101 || ids[1] != 100 {
		t.Errorf("IDs publication order = %v", ids)
	}
	// Transform rebinding: source 20 -> appended 101; -1 untouched.
	src, ok := reg.GetSource(ArchivedKindNPC, 100)
	if !ok {
		t.Fatal("archived npc source missing")
	}
	if src.References == nil || !src.References.HasTransforms || len(src.References.Transforms) != 2 ||
		src.References.Transforms[0] != 101 || src.References.Transforms[1] != -1 {
		t.Errorf("transform refs = %+v", src.References)
	}
	if src.References.SeqFields["idleSeqId"] != 50 {
		t.Errorf("seq fields = %v, want idleSeqId=50", src.References.SeqFields)
	}
	if _, ok := src.References.SeqFields["walkSeqId"]; ok {
		t.Error("native seq id 7 must not be rebound")
	}
	// Loc seq field rebound.
	locSrc, _ := reg.GetSource(ArchivedKindLoc, 100)
	if locSrc.References.SeqFields["seqId"] != 50 {
		t.Errorf("loc seq refs = %+v", locSrc.References)
	}
	// Obj link rebinding.
	objSrc, _ := reg.GetSource(ArchivedKindObj, 100)
	refs := objSrc.References
	if refs == nil || refs.Note == nil || *refs.Note != 101 || refs.NoteTemplate == nil || *refs.NoteTemplate != 102 {
		t.Errorf("obj link refs = %+v", refs)
	}
	if !refs.HasCountObj || refs.CountObj[0] != 103 || refs.CountObj[1] != 0 {
		t.Errorf("obj count refs = %v (only co>0 entries rebound)", refs.CountObj)
	}
	// The overlay loader decodes the rebound definition.
	loader := NewArchivedTypeLoader[*NpcType](mapLoader[*NpcType]{def: func(id int) *NpcType { return NewNpcType(id, testInfo(242)) }},
		NewNpcType,
		func(id int) (ArchivedTypeSource, bool) { return reg.GetSource(ArchivedKindNPC, id) },
		func() int { return reg.GetCount(ArchivedKindNPC) })
	loaded := loader.Load(100)
	if loaded.Name != "Old A" || loaded.IdleSeqId != 50 || loaded.WalkSeqId != 7 ||
		len(loaded.Transforms) != 2 || loaded.Transforms[0] != 101 {
		t.Errorf("loaded archived npc = %+v", loaded)
	}
	if loader.Load(100) != loaded {
		t.Error("archived load not cached")
	}
	if loader.Count() != 102 {
		t.Errorf("loader count = %d", loader.Count())
	}
	// Unregistered ids fall through to the base loader.
	if got := loader.Load(1); got.Name != "null" {
		t.Errorf("fallback load = %q", got.Name)
	}
}

func TestRegistryReplaceValidation(t *testing.T) {
	good := func() ArchivedDefinitionBundle {
		return archivedBundle("b1", archivedRow(ArchivedKindNPC, "sym", 10, 100, npcData("X", nil)))
	}
	cases := []struct {
		name   string
		mutate func(b *ArchivedDefinitionBundle)
	}{
		{"empty bundle id", func(b *ArchivedDefinitionBundle) { b.ID = "" }},
		{"bad bundle id", func(b *ArchivedDefinitionBundle) { b.ID = "Bad ID!" }},
		{"wrong target name", func(b *ArchivedDefinitionBundle) { b.TargetCache.Name = "other" }},
		{"wrong target revision", func(b *ArchivedDefinitionBundle) { b.TargetCache.Revision = 241 }},
		{"empty source name", func(b *ArchivedDefinitionBundle) { b.SourceCache.Name = "" }},
		{"wrong source game", func(b *ArchivedDefinitionBundle) { b.SourceCache.Game = GameRunescape }},
		{"zero source revision", func(b *ArchivedDefinitionBundle) { b.SourceCache.Revision = 0 }},
		{"source newer than target", func(b *ArchivedDefinitionBundle) { b.SourceCache.Revision = 243 }},
		{"bad timestamp", func(b *ArchivedDefinitionBundle) { b.SourceCache.Timestamp = "not-a-date" }},
		{"negative size", func(b *ArchivedDefinitionBundle) { b.SourceCache.Size = -1 }},
		{"no definitions", func(b *ArchivedDefinitionBundle) { b.Definitions = nil }},
		{"bad kind", func(b *ArchivedDefinitionBundle) { b.Definitions[0].Kind = "bogus" }},
		{"zero id", func(b *ArchivedDefinitionBundle) { b.Definitions[0].ID = 0 }},
		{"id too large", func(b *ArchivedDefinitionBundle) { b.Definitions[0].ID = 0xffff }},
		{"source id too large", func(b *ArchivedDefinitionBundle) { b.Definitions[0].SourceID = 0xffff }},
		{"bad symbol", func(b *ArchivedDefinitionBundle) { b.Definitions[0].Symbol = "has space" }},
		{"empty symbol", func(b *ArchivedDefinitionBundle) { b.Definitions[0].Symbol = "" }},
		{"bad base64", func(b *ArchivedDefinitionBundle) { b.Definitions[0].DataBase64 = "!!!" }},
		{"crc mismatch", func(b *ArchivedDefinitionBundle) { b.Definitions[0].CRC++ }},
		{"oversized payload", func(b *ArchivedDefinitionBundle) {
			b.Definitions[0].DataBase64 = strings.Repeat("AAAA", 22000)
		}},
		{"native collision", func(b *ArchivedDefinitionBundle) { b.Definitions[0].ID = 50 }},
		{"appended with expected crc", func(b *ArchivedDefinitionBundle) {
			crc := uint32(1)
			b.Definitions[0].ExpectedCurrentCRC = &crc
		}},
		{"unterminated payload", func(b *ArchivedDefinitionBundle) {
			data := (&enc{}).u8(2).str("X").done()
			b.Definitions[0].DataBase64 = base64.StdEncoding.EncodeToString(data)
			b.Definitions[0].CRC = CRC32(data)
		}},
		{"unknown opcode payload", func(b *ArchivedDefinitionBundle) {
			data := []byte{200, 0}
			b.Definitions[0].DataBase64 = base64.StdEncoding.EncodeToString(data)
			b.Definitions[0].CRC = CRC32(data)
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			reg := testRegistry(t)
			b := good()
			tc.mutate(&b)
			if err := reg.Replace([]ArchivedDefinitionBundle{b}); err == nil {
				t.Error("Replace accepted an invalid bundle")
			}
			if len(reg.IDs(ArchivedKindNPC)) != 0 {
				t.Error("failed Replace published state")
			}
		})
	}
}

func TestRegistryReplaceDuplicatesAndUnresolved(t *testing.T) {
	row := func(sym string, src, id int) ArchivedDefinitionRow {
		return archivedRow(ArchivedKindNPC, sym, src, id, npcData("X", nil))
	}
	// Duplicate bundle ids.
	reg := testRegistry(t)
	if err := reg.Replace([]ArchivedDefinitionBundle{archivedBundle("b1", row("a", 1, 100)), archivedBundle("b1", row("b", 2, 101))}); err == nil {
		t.Error("duplicate bundle ids accepted")
	}
	// Duplicate target id.
	if err := reg.Replace([]ArchivedDefinitionBundle{archivedBundle("b1", row("a", 1, 100), row("b", 2, 100))}); err == nil {
		t.Error("duplicate target accepted")
	}
	// Duplicate source id.
	if err := reg.Replace([]ArchivedDefinitionBundle{archivedBundle("b1", row("a", 1, 100), row("b", 1, 101))}); err == nil {
		t.Error("duplicate source accepted")
	}
	// Duplicate symbol.
	if err := reg.Replace([]ArchivedDefinitionBundle{archivedBundle("b1", row("a", 1, 100), row("a", 2, 101))}); err == nil {
		t.Error("duplicate symbol accepted")
	}
	// Unresolved transform reference.
	bad := archivedRow(ArchivedKindNPC, "a", 1, 100, npcData("X", func(e *enc) {
		e.u8(106).u16(5).u16(65535).u8(0).u16(999)
	}))
	if err := reg.Replace([]ArchivedDefinitionBundle{archivedBundle("b1", bad)}); err == nil {
		t.Error("unresolved reference accepted")
	}
	// Unresolved obj link.
	badObj := archivedRow(ArchivedKindObj, "o", 1, 100, (&enc{}).u8(97).u16(999).u8(0).done())
	if err := reg.Replace([]ArchivedDefinitionBundle{archivedBundle("b1", badObj)}); err == nil {
		t.Error("unresolved obj link accepted")
	}
}

func TestRegistryObjReplacement(t *testing.T) {
	reg := testRegistry(t)
	retired := (&enc{}).u8(2).str("null").u8(0).done()
	expected := CRC32(retired)
	newData := npcData("Restored", nil)
	row := archivedRow(ArchivedKindObj, "restored", 9, 5, newData)
	row.ExpectedCurrentCRC = &expected
	if err := reg.Replace([]ArchivedDefinitionBundle{archivedBundle("b1", row)}); err != nil {
		t.Fatal(err)
	}
	if got := reg.GetCount(ArchivedKindObj); got != 100 {
		t.Errorf("replacement count = %d, want native 100", got)
	}
	if _, ok := reg.GetSource(ArchivedKindObj, 5); !ok {
		t.Error("replacement source missing")
	}
	// Wrong expected CRC.
	reg2 := testRegistry(t)
	wrong := expected + 1
	row2 := archivedRow(ArchivedKindObj, "restored", 9, 5, newData)
	row2.ExpectedCurrentCRC = &wrong
	if err := reg2.Replace([]ArchivedDefinitionBundle{archivedBundle("b1", row2)}); err == nil {
		t.Error("wrong expected CRC accepted")
	}
	// Missing expected CRC.
	reg3 := testRegistry(t)
	row3 := archivedRow(ArchivedKindObj, "restored", 9, 5, newData)
	if err := reg3.Replace([]ArchivedDefinitionBundle{archivedBundle("b1", row3)}); err == nil {
		t.Error("replacement without expected CRC accepted")
	}
	// Non-retired native id (no current definition).
	reg4 := testRegistry(t)
	row4 := archivedRow(ArchivedKindObj, "restored", 9, 6, newData)
	row4.ExpectedCurrentCRC = &expected
	if err := reg4.Replace([]ArchivedDefinitionBundle{archivedBundle("b1", row4)}); err == nil {
		t.Error("replacement of missing native accepted")
	}
}

func TestRegistryAtomicityAndDeepCopy(t *testing.T) {
	reg := testRegistry(t)
	input := []ArchivedDefinitionBundle{archivedBundle("b1", archivedRow(ArchivedKindNPC, "a", 1, 100, npcData("First", nil)))}
	if err := reg.Replace(input); err != nil {
		t.Fatal(err)
	}
	// Mutating the caller's bundles afterwards cannot affect the view.
	input[0].Definitions[0].Symbol = "mutated"
	input[0].ID = "mutated"
	if _, ok := reg.Resolve("b1", ArchivedKindNPC, "a"); !ok {
		t.Error("input mutation leaked into the registry")
	}
	// A failing Replace leaves the previous view intact.
	bad := archivedBundle("b2", archivedRow(ArchivedKindNPC, "b", 2, 101, npcData("Second", nil)))
	bad.Definitions[0].CRC++
	if err := reg.Replace([]ArchivedDefinitionBundle{bad}); err == nil {
		t.Fatal("bad Replace accepted")
	}
	if ids := reg.IDs(ArchivedKindNPC); len(ids) != 1 || ids[0] != 100 {
		t.Errorf("view after failed Replace = %v", ids)
	}
	// GetBundles returns a deep copy.
	bundles := reg.GetBundles()
	bundles[0].Definitions[0].Symbol = "mutated"
	if _, ok := reg.Resolve("b1", ArchivedKindNPC, "a"); !ok {
		t.Error("GetBundles mutation leaked into the registry")
	}
	// Replace with an empty list clears the registry.
	if err := reg.Replace(nil); err != nil {
		t.Fatal(err)
	}
	if len(reg.IDs(ArchivedKindNPC)) != 0 || reg.GetCount(ArchivedKindNPC) != 100 {
		t.Error("empty Replace did not clear to native counts")
	}
}

func TestRegistrySeqSupport(t *testing.T) {
	reg, err := NewArchivedDefinitionRegistry(ArchivedDefinitionContext{
		CacheInfo: testInfo(242),
		Counts:    ArchivedNativeCounts{NPC: 100, Loc: 100, Obj: 100, SeqSupported: false},
	})
	if err != nil {
		t.Fatal(err)
	}
	if reg.GetCount(ArchivedKindSeq) != 0 {
		t.Errorf("unsupported seq count = %d", reg.GetCount(ArchivedKindSeq))
	}
	seqRow := archivedRow(ArchivedKindSeq, "s", 1, 10, (&enc{}).u8(0).done())
	if err := reg.Replace([]ArchivedDefinitionBundle{archivedBundle("b1", seqRow)}); err == nil {
		t.Error("seq row accepted without seq support")
	}
	if _, err := NewArchivedDefinitionRegistry(ArchivedDefinitionContext{Counts: ArchivedNativeCounts{NPC: -1}}); err == nil {
		t.Error("negative native count accepted")
	}
}

func TestDecodeArchivedTypeStrict(t *testing.T) {
	info := archivedSourceInfo(200)
	// Missing terminator.
	if _, err := DecodeArchivedType(NewNpcType, 100, ArchivedTypeSource{CacheInfo: info, DataBase64: base64.StdEncoding.EncodeToString([]byte{2, 'X', 0})}); err == nil {
		t.Error("unterminated payload decoded")
	}
	// Trailing data after the terminator.
	if _, err := DecodeArchivedType(NewNpcType, 100, ArchivedTypeSource{CacheInfo: info, DataBase64: base64.StdEncoding.EncodeToString([]byte{0, 1})}); err == nil {
		t.Error("trailing data decoded")
	}
	// Bad base64.
	if _, err := DecodeArchivedType(NewNpcType, 100, ArchivedTypeSource{CacheInfo: info, DataBase64: "!!!"}); err == nil {
		t.Error("bad base64 decoded")
	}
	// Reference length mismatch.
	data := npcData("X", func(e *enc) { e.u8(106).u16(5).u16(65535).u8(0).u16(20) })
	src := ArchivedTypeSource{CacheInfo: info, DataBase64: base64.StdEncoding.EncodeToString(data),
		References: &ArchivedReferences{HasTransforms: true, Transforms: []int{1, 2, 3}}}
	if _, err := DecodeArchivedType(NewNpcType, 100, src); err == nil {
		t.Error("mismatched transform refs applied")
	}
	// Obj refs on an NPC.
	src.References = &ArchivedReferences{Note: func() *int { v := 1; return &v }()}
	if _, err := DecodeArchivedType(NewNpcType, 100, src); err == nil {
		t.Error("obj refs applied to npc")
	}
	// Unknown seq field.
	src.References = &ArchivedReferences{SeqFields: map[string]int{"bogusField": 1}}
	if _, err := DecodeArchivedType(NewNpcType, 100, src); err == nil {
		t.Error("unknown seq field applied")
	}
	// Refs on a seq are rejected.
	seqSrc := ArchivedTypeSource{CacheInfo: info, DataBase64: base64.StdEncoding.EncodeToString([]byte{0}),
		References: &ArchivedReferences{SeqFields: map[string]int{"idleSeqId": 1}}}
	if _, err := DecodeArchivedType(NewSeqType, 50, seqSrc); err == nil {
		t.Error("refs applied to seq")
	}
	// Valid decode applies Post (footprint derived).
	ok, err := DecodeArchivedType(NewNpcType, 100, ArchivedTypeSource{CacheInfo: info, DataBase64: base64.StdEncoding.EncodeToString(npcData("X", nil))})
	if err != nil {
		t.Fatal(err)
	}
	if ok.FootprintSize != 51 || ok.CacheInfo().Revision != 200 {
		t.Errorf("archived decode: %+v", ok)
	}
}

func TestArchivedTypeLoaderSourceChange(t *testing.T) {
	base := mapLoader[*NpcType]{def: func(id int) *NpcType { return NewNpcType(id, testInfo(242)) }}
	current := ArchivedTypeSource{CacheInfo: archivedSourceInfo(200), DataBase64: base64.StdEncoding.EncodeToString(npcData("V1", nil))}
	loader := NewArchivedTypeLoader(base, NewNpcType,
		func(id int) (ArchivedTypeSource, bool) {
			if id == 100 {
				return current, true
			}
			return ArchivedTypeSource{}, false
		},
		func() int { return 101 })
	first := loader.Load(100)
	if first.Name != "V1" {
		t.Errorf("first = %q", first.Name)
	}
	// Same source -> same instance.
	if loader.Load(100) != first {
		t.Error("identical source re-decoded")
	}
	// Changed payload -> re-decode.
	current.DataBase64 = base64.StdEncoding.EncodeToString(npcData("V2", nil))
	second := loader.Load(100)
	if second == first || second.Name != "V2" {
		t.Errorf("changed source = %+v", second)
	}
	// ClearArchivedCache forces a re-decode of the same source.
	loader.ClearArchivedCache()
	if loader.Load(100) == second {
		t.Error("ClearArchivedCache did not drop the entry")
	}
	// A registered source that fails to decode panics.
	current.DataBase64 = "!!!"
	defer func() {
		if recover() == nil {
			t.Error("bad archived source did not panic")
		}
	}()
	loader.ClearArchivedCache()
	_ = loader.Load(100)
}
