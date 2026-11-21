package plugins

import (
	"math"
	"strings"
	"testing"

	"tsps-go/internal/game"
)

// Skills-family tests: the registration methods, emits, and gate
// contract in skills.go, grounded in PluginManager.ts
// (emitExperienceGained 1101-1106, emitPlayerLevelUp 1108-1116,
// emitCanGainExperience 1665-1674, registration guards 3325-3346
// and 3832-3834, getTaskManager 4666), PluginTypes.ts (event
// shapes 94-99, 102-106, 344-354; member declarations 1026, 1028,
// 1137, 1377), and SkillManager.ts (gate 138-141, level-up emit
// 185-191, gained emit 194). Fixtures only: newTestHub and
// testPlayer come from api_test.go / hooks_test.go.

func TestOnPlayerLevelUpRunAllOrdering(t *testing.T) {
	h, _ := newTestHub()
	var order []string
	var seen *PlayerLevelUpEvent
	newApi(h, "First").OnPlayerLevelUp(func(ev *PlayerLevelUpEvent) {
		order = append(order, "First")
		seen = ev
	})
	newApi(h, "Second").OnPlayerLevelUp(func(*PlayerLevelUpEvent) { order = append(order, "Second") })
	p := testPlayer("alice")
	h.EmitPlayerLevelUp(&PlayerLevelUpEvent{Player: p, Skill: 3, OldLevel: 9, NewLevel: 11})
	want := []string{"First", "Second"}
	if len(order) != len(want) || order[0] != want[0] || order[1] != want[1] {
		t.Fatalf("order = %v, want %v", order, want)
	}
	if seen == nil || seen.Player != p || seen.Skill != 3 || seen.OldLevel != 9 || seen.NewLevel != 11 {
		t.Fatalf("seen event = %+v, want player/skill 3/levels 9->11", seen)
	}
}

func TestOnPlayerLevelUpWrapperSkipsPlayerless(t *testing.T) {
	h, _ := newTestHub()
	calls := 0
	newApi(h, "Watcher").OnPlayerLevelUp(func(*PlayerLevelUpEvent) { calls++ })
	// A nil event and a player-less event run no handler (the
	// registration wrapper, PluginManager.ts:3332-3342).
	h.EmitPlayerLevelUp(nil)
	h.EmitPlayerLevelUp(&PlayerLevelUpEvent{Skill: 0, OldLevel: 1, NewLevel: 2})
	if calls != 0 {
		t.Fatalf("calls = %d, want 0", calls)
	}
	// Skill 0 (Attack) is a valid skill: the TS !event.skill check
	// degenerates in Go, and the handler runs.
	h.EmitPlayerLevelUp(&PlayerLevelUpEvent{Player: testPlayer("alice"), Skill: 0, OldLevel: 1, NewLevel: 2})
	if calls != 1 {
		t.Fatalf("calls = %d, want 1", calls)
	}
}

func TestOnExperienceGainedRunAllNoWrapper(t *testing.T) {
	h, _ := newTestHub()
	var order []string
	newApi(h, "First").OnExperienceGained(func(*ExperienceGainedEvent) { order = append(order, "First") })
	newApi(h, "Second").OnExperienceGained(func(*ExperienceGainedEvent) { order = append(order, "Second") })
	p := testPlayer("alice")
	h.EmitExperienceGained(&ExperienceGainedEvent{Player: p, Skill: 0, Experience: 130})
	if len(order) != 2 || order[0] != "First" || order[1] != "Second" {
		t.Fatalf("order = %v, want [First Second]", order)
	}
	// No registration wrapper (PluginManager.ts:3325-3327): a
	// player-less event DOES reach the handler, unlike level-up.
	// The handler must not dereference the player.
	seenNil := false
	h2, _ := newTestHub()
	newApi(h2, "Watcher").OnExperienceGained(func(ev *ExperienceGainedEvent) { seenNil = ev.Player == nil })
	h2.EmitExperienceGained(&ExperienceGainedEvent{Skill: 0, Experience: 10})
	if !seenNil {
		t.Fatal("player-less event did not reach the handler; want the no-wrapper behavior")
	}
	// A nil event runs no hooks (the Go-only emit guard).
	h2.EmitExperienceGained(nil)
}

