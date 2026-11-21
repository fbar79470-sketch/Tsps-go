package game

import "testing"

// Fixture actors implementing AreaActor. The coordinator has
// not yet added the Area accessors to *Player/*NPC, so tests
// wrap real players/NPCs (for typed member identity) with
// fixture-held area state. See AreaActor for the contract
// each method satisfies.

// fakePlayerActor is a player fixture: it holds a real
// *Player for identity, its own area slot, and a multi-icon
// value starting at the unset sentinel -1 (as the
// coordinator's *Player must — see MultiIconActor).
type fakePlayerActor struct {
	player        *Player
	area          *Area
	multiIcon     int
	multiIconSets []int
}

func newFakePlayer(index int, loc Location) *fakePlayerActor {
	p := NewPlayer("fixture", loc)
	p.SetIndex(index)
	return &fakePlayerActor{player: p, multiIcon: -1}
}

func (f *fakePlayerActor) Location() Location { return f.player.Location() }
func (f *fakePlayerActor) Area() *Area        { return f.area }
func (f *fakePlayerActor) SetArea(a *Area)    { f.area = a }
func (f *fakePlayerActor) GetIndex() int      { return f.player.Index }
func (f *fakePlayerActor) IsPlayer() bool     { return true }
func (f *fakePlayerActor) IsNPC() bool        { return false }
func (f *fakePlayerActor) AsPlayer() *Player  { return f.player }
func (f *fakePlayerActor) AsNPC() *NPC        { return nil }
func (f *fakePlayerActor) MultiIcon() int     { return f.multiIcon }
func (f *fakePlayerActor) SetMultiIcon(i int) {
	f.multiIcon = i
	f.multiIconSets = append(f.multiIconSets, i)
}
func (f *fakePlayerActor) moveTo(loc Location) { f.player.SetLocation(loc) }

// fakeNPCActor is an NPC fixture. It deliberately does NOT
// implement MultiIconActor: NPCs have no multi icon.
type fakeNPCActor struct {
	npc  *NPC
	area *Area
}

func newFakeNPC(index int, loc Location) *fakeNPCActor {
	n := NewNPC(1, loc)
	n.SetIndex(index)
	return &fakeNPCActor{npc: n}
}

func (f *fakeNPCActor) Location() Location { return f.npc.Location() }
func (f *fakeNPCActor) Area() *Area        { return f.area }
func (f *fakeNPCActor) SetArea(a *Area)    { f.area = a }
func (f *fakeNPCActor) GetIndex() int      { return f.npc.Index }
func (f *fakeNPCActor) IsPlayer() bool     { return false }
func (f *fakeNPCActor) IsNPC() bool        { return true }
func (f *fakeNPCActor) AsPlayer() *Player  { return nil }
func (f *fakeNPCActor) AsNPC() *NPC        { return f.npc }
func (f *fakeNPCActor) moveTo(loc Location) {
	f.npc.SetLocation(loc)
}

// fakeNeitherActor is neither player nor NPC: Area must
// track it in neither map but still run postEnter/postLeave.
type fakeNeitherActor struct {
	loc  Location
	area *Area
}

func (f *fakeNeitherActor) Location() Location { return f.loc }
func (f *fakeNeitherActor) Area() *Area        { return f.area }
func (f *fakeNeitherActor) SetArea(a *Area)    { f.area = a }
func (f *fakeNeitherActor) GetIndex() int      { return 7 }
func (f *fakeNeitherActor) IsPlayer() bool     { return false }
func (f *fakeNeitherActor) IsNPC() bool        { return false }
func (f *fakeNeitherActor) AsPlayer() *Player  { return nil }
func (f *fakeNeitherActor) AsNPC() *NPC        { return nil }

// fakePlayerNoIcon is a player fixture WITHOUT the
// MultiIconActor methods: AreaManager.Process must skip the
// multi-icon update for it without panicking.
type fakePlayerNoIcon struct {
	player *Player
	area   *Area
}

func (f *fakePlayerNoIcon) Location() Location { return f.player.Location() }
func (f *fakePlayerNoIcon) Area() *Area        { return f.area }
func (f *fakePlayerNoIcon) SetArea(a *Area)    { f.area = a }
func (f *fakePlayerNoIcon) GetIndex() int      { return f.player.Index }
func (f *fakePlayerNoIcon) IsPlayer() bool     { return true }
func (f *fakePlayerNoIcon) IsNPC() bool        { return false }
func (f *fakePlayerNoIcon) AsPlayer() *Player  { return f.player }
func (f *fakePlayerNoIcon) AsNPC() *NPC        { return nil }

