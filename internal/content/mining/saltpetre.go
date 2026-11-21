// Saltpetre deposits in Hosidius (Saltpetre.Mining.js).
// Digging needs a spade, gives no Mining experience, and
// yields 1 saltpetre, or 2 with a chance roughly equal to the
// player's Mining level out of 100.
package mining

import (
	"tsps-go/internal/game"
	"tsps-go/internal/plugins"
)

// Saltpetre constants (Saltpetre.Mining.js:14-21).
const (
	saltpetreDigAnimation = 830
	saltpetreDigSound     = 1470
	saltpetreDigTicks     = 2
	saltpetreItemID       = 13421 // ItemIds.SALTPETRE
	saltpetreNeedSpade    = "You need a spade to dig here."
	saltpetreNoSpace      = "You don't have enough inventory space."
)

// saltpetreObjectIDs are the deposit loc ids
// (Saltpetre.Mining.js:17).
var saltpetreObjectIDs = map[int]bool{
	27433: true,
	27434: true,
	27435: true,
	27436: true,
}

// spadeNames are the spade item names (Saltpetre.Mining.js:18).
var spadeNames = map[string]bool{
	"Spade": true, "Large spade": true, "Eastfloor spade": true,
}

// hasSpade reports whether the player carries a spade
// (Saltpetre.Mining.js:27-30).
func (m *mining) hasSpade(player *game.Player) bool {
	if player == nil || m.tables == nil {
		return false
	}
	inv := player.Inventory()
	if inv == nil {
		return false
	}
	for _, item := range inv.Items() {
		if item.Amount() > 0 && spadeNames[m.tables.ItemName(item.ID())] {
			return true
		}
	}
	return false
}

// saltpetreAmount is the yield: 2 with a chance equal to the
// visible Mining level out of 100 (Saltpetre.Mining.js:33-36).
func (m *mining) saltpetreAmount(player *game.Player) int {
	if m.randFloat()*100 < float64(miningLevel(player)) {
		return 2
	}
	return 1
}

// dig handles Dig on a saltpetre deposit
// (Saltpetre.Mining.js:38-72).
func (m *mining) digSaltpetre(event *plugins.ObjectInteractionEvent) bool {
	if event == nil || event.Player == nil || event.Object == nil {
		return false
	}
	if !saltpetreObjectIDs[event.Object.ID()] {
		return false
	}
	player := event.Player
	event.Handled = true
	if !m.hasSpade(player) {
		player.SendMessage(saltpetreNeedSpade)
		return true
	}
	if player.Inventory().IsFull() {
		player.SendMessage(saltpetreNoSpace)
		return true
	}
	player.PerformAnimation(saltpetreDigAnimation, 0)
	game.SendSound(player, game.NewSound(saltpetreDigSound, 1, 0, 0))
	// The 2-tick dig task. TS cancels the player's tasks first
	// (TaskManager.cancelTasks); Go has no per-player task
	// cancel, so a token gives the same replace semantics: a
	// second dig retires the first one's task.
	token := m.nextDigToken(player)
	object := event.Object
	m.scheduleDig(player, token, object)
	return true
}

// digTokens are the pending-dig tokens per player (see dig).
var digTokens = map[*game.Player]int{}

// nextDigToken issues a fresh dig token for the player.
func (m *mining) nextDigToken(player *game.Player) int {
	token := digTokens[player] + 1
	digTokens[player] = token
	return token
}

// scheduleDig schedules the dig's completion
// (Saltpetre.Mining.js:55-70).
func (m *mining) scheduleDig(player *game.Player, token int, object plugins.GameObject) {
	tm := m.taskManager()
	if tm == nil {
		return
	}
	task := game.NewTask(saltpetreDigTicks, func(cycle int64) bool {
		if digTokens[player] != token {
			return true // superseded by a newer dig
		}
		delete(digTokens, player)
		if player == nil || !player.IsRegistered() || player.Skills().CurrentLevel(game.SkillHitpoints) <= 0 ||
			!m.hasSpade(player) || player.PrivateArea() != objectPrivateArea(object) {
			return true
		}
		if !withinInteractionDistance(object, player.Location()) {
			return true
		}
		if player.Inventory().IsFull() {
			player.SendMessage(saltpetreNoSpace)
			return true
		}
		count := player.Inventory().GetAddableAmount(game.NewItem(saltpetreItemID, m.saltpetreAmount(player)))
		if count <= 0 {
			return true
		}
		player.Inventory().Adds(saltpetreItemID, count)
		player.SendMessage("You dig up some saltpetre.")
		m.emitEvent("mining:saltpetre", &saltpetreEvent{Player: player, Amount: count})
		return true
	})
	task.Name = "MiningSaltpetreDig"
	tm.Schedule(task)
}

// saltpetreEvent is the "mining:saltpetre" payload.
type saltpetreEvent struct {
	Player *game.Player
	Amount int
}

// registerSaltpetre wires the Saltpetre registration
// (Saltpetre.Mining.js:74-78).
func (m *mining) registerSaltpetre(api *plugins.Api) {
	if api == nil {
		return
	}
	api.OnObjectInteractionNamed("Saltpetre", map[string]plugins.ObjectInteractionHandler{"Dig": m.digSaltpetre})
}
