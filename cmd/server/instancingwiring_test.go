package main

import (
	"bytes"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"tsps-go/internal/game"
	"tsps-go/internal/plugins"
	"tsps-go/internal/session"
)

// fakeWiringRegionSource is a hand-built RegionDataSource for the
// instancing wiring tests.
type fakeWiringRegionSource struct {
	ids  []int
	data map[int]struct {
		terrain []byte
		objects []byte
		ok      bool
	}
}

func (f *fakeWiringRegionSource) RegionIDs() []int { return f.ids }

func (f *fakeWiringRegionSource) RegionData(regionID int) ([]byte, []byte, bool) {
	d, ok := f.data[regionID]
	if !ok {
		return nil, nil, false
	}
	return d.terrain, d.objects, d.ok
}

// fakeWiringObjectDefs is a hand-built ObjectDefSource.
type fakeWiringObjectDefs struct {
	defs map[int]game.ObjectDef
}

func (f *fakeWiringObjectDefs) ObjectDef(id int) (game.ObjectDef, bool) {
	d, ok := f.defs[id]
	return d, ok
}

// fakeWiringPort is a hand-built game.SessionPort recording sent
// frames, for the connected-player stream tests.
type fakeWiringPort struct {
	open bool
	sent [][]byte
}

func (f *fakeWiringPort) SendPacket(packet []byte) {
	f.sent = append(f.sent, append([]byte(nil), packet...))
}
func (f *fakeWiringPort) SendRunEnergy(units int, running bool, weightKg int, staminaTicks int) {}
func (f *fakeWiringPort) SendDestination(x, y int)                                              {}
func (f *fakeWiringPort) SendVarp(id, value int)                                                {}
func (f *fakeWiringPort) FlushTick(cycle int64) error                                           { return nil }
func (f *fakeWiringPort) Close(code int, reason string)                                         { f.open = false }
func (f *fakeWiringPort) IsOpen() bool                                                          { return f.open }

// fakeWiringChannel is a hand-built session.Channel recording
// sends, for the per-player sender test.
type fakeWiringChannel struct {
	open bool
	sent [][]byte
}

func (f *fakeWiringChannel) Kind() string       { return "websocket" }
func (f *fakeWiringChannel) RemoteAddr() string { return "1.2.3.4" }
func (f *fakeWiringChannel) Send(payload []byte) error {
	f.sent = append(f.sent, append([]byte(nil), payload...))
	return nil
}
func (f *fakeWiringChannel) Close(code int, reason string) { f.open = false }
func (f *fakeWiringChannel) IsOpen() bool                  { return f.open }

// stubWiringFlushSource is a hand-built session.FlushSource
// returning a fixed owner snapshot.
type stubWiringFlushSource struct {
	owner session.SyncPlayerSnapshot
}

func (s *stubWiringFlushSource) SyncView() (session.SyncPlayerSnapshot, []session.SyncPlayerSnapshot, []session.SyncNpcSnapshot) {
	return s.owner, nil, nil
}

// restoreInstancingGlobals clears the process-global seams
// InstallInstancing installs (the destroy hook and the template
// seams), so one wiring test cannot affect another.
func restoreInstancingGlobals(t *testing.T) {
	t.Helper()
	t.Cleanup(func() {
		game.SetPrivateAreaDestroyHook(nil)
		game.SetTemplateRegionSource(nil, false)
		game.SetTemplateObjectDefSource(nil)
	})
}