func TestEmitCanGainExperienceNoHooks(t *testing.T) {
	h, _ := newTestHub()
	allow, multiplier := h.EmitCanGainExperience(testPlayer("alice"), 0, 100)
	if allow != nil || multiplier != 1 {
		t.Fatalf("(allow, multiplier) = (%v, %v), want (nil, 1)", allow, multiplier)
	}
	// A nil Hub yields the same default.
	var nilHub *Hub
	allow, multiplier = nilHub.EmitCanGainExperience(testPlayer("alice"), 0, 100)
	if allow != nil || multiplier != 1 {
		t.Fatalf("nil-hub (allow, multiplier) = (%v, %v), want (nil, 1)", allow, multiplier)
	}
}

func TestEmitCanGainExperienceStacking(t *testing.T) {
	h, _ := newTestHub()
	// Hooks multiply what they find, so several stack
	// (PluginTypes.ts:349-352): 1 * 2 * 3 = 6. The second hook
	// sees the first hook's multiplier: one shared mutable event.
	newApi(h, "Double").OnCanGainExperience(func(ev *CanGainExperienceEvent) { ev.Multiplier *= 2 })
	newApi(h, "Triple").OnCanGainExperience(func(ev *CanGainExperienceEvent) {
		if ev.Multiplier != 2 {
			t.Errorf("second hook saw multiplier %v, want 2 (shared event)", ev.Multiplier)
		}
		ev.Multiplier *= 3
	})
	allow, multiplier := h.EmitCanGainExperience(testPlayer("alice"), 0, 100)
	if allow != nil || multiplier != 6 {
		t.Fatalf("(allow, multiplier) = (%v, %v), want (nil, 6)", allow, multiplier)
	}
}

func TestEmitCanGainExperienceDenyImmediate(t *testing.T) {
	h, _ := newTestHub()
	calls := 0
	newApi(h, "Booster").OnCanGainExperience(func(ev *CanGainExperienceEvent) {
		calls++
		ev.Multiplier *= 5
	})
	newApi(h, "Denier").OnCanGainExperience(func(ev *CanGainExperienceEvent) {
		calls++
		ev.Allow = BoolPtr(false)
	})
	newApi(h, "Late").OnCanGainExperience(func(ev *CanGainExperienceEvent) {
		calls++
		ev.Multiplier *= 100
	})
	allow, multiplier := h.EmitCanGainExperience(testPlayer("alice"), 0, 100)
	if allow == nil || *allow != false {
		t.Fatalf("allow = %v, want false", allow)
	}
	// A refusal wins and the multiplier resets to 1 — the
	// booster's *5 does not survive the deny.
	if multiplier != 1 {
		t.Fatalf("multiplier = %v, want 1 (reset on deny)", multiplier)
	}
	if calls != 2 {
		t.Fatalf("calls = %d, want 2 (third hook never runs)", calls)
	}
}

func TestEmitCanGainExperienceAllow(t *testing.T) {
	h, _ := newTestHub()
	newApi(h, "Allower").OnCanGainExperience(func(ev *CanGainExperienceEvent) {
		ev.Allow = BoolPtr(true)
	})
	// Silence after an allow cannot un-allow it: the answer is
	// not reset between hooks.
	newApi(h, "Silent").OnCanGainExperience(func(*CanGainExperienceEvent) {})
	allow, multiplier := h.EmitCanGainExperience(testPlayer("alice"), 0, 100)
	if allow == nil || *allow != true || multiplier != 1 {
		t.Fatalf("(allow, multiplier) = (%v, %v), want (true, 1)", allow, multiplier)
	}
	// The returned pointer is fresh: mutating the hook's pointer
	// afterwards cannot rewrite the answer.
	h2, _ := newTestHub()
	shared := BoolPtr(true)
	newApi(h2, "Reuser").OnCanGainExperience(func(ev *CanGainExperienceEvent) { ev.Allow = shared })
	allow, _ = h2.EmitCanGainExperience(testPlayer("alice"), 0, 100)
	*shared = false
	if allow == nil || *allow != true {
		t.Fatalf("allow after pointer reuse = %v, want true (fresh pointer)", allow)
	}
}

