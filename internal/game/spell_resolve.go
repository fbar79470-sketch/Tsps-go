package game

import (
	"strings"
	"sync"
)

// Spell resolution — NetworkBuilder.resolveClientCombatSpellId
// (NetworkBuilder.ts:1786-1800) and the spell tables it reads
// (CombatSpells.ts:2068-2116, EffectSpells.ts:222-237).
//
// The combat-spell table itself is Worker A's
// (internal/game/combat_spells*.go): spells register here at boot
// through RegisterCombatSpell / RegisterCombatSpellNamed, and the
// listeners resolve through GetCombatSpell / GetCombatSpellByName
// (contract C1 of PHASE27_PLAN.md). The registry — not the table
// — lives here so the listeners can resolve before any spell is
// registered: an unregistered id reads as unknown, never as a
// invented spell. Utility (effect) spells register separately
// through RegisterUtilitySpell: they share the casting surface
// below but are not CombatSpells, so the combat method never
// receives one (NPCOptionPacketListener.ts:178-183).

// SpellCaster is the casting surface shared by combat spells and
// utility (effect) spells: what the scripted NPC route and the
// alchemy/telegrab listeners need from a spell. CombatSpell
// satisfies it (CanCast, CanCastOnTarget, StartCast,
// BaseExperience are all CombatSpell methods); Worker A's
// EffectSpells port must satisfy it too for its spells to take
// the scripted route (Spell.ts: canCast, canCastOnTarget,
// startCast, baseExperience).
type SpellCaster interface {
	// CanCast reports whether the caster may cast (runes,
	// level, cooldowns); consume=true spends the cast cost.
	CanCast(caster *Player, consume bool) bool
	// CanCastOnTarget reports whether the spell may be cast
	// on this target (immunities, target restrictions).
	CanCastOnTarget(caster *Player, target *Mobile) bool
	// StartCast runs the cast's opening (animation, graphics;
	// rune consumption is part of CanCast(consume=true)).
	// The caster is a *Mobile, as in TS (CombatSpell.startCast,
	// CombatSpell.ts:50-101): an NPC holding a spell object
	// casts through this same path.
	StartCast(caster *Mobile, target *Mobile)
	// BaseExperience is the XP owed on a cast (paid once per
	// cast by the magic method, or by the scripted route's
	// consumeRunes closure).
	BaseExperience() int
}

var _ SpellCaster = (CombatSpell)(nil)

// SpellResolveEvent is the "magic:resolve-spell" custom-event
// payload (NPCOptionPacketListener.ts:161-176): plugins supply a
// scripted utility spell the legacy tables do not hold. Spell is
// set by the handler: a CombatSpell takes the combat route, any
// other SpellCaster takes the scripted route. It stays nil when
// no plugin supplies a spell.
type SpellResolveEvent struct {
	// Player is the caster.
	Player *Player
	// SpellID is the resolved client spell id.
	SpellID int
	// SpellName is the cache spell name, when the cache
	// resolved one for the click.
	SpellName string
	// SpellWidget is the packet's spell widget id.
	SpellWidget int
	// Spell is the plugin-supplied spell, or nil.
	Spell any
}

var (
	spellRegistryMu sync.RWMutex
	// combatSpellsByID is the CombatSpells table, keyed by the
	// client's widget-ish spell id.
	combatSpellsByID = map[int]CombatSpell{}
	// combatSpellsByName is the same table keyed by normalized
	// spell name (see normalizeSpellName).
	combatSpellsByName = map[string]CombatSpell{}
	// combatSpellIDsByName recovers the id for a name lookup:
	// the registry keeps an explicit name→id map so a name
	// lookup returns the id the client spell carries without
	// asking the spell for it (CombatSpell does carry
	// SpellID() now — Worker A added it — but the map is the
	// registration path's own record).
	combatSpellIDsByName = map[string]int{}
	// utilitySpellsByID is the EffectSpells table, keyed by
	// spell id: utility spells the combat method must never
	// receive.
	utilitySpellsByID = map[int]SpellCaster{}
)

// RegisterCombatSpell registers a combat spell under id.
// Worker A calls this once per spell at boot (or, equivalently,
// RegisterCombatSpellNamed when the name is known). Registering
// twice under one id replaces the earlier spell.
func RegisterCombatSpell(id int, spell CombatSpell) {
	if spell == nil {
		return
	}
	spellRegistryMu.Lock()
	defer spellRegistryMu.Unlock()
	combatSpellsByID[id] = spell
}

// RegisterCombatSpellNamed registers a combat spell under id and
// name, for the by-name fallback. The name is normalized the way
// CombatSpells.getCombatSpellByName does (see
// normalizeSpellName), so a display name ("Wind Strike") and the
// static property name ("WIND_STRIKE") register identically.
func RegisterCombatSpellNamed(id int, name string, spell CombatSpell) {
	if spell == nil {
		return
	}
	spellRegistryMu.Lock()
	defer spellRegistryMu.Unlock()
	combatSpellsByID[id] = spell
	if key := normalizeSpellName(name); key != "" {
		combatSpellsByName[key] = spell
		combatSpellIDsByName[key] = id
	}
}

// UnregisterCombatSpell removes the combat spell registered under
// id (and its name entries). Test and boot tooling: the table is
// otherwise append-only.
func UnregisterCombatSpell(id int) {
	spellRegistryMu.Lock()
	defer spellRegistryMu.Unlock()
	delete(combatSpellsByID, id)
	for key, spellID := range combatSpellIDsByName {
		if spellID == id {
			delete(combatSpellsByName, key)
			delete(combatSpellIDsByName, key)
		}
	}
}

