package session

import (
	"bytes"
	"encoding/binary"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"tsps-go/internal/music"
	"tsps-go/internal/protocol"
)

// fakeFlushSource is a FlushSource test double returning fixed
// snapshots and counting SyncView calls.
type fakeFlushSource struct {
	owner  SyncPlayerSnapshot
	locals []SyncPlayerSnapshot
	npcs   []SyncNpcSnapshot
	calls  int
}

func (f *fakeFlushSource) SyncView() (SyncPlayerSnapshot, []SyncPlayerSnapshot, []SyncNpcSnapshot) {
	f.calls++
	return f.owner, f.locals, f.npcs
}

func (f *fakeFlushSource) moveOwner(x, y, z int) {
	f.owner.X, f.owner.Y, f.owner.Z = x, y, z
}

func flushOwner(identity any) SyncPlayerSnapshot {
	return SyncPlayerSnapshot{
		Identity:        identity,
		Index:           1,
		X:               3200,
		Y:               3200,
		DisplayName:     "Test Player",
		AppearanceBlock: []byte{0x11, 0x22},
	}
}

// flushFramePackets decodes every packet of every frame the channel
// sent, in order. protocol.DecodeFrame decodes client->server frames
// only, so this uses the splitServerPackets mirror from
// bootstrap_test.go.
func flushFramePackets(t *testing.T, ch *fakeChannel) []protocol.Packet {
	t.Helper()
	var out []protocol.Packet
	for _, frame := range ch.sentBytes() {
		out = append(out, splitServerPackets(t, frame)...)
	}
	return out
}

func flushOpcodes(packets []protocol.Packet) []byte {
	out := make([]byte, 0, len(packets))
	for _, p := range packets {
		out = append(out, p.Opcode)
	}
	return out
}

func findOpcode(t *testing.T, packets []protocol.Packet, opcode byte) []byte {
	t.Helper()
	for _, p := range packets {
		if p.Opcode == opcode {
			return p.Payload
		}
	}
	t.Fatalf("no packet with opcode %d in %v", opcode, flushOpcodes(packets))
	return nil
}

func hasOpcode(packets []protocol.Packet, opcode byte) bool {
	for _, p := range packets {
		if p.Opcode == opcode {
			return true
		}
	}
	return false
}

// parseRebuild reads a REBUILD_NORMAL payload: regionX, regionY,
// reload byte, key count, then keyCount 16-byte keys.
func parseRebuild(t *testing.T, payload []byte) (regionX, regionY uint16, reload byte, keys [][4]int32) {
	t.Helper()
	if len(payload) < 7 {
		t.Fatalf("rebuild payload too short: %d bytes", len(payload))
	}
	regionX = binary.BigEndian.Uint16(payload[0:2])
	regionY = binary.BigEndian.Uint16(payload[2:4])
	reload = payload[4]
	count := int(binary.BigEndian.Uint16(payload[5:7]))
	if len(payload) != 7+count*16 {
		t.Fatalf("rebuild payload len %d, want %d for %d keys", len(payload), 7+count*16, count)
	}
	for i := 0; i < count; i++ {
		var key [4]int32
		for j := 0; j < 4; j++ {
			key[j] = int32(binary.BigEndian.Uint32(payload[7+i*16+j*4:]))
		}
		keys = append(keys, key)
	}
	return regionX, regionY, reload, keys
}

// parseSyncHeader reads the 12-byte header shared by the initial and
// regular player sync packets: scene base, local index, loop cycle,
// sync length.
func parseSyncHeader(t *testing.T, payload []byte) (baseX, baseY, localIndex uint16, loopCycle uint32, syncLen uint16) {
	t.Helper()
	if len(payload) < 12 {
		t.Fatalf("player sync payload too short: %d bytes", len(payload))
	}
	return binary.BigEndian.Uint16(payload[0:2]),
		binary.BigEndian.Uint16(payload[2:4]),
		binary.BigEndian.Uint16(payload[4:6]),
		binary.BigEndian.Uint32(payload[6:10]),
		binary.BigEndian.Uint16(payload[10:12])
}

