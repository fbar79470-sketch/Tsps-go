package cache

import (
	"fmt"
	"strings"
)

// NpcType is an NPC definition. It ports NpcType
// (codec/rs/config/npctype/NpcType.ts) and its loader from
// NpcTypeLoader.ts.
//
// Actions use "" for absent (a deleted slot in TypeScript).
// Desc is *string because its absence is meaningful. Params is nil
// until opcode 249 sets it.

// NpcDrawPriority is NpcDrawPriority (NpcType.ts:8-12).
type NpcDrawPriority int

const (
	// NpcDrawPriorityFirst is set by opcode 99.
	NpcDrawPriorityFirst NpcDrawPriority = 0
	// NpcDrawPriorityDefault is the default.
	NpcDrawPriorityDefault NpcDrawPriority = 1
	// NpcDrawPriorityLast is set by opcode 111 for oldschool caches.
	NpcDrawPriorityLast NpcDrawPriority = 2
)

// NpcTypeLoader loads NPC definitions. It is NpcTypeLoader
// (NpcTypeLoader.ts:646).
type NpcTypeLoader = TypeLoader[*NpcType]

type NpcType struct {
	baseType

	Name string
	Desc *string

	Size int

	ModelIds         []int
	ChatheadModelIds []int
	IdleSeqId        int
	TurnLeftSeqId    int
	TurnRightSeqId   int

	WalkSeqId      int
	WalkBackSeqId  int
	WalkLeftSeqId  int
	WalkRightSeqId int

	RecolorFrom   []int
	RecolorTo     []int
	RetextureFrom []int
	RetextureTo   []int

	Actions []string

	DrawMapDot bool

	CombatLevel int

	// Combat stats from opcodes 74..79, -1 when absent.
	AttackLevel   int
	DefenceLevel  int
	StrengthLevel int
	Hitpoints     int
	RangedLevel   int
	MagicLevel    int
	// AttackSpeed is in game ticks; -1 means use the combat def or
	// fall back to 4.
	AttackSpeed int

	WidthScale  int
	HeightScale int

	IsVisible    bool
	DrawPriority NpcDrawPriority

	Ambient  int
	Contrast int

	HeadIconPrayer        int
	HeadIconSpriteIds     []int
	HeadIconSpriteIndices []int

	RotationSpeed     int
	CanHideForOverlap bool
	OverlapTintHsl    int

	Transforms      []int
	TransformVarbit int
	TransformVarp   int

	IsInteractable bool
	IsClickable    bool
	IsClipped      bool
	IsFollower     bool

	RunSeqId      int
	RunBackSeqId  int
	RunLeftSeqId  int
	RunRightSeqId int

	CrawlSeqId      int
	CrawlBackSeqId  int
	CrawlLeftSeqId  int
	CrawlRightSeqId int

	Category int

	LoginScreenProps int
	SpawnDirection   int
	HeightOffset     int
	FootprintSize    int

	BasTypeId int

	Params ParamsMap
	IsPet  bool
}

// NewNpcType returns a default-initialized NpcType, mirroring the
// constructor (NpcType.ts:104-157).
func NewNpcType(id int, info CacheInfo) *NpcType {
	return &NpcType{
		baseType:        newBaseType(id, info),
		Name:            "null",
		Size:            1,
		IdleSeqId:       -1,
		TurnLeftSeqId:   -1,
		TurnRightSeqId:  -1,
		WalkSeqId:       -1,
		WalkBackSeqId:   -1,
		WalkLeftSeqId:   -1,
		WalkRightSeqId:  -1,
		Actions:         make([]string, 5),
		DrawMapDot:      true,
		CombatLevel:     -1,
		AttackLevel:     -1,
		DefenceLevel:    -1,
		StrengthLevel:   -1,
		Hitpoints:       -1,
		RangedLevel:     -1,
		MagicLevel:      -1,
		AttackSpeed:     -1,
		WidthScale:      128,
		HeightScale:     128,
		DrawPriority:    NpcDrawPriorityDefault,
		HeadIconPrayer:  -1,
		RotationSpeed:   32,
		OverlapTintHsl:  -1,
		TransformVarbit: -1,
		TransformVarp:   -1,
		IsInteractable:  true,
		IsClickable:     true,
		IsClipped:       true,
		RunSeqId:        -1,
		RunBackSeqId:    -1,
		RunLeftSeqId:    -1,
		RunRightSeqId:   -1,
		CrawlSeqId:      -1,
		CrawlBackSeqId:  -1,
		CrawlLeftSeqId:  -1,
		CrawlRightSeqId: -1,
		Category:        -1,
		SpawnDirection:  6,
		HeightOffset:    -1,
		FootprintSize:   -1,
		BasTypeId:       -1,
	}
}

