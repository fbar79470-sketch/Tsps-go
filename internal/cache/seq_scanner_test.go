package cache

import (
	"testing"
)

// Hand-built fixture tests for the sequence scanner
// (NpcAnimationScanner.ts). The enc helper is in
// fixtures_test.go; all byte streams are hand-built.

func scannerWithFiles(t *testing.T, files map[int][]byte) *SequenceScanner {
	t.Helper()
	return NewSequenceScanner(242, func(id int) ([]byte, bool) {
		data, ok := files[id]
		return data, ok
	}, 100)
}

func TestScannerDefaults(t *testing.T) {
	s := scannerWithFiles(t, map[int][]byte{
		1: {0}, // empty file: opcode 0 at once
	})
	def := s.decodeSequence(1)
	if def.Priority != -1 || def.ForcedPriority != -1 || def.ClientCycles != 0 {
		t.Errorf("empty seq = %+v", def)
	}
	if got := s.Priority(1); got != -1 {
		t.Errorf("Priority = %d, want -1", got)
	}
	// ForcedPriority defaults to 5 when unset (attacks are 6,
	// blocks 5).
	if got := s.ForcedPriority(1); got != 5 {
		t.Errorf("ForcedPriority = %d, want 5", got)
	}
	if got := s.StallTicks(1); got != 0 {
		t.Errorf("StallTicks = %d, want 0", got)
	}
	// A missing file decodes to the same defaults.
	if got := s.Priority(99); got != -1 {
		t.Errorf("missing Priority = %d, want -1", got)
	}
	if got := s.LastSequenceID(); got != 100 {
		t.Errorf("LastSequenceID = %d, want 100", got)
	}
}

func TestScannerFramesPriorityStall(t *testing.T) {
	// Two frames of 30 and 45 client cycles, forced
	// priority 6 (attack), standing priority 0.
	data := (&enc{}).
		u8(1).u16(2).u16(30).u16(45).u16(100).u16(101).u16(0).u16(0).
		u8(5).u8(6).
		u8(10).u8(0).
		u8(0).done()
	s := scannerWithFiles(t, map[int][]byte{7: data})
	if got := s.Priority(7); got != 0 {
		t.Errorf("Priority = %d, want 0", got)
	}
	if got := s.ForcedPriority(7); got != 6 {
		t.Errorf("ForcedPriority = %d, want 6", got)
	}
	// ceil(75/30) = 3 ticks.
	if got := s.StallTicks(7); got != 3 {
		t.Errorf("StallTicks = %d, want 3", got)
	}
	// A second lookup reads the cached definition, not the
	// file: mutate the source and confirm the values hold.
	s.file = func(id int) ([]byte, bool) { return nil, false }
	if got := s.StallTicks(7); got != 3 {
		t.Errorf("cached StallTicks = %d, want 3", got)
	}
}

func TestScannerStallCeil(t *testing.T) {
	// Exactly 30 cycles -> 1 tick; 31 -> 2 ticks.
	one := (&enc{}).u8(1).u16(1).u16(30).u16(5).u16(0).u8(0).done()
	two := (&enc{}).u8(1).u16(1).u16(31).u16(5).u16(0).u8(0).done()
	s := scannerWithFiles(t, map[int][]byte{1: one, 2: two})
	if got := s.StallTicks(1); got != 1 {
		t.Errorf("30 cycles StallTicks = %d, want 1", got)
	}
	if got := s.StallTicks(2); got != 2 {
		t.Errorf("31 cycles StallTicks = %d, want 2", got)
	}
}

func TestScannerSkeletal(t *testing.T) {
	// Cached-model opcode 13 (rev >= 226) marks the
	// sequence skeletal (clientCycles = -1); the skeletal
	// range opcode 15 then sets it to end - start.
	data := (&enc{}).
		u8(13).i32(123456).
		u8(15).u16(10).u16(70).
		u8(0).done()
	s := scannerWithFiles(t, map[int][]byte{3: data})
	// ceil(60/30) = 2 ticks.
	if got := s.StallTicks(3); got != 2 {
		t.Errorf("skeletal StallTicks = %d, want 2", got)
	}
	// A skeletal sequence with no range opcode stays at
	// the -1 sentinel, clamped to 0 cycles.
	bare := (&enc{}).u8(13).i32(9).u8(0).done()
	s2 := scannerWithFiles(t, map[int][]byte{4: bare})
	if got := s2.StallTicks(4); got != 0 {
		t.Errorf("bare skeletal StallTicks = %d, want 0", got)
	}
}

func TestScannerUnknownOpcodeEndsWalk(t *testing.T) {
	// Opcode 200 is unknown: the walk ends there, so the
	// priority opcode after it is never read and the
	// forced priority before it survives.
	data := (&enc{}).
		u8(5).u8(6).
		u8(200).
		u8(10).u8(1).
		u8(0).done()
	s := scannerWithFiles(t, map[int][]byte{5: data})
	if got := s.ForcedPriority(5); got != 6 {
		t.Errorf("ForcedPriority = %d, want 6", got)
	}
	if got := s.Priority(5); got != -1 {
		t.Errorf("Priority = %d, want -1 (walk ended at 200)", got)
	}
}

func TestScannerSkipsKnownOpcodes(t *testing.T) {
	// Opcode 2 (frame step), 12 (chat frames), 18 (name)
	// and 100 (blend table) are skipped, not decoded, so
	// the priority opcode after them still reads.
	data := (&enc{}).
		u8(2).u16(7).
		u8(12).u8(1).u16(50).u16(2).
		u8(18).str("name").
		u8(100).u8(1).u16(1).u16(10).
		u8(10).u8(1).
		u8(0).done()
	s := scannerWithFiles(t, map[int][]byte{6: data})
	if got := s.Priority(6); got != 1 {
		t.Errorf("Priority = %d, want 1", got)
	}
}

