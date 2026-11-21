package protocol

import (
	"bytes"
	"testing"
)

func intPtr(v int) *int { return &v }

func TestCreatePlayerSyncState(t *testing.T) {
	state := CreatePlayerSyncState(1, Tile{X: 3200, Y: 3200, Level: 0})
	if state.ActiveCount != 1 || state.Active[0] != 1 {
		t.Fatalf("active = count %d first %d, want 1/1", state.ActiveCount, state.Active[0])
	}
	if state.EmptyCount != 2046 {
		t.Fatalf("emptyCount = %d, want 2046", state.EmptyCount)
	}
	if state.Flags[1] != 0 || state.Flags[2] != 1 || state.Flags[2047] != 1 {
		t.Fatalf("flags[1,2,2047] = %d,%d,%d, want 0,1,1", state.Flags[1], state.Flags[2], state.Flags[2047])
	}
	if state.LastTiles[1] != (Tile{X: 3200, Y: 3200, Level: 0}) {
		t.Fatalf("lastTiles[1] = %+v", state.LastTiles[1])
	}
	if state.MovementTypes[1] != 1 {
		t.Fatalf("movementTypes[1] = %d, want 1", state.MovementTypes[1])
	}
	for i, v := range state.ViewPositions {
		if v != -1 {
			t.Fatalf("viewPositions[%d] = %d, want -1", i, v)
		}
	}
}

func TestCreateNpcSyncState(t *testing.T) {
	state := CreateNpcSyncState()
	if len(state.Indices) != 0 || len(state.LastTiles) != 0 || len(state.TypeIDs) != 0 {
		t.Fatalf("fresh npc state not empty: %+v", state)
	}
}

func TestEncodeInitialPlayerSync(t *testing.T) {
	// Hand-traced: header base (3152,3152), local index 1, cycle 100,
	// sync length 7; sync bits 1,0,11,1 + 30-bit packed tile, aligned,
	// then 0,11,2045-in-11-bits. Cross-checked with an independent
	// bit-packer.
	want := []byte{
		20, 0, 19,
		0x0C, 0x50, 0x0C, 0x50, 0, 1, 0, 0, 0, 0x64, 0, 7,
		0xB8, 0x64, 0x01, 0x90, 0x00, 0x7F, 0xF4,
	}
	if got := EncodeInitialPlayerSync(1, 3200, 3200, 0, 100); !bytes.Equal(got, want) {
		t.Fatalf("EncodeInitialPlayerSync = %x, want %x", got, want)
	}
	// A tile near the origin clamps the scene base to 0,0.
	got := EncodeInitialPlayerSync(1, 40, 40, 0, 0)
	if !bytes.Equal(got[3:7], []byte{0, 0, 0, 0}) {
		t.Fatalf("base for tile (40,40) = %x, want 00000000", got[3:7])
	}
}

func localView(x, y int) PlayerView {
	return PlayerView{Tile: Tile{X: x, Y: y, Level: 0}, Index: 1}
}

func TestEncodePlayerSyncFirstThenNoop(t *testing.T) {
	state := CreatePlayerSyncState(1, Tile{X: 3200, Y: 3200, Level: 0})
	views := []PlayerView{localView(3200, 3200)}
	// First frame: the local player has no stored interaction index, so
	// the encoder writes a FACE_ENTITY block (mask 0x40, target
	// 0xffffff). Pass 1 bits 1,1,00 -> 0xC0; pass 3 skip run -> 0x7F
	// 0xF4. Sync = 3 writer bytes + 4 block bytes.
	wantFirst := []byte{
		20, 0, 21,
		0x0C, 0x50, 0x0C, 0x50, 0, 1, 0, 0, 0, 0xC8, 0, 7,
		0xC0, 0x7F, 0xF4, 0x40, 0xFF, 0xFF, 0xFF, 0, 0,
	}
	if got := EncodePlayerSync(1, 3152, 3152, 200, views, state); !bytes.Equal(got, wantFirst) {
		t.Fatalf("first EncodePlayerSync = %x, want %x", got, wantFirst)
	}
	// Second frame, nothing changed: pass 1 writes 0 + skip(0), pass 3
	// writes the empty-slot skip run. No update blocks.
	wantNoop := []byte{
		20, 0, 17,
		0x0C, 0x50, 0x0C, 0x50, 0, 1, 0, 0, 0, 0xC9, 0, 3,
		0x00, 0x7F, 0xF4, 0, 0,
	}
	if got := EncodePlayerSync(1, 3152, 3152, 201, views, state); !bytes.Equal(got, wantNoop) {
		t.Fatalf("noop EncodePlayerSync = %x, want %x", got, wantNoop)
	}
	// A third identical frame is byte-identical apart from the cycle.
	got := EncodePlayerSync(1, 3152, 3152, 202, views, state)
	wantNoop[12] = 0xCA // loop cycle low byte
	if !bytes.Equal(got, wantNoop) {
		t.Fatalf("third EncodePlayerSync = %x, want %x", got, wantNoop)
	}
}

