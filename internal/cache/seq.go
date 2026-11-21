package cache

import "fmt"

// SeqType is an animation (sequence) definition. It ports SeqType
// (codec/rs/config/seqtype/SeqType.ts). There is no SeqTypeLoader
// file in TypeScript: CacheDefinitions loads sequences with the
// generic ArchiveTypeLoader (CacheDefinitions.ts:123), so
// NewArchiveSeqLoader is that instantiation.
//
// Frame playback helpers stay client-side in TypeScript and are not
// ported; only decodeOpcode and post are server-relevant (the
// header comment of SeqType.ts says to keep decodeOpcode identical
// to the client).

// SeqSoundEffect is one per-frame sound entry (SeqType.ts:7-14).
// Weight is nil when the cache revision does not carry it
// (pre-226); Location is the radius in tiles, treated as 0 when
// the entry has no falloff.
type SeqSoundEffect struct {
	ID          int
	Loops       int
	Location    int
	Weight      *int
	Attenuation int
}

// SeqBlendEntry is one opcode-100 blend-table row.
type SeqBlendEntry struct {
	Frame    int
	Interval int
}

// SeqTypeLoader loads animation definitions. It is the
// TypeLoader<SeqType> shape CacheDefinitions uses.
type SeqTypeLoader = TypeLoader[*SeqType]

type SeqType struct {
	baseType

	// CrossWorldSound is opcode 19: sounds heard across worlds.
	CrossWorldSound bool

	FrameIds            []int
	ChatFrameIds        []int
	FrameLengths        []int
	FrameSoundOverrides map[int]int
	SoundEffects        []int
	// FrameSounds maps frame -> sound effects, populated by
	// opcodes 14/15 depending on revision.
	FrameSounds map[int][]SeqSoundEffect

	FrameStep int

	// Masks carries a trailing 9999999 sentinel, as decoded.
	Masks []int

	Stretches bool

	// ForcedPriority is the animation interrupt priority (default
	// 5; higher is harder to interrupt).
	ForcedPriority int

	LeftHandItem  int
	RightHandItem int

	MaxLoops int
	Looping  bool

	// PrecedenceAnimating is the movement priority (0 blocks
	// movement, 1 allows, -1 auto -> derived by Post).
	PrecedenceAnimating int
	// Priority is the standing priority (0 blocks when stationary,
	// 2 allows, -1 auto -> derived by Post).
	Priority  int
	ReplyMode int

	BlendTable []SeqBlendEntry

	CachedModelId int
	// SkeletalId aliases CachedModelId.
	SkeletalId     int
	SkeletalStart  int
	SkeletalEnd    int
	SkeletalMasks  []bool
	VerticalOffset int

	// Op14 reports whether the cached-model opcode fired (opcode
	// 13 on rev >= 226, opcode 14 below).
	Op14 bool
}

// NewSeqType returns a default-initialized SeqType, mirroring the
// constructor (SeqType.ts:62-80) plus the field initializer for
// CrossWorldSound (SeqType.ts:18).
func NewSeqType(id int, info CacheInfo) *SeqType {
	return &SeqType{
		baseType:            newBaseType(id, info),
		FrameStep:           -1,
		ForcedPriority:      5,
		LeftHandItem:        -1,
		RightHandItem:       -1,
		MaxLoops:            99,
		PrecedenceAnimating: -1,
		Priority:            -1,
		ReplyMode:           2,
		CachedModelId:       -1,
		SkeletalId:          -1,
	}
}

// Decode runs the opcode loop over one file's bytes.
func (s *SeqType) Decode(data []byte) error {
	return s.DecodeFrom(NewByteBuffer(data))
}

// DecodeFrom runs the opcode loop over buf.
func (s *SeqType) DecodeFrom(buf *ByteBuffer) error { return decodeLoop(s, buf) }

// Post derives the auto priorities from the presence of masks,
// mirroring SeqType.post (SeqType.ts:82-98).
func (s *SeqType) Post() {
	if s.PrecedenceAnimating == -1 {
		if s.Masks == nil && s.SkeletalMasks == nil {
			s.PrecedenceAnimating = 0
		} else {
			s.PrecedenceAnimating = 2
		}
	}
	if s.Priority == -1 {
		if s.Masks == nil && s.SkeletalMasks == nil {
			s.Priority = 0
		} else {
			s.Priority = 2
		}
	}
}

// DecodeSeqType decodes one animation definition file.
func DecodeSeqType(id int, info CacheInfo, data []byte) (*SeqType, error) {
	s := NewSeqType(id, info)
	if err := s.Decode(data); err != nil {
		return s, err
	}
	s.Post()
	return s, nil
}

