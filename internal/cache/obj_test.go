package cache

import (
	"strings"
	"testing"
)

func TestObjDefaults(t *testing.T) {
	o := NewObjType(7, testInfo(242))
	if o.ID() != 7 || o.Name != "null" || o.Examine != nil || o.Model != 0 {
		t.Errorf("identity: id=%d name=%q examine=%v model=%d", o.ID(), o.Name, o.Examine, o.Model)
	}
	if o.Zoom2d != 2000 || o.Xan2d != 0 || o.Yan2d != 0 || o.Zan2d != 0 || o.OffsetX2d != 0 || o.OffsetY2d != 0 {
		t.Errorf("2d: %+v", o)
	}
	if o.Stackability != ObjStackabilitySometimes || o.Price != 1 || o.WearPos != -1 || o.WearPos2 != -1 || o.IsMembers {
		t.Errorf("stack/price/wear: %+v", o)
	}
	wantGround := []string{"", "", "Take", "", ""}
	wantInv := []string{"", "", "", "", "Drop"}
	if strings.Join(o.GroundActions, "|") != strings.Join(wantGround, "|") {
		t.Errorf("ground actions = %v", o.GroundActions)
	}
	if strings.Join(o.InventoryActions, "|") != strings.Join(wantInv, "|") {
		t.Errorf("inventory actions = %v", o.InventoryActions)
	}
	if o.Subops != nil || o.ShiftClickIndex != -2 {
		t.Errorf("subops=%v shift=%d", o.Subops, o.ShiftClickIndex)
	}
	if o.MaleModel != -1 || o.MaleModel1 != -1 || o.FemaleModel != -1 || o.FemaleModel1 != -1 ||
		o.MaleModel2 != -1 || o.FemaleModel2 != -1 || o.MaleHeadModel != -1 || o.MaleHeadModel2 != -1 ||
		o.FemaleHeadModel != -1 || o.FemaleHeadModel2 != -1 {
		t.Errorf("models: %+v", o)
	}
	if o.CountObj != nil || o.CountCo != nil || o.WearPos3 != -1 || o.Note != -1 || o.NoteTemplate != -1 {
		t.Errorf("counts/note: %+v", o)
	}
	if o.ResizeX != 128 || o.ResizeY != 128 || o.ResizeZ != 128 || o.Ambient != 0 || o.Contrast != 0 || o.Team != 0 {
		t.Errorf("resize/ambient: %+v", o)
	}
	if o.IsTradable || o.IsUnlockable || o.Weight != 0 || o.Category != -1 ||
		o.UnnotedID != -1 || o.NotedID != -1 || o.Placeholder != -1 || o.PlaceholderTemplate != -1 || o.Params != nil {
		t.Errorf("trade/weight/category/links: %+v", o)
	}
	if o.CacheType() != CacheTypeDat2 || o.CacheInfo().Revision != 242 {
		t.Errorf("cache info: %v %+v", o.CacheType(), o.CacheInfo())
	}
}

