package game

import "testing"

// Spec-orb tests (phase 26): the orb half of CombatSpecial.updateBar
// sent on special restore — varp 300 (energy x10) and varp 301
// (armed state) — grounded in CombatSpecial.updateBar
// (CombatSpecial.ts:231-234), PacketSender.updateSpecialAttackOrb
// (PacketSender.ts:393-395, sendConfig(300, special% * 10)),
// PacketSender.sendSpecialAttackState (PacketSender.ts:248-250,
// sendConfig(301, activated ? 1 : 0)), and
// RestoreSpecialAttackTask.execute (RestoreSpecialAttackTask.ts
// execute, trailing updateBar call after every restore cycle,
// including the final one at 100). The weapon special-bar/meter
// tail of updateBar is NOT sent (no Go weapon surface carries
// the bar/meter ids) and is not asserted here.

func TestSpecialRestoreSendsOrbVarps(t *testing.T) {
	tasks := &TaskManager{}
	SetSpecialRestoreTasks(tasks)
	t.Cleanup(func() { SetSpecialRestoreTasks(nil) })

	p, sess := combatTestPlayer(t, "alice", 3089, 3524)
	p.GetCombat().SetSpecialPercentage(80)
	if !EnsureSpecialRestore(&p.Mobile) {
		t.Fatal("ensure must start the task")
	}
	for cycle := int64(1); cycle <= SpecialRestoreDelayTicks; cycle++ {
		tasks.Process(cycle)
	}
	if got := p.GetCombat().SpecialPercentage(); got != 90 {
		t.Fatalf("spec after 50 ticks = %d, want 90", got)
	}
	// One restore cycle sends exactly the orb pair, orb first.
	if len(sess.varps) != 2 {
		t.Fatalf("varps = %v, want exactly 2 sends", sess.varps)
	}
	if sess.varps[0] != [2]int{SpecialAttackOrbVarp, 900} {
		t.Fatalf("varps[0] = %v, want [300 900]", sess.varps[0])
	}
	if sess.varps[1] != [2]int{SpecialAttackStateVarp, 0} {
		t.Fatalf("varps[1] = %v, want [301 0] (not armed)", sess.varps[1])
	}

	// The final cycle at 100 sends the orb pair again.
	for cycle := int64(51); cycle <= 2*SpecialRestoreDelayTicks; cycle++ {
		tasks.Process(cycle)
	}
	if got := p.GetCombat().SpecialPercentage(); got != 100 {
		t.Fatalf("spec after 100 ticks = %d, want 100", got)
	}
	if len(sess.varps) != 4 {
		t.Fatalf("varps = %v, want 4 sends after two cycles", sess.varps)
	}
	if sess.varps[2] != [2]int{SpecialAttackOrbVarp, 1000} {
		t.Fatalf("varps[2] = %v, want [300 1000]", sess.varps[2])
	}
	if sess.varps[3] != [2]int{SpecialAttackStateVarp, 0} {
		t.Fatalf("varps[3] = %v, want [301 0]", sess.varps[3])
	}
}

func TestSpecialRestoreSendsArmedState(t *testing.T) {
	tasks := &TaskManager{}
	SetSpecialRestoreTasks(tasks)
	t.Cleanup(func() { SetSpecialRestoreTasks(nil) })

	p, sess := combatTestPlayer(t, "alice", 3089, 3524)
	p.GetCombat().SetSpecialPercentage(80)
	p.GetCombat().SetSpecialActivated(true)
	if !EnsureSpecialRestore(&p.Mobile) {
		t.Fatal("ensure must start the task")
	}
	for cycle := int64(1); cycle <= SpecialRestoreDelayTicks; cycle++ {
		tasks.Process(cycle)
	}
	if len(sess.varps) != 2 || sess.varps[1] != [2]int{SpecialAttackStateVarp, 1} {
		t.Fatalf("varps = %v, want [300 900] then [301 1] (armed)", sess.varps)
	}
}