// DecodeOpcode decodes one opcode's payload, mirroring SeqType
// decodeOpcode (SeqType.ts:100-238). The opcode meanings shift at
// revisions 220 and 226; the branch order below follows the
// TypeScript if-else chain exactly, including that opcode 16 is
// only reachable as the vertical offset on rev >= 226. Unknown
// opcodes are errors (TypeScript warns, then throws; the warning
// has no Go equivalent in this package and is dropped).
func (s *SeqType) DecodeOpcode(opcode int, buf *ByteBuffer) error {
	rev226 := s.cacheInfo.Revision >= 226
	rev220 := s.cacheInfo.Revision >= 220

	if opcode == 1 {
		count, err := buf.ReadUnsignedShort()
		if err != nil {
			return err
		}
		s.FrameLengths = make([]int, count)
		s.FrameIds = make([]int, count)
		for i := 0; i < count; i++ {
			v, err := buf.ReadUnsignedShort()
			if err != nil {
				return err
			}
			s.FrameLengths[i] = v
		}
		for i := 0; i < count; i++ {
			v, err := buf.ReadUnsignedShort()
			if err != nil {
				return err
			}
			s.FrameIds[i] = v
		}
		for i := 0; i < count; i++ {
			v, err := buf.ReadUnsignedShort()
			if err != nil {
				return err
			}
			s.FrameIds[i] += v << 16
		}
	} else if opcode == 2 {
		v, err := buf.ReadUnsignedShort()
		if err != nil {
			return err
		}
		s.FrameStep = v
	} else if opcode == 3 {
		count, err := buf.ReadUnsignedByte()
		if err != nil {
			return err
		}
		s.Masks = make([]int, count+1)
		for i := 0; i < count; i++ {
			v, err := buf.ReadUnsignedByte()
			if err != nil {
				return err
			}
			s.Masks[i] = v
		}
		s.Masks[count] = 9999999
	} else if opcode == 4 {
		s.Stretches = true
	} else if opcode == 5 {
		v, err := buf.ReadUnsignedByte()
		if err != nil {
			return err
		}
		s.ForcedPriority = v
	} else if opcode == 6 {
		v, err := buf.ReadUnsignedShort()
		if err != nil {
			return err
		}
		s.LeftHandItem = v
	} else if opcode == 7 {
		v, err := buf.ReadUnsignedShort()
		if err != nil {
			return err
		}
		s.RightHandItem = v
	} else if opcode == 8 {
		v, err := buf.ReadUnsignedByte()
		if err != nil {
			return err
		}
		s.MaxLoops = v
		s.Looping = true
	} else if opcode == 9 {
		v, err := buf.ReadUnsignedByte()
		if err != nil {
			return err
		}
		s.PrecedenceAnimating = v
	} else if opcode == 10 {
		v, err := buf.ReadUnsignedByte()
		if err != nil {
			return err
		}
		s.Priority = v
	} else if opcode == 11 {
		v, err := buf.ReadUnsignedByte()
		if err != nil {
			return err
		}
		s.ReplyMode = v
	} else if opcode == 12 {
		count, err := buf.ReadUnsignedByte()
		if err != nil {
			return err
		}
		s.ChatFrameIds = make([]int, count)
		for i := 0; i < count; i++ {
			v, err := buf.ReadUnsignedShort()
			if err != nil {
				return err
			}
			s.ChatFrameIds[i] = v
		}
		for i := 0; i < count; i++ {
			v, err := buf.ReadUnsignedShort()
			if err != nil {
				return err
			}
			s.ChatFrameIds[i] += v << 16
		}
	} else if opcode == 13 && !rev226 {
		count, err := buf.ReadUnsignedByte()
		if err != nil {
			return err
		}
		s.SoundEffects = make([]int, count)
		for i := 0; i < count; i++ {
			v, err := buf.ReadMedium()
			if err != nil {
				return err
			}
			s.SoundEffects[i] = v
		}
	} else if opcode == cachedModelOpcode(rev226) {
		v, err := buf.ReadInt()
		if err != nil {
			return err
		}
		s.CachedModelId = int(v)
		s.SkeletalId = s.CachedModelId
		s.Op14 = true
	} else if opcode == frameSoundsOpcode(rev226) {
		entries, err := buf.ReadUnsignedShort()
		if err != nil {
			return err
		}
		s.FrameSounds = make(map[int][]SeqSoundEffect)
		for i := 0; i < entries; i++ {
			frame, err := buf.ReadUnsignedShort()
			if err != nil {
				return err
			}
			var effect SeqSoundEffect
			if rev220 {
				id, err := buf.ReadUnsignedShort()
				if err != nil {
					return err
				}
				effect.ID = id
				if rev226 {
					w, err := buf.ReadUnsignedByte()
					if err != nil {
						return err
					}
					effect.Weight = &w
				}
				loops, err := buf.ReadUnsignedByte()
				if err != nil {
					return err
				}
				location, err := buf.ReadUnsignedByte()
				if err != nil {
					return err
				}
				attenuation, err := buf.ReadUnsignedByte()
				if err != nil {
					return err
				}
				effect.Loops, effect.Location, effect.Attenuation = loops, location, attenuation
			} else {
				val, err := buf.ReadMedium()
				if err != nil {
					return err
				}
				effect.Location = val & 15
				effect.ID = val >> 8
				effect.Loops = (val >> 4) & 7
				effect.Attenuation = 0
			}
			s.FrameSounds[frame] = append(s.FrameSounds[frame], effect)
		}
	} else if opcode == skeletalRangeOpcode(rev226) {
		start, err := buf.ReadUnsignedShort()
		if err != nil {
			return err
		}
		end, err := buf.ReadUnsignedShort()
		if err != nil {
			return err
		}
		s.SkeletalStart = start
		s.SkeletalEnd = end
	} else if opcode == 16 {
		// Implied rev226 due to order of checks above.
		v, err := buf.ReadSignedByte()
		if err != nil {
			return err
		}
		s.VerticalOffset = v
	} else if opcode == 17 {
		count, err := buf.ReadUnsignedByte()
		if err != nil {
			return err
		}
		s.SkeletalMasks = make([]bool, 256)
		for i := 0; i < count; i++ {
			idx, err := buf.ReadUnsignedByte()
			if err != nil {
				return err
			}
			s.SkeletalMasks[idx] = true
		}
	} else if opcode == 18 {
		// buffer.readString(): a plain NUL-terminated string,
		// discarded. Note this is not Type.readString, so the
		// terminator is 0 even for non-dat2 caches.
		if _, err := buf.ReadString(); err != nil {
			return err
		}
	} else if opcode == 19 {
		// `crossworldsound=yes` (no payload).
		s.CrossWorldSound = true
	} else if opcode == 100 {
		count, err := buf.ReadUnsignedByte()
		if err != nil {
			return err
		}
		table := make([]SeqBlendEntry, 0, count)
		for i := 0; i < count; i++ {
			frame, err := buf.ReadUnsignedShort()
			if err != nil {
				return err
			}
			interval, err := buf.ReadUnsignedShort()
			if err != nil {
				return err
			}
			table = append(table, SeqBlendEntry{Frame: frame, Interval: interval})
		}
		s.BlendTable = table
	} else {
		return fmt.Errorf("SeqType: Opcode %d not implemented.", opcode)
	}
	return nil
}

