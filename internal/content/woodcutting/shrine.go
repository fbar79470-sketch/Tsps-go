// The Woodcutting Guild's shrine: the Go port of
// server/plugins/skills/woodcutting/Shrine.Woodcutting.js (53
// lines, commit bfd81e3). A bird's egg used on the shrine
// becomes a bird nest with seeds in the same slot (100 Prayer
// XP, the offering count, animation 3705, a projectile and a
// sound in the egg's colour), with a 1/1200 chance of each
// evil chicken piece instead of the nest. Data lives in
// guilddata.go (Shrine). Praying at the shrine is the Altars
// plugin's — NOT built here.
//
// Line numbers in comments cite Shrine.Woodcutting.js unless
// another file is named.
package woodcutting

import (
	"strconv"
	"strings"

	"tsps-go/internal/game"
	"tsps-go/internal/plugins"
)

// entShrine is the shrine unit's per-registration state. It
// holds only the owning woodcutting instance (for the emit
// func and the shared randomness); the offerings count is a
// persisted player attribute, not unit state.
type entShrine struct {
	w *woodcutting
}

// newEntShrine builds a shrine unit over the woodcutting
// instance that owns the emit func.
func newEntShrine(w *woodcutting) *entShrine {
	return &entShrine{w: w}
}

// register wires the unit's hooks, attach-only: the
// persisted offerings attribute (starting at 0) and the
// named item-on-object hook ("Bird's egg" on "Shrine").
func (s *entShrine) register(api *plugins.Api) {
	api.PersistAttribute(ShrineOfferingsAttribute, 0)
	api.OnItemOnObjectNamed("Bird's egg", "Shrine", s.offerEgg, nil)
}

// shrineReward rolls the offering's reward
// (Shrine.Woodcutting.js:12-16, reward): one of the four
// evil chicken pieces at 1/1200 each, otherwise the seed
// nest. Each piece rolls in JSON order; the first hit wins.
func shrineReward(randFloat func() float64) int {
	for _, piece := range Shrine.EvilChickenPieces {
		if randFloat()*float64(Shrine.EvilChickenChance) < 1 {
			return piece
		}
	}
	return Shrine.Nest
}

// shrineOfferingsCount reads the player's offerings count
// (offerEgg's read, Shrine.Woodcutting.js:30): an int
// attribute, 0 when absent or of another type — the
// logsChoppedCount pattern.
func shrineOfferingsCount(player *game.Player) int {
	if player == nil {
		return 0
	}
	switch v := player.GetAttribute(ShrineOfferingsAttribute).(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	default:
		return 0
	}
}

// offerEgg handles a bird's egg used on the shrine
// (Shrine.Woodcutting.js:18-48, offerEgg), in exact TS
// order: an egg the table does not know, or an object that
// is not the shrine, falls through (false); a slot that no
// longer holds the egg claims the use silently (true, no
// reward — the TS slot-identity check); otherwise the slot
// item is replaced with the reward, the inventory
// refreshes, 100 Prayer XP lands, the offerings count
// increments and the offer + count lines send, the
// offering animation plays, the egg-coloured projectile
// flies from the shrine tile, and the egg's sound plays.
func (s *entShrine) offerEgg(event *plugins.ItemOnObjectEvent) bool {
	if event == nil || event.Player == nil {
		return false
	}
	egg, ok := Shrine.Eggs[event.ItemID]
	if !ok || event.ObjectID != Shrine.ID {
		return false
	}
	player := event.Player
	inventory := player.Inventory()
	if inventory == nil {
		return true
	}
	if inventory.ItemAt(event.ItemSlot).ID() != event.ItemID {
		return true
	}
	inventory.SetItem(event.ItemSlot, game.NewItem(shrineReward(s.w.randFloat), 1))
	inventory.RefreshItems()
	if skills := player.Skills(); skills != nil {
		skills.AddExperiences(game.SkillPrayer, Shrine.XP)
	}
	count := shrineOfferingsCount(player) + 1
	player.SetAttribute(ShrineOfferingsAttribute, count)
	player.SendMessage(Shrine.Messages.Offer)
	if count == 1 {
		player.SendMessage(Shrine.Messages.CountOne)
	} else {
		player.SendMessage(strings.NewReplacer(
			"{count}", strconv.Itoa(count),
		).Replace(Shrine.Messages.Count))
	}
	player.PerformAnimation(Shrine.Anim, 0)
	loc := event.Location
	proj := Shrine.Projectile
	area := (*game.Area)(nil)
	if pa := player.PrivateArea(); pa != nil {
		area = pa.Area
	}
	game.NewProjectile(
		game.Location{X: loc.X + proj.FromX, Y: loc.Y + proj.FromY, Z: loc.Z},
		game.Location{X: loc.X + proj.ToX, Y: loc.Y + proj.ToY, Z: loc.Z},
		nil, egg.Projectile, proj.StartCycle, proj.EndCycle,
		float64(proj.StartHeight), float64(proj.EndHeight), area,
	).WithAngle(proj.Angle).SendProjectile()
	game.SendSoundEffect(player, egg.Sound, 1, 0, 1)
	return true
}
