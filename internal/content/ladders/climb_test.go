package ladders

import (
	"testing"

	"tsps-go/internal/game"
)

// fakeActor is a ClimbActor fake: it records animations and moves
// and lets the test drive location, hitpoints, area and the
// movement block.
type fakeActor struct {
	loc     game.Location
	hp      int
	area    *game.PrivateArea
	blocked bool
	anims   []int
	moved   []game.Location
}

func (a *fakeActor) Location() game.Location        { return a.loc }
func (a *fakeActor) Hitpoints() int                 { return a.hp }
func (a *fakeActor) PrivateArea() *game.PrivateArea { return a.area }
func (a *fakeActor) PerformAnimation(id, delay int) { a.anims = append(a.anims, id) }
func (a *fakeActor) MovementBlocked() bool          { return a.blocked }
func (a *fakeActor) SetMovementBlocked(b bool)      { a.blocked = b }
func (a *fakeActor) ResetMovementQueue()            {}
func (a *fakeActor) MoveTo(loc game.Location)       { a.loc = loc; a.moved = append(a.moved, loc) }

func TestClimbTickOrder(t *testing.T) {
	tm := game.NewTaskManager()
	actor := &fakeActor{loc: game.Location{X: 10, Y: 10, Z: 0}, hp: 10}
	dest := game.Location{X: 10, Y: 10, Z: 1}
	arrived := false
	task, ok := StartClimb(tm, actor, dest, ClimbUpAnimation, func() { arrived = true })
	if !ok || task == nil {
		t.Fatal("StartClimb refused a climbable climb")
	}
	if !actor.blocked {
		t.Error("movement not blocked after StartClimb")
	}
	// Tick 1: the animation plays first; no move, no arrival.
	tm.Process(1)
	if len(actor.anims) != 1 || actor.anims[0] != ClimbUpAnimation {
		t.Errorf("anims after tick 1 = %v, want [828]", actor.anims)
	}
	if len(actor.moved) != 0 || arrived {
		t.Errorf("moved=%v arrived=%v after tick 1, want no move and no arrival", actor.moved, arrived)
	}
	if !actor.blocked {
		t.Error("movement unblocked after tick 1 (animation only)")
	}
	// Tick 2: the plane move, then onArrival.
	tm.Process(2)
	if len(actor.moved) != 1 || actor.moved[0] != dest {
		t.Errorf("moved after tick 2 = %v, want [%v]", actor.moved, dest)
	}
	if !arrived {
		t.Error("onArrival not called after the move")
	}
	if actor.blocked {
		t.Error("movement still blocked after the climb finished")
	}
}

func TestClimbNoAnimationMovesOnFirstTick(t *testing.T) {
	tm := game.NewTaskManager()
	actor := &fakeActor{loc: game.Location{X: 10, Y: 10, Z: 0}, hp: 10}
	dest := game.Location{X: 10, Y: 10, Z: 1}
	arrived := false
	if _, ok := StartClimb(tm, actor, dest, NoAnimation, func() { arrived = true }); !ok {
		t.Fatal("StartClimb refused")
	}
	tm.Process(1)
	if len(actor.anims) != 0 {
		t.Errorf("anims = %v, want none for animate=false", actor.anims)
	}
	if len(actor.moved) != 1 || actor.moved[0] != dest || !arrived {
		t.Errorf("moved=%v arrived=%v after tick 1, want moved + arrived", actor.moved, arrived)
	}
	if actor.blocked {
		t.Error("movement still blocked after the climb finished")
	}
}

func TestClimbMovementBlockedRefusal(t *testing.T) {
	tm := game.NewTaskManager()
	actor := &fakeActor{loc: game.Location{X: 10, Y: 10, Z: 0}, hp: 10, blocked: true}
	if task, ok := StartClimb(tm, actor, game.Location{X: 10, Y: 10, Z: 1}, ClimbUpAnimation, nil); ok || task != nil {
		t.Errorf("StartClimb = (%v, %v), want refusal", task, ok)
	}
	if tm.Count() != 0 {
		t.Errorf("TaskManager count = %d, want 0 (nothing scheduled)", tm.Count())
	}
}

func TestClimbMovedMidClimbCancels(t *testing.T) {
	tm := game.NewTaskManager()
	actor := &fakeActor{loc: game.Location{X: 10, Y: 10, Z: 0}, hp: 10}
	dest := game.Location{X: 10, Y: 10, Z: 1}
	arrived := false
	if _, ok := StartClimb(tm, actor, dest, ClimbUpAnimation, func() { arrived = true }); !ok {
		t.Fatal("StartClimb refused")
	}
	tm.Process(1)                                 // animation plays
	actor.loc = game.Location{X: 11, Y: 10, Z: 0} // player moved
	tm.Process(2)
	if len(actor.moved) != 0 || arrived {
		t.Errorf("moved=%v arrived=%v after moving mid-climb, want neither", actor.moved, arrived)
	}
	if actor.blocked {
		t.Error("movement still blocked after the cancelled climb")
	}
}

