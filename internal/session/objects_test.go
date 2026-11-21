package session

import (
	"encoding/binary"
	"fmt"
	"testing"

	"tsps-go/internal/protocol"
)

// fakeObjectSource is an ObjectSource test double with fixed
// answers, recording standing/map-object lookups and delta
// reads.
type fakeObjectSource struct {
	scene      []ObjectView
	removed    []ObjectView
	ownerScene []ObjectView
	standing   map[objectShownKey]ObjectView
	mapObjects map[string]ObjectView
	deltas     []ObjectDeltaView

	deltaCalls       int
	lastStandingArgs [4]int
	lastMapArgs      [4]int
}

func (f *fakeObjectSource) SceneObjects(baseX, baseY, size int) []ObjectView { return f.scene }
func (f *fakeObjectSource) RemovedObjects() []ObjectView                     { return f.removed }
func (f *fakeObjectSource) OwnerSceneObjects(baseX, baseY, size int) []ObjectView {
	return f.ownerScene
}
func (f *fakeObjectSource) StandingObject(x, y, z, shape int) (ObjectView, bool) {
	f.lastStandingArgs = [4]int{x, y, z, shape}
	v, ok := f.standing[objectShownKey{x: x, y: y, z: z, typ: shape}]
	return v, ok
}
func (f *fakeObjectSource) MapObject(id, x, y, z int) (ObjectView, bool) {
	f.lastMapArgs = [4]int{id, x, y, z}
	v, ok := f.mapObjects[fmt.Sprintf("%d,%d,%d,%d", id, x, y, z)]
	return v, ok
}
func (f *fakeObjectSource) Deltas(cycle int64) []ObjectDeltaView {
	f.deltaCalls++
	return f.deltas
}

// obj builds an unowned ObjectView fixture.
func obj(id, x, y, z, typ, face int) ObjectView {
	return ObjectView{ID: id, X: x, Y: y, Z: z, Type: typ, Face: face}
}

// packetsSince returns the packets from frames the channel sent
// after the first before frames, in send order. Frames
// accumulate on the fake channel, so each observation slices
// only the frames its own flush produced.
func packetsSince(t *testing.T, ch *fakeChannel, before int) []protocol.Packet {
	t.Helper()
	var out []protocol.Packet
	for _, frame := range ch.sentBytes()[before:] {
		out = append(out, splitServerPackets(t, frame)...)
	}
	return out
}

// objectPackets flushes the session and returns the packets
// that flush sent, in send order.
func objectPackets(t *testing.T, s *Session, ch *fakeChannel) []protocol.Packet {
	t.Helper()
	before := len(ch.sentBytes())
	if err := s.Flush(); err != nil {
		t.Fatalf("Flush: %v", err)
	}
	return packetsSince(t, ch, before)
}

// parseLocAdd reads a LOC_ADD_CHANGE payload.
func parseLocAdd(t *testing.T, p []byte) (id, x, y, level, shape, rotation int) {
	t.Helper()
	if len(p) != 8 {
		t.Fatalf("LOC_ADD_CHANGE payload len = %d, want 8", len(p))
	}
	return int(binary.BigEndian.Uint16(p[0:2])), int(binary.BigEndian.Uint16(p[2:4])),
		int(binary.BigEndian.Uint16(p[4:6])), int(p[6]), int(p[7]) >> 2, int(p[7]) & 3
}

// parseLocDel reads a LOC_DEL payload.
func parseLocDel(t *testing.T, p []byte) (x, y, level, shape, rotation int) {
	t.Helper()
	if len(p) != 6 {
		t.Fatalf("LOC_DEL payload len = %d, want 6", len(p))
	}
	return int(binary.BigEndian.Uint16(p[0:2])), int(binary.BigEndian.Uint16(p[2:4])),
		int(p[4]), int(p[5]) >> 2, int(p[5]) & 3
}

// locFrames returns the loc add/del packets in order, as
// "add:<id>@<x>,<y>,<z>" / "del@<x>,<y>,<z>" strings.
func locFrames(t *testing.T, packets []protocol.Packet) []string {
	t.Helper()
	var out []string
	for _, p := range packets {
		switch p.Opcode {
		case protocol.ServerLocAddChange:
			id, x, y, z, _, _ := parseLocAdd(t, p.Payload)
			out = append(out, fmt.Sprintf("add:%d@%d,%d,%d", id, x, y, z))
		case protocol.ServerLocDel:
			x, y, z, _, _ := parseLocDel(t, p.Payload)
			out = append(out, fmt.Sprintf("del@%d,%d,%d", x, y, z))
		}
	}
	return out
}

