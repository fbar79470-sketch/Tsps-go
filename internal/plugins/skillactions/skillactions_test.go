package skillactions

import (
	"testing"

	"tsps-go/internal/game"
)

// fakeSession is a minimal Session: the pause flag plus a marker
// distinguishing instances for the identity checks.
type fakeSession struct {
	paused bool
	name   string
}

func (s *fakeSession) SetLevelUpPaused(paused bool) { s.paused = paused }

func TestPauseForLevelUpNoSession(t *testing.T) {
	player := game.NewPlayer("NoSession", game.Location{})
	stopped := false
	event := &LevelUpPopupOpenedPayload{Player: player, Skill: game.SkillMining, Pause: func(any, func(), func()) {
		t.Fatal("Pause must not be called with no session")
	}}
	PauseForLevelUp(event, map[*game.Player]Session{}, func(*game.Player) { stopped = true }, nil)
	if stopped {
		t.Fatal("stop must not run with no session")
	}
	// A nil event and a nil-valued session are also no-ops.
	PauseForLevelUp(nil, map[*game.Player]Session{}, func(*game.Player) { stopped = true }, nil)
	PauseForLevelUp(event, map[*game.Player]Session{player: nil}, func(*game.Player) { stopped = true }, nil)
	if stopped {
		t.Fatal("stop must not run for a nil event or nil session")
	}
}

func TestPauseForLevelUpNoPauseStops(t *testing.T) {
	player := game.NewPlayer("NoPause", game.Location{})
	session := &fakeSession{name: "a"}
	stopped := false
	PauseForLevelUp(
		&LevelUpPopupOpenedPayload{Player: player, Skill: game.SkillMining},
		map[*game.Player]Session{player: session},
		func(p *game.Player) {
			if p != player {
				t.Fatalf("stop got %p, want player %p", p, player)
			}
			stopped = true
		}, nil)
	if !stopped {
		t.Fatal("no Pause must stop the player")
	}
	if session.paused {
		t.Fatal("session must not be marked paused when stopped")
	}
}

func TestPauseForLevelUpPauseResume(t *testing.T) {
	player := game.NewPlayer("Resume", game.Location{})
	session := &fakeSession{name: "a"}
	sessions := map[*game.Player]Session{player: session}
	var resume, cancel func()
	var action any
	PauseForLevelUp(
		&LevelUpPopupOpenedPayload{Player: player, Skill: game.SkillMining, Pause: func(a any, r, c func()) {
			action, resume, cancel = a, r, c
		}},
		sessions,
		func(*game.Player) { t.Fatal("stop must not run on pause/resume") },
		func(resumed Session) {
			if resumed != Session(session) {
				t.Fatalf("onResume got %v, want session", resumed)
			}
		})
	if !session.paused {
		t.Fatal("session must be marked paused")
	}
	if action != any(session) {
		t.Fatalf("Pause action = %v, want the session", action)
	}
	if resume == nil || cancel == nil {
		t.Fatal("resume and cancel must be registered")
	}
	resume()
	if session.paused {
		t.Fatal("resume must clear the pause flag")
	}
}

func TestPauseForLevelUpCancelStops(t *testing.T) {
	player := game.NewPlayer("Cancel", game.Location{})
	session := &fakeSession{name: "a"}
	sessions := map[*game.Player]Session{player: session}
	var cancel func()
	PauseForLevelUp(
		&LevelUpPopupOpenedPayload{Player: player, Skill: game.SkillMining, Pause: func(_ any, _, c func()) { cancel = c }},
		sessions,
		func(p *game.Player) {
			if p != player {
				t.Fatalf("stop got %p, want player", p)
			}
		}, nil)
	if cancel == nil {
		t.Fatal("cancel must be registered")
	}
	cancel() // Must stop without panicking; the stop above asserts the player.
}

func TestPauseForLevelUpReplacedSessionNoOps(t *testing.T) {
	player := game.NewPlayer("Replaced", game.Location{})
	original := &fakeSession{name: "original"}
	replacement := &fakeSession{name: "replacement"}
	sessions := map[*game.Player]Session{player: original}
	var resume, cancel func()
	resumed := false
	stopped := false
	PauseForLevelUp(
		&LevelUpPopupOpenedPayload{Player: player, Skill: game.SkillMining, Pause: func(_ any, r, c func()) { resume, cancel = r, c }},
		sessions,
		func(*game.Player) { stopped = true },
		func(Session) { resumed = true })
	// The session is replaced before the pop-up closes.
	sessions[player] = replacement
	resume()
	if !original.paused {
		t.Fatal("resume must no-op for a replaced session (flag stays set)")
	}
	if resumed {
		t.Fatal("onResume must not run for a replaced session")
	}
	cancel()
	if stopped {
		t.Fatal("cancel must no-op for a replaced session")
	}
	if replacement.paused {
		t.Fatal("replacement session must be untouched")
	}
}

