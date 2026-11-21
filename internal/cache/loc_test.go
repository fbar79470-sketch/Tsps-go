package cache

import (
	"strings"
	"testing"
)

func TestLocDefaults(t *testing.T) {
	l := NewLocType(3, testInfo(242))
	if l.ID() != 3 || l.Name != "null" || l.Category != -1 || l.Desc != nil || l.LowDetail {
		t.Errorf("identity: %+v", l)
	}
	if l.SizeX != 1 || l.SizeY != 1 || l.ClipType != 2 || l.ClipMask != 0 || !l.BlocksProjectile ||
		l.IsInteractive != -1 || l.ContouredGround != -1 || l.ContourGroundType != 0 || l.ContourGroundParam != -1 {
		t.Errorf("size/clip: %+v", l)
	}
	if l.SeqId != -1 || l.DecorDisplacement != DefaultDecorDisplacement || l.Ambient != 0 || l.Contrast != 0 ||
		len(l.Actions) != 5 || l.MapFunctionId != -1 || l.MapSceneId != -1 || l.FlipMapSceneSprite ||
		l.IsRotated || !l.Clipped || l.ModelSizeX != 128 || l.ModelSizeHeight != 128 || l.ModelSizeY != 128 {
		t.Errorf("model/flags: %+v", l)
	}
	if l.SupportItems != -1 || l.TransformVarbit != -1 || l.TransformVarp != -1 || l.AmbientSoundId != -1 ||
		l.SoundFadeInDuration != 300 || l.SoundFadeOutDuration != 300 || l.SoundAreaRadiusOverride != -1 || !l.SeqRandomStart {
		t.Errorf("support/transform/sound: %+v", l)
	}
	l.Post()
	if l.IsInteractive != 0 || l.SupportItems != 1 {
		t.Errorf("post defaults: interactive=%d support=%d", l.IsInteractive, l.SupportItems)
	}
}

func TestLocPostDerivations(t *testing.T) {
	// Models with a normal first type -> interactive.
	a := NewLocType(1, testInfo(242))
	a.Models = [][]int{{100}}
	a.Types = []int{int(LocModelNormal)}
	a.Post()
	if a.IsInteractive != 1 {
		t.Errorf("normal model interactive = %d", a.IsInteractive)
	}
	// Models with a wall first type -> not interactive.
	b := NewLocType(2, testInfo(242))
	b.Models = [][]int{{100}}
	b.Types = []int{int(LocModelWall)}
	b.Post()
	if b.IsInteractive != 0 {
		t.Errorf("wall model interactive = %d", b.IsInteractive)
	}
	// Any action -> interactive even without models.
	c := NewLocType(3, testInfo(242))
	c.Actions[2] = "Open"
	c.Post()
	if c.IsInteractive != 1 {
		t.Errorf("action interactive = %d", c.IsInteractive)
	}
	// Explicit value preserved.
	d := NewLocType(4, testInfo(242))
	d.IsInteractive = 0
	d.Actions[0] = "Open"
	d.Post()
	if d.IsInteractive != 0 {
		t.Error("explicit interactive overwritten")
	}
	// ClipType 0 -> no item support.
	e := NewLocType(5, testInfo(242))
	e.ClipType = 0
	e.Post()
	if e.SupportItems != 0 {
		t.Errorf("clip 0 support = %d", e.SupportItems)
	}
}