var (
	_ AreaActor      = (*fakePlayerActor)(nil)
	_ AreaActor      = (*fakeNPCActor)(nil)
	_ AreaActor      = (*fakeNeitherActor)(nil)
	_ AreaActor      = (*fakePlayerNoIcon)(nil)
	_ MultiIconActor = (*fakePlayerActor)(nil)
)

func TestAreaEnterLeaveMembers(t *testing.T) {
	a := NewArea("Test", nil)
	player := newFakePlayer(1, Location{X: 0, Y: 0, Z: 0})
	npc := newFakeNPC(2, Location{X: 0, Y: 0, Z: 0})
	neither := &fakeNeitherActor{}

	var events []string
	a.PostEnterHook = func(actor AreaActor) { events = append(events, "enter") }
	a.PostLeaveHook = func(actor AreaActor, logout bool) {
		events = append(events, "leave")
	}

	a.Enter(player)
	a.Enter(npc)
	a.Enter(neither)
	if !a.HasMember(player) || !a.HasMember(npc) {
		t.Fatal("members missing after Enter")
	}
	if a.HasMember(neither) {
		t.Error("neither-actor is a member, want not tracked")
	}
	if got := a.GetPlayers(); len(got) != 1 || got[0] != player.player {
		t.Errorf("GetPlayers = %v, want [player]", got)
	}
	if got := a.GetNPCs(); len(got) != 1 || got[0] != npc.npc {
		t.Errorf("GetNPCs = %v, want [npc]", got)
	}
	// postEnter runs for all three, including the actor
	// tracked in neither map (Area.ts:33).
	if len(events) != 3 {
		t.Errorf("postEnter events = %d, want 3", len(events))
	}

	var logoutSeen []bool
	a.PostLeaveHook = func(actor AreaActor, logout bool) { logoutSeen = append(logoutSeen, logout) }
	a.Leave(player, false)
	a.Leave(npc, true)
	a.Leave(neither, false)
	if a.HasMember(player) || a.HasMember(npc) {
		t.Error("members still present after Leave")
	}
	if len(a.GetPlayers()) != 0 || len(a.GetNPCs()) != 0 {
		t.Error("member lists non-empty after Leave")
	}
	// postLeave runs even for the never-member (Area.ts:46).
	if len(logoutSeen) != 3 || logoutSeen[0] || !logoutSeen[1] || logoutSeen[2] {
		t.Errorf("postLeave logout flags = %v, want [false true false]", logoutSeen)
	}
}

// Entering a new area makes the previous area hear the actor
// leave first (Area.ts:22-25) — but only when the previous
// area differs AND still holds the actor.
func TestAreaEnterPreviousAreaLeave(t *testing.T) {
	a := NewArea("A", nil)
	b := NewArea("B", nil)
	var leftA []bool
	a.PostLeaveHook = func(actor AreaActor, logout bool) { leftA = append(leftA, logout) }

	player := newFakePlayer(1, Location{})
	a.Enter(player)
	player.SetArea(a)

	b.Enter(player)
	if len(leftA) != 1 || leftA[0] {
		t.Errorf("previous leave = %v, want [false]", leftA)
	}
	if a.HasMember(player) {
		t.Error("previous area still holds the actor")
	}
	if !b.HasMember(player) {
		t.Error("new area does not hold the actor")
	}

	// Previous area recorded but not holding the actor: no
	// leave call (the hasMember guard).
	c := NewArea("C", nil)
	leftA = nil
	player.SetArea(a) // recorded, but a no longer holds it
	c.Enter(player)
	if len(leftA) != 0 {
		t.Errorf("leave fired for non-member previous area: %v", leftA)
	}
}

// Re-entering the SAME area must not fire leave: the
// `previous !== this` identity check (Area.ts:23). This is
// the check an embedding-based design would break.
func TestAreaEnterSameAreaNoLeave(t *testing.T) {
	a := NewArea("A", nil)
	var leaves int
	a.PostLeaveHook = func(actor AreaActor, logout bool) { leaves++ }
	player := newFakePlayer(1, Location{})
	a.Enter(player)
	player.SetArea(a)
	a.Enter(player)
	if leaves != 0 {
		t.Errorf("leaves = %d, want 0 on same-area re-enter", leaves)
	}
	if !a.HasMember(player) {
		t.Error("actor not a member after re-enter")
	}
}

// Membership is identity, not index alone (Area.ts:54-56): a
// different actor reusing an index is not a member, and
// entering it overwrites the stale entry.
func TestAreaHasMemberIdentity(t *testing.T) {
	a := NewArea("A", nil)
	first := newFakePlayer(3, Location{})
	second := newFakePlayer(3, Location{})
	a.Enter(first)
	if a.HasMember(second) {
		t.Error("different actor with same index is a member")
	}
	a.Enter(second)
	if a.HasMember(first) {
		t.Error("stale actor still a member after overwrite")
	}
	if !a.HasMember(second) {
		t.Error("second actor not a member after Enter")
	}
	if got := a.GetPlayers(); len(got) != 1 || got[0] != second.player {
		t.Errorf("GetPlayers = %v, want [second]", got)
	}
}