func TestObjDecodeFull(t *testing.T) {
	data := (&enc{}).
		u8(1).u16(1234). // model
		u8(2).str("Bronze sword").
		u8(3).str("A sharp sword.").
		u8(4).u16(1500).u8(5).u16(100).u8(6).u16(200).
		u8(7).u16(40000).u8(8).u16(300). // offsetX wraps: 40000-65536
		u8(9).str("op9text").
		u8(10).u16(999). // discarded
		u8(11).          // stackable always
		u8(12).i32(-500).
		u8(13).u8(3).u8(14).u8(4).
		u8(15). // explicitly not tradeable
		u8(16). // members
		u8(23).u16(2000).u8(7).u8(24).u16(2001).u8(25).u16(3000).u8(9).u8(26).u16(3001).u8(27).u8(5).
		u8(30).str("Take").u8(31).str("Hidden").u8(32).str("Examine").
		u8(35).str("Wield").u8(36).str("Use").
		u8(40).u8(2).u16(10).u16(20).u16(11).u16(21).
		u8(41).u8(1).u16(30).u16(40).
		u8(42).i8(-1).
		u8(43).u8(1).u8(1).str("Sub A").u8(3).str("Sub C").u8(0). // subops, terminator 0
		u8(44).i32(777777).
		u8(45).i32(800001).u8(2).u8(46).i32(800002).u8(47).i32(800003).
		u8(48).i32(900001).u8(3).u8(49).i32(900002).u8(50).i32(900003).
		u8(51).i32(910001).u8(52).i32(910002).u8(53).i32(920001).u8(54).i32(920002).
		u8(65).                                                                           // tradeable
		u8(69).u8(1).u8(68).u16(1).u8(66).u16(1).u8(88).u8(1).u8(74).u8(1).u8(99).u16(1). // placeholders
		u8(75).i16(-25).
		u8(78).u16(4000).u8(79).u16(4001).
		u8(90).u16(5000).u8(91).u16(5001).u8(92).u16(5002).u8(93).u16(5003).
		u8(94).u16(42).u8(95).u16(300).u8(96).u8(9).
		u8(97).u16(600).u8(98).u16(601).
		u8(100).u16(700).u16(1).u8(101).u16(701).u16(100).
		u8(110).u16(0).u8(111).u16(2000).u8(112).u16(64). // resize: 0 and 2000 clamp to 128
		u8(113).i8(-3).u8(114).i8(4).u8(115).u8(6).
		u8(121).u16(1).u8(122).u16(1).
		u8(125).i8(1).i8(2).i8(3).u8(126).i8(-1).i8(-2).i8(-3).
		u8(127).u8(1).u16(1).u8(128).u8(1).u16(1).u8(129).u8(1).u16(1).u8(130).u8(1).u16(1).
		u8(132).u8(2).u16(1).u16(2).
		u8(139).u16(800).u8(140).u16(801).
		u8(142).u16(1).u8(146).u16(1).
		u8(148).u16(900).u8(149).u16(901).
		u8(150).u16(1).u8(154).u16(1).
		u8(160). // stackable never (overrides opcode 11)
		u8(161).u16(2).u16(10).u16(20).
		u8(200).u8(1).u8(2).str("x").u8(201).u8(1).u16(1).u16(2).i32(1).i32(2).str("y").
		u8(202).u8(1).u16(1).u16(2).u16(3).i32(1).i32(2).str("z").
		u8(251). // unlockable
		u8(249).u8(2).u8(1).medium(596).str("spell").u8(0).medium(604).i32(33).
		u8(0).done()
	o, err := DecodeObjType(42, testInfo(242), data)
	if err != nil {
		t.Fatal(err)
	}
	if o.Model != 777777 { // opcode 44 overwrote opcode 1
		t.Errorf("Model = %d", o.Model)
	}
	if o.Name != "Bronze sword" || o.Examine == nil || *o.Examine != "A sharp sword." {
		t.Errorf("name/examine = %q %v", o.Name, o.Examine)
	}
	if o.Zoom2d != 1500 || o.Xan2d != 100 || o.Yan2d != 200 || o.Zan2d != 300 {
		t.Errorf("2d = %d %d %d %d", o.Zoom2d, o.Xan2d, o.Yan2d, o.Zan2d)
	}
	if o.OffsetX2d != -25536 || o.OffsetY2d != 300 {
		t.Errorf("offsets = %d %d", o.OffsetX2d, o.OffsetY2d)
	}
	if o.Op9 != "op9text" {
		t.Errorf("Op9 = %q", o.Op9)
	}
	if o.Stackability != ObjStackabilityNever {
		t.Errorf("Stackability = %d", o.Stackability)
	}
	if o.Price != -500 || o.WearPos != 3 || o.WearPos2 != 4 || o.WearPos3 != 5 || !o.IsMembers || !o.IsTradable || !o.IsUnlockable {
		t.Errorf("price/wear/flags: %+v", o)
	}
	if o.MaleModel != 800001 || o.MaleOffset != 2 || o.MaleModel1 != 800002 || o.MaleModel2 != 4000 {
		t.Errorf("male models = %d %d %d %d", o.MaleModel, o.MaleOffset, o.MaleModel1, o.MaleModel2)
	}
	if o.FemaleModel != 900001 || o.FemaleOffset != 3 || o.FemaleModel1 != 900002 || o.FemaleModel2 != 4001 {
		t.Errorf("female models = %d %d %d %d", o.FemaleModel, o.FemaleOffset, o.FemaleModel1, o.FemaleModel2)
	}
	if o.MaleHeadModel != 5000 || o.MaleHeadModel2 != 5002 || o.FemaleHeadModel != 5001 || o.FemaleHeadModel2 != 5003 {
		t.Errorf("head models: %+v", o)
	}
	if strings.Join(o.GroundActions, "|") != "Take||Examine||" {
		t.Errorf("ground actions = %v (Hidden must clear)", o.GroundActions)
	}
	if o.InventoryActions[0] != "Wield" || o.InventoryActions[1] != "Use" || o.InventoryActions[4] != "Drop" {
		t.Errorf("inventory actions = %v", o.InventoryActions)
	}
	if len(o.RecolorFrom) != 2 || o.RecolorFrom[1] != 11 || o.RecolorTo[1] != 21 {
		t.Errorf("recolor = %v -> %v", o.RecolorFrom, o.RecolorTo)
	}
	if len(o.RetextureFrom) != 1 || o.RetextureTo[0] != 40 {
		t.Errorf("retexture = %v -> %v", o.RetextureFrom, o.RetextureTo)
	}
	if !o.HasRecolor() || !o.HasRetexture() {
		t.Error("HasRecolor/HasRetexture false")
	}
	if o.ShiftClickIndex != -1 {
		t.Errorf("ShiftClickIndex = %d", o.ShiftClickIndex)
	}
	if len(o.Subops) != 5 || o.Subops[1] == nil || len(o.Subops[1]) != 20 ||
		o.Subops[1][0] == nil || *o.Subops[1][0] != "Sub A" ||
		o.Subops[1][2] == nil || *o.Subops[1][2] != "Sub C" || o.Subops[1][1] != nil || o.Subops[0] != nil {
		t.Errorf("subops = %v", o.Subops)
	}
	if o.Weight != -25 || o.Category != 42 {
		t.Errorf("weight/category = %d %d", o.Weight, o.Category)
	}
	if o.Note != 600 || o.NoteTemplate != 601 || o.UnnotedID != 800 || o.NotedID != 801 ||
		o.Placeholder != 900 || o.PlaceholderTemplate != 901 {
		t.Errorf("links: %+v", o)
	}
	if len(o.CountObj) != 10 || o.CountObj[0] != 700 || o.CountCo[0] != 1 || o.CountObj[1] != 701 || o.CountCo[1] != 100 {
		t.Errorf("countObj = %v %v", o.CountObj, o.CountCo)
	}
	if o.ResizeX != 128 || o.ResizeY != 128 || o.ResizeZ != 64 {
		t.Errorf("resize = %d %d %d", o.ResizeX, o.ResizeY, o.ResizeZ)
	}
	if o.Ambient != -3 || o.Contrast != 20 || o.Team != 6 {
		t.Errorf("ambient/contrast/team = %d %d %d", o.Ambient, o.Contrast, o.Team)
	}
	if o.ManwearXOff != 1 || o.ManwearYOff != 2 || o.ManwearZOff != 3 ||
		o.WomanwearXOff != -1 || o.WomanwearYOff != -2 || o.WomanwearZOff != -3 {
		t.Errorf("wear offsets: %+v", o)
	}
	if s, ok := ParamString(o.Params, 596); !ok || s != "spell" {
		t.Errorf("param 596 = %q %v", s, ok)
	}
	if n, ok := ParamInt(o.Params, 604); !ok || n != 33 {
		t.Errorf("param 604 = %d %v", n, ok)
	}
}

