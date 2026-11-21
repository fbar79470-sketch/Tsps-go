package wildernessditch

import (
	"testing"

	"tsps-go/internal/game"
	"tsps-go/internal/plugins"
)

// fakeSession is a minimal game.SessionPort that records sound-effect
// sends, standing in for the session layer in hub tests.
type fakeSession struct {
	effects []soundEffectCall
}

type soundEffectCall struct {
	soundID, loopType, delay, volume int
}

func (f *fakeSession) SendPacket(packet []byte) {}
func (f *fakeSession) SendRunEnergy(units int, running bool, weightKg int, staminaTicks int) {
}
func (f *fakeSession) SendDestination(x, y int) {}
func (f *fakeSession) SendVarp(id, value int)   {}
func (f *fakeSession) FlushTick(cycle int64) error {
	return nil
}
func (f *fakeSession) Close(code int, reason string) {}
func (f *fakeSession) IsOpen() bool                  { return true }
func (f *fakeSession) SendSoundEffect(soundID, loopType, delay, volume int) {
	f.effects = append(f.effects, soundEffectCall{soundID, loopType, delay, volume})
}
func (f *fakeSession) SendAreaSound(soundID, x, y, level, loops, delay, radius, attenuation int) {
}

// fakeObjectDef is the minimal plugins.ObjectDefinition for the
// ditch: name "Wilderness Ditch", one Cross action.
type fakeObjectDef struct{}

func (fakeObjectDef) ID() int                { return 23271 }
func (fakeObjectDef) Name() string           { return "Wilderness Ditch" }
func (fakeObjectDef) SizeX() int             { return 1 }
func (fakeObjectDef) SizeY() int             { return 2 }
func (fakeObjectDef) BlockingMask() int      { return 0 }
func (fakeObjectDef) Interactions() []string { return []string{"Cross", "", "", "", ""} }

func newTestPlayer(name string, x, y int) (*game.Player, *fakeSession) {
	p := game.NewPlayer(name, game.Location{X: x, Y: y, Z: 0})
	sess := &fakeSession{}
	p.Session = sess
	return p, sess
}

// crossEvent builds a Cross click on a ditch piece at loc with the
// given face, from a player standing at src.
func crossEvent(p *game.Player, loc game.Location, face int, src game.Location) *plugins.ObjectInteractionEvent {
	obj := game.NewGameObject(23271, loc, 10, face, nil)
	return &plugins.ObjectInteractionEvent{
		Player:         p,
		Object:         obj,
		Definition:     fakeObjectDef{},
		ObjectID:       23271,
		ClickType:      1,
		Option:         "Cross",
		HasOption:      true,
		Location:       loc,
		SourceLocation: &src,
	}
}

func loadHub(t *testing.T, tasks *game.TaskManager) *plugins.Hub {
	t.Helper()
	hub := plugins.NewHub([]plugins.Plugin{Plugin}, plugins.HubOptions{MembersWorld: true})
	if tasks != nil {
		hub.SetTaskManager(tasks)
	}
	hub.Load()
	if len(hub.Loaded()) != 1 || hub.Loaded()[0] != "WildernessDitch" {
		t.Fatalf("hub loaded = %v, want [WildernessDitch]", hub.Loaded())
	}
	return hub
}

// TestResolveJump pins resolveJump (WildernessDitch.plugin.js):
// face 0/2 runs east-west and crosses north/south off the ditch's
// y+1 threshold (default 3522); face 1/3 runs north-south and crosses
// east/west off the ditch's x+1 threshold (default: the player's own
// x, so a missing ditch always crosses west); the source location,
// when present, is what the threshold compares against.
func TestResolveJump(t *testing.T) {
	ditch := game.Location{X: 3090, Y: 3521, Z: 0}
	turn := game.Location{X: 2996, Y: 3530, Z: 0}
	south := game.Location{X: 3089, Y: 3520, Z: 0}
	north := game.Location{X: 3089, Y: 3523, Z: 0}
	west := game.Location{X: 2995, Y: 3531, Z: 0}
	east := game.Location{X: 2997, Y: 3531, Z: 0}
	cases := []struct {
		name    string
		from    game.Location
		ditch   *game.Location
		source  *game.Location
		face    int
		wantDx  int
		wantDy  int
		wantDir int
	}{
		{"face 0 south side crosses north", south, &ditch, nil, 0, 0, 3, 0},
		{"face 0 north side crosses south", north, &ditch, nil, 0, 0, -3, 2},
		{"face 2 south side crosses north", south, &ditch, nil, 2, 0, 3, 0},
		{"face 2 north side crosses south", north, &ditch, nil, 2, 0, -3, 2},
		{"face 0 no ditch defaults to 3522 north", game.Location{X: 3089, Y: 3521, Z: 0}, nil, nil, 0, 0, 3, 0},
		{"face 0 no ditch on 3522 crosses south", game.Location{X: 3089, Y: 3522, Z: 0}, nil, nil, 0, 0, -3, 2},
		{"face 1 west side crosses east", west, &turn, nil, 1, 3, 0, 1},
		{"face 1 east side crosses west", east, &turn, nil, 3, -3, 0, 3},
		{"face 3 west side crosses east", west, &turn, nil, 3, 3, 0, 1},
		{"face 3 east side crosses west", east, &turn, nil, 3, -3, 0, 3},
		{"face 1 no ditch always crosses west", west, nil, nil, 1, -3, 0, 3},
		{"face 0 source overrides from", north, &ditch, &south, 0, 0, 3, 0},
		{"face 1 source overrides from", east, &turn, &west, 1, 3, 0, 1},
	}
	for _, c := range cases {
		dx, dy, dir := resolveJump(c.from, c.ditch, c.source, c.face)
		if dx != c.wantDx || dy != c.wantDy || dir != c.wantDir {
			t.Errorf("%s: resolveJump = (%d, %d, %d), want (%d, %d, %d)",
				c.name, dx, dy, dir, c.wantDx, c.wantDy, c.wantDir)
		}
	}
}

