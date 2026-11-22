// Enchant Jewellery: the plugin, the per-player repeat loop
// and its wiring (Spells.EnchantJewellery.js attach /
// keepEnchanting / stopEnchanting / pauseEnchanting).
//
// THE LOOP SHAPE: the smithing sessions.go precedent — a
// per-player sessions map plus ONE global 1-tick TaskManager
// task (ensureTask / processTick). The task's Action receives
// the world cycle from TaskManager.Process, so the loop paces
// on the same clock the click's event.Cycle and the cooldown
// attributes use (no second counter). A session casts at
// nextCastAt, then every repeat interval; it ends when a cast
// fails (out of runes, out of jewellery, level/spellbook
// refusal), the player walks away (movement queue, force
// movement, or location change from the start tile), dies, or
// logs out. Starting a new cast stops any existing session
// first, exactly as TS stopEnchanting does on every accepted
// click.
//
// RESIDUALS (recorded, not silent):
//   - (a) XP truncates to int at the grant site (17.5 -> 17):
//     the known integer-truncation gap; XP math is not changed.
//   - (b) No SkillLevelUp emitter exists in Go, so the
//     level-up pop-up never pauses a repeat here: a repeat
//     continues through a level-up, where TS pauses it until
//     the pop-up closes. The pauseSessions handler is
//     registered anyway (the smithing precedent), exactly as
//     TS registers pauseEnchanting, so it starts working the
//     moment the emitter lands.
//   - (c) The "magic:enchant-item" fallback (the Mage Training
//     Arena's shapes, cast with this spell's runes/level/XP)
//     has no Go answerer: an unhandled click reports "This
//     spell cannot be cast on that item.", as TS does when no
//     hook handles it.
//   - (d) The F2P item nuance (FreeToPlay.plugin.js: only the
//     corresponding amulet is free to enchant for Lvl-1–4)
//     rides on the "magic:enchant-item-policy" event, which no
//     Go plugin answers yet; the members-spell gate below is
//     the only F2P enforcement in this package.
package enchantjewellery

import (
	"tsps-go/internal/game"
	"tsps-go/internal/plugins"
	"tsps-go/internal/plugins/skillactions"
)

// enchantSession is one player's active repeating enchant
// (Spells.EnchantJewellery.js keepEnchanting's Task, retained
// in the repeats WeakMap): the spell, the jewellery id, the
// world cycle of the next cast, the tile the loop started on,
// and the level-up pause flag.
type enchantSession struct {
	spell         *enchantSpell
	itemID        int
	nextCastAt    int64
	startLocation game.Location
	levelUpPaused bool
}

// SetLevelUpPaused implements skillactions.Session.
func (s *enchantSession) SetLevelUpPaused(paused bool) { s.levelUpPaused = paused }

// enchanter is the plugin's per-registration state: the TS
// module-level state (api, the repeats map) as struct fields,
// per the Phase 21 race fix. One instance serves the
// registered Plugin value; tests build their own with
// newEnchanter.
type enchanter struct {
	api  *plugins.Api
	emit func(name string, payload any)
	// tasks is the directly-set TaskManager override (unit
	// tests set it); production leaves it nil and resolves
	// through the api instead — a Register-time capture would
	// read nil, because the TaskManager installs after Load.
	tasks *game.TaskManager
	// membersWorld defaults true: the TS no-world fixture path
	// allows everything, and the dev world is a members world.
	// cmd/server installs the real flag through
	// SetMembersWorld before hub Load.
	membersWorld bool
	// sessions maps each enchanting player to their active
	// repeat loop (the smithing sessions-map shape). The
	// global 1-tick task advances them all.
	sessions map[*game.Player]*enchantSession
	// taskScheduled guards ensureTask: the 1-tick global task
	// schedules once, on the first startEnchanting.
	taskScheduled bool
}

