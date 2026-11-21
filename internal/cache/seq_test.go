package cache

import (
	"testing"
)

func TestSeqDefaults(t *testing.T) {
	s := NewSeqType(1, testInfo(242))
	if s.FrameStep != -1 || s.ForcedPriority != 5 || s.LeftHandItem != -1 || s.RightHandItem != -1 ||
		s.MaxLoops != 99 || s.PrecedenceAnimating != -1 || s.Priority != -1 || s.ReplyMode != 2 ||
		s.CachedModelId != -1 || s.SkeletalId != -1 || s.CrossWorldSound || s.Stretches || s.Looping || s.Op14 {
		t.Errorf("defaults: %+v", s)
	}
	s.Post()
	if s.PrecedenceAnimating != 0 || s.Priority != 0 {
		t.Errorf("post without masks: %d %d", s.PrecedenceAnimating, s.Priority)
	}
	if s.IsSkeletalSeq() || s.GetSkeletalDuration() != 0 || s.IsSingleShot() {
		t.Error("skeletal/single-shot defaults")
	}
}

func TestSeqDecodeRev226(t *testing.T) {
	data := (&enc{}).
		u8(1).u16(2).u16(3).u16(4).u16(100).u16(101).u16(0).u16(1). // lengths, low ids, high ids
		u8(2).u16(7).
		u8(3).u8(2).u8(5).u8(6).
		u8(4).u8(5).u8(9).u8(6).u16(300).u8(7).u16(301).u8(8).u8(3).u8(9).u8(1).u8(10).u8(1).u8(11).u8(0).
		u8(12).u8(1).u16(50).u16(2).
		u8(13).i32(123456).                                                                              // cached model (rev226 opcode 13)
		u8(14).u16(2).u16(0).u16(900).u8(70).u8(2).u8(3).u8(4).u16(1).u16(901).u8(80).u8(1).u8(0).u8(0). // frame sounds with weight
		u8(15).u16(10).u16(25).                                                                          // skeletal range
		u8(16).i8(-5).                                                                                   // vertical offset (only reachable rev226)
		u8(17).u8(2).u8(0).u8(255).
		u8(18).str("discarded").
		u8(19).
		u8(100).u8(2).u16(1).u16(10).u16(2).u16(20).
		u8(0).done()
	s, err := DecodeSeqType(10, testInfo(242), data)
	if err != nil {
		t.Fatal(err)
	}
	if len(s.FrameIds) != 2 || s.FrameIds[0] != 100 || s.FrameIds[1] != 101+(1<<16) ||
		s.FrameLengths[0] != 3 || s.FrameLengths[1] != 4 {
		t.Errorf("frames = %v %v", s.FrameIds, s.FrameLengths)
	}
	if s.FrameStep != 7 || len(s.Masks) != 3 || s.Masks[0] != 5 || s.Masks[1] != 6 || s.Masks[2] != 9999999 {
		t.Errorf("step/masks = %d %v", s.FrameStep, s.Masks)
	}
	if !s.Stretches || s.ForcedPriority != 9 || s.LeftHandItem != 300 || s.RightHandItem != 301 ||
		s.MaxLoops != 3 || !s.Looping || s.PrecedenceAnimating != 1 || s.Priority != 1 || s.ReplyMode != 0 {
		t.Errorf("flags/priorities: %+v", s)
	}
	if len(s.ChatFrameIds) != 1 || s.ChatFrameIds[0] != 50+(2<<16) {
		t.Errorf("chat frames = %v", s.ChatFrameIds)
	}
	if s.CachedModelId != 123456 || s.SkeletalId != 123456 || !s.Op14 {
		t.Errorf("cached model = %d %d op14=%v", s.CachedModelId, s.SkeletalId, s.Op14)
	}
	fx := s.FrameSounds[0]
	if len(fx) != 1 || fx[0].ID != 900 || fx[0].Weight == nil || *fx[0].Weight != 70 ||
		fx[0].Loops != 2 || fx[0].Location != 3 || fx[0].Attenuation != 4 {
		t.Errorf("frame sounds[0] = %+v", fx)
	}
	if len(s.FrameSounds[1]) != 1 || s.FrameSounds[1][0].ID != 901 {
		t.Errorf("frame sounds[1] = %+v", s.FrameSounds[1])
	}
	if s.SkeletalStart != 10 || s.SkeletalEnd != 25 || !s.IsSkeletalSeq() || s.GetSkeletalDuration() != 15 {
		t.Errorf("skeletal: %+v", s)
	}
	if s.VerticalOffset != -5 {
		t.Errorf("vertical offset = %d", s.VerticalOffset)
	}
	if len(s.SkeletalMasks) != 256 || !s.SkeletalMasks[0] || !s.SkeletalMasks[255] || s.SkeletalMasks[1] {
		t.Error("skeletal masks")
	}
	if !s.CrossWorldSound {
		t.Error("cross-world sound flag")
	}
	if len(s.BlendTable) != 2 || s.BlendTable[0] != (SeqBlendEntry{Frame: 1, Interval: 10}) || s.BlendTable[1].Interval != 20 {
		t.Errorf("blend table = %v", s.BlendTable)
	}
	if !s.IsSingleShot() {
		t.Error("priority 1 should be single-shot")
	}
}

