// Ent trunks: the Go port of
// server/plugins/skills/woodcutting/EntTrunk.Woodcutting.js (162
// lines, commit bfd81e3). A dead Ent stays where it fell and four
// ticks after it dies it is an Ent trunk; only its killer may
// chop it, one noted log per success (two in the Wilderness),
// 25 XP, the usual 1/256 bird nest chance; the trunk lasts 101
// ticks and the Ent respawns 50 ticks later in the guild (15 in
// the Wilderness). Data lives in guilddata.go (Ents).
//
// Line numbers in comments cite EntTrunk.Woodcutting.js unless
// another file is named.
//
// Deviations, named:
//   - TS schedules the +2-tick transform with a one-shot
//     TaskManager task (later, lines 45-57); Go drives the
//     pending transforms from this unit's own 1-tick task
//     (the same tick counts, and unit-testable in unit time).
//     The task schedules lazily on the first Ent death, the
//     woodcutting ensureTask pattern (the TaskManager installs
//     after plugin Load, so a Register-time capture reads nil).
//   - TS sets the trunk's poison damage to 0 at transform
//     (setPoisonDamage?.(0)); Go calls Mobile.SetPoisonDamage
//     (0) directly — the method exists, so this is a straight
//     port, not a gap.
//   - XP is int throughout (SkillManager.AddExperiences takes
//     int): 25 x the lumberjack multiplier truncates, the
//     Phase 32 structural residual — a full lumberjack outfit
//     still pays 25, not 25.625.
package woodcutting

import (
	"strconv"
	"strings"

	"tsps-go/internal/game"
	"tsps-go/internal/plugins"
)

// entCutChanceTree is the pseudo-tree calculateCutChance reads
// the Ent chart from (EntTrunk.Woodcutting.js: tick passes
// {cutChance: ENTS.cutChance}): the chart has all eight rows,
// so CutChanceRow never scales off the normal chart.
var entCutChanceTree = &Tree{CutChance: Ents.CutChance}

// entTrunkState is one live trunk (EntTrunk.Woodcutting.js:18,
// trunks -> {killer, ent, until}).
type entTrunkState struct {
	killer *game.Player
	ent    EntNPC
	until  int64
}

// entChopSession is one player chopping a trunk
// (EntTrunk.Woodcutting.js:20, sessions -> {npc, axe, next,
// from}).
type entChopSession struct {
	npc  *game.NPC
	axe  *Axe
	next int64
	from game.Location
}

// entTransform is one pending trunk transform (the later()
// call in entDied, lines 57-66): the NPC, its killer and Ent
// row, and the module cycle it transforms on.
type entTransform struct {
	npc    *game.NPC
	killer *game.Player
	ent    EntNPC
	due    int64
}

// entTrunk is the Ent trunk unit's per-registration state: the
// TS module-level maps (trunks, sessions) plus the module's
// own cycle counter, as struct fields, per the Phase 21 race
// fix. One instance serves the registered Plugin value
// (held by woodcutting); tests build their own with
// newEntTrunk.
type entTrunk struct {
	w *woodcutting
	// cycle is the module's own tick counter (EntTrunk
	// .Woodcutting.js:30-31): tick() increments it, and
	// trunk/session times are measured in it.
	cycle int64
	// trunks maps a trunk NPC to its state; sessions maps a
	// chopping player to their session; pending holds the
	// transforms not yet due.
	trunks   map[*game.NPC]*entTrunkState
	sessions map[*game.Player]*entChopSession
	pending  []entTransform
	// tasks is the unit-test TaskManager override; production
	// resolves through w.taskManager(). taskScheduled guards
	// ensureTask: the 1-tick task schedules once, on the first
	// Ent death.
	tasks         *game.TaskManager
	taskScheduled bool
	// noteID resolves a log id to its note id (the
	// core.ItemDefinition.forId(log.id).getNoteId() read in
	// giveLogs). A test override; nil resolves through the
	// player's item-definition source
	// (game.ItemNoteIDSource), else the unnoted id.
	noteID func(id int) int
}