func TestClimbDiedMidClimbCancels(t *testing.T) {
	tm := game.NewTaskManager()
	actor := &fakeActor{loc: game.Location{X: 10, Y: 10, Z: 0}, hp: 10}
	arrived := false
	if _, ok := StartClimb(tm, actor, game.Location{X: 10, Y: 10, Z: 1}, ClimbUpAnimation, func() { arrived = true }); !ok {
		t.Fatal("StartClimb refused")
	}
	tm.Process(1) // animation plays
	actor.hp = 0  // player died
	tm.Process(2)
	if len(actor.moved) != 0 || arrived {
		t.Errorf("moved=%v arrived=%v after dying mid-climb, want neither", actor.moved, arrived)
	}
	if actor.blocked {
		t.Error("movement still blocked after the cancelled climb")
	}
}

func TestClimbPrivateAreaChangedCancels(t *testing.T) {
	tm := game.NewTaskManager()
	actor := &fakeActor{loc: game.Location{X: 10, Y: 10, Z: 0}, hp: 10}
	arrived := false
	if _, ok := StartClimb(tm, actor, game.Location{X: 10, Y: 10, Z: 1}, ClimbUpAnimation, func() { arrived = true }); !ok {
		t.Fatal("StartClimb refused")
	}
	tm.Process(1)                    // animation plays
	actor.area = &game.PrivateArea{} // player changed private area
	tm.Process(2)
	if len(actor.moved) != 0 || arrived {
		t.Errorf("moved=%v arrived=%v after area change, want neither", actor.moved, arrived)
	}
	if actor.blocked {
		t.Error("movement still blocked after the cancelled climb")
	}
}

func TestClimbStopUnblocks(t *testing.T) {
	tm := game.NewTaskManager()
	actor := &fakeActor{loc: game.Location{X: 10, Y: 10, Z: 0}, hp: 10}
	arrived := false
	task, ok := StartClimb(tm, actor, game.Location{X: 10, Y: 10, Z: 1}, ClimbUpAnimation, func() { arrived = true })
	if !ok || task == nil {
		t.Fatal("StartClimb refused")
	}
	tm.Process(1) // animation plays
	task.Stop()   // stop() unblocks movement
	if actor.blocked {
		t.Error("movement still blocked after task.Stop()")
	}
	tm.Process(2)
	if len(actor.moved) != 0 || arrived {
		t.Errorf("moved=%v arrived=%v after stop, want neither", actor.moved, arrived)
	}
}

func TestClimbNilSchedulerOrActor(t *testing.T) {
	tm := game.NewTaskManager()
	actor := &fakeActor{loc: game.Location{X: 10, Y: 10, Z: 0}, hp: 10}
	dest := game.Location{X: 10, Y: 10, Z: 1}
	if _, ok := StartClimb(nil, actor, dest, ClimbUpAnimation, nil); ok {
		t.Error("StartClimb(nil scheduler) started, want refusal")
	}
	if _, ok := StartClimb(tm, nil, dest, ClimbUpAnimation, nil); ok {
		t.Error("StartClimb(nil actor) started, want refusal")
	}
}

func TestPlayerClimberAdapter(t *testing.T) {
	p := game.NewPlayer("climber", game.Location{X: 3200, Y: 3200, Z: 0})
	actor := PlayerClimber(p)
	if actor == nil {
		t.Fatal("PlayerClimber(nil player) — expected non-nil for a real player")
	}
	if actor.Location() != p.Location() {
		t.Errorf("Location() = %v, want %v", actor.Location(), p.Location())
	}
	if actor.Hitpoints() <= 0 {
		t.Errorf("Hitpoints() = %d, want > 0 for a fresh player", actor.Hitpoints())
	}
	if actor.PrivateArea() != nil {
		t.Errorf("PrivateArea() = %v, want nil in the shared world", actor.PrivateArea())
	}
	if actor.MovementBlocked() {
		t.Error("MovementBlocked() = true, want false for a fresh player")
	}
	actor.SetMovementBlocked(true)
	if !actor.MovementBlocked() {
		t.Error("MovementBlocked() = false after SetMovementBlocked(true)")
	}
	actor.SetMovementBlocked(false)
	dest := game.Location{X: 3200, Y: 3200, Z: 1}
	actor.MoveTo(dest)
	if p.Location() != dest {
		t.Errorf("Location after MoveTo = %v, want %v", p.Location(), dest)
	}
	if PlayerClimber(nil) != nil {
		t.Error("PlayerClimber(nil) != nil, want nil")
	}
}
