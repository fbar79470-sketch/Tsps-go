// Package gracefuloutfit is the Go port of the GracefulOutfit plugin
// (server/plugins/items/GracefulOutfit.plugin.js): each graceful piece
// gives its own run-energy restoration bonus (hood 3%, top/legs 4%,
// gloves/boots/cape 3%), wearing all six adds another 10%, and an
// Agility cape (or max cape) counts as the graceful cape. Every colour
// variant keeps its piece's Wiki weight, so the stats panel shows the
// same -25 kg for all variants.
//
// The port is faithful: same plugin name, description, and members
// flag, same attach-only register, same pure bonus calculation, same
// signature-keyed bonus cache, same unconditional weight overwrite,
// and same set-effects line text. Line numbers in comments cite
// GracefulOutfit.plugin.js.
//
// Event inventory:
//
//   - Listens to "equipment:weights" (game.EquipmentWeightsEventName):
//     produced in Go by Player.CarriedWeightGrams (engine). This
//     package and WornWeights both answer it; handler order is
//     unobservable — no item matches both pieces' rules (graceful
//     pieces match by name prefix, worn-weight items by id, and the
//     two sets are disjoint).
//   - Listens to "equipment:effects" (game.EquipmentEffectsEventName):
//     NO producer in Go — the EquipmentStats plugin that emits it
//     (describeEffects, EquipmentStats.plugin.js:126-133) is not
//     ported, so this handler is registered but never fires. This
//     is the documented gap, not a wiring bug.
//   - Emits "skillcape:has-perk" (game.SkillcapeHasPerkEventName) as a
//     query: NO answerer in Go — the SkillCapes plugin that answers
//     it (answerPerk, Common.SkillCapes.js:124-130) is not ported, so
//     Result stays false and the Agility-cape substitute path never
//     triggers in production. That is the honest no-hooks default;
//     no skillcape content is invented here. The path is unit-covered
//     through a fake emit func.
//
// Two deliberate deviations, both forced by the language change:
//
//   - Bonus cache: TS keeps a WeakMap keyed by player, so entries
//     vanish with the player. Go has no weak map, so the cache is a
//     mutex-guarded map keyed by *game.Player (never by player
//     index — indexes are reused), cleaned up by an extra
//     OnPlayerDisconnect registration the TS plugin does not have
//     (the dateminutes precedent). The disconnect emit runs on the
//     tick goroutine (World.drainRemovePlayers phase), as do the
//     restore and weight call sites, so the mutex is insurance
//     rather than a proven need — kept, per the phase plan.
//   - Names: TS content reads item names via
//     coreApi.ItemDefinition.forId(id).getName(). Go content reads
//     them via Player.ItemName over the player's installed
//     definition source (the same Store); a Core ItemDefinition
//     surface stays table-only.
package gracefuloutfit

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"sync"

	"tsps-go/internal/game"
	"tsps-go/internal/plugins"
)

// piece is one row of the PIECES table
// (GracefulOutfit.plugin.js:9-16): the equipment slot, the name
// prefix that identifies the piece in any colour, the restore
// percent, and the Wiki weight in kg.
type piece struct {
	slot     int
	prefix   string
	percent  float64
	weightKg float64
}

// pieces is PIECES (GracefulOutfit.plugin.js:9-16), in TS order:
// the order is observable in the cache signature (ids joined in
// table order) and in the per-piece bonus sum.
var pieces = []piece{
	{slot: game.HeadSlot, prefix: "Graceful hood", percent: 0.03, weightKg: -3},
	{slot: game.BodySlot, prefix: "Graceful top", percent: 0.04, weightKg: -5},
	{slot: game.LegSlot, prefix: "Graceful legs", percent: 0.04, weightKg: -6},
	{slot: game.HandsSlot, prefix: "Graceful gloves", percent: 0.03, weightKg: -3},
	{slot: game.FeetSlot, prefix: "Graceful boots", percent: 0.03, weightKg: -4},
	{slot: game.CapeSlot, prefix: "Graceful cape", percent: 0.03, weightKg: -4},
}

// setBonus is SET_BONUS (GracefulOutfit.plugin.js:17): the extra
// restore fraction for wearing all six pieces.
const setBonus = 0.10

// setText is SET_TEXT (GracefulOutfit.plugin.js:107): the
// equipment-effects set-text id (enum 3047) for the full outfit.
const setText = 8

// gracefulCapePrefix identifies the cape row of PIECES, found by
// the pure function exactly as TS finds it
// (GracefulOutfit.plugin.js:49: PIECES.find by prefix).
const gracefulCapePrefix = "Graceful cape"

// cachedBonus is one WeakMap entry: the worn-id signature the
// bonus was computed for, and the bonus itself
// (GracefulOutfit.plugin.js:74-83).
type cachedBonus struct {
	signature string
	bonus     float64
}