func TestObjRevertAction(t *testing.T) {
	// "Revert" in slot 4 with "Drop" present moves to the last spare slot.
	data := (&enc{}).u8(39).str("Revert").u8(0).done()
	o, err := DecodeObjType(1, testInfo(242), data)
	if err != nil {
		t.Fatal(err)
	}
	if o.InventoryActions[3] != "Revert" || o.InventoryActions[4] != "Drop" {
		t.Errorf("revert placement = %v", o.InventoryActions)
	}
	// With no spare slot it overwrites slot 4.
	data2 := (&enc{}).u8(35).str("A").u8(36).str("B").u8(37).str("C").u8(38).str("D").u8(39).str("Revert").u8(0).done()
	o2, err := DecodeObjType(2, testInfo(242), data2)
	if err != nil {
		t.Fatal(err)
	}
	if o2.InventoryActions[4] != "Revert" {
		t.Errorf("revert overwrite = %v", o2.InventoryActions)
	}
	// "Revert" when slot 4 is not Drop stays in place.
	data3 := (&enc{}).u8(39).str("Other").u8(39).str("Revert").u8(0).done()
	o3, err := DecodeObjType(3, testInfo(242), data3)
	if err != nil {
		t.Fatal(err)
	}
	if o3.InventoryActions[4] != "Revert" {
		t.Errorf("revert non-drop = %v", o3.InventoryActions)
	}
}

