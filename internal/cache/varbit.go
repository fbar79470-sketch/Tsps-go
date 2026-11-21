package cache

import "fmt"

// VarBitType is a varbit definition: a bit range of one varp. It
// ports VarBitType (codec/rs/config/vartype/bit/VarBitType.ts) and
// its loader from VarBitTypeLoader.ts. VarcIntType, the client-var
// integer definition, is ported alongside it from
// codec/rs/config/vartype/VarcIntType.ts.
//
// VarBitType fields are undefined until opcode 1 in TypeScript, and
// VarManager.getVarbitDef treats a varbit with undefined baseVar as
// absent (VarManager.ts:115-122). Defined reports that presence
// here; the numeric fields are 0 when unset.

// VarBitTypeLoader loads varbit definitions. It is VarBitTypeLoader
// (VarBitTypeLoader.ts:124).
type VarBitTypeLoader = TypeLoader[*VarBitType]

type VarBitType struct {
	baseType

	BaseVar  int
	StartBit int
	EndBit   int

	defined bool
}

// NewVarBitType returns a default-initialized VarBitType.
func NewVarBitType(id int, info CacheInfo) *VarBitType {
	return &VarBitType{baseType: newBaseType(id, info)}
}

// Defined reports whether opcode 1 has set this varbit's fields.
func (v *VarBitType) Defined() bool { return v.defined }

// Decode runs the opcode loop over one file's bytes.
func (v *VarBitType) Decode(data []byte) error {
	return v.DecodeFrom(NewByteBuffer(data))
}

// DecodeFrom runs the opcode loop over buf.
func (v *VarBitType) DecodeFrom(buf *ByteBuffer) error { return decodeLoop(v, buf) }

// Post is a no-op: VarBitType has no post() override.
func (v *VarBitType) Post() {}

// DecodeVarBitType decodes one varbit definition file.
func DecodeVarBitType(id int, info CacheInfo, data []byte) (*VarBitType, error) {
	v := NewVarBitType(id, info)
	if err := v.Decode(data); err != nil {
		return v, err
	}
	v.Post()
	return v, nil
}

// DecodeOpcode decodes one opcode's payload, mirroring VarBitType
// decodeOpcode (VarBitType.ts:102-110). Only opcode 1 exists.
func (v *VarBitType) DecodeOpcode(opcode int, buf *ByteBuffer) error {
	if opcode == 1 {
		base, err := buf.ReadUnsignedShort()
		if err != nil {
			return err
		}
		start, err := buf.ReadUnsignedByte()
		if err != nil {
			return err
		}
		end, err := buf.ReadUnsignedByte()
		if err != nil {
			return err
		}
		v.BaseVar, v.StartBit, v.EndBit = base, start, end
		v.defined = true
		return nil
	}
	return fmt.Errorf("VarBitType: Opcode %d not implemented. id: %d", opcode, v.id)
}

// NewDummyVarBitLoader builds a loader that always returns fresh
// defaults. It is DummyVarBitTypeLoader (VarBitTypeLoader.ts:126-130).
func NewDummyVarBitLoader(info CacheInfo) *DummyTypeLoader[*VarBitType] {
	return NewDummyTypeLoader(NewVarBitType, info)
}

// LoadDatVarBitTypes decodes the legacy .dat varbit loader. It is
// DatVarBitTypeLoader.load (VarBitTypeLoader.ts:132-136), which
// uses the eager DatTypeLoader (not the indexed variant).
func LoadDatVarBitTypes(info CacheInfo, configArchive *Archive) (*DatTypeLoader[*VarBitType], error) {
	return LoadDatTypes(NewVarBitType, info, configArchive, "varbit")
}

// NewArchiveVarBitLoader builds the archive-backed varbit loader.
// It is ArchiveVarBitTypeLoader (VarBitTypeLoader.ts:138-145).
func NewArchiveVarBitLoader(info CacheInfo, archive *Archive) *ArchiveTypeLoader[*VarBitType] {
	return NewArchiveTypeLoader(NewVarBitType, info, archive)
}

// NewIndexVarBitLoader builds the index-backed varbit loader. It
// is IndexVarBitTypeLoader (VarBitTypeLoader.ts:147-151), with 10
// file-id bits.
func NewIndexVarBitLoader(info CacheInfo, index ArchiveSource) *IndexTypeLoader[*VarBitType] {
	return NewIndexTypeLoader(NewVarBitType, info, index, 10)
}

// VarcIntTypeLoader loads client-var integer definitions. It is
// VarcIntTypeLoader (VarcIntTypeLoader.ts:938).
type VarcIntTypeLoader = TypeLoader[*VarcIntType]

// VarcIntType is a client-var integer definition: the only
// property is whether it persists. It ports VarcIntType
// (VarcIntType.ts:924-932).
type VarcIntType struct {
	baseType

	Persist bool
}

// NewVarcIntType returns a default-initialized VarcIntType.
func NewVarcIntType(id int, info CacheInfo) *VarcIntType {
	return &VarcIntType{baseType: newBaseType(id, info)}
}

// Decode runs the opcode loop over one file's bytes.
func (v *VarcIntType) Decode(data []byte) error {
	return v.DecodeFrom(NewByteBuffer(data))
}

// DecodeFrom runs the opcode loop over buf.
func (v *VarcIntType) DecodeFrom(buf *ByteBuffer) error { return decodeLoop(v, buf) }

// Post is a no-op: VarcIntType has no post() override.
func (v *VarcIntType) Post() {}

// DecodeVarcIntType decodes one varc-int definition file.
func DecodeVarcIntType(id int, info CacheInfo, data []byte) (*VarcIntType, error) {
	v := NewVarcIntType(id, info)
	if err := v.Decode(data); err != nil {
		return v, err
	}
	v.Post()
	return v, nil
}

// DecodeOpcode mirrors VarcIntType decodeOpcode
// (VarcIntType.ts:927-931): opcode 2 sets Persist; every other
// opcode is silently ignored (there is no throw in TypeScript).
func (v *VarcIntType) DecodeOpcode(opcode int, _ *ByteBuffer) error {
	if opcode == 2 {
		v.Persist = true
	}
	return nil
}

// NewEmptyVarcIntLoader builds a loader that always returns fresh
// defaults. It is EmptyVarcIntTypeLoader
// (VarcIntTypeLoader.ts:940-944).
func NewEmptyVarcIntLoader(info CacheInfo) *DummyTypeLoader[*VarcIntType] {
	return NewDummyTypeLoader(NewVarcIntType, info)
}

// NewArchiveVarcIntLoader builds the archive-backed varc-int
// loader. It is ArchiveVarcIntTypeLoader
// (VarcIntTypeLoader.ts:946-953).
func NewArchiveVarcIntLoader(info CacheInfo, archive *Archive) *ArchiveTypeLoader[*VarcIntType] {
	return NewArchiveTypeLoader(NewVarcIntType, info, archive)
}