// newEntTrunk builds an Ent trunk unit over the woodcutting
// instance that owns the shared helpers (findBestUsableAxe,
// calculateCutChance, lumberjackMultiplier, maybeDropBirdNest)
// and the emit func.
func newEntTrunk(w *woodcutting) *entTrunk {
	return &entTrunk{
		w:        w,
		trunks:   make(map[*game.NPC]*entTrunkState),
		sessions: make(map[*game.Player]*entChopSession),
	}
}

// register wires the unit's hooks, attach-only (one line per
// hook): onNpcDeath, onNpcClick (trunk id, option 1), and
// onPlayerDisconnect.
func (e *entTrunk) register(api *plugins.Api) {
	api.OnNpcDeath(e.entDied)
	api.OnNpcClick([]int{Ents.TrunkID}, 1, e.chopTrunk)
	api.OnPlayerDisconnect(e.forget)
}

// ensureTask schedules the 1-tick EntTrunkTask once, on the
// lazily-resolved TaskManager (the woodcutting ensureTask
// pattern: the hub installs the TaskManager after plugin
// Load, so a Register-time capture reads nil; entDied runs
// on the tick goroutine through the death dispatch, which
// is the only goroutine the TaskManager allows Schedule
// from). Idempotent; a nil TaskManager schedules nothing
// and retries on the next death.
func (e *entTrunk) ensureTask() {
	if e.taskScheduled {
		return
	}
	var tm *game.TaskManager
	if e.tasks != nil {
		tm = e.tasks
	} else if e.w != nil {
		tm = e.w.taskManager()
	}
	if tm == nil {
		return
	}
	task := game.NewTask(1, func(cycle int64) bool {
		e.tick()
		return false
	})
	task.Name = "EntTrunkTask"
	tm.Schedule(task)
	e.taskScheduled = true
}

// startSession records a chopping session and sets the
// no-retaliate flag (EntTrunk.Woodcutting.js:26-29,
// startSession): being attacked meanwhile doesn't stop the
// chopping.
func (e *entTrunk) startSession(player *game.Player, session *entChopSession) {
	e.sessions[player] = session
	game.SetCombatFlag(&player.Mobile, game.CombatFlagNoRetaliate, true)
}

// endSession clears a chopping session and the no-retaliate
// flag (EntTrunk.Woodcutting.js:31-34, endSession). Ending
// a session that was never started clears nothing.
func (e *entTrunk) endSession(player *game.Player) {
	if _, ok := e.sessions[player]; !ok {
		return
	}
	delete(e.sessions, player)
	game.SetCombatFlag(&player.Mobile, game.CombatFlagNoRetaliate, false)
}

// entDied handles an Ent death (EntTrunk.Woodcutting.js:59-67,
// entDied), in exact TS order: only the two Ent NPC ids, only
// when the killer is a player; the remains are
// {ticks: transformTicks-2 + trunkTicks, respawnTicks}, and
// the transform is due transformTicks-2 ticks after the death
// event (four after it fell). The Api.OnNpcDeath wrapper
// already guards on a non-nil killer and NPC; the nil checks
// here are the direct-call (unit-test) guards.
func (e *entTrunk) entDied(event *plugins.NpcDeathEvent) {
	if event == nil || event.NPC == nil || event.Killer == nil {
		return
	}
	ent, ok := Ents.NPCs[event.NpcID]
	if !ok {
		return
	}
	untilTrunk := Ents.TransformTicks - 2
	event.Remains = &plugins.NpcDeathRemains{
		Ticks:           untilTrunk + Ents.TrunkTicks,
		RespawnTicks:    ent.RespawnTicks,
		HasRespawnTicks: true,
	}
	e.pending = append(e.pending, entTransform{
		npc:    event.NPC,
		killer: event.Killer,
		ent:    ent,
		due:    e.cycle + int64(untilTrunk),
	})
	e.ensureTask()
}