// newEnchanter builds an enchanter instance.
func newEnchanter() *enchanter {
	return &enchanter{
		membersWorld: true,
		sessions:     make(map[*game.Player]*enchantSession),
	}
}

// shared is the instance behind Plugin; SetMembersWorld
// operates on it.
var shared = newEnchanter()

func init() {
	plugins.Register(Plugin)
}

// SetMembersWorld installs the members-world flag for the
// registered Plugin value (TS: WorldDefinition.
// isMembersWorld()). cmd/server calls it before hub Load with
// the world definition's flag; it defaults true.
func SetMembersWorld(members bool) { shared.membersWorld = members }

// MembersWorld reports the installed members-world flag (the
// shared instance's view, for tests).
func MembersWorld() bool { return shared.membersWorld }

// ---- plugin ----

var Plugin = shared.plugin()

func (e *enchanter) plugin() plugins.Plugin {
	return plugins.Plugin{
		Name:        "EnchantJewellery",
		Description: "The Lvl-1 to Lvl-7 Enchant spells, which enchant rings, necklaces, bracelets and amulets from the inventory.",
		Members:     false,
		Register: func(api *plugins.Api) {
			e.api = api
			e.emit = api.EmitCustomEvent
			e.register(api)
		},
	}
}

// register wires the hooks (EnchantJewellery.plugin.js
// register, via Spells.EnchantJewellery.js attach): the
// onSpellOnItem family, the level-up pause, the disconnect
// stop, and the startup task (the smithing register shape).
func (e *enchanter) register(api *plugins.Api) {
	api.OnServerStartup(func(*plugins.ServerLifecycleEvent) { e.ensureTask() })
	api.OnPlayerDisconnect(e.disconnect)
	api.OnCustomEvent(skillactions.LevelUpPopupOpenedEventName, e.pauseSessions)
	api.OnSpellOnItem(e.castEnchantOnItem)
}

// disconnect stops the player's repeat loop
// (Spells.EnchantJewellery.js: a Task keyed to the player
// ends with them; the smithing disconnect precedent).
func (e *enchanter) disconnect(event *plugins.PlayerDisconnectEvent) {
	if event == nil {
		return
	}
	e.stopEnchanting(event.Player)
}

// pauseSessions holds sessions through a level-up pop-up
// (Spells.EnchantJewellery.js pauseEnchanting, via
// SkillActions). The emitter does not exist in Go yet, so
// this handler never fires (residual (b)); it is registered
// anyway, exactly as TS does.
func (e *enchanter) pauseSessions(payload any) {
	event, ok := payload.(*skillactions.LevelUpPopupOpenedPayload)
	if !ok || event == nil {
		return
	}
	sessions := make(map[*game.Player]skillactions.Session, len(e.sessions))
	for player, sess := range e.sessions {
		sessions[player] = sess
	}
	skillactions.PauseForLevelUp(event, sessions, func(player *game.Player) {
		e.stopEnchanting(player)
	}, nil)
}

// ---- the repeat loop ----

// emitEvent emits a custom event through the captured emit
// func; a nil emit (before Register) drops the emit, which is
// the no-hooks default for every query (the mining precedent).
func (e *enchanter) emitEvent(name string, payload any) {
	if e == nil || e.emit == nil {
		return
	}
	e.emit(name, payload)
}

// taskManager resolves the world's TaskManager lazily: the hub
// installs it after plugin Load (nil until installed, per the
// hub contract — cmd/server installs it from the built world),
// so a Register-time capture reads nil in production. The live
// Api value wins when present; the directly-set tasks field is
// the unit-test override. (The cooking/fletching taskManager
// pattern, Phase 32 hub-contract lesson: lazy resolution,
// Schedule only on the tick goroutine.)
func (e *enchanter) taskManager() *game.TaskManager {
	if e == nil {
		return nil
	}
	if e.api != nil {
		if tm := e.api.GetTaskManager(); tm != nil {
			return tm
		}
	}
	return e.tasks
}

