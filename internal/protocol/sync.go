package protocol

import (
	"encoding/binary"
	"math"
)

// Sync views, sync state, and the player/NPC sync encoders, ported
// line-by-line from ClientProtocol.ts (TSPS server, commit bfd81e3).
// The TS BitWriter takes writeBits(count, value); the Go BitWriter takes
// WriteBits(value, n), so every call site below swaps the two arguments
// (see the comments on the individual encoders). Optional TS fields are
// pointers in Go where absence differs from the zero value (a face
// direction of 0 is valid, an absent one writes no block); where the TS
// default equals the Go zero value the field is a plain value and the
// comment says so.

// Tile is a world tile (ClientProtocol.ts line 68).
type Tile struct {
	X     int
	Y     int
	Level int
}

// HitsplatView is one hitsplat in an update block (ClientProtocol.ts line
// 70). Delay defaults to 0, matching the TS `delay ?? 0`.
type HitsplatView struct {
	Type   int
	Damage int
	Delay  int
}

// HealthBar identifies a non-default hitpoints bar (ClientProtocol.ts line
// 72): its config id and width. A nil Bar means the default bar (id 0,
// width 30).
type HealthBar struct {
	ID    int
	Width int
}

// HealthView is an actor's hitpoints (ClientProtocol.ts line 72).
type HealthView struct {
	Current int
	Max     int
	Bar     *HealthBar
}

// HeadbarView is a headbar other than the hitpoints one (ClientProtocol.ts
// line 78). Fill/EndFill/Duration/Delay are pointers because TS defaults
// each with ?? and duration 0 suppresses the end-fill byte; Remove takes
// the bar away.
type HeadbarView struct {
	ID       int
	Fill     *int
	EndFill  *int
	Duration *int
	Delay    *int
	Remove   bool
}

// AnimationView is a sequence id plus its delay (ClientProtocol.ts line
// 79). An id of -1 clears the current sequence.
type AnimationView struct {
	ID    int
	Delay int
}

// GraphicView is a spotanim on an actor (ClientProtocol.ts line 81). Slot
// defaults to 0, matching the TS `slot ?? 0`.
type GraphicView struct {
	ID     int
	Height int
	Delay  int
	Slot   int
}

// ForcedMovementView is a glide from one tile delta to another
// (ClientProtocol.ts lines 82-90).
type ForcedMovementView struct {
	StartDeltaX      int
	StartDeltaY      int
	EndDeltaX        int
	EndDeltaY        int
	StartCycleOffset int
	EndCycleOffset   int
	Direction        int
}

// TintView is an OSRS HSL tint over client cycles (the inline `tint`
// shape on PlayerView/NpcView, ClientProtocol.ts lines 111 and 216).
// Hue/saturation/lightness of -1 keep the model's own values.
type TintView struct {
	StartCycle int
	EndCycle   int
	Hue        int
	Saturation int
	Lightness  int
	Weight     int
}

// ActorUpdateView is the update-block content shared by players and NPCs
// (ClientProtocol.ts lines 92-101). ForcedChat and InteractionIndex are
// pointers because presence itself sets a mask bit; Animation, Health,
// and the graphics/hits/bars lists follow the same rule, except that an
// empty non-nil Hits slice still sets the HIT mask bit (a JS array is
// truthy even when empty, so `view.hits ? HIT : 0` fires), while Bars
// and Graphics only count when non-empty (`?.length`). A nil Hits means
// absent.
type ActorUpdateView struct {
	ForcedChat       *string
	InteractionIndex *int
	Animation        *AnimationView
	Graphics         []GraphicView
	Hits             []HitsplatView
	Health           *HealthView
	// Bars is NPCs only: headbars besides the hitpoints one, sent with
	// the hit block.
	Bars []HeadbarView
}

// PlayerView is one visible player for a sync frame (ClientProtocol.ts
// lines 103-117). Appearance is the raw block from
// EncodePlayerAppearance; it is only written when the player is added or
// AppearanceDirty is set. MovementType is 1 (walk) or 2 (run); 0 means
// absent and the encoder derives it from the step distance, matching the
// TS `movementType ?? (distance === 2 ? 2 : 1)`.
type PlayerView struct {
	Tile
	ActorUpdateView
	Index             int
	Appearance        []byte
	MovementType      int
	ResetPath         bool
	AppearanceDirty   bool
	FaceDirection     *int
	ForcedMovement    *ForcedMovementView
	ForcedMovementEnd *Tile
	Tint              *TintView
	// WorldView is the world entity (boat) whose deck the player stands
	// on; nil means the main world.
	WorldView    *int
	FinePosition *FinePositionView
}

// FinePositionView is a smooth-movement position update, carried after
// the player sync payload (ClientProtocol.ts lines 119-123). Blocked and
// Snap default to false, matching the TS `!!` coercion.
type FinePositionView struct {
	Index    int
	Seq      int
	X        float64
	Y        float64
	Rotation int
	Level    int
	Active   bool
	Moving   bool
	Running  bool
	Blocked  bool
	Snap     bool
}

// finePositionPayload renders one fine position as its 18-byte payload
// (finePositionPayload, ClientProtocol.ts lines 125-135). It is shared by
// EncodeFinePosition and the fine-position trailer of EncodePlayerSync.
func finePositionPayload(position *FinePositionView) []byte {
	out := make([]byte, 18)
	binary.BigEndian.PutUint16(out[0:2], uint16(position.Index))
	binary.BigEndian.PutUint32(out[2:6], uint32(position.Seq))
	// TS rounds x*256/y*256 with Math.round; math.Round matches it
	// exactly (half toward +Infinity).
	binary.BigEndian.PutUint32(out[6:10], uint32(math.Round(position.X*256)))
	binary.BigEndian.PutUint32(out[10:14], uint32(math.Round(position.Y*256)))
	binary.BigEndian.PutUint16(out[14:16], uint16(position.Rotation&2047))
	out[16] = byte(position.Level & 3)
	flags := 0
	if position.Active {
		flags |= 1
	}
	if position.Moving {
		flags |= 2
	}
	if position.Running {
		flags |= 4
	}
	if position.Blocked {
		flags |= 8
	}
	if position.Snap {
		flags |= 16
	}
	out[17] = byte(flags)
	return out
}

// EncodeFinePosition encodes the MOVEMENT_POSITION packet (opcode 23,
// fixed length 18) for one fine position (encodeFinePosition,
// ClientProtocol.ts lines 137-139).
func EncodeFinePosition(position *FinePositionView) []byte {
	return mustEncode(ServerMovementPosition, finePositionPayload(position))
}