func equalStrings(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("frames = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("frames = %v, want %v", got, want)
		}
	}
}

func TestObjectSyncSnapshotOrdering(t *testing.T) {
	// Removals before additions, hidden removals with the
	// removals, owner objects last (ObjectManager.ts:184-253).
	ch := newFakeChannel()
	s := NewSession(ch)
	src := &fakeObjectSource{
		removed:    []ObjectView{obj(1, 3200, 3200, 0, 10, 0)},
		scene:      []ObjectView{obj(2, 3201, 3200, 0, 10, 0)},
		ownerScene: []ObjectView{obj(3, 3202, 3200, 0, 10, 0)},
		mapObjects: map[string]ObjectView{
			"4,3203,3200,0": obj(4, 3203, 3200, 0, 10, 0),
		},
	}
	s.Objects().SetObjectSource(src, "self")
	if !s.Objects().HideMapObjectFor(4, 3203, 3200, 0) {
		t.Fatal("HideMapObjectFor: want true")
	}
	// Drain the hide frame so only the snapshot is observed.
	_ = objectPackets(t, s, ch)

	s.Objects().OnRegionChange(3152, 3152)
	packets := objectPackets(t, s, ch)
	equalStrings(t, locFrames(t, packets), []string{
		"del@3200,3200,0", // removed object
		"del@3203,3200,0", // hidden object
		"add:2@3201,3200,0",
		"add:3@3202,3200,0", // owner object, sent raw
	})
}

func TestObjectSyncSceneBox(t *testing.T) {
	// The scene box is [base, base+104) in x/y, every plane
	// (ObjectManager.ts:195-197): edges inclusive/exclusive as
	// TS writes them, planes unfiltered.
	ch := newFakeChannel()
	s := NewSession(ch)
	src := &fakeObjectSource{
		scene: []ObjectView{
			obj(1, 3152, 3152, 0, 10, 0), // low corner: in
			obj(2, 3255, 3255, 0, 10, 0), // high corner: in
			obj(3, 3256, 3200, 0, 10, 0), // x past edge: out
			obj(4, 3151, 3200, 0, 10, 0), // x below edge: out
			obj(5, 3200, 3256, 0, 10, 0), // y past edge: out
			obj(6, 3200, 3151, 0, 10, 0), // y below edge: out
			obj(7, 3200, 3200, 1, 10, 0), // plane 1: in
			obj(8, 3200, 3200, 3, 10, 0), // plane 3: in
		},
		removed: []ObjectView{
			obj(9, 3256, 3200, 0, 10, 0),  // removed, out of box: skipped
			obj(10, 3200, 3200, 2, 10, 0), // removed, plane 2: sent
		},
	}
	s.Objects().SetObjectSource(src, "self")
	s.Objects().OnRegionChange(3152, 3152)
	packets := objectPackets(t, s, ch)
	equalStrings(t, locFrames(t, packets), []string{
		"del@3200,3200,2",
		"add:1@3152,3152,0",
		"add:2@3255,3255,0",
		"add:7@3200,3200,1",
		"add:8@3200,3200,3",
	})
}

func TestObjectSyncOwnerFilter(t *testing.T) {
	// Unowned objects are global; owned objects go only to
	// their owner (ObjectManager.ts:233-235).
	ch := newFakeChannel()
	s := NewSession(ch)
	owned := obj(2, 3201, 3200, 0, 10, 0)
	owned.Owner = "self"
	others := obj(3, 3202, 3200, 0, 10, 0)
	others.Owner = "other"
	src := &fakeObjectSource{
		scene: []ObjectView{obj(1, 3200, 3200, 0, 10, 0), owned, others},
	}
	s.Objects().SetObjectSource(src, "self")
	s.Objects().OnRegionChange(3152, 3152)
	packets := objectPackets(t, s, ch)
	equalStrings(t, locFrames(t, packets), []string{
		"add:1@3200,3200,0",
		"add:2@3201,3200,0",
	})
}

