package cache

import "sync"

// The sequence (animation) scanner: the port of
// NpcAnimationScanner.ts
// (server/src/main/typescript/elvarg/game/cache/NpcAnimationScanner.ts).
// It decodes the seq config archive's three runtime fields — the
// standing priority (opcode 10), the interrupt priority (opcode 5)
// and the client-cycle length (opcode 1 frame lengths, or the
// skeletal frame range) — with its own opcode walk, exactly as
// TypeScript does, rather than reusing the decoded SeqType: the
// scanner deliberately bypasses SeqType.post, so an unset
// priority stays -1 instead of deriving to 0/2, and an unknown
// opcode ends the walk at the defaults instead of erroring
// (SeqType.DecodeOpcode errors on unknown opcodes).
//
// The scanner reads the NATIVE seq archive (configs.getArchive
// (ConfigType.DAT2.seqs) in TypeScript): CacheDefinitions'
// archiveContext.CurrentDefinition(ArchivedKindSeq, id) returns
// those native bytes. Archived-definition overlays (retired
// definitions replaced in place) are not consulted, matching
// TypeScript, which has no such overlay.

// SequenceDefinition is the seq fields core needs from the
// client's own SeqType (NpcAnimationScanner.ts): how long the
// animation stays on screen, its interrupt priority and its
// standing priority.
type SequenceDefinition struct {
	// Priority is SeqType.priority (opcode 10); -1 when the
	// cache doesn't set it.
	Priority int
	// ForcedPriority is SeqType.forcedPriority (opcode 5); -1
	// when unset.
	ForcedPriority int
	// ClientCycles is one pass of the animation in client
	// cycles; 0 when the sequence has no frames.
	ClientCycles int
}

// clientCyclesPerTick is one game tick in client cycles
// (NpcAnimationScanner.ts: CLIENT_CYCLES_PER_TICK), as
// scripts/dump-seq.ts counts them.
const clientCyclesPerTick = 30

// SequenceScanner decodes sequence definitions from the seq
// config archive, caching one definition per id
// (sequenceDefinitions in TypeScript). It is safe for
// concurrent use.
type SequenceScanner struct {
	revision   int
	file       func(id int) ([]byte, bool)
	lastFileID int

	mu      sync.Mutex
	decoded map[int]SequenceDefinition
}

// NewSequenceScanner builds a scanner over a raw seq-file
// source: file returns one seq file's bytes, or false when the
// archive has no such file (a missing file decodes to the
// defaults, as TypeScript's `?.data ?? new Int8Array()` does).
// revision is the active cache revision: the opcode meanings
// shift at revisions 220 and 226, and the target rev is 242,
// so both are true. lastFileID is the archive's highest file
// id (getLastSequenceId); -1 when the archive is absent.
func NewSequenceScanner(revision int, file func(id int) ([]byte, bool), lastFileID int) *SequenceScanner {
	return &SequenceScanner{
		revision:   revision,
		file:       file,
		lastFileID: lastFileID,
		decoded:    make(map[int]SequenceDefinition),
	}
}

// SequenceScannerForCache builds a scanner over a cache
// facade's native seq archive. A nil facade degrades to the
// no-cache behavior: every id decodes to the defaults, as
// TypeScript's try/catch around the archive access does
// (tests, tools). The revision comes from the facade's
// CacheInfo, matching CachePipeline.getActive().revision.
func SequenceScannerForCache(defs *CacheDefinitions) *SequenceScanner {
	if defs == nil {
		return NewSequenceScanner(242, nil, -1)
	}
	return NewSequenceScanner(defs.info.Revision, defs.seqFile, defs.nativeSeqCount()-1)
}

// seqFile returns one native seq file's bytes, or false when
// the archive has no such file. It is the Go form of
// sequences().getFile(id)?.data in decodeSequence: the native
// archive, not the archived-definition overlay.
func (d *CacheDefinitions) seqFile(id int) ([]byte, bool) {
	if d == nil || d.archiveContext.CurrentDefinition == nil {
		return nil, false
	}
	_, data, ok := d.archiveContext.CurrentDefinition(ArchivedKindSeq, id)
	return data, ok
}

// nativeSeqCount returns the native seq archive's file count
// (one past its highest file id), or 0 with no archive.
func (d *CacheDefinitions) nativeSeqCount() int {
	if d == nil {
		return 0
	}
	return d.archiveContext.Counts.Seq
}

