package cache

import (
	"fmt"
	"strings"
)

// ObjType is an item definition. It ports ObjType
// (codec/rs/config/objtype/ObjType.ts) and its loader from
// ObjTypeLoader.ts.
//
// Field notes: action slots use "" for absent (TypeScript null / a
// deleted slot); every truthiness check in the TypeScript code
// treats "" and null alike. Examine is *string because its absence
// is meaningful (undefined until opcode 3). Op9 is a plain string:
// it is undefined until opcode 9 in TypeScript, but no consumer
// distinguishes that from "".

// ObjStackability is ObjStackability
// (codec/rs/config/objtype/ObjStackability.ts).
type ObjStackability int

const (
	// ObjStackabilitySometimes (0) is the default.
	ObjStackabilitySometimes ObjStackability = 0
	// ObjStackabilityAlways (1) is set by opcode 11.
	ObjStackabilityAlways ObjStackability = 1
	// ObjStackabilityNever (2) is set by opcode 160.
	ObjStackabilityNever ObjStackability = 2
)

// ObjTypeLoader loads item definitions. It is ObjTypeLoader
// (ObjTypeLoader.ts:653).
type ObjTypeLoader = TypeLoader[*ObjType]

type ObjType struct {
	baseType

	Model   int
	Name    string
	Examine *string

	RecolorFrom []int
	RecolorTo   []int

	RetextureFrom []int
	RetextureTo   []int

	Zoom2d int
	Xan2d  int
	Yan2d  int
	Zan2d  int

	OffsetX2d int
	OffsetY2d int

	Op9 string

	Stackability ObjStackability

	Price int

	WearPos  int
	WearPos2 int

	IsMembers bool

	GroundActions    []string
	InventoryActions []string
	// Subops[opIndex][subIndex] is the nested "Choose Option" submenu
	// text per inventory op; nil when the item has none, a nil row
	// when that op has none, a nil entry when that subop has none.
	Subops          [][]*string
	ShiftClickIndex int

	MaleModel    int
	MaleModel1   int
	MaleOffset   int
	FemaleModel  int
	FemaleModel1 int
	FemaleOffset int

	MaleModel2   int
	FemaleModel2 int

	MaleHeadModel    int
	MaleHeadModel2   int
	FemaleHeadModel  int
	FemaleHeadModel2 int

	CountObj []int
	CountCo  []int

	WearPos3 int

	Note         int
	NoteTemplate int

	ResizeX int
	ResizeY int
	ResizeZ int

	Ambient  int
	Contrast int

	Team int

	IsTradable   bool
	IsUnlockable bool

	Weight int

	Category int

	UnnotedID int
	NotedID   int

	Placeholder         int
	PlaceholderTemplate int

	Params ParamsMap

	ManwearXOff   int
	ManwearYOff   int
	ManwearZOff   int
	WomanwearXOff int
	WomanwearYOff int
	WomanwearZOff int

	// postProcessed is the __postProcessed marker
	// PostProcessedObjTypeLoader sets (ObjTypeLoader.ts:669-673).
	postProcessed bool
}

// NewObjType returns a default-initialized ObjType, mirroring the
// constructor (ObjType.ts:104-160).
func NewObjType(id int, info CacheInfo) *ObjType {
	return &ObjType{
		baseType:            newBaseType(id, info),
		Model:               0,
		Name:                "null",
		Zoom2d:              2000,
		Stackability:        ObjStackabilitySometimes,
		Price:               1,
		WearPos:             -1,
		WearPos2:            -1,
		GroundActions:       []string{"", "", "Take", "", ""},
		InventoryActions:    []string{"", "", "", "", "Drop"},
		ShiftClickIndex:     -2,
		MaleModel:           -1,
		MaleModel1:          -1,
		FemaleModel:         -1,
		FemaleModel1:        -1,
		MaleModel2:          -1,
		FemaleModel2:        -1,
		MaleHeadModel:       -1,
		MaleHeadModel2:      -1,
		FemaleHeadModel:     -1,
		FemaleHeadModel2:    -1,
		WearPos3:            -1,
		Note:                -1,
		NoteTemplate:        -1,
		ResizeX:             128,
		ResizeY:             128,
		ResizeZ:             128,
		Category:            -1,
		UnnotedID:           -1,
		NotedID:             -1,
		Placeholder:         -1,
		PlaceholderTemplate: -1,
	}
}