// GetCombatSpell returns the combat spell registered for id, or
// nil when none is (CombatSpells.getCombatSpell,
// CombatSpells.ts:2068-2084: a linear scan returning null).
func GetCombatSpell(id int) CombatSpell {
	spellRegistryMu.RLock()
	defer spellRegistryMu.RUnlock()
	return combatSpellsByID[id]
}

// GetCombatSpellByName returns the combat spell registered under
// name, or nil (CombatSpells.getCombatSpellByName,
// CombatSpells.ts:2105-2113: the name is upper-cased with
// non-alphanumerics stripped, and matched against the static
// property names with underscores stripped).
func GetCombatSpellByName(name string) CombatSpell {
	spellRegistryMu.RLock()
	defer spellRegistryMu.RUnlock()
	return combatSpellsByName[normalizeSpellName(name)]
}

// combatSpellIDByName returns the id registered under name, for
// the resolver: the id-lookup tables answer by id, and the name
// lookup must return the id the client spell carries.
func combatSpellIDByName(name string) (int, bool) {
	spellRegistryMu.RLock()
	defer spellRegistryMu.RUnlock()
	id, ok := combatSpellIDsByName[normalizeSpellName(name)]
	return id, ok
}

// normalizeSpellName is CombatSpells.getCombatSpellByName's
// normalization (CombatSpells.ts:2106-2108): upper-case, strip
// every non-alphanumeric (which also strips the underscores of
// the static property names).
func normalizeSpellName(name string) string {
	var out strings.Builder
	out.Grow(len(name))
	for _, r := range strings.ToUpper(name) {
		if (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			out.WriteRune(r)
		}
	}
	return out.String()
}

// RegisterUtilitySpell registers a utility (effect) spell under
// id (EffectSpells.forSpellId, EffectSpells.ts:227-237). Worker A
// calls this once per EffectSpells entry at boot.
func RegisterUtilitySpell(id int, spell SpellCaster) {
	if spell == nil {
		return
	}
	spellRegistryMu.Lock()
	defer spellRegistryMu.Unlock()
	utilitySpellsByID[id] = spell
}

// UnregisterUtilitySpell removes the utility spell registered
// under id. Test and boot tooling.
func UnregisterUtilitySpell(id int) {
	spellRegistryMu.Lock()
	defer spellRegistryMu.Unlock()
	delete(utilitySpellsByID, id)
}

// GetUtilitySpell returns the utility spell registered for id, or
// nil (EffectSpells.forSpellId).
func GetUtilitySpell(id int) SpellCaster {
	spellRegistryMu.RLock()
	defer spellRegistryMu.RUnlock()
	return utilitySpellsByID[id]
}

// ResolveSpell returns the spells registered for id: the combat
// spell when one is registered (which also serves as the
// SpellCaster for the scripted route), else the utility spell.
// ok is false when neither table holds the id.
func ResolveSpell(id int) (combat CombatSpell, utility SpellCaster, ok bool) {
	if c := GetCombatSpell(id); c != nil {
		return c, c, true
	}
	if u := GetUtilitySpell(id); u != nil {
		return nil, u, true
	}
	return nil, nil, false
}

// ResolveClientCombatSpellID ports
// NetworkBuilder.resolveClientCombatSpellId
// (NetworkBuilder.ts:1786-1800): the id tries first — spellChild,
// spellWidget & 0xffff, spellItemId — through GetCombatSpell,
// then the cache spell-name fallback (getCombatSpellByName),
// then MAGIC_ITEMS.resolveSpellId. spellName is the cache lookup
// (CacheDefinitions.getSpellName, internal/cache); nil means no
// cache is wired and the name fallback is skipped, resolving by
// id only. -1 means no spell resolved.
func ResolveClientCombatSpellID(spellWidget, spellChild, spellItemID int, spellName func(widgetID, itemID int) (string, bool)) int {
	for _, id := range []int{spellChild, spellWidget & 0xffff, spellItemID} {
		if GetCombatSpell(id) != nil {
			return id
		}
	}
	if spellName == nil {
		return -1
	}
	widgetID := ((spellWidget >> 16) << 16) | (spellChild & 0xffff)
	name, ok := spellName(spellWidget, spellItemID)
	if !ok {
		name, ok = spellName(widgetID, spellItemID)
	}
	if !ok || name == "" {
		return -1
	}
	if id, ok := combatSpellIDByName(name); ok {
		return id
	}
	return ResolveMagicItemSpellID(name)
}

// ResolveMagicItemSpellID ports MAGIC_ITEMS.resolveSpellId
// (MagicOnItemPacketListener.ts:66-89, the resolveSpellId the
// NetworkBuilder fallback calls): Low Alchemy 1162, High Alchemy
// 1178, Telekinetic Grab 1168, and the four reanimations
// (ArceuusItemSpells.ts:33-36). The reanimation ids resolve here
// so the name fallback names them; their handlers are Arceuus
// content (NOT STARTED). -1 when the name matches nothing.
func ResolveMagicItemSpellID(name string) int {
	switch strings.TrimSpace(strings.ToLower(name)) {
	case "low level alchemy":
		return 1162
	case "high level alchemy":
		return 1178
	case "telekinetic grab":
		return 1168
	case "basic reanimation":
		return 11997
	case "adept reanimation":
		return 2885
	case "expert reanimation":
		return 6208
	case "master reanimation":
		return 6888
	default:
		return -1
	}
}
