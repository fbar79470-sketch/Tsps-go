// Spell hooks and helpers for the magic listeners: the
// spell-on-NPC (NPCOptionPacketListener.castSpell), spell-on-player
// (MagicOnPlayerPacketListener.cast), and spell-on-item /
// spell-on-ground (MagicOnItemPacketListener.castOnItem /
// castGroundItem) routes. The hub families behind these seams are
// Worker C's (PHASE27_PLAN.md contracts C2–C4); the coordinator
// wires each field in cmd/server, and every field degrades to the
// no-hooks default when unset — exactly as the other Deps hook
// structs in this package do. This package does not import
// internal/plugins or internal/cache: the cache spell-name lookup
// and the hub emits arrive through func fields.
//
// Rune ids are ItemIdentifiers.ts (fire 554, air 556, nature 561,
// law 563, coins 995); spellbook interface ids are
// MagicSpellbook.ts:15-18 (NORMAL 1151, ANCIENT 12855, LUNAR
// 29999, ARCEUUS 39999); the infinite-staff sets are
// MagicOnItemPacketListener.ts:59-60.

package listeners

import (
	"strings"

	"tsps-go/internal/game"
)

const (
	// Spellbook interface ids (MagicSpellbook.ts:15-18).
	spellbookNormalInterfaceID  = 1151
	spellbookAncientInterfaceID = 12855
	spellbookLunarInterfaceID   = 29999
	spellbookArceuusInterfaceID = 39999
)

const (
	// Rune and coin item ids (ItemIdentifiers.ts).
	runeFireID   = 554
	runeAirID    = 556
	runeNatureID = 561
	runeLawID    = 563
	itemCoinsID  = 995
)

const (
	// Effect-spell ids (EffectSpells.ts:142-221). Worker A ports
	// the table; the ids are named here so the listeners can
	// route without importing it. The reanimation ids are
	// ArceuusItemSpells.ts:33-36.
	spellBonesToBananasID = 1159
	spellLowAlchemyID     = 1162
	spellTelegrabID       = 1168
	spellSuperheatItemID  = 1173
	spellHighAlchemyID    = 1178
	spellBonesToPeachesID = 15877
	spellBakePieID        = 9107
	spellVengeanceOtherID = 9135
	spellVengeanceID      = 9136

	spellBasicReanimationID  = 11997
	spellAdeptReanimationID  = 2885
	spellExpertReanimationID = 6208
	spellMasterReanimationID = 6888
)

// airStaves and fireStaves are the infinite-rune staves
// (MagicOnItemPacketListener.ts:59-60): wielding one waives the
// matching rune from a cast's cost. The elemental staves 3053 /
// 3054 (air/fire) are in both sets, as in TS.
var airStaves = map[int]bool{
	1381: true, 1397: true, 1405: true, 6562: true, 6563: true, 3053: true, 3054: true,
}

var fireStaves = map[int]bool{
	1387: true, 1393: true, 1401: true, 3053: true, 3054: true,
}

// SpellHooks carries the spell seams the magic listeners need:
// the cache spell-name lookup, the spellbook read, the combat
// cast, the scripted-cast hook emit, and the rune/disabled
// gates. Every field is optional and degrades to the no-hooks
// default when unset. The coordinator wires them in cmd/server
// (cache lookup over CacheDefinitions.GetSpellName, cast over
// Player.GetCombat().CastSpellOn, task scheduling over
// World.Tasks().Schedule, the rest over the hub).
type SpellHooks struct {
	// SpellName resolves the cache spell name for a click
	// (CacheDefinitions.getSpellName,
	// internal/cache/facade.go: GetSpellName). Nil means no
	// cache is wired: spell ids resolve by id only and the
	// name fallback is skipped.
	SpellName func(widgetID, itemID int) (name string, ok bool)
	// SpellbookInterfaceID reports the interface id of the
	// player's current spellbook (MagicSpellbook interface
	// ids above; Worker C's Player.Spellbook() is the source
	// once it lands). Nil means the NORMAL book (1151), the
	// TS default.
	SpellbookInterfaceID func(player *game.Player) int
	// CastSpell casts a combat spell on a target: the Go form
	// of player.getCombat().castSpellOn(target, spell)
	// (Combat.castSpellOn). When nil, spell casts drop with
	// their listener reason.
	CastSpell func(attacker *game.Player, target *game.Mobile, spell game.CombatSpell)
	// EmitSpellOnNpc runs the onSpellOnNpc hook family
	// (PluginManager.emitSpellOnNpc, PluginManager.ts:
	// 1967-1973) for a scripted cast, reporting whether a
	// hook claimed it. Nil means no hooks are wired.
	EmitSpellOnNpc func(ev *SpellOnNpcEvent) bool
	// SpellRuneBypass is the onSpellRuneBypass first-answer
	// gate (PluginManager.emitSpellRuneBypass,
	// PluginManager.ts:1851-1876; contract C3). The
	// spellbook is always NORMAL for the listener casts
	// (alchemy, telegrab), so it is not passed. Nil means no
	// bypass.
	SpellRuneBypass func(player *game.Player, spellID, itemID int) bool
	// SpellDisabled is the onSpellDisabled first-answer gate
	// (PluginManager.emitSpellDisabled, PluginManager.ts:
	// 1824-1849; contract C3). spell is the combat spell
	// when the cast has one, else nil (alchemy has no
	// CombatSpell in Go yet — Worker A ports EffectSpells).
	// Nil means not disabled.
	SpellDisabled func(player *game.Player, spellID int, spell any) bool
	// CheckSpellRuneSource reports whether a rune source
	// (rune pouch) covers the missing runes
	// (PluginManager.checkSpellRuneSource; contract C3). Nil
	// means no source exists: the inventory must cover the
	// whole cost.
	CheckSpellRuneSource func(player *game.Player, missing []game.Item) bool
	// ConsumeSpellRuneSource spends the missing runes from
	// the rune source (PluginManager.consumeSpellRuneSource;
	// contract C3). Nil means no source exists.
	ConsumeSpellRuneSource func(player *game.Player, missing []game.Item)
	// ScheduleTask schedules a tick task (TaskManager.Schedule,
	// World.Tasks()): the telegrab landing and the queued
	// alchemy re-cast. Nil means no task system is wired and
	// delayed spell effects cannot run.
	ScheduleTask func(task *game.Task)
}

