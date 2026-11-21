package plugins

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"tsps-go/internal/game"
)

// fakeInstancingSource is a hand-built RegionDataSource for the
// instancing tests: one region's map bytes, or nothing.
type fakeInstancingSource struct {
	ids  []int
	data map[int]struct {
		terrain []byte
		objects []byte
		ok      bool
	}
}

func (f *fakeInstancingSource) RegionIDs() []int { return f.ids }

func (f *fakeInstancingSource) RegionData(regionID int) ([]byte, []byte, bool) {
	d, ok := f.data[regionID]
	if !ok {
		return nil, nil, false
	}
	return d.terrain, d.objects, d.ok
}

// instancingWiringRecorder captures the reload/stream calls the
// apply step makes, for the replace/restore tests.
type instancingWiringRecorder struct {
	reloads      []int
	streams      []int
	packets      []*game.RegionReplacementPacket
	reloadAnswer bool
	streamCount  int
	delivered    bool
}

func (r *instancingWiringRecorder) wiring(mgr *game.MapRegionReplacementManager) RegionReplacementWiring {
	return RegionReplacementWiring{
		Manager: mgr,
		ReloadRegion: func(regionID int) bool {
			r.reloads = append(r.reloads, regionID)
			return r.reloadAnswer
		},
		StreamToConnected: func(regionID int) int {
			r.streams = append(r.streams, regionID)
			return r.streamCount
		},
		StreamPacketToConnected: func(packet *game.RegionReplacementPacket) bool {
			r.packets = append(r.packets, packet)
			return r.delivered
		},
	}
}

func TestGetAreaManagerNilUntilInstalled(t *testing.T) {
	h, _ := newTestHub()
	api := newApi(h, "mine")
	if api.GetAreaManager() != nil {
		t.Fatal("GetAreaManager non-nil before install")
	}
	if h.AreaManager() != nil {
		t.Fatal("Hub.AreaManager non-nil before install")
	}
	h.SetAreaManager(nil)
	if api.GetAreaManager() != nil {
		t.Fatal("GetAreaManager non-nil after nil install")
	}
	var nilHub *Hub
	if nilHub.AreaManager() != nil {
		t.Fatal("nil Hub AreaManager non-nil")
	}
	nilHub.SetAreaManager(game.NewAreaManager()) // no panic
	var nilApi *Api
	if nilApi.GetAreaManager() != nil {
		t.Fatal("nil Api GetAreaManager non-nil")
	}
	if newApi(nil, "mine").GetAreaManager() != nil {
		t.Fatal("nil-hub Api GetAreaManager non-nil")
	}
}

func TestGetAreaManagerQueryDelegation(t *testing.T) {
	h, _ := newTestHub()
	am := game.NewAreaManager()
	area := game.NewArea("Test", []game.Boundary{game.NewBoundary(0, 10, 0, 10, 0)})
	am.Register(area)
	h.SetAreaManager(am)
	if h.AreaManager() != am {
		t.Fatal("Hub.AreaManager did not return the installed manager")
	}
	q := newApi(h, "mine").GetAreaManager()
	if q == nil {
		t.Fatal("GetAreaManager nil after install")
	}
	if q.Get(game.Location{X: 5, Y: 5, Z: 0}) != area {
		t.Fatal("query Get did not return the registered area")
	}
	if q.Get(game.Location{X: 50, Y: 50, Z: 0}) != nil {
		t.Fatal("query Get returned an area outside the bounds")
	}
	if !q.Inside(game.Location{X: 5, Y: 5, Z: 0}, area) {
		t.Fatal("query Inside false inside the bounds")
	}
	if q.Inside(game.Location{X: 50, Y: 50, Z: 0}, area) {
		t.Fatal("query Inside true outside the bounds")
	}
	p := testPlayer("alice")
	if q.InMulti(p) {
		t.Fatal("query InMulti true with no multi area")
	}
}