func TestObjectSyncDrainDeltas(t *testing.T) {
	ch := newFakeChannel()
	s := NewSession(ch)
	// Replay the scene rooted at (3152, 3152), level 0, as a
	// prior FlushTick would have.
	s.hasReplayedScene = true
	s.replayedSceneBaseX, s.replayedSceneBaseY = 3152, 3152
	s.replayedSceneLevel = 0
	other := obj(6, 3200, 3200, 0, 10, 0)
	other.Owner = "other"
	src := &fakeObjectSource{
		deltas: []ObjectDeltaView{
			{Object: obj(1, 3200, 3200, 0, 10, 0)},                // in-scene spawn: sent
			{Object: obj(2, 3400, 3400, 0, 10, 0)},                // out-of-scene spawn: skipped
			{Object: obj(3, 3200, 3200, 1, 10, 0)},                // off-level spawn: skipped
			{Object: obj(4, 3201, 3200, 0, 10, 0), Removed: true}, // in-scene despawn: removal
			{Object: obj(-1, 3202, 3200, 0, 10, 0)},               // id -1 forces despawn
			{Object: other},                                       // another owner's: skipped
		},
	}
	s.Objects().SetObjectSource(src, "self")
	s.Objects().DrainDeltas(9)
	packets := objectPackets(t, s, ch)
	equalStrings(t, locFrames(t, packets), []string{
		"add:1@3200,3200,0",
		"del@3201,3200,0",
		"del@3202,3200,0",
	})
	if src.deltaCalls != 1 {
		t.Fatalf("Deltas calls = %d, want 1", src.deltaCalls)
	}
}

func TestObjectSyncMissedRemovalReplay(t *testing.T) {
	// A shown object removed while its tile is outside the
	// scene replays on re-entry, restoring the standing map
	// object where TS does (ObjectManager.ts:88-111).
	newSetup := func(t *testing.T) (*Session, *fakeChannel, *fakeObjectSource) {
		t.Helper()
		ch := newFakeChannel()
		s := NewSession(ch)
		src := &fakeObjectSource{
			scene: []ObjectView{obj(10, 3200, 3200, 0, 10, 1)},
		}
		s.Objects().SetObjectSource(src, "self")
		// Show the object in the first scene.
		s.Objects().OnRegionChange(3152, 3152)
		_ = objectPackets(t, s, ch)
		// The player walks away: replay a far scene, then the
		// removal arrives while the tile is out of scene.
		s.hasReplayedScene = true
		s.replayedSceneBaseX, s.replayedSceneBaseY = 3400, 3400
		s.replayedSceneLevel = 0
		src.deltas = []ObjectDeltaView{{Object: obj(10, 3200, 3200, 0, 10, 1), Removed: true}}
		s.Objects().DrainDeltas(10)
		if got := locFrames(t, objectPackets(t, s, ch)); len(got) != 0 {
			t.Fatalf("away drain sent %v, want nothing", got)
		}
		return s, ch, src
	}

	t.Run("standing restored", func(t *testing.T) {
		s, ch, src := newSetup(t)
		src.standing = map[objectShownKey]ObjectView{
			{x: 3200, y: 3200, z: 0, typ: 10}: obj(11, 3200, 3200, 0, 10, 0),
		}
		src.scene = nil
		s.Objects().OnRegionChange(3152, 3152)
		packets := objectPackets(t, s, ch)
		equalStrings(t, locFrames(t, packets), []string{"add:11@3200,3200,0"})
	})
	t.Run("standing also removed", func(t *testing.T) {
		s, ch, src := newSetup(t)
		standing := obj(11, 3200, 3200, 0, 10, 0)
		src.standing = map[objectShownKey]ObjectView{
			{x: 3200, y: 3200, z: 0, typ: 10}: standing,
		}
		src.removed = []ObjectView{standing}
		src.scene = nil
		s.Objects().OnRegionChange(3152, 3152)
		packets := objectPackets(t, s, ch)
		// Two removal frames: the missed-removal replay
		// (which falls back to the removal because the
		// standing object is itself removed) AND the
		// removed-objects loop, which replays every removed
		// object in the scene — TS sends both
		// (ObjectManager.ts:88-111, 198-212).
		equalStrings(t, locFrames(t, packets), []string{"del@3200,3200,0", "del@3200,3200,0"})
	})
	t.Run("no standing object", func(t *testing.T) {
		s, ch, src := newSetup(t)
		src.scene = nil
		s.Objects().OnRegionChange(3152, 3152)
		packets := objectPackets(t, s, ch)
		equalStrings(t, locFrames(t, packets), []string{"del@3200,3200,0"})
	})
	t.Run("still out of scene", func(t *testing.T) {
		s, ch, src := newSetup(t)
		src.scene = nil
		// Re-entering a scene that still excludes the tile
		// replays nothing; the missed entry survives.
		s.Objects().OnRegionChange(3400, 3400)
		packets := objectPackets(t, s, ch)
		if got := locFrames(t, packets); len(got) != 0 {
			t.Fatalf("out-of-scene replay sent %v, want nothing", got)
		}
		// Coming back to the tile's scene replays it now.
		s.Objects().OnRegionChange(3152, 3152)
		packets = objectPackets(t, s, ch)
		equalStrings(t, locFrames(t, packets), []string{"del@3200,3200,0"})
	})
	t.Run("identity mismatch not recorded", func(t *testing.T) {
		s, ch, src := newSetup(t)
		// The shown entry (face 1) differs from a replacement
		// shown later: re-show face 2 in the same slot, then
		// remove the face-1 object while away — the missed
		// removal must NOT record (recordMissedRemoval
		// requires identity equality).
		src.scene = []ObjectView{obj(10, 3200, 3200, 0, 10, 2)}
		s.Objects().OnRegionChange(3152, 3152)
		_ = objectPackets(t, s, ch)
		s.hasReplayedScene = true
		s.replayedSceneBaseX, s.replayedSceneBaseY = 3400, 3400
		src.deltas = []ObjectDeltaView{{Object: obj(10, 3200, 3200, 0, 10, 1), Removed: true}}
		s.Objects().DrainDeltas(11)
		src.scene = nil
		s.Objects().OnRegionChange(3152, 3152)
		packets := objectPackets(t, s, ch)
		if got := locFrames(t, packets); len(got) != 0 {
			t.Fatalf("mismatched replay sent %v, want nothing", got)
		}
	})
}