func TestLocDecodeFull(t *testing.T) {
	data := (&enc{}).
		u8(1).u8(2).u16(100).u8(10).u16(101).u8(0).
		u8(2).str("Bank booth").u8(3).str("A booth.").
		u8(14).u8(3).u8(15).u8(2).
		u8(17).u8(27).u8(18).
		u8(19).u8(1).
		u8(21).u8(22).u8(23).u8(24).u16(500).u8(25).u8(28).u8(5).u8(29).i8(-4).u8(39).i8(2).
		u8(30).str("Open").u8(31).str("Hidden").u8(34).str("Examine").
		u8(40).u8(1).u16(5).u16(6).u8(41).u8(1).u16(7).u16(8).
		u8(44).u16(1).u8(45).u16(2).
		u8(60).u16(700).u8(61).u16(9).u8(62).u8(64).
		u8(65).u16(200).u8(66).u16(150).u8(67).u16(100).u8(68).u16(701).u8(69).u8(3).
		u8(70).i16(-10).u8(71).i16(20).u8(72).i16(-30).u8(73).u8(74).u8(75).u8(0).
		u8(77).u16(5).u16(65535).u8(1).u16(200).u16(201).
		u8(78).u16(800).u8(12).u8(3). // rev>=220: min distance byte present
		u8(79).u16(10).u16(20).u8(14).u8(4).u8(2).u16(801).u16(802).
		u8(81).u8(2).
		u8(82).u16(702).u8(80).u8(1).u16(1).u16(2).u8(88).u8(89).u8(90).u8(91).u8(7).
		u8(93).u8(1).u16(100).u8(2).u16(200). // rev>=220 fade curves
		u8(94).u8(95).u8(9).u8(96).u8(9).
		u8(99).u8(1).u16(1).u8(100).u8(1).u8(2).str("x").u8(101).u8(1).u16(1).u16(2).i32(1).i32(2).str("y").u8(102).u8(1).u16(1).u16(2).u16(3).i32(1).i32(2).u8(103).u8(104).u8(1).
		u8(105). // no-op: FlipMapSceneSprite stays false
		u8(106).u8(2).u16(900).u8(5).u16(901).u8(6).
		u8(107).u16(703).
		u8(150).str("MemberA").u8(154).str("MemberE").
		u8(160).u8(1).u16(1).u8(163).i8(1).i8(2).i8(3).i8(4).u8(167).u16(1).u8(168).u8(169).u8(170).smart(5).u8(171).smart(200).u8(173).u16(1).u16(2).
		u8(177).u8(178).u8(9).u8(176).u8(189).u8(190).u8(1).u8(191).u8(0).
		u8(249).u8(1).u8(0).medium(1).i32(7).
		u8(0).done()
	l, err := DecodeLocType(50, testInfo(242), data)
	if err != nil {
		t.Fatal(err)
	}
	if len(l.Models) != 2 || l.Models[0][0] != 100 || l.Models[1][0] != 101 || len(l.Types) != 2 || l.Types[0] != 10 || l.Types[1] != 0 {
		t.Errorf("models/types = %v %v", l.Models, l.Types)
	}
	if l.Name != "Bank booth" || l.Desc == nil || *l.Desc != "A booth." {
		t.Errorf("name/desc = %q %v", l.Name, l.Desc)
	}
	if l.SizeX != 3 || l.SizeY != 2 || l.ClipType != 1 || l.BlocksProjectile || l.IsInteractive != 1 {
		t.Errorf("size/clip/interactive: %+v", l)
	}
	if l.ContouredGround != 512 || l.ContourGroundType != 4 || l.ContourGroundParam != 512 ||
		!l.MergeNormals || !l.ModelClipped || l.SeqId != 500 || l.DecorDisplacement != 5 || l.Ambient != -4 || l.Contrast != 50 {
		t.Errorf("contour/seq/decor/ambient: %+v", l)
	}
	if strings.Join(l.Actions, "|") != "MemberA||||MemberE" {
		t.Errorf("actions = %v", l.Actions)
	}
	if len(l.RecolorFrom) != 1 || l.RecolorTo[0] != 6 || len(l.RetextureFrom) != 1 || l.RetextureTo[0] != 8 {
		t.Errorf("recolor/retexture: %v %v", l.RecolorFrom, l.RetextureTo)
	}
	if l.MapFunctionId != 703 || l.Category != 9 || !l.IsRotated || l.Clipped ||
		l.ModelSizeX != 200 || l.ModelSizeHeight != 150 || l.ModelSizeY != 100 || l.MapSceneId != 701 || l.ClipMask != 3 {
		t.Errorf("map/category/sizes: %+v", l)
	}
	if l.OffsetX != -10 || l.OffsetHeight != 20 || l.OffsetY != -30 || !l.ObstructsGround || !l.IsHollow || l.SupportItems != 0 {
		t.Errorf("offsets/flags/support: %+v", l)
	}
	if l.TransformVarbit != 5 || l.TransformVarp != -1 || len(l.Transforms) != 3 ||
		l.Transforms[0] != 200 || l.Transforms[1] != 201 || l.Transforms[2] != -1 {
		t.Errorf("transforms = %d %d %v", l.TransformVarbit, l.TransformVarp, l.Transforms)
	}
	if l.AmbientSoundId != 800 || l.SoundMaxDistance != 14 || l.SoundMinDistance != 4 ||
		l.AmbientSoundChangeTicksMin != 10 || l.AmbientSoundChangeTicksMax != 20 ||
		len(l.AmbientSoundIds) != 2 || l.AmbientSoundIds[1] != 802 {
		t.Errorf("sounds: %+v", l)
	}
	if l.SeqRandomStart || !l.DeferredAmbientSwap || l.SoundDistanceFadeCurve != 7 ||
		l.SoundFadeInCurve != 1 || l.SoundFadeInDuration != 100 || l.SoundFadeOutCurve != 2 || l.SoundFadeOutDuration != 200 {
		t.Errorf("sound flags/fades: %+v", l)
	}
	if l.FlipMapSceneSprite {
		t.Error("opcode 105 must be a no-op")
	}
	if len(l.RandomSeqIds) != 2 || l.RandomSeqIds[0] != 900 || l.RandomSeqDelays[1] != 6 {
		t.Errorf("random seqs = %v %v", l.RandomSeqIds, l.RandomSeqDelays)
	}
	if !l.HasSpatialAmbient || !l.UseExactSoundPosition || !l.DeferredAmbientSwap || l.ResetAmbientOnLoopRestart {
		t.Errorf("spatial/exact/deferred/reset: %+v", l)
	}
	if l.SoundAreaRadiusOverride != -1 || l.LoopMultiSoundSequentially {
		t.Errorf("op177/178 must not set loop/radius fields: %+v", l)
	}
	if v, ok := ParamInt(l.Params, 1); !ok || v != 7 {
		t.Errorf("param 1 = %d %v", v, ok)
	}
}