func TestInstallInstancing(t *testing.T) {
	restoreInstancingGlobals(t)
	hub := plugins.NewHub(nil, plugins.HubOptions{Logger: log.New(&bytes.Buffer{}, "", 0)})
	world := game.NewWorld(game.Location{X: 3000, Y: 3000})
	replacements := game.NewMapRegionReplacementManager()
	src := &fakeWiringRegionSource{ids: []int{257}, data: map[int]struct {
		terrain []byte
		objects []byte
		ok      bool
	}{257: {terrain: []byte{0, 50, 0, 0}, objects: nil, ok: true}}}
	defs := &fakeWiringObjectDefs{defs: map[int]game.ObjectDef{
		100: {SizeX: 1, SizeY: 1, ClipType: 1, BlocksProjectile: true},
	}}

	// A replacement dir with one loose terrain file.
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "258.terrain.dat"), []byte{7, 7}, 0o600); err != nil {
		t.Fatal(err)
	}
	var warnings []string
	InstallInstancing(hub, world, replacements, src, defs, true,
		func(regionID int) (int, int, bool) { return 11, 22, true },
		dir, func(msg string) { warnings = append(warnings, msg) })

	// The hub wiring is installed.
	if hub.AreaManager() != world.Areas() {
		t.Fatal("hub AreaManager is not the world's")
	}
	wiring := hub.RegionReplacementWiring()
	if wiring.Manager != replacements {
		t.Fatal("hub wiring manager mismatch")
	}
	if wiring.ReloadRegion == nil || wiring.StreamToConnected == nil || wiring.StreamPacketToConnected == nil {
		t.Fatal("hub wiring funcs missing")
	}
	// The template source seam reads through the installed
	// source (decode-once cache included).
	region := game.GetTemplateRegion(257)
	if region == nil || region.Settings[0] != 1 {
		t.Fatalf("template region = %v, want decoded settings", region)
	}
	// The replacement dir loaded.
	if _, _, ok := replacements.GetReplacementMapData(258); !ok {
		t.Fatalf("dir replacement not loaded (warnings: %v)", warnings)
	}
	// The native source seam answers through the same source.
	packet, err := replacements.NativeRegionPacket(257)
	if err != nil || packet == nil {
		t.Fatalf("native packet = (%v, %v)", packet, err)
	}
}

func TestInstallInstancingNilSafety(t *testing.T) {
	restoreInstancingGlobals(t)
	replacements := game.NewMapRegionReplacementManager()
	// Every nil combination installs without panicking; the
	// installs that can proceed still do.
	InstallInstancing(nil, nil, nil, nil, nil, true, nil, "", nil)
	hub := plugins.NewHub(nil, plugins.HubOptions{Logger: log.New(&bytes.Buffer{}, "", 0)})
	InstallInstancing(hub, nil, replacements, nil, nil, true, nil, "", nil)
	if hub.AreaManager() != nil {
		t.Fatal("nil world installed an AreaManager")
	}
	if w := hub.RegionReplacementWiring(); w.Manager != replacements {
		t.Fatal("nil world skipped the manager install")
	} else if w.ReloadRegion(257) {
		t.Fatal("nil-world reload reported success")
	} else if w.StreamToConnected(257) != 0 {
		t.Fatal("nil-world stream reported sends")
	} else if !w.StreamPacketToConnected(nil) {
		t.Fatal("nil-world packet stream reported failure")
	}
	world := game.NewWorld(game.Location{X: 3000, Y: 3000})
	InstallInstancing(hub, world, nil, nil, nil, true, nil, "", nil)
	if w := hub.RegionReplacementWiring(); w.Manager != nil {
		t.Fatal("nil manager left a stale wiring manager")
	}
	InstallInstancing(nil, world, replacements, nil, nil, true, nil, "", nil)
}