// cachedModelOpcode is 13 on rev >= 226, 14 below (SeqType.ts:152).
func cachedModelOpcode(rev226 bool) int {
	if rev226 {
		return 13
	}
	return 14
}

// frameSoundsOpcode is 14 on rev >= 226, 15 below (SeqType.ts:156).
func frameSoundsOpcode(rev226 bool) int {
	if rev226 {
		return 14
	}
	return 15
}

// skeletalRangeOpcode is 15 on rev >= 226, 16 below
// (SeqType.ts:185).
func skeletalRangeOpcode(rev226 bool) int {
	if rev226 {
		return 15
	}
	return 16
}

// IsSkeletalSeq reports whether the sequence is skeletal.
func (s *SeqType) IsSkeletalSeq() bool { return s.SkeletalId >= 0 }

// GetSkeletalDuration returns the skeletal frame range length.
func (s *SeqType) GetSkeletalDuration() int { return s.SkeletalEnd - s.SkeletalStart }

// IsSingleShot reports whether the animation is cancelled when
// movement starts (priority == 1). It mirrors SeqType.isSingleShot
// (SeqType.ts:253-255).
func (s *SeqType) IsSingleShot() bool { return s.Priority == 1 }

// NewArchiveSeqLoader builds the archive-backed animation loader:
// the generic ArchiveTypeLoader instantiated for SeqType, as
// CacheDefinitions does (CacheDefinitions.ts:123).
func NewArchiveSeqLoader(info CacheInfo, archive *Archive) *ArchiveTypeLoader[*SeqType] {
	return NewArchiveTypeLoader(NewSeqType, info, archive)
}