// Preload loads the seq archive at startup, so no animation's
// first lookup decodes it mid-tick (preloadSequences,
// NpcAnimationScanner.ts, called from GameBuilder.ts:40).
// In Go the archive is already decompressed when
// NewCacheDefinitions builds the facade, so there is nothing
// left to load: this is a warm-up probe that confirms the
// source answers, kept as the boot call site for parity.
func (s *SequenceScanner) Preload() {
	if s == nil || s.file == nil {
		return
	}
	s.file(s.lastFileID)
}

// Priority returns SeqType.priority (opcode 10; -1 when
// unset): 1 means the client drops the seq when the actor
// takes a step, as skilling loops do
// (getSequencePriority, NpcAnimationScanner.ts).
func (s *SequenceScanner) Priority(id int) int {
	return s.decodeSequence(id).Priority
}

// ForcedPriority returns SeqType.forcedPriority (opcode 5; 5
// when unset): a sequence can't replace one with a higher
// value (attacks are 6, blocks 5, so a block never cuts a
// swing short) (getSequenceForcedPriority,
// NpcAnimationScanner.ts).
func (s *SequenceScanner) ForcedPriority(id int) int {
	forced := s.decodeSequence(id).ForcedPriority
	if forced == -1 {
		return 5
	}
	return forced
}

// StallTicks returns how many ticks the client keeps an
// animation on screen (its frame lengths / 30)
// (getSequenceStall, NpcAnimationScanner.ts): the partial
// half of the Wiki's Animation stall. A tick is 30 client
// cycles.
func (s *SequenceScanner) StallTicks(id int) int {
	cycles := s.decodeSequence(id).ClientCycles
	if cycles <= 0 {
		return 0
	}
	return (cycles + clientCyclesPerTick - 1) / clientCyclesPerTick
}

// LastSequenceID returns the archive's highest sequence id
// (getLastSequenceId, NpcAnimationScanner.ts: sequences().
// lastFileId), or -1 with no archive.
func (s *SequenceScanner) LastSequenceID() int {
	if s == nil {
		return -1
	}
	return s.lastFileID
}

// decodeSequence reads the seq config once per id
// (decodeSequence, NpcAnimationScanner.ts). An unknown opcode
// ends the walk: the fields after it are then left at their
// defaults, which is what the previous priority-only decoder
// did. A missing file, a short read, or no cache at all all
// decode to the defaults and are cached as such.
func (s *SequenceScanner) decodeSequence(id int) SequenceDefinition {
	s.mu.Lock()
	if cached, ok := s.decoded[id]; ok {
		s.mu.Unlock()
		return cached
	}
	s.mu.Unlock()

	definition := SequenceDefinition{Priority: -1, ForcedPriority: -1, ClientCycles: 0}
	if s != nil && s.file != nil {
		data, _ := s.file(id)
		revision := s.revision
		rev226 := revision >= 226
		rev220 := revision >= 220
		buf := NewByteBuffer(data)
	walk:
		for buf.Remaining() > 0 {
			opcode, err := buf.ReadUnsignedByte()
			if err != nil {
				break
			}
			if opcode == 0 {
				break
			}
			if opcode == 1 {
				count, err := buf.ReadUnsignedShort()
				if err != nil || buf.Remaining() < count*6 {
					break
				}
				for frame := 0; frame < count; frame++ {
					length, err := buf.ReadUnsignedShort()
					if err != nil {
						break walk
					}
					definition.ClientCycles += length
				}
				buf.Offset += count * 4
				continue
			}
			if opcode == 3 || opcode == 17 {
				count, err := buf.ReadUnsignedByte()
				if err != nil || buf.Remaining() < count {
					break
				}
				buf.Offset += count
				continue
			}
			if opcode == 5 || opcode == 9 || opcode == 10 {
				value, err := buf.ReadUnsignedByte()
				if err != nil {
					break
				}
				if opcode == 5 {
					definition.ForcedPriority = value
				} else if opcode == 10 {
					definition.Priority = value
				}
				continue
			}
			// cachedModelId: the animation renders from a
			// skeleton, so its length is the frame range
			// below rather than a per-frame list.
			if opcode == cachedModelOpcode(rev226) && buf.Remaining() >= 4 {
				buf.Offset += 4
				definition.ClientCycles = -1
				continue
			}
			// skeletalStart, skeletalEnd.
			if opcode == skeletalRangeOpcode(rev226) && definition.ClientCycles == -1 && buf.Remaining() >= 4 {
				start, err := buf.ReadUnsignedShort()
				if err != nil {
					break
				}
				end, err := buf.ReadUnsignedShort()
				if err != nil {
					break
				}
				definition.ClientCycles = end - start
				continue
			}
			if opcode == 14 && rev226 {
				entries, err := buf.ReadUnsignedShort()
				if err != nil {
					break
				}
				stride := 4
				if rev220 {
					stride = 8
				}
				if buf.Remaining() < entries*stride {
					break
				}
				buf.Offset += entries * stride
				continue
			}
			if opcode == 16 && rev226 {
				if buf.Remaining() < 1 {
					break
				}
				buf.Offset++
				continue
			}
			if !skipSequenceOpcode(buf, opcode, revision) {
				break
			}
		}
		if definition.ClientCycles < 0 {
			definition.ClientCycles = 0
		}
	}

	s.mu.Lock()
	s.decoded[id] = definition
	s.mu.Unlock()
	return definition
}