func TestInstallInstancingDestroyHook(t *testing.T) {
	restoreInstancingGlobals(t)
	hub := plugins.NewHub(nil, plugins.HubOptions{Logger: log.New(&bytes.Buffer{}, "", 0)})
	world := game.NewWorld(game.Location{X: 3000, Y: 3000})
	InstallInstancing(hub, world, game.NewMapRegionReplacementManager(), nil, nil, true, nil, "", nil)

	// A registered NPC and a registered object belonging to a
	// private area.
	npc := game.NewNPC(1, game.Location{X: 3000, Y: 3000, Z: 0})
	npcIndex, ok := world.NPCs().Add(npc)
	if !ok {
		t.Fatal("NPC add failed")
	}
	pa := game.NewPrivateArea([]game.Boundary{game.NewBoundary(3000, 3010, 3000, 3010, 0)})
	pa.Add(npc)
	obj := game.NewGameObject(100, game.Location{X: 3001, Y: 3001, Z: 0}, 10, 0, pa)
	world.GameObjects().Register(obj, false)
	// At is the runtime-object query (any private area); Get
	// is the shared-world view and does not see area objects.
	if got := world.GameObjects().At(obj.Location()); len(got) != 1 || got[0] != obj {
		t.Fatalf("object not registered before Destroy: %v", got)
	}

	// Destroy runs the hook: the object deregisters at once,
	// and the NPC leaves on the next world tick.
	pa.Destroy()
	if got := world.GameObjects().At(obj.Location()); len(got) != 0 {
		t.Fatalf("destroy hook did not deregister the area object: %v", got)
	}
	world.Process()
	if _, ok := world.NPCs().Get(npcIndex); ok {
		t.Fatal("destroy hook did not queue the NPC for removal")
	}
}

func TestInstallInstancingReloadAndStreams(t *testing.T) {
	restoreInstancingGlobals(t)
	hub := plugins.NewHub(nil, plugins.HubOptions{Logger: log.New(&bytes.Buffer{}, "", 0)})
	world := game.NewWorld(game.Location{X: 3000, Y: 3000})
	// A cache-backed RegionManager over one indexed region, so
	// reload has something to reload.
	terrain := make([]byte, 4*64*64*2) // all terminators
	src := &fakeWiringRegionSource{ids: []int{257}, data: map[int]struct {
		terrain []byte
		objects []byte
		ok      bool
	}{257: {terrain: terrain, objects: []byte{0}, ok: true}}}
	world.SetRegionManager(game.NewRegionManager(src, nil, true))
	replacements := game.NewMapRegionReplacementManager()
	InstallInstancing(hub, world, replacements, src, nil, true, nil, "", nil)
	wiring := hub.RegionReplacementWiring()

	if !wiring.ReloadRegion(257) {
		t.Fatal("reload of an indexed region reported failure")
	}
	if wiring.ReloadRegion(999) {
		t.Fatal("reload of an unindexed region reported success")
	}

	// Two players: one connected, one not. Only the connected
	// player receives the stream.
	if _, err := replacements.ReplaceMapRegionData(257, []byte{1, 2, 3}, nil, "test"); err != nil {
		t.Fatal(err)
	}
	open := &fakeWiringPort{open: true}
	p1 := game.NewPlayer("alice", game.Location{X: 3200, Y: 3200})
	p1.Session = open
	if _, ok := world.Players().Add(p1); !ok {
		t.Fatal("player add failed")
	}
	closed := &fakeWiringPort{open: false}
	p2 := game.NewPlayer("bob", game.Location{X: 3200, Y: 3200})
	p2.Session = closed
	if _, ok := world.Players().Add(p2); !ok {
		t.Fatal("player add failed")
	}
	if streamed := wiring.StreamToConnected(257); streamed != 1 {
		t.Fatalf("StreamToConnected = %d, want 1", streamed)
	}
	if len(open.sent) != 1 || len(closed.sent) != 0 {
		t.Fatalf("sent counts: open=%d closed=%d", len(open.sent), len(closed.sent))
	}
	if len(open.sent[0]) == 0 || open.sent[0][0] != 144 {
		t.Fatalf("streamed frame opcode = %v, want 144 first", open.sent[0][:1])
	}
	if !bytes.Contains(open.sent[0], []byte{0x01, 0x01, 0x01, 0, 3, 0, 0, 1, 2, 3}) {
		t.Fatalf("streamed frame lacks the replacement payload: %v", open.sent[0])
	}

	// The packet stream delivers the native packet to the
	// connected player and reports success; with no connected
	// players it still reports success.
	packet, err := game.BuildRegionReplacementPacket(257, true, []byte{9}, nil)
	if err != nil {
		t.Fatal(err)
	}
	open.sent = nil
	if !wiring.StreamPacketToConnected(&packet) {
		t.Fatal("packet stream reported failure with a connected player")
	}
	if len(open.sent) != 1 {
		t.Fatalf("packet stream sent %d frames, want 1", len(open.sent))
	}
	open.open = false
	if !wiring.StreamPacketToConnected(&packet) {
		t.Fatal("packet stream reported failure with no connected players")
	}
}

