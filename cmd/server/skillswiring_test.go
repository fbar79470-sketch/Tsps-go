package main

import (
	"io"
	"log"
	"testing"

	"tsps-go/internal/game"
	"tsps-go/internal/plugins"
	"tsps-go/internal/session"
)

// Skills-wiring tests: InstallSkillHooks (seam population, gate
// forwarding, event translation, the TaskManager install),
// installSkillExperienceRate, and installSkillExpDropWiring
// (nil-safety, non-disturbance of the update hook). Fixtures
// only: a hub with a test plugin, a world, and hand-built
// players. The game.SkillHooks global and the world experience
// rate are saved and restored around every test that installs
// them, so buildServer tests sharing this package see no
// leakage.

// skillsTestHub builds a hub whose single plugin registers the
// given skills hooks, then loads it.
func skillsTestHub(t *testing.T, register func(*plugins.Api)) *plugins.Hub {
	t.Helper()
	hub := plugins.NewHub([]plugins.Plugin{{
		Name:     "SkillsProbe",
		Register: register,
	}}, plugins.HubOptions{Logger: log.New(io.Discard, "", 0)})
	hub.Load()
	return hub
}

// saveSkillSeam snapshots the game.SkillHooks seam and the world
// experience rate and restores both when the test ends.
func saveSkillSeam(t *testing.T) {
	t.Helper()
	savedHooks := game.SkillHooks
	savedRate := game.ExperienceRate()
	t.Cleanup(func() {
		game.SkillHooks = savedHooks
		game.SetExperienceRate(savedRate)
	})
}

func TestInstallSkillHooksGateForwards(t *testing.T) {
	saveSkillSeam(t)
	game.SkillHooks = game.SkillsHooks{}
	installSkillExperienceRate(1)
	deny := false
	hub := skillsTestHub(t, func(api *plugins.Api) {
		api.OnCanGainExperience(func(e *plugins.CanGainExperienceEvent) {
			switch e.Experience {
			case 100:
				e.Allow = &deny
			case 50:
				e.Multiplier *= 2
			}
		})
	})
	world := game.NewWorld(game.Location{X: 3000, Y: 3000})
	InstallSkillHooks(hub, world)

	player := game.NewPlayer("gateprobe", game.Location{X: 3000, Y: 3000})
	// 50 doubled by the gate: 100 lands, Mining levels 1 -> 2.
	player.Skills().AddExperience(game.SkillMining, 50)
	if got := player.Skills().Experience(game.SkillMining); got != 100 {
		t.Fatalf("Mining XP after gated 50 = %d, want 100", got)
	}
	// 100 refused by the gate: nothing lands, before any send.
	player.Skills().AddExperience(game.SkillMining, 100)
	if got := player.Skills().Experience(game.SkillMining); got != 100 {
		t.Fatalf("Mining XP after refused 100 = %d, want 100 (unchanged)", got)
	}
	// An unanswered gain lands unmultiplied.
	player.Skills().AddExperience(game.SkillMining, 10)
	if got := player.Skills().Experience(game.SkillMining); got != 110 {
		t.Fatalf("Mining XP after plain 10 = %d, want 110", got)
	}
}

func TestInstallSkillHooksEmitsTranslate(t *testing.T) {
	saveSkillSeam(t)
	game.SkillHooks = game.SkillsHooks{}
	installSkillExperienceRate(1)
	var levelUp *plugins.PlayerLevelUpEvent
	var gained *plugins.ExperienceGainedEvent
	hub := skillsTestHub(t, func(api *plugins.Api) {
		api.OnPlayerLevelUp(func(e *plugins.PlayerLevelUpEvent) { levelUp = e })
		api.OnExperienceGained(func(e *plugins.ExperienceGainedEvent) { gained = e })
	})
	InstallSkillHooks(hub, game.NewWorld(game.Location{X: 3000, Y: 3000}))

	player := game.NewPlayer("emitprobe", game.Location{X: 3000, Y: 3000})
	player.Skills().AddExperience(game.SkillMining, 100)
	if levelUp == nil {
		t.Fatal("level-up hook did not fire")
	}
	if levelUp.Player != player || levelUp.Skill != game.SkillMining || levelUp.OldLevel != 1 || levelUp.NewLevel != 2 {
		t.Fatalf("level-up event = %+v, want player/skill 14/old 1/new 2", levelUp)
	}
	if gained == nil {
		t.Fatal("experience-gained hook did not fire")
	}
	if gained.Player != player || gained.Skill != game.SkillMining || gained.Experience != 100 {
		t.Fatalf("gained event = %+v, want player/skill 14/xp 100 (landed delta)", gained)
	}
	// A nil game event through the seam is a no-op, not a panic.
	game.SkillHooks.EmitPlayerLevelUp(nil)
	game.SkillHooks.EmitExperienceGained(nil)
}

