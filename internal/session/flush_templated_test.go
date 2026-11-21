package session

import (
	"encoding/binary"
	"testing"

	"tsps-go/internal/protocol"
)

// templatedAreaFixture is a hand-built SyncArea snapshot source
// for the rebuild tests: a stable comparable identity, a
// mutable scene version, and a Build closure recording the
// center chunks it is asked to render.
type templatedAreaFixture struct {
	identity  *struct{ name string }
	scene     *TemplatedSceneSnapshot
	palette   [4][13][13]int32
	regionIDs []int
	builds    [][2]int
}

func newTemplatedAreaFixture(name string, paletteValue int32, regionIDs []int) *templatedAreaFixture {
	f := &templatedAreaFixture{
		identity:  &struct{ name string }{name},
		regionIDs: regionIDs,
	}
	for plane := range f.palette {
		for x := range f.palette[plane] {
			for y := range f.palette[plane][x] {
				f.palette[plane][x][y] = -1
			}
		}
	}
	f.palette[0][6][6] = paletteValue
	f.scene = &TemplatedSceneSnapshot{
		SceneVersion: 0,
		Build: func(centerChunkX, centerChunkY int) ([4][13][13]int32, []int) {
			f.builds = append(f.builds, [2]int{centerChunkX, centerChunkY})
			return f.palette, f.regionIDs
		},
	}
	return f
}

func (f *templatedAreaFixture) snapshot() *SyncAreaSnapshot {
	return &SyncAreaSnapshot{Identity: f.identity, Templated: f.scene}
}

// flushPacketsSince decodes the packets of the frames sent after
// the first n frames, isolating one FlushTick's output.
func flushPacketsSince(t *testing.T, ch *fakeChannel, n int) []protocol.Packet {
	t.Helper()
	var out []protocol.Packet
	for _, frame := range ch.sentBytes()[n:] {
		out = append(out, splitServerPackets(t, frame)...)
	}
	return out
}

// parseRebuildRegion reads a REBUILD_REGION payload: regionY,
// reload byte, regionX, key count, the bit-packed palette, then
// the keys (see EncodeRebuildRegion).
func parseRebuildRegion(t *testing.T, payload []byte) (regionX, regionY uint16, reload byte, keys [][4]int32, palette [4][13][13]int32) {
	t.Helper()
	if len(payload) < 7 {
		t.Fatalf("rebuild-region payload too short: %d bytes", len(payload))
	}
	regionY = binary.BigEndian.Uint16(payload[0:2])
	reload = payload[2]
	regionX = binary.BigEndian.Uint16(payload[3:5])
	count := int(binary.BigEndian.Uint16(payload[5:7]))
	bitOffset := 56
	readBits := func(n int) uint32 {
		var value uint32
		for i := 0; i < n; i++ {
			bit := (payload[bitOffset>>3] >> (7 - (bitOffset & 7))) & 1
			value = (value << 1) | uint32(bit)
			bitOffset++
		}
		return value
	}
	for plane := 0; plane < 4; plane++ {
		for x := 0; x < 13; x++ {
			for y := 0; y < 13; y++ {
				if readBits(1) == 0 {
					palette[plane][x][y] = -1
				} else {
					palette[plane][x][y] = int32(readBits(26))
				}
			}
		}
	}
	bitBytes := (bitOffset - 56 + 7) / 8
	if len(payload) != 7+bitBytes+count*16 {
		t.Fatalf("rebuild-region payload len %d, want %d for %d keys", len(payload), 7+bitBytes+count*16, count)
	}
	for i := 0; i < count; i++ {
		var key [4]int32
		for j := 0; j < 4; j++ {
			key[j] = int32(binary.BigEndian.Uint32(payload[7+bitBytes+i*16+j*4:]))
		}
		keys = append(keys, key)
	}
	return regionX, regionY, reload, keys, palette
}

func newTemplatedFlushHarness() (*Session, *fakeChannel, *fakeFlushSource) {
	ch := newFakeChannel()
	s := NewSession(ch)
	src := &fakeFlushSource{owner: flushOwner(1)}
	s.AttachSyncSource(src)
	// The provider echoes the region id in the key's first word,
	// so key order and region-id resolution are observable.
	s.SetXteaProvider(func(regionID int) [4]int32 { return [4]int32{int32(regionID), 0, 0, 0} })
	return s, ch, src
}

