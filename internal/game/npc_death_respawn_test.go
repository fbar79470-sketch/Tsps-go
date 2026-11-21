package game

import "testing"

// TestNPCKillRespawnsCloneAndRemovesCorpse is the regression test
// for the Phase 26 live-smoke finding that a killed goblin stayed
// in the world at HP 0 with Dying=true and no respawn clone
// arrived in 250 ticks. It kills a goblin through the real combat
// pipeline in a ticking world (production wiring: respawn
// scheduler + CombatHits sweep + world cycle source) and asserts
// both halves of the death flow: a respawn clone joins the world
// after the definition delay, and the corpse leaves it.
//
// Grounding: NPCDeathTask.remove (NPCDeathTask.ts:92-110) removes
// the dead NPC from the world and queues the respawn; the Go
// death handling calls BeginRespawn at the remove point
// (combat_death.go dispatchNpcDeath) and must also queue the
// removal, or the corpse stays registered forever.
func TestNPCKillRespawnsCloneAndRemovesCorpse(t *testing.T) {
	resetCombatHooks(t)
	// Maximum rolls so the player's attacks always land.
	setFixedRandom(t, func(max int) int { return max }, 0.5)
	w := NewWorld(Location{X: 3000, Y: 3000, Z: 0})
	w.InstallNpcRespawnWiring()
	w.Hooks.CombatHits = ProcessAllHitQueues
	SetCombatCycleSource(w.ProcessCycle)
	t.Cleanup(func() {
		SetNpcRespawnScheduler(nil)
		SetNpcRemover(nil)
		SetUniqueHolderLiveness(nil)
		SetCombatCycleSource(nil)
		lastHitQueueCycle = 0
	})

	// Unregistered entities: the drain assigns indices on add
	// (MobileList.Add refuses already-registered mobiles).
	player := NewPlayer("killer", Location{X: 3000, Y: 3000, Z: 0})
	player.Session = newFakeSession()
	player.GetCombat() // bind identity
	setPlayerLevels(player, map[int]int{
		SkillHitpoints: 99, SkillAttack: 99, SkillStrength: 99, SkillDefence: 99,
	})
	goblin := NewNPC(3030, Location{X: 3001, Y: 3000, Z: 0})
	goblin.SetDefinition(newCombatFakeDef())
	goblin.Hitpoints = 5
	goblin.GetCombat() // bind identity + coordinator seams
	w.QueueAddPlayer(player)
	w.QueueAddNPC(goblin)
	w.Process()
	if w.NPCs().Len() != 1 {
		t.Fatalf("world NPCs after add = %d, want 1", w.NPCs().Len())
	}

	player.GetCombat().Attack(&goblin.Mobile)
	killed := false
	for i := 0; i < 200 && !killed; i++ {
		w.Process()
		if goblin.Dying {
			killed = true
		}
	}
	if !killed {
		t.Fatalf("goblin never died in 200 ticks (hp=%d, dying=%v)", goblin.Hitpoints, goblin.Dying)
	}

	// The definition respawn delay is 25 ticks; allow 60 for
	// the death ticks + delay + queue drain.
	cloneSeen := false
	corpseGone := false
	for i := 0; i < 60; i++ {
		w.Process()
		originalPresent := false
		clonePresent := false
		for _, n := range w.NPCs().All() {
			if n == goblin {
				originalPresent = true
			} else if n.ID == 3030 {
				clonePresent = true
				if n.Hitpoints <= 0 || n.Dying {
					t.Errorf("respawn clone must be alive, got hp=%d dying=%v", n.Hitpoints, n.Dying)
				}
			}
		}
		if clonePresent {
			cloneSeen = true
		}
		if !originalPresent {
			corpseGone = true
		}
		if cloneSeen && corpseGone {
			break
		}
	}
	if !cloneSeen {
		t.Errorf("no respawn clone joined the world within 60 ticks of the kill (NPCs=%d)", w.NPCs().Len())
	}
	if !corpseGone {
		t.Errorf("dead goblin still in the world 60 ticks after the kill (hp=%d, dying=%v)", goblin.Hitpoints, goblin.Dying)
	}
}