// TestPluginMetadata pins the registered name and description against
// the plugin file (WildernessDitch.plugin.js module.exports).
func TestPluginMetadata(t *testing.T) {
	if Plugin.Name != "WildernessDitch" {
		t.Errorf("Plugin.Name = %q, want %q", Plugin.Name, "WildernessDitch")
	}
	want := "Crossing the Wilderness Ditch with the jump animation and sound, landing three tiles on, in and out of the Wilderness."
	if Plugin.Description != want {
		t.Errorf("Plugin.Description = %q, want %q", Plugin.Description, want)
	}
}

// TestCrossDitchFullFlow drives a south-side Cross through the hub:
// the handler claims the click, the jump sound goes out, the player
// lands 3 tiles north on the 3rd tick, and the click delay resets —
// so an immediate second Cross is consumed without a second jump.
func TestCrossDitchFullFlow(t *testing.T) {
	tasks := game.NewTaskManager()
	hub := loadHub(t, tasks)
	p, sess := newTestPlayer("alice", 3089, 3520)
	ditch := game.Location{X: 3090, Y: 3521, Z: 0}
	if !hub.EmitObjectInteraction(crossEvent(p, ditch, 0, p.Location())) {
		t.Fatal("south-side Cross: want handled")
	}
	if len(sess.effects) != 1 || sess.effects[0] != (soundEffectCall{soundID: 2462, loopType: 0, delay: 25, volume: 1}) {
		t.Fatalf("sound effects = %+v, want one (2462, loop 0, delay 25, volume 1)", sess.effects)
	}
	for cycle := int64(1); cycle <= 2; cycle++ {
		tasks.Process(cycle)
		if p.Location() != (game.Location{X: 3089, Y: 3520, Z: 0}) {
			t.Fatalf("after tick %d: player moved early to %+v", cycle, p.Location())
		}
	}
	tasks.Process(3)
	if p.Location() != (game.Location{X: 3089, Y: 3523, Z: 0}) {
		t.Fatalf("after tick 3: location = %+v, want (3089, 3523, 0)", p.Location())
	}
	if p.ForceMovement() != nil {
		t.Error("after landing: want no active force movement")
	}
	// The click delay reset stamps the current cycle (0 here): an
	// immediate second Cross is consumed silently — no second sound,
	// no second jump.
	if !hub.EmitObjectInteraction(crossEvent(p, ditch, 0, p.Location())) {
		t.Fatal("immediate second Cross: want handled (consumed, not fallen through)")
	}
	if len(sess.effects) != 1 {
		t.Fatalf("sound effects after second Cross = %d, want 1", len(sess.effects))
	}
	for cycle := int64(4); cycle <= 6; cycle++ {
		tasks.Process(cycle)
	}
	if p.Location() != (game.Location{X: 3089, Y: 3523, Z: 0}) {
		t.Fatalf("after the second Cross + 3 ticks: location = %+v, want the player unmoved", p.Location())
	}
}

// TestCrossDitchBackSouth crosses the same player back over the ditch
// from the north side, landing 3 tiles south.
func TestCrossDitchBackSouth(t *testing.T) {
	tasks := game.NewTaskManager()
	hub := loadHub(t, tasks)
	p, _ := newTestPlayer("alice", 3089, 3523)
	ditch := game.Location{X: 3090, Y: 3521, Z: 0}
	if !hub.EmitObjectInteraction(crossEvent(p, ditch, 0, p.Location())) {
		t.Fatal("north-side Cross: want handled")
	}
	for cycle := int64(1); cycle <= 3; cycle++ {
		tasks.Process(cycle)
	}
	if p.Location() != (game.Location{X: 3089, Y: 3520, Z: 0}) {
		t.Fatalf("after tick 3: location = %+v, want (3089, 3520, 0)", p.Location())
	}
}