// skipSequenceOpcode skips one opcode's payload without
// decoding it (skipSequenceOpcode, NpcAnimationScanner.ts).
// It reports false when the opcode is unknown or the buffer
// runs out, ending the walk.
func skipSequenceOpcode(buf *ByteBuffer, opcode, revision int) bool {
	skip := func(length int) bool {
		if buf.Remaining() < length {
			return false
		}
		buf.Offset += length
		return true
	}
	if opcode == 2 || opcode == 6 || opcode == 7 {
		return skip(2)
	}
	if opcode == 4 {
		return true
	}
	if opcode == 5 || opcode == 8 || opcode == 9 || opcode == 10 || opcode == 11 || opcode == 16 {
		return skip(1)
	}
	if opcode == 3 || opcode == 17 {
		if buf.Remaining() < 1 {
			return false
		}
		count, err := buf.ReadUnsignedByte()
		if err != nil {
			return false
		}
		return skip(count)
	}
	if opcode == 12 {
		if buf.Remaining() < 1 {
			return false
		}
		count, err := buf.ReadUnsignedByte()
		if err != nil {
			return false
		}
		return skip(count * 4)
	}
	if opcode == 13 {
		if revision >= 226 {
			return skip(4)
		}
		if buf.Remaining() < 1 {
			return false
		}
		count, err := buf.ReadUnsignedByte()
		if err != nil {
			return false
		}
		return skip(count * 3)
	}
	if opcode == 14 {
		if revision < 226 {
			return skip(4)
		}
		if buf.Remaining() < 2 {
			return false
		}
		entries, err := buf.ReadUnsignedShort()
		if err != nil {
			return false
		}
		return skip(entries * 8)
	}
	if opcode == 15 {
		if revision >= 226 {
			return skip(4)
		}
		return false
	}
	if opcode == 18 {
		for buf.Remaining() > 0 {
			b, err := buf.ReadUnsignedByte()
			if err != nil || b == 0 {
				break
			}
		}
		return true
	}
	if opcode == 100 {
		if buf.Remaining() < 1 {
			return false
		}
		count, err := buf.ReadUnsignedByte()
		if err != nil {
			return false
		}
		return skip(count * 4)
	}
	return false
}

// SequenceFrameIDs returns the frame ids of one sequence file's
// bytes (getSequenceFrameIds, NpcAnimationScanner.ts): the
// low ids of opcode 1, each OR-ed with its high id. An
// opcode-0 file, an unknown opcode the skipper cannot pass,
// or a short read all yield no ids. It is used only by
// findNpcRigAnimations in TypeScript, which is NOT ported
// (tooling-only: its only caller is the AdminCommands
// plugin's rig-scan command); the function itself is ported
// because the scanner file defines it and later tooling may
// need it. revision defaults the walk the same way the
// scanner's does (237 in TypeScript when omitted).
func SequenceFrameIDs(data []byte, revision int) []int {
	buf := NewByteBuffer(data)
	for buf.Remaining() > 0 {
		opcode, err := buf.ReadUnsignedByte()
		if err != nil {
			return []int{}
		}
		if opcode == 0 {
			return []int{}
		}
		if opcode != 1 {
			if !skipSequenceOpcode(buf, opcode, revision) {
				return []int{}
			}
			continue
		}
		if buf.Remaining() < 2 {
			return []int{}
		}
		count, err := buf.ReadUnsignedShort()
		if err != nil {
			return []int{}
		}
		if buf.Remaining() < count*6 {
			return []int{}
		}
		buf.Offset += count * 2 // frame lengths
		frameIDs := make([]int, count)
		for i := range frameIDs {
			v, err := buf.ReadUnsignedShort()
			if err != nil {
				return []int{}
			}
			frameIDs[i] = v
		}
		for i := range frameIDs {
			v, err := buf.ReadUnsignedShort()
			if err != nil {
				return []int{}
			}
			frameIDs[i] |= v << 16
		}
		return frameIDs
	}
	return []int{}
}