// PlayerSyncState is the per-viewer memory encodePlayerSync updates in
// place (ClientProtocol.ts lines 177-196). All slices are 2048 entries,
// matching the TS typed arrays; maps hold only known players. Use
// CreatePlayerSyncState to build one.
type PlayerSyncState struct {
	Flags              []uint8
	Active             []uint16
	ActiveCount        int
	Empty              []uint16
	EmptyCount         int
	Regions            []int32
	LastTiles          map[int]Tile
	MovementTypes      map[int]int
	InteractionIndices map[int]int
	// WorldViews maps a known player to the world view last sent for
	// them, or -1 for the main world.
	WorldViews         map[int]int
	ViewPositions      []int16
	MovementChanged    []uint8
	MovementDx         []int16
	MovementDy         []int16
	MovementPlaneDelta []uint8
	NextMovementTypes  []uint8
	UpdateBlocks       [][]byte
}

// FaceTile is a tile an NPC turns to once (the inline `faceTile` shape on
// NpcView, ClientProtocol.ts line 207).
type FaceTile struct {
	X int
	Y int
}

// ModelCustomisationEntry is one body-model slot override (ClientProtocol.ts
// line 209).
type ModelCustomisationEntry struct {
	Slot    int
	ModelID int
}

// BaseAnimationEntry is one stand/walk/turn sequence override: slot 0
// idle, 1 walk, 2 turn left, 3 turn right (ClientProtocol.ts line 211).
type BaseAnimationEntry struct {
	Slot  int
	SeqID int
}

// TransparencyView is an NPC model fade (ClientProtocol.ts line 213):
// From (-1: its current look) to To, 0 opaque to 255 invisible.
type TransparencyView struct {
	StartCycles int
	EndCycles   int
	From        int
	To          int
}

// NpcView is one visible NPC for a sync frame (ClientProtocol.ts lines
// 198-222). HeadIcon is an index in headicons_prayer (-1, or nil, means
// none). ModelCustomisation and BaseAnimations are nil-absent: a
// non-nil empty slice writes a zero-count block that resets the
// overrides, matching the TS truthiness check. WalkDirection and
// RunDirection are -1 when the NPC does not step.
type NpcView struct {
	Tile
	ActorUpdateView
	Index              int
	TypeID             int
	HeadIcon           *int
	ExactMove          *ForcedMovementView
	FaceTile           *FaceTile
	ModelCustomisation []ModelCustomisationEntry
	BaseAnimations     []BaseAnimationEntry
	Transparency       *TransparencyView
	Tint               *TintView
	Crawl              bool
	WorldView          *int
	Rotation           int
	WalkDirection      int
	RunDirection       int
}

// NpcSyncState is the per-viewer memory encodeNpcSync updates in place
// (ClientProtocol.ts lines 224-231). Use CreateNpcSyncState to build
// one.
type NpcSyncState struct {
	Indices            []int
	LastTiles          map[int]Tile
	TypeIDs            map[int]int
	HeadIcons          map[int]int
	InteractionIndices map[int]int
	WorldViews         map[int]int
}

// SkillView is one skill row for the skills packets (ClientProtocol.ts
// lines 251-258).
type SkillView struct {
	ID           int
	XP           int
	BaseLevel    int
	VirtualLevel int
	Boost        int
	CurrentLevel int
}

// CreatePlayerSyncState builds the initial sync state for a viewer
// (createPlayerSyncState, ClientProtocol.ts lines 2945-2978). The local
// player starts active; every other slot starts empty with its flag bit
// set, mirroring the empty-slot skip in the initial sync packet.
func CreatePlayerSyncState(localIndex int, tile Tile) *PlayerSyncState {
	flags := make([]uint8, 2048)
	active := make([]uint16, 2048)
	empty := make([]uint16, 2048)
	active[0] = uint16(localIndex)
	emptyCount := 0
	for index := 1; index < 2048; index++ {
		if index == localIndex {
			continue
		}
		flags[index] = 1
		empty[emptyCount] = uint16(index)
		emptyCount++
	}
	viewPositions := make([]int16, 2048)
	for i := range viewPositions {
		viewPositions[i] = -1
	}
	return &PlayerSyncState{
		Flags:              flags,
		Active:             active,
		ActiveCount:        1,
		Empty:              empty,
		EmptyCount:         emptyCount,
		Regions:            make([]int32, 2048),
		LastTiles:          map[int]Tile{localIndex: tile},
		MovementTypes:      map[int]int{localIndex: 1},
		InteractionIndices: map[int]int{},
		WorldViews:         map[int]int{},
		ViewPositions:      viewPositions,
		MovementChanged:    make([]uint8, 2048),
		MovementDx:         make([]int16, 2048),
		MovementDy:         make([]int16, 2048),
		MovementPlaneDelta: make([]uint8, 2048),
		NextMovementTypes:  make([]uint8, 2048),
		UpdateBlocks:       [][]byte{},
	}
}

// CreateNpcSyncState builds the initial NPC sync state for a viewer
// (createNpcSyncState, ClientProtocol.ts lines 2980-2985).
func CreateNpcSyncState() *NpcSyncState {
	return &NpcSyncState{
		Indices:            []int{},
		LastTiles:          map[int]Tile{},
		TypeIDs:            map[int]int{},
		HeadIcons:          map[int]int{},
		InteractionIndices: map[int]int{},
		WorldViews:         map[int]int{},
	}
}

// EncodeInitialPlayerSync encodes the first PLAYER_SYNC packet after
// login (encodeInitialPlayerSync, ClientProtocol.ts lines 2903-2934):
// a 12-byte header (scene base tile, local index, loop cycle, sync
// length) plus the bit-packed sync. The local player teleports from the
// initial 0,0; the remaining passes contain no matching indices, and the
// fourth pass skips every empty slot after the first. TS BitWriter calls
// are writeBits(count, value); the Go calls below are WriteBits(value,
// count).
func EncodeInitialPlayerSync(localIndex, tileX, tileY, level, loopCycle int) []byte {
	writer := NewBitWriter()
	writer.WriteBits(1, 1) // local player has movement
	writer.WriteBits(0, 1) // no update block; appearance arrived in handshake
	writer.WriteBits(3, 2) // teleport
	writer.WriteBits(1, 1) // absolute displacement from initial 0,0
	packed := uint32(((level & 3) << 28) | ((tileX & 0x3fff) << 14) | (tileY & 0x3fff))
	writer.WriteBits(packed, 30)
	writer.AlignToByte()
	// Passes two and three contain no matching indices. Pass four skips
	// every empty player slot after the first one.
	writer.AlignToByte()
	writer.AlignToByte()
	writer.WriteBits(0, 1)
	writer.WriteBits(3, 2)
	writer.WriteBits(2045, 11)
	sync := writer.Bytes()

	baseX := (tileX - 48) &^ 7
	if baseX < 0 {
		baseX = 0
	}
	baseY := (tileY - 48) &^ 7
	if baseY < 0 {
		baseY = 0
	}
	header := make([]byte, 12)
	binary.BigEndian.PutUint16(header[0:2], uint16(baseX))
	binary.BigEndian.PutUint16(header[2:4], uint16(baseY))
	binary.BigEndian.PutUint16(header[4:6], uint16(localIndex))
	binary.BigEndian.PutUint32(header[6:10], uint32(loopCycle))
	binary.BigEndian.PutUint16(header[10:12], uint16(len(sync)))
	return mustEncode(ServerPlayerSync, append(header, sync...))
}