// Decode runs the opcode loop over one file's bytes.
func (o *ObjType) Decode(data []byte) error {
	return o.DecodeFrom(NewByteBuffer(data))
}

// DecodeFrom runs the opcode loop over buf.
func (o *ObjType) DecodeFrom(buf *ByteBuffer) error { return decodeLoop(o, buf) }

// Post is a no-op: ObjType has no post() override in TypeScript.
func (o *ObjType) Post() {}

// DecodeObjType decodes one item definition file: defaults, the
// opcode loop, then Post. It mirrors the load path of
// BaseTypeLoader.load for a present file.
func DecodeObjType(id int, info CacheInfo, data []byte) (*ObjType, error) {
	o := NewObjType(id, info)
	if err := o.Decode(data); err != nil {
		return o, err
	}
	o.Post()
	return o, nil
}

func (o *ObjType) readModelID(buf *ByteBuffer) (int, error) {
	if o.isLargeModelID() {
		return buf.ReadBigSmart()
	}
	return buf.ReadUnsignedShort()
}

// DecodeOpcode decodes one opcode's payload, mirroring ObjType
// decodeOpcode (ObjType.ts:170-503). Unknown opcodes are errors,
// as the TypeScript throw is. Several newer-cache opcodes consume
// a placeholder short or byte to keep the stream aligned; those
// values are discarded, exactly as in TypeScript.
func (o *ObjType) DecodeOpcode(opcode int, buf *ByteBuffer) error {
	if opcode == 1 {
		v, err := o.readModelID(buf)
		if err != nil {
			return err
		}
		o.Model = v
	} else if opcode == 2 {
		s, err := o.readString(buf)
		if err != nil {
			return err
		}
		o.Name = s
	} else if opcode == 3 {
		s, err := o.readString(buf)
		if err != nil {
			return err
		}
		o.Examine = &s
	} else if opcode == 4 {
		v, err := buf.ReadUnsignedShort()
		if err != nil {
			return err
		}
		o.Zoom2d = v
	} else if opcode == 5 {
		v, err := buf.ReadUnsignedShort()
		if err != nil {
			return err
		}
		o.Xan2d = v
	} else if opcode == 6 {
		v, err := buf.ReadUnsignedShort()
		if err != nil {
			return err
		}
		o.Yan2d = v
	} else if opcode == 7 {
		v, err := buf.ReadUnsignedShort()
		if err != nil {
			return err
		}
		if v > 32767 {
			v -= 65536
		}
		o.OffsetX2d = v
	} else if opcode == 8 {
		v, err := buf.ReadUnsignedShort()
		if err != nil {
			return err
		}
		if v > 32767 {
			v -= 65536
		}
		o.OffsetY2d = v
	} else if opcode == 9 {
		s, err := o.readString(buf)
		if err != nil {
			return err
		}
		o.Op9 = s
	} else if opcode == 10 {
		if _, err := buf.ReadUnsignedShort(); err != nil {
			return err
		}
	} else if opcode == 11 {
		o.Stackability = ObjStackabilityAlways
	} else if opcode == 12 {
		v, err := buf.ReadInt()
		if err != nil {
			return err
		}
		o.Price = int(v)
	} else if opcode == 13 {
		v, err := buf.ReadUnsignedByte()
		if err != nil {
			return err
		}
		o.WearPos = v
	} else if opcode == 14 {
		v, err := buf.ReadUnsignedByte()
		if err != nil {
			return err
		}
		o.WearPos2 = v
	} else if opcode == 15 {
		// Rev 241: explicitly not tradeable (`tradeable=no`).
		o.IsTradable = false
	} else if opcode == 16 {
		o.IsMembers = true
	} else if opcode == 23 {
		v, err := o.readModelID(buf)
		if err != nil {
			return err
		}
		o.MaleModel = v
		off, err := buf.ReadUnsignedByte()
		if err != nil {
			return err
		}
		o.MaleOffset = off
	} else if opcode == 24 {
		v, err := o.readModelID(buf)
		if err != nil {
			return err
		}
		o.MaleModel1 = v
	} else if opcode == 25 {
		v, err := o.readModelID(buf)
		if err != nil {
			return err
		}
		o.FemaleModel = v
		off, err := buf.ReadUnsignedByte()
		if err != nil {
			return err
		}
		o.FemaleOffset = off
	} else if opcode == 26 {
		v, err := o.readModelID(buf)
		if err != nil {
			return err
		}
		o.FemaleModel1 = v
	} else if opcode == 27 {
		v, err := buf.ReadUnsignedByte()
		if err != nil {
			return err
		}
		o.WearPos3 = v
	} else if opcode >= 30 && opcode < 35 {
		s, err := o.readString(buf)
		if err != nil {
			return err
		}
		if strings.ToLower(s) == "hidden" {
			s = ""
		}
		o.GroundActions[opcode-30] = s
	} else if opcode >= 35 && opcode < 40 {
		index := opcode - 35
		action, err := o.readString(buf)
		if err != nil {
			return err
		}
		if index == 4 && strings.ToLower(action) == "revert" &&
			strings.ToLower(o.InventoryActions[4]) == "drop" {
			spare := -1
			for i := 3; i >= 0; i-- {
				if o.InventoryActions[i] == "" {
					spare = i
					break
				}
			}
			if spare >= 0 {
				o.InventoryActions[spare] = action
			} else {
				o.InventoryActions[index] = action
			}
		} else {
			o.InventoryActions[index] = action
		}
	} else if opcode == 40 {
		count, err := buf.ReadUnsignedByte()
		if err != nil {
			return err
		}
		o.RecolorFrom = make([]int, count)
		o.RecolorTo = make([]int, count)
		for i := 0; i < count; i++ {
			from, err := buf.ReadUnsignedShort()
			if err != nil {
				return err
			}
			to, err := buf.ReadUnsignedShort()
			if err != nil {
				return err
			}
			o.RecolorFrom[i] = from
			o.RecolorTo[i] = to
		}
	} else if opcode == 41 {
		count, err := buf.ReadUnsignedByte()
		if err != nil {
			return err
		}
		o.RetextureFrom = make([]int, count)
		o.RetextureTo = make([]int, count)
		for i := 0; i < count; i++ {
			from, err := buf.ReadUnsignedShort()
			if err != nil {
				return err
			}
			to, err := buf.ReadUnsignedShort()
			if err != nil {
				return err
			}
			o.RetextureFrom[i] = from
			o.RetextureTo[i] = to
		}
	} else if opcode == 42 {
		v, err := buf.ReadSignedByte()
		if err != nil {
			return err
		}
		o.ShiftClickIndex = v
	} else if opcode == 43 {
		opID, err := buf.ReadUnsignedByte()
		if err != nil {
			return err
		}
		if o.Subops == nil {
			o.Subops = make([][]*string, 5)
		}
		validOp := opID >= 0 && opID < 5
		if validOp && o.Subops[opID] == nil {
			o.Subops[opID] = make([]*string, 20)
		}
		for {
			raw, err := buf.ReadUnsignedByte()
			if err != nil {
				return err
			}
			subopID := raw - 1
			if subopID == -1 {
				break
			}
			text, err := o.readString(buf)
			if err != nil {
				return err
			}
			if validOp && subopID >= 0 && subopID < 20 {
				t := text
				o.Subops[opID][subopID] = &t
			}
		}
	} else if opcode == 44 {
		v, err := buf.ReadInt()
		if err != nil {
			return err
		}
		o.Model = int(v)
	} else if opcode == 45 {
		v, err := buf.ReadInt()
		if err != nil {
			return err
		}
		o.MaleModel = int(v)
		off, err := buf.ReadUnsignedByte()
		if err != nil {
			return err
		}
		o.MaleOffset = off
	} else if opcode == 46 {
		v, err := buf.ReadInt()
		if err != nil {
			return err
		}
		o.MaleModel1 = int(v)
	} else if opcode == 47 {
		v, err := buf.ReadInt()
		if err != nil {
			return err
		}
		o.MaleModel2 = int(v)
	} else if opcode == 48 {
		v, err := buf.ReadInt()
		if err != nil {
			return err
		}
		o.FemaleModel = int(v)
		off, err := buf.ReadUnsignedByte()
		if err != nil {
			return err
		}
		o.FemaleOffset = off
	} else if opcode == 49 {
		v, err := buf.ReadInt()
		if err != nil {
			return err
		}
		o.FemaleModel1 = int(v)
	} else if opcode == 50 {
		v, err := buf.ReadInt()
		if err != nil {
			return err
		}
		o.FemaleModel2 = int(v)
	} else if opcode == 51 {
		v, err := buf.ReadInt()
		if err != nil {
			return err
		}
		o.MaleHeadModel = int(v)
	} else if opcode == 52 {
		v, err := buf.ReadInt()
		if err != nil {
			return err
		}
		o.MaleHeadModel2 = int(v)
	} else if opcode == 53 {
		v, err := buf.ReadInt()
		if err != nil {
			return err
		}
		o.FemaleHeadModel = int(v)
	} else if opcode == 54 {
		v, err := buf.ReadInt()
		if err != nil {
			return err
		}
		o.FemaleHeadModel2 = int(v)
	} else if opcode == 65 {
		o.IsTradable = true
	} else if opcode == 160 {
		// Rev 241: `stackable=never`.
		o.Stackability = ObjStackabilityNever
	} else if opcode == 161 {
		// `keeponlyduringseqs`: the sequences during which the
		// item is kept; read past, not stored.
		count, err := buf.ReadUnsignedShort()
		if err != nil {
			return err
		}
		for i := 0; i < count; i++ {
			if _, err := buf.ReadUnsignedShort(); err != nil {
				return err
			}
		}
	} else if opcode == 251 {
		o.IsUnlockable = true
	} else if opcode == 69 || opcode == 88 || opcode == 74 {
		// OSRS newer: small flag; consume a byte to maintain alignment.
		if _, err := buf.ReadUnsignedByte(); err != nil {
			return err
		}
	} else if opcode == 68 || opcode == 66 || opcode == 72 || opcode == 80 ||
		opcode == 81 || opcode == 82 || opcode == 83 || opcode == 84 ||
		opcode == 85 || opcode == 86 || opcode == 87 || opcode == 70 ||
		opcode == 71 || opcode == 76 || opcode == 77 || opcode == 67 ||
		opcode == 116 || opcode == 117 || opcode == 118 || opcode == 119 ||
		opcode == 99 {
		// OSRS newer: short value placeholder (opcode 99 is rev 241
		// `recolall`, one HSL colour); consume to keep stream alignment.
		if _, err := buf.ReadUnsignedShort(); err != nil {
			return err
		}
	} else if opcode == 75 {
		v, err := buf.ReadShort()
		if err != nil {
			return err
		}
		o.Weight = v
	} else if opcode == 78 {
		v, err := o.readModelID(buf)
		if err != nil {
			return err
		}
		o.MaleModel2 = v
	} else if opcode == 79 {
		v, err := o.readModelID(buf)
		if err != nil {
			return err
		}
		o.FemaleModel2 = v
	} else if opcode == 90 {
		v, err := o.readModelID(buf)
		if err != nil {
			return err
		}
		o.MaleHeadModel = v
	} else if opcode == 91 {
		v, err := o.readModelID(buf)
		if err != nil {
			return err
		}
		o.FemaleHeadModel = v
	} else if opcode == 92 {
		v, err := o.readModelID(buf)
		if err != nil {
			return err
		}
		o.MaleHeadModel2 = v
	} else if opcode == 93 {
		v, err := o.readModelID(buf)
		if err != nil {
			return err
		}
		o.FemaleHeadModel2 = v
	} else if opcode == 94 {
		v, err := buf.ReadUnsignedShort()
		if err != nil {
			return err
		}
		o.Category = v
	} else if opcode == 95 {
		v, err := buf.ReadUnsignedShort()
		if err != nil {
			return err
		}
		o.Zan2d = v
	} else if opcode == 96 {
		if _, err := buf.ReadUnsignedByte(); err != nil {
			return err
		}
	} else if opcode == 97 {
		v, err := buf.ReadUnsignedShort()
		if err != nil {
			return err
		}
		o.Note = v
	} else if opcode == 98 {
		v, err := buf.ReadUnsignedShort()
		if err != nil {
			return err
		}
		o.NoteTemplate = v
	} else if opcode >= 100 && opcode < 110 {
		if o.CountObj == nil {
			o.CountObj = make([]int, 10)
			o.CountCo = make([]int, 10)
		}
		obj, err := buf.ReadUnsignedShort()
		if err != nil {
			return err
		}
		co, err := buf.ReadUnsignedShort()
		if err != nil {
			return err
		}
		o.CountObj[opcode-100] = obj
		o.CountCo[opcode-100] = co
	} else if opcode == 110 {
		raw, err := buf.ReadUnsignedShort()
		if err != nil {
			return err
		}
		o.ResizeX = normalizeResize(raw)
	} else if opcode == 111 {
		raw, err := buf.ReadUnsignedShort()
		if err != nil {
			return err
		}
		o.ResizeY = normalizeResize(raw)
	} else if opcode == 112 {
		raw, err := buf.ReadUnsignedShort()
		if err != nil {
			return err
		}
		o.ResizeZ = normalizeResize(raw)
	} else if opcode == 113 {
		v, err := buf.ReadSignedByte()
		if err != nil {
			return err
		}
		o.Ambient = v
	} else if opcode == 114 {
		v, err := buf.ReadSignedByte()
		if err != nil {
			return err
		}
		o.Contrast = v * 5
	} else if opcode == 115 {
		v, err := buf.ReadUnsignedByte()
		if err != nil {
			return err
		}
		o.Team = v
	} else if opcode == 121 || opcode == 122 {
		if _, err := buf.ReadUnsignedShort(); err != nil {
			return err
		}
	} else if opcode == 125 {
		x, err := buf.ReadSignedByte()
		if err != nil {
			return err
		}
		y, err := buf.ReadSignedByte()
		if err != nil {
			return err
		}
		z, err := buf.ReadSignedByte()
		if err != nil {
			return err
		}
		o.ManwearXOff, o.ManwearYOff, o.ManwearZOff = x, y, z
	} else if opcode == 126 {
		x, err := buf.ReadSignedByte()
		if err != nil {
			return err
		}
		y, err := buf.ReadSignedByte()
		if err != nil {
			return err
		}
		z, err := buf.ReadSignedByte()
		if err != nil {
			return err
		}
		o.WomanwearXOff, o.WomanwearYOff, o.WomanwearZOff = x, y, z
	} else if opcode == 127 || opcode == 128 || opcode == 129 || opcode == 130 {
		if _, err := buf.ReadUnsignedByte(); err != nil {
			return err
		}
		if _, err := buf.ReadUnsignedShort(); err != nil {
			return err
		}
	} else if opcode == 132 {
		count, err := buf.ReadUnsignedByte()
		if err != nil {
			return err
		}
		for i := 0; i < count; i++ {
			if _, err := buf.ReadUnsignedShort(); err != nil {
				return err
			}
		}
	} else if opcode == 139 {
		v, err := buf.ReadUnsignedShort()
		if err != nil {
			return err
		}
		o.UnnotedID = v
	} else if opcode == 140 {
		v, err := buf.ReadUnsignedShort()
		if err != nil {
			return err
		}
		o.NotedID = v
	} else if (opcode >= 142 && opcode < 147) || (opcode >= 150 && opcode < 155) {
		if _, err := buf.ReadUnsignedShort(); err != nil {
			return err
		}
	} else if opcode == 148 {
		v, err := buf.ReadUnsignedShort()
		if err != nil {
			return err
		}
		o.Placeholder = v
	} else if opcode == 149 {
		v, err := buf.ReadUnsignedShort()
		if err != nil {
			return err
		}
		o.PlaceholderTemplate = v
	} else if opcode == 200 {
		if _, err := buf.ReadUnsignedByte(); err != nil {
			return err
		}
		if _, err := buf.ReadUnsignedByte(); err != nil {
			return err
		}
		if _, err := o.readString(buf); err != nil {
			return err
		}
	} else if opcode == 201 {
		if _, err := buf.ReadUnsignedByte(); err != nil {
			return err
		}
		if _, err := buf.ReadUnsignedShort(); err != nil {
			return err
		}
		if _, err := buf.ReadUnsignedShort(); err != nil {
			return err
		}
		if _, err := buf.ReadInt(); err != nil {
			return err
		}
		if _, err := buf.ReadInt(); err != nil {
			return err
		}
		if _, err := o.readString(buf); err != nil {
			return err
		}
	} else if opcode == 202 {
		if _, err := buf.ReadUnsignedByte(); err != nil {
			return err
		}
		if _, err := buf.ReadUnsignedShort(); err != nil {
			return err
		}
		if _, err := buf.ReadUnsignedShort(); err != nil {
			return err
		}
		if _, err := buf.ReadUnsignedShort(); err != nil {
			return err
		}
		if _, err := buf.ReadInt(); err != nil {
			return err
		}
		if _, err := buf.ReadInt(); err != nil {
			return err
		}
		if _, err := o.readString(buf); err != nil {
			return err
		}
	} else if opcode == 249 {
		params, err := ReadParamsMap(buf, o.Params)
		if err != nil {
			return err
		}
		o.Params = params
	} else {
		return fmt.Errorf("ObjType: Opcode %d not implemented.", opcode)
	}
	return nil
}