func TestSpecialRestoreNpcSendsNoOrb(t *testing.T) {
	tasks := &TaskManager{}
	SetSpecialRestoreTasks(tasks)
	t.Cleanup(func() { SetSpecialRestoreTasks(nil) })

	npc := combatTestNPC(t, 1, 3001, 3000, nil)
	npc.GetCombat().SetSpecialPercentage(80)
	if !EnsureSpecialRestore(&npc.Mobile) {
		t.Fatal("ensure must start the task")
	}
	for cycle := int64(1); cycle <= SpecialRestoreDelayTicks; cycle++ {
		tasks.Process(cycle)
	}
	// TS guards the updateBar call with isPlayer: an NPC restores
	// its energy with no orb send (and no session to send to).
	if got := npc.GetCombat().SpecialPercentage(); got != 90 {
		t.Fatalf("npc spec after 50 ticks = %d, want 90", got)
	}
}

// DrainSpecial re-sends the orb (CombatSpecial.drain,
// CombatSpecial.ts:170-182: decrement, ensureRestoreTask, then
// updateBar for a player): varp 300 then varp 301, in TS
// order, at the drained energy.
func TestDrainSpecialSendsOrbVarps(t *testing.T) {
	p, sess := combatTestPlayer(t, "alice", 3089, 3524)
	p.GetCombat().SetSpecialPercentage(100)
	p.GetCombat().DrainSpecial(25)
	if got := p.GetCombat().SpecialPercentage(); got != 75 {
		t.Fatalf("spec after drain 25 = %d, want 75", got)
	}
	if len(sess.varps) != 2 {
		t.Fatalf("varps = %v, want exactly the orb pair", sess.varps)
	}
	if sess.varps[0] != [2]int{SpecialAttackOrbVarp, 750} {
		t.Fatalf("varps[0] = %v, want [300 750]", sess.varps[0])
	}
	if sess.varps[1] != [2]int{SpecialAttackStateVarp, 0} {
		t.Fatalf("varps[1] = %v, want [301 0] (not armed)", sess.varps[1])
	}
}

// A drain on an NPC sends no orb (TS guards updateBar with
// isPlayer; the helper no-ops for a non-player Mobile).
func TestDrainSpecialNpcSendsNoOrb(t *testing.T) {
	npc := combatTestNPC(t, 1, 3001, 3000, nil)
	npc.GetCombat().SetSpecialPercentage(100)
	npc.GetCombat().DrainSpecial(25)
	if got := npc.GetCombat().SpecialPercentage(); got != 75 {
		t.Fatalf("npc spec after drain 25 = %d, want 75", got)
	}
}

// AssignWeaponInterface sends the orb (WeaponInterfaceManager
// .assign, WeaponInterfaceManager.ts:79-80: CombatSpecial.assign
// then CombatSpecial.updateBar): the login send, since onLogin
// calls assign (Player.ts:794). Varps 300/301 arrive in TS
// order, after the tab sends and before the fight-type varp.
func TestAssignWeaponInterfaceSendsOrbVarps(t *testing.T) {
	installFixtureStyles(t)
	p, session := stylePlayer(nil)
	p.GetCombat().SetSpecialPercentage(50)
	p.GetCombat().SetSpecialActivated(true)
	AssignWeaponInterface(p)
	if v, ok := findVarp(session, SpecialAttackOrbVarp); !ok || v != 500 {
		t.Fatalf("varp 300 = %d/%v, want 500/true", v, ok)
	}
	if v, ok := findVarp(session, SpecialAttackStateVarp); !ok || v != 1 {
		t.Fatalf("varp 301 = %d/%v, want 1/true (armed)", v, ok)
	}
	orb := -1
	state := -1
	for i, v := range session.varps {
		if v[0] == SpecialAttackOrbVarp {
			orb = i
		}
		if v[0] == SpecialAttackStateVarp {
			state = i
		}
	}
	if orb < 0 || state != orb+1 {
		t.Fatalf("varps = %v, want 300 immediately before 301", session.varps)
	}
}