// outfit holds the plugin's per-player bonus cache and the
// skillcape-query emit func captured from the Api at Register.
// One instance serves the registered Plugin value; tests build
// their own with a fake emit func.
type outfit struct {
	mu    sync.Mutex
	cache map[*game.Player]cachedBonus
	emit  func(name string, payload any)
}

func newOutfit(emit func(name string, payload any)) *outfit {
	return &outfit{cache: make(map[*game.Player]cachedBonus), emit: emit}
}

// capeIsSubstitute reports whether the player counts as wearing a
// graceful cape through an Agility (or max) cape
// (capeIsSubstitute, GracefulOutfit.plugin.js:21-25): it emits
// "skillcape:has-perk" with {player, skill: "AGILITY", result:
// false} and returns whether the request came back true. With no
// emit func installed (before Register) or no answerers (the Go
// state: SkillCapes is not ported), Result stays false. The
// Inventory field stays false: the TS request carries no
// inventory flag.
func (s *outfit) capeIsSubstitute(player *game.Player) bool {
	request := &game.SkillcapePerkRequest{Player: player, Skill: "AGILITY", Result: false}
	if s.emit != nil {
		s.emit(game.SkillcapeHasPerkEventName, request)
	}
	return request.Result
}

// itemName returns the definition name of item, or "" when the
// item has no usable id or no name (itemName,
// GracefulOutfit.plugin.js:27-33: id <= 0 returns null, else
// coreApi.ItemDefinition.forId(id)?.getName?.() ?? null). Go ""
// plays the TS null role: it matches no prefix. A nil player
// yields "" (Go nil-safety; TS never calls with a nil player).
func itemName(p *game.Player, item game.Item) string {
	if p == nil || item.ID() <= 0 {
		return ""
	}
	return p.ItemName(item.ID())
}

// gracefulRestoreBonusFromSlots is the pure bonus calculation
// (GracefulOutfit.plugin.js:36-58), factored so it tests without
// a player: per piece whose slot name starts with the prefix, add
// the percent; a substitute cape adds the cape percent when the
// cape slot does not already hold a graceful cape; 0 pieces
// returns 0; all 6 pieces add SET_BONUS. A missing slot reads as
// "" (the TS null), matching nothing. String.startsWith is
// strings.HasPrefix.
func gracefulRestoreBonusFromSlots(nameBySlot map[int]string, substitute bool) float64 {
	var bonus float64
	worn := 0
	for _, pc := range pieces {
		if name := nameBySlot[pc.slot]; name != "" && strings.HasPrefix(name, pc.prefix) {
			bonus += pc.percent
			worn++
		}
	}
	// An Agility (or max) cape is a graceful cape: its own 3%,
	// alone too, and the sixth piece of the set.
	var cape piece
	for _, pc := range pieces {
		if pc.prefix == gracefulCapePrefix {
			cape = pc
			break
		}
	}
	capeName := nameBySlot[cape.slot]
	if substitute && !(capeName != "" && strings.HasPrefix(capeName, cape.prefix)) {
		bonus += cape.percent
		worn++
	}
	if worn == 0 {
		return 0
	}
	if worn == len(pieces) {
		bonus += setBonus
	}
	return bonus
}

// energyRestoreBonus builds the name-by-slot map for the six
// slots from the player's equipment and calls the pure function
// with the skillcape query (energyRestoreBonus,
// GracefulOutfit.plugin.js:60-67). A nil player or missing
// equipment reads every slot as "" and returns 0 without
// emitting (Go nil-safety; TS never calls with a nil player).
func (s *outfit) energyRestoreBonus(player *game.Player) float64 {
	if player == nil {
		return 0
	}
	nameBySlot := make(map[int]string, len(pieces))
	equipment := player.Equipment()
	for _, pc := range pieces {
		var name string
		if equipment != nil {
			name = itemName(player, equipment.ItemAt(pc.slot))
		}
		nameBySlot[pc.slot] = name
	}
	return gracefulRestoreBonusFromSlots(nameBySlot, s.capeIsSubstitute(player))
}

// wornSignature joins the six worn ids in PIECES order
// (GracefulOutfit.plugin.js:76: PIECES.map(slot id ?? -1)
// .join(",")). A missing equipment container reads every slot
// as -1.
func wornSignature(player *game.Player) string {
	ids := make([]string, len(pieces))
	equipment := player.Equipment()
	for i, pc := range pieces {
		id := -1
		if equipment != nil {
			id = equipment.ItemAt(pc.slot).ID()
		}
		ids[i] = strconv.Itoa(id)
	}
	return strings.Join(ids, ",")
}

