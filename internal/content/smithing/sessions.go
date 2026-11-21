// Sessions.Smithing.js: one 1-tick task driving every
// player's make-X loop. Each cycle starts firstDelay ticks
// after the session (then every interval ticks, read as each
// cycle opens): begin runs the cycle's opening (a message, the
// animation), and step makes the item finishDelay ticks later.
// Either returning false ends the session, as do remaining
// running out and the player moving, dying or logging out.
package smithing

import (
	"tsps-go/internal/game"
	"tsps-go/internal/plugins/skillactions"
)

// smithSession is one player's active make-X loop
// (Sessions.Smithing.js SESSIONS entry).
type smithSession struct {
	kind        string // "smelt", "smith", "cannonballs", "spears", "timeline"
	skill       int
	remaining   int
	interval    func(*game.Player) int
	firstDelay  int
	finishDelay int
	begin       func(*game.Player) bool
	step        func(*game.Player) bool
	onStop      func(*game.Player)

	startLocation game.Location
	nextCycle     int64
	finishAt      int64
	hasFinish     bool
	levelUpPaused bool
}

// SetLevelUpPaused implements skillactions.Session.
func (s *smithSession) SetLevelUpPaused(paused bool) { s.levelUpPaused = paused }

// sessionOptions is the start() argument
// (Sessions.Smithing.js start).
type sessionOptions struct {
	kind        string
	skill       int
	remaining   int
	interval    func(*game.Player) int
	firstDelay  int
	finishDelay int
	begin       func(*game.Player) bool
	step        func(*game.Player) bool
	onStop      func(*game.Player)
}

// constInterval is a fixed-tick interval.
func constInterval(ticks int) func(*game.Player) int {
	return func(*game.Player) int { return ticks }
}

// startSession begins a make-X session, stopping any existing
// one first (Sessions.Smithing.js start). The task's next run
// is the tick the session started on (offset 0): nextCycle is
// tick+1+firstDelay. Movement before admission (such as
// approaching a furnace) belongs to the old action, so the
// start location is snapshotted here.
func (s *smithing) startSession(player *game.Player, opts sessionOptions) *smithSession {
	if player == nil {
		return nil
	}
	s.stopSession(player, false)
	interval := opts.interval
	if interval == nil {
		interval = constInterval(1)
	}
	sess := &smithSession{
		kind:          opts.kind,
		skill:         opts.skill,
		remaining:     opts.remaining,
		interval:      interval,
		firstDelay:    opts.firstDelay,
		finishDelay:   opts.finishDelay,
		begin:         opts.begin,
		step:          opts.step,
		onStop:        opts.onStop,
		startLocation: player.Location(),
		nextCycle:     s.tick + 1 + int64(opts.firstDelay),
	}
	s.sessions[player] = sess
	s.ensureTask()
	return sess
}

// stopSession ends a player's session (Sessions.Smithing.js
// stop), resetting the animation unless told not to. A player
// with no session is a no-op.
func (s *smithing) stopSession(player *game.Player, resetAnimation bool) {
	if player == nil {
		return
	}
	sess, ok := s.sessions[player]
	if !ok {
		return
	}
	delete(s.sessions, player)
	if sess.onStop != nil {
		sess.onStop(player)
	}
	if resetAnimation {
		player.PerformAnimation(game.ResetAnimationID, 0)
	}
}

// sessionTypeOf is the type of a player's session, or "" when
// there is none (Sessions.Smithing.js typeOf).
func (s *smithing) sessionTypeOf(player *game.Player) string {
	if sess, ok := s.sessions[player]; ok && sess != nil {
		return sess.kind
	}
	return ""
}

// pauseSessions holds sessions through a level-up pop-up
// (Sessions.Smithing.js pause, via SkillActions). The emitter
// does not exist in Go yet, so this handler never fires
// (gap 26b); it is registered anyway, exactly as TS does.
func (s *smithing) pauseSessions(payload any) {
	event, ok := payload.(*skillactions.LevelUpPopupOpenedPayload)
	if !ok || event == nil {
		return
	}
	sessions := make(map[*game.Player]skillactions.Session, len(s.sessions))
	for player, sess := range s.sessions {
		sessions[player] = sess
	}
	skillactions.PauseForLevelUp(event, sessions, func(player *game.Player) {
		s.stopSession(player, false)
	}, nil)
}

// timelineStep is one step of a one-off timed action
// (Sessions.Smithing.js timeline): run at its tick offset
// from now (offset 0 is this tick); walking off cancels the
// rest.
type timelineStep struct {
	at  int
	run func(*game.Player) bool
}