func TestFlushTickFirstTickFrameOrder(t *testing.T) {
	ch := newFakeChannel()
	s := NewSession(ch)
	src := &fakeFlushSource{owner: flushOwner(new(int))}
	s.AttachSyncSource(src)

	if err := s.FlushTick(7); err != nil {
		t.Fatalf("FlushTick: %v", err)
	}
	if src.calls != 1 {
		t.Fatalf("SyncView calls = %d, want 1", src.calls)
	}
	// One frame: every packet queued by the flush goes out together.
	if got := ch.sentBytes(); len(got) != 1 {
		t.Fatalf("FlushTick sent %d frames, want 1", len(got))
	}
	packets := flushFramePackets(t, ch)
	// TS order (PlayerSession.ts:461-462 hide-ops, 488 rebuild,
	// 612-619 tick/player sync/NPC sync).
	want := []byte{
		protocol.ServerHideNpcOps, protocol.ServerHideObjOps,
		protocol.ServerRebuildNormal,
		protocol.ServerTick, protocol.ServerPlayerSync, protocol.ServerNpcInfo,
	}
	if got := flushOpcodes(packets); !bytes.Equal(got, want) {
		t.Fatalf("first-tick opcodes = %v, want %v", got, want)
	}
	if got := findOpcode(t, packets, protocol.ServerHideNpcOps); !bytes.Equal(got, []byte{0}) {
		t.Errorf("hide-npc-ops payload = %v, want [0]", got)
	}
	if got := findOpcode(t, packets, protocol.ServerHideObjOps); !bytes.Equal(got, []byte{0}) {
		t.Errorf("hide-obj-ops payload = %v, want [0]", got)
	}

	regionX, regionY, reload, keys := parseRebuild(t, findOpcode(t, packets, protocol.ServerRebuildNormal))
	if regionX != 400 || regionY != 400 {
		t.Errorf("rebuild region = (%d,%d), want (400,400)", regionX, regionY)
	}
	if reload != 1 {
		t.Errorf("rebuild reload byte = %d, want 1 (forceReload false)", reload)
	}
	if len(keys) != 4 {
		t.Fatalf("rebuild keys = %d, want 4", len(keys))
	}
	// No XTEA provider: all-zero keys.
	for i, key := range keys {
		if key != [4]int32{} {
			t.Errorf("key %d = %v, want zeros", i, key)
		}
	}

	tick := findOpcode(t, packets, protocol.ServerTick)
	if len(tick) != 8 || int32(binary.BigEndian.Uint32(tick[0:4])) != 7 {
		t.Errorf("tick payload = %v, want tick 7 + serverTime", tick)
	}

	syncPayload := findOpcode(t, packets, protocol.ServerPlayerSync)
	baseX, baseY, localIndex, loopCycle, syncLen := parseSyncHeader(t, syncPayload)
	if baseX != 3152 || baseY != 3152 {
		t.Errorf("initial sync base = (%d,%d), want (3152,3152)", baseX, baseY)
	}
	if localIndex != 1 || loopCycle != 7 {
		t.Errorf("initial sync header index/cycle = %d/%d, want 1/7", localIndex, loopCycle)
	}
	// The initial form has no fine-position trailer: the payload is
	// exactly the 12-byte header plus the sync bytes.
	if int(syncLen) != len(syncPayload)-12 {
		t.Errorf("initial syncLen = %d, payload len %d, want header+sync only", syncLen, len(syncPayload))
	}
}

func TestFlushTickSecondTickNoRebuild(t *testing.T) {
	ch := newFakeChannel()
	s := NewSession(ch)
	src := &fakeFlushSource{owner: flushOwner(new(int))}
	s.AttachSyncSource(src)

	if err := s.FlushTick(7); err != nil {
		t.Fatalf("FlushTick 1: %v", err)
	}
	if err := s.FlushTick(8); err != nil {
		t.Fatalf("FlushTick 2: %v", err)
	}
	frames := ch.sentBytes()
	if len(frames) != 2 {
		t.Fatalf("sent %d frames, want 2", len(frames))
	}
	// The second frame holds only the per-tick frame: no hide-ops,
	// no rebuild for a stationary player.
	packets := splitServerPackets(t, frames[1])
	want := []byte{protocol.ServerTick, protocol.ServerPlayerSync, protocol.ServerNpcInfo}
	if got := flushOpcodes(packets); !bytes.Equal(got, want) {
		t.Fatalf("second-tick opcodes = %v, want %v", got, want)
	}
	tick := findOpcode(t, packets, protocol.ServerTick)
	if int32(binary.BigEndian.Uint32(tick[0:4])) != 8 {
		t.Errorf("second tick payload tick = %d, want 8", int32(binary.BigEndian.Uint32(tick[0:4])))
	}
	// The regular form carries the 2-byte fine-position trailer
	// after the sync bytes.
	syncPayload := findOpcode(t, packets, protocol.ServerPlayerSync)
	_, _, _, _, syncLen := parseSyncHeader(t, syncPayload)
	if len(syncPayload) != 12+int(syncLen)+2 {
		t.Errorf("regular sync payload len %d, want 12+syncLen(%d)+2", len(syncPayload), syncLen)
	}
	if s.pendingInitialLocalAppearance {
		t.Error("pendingInitialLocalAppearance still set after the first view build")
	}
}

