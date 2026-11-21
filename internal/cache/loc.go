package cache

import (
	"fmt"
	"strings"
)

// LocType is an object (location) definition. It ports LocType
// (codec/rs/config/loctype/LocType.ts) and its loader from
// LocTypeLoader.ts.
//
// Actions use "" for absent (a deleted slot in TypeScript).
// Desc is *string because its absence is meaningful. Params is nil
// until opcode 249 sets it. Types is nil when the loc carries no
// model types (opcode 5 clears it).

// LocModelType is LocModelType (loctype/LocModelType.ts). The
// TypeScript member NORMAL_DIAGIONAL keeps its misspelling here as
// LocModelNormalDiagonal; the value (11) is what matters.
type LocModelType int

const (
	LocModelWall                              LocModelType = 0
	LocModelWallTriCorner                     LocModelType = 1
	LocModelWallCorner                        LocModelType = 2
	LocModelWallRectCorner                    LocModelType = 3
	LocModelWallDecorationInside              LocModelType = 4
	LocModelWallDecorationOutside             LocModelType = 5
	LocModelWallDecorationDiagonalOutside     LocModelType = 6
	LocModelWallDecorationDiagonalInside      LocModelType = 7
	LocModelWallDecorationDiagonalDouble      LocModelType = 8
	LocModelWallDiagonal                      LocModelType = 9
	LocModelNormal                            LocModelType = 10
	LocModelNormalDiagonal                    LocModelType = 11
	LocModelRoofSloped                        LocModelType = 12
	LocModelRoofSlopedOuterCorner             LocModelType = 13
	LocModelRoofSlopedInnerCorner             LocModelType = 14
	LocModelRoofSlopedHardInnerCorner         LocModelType = 15
	LocModelRoofSlopedHardOuterCorner         LocModelType = 16
	LocModelRoofFlat                          LocModelType = 17
	LocModelRoofSlopedOverhang                LocModelType = 18
	LocModelRoofSlopedOverhangOuterCorner     LocModelType = 19
	LocModelRoofSlopedOverhangInnerCorner     LocModelType = 20
	LocModelRoofSlopedOverhangHardOuterCorner LocModelType = 21
	LocModelFloorDecoration                   LocModelType = 22
)

// DefaultDecorDisplacement is LocType.DEFAULT_DECOR_DISPLACEMENT
// (LocType.ts:10).
const DefaultDecorDisplacement = 16

// LocTypeLoader loads object definitions. It is LocTypeLoader
// (LocTypeLoader.ts:648).
type LocTypeLoader = TypeLoader[*LocType]

type LocType struct {
	baseType

	LowDetail bool

	Models [][]int
	Types  []int

	Name     string
	Category int
	Desc     *string

	RecolorFrom   []int
	RecolorTo     []int
	RetextureFrom []int
	RetextureTo   []int

	SizeX int
	SizeY int

	ClipType         int
	ClipMask         int
	BlocksProjectile bool

	IsInteractive int

	ContouredGround int

	ContourGroundType  int
	ContourGroundParam int

	MergeNormals bool
	ModelClipped bool

	SeqId int

	DecorDisplacement int

	Ambient  int
	Contrast int

	Actions []string

	MapFunctionId      int
	MapSceneId         int
	FlipMapSceneSprite bool

	IsRotated bool
	Clipped   bool

	ModelSizeX      int
	ModelSizeHeight int
	ModelSizeY      int

	OffsetX      int
	OffsetHeight int
	OffsetY      int

	ObstructsGround bool
	IsHollow        bool
	SupportItems    int

	Transforms      []int
	TransformVarbit int
	TransformVarp   int

	AmbientSoundId int

	SoundMaxDistance           int
	AmbientSoundChangeTicksMin int
	AmbientSoundChangeTicksMax int
	SoundMinDistance           int
	SoundDistanceFadeCurve     int
	SoundFadeInDuration        int
	SoundFadeOutDuration       int
	SoundFadeInCurve           int
	SoundFadeOutCurve          int
	SoundAreaRadiusOverride    int
	HasSpatialAmbient          bool
	LoopMultiSoundSequentially bool
	UseExactSoundPosition      bool
	DeferredAmbientSwap        bool
	ResetAmbientOnLoopRestart  bool

	AmbientSoundIds []int

	SeqRandomStart bool

	RandomSeqIds    []int
	RandomSeqDelays []int

	Params ParamsMap
}