// runDirections is RUN_DIRECTIONS (ClientProtocol.ts lines 2936-2939):
// the 16 two-step run offsets in wire order, as (dx, dy) pairs instead of
// "dx,dy" strings.
var runDirections = [][2]int{
	{-2, -2}, {-1, -2}, {0, -2}, {1, -2}, {2, -2},
	{-2, -1}, {2, -1},
	{-2, 0}, {2, 0},
	{-2, 1}, {2, 1},
	{-2, 2}, {-1, 2}, {0, 2}, {1, 2}, {2, 2},
}

func runDirectionIndex(dx, dy int) int {
	for i, d := range runDirections {
		if d[0] == dx && d[1] == dy {
			return i
		}
	}
	return -1
}

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func boolBit(b bool) uint32 {
	if b {
		return 1
	}
	return 0
}

// writeSkipCount writes a skip run the way encodePlayerSync does
// (writeSkipCount, ClientProtocol.ts lines 3064-3075): 2-bit kind, then
// 0/5/8/11 bits of count.
func writeSkipCount(writer *BitWriter, count int) {
	if count == 0 {
		writer.WriteBits(0, 2)
	} else if count < 32 {
		writer.WriteBits(1, 2)
		writer.WriteBits(uint32(count), 5)
	} else if count < 256 {
		writer.WriteBits(2, 2)
		writer.WriteBits(uint32(count), 8)
	} else {
		writer.WriteBits(3, 2)
		writer.WriteBits(uint32(count), 11)
	}
}

// Update-block mask bits (PLAYER_MASK/NPC_MASK, ClientProtocol.ts lines
// 3077-3102).
const (
	playerMaskForcedChat    = 0x01
	playerMaskFaceDir       = 0x02
	playerMaskAppearance    = 0x04
	playerMaskAnimation     = 0x08
	playerMaskHit           = 0x20
	playerMaskFaceEntity    = 0x40
	playerMaskForceMovement = 0x400
	playerMaskMovementType  = 0x1000
	playerMaskTint          = 0x200
	playerMaskMovementFlag  = 0x2000
	playerMaskSpotAnim      = 0x10000

	npcMaskFaceEntity         = 0x08
	npcMaskAnimation          = 0x10
	npcMaskHit                = 0x20
	npcMaskForcedChat         = 0x40
	npcMaskHeadIcons          = 0x200
	npcMaskExactMove          = 0x400
	npcMaskFaceTile           = 0x800
	npcMaskModelCustomisation = 0x1000
	npcMaskBaseAnimations     = 0x2000
	npcMaskTransparency       = 0x8000
	npcMaskSpotAnim           = 0x20000
	npcMaskTint               = 0x100
)

// headiconsPrayerArchive is HEADICONS_PRAYER_ARCHIVE (ClientProtocol.ts
// line 3105): the sprite group an NPC head icon indexes into.
const headiconsPrayerArchive = 440

// The blk* helpers append one encoded value to an update block, mirroring
// the TS number[] helpers (writeMask, byteA/byteC/byteS, shortBE/LE/BEA/
// LEA, intME, smart, writeText; ClientProtocol.ts lines 3107-3175). Each
// masks to its wire width exactly as the TS shifts do, so negative inputs
// wrap the same way.
func blkByte(b []byte, v int) []byte  { return append(b, byte(v&0xff)) }
func blkByteA(b []byte, v int) []byte { return append(b, byte((v+128)&0xff)) }
func blkByteC(b []byte, v int) []byte { return append(b, byte((-v)&0xff)) }
func blkByteS(b []byte, v int) []byte { return append(b, byte((128-v)&0xff)) }
func blkShortBE(b []byte, v int) []byte {
	return append(b, byte((v>>8)&0xff), byte(v&0xff))
}
func blkShortLE(b []byte, v int) []byte {
	return append(b, byte(v&0xff), byte((v>>8)&0xff))
}
func blkShortBEA(b []byte, v int) []byte {
	return append(b, byte((v>>8)&0xff), byte((v+128)&0xff))
}
func blkShortLEA(b []byte, v int) []byte {
	return append(b, byte((v+128)&0xff), byte((v>>8)&0xff))
}
func blkIntME(b []byte, v int) []byte {
	return append(b, byte((v>>8)&0xff), byte(v&0xff), byte((v>>24)&0xff), byte((v>>16)&0xff))
}
func blkSmart(b []byte, v int) []byte {
	safe := v
	if safe < 0 {
		safe = 0
	}
	if safe > 32767 {
		safe = 32767
	}
	if safe < 128 {
		return append(b, byte(safe))
	}
	return blkShortBE(b, safe+32768)
}
func blkText(b []byte, s string) []byte { return append(b, latin1Bytes(s)...) }

// blkMask writes a one-, two-, or three-byte mask (writeMask,
// ClientProtocol.ts lines 3107-3114). The second byte is present when any
// bit above the low byte is set (flagged by 0x80 in the first byte); the
// third byte is present when any bit above the low 16 is set (flagged by
// 0x4000 in the second byte).
func blkMask(b []byte, rawMask int) []byte {
	third := rawMask&0xffff0000 != 0
	second := third || rawMask&0xff00 != 0
	mask := rawMask
	if third {
		mask |= 0x4000
	}
	first := mask & 0xff
	if second {
		first |= 0x80
	}
	b = append(b, byte(first))
	if second {
		b = append(b, byte((mask>>8)&0xff))
	}
	if third {
		b = append(b, byte((mask>>16)&0xff))
	}
	return b
}

// scaledHealth scales current/max into the bar width (scaledHealth,
// ClientProtocol.ts lines 3177-3181): 0 when dead or maxless, otherwise
// clamped to 1..width.
func scaledHealth(health *HealthView) int {
	width := 30
	if health.Bar != nil {
		width = health.Bar.Width
	}
	if health.Current <= 0 || health.Max <= 0 {
		return 0
	}
	value := (health.Current * width) / health.Max
	if value < 1 {
		value = 1
	}
	if value > width {
		value = width
	}
	return value
}