// timeline runs a one-off timed action (godsword or shield
// work in TS; the seam is ported for item 3): steps run at
// their tick offsets, earliest first.
func (s *smithing) timeline(player *game.Player, kind string, steps []timelineStep) {
	ordered := append([]timelineStep(nil), steps...)
	for i := 1; i < len(ordered); i++ {
		for j := i; j > 0 && ordered[j].at < ordered[j-1].at; j-- {
			ordered[j], ordered[j-1] = ordered[j-1], ordered[j]
		}
	}
	elapsed := -1
	var sess *smithSession
	sess = s.startSession(player, sessionOptions{
		kind:      kind,
		skill:     game.SkillSmithing,
		remaining: 1,
		interval:  constInterval(1),
		begin:     func(*game.Player) bool { return true },
		step: func(actor *game.Player) bool {
			elapsed++
			for len(ordered) > 0 && ordered[0].at <= elapsed {
				step := ordered[0]
				ordered = ordered[1:]
				if step.run != nil && !step.run(actor) {
					return false
				}
			}
			// Steps remain: keep the session alive for the
			// next cycle (TS: session.remaining++).
			if len(ordered) > 0 && sess != nil {
				sess.remaining++
			}
			return true
		},
	})
}

// registerEveryTick registers per-tick work other than the
// session advance (Sessions.Smithing.js everyTick): closing
// menus a player walked away from.
func (s *smithing) registerEveryTick(fn func()) {
	if fn != nil {
		s.everyTick = append(s.everyTick, fn)
	}
}

// advanceSession runs one session's cycle-open / finish
// (Sessions.Smithing.js advance). False ends the session.
func (s *smithing) advanceSession(player *game.Player, sess *smithSession, tick int64) bool {
	if !sess.hasFinish && tick >= sess.nextCycle {
		// Content may work faster ("skills:action-interval",
		// by the session's type).
		pace := sess.interval(player)
		sess.nextCycle = tick + int64(skillactions.ActionInterval(s.emitEvent, player, sess.skill, sess.kind, pace, 0))
		if sess.begin != nil && !sess.begin(player) {
			return false
		}
		sess.finishAt = tick + int64(sess.finishDelay)
		sess.hasFinish = true
	}
	if !sess.hasFinish || tick < sess.finishAt {
		return true
	}
	sess.hasFinish = false
	// Content may make several items in the one cycle
	// ("skills:actions-per-cycle").
	actions := s.actionsPerCycle(player, sess.skill, sess.kind)
	for done := 0; done < actions; done++ {
		if sess.step == nil || !sess.step(player) {
			if done == 0 {
				return false
			}
			s.stopSession(player, false)
			return true
		}
		sess.remaining--
		if sess.remaining <= 0 {
			s.stopSession(player, false)
			return true
		}
	}
	return true
}

// processSmithingTick runs one tick of the global SmithingTask
// (Sessions.Smithing.js process). Sessions started during the
// tick's packets get their opening on the next tick.
func (s *smithing) processSmithingTick(cycle int64) {
	for _, fn := range s.everyTick {
		fn()
	}
	for player, sess := range s.sessions {
		if player == nil || !player.IsRegistered() || player.Skills().CurrentLevel(game.SkillHitpoints) <= 0 {
			s.stopSession(player, false)
			continue
		}
		// A one-step route can drain between task ticks, so
		// the queue alone misses the walk.
		movement := player.MovementQueue()
		if (movement != nil && (movement.Size() > 0 || movement.IsMoving())) || player.ForceMovement() != nil ||
			!player.Location().Equals(sess.startLocation) {
			s.stopSession(player, true)
			continue
		}
		if sess.levelUpPaused {
			continue
		}
		if !s.advanceSession(player, sess, cycle) {
			s.stopSession(player, true)
		}
	}
}

// taskManager resolves the world's TaskManager lazily: the hub
// installs it after plugin Load (nil until installed, per the
// hub contract), so a Register-time capture reads nil in
// production (the mining taskManager precedent).
func (s *smithing) taskManager() *game.TaskManager {
	if s.api != nil {
		if tm := s.api.GetTaskManager(); tm != nil {
			return tm
		}
	}
	return s.tasks
}

// ensureTask schedules the 1-tick global SmithingTask once, on
// the lazily-resolved TaskManager. Idempotent. A nil
// TaskManager schedules nothing and retries on the next call
// (the mining ensureTask shape).
func (s *smithing) ensureTask() {
	if s.taskScheduled {
		return
	}
	tm := s.taskManager()
	if tm == nil {
		return
	}
	task := game.NewTask(1, func(cycle int64) bool {
		s.tick++
		s.processSmithingTick(s.tick)
		return false
	})
	task.Name = "SmithingTask"
	tm.Schedule(task)
	s.taskScheduled = true
}

// emitEvent emits a custom event through the captured emit
// func; a nil emit (before Register) drops the emit, which is
// the no-hooks default for every query (the mining precedent).
func (s *smithing) emitEvent(name string, payload any) {
	if s == nil || s.emit == nil {
		return
	}
	s.emit(name, payload)
}

// ---- test seam ----

// setTick sets the session clock (tests drive ticks directly,
// the mining processMiningTick precedent).
func (s *smithing) setTick(tick int64) { s.tick = tick }