func TestEncodePlayerSyncWalkStep(t *testing.T) {
	state := CreatePlayerSyncState(1, Tile{X: 3200, Y: 3200, Level: 0})
	EncodePlayerSync(1, 3152, 3152, 200, []PlayerView{localView(3200, 3200)}, state)
	EncodePlayerSync(1, 3152, 3152, 201, []PlayerView{localView(3200, 3200)}, state)
	// One step east: update bit, no block, walk code 01, direction 4
	// (table index (0+1)*3+1+1 = 5). Pass 1 bits 1,0,01,100 -> 0x98.
	want := []byte{
		20, 0, 17,
		0x0C, 0x50, 0x0C, 0x50, 0, 1, 0, 0, 0, 0xCA, 0, 3,
		0x98, 0x7F, 0xF4, 0, 0,
	}
	if got := EncodePlayerSync(1, 3152, 3152, 202, []PlayerView{localView(3201, 3200)}, state); !bytes.Equal(got, want) {
		t.Fatalf("walk EncodePlayerSync = %x, want %x", got, want)
	}
	if state.LastTiles[1] != (Tile{X: 3201, Y: 3200, Level: 0}) {
		t.Fatalf("lastTiles[1] = %+v, want (3201,3200,0)", state.LastTiles[1])
	}
}

func TestEncodePlayerSyncTeleport(t *testing.T) {
	state := CreatePlayerSyncState(1, Tile{X: 3200, Y: 3200, Level: 0})
	EncodePlayerSync(1, 3152, 3152, 200, []PlayerView{localView(3200, 3200)}, state)
	EncodePlayerSync(1, 3152, 3152, 201, []PlayerView{localView(3200, 3200)}, state)
	// Teleport +800,+800: update bit, no block, code 11, absolute bit,
	// 30-bit packed delta. Cross-checked with an independent bit-packer.
	want := []byte{
		20, 0, 21,
		0x0C, 0x50, 0x0C, 0x50, 0, 1, 0, 0, 0, 0xCA, 0, 7,
		0xB8, 0x19, 0x00, 0x64, 0x00, 0x7F, 0xF4, 0, 0,
	}
	if got := EncodePlayerSync(1, 3152, 3152, 202, []PlayerView{localView(4000, 4000)}, state); !bytes.Equal(got, want) {
		t.Fatalf("teleport EncodePlayerSync = %x, want %x", got, want)
	}
}

func TestEncodePlayerSyncAddsRemotePlayer(t *testing.T) {
	state := CreatePlayerSyncState(1, Tile{X: 3200, Y: 3200, Level: 0})
	remote := PlayerView{
		Tile:       Tile{X: 3205, Y: 3200, Level: 0},
		Index:      2,
		Appearance: []byte{0xAA, 0xBB},
	}
	views := []PlayerView{localView(3200, 3200), remote}
	// Local block [40 FF FF FF] (interaction write on the first frame);
	// remote block [44 FF FF FF FE AA BB] (appearance + interaction,
	// appearance length byteC(2) = 0xFE). Writer bytes cross-checked
	// with an independent bit-packer.
	want := []byte{
		20, 0, 32,
		0x0C, 0x50, 0x0C, 0x50, 0, 1, 0, 0, 0, 0xC8, 0, 18,
		0xC0, 0x86, 0x42, 0xB2, 0x01, 0x7F, 0xF0,
		0x40, 0xFF, 0xFF, 0xFF,
		0x44, 0xFF, 0xFF, 0xFF, 0xFE, 0xAA, 0xBB,
		0, 0,
	}
	if got := EncodePlayerSync(1, 3152, 3152, 200, views, state); !bytes.Equal(got, want) {
		t.Fatalf("add-player EncodePlayerSync = %x, want %x", got, want)
	}
	if state.ActiveCount != 2 || state.LastTiles[2] != (Tile{X: 3205, Y: 3200, Level: 0}) {
		t.Fatalf("after add: activeCount=%d lastTiles[2]=%+v", state.ActiveCount, state.LastTiles[2])
	}
	if state.InteractionIndices[2] != -1 {
		t.Fatalf("interactionIndices[2] = %d, want -1", state.InteractionIndices[2])
	}
}

func TestEncodePlayerSyncMissingLocalPanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatalf("EncodePlayerSync without local player did not panic")
		}
	}()
	state := CreatePlayerSyncState(1, Tile{X: 3200, Y: 3200, Level: 0})
	EncodePlayerSync(1, 3152, 3152, 200, []PlayerView{{Tile: Tile{X: 3200, Y: 3200}, Index: 2}}, state)
}

func TestEncodePlayerSyncFinePositionTrailer(t *testing.T) {
	state := CreatePlayerSyncState(1, Tile{X: 3200, Y: 3200, Level: 0})
	view := localView(3200, 3200)
	view.FinePosition = &FinePositionView{Index: 1, Seq: 7, X: 1, Y: 2, Rotation: 512, Level: 0, Active: true, Moving: true}
	EncodePlayerSync(1, 3152, 3152, 200, []PlayerView{view}, state)
	got := EncodePlayerSync(1, 3152, 3152, 201, []PlayerView{view}, state)
	// Trailer: count 1 + one 18-byte fine payload. The sync section is
	// the 3-byte noop writer; the trailer follows it.
	if len(got) != 3+12+3+2+18 {
		t.Fatalf("fine-position packet length = %d, want %d", len(got), 3+12+3+2+18)
	}
	trailer := got[len(got)-20:]
	if trailer[0] != 0 || trailer[1] != 1 {
		t.Fatalf("fine count = %x, want 0001", trailer[:2])
	}
	wantPayload := finePositionPayload(view.FinePosition)
	if !bytes.Equal(trailer[2:], wantPayload) {
		t.Fatalf("fine payload = %x, want %x", trailer[2:], wantPayload)
	}
}

func TestEncodeFinePosition(t *testing.T) {
	// index 1, seq -1, x 1.5 (384), y 2.25 (576), rotation 2048 -> 0
	// after & 2047, level 2, all five flags set -> 0x1F.
	want := []byte{
		23,
		0, 1, 0xFF, 0xFF, 0xFF, 0xFF, 0, 0, 1, 0x80, 0, 0, 2, 0x40, 0, 0, 2, 0x1F,
	}
	got := EncodeFinePosition(&FinePositionView{
		Index: 1, Seq: -1, X: 1.5, Y: 2.25, Rotation: 2048, Level: 2,
		Active: true, Moving: true, Running: true, Blocked: true, Snap: true,
	})
	if !bytes.Equal(got, want) {
		t.Fatalf("EncodeFinePosition = %x, want %x", got, want)
	}
}

func npcView(index, typeID, x, y int) NpcView {
	return NpcView{
		Tile:          Tile{X: x, Y: y, Level: 0},
		Index:         index,
		TypeID:        typeID,
		Rotation:      2,
		WalkDirection: -1,
		RunDirection:  -1,
	}
}

func TestEncodeNpcSyncEmpty(t *testing.T) {
	state := CreateNpcSyncState()
	want := []byte{
		21, 0, 14,
		0, 0, 0, 5, 0, 0x0C, 0x80, 0x0C, 0x80, 0, 3,
		0x00, 0xFF, 0xFF,
	}
	got := EncodeNpcSync(5, Tile{X: 3200, Y: 3200, Level: 0}, nil, state, nil, nil, false)
	if !bytes.Equal(got, want) {
		t.Fatalf("empty EncodeNpcSync = %x, want %x", got, want)
	}
}

func TestEncodeNpcSyncAdditionThenSteady(t *testing.T) {
	state := CreateNpcSyncState()
	local := Tile{X: 3200, Y: 3200, Level: 0}
	view := npcView(1, 100, 3201, 3200)
	// Addition: count 0, index 1, no block, no world view, not a re-add,
	// dy 0 / dx 1 in 5 bits, rotation 2, type 100, terminator.
	wantAdd := []byte{
		21, 0, 20,
		0, 0, 0, 5, 0, 0x0C, 0x80, 0x0C, 0x80, 0, 9,
		0x00, 0x00, 0x01, 0x00, 0x0A, 0x00, 0x64, 0xFF, 0xFF,
	}
	if got := EncodeNpcSync(5, local, []NpcView{view}, state, nil, nil, false); !bytes.Equal(got, wantAdd) {
		t.Fatalf("add EncodeNpcSync = %x, want %x", got, wantAdd)
	}
	if len(state.Indices) != 1 || state.Indices[0] != 1 || state.TypeIDs[1] != 100 {
		t.Fatalf("npc state after add: %+v", state)
	}
	// Steady: count 1, one 0 bit (no movement, no block), terminator.
	wantSteady := []byte{
		21, 0, 15,
		0, 0, 0, 6, 0, 0x0C, 0x80, 0x0C, 0x80, 0, 4,
		0x01, 0x7F, 0xFF, 0x80,
	}
	if got := EncodeNpcSync(6, local, []NpcView{view}, state, nil, nil, false); !bytes.Equal(got, wantSteady) {
		t.Fatalf("steady EncodeNpcSync = %x, want %x", got, wantSteady)
	}
}