// TestCrossDitchTurnSection crosses the north-south turn piece at
// 2996,3530-3533 east/west: from the west side the player lands 3
// tiles east.
func TestCrossDitchTurnSection(t *testing.T) {
	tasks := game.NewTaskManager()
	hub := loadHub(t, tasks)
	p, _ := newTestPlayer("alice", 2995, 3531)
	ditch := game.Location{X: 2996, Y: 3530, Z: 0}
	if !hub.EmitObjectInteraction(crossEvent(p, ditch, 3, p.Location())) {
		t.Fatal("turn-section Cross: want handled")
	}
	for cycle := int64(1); cycle <= 3; cycle++ {
		tasks.Process(cycle)
	}
	if p.Location() != (game.Location{X: 2998, Y: 3531, Z: 0}) {
		t.Fatalf("after tick 3: location = %+v, want (2998, 3531, 0)", p.Location())
	}
}

// TestCrossDitchBlockedByActiveForceMovement pins the first guard:
// a player already gliding has the Cross consumed silently — no
// sound, no new task, no movement.
func TestCrossDitchBlockedByActiveForceMovement(t *testing.T) {
	tasks := game.NewTaskManager()
	hub := loadHub(t, tasks)
	p, sess := newTestPlayer("alice", 3089, 3520)
	p.SetForceMovement(&game.ForceMovement{Start: p.Location(), Delta: game.Location{X: 0, Y: 3}, Direction: 0})
	ditch := game.Location{X: 3090, Y: 3521, Z: 0}
	if !hub.EmitObjectInteraction(crossEvent(p, ditch, 0, p.Location())) {
		t.Fatal("Cross during a glide: want handled (consumed, not fallen through)")
	}
	if len(sess.effects) != 0 {
		t.Fatalf("sound effects = %+v, want none", sess.effects)
	}
	for cycle := int64(1); cycle <= 3; cycle++ {
		tasks.Process(cycle)
	}
	if p.Location() != (game.Location{X: 3089, Y: 3520, Z: 0}) {
		t.Fatalf("after 3 ticks: location = %+v, want the player unmoved", p.Location())
	}
}

// TestCrossDitchClickDelayBlocks pins the second guard: a Cross
// within 250 ms of a click-delay reset is consumed silently; once a
// cycle (600 ms) has passed, the same Cross jumps.
func TestCrossDitchClickDelayBlocks(t *testing.T) {
	tasks := game.NewTaskManager()
	hub := loadHub(t, tasks)
	p, sess := newTestPlayer("alice", 3089, 3520)
	// Install the cycle source BEFORE the reset, so the stamp is
	// taken from this test's cycle (see TestClickDelayElapsed).
	cycle := int64(0)
	game.SetCombatCycleSource(func() int64 { return cycle })
	defer game.SetCombatCycleSource(nil)
	p.ResetClickDelay()
	ditch := game.Location{X: 3090, Y: 3521, Z: 0}
	if !hub.EmitObjectInteraction(crossEvent(p, ditch, 0, p.Location())) {
		t.Fatal("Cross inside the click delay: want handled (consumed, not fallen through)")
	}
	if len(sess.effects) != 0 {
		t.Fatalf("sound effects = %+v, want none", sess.effects)
	}
	cycle = 1
	if !hub.EmitObjectInteraction(crossEvent(p, ditch, 0, p.Location())) {
		t.Fatal("Cross after the click delay: want handled")
	}
	if len(sess.effects) != 1 {
		t.Fatalf("sound effects = %d, want 1", len(sess.effects))
	}
	for c := int64(1); c <= 3; c++ {
		tasks.Process(c)
	}
	if p.Location() != (game.Location{X: 3089, Y: 3523, Z: 0}) {
		t.Fatalf("after tick 3: location = %+v, want (3089, 3523, 0)", p.Location())
	}
}

// TestCrossDitchNilTaskManagerFallsThrough pins the one guard that
// does NOT consume: with no TaskManager installed (impossible in
// production), the handler returns false so the failure is visible
// instead of silently swallowed.
func TestCrossDitchNilTaskManagerFallsThrough(t *testing.T) {
	hub := loadHub(t, nil)
	p, sess := newTestPlayer("alice", 3089, 3520)
	ditch := game.Location{X: 3090, Y: 3521, Z: 0}
	if hub.EmitObjectInteraction(crossEvent(p, ditch, 0, p.Location())) {
		t.Fatal("Cross with no TaskManager: want NOT handled (fall through)")
	}
	if len(sess.effects) != 0 {
		t.Fatalf("sound effects = %+v, want none", sess.effects)
	}
}