func TestFlushTickSceneBaseRecenter(t *testing.T) {
	ch := newFakeChannel()
	s := NewSession(ch)
	src := &fakeFlushSource{owner: flushOwner(new(int))}
	s.AttachSyncSource(src)

	flush := func(cycle int64) []protocol.Packet {
		t.Helper()
		before := len(ch.sentBytes())
		if err := s.FlushTick(cycle); err != nil {
			t.Fatalf("FlushTick(%d): %v", cycle, err)
		}
		frames := ch.sentBytes()
		if len(frames) != before+1 {
			t.Fatalf("FlushTick(%d) sent %d frames, want 1 more", cycle, len(frames)-before)
		}
		return splitServerPackets(t, frames[before])
	}

	// Tick 1: base (3152,3152), initial rebuild.
	if p := flush(1); !hasOpcode(p, protocol.ServerRebuildNormal) {
		t.Error("tick 1: no rebuild")
	}
	// Tick 2: localX = 3240-3152 = 88 -> recenter X to 3192.
	src.moveOwner(3240, 3200, 0)
	p := flush(2)
	regionX, _, _, _ := parseRebuild(t, findOpcode(t, p, protocol.ServerRebuildNormal))
	if regionX != 405 {
		t.Errorf("tick 2 rebuild regionX = %d, want 405", regionX)
	}
	baseX, _, _, _, _ := parseSyncHeader(t, findOpcode(t, p, protocol.ServerPlayerSync))
	if baseX != 3192 {
		t.Errorf("tick 2 sync baseX = %d, want 3192", baseX)
	}
	// Tick 3: localY = 3288-3152 = 136 -> recenter Y to 3240.
	src.moveOwner(3240, 3288, 0)
	p = flush(3)
	_, regionY, _, _ := parseRebuild(t, findOpcode(t, p, protocol.ServerRebuildNormal))
	if regionY != 411 {
		t.Errorf("tick 3 rebuild regionY = %d, want 411", regionY)
	}
	_, baseY, _, _, _ := parseSyncHeader(t, findOpcode(t, p, protocol.ServerPlayerSync))
	if baseY != 3240 {
		t.Errorf("tick 3 sync baseY = %d, want 3240", baseY)
	}
	// Tick 4: localX = 3200-3192 = 8 < 16 -> recenter X to 3152.
	src.moveOwner(3200, 3288, 0)
	p = flush(4)
	if !hasOpcode(p, protocol.ServerRebuildNormal) {
		t.Error("tick 4: no rebuild after crossing the low boundary")
	}
	baseX, _, _, _, _ = parseSyncHeader(t, findOpcode(t, p, protocol.ServerPlayerSync))
	if baseX != 3152 {
		t.Errorf("tick 4 sync baseX = %d, want 3152", baseX)
	}
	// Tick 5: stationary -> no rebuild.
	if p := flush(5); hasOpcode(p, protocol.ServerRebuildNormal) {
		t.Errorf("tick 5: unexpected rebuild in %v", flushOpcodes(p))
	}
}

func TestFlushTickSceneBaseNegativeClamp(t *testing.T) {
	ch := newFakeChannel()
	s := NewSession(ch)
	owner := flushOwner(new(int))
	owner.X, owner.Y = 10, 10
	src := &fakeFlushSource{owner: owner}
	s.AttachSyncSource(src)

	if err := s.FlushTick(1); err != nil {
		t.Fatalf("FlushTick: %v", err)
	}
	packets := flushFramePackets(t, ch)
	// (10-48) & ~7 is -40 in both JS and Go; max(0, -40) = 0.
	baseX, baseY, _, _, _ := parseSyncHeader(t, findOpcode(t, packets, protocol.ServerPlayerSync))
	if baseX != 0 || baseY != 0 {
		t.Errorf("clamped sync base = (%d,%d), want (0,0)", baseX, baseY)
	}
	regionX, regionY, _, keys := parseRebuild(t, findOpcode(t, packets, protocol.ServerRebuildNormal))
	if regionX != 1 || regionY != 1 {
		t.Errorf("rebuild region = (%d,%d), want (1,1)", regionX, regionY)
	}
	// Center chunk (1,1): trunc((1-6)/8) = 0 and trunc((1+6)/8) = 0,
	// so exactly one region, id 0, with a zero key (no provider).
	if len(keys) != 1 || keys[0] != [4]int32{} {
		t.Errorf("rebuild keys = %v, want one zero key", keys)
	}
}

func TestFlushTickLevelChangeDoesNotRebuild(t *testing.T) {
	// TS passes sceneRebuildFor a change flag WITHOUT the level term
	// (PlayerSession.ts:481-487): a pure plane change replays the
	// scene bookkeeping but needs no map rebuild.
	ch := newFakeChannel()
	s := NewSession(ch)
	src := &fakeFlushSource{owner: flushOwner(new(int))}
	s.AttachSyncSource(src)

	if err := s.FlushTick(1); err != nil {
		t.Fatalf("FlushTick 1: %v", err)
	}
	src.moveOwner(3200, 3200, 1)
	if err := s.FlushTick(2); err != nil {
		t.Fatalf("FlushTick 2: %v", err)
	}
	frames := ch.sentBytes()
	if p := splitServerPackets(t, frames[1]); hasOpcode(p, protocol.ServerRebuildNormal) {
		t.Errorf("level-only change sent a rebuild: %v", flushOpcodes(p))
	}
	if s.replayedSceneLevel != 1 || !s.hasReplayedScene {
		t.Errorf("replay bookkeeping: level=%d replayed=%v, want 1/true", s.replayedSceneLevel, s.hasReplayedScene)
	}
}

func TestFlushTickXteaProviderOrder(t *testing.T) {
	ch := newFakeChannel()
	s := NewSession(ch)
	src := &fakeFlushSource{owner: flushOwner(new(int))}
	s.AttachSyncSource(src)
	var gotIDs []int
	s.SetXteaProvider(func(regionID int) [4]int32 {
		gotIDs = append(gotIDs, regionID)
		return [4]int32{int32(regionID), int32(-regionID), 1, 2}
	})

	if err := s.FlushTick(1); err != nil {
		t.Fatalf("FlushTick: %v", err)
	}
	// Center chunk (400,400): mapX/mapY each range 49..50, mapX
	// outer, mapY inner; region id (mapX<<8)|mapY.
	wantIDs := []int{(49 << 8) | 49, (49 << 8) | 50, (50 << 8) | 49, (50 << 8) | 50}
	if !equalInts(gotIDs, wantIDs) {
		t.Errorf("provider region ids = %v, want %v", gotIDs, wantIDs)
	}
	packets := flushFramePackets(t, ch)
	_, _, _, keys := parseRebuild(t, findOpcode(t, packets, protocol.ServerRebuildNormal))
	if len(keys) != 4 {
		t.Fatalf("keys = %d, want 4", len(keys))
	}
	if keys[0] != [4]int32{12593, -12593, 1, 2} {
		t.Errorf("first key = %v, want [12593 -12593 1 2]", keys[0])
	}
	if keys[3] != [4]int32{12850, -12850, 1, 2} {
		t.Errorf("last key = %v, want [12850 -12850 1 2]", keys[3])
	}
}

