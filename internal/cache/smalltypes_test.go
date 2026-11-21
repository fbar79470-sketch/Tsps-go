package cache

import (
	"testing"
)

func TestSpotAnimDefaults(t *testing.T) {
	s := NewSpotAnimType(1, testInfo(242))
	if s.ModelId != 0 || s.AnimationId != -1 || s.ResizeX != 128 || s.ResizeY != 128 || s.Rotation != 0 ||
		s.Ambient != 0 || s.Contrast != 0 || s.Rotate || s.RecolAll != -1 || s.DebugName != nil {
		t.Errorf("defaults: %+v", s)
	}
	// Recolor arrays start empty but non-nil.
	if s.RecolorFrom == nil || s.RecolorTo == nil || s.RetextureFrom == nil || s.RetextureTo == nil {
		t.Error("recolor arrays must be non-nil empty")
	}
}

func TestSpotAnimDecode(t *testing.T) {
	data := (&enc{}).
		u8(1).u16(500).
		u8(2).u16(600).
		u8(3).i32(70000). // overwrites model id
		u8(4).u16(200).u8(5).u16(150).u8(6).u16(90).u8(7).u8(3).u8(8).u8(4).
		u8(9).str("debug name").
		u8(10).
		u8(40).u8(1).u16(10).u16(20).
		u8(40).u8(1).u16(11).u16(21). // appends, unlike other types
		u8(41).u8(1).u16(30).u16(40).
		u8(42).u16(99).
		u8(0).done()
	s, err := DecodeSpotAnimType(7, testInfo(242), data)
	if err != nil {
		t.Fatal(err)
	}
	if s.ModelId != 70000 || s.AnimationId != 600 || s.ResizeX != 200 || s.ResizeY != 150 ||
		s.Rotation != 90 || s.Ambient != 3 || s.Contrast != 4 || !s.Rotate || s.RecolAll != 99 {
		t.Errorf("fields: %+v", s)
	}
	if s.DebugName == nil || *s.DebugName != "debug name" {
		t.Errorf("debug name = %v", s.DebugName)
	}
	if len(s.RecolorFrom) != 2 || s.RecolorFrom[0] != 10 || s.RecolorFrom[1] != 11 || s.RecolorTo[1] != 21 {
		t.Errorf("recolor append = %v -> %v", s.RecolorFrom, s.RecolorTo)
	}
	if len(s.RetextureFrom) != 1 || s.RetextureTo[0] != 40 {
		t.Errorf("retexture = %v -> %v", s.RetextureFrom, s.RetextureTo)
	}
}

func TestSpotAnimUnknownOpcode(t *testing.T) {
	if _, err := DecodeSpotAnimType(1, testInfo(242), []byte{11, 0}); err == nil {
		t.Error("opcode 11 decoded, want error")
	}
}

func TestVarBitDecode(t *testing.T) {
	v := NewVarBitType(1, testInfo(242))
	if v.Defined() || v.BaseVar != 0 || v.StartBit != 0 || v.EndBit != 0 {
		t.Errorf("unset varbit: %+v defined=%v", v, v.Defined())
	}
	decoded, err := DecodeVarBitType(2, testInfo(242), (&enc{}).u8(1).u16(100).u8(3).u8(7).u8(0).done())
	if err != nil {
		t.Fatal(err)
	}
	if !decoded.Defined() || decoded.BaseVar != 100 || decoded.StartBit != 3 || decoded.EndBit != 7 {
		t.Errorf("decoded varbit: %+v", decoded)
	}
	if _, err := DecodeVarBitType(3, testInfo(242), []byte{2, 0}); err == nil {
		t.Error("opcode 2 decoded, want error")
	}
	// Empty file: undefined, fields zero.
	empty, err := DecodeVarBitType(4, testInfo(242), []byte{0})
	if err != nil {
		t.Fatal(err)
	}
	if empty.Defined() {
		t.Error("empty varbit defined")
	}
}

func TestVarBitLoaders(t *testing.T) {
	info := testInfo(242)
	archive := buildTestArchive(t, 14, map[int][]byte{
		0: (&enc{}).u8(1).u16(1).u8(0).u8(1).u8(0).done(),
	}, nil)
	loader := NewArchiveVarBitLoader(info, archive)
	if !loader.Load(0).Defined() || loader.Load(1).Defined() {
		t.Error("archive varbit loader")
	}
	source := newFakeSource(map[int]*Archive{0: archive})
	indexLoader := NewIndexVarBitLoader(info, source)
	if indexLoader.Load(0).BaseVar != 1 {
		t.Error("index varbit loader")
	}
	dummy := NewDummyVarBitLoader(info)
	if dummy.Load(0).Defined() || dummy.Count() != 0 {
		t.Error("dummy varbit loader")
	}
}