func TestLocActionsGrowPastFive(t *testing.T) {
	// Opcodes 35..38 write sparse slots 5..8 (TS Array(5) grows).
	data := (&enc{}).u8(35).str("Sixth").u8(38).str("Ninth").u8(0).done()
	l, err := DecodeLocType(1, testInfo(242), data)
	if err != nil {
		t.Fatal(err)
	}
	if len(l.Actions) != 9 || l.Actions[5] != "Sixth" || l.Actions[8] != "Ninth" || l.Actions[6] != "" {
		t.Errorf("grown actions = %v", l.Actions)
	}
	// Post only consults the first five slots.
	if l.IsInteractive != 0 {
		t.Errorf("interactive = %d, want 0 (slots 5+ do not count)", l.IsInteractive)
	}
}

func TestLocOpcode1SkipAndLowDetail(t *testing.T) {
	// Second opcode 1 with models already set skips count*3 bytes.
	data := (&enc{}).
		u8(1).u8(1).u16(100).u8(10).
		u8(1).u8(1).u16(999).u8(9). // skipped
		u8(2).str("After").u8(0).done()
	l, err := DecodeLocType(1, testInfo(242), data)
	if err != nil {
		t.Fatal(err)
	}
	if l.Models[0][0] != 100 || l.Types[0] != 10 || l.Name != "After" {
		t.Errorf("skip: models=%v types=%v name=%q", l.Models, l.Types, l.Name)
	}
	// With LowDetail set, the second opcode 1 re-reads.
	l2 := NewLocType(2, testInfo(242))
	l2.LowDetail = true
	if err := l2.Decode(data); err != nil {
		t.Fatal(err)
	}
	if l2.Models[0][0] != 999 || l2.Types[0] != 9 {
		t.Errorf("low-detail re-read: models=%v types=%v", l2.Models, l2.Types)
	}
}