func TestFlushTickEmptyNpcsStillSendsNpcSync(t *testing.T) {
	ch := newFakeChannel()
	s := NewSession(ch)
	src := &fakeFlushSource{
		owner: flushOwner(new(int)),
		npcs: []SyncNpcSnapshot{
			{Identity: new(int), Index: 100, TypeID: 1, X: 3201, Y: 3201},
		},
	}
	s.AttachSyncSource(src)

	if err := s.FlushTick(1); err != nil {
		t.Fatalf("FlushTick 1: %v", err)
	}
	// The NPC leaves local view: the second tick still sends an NPC
	// sync packet (carrying the removal).
	src.npcs = nil
	if err := s.FlushTick(2); err != nil {
		t.Fatalf("FlushTick 2: %v", err)
	}
	frames := ch.sentBytes()
	for i, frame := range frames {
		if p := splitServerPackets(t, frame); !hasOpcode(p, protocol.ServerNpcInfo) {
			t.Errorf("frame %d: no NPC sync in %v", i, flushOpcodes(p))
		}
	}
}

func TestFlushTickNilSourceIsQueueOnly(t *testing.T) {
	ch := newFakeChannel()
	s := NewSession(ch)
	// No source attached: the queue-only flush, as before.
	s.SendPacket([]byte{9})
	if err := s.FlushTick(1); err != nil {
		t.Fatalf("FlushTick: %v", err)
	}
	if got := ch.sentBytes(); len(got) != 1 || !bytes.Equal(got[0], []byte{9}) {
		t.Fatalf("queue-only FlushTick sent %v, want [[9]]", got)
	}

	// Attaching then detaching a source degrades back to queue-only.
	src := &fakeFlushSource{owner: flushOwner(new(int))}
	s.AttachSyncSource(src)
	if err := s.FlushTick(2); err != nil {
		t.Fatalf("FlushTick with source: %v", err)
	}
	if src.calls != 1 {
		t.Fatalf("SyncView calls = %d, want 1", src.calls)
	}
	s.AttachSyncSource(nil)
	s.SendPacket([]byte{8})
	if err := s.FlushTick(3); err != nil {
		t.Fatalf("FlushTick after detach: %v", err)
	}
	if src.calls != 1 {
		t.Errorf("SyncView called after detach: %d calls", src.calls)
	}
	frames := ch.sentBytes()
	if len(frames) != 3 || !bytes.Equal(frames[2], []byte{8}) {
		t.Errorf("frames after detach = %v, want third frame [8]", frames)
	}
}

func TestFlushTickClosedChannelSendsNothing(t *testing.T) {
	ch := newFakeChannel()
	s := NewSession(ch)
	src := &fakeFlushSource{owner: flushOwner(new(int))}
	s.AttachSyncSource(src)
	s.SendPacket([]byte{1})
	ch.Close(1000, "bye")

	// The gate runs before SyncView: no snapshot, no sends, and the
	// pending queue is dropped with ErrSessionClosed, as Flush does.
	if err := s.FlushTick(1); !errors.Is(err, ErrSessionClosed) {
		t.Errorf("FlushTick on closed channel = %v, want ErrSessionClosed", err)
	}
	if src.calls != 0 {
		t.Errorf("SyncView calls = %d, want 0 (gate before snapshot)", src.calls)
	}
	if got := ch.sentBytes(); len(got) != 0 {
		t.Errorf("closed channel sent %v", got)
	}

	// An empty queue on a closed channel is a nil no-op, as Flush is.
	ch2 := newFakeChannel()
	s2 := NewSession(ch2)
	src2 := &fakeFlushSource{owner: flushOwner(new(int))}
	s2.AttachSyncSource(src2)
	ch2.Close(1000, "bye")
	if err := s2.FlushTick(1); err != nil {
		t.Errorf("FlushTick (empty queue, closed) = %v, want nil", err)
	}
	if src2.calls != 0 || len(ch2.sentBytes()) != 0 {
		t.Errorf("closed empty flush: calls=%d sends=%d", src2.calls, len(ch2.sentBytes()))
	}
}

func TestFlushTickQueuedPacketsPrecedeSync(t *testing.T) {
	ch := newFakeChannel()
	s := NewSession(ch)
	src := &fakeFlushSource{owner: flushOwner(new(int))}
	s.AttachSyncSource(src)
	// A packet queued during the tick goes out before the sync frame,
	// in queue order, in the same flush.
	s.SendPacket(protocol.EncodeVarbit(1, 2))
	if err := s.FlushTick(1); err != nil {
		t.Fatalf("FlushTick: %v", err)
	}
	packets := flushFramePackets(t, ch)
	if len(packets) == 0 || packets[0].Opcode != protocol.ServerVarbit {
		t.Fatalf("first packet opcode = %v, want varbit first", flushOpcodes(packets))
	}
}

