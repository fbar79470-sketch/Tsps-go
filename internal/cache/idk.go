package cache

import "fmt"

// IdkType is an identity kit (a player body part: head, jaw,
// torso, ...). It ports IdkType (codec/rs/config/idktype/IdkType.ts)
// and its loader from IdkTypeLoader.ts. Only what the server needs
// to choose kits is kept; every other opcode is read past, as in
// TypeScript.

// IdkTypeLoader loads identity kits. It is IdkTypeLoader
// (IdkTypeLoader.ts:196).
type IdkTypeLoader = TypeLoader[*IdkType]

type IdkType struct {
	baseType

	// BodyPartId is 0-6 for a male body part (head, jaw, torso,
	// arms, hands, legs, feet), 7-13 female.
	BodyPartId int

	// NonSelectable means the kit is not offered in the character
	// designer.
	NonSelectable bool
}

// NewIdkType returns a default-initialized IdkType, mirroring the
// field initializers (IdkType.ts:161-164).
func NewIdkType(id int, info CacheInfo) *IdkType {
	return &IdkType{baseType: newBaseType(id, info), BodyPartId: -1}
}

// Decode runs the opcode loop over one file's bytes.
func (k *IdkType) Decode(data []byte) error {
	return k.DecodeFrom(NewByteBuffer(data))
}

// DecodeFrom runs the opcode loop over buf.
func (k *IdkType) DecodeFrom(buf *ByteBuffer) error { return decodeLoop(k, buf) }

// Post is a no-op: IdkType has no post() override.
func (k *IdkType) Post() {}

// DecodeIdkType decodes one identity-kit definition file.
func DecodeIdkType(id int, info CacheInfo, data []byte) (*IdkType, error) {
	k := NewIdkType(id, info)
	if err := k.Decode(data); err != nil {
		return k, err
	}
	k.Post()
	return k, nil
}

// skip advances the buffer by length bytes. It is IdkType.skip
// (IdkType.ts:187-189), which adds to the offset unchecked;
// skipping past the end is a buffer overflow here, which the dat2
// decode loop treats as a clean stop. Skipping past the end in
// TypeScript leaves the offset out of range and the next read
// throws, so the observable result is the same.
func skipBytes(buf *ByteBuffer, length int) error {
	if length < 0 || buf.Offset+length > buf.Len() {
		return ErrBufferOverflow
	}
	buf.Offset += length
	return nil
}

// DecodeOpcode decodes one opcode's payload, mirroring IdkType
// decodeOpcode (IdkType.ts:166-184).
func (k *IdkType) DecodeOpcode(opcode int, buf *ByteBuffer) error {
	if opcode == 1 {
		v, err := buf.ReadUnsignedByte()
		if err != nil {
			return err
		}
		k.BodyPartId = v
	} else if opcode == 2 {
		count, err := buf.ReadUnsignedByte()
		if err != nil {
			return err
		}
		if err := skipBytes(buf, count*2); err != nil {
			return err
		}
	} else if opcode == 3 {
		k.NonSelectable = true
	} else if opcode == 5 {
		count, err := buf.ReadUnsignedByte()
		if err != nil {
			return err
		}
		if err := skipBytes(buf, count*4); err != nil {
			return err
		}
	} else if opcode == 40 || opcode == 41 {
		count, err := buf.ReadUnsignedByte()
		if err != nil {
			return err
		}
		if err := skipBytes(buf, count*4); err != nil {
			return err
		}
	} else if opcode >= 60 && opcode < 70 {
		if err := skipBytes(buf, 2); err != nil {
			return err
		}
	} else if opcode >= 70 && opcode < 80 {
		if err := skipBytes(buf, 4); err != nil {
			return err
		}
	} else {
		return fmt.Errorf("IdkType: Opcode %d not implemented. id: %d", opcode, k.id)
	}
	return nil
}

// NewArchiveIdkLoader builds the archive-backed identity-kit
// loader. It is ArchiveIdkTypeLoader (IdkTypeLoader.ts:198-202).
func NewArchiveIdkLoader(info CacheInfo, archive *Archive) *ArchiveTypeLoader[*IdkType] {
	return NewArchiveTypeLoader(NewIdkType, info, archive)
}