// Member lists follow JS Object.values order: ascending
// numeric index (Area.ts:126-132).
func TestAreaGetMembersSortedByIndex(t *testing.T) {
	a := NewArea("A", nil)
	for _, index := range []int{5, 2, 9, 1} {
		a.Enter(newFakePlayer(index, Location{}))
	}
	got := a.GetPlayers()
	want := []int{1, 2, 5, 9}
	if len(got) != len(want) {
		t.Fatalf("GetPlayers len = %d, want %d", len(got), len(want))
	}
	for i, index := range want {
		if got[i].Index != index {
			t.Errorf("GetPlayers[%d].Index = %d, want %d", i, got[i].Index, index)
		}
	}
}

func areaBoolPtr(b bool) *bool { return &b }

func TestAreaDefaults(t *testing.T) {
	a := NewArea("Defaults", []Boundary{NewBoundary(0, 1, 0, 1, 0)})
	player := newFakePlayer(1, Location{})
	npc := newFakeNPC(2, Location{})

	// No-op defaults must not panic.
	a.Process(player)

	if track, ok := a.GetMusicTrack(player.player); ok || track != 0 {
		t.Errorf("GetMusicTrack = (%d, %v), want (0, false)", track, ok)
	}
	if a.CanAttack(player, npc, nil) != nil {
		t.Error("CanAttack default non-nil, want no opinion")
	}
	if a.CanCastOnNpc(player.player, npc.npc, nil) != nil {
		t.Error("CanCastOnNpc default non-nil")
	}
	if a.CanTeleport(player.player, 0, nil) != nil {
		t.Error("CanTeleport default non-nil")
	}
	if a.CanTrade(player.player, player.player) != nil {
		t.Error("CanTrade default non-nil")
	}
	if a.HandleForcedWalk(player.player, Location{}) {
		t.Error("HandleForcedWalk default true, want false")
	}
	if a.NpcAggressionTolerance(player.player, npc.npc) != nil {
		t.Error("NpcAggressionTolerance default non-nil")
	}
	if a.NpcAggression(player.player, npc.npc) != nil {
		t.Error("NpcAggression default non-nil")
	}
	if a.IsMulti() {
		t.Error("IsMulti default true, want false")
	}
	if a.GetName() != "Defaults" {
		t.Errorf("GetName = %q, want %q", a.GetName(), "Defaults")
	}
	if a.PluginName() != "" {
		t.Errorf("PluginName = %q, want empty", a.PluginName())
	}
	a.SetPluginName("SomePlugin")
	if a.PluginName() != "SomePlugin" {
		t.Errorf("PluginName = %q, want SomePlugin", a.PluginName())
	}
	if got := a.GetBoundaries(); len(got) != 1 {
		t.Errorf("GetBoundaries len = %d, want 1", len(got))
	}
	if NewArea("Empty", nil).GetBoundaries() != nil {
		t.Error("nil boundaries not preserved")
	}
}

func TestAreaHooks(t *testing.T) {
	a := NewArea("Hooked", nil)
	player := newFakePlayer(1, Location{})
	npc := newFakeNPC(2, Location{})

	var processed []AreaActor
	a.ProcessHook = func(actor AreaActor) { processed = append(processed, actor) }
	a.MusicTrackHook = func(p *Player) (int, bool) { return 98, true }
	a.CanAttackHook = func(attacker, target AreaActor, method any) *bool { return areaBoolPtr(false) }
	a.CanCastOnNpcHook = func(p *Player, n *NPC, spell any) *bool { return areaBoolPtr(true) }
	a.CanTeleportHook = func(p *Player, limit int, dest any) *bool { return nil }
	a.CanTradeHook = func(p, target *Player) *bool { return areaBoolPtr(true) }
	a.HandleForcedWalkHook = func(p *Player, dest Location) bool { return true }
	a.NpcAggressionToleranceHook = func(p *Player, n *NPC) *bool { return areaBoolPtr(true) }
	a.NpcAggressionHook = func(p *Player, n *NPC) *bool { return areaBoolPtr(false) }
	a.IsMultiHook = func() bool { return true }

	a.Process(player)
	if len(processed) != 1 || processed[0] != AreaActor(player) {
		t.Error("ProcessHook did not fire with the actor")
	}
	if track, ok := a.GetMusicTrack(player.player); !ok || track != 98 {
		t.Errorf("GetMusicTrack = (%d, %v), want (98, true)", track, ok)
	}
	if got := a.CanAttack(player, npc, nil); got == nil || *got {
		t.Errorf("CanAttack = %v, want false", got)
	}
	if got := a.CanCastOnNpc(player.player, npc.npc, nil); got == nil || !*got {
		t.Errorf("CanCastOnNpc = %v, want true", got)
	}
	if a.CanTeleport(player.player, 0, nil) != nil {
		t.Error("CanTeleport hook returning nil must stay nil (no opinion)")
	}
	if got := a.CanTrade(player.player, player.player); got == nil || !*got {
		t.Errorf("CanTrade = %v, want true", got)
	}
	if !a.HandleForcedWalk(player.player, Location{}) {
		t.Error("HandleForcedWalk hook not honored")
	}
	if got := a.NpcAggressionTolerance(player.player, npc.npc); got == nil || !*got {
		t.Errorf("NpcAggressionTolerance = %v, want true", got)
	}
	if got := a.NpcAggression(player.player, npc.npc); got == nil || *got {
		t.Errorf("NpcAggression = %v, want false", got)
	}
	if !a.IsMulti() {
		t.Error("IsMulti hook not honored")
	}
}

