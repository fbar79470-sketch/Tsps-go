package standardeffects

import (
	"tsps-go/internal/game"
	"tsps-go/internal/plugins"
)

// teleblockTick is the per-player Tele Block driver
// (TeleBlock.plugin.js:14-27, tickTeleBlock, via
// onPlayerProcess):
//
//   - Wiki (Teleport Block, Elder Chaos druid): the block is
//     removed when the player leaves the Wilderness. The Ferox
//     Enclave and its buffer are PvP-tagged safe ground, so
//     they still count as the Wilderness here: a blocked player
//     stays blocked at the barrier. That is exactly what
//     Wilderness.IsPvpArea answers (PvP-tagged ground, safe
//     carve-outs included).
//   - rsprox captures (recordings 435, 437): "Your Tele Block
//     will expire in 30 seconds." once, 30 seconds before the
//     block ends.
//   - A block whose time is up is removed.
//
// The apply/remove/warn calls are the engine's
// (game.TeleBlockApply / TeleBlockRemove / TeleBlockWarn); this
// is only the per-tick driver. The effect-timer frame TS sends
// has no Go seam (game.TeleBlockApply comment) and is untouched
// here.
//
// With no Wilderness installed the PvP check cannot be
// evaluated, so the block is kept until expiry (fail-closed):
// in production the Wilderness is always installed.
func (s *standardEffects) teleblockTick(event *plugins.PlayerProcessEvent) {
	if event == nil || event.Player == nil {
		return
	}
	player := event.Player
	if !game.TeleBlockIsBlocked(player) {
		return
	}
	if wilderness := s.resolveWilderness(); wilderness != nil && !wilderness.IsPvpArea(player.Location()) {
		game.TeleBlockRemove(player)
		return
	}
	remaining := game.TeleBlockRemainingSeconds(player)
	if remaining == 0 {
		game.TeleBlockRemove(player)
		return
	}
	if remaining <= game.TeleBlockWarningSeconds {
		if warned, _ := player.GetAttribute(game.TeleBlockWarnedAttribute).(bool); !warned {
			game.TeleBlockWarn(player)
		}
	}
}