// SpellOnPlayerEvent is the listener-facing form of the
// "magic:spell-on-player" custom-event payload
// (NetworkBuilder.ts:786-806): the caster, the target, the cache
// spell name, and the packet's spell selection. A handler sets
// Handled to claim the click (the cast is then skipped).
type SpellOnPlayerEvent struct {
	Player      *game.Player
	Target      *game.Player
	SpellName   string
	SpellWidget int
	SpellChild  int
	SpellItemID int
	Handled     bool
}

// SpellOnNpcEvent is the listener-facing form of the onSpellOnNpc
// hook event (NPCOptionPacketListener.ts:212-236,
// castSpellOnScriptedNpc): the caster, the NPC, its content id
// and world index, the spell, and the consumeRunes closure the
// hook calls to pay for the cast (runes + XP). A hook sets
// Handled to claim the cast.
type SpellOnNpcEvent struct {
	Player   *game.Player
	NPC      *game.NPC
	NpcID    int
	NpcIndex int
	Spell    game.SpellCaster
	SpellID  int
	Location game.Location
	// ConsumeRunes pays for the cast: canCast(consume=true),
	// startCast, and the base XP grant. It reports false when
	// the cast cannot be paid (already consumed, or the
	// runes/level check fails).
	ConsumeRunes func() bool
	Handled      bool
}

// spellNameOf resolves the cache spell name for a click through
// the SpellName seam, or reports absent when no cache is wired.
func spellNameOf(deps Deps, widgetID, itemID int) (string, bool) {
	if deps.SpellHooks.SpellName == nil {
		return "", false
	}
	return deps.SpellHooks.SpellName(widgetID, itemID)
}

// resolveSpellID resolves the client spell id for a click
// (NetworkBuilder.resolveClientCombatSpellId,
// NetworkBuilder.ts:1786-1800) through the SpellName seam.
func resolveSpellID(deps Deps, widgetID, child, itemID int) int {
	return game.ResolveClientCombatSpellID(widgetID, child, itemID, deps.SpellHooks.SpellName)
}

// isNormalSpellbook reports whether the player is on the NORMAL
// spellbook, by interface id (MagicOnItemPacketListener.ts:152,
// 197: getSpellbook().getInterfaceId() === NORMAL). A nil seam
// reads NORMAL, the TS default book.
func isNormalSpellbook(deps Deps, p *game.Player) bool {
	if deps.SpellHooks.SpellbookInterfaceID == nil {
		return true
	}
	return deps.SpellHooks.SpellbookInterfaceID(p) == spellbookNormalInterfaceID
}

// emitCustom emits a custom event through the player's sink
// (Player.EmitCustomEvent, installed by cmd/server over
// Hub.EmitCustomEvent). Nil-safe: with no sink installed the
// event is silently unobserved, the no-hooks default.
func emitCustom(p *game.Player, name string, payload any) {
	if p == nil || p.EmitCustomEvent == nil {
		return
	}
	p.EmitCustomEvent(name, payload)
}

// hasInfiniteAirRune / hasInfiniteFireRune report whether the
// wielded weapon waives the matching rune
// (MagicOnItemPacketListener.ts:92-103: the weapon id in the
// AIR_STAVES / FIRE_STAVES sets).
func hasInfiniteAirRune(p *game.Player) bool {
	if p == nil || p.Equipment() == nil {
		return false
	}
	return airStaves[p.Equipment().GetWeapon().ID()]
}