// recordingCaller records AreaMethodCaller invocations and
// delegates to the real methods.
type recordingCaller struct {
	calls []string
}

func (c *recordingCaller) Enter(area *Area, actor AreaActor) {
	c.calls = append(c.calls, "enter:"+area.GetName())
	area.Enter(actor)
}
func (c *recordingCaller) Leave(area *Area, actor AreaActor, logout bool) {
	c.calls = append(c.calls, "leave:"+area.GetName())
	area.Leave(actor, logout)
}
func (c *recordingCaller) Process(area *Area, actor AreaActor) {
	c.calls = append(c.calls, "process:"+area.GetName())
	area.Process(actor)
}
func (c *recordingCaller) MusicTrack(area *Area, player *Player) (int, bool) {
	c.calls = append(c.calls, "music:"+area.GetName())
	return area.GetMusicTrack(player)
}

// The callArea seam routes manager-driven enter/leave/
// process calls (and the previous-area leave inside Enter)
// through the installed caller, and restores direct calls
// when reset.
func TestAreaMethodCallerSeam(t *testing.T) {
	rec := &recordingCaller{}
	SetAreaMethodCaller(rec)
	defer SetAreaMethodCaller(nil)

	mgr := NewAreaManager()
	a := NewArea("A", []Boundary{NewBoundary(0, 10, 0, 10, 0)})
	mgr.Register(a)
	player := newFakePlayer(1, Location{X: 5, Y: 5, Z: 0})
	mgr.Process(player)
	mgr.Process(player) // stationary: process only, no re-enter
	player.moveTo(Location{X: 50, Y: 50, Z: 0})
	mgr.Process(player)

	want := []string{"enter:A", "process:A", "process:A", "leave:A"}
	if len(rec.calls) != len(want) {
		t.Fatalf("caller calls = %v, want %v", rec.calls, want)
	}
	for i, call := range want {
		if rec.calls[i] != call {
			t.Errorf("caller calls[%d] = %q, want %q", i, rec.calls[i], call)
		}
	}

	// The previous-area leave inside Enter also routes
	// through the seam.
	rec.calls = nil
	b := NewArea("B", nil)
	a.Enter(player)
	player.SetArea(a)
	b.Enter(player)
	if len(rec.calls) != 1 || rec.calls[0] != "leave:A" {
		t.Errorf("previous-leave calls = %v, want [leave:A]", rec.calls)
	}

	// MusicTrack routes through the seam; a nil area never
	// reaches the caller.
	rec.calls = nil
	a.MusicTrackHook = func(p *Player) (int, bool) { return 42, true }
	if track, ok := CallAreaMusicTrack(a, player.player); !ok || track != 42 {
		t.Errorf("CallAreaMusicTrack = (%d, %v), want (42, true)", track, ok)
	}
	if track, ok := CallAreaMusicTrack(nil, player.player); ok || track != 0 {
		t.Errorf("CallAreaMusicTrack(nil) = (%d, %v), want (0, false)", track, ok)
	}
	if len(rec.calls) != 1 || rec.calls[0] != "music:A" {
		t.Errorf("music calls = %v, want [music:A]", rec.calls)
	}

	// Resetting restores direct calls: no recording, hooks
	// still fire.
	SetAreaMethodCaller(nil)
	rec.calls = nil
	var processed int
	a.ProcessHook = func(actor AreaActor) { processed++ }
	callAreaProcess(a, player)
	if processed != 1 {
		t.Error("direct call after reset did not run the hook")
	}
	if len(rec.calls) != 0 {
		t.Errorf("calls recorded after reset: %v", rec.calls)
	}
}
