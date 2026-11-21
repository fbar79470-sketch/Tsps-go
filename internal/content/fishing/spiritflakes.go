// SpiritFlakes is the Go port of plugins/skills/fishing/
// SpiritFlakes.Fishing.js (commit bfd81e3): carried spirit
// flakes give a 50% chance to catch an extra fish, for no
// additional experience, and the effect stacks with Rada's
// blessing. "Players will always lose a flake per fish
// caught, regardless of whether they get a single or double
// catch."
//
// consumeFlake uses up one carried flake for a landed fish;
// true when there was one. Flakes in a tackle box are not
// used: the Wiki does not say they are. No hooks are
// registered here: the core loop calls consumeFlake per
// landed fish (not for the park net) and passes the result
// into rollExtraFish as the flake chance.
package fishing

import (
	"tsps-go/internal/game"
)

// SpiritFlakeExtraFishChance is EXTRA_FISH_CHANCE
// (SpiritFlakes.Fishing.js): carried spirit flakes give a
// 50% chance of an extra fish.
const SpiritFlakeExtraFishChance = 0.5

// consumeFlake uses up one carried spirit flake for a
// landed fish (consumeFlake, SpiritFlakes.Fishing.js);
// true when there was one. Nil ids read as no flakes.
func (f *fishing) consumeFlake(player *game.Player) bool {
	if player == nil || f == nil || f.ids == nil {
		return false
	}
	inv := player.Inventory()
	if inv == nil {
		return false
	}
	if !inv.Contains(f.ids.SpiritFlakes) {
		return false
	}
	inv.DeleteNumber(f.ids.SpiritFlakes, 1)
	return true
}