func TestSyncAppearanceRoundTrip(t *testing.T) {
	s := NewSession(newFakeChannel())
	if got := s.SyncAppearance(); got != nil {
		t.Errorf("SyncAppearance before set = %v, want nil", got)
	}
	block := []byte{1, 2, 3}
	s.SetSyncAppearance(block)
	block[0] = 99 // mutating the input must not affect the stored copy
	if got := s.SyncAppearance(); !bytes.Equal(got, []byte{1, 2, 3}) {
		t.Errorf("SyncAppearance = %v, want [1 2 3]", got)
	}
	got := s.SyncAppearance()
	got[0] = 77 // mutating the result must not affect the stored copy
	if again := s.SyncAppearance(); !bytes.Equal(again, []byte{1, 2, 3}) {
		t.Errorf("SyncAppearance after mutation = %v, want [1 2 3]", again)
	}
	s.SetSyncAppearance(nil)
	if got := s.SyncAppearance(); got != nil {
		t.Errorf("SyncAppearance after clear = %v, want nil", got)
	}
}

func TestCreatePlayerViewAppearanceCache(t *testing.T) {
	s := NewSession(newFakeChannel())
	s.playerSyncState = protocol.CreatePlayerSyncState(1, protocol.Tile{X: 3200, Y: 3200})
	idA, idB, idC := new(int), new(int), new(int)
	blockA := []byte{0xA1, 0xA2}
	origA := append([]byte(nil), blockA...)
	blockB := []byte{0xB1}
	blockC := []byte{0xC1, 0xC2, 0xC3}

	snap := func(id any, block []byte, flag bool) SyncPlayerSnapshot {
		return SyncPlayerSnapshot{
			Identity: id, Index: 2, X: 3201, Y: 3201,
			DisplayName: "Other", AppearanceBlock: block, AppearanceFlag: flag,
		}
	}

	// Unknown index: dirty, built from the snapshot block and cached.
	view := s.createPlayerView(snap(idA, blockA, false), false)
	if !view.AppearanceDirty {
		t.Error("unknown index: AppearanceDirty = false")
	}
	if !bytes.Equal(view.Appearance, origA) {
		t.Errorf("unknown index appearance = %v, want %v", view.Appearance, origA)
	}
	// The cache holds a copy: mutating the snapshot block afterwards
	// must not change the cached payload.
	blockA[0] = 0xFF
	// Simulate what EncodePlayerSync does after encoding: the index
	// becomes known.
	s.playerSyncState.LastTiles[2] = protocol.Tile{X: 3201, Y: 3201}

	// Same identity, known index, no flag: the cached payload is
	// reused even though the snapshot now carries a different block.
	view = s.createPlayerView(snap(idA, blockB, false), false)
	if view.AppearanceDirty {
		t.Error("cache hit: AppearanceDirty = true")
	}
	if !bytes.Equal(view.Appearance, origA) {
		t.Errorf("cache hit appearance = %v, want cached %v", view.Appearance, origA)
	}

	// Identity change for the same index: the payload is rebuilt
	// from the snapshot (and re-cached), but dirty stays false —
	// TS rebuilds on identity mismatch without setting the dirty
	// flag (PlayerSession.ts:670-671, 695-700).
	view = s.createPlayerView(snap(idB, blockB, false), false)
	if view.AppearanceDirty {
		t.Error("identity change: AppearanceDirty = true, want false (TS rebuilds without dirty)")
	}
	if !bytes.Equal(view.Appearance, blockB) {
		t.Errorf("identity change appearance = %v, want %v", view.Appearance, blockB)
	}

	// AppearanceFlag set: dirty, rebuilt from the snapshot block.
	view = s.createPlayerView(snap(idB, blockC, true), false)
	if !view.AppearanceDirty {
		t.Error("AppearanceFlag: AppearanceDirty = false")
	}
	if !bytes.Equal(view.Appearance, blockC) {
		t.Errorf("AppearanceFlag appearance = %v, want %v", view.Appearance, blockC)
	}

	// forceAppearance (the pending initial local appearance): dirty
	// even for a known index with the same identity.
	view = s.createPlayerView(snap(idB, blockC, false), true)
	if !view.AppearanceDirty {
		t.Error("forceAppearance: AppearanceDirty = false")
	}

	// Nil snapshot block: the default fallback block, built from the
	// nil-save bootstrap helpers with the default animation array.
	view = s.createPlayerView(snap(idC, nil, false), false)
	_, total, combat := bootstrapSkills(nil)
	want := protocol.EncodePlayerAppearance(*bootstrapAppearance(nil), "Other", combat, total, []int{808, 823, 819, 820, 821, 822, 824})
	if !bytes.Equal(view.Appearance, want) {
		t.Errorf("fallback appearance differs from the bootstrap-helper block")
	}
	if total != 33 || combat != 3 {
		t.Errorf("bootstrapSkills(nil) total/combat = %d/%d, want 33/3", total, combat)
	}
}

