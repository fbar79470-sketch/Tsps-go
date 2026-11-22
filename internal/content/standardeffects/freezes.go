package standardeffects

import (
	"tsps-go/internal/game"
)

// Freeze kinds, as carried by the "combat:frozen" payload
// (CombatFactory.ts:1665): "ice" for ice spells, "bind" for
// Bind/Snare/Entangle.
const (
	freezeKindIce  = "ice"
	freezeKindBind = "bind"
)

// freezePvPRange is the distance in tiles within which a
// player-applied freeze holds (Freezes.plugin.js:7,
// PVP_RANGE 10 — X and Y only, so a ladder keeps it).
const freezePvPRange = 10

// freezeWarningTicks is the freeze time left when the warning
// goes out (Freezes.plugin.js:11, WARNING_TICKS 3).
const freezeWarningTicks = 3

// freezeKindMessages is the per-kind aftermath text
// (Freezes.plugin.js:13-24, KINDS): ice spells say so in red,
// and Ice and Bind-type freezes warn the frozen player shortly
// before they end.
var freezeKindMessages = map[string]struct {
	applied string
	warning string
}{
	freezeKindIce: {
		applied: "<col=ef1020>You have been frozen!</col>",
		warning: "<col=ef1020>The freeze cast upon you is beginning to thaw.</col>",
	},
	freezeKindBind: {
		applied: "",
		warning: "<col=ef1020>The holding effect is beginning to fade.</col>",
	},
}

// freezeApplied is the "combat:frozen" handler
// (Freezes.plugin.js:70-81, frozen): the aftermath of a
// landed freeze. Only player targets get anything. An ice
// freeze says so in red; a freeze one player puts on another
// is watched while the two stay within 10 tiles, and either
// kind warns shortly before it ends. With no holder and no
// warning there is nothing to watch.
//
// DEVIATION: the red applied message goes out through
// SendMessage, not the typed SPAM channel TS uses
// (sendTypedMessage(ChatType.SPAM, ...)): the spam chat-type
// constant is unexported in internal/game and no content
// package sends typed messages. The text is identical.
func (s *standardEffects) freezeApplied(payload any) {
	frozen, ok := payload.(*game.CombatFrozenPayload)
	if !ok || frozen == nil || frozen.Target == nil {
		return
	}
	target := frozen.Target.AsPlayer()
	if target == nil {
		return
	}
	messages, ok := freezeKindMessages[frozen.Kind]
	if !ok {
		return
	}
	if messages.applied != "" {
		target.SendMessage(messages.applied)
	}
	holder := freezeHolder(target, frozen.Source)
	if holder == nil && messages.warning == "" {
		return
	}
	s.watchFreeze(target, holder, messages.warning)
}

// freezeHolder is the held-by-a-player check
// (Freezes.plugin.js:33-35, heldByPlayer): the freeze holder
// is a player source that is not the target itself.
func freezeHolder(target *game.Player, source *game.Mobile) *game.Player {
	if target == nil || source == nil {
		return nil
	}
	holder := source.AsPlayer()
	if holder == nil || holder == target {
		return nil
	}
	return holder
}

// freezeWatchState is the per-watch mutable state: the freeze
// time left the last time the watch ran, and whether the
// warning has gone out.
type freezeWatchState struct {
	prevLeft int
	warned   bool
}

// watchFreeze submits the per-tick freeze watch
// (Freezes.plugin.js:20-30, FreezeWatch: a Task of delay 1
// over the target). Without a TaskManager there is nothing to
// schedule on — the freeze still holds through its timer; only
// the warning and the 10-tile release are lost.
func (s *standardEffects) watchFreeze(target, holder *game.Player, warning string) {
	tm := s.taskManager()
	if tm == nil || target == nil {
		return
	}
	state := &freezeWatchState{prevLeft: target.GetTimers().Left(game.TimerKeyFreeze)}
	task := game.NewTask(1, func(cycle int64) bool {
		return !s.watchFreezeTick(target, holder, warning, state)
	})
	task.Name = "FreezeWatch"
	task.Key = target.Index
	tm.Schedule(task)
}

// watchFreezeTick is one tick of a watched freeze
// (Freezes.plugin.js:44-58, watch): false once there is
// nothing left to watch. The target must still be registered
// and its freeze timer still running; a freeze one player put
// on another only holds while the two stay within 10 tiles;
// the warning goes out at three ticks left.
//
// The TS identity check (the watched timer object must still
// be the target's FREEZE timer) has no Go form — Go timers
// are values, not identities — so a freeze time left that
// GREW since the last tick stands in for it: a re-freeze
// replaces the watched freeze, and the old watch stops.
func (s *standardEffects) watchFreezeTick(target, holder *game.Player, warning string, state *freezeWatchState) bool {
	if target == nil || !target.IsRegistered() {
		return false
	}
	left := target.GetTimers().Left(game.TimerKeyFreeze)
	if left <= 0 {
		return false
	}
	if state != nil && left > state.prevLeft {
		return false
	}
	if state != nil {
		state.prevLeft = left
	}
	if holder != nil && holder.IsRegistered() && !withinFreezeRange(target, holder) {
		target.GetTimers().Cancel(game.TimerKeyFreeze)
		return false
	}
	if warning != "" && state != nil && !state.warned && left == freezeWarningTicks {
		target.SendMessage(warning)
		state.warned = true
	}
	return true
}

// withinFreezeRange reports whether the target and the holder
// are within 10 tiles of each other, X and Y only
// (Freezes.plugin.js:37-42, withinRange: Chebyshev distance,
// so a ladder keeps the freeze).
func withinFreezeRange(target, holder *game.Player) bool {
	if target == nil || holder == nil {
		return false
	}
	a := target.Location()
	b := holder.Location()
	dx := a.X - b.X
	if dx < 0 {
		dx = -dx
	}
	dy := a.Y - b.Y
	if dy < 0 {
		dy = -dy
	}
	if dx > dy {
		return dx <= freezePvPRange
	}
	return dy <= freezePvPRange
}
