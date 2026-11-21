package cache

import "fmt"

// SpotAnimType is a graphic definition: what a spell, an emote or a
// loc effect plays. It ports SpotAnimType
// (codec/rs/config/spotanimtype/SpotAnimType.ts) and its loader
// from SpotAnimTypeLoader.ts.
//
// DebugName is *string because its absence is meaningful. Recolor
// and retexture arrays start empty (non-nil, as the TypeScript
// field initializers) and opcodes 40/41 append to them, unlike the
// other types, which replace.

// SpotAnimTypeLoader loads graphic definitions. It is
// SpotAnimTypeLoader (SpotAnimTypeLoader.ts:83).
type SpotAnimTypeLoader = TypeLoader[*SpotAnimType]

type SpotAnimType struct {
	baseType

	ModelId       int
	AnimationId   int
	ResizeX       int
	ResizeY       int
	Rotation      int
	Ambient       int
	Contrast      int
	Rotate        bool
	RecolAll      int
	RecolorFrom   []int
	RecolorTo     []int
	RetextureFrom []int
	RetextureTo   []int
	DebugName     *string
}

// NewSpotAnimType returns a default-initialized SpotAnimType,
// mirroring the field initializers (SpotAnimType.ts:6-20).
func NewSpotAnimType(id int, info CacheInfo) *SpotAnimType {
	return &SpotAnimType{
		baseType:      newBaseType(id, info),
		AnimationId:   -1,
		ResizeX:       128,
		ResizeY:       128,
		RecolAll:      -1,
		RecolorFrom:   []int{},
		RecolorTo:     []int{},
		RetextureFrom: []int{},
		RetextureTo:   []int{},
	}
}

// Decode runs the opcode loop over one file's bytes.
func (s *SpotAnimType) Decode(data []byte) error {
	return s.DecodeFrom(NewByteBuffer(data))
}

// DecodeFrom runs the opcode loop over buf.
func (s *SpotAnimType) DecodeFrom(buf *ByteBuffer) error { return decodeLoop(s, buf) }

// Post is a no-op: SpotAnimType has no post() override.
func (s *SpotAnimType) Post() {}

// DecodeSpotAnimType decodes one graphic definition file.
func DecodeSpotAnimType(id int, info CacheInfo, data []byte) (*SpotAnimType, error) {
	s := NewSpotAnimType(id, info)
	if err := s.Decode(data); err != nil {
		return s, err
	}
	s.Post()
	return s, nil
}

// DecodeOpcode decodes one opcode's payload, mirroring
// SpotAnimType decodeOpcode (SpotAnimType.ts:22-76). Note opcode 9
// uses buffer.readString() directly (a plain NUL-terminated
// string), not Type.readString.
func (s *SpotAnimType) DecodeOpcode(opcode int, buf *ByteBuffer) error {
	switch opcode {
	case 1:
		v, err := buf.ReadUnsignedShort()
		if err != nil {
			return err
		}
		s.ModelId = v
	case 2:
		v, err := buf.ReadUnsignedShort()
		if err != nil {
			return err
		}
		s.AnimationId = v
	case 3:
		v, err := buf.ReadInt()
		if err != nil {
			return err
		}
		s.ModelId = int(v)
	case 4:
		v, err := buf.ReadUnsignedShort()
		if err != nil {
			return err
		}
		s.ResizeX = v
	case 5:
		v, err := buf.ReadUnsignedShort()
		if err != nil {
			return err
		}
		s.ResizeY = v
	case 6:
		v, err := buf.ReadUnsignedShort()
		if err != nil {
			return err
		}
		s.Rotation = v
	case 7:
		v, err := buf.ReadUnsignedByte()
		if err != nil {
			return err
		}
		s.Ambient = v
	case 8:
		v, err := buf.ReadUnsignedByte()
		if err != nil {
			return err
		}
		s.Contrast = v
	case 9:
		name, err := buf.ReadString()
		if err != nil {
			return err
		}
		s.DebugName = &name
	case 10:
		s.Rotate = true
	case 40:
		count, err := buf.ReadUnsignedByte()
		if err != nil {
			return err
		}
		for i := 0; i < count; i++ {
			from, err := buf.ReadUnsignedShort()
			if err != nil {
				return err
			}
			to, err := buf.ReadUnsignedShort()
			if err != nil {
				return err
			}
			s.RecolorFrom = append(s.RecolorFrom, from)
			s.RecolorTo = append(s.RecolorTo, to)
		}
	case 41:
		count, err := buf.ReadUnsignedByte()
		if err != nil {
			return err
		}
		for i := 0; i < count; i++ {
			from, err := buf.ReadUnsignedShort()
			if err != nil {
				return err
			}
			to, err := buf.ReadUnsignedShort()
			if err != nil {
				return err
			}
			s.RetextureFrom = append(s.RetextureFrom, from)
			s.RetextureTo = append(s.RetextureTo, to)
		}
	case 42:
		v, err := buf.ReadUnsignedShort()
		if err != nil {
			return err
		}
		s.RecolAll = v
	default:
		return fmt.Errorf("SpotAnimType: opcode %d not implemented. id: %d", opcode, s.id)
	}
	return nil
}

// NewArchiveSpotAnimLoader builds the archive-backed graphic
// loader. It is ArchiveSpotAnimTypeLoader
// (SpotAnimTypeLoader.ts:85-92).
func NewArchiveSpotAnimLoader(info CacheInfo, archive *Archive) *ArchiveTypeLoader[*SpotAnimType] {
	return NewArchiveTypeLoader(NewSpotAnimType, info, archive)
}
