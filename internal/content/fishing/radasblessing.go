// RadasBlessing is the Go port of plugins/skills/fishing/
// RadasBlessing.Fishing.js (commit bfd81e3): Rada's
// blessing 1 to 4, worn in the ammunition slot, give a 2%,
// 4%, 6% or 8% chance to catch two fish at once, with no
// additional experience. It is a members' benefit: a
// blessing saved as worn stays on a free world, where
// FreeToPlay only blocks equipping it — so the members
// check is on the world, not the item (the same shape as
// Woodcutting's logsPerChop).
//
// rollExtraFish rolls the extra fish for one landed fish:
// "flakes" for the first flakeChance of the roll,
// "blessing" for the worn tier's chance on top of it (the
// two stack), else "". The junk filter lives in the core
// loop, not here: the loop only rolls extra fish for real
// fish, never for junk. No hooks are registered here: the
// loop calls rollExtraFish per landed fish.
package fishing

import (
	"math/rand/v2"

	"tsps-go/internal/game"
)

// RadaBlessingExtraFishMessage is the extra-fish line
// (EXTRA_FISH_MESSAGE, RadasBlessing.Fishing.js): the chat
// channel is Fishing's own — no capture of it exists, so
// the loop sends it as a spam line.
const RadaBlessingExtraFishMessage = "Rada's blessing enabled you to catch an extra fish."

// radasBlessingChance is one worn tier's extra-fish chance:
// the blessing id and its chance, tiers 4 to 1.
type radasBlessingChance struct {
	id     int
	chance float64
}

// radasBlessingChances builds the tier table from the
// resolved ids (attach, RadasBlessing.Fishing.js): 4 to 1
// at 8/6/4/2%. Nil ids read as no blessing.
func (f *fishing) radasBlessingChances() []radasBlessingChance {
	if f == nil || f.ids == nil {
		return nil
	}
	return []radasBlessingChance{
		{id: f.ids.RadasBlessing4, chance: 0.08},
		{id: f.ids.RadasBlessing3, chance: 0.06},
		{id: f.ids.RadasBlessing2, chance: 0.04},
		{id: f.ids.RadasBlessing1, chance: 0.02},
	}
}

// radasExtraFishChance is the worn tier's chance of a
// second fish, 0 without a blessing (extraFishChance,
// RadasBlessing.Fishing.js). It is a members' benefit: 0
// on a free world (f.membersWorld, installed through
// SetMembersWorld before hub Load). The first tier found
// in 4→1 order wins.
func (f *fishing) radasExtraFishChance(player *game.Player) float64 {
	if f == nil || !f.membersWorld || player == nil {
		return 0
	}
	equipment := player.Equipment()
	if equipment == nil {
		return 0
	}
	for _, tier := range f.radasBlessingChances() {
		if equipment.Contains(tier.id) {
			return tier.chance
		}
	}
	return 0
}

// rollExtraFish rolls the extra fish for one landed fish
// (rollExtraFish, RadasBlessing.Fishing.js): "flakes" for
// the first flakeChance of the roll, "blessing" for the
// worn tier's chance on top of it (the two stack), else "".
// flakeChance is the spirit-flakes chance the loop passes
// in (SpiritFlakeExtraFishChance when a flake was consumed,
// else 0).
func (f *fishing) rollExtraFish(player *game.Player, flakeChance float64) string {
	roll := rand.Float64()
	if roll < flakeChance {
		return "flakes"
	}
	if roll < flakeChance+f.radasExtraFishChance(player) {
		return "blessing"
	}
	return ""
}