// normalizeResize is ObjType.normalizeResize (ObjType.ts:505-515):
// resize factors outside 1..1024 fall back to the vanilla scale
// 128, so garbage values from newer revisions cannot explode
// wearable models.
func normalizeResize(raw int) int {
	if raw <= 0 || raw > 1024 {
		return 128
	}
	return raw
}

// GenCert derives a noted (certificate) item from template and
// original, mirroring ObjType.genCert (ObjType.ts:517-534). Recolor
// and retexture slices are shared with the template, as in
// TypeScript.
func (o *ObjType) GenCert(template, original *ObjType) {
	o.Model = template.Model
	o.Zoom2d = template.Zoom2d
	o.Xan2d = template.Xan2d
	o.Yan2d = template.Yan2d
	o.Zan2d = template.Zan2d
	o.OffsetX2d = template.OffsetX2d
	o.OffsetY2d = template.OffsetY2d
	o.RecolorFrom = template.RecolorFrom
	o.RecolorTo = template.RecolorTo
	o.RetextureFrom = template.RetextureFrom
	o.RetextureTo = template.RetextureTo
	o.Name = original.Name
	o.IsMembers = original.IsMembers
	o.Price = original.Price
	o.IsTradable = original.IsTradable
	o.Stackability = ObjStackabilityAlways
}