func TestFlushTickTemplatedRebuild(t *testing.T) {
	s, ch, src := newTemplatedFlushHarness()
	fix := newTemplatedAreaFixture("instance", 0x123456, []int{257})
	src.owner.SyncArea = fix.snapshot()

	if err := s.FlushTick(1); err != nil {
		t.Fatal(err)
	}
	packets := flushPacketsSince(t, ch, 0)
	if !hasOpcode(packets, protocol.ServerRebuildRegion) {
		t.Fatalf("no REBUILD_REGION in %v", flushOpcodes(packets))
	}
	if hasOpcode(packets, protocol.ServerRebuildNormal) {
		t.Fatalf("unexpected REBUILD_NORMAL in %v", flushOpcodes(packets))
	}
	regionX, regionY, reload, keys, palette := parseRebuildRegion(t, findOpcode(t, packets, protocol.ServerRebuildRegion))
	if regionX != 400 || regionY != 400 {
		t.Fatalf("rebuild center = (%d,%d), want (400,400)", regionX, regionY)
	}
	if reload != 1 {
		t.Fatalf("reload byte = %d, want 1 (forceReload)", reload)
	}
	if len(keys) != 1 || keys[0][0] != 257 {
		t.Fatalf("keys = %v, want one key for region 257", keys)
	}
	if palette[0][6][6] != 0x123456 {
		t.Fatalf("palette center = %#x, want 0x123456", palette[0][6][6])
	}
	if len(fix.builds) != 1 || fix.builds[0] != [2]int{400, 400} {
		t.Fatalf("Build calls = %v, want one at (400,400)", fix.builds)
	}

	// A steady second tick sends no rebuild at all.
	sent := len(ch.sentBytes())
	if err := s.FlushTick(2); err != nil {
		t.Fatal(err)
	}
	packets = flushPacketsSince(t, ch, sent)
	if hasOpcode(packets, protocol.ServerRebuildRegion) || hasOpcode(packets, protocol.ServerRebuildNormal) {
		t.Fatalf("steady tick rebuilt: %v", flushOpcodes(packets))
	}
}

func TestFlushTickTemplatedSceneVersionBump(t *testing.T) {
	s, ch, src := newTemplatedFlushHarness()
	fix := newTemplatedAreaFixture("instance", 0x123456, []int{257})
	src.owner.SyncArea = fix.snapshot()
	if err := s.FlushTick(1); err != nil {
		t.Fatal(err)
	}
	// A scene-version bump re-sends the region packet, with a
	// fresh Build: the palette reflects the new chunk layout.
	fix.scene.SceneVersion = 1
	fix.palette[0][6][6] = 0x654321
	sent := len(ch.sentBytes())
	if err := s.FlushTick(2); err != nil {
		t.Fatal(err)
	}
	packets := flushPacketsSince(t, ch, sent)
	if !hasOpcode(packets, protocol.ServerRebuildRegion) {
		t.Fatalf("version bump sent no REBUILD_REGION: %v", flushOpcodes(packets))
	}
	_, _, _, _, palette := parseRebuildRegion(t, findOpcode(t, packets, protocol.ServerRebuildRegion))
	if palette[0][6][6] != 0x654321 {
		t.Fatalf("rebuilt palette center = %#x, want 0x654321", palette[0][6][6])
	}
}

