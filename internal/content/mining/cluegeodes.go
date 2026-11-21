// Clue geodes (ClueGeodes.Mining.js): every successful mining
// action rolls each clue tier the player does not already own.
// Opening a geode gives the clue scroll.
package mining

import (
	"fmt"

	"tsps-go/internal/game"
	"tsps-go/internal/plugins"
)

// clueGeodeTier is one buildTiers row
// (ClueGeodes.Mining.js:13-21).
type clueGeodeTier struct {
	name     string
	modifier float64
	geodeID  int
	scrollID int
}

// clueGeodeTiers are the five tiers (ClueGeodes.Mining.js:14-20).
var clueGeodeTiers = []clueGeodeTier{
	{name: "beginner", modifier: 0.2, geodeID: 23442, scrollID: 23182},
	{name: "easy", modifier: 1.7, geodeID: 20358, scrollID: 2677},
	{name: "medium", modifier: 2, geodeID: 20360, scrollID: 2801},
	{name: "hard", modifier: 3.3, geodeID: 20362, scrollID: 2722},
	{name: "elite", modifier: 10, geodeID: 20364, scrollID: 12073},
}

// geodeChance is 1-in-this-many for one tier (geodeChance,
// ClueGeodes.Mining.js:24-28): the Wiki formula on the visible
// Mining level.
func geodeChance(petBase, miningLevel int, modifier float64, combatAchievement bool) int {
	base := petBase * 7 / 10
	adjusted := base
	if combatAchievement {
		adjusted = base * 95 / 100
	}
	return adjusted * int(modifier*10+0.5) / ((100 + miningLevel) * 10)
}

// clueVesselChanceRequest is the "treasure-trails:vessel-chance"
// payload.
type clueVesselChanceRequest struct {
	Player     *game.Player
	Vessel     string
	Tier       string
	Multiplier float64
	Allowed    *bool
}

// combatAchievementTierRequest is the "combat-achievements:tier"
// payload.
type combatAchievementTierRequest struct {
	Player              *game.Player
	Tier                string
	CurrentThresholdMet bool
}

// canReceiveClueRequest is the "treasure-trails:can-receive"
// payload.
type canReceiveClueRequest struct {
	Player  *game.Player
	Tier    string
	Allowed *bool
}

// clueItemRequest is the "treasure-trails:clue-item" payload.
type clueItemRequest struct {
	Player *game.Player
	ItemID int
	Box    bool
}

// ownsClue reports whether the player already owns a clue of
// this tier (ownsClue, ClueGeodes.Mining.js:31-46): Treasure
// Trails answers when it can; otherwise the fallback scans the
// inventory and every bank tab for any form of the tier
// ("Clue scroll (beginner)", "Clue bottle (beginner)", "Clue
// nest (beginner)", "Clue geode (beginner)", "Scroll box
// (beginner)") by cache name. Treasure Trails is NOT STARTED
// in Go, so the name scan is the live path; Combat
// Achievements is likewise unported, so the CA factor never
// applies.
func (m *mining) ownsClue(player *game.Player, tier clueGeodeTier) bool {
	if player == nil || m.tables == nil {
		return false
	}
	req := &canReceiveClueRequest{Player: player, Tier: tier.name}
	m.emitEvent("treasure-trails:can-receive", req)
	if req.Allowed != nil {
		return !*req.Allowed
	}
	forms := []string{"Clue scroll", "Clue bottle", "Clue nest", "Clue geode", "Scroll box"}
	names := make(map[string]bool, len(forms))
	for _, form := range forms {
		names[fmt.Sprintf("%s (%s)", form, tier.name)] = true
	}
	contains := func(items []game.Item) bool {
		for _, item := range items {
			if item.Amount() > 0 && names[m.tables.ItemName(item.ID())] {
				return true
			}
		}
		return false
	}
	if inv := player.Inventory(); inv != nil && contains(inv.Items()) {
		return true
	}
	for tab := 0; tab < player.BankCount(); tab++ {
		if bank := player.Bank(tab); bank != nil && contains(bank.Items()) {
			return true
		}
	}
	return false
}