func TestLocOpcode5And6And7(t *testing.T) {
	// Opcode 5: single model list, types cleared.
	l, err := DecodeLocType(1, testInfo(242), (&enc{}).u8(5).u8(3).u16(1).u16(2).u16(3).u8(0).done())
	if err != nil {
		t.Fatal(err)
	}
	if len(l.Models) != 1 || len(l.Models[0]) != 3 || l.Models[0][2] != 3 || l.Types != nil {
		t.Errorf("op5: models=%v types=%v", l.Models, l.Types)
	}
	// Opcode 5 with models set skips count*2 bytes.
	l2, err := DecodeLocType(2, testInfo(242), (&enc{}).u8(1).u8(1).u16(50).u8(10).u8(5).u8(2).u16(1).u16(2).u8(0).done())
	if err != nil {
		t.Fatal(err)
	}
	if l2.Models[0][0] != 50 || l2.Types[0] != 10 {
		t.Errorf("op5 skip: models=%v types=%v", l2.Models, l2.Types)
	}
	// Opcode 6: int32 models with types.
	l3, err := DecodeLocType(3, testInfo(242), (&enc{}).u8(6).u8(1).i32(70000).u8(10).u8(0).done())
	if err != nil {
		t.Fatal(err)
	}
	if l3.Models[0][0] != 70000 || l3.Types[0] != 10 {
		t.Errorf("op6: models=%v types=%v", l3.Models, l3.Types)
	}
	// Opcode 7: int32 model list.
	l4, err := DecodeLocType(4, testInfo(242), (&enc{}).u8(7).u8(2).i32(70001).i32(70002).u8(0).done())
	if err != nil {
		t.Fatal(err)
	}
	if len(l4.Models[0]) != 2 || l4.Models[0][1] != 70002 || l4.Types != nil {
		t.Errorf("op7: models=%v types=%v", l4.Models, l4.Types)
	}
}

func TestLocNewModelsFormat(t *testing.T) {
	info := CacheInfo{Game: GameRunescape, Revision: 600}
	// Opcode 1: count, per entry (signed type, model count, models).
	data := (&enc{}).u8(1).u8(2).i8(10).u8(2).u16(100).u16(101).i8(0).u8(1).u16(102).u8(0).done()
	l, err := DecodeLocType(1, info, data)
	if err != nil {
		t.Fatal(err)
	}
	if len(l.Types) != 2 || l.Types[0] != 10 || len(l.Models[0]) != 2 || l.Models[1][0] != 102 {
		t.Errorf("new-format models = %v %v", l.Models, l.Types)
	}
	// Opcode 5: main block then a skipped block.
	data2 := (&enc{}).u8(5).u8(1).i8(10).u8(1).u16(200).u8(1).i8(0).u8(1).u16(999).u8(0).done()
	l2, err := DecodeLocType(2, info, data2)
	if err != nil {
		t.Fatal(err)
	}
	if l2.Models[0][0] != 200 {
		t.Errorf("op5 new-format models = %v", l2.Models)
	}
}

