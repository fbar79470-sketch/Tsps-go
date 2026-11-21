package cache

import (
	"strings"
	"testing"
)

func TestNpcDefaults(t *testing.T) {
	n := NewNpcType(5, testInfo(242))
	if n.ID() != 5 || n.Name != "null" || n.Desc != nil || n.Size != 1 || n.Category != -1 {
		t.Errorf("identity: %+v", n)
	}
	if n.IdleSeqId != -1 || n.WalkSeqId != -1 || n.RunSeqId != -1 || n.WalkBackSeqId != -1 || n.WalkLeftSeqId != -1 ||
		n.WalkRightSeqId != -1 || n.TurnLeftSeqId != -1 || n.TurnRightSeqId != -1 || n.CrawlSeqId != -1 || n.CrawlBackSeqId != -1 ||
		n.CrawlLeftSeqId != -1 || n.CrawlRightSeqId != -1 || n.RunBackSeqId != -1 || n.RunLeftSeqId != -1 || n.RunRightSeqId != -1 {
		t.Errorf("seqs: %+v", n)
	}
	if strings.Join(n.Actions, "|") != "||||" || n.HeadIconSpriteIds != nil || n.HeadIconPrayer != -1 {
		t.Errorf("actions/headicons: %v %v %d", n.Actions, n.HeadIconSpriteIds, n.HeadIconPrayer)
	}
	if !n.DrawMapDot || !n.IsInteractable || !n.IsClickable || !n.IsClipped || n.IsFollower ||
		n.DrawPriority != NpcDrawPriorityDefault || n.IsVisible || n.IsPet {
		t.Errorf("render/interaction: %+v", n)
	}
	if n.CombatLevel != -1 || n.AttackLevel != -1 || n.AttackSpeed != -1 || n.WidthScale != 128 || n.HeightScale != 128 ||
		n.RotationSpeed != 32 || n.OverlapTintHsl != -1 || n.SpawnDirection != 6 || n.HeightOffset != -1 ||
		n.FootprintSize != -1 || n.BasTypeId != -1 || n.TransformVarbit != -1 || n.TransformVarp != -1 {
		t.Errorf("stats/scales: %+v", n)
	}
	n.Post()
	if n.FootprintSize != 51 { // floor(0.4 * 1 * 128)
		t.Errorf("footprint = %d, want 51", n.FootprintSize)
	}
	n2 := NewNpcType(6, testInfo(242))
	n2.Size = 2
	n2.Post()
	if n2.FootprintSize != 102 {
		t.Errorf("size-2 footprint = %d, want 102", n2.FootprintSize)
	}
}