// writeHits writes the hitsplats plus the health/headbar section
// (writeHits, ClientProtocol.ts lines 3183-3205). npc selects the NPC
// byte transforms (byteS/byteA/byteC) versus the player ones
// (byteC/byteC/plain).
func writeHits(b []byte, view *ActorUpdateView, npc bool) []byte {
	count := len(view.Hits)
	if count > 255 {
		count = 255
	}
	if npc {
		b = blkByteS(b, count)
	} else {
		b = blkByteC(b, count)
	}
	for _, hit := range view.Hits[:count] {
		b = blkSmart(b, hit.Type)
		b = blkSmart(b, hit.Damage)
		b = blkSmart(b, hit.Delay)
	}
	var bars []HeadbarView
	if npc {
		bars = view.Bars
	}
	barCount := len(bars)
	if view.Health != nil {
		barCount++
	}
	if barCount > 255 {
		barCount = 255
	}
	if npc {
		b = blkByteA(b, barCount)
	} else {
		b = blkByteC(b, barCount)
	}
	if view.Health != nil {
		barID := 0
		if view.Health.Bar != nil {
			barID = view.Health.Bar.ID
		}
		b = blkSmart(b, barID)
		b = blkSmart(b, 0)
		b = blkSmart(b, 0)
		value := scaledHealth(view.Health)
		if npc {
			b = blkByteC(b, value)
		} else {
			b = blkByte(b, value)
		}
	}
	limit := barCount
	if view.Health != nil {
		limit--
	}
	if limit > len(bars) {
		limit = len(bars)
	}
	for _, bar := range bars[:limit] {
		b = writeHeadbar(b, bar)
	}
	return b
}

// writeHeadbar writes one NPC headbar (writeHeadbar, ClientProtocol.ts
// lines 3208-3220): id, duration (32767 removes it), delay, fill and,
// when it moves, its end fill.
func writeHeadbar(b []byte, bar HeadbarView) []byte {
	b = blkSmart(b, bar.ID)
	if bar.Remove {
		return blkSmart(b, 32767)
	}
	duration := 0
	if bar.Duration != nil {
		duration = *bar.Duration
	}
	if duration < 0 {
		duration = 0
	}
	if duration > 32766 {
		duration = 32766
	}
	b = blkSmart(b, duration)
	delay := 0
	if bar.Delay != nil && *bar.Delay > 0 {
		delay = *bar.Delay
	}
	b = blkSmart(b, delay)
	fill := 0
	if bar.Fill != nil {
		fill = *bar.Fill
	}
	b = blkByteC(b, fill&0xff)
	if duration > 0 {
		endFill := fill
		if bar.EndFill != nil {
			endFill = *bar.EndFill
		}
		b = blkByteS(b, endFill&0xff)
	}
	return b
}

// playerUpdateMask computes a player update-block mask
// (playerUpdateMask, ClientProtocol.ts lines 3222-3236).
func playerUpdateMask(view *PlayerView, writeMovementType, writeAppearance, writeInteraction bool) int {
	mask := 0
	if view.ForcedChat != nil {
		mask |= playerMaskForcedChat
	}
	if view.FaceDirection != nil {
		mask |= playerMaskFaceDir
	}
	if writeAppearance {
		mask |= playerMaskAppearance
	}
	if view.Animation != nil {
		mask |= playerMaskAnimation
	}
	if view.Hits != nil {
		mask |= playerMaskHit
	}
	if writeInteraction {
		mask |= playerMaskFaceEntity
	}
	if view.ForcedMovement != nil {
		mask |= playerMaskForceMovement
	}
	if writeMovementType {
		mask |= playerMaskMovementType
	}
	if view.ResetPath {
		mask |= playerMaskMovementFlag
	}
	if view.Tint != nil {
		mask |= playerMaskTint
	}
	if len(view.Graphics) > 0 {
		mask |= playerMaskSpotAnim
	}
	return mask
}

// npcUpdateMask computes an NPC update-block mask (npcUpdateMask,
// ClientProtocol.ts lines 3238-3253).
func npcUpdateMask(view *NpcView, writeInteraction, writeHeadIcon bool) int {
	mask := 0
	if writeInteraction {
		mask |= npcMaskFaceEntity
	}
	if writeHeadIcon {
		mask |= npcMaskHeadIcons
	}
	if view.Animation != nil {
		mask |= npcMaskAnimation
	}
	if view.Hits != nil || len(view.Bars) > 0 {
		mask |= npcMaskHit
	}
	if view.ForcedChat != nil {
		mask |= npcMaskForcedChat
	}
	if view.Tint != nil {
		mask |= npcMaskTint
	}
	if view.ExactMove != nil {
		mask |= npcMaskExactMove
	}
	if view.FaceTile != nil {
		mask |= npcMaskFaceTile
	}
	if view.ModelCustomisation != nil {
		mask |= npcMaskModelCustomisation
	}
	if view.BaseAnimations != nil {
		mask |= npcMaskBaseAnimations
	}
	if view.Transparency != nil {
		mask |= npcMaskTransparency
	}
	if len(view.Graphics) > 0 {
		mask |= npcMaskSpotAnim
	}
	return mask
}

// writeForcedMovement writes the forced-movement fields, as players'
// forced movement writes them (writeForcedMovement, ClientProtocol.ts
// lines 3256-3264). NPC exact movement uses the same layout.
func writeForcedMovement(b []byte, movement *ForcedMovementView) []byte {
	b = blkByteS(b, movement.StartDeltaX)
	b = blkByte(b, movement.StartDeltaY)
	b = blkByte(b, movement.EndDeltaX)
	b = blkByteA(b, movement.EndDeltaY)
	b = blkShortBEA(b, movement.StartCycleOffset)
	b = blkShortBE(b, movement.EndCycleOffset)
	b = blkShortLEA(b, movement.Direction&2047)
	return b
}

// writePlayerUpdateBlock renders one player's update block
// (writePlayerUpdateBlock, ClientProtocol.ts lines 3266-3306). movementType
// is the movement type to write when writeMovementType is set (0 when the
// TS value is undefined, which writes the same byteC(0)).
func writePlayerUpdateBlock(view *PlayerView, writeMovementType bool, movementType int, writeAppearance, writeInteraction bool) []byte {
	var b []byte
	b = blkMask(b, playerUpdateMask(view, writeMovementType, writeAppearance, writeInteraction))
	if view.ForcedChat != nil {
		b = blkText(b, *view.ForcedChat)
	}
	if view.FaceDirection != nil {
		b = blkShortLE(b, *view.FaceDirection&2047)
	}
	if writeInteraction {
		target := 0xffffff
		if view.InteractionIndex != nil && *view.InteractionIndex >= 0 {
			target = *view.InteractionIndex & 0xffffff
		}
		b = blkShortBE(b, target&0xffff)
		b = blkByte(b, (target>>16)&0xff)
	}
	if view.Animation != nil {
		id := view.Animation.ID
		if id < 0 {
			id = 0xffff
		}
		b = blkShortLEA(b, id)
		b = blkByte(b, view.Animation.Delay)
	}
	if view.Hits != nil {
		b = writeHits(b, &view.ActorUpdateView, false)
	}
	if writeMovementType {
		b = blkByteC(b, movementType)
	}
	if writeAppearance {
		length := len(view.Appearance)
		if length > 255 {
			length = 255
		}
		b = blkByteC(b, length)
		b = append(b, view.Appearance[:length]...)
	}
	if view.ForcedMovement != nil {
		b = writeForcedMovement(b, view.ForcedMovement)
	}
	if view.ResetPath {
		b = blkByteS(b, 127)
	}
	if len(view.Graphics) > 0 {
		b = blkByteA(b, len(view.Graphics))
		for _, graphic := range view.Graphics {
			b = blkByte(b, graphic.Slot)
			id := graphic.ID
			if id < 0 {
				id = 0xffff
			}
			b = blkShortBE(b, id)
			b = blkIntME(b, ((graphic.Height&0xffff)<<16)|(graphic.Delay&0xffff))
		}
	}
	if view.Tint != nil {
		b = blkShortLE(b, view.Tint.StartCycle&0xffff)
		b = blkShortLE(b, view.Tint.EndCycle&0xffff)
		b = blkByteS(b, view.Tint.Hue)
		b = blkByte(b, view.Tint.Saturation)
		b = blkByteA(b, view.Tint.Lightness)
		b = blkByteC(b, view.Tint.Weight)
	}
	return b
}