func TestScannerRev220FrameSounds(t *testing.T) {
	// Rev 220: opcode 15 is frame sounds without weight
	// (stride 4, rev220 false) — the scanner skips it via
	// skipSequenceOpcode only on rev >= 226; below 226 the
	// opcode-15 skip returns false and the walk ends.
	data := (&enc{}).
		u8(15).u16(1).u16(2).u16(800).u8(1).u8(2).u8(3).
		u8(10).u8(2).
		u8(0).done()
	s := NewSequenceScanner(220, func(id int) ([]byte, bool) {
		if id == 8 {
			return data, true
		}
		return nil, false
	}, 100)
	if got := s.Priority(8); got != -1 {
		t.Errorf("rev220 Priority = %d, want -1 (walk ended at opcode 15)", got)
	}
}

func TestSequenceFrameIDs(t *testing.T) {
	data := (&enc{}).
		u8(1).u16(2).u16(3).u16(4).u16(100).u16(101).u16(0).u16(1).
		u8(0).done()
	got := SequenceFrameIDs(data, 242)
	if len(got) != 2 || got[0] != 100 || got[1] != 101+(1<<16) {
		t.Errorf("SequenceFrameIDs = %v", got)
	}
	// Opcode 0 with no frames yields no ids.
	if got := SequenceFrameIDs([]byte{0}, 242); len(got) != 0 {
		t.Errorf("empty SequenceFrameIDs = %v", got)
	}
	// An unknown opcode the skipper cannot pass yields no
	// ids.
	if got := SequenceFrameIDs([]byte{200, 0}, 242); len(got) != 0 {
		t.Errorf("unknown-opcode SequenceFrameIDs = %v", got)
	}
	// A skipped opcode before the frames still finds them.
	prefixed := (&enc{}).u8(2).u16(7).
		u8(1).u16(1).u16(9).u16(77).u16(0).
		u8(0).done()
	if got := SequenceFrameIDs(prefixed, 242); len(got) != 1 || got[0] != 77 {
		t.Errorf("prefixed SequenceFrameIDs = %v", got)
	}
}

func TestScannerNoCache(t *testing.T) {
	// A nil file source (no cache) decodes every id to
	// the defaults, as the TypeScript try/catch does.
	s := NewSequenceScanner(242, nil, -1)
	if got := s.Priority(422); got != -1 {
		t.Errorf("no-cache Priority = %d, want -1", got)
	}
	if got := s.ForcedPriority(422); got != 5 {
		t.Errorf("no-cache ForcedPriority = %d, want 5", got)
	}
	if got := s.StallTicks(422); got != 0 {
		t.Errorf("no-cache StallTicks = %d, want 0", got)
	}
	if got := s.LastSequenceID(); got != -1 {
		t.Errorf("no-cache LastSequenceID = %d, want -1", got)
	}
	if got := SequenceScannerForCache(nil).Priority(422); got != -1 {
		t.Errorf("nil-cache scanner Priority = %d, want -1", got)
	}
}

// TestScannerRealCache spot-checks the scanner against the
// real rev-242 cache (the seed cache the smoke harness
// uses), cross-checking each value against the full SeqType
// decoder: forced priority must match SeqType's, and the
// stall must be ceil(sum of frame lengths / 30) for
// non-skeletal sequences. It skips when the seed cache is
// absent (other checkouts, CI).
func TestScannerRealCache(t *testing.T) {
	const seedCache = "/home/hatch/smoke27/seed-root/caches/osrs-242_2026-11-03"
	store, err := OpenStore(seedCache)
	if err != nil {
		t.Skipf("seed cache absent (%v)", err)
	}
	defer store.Close()
	defs, err := NewCacheDefinitionsFromStore(testInfo(242), store)
	if err != nil {
		t.Fatal(err)
	}
	scanner := SequenceScannerForCache(defs)
	if scanner.LastSequenceID() <= 0 {
		t.Fatalf("LastSequenceID = %d, want > 0", scanner.LastSequenceID())
	}
	// Known animation ids: 422 unarmed punch, 424 block,
	// 836 death, 1658 whip attack, 808 stand, 819 walk.
	for _, id := range []int{422, 424, 836, 1658, 808, 819} {
		seq := defs.GetSeq(id)
		if seq == nil {
			t.Fatalf("GetSeq(%d) = nil", id)
		}
		if got, want := scanner.ForcedPriority(id), seq.ForcedPriority; got != want {
			t.Errorf("seq %d ForcedPriority = %d, SeqType = %d", id, got, want)
		}
		if seq.IsSkeletalSeq() {
			continue
		}
		cycles := 0
		for _, length := range seq.FrameLengths {
			cycles += length
		}
		wantStall := (cycles + 29) / 30
		if got := scanner.StallTicks(id); got != wantStall {
			t.Errorf("seq %d StallTicks = %d, want %d (cycles %d)", id, got, wantStall, cycles)
		}
		// The scanner's raw priority is -1 when unset, or
		// the opcode-10 value; SeqType.post derives -1 to
		// 0/2, so only compare when the scanner found one.
		if p := scanner.Priority(id); p != -1 && p != seq.Priority {
			t.Errorf("seq %d Priority = %d, SeqType = %d", id, p, seq.Priority)
		}
	}
}