func TestSeqPostWithMasks(t *testing.T) {
	s, err := DecodeSeqType(1, testInfo(242), (&enc{}).u8(3).u8(1).u8(9).u8(0).done())
	if err != nil {
		t.Fatal(err)
	}
	if s.PrecedenceAnimating != 2 || s.Priority != 2 {
		t.Errorf("post with masks: %d %d", s.PrecedenceAnimating, s.Priority)
	}
	s2, err := DecodeSeqType(2, testInfo(242), (&enc{}).u8(17).u8(1).u8(3).u8(0).done())
	if err != nil {
		t.Fatal(err)
	}
	if s2.PrecedenceAnimating != 2 || s2.Priority != 2 {
		t.Errorf("post with skeletal masks: %d %d", s2.PrecedenceAnimating, s2.Priority)
	}
}

func TestSeqDecodeRev220(t *testing.T) {
	// Rev 220: opcode 13 = sound effects (mediums), 14 = cached
	// model, 15 = frame sounds WITHOUT weight, 16 = skeletal range.
	data := (&enc{}).
		u8(13).u8(2).medium(0x010203).medium(0x040506).
		u8(14).i32(777).
		u8(15).u16(1).u16(2).u16(800).u8(1).u8(2).u8(3).
		u8(16).u16(5).u16(9).
		u8(0).done()
	s, err := DecodeSeqType(1, testInfo(220), data)
	if err != nil {
		t.Fatal(err)
	}
	if len(s.SoundEffects) != 2 || s.SoundEffects[0] != 0x010203 || s.SoundEffects[1] != 0x040506 {
		t.Errorf("sound effects = %v", s.SoundEffects)
	}
	if s.CachedModelId != 777 || !s.Op14 {
		t.Errorf("cached model = %d op14=%v", s.CachedModelId, s.Op14)
	}
	fx := s.FrameSounds[2]
	if len(fx) != 1 || fx[0].ID != 800 || fx[0].Weight != nil || fx[0].Loops != 1 || fx[0].Location != 2 || fx[0].Attenuation != 3 {
		t.Errorf("rev220 frame sound = %+v (weight must be nil)", fx)
	}
	if s.SkeletalStart != 5 || s.SkeletalEnd != 9 || s.VerticalOffset != 0 {
		t.Errorf("skeletal range/offset: %+v", s)
	}
}

func TestSeqDecodePre220FrameSounds(t *testing.T) {
	// Pre-220: frame sounds pack id/loops/location into a medium:
	// location = val&15, id = val>>8, loops = (val>>4)&7.
	// val = (800<<8) | (3<<4) | 5.
	val := (800 << 8) | (3 << 4) | 5
	data := (&enc{}).u8(15).u16(1).u16(0).medium(val).u8(0).done()
	s, err := DecodeSeqType(1, testInfo(210), data)
	if err != nil {
		t.Fatal(err)
	}
	fx := s.FrameSounds[0]
	if len(fx) != 1 || fx[0].ID != 800 || fx[0].Loops != 3 || fx[0].Location != 5 || fx[0].Attenuation != 0 || fx[0].Weight != nil {
		t.Errorf("pre-220 frame sound = %+v", fx)
	}
}

func TestSeqUnknownOpcode(t *testing.T) {
	if _, err := DecodeSeqType(1, testInfo(242), []byte{20, 0}); err == nil {
		t.Error("opcode 20 decoded, want error")
	}
	if _, err := DecodeSeqType(1, testInfo(242), []byte{101, 0}); err == nil {
		t.Error("opcode 101 decoded, want error")
	}
}
