package game

import "sync"

// Special-attack energy restore: the port of
// RestoreSpecialAttackTask (task/impl/RestoreSpecialAttackTask.ts)
// plus CombatSpecial.ensureRestoreTask (CombatSpecial.ts:192-210).
//
// TS behavior: when a fighter's special energy drops below 100, a
// task restores +10% every 50 ticks (25 with the Lightbearer,
// through EquipmentEffects) until 100, then stops. The task
// starts on login (Player.ts:794) and on drain
// (CombatSpecial.ts:178). Logging out restarts the timer: the
// restore timer is not saved, so the first 10% after login
// takes the full 50 ticks (PlayerSave.ts, fromPlayer comment).
//
// Go shape: the restore task runs on the world's TaskManager
// through the installed scheduler seam (the NpcRespawnScheduler
// precedent in npc_respawn.go). The recovering state TS keeps
// in two places — Mobile.isRecoveringSpecialAttack plus the
// runningTasks WeakMap — collapses into one mutex-guarded map
// here: EnsureSpecialRestore is called from Player.Process
// (tick goroutine) and the map guard keeps the check-and-set
// atomic against any future caller.
//
// Deviations, all documented:
//   - The delay is flat 50 ticks: EquipmentEffects modifiers
//     (Lightbearer) are NOT STARTED, so no 25-tick path exists.
//   - The restore amount is fixed at 10: TS lets content change
//     it through the "combat-special:regenerate" custom event;
//     no such emit exists in Go yet.
//   - The weapon special-bar/meter tail of updateBar is NOT
//     sent, and must not be: in TS it goes through
//     PacketSender.sendInterfaceComponentMoval
//     (CombatSpecial.ts:240-259), whose only send path is
//     PlayerSession.write — a no-op at bfd81e3
//     (PlayerSession.ts:228-230: "Unported callers remain
//     no-ops until they use a native packet encoder"). The
//     tail is dead in TS itself, so Go sending nothing is
//     the faithful port, not a gap: the bar/meter ids DO exist
//     in Go (WeaponInterface.SpecialBar/SpecialMeter), but
//     there is no live TS send to mirror. A native
//     WIDGET_SET_POSITION-based tail would be a deviation,
//     not a port, and is not built here.
//   - The player restore countdown
//     (getSpecialAttackRestore) has no Go equivalent: nothing
//     reads the remaining time.

// SpecialAttackOrbVarp is the varp the special-attack orb reads:
// CombatSpecial.updateBar's updateSpecialAttackOrb sends
// sendConfig(300, specialPercentage * 10)
// (PacketSender.ts:393-395, CombatSpecial.ts:232-233).
const SpecialAttackOrbVarp = 300

// SpecialAttackStateVarp is the varp for the armed state:
// CombatSpecial.updateBar's sendSpecialAttackState sends
// sendConfig(301, activated ? 1 : 0)
// (PacketSender.ts:248-250, CombatSpecial.ts:234).
const SpecialAttackStateVarp = 301

// SpecialRestoreDelayTicks is one 10% restore cycle: 50 ticks
// (RestoreSpecialAttackTask.ts DEFAULT_RESTORE_DELAY_TICKS).
const SpecialRestoreDelayTicks = 50

// specialRestoreAmount is the energy restored per cycle: 10%
// (RestoreSpecialAttackTask.ts execute, regen.amount default).
const specialRestoreAmount = 10

// specialRestoreTaskName identifies restore tasks in the
// TaskManager (slow-task reports, HasActiveTask checks).
const specialRestoreTaskName = "restore-special-attack"

// specialRestoreTasks is the TaskManager restore tasks are
// submitted to, installed once at boot via
// InstallSpecialRestoreWiring. Nil means no restore tasks are
// scheduled (EnsureSpecialRestore still records the
// recovering state, so a later install does not double-start;
// in practice the wiring installs before the engine starts).
var specialRestoreTasks *TaskManager

// restoringSpecial tracks fighters with a live restore task,
// the Go form of isRecoveringSpecialAttack + runningTasks.
// The mutex guards it: EnsureSpecialRestore runs on the tick
// goroutine, but the map is also read from the task action,
// and a future login-path caller must not race either.
var (
	restoringSpecialMu sync.Mutex
	restoringSpecial   = make(map[*Mobile]bool)
)

// InstallSpecialRestoreWiring installs the TaskManager
// special-restore tasks run on. The coordinator calls it once
// at boot, before the engine starts (the InstallNpcRespawnWiring
// precedent). A nil manager means restore tasks are never
// scheduled.
func (w *World) InstallSpecialRestoreWiring() {
	specialRestoreTasks = w.Tasks()
}