func TestObjLargeModelID(t *testing.T) {
	info := CacheInfo{Game: GameRunescape, Revision: 700}
	data := (&enc{}).u8(1).bigSmart(70000).u8(0).done()
	o, err := DecodeObjType(1, info, data)
	if err != nil {
		t.Fatal(err)
	}
	if o.Model != 70000 {
		t.Errorf("Model = %d, want big-smart 70000", o.Model)
	}
}

func TestObjUnknownOpcode(t *testing.T) {
	if _, err := DecodeObjType(1, testInfo(242), []byte{250, 0}); err == nil {
		t.Error("opcode 250 decoded, want error")
	}
}

func TestObjGenCertBoughtPlaceholder(t *testing.T) {
	info := testInfo(242)
	template := NewObjType(1, info)
	template.Model = 111
	template.Zoom2d = 900
	template.Name = "Template"
	template.RecolorFrom = []int{1}
	template.RecolorTo = []int{2}
	original := NewObjType(2, info)
	original.Name = "Logs"
	original.Price = 50
	original.IsMembers = true
	original.IsTradable = true

	cert := NewObjType(3, info)
	cert.GenCert(template, original)
	if cert.Model != 111 || cert.Zoom2d != 900 || cert.Name != "Logs" || cert.Price != 50 ||
		!cert.IsMembers || !cert.IsTradable || cert.Stackability != ObjStackabilityAlways || len(cert.RecolorFrom) != 1 {
		t.Errorf("GenCert = %+v", cert)
	}

	bought := NewObjType(4, info)
	original.InventoryActions = []string{"A", "B", "C", "D", "Drop"}
	original.Subops = [][]*string{{ptrStr("s")}, nil, nil, nil, nil}
	bought.GenBought(template, original)
	if bought.Model != 111 || bought.Name != "Logs" || bought.Price != 0 || bought.Stackability != ObjStackabilitySometimes {
		t.Errorf("GenBought = %+v", bought)
	}
	if strings.Join(bought.InventoryActions, "|") != "A|B|C|D|Discard" {
		t.Errorf("GenBought actions = %v", bought.InventoryActions)
	}
	if bought.Subops == nil || bought.Subops[0] == nil || *bought.Subops[0][0] != "s" || bought.Subops[4] != nil {
		t.Errorf("GenBought subops = %v", bought.Subops)
	}

	placeholder := NewObjType(5, info)
	template.Stackability = ObjStackabilityAlways
	placeholder.GenPlaceholder(template, original)
	if placeholder.Name != "Logs" || placeholder.Price != 0 || placeholder.IsMembers || placeholder.IsTradable ||
		placeholder.Stackability != ObjStackabilityAlways || placeholder.Model != 111 {
		t.Errorf("GenPlaceholder = %+v", placeholder)
	}
}