func TestInstallPlayerRegionReplacements(t *testing.T) {
	// Nil-safety first.
	InstallPlayerRegionReplacements(nil, nil)
	InstallPlayerRegionReplacements(nil, game.NewMapRegionReplacementManager())
	ch := &fakeWiringChannel{open: true}
	sess := session.NewSession(ch)
	InstallPlayerRegionReplacements(sess, nil)

	// A replacement for the region the owner stands in streams
	// on the first flush (window change), and only once.
	mgr := game.NewMapRegionReplacementManager()
	if _, err := mgr.ReplaceMapRegionData(12850, []byte{1, 2, 3}, nil, "test"); err != nil {
		t.Fatal(err)
	}
	InstallPlayerRegionReplacements(sess, mgr)
	sess.AttachSyncSource(&stubWiringFlushSource{owner: session.SyncPlayerSnapshot{
		Identity: 1, Index: 1, X: 3200, Y: 3200, Z: 0, DisplayName: "Test",
	}})
	if err := sess.FlushTick(1); err != nil {
		t.Fatal(err)
	}
	var sent []byte
	for _, frame := range ch.sent {
		sent = append(sent, frame...)
	}
	payload := []byte{0x32, 0x32, 0x01, 0, 3, 0, 0, 1, 2, 3}
	if !bytes.Contains(sent, payload) {
		t.Fatalf("first flush lacks the replacement payload in %d sent bytes", len(sent))
	}
	if err := sess.FlushTick(2); err != nil {
		t.Fatal(err)
	}
	sent = nil
	for _, frame := range ch.sent {
		sent = append(sent, frame...)
	}
	if n := bytes.Count(sent, payload); n != 1 {
		t.Fatalf("replacement payload appears %d times, want 1 (window unchanged)", n)
	}

	// Without the install, no replacement streams.
	ch2 := &fakeWiringChannel{open: true}
	sess2 := session.NewSession(ch2)
	sess2.AttachSyncSource(&stubWiringFlushSource{owner: session.SyncPlayerSnapshot{
		Identity: 1, Index: 1, X: 3200, Y: 3200, Z: 0, DisplayName: "Test",
	}})
	if err := sess2.FlushTick(1); err != nil {
		t.Fatal(err)
	}
	var sent2 []byte
	for _, frame := range ch2.sent {
		sent2 = append(sent2, frame...)
	}
	if bytes.Contains(sent2, payload) {
		t.Fatal("uninstalled session streamed a replacement")
	}
}

func TestInstallInstancingPackDirWarning(t *testing.T) {
	restoreInstancingGlobals(t)
	hub := plugins.NewHub(nil, plugins.HubOptions{Logger: log.New(&bytes.Buffer{}, "", 0)})
	world := game.NewWorld(game.Location{X: 3000, Y: 3000})
	replacements := game.NewMapRegionReplacementManager()
	// A directory with an unloadable pack warns and continues;
	// the warning reaches the caller's warn func.
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "257.pack"), []byte{1, 2, 3}, 0o600); err != nil {
		t.Fatal(err)
	}
	var warnings []string
	InstallInstancing(hub, world, replacements, nil, nil, true,
		func(regionID int) (int, int, bool) { return 11, 22, true },
		dir, func(msg string) { warnings = append(warnings, msg) })
	if len(warnings) != 1 || !strings.Contains(warnings[0], "skipping region replacement") {
		t.Fatalf("warnings = %v", warnings)
	}
	if _, _, ok := replacements.GetReplacementMapData(257); ok {
		t.Fatal("unloadable pack installed a replacement")
	}
}