// SetSpecialRestoreTasks installs the TaskManager directly,
// for tests. Production uses InstallSpecialRestoreWiring.
func SetSpecialRestoreTasks(tasks *TaskManager) {
	specialRestoreTasks = tasks
}

// IsRecoveringSpecialAttack reports whether a restore task is
// running for m (Mobile.isRecoveringSpecialAttack). A nil
// Mobile answers false.
func IsRecoveringSpecialAttack(m *Mobile) bool {
	if m == nil {
		return false
	}
	restoringSpecialMu.Lock()
	defer restoringSpecialMu.Unlock()
	return restoringSpecial[m]
}

// EnsureSpecialRestore starts the restore task for m when its
// energy is below 100 and no restore is running
// (CombatSpecial.ensureRestoreTask), returning whether a task
// was started. It is idempotent: a second call while a task
// runs returns false. With no TaskManager installed it
// records nothing and returns false — the Player.Process
// call site retries every tick, so a late install still
// starts the restore.
//
// Callers: Player.Process (every tick; covers login, since no
// tick-goroutine-safe submit exists on the login path) and
// Combat.DrainSpecial (the CombatSpecial.ts:178 drain site).
func EnsureSpecialRestore(m *Mobile) bool {
	if m == nil || m.GetCombat().SpecialPercentage() >= 100 {
		return false
	}
	restoringSpecialMu.Lock()
	if restoringSpecial[m] {
		restoringSpecialMu.Unlock()
		return false
	}
	tasks := specialRestoreTasks
	if tasks == nil {
		restoringSpecialMu.Unlock()
		return false
	}
	restoringSpecial[m] = true
	restoringSpecialMu.Unlock()
	tasks.Schedule(newSpecialRestoreTask(m))
	return true
}

// newSpecialRestoreTask returns the restore task for m: first
// execution after the full delay (login restarts the timer),
// repeating every delay until the energy is full or the
// fighter leaves the world (RestoreSpecialAttackTask).
func newSpecialRestoreTask(m *Mobile) *Task {
	task := NewTask(SpecialRestoreDelayTicks, func(cycle int64) (done bool) {
		return restoreSpecialTick(m)
	})
	task.Name = specialRestoreTaskName
	task.Key = m
	return task
}

// restoreSpecialTick runs one restore cycle, returning done
// when the task must stop (RestoreSpecialAttackTask.execute):
// the fighter left the world, the energy reached 100, or the
// recovering state was cleared elsewhere. Each cycle restores
// 10%, clamped at 100.
func restoreSpecialTick(m *Mobile) (done bool) {
	if m == nil {
		return true
	}
	combat := m.GetCombat()
	if !m.IsRegistered() || combat.SpecialPercentage() >= 100 {
		clearRestoringSpecial(m)
		return true
	}
	amount := combat.SpecialPercentage() + specialRestoreAmount
	if amount >= 100 {
		amount = 100
	}
	combat.SetSpecialPercentage(amount)
	updateSpecialAttackOrb(m)
	if amount >= 100 {
		clearRestoringSpecial(m)
		return true
	}
	return false
}

// updateSpecialAttackOrb sends the orb half of
// CombatSpecial.updateBar (CombatSpecial.ts:231-234) for a
// restoring fighter: varp 300 carries the energy x10 and varp
// 301 the armed state, in TS order (orb first, state second).
// RestoreSpecialAttackTask.execute calls updateBar after every
// restore cycle, including the final one that reaches 100
// (RestoreSpecialAttackTask.ts execute, trailing updateBar
// call). NPCs get nothing: TS guards the call with isPlayer.
// A player with no session drops both sends (Player.SendVarp
// is a nil-session no-op). The weapon special-bar/meter tail
// of updateBar is NOT sent here: in TS it goes through the
// no-op PlayerSession.write (see the file header).
func updateSpecialAttackOrb(m *Mobile) {
	player := combatantFor(m).Player()
	if player == nil {
		return
	}
	player.SendVarp(SpecialAttackOrbVarp, player.GetCombat().SpecialPercentage()*10)
	activated := 0
	if player.GetCombat().IsSpecialActivated() {
		activated = 1
	}
	player.SendVarp(SpecialAttackStateVarp, activated)
}

// clearRestoringSpecial drops m from the recovering map.
func clearRestoringSpecial(m *Mobile) {
	restoringSpecialMu.Lock()
	defer restoringSpecialMu.Unlock()
	delete(restoringSpecial, m)
}