// GenBought derives a bought (lent) item from template and
// original, mirroring ObjType.genBought (ObjType.ts:536-581).
func (o *ObjType) GenBought(template, original *ObjType) {
	o.Model = template.Model
	o.Zoom2d = template.Zoom2d
	o.Xan2d = template.Xan2d
	o.Yan2d = template.Yan2d
	o.Zan2d = template.Zan2d
	o.OffsetX2d = template.OffsetX2d
	o.OffsetY2d = template.OffsetY2d
	o.RecolorFrom = original.RecolorFrom
	o.RecolorTo = original.RecolorTo
	o.RetextureFrom = original.RetextureFrom
	o.RetextureTo = original.RetextureTo
	o.Name = original.Name
	o.IsMembers = original.IsMembers
	o.Stackability = original.Stackability
	o.MaleModel = original.MaleModel
	o.MaleModel1 = original.MaleModel1
	o.MaleModel2 = original.MaleModel2
	o.FemaleModel = original.FemaleModel
	o.FemaleModel1 = original.FemaleModel1
	o.FemaleModel2 = original.FemaleModel2
	o.MaleHeadModel = original.MaleHeadModel
	o.MaleHeadModel2 = original.MaleHeadModel2
	o.FemaleHeadModel = original.FemaleHeadModel
	o.FemaleHeadModel2 = original.FemaleHeadModel2
	o.Team = original.Team
	o.GroundActions = original.GroundActions
	o.Weight = original.Weight
	o.InventoryActions = make([]string, 5)
	for i := 0; i < 4 && i < len(original.InventoryActions); i++ {
		o.InventoryActions[i] = original.InventoryActions[i]
	}
	o.InventoryActions[4] = "Discard"
	if original.Subops != nil {
		o.Subops = make([][]*string, 5)
		for i := 0; i < 4 && i < len(original.Subops); i++ {
			o.Subops[i] = original.Subops[i]
		}
	} else {
		o.Subops = nil
	}
	o.Price = 0
}

