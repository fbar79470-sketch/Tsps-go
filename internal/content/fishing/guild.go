// Guild is the Go port of the Fishing Guild's invisible
// boost (plugins/skills/fishing/Guild.Fishing.js, commit
// bfd81e3): inside the guild, an invisible +7 Fishing
// boost applies to catch rolls. The core loop adds it to
// catchLevel (plugin.js:~655; Guild.Fishing.js:22-28).
//
// The guild's map region runs from the door wall up to the
// docks (x 2560–2623, y 3394–3436, z 0). It holds only the
// guild's own spots, so these bounds stand in for the
// fenced grounds. Kylie Minnow's platform (y 3437 up) is
// not part of the guild and gets no boost.
//
// The Guild DOOR is DEFERRED (PHASE35_PLAN.md §7): it
// needs the `door:toggle` custom event plus
// ObstacleRunner.run (one tile per tick through the
// doorway), and no ForceMovement system exists in Go (gap
// 16). Door tile (2611,3394,0), level 68 (boosts count),
// object id 422. Nothing for the door is registered here;
// the door is a stub comment only, not a half-built hook.
package fishing

import (
	"tsps-go/internal/game"
)

// ---- Guild constants (Guild.Fishing.js:7-18) ----

const (
	// guildInvisibleBoostValue is INVISIBLE_BOOST: +7
	// Fishing inside the guild.
	guildInvisibleBoostValue = 7
	// guildMinX / guildMaxX / guildMinY / guildMaxY /
	// guildZ are GUILD_BOUNDS: the guild's map region, from
	// the door wall (y 3394) up to the docks (y 3436).
	guildMinX = 2560
	guildMaxX = 2623
	guildMinY = 3394
	guildMaxY = 3436
	guildZ    = 0
)

// isInGuild reports whether the player is inside the
// Fishing Guild (isInGuild, Guild.Fishing.js): the
// location within the guild bounds, z 0.
func (f *fishing) isInGuild(player *game.Player) bool {
	if player == nil {
		return false
	}
	loc := player.Location()
	return loc.Z == guildZ && loc.X >= guildMinX && loc.X <= guildMaxX &&
		loc.Y >= guildMinY && loc.Y <= guildMaxY
}

// guildInvisibleBoost is the guild's invisible Fishing
// boost (invisibleBoost, Guild.Fishing.js): +7 inside the
// guild, 0 outside. The core loop adds it to catchLevel.
func (f *fishing) guildInvisibleBoost(player *game.Player) int {
	if f.isInGuild(player) {
		return guildInvisibleBoostValue
	}
	return 0
}