func TestObjectSyncHideShow(t *testing.T) {
	ch := newFakeChannel()
	s := NewSession(ch)
	base := obj(4, 3200, 3200, 0, 10, 0)
	src := &fakeObjectSource{
		mapObjects: map[string]ObjectView{"4,3200,3200,0": base},
	}
	s.Objects().SetObjectSource(src, "self")

	// Hiding a missing base answers false and sends nothing.
	if s.Objects().HideMapObjectFor(99, 3200, 3200, 0) {
		t.Fatal("HideMapObjectFor(missing): want false")
	}
	// Hiding the base sends the removal at once.
	if !s.Objects().HideMapObjectFor(4, 3200, 3200, 0) {
		t.Fatal("HideMapObjectFor: want true")
	}
	packets := objectPackets(t, s, ch)
	equalStrings(t, locFrames(t, packets), []string{"del@3200,3200,0"})

	// The hidden removal re-sends on every scene load.
	s.Objects().OnRegionChange(3152, 3152)
	packets = objectPackets(t, s, ch)
	equalStrings(t, locFrames(t, packets), []string{"del@3200,3200,0"})

	// Showing sends the add and clears the hidden entry.
	s.Objects().ShowMapObjectFor(4, 3200, 3200, 0)
	packets = objectPackets(t, s, ch)
	equalStrings(t, locFrames(t, packets), []string{"add:4@3200,3200,0"})
	s.Objects().OnRegionChange(3152, 3152)
	packets = objectPackets(t, s, ch)
	if got := locFrames(t, packets); len(got) != 0 {
		t.Fatalf("post-show snapshot sent %v, want nothing", got)
	}

	// Showing without a hidden entry sends nothing.
	s.Objects().ShowMapObjectFor(4, 3200, 3200, 0)
	packets = objectPackets(t, s, ch)
	if got := locFrames(t, packets); len(got) != 0 {
		t.Fatalf("show-without-hide sent %v, want nothing", got)
	}
}

func TestObjectSyncHideShowClampsHighPlanes(t *testing.T) {
	// MapObjects.get clamps z >= 4 to 0 (MapObjects.ts:107-109);
	// the sync applies the clamp before querying.
	ch := newFakeChannel()
	s := NewSession(ch)
	src := &fakeObjectSource{
		mapObjects: map[string]ObjectView{"4,3200,3200,0": obj(4, 3200, 3200, 0, 10, 0)},
	}
	s.Objects().SetObjectSource(src, "self")
	s.Objects().HideMapObjectFor(4, 3200, 3200, 5)
	if got := src.lastMapArgs; got != [4]int{4, 3200, 3200, 0} {
		t.Fatalf("MapObject args = %v, want [4 3200 3200 0]", got)
	}
}