// giveGeode gives the player a clue geode (giveGeode,
// ClueGeodes.Mining.js:48-58): the "treasure-trails:clue-item"
// rewrite (no answerer in Go), then into the inventory —
// falling to the ground when full (there is no forceAdd in Go;
// the giveGem precedent). The find line is a game message.
func (m *mining) giveGeode(player *game.Player, geodeID int) {
	if player == nil {
		return
	}
	req := &clueItemRequest{Player: player, ItemID: geodeID}
	m.emitEvent("treasure-trails:clue-item", req)
	geodeID = req.ItemID
	m.deliverOre(player, func() *game.Item {
		item := game.NewItem(geodeID, 1)
		return &item
	}())
	player.SendMessage("You find a clue geode!")
	m.emitEvent("mining:geode", &geodeEvent{Player: player, ItemID: geodeID})
}

// geodeEvent is the "mining:geode" payload.
type geodeEvent struct {
	Player *game.Player
	ItemID int
}

// rollClueGeodes rolls every tier on a successful mining action
// (rollClueGeodes, ClueGeodes.Mining.js:60-84). Anything that
// emits "mining:success" with a petBase rolls them.
func (m *mining) rollClueGeodes(payload any) {
	event, ok := payload.(*successEvent)
	if !ok || event == nil || event.Player == nil || event.Skill != game.SkillMining || event.PetBase <= 0 {
		return
	}
	level := miningLevel(event.Player)
	for _, tier := range clueGeodeTiers {
		boost := &clueVesselChanceRequest{Player: event.Player, Vessel: "geode", Tier: tier.name, Multiplier: 1}
		m.emitEvent("treasure-trails:vessel-chance", boost)
		if boost.Allowed != nil && !*boost.Allowed {
			continue
		}
		ca := &combatAchievementTierRequest{Player: event.Player, Tier: tier.name}
		if tier.name != "beginner" {
			m.emitEvent("combat-achievements:tier", ca)
		}
		base := geodeChance(event.PetBase, level, tier.modifier, ca.CurrentThresholdMet)
		chance := base
		if chance >= 1 && boost.Multiplier > 1 {
			chance = base / int(boost.Multiplier)
			if chance < 1 {
				chance = 1
			}
		}
		if chance < 1 || int(m.randFloat()*float64(chance)) != 0 {
			continue
		}
		if m.ownsClue(event.Player, tier) {
			m.emitEvent("treasure-trails:missed-clue", &missedClueEvent{Player: event.Player, Tier: tier.name})
			continue
		}
		m.giveGeode(event.Player, tier.geodeID)
	}
}

// missedClueEvent is the "treasure-trails:missed-clue" payload.
type missedClueEvent struct {
	Player *game.Player
	Tier   string
}

// openClueGeode opens a geode for its clue scroll
// (openClueGeode, ClueGeodes.Mining.js:86-101).
func (m *mining) openClueGeode(event *plugins.PluginItemActionEvent) bool {
	if event == nil || event.Player == nil {
		return false
	}
	req := &openVesselRequest{Player: event.Player, ItemID: event.ItemID, Slot: event.Slot}
	m.emitEvent("treasure-trails:open-vessel", req)
	if req.Handled {
		return true
	}
	var tier *clueGeodeTier
	for i := range clueGeodeTiers {
		if clueGeodeTiers[i].geodeID == event.ItemID {
			tier = &clueGeodeTiers[i]
			break
		}
	}
	if tier == nil {
		return false
	}
	inv := event.Player.Inventory()
	if inv == nil {
		return true
	}
	item := inv.ItemAt(event.Slot)
	if item.ID() != tier.geodeID || item.Amount() < 1 {
		return true
	}
	inv.DeleteAtSlot(event.Slot, 1, true)
	inv.Add(game.NewItem(tier.scrollID, 1), true)
	event.Player.SendMessage("You open the geode and find a clue scroll.")
	return true
}

// openVesselRequest is the "treasure-trails:open-vessel" payload.
type openVesselRequest struct {
	Player  *game.Player
	ItemID  int
	Slot    int
	Handled bool
}

// registerClueGeodes wires the ClueGeodes registrations
// (ClueGeodes.Mining.js:103-109).
func (m *mining) registerClueGeodes(api *plugins.Api) {
	if api == nil {
		return
	}
	for _, tier := range clueGeodeTiers {
		name := fmt.Sprintf("Clue geode (%s)", tier.name)
		api.OnItemActionNamed(name, map[string]plugins.ItemActionHandler{"Open": m.openClueGeode}, nil)
	}
	api.OnCustomEvent("mining:success", m.rollClueGeodes)
}