func TestEmitCanGainExperienceSanitize(t *testing.T) {
	for _, tc := range []struct {
		name string
		set  float64
		want float64
	}{
		{"NaN", math.NaN(), 1},
		{"positive infinity", math.Inf(1), 1},
		{"negative infinity", math.Inf(-1), 1},
		{"zero", 0, 1},
		{"negative", -2, 1},
		{"fraction", 0.5, 0.5},
		{"large", 100, 100},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h, _ := newTestHub()
			newApi(h, "Setter").OnCanGainExperience(func(ev *CanGainExperienceEvent) { ev.Multiplier = tc.set })
			_, multiplier := h.EmitCanGainExperience(testPlayer("alice"), 0, 100)
			if multiplier != tc.want {
				t.Fatalf("multiplier = %v, want %v", multiplier, tc.want)
			}
		})
	}
}

func TestEmitCanGainExperiencePanickingHookIsolated(t *testing.T) {
	h, buf := newTestHub()
	newApi(h, "Panicker").OnCanGainExperience(func(*CanGainExperienceEvent) {
		panic("boom")
	})
	newApi(h, "Booster").OnCanGainExperience(func(ev *CanGainExperienceEvent) { ev.Multiplier *= 4 })
	allow, multiplier := h.EmitCanGainExperience(testPlayer("alice"), 0, 100)
	if allow != nil || multiplier != 4 {
		t.Fatalf("(allow, multiplier) = (%v, %v), want (nil, 4)", allow, multiplier)
	}
	if !strings.Contains(buf.String(), "can_gain_experience hook failed (Panicker)") {
		t.Fatalf("log = %q, want the panic-isolation line", buf.String())
	}
}

func TestEmitSkillsPanickingHookIsolated(t *testing.T) {
	h, buf := newTestHub()
	newApi(h, "Panicker").OnPlayerLevelUp(func(*PlayerLevelUpEvent) { panic("boom") })
	ran := false
	newApi(h, "Watcher").OnPlayerLevelUp(func(*PlayerLevelUpEvent) { ran = true })
	h.EmitPlayerLevelUp(&PlayerLevelUpEvent{Player: testPlayer("alice"), Skill: 0, OldLevel: 1, NewLevel: 2})
	if !ran {
		t.Fatal("second level-up hook did not run after the panicking hook")
	}
	if !strings.Contains(buf.String(), "player_level_up hook failed (Panicker)") {
		t.Fatalf("log = %q, want the panic-isolation line", buf.String())
	}

	h2, buf2 := newTestHub()
	newApi(h2, "Panicker").OnExperienceGained(func(*ExperienceGainedEvent) { panic("boom") })
	ran = false
	newApi(h2, "Watcher").OnExperienceGained(func(*ExperienceGainedEvent) { ran = true })
	h2.EmitExperienceGained(&ExperienceGainedEvent{Player: testPlayer("alice"), Skill: 0, Experience: 10})
	if !ran {
		t.Fatal("second gained hook did not run after the panicking hook")
	}
	if !strings.Contains(buf2.String(), "experience_gained hook failed (Panicker)") {
		t.Fatalf("log = %q, want the panic-isolation line", buf2.String())
	}
}