// writeNpcUpdateBlock renders one NPC's update block
// (writeNpcUpdateBlock, ClientProtocol.ts lines 3308-3357). Note the
// field order differs from the player block (tint after the say text,
// graphics before animation), as the client reads NPC blocks.
func writeNpcUpdateBlock(view *NpcView, writeInteraction, writeHeadIcon bool) []byte {
	var b []byte
	b = blkMask(b, npcUpdateMask(view, writeInteraction, writeHeadIcon))
	if writeInteraction {
		target := 0xffffff
		if view.InteractionIndex != nil && *view.InteractionIndex >= 0 {
			target = *view.InteractionIndex & 0xffffff
		}
		b = blkShortLEA(b, target)
		b = blkByteA(b, target>>16)
	}
	if view.Hits != nil || len(view.Bars) > 0 {
		b = writeHits(b, &view.ActorUpdateView, true)
	}
	if view.ForcedChat != nil {
		b = blkText(b, *view.ForcedChat)
	}
	if view.Tint != nil {
		// After the say text, as the client reads an NPC's blocks (not
		// the player block's order).
		b = blkShortBEA(b, view.Tint.StartCycle&0xffff)
		b = blkShortBEA(b, view.Tint.EndCycle&0xffff)
		b = blkByteC(b, view.Tint.Hue)
		b = blkByteS(b, view.Tint.Saturation)
		b = blkByteC(b, view.Tint.Lightness)
		b = blkByteS(b, view.Tint.Weight)
	}
	if len(view.Graphics) > 0 {
		b = blkByte(b, len(view.Graphics))
		for _, graphic := range view.Graphics {
			b = blkByteA(b, graphic.Slot&0xff)
			id := graphic.ID
			if id < 0 {
				id = 0xffff
			}
			b = blkShortLE(b, id)
			b = blkIntME(b, ((graphic.Height&0xffff)<<16)|(graphic.Delay&0xffff))
		}
	}
	if view.Animation != nil {
		id := view.Animation.ID
		if id < 0 {
			id = 0xffff
		}
		b = blkShortBE(b, id)
		b = blkByte(b, view.Animation.Delay)
	}
	if writeHeadIcon {
		// A count of icons, each a sprite group and an index in it
		// (none clears them).
		icon := -1
		if view.HeadIcon != nil {
			icon = *view.HeadIcon
		}
		if icon >= 0 {
			b = blkByte(b, 1)
			b = blkShortBE(b, headiconsPrayerArchive)
			b = blkByte(b, icon)
		} else {
			b = blkByte(b, 0)
		}
	}
	if view.ExactMove != nil {
		b = writeForcedMovement(b, view.ExactMove)
	}
	if view.FaceTile != nil {
		b = blkShortBE(b, view.FaceTile.X&0xffff)
		b = blkShortBE(b, view.FaceTile.Y&0xffff)
	}
	if view.ModelCustomisation != nil {
		b = blkByte(b, len(view.ModelCustomisation))
		for _, entry := range view.ModelCustomisation {
			b = blkByte(b, entry.Slot)
			b = blkShortLE(b, entry.ModelID&0xffff)
		}
	}
	if view.BaseAnimations != nil {
		b = blkByte(b, len(view.BaseAnimations))
		for _, entry := range view.BaseAnimations {
			b = blkByte(b, entry.Slot)
			seqID := entry.SeqID
			if seqID < 0 {
				seqID = 0xffff
			}
			b = blkShortBE(b, seqID)
		}
	}
	if view.Transparency != nil {
		// Start and end cycles into the tick, the transparency it
		// fades from (0xffff: its current look) and to: 0 opaque, 255
		// invisible.
		b = blkShortBE(b, view.Transparency.StartCycles&0xffff)
		b = blkShortBE(b, view.Transparency.EndCycles&0xffff)
		from := view.Transparency.From & 0xff
		if view.Transparency.From < 0 {
			from = 0xffff
		}
		b = blkShortBE(b, from)
		b = blkByte(b, view.Transparency.To)
	}
	return b
}

// packRegion packs a tile into the 32-bit region key encodePlayerSync
// keeps per known player: plane in the top 2 bits, region x/y (tile
// >> 13) in the next bytes (ClientProtocol.ts lines 3482-3484 and
// 3514-3515).
func packRegion(tile Tile) uint32 {
	return ((uint32(tile.Level) & 3) << 28) |
		(((uint32(tile.X) >> 13) & 0xff) << 14) |
		((uint32(tile.Y) >> 13) & 0xff)
}