func TestNpcDecodeFull(t *testing.T) {
	data := (&enc{}).
		u8(1).u8(2).u16(100).u16(101).
		u8(2).str("Goblin").u8(3).str("A goblin.").
		u8(12).u8(2).u8(13).u16(500).u8(14).u16(501).u8(15).u16(502).u8(16).u16(503).
		u8(17).u16(504).u16(505).u16(506).u16(507).
		u8(18).u16(7).
		u8(30).str("Attack").u8(31).str("Hidden").u8(32).str("Talk-to").
		u8(40).u8(1).u16(5).u16(6).u8(41).u8(1).u16(7).u16(8).
		u8(44).u16(1).u8(45).u16(2).
		u8(60).u8(2).u16(900).u16(901).
		u8(61).u8(1).i32(123456).u8(62).u8(1).i32(123457).
		u8(74).u16(10).u8(75).u16(65535).u8(76).u16(20).u8(77).u16(30).u8(78).u16(40).u8(79).u16(50).
		u8(93). // drawMapDot = false
		u8(95).u16(3).u8(97).u16(200).u8(98).u16(150).
		u8(99). // draw priority first
		u8(100).i8(-2).u8(101).i8(3).
		u8(102).u8(0b101).bigSmart(700).smartMin1(9).bigSmart(701).smartMin1(300). // bitmask: entries 0 and 2
		u8(103).u16(256).
		u8(106).u16(65535).u16(10).u8(1).u16(100).u16(101). // varbit -1, varp 10, transforms [100,101,-1]
		u8(107).u8(109).
		u8(111). // oldschool: draw priority last (overwrites opcode 99)
		u8(112).u8(113).u16(1).u16(2).
		u8(114).u16(603).u8(115).u16(610).u16(611).u16(612).u16(613).
		u8(116).u16(605).u8(117).u16(620).u16(621).u16(622).u16(623).
		u8(119).i8(1).
		u8(121).u8(1).u8(1).i8(1).i8(2).i8(3). // discarded offsets
		u8(122).u8(123).
		u8(124).u16(65535).u8(125).i8(-3).u8(126).u16(300).u8(127).u16(4).
		u8(128).u8(1).u8(130).
		u8(134).u16(1).u16(2).u16(3).u16(4).
		u8(135).u8(1).u16(1).u8(136).u8(1).u16(1).u8(137).u16(1).u8(138).u16(1).u8(139).u16(1).
		u8(140).u8(1).u8(141).u8(143).u8(142).u16(1).u8(144).u16(1).u8(145).u8(146).u16(42).u8(147).
		u8(150).str("MemberA").u8(154).str("MemberE").
		u8(155).i8(1).i8(2).i8(3).i8(4).u8(158).u8(159).u8(160).u8(1).u16(9).u8(161).u8(162).u8(163).u8(1).u8(164).u16(1).u16(2).u8(165).u8(1).u8(168).u8(1).
		u8(170).u16(1).u8(171).u16(1).u8(172).u16(1).u8(173).u16(1).u8(174).u16(1).u8(175).u16(1).
		u8(251).u8(1).u8(2).str("x").u8(252).u8(1).u16(1).u16(2).i32(1).i32(2).str("y").u8(253).u8(1).u16(1).u16(2).u16(3).i32(1).i32(2).str("z").
		u8(249).u8(1).u8(0).medium(1).i32(42).
		u8(0).done()
	n, err := DecodeNpcType(99, testInfo(242), data)
	if err != nil {
		t.Fatal(err)
	}
	// Opcode 61 overwrote the opcode-1 models with int32 models.
	if len(n.ModelIds) != 1 || n.ModelIds[0] != 123456 {
		t.Errorf("models = %v", n.ModelIds)
	}
	if len(n.ChatheadModelIds) != 1 || n.ChatheadModelIds[0] != 123457 {
		t.Errorf("chathead = %v", n.ChatheadModelIds)
	}
	if n.Name != "Goblin" || n.Desc == nil || *n.Desc != "A goblin." {
		t.Errorf("name/desc = %q %v", n.Name, n.Desc)
	}
	if n.Size != 2 || n.IdleSeqId != 500 || n.TurnLeftSeqId != 502 || n.TurnRightSeqId != 503 ||
		n.WalkSeqId != 504 || n.WalkBackSeqId != 505 || n.WalkLeftSeqId != 506 || n.WalkRightSeqId != 507 || n.Category != 7 {
		t.Errorf("size/seqs/category: %+v", n)
	}
	if strings.Join(n.Actions, "|") != "MemberA||Talk-to||MemberE" {
		t.Errorf("actions = %v (member ops overwrite slots 0 and 4)", n.Actions)
	}
	if len(n.RecolorFrom) != 1 || n.RecolorTo[0] != 6 || len(n.RetextureFrom) != 1 || n.RetextureTo[0] != 8 {
		t.Errorf("recolor/retexture: %v %v", n.RecolorFrom, n.RetextureTo)
	}
	if n.AttackLevel != 10 || n.DefenceLevel != -1 || n.StrengthLevel != 20 || n.Hitpoints != 30 ||
		n.RangedLevel != 40 || n.MagicLevel != 50 || n.CombatLevel != 3 {
		t.Errorf("stats: %+v", n)
	}
	if n.DrawMapDot || n.WidthScale != 200 || n.HeightScale != 150 {
		t.Errorf("mapdot/scale: %+v", n)
	}
	if n.DrawPriority != NpcDrawPriorityLast {
		t.Errorf("priority = %d, want last (opcode 111 after 99)", n.DrawPriority)
	}
	if n.Ambient != -2 || n.Contrast != 15 {
		t.Errorf("ambient/contrast = %d %d", n.Ambient, n.Contrast)
	}
	if len(n.HeadIconSpriteIds) != 2 || n.HeadIconSpriteIds[0] != 700 || n.HeadIconSpriteIds[1] != 701 ||
		len(n.HeadIconSpriteIndices) != 2 || n.HeadIconSpriteIndices[0] != 9 || n.HeadIconSpriteIndices[1] != 300 {
		t.Errorf("head icons = %v %v", n.HeadIconSpriteIds, n.HeadIconSpriteIndices)
	}
	if n.RotationSpeed != 256 {
		t.Errorf("rotation speed = %d", n.RotationSpeed)
	}
	if n.TransformVarbit != -1 || n.TransformVarp != 10 || len(n.Transforms) != 3 ||
		n.Transforms[0] != 100 || n.Transforms[1] != 101 || n.Transforms[2] != -1 {
		t.Errorf("transforms = %d %d %v", n.TransformVarbit, n.TransformVarp, n.Transforms)
	}
	if n.IsInteractable || n.IsClipped || !n.IsClickable || !n.IsFollower {
		t.Errorf("interaction flags: %+v", n)
	}
	if n.RunSeqId != 610 || n.RunBackSeqId != 611 || n.RunLeftSeqId != 612 || n.RunRightSeqId != 613 ||
		n.CrawlSeqId != 620 || n.CrawlBackSeqId != 621 || n.CrawlLeftSeqId != 622 || n.CrawlRightSeqId != 623 {
		t.Errorf("run/crawl: %+v", n)
	}
	if n.LoginScreenProps != 1 || n.HeightOffset != -1 || n.SpawnDirection != -3 || n.FootprintSize != 300 || n.BasTypeId != 4 {
		t.Errorf("login/height/dir/footprint/bas: %+v", n)
	}
	if !n.CanHideForOverlap || n.OverlapTintHsl != 42 {
		t.Errorf("overlap: %+v", n)
	}
	if n.Params == nil {
		t.Error("params missing")
	} else if v, ok := ParamInt(n.Params, 1); !ok || v != 42 {
		t.Errorf("param 1 = %d %v", v, ok)
	}
	n.Post() // explicit footprint preserved
	if n.FootprintSize != 300 {
		t.Errorf("footprint after Post = %d, want explicit 300", n.FootprintSize)
	}
}

