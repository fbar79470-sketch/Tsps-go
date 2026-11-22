package standardeffects

import (
	"tsps-go/internal/game"
	"tsps-go/internal/plugins"
)

// Rune ids (ItemIdentifiers.ts:553-556).
const (
	runeFireID  = 554
	runeWaterID = 555
	runeAirID   = 556
	runeEarthID = 557
)

// staffGroup is one row of the TS staff table: the weapon ids
// that supply the runes (ElementalStaves.plugin.js:6-19,
// STAFF_GROUPS).
type staffGroup struct {
	staves []int
	runes  []int
}

// staffGroups is STAFF_GROUPS verbatim
// (ElementalStaves.plugin.js:6-19), with ids from
// ItemIdentifiers.ts:
//
//   - Staff of air 1381, air battlestaff 1397, mystic air
//     staff 1405 → air.
//   - Staff of water 1383, water battlestaff 1395, mystic
//     water staff 1403 → water.
//   - Staff of earth 1385, earth battlestaff 1399, mystic
//     earth staff 1407 → earth.
//   - Staff of fire 1387, fire battlestaff 1393, mystic fire
//     staff 1401 → fire.
//   - Lava battlestaff 3053, mystic lava staff 3054, lava
//     battlestaff 3 21198, mystic lava staff 3 21200 → fire,
//     earth.
//   - Mud battlestaff 6562, mystic mud staff 6563 → water,
//     earth.
//   - Steam battlestaff 11787, mystic steam staff 11789, steam
//     battlestaff 3 12795, mystic steam staff 3 12796 → fire,
//     water.
//   - Smoke battlestaff 11998, mystic smoke staff 12000 →
//     fire, air.
//   - Mist battlestaff 20730, mystic mist staff 20733 → water,
//     air.
//   - Dust battlestaff 20736, mystic dust staff 20739 → air,
//     earth.
//   - Kodai wand 21006, Kodai wand 3 23626 → water.
//   - Twinflame staff 30634 → fire, water.
//   - Shadowflame quadrant 33253 → air, water, earth, fire
//     (Wiki, Shadowflame quadrant: an infinite source of all
//     four).
var staffGroups = []staffGroup{
	{[]int{1381, 1397, 1405}, []int{runeAirID}},
	{[]int{1383, 1395, 1403}, []int{runeWaterID}},
	{[]int{1385, 1399, 1407}, []int{runeEarthID}},
	{[]int{1387, 1393, 1401}, []int{runeFireID}},
	{[]int{3053, 3054, 21198, 21200}, []int{runeFireID, runeEarthID}},
	{[]int{6562, 6563}, []int{runeWaterID, runeEarthID}},
	{[]int{11787, 11789, 12795, 12796}, []int{runeFireID, runeWaterID}},
	{[]int{11998, 12000}, []int{runeFireID, runeAirID}},
	{[]int{20730, 20733}, []int{runeWaterID, runeAirID}},
	{[]int{20736, 20739}, []int{runeAirID, runeEarthID}},
	{[]int{21006, 23626}, []int{runeWaterID}},
	{[]int{30634}, []int{runeFireID, runeWaterID}},
	{[]int{33253}, []int{runeAirID, runeWaterID, runeEarthID, runeFireID}},
}

// suppliedRunes is the runes the player's wielded weapon
// supplies (ElementalStaves.plugin.js:35-40, suppliedRunes,
// weapon half): the first matching staff group's runes, or
// nil.
//
// The shield-slot half (OFFHAND_GROUPS: the Devil's element,
// ElementalStaves.plugin.js:25-28) is NOT ported: TS only
// counts it for a Demonic Pacts character
// (inDemonicPacts, EchoItems.js), a Leagues system with no Go
// seam. A Devil's element in the shield slot supplies nothing
// here.
func suppliedRunes(player *game.Player) []int {
	if player == nil || player.Equipment() == nil {
		return nil
	}
	weapon := player.Equipment().GetWeapon().ID()
	for _, group := range staffGroups {
		for _, id := range group.staves {
			if id == weapon {
				return group.runes
			}
		}
	}
	return nil
}

// staveRuneBypass is the onSpellRuneBypass hook
// (ElementalStaves.plugin.js:42-46, bypassSuppliedRune): a
// rune the wielded staff supplies is bypassed. The whole-cost
// query (RuneID 0) never matches — 0 is not a rune id —
// exactly as TS's integer check skips the undefined rune id.
// Leaving Bypass nil for an unsupplied rune lets later hooks
// answer, per the first-answer gate.
func (s *standardEffects) staveRuneBypass(event *plugins.SpellRuneBypassEvent) {
	if event == nil || event.Player == nil {
		return
	}
	for _, id := range suppliedRunes(event.Player) {
		if id == event.RuneID {
			event.Bypass = plugins.BoolPtr(true)
			return
		}
	}
}

// ElementalStavesRunesRequest is the "elemental-staves:runes"
// query (ElementalStaves.plugin.js:49-52,
// reportSuppliedRunes): the player in, the rune ids the
// wielded staff (and offhand) supply out. There is no querier
// in Go yet; the answerer is registered for fidelity.
type ElementalStavesRunesRequest struct {
	// Player is the player whose staff is queried.
	Player *game.Player
	// Runes is the answer: the supplied rune ids.
	Runes []int
}

// reportSuppliedRunes answers the "elemental-staves:runes"
// query (ElementalStaves.plugin.js:49-52).
func (s *standardEffects) reportSuppliedRunes(payload any) {
	request, ok := payload.(*ElementalStavesRunesRequest)
	if !ok || request == nil {
		return
	}
	request.Runes = suppliedRunes(request.Player)
}