func hasInfiniteFireRune(p *game.Player) bool {
	if p == nil || p.Equipment() == nil {
		return false
	}
	return fireStaves[p.Equipment().GetWeapon().ID()]
}

// runeCosts filters a cast's rune costs the way
// MagicOnItemPacketListener.runeCosts does
// (MagicOnItemPacketListener.ts:105-110): a cost is waived when
// the onSpellRuneBypass gate answers true for it, or when an
// infinite staff covers it.
func runeCosts(deps Deps, p *game.Player, spellID int, costs []game.Item) []game.Item {
	var out []game.Item
	for _, cost := range costs {
		if deps.SpellHooks.SpellRuneBypass != nil &&
			deps.SpellHooks.SpellRuneBypass(p, spellID, cost.ID()) {
			continue
		}
		if cost.ID() == runeAirID && hasInfiniteAirRune(p) {
			continue
		}
		if cost.ID() == runeFireID && hasInfiniteFireRune(p) {
			continue
		}
		out = append(out, cost)
	}
	return out
}

// inventoryShortfall is Spell.inventoryShortfall
// (Spell.ts:176-192): the costs the inventory cannot cover, one
// entry per short id with the missing amount. TS accumulates
// the costs into a Map, so the shortfall list follows the
// spell's cost order — the first missing rune is the first
// rune in the cost list (law before air for telegrab), not
// Go map order.
func inventoryShortfall(p *game.Player, costs []game.Item) []game.Item {
	required := map[int]int{}
	var order []int
	for _, item := range costs {
		if !item.IsValid() {
			continue
		}
		if _, ok := required[item.ID()]; !ok {
			order = append(order, item.ID())
		}
		required[item.ID()] += item.Amount()
	}
	var missing []game.Item
	for _, id := range order {
		if short := required[id] - p.Inventory().GetAmount(id); short > 0 {
			missing = append(missing, game.NewItem(id, short))
		}
	}
	return missing
}

// hasRunes reports whether the player can pay costs: the
// inventory covers them, or a rune source covers the shortfall
// (MagicOnItemPacketListener.ts:112-115).
func hasRunes(deps Deps, p *game.Player, costs []game.Item) bool {
	missing := inventoryShortfall(p, costs)
	if len(missing) == 0 {
		return true
	}
	if deps.SpellHooks.CheckSpellRuneSource != nil {
		return deps.SpellHooks.CheckSpellRuneSource(p, missing)
	}
	return false
}

// consumeRunes spends costs from the inventory, then the
// shortfall from the rune source
// (MagicOnItemPacketListener.ts:117-121). Callers check
// hasRunes first, so the source always covers what is missing.
func consumeRunes(deps Deps, p *game.Player, costs []game.Item) {
	missing := inventoryShortfall(p, costs)
	for _, item := range costs {
		p.Inventory().DeleteNumber(item.ID(), item.Amount())
	}
	if len(missing) > 0 && deps.SpellHooks.ConsumeSpellRuneSource != nil {
		deps.SpellHooks.ConsumeSpellRuneSource(p, missing)
	}
}

// missingItemsMessage is Spell.missingItemsMessage
// (Spell.ts:125-136): the first shortfall item's name, title-cased
// + "s" for runes, lower-cased + "s" otherwise. The cache rune
// order is not passed (no cache-rune-order seam in the listeners),
// so the shortfall order stands. An unnamed shortfall reads the
// generic line.
func missingItemsMessage(p *game.Player, shortfall []game.Item) string {
	if len(shortfall) == 0 {
		return "You do not have the required items to cast this spell."
	}
	name := p.ItemName(shortfall[0].ID())
	if name == "" {
		return "You do not have the required items to cast this spell."
	}
	if strings.HasSuffix(strings.ToLower(name), "rune") {
		return "You do not have enough " + titleCaseWords(name) + "s to cast this spell."
	}
	return "You do not have enough " + strings.ToLower(name) + "s to cast this spell."
}

// titleCaseWords title-cases each word (Spell.ts:134: the
// /\b\w/g replacement on rune names).
func titleCaseWords(s string) string {
	words := strings.Fields(s)
	for i, w := range words {
		if len(w) == 0 {
			continue
		}
		words[i] = strings.ToUpper(w[:1]) + w[1:]
	}
	return strings.Join(words, " ")
}

// attrInt reads a numeric player attribute as an int64 (the
// alchemy pacing stamps are cycle numbers). A missing attribute,
// or one of another shape, reads 0.
func attrInt(p *game.Player, key string) int64 {
	switch v := p.GetAttribute(key).(type) {
	case int64:
		return v
	case int:
		return int64(v)
	case float64:
		return int64(v)
	default:
		return 0
	}
}