// GenPlaceholder derives a bank placeholder from template and
// original, mirroring ObjType.genPlaceholder
// (ObjType.ts:583-600).
func (o *ObjType) GenPlaceholder(template, original *ObjType) {
	o.Model = template.Model
	o.Zoom2d = template.Zoom2d
	o.Xan2d = template.Xan2d
	o.Yan2d = template.Yan2d
	o.Zan2d = template.Zan2d
	o.OffsetX2d = template.OffsetX2d
	o.OffsetY2d = template.OffsetY2d
	o.RecolorFrom = template.RecolorFrom
	o.RecolorTo = template.RecolorTo
	o.RetextureFrom = template.RetextureFrom
	o.RetextureTo = template.RetextureTo
	o.Stackability = template.Stackability
	o.Name = original.Name
	o.Price = 0
	o.IsMembers = false
	o.IsTradable = false
}

// GetCountObj returns the stacked variant for a stack of count, or
// the item itself when it has no count variants or count is 1. It
// mirrors ObjType.getCountObj (ObjType.ts:602-618).
func (o *ObjType) GetCountObj(loader ObjTypeLoader, count int) *ObjType {
	if o.CountObj != nil && count > 1 {
		newID := -1
		n := len(o.CountCo)
		if len(o.CountObj) < n {
			n = len(o.CountObj)
		}
		if n > 10 {
			n = 10
		}
		for i := 0; i < n; i++ {
			if count >= o.CountCo[i] && o.CountCo[i] != 0 {
				newID = o.CountObj[i]
			}
		}
		if newID != -1 {
			return loader.Load(newID)
		}
	}
	return o
}