func ptrStr(s string) *string { return &s }

func TestObjGetCountObj(t *testing.T) {
	info := testInfo(242)
	base, err := DecodeObjType(10, info, (&enc{}).u8(100).u16(20).u16(1).u8(101).u16(30).u16(100).u8(0).done())
	if err != nil {
		t.Fatal(err)
	}
	loader := mapLoader[*ObjType]{
		types: map[int]*ObjType{20: NewObjType(20, info), 30: NewObjType(30, info)},
		def:   func(id int) *ObjType { return NewObjType(id, info) },
	}
	if got := base.GetCountObj(loader, 1); got != base {
		t.Error("count 1 should return self")
	}
	if got := base.GetCountObj(loader, 50); got.ID() != 20 {
		t.Errorf("count 50 -> id %d", got.ID())
	}
	if got := base.GetCountObj(loader, 150); got.ID() != 30 {
		t.Errorf("count 150 -> id %d", got.ID())
	}
	plain := NewObjType(11, info)
	if got := plain.GetCountObj(loader, 99); got != plain {
		t.Error("no count variants should return self")
	}
}

func TestObjGetShiftClickIndex(t *testing.T) {
	o := NewObjType(1, testInfo(242)) // default -2, slot 4 is Drop
	if got := o.GetShiftClickIndex(); got != 4 {
		t.Errorf("default shift-click = %d, want 4", got)
	}
	o.ShiftClickIndex = 1
	o.InventoryActions[1] = "Use"
	if got := o.GetShiftClickIndex(); got != 1 {
		t.Errorf("shift-click 1 = %d", got)
	}
	o.InventoryActions[1] = ""
	if got := o.GetShiftClickIndex(); got != -1 {
		t.Errorf("empty slot shift-click = %d, want -1", got)
	}
	o.ShiftClickIndex = -1
	if got := o.GetShiftClickIndex(); got != -1 {
		t.Errorf("shift-click -1 = %d", got)
	}
	o.ShiftClickIndex = 9 // out of range: no action
	if got := o.GetShiftClickIndex(); got != -1 {
		t.Errorf("shift-click 9 = %d, want -1", got)
	}
}

func TestPostProcessedObjLoader(t *testing.T) {
	info := testInfo(242)
	archive := buildTestArchive(t, 10, map[int][]byte{
		100: (&enc{}).u8(97).u16(102).u8(98).u16(101).u8(0).done(), // note=102, template=101
		101: (&enc{}).u8(1).u16(777).u8(4).u16(999).u8(0).done(),   // template
		102: (&enc{}).u8(2).str("Logs").u8(12).i32(50).u8(16).u8(65).u8(0).done(),
		200: (&enc{}).u8(97).u16(201).u8(98).u16(201).u8(0).done(), // self-referential cycle
		201: (&enc{}).u8(97).u16(200).u8(98).u16(200).u8(0).done(),
	}, nil)
	loader := NewPostProcessedObjTypeLoader(NewArchiveObjLoader(info, archive))
	cert := loader.Load(100)
	if cert.Name != "Logs" || cert.Model != 777 || cert.Zoom2d != 999 || cert.Price != 50 ||
		!cert.IsMembers || !cert.IsTradable || cert.Stackability != ObjStackabilityAlways {
		t.Errorf("post-processed cert = %+v", cert)
	}
	if loader.Load(100) != cert {
		t.Error("post-processed instance not cached")
	}
	if loader.Count() != 202 {
		t.Errorf("Count = %d", loader.Count())
	}
	// Cycles terminate (marker set before resolving dependencies).
	cyclic := loader.Load(200)
	if cyclic == nil {
		t.Error("cyclic load returned nil")
	}
	loader.ClearCache()
}
