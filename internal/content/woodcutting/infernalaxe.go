// InfernalAxe is the Go port of plugins/skills/woodcutting/
// InfernalAxe.Woodcutting.js (87 lines, commit bfd81e3): the
// infernal axe's 1/3 chance that each log is burnt as it is
// cut, giving half the Firemaking experience for that log
// and using one of 5,000 charges. Woodcutting experience is
// unaffected, and a burnt log still gets the "You get some
// logs." line from the woodcutting loop. An axe with no
// charges turns into its uncharged form, which cuts like a
// dragon axe. Axes with no charge data yet are treated as
// fully charged.
//
// All ids are ItemIdentifiers.ts values, verified against
// the real rev-242 seed cache (probe, 2026-11-20): charged
// 13241 ("Infernal axe", actions include "Check"), 25066 and
// 30347 ("Infernal axe (or)", "Check"); uncharged 13242,
// 25371, 30348 (all "Infernal axe (uncharged)"). The map is
// verbatim from the unit file: INFERNAL_AXE_2 (14028) is in
// no map.
//
// HONEST DEGRADATION: tryBurnLog emits "firemaking:log-xp"
// and the burn fires only when a handler answers with xp >
// 0 — but Firemaking is NOT STARTED, so no Go answerer
// exists yet, request XP stays unanswered, and the burn
// NEVER fires. The burn is wired into the core loop anyway
// (chopSuccess, before prepareLog, exactly as
// Woodcutting.plugin.js:1410-1413 has it) so it starts
// working the moment a Firemaking port answers the query;
// no answerer is invented here. The same applies to the
// "firemaking:xp-multiplier" query (the Pyromancer outfit
// bonus): it defaults to 1 when unanswered.
//
// Go differences: TS mutates the live Item reference; Go
// Items are values, so mutations write back through the
// container at the carried slot (axeSlot / writeAxeItem in
// crystalaxe.go). Firemaking XP is int in Go (TS stores
// tenths as floats), so the half-XP grant truncates toward
// zero, like the Woodcutting XP truncation in chopSuccess.

package woodcutting

import (
	"fmt"

	"tsps-go/internal/game"
	"tsps-go/internal/plugins"
)

// ---- InfernalAxe constants (InfernalAxe.Woodcutting.js:8-12) ----

const (
	// infernalBurnChance is BURN_CHANCE: the burn roll is
	// 1 in 3 (Math.floor(Math.random() * 3) === 0).
	infernalBurnChance = 3
	// infernalMaxCharges is MAX_CHARGES: an axe holds at
	// most 5,000 charges.
	infernalMaxCharges = 5000
	// infernalChargesMetaKey is CHARGES_META_KEY: the
	// item-meta key holding {"charges": N}.
	infernalChargesMetaKey = "infernal-axe"
	// infernalBurnGraphic is BURN_GRAPHIC: the smokepuff
	// spotanim (86) played on a burnt log.
	infernalBurnGraphic = 86
	// infernalBurnGraphicHeight is BURN_GRAPHIC_HEIGHT: the
	// smokepuff plays at height 150.
	infernalBurnGraphicHeight = 150
	// firemakingLogXPEventName is the "firemaking:log-xp"
	// custom event: the querier asks Firemaking for the
	// Firemaking XP of a log; the answerer sets XP (and
	// HasXP) on the request. No answerer exists yet — see
	// the file header.
	firemakingLogXPEventName = "firemaking:log-xp"
	// firemakingXPMultiplierEventName is the
	// "firemaking:xp-multiplier" custom event: the querier
	// asks for the Firemaking XP multiplier (the Pyromancer
	// outfit bonus); it defaults to 1 when unanswered.
	firemakingXPMultiplierEventName = "firemaking:xp-multiplier"
)

// unchargedByChargedInfernal is the unit file's
// unchargedByCharged map, verbatim: INFERNAL_AXE (13241) →
// INFERNAL_AXE_UNCHARGED_ (13242); INFERNAL_AXE_OR_ (25066)
// → INFERNAL_AXE_UNCHARGED__2 (25371); INFERNAL_AXE_OR__3
// (30347) → INFERNAL_AXE_UNCHARGED__4 (30348).
// (ItemIdentifiers.ts:11696-11697, 23126, 23396, 28160-28161.)
var unchargedByChargedInfernal = map[int]int{
	13241: 13242,
	25066: 25371,
	30347: 30348,
}

// FiremakingLogXPRequest is the payload of the
// "firemaking:log-xp" custom event
// (InfernalAxe.Woodcutting.js:47-50): the log id and the
// answer — the log's Firemaking XP, with HasXP reporting
// whether any handler answered. The querier reads XP after
// the emit returns; with no handlers it stays unanswered.
type FiremakingLogXPRequest struct {
	Player *game.Player
	LogID  int
	XP     int
	HasXP  bool
}

// FiremakingXPMultiplierRequest is the payload of the
// "firemaking:xp-multiplier" custom event
// (InfernalAxe.Woodcutting.js:54-56): the player and the
// multiplier, which starts at 1 and a handler (the
// Pyromancer outfit bonus) may raise.
type FiremakingXPMultiplierRequest struct {
	Player     *game.Player
	Multiplier float64
}