func TestHubRegionReplacementWiringStorage(t *testing.T) {
	h, _ := newTestHub()
	if w := h.RegionReplacementWiring(); w.Manager != nil || w.ReloadRegion != nil {
		t.Fatalf("uninstalled wiring = %+v, want zero", w)
	}
	mgr := game.NewMapRegionReplacementManager()
	rec := &instancingWiringRecorder{}
	h.SetRegionReplacementWiring(rec.wiring(mgr))
	if w := h.RegionReplacementWiring(); w.Manager != mgr {
		t.Fatal("installed wiring did not return the manager")
	}
	h.SetRegionReplacementWiring(RegionReplacementWiring{})
	if w := h.RegionReplacementWiring(); w.Manager != nil {
		t.Fatal("zero wiring did not clear the manager")
	}
	var nilHub *Hub
	if w := nilHub.RegionReplacementWiring(); w.Manager != nil {
		t.Fatal("nil Hub wiring non-zero")
	}
	nilHub.SetRegionReplacementWiring(rec.wiring(mgr)) // no panic
}

func TestReplaceMapRegionPack(t *testing.T) {
	// Build a valid .pack through a scratch manager.
	scratch := game.NewMapRegionReplacementManager()
	scratch.SetArchiveIDs(func(regionID int) (int, int, bool) { return 11, 22, true })
	if _, err := scratch.ReplaceMapRegionData(257, []byte{1, 2}, []byte{3}, "editor"); err != nil {
		t.Fatal(err)
	}
	pack, ok := scratch.GetRegionPack(257, nil)
	if !ok {
		t.Fatal("scratch pack build failed")
	}
	path := filepath.Join(t.TempDir(), "257.pack")
	if err := os.WriteFile(path, pack, 0o600); err != nil {
		t.Fatal(err)
	}

	h, logBuf := newTestHub()
	mgr := game.NewMapRegionReplacementManager()
	mgr.SetArchiveIDs(func(regionID int) (int, int, bool) { return 11, 22, true })
	rec := &instancingWiringRecorder{reloadAnswer: true, streamCount: 2}
	h.SetRegionReplacementWiring(rec.wiring(mgr))
	newApi(h, "mine").ReplaceMapRegion(257, path)
	if _, _, ok := mgr.GetReplacementMapData(257); !ok {
		t.Fatal("pack replace did not install")
	}
	if len(rec.reloads) != 1 || rec.reloads[0] != 257 {
		t.Fatalf("reloads = %v, want [257]", rec.reloads)
	}
	if len(rec.streams) != 1 || rec.streams[0] != 257 {
		t.Fatalf("streams = %v, want [257]", rec.streams)
	}
	if !strings.Contains(logBuf.String(), "replaced map region") || !strings.Contains(logBuf.String(), "clippingReloaded=true") || !strings.Contains(logBuf.String(), "streamedPlayers=2") {
		t.Fatalf("apply log = %q", logBuf.String())
	}

	// An invalid region id warns and installs nothing.
	logBuf.Reset()
	newApi(h, "mine").ReplaceMapRegion(-1, path)
	if !strings.Contains(logBuf.String(), "invalid replaceMapRegion") {
		t.Fatalf("invalid-id log = %q", logBuf.String())
	}
	// A missing file logs an error and installs nothing.
	logBuf.Reset()
	newApi(h, "mine").ReplaceMapRegion(258, filepath.Join(t.TempDir(), "missing.pack"))
	if !strings.Contains(logBuf.String(), "failed to replace map region 258") {
		t.Fatalf("missing-file log = %q", logBuf.String())
	}
	if _, _, ok := mgr.GetReplacementMapData(258); ok {
		t.Fatal("missing file installed a replacement")
	}
	// No manager installed: warn, no-op.
	h2, logBuf2 := newTestHub()
	newApi(h2, "mine").ReplaceMapRegion(257, path)
	if !strings.Contains(logBuf2.String(), "no region replacement manager installed") {
		t.Fatalf("no-manager log = %q", logBuf2.String())
	}
}