// Decode runs the opcode loop over one file's bytes.
func (n *NpcType) Decode(data []byte) error {
	return n.DecodeFrom(NewByteBuffer(data))
}

// DecodeFrom runs the opcode loop over buf.
func (n *NpcType) DecodeFrom(buf *ByteBuffer) error { return decodeLoop(n, buf) }

// Post derives FootprintSize when unset, mirroring NpcType.post
// (NpcType.ts:535-539).
func (n *NpcType) Post() {
	if n.FootprintSize < 0 {
		size := n.Size
		if size < 1 {
			size = 1
		}
		n.FootprintSize = int(0.4 * float64(size*128))
	}
}

// DecodeNpcType decodes one NPC definition file.
func DecodeNpcType(id int, info CacheInfo, data []byte) (*NpcType, error) {
	n := NewNpcType(id, info)
	if err := n.Decode(data); err != nil {
		return n, err
	}
	n.Post()
	return n, nil
}

// DecodeOpcode decodes one opcode's payload, mirroring NpcType
// decodeOpcode (NpcType.ts:163-533). Unknown opcodes are errors.
// Payloads the server discards (sounds, cursors, shadows, map
// functions) are still read past, exactly as in TypeScript.
func (n *NpcType) DecodeOpcode(opcode int, buf *ByteBuffer) error {
	if opcode == 1 {
		count, err := buf.ReadUnsignedByte()
		if err != nil {
			return err
		}
		n.ModelIds = make([]int, count)
		for i := 0; i < count; i++ {
			var v int
			var err error
			if n.isLargeModelID() {
				v, err = buf.ReadBigSmart()
			} else {
				v, err = buf.ReadUnsignedShort()
			}
			if err != nil {
				return err
			}
			n.ModelIds[i] = v
		}
	} else if opcode == 2 {
		s, err := n.readString(buf)
		if err != nil {
			return err
		}
		n.Name = s
	} else if opcode == 3 {
		s, err := n.readString(buf)
		if err != nil {
			return err
		}
		n.Desc = &s
	} else if opcode == 12 {
		v, err := buf.ReadUnsignedByte()
		if err != nil {
			return err
		}
		n.Size = v
	} else if opcode == 13 {
		v, err := buf.ReadUnsignedShort()
		if err != nil {
			return err
		}
		n.IdleSeqId = v
	} else if opcode == 14 {
		v, err := buf.ReadUnsignedShort()
		if err != nil {
			return err
		}
		n.WalkSeqId = v
	} else if opcode == 15 {
		v, err := buf.ReadUnsignedShort()
		if err != nil {
			return err
		}
		n.TurnLeftSeqId = v
	} else if opcode == 16 {
		if n.cacheInfo.Game == GameRunescape && n.cacheInfo.Revision < 254 {
			// disposeAlpha? No payload is read.
		} else {
			v, err := buf.ReadUnsignedShort()
			if err != nil {
				return err
			}
			n.TurnRightSeqId = v
		}
	} else if opcode == 17 {
		walk, err := buf.ReadUnsignedShort()
		if err != nil {
			return err
		}
		back, err := buf.ReadUnsignedShort()
		if err != nil {
			return err
		}
		left, err := buf.ReadUnsignedShort()
		if err != nil {
			return err
		}
		right, err := buf.ReadUnsignedShort()
		if err != nil {
			return err
		}
		n.WalkSeqId, n.WalkBackSeqId, n.WalkLeftSeqId, n.WalkRightSeqId = walk, back, left, right
	} else if opcode == 18 {
		v, err := buf.ReadUnsignedShort()
		if err != nil {
			return err
		}
		n.Category = v
	} else if opcode >= 30 && opcode < 35 {
		s, err := n.readString(buf)
		if err != nil {
			return err
		}
		if strings.ToLower(s) == "hidden" {
			s = ""
		}
		n.Actions[opcode-30] = s
	} else if opcode == 40 {
		count, err := buf.ReadUnsignedByte()
		if err != nil {
			return err
		}
		n.RecolorFrom = make([]int, count)
		n.RecolorTo = make([]int, count)
		for i := 0; i < count; i++ {
			from, err := buf.ReadUnsignedShort()
			if err != nil {
				return err
			}
			to, err := buf.ReadUnsignedShort()
			if err != nil {
				return err
			}
			n.RecolorFrom[i] = from
			n.RecolorTo[i] = to
		}
	} else if opcode == 41 {
		count, err := buf.ReadUnsignedByte()
		if err != nil {
			return err
		}
		n.RetextureFrom = make([]int, count)
		n.RetextureTo = make([]int, count)
		for i := 0; i < count; i++ {
			from, err := buf.ReadUnsignedShort()
			if err != nil {
				return err
			}
			to, err := buf.ReadUnsignedShort()
			if err != nil {
				return err
			}
			n.RetextureFrom[i] = from
			n.RetextureTo[i] = to
		}
	} else if opcode == 44 || opcode == 45 {
		if _, err := buf.ReadUnsignedShort(); err != nil {
			return err
		}
	} else if opcode == 60 {
		count, err := buf.ReadUnsignedByte()
		if err != nil {
			return err
		}
		n.ChatheadModelIds = make([]int, count)
		for i := 0; i < count; i++ {
			var v int
			var err error
			if n.isLargeModelID() {
				v, err = buf.ReadBigSmart()
			} else {
				v, err = buf.ReadUnsignedShort()
			}
			if err != nil {
				return err
			}
			n.ChatheadModelIds[i] = v
		}
	} else if opcode == 61 {
		count, err := buf.ReadUnsignedByte()
		if err != nil {
			return err
		}
		n.ModelIds = make([]int, count)
		for i := 0; i < count; i++ {
			v, err := buf.ReadInt()
			if err != nil {
				return err
			}
			n.ModelIds[i] = int(v)
		}
	} else if opcode == 62 {
		count, err := buf.ReadUnsignedByte()
		if err != nil {
			return err
		}
		n.ChatheadModelIds = make([]int, count)
		for i := 0; i < count; i++ {
			v, err := buf.ReadInt()
			if err != nil {
				return err
			}
			n.ChatheadModelIds[i] = int(v)
		}
	} else if opcode >= 74 && opcode <= 79 {
		v, err := buf.ReadUnsignedShort()
		if err != nil {
			return err
		}
		if v == 65535 {
			v = -1
		}
		switch opcode {
		case 74:
			n.AttackLevel = v
		case 75:
			n.DefenceLevel = v
		case 76:
			n.StrengthLevel = v
		case 77:
			n.Hitpoints = v
		case 78:
			n.RangedLevel = v
		case 79:
			n.MagicLevel = v
		}
	} else if opcode == 93 {
		n.DrawMapDot = false
	} else if opcode == 95 {
		v, err := buf.ReadUnsignedShort()
		if err != nil {
			return err
		}
		n.CombatLevel = v
	} else if opcode == 97 {
		v, err := buf.ReadUnsignedShort()
		if err != nil {
			return err
		}
		n.WidthScale = v
	} else if opcode == 98 {
		v, err := buf.ReadUnsignedShort()
		if err != nil {
			return err
		}
		n.HeightScale = v
	} else if opcode == 99 {
		n.DrawPriority = NpcDrawPriorityFirst
	} else if opcode == 100 {
		v, err := buf.ReadSignedByte()
		if err != nil {
			return err
		}
		n.Ambient = v
	} else if opcode == 101 {
		v, err := buf.ReadSignedByte()
		if err != nil {
			return err
		}
		n.Contrast = v * 5
	} else if opcode == 102 {
		if (n.cacheInfo.Game == GameOldschool && n.cacheInfo.Revision < 210) ||
			n.cacheInfo.Game == GameRunescape {
			// Legacy single head-icon value.
			v, err := buf.ReadUnsignedShort()
			if err != nil {
				return err
			}
			n.HeadIconPrayer = v
		} else {
			// OSRS newer format: bitmask of multiple head icons.
			mask, err := buf.ReadUnsignedByte()
			if err != nil {
				return err
			}
			count := 0
			for m := mask; m != 0; m &= m - 1 {
				count++
			}
			n.HeadIconSpriteIds = make([]int, count)
			n.HeadIconSpriteIndices = make([]int, count)
			out := 0
			for m := mask; m != 0; m >>= 1 {
				if m&1 != 0 {
					id, err := buf.ReadBigSmart()
					if err != nil {
						return err
					}
					idx, err := buf.ReadUnsignedSmartMin1()
					if err != nil {
						return err
					}
					n.HeadIconSpriteIds[out] = id
					n.HeadIconSpriteIndices[out] = idx
					out++
				}
			}
		}
	} else if opcode == 103 {
		v, err := buf.ReadUnsignedShort()
		if err != nil {
			return err
		}
		n.RotationSpeed = v
	} else if opcode == 106 || opcode == 118 {
		varbit, err := buf.ReadUnsignedShort()
		if err != nil {
			return err
		}
		if varbit == 65535 {
			varbit = -1
		}
		n.TransformVarbit = varbit
		varp, err := buf.ReadUnsignedShort()
		if err != nil {
			return err
		}
		if varp == 65535 {
			varp = -1
		}
		n.TransformVarp = varp
		var3 := -1
		if opcode == 118 {
			v, err := buf.ReadUnsignedShort()
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
		n.Transforms = make([]int, count+2)
		for i := 0; i <= count; i++ {
			v, err := buf.ReadUnsignedShort()
			if err != nil {
				return err
			}
			if v == 65535 {
				v = -1
			}
			n.Transforms[i] = v
		}
		n.Transforms[count+1] = var3
	} else if opcode == 107 {
		n.IsInteractable = false
	} else if opcode == 109 {
		n.IsClipped = false
	} else if opcode == 111 {
		if n.cacheInfo.Game == GameOldschool {
			n.DrawPriority = NpcDrawPriorityLast
		} else {
			// hasShadow = false; no field, no payload.
		}
	} else if opcode == 112 {
		// old; no payload.
	} else if opcode == 113 {
		if _, err := buf.ReadUnsignedShort(); err != nil {
			return err
		}
		if _, err := buf.ReadUnsignedShort(); err != nil {
			return err
		}
	} else if opcode == 114 {
		if n.cacheInfo.Game == GameOldschool {
			v, err := buf.ReadUnsignedShort()
			if err != nil {
				return err
			}
			n.RunSeqId = v
		} else {
			if _, err := buf.ReadSignedByte(); err != nil {
				return err
			}
			if _, err := buf.ReadSignedByte(); err != nil {
				return err
			}
		}
	} else if opcode == 115 {
		if n.cacheInfo.Game == GameOldschool {
			run, err := buf.ReadUnsignedShort()
			if err != nil {
				return err
			}
			back, err := buf.ReadUnsignedShort()
			if err != nil {
				return err
			}
			left, err := buf.ReadUnsignedShort()
			if err != nil {
				return err
			}
			right, err := buf.ReadUnsignedShort()
			if err != nil {
				return err
			}
			n.RunSeqId, n.RunBackSeqId, n.RunLeftSeqId, n.RunRightSeqId = run, back, left, right
		} else {
			if _, err := buf.ReadUnsignedByte(); err != nil {
				return err
			}
			if _, err := buf.ReadUnsignedByte(); err != nil {
				return err
			}
		}
	} else if opcode == 116 {
		v, err := buf.ReadUnsignedShort()
		if err != nil {
			return err
		}
		n.CrawlSeqId = v
	} else if opcode == 117 {
		crawl, err := buf.ReadUnsignedShort()
		if err != nil {
			return err
		}
		back, err := buf.ReadUnsignedShort()
		if err != nil {
			return err
		}
		left, err := buf.ReadUnsignedShort()
		if err != nil {
			return err
		}
		right, err := buf.ReadUnsignedShort()
		if err != nil {
			return err
		}
		n.CrawlSeqId, n.CrawlBackSeqId, n.CrawlLeftSeqId, n.CrawlRightSeqId = crawl, back, left, right
	} else if opcode == 119 {
		v, err := buf.ReadSignedByte()
		if err != nil {
			return err
		}
		n.LoginScreenProps = v
	} else if opcode == 121 {
		// Model offsets are read past and discarded in TypeScript
		// (a local array that is never stored). Note the TypeScript
		// code reads this.modelIds.length first and would throw
		// when no models were decoded; here the offsets are simply
		// consumed, which leaves the same observable state.
		count, err := buf.ReadUnsignedByte()
		if err != nil {
			return err
		}
		for i := 0; i < count; i++ {
			if _, err := buf.ReadUnsignedByte(); err != nil {
				return err
			}
			for j := 0; j < 3; j++ {
				if _, err := buf.ReadSignedByte(); err != nil {
					return err
				}
			}
		}
	} else if opcode == 122 {
		if n.cacheInfo.Game == GameOldschool {
			n.IsFollower = true
		} else {
			if n.isLargeModelID() {
				if _, err := buf.ReadBigSmart(); err != nil {
					return err
				}
			} else {
				if _, err := buf.ReadUnsignedShort(); err != nil {
					return err
				}
			}
		}
	} else if opcode == 123 {
		if n.cacheInfo.Game == GameOldschool {
			// lowPriorityFollowerOps = true; no field, no payload.
		} else {
			if _, err := buf.ReadUnsignedShort(); err != nil {
				return err
			}
		}
	} else if opcode == 124 {
		v, err := buf.ReadUnsignedShort()
		if err != nil {
			return err
		}
		if v == 0xffff {
			v = -1
		}
		n.HeightOffset = v
	} else if opcode == 125 {
		v, err := buf.ReadSignedByte()
		if err != nil {
			return err
		}
		n.SpawnDirection = v
	} else if opcode == 126 {
		v, err := buf.ReadUnsignedShort()
		if err != nil {
			return err
		}
		n.FootprintSize = v
	} else if opcode == 127 {
		v, err := buf.ReadUnsignedShort()
		if err != nil {
			return err
		}
		n.BasTypeId = v
	} else if opcode == 128 {
		if _, err := buf.ReadUnsignedByte(); err != nil {
			return err
		}
	} else if opcode == 130 {
		// readyanimduringanim = true; no field, no payload.
	} else if opcode == 134 {
		for i := 0; i < 4; i++ {
			if _, err := buf.ReadUnsignedShort(); err != nil {
				return err
			}
		}
	} else if opcode == 135 || opcode == 136 {
		if _, err := buf.ReadUnsignedByte(); err != nil {
			return err
		}
		if _, err := buf.ReadUnsignedShort(); err != nil {
			return err
		}
	} else if opcode == 137 {
		if _, err := buf.ReadUnsignedShort(); err != nil {
			return err
		}
	} else if opcode == 138 || opcode == 139 {
		if n.isLargeModelID() {
			if _, err := buf.ReadBigSmart(); err != nil {
				return err
			}
		} else {
			if _, err := buf.ReadUnsignedShort(); err != nil {
				return err
			}
		}
	} else if opcode == 140 {
		if _, err := buf.ReadUnsignedByte(); err != nil {
			return err
		}
	} else if opcode == 141 || opcode == 143 || opcode == 147 || opcode == 158 || opcode == 159 || opcode == 161 || opcode == 162 {
		// Boolean flags with no payload and no stored field.
	} else if opcode == 142 {
		if _, err := buf.ReadUnsignedShort(); err != nil {
			return err
		}
	} else if opcode == 144 {
		if _, err := buf.ReadUnsignedShort(); err != nil {
			return err
		}
	} else if opcode == 145 {
		n.CanHideForOverlap = true
	} else if opcode == 146 {
		v, err := buf.ReadUnsignedShort()
		if err != nil {
			return err
		}
		n.OverlapTintHsl = v
	} else if opcode >= 150 && opcode < 155 {
		// Member-only options. TypeScript sets isMember = true
		// unconditionally, so the action is kept unless it is
		// "hidden".
		s, err := n.readString(buf)
		if err != nil {
			return err
		}
		if strings.ToLower(s) == "hidden" {
			s = ""
		}
		n.Actions[opcode-150] = s
	} else if opcode == 155 {
		for i := 0; i < 4; i++ {
			if _, err := buf.ReadSignedByte(); err != nil {
				return err
			}
		}
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
	} else if opcode == 163 || opcode == 165 || opcode == 168 {
		if _, err := buf.ReadUnsignedByte(); err != nil {
			return err
		}
	} else if opcode == 164 {
		if _, err := buf.ReadUnsignedShort(); err != nil {
			return err
		}
		if _, err := buf.ReadUnsignedShort(); err != nil {
			return err
		}
	} else if opcode >= 170 && opcode < 176 {
		if _, err := buf.ReadUnsignedShort(); err != nil {
			return err
		}
	} else if opcode == 251 {
		if _, err := buf.ReadUnsignedByte(); err != nil {
			return err
		}
		if _, err := buf.ReadUnsignedByte(); err != nil {
			return err
		}
		if _, err := n.readString(buf); err != nil {
			return err
		}
	} else if opcode == 252 {
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
		if _, err := n.readString(buf); err != nil {
			return err
		}
	} else if opcode == 253 {
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
		if _, err := n.readString(buf); err != nil {
			return err
		}
	} else if opcode == 249 {
		params, err := ReadParamsMap(buf, n.Params)
		if err != nil {
			return err
		}
		n.Params = params
	} else {
		return fmt.Errorf("NpcType: Opcode %d not implemented. ID: %d. cache: %+v", opcode, n.id, n.cacheInfo)
	}
	return nil
}

// GetIdleSeqId returns the BAS idle sequence when the NPC has a
// BAS type, else its own idle sequence. It mirrors
// NpcType.getIdleSeqId (NpcType.ts:541-546).
func (n *NpcType) GetIdleSeqId(loader BasTypeLoader) int {
	if n.BasTypeId != -1 {
		return loader.Load(n.BasTypeId).IdleSeqId
	}
	return n.IdleSeqId
}

// GetWalkSeqId returns the BAS walk sequence when the NPC has a
// BAS type, else its own walk sequence. It mirrors
// NpcType.getWalkSeqId (NpcType.ts:548-553).
func (n *NpcType) GetWalkSeqId(loader BasTypeLoader) int {
	if n.BasTypeId != -1 {
		return loader.Load(n.BasTypeId).WalkSeqId
	}
	return n.WalkSeqId
}

// MovementSeqSet is the set of movement sequences
// GetMovementSeqSet returns (NpcType.ts:555-571).
type MovementSeqSet struct {
	Idle       int
	Walk       int
	WalkBack   int
	WalkLeft   int
	WalkRight  int
	Run        int
	RunBack    int
	RunLeft    int
	RunRight   int
	Crawl      int
	CrawlBack  int
	CrawlLeft  int
	CrawlRight int
	TurnLeft   int
	TurnRight  int
}

// GetMovementSeqSet returns the BAS movement set when the NPC has
// a BAS type, else its own sequences. It mirrors
// NpcType.getMovementSeqSet (NpcType.ts:555-610). Note the BAS
// turn fields come from the BAS idle-left/right sequences.
func (n *NpcType) GetMovementSeqSet(loader BasTypeLoader) MovementSeqSet {
	if n.BasTypeId != -1 {
		bas := loader.Load(n.BasTypeId)
		return MovementSeqSet{
			Idle: bas.IdleSeqId, Walk: bas.WalkSeqId,
			WalkBack: bas.WalkBackSeqId, WalkLeft: bas.WalkLeftSeqId, WalkRight: bas.WalkRightSeqId,
			Run: bas.RunSeqId, RunBack: bas.RunBackSeqId, RunLeft: bas.RunLeftSeqId, RunRight: bas.RunRightSeqId,
			Crawl: bas.CrawlSeqId, CrawlBack: bas.CrawlBackSeqId, CrawlLeft: bas.CrawlLeftSeqId, CrawlRight: bas.CrawlRightSeqId,
			TurnLeft: bas.IdleLeftSeqId, TurnRight: bas.IdleRightSeqId,
		}
	}
	return MovementSeqSet{
		Idle: n.IdleSeqId, Walk: n.WalkSeqId,
		WalkBack: n.WalkBackSeqId, WalkLeft: n.WalkLeftSeqId, WalkRight: n.WalkRightSeqId,
		Run: n.RunSeqId, RunBack: n.RunBackSeqId, RunLeft: n.RunLeftSeqId, RunRight: n.RunRightSeqId,
		Crawl: n.CrawlSeqId, CrawlBack: n.CrawlBackSeqId, CrawlLeft: n.CrawlLeftSeqId, CrawlRight: n.CrawlRightSeqId,
		TurnLeft: n.TurnLeftSeqId, TurnRight: n.TurnRightSeqId,
	}
}

// Transform resolves one morph step: the target NPC for the current
// varbit/varp value, or nil when this NPC has no transforms or the
// target is -1. It mirrors NpcType.transform
// (NpcType.ts:612-633). The default target is the last transforms
// entry; an in-range var value selects an earlier one.
func (n *NpcType) Transform(vars VarReader, loader NpcTypeLoader) *NpcType {
	if len(n.Transforms) == 0 {
		return nil
	}
	transformIndex := -1
	if n.TransformVarbit != -1 {
		transformIndex = vars.GetVarbit(n.TransformVarbit)
	} else if n.TransformVarp != -1 {
		transformIndex = vars.GetVarp(n.TransformVarp)
	}
	transformID := n.Transforms[len(n.Transforms)-1]
	if transformIndex >= 0 && transformIndex < len(n.Transforms)-1 {
		transformID = n.Transforms[transformIndex]
	}
	if transformID == -1 {
		return nil
	}
	return loader.Load(transformID)
}

// archivedSeqField returns one of the animation fields the archived
// registry may rebind, by its TypeScript field name
// (ArchivedTypeLoader.ts:554-556).
func (n *NpcType) archivedSeqField(field string) (int, bool) {
	switch field {
	case "idleSeqId":
		return n.IdleSeqId, true
	case "turnLeftSeqId":
		return n.TurnLeftSeqId, true
	case "turnRightSeqId":
		return n.TurnRightSeqId, true
	case "walkSeqId":
		return n.WalkSeqId, true
	case "walkBackSeqId":
		return n.WalkBackSeqId, true
	case "walkLeftSeqId":
		return n.WalkLeftSeqId, true
	case "walkRightSeqId":
		return n.WalkRightSeqId, true
	case "runSeqId":
		return n.RunSeqId, true
	case "runBackSeqId":
		return n.RunBackSeqId, true
	case "runLeftSeqId":
		return n.RunLeftSeqId, true
	case "runRightSeqId":
		return n.RunRightSeqId, true
	case "crawlSeqId":
		return n.CrawlSeqId, true
	case "crawlBackSeqId":
		return n.CrawlBackSeqId, true
	case "crawlLeftSeqId":
		return n.CrawlLeftSeqId, true
	case "crawlRightSeqId":
		return n.CrawlRightSeqId, true
	default:
		return 0, false
	}
}

func (n *NpcType) setArchivedSeqField(field string, value int) bool {
	switch field {
	case "idleSeqId":
		n.IdleSeqId = value
	case "turnLeftSeqId":
		n.TurnLeftSeqId = value
	case "turnRightSeqId":
		n.TurnRightSeqId = value
	case "walkSeqId":
		n.WalkSeqId = value
	case "walkBackSeqId":
		n.WalkBackSeqId = value
	case "walkLeftSeqId":
		n.WalkLeftSeqId = value
	case "walkRightSeqId":
		n.WalkRightSeqId = value
	case "runSeqId":
		n.RunSeqId = value
	case "runBackSeqId":
		n.RunBackSeqId = value
	case "runLeftSeqId":
		n.RunLeftSeqId = value
	case "runRightSeqId":
		n.RunRightSeqId = value
	case "crawlSeqId":
		n.CrawlSeqId = value
	case "crawlBackSeqId":
		n.CrawlBackSeqId = value
	case "crawlLeftSeqId":
		n.CrawlLeftSeqId = value
	case "crawlRightSeqId":
		n.CrawlRightSeqId = value
	default:
		return false
	}
	return true
}

// NewArchiveNpcLoader builds the archive-backed NPC loader. It is
// ArchiveNpcTypeLoader (NpcTypeLoader.ts:654-658).
func NewArchiveNpcLoader(info CacheInfo, archive *Archive) *ArchiveTypeLoader[*NpcType] {
	return NewArchiveTypeLoader(NewNpcType, info, archive)
}

// NewIndexNpcLoader builds the index-backed NPC loader. It is
// IndexNpcTypeLoader (NpcTypeLoader.ts:660-664), with 7 file-id
// bits.
func NewIndexNpcLoader(info CacheInfo, index ArchiveSource) *IndexTypeLoader[*NpcType] {
	return NewIndexTypeLoader(NewNpcType, info, index, 7)
}

// LoadDatNpcTypes decodes the legacy indexed .dat/.idx NPC loader.
// It is DatNpcTypeLoader.load (NpcTypeLoader.ts:648-652).
func LoadDatNpcTypes(info CacheInfo, configArchive *Archive) (*IndexedDatTypeLoader[*NpcType], error) {
	return LoadIndexedDatTypes(NewNpcType, info, configArchive, "npc")
}
