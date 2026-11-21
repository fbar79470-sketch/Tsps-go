// Prospector outfit (ProspectorOutfit.Mining.js): helmet 0.4%,
// jacket 0.8%, legs 0.6%, boots 0.2% more Mining XP, plus 0.5%
// for all four, so 2.5%. The golden recolours count the same,
// and a mix of both still completes the set. Varrock armour 4
// acts as a prospector jacket. Pieces are matched by cache
// name, so every id the cache gives a piece counts.
package mining

import (
	"strings"

	"tsps-go/internal/game"
	"tsps-go/internal/plugins"
)

// prospectorSetBonus is the full-set bonus
// (ProspectorOutfit.Mining.js:11).
const prospectorSetBonus = 0.005

// prospectorSetText is the full outfit in the equipment effects
// panel (enum 3047) (ProspectorOutfit.Mining.js:13).
const prospectorSetText = 18

// prospectorPiece is one outfit piece
// (ProspectorOutfit.Mining.js:56-61).
type prospectorPiece struct {
	name  string
	slot  int
	bonus float64
	stand int // id that stands in for the piece (Varrock armour 4 for the jacket)
}

var prospectorPieces = []prospectorPiece{
	{name: "prospector helmet", slot: game.HeadSlot, bonus: 0.004},
	{name: "prospector jacket", slot: game.BodySlot, bonus: 0.008, stand: 13107},
	{name: "prospector legs", slot: game.LegSlot, bonus: 0.006},
	{name: "prospector boots", slot: game.FeetSlot, bonus: 0.002},
}

// prospectorPieceFor resolves an equipped item to its piece name
// (pieceFor, ProspectorOutfit.Mining.js:20-28): the cache name,
// lowercased, with a leading "golden " stripped.
func (m *mining) prospectorPieceFor(item game.Item) string {
	if !item.IsValid() || m.tables == nil {
		return ""
	}
	name := strings.ToLower(m.tables.ItemName(item.ID()))
	name = strings.TrimPrefix(name, "golden ")
	for _, piece := range prospectorPieces {
		if piece.name == name {
			return piece.name
		}
	}
	return ""
}

// prospectorWornPieces returns the pieces worn, by name: the
// jacket slot also takes Varrock armour 4 (wornPieces,
// ProspectorOutfit.Mining.js:31-38).
func (m *mining) prospectorWornPieces(player *game.Player) []prospectorPiece {
	if player == nil || player.Equipment() == nil {
		return nil
	}
	var worn []prospectorPiece
	for _, piece := range prospectorPieces {
		item := player.Equipment().ItemAt(piece.slot)
		if m.prospectorPieceFor(item) == piece.name || (piece.stand != 0 && item.ID() == piece.stand) {
			worn = append(worn, piece)
		}
	}
	return worn
}

// prospectorMultiplier is the factor on a rock's Mining XP: 1
// with nothing worn (xpMultiplier,
// ProspectorOutfit.Mining.js:41-46).
func (m *mining) prospectorMultiplier(player *game.Player) float64 {
	worn := m.prospectorWornPieces(player)
	bonus := 0.0
	for _, piece := range worn {
		bonus += piece.bonus
	}
	multiplier := 1 + bonus
	if len(worn) == len(prospectorPieces) {
		multiplier += prospectorSetBonus
	}
	return multiplier
}

// xpMultiplierRequest is the "mining:xp-multiplier" payload.
type xpMultiplierRequest struct {
	Player     *game.Player
	Multiplier float64
}

// answerProspectorMultiplier answers "mining:xp-multiplier"
// (ProspectorOutfit.Mining.js:49-51): Mining XP earned outside
// a rock session asks for the outfit.
func (m *mining) answerProspectorMultiplier(payload any) {
	req, ok := payload.(*xpMultiplierRequest)
	if !ok || req == nil || req.Player == nil {
		return
	}
	req.Multiplier *= m.prospectorMultiplier(req.Player)
}

// addProspectorSetText adds the full-outfit set text to an
// "equipment:effects" request (ProspectorOutfit.Mining.js:53-55;
// the woodcutting addLumberjackSetText precedent). The producer
// (EquipmentStats) is NOT STARTED in Go, so this handler is
// registered but never fires.
func (m *mining) addProspectorSetText(payload any) {
	req, ok := payload.(*game.EquipmentEffectsRequest)
	if !ok || req == nil || req.Player == nil {
		return
	}
	if len(m.prospectorWornPieces(req.Player)) == len(prospectorPieces) {
		req.SetTexts = append(req.SetTexts, prospectorSetText)
	}
}

// registerProspectorOutfit wires the ProspectorOutfit
// registrations (ProspectorOutfit.Mining.js:63-67).
func (m *mining) registerProspectorOutfit(api *plugins.Api) {
	if api == nil {
		return
	}
	api.OnCustomEvent("mining:xp-multiplier", m.answerProspectorMultiplier)
	api.OnCustomEvent("equipment:effects", m.addProspectorSetText)
}