func TestReplaceMapRegionFromFiles(t *testing.T) {
	dir := t.TempDir()
	terrainPath := filepath.Join(dir, "terrain.dat")
	objectPath := filepath.Join(dir, "objects.dat")
	if err := os.WriteFile(terrainPath, []byte{1, 2}, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(objectPath, []byte{3}, 0o600); err != nil {
		t.Fatal(err)
	}
	h, _ := newTestHub()
	mgr := game.NewMapRegionReplacementManager()
	rec := &instancingWiringRecorder{reloadAnswer: false}
	h.SetRegionReplacementWiring(rec.wiring(mgr))
	newApi(h, "mine").ReplaceMapRegionFromFiles(257, terrainPath, objectPath)
	if _, _, ok := mgr.GetReplacementMapData(257); !ok {
		t.Fatal("file-pair replace did not install")
	}
	// A failed reload still streams (TS logs and continues).
	if len(rec.reloads) != 1 || len(rec.streams) != 1 {
		t.Fatalf("reloads=%v streams=%v, want one each", rec.reloads, rec.streams)
	}
}

func TestReplaceMapRegionDataOwnership(t *testing.T) {
	h, logBuf := newTestHub()
	mgr := game.NewMapRegionReplacementManager()
	rec := &instancingWiringRecorder{reloadAnswer: true, streamCount: 1}
	h.SetRegionReplacementWiring(rec.wiring(mgr))
	api := newApi(h, "mine")

	// Invalid input is refused.
	if api.ReplaceMapRegionData(-1, []byte{1}, nil) {
		t.Fatal("invalid region id accepted")
	}
	if api.ReplaceMapRegionData(257, nil, nil) {
		t.Fatal("empty terrain accepted")
	}
	// First install succeeds and applies.
	if !api.ReplaceMapRegionData(257, []byte{1}, []byte{2}) {
		t.Fatal("first install refused")
	}
	if len(rec.reloads) != 1 || len(rec.streams) != 1 {
		t.Fatalf("apply calls: reloads=%v streams=%v", rec.reloads, rec.streams)
	}
	// Another plugin's region is refused, with a warning, and
	// the existing replacement stands.
	other := newApi(h, "other")
	logBuf.Reset()
	if other.ReplaceMapRegionData(257, []byte{9}, nil) {
		t.Fatal("other owner's region overwritten")
	}
	if !strings.Contains(logBuf.String(), "refused: region 257 is replaced by mine") {
		t.Fatalf("refusal log = %q", logBuf.String())
	}
	got, _, _ := mgr.GetReplacementMapData(257)
	if !bytes.Equal(got, []byte{1}) {
		t.Fatalf("refused install overwrote the data: %v", got)
	}
	// The owner may re-install its own region.
	if !api.ReplaceMapRegionData(257, []byte{9}, nil) {
		t.Fatal("owner re-install refused")
	}
	// No manager: refused.
	h2, _ := newTestHub()
	if newApi(h2, "mine").ReplaceMapRegionData(257, []byte{1}, nil) {
		t.Fatal("no-manager install accepted")
	}
	// Nil Api / nil hub: refused, no panic.
	var nilApi *Api
	if nilApi.ReplaceMapRegionData(257, []byte{1}, nil) {
		t.Fatal("nil Api install accepted")
	}
	if newApi(nil, "mine").ReplaceMapRegionData(257, []byte{1}, nil) {
		t.Fatal("nil-hub install accepted")
	}
}

func TestRestoreMapRegion(t *testing.T) {
	native := &fakeInstancingSource{ids: []int{257}, data: map[int]struct {
		terrain []byte
		objects []byte
		ok      bool
	}{257: {terrain: []byte{5, 6}, objects: []byte{7}, ok: true}}}
	h, logBuf := newTestHub()
	mgr := game.NewMapRegionReplacementManager()
	mgr.SetNativeSource(native)
	rec := &instancingWiringRecorder{reloadAnswer: true, delivered: true}
	h.SetRegionReplacementWiring(rec.wiring(mgr))
	api := newApi(h, "mine")

	if !api.ReplaceMapRegionData(257, []byte{1}, []byte{2}) {
		t.Fatal("install refused")
	}
	rec.reloads, rec.packets = nil, nil
	if !api.RestoreMapRegion(257) {
		t.Fatal("restore refused")
	}
	if _, _, ok := mgr.GetReplacementMapData(257); ok {
		t.Fatal("restore did not remove the replacement")
	}
	if len(rec.reloads) != 1 || rec.reloads[0] != 257 {
		t.Fatalf("restore reloads = %v, want [257]", rec.reloads)
	}
	if len(rec.packets) != 1 || rec.packets[0] == nil {
		t.Fatalf("restore packets = %v, want one native packet", rec.packets)
	}
	// The streamed packet carries the NATIVE map bytes.
	payload := rec.packets[0].Payload
	if len(payload) < 7 || binary.BigEndian.Uint16(payload[0:2]) != 257 || !bytes.Equal(payload[7:], []byte{5, 6, 7}) {
		t.Fatalf("native packet payload = %v", payload)
	}
	// Restoring again: nothing to restore.
	if api.RestoreMapRegion(257) {
		t.Fatal("second restore accepted")
	}

	// Another owner's region: refused silently, kept.
	if !api.ReplaceMapRegionData(257, []byte{1}, nil) {
		t.Fatal("re-install refused")
	}
	if newApi(h, "other").RestoreMapRegion(257) {
		t.Fatal("other owner's restore accepted")
	}
	if _, _, ok := mgr.GetReplacementMapData(257); !ok {
		t.Fatal("refused restore removed the replacement")
	}

	// A failed stream reports undelivered, but the removal
	// stands (TS removes before streaming).
	rec.delivered = false
	if api.RestoreMapRegion(257) {
		t.Fatal("restore reported delivered with a failing stream")
	}
	if _, _, ok := mgr.GetReplacementMapData(257); ok {
		t.Fatal("failed-stream restore kept the replacement")
	}

	// No native map: the replacement is kept, with a warning.
	h2, logBuf2 := newTestHub()
	mgr2 := game.NewMapRegionReplacementManager() // no native source
	rec2 := &instancingWiringRecorder{delivered: true}
	h2.SetRegionReplacementWiring(rec2.wiring(mgr2))
	api2 := newApi(h2, "mine")
	if !api2.ReplaceMapRegionData(257, []byte{1}, nil) {
		t.Fatal("install refused")
	}
	if api2.RestoreMapRegion(257) {
		t.Fatal("restore accepted with no native map")
	}
	if !strings.Contains(logBuf2.String(), "kept region 257") {
		t.Fatalf("kept log = %q", logBuf2.String())
	}
	if _, _, ok := mgr2.GetReplacementMapData(257); !ok {
		t.Fatal("no-native-map restore removed the replacement")
	}
	_ = logBuf
}

func TestCoreInstancingAccessors(t *testing.T) {
	h, _ := newTestHub()
	core := newApi(h, "mine").Core()
	if core == nil {
		t.Fatal("Api.Core nil")
	}
	if core.MapRegionReplacementManager() != nil {
		t.Fatal("Core manager non-nil before install")
	}
	mgr := game.NewMapRegionReplacementManager()
	h.SetRegionReplacementWiring(RegionReplacementWiring{Manager: mgr})
	if core.MapRegionReplacementManager() != mgr {
		t.Fatal("Core manager did not return the installed manager")
	}
	var nilCore *Core
	if nilCore.MapRegionReplacementManager() != nil {
		t.Fatal("nil Core manager non-nil")
	}

	// Template region accessors delegate to the game functions.
	src := &fakeInstancingSource{ids: []int{257}, data: map[int]struct {
		terrain []byte
		objects []byte
		ok      bool
	}{257: {terrain: []byte{0, 50, 0, 0}, objects: nil, ok: true}}}
	game.SetTemplateRegionSource(src, true)
	defer game.SetTemplateRegionSource(nil, false)
	cached := core.GetTemplateRegion(257)
	if cached == nil || cached.Settings[0] != 1 {
		t.Fatalf("GetTemplateRegion = %v, want decoded settings", cached)
	}
	if again := core.GetTemplateRegion(257); again != cached {
		t.Fatal("GetTemplateRegion did not return the cached decode")
	}
	fresh := core.ReadTemplateRegion(257)
	if fresh == nil || fresh == cached {
		t.Fatal("ReadTemplateRegion returned the cached pointer")
	}
	if core.GetTemplateRegion(999) != nil || core.ReadTemplateRegion(999) != nil {
		t.Fatal("absent region answered non-nil")
	}
}

func TestApplyWithNilWiringFuncs(t *testing.T) {
	h, logBuf := newTestHub()
	mgr := game.NewMapRegionReplacementManager()
	// Manager installed, reload/stream funcs nil: the apply
	// step degrades to logging only, without panicking.
	h.SetRegionReplacementWiring(RegionReplacementWiring{Manager: mgr})
	if !newApi(h, "mine").ReplaceMapRegionData(257, []byte{1}, nil) {
		t.Fatal("install refused with nil wiring funcs")
	}
	if !strings.Contains(logBuf.String(), "clippingReloaded=false") || !strings.Contains(logBuf.String(), "streamedPlayers=0") {
		t.Fatalf("apply log = %q", logBuf.String())
	}
}
