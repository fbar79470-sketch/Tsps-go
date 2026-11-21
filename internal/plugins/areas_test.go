package plugins

import (
	"strings"
	"testing"

	"tsps-go/internal/game"
)

// fakeAreaRegistry captures registered areas in order.
type fakeAreaRegistry struct {
	areas []*game.Area
}

func (f *fakeAreaRegistry) Register(area *game.Area) { f.areas = append(f.areas, area) }

// areaTestActor is a hand-built AreaActor fixture: the coordinator has not
// yet added the Area accessors to *Player/*NPC, so tests wrap a real player
// with fixture-held area state (the same shape as the game package's area
// fixtures). It deliberately does not implement MultiIconActor.
type areaTestActor struct {
	player *game.Player
	area   *game.Area
}

func newAreaTestActor(index int, loc game.Location) *areaTestActor {
	p := game.NewPlayer("fixture", loc)
	p.SetIndex(index)
	return &areaTestActor{player: p}
}

func (f *areaTestActor) Location() game.Location { return f.player.Location() }
func (f *areaTestActor) Area() *game.Area        { return f.area }
func (f *areaTestActor) SetArea(a *game.Area)    { f.area = a }
func (f *areaTestActor) GetIndex() int           { return f.player.Index }
func (f *areaTestActor) IsPlayer() bool          { return true }
func (f *areaTestActor) IsNPC() bool             { return false }
func (f *areaTestActor) AsPlayer() *game.Player  { return f.player }
func (f *areaTestActor) AsNPC() *game.NPC        { return nil }

func TestRegisterAreaSetsPluginNameAndPushes(t *testing.T) {
	h, _ := newTestHub()
	reg := &fakeAreaRegistry{}
	h.SetAreaRegistry(reg)
	area := game.NewArea("MyArea", nil)
	newApi(h, "my-plugin").RegisterArea(area)
	if area.PluginName() != "my-plugin" {
		t.Fatalf("PluginName = %q, want my-plugin", area.PluginName())
	}
	if len(reg.areas) != 1 || reg.areas[0] != area {
		t.Fatalf("registry areas = %v, want [%v]", reg.areas, area)
	}
	if h.AreaRegistry() != AreaRegistry(reg) {
		t.Fatal("AreaRegistry() did not return the installed registry")
	}
}

func TestRegisterAreaQueuesUntilInstalled(t *testing.T) {
	h, _ := newTestHub()
	first := game.NewArea("First", nil)
	second := game.NewArea("Second", nil)
	newApi(h, "plug-a").RegisterArea(first)
	newApi(h, "plug-b").RegisterArea(second)
	// Plugin names are set at registration time, before installation.
	if first.PluginName() != "plug-a" || second.PluginName() != "plug-b" {
		t.Fatalf("plugin names = %q/%q, want plug-a/plug-b before install", first.PluginName(), second.PluginName())
	}
	if h.AreaRegistry() != nil {
		t.Fatal("AreaRegistry() non-nil before installation")
	}
	reg := &fakeAreaRegistry{}
	h.SetAreaRegistry(reg)
	if len(reg.areas) != 2 || reg.areas[0] != first || reg.areas[1] != second {
		t.Fatalf("flushed areas = %v, want [First Second] in registration order", reg.areas)
	}
	// Later registrations push directly, after the flushed ones.
	third := game.NewArea("Third", nil)
	newApi(h, "plug-c").RegisterArea(third)
	if len(reg.areas) != 3 || reg.areas[2] != third {
		t.Fatalf("areas = %v, want Third appended directly", reg.areas)
	}
}

func TestRegisterAreaAfterLoadStillRegisters(t *testing.T) {
	// RegisterArea has no load-phase gate: the TS closure has no phase
	// check, and load-time registration (before the world exists) is the
	// normal case. Sealing the hub must not refuse it.
	h, _ := newTestHub()
	h.Load()
	area := game.NewArea("Late", nil)
	newApi(h, "late-plugin").RegisterArea(area)
	reg := &fakeAreaRegistry{}
	h.SetAreaRegistry(reg)
	if len(reg.areas) != 1 || reg.areas[0] != area {
		t.Fatalf("areas = %v, want the post-Load registration flushed", reg.areas)
	}
}

func TestSetAreaRegistryNilKeepsQueue(t *testing.T) {
	h, _ := newTestHub()
	area := game.NewArea("Queued", nil)
	newApi(h, "p").RegisterArea(area)
	h.SetAreaRegistry(nil)
	if h.AreaRegistry() != nil {
		t.Fatal("AreaRegistry() non-nil after clearing with nil")
	}
	reg := &fakeAreaRegistry{}
	h.SetAreaRegistry(reg)
	if len(reg.areas) != 1 || reg.areas[0] != area {
		t.Fatalf("areas = %v, want the queued area flushed after a nil-clear cycle", reg.areas)
	}
}

func TestRegisterAreaNilAreaPanics(t *testing.T) {
	h, _ := newTestHub()
	assertPanicsWith(t, func() { newApi(h, "p").RegisterArea(nil) }, "nil area")
}

