package cache

import "fmt"

// BasType is a base animation set (BAS): the idle/walk/run/crawl
// sequences for a body, plus turning behaviour. It ports BasType
// (codec/rs/config/bastype/BasType.ts) and its loader from
// BasTypeLoader.ts.
//
// CacheDefinitions never builds a BAS loader (there is no BAS
// archive in the OSRS DAT2 config layout; ConfigType.RS2.bas is
// 32, which OSRS uses for hit splats), but NpcType resolves
// movement sequences through a BasTypeLoader when BasTypeId is
// set, so the decoder and loaders are ported for that path.

// BasTypeLoader loads base animation sets. It is BasTypeLoader
// (BasTypeLoader.ts:364).
type BasTypeLoader = TypeLoader[*BasType]

type BasType struct {
	baseType

	IdleSeqId       int
	WalkSeqId       int
	CrawlSeqId      int
	CrawlBackSeqId  int
	CrawlLeftSeqId  int
	CrawlRightSeqId int
	RunSeqId        int
	RunBackSeqId    int
	RunLeftSeqId    int
	RunRightSeqId   int
	WalkBackSeqId   int
	WalkLeftSeqId   int
	WalkRightSeqId  int
	IdleLeftSeqId   int
	IdleRightSeqId  int

	// ModelRotateTranslate[bodyPartId] is [rotateX, rotateY,
	// rotateZ, translateX, translateY, translateZ]; nil rows are
	// body parts opcode 27 never set.
	ModelRotateTranslate [][]int

	// Turning behaviour (OSRS BAS). Units match the RS engine:
	// YawAcceleration in arbitrary units and YawMaxSpeed in
	// 0..2047 per full circle.
	YawAcceleration int
	YawMaxSpeed     int
}

// NewBasType returns a default-initialized BasType, mirroring the
// field initializers (BasType.ts:207-228).
func NewBasType(id int, info CacheInfo) *BasType {
	return &BasType{
		baseType:        newBaseType(id, info),
		IdleSeqId:       -1,
		WalkSeqId:       -1,
		CrawlSeqId:      -1,
		CrawlBackSeqId:  -1,
		CrawlLeftSeqId:  -1,
		CrawlRightSeqId: -1,
		RunSeqId:        -1,
		RunBackSeqId:    -1,
		RunLeftSeqId:    -1,
		RunRightSeqId:   -1,
		WalkBackSeqId:   -1,
		WalkLeftSeqId:   -1,
		WalkRightSeqId:  -1,
		IdleLeftSeqId:   -1,
		IdleRightSeqId:  -1,
	}
}

// Decode runs the opcode loop over one file's bytes.
func (b *BasType) Decode(data []byte) error {
	return b.DecodeFrom(NewByteBuffer(data))
}

// DecodeFrom runs the opcode loop over buf.
func (b *BasType) DecodeFrom(buf *ByteBuffer) error { return decodeLoop(b, buf) }

// Post is a no-op: BasType has no post() override.
func (b *BasType) Post() {}

// DecodeBasType decodes one BAS definition file.
func DecodeBasType(id int, info CacheInfo, data []byte) (*BasType, error) {
	b := NewBasType(id, info)
	if err := b.Decode(data); err != nil {
		return b, err
	}
	b.Post()
	return b, nil
}

// readSeqID reads a sequence id, mapping 0xffff to -1, as every
// BAS sequence opcode does.
func readSeqID(buf *ByteBuffer) (int, error) {
	v, err := buf.ReadUnsignedShort()
	if err != nil {
		return 0, err
	}
	if v == 0xffff {
		return -1, nil
	}
	return v, nil
}