func TestSkillsRegistrationAfterLoadIgnored(t *testing.T) {
	h, buf := newTestHub()
	h.Load()
	newApi(h, "Late").OnPlayerLevelUp(func(*PlayerLevelUpEvent) {})
	newApi(h, "Late").OnExperienceGained(func(*ExperienceGainedEvent) {})
	newApi(h, "Late").OnCanGainExperience(func(*CanGainExperienceEvent) {})
	for _, family := range []string{"onPlayerLevelUp", "onExperienceGained", "onCanGainExperience"} {
		if !strings.Contains(buf.String(), family) {
			t.Fatalf("log = %q, want a post-load refusal for %s", buf.String(), family)
		}
	}
	// A nil handler is ignored silently even before load, and
	// registers nothing.
	h2, _ := newTestHub()
	newApi(h2, "Nil").OnPlayerLevelUp(nil)
	newApi(h2, "Nil").OnExperienceGained(nil)
	newApi(h2, "Nil").OnCanGainExperience(nil)
	allow, multiplier := h2.EmitCanGainExperience(testPlayer("alice"), 0, 100)
	if allow != nil || multiplier != 1 {
		t.Fatalf("nil-handler (allow, multiplier) = (%v, %v), want (nil, 1)", allow, multiplier)
	}
}

func TestGetTaskManagerNilBeforeInstall(t *testing.T) {
	h, _ := newTestHub()
	if got := newApi(h, "Reader").GetTaskManager(); got != nil {
		t.Fatalf("GetTaskManager before install = %v, want nil", got)
	}
	if got := h.TaskManager(); got != nil {
		t.Fatalf("Hub.TaskManager before install = %v, want nil", got)
	}
	// A nil Api and a nil Hub yield nil.
	var nilApi *Api
	if got := nilApi.GetTaskManager(); got != nil {
		t.Fatalf("nil-Api GetTaskManager = %v, want nil", got)
	}
	var nilHub *Hub
	if got := nilHub.TaskManager(); got != nil {
		t.Fatalf("nil-Hub TaskManager = %v, want nil", got)
	}
	nilHub.SetTaskManager(game.NewTaskManager()) // must not panic
}

func TestGetTaskManagerReturnsInstalled(t *testing.T) {
	h, _ := newTestHub()
	tm := game.NewTaskManager()
	h.SetTaskManager(tm)
	if got := newApi(h, "Reader").GetTaskManager(); got != tm {
		t.Fatalf("GetTaskManager = %v, want the installed %v", got, tm)
	}
	if got := h.TaskManager(); got != tm {
		t.Fatalf("Hub.TaskManager = %v, want the installed %v", got, tm)
	}
	// Installing nil clears the handle.
	h.SetTaskManager(nil)
	if got := newApi(h, "Reader").GetTaskManager(); got != nil {
		t.Fatalf("GetTaskManager after clear = %v, want nil", got)
	}
	// The installed manager schedules: Schedule is TS submit.
	h.SetTaskManager(tm)
	task := game.NewTask(0, func(cycle int64) bool { return true })
	tm.Schedule(task)
	if !task.IsRunning() {
		t.Fatal("task scheduled through the installed manager is not running")
	}
}

func TestEmitCanGainExperienceEventFields(t *testing.T) {
	h, _ := newTestHub()
	var seen *CanGainExperienceEvent
	newApi(h, "Watcher").OnCanGainExperience(func(ev *CanGainExperienceEvent) { seen = ev })
	p := testPlayer("alice")
	h.EmitCanGainExperience(p, 5, 250)
	if seen == nil || seen.Player != p || seen.Skill != 5 || seen.Experience != 250 {
		t.Fatalf("seen event = %+v, want player/skill 5/experience 250", seen)
	}
	if seen.Allow != nil || seen.Multiplier != 1 {
		t.Fatalf("initial (allow, multiplier) = (%v, %v), want (nil, 1)", seen.Allow, seen.Multiplier)
	}
}