// GetShiftClickIndex returns the shift-click action index, or -1.
// It mirrors ObjType.getShiftClickIndex (ObjType.ts:620-632).
func (o *ObjType) GetShiftClickIndex() int {
	if o.ShiftClickIndex != -1 && o.InventoryActions != nil {
		if o.ShiftClickIndex >= 0 {
			if o.ShiftClickIndex < len(o.InventoryActions) && o.InventoryActions[o.ShiftClickIndex] != "" {
				return o.ShiftClickIndex
			}
			return -1
		}
		if len(o.InventoryActions) > 4 && strings.ToLower(o.InventoryActions[4]) == "drop" {
			return 4
		}
		return -1
	}
	return -1
}

// HasRecolor reports whether the item carries recolors.
func (o *ObjType) HasRecolor() bool { return o.RecolorTo != nil }

// HasRetexture reports whether the item carries retextures.
func (o *ObjType) HasRetexture() bool { return o.RetextureTo != nil }

// NewArchiveObjLoader builds the archive-backed item loader. It is
// ArchiveObjTypeLoader (ObjTypeLoader.ts:717-721).
func NewArchiveObjLoader(info CacheInfo, archive *Archive) *ArchiveTypeLoader[*ObjType] {
	return NewArchiveTypeLoader(NewObjType, info, archive)
}