func TestCreatePlayerViewMovementAndFlags(t *testing.T) {
	s := NewSession(newFakeChannel())
	s.playerSyncState = protocol.CreatePlayerSyncState(1, protocol.Tile{X: 3200, Y: 3200})
	base := SyncPlayerSnapshot{Identity: new(int), Index: 2, X: 3201, Y: 3201, DisplayName: "Other", AppearanceBlock: []byte{1}}

	// No step: movement type absent (0), which the encoder derives.
	view := s.createPlayerView(base, false)
	if view.MovementType != 0 {
		t.Errorf("no step MovementType = %d, want 0 (absent)", view.MovementType)
	}
	// Walking step: 1. Running step: 2, winning over walking.
	base.WalkingDx = 1
	if view := s.createPlayerView(base, false); view.MovementType != 1 {
		t.Errorf("walking MovementType = %d, want 1", view.MovementType)
	}
	base.RunningDy = -1
	if view := s.createPlayerView(base, false); view.MovementType != 2 {
		t.Errorf("running MovementType = %d, want 2", view.MovementType)
	}

	// NeedsPlacement -> ResetPath; interaction index always -1; the
	// absent systems stay absent.
	base.WalkingDx, base.RunningDy = 0, 0
	base.NeedsPlacement = true
	view = s.createPlayerView(base, false)
	if !view.ResetPath {
		t.Error("NeedsPlacement: ResetPath = false")
	}
	if view.InteractionIndex == nil || *view.InteractionIndex != -1 {
		t.Errorf("InteractionIndex = %v, want -1", view.InteractionIndex)
	}
	if view.FaceDirection != nil || view.ForcedMovement != nil || view.ForcedMovementEnd != nil ||
		view.Tint != nil || view.WorldView != nil || view.FinePosition != nil {
		t.Error("absent view fields are set")
	}
	if view.Animation != nil || view.Health != nil || view.Hits != nil ||
		view.ForcedChat != nil || len(view.Graphics) != 0 {
		t.Error("actor updates are not empty")
	}
	if view.X != 3201 || view.Y != 3201 || view.Level != 0 || view.Index != 2 {
		t.Errorf("view tile/index = (%d,%d,%d)/%d", view.X, view.Y, view.Level, view.Index)
	}
}

func TestClientDirection(t *testing.T) {
	// The table [0,1,2,3,-1,4,5,6,7][(y+1)*3+x+1] (PlayerSession.ts:894-901).
	// dx/dy are unit steps (-1..1), the Direction domain; the last two
	// cases exercise the TS `?? -1` on out-of-table lookups.
	cases := []struct{ dx, dy, want int }{
		{-1, -1, 0}, {0, -1, 1}, {1, -1, 2},
		{-1, 0, 3}, {0, 0, -1}, {1, 0, 4},
		{-1, 1, 5}, {0, 1, 6}, {1, 1, 7},
		{0, 2, -1}, {-2, -2, -1},
	}
	for _, tc := range cases {
		if got := clientDirection(tc.dx, tc.dy); got != tc.want {
			t.Errorf("clientDirection(%d,%d) = %d, want %d", tc.dx, tc.dy, got, tc.want)
		}
	}
}

func TestCreateNpcView(t *testing.T) {
	view := createNpcView(SyncNpcSnapshot{
		Identity: new(int), Index: 100, TypeID: 42, X: 3205, Y: 3206, Z: 1,
		FacingDx: 1, WalkingDy: 1, RunningDx: 1, RunningDy: 1,
	})
	if view.Index != 100 || view.TypeID != 42 {
		t.Errorf("npc index/type = %d/%d, want 100/42", view.Index, view.TypeID)
	}
	if view.X != 3205 || view.Y != 3206 || view.Level != 1 {
		t.Errorf("npc tile = (%d,%d,%d)", view.X, view.Y, view.Level)
	}
	if view.Rotation != 4 {
		t.Errorf("facing (1,0) rotation = %d, want 4", view.Rotation)
	}
	if view.WalkDirection != 6 {
		t.Errorf("walking (0,1) direction = %d, want 6", view.WalkDirection)
	}
	if view.RunDirection != 7 {
		t.Errorf("running (1,1) direction = %d, want 7", view.RunDirection)
	}
	if view.InteractionIndex == nil || *view.InteractionIndex != -1 {
		t.Errorf("InteractionIndex = %v, want -1", view.InteractionIndex)
	}
	if view.HeadIcon != nil || view.ExactMove != nil || view.FaceTile != nil ||
		view.ModelCustomisation != nil || view.BaseAnimations != nil ||
		view.Transparency != nil || view.Tint != nil || view.WorldView != nil || view.Crawl {
		t.Error("absent NPC view fields are set")
	}
	if view.Animation != nil || view.Health != nil || view.Hits != nil ||
		view.ForcedChat != nil || len(view.Graphics) != 0 || len(view.Bars) != 0 {
		t.Error("NPC actor updates are not empty")
	}

	// No facing, no steps: rotation and both directions are -1.
	still := createNpcView(SyncNpcSnapshot{Index: 101, TypeID: 7, X: 3200, Y: 3200})
	if still.Rotation != -1 || still.WalkDirection != -1 || still.RunDirection != -1 {
		t.Errorf("still NPC rotation/walk/run = %d/%d/%d, want -1/-1/-1",
			still.Rotation, still.WalkDirection, still.RunDirection)
	}
}