func TestNpcOpcode102Legacy(t *testing.T) {
	// Rev 200 (oldschool < 210): opcode 102 reads a single u16
	// prayer icon id.
	data := (&enc{}).u8(102).u16(42).u8(0).done()
	n, err := DecodeNpcType(1, testInfo(200), data)
	if err != nil {
		t.Fatal(err)
	}
	if n.HeadIconPrayer != 42 || n.HeadIconSpriteIds != nil {
		t.Errorf("legacy head icon = %d %v", n.HeadIconPrayer, n.HeadIconSpriteIds)
	}
	// Rev 242: opcode 102 with mask 0 yields non-nil empty arrays.
	data2 := (&enc{}).u8(102).u8(0).u8(0).done()
	n2, err := DecodeNpcType(2, testInfo(242), data2)
	if err != nil {
		t.Fatal(err)
	}
	if n2.HeadIconSpriteIds == nil || len(n2.HeadIconSpriteIds) != 0 || n2.HeadIconSpriteIndices == nil {
		t.Errorf("mask-0 head icons = %v %v (want non-nil empty)", n2.HeadIconSpriteIds, n2.HeadIconSpriteIndices)
	}
}

func TestNpcOpcode16RunescapeOld(t *testing.T) {
	// Runescape rev < 254: opcode 16 consumes nothing; the next
	// opcode must still decode. Non-dat2 strings end with 0x0a.
	info := CacheInfo{Game: GameRunescape, Revision: 200}
	data := (&enc{}).u8(16).u8(2).bytes('N', 'a', 'm', 'e', 'd', 0x0a).u8(0).done()
	n, err := DecodeNpcType(1, info, data)
	if err != nil {
		t.Fatal(err)
	}
	if n.Name != "Named" || n.TurnRightSeqId != -1 {
		t.Errorf("name=%q turnRight=%d", n.Name, n.TurnRightSeqId)
	}
}