func TestObjectSyncNoSource(t *testing.T) {
	// With no source installed, every entry point is a silent
	// no-op, and a nil receiver is safe (the zero-value
	// Session case).
	ch := newFakeChannel()
	s := NewSession(ch)
	s.Objects().OnRegionChange(3152, 3152)
	s.Objects().DrainDeltas(1)
	if s.Objects().HideMapObjectFor(1, 3200, 3200, 0) {
		t.Fatal("HideMapObjectFor without source: want false")
	}
	s.Objects().ShowMapObjectFor(1, 3200, 3200, 0)
	packets := objectPackets(t, s, ch)
	if got := locFrames(t, packets); len(got) != 0 {
		t.Fatalf("no-source sync sent %v, want nothing", got)
	}
	if s.Objects() == nil {
		t.Fatal("NewSession Objects() is nil")
	}
	var nilSync *ObjectSync
	nilSync.OnRegionChange(0, 0)
	nilSync.DrainDeltas(1)
	nilSync.HideMapObjectFor(1, 0, 0, 0)
	nilSync.ShowMapObjectFor(1, 0, 0, 0)
	nilSync.SetObjectSource(nil, nil)
	if nilSync.ObjectSource() != nil {
		t.Fatal("nil ObjectSource: want nil")
	}
	var nilSession *Session
	if nilSession.Objects() != nil {
		t.Fatal("nil Session Objects(): want nil")
	}
}

func TestIsTileInScene(t *testing.T) {
	s := NewSession(newFakeChannel())
	// No replayed scene: everything is out.
	if s.IsTileInScene(3200, 3200, 0) {
		t.Fatal("IsTileInScene without replay: want false")
	}
	s.hasReplayedScene = true
	s.replayedSceneBaseX, s.replayedSceneBaseY = 3152, 3152
	s.replayedSceneLevel = 0
	cases := []struct {
		x, y, z int
		want    bool
	}{
		{3152, 3152, 0, true},
		{3255, 3255, 0, true},
		{3256, 3200, 0, false},
		{3151, 3200, 0, false},
		{3200, 3256, 0, false},
		{3200, 3200, 1, false}, // off-level: CrossFloor degenerates
		{3200, 3200, 0, true},
	}
	for _, tc := range cases {
		if got := s.IsTileInScene(tc.x, tc.y, tc.z); got != tc.want {
			t.Fatalf("IsTileInScene(%d,%d,%d) = %v, want %v", tc.x, tc.y, tc.z, got, tc.want)
		}
	}
	var nilSession *Session
	if nilSession.IsTileInScene(0, 0, 0) {
		t.Fatal("nil IsTileInScene: want false")
	}
}

func TestSharedObjectDeltaFeed(t *testing.T) {
	// The fetch runs once per cycle, shared by every caller in
	// that cycle; a new cycle refetches.
	calls := 0
	feed := NewSharedObjectDeltaFeed(func() []ObjectDeltaView {
		calls++
		return []ObjectDeltaView{{Object: obj(calls, 3200, 3200, 0, 10, 0)}}
	})
	first := feed.Deltas(7)
	second := feed.Deltas(7)
	if calls != 1 {
		t.Fatalf("fetch calls after two Deltas(7) = %d, want 1", calls)
	}
	if len(first) != 1 || len(second) != 1 || first[0].Object.ID != 1 || second[0].Object.ID != 1 {
		t.Fatalf("Deltas(7) = %v / %v, want the shared snapshot", first, second)
	}
	third := feed.Deltas(8)
	if calls != 2 {
		t.Fatalf("fetch calls after Deltas(8) = %d, want 2", calls)
	}
	if len(third) != 1 || third[0].Object.ID != 2 {
		t.Fatalf("Deltas(8) = %v, want the fresh snapshot", third)
	}
	// Nil feed and nil fetch answer nil.
	var nilFeed *SharedObjectDeltaFeed
	if nilFeed.Deltas(1) != nil {
		t.Fatal("nil feed Deltas: want nil")
	}
	if NewSharedObjectDeltaFeed(nil).Deltas(1) != nil {
		t.Fatal("nil fetch Deltas: want nil")
	}
}