// NewIndexObjLoader builds the index-backed item loader. It is
// IndexObjTypeLoader (ObjTypeLoader.ts:723-727), with the default
// 8 file-id bits.
func NewIndexObjLoader(info CacheInfo, index ArchiveSource) *IndexTypeLoader[*ObjType] {
	return NewIndexTypeLoader(NewObjType, info, index, 8)
}

// LoadDatObjTypes decodes the legacy indexed .dat/.idx item loader.
// It is DatObjTypeLoader.load (ObjTypeLoader.ts:711-715).
func LoadDatObjTypes(info CacheInfo, configArchive *Archive) (*IndexedDatTypeLoader[*ObjType], error) {
	return LoadIndexedDatTypes(NewObjType, info, configArchive, "obj")
}

// PostProcessedObjTypeLoader wraps an item loader and applies the
// note/bought/placeholder derivations after decode, mirroring
// PostProcessedObjTypeLoader (ObjTypeLoader.ts:664-709). The
// processed marker is set before dependencies resolve, so
// reference cycles terminate with a partially derived item, as in
// TypeScript.
type PostProcessedObjTypeLoader struct {
	base ObjTypeLoader
}

// NewPostProcessedObjTypeLoader wraps base with post-processing.
func NewPostProcessedObjTypeLoader(base ObjTypeLoader) *PostProcessedObjTypeLoader {
	return &PostProcessedObjTypeLoader{base: base}
}

// Load returns the post-processed item for id.
func (l *PostProcessedObjTypeLoader) Load(id int) *ObjType {
	obj := l.base.Load(id)
	if obj.postProcessed {
		return obj
	}
	// Mark before resolving dependencies to avoid recursion loops.
	obj.postProcessed = true

	// Note (certificate). The TypeScript version swallows errors
	// from each derivation; Load cannot fail here, so there is
	// nothing to swallow.
	if obj.NoteTemplate != -1 {
		obj.GenCert(l.Load(obj.NoteTemplate), l.Load(obj.Note))
	}
	// Bought (notedId/unnotedId link variant).
	if obj.NotedID != -1 {
		obj.GenBought(l.Load(obj.NotedID), l.Load(obj.UnnotedID))
	}
	// Placeholder.
	if obj.PlaceholderTemplate != -1 {
		obj.GenPlaceholder(l.Load(obj.PlaceholderTemplate), l.Load(obj.Placeholder))
	}
	return obj
}

// Count delegates to the wrapped loader.
func (l *PostProcessedObjTypeLoader) Count() int { return l.base.Count() }

// ClearCache delegates to the wrapped loader.
func (l *PostProcessedObjTypeLoader) ClearCache() { l.base.ClearCache() }