func TestPauseForLevelUpNilStopAndOnResumeSafe(t *testing.T) {
	player := game.NewPlayer("NilSafe", game.Location{})
	session := &fakeSession{name: "a"}
	sessions := map[*game.Player]Session{player: session}
	var resume, cancel func()
	// Nil stop with no Pause: no-op, no panic.
	PauseForLevelUp(&LevelUpPopupOpenedPayload{Player: player}, sessions, nil, nil)
	// Nil onResume on resume, nil stop on cancel: no-ops, no panic.
	PauseForLevelUp(
		&LevelUpPopupOpenedPayload{Player: player, Pause: func(_ any, r, c func()) { resume, cancel = r, c }},
		sessions, nil, nil)
	resume()
	if session.paused {
		t.Fatal("resume must clear the flag even with nil onResume")
	}
	cancel()
}

func TestActionIntervalBaseAndFloor(t *testing.T) {
	player := game.NewPlayer("Interval", game.Location{})
	if got := ActionInterval(nil, player, game.SkillMining, "mine", 4, 0); got != 4 {
		t.Fatalf("nil emitter: got %d, want 4", got)
	}
	if got := ActionInterval(nil, player, game.SkillMining, "mine", 2, 5); got != 1 {
		t.Fatalf("saving past zero: got %d, want 1", got)
	}
	if got := ActionInterval(nil, player, game.SkillMining, "mine", 0, 0); got != 1 {
		t.Fatalf("zero ticks: got %d, want 1", got)
	}
	if got := ActionInterval(nil, player, game.SkillMining, "mine", 1, 0); got != 1 {
		t.Fatalf("one tick: got %d, want 1", got)
	}
}

func TestActionIntervalContentLowersTicks(t *testing.T) {
	player := game.NewPlayer("Lower", game.Location{})
	var seenName string
	var seen *ActionIntervalPayload
	emit := func(name string, payload any) {
		seenName = name
		seen = payload.(*ActionIntervalPayload)
		seen.Ticks = 2 // A league relic's 2-tick pace.
	}
	if got := ActionInterval(emit, player, game.SkillFishing, "fish", 5, 0); got != 2 {
		t.Fatalf("content lowering: got %d, want 2", got)
	}
	if seenName != ActionIntervalEventName {
		t.Fatalf("event name = %q, want %q", seenName, ActionIntervalEventName)
	}
	if seen == nil || seen.Player != player || seen.Skill != game.SkillFishing || seen.Kind != "fish" {
		t.Fatalf("payload = %+v, want player/skill/kind carried", seen)
	}
}

func TestActionIntervalSavingStacksAfterContent(t *testing.T) {
	player := game.NewPlayer("Stack", game.Location{})
	// Content answers 3, tool saving 1: 3 - 1 = 2.
	lowering := func(_ string, payload any) { payload.(*ActionIntervalPayload).Ticks = 3 }
	if got := ActionInterval(lowering, player, game.SkillFletching, "fletch", 5, 1); got != 2 {
		t.Fatalf("content 3 minus saving 1: got %d, want 2", got)
	}
	// Content answers 2, tool saving 1: floor still 1.
	if got := ActionInterval(lowering, player, game.SkillFletching, "fletch", 5, 5); got != 1 {
		t.Fatalf("content 3 minus saving 5: got %d, want 1", got)
	}
}

func TestActionIntervalContentRaisingTicksApplies(t *testing.T) {
	// TS reads request.ticks back verbatim after the emit; a
	// handler raising ticks raises the interval the same way.
	player := game.NewPlayer("Raise", game.Location{})
	emit := func(_ string, payload any) { payload.(*ActionIntervalPayload).Ticks = 9 }
	if got := ActionInterval(emit, player, game.SkillMining, "mine", 4, 1); got != 8 {
		t.Fatalf("content 9 minus saving 1: got %d, want 8", got)
	}
}