// ensureTask schedules the 1-tick global EnchantTask once, on
// the lazily-resolved TaskManager. Idempotent. A nil
// TaskManager schedules nothing and retries on the next call
// (the mining / smithing ensureTask shape).
func (e *enchanter) ensureTask() {
	if e.taskScheduled {
		return
	}
	tm := e.taskManager()
	if tm == nil {
		return
	}
	task := game.NewTask(1, func(cycle int64) bool {
		e.processTick(cycle)
		return false
	})
	task.Name = "EnchantJewelleryTask"
	tm.Schedule(task)
	e.taskScheduled = true
}

// startEnchanting starts the repeat loop for a cast accepted
// at cycle, with the first repeat cast at cycle+wait
// (Spells.EnchantJewellery.js keepEnchanting): it stops any
// running loop first, snapshots the start location, and
// registers the session for the global 1-tick task. A wait of
// 0 means the next cast is immediate — the caller casts it
// directly after starting, exactly as TS registers the loop
// before the first cast so its first XP drop can pause it too.
func (e *enchanter) startEnchanting(player *game.Player, spell *enchantSpell, itemID int, cycle int64, wait int) {
	if player == nil || spell == nil {
		return
	}
	e.stopEnchanting(player)
	e.sessions[player] = &enchantSession{
		spell:         spell,
		itemID:        itemID,
		nextCastAt:    cycle + int64(wait),
		startLocation: player.Location(),
	}
	e.ensureTask()
}

// stopEnchanting ends a player's repeat loop
// (Spells.EnchantJewellery.js stopEnchanting). A player with
// no session is a no-op.
func (e *enchanter) stopEnchanting(player *game.Player) {
	if player == nil {
		return
	}
	delete(e.sessions, player)
}

// sessionFor returns the player's active repeat session, or
// nil (tests assert on loop state).
func (e *enchanter) sessionFor(player *game.Player) *enchantSession {
	if e == nil {
		return nil
	}
	return e.sessions[player]
}

// processTick runs one tick of the global EnchantTask (the
// smithing processSmithingTick shape): every session is
// advanced; a session that ends (or whose player is gone,
// dead, or walked off) is stopped.
func (e *enchanter) processTick(cycle int64) {
	for player, sess := range e.sessions {
		if player == nil || !player.IsRegistered() || player.Skills() == nil ||
			player.Skills().CurrentLevel(game.SkillHitpoints) <= 0 {
			e.stopEnchanting(player)
			continue
		}
		// A one-step route can drain between task ticks, so
		// the queue alone misses the walk (the smithing
		// processSmithingTick comment).
		movement := player.MovementQueue()
		if (movement != nil && (movement.Size() > 0 || movement.IsMoving())) || player.ForceMovement() != nil ||
			!player.Location().Equals(sess.startLocation) {
			e.stopEnchanting(player)
			continue
		}
		if sess.levelUpPaused {
			continue
		}
		if cycle < sess.nextCastAt {
			continue
		}
		if !e.enchant(player, sess.spell, sess.itemID, cycle) {
			e.stopEnchanting(player)
			continue
		}
		sess.nextCastAt = cycle + int64(e.repeatTicksFor(player))
	}
}

// repeatTicksFor is the ticks between repeated casts
// (Spells.EnchantJewellery.js repeatTicks): the base 7,
// lowered by content through "skills:action-interval" (a
// league relic's 2-tick pace), never under 1.
func (e *enchanter) repeatTicksFor(player *game.Player) int {
	return skillactions.ActionInterval(e.emitEvent, player, game.SkillMagic, "enchant", repeatTicks, 0)
}

// castTicksFor is a cast's own cooldown in ticks
// (Spells.EnchantJewellery.js castTicks): the base 3, lowered
// by content the same way, never under 1.
func (e *enchanter) castTicksFor(player *game.Player) int {
	return skillactions.ActionInterval(e.emitEvent, player, game.SkillMagic, "enchant", castTicks, 0)
}