// rollEntLog rolls the log a success gives
// (EntTrunk.Woodcutting.js:69-81, rollLog): those the
// player's base level allows, weighted (better axes favour
// magic — the weightPerAxeTier bonus). The roll walks the
// weights down and lands on the first entry that takes it
// below 0; an over-total roll lands on the last entry. No
// eligible log yields nil (unreachable in practice: Logs
// is open from level 1).
func rollEntLog(baseLevel int, axe *Axe, randFloat func() float64) *EntLog {
	if axe == nil {
		return nil
	}
	type choice struct {
		log    *EntLog
		weight int
	}
	var choices []choice
	total := 0
	for i := range Ents.Logs {
		log := &Ents.Logs[i]
		if baseLevel < log.MinLevel || baseLevel > log.MaxLevel {
			continue
		}
		weight := log.Weight + log.WeightPerAxeTier*axe.Tier
		choices = append(choices, choice{log: log, weight: weight})
		total += weight
	}
	if len(choices) == 0 {
		return nil
	}
	roll := randFloat() * float64(total)
	for _, c := range choices {
		roll -= float64(c.weight)
		if roll < 0 {
			return c.log
		}
	}
	return choices[len(choices)-1].log
}

// maxWoodcuttingLevel reads the player's base Woodcutting
// level (getMaxLevel, the rollLog call site in giveLogs):
// 0 without a skill manager.
func maxWoodcuttingLevel(player *game.Player) int {
	if player == nil {
		return 0
	}
	skills := player.Skills()
	if skills == nil {
		return 0
	}
	return skills.GetMaxLevels(game.SkillWoodcutting)
}

// notedLogID resolves a log id to its note id (giveLogs:
// core.ItemDefinition.forId(log.id).getNoteId()): the note
// id when positive, else the log id itself. The test
// override wins; otherwise the player's item-definition
// source, when it carries the ItemNoteIDSource capability
// (the production source does, over the Store); a source
// without it yields the unnoted id, matching the DEFAULT
// definition's -1 note id.
func (e *entTrunk) notedLogID(player *game.Player, logID int) int {
	if e.noteID != nil {
		if noted := e.noteID(logID); noted > 0 {
			return noted
		}
		return logID
	}
	if player != nil {
		if src, ok := player.ItemDefinitions().(game.ItemNoteIDSource); ok && src != nil {
			if noted := src.NoteID(logID); noted > 0 {
				return noted
			}
		}
	}
	return logID
}

// chopTrunk handles a click on an Ent trunk
// (EntTrunk.Woodcutting.js:83-100, chopTrunk), in exact TS
// order. The handler only runs for the trunk id at click
// type 1, so every path returns true (claims the click,
// as TS does — even a non-trunk or a non-killer click
// claims it and shows nothing, or the no-axe/full line):
// a missing trunk or a non-killer claims silently; no
// usable axe sends the no-axe line; a full inventory sends
// the full line; otherwise the swing line, the axe swing,
// and a session starting its first roll chopTicks ticks
// out, from the player's current tile, with no-retaliate
// set.
func (e *entTrunk) chopTrunk(event *plugins.NpcInteractionEvent) bool {
	if event == nil || event.Player == nil || event.NPC == nil {
		return true
	}
	player := event.Player
	trunk, ok := e.trunks[event.NPC]
	if !ok || trunk.killer != player {
		return true
	}
	axe := e.w.findBestUsableAxe(player)
	if axe == nil {
		player.SendMessage(Ents.Messages.NoAxe)
		return true
	}
	inventory := player.Inventory()
	if inventory == nil || inventory.GetFreeSlots() <= 0 {
		player.SendMessage(Ents.Messages.Full)
		return true
	}
	player.SendMessage(Ents.Messages.Swing)
	player.PerformAnimation(axe.AnimationID, 0)
	e.startSession(player, &entChopSession{
		npc:  event.NPC,
		axe:  axe,
		next: e.cycle + int64(Ents.ChopTicks),
		from: player.Location(),
	})
	return true
}