// EncodePlayerSync encodes one PLAYER_SYNC packet (opcode 20, 2-byte
// length prefix) for a viewer (encodePlayerSync, ClientProtocol.ts lines
// 3359-3565). state is updated in place and must come from
// CreatePlayerSyncState (or a previous EncodePlayerSync call); views
// must include the local player or it panics, mirroring the TS throw.
// The payload is the 12-byte header (scene base, local index, loop
// cycle, sync length), the bit-packed sync plus update blocks, and a
// fine-position trailer (count + 18 bytes each). TS BitWriter calls are
// writeBits(count, value); the Go calls below are WriteBits(value,
// count).
func EncodePlayerSync(localIndex, baseX, baseY, loopCycle int, views []PlayerView, state *PlayerSyncState) []byte {
	writer := NewBitWriter()
	viewPositions := state.ViewPositions
	for i := range viewPositions {
		viewPositions[i] = -1
	}
	for position, view := range views {
		index := view.Index
		if index > 0 && index < 2048 && viewPositions[index] < 0 {
			// Movement can't carry a world view: a known player
			// boarding or leaving a boat is removed this frame and
			// added back with their new world view the next.
			if index != localIndex {
				if _, known := state.LastTiles[index]; known {
					prevWorldView := -1
					if v, ok := state.WorldViews[index]; ok {
						prevWorldView = v
					}
					curWorldView := -1
					if view.WorldView != nil {
						curWorldView = *view.WorldView
					}
					if prevWorldView != curWorldView {
						continue
					}
				}
			}
			viewPositions[index] = int16(position)
		}
	}
	viewAt := func(index int) *PlayerView {
		if index < 0 || index >= len(viewPositions) {
			return nil
		}
		pos := viewPositions[index]
		if pos < 0 {
			return nil
		}
		return &views[pos]
	}
	localView := viewAt(localIndex)
	if localView == nil {
		panic("protocol: player sync is missing its local player")
	}

	for position, view := range views {
		index := view.Index
		if index <= 0 || index >= 2048 || viewPositions[index] != int16(position) {
			continue
		}
		from, known := state.LastTiles[index]
		if !known {
			state.MovementChanged[index] = 1
			state.MovementDx[index] = 0
			state.MovementDy[index] = 0
			state.MovementPlaneDelta[index] = 0
			state.NextMovementTypes[index] = 0
			continue
		}
		tile := view.Tile
		if view.ForcedMovementEnd != nil && view.ForcedMovement == nil {
			tile = *view.ForcedMovementEnd
		}
		dx := tile.X - from.X
		dy := tile.Y - from.Y
		planeDelta := (tile.Level - from.Level) & 3
		distance := max(absInt(dx), absInt(dy))
		if dx != 0 || dy != 0 || planeDelta != 0 {
			state.MovementChanged[index] = 1
		} else {
			state.MovementChanged[index] = 0
		}
		state.MovementDx[index] = int16(dx)
		state.MovementDy[index] = int16(dy)
		state.MovementPlaneDelta[index] = uint8(planeDelta)
		if planeDelta == 0 && distance > 0 && distance <= 2 {
			nextType := view.MovementType
			if nextType == 0 {
				if distance == 2 {
					nextType = 2
				} else {
					nextType = 1
				}
			}
			state.NextMovementTypes[index] = uint8(nextType)
		} else {
			state.NextMovementTypes[index] = 0
		}
	}

	state.UpdateBlocks = state.UpdateBlocks[:0]
	wantsInteractionWrite := func(index int, view *PlayerView) bool {
		prev, ok := state.InteractionIndices[index]
		cur := -1
		if view.InteractionIndex != nil {
			cur = *view.InteractionIndex
		}
		// Absent in the map reads as undefined in TS, which never
		// equals the current index, so the first frame writes it.
		return !ok || prev != cur
	}
	writeMovementTypeFor := func(index int) (bool, int) {
		nextType := int(state.NextMovementTypes[index])
		if nextType == 0 {
			return false, 0
		}
		prev, ok := state.MovementTypes[index]
		return !ok || prev != nextType, nextType
	}
	needsBlock := func(index int) bool {
		view := viewAt(index)
		if view == nil {
			return false
		}
		writeMovementType, _ := writeMovementTypeFor(index)
		return playerUpdateMask(view, writeMovementType, view.AppearanceDirty, wantsInteractionWrite(index, view)) != 0
	}
	shouldUpdatePlayer := func(index int) bool {
		return viewPositions[index] < 0 || state.MovementChanged[index] != 0 || needsBlock(index)
	}
	appendUpdateBlock := func(index int, forceAppearance bool) {
		view := viewAt(index)
		if view == nil {
			return
		}
		writeMovementType, nextType := writeMovementTypeFor(index)
		state.UpdateBlocks = append(state.UpdateBlocks, writePlayerUpdateBlock(
			view,
			writeMovementType,
			nextType,
			forceAppearance || view.AppearanceDirty,
			wantsInteractionWrite(index, view),
		))
	}
	writePlayerUpdate := func(index int) {
		view := viewAt(index)
		block := view != nil && needsBlock(index)
		writer.WriteBits(boolBit(block), 1)
		if view == nil {
			writer.WriteBits(0, 2)
			writer.WriteBits(0, 1)
			if last, ok := state.LastTiles[index]; ok {
				state.Regions[index] = int32(packRegion(last))
			}
			return
		}
		dx := int(state.MovementDx[index])
		dy := int(state.MovementDy[index])
		planeDelta := int(state.MovementPlaneDelta[index])
		if state.MovementChanged[index] == 0 {
			writer.WriteBits(0, 2)
		} else if planeDelta == 0 && max(absInt(dx), absInt(dy)) == 1 && state.NextMovementTypes[index] != 2 {
			direction := [...]int{0, 1, 2, 3, -1, 4, 5, 6, 7}[(dy+1)*3+dx+1]
			writer.WriteBits(1, 2)
			writer.WriteBits(uint32(direction), 3)
		} else {
			runDirection := -1
			if planeDelta == 0 && state.NextMovementTypes[index] == 2 {
				runDirection = runDirectionIndex(dx, dy)
			}
			if runDirection >= 0 {
				writer.WriteBits(2, 2)
				writer.WriteBits(uint32(runDirection), 4)
			} else {
				writer.WriteBits(3, 2)
				if dx >= -16 && dx <= 15 && dy >= -16 && dy <= 15 {
					writer.WriteBits(0, 1)
					writer.WriteBits(uint32((planeDelta<<10)|((dx&0x1f)<<5)|(dy&0x1f)), 12)
				} else {
					writer.WriteBits(1, 1)
					writer.WriteBits(uint32((planeDelta<<28)|((dx&0x3fff)<<14)|(dy&0x3fff)), 30)
				}
			}
		}
		if block {
			appendUpdateBlock(index, false)
		}
	}
	writeExternalUpdate := func(index int) {
		view := viewAt(index)
		packedRegion := packRegion(view.Tile)
		writer.WriteBits(0, 2)
		sameRegion := state.Regions[index] == int32(packedRegion)
		writer.WriteBits(boolBit(!sameRegion), 1)
		if !sameRegion {
			current := uint32(state.Regions[index])
			planeDelta := ((packedRegion >> 28) - (current >> 28)) & 3
			dx := (((packedRegion >> 14) & 0xff) - ((current >> 14) & 0xff)) & 0xff
			dy := ((packedRegion & 0xff) - (current & 0xff)) & 0xff
			writer.WriteBits(3, 2)
			writer.WriteBits((planeDelta<<16)|(dx<<8)|dy, 18)
			state.Regions[index] = int32(packedRegion)
		}
		writer.WriteBits(uint32(view.X&0x1fff), 13)
		writer.WriteBits(uint32(view.Y&0x1fff), 13)
		if view.WorldView != nil {
			writer.WriteBits(1, 1)
			writer.WriteBits(uint32(*view.WorldView), 16)
		} else {
			writer.WriteBits(0, 1)
		}
		writer.WriteBits(1, 1) // appearance follows
		appendUpdateBlock(index, true)
	}
	writePass := func(indices []uint16, count int, wantBit uint8, shouldUpdate func(int) bool, writeUpdate func(int), markUpdated bool) {
		skip := 0
		for offset := 0; offset < count; offset++ {
			index := int(indices[offset])
			if state.Flags[index]&1 != wantBit {
				continue
			}
			if skip > 0 {
				skip--
				state.Flags[index] |= 2
				continue
			}
			if shouldUpdate(index) {
				writer.WriteBits(1, 1)
				writeUpdate(index)
				if markUpdated {
					state.Flags[index] |= 2
				}
				continue
			}
			run := 0
			for next := offset + 1; next < count && run < 2047; next++ {
				nextIndex := int(indices[next])
				if state.Flags[nextIndex]&1 != wantBit {
					continue
				}
				if shouldUpdate(nextIndex) {
					break
				}
				run++
			}
			writer.WriteBits(0, 1)
			writeSkipCount(writer, run)
			state.Flags[index] |= 2
			skip = run
		}
	}

	writePass(state.Active, state.ActiveCount, 0, shouldUpdatePlayer, writePlayerUpdate, false)
	writer.AlignToByte()
	writePass(state.Active, state.ActiveCount, 1, shouldUpdatePlayer, writePlayerUpdate, false)
	writer.AlignToByte()
	hasView := func(index int) bool { return viewPositions[index] >= 0 }
	writePass(state.Empty, state.EmptyCount, 1, hasView, writeExternalUpdate, true)
	writer.AlignToByte()
	writePass(state.Empty, state.EmptyCount, 0, hasView, writeExternalUpdate, true)

	for offset := 0; offset < state.ActiveCount; offset++ {
		index := int(state.Active[offset])
		if viewPositions[index] >= 0 {
			continue
		}
		delete(state.LastTiles, index)
		delete(state.MovementTypes, index)
		delete(state.InteractionIndices, index)
		delete(state.WorldViews, index)
	}

	state.ActiveCount = 0
	state.EmptyCount = 0
	for index := 1; index < 2048; index++ {
		state.Flags[index] >>= 1
		if viewPositions[index] >= 0 {
			state.Active[state.ActiveCount] = uint16(index)
			state.ActiveCount++
		} else {
			state.Empty[state.EmptyCount] = uint16(index)
			state.EmptyCount++
		}
	}

	for offset := 0; offset < state.ActiveCount; offset++ {
		index := int(state.Active[offset])
		view := viewAt(index)
		nextType := int(state.NextMovementTypes[index])
		// Note: unlike the movement pre-pass, the stored tile always
		// prefers forcedMovementEnd when present.
		tile := view.Tile
		if view.ForcedMovementEnd != nil {
			tile = *view.ForcedMovementEnd
		}
		state.LastTiles[index] = tile
		if nextType != 0 {
			state.MovementTypes[index] = nextType
		}
		interactionIndex := -1
		if view.InteractionIndex != nil {
			interactionIndex = *view.InteractionIndex
		}
		state.InteractionIndices[index] = interactionIndex
		worldView := -1
		if view.WorldView != nil {
			worldView = *view.WorldView
		}
		state.WorldViews[index] = worldView
	}

	sync := writer.Bytes()
	for _, block := range state.UpdateBlocks {
		sync = append(sync, block...)
	}
	header := make([]byte, 12)
	binary.BigEndian.PutUint16(header[0:2], uint16(baseX))
	binary.BigEndian.PutUint16(header[2:4], uint16(baseY))
	binary.BigEndian.PutUint16(header[4:6], uint16(localIndex))
	binary.BigEndian.PutUint32(header[6:10], uint32(loopCycle))
	binary.BigEndian.PutUint16(header[10:12], uint16(len(sync)))
	var fine [][]byte
	for i := range views {
		if views[i].FinePosition != nil {
			fine = append(fine, finePositionPayload(views[i].FinePosition))
		}
	}
	payload := append(header, sync...)
	count := make([]byte, 2)
	binary.BigEndian.PutUint16(count, uint16(len(fine)))
	payload = append(payload, count...)
	for _, f := range fine {
		payload = append(payload, f...)
	}
	return mustEncode(ServerPlayerSync, payload)
}

