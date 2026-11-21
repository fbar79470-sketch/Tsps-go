package game

import "testing"

// recordingIconSession is a fakeSession that also records
// multi-icon sends (the MultiIconSender extension).
type recordingIconSession struct {
	*fakeSession
	icons []int
}

func (s *recordingIconSession) SendMultiIcon(value int) {
	s.icons = append(s.icons, value)
}

func TestPlayerProcessRunsAreaPhase(t *testing.T) {
	w := NewWorld(Location{X: 3000, Y: 3000})
	var entered, processed []string
	area := NewArea("TestArea", []Boundary{NewBoundary(2990, 3010, 2990, 3010, 0)})
	area.PostEnterHook = func(a AreaActor) { entered = append(entered, "enter") }
	area.ProcessHook = func(a AreaActor) { processed = append(processed, "process") }
	w.Areas().Register(area)

	p := NewPlayer("alice", Location{X: 3000, Y: 3000})
	sess := &recordingIconSession{fakeSession: newFakeSession()}
	p.Session = sess
	w.QueueAddPlayer(p)
	w.Process()

	if p.Area() != area {
		t.Fatal("player in the area must resolve it on the first tick")
	}
	if len(entered) != 1 {
		t.Fatalf("enter fired %d times, want 1", len(entered))
	}
	if len(processed) != 1 {
		t.Fatalf("process fired %d times, want 1", len(processed))
	}
	if !area.HasMember(p) {
		t.Fatal("player must be an area member after entering")
	}
	// The first tick always sends the multi icon (TS: the
	// uninitialized field compares unequal to 0): area is not
	// multi and no Wilderness is installed, so 0.
	if len(sess.icons) != 1 || sess.icons[0] != 0 {
		t.Fatalf("icons = %v, want [0]", sess.icons)
	}
	// Staying put: no re-enter, process each tick, no icon resend.
	w.Process()
	if len(entered) != 1 {
		t.Fatalf("enter fired %d times after staying, want 1", len(entered))
	}
	if len(processed) != 2 {
		t.Fatalf("process fired %d times, want 2", len(processed))
	}
	if len(sess.icons) != 1 {
		t.Fatalf("icons = %v, want no resend", sess.icons)
	}
}

func TestPlayerMultiIconFollowsArea(t *testing.T) {
	w := NewWorld(Location{X: 3000, Y: 3000})
	area := NewArea("MultiArea", []Boundary{NewBoundary(2990, 3010, 2990, 3010, 0)})
	area.IsMultiHook = func() bool { return true }
	w.Areas().Register(area)

	p := NewPlayer("bob", Location{X: 3000, Y: 3000})
	sess := &recordingIconSession{fakeSession: newFakeSession()}
	p.Session = sess
	w.QueueAddPlayer(p)
	w.Process()
	if len(sess.icons) != 1 || sess.icons[0] != 1 {
		t.Fatalf("icons = %v, want [1] in a multi area", sess.icons)
	}
	// Walk out: the icon clears one tick after leaving (the TS
	// stale-area subtlety — AreaManager.Process reads the
	// previous area for the multi-icon block on the leave
	// tick, so the clear lands on the following tick).
	p.SetLocation(Location{X: 3100, Y: 3100})
	w.Process()
	if len(sess.icons) != 1 {
		t.Fatalf("icons = %v, want no send on the leave tick", sess.icons)
	}
	w.Process()
	if len(sess.icons) != 2 || sess.icons[1] != 0 {
		t.Fatalf("icons = %v, want [1 0]", sess.icons)
	}
}

func TestPlayerAreaLeaveOnRemoval(t *testing.T) {
	w := NewWorld(Location{X: 3000, Y: 3000})
	var left []bool
	area := NewArea("LeaveArea", []Boundary{NewBoundary(2990, 3010, 2990, 3010, 0)})
	area.PostLeaveHook = func(a AreaActor, logout bool) { left = append(left, logout) }
	w.Areas().Register(area)

	p := NewPlayer("carol", Location{X: 3000, Y: 3000})
	p.Session = newFakeSession()
	w.QueueAddPlayer(p)
	w.Process()
	if !area.HasMember(p) {
		t.Fatal("player must be a member before removal")
	}
	w.QueueRemovePlayer(p.Index)
	w.Process()
	if len(left) != 1 || !left[0] {
		t.Fatalf("leave calls = %v, want one logout=true leave", left)
	}
	if area.HasMember(p) {
		t.Fatal("removed player must not stay an area member")
	}
	if len(w.Areas().hints) != 0 {
		t.Fatalf("hints = %d entries, want 0 after removal", len(w.Areas().hints))
	}
}