func TestEncodeNpcSyncAdditionWithAnimation(t *testing.T) {
	state := CreateNpcSyncState()
	local := Tile{X: 3200, Y: 3200, Level: 0}
	view := npcView(1, 100, 3201, 3200)
	view.Animation = &AnimationView{ID: 100, Delay: 2}
	// Block bit set (writer byte 4 becomes 0x80); block is mask 0x10,
	// animation shortBE(100), delay 2.
	want := []byte{
		21, 0, 24,
		0, 0, 0, 5, 0, 0x0C, 0x80, 0x0C, 0x80, 0, 13,
		0x00, 0x00, 0x01, 0x80, 0x0A, 0x00, 0x64, 0xFF, 0xFF,
		0x10, 0x00, 0x64, 0x02,
	}
	if got := EncodeNpcSync(5, local, []NpcView{view}, state, nil, nil, false); !bytes.Equal(got, want) {
		t.Fatalf("animation EncodeNpcSync = %x, want %x", got, want)
	}
}

func TestEncodeNpcSyncAdditionWithHits(t *testing.T) {
	state := CreateNpcSyncState()
	local := Tile{X: 3200, Y: 3200, Level: 0}
	view := npcView(1, 100, 3201, 3200)
	view.Hits = []HitsplatView{{Type: 1, Damage: 5}}
	view.Health = &HealthView{Current: 50, Max: 100}
	// Hit block: mask 0x20, byteS(1)=0x7F, smart(1,5,0), byteA(1)=0x81,
	// bar smarts 0,0,0, scaled health 15 as byteC = 0xF1.
	want := []byte{
		21, 0, 30,
		0, 0, 0, 5, 0, 0x0C, 0x80, 0x0C, 0x80, 0, 19,
		0x00, 0x00, 0x01, 0x80, 0x0A, 0x00, 0x64, 0xFF, 0xFF,
		0x20, 0x7F, 0x01, 0x05, 0x00, 0x81, 0x00, 0x00, 0x00, 0xF1,
	}
	if got := EncodeNpcSync(5, local, []NpcView{view}, state, nil, nil, false); !bytes.Equal(got, want) {
		t.Fatalf("hits EncodeNpcSync = %x, want %x", got, want)
	}
}

func TestEncodeNpcSyncWalkAndRemoval(t *testing.T) {
	state := CreateNpcSyncState()
	local := Tile{X: 3200, Y: 3200, Level: 0}
	view := npcView(1, 100, 3201, 3200)
	EncodeNpcSync(5, local, []NpcView{view}, state, nil, nil, false)
	// Walk step, direction 3, no block.
	view.WalkDirection = 3
	wantWalk := []byte{
		21, 0, 15,
		0, 0, 0, 6, 0, 0x0C, 0x80, 0x0C, 0x80, 0, 4,
		0x01, 0xAD, 0xFF, 0xFE,
	}
	if got := EncodeNpcSync(6, local, []NpcView{view}, state, nil, nil, false); !bytes.Equal(got, wantWalk) {
		t.Fatalf("walk EncodeNpcSync = %x, want %x", got, wantWalk)
	}
	// Gone: count 1, removal bits 1,11, terminator.
	wantRemoval := []byte{
		21, 0, 15,
		0, 0, 0, 7, 0, 0x0C, 0x80, 0x0C, 0x80, 0, 4,
		0x01, 0xFF, 0xFF, 0xE0,
	}
	if got := EncodeNpcSync(7, local, nil, state, nil, nil, false); !bytes.Equal(got, wantRemoval) {
		t.Fatalf("removal EncodeNpcSync = %x, want %x", got, wantRemoval)
	}
	if len(state.Indices) != 0 {
		t.Fatalf("indices after removal = %v, want empty", state.Indices)
	}
}