func TestVarcIntDecode(t *testing.T) {
	v := NewVarcIntType(1, testInfo(242))
	if v.Persist {
		t.Error("default persist")
	}
	// Opcode 2 sets persist; all other opcodes are silently
	// ignored, with no payload consumed.
	decoded, err := DecodeVarcIntType(2, testInfo(242), []byte{5, 2, 99, 0})
	if err != nil {
		t.Fatal(err)
	}
	if !decoded.Persist {
		t.Error("persist not set")
	}
	empty := NewEmptyVarcIntLoader(testInfo(242))
	if empty.Load(0).Persist || empty.Count() != 0 {
		t.Error("empty varc loader")
	}
}

func TestIdkDecode(t *testing.T) {
	k := NewIdkType(1, testInfo(242))
	if k.BodyPartId != -1 || k.NonSelectable {
		t.Errorf("defaults: %+v", k)
	}
	data := (&enc{}).
		u8(1).u8(3).
		u8(2).u8(2).bytes(1, 2, 3, 4). // skip 4 bytes
		u8(3).
		u8(5).u8(1).bytes(1, 2, 3, 4). // skip 4 bytes
		u8(40).u8(1).bytes(1, 2, 3, 4).
		u8(41).u8(0).
		u8(60).bytes(1, 2).u8(69).bytes(1, 2).
		u8(70).bytes(1, 2, 3, 4).u8(79).bytes(1, 2, 3, 4).
		u8(0).done()
	decoded, err := DecodeIdkType(2, testInfo(242), data)
	if err != nil {
		t.Fatal(err)
	}
	if decoded.BodyPartId != 3 || !decoded.NonSelectable {
		t.Errorf("decoded: %+v", decoded)
	}
	if _, err := DecodeIdkType(3, testInfo(242), []byte{4, 0}); err == nil {
		t.Error("opcode 4 decoded, want error")
	}
	// Skipping past the end is a clean stop for dat2 caches.
	truncated, err := DecodeIdkType(4, testInfo(242), (&enc{}).u8(1).u8(2).u8(2).u8(10).bytes(1, 2).done())
	if err != nil {
		t.Errorf("truncated skip = %v, want nil", err)
	}
	if truncated.BodyPartId != 2 {
		t.Errorf("body part after truncated skip = %d", truncated.BodyPartId)
	}
	archive := buildTestArchive(t, 3, map[int][]byte{0: (&enc{}).u8(1).u8(7).u8(0).done()}, nil)
	if got := NewArchiveIdkLoader(testInfo(242), archive).Load(0); got.BodyPartId != 7 {
		t.Errorf("archive idk loader: %+v", got)
	}
}

func TestBasDefaultsAndDecode(t *testing.T) {
	b := NewBasType(1, testInfo(242))
	if b.IdleSeqId != -1 || b.WalkSeqId != -1 || b.RunSeqId != -1 || b.CrawlSeqId != -1 ||
		b.IdleLeftSeqId != -1 || b.WalkBackSeqId != -1 || b.ModelRotateTranslate != nil ||
		b.YawAcceleration != 0 || b.YawMaxSpeed != 0 {
		t.Errorf("defaults: %+v", b)
	}
	data := (&enc{}).
		u8(1).u16(100).u16(65535). // idle 100, walk -1
		u8(2).u16(101).u8(3).u16(102).u8(4).u16(103).u8(5).u16(104).
		u8(6).u16(105).u8(7).u16(106).u8(8).u16(107).u8(9).u16(108).
		u8(26).u8(1).u8(2).
		u8(27).u8(3).i16(1).i16(2).i16(3).i16(4).i16(5).i16(6).
		u8(27).u8(14).i16(7).i16(8).i16(9).i16(10).i16(11).i16(12). // grows past 12
		u8(29).u8(9).u8(30).u16(2047).
		u8(31).u8(1).u8(32).u16(1).u8(33).i16(1).u8(34).u8(1).u8(35).u16(1).u8(36).i16(1).u8(37).u8(1).
		u8(38).u16(200).u8(39).u16(201).u8(40).u16(202).u8(41).u16(203).u8(42).u16(204).
		u8(43).u16(1).u8(51).u16(1).
		u8(52).u8(1).u16(1).u8(2).
		u8(53).u8(54).u8(1).u8(2).u8(55).u8(1).u16(1).u8(56).u8(1).i16(1).i16(2).i16(3).
		u8(0).done()
	decoded, err := DecodeBasType(2, testInfo(242), data)
	if err != nil {
		t.Fatal(err)
	}
	if decoded.IdleSeqId != 100 || decoded.WalkSeqId != -1 || decoded.CrawlSeqId != 101 ||
		decoded.CrawlBackSeqId != 102 || decoded.CrawlLeftSeqId != 103 || decoded.CrawlRightSeqId != 104 ||
		decoded.RunSeqId != 105 || decoded.RunBackSeqId != 106 || decoded.RunLeftSeqId != 107 || decoded.RunRightSeqId != 108 {
		t.Errorf("seqs: %+v", decoded)
	}
	if len(decoded.ModelRotateTranslate) != 15 || decoded.ModelRotateTranslate[3][5] != 6 ||
		decoded.ModelRotateTranslate[14][0] != 7 || decoded.ModelRotateTranslate[0] != nil {
		t.Errorf("rotate/translate = %v", decoded.ModelRotateTranslate)
	}
	if decoded.YawAcceleration != 9 || decoded.YawMaxSpeed != 2047 {
		t.Errorf("yaw = %d %d", decoded.YawAcceleration, decoded.YawMaxSpeed)
	}
	if decoded.IdleLeftSeqId != 200 || decoded.IdleRightSeqId != 201 || decoded.WalkBackSeqId != 202 ||
		decoded.WalkLeftSeqId != 203 || decoded.WalkRightSeqId != 204 {
		t.Errorf("turn/walk: %+v", decoded)
	}
	if _, err := DecodeBasType(3, testInfo(242), []byte{10, 0}); err == nil {
		t.Error("opcode 10 decoded, want error")
	}
	archive := buildTestArchive(t, 32, map[int][]byte{0: (&enc{}).u8(1).u16(1).u16(2).u8(0).done()}, nil)
	if got := NewArchiveBasLoader(testInfo(242), archive).Load(0); got.IdleSeqId != 1 {
		t.Errorf("archive bas loader: %+v", got)
	}
	if NewDummyBasLoader(testInfo(242)).Load(0).IdleSeqId != -1 {
		t.Error("dummy bas loader")
	}
}