func TestNpcTransform(t *testing.T) {
	info := testInfo(242)
	// Opcode 118: varbit, varp, var3, count, count+1 shorts;
	// last entry is var3.
	base, err := DecodeNpcType(10, info, (&enc{}).
		u8(118).u16(5).u16(65535).u16(22).u8(1).u16(20).u16(21).u8(0).done())
	if err != nil {
		t.Fatal(err)
	}
	if len(base.Transforms) != 3 || base.Transforms[0] != 20 || base.Transforms[1] != 21 || base.Transforms[2] != 22 {
		t.Fatalf("transforms = %v", base.Transforms)
	}
	morph := NewNpcType(21, info)
	morph.Name = "Morphed"
	loader := mapLoader[*NpcType]{types: map[int]*NpcType{21: morph}, def: func(id int) *NpcType { return NewNpcType(id, info) }}
	vars := fakeVars{varbits: map[int]int{5: 1}}
	if got := base.Transform(vars, loader); got != morph {
		t.Errorf("varbit transform = %+v, want morphed", got)
	}
	// Out-of-range var value falls back to the last entry.
	vars.varbits[5] = 99
	if got := base.Transform(vars, loader); got == nil || got.ID() != 22 {
		t.Errorf("default transform = %v", got)
	}
	// A default of -1 yields nil.
	neg, err := DecodeNpcType(11, info, (&enc{}).
		u8(106).u16(5).u16(65535).u8(1).u16(20).u16(21).u8(0).done())
	if err != nil {
		t.Fatal(err)
	}
	if len(neg.Transforms) != 3 || neg.Transforms[2] != -1 {
		t.Fatalf("op106 transforms = %v", neg.Transforms)
	}
	vars.varbits[5] = -1
	if got := neg.Transform(vars, loader); got != nil {
		t.Errorf("-1 transform = %v, want nil", got)
	}
	// No transforms -> nil.
	if got := NewNpcType(1, info).Transform(vars, loader); got != nil {
		t.Error("transformless npc transformed")
	}
	// Varp-driven transform.
	varp, err := DecodeNpcType(12, info, (&enc{}).
		u8(106).u16(65535).u16(8).u8(0).u16(30).u8(0).done())
	if err != nil {
		t.Fatal(err)
	}
	vars2 := fakeVars{varps: map[int]int{8: 0}}
	if got := varp.Transform(vars2, loader); got == nil || got.ID() != 30 {
		t.Errorf("varp transform = %v", got)
	}
}

func TestNpcMovementSeqSet(t *testing.T) {
	info := testInfo(242)
	n := NewNpcType(1, info)
	n.IdleSeqId = 10
	n.WalkSeqId = 11
	n.TurnLeftSeqId = 12
	n.TurnRightSeqId = 13
	// Without a BAS type, the NPC's own sequences are used.
	set := n.GetMovementSeqSet(nil)
	if set.Idle != 10 || set.Walk != 11 || set.TurnLeft != 12 || set.TurnRight != 13 {
		t.Errorf("own set = %+v", set)
	}
	if n.GetIdleSeqId(nil) != 10 || n.GetWalkSeqId(nil) != 11 {
		t.Error("idle/walk getters without BAS")
	}
	// With a BAS type, BAS sequences win; turns come from the
	// BAS idle-left/right fields.
	n.BasTypeId = 0
	bas := NewBasType(0, info)
	bas.IdleSeqId = 20
	bas.WalkSeqId = 21
	bas.IdleLeftSeqId = 22
	bas.IdleRightSeqId = 23
	loader := mapLoader[*BasType]{types: map[int]*BasType{0: bas}, def: func(id int) *BasType { return NewBasType(id, info) }}
	set2 := n.GetMovementSeqSet(loader)
	if set2.Idle != 20 || set2.Walk != 21 || set2.TurnLeft != 22 || set2.TurnRight != 23 {
		t.Errorf("BAS set = %+v", set2)
	}
	if n.GetIdleSeqId(loader) != 20 || n.GetWalkSeqId(loader) != 21 {
		t.Error("idle/walk getters with BAS")
	}
}

func TestNpcUnknownOpcode(t *testing.T) {
	if _, err := DecodeNpcType(1, testInfo(242), []byte{200, 0}); err == nil {
		t.Error("opcode 200 decoded, want error")
	}
}
