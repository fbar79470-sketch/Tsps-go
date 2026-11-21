// Varrock armour (VarrockArmour.Mining.js): a worn Varrock
// armour gives a 10% chance of an extra ore from eligible
// rocks, gated by armour tier against the ore.
package mining

import (
	"tsps-go/internal/game"
	"tsps-go/internal/plugins"
)

// varrockArmourTierByID maps a Varrock armour id to its tier
// (VarrockArmour.Mining.js:16-19).
var varrockArmourTierByID = map[int]int{
	13104: 1, // VARROCK_ARMOUR_1
	13105: 2, // VARROCK_ARMOUR_2
	13106: 3, // VARROCK_ARMOUR_3
	13107: 4, // VARROCK_ARMOUR_4
}

// varrockArmourTierByOre is the lowest armour tier that adds
// an ore from the rock (VarrockArmour.Mining.js:21-30).
// Sandstone and granite pieces are keyed by each size the rock
// yields.
var varrockArmourTierByOre = map[int]int{
	23905: 1, // TEPHRA
	434:   1, // CLAY
	436:   1, // COPPER_ORE
	438:   1, // TIN_ORE
	668:   1, // BLURITE_ORE
	3211:  1, // LIMESTONE
	440:   1, // IRON_ORE
	442:   1, // SILVER_ORE
	453:   1, // COAL
	444:   1, // GOLD_ORE
	31716: 1, // LEAD_ORE
	447:   2, // MITHRIL_ORE
	13356: 2, // LOVAKITE_ORE
	1761:  2, // SOFT_CLAY
	31722: 2, // RUBIUM_SPLINTERS
	6971:  2, // sandstone 1kg
	6973:  2, // sandstone 2kg
	6975:  2, // sandstone 5kg
	6977:  2, // sandstone 10kg
	6979:  2, // granite 500g
	6981:  2, // granite 2kg
	6983:  2, // granite 5kg
	449:   3, // ADAMANTITE_ORE
	34020: 3, // SUNSTONE
	451:   4, // RUNITE_ORE
	31719: 4, // NICKEL_ORE
	21347: 4, // AMETHYST
}

// varrockExtraOre answers "mining:extra-ore"
// (VarrockArmour.Mining.js:9-14): members only, a worn Varrock
// armour whose tier covers the ore, 10% chance. The bonus
// carries no message (the core's extra-ore line comes from
// bonus.message, which Varrock never sets).
func (m *mining) varrockExtraOre(payload any) {
	req, ok := payload.(*extraOreRequest)
	if !ok || req == nil || req.Player == nil || !m.membersWorld {
		return
	}
	body := req.Player.Equipment().ItemAt(game.BodySlot)
	tier, ok := varrockArmourTierByID[body.ID()]
	if !ok {
		return
	}
	required, ok := varrockArmourTierByOre[req.OreID]
	if !ok || tier < required || m.randFloat() >= 0.1 {
		return
	}
	req.Bonuses = append(req.Bonuses, ExtraOreBonus{})
}

// registerVarrockArmour wires the VarrockArmour registration
// (VarrockArmour.Mining.js:32-34).
func (m *mining) registerVarrockArmour(api *plugins.Api) {
	if api == nil {
		return
	}
	api.OnCustomEvent("mining:extra-ore", m.varrockExtraOre)
}