// TestFlushTickPhase8Blocks covers the wired phase 8 paths through
// FlushTick itself: the music state machine sends PLAY_SONG on
// entering an area, the region-change block sends the visible
// ground-item snapshot, and a queued NPC effect sends only for a
// visible, encoded-this-tick NPC — and none of them repeats on a
// tick with no change.
func TestFlushTickPhase8Blocks(t *testing.T) {
	// Fixture music areas: one area, track 500, covering the
	// whole 64x64 region of (3200, 3200) (region 12850).
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "music-areas.json"), []byte(
		`{"areas":[{"tracks":[500]}],"tiles":{"0":{"12850":"0*4096"}}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "music-areas-classic.json"), []byte(
		`{"areas":[],"tiles":{}}`), 0o600); err != nil {
		t.Fatal(err)
	}

	ch := newFakeChannel()
	s := NewSession(ch)
	npcIdentity := new(int)
	src := &fakeFlushSource{
		owner: flushOwner(new(int)),
		npcs: []SyncNpcSnapshot{
			{Identity: npcIdentity, Index: 5, TypeID: 1234, X: 3201, Y: 3201},
		},
	}
	s.AttachSyncSource(src)
	s.SetMusicData(music.LoadAreas(dir))
	s.GroundItems().SetGroundItemSource(func() []protocol.GroundItemView {
		return []protocol.GroundItemView{
			{ID: 1, ItemID: 995, Quantity: 10, X: 3200, Y: 3200, EnabledOps: 0x1f, OwnerID: -1},
		}
	})
	// Effects carry pre-encoded packets; a destination packet
	// stands in (it parses under the server length table).
	effect := protocol.EncodeDestination(1, 2)
	decoy := protocol.EncodeDestination(9, 9)
	s.QueueNpcEffect(npcIdentity, 5, effect)
	s.QueueNpcEffect(new(int), 5, decoy)

	if err := s.FlushTick(7); err != nil {
		t.Fatalf("FlushTick: %v", err)
	}
	packets := flushFramePackets(t, ch)
	opcodes := flushOpcodes(packets)
	indexOf := func(opcode byte) int {
		for i, op := range opcodes {
			if op == opcode {
				return i
			}
		}
		return -1
	}
	// PLAY_SONG for track 500, before the tick frame: the
	// region_updates block runs before the sync frames.
	songIdx := indexOf(protocol.ServerPlaySong)
	if songIdx < 0 {
		t.Fatalf("no PLAY_SONG in %v", opcodes)
	}
	song := packets[songIdx].Payload
	if len(song) != 10 || binary.BigEndian.Uint16(song[0:2]) != 500 {
		t.Errorf("PLAY_SONG payload = %v, want track 500 + fades", song)
	}
	if tickIdx := indexOf(protocol.ServerTick); songIdx > tickIdx {
		t.Errorf("PLAY_SONG at %d, after TICK at %d", songIdx, tickIdx)
	}
	// GROUND_ITEMS with the one visible stack, also before the
	// tick frame.
	groundIdx := indexOf(protocol.ServerGroundItems)
	if groundIdx < 0 {
		t.Fatalf("no GROUND_ITEMS in %v", opcodes)
	}
	ground := packets[groundIdx].Payload
	if len(ground) != 6+protocol.GroundItemBytes {
		t.Fatalf("GROUND_ITEMS payload len = %d, want %d", len(ground), 6+protocol.GroundItemBytes)
	}
	if binary.BigEndian.Uint16(ground[4:6]) != 1 {
		t.Errorf("GROUND_ITEMS count = %d, want 1", binary.BigEndian.Uint16(ground[4:6]))
	}
	if binary.BigEndian.Uint16(ground[6+4:6+6]) != 995 {
		t.Errorf("GROUND_ITEMS stack item = %d, want 995", binary.BigEndian.Uint16(ground[6+4:6+6]))
	}
	// The effect sends, after the NPC sync; the wrong-identity
	// decoy never sends.
	frames := ch.sentBytes()
	var all []byte
	for _, f := range frames {
		all = append(all, f...)
	}
	if !bytes.Contains(all, effect) {
		t.Errorf("effect packet not sent; opcodes %v", opcodes)
	}
	if bytes.Contains(all, decoy) {
		t.Errorf("wrong-identity effect packet sent")
	}
	if npcIdx := indexOf(protocol.ServerNpcInfo); npcIdx < 0 {
		t.Fatalf("no NPC sync in %v", opcodes)
	} else if effectAt := bytes.Index(all, effect); effectAt < 0 {
		t.Fatalf("effect packet missing from frames")
	} else {
		// The effect must come after the NPC sync frame's
		// start: frames are concatenated in send order and
		// the NPC sync is the last sync packet, so the
		// effect's offset must exceed the NPC sync opcode's
		// offset within the concatenated bytes.
		npcAt := bytes.Index(all, packets[npcIdx].Payload)
		if effectAt < npcAt {
			t.Errorf("effect at byte %d, before NPC sync payload at %d", effectAt, npcAt)
		}
	}

	// Second tick, no change: no song, no ground items, no
	// effect resend (the take discarded the decoy too).
	before := len(ch.sentBytes())
	if err := s.FlushTick(8); err != nil {
		t.Fatalf("FlushTick: %v", err)
	}
	packets = packetsOf(t, ch.sentBytes()[before:])
	if hasOpcode(packets, protocol.ServerPlaySong) {
		t.Errorf("second tick sent PLAY_SONG: %v", flushOpcodes(packets))
	}
	if hasOpcode(packets, protocol.ServerGroundItems) {
		t.Errorf("second tick sent GROUND_ITEMS: %v", flushOpcodes(packets))
	}
	for _, f := range ch.sentBytes()[before:] {
		if bytes.Contains(f, effect) || bytes.Contains(f, decoy) {
			t.Errorf("second tick resent an effect")
		}
	}

	// Crossing into another 64x64 region re-sends ground items;
	// the new tile has no music area, so the gap keeps the last
	// area and no new song starts.
	src.moveOwner(3300, 3300, 0)
	before = len(ch.sentBytes())
	if err := s.FlushTick(9); err != nil {
		t.Fatalf("FlushTick: %v", err)
	}
	packets = packetsOf(t, ch.sentBytes()[before:])
	if !hasOpcode(packets, protocol.ServerGroundItems) {
		t.Errorf("region-change tick sent no GROUND_ITEMS: %v", flushOpcodes(packets))
	}
	if hasOpcode(packets, protocol.ServerPlaySong) {
		t.Errorf("gap tile started a new song: %v", flushOpcodes(packets))
	}
}

// packetsOf decodes the packets of the given frames, in order
// (flushFramePackets over an explicit frame slice, for tests
// that flush more than once on one channel).
func packetsOf(t *testing.T, frames [][]byte) []protocol.Packet {
	t.Helper()
	var out []protocol.Packet
	for _, frame := range frames {
		out = append(out, splitServerPackets(t, frame)...)
	}
	return out
}

// TestCreateViewsCarryForcedChat pins the sync-view population of
// overhead chat: a snapshot carrying ForcedChat produces a view
// whose update block carries the same text, and a snapshot without
// it produces a view with none.
func TestCreateViewsCarryForcedChat(t *testing.T) {
	s := NewSession(NewHeadlessChannel("owner"))
	text := "hello over head"
	view := s.createPlayerView(SyncPlayerSnapshot{Index: 2, X: 3201, Y: 3201, DisplayName: "Other", ForcedChat: &text}, false)
	if view.ForcedChat == nil || *view.ForcedChat != text {
		t.Errorf("player view ForcedChat = %v, want %q", view.ForcedChat, text)
	}
	plain := s.createPlayerView(SyncPlayerSnapshot{Index: 2, X: 3201, Y: 3201, DisplayName: "Other"}, false)
	if plain.ForcedChat != nil {
		t.Errorf("player view ForcedChat = %q, want nil", *plain.ForcedChat)
	}
	npcView := createNpcView(SyncNpcSnapshot{Index: 101, TypeID: 7, X: 3200, Y: 3200, ForcedChat: &text})
	if npcView.ForcedChat == nil || *npcView.ForcedChat != text {
		t.Errorf("npc view ForcedChat = %v, want %q", npcView.ForcedChat, text)
	}
}

// TestCreateViewsCarryAnimationGraphic covers the Phase 29
// animation/graphic view wiring: a snapshot's animation and
// graphics land on the player and NPC views verbatim (slot 0
// first, a cleared slot as id -1), and an empty snapshot
// sends no animation block.
func TestCreateViewsCarryAnimationGraphic(t *testing.T) {
	s := NewSession(NewHeadlessChannel("owner"))
	anim := &protocol.AnimationView{ID: 422, Delay: 1}
	graphics := []protocol.GraphicView{
		{Slot: 0, ID: 100, Height: 50, Delay: 0},
		{Slot: 2, ID: -1, Height: 0, Delay: 0},
	}
	view := s.createPlayerView(SyncPlayerSnapshot{Index: 2, X: 3201, Y: 3201, DisplayName: "Other", Animation: anim, Graphics: graphics}, false)
	if view.Animation == nil || view.Animation.ID != 422 || view.Animation.Delay != 1 {
		t.Errorf("player view Animation = %+v, want id 422 delay 1", view.Animation)
	}
	if len(view.Graphics) != 2 || view.Graphics[0].ID != 100 || view.Graphics[0].Height != 50 || view.Graphics[1].Slot != 2 || view.Graphics[1].ID != -1 {
		t.Errorf("player view Graphics = %+v, want slot-0 id 100 + cleared slot 2", view.Graphics)
	}
	plain := s.createPlayerView(SyncPlayerSnapshot{Index: 2, X: 3201, Y: 3201, DisplayName: "Other"}, false)
	if plain.Animation != nil || len(plain.Graphics) != 0 {
		t.Errorf("plain view Animation/Graphics = %+v/%v, want nil/empty", plain.Animation, plain.Graphics)
	}
	npcView := createNpcView(SyncNpcSnapshot{Index: 101, TypeID: 7, X: 3200, Y: 3200, Animation: anim, Graphics: graphics})
	if npcView.Animation == nil || npcView.Animation.ID != 422 {
		t.Errorf("npc view Animation = %+v, want id 422", npcView.Animation)
	}
	if len(npcView.Graphics) != 2 || npcView.Graphics[1].ID != -1 {
		t.Errorf("npc view Graphics = %+v, want 2 entries with a cleared slot", npcView.Graphics)
	}
}
