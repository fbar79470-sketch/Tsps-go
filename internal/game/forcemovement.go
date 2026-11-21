// Package game — forced movement (ForceMovement.ts,
// ForceMovementTask.ts, TickStopwatch.ts click delay): the glide a
// ditch jump, agility obstacle, or knockback moves a player along.
package game

// ForceMovement is the Go form of game/model/ForceMovement.ts: a glide
// from a start tile by a delta, with client cycle offsets and a facing
// direction, played with an animation. The ditch passes the DELTA as
// End in TS (new Location(jump.dx, jump.dy)); the task lands the player
// at Start + Delta. The field is named Delta here so the arithmetic at
// the use sites reads as what it is; the sync view math is
// PlayerSession.ts:766-786.
//
// Direction is the ditch's 0-3 cardinal scheme (0 north, 1 east, 2
// south, 3 west), NOT Direction.valueOf (0 north-west, 1 north, ...):
// resolveJump returns these values and the sync source maps them
// through [1024, 1536, 0, 512] (PlayerSession.ts:777).
type ForceMovement struct {
	// Start is the tile the glide begins on (getStart).
	Start Location
	// Delta is the displacement (getEnd in TS — the ditch passes the
	// jump delta, not an absolute tile).
	Delta Location
	// Speed is the start cycle offset (getSpeed; 0 for the ditch).
	Speed int
	// ReverseSpeed is the end cycle offset (getReverseSpeed; 70 for
	// the ditch).
	ReverseSpeed int
	// Direction is the facing during the glide, 0 north / 1 east /
	// 2 south / 3 west (getDirection).
	Direction int
	// Animation is the sequence played during the glide (getAnimation;
	// 6132 for the ditch), or 0 for none.
	Animation int
}

// ForceMovement returns the player's active forced movement, or nil
// (Player.getForceMovement, Player.ts:1203-1205).
func (p *Player) ForceMovement() *ForceMovement {
	if p == nil {
		return nil
	}
	return p.forceMovement
}

// SetForceMovement installs or clears the player's forced movement
// (Player.setForceMovement, Player.ts:1207-1211): setting a non-nil
// movement flags Flag.FORCED_MOVEMENT for this tick's sync. Clearing
// does not clear the flag — ResetUpdating does that at the end of the
// tick, as in TS.
func (p *Player) SetForceMovement(fm *ForceMovement) {
	if p == nil {
		return
	}
	p.forceMovement = fm
	if fm != nil {
		p.Flags = p.Flags.Set(FlagForcedMovement)
	}
}

// ClickDelayElapsed reports whether at least ms have elapsed since the
// player's click delay was last reset (TickStopwatch.elapsedTime,
// TickStopwatch.ts): elapsed = (currentCycle - tick) * 600, and a
// never-reset stopwatch reports every delay as elapsed (Infinity).
// The cycle comes from combatCycle(), the established shared world-cycle
// source for code without a cycle argument (combatant.go) — the ditch
// handler runs from a walk-to arrival that carries no cycle. This
// mirrors the lastItemPickupCycle precedent (player.go) with a
// millisecond threshold instead of a same-cycle check: 250 ms is under
// one 600 ms tick, so in practice the gate blocks only a reset on the
// current cycle.
func (p *Player) ClickDelayElapsed(ms int) bool {
	if p == nil || p.clickDelayCycle < 0 {
		return true
	}
	return (combatCycle()-p.clickDelayCycle)*600 >= int64(ms)
}

// ResetClickDelay restarts the player's click delay at the current
// cycle (TickStopwatch.reset, TickStopwatch.ts).
func (p *Player) ResetClickDelay() {
	if p == nil {
		return
	}
	p.clickDelayCycle = combatCycle()
}

// ForceMovementTask ports game/task/impl/ForceMovementTask.ts: a
// delayed task that lands the player at the end of a forced glide.
//
// Constructor side effects (ForceMovementTask.ts constructor) run in
// NewForceMovementTask, before scheduling, exactly as TS runs them in
// the constructor before TaskManager.submit: combat reset, movement
// queue reset, and installing the force movement on the player. The
// inner *Task is what the caller schedules on the world's TaskManager;
// Task() returns it.
type ForceMovementTask struct {
	player *Player
	start  Location
	delta  Location
	fm     *ForceMovement
	task   *Task
}

// NewForceMovementTask builds the task for player with the given tick
// delay and force movement, running the TS constructor side effects.
func NewForceMovementTask(player *Player, delay int, fm *ForceMovement) *ForceMovementTask {
	t := &ForceMovementTask{player: player, start: fm.Start, delta: fm.Delta, fm: fm}
	if c := player.GetCombat(); c != nil {
		c.Reset()
	}
	if q := player.MovementQueue(); q != nil {
		q.Reset(true)
	}
	player.SetForceMovement(fm)
	inner := NewTask(delay, func(cycle int64) bool { return t.execute() })
	inner.Key = player
	inner.Name = "ForceMovementTask"
	// TS stop() clears only the force movement instance this task owns,
	// so a task cancelled before execute() leaves no stale state
	// (ForceMovementTask.ts stop()). OnStop is the Go form of that
	// override; it fires from Task.Stop, which CancelTasks uses.
	inner.OnStop = func() { t.clearIfOwned() }
	t.task = inner
	return t
}

// Task returns the tick task to schedule on the world's TaskManager.
func (t *ForceMovementTask) Task() *Task { return t.task }

// Stop stops the task, clearing the owned force movement
// (ForceMovementTask.ts stop()).
func (t *ForceMovementTask) Stop() { t.task.Stop() }

// execute runs when the delay expires (ForceMovementTask.ts execute):
// if the player's force movement is no longer this task's instance,
// stop without moving; otherwise moveTo the landing tile (start +
// delta, same level — MovePlayerTo is the Go form of Mobile.moveTo),
// clear the force movement, and stop. Returning true stops the task.
func (t *ForceMovementTask) execute() bool {
	if t.player.ForceMovement() != t.fm {
		t.task.Stop()
		return true
	}
	dest := Location{X: t.start.X + t.delta.X, Y: t.start.Y + t.delta.Y, Z: t.player.Location().Z}
	MovePlayerTo(t.player, dest)
	t.player.SetForceMovement(nil)
	return true
}

// clearIfOwned clears the player's force movement only when it is
// this task's instance (ForceMovementTask.ts stop()).
func (t *ForceMovementTask) clearIfOwned() {
	if t.player.ForceMovement() == t.fm {
		t.player.SetForceMovement(nil)
	}
}