// giveLogs pays one successful trunk chop
// (EntTrunk.Woodcutting.js:102-116, giveLogs), in exact TS
// order: roll the log (nil rolls true — the session
// continues); the noted log id; a full inventory that does
// not already hold the noted log sends the full line and
// returns false (the session ends); otherwise the logs
// (1 guild / 2 wilderness) land, 25 XP times the lumberjack
// multiplier, the yield line, the bird-nest roll, and the
// woodcutting:ent-logs emit (with the UNNOTED log id and
// the Ent's location label).
func (e *entTrunk) giveLogs(player *game.Player, trunk *entTrunkState, axe *Axe) bool {
	log := rollEntLog(maxWoodcuttingLevel(player), axe, e.w.randFloat)
	if log == nil {
		return true
	}
	itemID := e.notedLogID(player, log.ID)
	inventory := player.Inventory()
	if inventory == nil {
		return false
	}
	if inventory.GetFreeSlots() <= 0 && !inventory.Contains(itemID) {
		player.SendMessage(Ents.Messages.Full)
		return false
	}
	inventory.Adds(itemID, trunk.ent.LogsPerSuccess)
	xp := int(float64(Ents.XP) * e.w.lumberjackMultiplier(player))
	if skills := player.Skills(); skills != nil {
		skills.AddExperiences(game.SkillWoodcutting, xp)
	}
	player.SendMessage(strings.NewReplacer(
		"{amount}", strconv.Itoa(trunk.ent.LogsPerSuccess),
		"{log}", log.Name,
	).Replace(Ents.Messages.Yield))
	e.w.maybeDropBirdNest(player)
	e.w.emitEvent(EntLogsEventName, &EntLogsEvent{
		Player: player,
		ItemID: log.ID,
		Where:  trunk.ent.Where,
	})
	return true
}

// tick is the module's own 1-tick task
// (EntTrunk.Woodcutting.js:118-133, tick): transforms due
// this cycle (HP 1, HP locked, poison cleared, transform
// to the trunk, trunk registered until now+trunkTicks),
// trunk expiry (at its until, or when the NPC left the
// world), then one chop attempt per session due this cycle
// (trunk gone / player unregistered / player moved off
// the chop tile ends the session; every three ticks the
// axe swings and the cut chance rolls against the Ent
// chart; a failed giveLogs ends the session).
func (e *entTrunk) tick() {
	e.cycle++
	var remaining []entTransform
	for _, p := range e.pending {
		if e.cycle < p.due {
			remaining = append(remaining, p)
			continue
		}
		// Nothing walks to a 0 HP NPC, so the trunk couldn't
		// be chopped: a locked 1 HP, as the other remains
		// (RuniteGolem.plugin.js, Araxxor's corpse). The
		// respawn is a new NPC.
		p.npc.Hitpoints = 1
		p.npc.SetHitpointsLocked(true)
		p.npc.SetPoisonDamage(0)
		p.npc.SetNpcTransformationID(Ents.TrunkID)
		e.trunks[p.npc] = &entTrunkState{
			killer: p.killer,
			ent:    p.ent,
			until:  e.cycle + int64(Ents.TrunkTicks),
		}
	}
	e.pending = remaining
	for npc, trunk := range e.trunks {
		if e.cycle >= trunk.until || !npc.IsRegistered() {
			delete(e.trunks, npc)
		}
	}
	for player, session := range e.sessions {
		trunk := e.trunks[session.npc]
		if trunk == nil || !player.IsRegistered() || !player.Location().Equals(session.from) {
			e.endSession(player)
			continue
		}
		if e.cycle < session.next {
			continue
		}
		session.next = e.cycle + int64(Ents.ChopTicks)
		player.PerformAnimation(session.axe.AnimationID, 0)
		level := woodcuttingLevel(player)
		if e.w.randFloat() >= calculateCutChance(level, entCutChanceTree, session.axe) {
			continue
		}
		if !e.giveLogs(player, trunk, session.axe) {
			e.endSession(player)
		}
	}
}

// forget clears a disconnecting player's session
// (EntTrunk.Woodcutting.js:135-137, forget): the
// no-retaliate flag clears with it.
func (e *entTrunk) forget(event *plugins.PlayerDisconnectEvent) {
	if event == nil || event.Player == nil {
		return
	}
	e.endSession(event.Player)
}