func TestFlushTickEnterAndLeaveTemplated(t *testing.T) {
	s, ch, src := newTemplatedFlushHarness()
	// Steady in the main world: the first flush sends a normal
	// rebuild.
	if err := s.FlushTick(1); err != nil {
		t.Fatal(err)
	}
	if !hasOpcode(flushPacketsSince(t, ch, 0), protocol.ServerRebuildNormal) {
		t.Fatal("first main-world flush sent no REBUILD_NORMAL")
	}
	sent := len(ch.sentBytes())
	if err := s.FlushTick(2); err != nil {
		t.Fatal(err)
	}
	if hasOpcode(flushPacketsSince(t, ch, sent), protocol.ServerRebuildNormal) {
		t.Fatal("steady main-world tick rebuilt")
	}

	// Entering the instance (same tile) sends a region packet.
	fix := newTemplatedAreaFixture("instance", 0x123456, []int{257})
	src.owner.SyncArea = fix.snapshot()
	sent = len(ch.sentBytes())
	if err := s.FlushTick(3); err != nil {
		t.Fatal(err)
	}
	packets := flushPacketsSince(t, ch, sent)
	if !hasOpcode(packets, protocol.ServerRebuildRegion) {
		t.Fatalf("entering the instance sent no REBUILD_REGION: %v", flushOpcodes(packets))
	}

	// Leaving sends a normal rebuild with forceReload (the
	// inverted reload byte reads 0) and provider-resolved keys.
	src.owner.SyncArea = nil
	sent = len(ch.sentBytes())
	if err := s.FlushTick(4); err != nil {
		t.Fatal(err)
	}
	packets = flushPacketsSince(t, ch, sent)
	if hasOpcode(packets, protocol.ServerRebuildRegion) {
		t.Fatalf("leaving sent REBUILD_REGION: %v", flushOpcodes(packets))
	}
	if !hasOpcode(packets, protocol.ServerRebuildNormal) {
		t.Fatalf("leaving sent no REBUILD_NORMAL: %v", flushOpcodes(packets))
	}
	regionX, regionY, reload, keys := parseRebuild(t, findOpcode(t, packets, protocol.ServerRebuildNormal))
	if regionX != 400 || regionY != 400 {
		t.Fatalf("rebuild center = (%d,%d), want (400,400)", regionX, regionY)
	}
	if reload != 0 {
		t.Fatalf("reload byte = %d, want 0 (forceReload set)", reload)
	}
	// Center chunk (400,400): regions (49..50, 49..50), mapX
	// outer, mapY inner.
	wantKeys := []int32{12593, 12594, 12849, 12850}
	if len(keys) != len(wantKeys) {
		t.Fatalf("keys = %v, want %v", keys, wantKeys)
	}
	for i, want := range wantKeys {
		if keys[i][0] != want {
			t.Fatalf("keys[%d] = %v, want first word %d", i, keys[i], want)
		}
	}

	// A later base move (no area involved) rebuilds normally
	// WITHOUT forceReload (reload byte 1).
	src.moveOwner(3400, 3400, 0)
	sent = len(ch.sentBytes())
	if err := s.FlushTick(5); err != nil {
		t.Fatal(err)
	}
	packets = flushPacketsSince(t, ch, sent)
	_, _, reload, _ = parseRebuild(t, findOpcode(t, packets, protocol.ServerRebuildNormal))
	if reload != 1 {
		t.Fatalf("reload byte = %d, want 1 (no forceReload)", reload)
	}
}

func TestFlushTickPlainPrivateAreaNoRebuild(t *testing.T) {
	s, ch, src := newTemplatedFlushHarness()
	areaID := &struct{ name string }{"plain"}
	src.owner.SyncArea = &SyncAreaSnapshot{Identity: areaID, Templated: nil}

	// First flush inside a plain private area: no rebuild (the
	// cache map draws it; sceneRebuildFor returns null).
	if err := s.FlushTick(1); err != nil {
		t.Fatal(err)
	}
	packets := flushPacketsSince(t, ch, 0)
	if hasOpcode(packets, protocol.ServerRebuildRegion) || hasOpcode(packets, protocol.ServerRebuildNormal) {
		t.Fatalf("plain-area first flush rebuilt: %v", flushOpcodes(packets))
	}
	// Moving the scene base inside the area: still no rebuild.
	src.moveOwner(3400, 3400, 0)
	sent := len(ch.sentBytes())
	if err := s.FlushTick(2); err != nil {
		t.Fatal(err)
	}
	packets = flushPacketsSince(t, ch, sent)
	if hasOpcode(packets, protocol.ServerRebuildRegion) || hasOpcode(packets, protocol.ServerRebuildNormal) {
		t.Fatalf("plain-area base move rebuilt: %v", flushOpcodes(packets))
	}
	// Leaving the area: a normal rebuild with forceReload.
	src.owner.SyncArea = nil
	sent = len(ch.sentBytes())
	if err := s.FlushTick(3); err != nil {
		t.Fatal(err)
	}
	packets = flushPacketsSince(t, ch, sent)
	if !hasOpcode(packets, protocol.ServerRebuildNormal) {
		t.Fatalf("leaving the plain area sent no REBUILD_NORMAL: %v", flushOpcodes(packets))
	}
	_, _, reload, _ := parseRebuild(t, findOpcode(t, packets, protocol.ServerRebuildNormal))
	if reload != 0 {
		t.Fatalf("reload byte = %d, want 0 (forceReload set)", reload)
	}
}