func TestInstallSkillHooksNilSafe(t *testing.T) {
	saveSkillSeam(t)
	game.SkillHooks = game.SkillsHooks{}
	// A nil hub installs nothing: the seam stays zero (inert).
	InstallSkillHooks(nil, game.NewWorld(game.Location{X: 3000, Y: 3000}))
	if game.SkillHooks.CanGainExperience != nil || game.SkillHooks.EmitPlayerLevelUp != nil || game.SkillHooks.EmitExperienceGained != nil {
		t.Fatal("nil hub populated the seam")
	}
	// A nil world still installs the seam adapters, but no
	// TaskManager handle.
	hub := skillsTestHub(t, func(*plugins.Api) {})
	InstallSkillHooks(hub, nil)
	if game.SkillHooks.CanGainExperience == nil || game.SkillHooks.EmitPlayerLevelUp == nil || game.SkillHooks.EmitExperienceGained == nil {
		t.Fatal("nil world left the seam unpopulated")
	}
	if got := hub.TaskManager(); got != nil {
		t.Fatalf("TaskManager() = %v, want nil with a nil world", got)
	}
}

func TestInstallSkillHooksTaskManager(t *testing.T) {
	saveSkillSeam(t)
	hub := skillsTestHub(t, func(*plugins.Api) {})
	world := game.NewWorld(game.Location{X: 3000, Y: 3000})
	InstallSkillHooks(hub, world)
	if got := hub.TaskManager(); got != world.Tasks() {
		t.Fatalf("TaskManager() = %p, want the world's %p", got, world.Tasks())
	}
}

func TestInstallSkillExperienceRate(t *testing.T) {
	saveSkillSeam(t)
	installSkillExperienceRate(2.5)
	if got := game.ExperienceRate(); got != 2.5 {
		t.Fatalf("ExperienceRate() = %v, want 2.5", got)
	}
	// Invalid values are ignored by the setter (the wiring
	// passes the parsed value through unconditionally).
	installSkillExperienceRate(0)
	installSkillExperienceRate(-1)
	if got := game.ExperienceRate(); got != 2.5 {
		t.Fatalf("ExperienceRate() after invalid sets = %v, want 2.5", got)
	}
	// The rate multiplies gains end to end through the seam.
	game.SkillHooks = game.SkillsHooks{}
	player := game.NewPlayer("rateprobe", game.Location{X: 3000, Y: 3000})
	player.Skills().AddExperience(game.SkillMining, 10)
	if got := player.Skills().Experience(game.SkillMining); got != 25 {
		t.Fatalf("Mining XP at rate 2.5 after 10 = %d, want 25", got)
	}
}

func TestInstallSkillExpDropWiring(t *testing.T) {
	// Nil-safety: every nil combination installs nothing and
	// never panics.
	installSkillExpDropWiring(nil, nil)
	player := game.NewPlayer("dropprobe", game.Location{X: 3000, Y: 3000})
	installSkillExpDropWiring(nil, session.NewSession(&recordingChannel{}))
	installSkillExpDropWiring(player, nil)

	// The install does not disturb the update hook owned by
	// installSkillsAppearanceWiring: the update hook still
	// fires after the exp-drop install, and XP still lands.
	updates := 0
	player.Skills().SetUpdateHook(func(game.SkillUpdate) { updates++ })
	installSkillExpDropWiring(player, session.NewSession(&recordingChannel{}))
	player.Skills().AddExperience(game.SkillMining, 10)
	if updates == 0 {
		t.Fatal("update hook stopped firing after the exp-drop install")
	}
	if got := player.Skills().Experience(game.SkillMining); got != 10 {
		t.Fatalf("Mining XP = %d, want 10", got)
	}
}