func TestFlushTickObjectBlocks(t *testing.T) {
	// The object blocks inside real FlushTick: the initial
	// snapshot after the rebuild and before the tick frame
	// (PlayerSession.ts:509-513), the delta drain before the
	// tick frame on later ticks, and the non-initial snapshot
	// after the NPC sync (PlayerSession.ts:648-650).
	ch := newFakeChannel()
	s := NewSession(ch)
	src := &fakeFlushSource{owner: flushOwner(new(int))}
	s.AttachSyncSource(src)
	fake := &fakeObjectSource{
		scene: []ObjectView{
			obj(100, 3200, 3200, 0, 10, 0), // in the first scene
			obj(300, 3400, 3400, 0, 10, 0), // in the moved scene
		},
		deltas: []ObjectDeltaView{
			{Object: obj(200, 3200, 3200, 0, 10, 0)},
		},
	}
	s.Objects().SetObjectSource(fake, "self")

	// Flush 1 (initial): the snapshot sends id 100 after the
	// rebuild, before the tick frame. The delta does NOT send:
	// no scene has been replayed yet, so isTileInScene is
	// false and the next snapshot covers it.
	if err := s.FlushTick(1); err != nil {
		t.Fatalf("FlushTick(1): %v", err)
	}
	packets := flushFramePackets(t, ch)
	indexOf := func(opcode byte, wantID int) int {
		for i, p := range packets {
			if p.Opcode != opcode {
				continue
			}
			if opcode == protocol.ServerLocAddChange {
				id, _, _, _, _, _ := parseLocAdd(t, p.Payload)
				if id != wantID {
					continue
				}
			}
			return i
		}
		return -1
	}
	rebuildIdx := indexOf(protocol.ServerRebuildNormal, 0)
	snapIdx := indexOf(protocol.ServerLocAddChange, 100)
	tickIdx := indexOf(protocol.ServerTick, 0)
	if rebuildIdx < 0 || snapIdx < 0 || tickIdx < 0 {
		t.Fatalf("flush 1 missing frames: rebuild=%d snap=%d tick=%d in %v",
			rebuildIdx, snapIdx, tickIdx, flushOpcodes(packets))
	}
	if !(rebuildIdx < snapIdx && snapIdx < tickIdx) {
		t.Fatalf("flush 1 order: rebuild=%d snap=%d tick=%d, want rebuild < snap < tick",
			rebuildIdx, snapIdx, tickIdx)
	}
	if idx := indexOf(protocol.ServerLocAddChange, 200); idx >= 0 {
		t.Fatalf("flush 1 sent the delta frame at %d, want none", idx)
	}

	// Flush 2 (same position): no scene change, so no
	// snapshot; the delta drains before the tick frame.
	before := len(ch.sentBytes())
	if err := s.FlushTick(2); err != nil {
		t.Fatalf("FlushTick(2): %v", err)
	}
	packets = packetsSince(t, ch, before)
	if idx := indexOf(protocol.ServerLocAddChange, 100); idx >= 0 {
		t.Fatalf("flush 2 re-sent the snapshot at %d, want none", idx)
	}
	deltaIdx := indexOf(protocol.ServerLocAddChange, 200)
	tickIdx = indexOf(protocol.ServerTick, 0)
	if deltaIdx < 0 || tickIdx < 0 || deltaIdx > tickIdx {
		t.Fatalf("flush 2: delta=%d tick=%d, want delta present before tick", deltaIdx, tickIdx)
	}

	// Flush 3 (owner moved far): the scene changes, so the
	// non-initial snapshot sends id 300 AFTER the NPC sync.
	before = len(ch.sentBytes())
	src.moveOwner(3400, 3400, 0)
	if err := s.FlushTick(3); err != nil {
		t.Fatalf("FlushTick(3): %v", err)
	}
	packets = packetsSince(t, ch, before)
	npcIdx := indexOf(protocol.ServerNpcInfo, 0)
	snapIdx = indexOf(protocol.ServerLocAddChange, 300)
	if npcIdx < 0 || snapIdx < 0 || snapIdx < npcIdx {
		t.Fatalf("flush 3: npc=%d snap=%d, want snapshot after NPC sync", npcIdx, snapIdx)
	}
	if idx := indexOf(protocol.ServerLocAddChange, 100); idx >= 0 {
		t.Fatalf("flush 3 re-sent the old-scene object at %d, want none", idx)
	}
}

func TestFlushTickNoObjectSource(t *testing.T) {
	// With no object source installed, FlushTick sends no loc
	// frames at all (the pre-Phase-13 behavior).
	ch := newFakeChannel()
	s := NewSession(ch)
	s.AttachSyncSource(&fakeFlushSource{owner: flushOwner(new(int))})
	if err := s.FlushTick(1); err != nil {
		t.Fatalf("FlushTick: %v", err)
	}
	for _, p := range flushFramePackets(t, ch) {
		if p.Opcode == protocol.ServerLocAddChange || p.Opcode == protocol.ServerLocDel {
			t.Fatalf("unexpected loc frame opcode %d without a source", p.Opcode)
		}
	}
}