func TestPlayerAreaLeaveFailureDoesNotAbortRemoval(t *testing.T) {
	w := NewWorld(Location{X: 3000, Y: 3000})
	area := NewArea("PanicArea", []Boundary{NewBoundary(2990, 3010, 2990, 3010, 0)})
	area.PostLeaveHook = func(a AreaActor, logout bool) { panic("postLeave boom") }
	w.Areas().Register(area)

	p := NewPlayer("dave", Location{X: 3000, Y: 3000})
	p.Session = newFakeSession()
	w.QueueAddPlayer(p)
	w.Process()
	w.QueueRemovePlayer(p.Index)
	w.Process() // must not panic: the leave failure is reported, removal completes.
	if p.IsRegistered() {
		t.Fatal("player must be removed despite the throwing leave")
	}
}

func TestNPCProcessRunsAreaPhase(t *testing.T) {
	w := NewWorld(Location{X: 3000, Y: 3000})
	var entered int
	area := NewArea("NpcArea", []Boundary{NewBoundary(2990, 3010, 2990, 3010, 0)})
	area.PostEnterHook = func(a AreaActor) { entered++ }
	w.Areas().Register(area)

	n := NewNPC(1, Location{X: 3000, Y: 3000})
	w.QueueAddNPC(n)
	w.Process() // registers; first mechanics turn waits a tick.
	w.Process()
	if entered != 1 {
		t.Fatalf("enter fired %d times, want 1", entered)
	}
	if n.Area() != area {
		t.Fatal("NPC in the area must resolve it")
	}
	if !area.HasMember(n) {
		t.Fatal("NPC must be an area member after entering")
	}
}

func TestBotAreaDownsampling(t *testing.T) {
	// Bots process areas every third tick while idle
	// (Player.ts:547-554). Drive Process directly with the
	// manager installed, as World would on add.
	m := NewAreaManager()
	var processed int
	area := NewArea("BotArea", []Boundary{NewBoundary(2990, 3010, 2990, 3010, 0)})
	area.ProcessHook = func(a AreaActor) { processed++ }
	m.Register(area)

	p := NewPlayer("bot", Location{X: 3000, Y: 3000})
	p.IsBot = true
	p.areaManager = m
	for cycle := int64(1); cycle <= 6; cycle++ {
		p.Process(cycle)
	}
	if processed != 2 {
		t.Fatalf("area process ran %d times in 6 idle bot ticks, want 2", processed)
	}
	// A moving bot processes every tick.
	processed = 0
	p2 := NewPlayer("bot2", Location{X: 3000, Y: 3000})
	p2.IsBot = true
	p2.areaManager = m
	// Force the moving term: enqueue a route so the queue
	// reports moving. A zero-step route still counts as
	// moving while the queue has pending work; instead drive
	// the term through a queued checkpoint.
	p2.MovementQueue().AddCheckpoint(Location{X: 3001, Y: 3000, Z: 0})
	p2.Process(1)
	if processed != 1 {
		t.Fatalf("area process ran %d times for a moving bot, want 1", processed)
	}
}

func TestPlayerMultiIconUnsetSentinel(t *testing.T) {
	p := NewPlayer("eve", Location{})
	if p.MultiIcon() != -1 {
		t.Fatalf("MultiIcon() = %d, want -1 (unset sentinel)", p.MultiIcon())
	}
	// SetMultiIcon change-detects: sending the recorded value
	// again sends nothing.
	sess := &recordingIconSession{fakeSession: newFakeSession()}
	p.Session = sess
	p.SetMultiIcon(0)
	p.SetMultiIcon(0)
	if len(sess.icons) != 1 {
		t.Fatalf("icons = %v, want exactly one send", sess.icons)
	}
	// A session-less player records without sending.
	p2 := NewPlayer("frank", Location{})
	p2.SetMultiIcon(1)
	if p2.MultiIcon() != 1 {
		t.Fatalf("MultiIcon() = %d, want 1", p2.MultiIcon())
	}
}