// NewLocType returns a default-initialized LocType, mirroring the
// constructor (LocType.ts:109-163).
func NewLocType(id int, info CacheInfo) *LocType {
	return &LocType{
		baseType:                newBaseType(id, info),
		Name:                    "null",
		Category:                -1,
		SizeX:                   1,
		SizeY:                   1,
		ClipType:                2,
		BlocksProjectile:        true,
		IsInteractive:           -1,
		ContouredGround:         -1,
		ContourGroundParam:      -1,
		SeqId:                   -1,
		DecorDisplacement:       DefaultDecorDisplacement,
		Actions:                 make([]string, 5),
		MapFunctionId:           -1,
		MapSceneId:              -1,
		Clipped:                 true,
		ModelSizeX:              128,
		ModelSizeHeight:         128,
		ModelSizeY:              128,
		SupportItems:            -1,
		TransformVarbit:         -1,
		TransformVarp:           -1,
		AmbientSoundId:          -1,
		SoundFadeInDuration:     300,
		SoundFadeOutDuration:    300,
		SoundAreaRadiusOverride: -1,
		SeqRandomStart:          true,
	}
}

// Decode runs the opcode loop over one file's bytes.
func (l *LocType) Decode(data []byte) error {
	return l.DecodeFrom(NewByteBuffer(data))
}

// DecodeFrom runs the opcode loop over buf.
func (l *LocType) DecodeFrom(buf *ByteBuffer) error { return decodeLoop(l, buf) }

// Post derives IsInteractive and SupportItems when unset,
// mirroring LocType.post (LocType.ts:582-605). The commented-out
// hollow-object clip change in TypeScript stays commented out.
func (l *LocType) Post() {
	if l.IsInteractive == -1 {
		l.IsInteractive = 0
		if l.Models != nil && (l.Types == nil || (len(l.Types) > 0 && l.Types[0] == int(LocModelNormal))) {
			l.IsInteractive = 1
		}
		for i := 0; i < 5 && i < len(l.Actions); i++ {
			if l.Actions[i] != "" {
				l.IsInteractive = 1
			}
		}
	}
	if l.SupportItems == -1 {
		if l.ClipType != 0 {
			l.SupportItems = 1
		} else {
			l.SupportItems = 0
		}
	}
}

// DecodeLocType decodes one object definition file.
func DecodeLocType(id int, info CacheInfo, data []byte) (*LocType, error) {
	l := NewLocType(id, info)
	if err := l.Decode(data); err != nil {
		return l, err
	}
	l.Post()
	return l, nil
}

func (l *LocType) isNewModelsFormat() bool {
	return l.cacheInfo.Game == GameRunescape && l.cacheInfo.Revision >= 582
}

