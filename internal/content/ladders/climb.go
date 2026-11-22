package ladders

import (
	"tsps-go/internal/game"
)

// ClimbUpAnimation / ClimbDownAnimation are the climb animations.
// Ladders.plugin.js: CLIMB_UP = 828, CLIMB_DOWN = 827.
const (
	ClimbUpAnimation   = 828
	ClimbDownAnimation = 827
)

// NoAnimation is the animation id meaning "play none" (animate:
// false — stairs that are walked rather than climbed, and
// gangplank crossings). Ladders.plugin.js: climbUp/climbDown pass
// null when event.animate === false; crossOption passes null.
const NoAnimation = 0

// Scheduler schedules a tick task: the Go form of
// TaskManager.submit. *game.TaskManager satisfies it; content
// schedules from gameplay handlers, never during Start() before
// the engine runs (Phase 45's startup-order race fix —
// cmd/server/server.go emits startup hooks before starting the
// engine).
type Scheduler interface {
	Schedule(task *game.Task)
}

// ClimbActor is the narrow player surface the climb task needs.
// A *game.Player adapts through PlayerClimber; tests substitute
// fakes. Hitpoints is the live hitpoints (player: the Hitpoints
// skill's current level, as playerCombatant.Hitpoints reads it);
// PrivateArea is the player's current private area, or nil.
type ClimbActor interface {
	Location() game.Location
	Hitpoints() int
	PrivateArea() *game.PrivateArea
	PerformAnimation(id, delay int)
	MovementBlocked() bool
	SetMovementBlocked(blocked bool)
	ResetMovementQueue()
	MoveTo(loc game.Location)
}

// playerClimber adapts *game.Player to ClimbActor.
type playerClimber struct {
	player *game.Player
}

// PlayerClimber adapts a player to the climb task's actor surface.
// A nil player yields a nil actor.
func PlayerClimber(player *game.Player) ClimbActor {
	if player == nil {
		return nil
	}
	return &playerClimber{player: player}
}

func (c *playerClimber) Location() game.Location { return c.player.Location() }

func (c *playerClimber) Hitpoints() int {
	skills := c.player.Skills()
	if skills == nil {
		return 0
	}
	return skills.CurrentLevel(game.SkillHitpoints)
}

func (c *playerClimber) PrivateArea() *game.PrivateArea { return c.player.PrivateArea() }

func (c *playerClimber) PerformAnimation(id, delay int) { c.player.PerformAnimation(id, delay) }

func (c *playerClimber) MovementBlocked() bool {
	q := c.player.MovementQueue()
	return q != nil && q.IsMovementBlocked()
}

func (c *playerClimber) SetMovementBlocked(blocked bool) {
	if q := c.player.MovementQueue(); q != nil {
		q.SetBlockMovement(blocked)
	}
}

func (c *playerClimber) ResetMovementQueue() {
	if q := c.player.MovementQueue(); q != nil {
		q.Reset(true)
	}
}

// MoveTo is the Go form of Mobile.moveTo (Mobile.ts:194-204):
// game.MovePlayerTo (sailing.go:950) — notify the teleport
// listeners, reset the queue, set the location, mark the sync
// placement. It is the closest precedent to the climb's
// player.moveTo(target): player-death respawn
// (combat_death.go:337,424) and ForceMovementTask
// (forcemovement.go:146) both move the player with it, and
// Mobile.SetLocation alone (entity.go:205) skips the teleport
// listeners and sync placement the move needs.
func (c *playerClimber) MoveTo(loc game.Location) { game.MovePlayerTo(c.player, loc) }

// StartClimb starts a climb: the Go port of climb() in
// Ladders.plugin.js:101-131.
//
//   - Refuses when movement is blocked (or the scheduler/actor is
//     nil): returns nil, false, and schedules nothing.
//   - Blocks movement and resets the queue, then submits a 1-cycle
//     task. On the first execution the player is still on the
//     start tile, alive (HP > 0), and in the same private area,
//     the animation plays first (tick 1); on the next execution
//     the player moves to dest and onArrival runs right after the
//     move (tick 2). A refused or cancelled climb never calls
//     onArrival. stop() unblocks movement (the task's OnStop,
//     which fires when the task is stopped from outside, and the
//     explicit unblock on both terminal paths — Action returning
//     done=true stops the task without calling stop(), as
//     Task.execute documents).
//
// The returned task is the scheduled task (nil when refused); a
// caller may stop it to cancel the climb.
//
// Deviation from upstream, deliberate: TS constructs the task as
// new Task(1, player, !movement.didMovePreviousCycle()) —
// immediate when the player did not move the previous cycle, so
// the animation can play on the submit tick. The climb entry
// carries no cycle to evaluate DidMovePreviousCycle against, so
// Go always uses a delay-1 non-immediate task: driven by
// successive TaskManager.Process calls, the animation lands on
// tick 1 and the move on tick 2 (the order the plan pins), and on
// the live tick order the animation lands on the click tick and
// the move on the next.
func StartClimb(sched Scheduler, actor ClimbActor, dest game.Location, animation int, onArrival func()) (*game.Task, bool) {
	if sched == nil || actor == nil {
		return nil, false
	}
	if actor.MovementBlocked() {
		return nil, false
	}
	start := actor.Location()
	area := actor.PrivateArea()
	actor.SetMovementBlocked(true)
	actor.ResetMovementQueue()
	animated := false
	unblocked := false
	unblock := func() {
		if !unblocked {
			unblocked = true
			actor.SetMovementBlocked(false)
		}
	}
	task := game.NewTask(1, func(cycle int64) bool {
		if actor.Location() != start || actor.Hitpoints() <= 0 || actor.PrivateArea() != area {
			unblock()
			return true
		}
		if animation != NoAnimation && !animated {
			actor.PerformAnimation(animation, 0)
			animated = true
			return false
		}
		actor.MoveTo(dest)
		unblock()
		if onArrival != nil {
			onArrival()
		}
		return true
	})
	task.Name = "LadderClimb"
	task.Key = actor
	task.OnStop = unblock
	sched.Schedule(task)
	return task, true
}