// infernalCharges reads an item's infernal-axe charges
// (InfernalAxe.Woodcutting.js:15-18): the saved value
// clamped to [0, MAX_CHARGES], else MAX_CHARGES — an axe
// with no charge data yet is fully charged.
func infernalCharges(item game.Item) int {
	if saved, ok := chargesFromMeta(item, infernalChargesMetaKey); ok {
		return min(max(saved, 0), infernalMaxCharges)
	}
	return infernalMaxCharges
}

// findChargedInfernalAxe finds the charged infernal axe of
// axeID the player is wielding or carrying, wielded first
// (InfernalAxe.Woodcutting.js:20-25): the weapon slot when
// its id equals axeID, else the first inventory item with
// that id.
func findChargedInfernalAxe(player *game.Player, axeID int) (axeSlot, bool) {
	if player == nil {
		return axeSlot{}, false
	}
	if eq := player.Equipment(); eq != nil {
		weapon := eq.ItemAt(game.WeaponSlot)
		if weapon.ID() == axeID {
			return axeSlot{item: weapon, slot: game.WeaponSlot, wielded: true}, true
		}
	}
	if inv := player.Inventory(); inv != nil {
		for i, item := range inv.Items() {
			if item.ID() == axeID {
				return axeSlot{item: item, slot: i}, true
			}
		}
	}
	return axeSlot{}, false
}

// useInfernalCharge spends one charge of the axe at loc
// (InfernalAxe.Woodcutting.js:27-37): decrement the saved
// charges, or at 0 set the id to the uncharged form, clear
// the meta, refresh, and message.
func useInfernalCharge(player *game.Player, loc axeSlot) {
	left := infernalCharges(loc.item) - 1
	if left > 0 {
		item := loc.item
		item.SetMetaValue(infernalChargesMetaKey, chargesMetaValue(left))
		writeAxeItem(player, loc, item)
		return
	}
	uncharged, ok := unchargedByChargedInfernal[loc.item.ID()]
	if !ok {
		return
	}
	item := loc.item
	item.SetID(uncharged)
	item.SetMetaValue(infernalChargesMetaKey, nil)
	writeAxeItem(player, loc, item)
	refreshContainers(player)
	player.SendMessage("Your infernal axe has run out of charges.")
}

// tryBurnLog burns the just-cut log instead of putting it in
// the inventory (InfernalAxe.Woodcutting.js:40-62), in TS
// order: not a charged infernal axe id, or the 1/3 roll
// misses → false; find the charged axe; ask Firemaking for
// the log's XP — no axe, or no positive XP → false (with no
// Firemaking answerer yet, the burn never fires: see the
// file header); ask for the XP multiplier; grant half the
// Firemaking XP; smokepuff graphic at height 150; the fire
// sound; spend one charge; return true. The caller is
// chopSuccess, before prepareLog.
func (w *woodcutting) tryBurnLog(player *game.Player, axeID, logID int) bool {
	if _, ok := unchargedByChargedInfernal[axeID]; !ok {
		return false
	}
	if w.randInt(0, infernalBurnChance-1) != 0 {
		return false
	}
	loc, ok := findChargedInfernalAxe(player, axeID)
	request := &FiremakingLogXPRequest{Player: player, LogID: logID}
	w.emitEvent(firemakingLogXPEventName, request)
	if !ok || !request.HasXP || request.XP <= 0 {
		return false
	}
	outfit := &FiremakingXPMultiplierRequest{Player: player, Multiplier: 1}
	w.emitEvent(firemakingXPMultiplierEventName, outfit)
	if skills := player.Skills(); skills != nil {
		skills.AddExperiences(game.SkillFiremaking, int(float64(request.XP)/2*outfit.Multiplier))
	}
	player.PerformGraphic(infernalBurnGraphic, infernalBurnGraphicHeight, 0)
	game.SendSoundEffect(player, game.SoundFireSuccessful.ID(), 1, 0, 1)
	useInfernalCharge(player, loc)
	return true
}

// checkInfernalAxe answers the Check action
// (InfernalAxe.Woodcutting.js:65-70): "Infernal axe: X.X%
// remaining." It returns false — the click falls through —
// when the item is not a charged infernal axe.
func (w *woodcutting) checkInfernalAxe(event *plugins.PluginItemActionEvent) bool {
	if event == nil || event.Player == nil {
		return false
	}
	item := actionItem(event.Player, event)
	if _, ok := unchargedByChargedInfernal[item.ID()]; !ok {
		return false
	}
	pct := float64(infernalCharges(item)) / float64(infernalMaxCharges) * 100
	event.Player.SendMessage(fmt.Sprintf("Infernal axe: %.1f%% remaining.", pct))
	return true
}

// registerInfernalAxe wires the InfernalAxe registrations
// (InfernalAxe.Woodcutting.js:72-79): Check per charged axe
// name.
func (w *woodcutting) registerInfernalAxe(api *plugins.Api) {
	if api == nil {
		return
	}
	check := map[string]plugins.ItemActionHandler{"Check": w.checkInfernalAxe}
	api.OnItemActionNamed("Infernal axe", check, nil)
	api.OnItemActionNamed("Infernal axe (or)", check, nil)
}