// EncodeNpcSync encodes one NPC_INFO packet (opcode 21, 2-byte length
// prefix) for a viewer (encodeNpcSync, ClientProtocol.ts lines
// 3611-3747). NOTE: the TS function is named encodeNpcSync; PORT_STATUS.md
// calls it encodeNpcInfo after the opcode, but the function name is
// encodeNpcSync and this port keeps it.
//
// state is updated in place and must come from CreateNpcSyncState (or a
// previous EncodeNpcSync call). local is the root tile new main-world
// NPCs are placed relative to. deckLocal is the player's deck tile when
// they stand on a boat, or nil; deckBase resolves a boat world view to
// its deck base tile, or nil when unknown (nil deckBase, or a nil return,
// falls back to deckLocal, then local, matching the TS `??` chain).
// crossFloor keeps NPCs on other planes visible instead of dropping them.
// TS BitWriter calls are writeBits(count, value); the Go calls below are
// WriteBits(value, count).
func EncodeNpcSync(loopCycle int, local Tile, views []NpcView, state *NpcSyncState, deckLocal *Tile, deckBase func(worldView int) *Tile, crossFloor bool) []byte {
	writer := NewBitWriter()
	desired := make(map[int]*NpcView, len(views))
	var ordered []*NpcView
	for i := range views {
		view := &views[i]
		if view.Index >= 0 && view.Index < 0xffff {
			if _, ok := desired[view.Index]; !ok {
				desired[view.Index] = view
				ordered = append(ordered, view)
			}
		}
	}
	var nextIndices []int
	readd := map[int]bool{}
	var updateBlocks [][]byte

	knownCount := len(state.Indices)
	if knownCount > 255 {
		knownCount = 255
	}
	writer.WriteBits(uint32(knownCount), 8)
	for _, index := range state.Indices[:knownCount] {
		view := desired[index]
		last, hasLast := state.LastTiles[index]
		remove := false
		switch {
		case view == nil || !hasLast:
			remove = true
		case crossFloor && view.Level != last.Level:
			remove = true
		case !crossFloor && view.Level != local.Level:
			remove = true
		default:
			typeID, hasType := state.TypeIDs[index]
			prevWorldView := -1
			if v, ok := state.WorldViews[index]; ok {
				prevWorldView = v
			}
			curWorldView := -1
			if view.WorldView != nil {
				curWorldView = *view.WorldView
			}
			remove = !hasType || typeID != view.TypeID || prevWorldView != curWorldView
		}
		if remove {
			writer.WriteBits(1, 1)
			writer.WriteBits(3, 2)
			if view != nil {
				readd[index] = true
			}
			continue
		}
		writeInteraction := npcWantsInteractionWrite(state, index, view)
		writeHeadIcon := npcWantsHeadIconWrite(state, index, view)
		block := npcUpdateMask(view, writeInteraction, writeHeadIcon) != 0
		switch {
		case view.RunDirection >= 0 && view.WalkDirection >= 0:
			writer.WriteBits(1, 1)
			writer.WriteBits(2, 2)
			writer.WriteBits(1, 1)
			writer.WriteBits(uint32(view.WalkDirection), 3)
			writer.WriteBits(uint32(view.RunDirection), 3)
			writer.WriteBits(boolBit(block), 1)
			nextIndices = append(nextIndices, index)
		case view.WalkDirection >= 0 && view.Crawl:
			// A crawl: the run form of the step with its run flag
			// clear (half walking speed).
			writer.WriteBits(1, 1)
			writer.WriteBits(2, 2)
			writer.WriteBits(0, 1)
			writer.WriteBits(uint32(view.WalkDirection), 3)
			writer.WriteBits(boolBit(block), 1)
			nextIndices = append(nextIndices, index)
		case view.WalkDirection >= 0:
			writer.WriteBits(1, 1)
			writer.WriteBits(1, 2)
			writer.WriteBits(uint32(view.WalkDirection), 3)
			writer.WriteBits(boolBit(block), 1)
			nextIndices = append(nextIndices, index)
		case view.X == last.X && view.Y == last.Y:
			if block {
				writer.WriteBits(1, 1)
				writer.WriteBits(0, 2)
			} else {
				writer.WriteBits(0, 1)
			}
			nextIndices = append(nextIndices, index)
		default:
			writer.WriteBits(1, 1)
			writer.WriteBits(3, 2)
			readd[index] = true
		}
		if block && len(nextIndices) > 0 && nextIndices[len(nextIndices)-1] == index {
			updateBlocks = append(updateBlocks, writeNpcUpdateBlock(view, writeInteraction, writeHeadIcon))
		}
	}

	nextSet := map[int]bool{}
	for _, index := range nextIndices {
		nextSet[index] = true
	}
	var additions []*NpcView
	for _, view := range ordered {
		if !nextSet[view.Index] {
			additions = append(additions, view)
		}
	}
	// Main-world NPCs are relative to the root tile; deck NPCs to the
	// player's deck tile, with the boat's entity index as their world
	// view.
	baseFor := func(view *NpcView) Tile {
		if view.WorldView == nil {
			return local
		}
		if deckBase != nil {
			if base := deckBase(*view.WorldView); base != nil {
				return *base
			}
		}
		if deckLocal != nil {
			return *deckLocal
		}
		return local
	}
	large := false
	for _, view := range additions {
		base := baseFor(view)
		dx := view.X - base.X
		dy := view.Y - base.Y
		if dx < -16 || dx > 15 || dy < -16 || dy > 15 {
			large = true
			break
		}
	}
	signedBits := func(value, bits int) uint32 {
		if value < 0 {
			value += 1 << uint(bits)
		}
		return uint32(value & ((1 << uint(bits)) - 1))
	}
	for _, view := range additions {
		if len(nextIndices) >= 255 {
			break
		}
		if !crossFloor && view.Level != local.Level {
			continue
		}
		base := baseFor(view)
		writer.WriteBits(uint32(view.Index), 16)
		writeInteraction := interactionIndexOr(view, -1) >= 0
		writeHeadIcon := headIconOr(view, -1) >= 0
		block := npcUpdateMask(view, writeInteraction, writeHeadIcon) != 0
		writer.WriteBits(boolBit(block), 1)
		if view.WorldView != nil {
			writer.WriteBits(1, 1)
			writer.WriteBits(uint32(*view.WorldView&0xffff), 32)
		} else {
			writer.WriteBits(0, 1)
		}
		writer.WriteBits(boolBit(readd[view.Index]), 1)
		bits := 5
		if large {
			bits = 8
		}
		writer.WriteBits(signedBits(view.Y-base.Y, bits), bits)
		writer.WriteBits(signedBits(view.X-base.X, bits), bits)
		writer.WriteBits(uint32(view.Rotation&7), 3)
		writer.WriteBits(uint32(view.TypeID&0xffff), 16)
		if crossFloor {
			writer.WriteBits(uint32(view.Level&3), 2)
		}
		nextIndices = append(nextIndices, view.Index)
		nextSet[view.Index] = true
		if block {
			updateBlocks = append(updateBlocks, writeNpcUpdateBlock(view, writeInteraction, writeHeadIcon))
		}
	}
	writer.WriteBits(0xffff, 16)

	state.Indices = nextIndices
	clear(state.LastTiles)
	clear(state.TypeIDs)
	clear(state.HeadIcons)
	clear(state.InteractionIndices)
	clear(state.WorldViews)
	for _, index := range nextIndices {
		if view := desired[index]; view != nil {
			state.LastTiles[index] = Tile{X: view.X, Y: view.Y, Level: view.Level}
			state.TypeIDs[index] = view.TypeID
			state.HeadIcons[index] = headIconOr(view, -1)
			state.InteractionIndices[index] = interactionIndexOr(view, -1)
			worldView := -1
			if view.WorldView != nil {
				worldView = *view.WorldView
			}
			state.WorldViews[index] = worldView
		}
	}
	sync := writer.Bytes()
	for _, block := range updateBlocks {
		sync = append(sync, block...)
	}
	// The client places new main-world NPCs relative to this tile (the
	// tile under a player on a boat deck, otherwise their own).
	header := make([]byte, 11)
	binary.BigEndian.PutUint32(header[0:4], uint32(loopCycle))
	flags := 0
	if large {
		flags |= 1
	}
	if crossFloor {
		flags |= 2
	}
	header[4] = byte(flags)
	binary.BigEndian.PutUint16(header[5:7], uint16(local.X))
	binary.BigEndian.PutUint16(header[7:9], uint16(local.Y))
	binary.BigEndian.PutUint16(header[9:11], uint16(len(sync)))
	return mustEncode(ServerNpcInfo, append(header, sync...))
}

func interactionIndexOr(view *NpcView, def int) int {
	if view.InteractionIndex != nil {
		return *view.InteractionIndex
	}
	return def
}

func headIconOr(view *NpcView, def int) int {
	if view.HeadIcon != nil {
		return *view.HeadIcon
	}
	return def
}

// npcWantsInteractionWrite reports whether a known NPC's interaction
// index differs from the last sent one. An index absent from the map
// reads as undefined in TS, which never equals the current value.
func npcWantsInteractionWrite(state *NpcSyncState, index int, view *NpcView) bool {
	prev, ok := state.InteractionIndices[index]
	return !ok || prev != interactionIndexOr(view, -1)
}

// npcWantsHeadIconWrite reports whether a known NPC's head icon differs
// from the last sent one; an absent entry reads as -1.
func npcWantsHeadIconWrite(state *NpcSyncState, index int, view *NpcView) bool {
	prev := -1
	if v, ok := state.HeadIcons[index]; ok {
		prev = v
	}
	return prev != headIconOr(view, -1)
}