func TestLocTransformsRev242(t *testing.T) {
	// Opcode 97: signed-medium multiloc ids, no fallback.
	data := (&enc{}).u8(97).u16(5).u16(65535).u8(1).medium(0x010203).medium(0xFFFFFE).u8(0).done()
	l, err := DecodeLocType(1, testInfo(242), data)
	if err != nil {
		t.Fatal(err)
	}
	if l.TransformVarbit != 5 || len(l.Transforms) != 3 || l.Transforms[0] != 0x010203 ||
		l.Transforms[1] != -2 || l.Transforms[2] != -1 {
		t.Errorf("op97 transforms = %v", l.Transforms)
	}
	// Opcode 98 adds a signed-medium fallback read before the count.
	data2 := (&enc{}).u8(98).u16(65535).u16(8).medium(0xFFFFFF).u8(0).medium(300).u8(0).done()
	l2, err := DecodeLocType(2, testInfo(242), data2)
	if err != nil {
		t.Fatal(err)
	}
	if l2.TransformVarp != 8 || len(l2.Transforms) != 2 || l2.Transforms[0] != 300 || l2.Transforms[1] != -1 {
		t.Errorf("op98 transforms = %v", l2.Transforms)
	}
	// Opcode 92: var3 read before the count.
	data3 := (&enc{}).u8(92).u16(5).u16(65535).u16(77).u8(0).u16(55).u8(0).done()
	l3, err := DecodeLocType(3, testInfo(242), data3)
	if err != nil {
		t.Fatal(err)
	}
	if len(l3.Transforms) != 2 || l3.Transforms[0] != 55 || l3.Transforms[1] != 77 {
		t.Errorf("op92 transforms = %v", l3.Transforms)
	}
}

func TestLocSoundsPre220(t *testing.T) {
	// Rev < 220: opcode 78 has no min-distance byte; opcode 93
	// sets contour type 3 with a signed short param.
	data := (&enc{}).u8(78).u16(800).u8(12).u8(93).i16(-7).u8(0).done()
	l, err := DecodeLocType(1, testInfo(210), data)
	if err != nil {
		t.Fatal(err)
	}
	if l.AmbientSoundId != 800 || l.SoundMaxDistance != 12 || l.SoundMinDistance != 0 {
		t.Errorf("pre-220 sound: %+v", l)
	}
	if l.ContourGroundType != 3 || l.ContourGroundParam != -7 {
		t.Errorf("pre-220 op93: type=%d param=%d", l.ContourGroundType, l.ContourGroundParam)
	}
}

func TestLocTransform(t *testing.T) {
	info := testInfo(242)
	base, err := DecodeLocType(10, info, (&enc{}).u8(77).u16(5).u16(65535).u8(1).u16(65535).u16(21).u8(0).done())
	if err != nil {
		t.Fatal(err)
	}
	// Transforms = [-1, 21, -1]: in-range -1 falls through to the
	// default (last entry, also -1) -> nil, unlike NPC.
	loader := mapLoader[*LocType]{types: map[int]*LocType{21: NewLocType(21, info)}, def: func(id int) *LocType { return NewLocType(id, info) }}
	vars := fakeVars{varbits: map[int]int{5: 0}}
	if got := base.Transform(vars, loader); got != nil {
		t.Errorf("in-range -1 transform = %v, want nil (fallthrough to -1 default)", got)
	}
	vars.varbits[5] = 1
	if got := base.Transform(vars, loader); got == nil || got.ID() != 21 {
		t.Errorf("transform 1 = %v", got)
	}
	// Fallthrough to a real default.
	withDefault, err := DecodeLocType(11, info, (&enc{}).u8(92).u16(5).u16(65535).u16(30).u8(0).u16(65535).u8(0).done())
	if err != nil {
		t.Fatal(err)
	}
	vars.varbits[5] = 0
	if got := withDefault.Transform(vars, loader); got == nil || got.ID() != 30 {
		t.Errorf("fallthrough transform = %v, want 30", got)
	}
	if got := NewLocType(1, info).Transform(vars, loader); got != nil {
		t.Error("transformless loc transformed")
	}
}

func TestLocUnknownOpcode(t *testing.T) {
	if _, err := DecodeLocType(1, testInfo(242), []byte{200, 0}); err == nil {
		t.Error("opcode 200 decoded, want error")
	}
}