func TestFlushTickTemplatedNilBuild(t *testing.T) {
	s, ch, src := newTemplatedFlushHarness()
	areaID := &struct{ name string }{"instance"}
	src.owner.SyncArea = &SyncAreaSnapshot{
		Identity:  areaID,
		Templated: &TemplatedSceneSnapshot{SceneVersion: 0, Build: nil},
	}
	// A nil Build degrades to no packet, without error, and the
	// replay bookkeeping still records the scene (no repeat).
	if err := s.FlushTick(1); err != nil {
		t.Fatal(err)
	}
	if hasOpcode(flushPacketsSince(t, ch, 0), protocol.ServerRebuildRegion) {
		t.Fatal("nil Build sent REBUILD_REGION")
	}
	sent := len(ch.sentBytes())
	if err := s.FlushTick(2); err != nil {
		t.Fatal(err)
	}
	if hasOpcode(flushPacketsSince(t, ch, sent), protocol.ServerRebuildRegion) {
		t.Fatal("nil Build sent REBUILD_REGION on the steady tick")
	}
}

func TestFlushTickReplacementSenderHook(t *testing.T) {
	s, ch, src := newTemplatedFlushHarness()
	var calls [][2]int
	s.SetRegionReplacementSender(func(tileX, tileY int) {
		calls = append(calls, [2]int{tileX, tileY})
	})
	// First flush: the window changed (never replayed); the
	// hook fires with the scene center. Scene base for
	// (3200,3200) is (3152,3152), so the center is (3200,3200).
	if err := s.FlushTick(1); err != nil {
		t.Fatal(err)
	}
	if len(calls) != 1 || calls[0] != [2]int{3200, 3200} {
		t.Fatalf("hook calls = %v, want [(3200,3200)]", calls)
	}
	// Steady tick: the window did not change; no call.
	if err := s.FlushTick(2); err != nil {
		t.Fatal(err)
	}
	if len(calls) != 1 {
		t.Fatalf("hook calls = %v, want no second call", calls)
	}
	// A pure plane change: the window did not change (base
	// terms only) and no rebuild fires (level excluded).
	src.moveOwner(3200, 3200, 1)
	sent := len(ch.sentBytes())
	if err := s.FlushTick(3); err != nil {
		t.Fatal(err)
	}
	if len(calls) != 1 {
		t.Fatalf("hook calls = %v, want no call on a plane change", calls)
	}
	if hasOpcode(flushPacketsSince(t, ch, sent), protocol.ServerRebuildNormal) {
		t.Fatal("plane change sent REBUILD_NORMAL")
	}
	// A base move: the window changed; the hook fires with the
	// new scene center (base (3248,3248) -> center (3296,3296)).
	src.moveOwner(3300, 3300, 1)
	if err := s.FlushTick(4); err != nil {
		t.Fatal(err)
	}
	if len(calls) != 2 || calls[1] != [2]int{3296, 3296} {
		t.Fatalf("hook calls = %v, want second call (3296,3296)", calls)
	}
	// Clearing the hook stops the sends.
	s.SetRegionReplacementSender(nil)
	src.moveOwner(3500, 3500, 1)
	if err := s.FlushTick(5); err != nil {
		t.Fatal(err)
	}
	if len(calls) != 2 {
		t.Fatalf("hook calls = %v, want no call after clearing", calls)
	}
}