// DecodeOpcode decodes one opcode's payload, mirroring BasType
// decodeOpcode (BasType.ts:230-357). Unknown opcodes are errors.
// Roll/pitch/movement fields are read past but not stored, as in
// TypeScript.
func (b *BasType) DecodeOpcode(opcode int, buf *ByteBuffer) error {
	if opcode == 1 {
		idle, err := readSeqID(buf)
		if err != nil {
			return err
		}
		walk, err := readSeqID(buf)
		if err != nil {
			return err
		}
		b.IdleSeqId, b.WalkSeqId = idle, walk
	} else if opcode == 2 {
		v, err := readSeqID(buf)
		if err != nil {
			return err
		}
		b.CrawlSeqId = v
	} else if opcode == 3 {
		v, err := readSeqID(buf)
		if err != nil {
			return err
		}
		b.CrawlBackSeqId = v
	} else if opcode == 4 {
		v, err := readSeqID(buf)
		if err != nil {
			return err
		}
		b.CrawlLeftSeqId = v
	} else if opcode == 5 {
		v, err := readSeqID(buf)
		if err != nil {
			return err
		}
		b.CrawlRightSeqId = v
	} else if opcode == 6 {
		v, err := readSeqID(buf)
		if err != nil {
			return err
		}
		b.RunSeqId = v
	} else if opcode == 7 {
		v, err := readSeqID(buf)
		if err != nil {
			return err
		}
		b.RunBackSeqId = v
	} else if opcode == 8 {
		v, err := readSeqID(buf)
		if err != nil {
			return err
		}
		b.RunLeftSeqId = v
	} else if opcode == 9 {
		v, err := readSeqID(buf)
		if err != nil {
			return err
		}
		b.RunRightSeqId = v
	} else if opcode == 26 {
		if _, err := buf.ReadUnsignedByte(); err != nil {
			return err
		}
		if _, err := buf.ReadUnsignedByte(); err != nil {
			return err
		}
	} else if opcode == 27 {
		if b.ModelRotateTranslate == nil {
			b.ModelRotateTranslate = make([][]int, 12)
		}
		bodyPartID, err := buf.ReadUnsignedByte()
		if err != nil {
			return err
		}
		// TypeScript grows the sparse array past 12 when a larger
		// body part id appears; grow the slice the same way.
		for len(b.ModelRotateTranslate) <= bodyPartID {
			b.ModelRotateTranslate = append(b.ModelRotateTranslate, nil)
		}
		row := make([]int, 6)
		for i := 0; i < 6; i++ {
			v, err := buf.ReadShort()
			if err != nil {
				return err
			}
			row[i] = v
		}
		b.ModelRotateTranslate[bodyPartID] = row
	} else if opcode == 29 {
		v, err := buf.ReadUnsignedByte()
		if err != nil {
			return err
		}
		b.YawAcceleration = v
	} else if opcode == 30 {
		v, err := buf.ReadUnsignedShort()
		if err != nil {
			return err
		}
		b.YawMaxSpeed = v
	} else if opcode == 31 {
		if _, err := buf.ReadUnsignedByte(); err != nil {
			return err
		}
	} else if opcode == 32 {
		if _, err := buf.ReadUnsignedShort(); err != nil {
			return err
		}
	} else if opcode == 33 {
		if _, err := buf.ReadShort(); err != nil {
			return err
		}
	} else if opcode == 34 {
		if _, err := buf.ReadUnsignedByte(); err != nil {
			return err
		}
	} else if opcode == 35 {
		if _, err := buf.ReadUnsignedShort(); err != nil {
			return err
		}
	} else if opcode == 36 {
		if _, err := buf.ReadShort(); err != nil {
			return err
		}
	} else if opcode == 37 {
		if _, err := buf.ReadUnsignedByte(); err != nil {
			return err
		}
	} else if opcode == 38 {
		v, err := readSeqID(buf)
		if err != nil {
			return err
		}
		b.IdleLeftSeqId = v
	} else if opcode == 39 {
		v, err := readSeqID(buf)
		if err != nil {
			return err
		}
		b.IdleRightSeqId = v
	} else if opcode == 40 {
		v, err := readSeqID(buf)
		if err != nil {
			return err
		}
		b.WalkBackSeqId = v
	} else if opcode == 41 {
		v, err := readSeqID(buf)
		if err != nil {
			return err
		}
		b.WalkLeftSeqId = v
	} else if opcode == 42 {
		v, err := readSeqID(buf)
		if err != nil {
			return err
		}
		b.WalkRightSeqId = v
	} else if opcode >= 43 && opcode <= 51 {
		if _, err := buf.ReadUnsignedShort(); err != nil {
			return err
		}
	} else if opcode == 52 {
		count, err := buf.ReadUnsignedByte()
		if err != nil {
			return err
		}
		for i := 0; i < count; i++ {
			if _, err := buf.ReadUnsignedShort(); err != nil {
				return err
			}
			if _, err := buf.ReadUnsignedByte(); err != nil {
				return err
			}
		}
	} else if opcode == 53 {
		// No payload.
	} else if opcode == 54 {
		if _, err := buf.ReadUnsignedByte(); err != nil {
			return err
		}
		if _, err := buf.ReadUnsignedByte(); err != nil {
			return err
		}
	} else if opcode == 55 {
		if _, err := buf.ReadUnsignedByte(); err != nil {
			return err
		}
		if _, err := buf.ReadUnsignedShort(); err != nil {
			return err
		}
	} else if opcode == 56 {
		if _, err := buf.ReadUnsignedByte(); err != nil {
			return err
		}
		for i := 0; i < 3; i++ {
			if _, err := buf.ReadShort(); err != nil {
				return err
			}
		}
	} else {
		return fmt.Errorf("BasType: Unknown opcode: %d", opcode)
	}
	return nil
}

// NewDummyBasLoader builds a loader that always returns fresh
// defaults. It is DummyBasTypeLoader (BasTypeLoader.ts:366-370).
func NewDummyBasLoader(info CacheInfo) *DummyTypeLoader[*BasType] {
	return NewDummyTypeLoader(NewBasType, info)
}

// NewArchiveBasLoader builds the archive-backed BAS loader. It is
// ArchiveBasTypeLoader (BasTypeLoader.ts:372-376).
func NewArchiveBasLoader(info CacheInfo, archive *Archive) *ArchiveTypeLoader[*BasType] {
	return NewArchiveTypeLoader(NewBasType, info, archive)
}