func TestDbRowDecode(t *testing.T) {
	// Opcode 3: column count byte, then per column: column id,
	// type count, unsigned-short-smart types, tuple count
	// (unsigned-short-smart), then tuples of (string, int) pairs.
	// Column 1: types [36 (string), 1 (int)], 2 tuples.
	// Column 2: types [42 (string)], 0 tuples -> empty key set.
	data := (&enc{}).
		u8(3).u8(2).
		u8(1).u8(2).smart(36).smart(1).smart(2).str("one").i32(11).str("two").i32(22).
		u8(2).u8(1).smart(42).smart(0).
		u8(255).
		u8(4).varInt2(300).
		u8(0).done()
	row, err := DecodeDbRow(5, data)
	if err != nil {
		t.Fatal(err)
	}
	if row.ID() != 5 || row.TableID != 300 {
		t.Errorf("id/table = %d %d", row.ID(), row.TableID)
	}
	col := row.Column(1)
	if len(col) != 4 || col[0] != "one" || col[1] != int32(11) || col[2] != "two" || col[3] != int32(22) {
		t.Errorf("column 1 = %v", col)
	}
	// Empty column still gets a key.
	if _, ok := row.Columns[2]; !ok {
		t.Error("column 2 key missing")
	}
	if len(row.Column(2)) != 0 {
		t.Errorf("column 2 = %v", row.Column(2))
	}
	if len(row.Column(99)) != 0 {
		t.Error("missing column not empty")
	}
	if row.String(1) != "one" || row.Int(1) != 0 {
		t.Errorf("accessors: string=%q int=%d", row.String(1), row.Int(1))
	}
	// Int on a numeric-first column.
	row2, err := DecodeDbRow(6, (&enc{}).u8(3).u8(1).u8(0).u8(1).smart(1).smart(1).i32(-9).u8(255).u8(0).done())
	if err != nil {
		t.Fatal(err)
	}
	if row2.Int(0) != -9 || row2.Int(9) != 0 || row2.Int(9, 42) != 42 || row2.String(0) != "" {
		t.Errorf("int accessors: %+v", row2.Columns)
	}
	// Table id without decoding columns.
	if id, err := ReadDbRowTableID(data); err != nil || id != 300 {
		t.Errorf("ReadDbRowTableID = %d, %v", id, err)
	}
	// Unknown opcode stops decoding silently, returning the row
	// decoded so far.
	row3, err := DecodeDbRow(7, (&enc{}).u8(4).varInt2(55).u8(99).u8(4).varInt2(66).u8(0).done())
	if err != nil {
		t.Fatal(err)
	}
	if row3.TableID != 55 {
		t.Errorf("unknown-opcode stop: table = %d", row3.TableID)
	}
	// Truncated data errors.
	if _, err := DecodeDbRow(8, []byte{3, 1, 1}); err == nil {
		t.Error("truncated row decoded, want error")
	}
	// No table id -> -1.
	row4, err := DecodeDbRow(9, []byte{0})
	if err != nil || row4.TableID != -1 {
		t.Errorf("empty row = %+v, %v", row4, err)
	}
}