// applyGracefulRunEnergy is the registered run-energy restore
// modifier (applyGracefulRunEnergy,
// GracefulOutfit.plugin.js:74-83): the cached bonus applies
// until the six worn ids change, and the skillcape query only
// runs when the equipment actually moves. A bonus of 0 returns
// the units unchanged. The mutex guards only the map accesses:
// the bonus computation (which emits synchronously) runs
// outside the lock, so a re-entrant emit can never deadlock on
// the cache. A nil player returns the units unchanged (Go
// nil-safety).
func (s *outfit) applyGracefulRunEnergy(player *game.Player, units float64) float64 {
	if player == nil {
		return units
	}
	signature := wornSignature(player)
	s.mu.Lock()
	cached, ok := s.cache[player]
	s.mu.Unlock()
	if !ok || cached.signature != signature {
		cached = cachedBonus{signature: signature, bonus: s.energyRestoreBonus(player)}
		s.mu.Lock()
		s.cache[player] = cached
		s.mu.Unlock()
	}
	if cached.bonus > 0 {
		return units * (1 + cached.bonus)
	}
	return units
}

// addGracefulWeights writes every worn graceful piece's Wiki
// weight (kg, NOT grams) into slotWeights, unconditionally
// overwriting whatever is there, and returns the worn piece
// count (addGracefulWeights, GracefulOutfit.plugin.js:91-104).
// It is a no-op returning 0 when there is no player or no
// slotWeights map. Nothing is stored per player.
func addGracefulWeights(player *game.Player, slotWeights map[int]float64) int {
	if player == nil || slotWeights == nil {
		return 0
	}
	equipment := player.Equipment()
	if equipment == nil {
		return 0
	}
	worn := 0
	for _, pc := range pieces {
		if name := itemName(player, equipment.ItemAt(pc.slot)); name != "" && strings.HasPrefix(name, pc.prefix) {
			slotWeights[pc.slot] = pc.weightKg
			worn++
		}
	}
	return worn
}

// addGracefulEffects fills the graceful weights first, then,
// when all six pieces are worn — or five with a substitute
// cape — pushes the set line and set text onto the effects
// request (addGracefulEffects, GracefulOutfit.plugin.js:109-117).
// The bonus is recomputed via energyRestoreBonus (which
// re-queries the skillcape), exactly as TS does. Math.round is
// math.Round (identical for the positive values here); the TS
// request.setTexts?.push degenerates to a plain append (a Go
// nil slice appends safely, and the only producer passes a
// slice). A nil request or player is a no-op.
func (s *outfit) addGracefulEffects(request *game.EquipmentEffectsRequest) {
	if request == nil || request.Player == nil {
		return
	}
	worn := addGracefulWeights(request.Player, request.SlotWeights)
	if worn == len(pieces) || (worn == len(pieces)-1 && s.capeIsSubstitute(request.Player)) {
		bonus := s.energyRestoreBonus(request.Player)
		request.Lines = append(request.Lines, fmt.Sprintf("Graceful set: +%d%% run energy restore", int(math.Round(bonus*100))))
		request.SetTexts = append(request.SetTexts, setText)
	}
}

// onEquipmentEffects is the "equipment:effects" custom-event
// handler: it type-asserts the opaque payload and delegates to
// addGracefulEffects. A payload of any other type is ignored.
func (s *outfit) onEquipmentEffects(payload any) {
	request, ok := payload.(*game.EquipmentEffectsRequest)
	if !ok || request == nil {
		return
	}
	s.addGracefulEffects(request)
}

// onEquipmentWeights is the "equipment:weights" custom-event
// handler: it type-asserts the opaque payload and delegates to
// addGracefulWeights. A payload of any other type is ignored.
func (s *outfit) onEquipmentWeights(payload any) {
	request, ok := payload.(*game.EquipmentWeightsRequest)
	if !ok || request == nil {
		return
	}
	addGracefulWeights(request.Player, request.SlotWeights)
}

// onPlayerDisconnect drops the player's cached bonus when the
// player leaves the world: the Go stand-in for the TS WeakMap,
// which drops entries with the player. Without it the map would
// retain every player that ever logged in. This registration
// has no TS counterpart (the TS plugin has no disconnect hook).
func (s *outfit) onPlayerDisconnect(event *plugins.PlayerDisconnectEvent) {
	if event == nil {
		return
	}
	s.mu.Lock()
	delete(s.cache, event.Player)
	s.mu.Unlock()
}

func (s *outfit) plugin() plugins.Plugin {
	return plugins.Plugin{
		Name:        "GracefulOutfit",
		Description: "Gives each graceful piece its run-energy restoration bonus, plus the full-set bonus, and counts an Agility or max cape as the graceful cape.",
		Members:     true,
		Register: func(api *plugins.Api) {
			// The emit func is captured here, as TS captures
			// pluginApi in register (GracefulOutfit.plugin.js:131).
			s.emit = api.EmitCustomEvent
			api.RegisterRunEnergyRestoreModifier(s.applyGracefulRunEnergy)
			api.OnCustomEvent(game.EquipmentEffectsEventName, s.onEquipmentEffects)
			api.OnCustomEvent(game.EquipmentWeightsEventName, s.onEquipmentWeights)
			// Disconnect cleanup only; see the package comment.
			api.OnPlayerDisconnect(s.onPlayerDisconnect)
		},
	}
}

// Plugin is the registered GracefulOutfit plugin value.
var Plugin = newOutfit(nil).plugin()

func init() {
	plugins.Register(Plugin)
}
