// AnglerOutfit is the Go port of plugins/skills/fishing/
// AnglerOutfit.Fishing.js (commit bfd81e3): the angler's
// outfit Fishing XP bonus — hat 0.4%, top 0.8%, waders
// 0.6%, boots 0.2%, plus 0.5% for all four, so 2.5% more
// Fishing XP — with the spirit angler pieces counting the
// same, and the full-set text in the equipment effects
// panel (enum 3047: angler 16, spirit 28 — the spirit text
// only when every piece is spirit).
//
// The Gita Prymes exchange is DEFERRED (PHASE35_PLAN.md
// §7): it needs onItemOnNpc (table-only — no Go family
// exists) plus the npc-dialogue:* custom events and the
// Gita Prymes transcript page, none of which exist in Go.
// Nothing for Gita is registered here; the exchange is a
// stub comment only, not a half-built hook.
//
// The set-text handler follows the Woodcutting Lumberjack
// precedent (woodcutting.go addLumberjackSetText): the
// producer (EquipmentStats) is NOT STARTED in Go, so the
// handler is registered but never fires. The XP multiplier
// itself does not depend on the producer — the core loop
// calls anglerXPMultiplier directly per catch.
package fishing

import (
	"tsps-go/internal/game"
	"tsps-go/internal/plugins"
)

// ---- AnglerOutfit constants (AnglerOutfit.Fishing.js) ----

const (
	// anglerHatBonus is the hat's XP bonus (pieces: hat,
	// bonus 0.004).
	anglerHatBonus = 0.004
	// anglerTopBonus is the top's XP bonus (0.008).
	anglerTopBonus = 0.008
	// anglerWadersBonus is the waders' XP bonus (0.006).
	anglerWadersBonus = 0.006
	// anglerBootsBonus is the boots' XP bonus (0.002).
	anglerBootsBonus = 0.002
	// anglerSetBonus is the full-set bonus (SET_BONUS,
	// 0.005).
	anglerSetBonus = 0.005
	// anglerSetText is the full angler set text
	// (SET_TEXT.angler, enum 3047: 16).
	anglerSetText = 16
	// spiritAnglerSetText is the full spirit set text
	// (SET_TEXT.spirit, enum 3047: 28).
	spiritAnglerSetText = 28
)

// anglerPiece is one outfit piece: its equipment slot, the
// ids that count (angler, then spirit), and its XP bonus.
type anglerPiece struct {
	slot  int
	ids   [2]int
	bonus float64
}

// anglerPieces builds the four pieces from the resolved ids
// (AnglerOutfit.Fishing.js attach: pieces). Nil ids read as
// no pieces, so the multiplier degrades to 1 and the full
// set to false — fail-closed, as with nil tables elsewhere.
func (f *fishing) anglerPieces() []anglerPiece {
	if f == nil || f.ids == nil {
		return nil
	}
	ids := f.ids
	return []anglerPiece{
		{slot: game.HeadSlot, ids: [2]int{ids.AnglerHat, ids.SpiritAnglerHeadband}, bonus: anglerHatBonus},
		{slot: game.BodySlot, ids: [2]int{ids.AnglerTop, ids.SpiritAnglerTop}, bonus: anglerTopBonus},
		{slot: game.LegSlot, ids: [2]int{ids.AnglerWaders, ids.SpiritAnglerWaders}, bonus: anglerWadersBonus},
		{slot: game.FeetSlot, ids: [2]int{ids.AnglerBoots, ids.SpiritAnglerBoots}, bonus: anglerBootsBonus},
	}
}

// anglerWornPieces returns the pieces the player is wearing
// (wornPieces, AnglerOutfit.Fishing.js): a piece counts when
// the slot item's id is one of its two ids.
func (f *fishing) anglerWornPieces(player *game.Player) []anglerPiece {
	if player == nil {
		return nil
	}
	equipment := player.Equipment()
	if equipment == nil {
		return nil
	}
	var worn []anglerPiece
	for _, piece := range f.anglerPieces() {
		wornID := equipment.ItemAt(piece.slot).ID()
		if wornID == piece.ids[0] || wornID == piece.ids[1] {
			worn = append(worn, piece)
		}
	}
	return worn
}

// anglerXPMultiplier is the angler outfit's Fishing XP
// multiplier (xpMultiplier, AnglerOutfit.Fishing.js): 1 +
// the worn pieces' bonuses + the full-set bonus when all
// four are worn. Spirit pieces count the same as angler
// pieces.
func (f *fishing) anglerXPMultiplier(player *game.Player) float64 {
	pieces := f.anglerPieces()
	if len(pieces) == 0 {
		return 1
	}
	worn := f.anglerWornPieces(player)
	bonus := 0.0
	for _, piece := range worn {
		bonus += piece.bonus
	}
	if len(worn) == len(pieces) {
		bonus += anglerSetBonus
	}
	return 1 + bonus
}

// anglerWearsFullOutfit reports whether the player wears
// the whole angler outfit (wearsFullOutfit,
// AnglerOutfit.Fishing.js), spirit pieces included. Kylie
// Minnow's platform check reads this too (deferred with
// the platform itself).
func (f *fishing) anglerWearsFullOutfit(player *game.Player) bool {
	pieces := f.anglerPieces()
	if len(pieces) == 0 {
		return false
	}
	return len(f.anglerWornPieces(player)) == len(pieces)
}

// addAnglerSetText adds the full-outfit set text to an
// "equipment:effects" request (addSetText,
// AnglerOutfit.Fishing.js): the spirit text only when every
// piece is spirit, else the angler text. The producer
// (EquipmentStats) is NOT STARTED in Go, so this handler
// is registered but never fires — the GracefulOutfit /
// Lumberjack precedent.
func (f *fishing) addAnglerSetText(payload any) {
	request, ok := payload.(*game.EquipmentEffectsRequest)
	if !ok || request == nil || request.Player == nil {
		return
	}
	if !f.anglerWearsFullOutfit(request.Player) {
		return
	}
	pieces := f.anglerPieces()
	equipment := request.Player.Equipment()
	if equipment == nil {
		return
	}
	spirit := true
	for _, piece := range pieces {
		if equipment.ItemAt(piece.slot).ID() != piece.ids[1] {
			spirit = false
			break
		}
	}
	if spirit {
		request.SetTexts = append(request.SetTexts, spiritAnglerSetText)
		return
	}
	request.SetTexts = append(request.SetTexts, anglerSetText)
}

// registerAnglerOutfit wires the AnglerOutfit registration
// (AnglerOutfit.Fishing.js attach): the equipment:effects
// set text only. The Gita Prymes exchange is DEFERRED —
// onItemOnNpc has no Go family and the npc-dialogue:*
// events have no Go emitter — so nothing for Gita is
// registered here.
func (f *fishing) registerAnglerOutfit(api *plugins.Api) {
	if api == nil {
		return
	}
	api.OnCustomEvent(game.EquipmentEffectsEventName, f.addAnglerSetText)
}