// skipNewModels consumes one new-format model block, mirroring
// LocType.skipNewModels (LocType.ts:165-180).
func (l *LocType) skipNewModels(buf *ByteBuffer) error {
	count, err := buf.ReadUnsignedByte()
	if err != nil {
		return err
	}
	for i := 0; i < count; i++ {
		if _, err := buf.ReadSignedByte(); err != nil {
			return err
		}
		modelCount, err := buf.ReadUnsignedByte()
		if err != nil {
			return err
		}
		for j := 0; j < modelCount; j++ {
			if l.isLargeModelID() {
				if _, err := buf.ReadBigSmart(); err != nil {
					return err
				}
			} else {
				if _, err := buf.ReadUnsignedShort(); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// DecodeOpcode decodes one opcode's payload, mirroring LocType
// decodeOpcode (LocType.ts:190-580). Unknown opcodes are errors.
//
// Two TypeScript quirks are preserved: opcode 105 declares a local
// constant and never sets FlipMapSceneSprite, so it is a no-op
// here too; and the second opcode 177/178 branches in the source
// (LocType.ts:565-568) are unreachable because the first 177/178
// branches (LocType.ts:559-563) match first, so 177 is a no-op and
// 178 consumes one byte.
func (l *LocType) DecodeOpcode(opcode int, buf *ByteBuffer) error {
	if l.isNewModelsFormat() && (opcode == 1 || opcode == 5) {
		// something in preferences, true breaks some roofs?
		someBool := false
		if opcode == 5 && someBool {
			if err := l.skipNewModels(buf); err != nil {
				return err
			}
		}
		count, err := buf.ReadUnsignedByte()
		if err != nil {
			return err
		}
		l.Types = make([]int, count)
		l.Models = make([][]int, count)
		for i := 0; i < count; i++ {
			typ, err := buf.ReadSignedByte()
			if err != nil {
				return err
			}
			l.Types[i] = typ
			modelCount, err := buf.ReadUnsignedByte()
			if err != nil {
				return err
			}
			l.Models[i] = make([]int, modelCount)
			for j := 0; j < modelCount; j++ {
				var v int
				var err error
				if l.isLargeModelID() {
					v, err = buf.ReadBigSmart()
				} else {
					v, err = buf.ReadUnsignedShort()
				}
				if err != nil {
					return err
				}
				l.Models[i][j] = v
			}
		}
		if opcode == 5 && !someBool {
			if err := l.skipNewModels(buf); err != nil {
				return err
			}
		}
	} else if opcode == 1 {
		count, err := buf.ReadUnsignedByte()
		if err != nil {
			return err
		}
		if count > 0 {
			if l.Models != nil && !l.LowDetail {
				buf.Offset += count * 3
			} else {
				l.Models = make([][]int, count)
				l.Types = make([]int, count)
				for i := 0; i < count; i++ {
					l.Models[i] = make([]int, 1)
					model, err := buf.ReadUnsignedShort()
					if err != nil {
						return err
					}
					typ, err := buf.ReadUnsignedByte()
					if err != nil {
						return err
					}
					l.Models[i][0] = model
					l.Types[i] = typ
				}
			}
		}
	} else if opcode == 2 {
		s, err := l.readString(buf)
		if err != nil {
			return err
		}
		l.Name = s
	} else if opcode == 3 {
		s, err := l.readString(buf)
		if err != nil {
			return err
		}
		l.Desc = &s
	} else if opcode == 5 {
		count, err := buf.ReadUnsignedByte()
		if err != nil {
			return err
		}
		if count > 0 {
			if l.Models != nil && !l.LowDetail {
				buf.Offset += count * 2
			} else {
				l.Types = nil
				l.Models = make([][]int, 1)
				l.Models[0] = make([]int, count)
				for i := 0; i < count; i++ {
					v, err := buf.ReadUnsignedShort()
					if err != nil {
						return err
					}
					l.Models[0][i] = v
				}
			}
		}
	} else if opcode == 6 {
		count, err := buf.ReadUnsignedByte()
		if err != nil {
			return err
		}
		if count > 0 {
			l.Models = make([][]int, count)
			l.Types = make([]int, count)
			for i := 0; i < count; i++ {
				l.Models[i] = make([]int, 1)
				model, err := buf.ReadInt()
				if err != nil {
					return err
				}
				typ, err := buf.ReadUnsignedByte()
				if err != nil {
					return err
				}
				l.Models[i][0] = int(model)
				l.Types[i] = typ
			}
		}
	} else if opcode == 7 {
		count, err := buf.ReadUnsignedByte()
		if err != nil {
			return err
		}
		if count > 0 {
			l.Types = nil
			l.Models = make([][]int, 1)
			l.Models[0] = make([]int, count)
			for i := 0; i < count; i++ {
				v, err := buf.ReadInt()
				if err != nil {
					return err
				}
				l.Models[0][i] = int(v)
			}
		}
	} else if opcode == 14 {
		v, err := buf.ReadUnsignedByte()
		if err != nil {
			return err
		}
		l.SizeX = v
	} else if opcode == 15 {
		v, err := buf.ReadUnsignedByte()
		if err != nil {
			return err
		}
		l.SizeY = v
	} else if opcode == 17 {
		l.ClipType = 0
		l.BlocksProjectile = false
	} else if opcode == 18 {
		l.BlocksProjectile = false
	} else if opcode == 19 {
		v, err := buf.ReadUnsignedByte()
		if err != nil {
			return err
		}
		l.IsInteractive = v
	} else if opcode == 21 {
		l.ContouredGround = 0
		l.ContourGroundType = 1
	} else if opcode == 22 {
		l.MergeNormals = true
	} else if opcode == 23 {
		l.ModelClipped = true
	} else if opcode == 24 {
		var v int
		var err error
		if l.isLargeModelID() {
			v, err = buf.ReadBigSmart()
		} else {
			v, err = buf.ReadUnsignedShort()
		}
		if err != nil {
			return err
		}
		if v == 65535 {
			v = -1
		}
		l.SeqId = v
	} else if opcode == 25 {
		// disposeAlpha? No payload.
	} else if opcode == 27 {
		l.ClipType = 1
	} else if opcode == 28 {
		v, err := buf.ReadUnsignedByte()
		if err != nil {
			return err
		}
		l.DecorDisplacement = v
	} else if opcode == 29 {
		v, err := buf.ReadSignedByte()
		if err != nil {
			return err
		}
		l.Ambient = v
	} else if opcode == 39 {
		v, err := buf.ReadSignedByte()
		if err != nil {
			return err
		}
		l.Contrast = v * 25
	} else if opcode >= 30 && opcode < 39 {
		s, err := l.readString(buf)
		if err != nil {
			return err
		}
		if strings.ToLower(s) == "hidden" {
			s = ""
		}
		// TypeScript writes indices 0..8 into an Array(5); JS
		// arrays grow, so slots 5..8 exist sparsely. Grow the
		// slice the same way instead of panicking.
		idx := opcode - 30
		if idx >= len(l.Actions) {
			grown := make([]string, idx+1)
			copy(grown, l.Actions)
			l.Actions = grown
		}
		l.Actions[idx] = s
	} else if opcode == 40 {
		count, err := buf.ReadUnsignedByte()
		if err != nil {
			return err
		}
		l.RecolorFrom = make([]int, count)
		l.RecolorTo = make([]int, count)
		for i := 0; i < count; i++ {
			from, err := buf.ReadUnsignedShort()
			if err != nil {
				return err
			}
			to, err := buf.ReadUnsignedShort()
			if err != nil {
				return err
			}
			l.RecolorFrom[i] = from
			l.RecolorTo[i] = to
		}
	} else if opcode == 41 {
		count, err := buf.ReadUnsignedByte()
		if err != nil {
			return err
		}
		l.RetextureFrom = make([]int, count)
		l.RetextureTo = make([]int, count)
		for i := 0; i < count; i++ {
			from, err := buf.ReadUnsignedShort()
			if err != nil {
				return err
			}
			to, err := buf.ReadUnsignedShort()
			if err != nil {
				return err
			}
			l.RetextureFrom[i] = from
			l.RetextureTo[i] = to
		}
	} else if opcode == 44 || opcode == 45 {
		if _, err := buf.ReadUnsignedShort(); err != nil {
			return err
		}
	} else if opcode == 60 {
		v, err := buf.ReadUnsignedShort()
		if err != nil {
			return err
		}
		l.MapFunctionId = v
	} else if opcode == 61 {
		v, err := buf.ReadUnsignedShort()
		if err != nil {
			return err
		}
		l.Category = v
	} else if opcode == 62 {
		l.IsRotated = true
	} else if opcode == 64 {
		l.Clipped = false
	} else if opcode == 65 {
		v, err := buf.ReadUnsignedShort()
		if err != nil {
			return err
		}
		l.ModelSizeX = v
	} else if opcode == 66 {
		v, err := buf.ReadUnsignedShort()
		if err != nil {
			return err
		}
		l.ModelSizeHeight = v
	} else if opcode == 67 {
		v, err := buf.ReadUnsignedShort()
		if err != nil {
			return err
		}
		l.ModelSizeY = v
	} else if opcode == 68 {
		v, err := buf.ReadUnsignedShort()
		if err != nil {
			return err
		}
		l.MapSceneId = v
	} else if opcode == 69 {
		v, err := buf.ReadUnsignedByte()
		if err != nil {
			return err
		}
		l.ClipMask = v
	} else if opcode == 70 {
		v, err := buf.ReadShort()
		if err != nil {
			return err
		}
		l.OffsetX = v
	} else if opcode == 71 {
		v, err := buf.ReadShort()
		if err != nil {
			return err
		}
		l.OffsetHeight = v
	} else if opcode == 72 {
		v, err := buf.ReadShort()
		if err != nil {
			return err
		}
		l.OffsetY = v
	} else if opcode == 73 {
		l.ObstructsGround = true
	} else if opcode == 74 {
		l.IsHollow = true
	} else if opcode == 75 {
		v, err := buf.ReadUnsignedByte()
		if err != nil {
			return err
		}
		l.SupportItems = v
	} else if opcode == 77 || opcode == 92 {
		varbit, err := buf.ReadUnsignedShort()
		if err != nil {
			return err
		}
		if varbit == 65535 {
			varbit = -1
		}
		l.TransformVarbit = varbit
		varp, err := buf.ReadUnsignedShort()
		if err != nil {
			return err
		}
		if varp == 65535 {
			varp = -1
		}
		l.TransformVarp = varp
		var3 := -1
		if opcode == 92 {
			var v int
			var err error
			if l.isLargeModelID() {
				v, err = buf.ReadBigSmart()
			} else {
				v, err = buf.ReadUnsignedShort()
			}
			if err != nil {
				return err
			}
			if v == 65535 {
				v = -1
			}
			var3 = v
		}
		count, err := buf.ReadUnsignedByte()
		if err != nil {
			return err
		}
		l.Transforms = make([]int, count+2)
		for i := 0; i <= count; i++ {
			var v int
			var err error
			if l.isLargeModelID() {
				v, err = buf.ReadBigSmart()
			} else {
				v, err = buf.ReadUnsignedShort()
			}
			if err != nil {
				return err
			}
			if v == 65535 {
				v = -1
			}
			l.Transforms[i] = v
		}
		l.Transforms[count+1] = var3
	} else if opcode == 78 {
		id, err := buf.ReadUnsignedShort()
		if err != nil {
			return err
		}
		l.AmbientSoundId = id
		max, err := buf.ReadUnsignedByte()
		if err != nil {
			return err
		}
		l.SoundMaxDistance = max
		if l.cacheInfo.Game == GameOldschool && l.cacheInfo.Revision >= 220 {
			min, err := buf.ReadUnsignedByte()
			if err != nil {
				return err
			}
			l.SoundMinDistance = min
		}
	} else if opcode == 79 {
		min, err := buf.ReadUnsignedShort()
		if err != nil {
			return err
		}
		max, err := buf.ReadUnsignedShort()
		if err != nil {
			return err
		}
		l.AmbientSoundChangeTicksMin = min
		l.AmbientSoundChangeTicksMax = max
		dist, err := buf.ReadUnsignedByte()
		if err != nil {
			return err
		}
		l.SoundMaxDistance = dist
		if l.cacheInfo.Game == GameOldschool && l.cacheInfo.Revision >= 220 {
			smin, err := buf.ReadUnsignedByte()
			if err != nil {
				return err
			}
			l.SoundMinDistance = smin
		}
		count, err := buf.ReadUnsignedByte()
		if err != nil {
			return err
		}
		l.AmbientSoundIds = make([]int, count)
		for i := 0; i < count; i++ {
			v, err := buf.ReadUnsignedShort()
			if err != nil {
				return err
			}
			l.AmbientSoundIds[i] = v
		}
	} else if opcode == 81 {
		v, err := buf.ReadUnsignedByte()
		if err != nil {
			return err
		}
		l.ContouredGround = v * 256
		l.ContourGroundType = 2
		// toSigned16bit (MathUtil.ts): (value << 16) >> 16.
		l.ContourGroundParam = int(int32(l.ContouredGround<<16) >> 16)
	} else if opcode == 82 {
		if l.cacheInfo.Game == GameOldschool {
			v, err := buf.ReadUnsignedShort()
			if err != nil {
				return err
			}
			l.MapFunctionId = v
		} else {
			// hd only = true? No field, no payload.
		}
	} else if opcode == 80 {
		count, err := buf.ReadUnsignedByte()
		if err != nil {
			return err
		}
		for i := 0; i < count; i++ {
			if _, err := buf.ReadUnsignedShort(); err != nil {
				return err
			}
			if _, err := buf.ReadUnsignedShort(); err != nil {
				return err
			}
		}
	} else if opcode == 88 {
		// No payload, no stored field.
	} else if opcode == 89 {
		l.SeqRandomStart = false
	} else if opcode == 90 {
		l.DeferredAmbientSwap = true
	} else if opcode == 91 {
		v, err := buf.ReadUnsignedByte()
		if err != nil {
			return err
		}
		l.SoundDistanceFadeCurve = v
	} else if opcode == 93 {
		if l.cacheInfo.Game == GameOldschool && l.cacheInfo.Revision >= 220 {
			inCurve, err := buf.ReadUnsignedByte()
			if err != nil {
				return err
			}
			inDur, err := buf.ReadUnsignedShort()
			if err != nil {
				return err
			}
			outCurve, err := buf.ReadUnsignedByte()
			if err != nil {
				return err
			}
			outDur, err := buf.ReadUnsignedShort()
			if err != nil {
				return err
			}
			l.SoundFadeInCurve = inCurve
			l.SoundFadeInDuration = inDur
			l.SoundFadeOutCurve = outCurve
			l.SoundFadeOutDuration = outDur
		} else {
			l.ContourGroundType = 3
			v, err := buf.ReadShort()
			if err != nil {
				return err
			}
			l.ContourGroundParam = v
		}
	} else if opcode == 94 {
		l.ContourGroundType = 4
	} else if opcode == 95 {
		if l.cacheInfo.Game == GameOldschool {
			if _, err := buf.ReadUnsignedByte(); err != nil {
				return err
			}
		} else {
			l.ContourGroundType = 5
			if l.cacheInfo.Game == GameRunescape && l.cacheInfo.Revision >= 614 {
				v, err := buf.ReadUnsignedShort()
				if err != nil {
					return err
				}
				l.ContourGroundParam = v
			}
		}
	} else if opcode == 96 {
		if l.cacheInfo.Game == GameOldschool {
			if _, err := buf.ReadUnsignedByte(); err != nil {
				return err
			}
		}
	} else if l.cacheInfo.Game == GameOldschool && (opcode == 97 || opcode == 98) {
		// Rev 242: multiloc with 24-bit signed loc ids (77/92 with
		// g3s ids; 98 adds the default).
		varbit, err := buf.ReadUnsignedShort()
		if err != nil {
			return err
		}
		if varbit == 65535 {
			varbit = -1
		}
		l.TransformVarbit = varbit
		varp, err := buf.ReadUnsignedShort()
		if err != nil {
			return err
		}
		if varp == 65535 {
			varp = -1
		}
		l.TransformVarp = varp
		fallback := -1
		if opcode == 98 {
			v, err := buf.ReadSignedMedium()
			if err != nil {
				return err
			}
			fallback = v
		}
		count, err := buf.ReadUnsignedByte()
		if err != nil {
			return err
		}
		l.Transforms = make([]int, count+2)
		for i := 0; i <= count; i++ {
			v, err := buf.ReadSignedMedium()
			if err != nil {
				return err
			}
			l.Transforms[i] = v
		}
		l.Transforms[count+1] = fallback
	} else if opcode == 97 {
		// adjustMapSceneRotation = true; no field, no payload.
	} else if opcode == 98 {
		// hasAnimation = true; no field, no payload.
	} else if opcode == 99 {
		if _, err := buf.ReadUnsignedByte(); err != nil {
			return err
		}
		if _, err := buf.ReadUnsignedShort(); err != nil {
			return err
		}
	} else if opcode == 100 {
		if _, err := buf.ReadUnsignedByte(); err != nil {
			return err
		}
		if _, err := buf.ReadUnsignedByte(); err != nil {
			return err
		}
		if _, err := l.readString(buf); err != nil {
			return err
		}
	} else if opcode == 101 {
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
		if _, err := l.readString(buf); err != nil {
			return err
		}
	} else if opcode == 102 {
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
	} else if opcode == 103 {
		// occludeType = 0; no field, no payload.
	} else if opcode == 104 {
		if _, err := buf.ReadUnsignedByte(); err != nil {
			return err
		}
	} else if opcode == 105 {
		// TypeScript declares a local `const flipMapSceneSprite =
		// true` here and never assigns the field; preserved as a
		// no-op.
	} else if opcode == 106 {
		count, err := buf.ReadUnsignedByte()
		if err != nil {
			return err
		}
		l.RandomSeqIds = make([]int, count)
		l.RandomSeqDelays = make([]int, count)
		for i := 0; i < count; i++ {
			var v int
			var err error
			if l.isLargeModelID() {
				v, err = buf.ReadBigSmart()
			} else {
				v, err = buf.ReadUnsignedShort()
			}
			if err != nil {
				return err
			}
			delay, err := buf.ReadUnsignedByte()
			if err != nil {
				return err
			}
			l.RandomSeqIds[i] = v
			l.RandomSeqDelays[i] = delay
		}
	} else if opcode == 107 {
		v, err := buf.ReadUnsignedShort()
		if err != nil {
			return err
		}
		l.MapFunctionId = v
	} else if opcode >= 150 && opcode < 155 {
		s, err := l.readString(buf)
		if err != nil {
			return err
		}
		if strings.ToLower(s) == "hidden" {
			s = ""
		}
		l.Actions[opcode-150] = s
	} else if opcode == 160 {
		count, err := buf.ReadUnsignedByte()
		if err != nil {
			return err
		}
		for i := 0; i < count; i++ {
			if _, err := buf.ReadUnsignedShort(); err != nil {
				return err
			}
		}
	} else if opcode == 163 {
		for i := 0; i < 4; i++ {
			if _, err := buf.ReadSignedByte(); err != nil {
				return err
			}
		}
	} else if opcode == 167 {
		if _, err := buf.ReadUnsignedShort(); err != nil {
			return err
		}
	} else if opcode == 168 || opcode == 169 {
		// No payload, no stored field.
	} else if opcode == 170 || opcode == 171 {
		if _, err := buf.ReadUnsignedSmart(); err != nil {
			return err
		}
	} else if opcode == 173 {
		if _, err := buf.ReadUnsignedShort(); err != nil {
			return err
		}
		if _, err := buf.ReadUnsignedShort(); err != nil {
			return err
		}
	} else if opcode == 177 {
		// First 177 branch (LocType.ts:559): no payload. The second
		// 177 branch (loopMultiSoundSequentially) is unreachable.
	} else if opcode == 178 {
		// First 178 branch (LocType.ts:561): consume one byte. The
		// second 178 branch (soundAreaRadiusOverride) is
		// unreachable.
		if _, err := buf.ReadUnsignedByte(); err != nil {
			return err
		}
	} else if opcode == 176 {
		l.HasSpatialAmbient = true
	} else if opcode == 189 {
		l.UseExactSoundPosition = true
	} else if opcode == 190 {
		v, err := buf.ReadUnsignedByte()
		if err != nil {
			return err
		}
		l.DeferredAmbientSwap = v == 1
	} else if opcode == 191 {
		v, err := buf.ReadUnsignedByte()
		if err != nil {
			return err
		}
		l.ResetAmbientOnLoopRestart = v == 1
	} else if opcode == 249 {
		params, err := ReadParamsMap(buf, l.Params)
		if err != nil {
			return err
		}
		l.Params = params
	} else {
		return fmt.Errorf("LocType: Opcode %d not implemented. id: %d", opcode, l.id)
	}
	return nil
}

// Transform resolves one morph step, mirroring LocType.transform
// (LocType.ts:607-635). Unlike NpcType.transform, an in-range var
// value naming -1 falls through to the default (last) entry.
func (l *LocType) Transform(vars VarReader, loader LocTypeLoader) *LocType {
	if len(l.Transforms) == 0 {
		return nil
	}
	transformIndex := -1
	if l.TransformVarbit != -1 {
		transformIndex = vars.GetVarbit(l.TransformVarbit)
	} else if l.TransformVarp != -1 {
		transformIndex = vars.GetVarp(l.TransformVarp)
	}
	transformID := -1
	if transformIndex >= 0 && transformIndex < len(l.Transforms)-1 && l.Transforms[transformIndex] != -1 {
		transformID = l.Transforms[transformIndex]
	} else {
		transformID = l.Transforms[len(l.Transforms)-1]
	}
	if transformID == -1 {
		return nil
	}
	return loader.Load(transformID)
}

// NewArchiveLocLoader builds the archive-backed object loader. It
// is ArchiveLocTypeLoader (LocTypeLoader.ts:656-660).
func NewArchiveLocLoader(info CacheInfo, archive *Archive) *ArchiveTypeLoader[*LocType] {
	return NewArchiveTypeLoader(NewLocType, info, archive)
}

// NewIndexLocLoader builds the index-backed object loader. It is
// IndexLocTypeLoader (LocTypeLoader.ts:662-666), with the default
// 8 file-id bits.
func NewIndexLocLoader(info CacheInfo, index ArchiveSource) *IndexTypeLoader[*LocType] {
	return NewIndexTypeLoader(NewLocType, info, index, 8)
}

// LoadDatLocTypes decodes the legacy indexed .dat/.idx object
// loader. It is DatLocTypeLoader.load (LocTypeLoader.ts:650-654).
func LoadDatLocTypes(info CacheInfo, configArchive *Archive) (*IndexedDatTypeLoader[*LocType], error) {
	return LoadIndexedDatTypes(NewLocType, info, configArchive, "loc")
}