func TestRegisterAreaNilSafety(t *testing.T) {
	var nilApi *Api
	nilApi.RegisterArea(game.NewArea("X", nil)) // no-op, no panic
	nilApi.RegisterArea(nil)                    // no-op even for a nil area
	var nilHub *Hub
	nilHub.SetAreaRegistry(&fakeAreaRegistry{}) // no-op
	if nilHub.AreaRegistry() != nil {
		t.Fatal("nil hub AreaRegistry() non-nil")
	}
}

func TestAreaCallerPassThrough(t *testing.T) {
	h, _ := newTestHub()
	caller := h.AreaMethodCaller()
	area := game.NewArea("PassThrough", nil)
	var calls []string
	area.PostEnterHook = func(game.AreaActor) { calls = append(calls, "enter") }
	area.PostLeaveHook = func(game.AreaActor, bool) { calls = append(calls, "leave") }
	area.ProcessHook = func(game.AreaActor) { calls = append(calls, "process") }
	area.MusicTrackHook = func(*game.Player) (int, bool) { return 42, true }

	actor := newAreaTestActor(1, game.Location{X: 100, Y: 100, Z: 0})
	caller.Enter(area, actor)
	if !area.HasMember(actor) {
		t.Fatal("actor is not a member after caller.Enter")
	}
	caller.Process(area, actor)
	track, ok := caller.MusicTrack(area, actor.player)
	if track != 42 || !ok {
		t.Fatalf("MusicTrack = (%d, %v), want (42, true)", track, ok)
	}
	caller.Leave(area, actor, false)
	if area.HasMember(actor) {
		t.Fatal("actor is still a member after caller.Leave")
	}
	want := []string{"enter", "process", "leave"}
	if len(calls) != len(want) {
		t.Fatalf("calls = %v, want %v", calls, want)
	}
	for i := range want {
		if calls[i] != want[i] {
			t.Fatalf("calls = %v, want %v", calls, want)
		}
	}
}

func TestAreaCallerIsolatesPanics(t *testing.T) {
	h, buf := newTestHub()
	caller := h.AreaMethodCaller()
	area := game.NewArea("BoomArea", nil)
	area.SetPluginName("boom-plugin")
	area.PostEnterHook = func(game.AreaActor) { panic("enter boom") }
	area.PostLeaveHook = func(game.AreaActor, bool) { panic("leave boom") }
	area.ProcessHook = func(game.AreaActor) { panic("process boom") }
	area.MusicTrackHook = func(*game.Player) (int, bool) { panic("music boom") }

	actor := newAreaTestActor(1, game.Location{X: 100, Y: 100, Z: 0})
	// None of these may propagate a panic.
	caller.Enter(area, actor)
	caller.Process(area, actor)
	track, ok := caller.MusicTrack(area, actor.player)
	caller.Leave(area, actor, true)
	if track != 0 || ok {
		t.Fatalf("MusicTrack after panic = (%d, %v), want (0, false): failure returns no opinion", track, ok)
	}
	logged := buf.String()
	for _, want := range []string{
		"[plugins] area enter failed (boom-plugin)",
		"[plugins] area leave failed (boom-plugin)",
		"[plugins] area process failed (boom-plugin)",
		"[plugins] area getMusicTrack failed (boom-plugin)",
		"enter boom", "leave boom", "process boom", "music boom",
	} {
		if !strings.Contains(logged, want) {
			t.Fatalf("log = %q, want it to contain %q", logged, want)
		}
	}
}

func TestAreaCallerLabelFallsBackToAreaName(t *testing.T) {
	h, buf := newTestHub()
	caller := h.AreaMethodCaller()
	area := game.NewArea("Unnamed", nil) // no plugin name set
	area.ProcessHook = func(game.AreaActor) { panic("x") }
	caller.Process(area, newAreaTestActor(1, game.Location{}))
	if !strings.Contains(buf.String(), "[plugins] area process failed (Unnamed)") {
		t.Fatalf("log = %q, want the area-name fallback label", buf.String())
	}
}

func TestAreaCallerDefaultMusicTrack(t *testing.T) {
	h, _ := newTestHub()
	caller := h.AreaMethodCaller()
	area := game.NewArea("Silent", nil) // no MusicTrackHook: TS null default
	track, ok := caller.MusicTrack(area, game.NewPlayer("p", game.Location{}))
	if track != 0 || ok {
		t.Fatalf("MusicTrack = (%d, %v), want (0, false) for an area with no opinion", track, ok)
	}
}

func TestAreaCallerNilHub(t *testing.T) {
	var nilHub *Hub
	caller := nilHub.AreaMethodCaller()
	if caller == nil {
		t.Fatal("nil hub AreaMethodCaller() returned nil")
	}
	// The caller logs through log.Default() and still isolates.
	area := game.NewArea("X", nil)
	area.MusicTrackHook = func(*game.Player) (int, bool) { panic("x") }
	if track, ok := caller.MusicTrack(area, game.NewPlayer("p", game.Location{})); track != 0 || ok {
		t.Fatalf("MusicTrack = (%d, %v), want (0, false)", track, ok)
	}
}
