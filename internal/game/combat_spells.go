package game

import (
	"fmt"
	"math"
	"strings"
	"unicode"
)

// Combat spells — the standard spellbook (Worker A, Phase 27).
//
// The port of game/content/combat/magic/Spell.ts (293 lines),
// CombatSpell.ts (152), CombatNormalSpell.ts (105),
// CombatEffectSpell.ts (88) and the standard-book statics of
// CombatSpells.ts (2,116), plus the utility table of
// EffectSpells.ts (268) and Charge (CombatSpells.ts:26-79),
// TeleBlock.ts (70) and the god-spell helpers of CombatSpells.ts.
//
// The tables live in combat_spells_data.go (brace-counted,
// verbatim extraction); this file is the behavior: the Spell
// base CanCast, the two CombatSpell types, the registries the
// listeners resolve through (spell_resolve.go: GetCombatSpell /
// GetCombatSpellByName / GetUtilitySpell), Charge, the god-spell
// rules, and TeleBlock.
//
// CONTRACTS (PHASE27_PLAN.md). C1: spells register here at boot
// through InstallStandardSpells; listeners resolve through
// GetCombatSpell / GetCombatSpellByName (spell_resolve.go). C2:
// Player.Spellbook / SetSpellbook and ChangeSpellbook live in
// spellbook.go (Worker C) and are called, not redefined, here.
// C3: the hub gates live in internal/plugins/spells.go
// (EmitSpellDisabled / EmitSpellRuneBypass /
// CheckSpellRuneSource / ConsumeSpellRuneSource); this file
// calls them through the four CombatHooks fields added in
// combat_hooks.go, which cmd/server wires (Worker C). C4: the
// magic:* custom-event payloads below are emitted through
// Player.EmitCustomEvent, nil-guarded.
//
// SCOPE. Standard book only: Ancients, Lunar and Arceuus are
// separate books and are NOT here. Bind/curse/freeze
// APPLICATION is plugin content (StandardBindings /
// StandardCurses / Freezes listen on magic:utility-effect):
// this file emits the events with the TS payload shape and
// applies nothing. Animation, graphic and projectile
// producers went live in Phase 29 (startSpellCast, the
// TeleBlock hit/miss projectile, Charge's cast animation and
// graphic), with the cast/impact sounds (Worker C's pickers,
// CastSoundFor / ImpactSoundFor, called from the magic
// method).

// Spell attribute keys (Spell.ts, CombatSpells.ts, TeleBlock.ts).
const (
	// spellNextCastAtAttribute is Spell.NEXT_CAST_AT
	// ("magic:next-cast-at", Spell.ts:40): the cycle a shared
	// -delay spell may next be cast on (Spell.ts:239-241).
	spellNextCastAtAttribute = "magic:next-cast-at"
	// spellCastDelayTicks is Spell.CAST_DELAY_TICKS (Spell.ts:41).
	spellCastDelayTicks = 1
	// spellCastContextAttribute holds the paid cast's
	// SpellCastContext on the caster (the Go form of Spell's
	// per-caster WeakMap, Spell.ts:30-31). It is an
	// unregistered attribute, so snapshots drop it: the
	// context is runtime-only and never persists.
	spellCastContextAttribute = "magic:cast-context"
	// spellbookSwapAttribute is the lunar:spellbook-swap flag
	// (Spell.ts:17-24, endSpellbookSwap).
	spellbookSwapAttribute = "lunar:spellbook-swap"
	// chargeUntilAttribute is CombatSpells.CHARGE_UNTIL
	// ("charge:until", CombatSpells.ts:385): the cycle the
	// Charge effect runs to.
	chargeUntilAttribute = "charge:until"
	// chargeNextCastAttribute is CombatSpells.CHARGE_NEXT_CAST
	// ("charge:next-cast", CombatSpells.ts:386): the cycle
	// Charge may next be cast on.
	chargeNextCastAttribute = "charge:next-cast"
	// ChargeTicks is CombatSpells.CHARGE_TICKS (700 ticks,
	// CombatSpells.ts:389): how long the Charge effect lasts.
	ChargeTicks = 700
	// chargeCooldownTicks is Charge's getCastCooldown
	// (60_000 ms, CombatSpells.ts:37-39) in ticks: 60_000 /
	// 600 (TickRateMs, constants.go).
	chargeCooldownTicks = 100
	// teleBlockUntilAttribute holds the cycle a Tele Block
	// runs to. TS keeps a wall-clock SecondsTimer on Combat
	// (Combat.ts:70, 815, getTeleblockTimer); Go has no
	// teleblock timer field on Combat and no TimerKey for it
	// (TS uses no TimerKey), so the expiry is cycle-stamped
	// here, the prayerBlockUntilCycle precedent (combat.go).
	teleBlockUntilAttribute = "tele-block:until"
)

// TeleBlockWarnedAttribute is TeleBlock.WARNED_ATTRIBUTE
// ("tele-block:warned", TeleBlock.ts:17): set once the 30-second
// warning was sent for the running block. The warning tick
// itself is the TeleBlock plugin's (plugins/combat/
// TeleBlock.plugin.js, onPlayerProcess), not this file's.
const TeleBlockWarnedAttribute = "tele-block:warned"

// TeleBlock durations in seconds (TeleBlock.ts:14-16).
const (
	// TeleBlockDurationSeconds is TeleBlock.DURATION_SECONDS.
	TeleBlockDurationSeconds = 300
	// TeleBlockProtectedDurationSeconds is
	// TeleBlock.PROTECTED_DURATION_SECONDS: the block when
	// Protect from Magic is up as the spell lands.
	TeleBlockProtectedDurationSeconds = 150
	// TeleBlockWarningSeconds is TeleBlock.WARNING_SECONDS.
	TeleBlockWarningSeconds = 30
)

// spellXP converts a TS spell XP float to the CombatSpell
// interface's int. TS pays floats (5.5 etc.) and keeps tenths;
// Go stores integer experience and cannot keep tenths
// (skills.go:517-523), so the table's verbatim float rounds to
// nearest: the per-cast error is at most 0.5 XP, where
// truncation would bias -0.5 XP on every fractional cast.
// math.Round rounds halves away from zero (5.5 -> 6).
func spellXP(xp float64) int {
	return int(math.Round(xp))
}

// Graphic heights are Worker A's constants in
// mobile_animation.go (GraphicHeight.ts: LOW 0, MIDDLE 50,
// HIGH 100, SURGE_CAST 92, SURGE_IMPACT 124, ARCHER_DRAW
// 96); the spell and ranged producers use them directly.

// ---- Custom-event payloads (contract C4) ----
//
// Names are verbatim (Spell.ts, CombatSpell.ts,
// CombatEffectSpell.ts). Payloads are pointers: hooks mutate
// the answer fields in place, as TS plugins mutate the event
// object. They follow the PrivateAreaEnteredPayload precedent
// (private_area.go): named game types, emitted through
// Player.EmitCustomEvent.

// MagicSpellCostPayload is the "magic:spell-cost" event
// (Spell.ts:129-137, itemsToConsume): the runes the cast will
// spend (Items) and the runes a staff or tome supplies
// (Covered, never spent). Hooks may replace Items; CanCast
// spends whatever Items holds after the emit.
type MagicSpellCostPayload struct {
	// Player is the caster.
	Player *Player
	// Spell is the casting spell (a CombatSpell, a
	// *utilitySpell, or the *chargeSpell).
	Spell any
	// Items are the runes the cast will spend.
	Items []Item
	// Covered are the runes a bypass supplies; never spent.
	Covered []Item
}

// MagicRuneSavingPayload is the "magic:rune-saving" event
// (Spell.ts:226-233): staff of the dead / Kodai wand plugins
// may negate the rune cost of a combat spell. Offensive is
// true for combat spells (TS: typeof maximumHit ===
// "function"). Message is "" when the plugin sends none (TS
// null).
type MagicRuneSavingPayload struct {
	// Player is the caster.
	Player *Player
	// Offensive is true for a combat spell.
	Offensive bool
	// Saved is set by a hook that negates the cost.
	Saved bool
	// Message is the line the hook sends, or "".
	Message string
}

// MagicCastCommittedPayload is the "magic:cast-committed"
// event (Spell.ts:244-248): a paid cast, with its context and
// whether its runes were saved.
type MagicCastCommittedPayload struct {
	// Player is the caster.
	Player *Player
	// Spell is the casting spell (see MagicSpellCostPayload).
	Spell any
	// Context is the paid cast's context.
	Context *SpellCastContext
	// RunesSaved reports the rune-saving answer.
	RunesSaved bool
}

// MagicHitEffectsPayload is the "magic:hit-effects" /
// "magic:hit-effects-applied" event (CombatSpell.ts:125-134,
// onHitCalc): the same object is emitted for both, so one
// pointer serves. Allowed false skips the spell's effect;
// EffectMultiplier scales it (1 by default).
type MagicHitEffectsPayload struct {
	// Attacker is the caster.
	Attacker *Mobile
	// Target is the hit's target.
	Target *Mobile
	// Spell is the spell that hit.
	Spell CombatSpell
	// Hit is the pending hit being resolved.
	Hit *PendingHit
	// Allowed is false when a hook vetoes the effect.
	Allowed bool
	// EffectMultiplier scales the spell's effect.
	EffectMultiplier float64
}

// MagicUtilityEffectPayload is the "magic:utility-effect-check"
// / "magic:utility-effect" event (CombatEffectSpell.ts:60-68,
// spellEffect): the same object is emitted for both. Allowed
// false stops the effect; Handled true (set by a plugin that
// applied the effect, e.g. a bind) skips the spell's own
// fallback.
type MagicUtilityEffectPayload struct {
	// Attacker is the caster.
	Attacker *Mobile
	// Target is the spell's target.
	Target *Mobile
	// Spell is the effect spell.
	Spell CombatSpell
	// Context is the paid cast's context, or nil when the
	// cast has none (e.g. an NPC caster).
	Context *SpellCastContext
	// Allowed is false when a hook vetoes the effect.
	Allowed bool
	// Handled is true when a plugin applied the effect.
	Handled bool
}

// MageArenaCapeReceivedPayload is the "mage-arena:cape-received"
// event (ChargeSpell.cast, CombatSpells.ts:47-50): the Mage
// Arena plugin answers whether the player holds a god cape.
// Received is nil when no plugin answers (TS null).
type MageArenaCapeReceivedPayload struct {
	// Player is the caster.
	Player *Player
	// Received is the plugin's answer, or nil.
	Received *bool
}

// MagicSpellUnlockedPayload is the "magic:spell-unlocked"
// event (ChargeSpell.cast, CombatSpells.ts:51-56): a league
// relic may lift a spell's quest requirement. Unlocked is
// false unless a plugin sets it.
type MagicSpellUnlockedPayload struct {
	// Player is the caster.
	Player *Player
	// Name is the spell's name ("charge").
	Name string
	// Unlocked is set by a plugin that lifts the requirement.
	Unlocked bool
}

// MagicChargeCastPayload is the "magic:charge-cast" event
// (ChargeSpell.cast, CombatSpells.ts:78): a Charge cast, with
// the cycle the effect runs to. The plugin that says "Your
// magical charge fades away" schedules itself from this.
type MagicChargeCastPayload struct {
	// Player is the caster.
	Player *Player
	// Until is the cycle the Charge effect runs to.
	Until int64
}

// SpellCastContext is Spell.SpellCastContext (Spell.ts:11-15):
// a paid cast's runtime-only snapshot. The spell, the tick it
// was paid on, and a data map for per-cast state (the
// projectile-visual slot, CombatSpell.ts:89-102, filled by
// queueProjectileVisual for a qualifying standard cast).
type SpellCastContext struct {
	// Spell is the casting spell (see MagicSpellCostPayload).
	Spell any
	// Tick is the cycle the cast was paid on.
	Tick int64
	// Data is per-cast state, keyed by name.
	Data map[string]any
}

// projectileVisualContextKey is CombatSpell.ts
// PROJECTILE_VISUAL_CONTEXT ("magic:projectile-visual"): the
// SpellCastContext data slot holding a standard cast's queued
// impact visual.
const projectileVisualContextKey = "magic:projectile-visual"

// projectileVisual is CombatSpell.ts's ProjectileVisual: a
// standard cast's impact graphic (and its area sound) queued
// for the end of its projectile, so they land when the
// projectile does.
type projectileVisual struct {
	// target is the cast's target.
	target *Mobile
	// endCycleOffset is the projectile's landing offset in
	// client cycles (Projectile.getTiming, legacy magic).
	endCycleOffset int
	// queued reports the impact graphic was already played
	// (with its landing delay) at cast time.
	queued bool
	// soundQueued reports the impact area sound was already
	// sent (with its landing delay) at cast time.
	soundQueued bool
}

// getProjectileVisual is CombatSpell.getProjectileVisual
// (CombatSpell.ts:103-107): the queued impact visual for this
// spell and target, or nil (no context, another spell's
// context, or another target).
func getProjectileVisual(spell CombatSpell, ctx *SpellCastContext, target *Mobile) *projectileVisual {
	if ctx == nil || ctx.Spell != spell {
		return nil
	}
	visual, _ := ctx.Data[projectileVisualContextKey].(*projectileVisual)
	if visual == nil || visual.target != target {
		return nil
	}
	return visual
}

// storeSpellCastContext records the paid cast's context on the
// caster (Spell.ts:242-243, castContexts.set).
func storeSpellCastContext(player *Player, ctx *SpellCastContext) {
	if player == nil || ctx == nil {
		return
	}
	player.SetAttribute(spellCastContextAttribute, ctx)
}

// spellCastContext recovers the caster's paid-cast context for
// spell. It matches on spell identity only — no tick check:
// TS hands finishCast the context the PendingHit captured at
// cast time (PendingHit.ts:115, 234, getSpellContext), and a
// second cast cannot start before the first hit lands (the
// attack cycle is longer than the hit delay), so the stored
// context is still the hit's. Nil when the caster has none
// (Spell.ts:32-35, getCastContext, without its same-tick term,
// which governs startCast's same-tick projectile-visual read,
// not finishCast).
func spellCastContext(player *Player, spell any) *SpellCastContext {
	if player == nil || spell == nil {
		return nil
	}
	ctx, _ := player.GetAttribute(spellCastContextAttribute).(*SpellCastContext)
	if ctx == nil || ctx.Spell != spell {
		return nil
	}
	return ctx
}

// emitCustomEvent emits a magic custom event through the
// caster's sink, nil-guarded (player.go:384: a nil sink means
// no subscribers). The sink is per-player but global in
// effect (the coordinator installs Hub.EmitCustomEvent), so an
// NPC caster's event goes out through the target's player
// when the caster has none.
func emitCustomEvent(attacker, target *Mobile, name string, payload any) {
	if attacker != nil {
		if p := combatantFor(attacker).Player(); p != nil && p.EmitCustomEvent != nil {
			p.EmitCustomEvent(name, payload)
			return
		}
	}
	if target != nil {
		if p := combatantFor(target).Player(); p != nil && p.EmitCustomEvent != nil {
			p.EmitCustomEvent(name, payload)
		}
	}
}

// emitPlayerEvent emits a magic custom event through one
// player's sink, nil-guarded.
func emitPlayerEvent(player *Player, name string, payload any) {
	if player != nil && player.EmitCustomEvent != nil {
		player.EmitCustomEvent(name, payload)
	}
}

// attributeTick reads a cycle-valued attribute as an int64: 0
// when absent or of another shape. TS reads
// Number(player.getAttribute(key) ?? 0) (Spell.ts:232-236).
func attributeTick(player *Player, key string) int64 {
	if player == nil {
		return 0
	}
	switch v := player.GetAttribute(key).(type) {
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

// asCombatSpell returns spell as a CombatSpell, or nil when it
// is not one (utility and Charge spells carry no CombatSpell;
// the SpellDisabled hook's spell argument is nil for them,
// plugins/spells.go: SpellDisabledEvent).
func asCombatSpell(spell any) CombatSpell {
	if s, ok := spell.(CombatSpell); ok {
		return s
	}
	return nil
}

// ---- The Spell base (Spell.ts) ----

// spellBase is the shared Spell state behind every spell type
// in this file: what Spell.canCast reads (level, runes,
// equipment, spellbook, cooldown) plus the two flags the
// consume path needs (sharedDelay, offensive). Combat and
// utility spells build one from their table entry; Charge
// builds one literally.
type spellBase struct {
	// id is the spell's client id (spellId()).
	id int
	// level is the Magic level required.
	level int
	// runes are the itemsRequired cost, in TS order.
	runes []Item
	// equipmentRequired returns the equipmentRequired cost,
	// or nil when the spell needs none. Nil means "no
	// requirement" (TS returns null/undefined).
	equipmentRequired func(player *Player) []Item
	// spellbook is the book the spell belongs to.
	spellbook Spellbook
	// members is the spell's members flag (Spell.members).
	// Recorded only: CanCast does not enforce it, exactly
	// as TS canCast does not (members is world-gated).
	members bool
	// missingEquipmentMessage is the spell's own refusal line,
	// or "" for Spell's default.
	missingEquipmentMessage string
	// cooldownAttr is the per-spell cooldown attribute, or ""
	// when the spell has none (Spell.getCastCooldown).
	cooldownAttr string
	// cooldownTicks is the cooldown in ticks (TS duration in
	// ms / 600, Spell.ts:239-243).
	cooldownTicks int
	// sharedDelay is usesSharedCastDelay (CombatSpell: true;
	// Spell base and utility spells: false).
	sharedDelay bool
	// offensive is true for combat spells (TS: typeof
	// maximumHit === "function", Spell.ts:228).
	offensive bool
}

// missingEquipmentMessageOrDefault is Spell.
// missingEquipmentMessage (Spell.ts:70-72): the spell's own
// line, else the default.
func (b *spellBase) missingEquipmentMessageOrDefault() string {
	if b.missingEquipmentMessage != "" {
		return b.missingEquipmentMessage
	}
	return "You do not have the required equipment to cast this spell."
}

// runesRequired returns a copy of the rune cost: callers
// never mutate the base's own slice (itemsToConsume builds
// its paid/covered splits from it).
func (b *spellBase) runesRequired() []Item {
	if len(b.runes) == 0 {
		return nil
	}
	out := make([]Item, len(b.runes))
	copy(out, b.runes)
	return out
}

// equipment returns the equipment cost, or nil.
func (b *spellBase) equipment(player *Player) []Item {
	if b.equipmentRequired == nil {
		return nil
	}
	return b.equipmentRequired(player)
}

// canCastSpell is Spell.canCast (Spell.ts:190-251), shared by
// every spell type: level, disabled gate, spellbook match,
// rune shortfall + rune-source, equipment, then on consume
// the shared delay / cooldown gate, rune-saving, rune
// deletion / rune-source consumption, endSpellbookSwap, the
// cast context and the cast-committed event. The refusal
// order is TS order, including the two silent ones (the
// disabled gate and the delay/cooldown gate send nothing)
// and the equipment check living inside the rune-cost branch
// (a spell with no rune cost skips it, Spell.ts:217-230).
func canCastSpell(player *Player, base *spellBase, spell any, consume bool) bool {
	if player == nil || base == nil {
		return false
	}
	combat := player.GetCombat()
	if combat == nil || player.Skills() == nil {
		return false
	}
	if player.Skills().CurrentLevel(SkillMagic) < base.level {
		player.SendMessage(fmt.Sprintf("You need a Magic level of %d to cast this spell.", base.level))
		combat.Reset()
		return false
	}
	if Hooks.SpellDisabled != nil && Hooks.SpellDisabled(player, base.spellbook, base.id, asCombatSpell(spell)) {
		combat.SetCastSpell(nil)
		combat.Reset()
		return false
	}
	if player.Spellbook() != base.spellbook {
		// Autocasting.setAutocast(player, null)
		// (Spell.ts:206-208): the full setAutocast (indicators
		// + BonusManager note) is autocasting.go; clearing the
		// spell is the state effect CanCast needs.
		SetAutocast(player, nil)
		combat.SetCastSpell(nil)
		combat.Reset()
		return false
	}
	items := base.runesRequired()
	var toConsume, shortfall []Item
	if len(items) > 0 {
		toConsume = itemsToConsume(player, base, spell, items)
		shortfall = inventoryShortfall(player, toConsume)
		// A rune source (rune pouch) may cover what the
		// inventory cannot.
		if len(shortfall) > 0 && !checkSpellRuneSource(player, shortfall) {
			player.SendMessage(missingItemsMessage(player, shortfall))
			combat.SetCastSpell(nil)
			combat.Reset()
			return false
		}
		if equipment := base.equipment(player); len(equipment) > 0 && !playerHasEquipment(player, equipment) {
			player.SendMessage(base.missingEquipmentMessageOrDefault())
			combat.SetCastSpell(nil)
			combat.Reset()
			return false
		}
	}
	if !consume {
		return true
	}
	now := combatCycle()
	if (base.sharedDelay && attributeTick(player, spellNextCastAtAttribute) > now) ||
		(base.cooldownAttr != "" && attributeTick(player, base.cooldownAttr) > now) {
		return false
	}
	// Plugins (staff of the dead, Kodai wand) may negate the
	// rune cost of a combat spell.
	saving := &MagicRuneSavingPayload{Player: player, Offensive: base.offensive}
	emitPlayerEvent(player, "magic:rune-saving", saving)
	if saving.Saved {
		if saving.Message != "" {
			player.SendMessage(saving.Message)
		}
	} else {
		if inv := player.Inventory(); inv != nil {
			for _, item := range toConsume {
				inv.Deletes(item)
			}
		}
		if len(shortfall) > 0 {
			consumeSpellRuneSource(player, shortfall)
		}
	}
	// A cast whose runes a weapon saved is still the swap's
	// one spell.
	endSpellbookSwap(player)
	if base.sharedDelay {
		player.SetAttribute(spellNextCastAtAttribute, now+spellCastDelayTicks)
	}
	if base.cooldownAttr != "" {
		player.SetAttribute(base.cooldownAttr, now+int64(base.cooldownTicks))
	}
	ctx := &SpellCastContext{Spell: spell, Tick: now, Data: map[string]any{}}
	storeSpellCastContext(player, ctx)
	emitPlayerEvent(player, "magic:cast-committed", &MagicCastCommittedPayload{
		Player: player, Spell: spell, Context: ctx, RunesSaved: saving.Saved,
	})
	return true
}

// itemsToConsume is Spell.itemsToConsume (Spell.ts:118-137):
// the whole-cost bypass first (RuneID 0, the hub's whole-cost
// sentinel — plugins/spells.go, SpellRuneBypassEvent), then
// the per-rune bypass, then the magic:spell-cost event with
// the paid/covered split. It returns the event's Items: hooks
// may replace the list.
func itemsToConsume(player *Player, base *spellBase, spell any, items []Item) []Item {
	if len(items) == 0 {
		return nil
	}
	if Hooks.SpellRuneBypass != nil && Hooks.SpellRuneBypass(player, base.spellbook, base.id, 0) {
		return nil
	}
	// `covered`: the runes a staff or tome supplies (the
	// per-rune bypass), which the cast never spends.
	var paid, covered []Item
	for _, item := range items {
		bypassed := Hooks.SpellRuneBypass != nil &&
			Hooks.SpellRuneBypass(player, base.spellbook, base.id, item.ID())
		if bypassed {
			covered = append(covered, item)
		} else {
			paid = append(paid, item)
		}
	}
	cost := &MagicSpellCostPayload{Player: player, Spell: spell, Items: paid, Covered: covered}
	emitPlayerEvent(player, "magic:spell-cost", cost)
	return cost.Items
}

// checkSpellRuneSource is PluginManager.checkSpellRuneSource
// (PluginManager.ts:5056-5074) through the CombatHooks seam:
// the first source that can supply every missing item answers
// true. Nil (no source wired) is false; an empty list is
// false, as in TS.
func checkSpellRuneSource(player *Player, items []Item) bool {
	if len(items) == 0 || Hooks.CheckSpellRuneSource == nil {
		return false
	}
	return Hooks.CheckSpellRuneSource(player, items)
}

// consumeSpellRuneSource is
// PluginManager.consumeSpellRuneSource
// (PluginManager.ts:5076-5092) through the CombatHooks seam.
// Nil is a no-op.
func consumeSpellRuneSource(player *Player, items []Item) {
	if len(items) == 0 || Hooks.ConsumeSpellRuneSource == nil {
		return
	}
	Hooks.ConsumeSpellRuneSource(player, items)
}

// inventoryShortfall is Spell.inventoryShortfall
// (Spell.ts:168-184): the part of items the inventory cannot
// fully supply, aggregated per item, in items order (TS
// iterates an insertion-ordered Map).
func inventoryShortfall(player *Player, items []Item) []Item {
	required := make(map[int]int)
	var order []int
	for _, item := range items {
		if _, ok := required[item.ID()]; !ok {
			order = append(order, item.ID())
		}
		required[item.ID()] += item.Amount()
	}
	var missing []Item
	var inv *Inventory
	if player != nil {
		inv = player.Inventory()
	}
	for _, id := range order {
		have := 0
		if inv != nil {
			have = inv.GetAmount(id)
		}
		if short := required[id] - have; short > 0 {
			missing = append(missing, NewItem(id, short))
		}
	}
	return missing
}

// missingItemsMessage is Spell.missingItemsMessage
// (Spell.ts:139-156): the first shortfall item no rune source
// covers (else the first shortfall item), a rune by its name
// in title case and plural, anything else in lower case and
// plural ("You do not have enough Law Runes to cast this
// spell."). The cache rune order (Spell.ts:148-167,
// cacheRuneOrder) has no Go form — no cache spell-name lookup
// exists in Go (spell_resolve.go: ResolveClientCombatSpellID
// documents the same gap) — so the order is the spell's own
// itemsRequired order.
func missingItemsMessage(player *Player, shortfall []Item) string {
	if len(shortfall) == 0 {
		return "You do not have the required items to cast this spell."
	}
	first := shortfall[0]
	for _, item := range shortfall {
		if !checkSpellRuneSource(player, []Item{item}) {
			first = item
			break
		}
	}
	name := ""
	if player != nil {
		name = player.ItemName(first.ID())
	}
	if name == "" {
		return "You do not have the required items to cast this spell."
	}
	var plural string
	if strings.HasSuffix(strings.ToLower(name), "rune") {
		plural = titleCaseWords(name) + "s"
	} else {
		plural = strings.ToLower(name) + "s"
	}
	return fmt.Sprintf("You do not have enough %s to cast this spell.", plural)
}

// titleCaseWords uppercases the first letter of each word
// (TS: name.replace(/\b\w/g, letter => letter.toUpperCase()),
// Spell.ts:153).
func titleCaseWords(name string) string {
	var out []rune
	boundary := true
	for _, r := range name {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			if boundary {
				out = append(out, unicode.ToUpper(r))
			} else {
				out = append(out, r)
			}
			boundary = false
		} else {
			out = append(out, r)
			boundary = true
		}
	}
	return string(out)
}

// playerHasEquipment reports whether the player wears every
// item (presence; TS containsAllItem on the equipment
// container, CombatSpells.ts equipmentRequired amounts are
// always 1). A nil player or nil equipment wears nothing.
func playerHasEquipment(player *Player, equipment []Item) bool {
	if player == nil {
		return false
	}
	eq := player.Equipment()
	if eq == nil {
		return false
	}
	return eq.ContainsAllItem(equipment)
}

// endSpellbookSwap is Spell.endSpellbookSwap (Spell.ts:17-24):
// every cast that pays (or is spared) its runes ends the
// Spellbook Swap: clear the flag and restore the Lunar book
// (ChangeSpellbook with bypass, the TS changeSpellbook
// (player, LUNAR, true) call — bypassRequirements true, the
// default switch line).
func endSpellbookSwap(player *Player) {
	if player == nil || player.GetAttribute(spellbookSwapAttribute) == nil {
		return
	}
	player.SetAttribute(spellbookSwapAttribute, nil)
	ChangeSpellbook(player, SpellbookLunar, true, nil)
}

// ---- CombatSpell types (CombatNormalSpell.ts, CombatEffectSpell.ts) ----

// emitHitEffects is CombatSpell.onHitCalc
// (CombatSpell.ts:123-134): accurate hits emit
// magic:hit-effects (allowed + effectMultiplier), then the
// spell's spellEffectOnHitCalc, then magic:hit-effects-applied.
// The middle call is the TS base no-op for every standard
// spell (no standard static overrides spellEffectOnHitCalc;
// only the Ancient extend does, and Ancients are not here),
// so the two emits are adjacent here.
func emitHitEffects(spell CombatSpell, hit *PendingHit) {
	if hit == nil || !hit.IsAccurate() {
		return
	}
	effect := &MagicHitEffectsPayload{
		Attacker: hit.Attacker(), Target: hit.Target(), Spell: spell,
		Hit: hit, Allowed: true, EffectMultiplier: 1,
	}
	emitCustomEvent(hit.Attacker(), hit.Target(), "magic:hit-effects", effect)
	if !effect.Allowed {
		return
	}
	emitCustomEvent(hit.Attacker(), hit.Target(), "magic:hit-effects-applied", effect)
}

// standardSpellDefOf returns the standard-book table row
// behind a spell, or nil for a spell that is not a standard
// combatNormalSpell / combatEffectSpell (content spells carry
// no table row, so they carry no start/end graphic ids).
func standardSpellDefOf(spell CombatSpell) *standardSpellDef {
	switch s := spell.(type) {
	case *combatNormalSpell:
		return s.def
	case *combatEffectSpell:
		return s.def
	default:
		return nil
	}
}

// spellStartGraphicHeight is the start graphic's height
// (CombatSpells.ts): the surge casts use SURGE_CAST (92);
// every other standard start graphic is HIGH (100).
func spellStartGraphicHeight(spell CombatSpell) int {
	switch spell.SpellID() {
	case 21876, 21877, 21878, 21879:
		return graphicHeightSurgeCast
	default:
		return graphicHeightHigh
	}
}

// spellEndGraphicLowHeight marks the standard end graphics
// built with no height (CombatSpells.ts: Graphic(id) — LOW):
// Crumble Undead 147, Iban Blast 89, Magic Dart 329, the god
// spells 76/77/78, Vulnerability 169, Enfeeble 172, Tele Block
// 345. Every other standard end graphic is HIGH, except the
// Water/Fire Surge impacts (SURGE_IMPACT, 124).
func spellEndGraphicHeight(spell CombatSpell) int {
	switch spell.SpellID() {
	case 1171, 1539, 12037, 1190, 1191, 1192, 1542, 1543, 12445:
		return graphicHeightLow
	case 21877, 21879:
		return graphicHeightSurgeImpact
	default:
		return graphicHeightHigh
	}
}

// castAnimationFor is CombatSpell.castAnimationFor
// (CombatSpell.ts:39-48): the spell's cast animation, or its
// staff variant while the caster wields a staff
// (STAFF_CAST_ANIMATIONS, staffCastAnimations). The staff
// variant is player-only (TS: cast.isPlayer() &&
// hasStaffEquipped): an NPC caster always plays the base
// animation. -1 means the spell has no cast animation (TS
// null).
func castAnimationFor(caster *Mobile, castAnim int) int {
	if castAnim < 0 {
		return -1
	}
	if staffAnim, ok := staffCastAnimations[castAnim]; ok {
		if player := combatantFor(caster).Player(); player != nil && hasStaffEquipped(player) {
			return staffAnim
		}
	}
	return castAnim
}

// startSpellCast is the shared CombatSpell.startCast
// (CombatSpell.ts:50-101) for the standard-book spell kinds:
// the cast animation (with its staff variant), the start
// graphic, and the cast projectile (delay 0, speed 20,
// heights 43/31 — the legacy-magic shape). A spell with no
// projectile (the god spells, Tele Block) plays the animation
// and graphic only. The queued end-graphic for legacy-magic
// timing is recorded on the paid cast's context when the
// spell qualifies (player caster, NORMAL spellbook, no fixed
// hit delay, radius 0).
//
// The caster is a *Mobile, as in TS: an NPC caster plays
// its cast animation and fires its projectile, but skips
// the start graphic when its id is one of 2000/109/3580/
// 2007 (CombatSpell.ts:60-72) — and never queues the impact
// visual, which TS records only for player casters
// (CombatSpell.ts:89-102).
func startSpellCast(spell CombatSpell, caster *Mobile, target *Mobile) {
	def := standardSpellDefOf(spell)
	if def == nil || caster == nil || target == nil {
		return
	}
	if anim := castAnimationFor(caster, def.castAnim); anim >= 0 {
		caster.PerformAnimation(anim, 0)
	}
	if def.startGraphic >= 0 && !npcSkipsStartGraphic(caster) {
		caster.PerformGraphic(def.startGraphic, spellStartGraphicHeight(spell), 0)
	}
	if def.projectile < 0 {
		return
	}
	source := projectileMobile(caster)
	victim := projectileMobile(target)
	if source == nil || victim == nil {
		return
	}
	proj := CreateProjectile(source, victim, def.projectile, 0, 20, 43, 31)
	proj.SendProjectile()
	queueProjectileVisual(spell, caster, target, proj)
}

// npcSkipsStartGraphic is the start-graphic exclusion in
// CombatSpell.startCast (CombatSpell.ts:60-72): NPC ids
// 2000, 109, 3580 and 2007 cast without a start graphic.
// A player caster, or an NPC with any other id, never
// skips.
func npcSkipsStartGraphic(caster *Mobile) bool {
	npc := combatantFor(caster).NPC()
	if npc == nil {
		return false
	}
	switch npc.ID {
	case 2000, 109, 3580, 2007:
		return true
	default:
		return false
	}
}

// queueProjectileVisual records a standard cast's impact
// visual on the paid cast's context (CombatSpell.ts:89-102):
// only for a player caster, the NORMAL spellbook, a
// legacy-magic projectile whose end lies past its start, and
// a spell with no fixed hit delay and no radius. Anything
// else keeps the impact-time path in HandleAfterHitEffects.
func queueProjectileVisual(spell CombatSpell, caster *Mobile, target *Mobile, proj *Projectile) {
	if proj == nil {
		return
	}
	timing := proj.GetTiming()
	if !timing.LegacyMagic || timing.EndCycleOffset <= timing.StartCycleOffset {
		return
	}
	if spellbookOfSpell(spell) != SpellbookNormal || spell.HitDelay() >= 0 || spell.SpellRadius() > 0 {
		return
	}
	player := combatantFor(caster).Player()
	if player == nil {
		return
	}
	ctx := spellCastContext(player, spell)
	if ctx == nil {
		return
	}
	if ctx.Data == nil {
		ctx.Data = map[string]any{}
	}
	ctx.Data[projectileVisualContextKey] = &projectileVisual{target: target, endCycleOffset: timing.EndCycleOffset}
}

// combatNormalSpell is CombatNormalSpell (CombatNormalSpell.ts):
// a direct-damage standard spell. FinishCast runs the
// options finishCast (the god drains) on accurate hits with
// damage > 0; OnHitCalc is the shared CombatSpell.onHitCalc.
type combatNormalSpell struct {
	base *spellBase
	def  *standardSpellDef
}

func (s *combatNormalSpell) SpellID() int          { return s.base.id }
func (s *combatNormalSpell) MaximumHit() int       { return s.def.maxHit }
func (s *combatNormalSpell) BaseExperience() int   { return spellXP(s.def.baseXP) }
func (s *combatNormalSpell) SplashExperience() int { return spellXP(s.def.splashXP) }

// AttackSpeed is CombatSpell.getAttackSpeed
// (CombatSpell.ts:137-139): 5 for every combat spell.
func (s *combatNormalSpell) AttackSpeed() int { return 5 }

// HitDelay is CombatSpell.hitDelay (CombatSpell.ts:147-149):
// -1 (no fixed delay; the magic method computes it from
// distance).
func (s *combatNormalSpell) HitDelay() int { return -1 }

// CanCast is Spell.canCast through the shared base.
func (s *combatNormalSpell) CanCast(caster *Player, consume bool) bool {
	return canCastSpell(caster, s.base, s, consume)
}

// CanCastOnTarget is CombatNormalSpell.canCastOnTarget
// (CombatNormalSpell.ts:38-40): the options override, else
// true. Only Crumble Undead overrides (the undead only).
func (s *combatNormalSpell) CanCastOnTarget(caster *Player, target *Mobile) bool {
	if s.def.crumbleUndead {
		return crumbleUndeadCanCastOnTarget(caster, target)
	}
	return true
}

// StartCast is CombatSpell.startCast (CombatSpell.ts:50-101):
// the cast animation (with its staff variant), start graphic,
// and projectile — see startSpellCast.
func (s *combatNormalSpell) StartCast(caster *Mobile, target *Mobile) {
	startSpellCast(s, caster, target)
}

// FinishCast is CombatNormalSpell.finishCast
// (CombatNormalSpell.ts:71-77): nothing on a splash or a
// 0-damage hit; otherwise the options finishCast — the god
// drains (CombatSpells.ts:345-373).
func (s *combatNormalSpell) FinishCast(attacker, target *Mobile, accurate bool, damage int) {
	if !accurate || damage <= 0 {
		return
	}
	switch s.def.godDrain {
	case "prayer":
		saradominStrikeDrain(attacker, target)
	case "defence":
		godSpellDrain(attacker, target, SkillDefence)
	case "magic":
		godSpellDrain(attacker, target, SkillMagic)
	}
}

// OnHitCalc is CombatSpell.onHitCalc (CombatSpell.ts:123-134).
func (s *combatNormalSpell) OnHitCalc(hit *PendingHit) { emitHitEffects(s, hit) }

// SpellRadius is the barrage radius (CombatSpell.ts, via the
// Ancient extend): 0 — no standard-book spell is an area
// spell.
func (s *combatNormalSpell) SpellRadius() int { return 0 }

// combatEffectSpell is CombatEffectSpell
// (CombatEffectSpell.ts): a max-hit-0-or-more effect spell
// (binds, curses, Tele Block). MaximumHit returns the declared
// value (SNARE 2, ENTANGLE 5, the rest 0); FinishCast runs
// spellEffect on accurate hits.
type combatEffectSpell struct {
	base *spellBase
	def  *standardSpellDef
}

func (s *combatEffectSpell) SpellID() int          { return s.base.id }
func (s *combatEffectSpell) MaximumHit() int       { return s.def.maxHit }
func (s *combatEffectSpell) BaseExperience() int   { return spellXP(s.def.baseXP) }
func (s *combatEffectSpell) SplashExperience() int { return spellXP(s.def.splashXP) }
func (s *combatEffectSpell) AttackSpeed() int      { return 5 }
func (s *combatEffectSpell) HitDelay() int         { return -1 }

// CanCast is Spell.canCast through the shared base.
func (s *combatEffectSpell) CanCast(caster *Player, consume bool) bool {
	return canCastSpell(caster, s.base, s, consume)
}

// CanCastOnTarget is CombatEffectSpell's (inherited from
// CombatSpell, CombatSpell.ts:31-33): always true — no
// standard effect spell overrides it.
func (s *combatEffectSpell) CanCastOnTarget(caster *Player, target *Mobile) bool { return true }

// StartCast is CombatSpell.startCast: as for
// combatNormalSpell (see startSpellCast). Tele Block's
// projectile is not here — TeleBlockSpell sends it from
// OnHitCalc once the hit is rolled.
func (s *combatEffectSpell) StartCast(caster *Mobile, target *Mobile) {
	startSpellCast(s, caster, target)
}

// FinishCast is CombatEffectSpell.finishCast
// (CombatEffectSpell.ts:49-53): the spell effect runs on
// accurate hits (damage is irrelevant: effect spells deal
// little or no damage).
func (s *combatEffectSpell) FinishCast(attacker, target *Mobile, accurate bool, damage int) {
	if !accurate {
		return
	}
	s.spellEffect(attacker, target)
}

// OnHitCalc is CombatSpell.onHitCalc. Tele Block's override
// (TeleBlockSpell, CombatSpells.ts:198-210) additionally
// sends the hit/miss projectile (1299 on a hit, 1300 on a
// miss) once the hit is rolled: the purple sphere on a hit,
// the old travel graphic on a fail.
func (s *combatEffectSpell) OnHitCalc(hit *PendingHit) {
	emitHitEffects(s, hit)
	if !s.def.teleblock || hit == nil {
		return
	}
	source := projectileMobile(hit.Attacker())
	victim := projectileMobile(hit.Target())
	if source == nil || victim == nil {
		return
	}
	projectileID := 1300
	if hit.IsAccurate() {
		projectileID = 1299
	}
	CreateProjectile(source, victim, projectileID, 0, 20, 43, 31).SendProjectile()
}

// SpellRadius is 0 (see combatNormalSpell).
func (s *combatEffectSpell) SpellRadius() int { return 0 }

// spellEffect is CombatEffectSpell.spellEffect
// (CombatEffectSpell.ts:54-61): the magic:utility-effect-check
// event, then magic:utility-effect, then the spell's own
// effect when no plugin handled it. Only Tele Block carries
// an own effect among the standard spells (its inline
// spellEffect, CombatSpells.ts:1678-1699); the binds and
// curses have none — their application is plugin content
// (StandardBindings / StandardCurses / Freezes) and a cast
// spends runes, rolls the hit and emits, but applies no
// freeze until its plugin ports.
func (s *combatEffectSpell) spellEffect(cast, target *Mobile) {
	var ctx *SpellCastContext
	if cast != nil {
		if p := combatantFor(cast).Player(); p != nil {
			ctx = spellCastContext(p, s)
		}
	}
	event := &MagicUtilityEffectPayload{
		Attacker: cast, Target: target, Spell: s, Context: ctx,
		Allowed: true, Handled: false,
	}
	emitCustomEvent(cast, target, "magic:utility-effect-check", event)
	if !event.Allowed {
		return
	}
	emitCustomEvent(cast, target, "magic:utility-effect", event)
	if !event.Handled && s.def.teleblock {
		teleblockSpellEffect(cast, target)
	}
}

// crumbleUndeadCanCastOnTarget is CRUMBLE_UNDEAD's
// canCastOnTarget (CombatSpells.ts:829-842): the target's
// definition must be undead, or the target unattackable
// (which leaves scripted, unattackable targets to their
// quest's handler — Enakhra's Lament's boneguard). Anything
// else gets the line and a refusal. A player target has no
// definition and is refused. A definition without an
// IsUndead method reads as not undead, and one without an
// IsAttackable method reads as attackable — the TS
// optional-chaining defaults (isUndead?.() is undefined,
// !isAttackable?.() is true only when the method is absent
// ... precisely: `definition.isUndead?.() ||
// !definition.isAttackable?.()`).
func crumbleUndeadCanCastOnTarget(cast *Player, target *Mobile) bool {
	const refusal = "This spell only affects skeletons, zombies, ghosts and shades."
	npc := combatantFor(target).NPC()
	if npc == nil {
		if cast != nil {
			cast.SendMessage(refusal)
		}
		return false
	}
	def := npc.Definition()
	if def == nil {
		if cast != nil {
			cast.SendMessage(refusal)
		}
		return false
	}
	undead := false
	if u, ok := def.(interface{ IsUndead() bool }); ok {
		undead = u.IsUndead()
	}
	attackable := true
	if a, ok := def.(interface{ IsAttackable() bool }); ok {
		attackable = a.IsAttackable()
	}
	if undead || !attackable {
		return true
	}
	if cast != nil {
		cast.SendMessage(refusal)
	}
	return false
}

// ---- Table -> spells ----

// runesToItems converts a table rune list to Items (TS `new
// Item(id, amount)`).
func runesToItems(runes []spellRune) []Item {
	if len(runes) == 0 {
		return nil
	}
	out := make([]Item, 0, len(runes))
	for _, r := range runes {
		out = append(out, NewItem(r.id, r.amount))
	}
	return out
}

// equipmentFuncFor resolves a table equipment kind to the
// equipmentRequired function (nil when the spell needs none).
func equipmentFuncFor(def *standardSpellDef) func(player *Player) []Item {
	switch def.equipment {
	case spellEquipmentGodSaradomin:
		return func(player *Player) []Item { return godStaffRequired(player, godSaradominStaves) }
	case spellEquipmentGodGuthix:
		return func(player *Player) []Item { return godStaffRequired(player, godGuthixStaves) }
	case spellEquipmentGodZamorak:
		return func(player *Player) []Item { return godStaffRequired(player, godZamorakStaves) }
	case spellEquipmentIban:
		return ibanStaffRequired
	default:
		return nil
	}
}

// newCombatSpellFromDef builds the CombatSpell for one table
// entry (the CombatNormalSpell / CombatEffectSpell options
// object, CombatSpells.ts). The spellbook is NORMAL for every
// standard-book spell (Spell.getSpellbook's default,
// Spell.ts:85-87).
func newCombatSpellFromDef(def *standardSpellDef) CombatSpell {
	base := &spellBase{
		id:                      def.id,
		level:                   def.level,
		runes:                   runesToItems(def.runes),
		equipmentRequired:       equipmentFuncFor(def),
		spellbook:               SpellbookNormal,
		members:                 def.members,
		missingEquipmentMessage: def.missingEquipmentMessage,
		sharedDelay:             true,
		offensive:               true,
	}
	if def.kind == "effect" {
		return &combatEffectSpell{base: base, def: def}
	}
	return &combatNormalSpell{base: base, def: def}
}

// standardCombatSpells are the 36 standard-book CombatSpells,
// singletons built once from the table in declaration order.
// The registry holds these same pointers, so identity
// comparisons (autocasting.go: autocastSpellIndex) see the
// table's spells.
var standardCombatSpells = func() []CombatSpell {
	out := make([]CombatSpell, 0, len(standardSpellDefs))
	for i := range standardSpellDefs {
		out = append(out, newCombatSpellFromDef(&standardSpellDefs[i]))
	}
	return out
}()

// InstallStandardSpells registers the standard spellbook and
// the utility table with the resolvers (spell_resolve.go:
// RegisterCombatSpellNamed / RegisterUtilitySpell). Call once
// at boot, before ticking starts — the cmd/server wiring
// calls it next to InstallRangedData. Re-registering the same
// singletons is a no-op in effect. Combat spells register
// under id and name (CombatSpells.getCombatSpell /
// getCombatSpellByName, CombatSpells.ts:2068-2116); utility
// spells register under id and their map aliases
// (EffectSpells.forSpellId, EffectSpells.ts:227-237, 264-265).
func InstallStandardSpells() {
	for _, spell := range standardCombatSpells {
		var def *standardSpellDef
		switch s := spell.(type) {
		case *combatNormalSpell:
			def = s.def
		case *combatEffectSpell:
			def = s.def
		}
		if def == nil {
			continue
		}
		RegisterCombatSpellNamed(def.id, def.name, spell)
	}
	for _, entry := range utilitySpells {
		RegisterUtilitySpell(entry.entry.id, entry)
		for _, alias := range entry.entry.aliases {
			RegisterUtilitySpell(alias, entry)
		}
	}
}

// uninstallStandardSpells removes every standard-book and
// utility registration InstallStandardSpells made. The
// registry is process-global, so a test that installs the
// table must unregister it on cleanup — otherwise a later
// test that expects an empty registry (TestCombatSpellRegistry)
// sees the real spells (notably "Fire Bolt") and fails.
func uninstallStandardSpells() {
	for _, def := range standardSpellDefs {
		UnregisterCombatSpell(def.id)
	}
	for _, entry := range utilitySpells {
		UnregisterUtilitySpell(entry.entry.id)
		for _, alias := range entry.entry.aliases {
			UnregisterUtilitySpell(alias)
		}
	}
}

// ---- Utility spells (EffectSpells.ts) ----

// utilitySpell is one EffectSpells static behind the
// SpellCaster surface (spell_resolve.go): what the scripted
// NPC route and the alchemy/telegrab listeners need. It is
// NOT a CombatSpell — TS EffectSpells extends Spell, and the
// combat method must never receive one. Handlers for the
// entries live with their listeners (Worker B: alchemy,
// Telekinetic Grab); the rest are table entries with
// documented NOT STARTED handlers (Bones to Bananas/Peaches,
// Superheat Item, Bake Pie, Vengeance, Vengeance Other).
type utilitySpell struct {
	entry *effectSpellEntry
	base  *spellBase
}

// newUtilitySpell builds the SpellCaster for one table entry.
func newUtilitySpell(entry *effectSpellEntry) *utilitySpell {
	return &utilitySpell{
		entry: entry,
		base: &spellBase{
			id:        entry.id,
			level:     entry.level,
			runes:     runesToItems(entry.runes),
			spellbook: entry.spellbook,
			members:   entry.members,
			// Utility spells do not use the shared cast delay
			// (Spell.usesSharedCastDelay is false; only
			// CombatSpell overrides it true).
			sharedDelay: false,
			offensive:   false,
		},
	}
}

// utilitySpells are the EffectSpells singletons, in table
// order, built once from effectSpellEntries.
var utilitySpells = func() []*utilitySpell {
	out := make([]*utilitySpell, 0, len(effectSpellEntries))
	for i := range effectSpellEntries {
		out = append(out, newUtilitySpell(&effectSpellEntries[i]))
	}
	return out
}()

// SpellID returns the utility spell's id.
func (s *utilitySpell) SpellID() int { return s.entry.id }

// Entry returns the table entry behind this spell (id, level,
// runes, spellbook, members) for its handlers.
func (s *utilitySpell) Entry() *effectSpellEntry { return s.entry }

// CanCast is Spell.canCast through the shared base.
func (s *utilitySpell) CanCast(caster *Player, consume bool) bool {
	return canCastSpell(caster, s.base, s, consume)
}

// CanCastOnTarget is Spell.canCastOnTarget (Spell.ts:61-64):
// true by default — utility spells cast on scripted NPCs
// (the Theatre's guardian) by default.
func (s *utilitySpell) CanCastOnTarget(caster *Player, target *Mobile) bool { return true }

// StartCast is EffectSpells.startCast (EffectSpells.ts:249-251):
// a no-op function in TS, so a no-op here.
func (s *utilitySpell) StartCast(caster *Mobile, target *Mobile) {}

// BaseExperience is the entry's verbatim base XP, rounded to
// the interface's int (see spellXP). Only Bones to
// Bananas/Peaches and Vengeance pay it (EffectSpells.
// handleSpell); the alchemy/telegrab entries' numbers are
// dead values TS never pays (combat_spells_data.go) — do not
// read them as XP.
func (s *utilitySpell) BaseExperience() int { return spellXP(s.entry.baseXP) }

// IsFruitBone reports whether id is a bone Bones to Bananas
// / Bones to Peaches turns into fruit (EffectSpells.
// FRUIT_BONES, EffectSpells.ts:19-21).
func IsFruitBone(id int) bool {
	for _, bone := range effectSpellFruitBones {
		if bone == id {
			return true
		}
	}
	return false
}

// ---- God spells (CombatSpells.ts:331-407) ----

// godStaffRequired is CombatSpells.godStaffRequired
// (CombatSpells.ts:378-384): the wielded staff when it is one
// of the set, else the set's first item, which the cast then
// reports missing.
//
// DEVIATION: TS also accepts ItemAliases.baseOf(weaponId) (a
// minigame copy — Last Man Standing's br_sotd — casts as the
// staff it copies). Go has no ItemAliases, so a copy does not
// count; only the listed ids do.
func godStaffRequired(player *Player, staves []int) []Item {
	weaponID := -1
	if player != nil && player.Equipment() != nil {
		weaponID = player.Equipment().GetWeapon().ID()
	}
	for _, id := range staves {
		if id == weaponID {
			return []Item{NewItem(weaponID, 1)}
		}
	}
	first := -1
	if len(staves) > 0 {
		first = staves[0]
	}
	return []Item{NewItem(first, 1)}
}

// ibanStaffRequired is IBAN_BLAST's equipmentRequired
// (CombatSpells.ts:974-980): the wielded Iban's staff when
// one is wielded, else Iban's staff itself (which the cast
// then reports missing — with the default line: Iban Blast
// sets no missingEquipmentMessage).
func ibanStaffRequired(player *Player) []Item {
	weaponID := -1
	if player != nil && player.Equipment() != nil {
		weaponID = player.Equipment().GetWeapon().ID()
	}
	for _, id := range ibanBlastStaves {
		if id == weaponID {
			return []Item{NewItem(weaponID, 1)}
		}
	}
	first := -1
	if len(ibanBlastStaves) > 0 {
		first = ibanBlastStaves[0]
	}
	return []Item{NewItem(first, 1)}
}

// godSpellDrain is CombatSpells.godSpellDrain
// (CombatSpells.ts:350-366): a player's hit lowers the
// target's Defence (Claws of Guthix) or Magic (Flames of
// Zamorak) by 1 + 5%. It neither stacks nor reapplies until
// the stat is back to its base level (a boosted stat is
// drained too).
//
// GAP — NPC targets: TS drains the NPC's current level
// (npc.setCurrentLevels). Go NPC levels are definition stats
// with no setter (combatant.go: "no drains in Go"), so the
// NPC branch is NOT ported; only player targets drain.
func godSpellDrain(cast, target *Mobile, skill int) {
	if cast == nil || !combatantFor(cast).IsPlayer() {
		return
	}
	victim := combatantFor(target).Player()
	if victim == nil || victim.Skills() == nil {
		return
	}
	skills := victim.Skills()
	current := skills.CurrentLevel(skill)
	if current < skills.MaxLevel(skill) {
		return
	}
	skills.SetCurrentLevelCombat(skill, max(0, current-(1+current*5/100)))
}

// saradominStrikeDrain is CombatSpells.saradominStrikeDrain
// (CombatSpells.ts:369-376): a player's hit lowers the
// target player's Prayer points by 1; most NPCs have none
// (TS returns unless the target is a player).
func saradominStrikeDrain(cast, target *Mobile) {
	if cast == nil || !combatantFor(cast).IsPlayer() {
		return
	}
	victim := combatantFor(target).Player()
	if victim == nil || victim.Skills() == nil {
		return
	}
	skills := victim.Skills()
	current := skills.CurrentLevel(SkillPrayer)
	if current <= 0 {
		return
	}
	skills.SetCurrentLevelCombat(SkillPrayer, current-1)
}

// ApplyChargeMaxHit is CombatSpells.applyChargeMaxHit
// (CombatSpells.ts:399-407): while Charged (the charge:until
// attribute past the current cycle) and wearing the matching
// god cape, a god spell's max hit is 30.
//
// GAP — no call site: TS calls it from
// DamageFormulas.getMagicHitBounds (DamageFormulas.ts:227),
// but Go's GetMagicHitBounds explicitly skips it
// (combat_formulas.go:905, "// combat:magic-base-max-hit,
// applyChargeMaxHit: skipped"). Wiring it through the
// existing ModifyMagicMaxHit hook would apply it AFTER the
// magic damage bonus, where TS applies it BEFORE (and
// before baseMaxHit is snapshotted), so no correct seam
// exists yet — the formulas file owns the call site, and it
// is outside this phase's file ownership. The function is
// implemented and unit-tested so the call site can land in
// one line when it does.
func ApplyChargeMaxHit(caster *Mobile, maxHit int, spell CombatSpell) int {
	if caster == nil || !combatantFor(caster).IsPlayer() {
		return maxHit
	}
	player := combatantFor(caster).Player()
	if player == nil {
		return maxHit
	}
	if attributeTick(player, chargeUntilAttribute) <= combatCycle() {
		return maxHit
	}
	if spell == nil {
		spell = player.GetCombat().SelectedSpell()
	}
	if spell == nil {
		return maxHit
	}
	capes, ok := godSpellCapes[spell.SpellID()]
	if !ok {
		return maxHit
	}
	capeID := -1
	if eq := player.Equipment(); eq != nil {
		capeID = eq.ItemAt(CapeSlot).ID()
	}
	for _, id := range capes {
		if id == capeID {
			return 30
		}
	}
	return maxHit
}

// ---- TeleBlock (TeleBlock.ts) ----

// teleBlockTicks converts seconds to ticks at 600 ms per tick
// (TickRateMs): 300 s is 500 ticks, 150 s is 250.
func teleBlockTicks(seconds int) int64 {
	return int64(seconds) * 1000 / TickRateMs
}

// TeleBlockIsBlocked is TeleBlock.isBlocked (TeleBlock.ts:22-24):
// the block runs while its expiry cycle is in the future.
func TeleBlockIsBlocked(player *Player) bool {
	if player == nil {
		return false
	}
	return attributeTick(player, teleBlockUntilAttribute) > combatCycle()
}

// TeleBlockRemainingSeconds reports the whole seconds left on
// a block (TeleBlock.ts:57-65, secondsRemaining, in ticks).
func TeleBlockRemainingSeconds(player *Player) int {
	if player == nil {
		return 0
	}
	remaining := attributeTick(player, teleBlockUntilAttribute) - combatCycle()
	if remaining <= 0 {
		return 0
	}
	return int(remaining * TickRateMs / 1000)
}

// TeleBlockDescribe is TeleBlock.describe (TeleBlock.ts:26-35):
// "5 minutes", "2 minutes, 30 seconds", "1 minute, 27
// seconds" — the captured wording.
func TeleBlockDescribe(totalSeconds int) string {
	seconds := totalSeconds
	if seconds < 0 {
		seconds = 0
	}
	minutes := seconds / 60
	rest := seconds % 60
	var parts []string
	if minutes > 0 {
		if minutes == 1 {
			parts = append(parts, "1 minute")
		} else {
			parts = append(parts, fmt.Sprintf("%d minutes", minutes))
		}
	}
	if rest > 0 || len(parts) == 0 {
		if rest == 1 {
			parts = append(parts, "1 second")
		} else {
			parts = append(parts, fmt.Sprintf("%d seconds", rest))
		}
	}
	return strings.Join(parts, ", ")
}

// TeleBlockApply is TeleBlock.apply (TeleBlock.ts:41-55):
// starts (or restarts) the block and tells the victim. The
// duration is 5 minutes, halved when Protect from Magic is
// up as the spell lands. It returns the seconds applied.
//
// GAP — the effect-timer frame: TS sends
// sendEffectTimer(seconds, EffectTimer.TELE_BLOCK). Go has
// no SendEffectTimer seam, so the frame is not sent (a
// Phase 29 presentation gap); the message and the block
// itself are live.
func TeleBlockApply(victim *Player, casterName string) int {
	if victim == nil {
		return 0
	}
	seconds := TeleBlockDurationSeconds
	if PrayerActive(&victim.Mobile, PrayerProtectFromMagic) {
		seconds = TeleBlockProtectedDurationSeconds
	}
	victim.SetAttribute(teleBlockUntilAttribute, combatCycle()+teleBlockTicks(seconds))
	victim.SetAttribute(TeleBlockWarnedAttribute, false)
	victim.SendMessage(fmt.Sprintf("<col=c356ef>A Tele Block spell has been cast on you by %s. It will expire in %s.</col>", casterName, TeleBlockDescribe(seconds)))
	return seconds
}

// TeleBlockRemove is TeleBlock.remove (TeleBlock.ts:57-60):
// stops the block. The effect-timer clear shares the
// SendEffectTimer gap above.
func TeleBlockRemove(player *Player) {
	if player == nil {
		return
	}
	player.SetAttribute(teleBlockUntilAttribute, nil)
}

// TeleBlockAttemptMessage is TeleBlock.attemptMessage
// (TeleBlock.ts:63-66): what a teleport attempt says while
// blocked.
func TeleBlockAttemptMessage(player *Player) string {
	return fmt.Sprintf("A teleport block has been cast on you. It should wear off in %s.", TeleBlockDescribe(TeleBlockRemainingSeconds(player)))
}

// TeleBlockWarn is TeleBlock.warn (TeleBlock.ts:68-71): the
// 30-second warning, sent once by the TeleBlock plugin's
// per-tick check (plugins/combat/TeleBlock.plugin.js).
func TeleBlockWarn(player *Player) {
	if player == nil {
		return
	}
	player.SetAttribute(TeleBlockWarnedAttribute, true)
	player.SendMessage(fmt.Sprintf("<col=c356ef>Your Tele Block will expire in %d seconds.", TeleBlockWarningSeconds))
}

// teleblockSpellEffect is TELEBLOCK's inline spellEffect
// (CombatSpells.ts:1678-1699): a player target already
// blocked is refused with a line to the caster; otherwise
// the block applies. An NPC target gets "Your spell has no
// effect on this target." (TS has no branch for a non-player
// non-NPC target.)
func teleblockSpellEffect(cast, target *Mobile) {
	if victim := combatantFor(target).Player(); victim != nil {
		if TeleBlockIsBlocked(victim) {
			if caster := combatantFor(cast).Player(); caster != nil {
				caster.SendMessage("The spell has no effect because the player is already teleblocked.")
			}
			return
		}
		casterName := "a monster"
		if caster := combatantFor(cast).Player(); caster != nil {
			casterName = caster.Username
		}
		TeleBlockApply(victim, casterName)
		return
	}
	if combatantFor(target).NPC() != nil {
		if caster := combatantFor(cast).Player(); caster != nil {
			caster.SendMessage("Your spell has no effect on this target.")
		}
	}
}

// ---- Charge (CombatSpells.ts:26-79) ----

// chargeSpell is ChargeSpell (CombatSpells.ts:26-79): the
// self-cast Charge spell (spellId 0 — it is not a CombatSpell
// and getCombatSpell(0) is nil in TS, CombatSpells.ts:2074).
// Level 80, members-only, 180 base XP, air/fire/blood runes,
// once every 60 seconds (100 ticks) on the charge:next-cast
// attribute, effect lasting 700 ticks on charge:until.
type chargeSpell struct {
	base *spellBase
}

// chargeSpellInstance is the Charge singleton (CombatSpells.
// CHARGE, CombatSpells.ts:388).
var chargeSpellInstance = &chargeSpell{base: &spellBase{
	id:        0,
	level:     80,
	runes:     []Item{NewItem(556, 3), NewItem(554, 3), NewItem(565, 3)},
	spellbook: SpellbookNormal,
	members:   true,
	// Charge does not use the shared cast delay (Spell base
	// default); its own cooldown gates it.
	cooldownAttr:  chargeNextCastAttribute,
	cooldownTicks: chargeCooldownTicks,
	sharedDelay:   false,
	offensive:     false,
}}

// CanCast is Spell.canCast through the shared base.
func (s *chargeSpell) CanCast(caster *Player, consume bool) bool {
	return canCastSpell(caster, s.base, s, consume)
}

// HandleSelfSpell is CombatSpells.handleSelf
// (CombatSpells.ts:395-397): the self-cast route
// (NetworkBuilder.ts:1073-1075). True when name is "charge"
// (trimmed, case-insensitive) — the cast is then attempted,
// refusals included. False for any other name: the caller
// tries the next self-cast route. The call site is
// listener-side (Worker B's interface-action dispatch);
// this function is the implementation.
func HandleSelfSpell(player *Player, name string) bool {
	if !strings.EqualFold(strings.TrimSpace(name), "charge") {
		return false
	}
	return chargeSpellInstance.cast(player)
}

// cast is ChargeSpell.cast (CombatSpells.ts:41-79): the Mage
// Arena cape gate (before the runes), then the canCast
// check-then-pay pair (the cooldown is checked before the
// runes are taken, so a refused cast costs nothing), then
// 180 Magic XP, the 700-tick effect, the red line, and the
// charge-cast event. The animation (811), graphic (111) and
// sound (1651) have no Go producers (Phase 29) and are not
// played.
func (s *chargeSpell) cast(player *Player) bool {
	if player == nil {
		return true
	}
	// Wiki (Charge): it needs the Mage Arena I miniquest,
	// which ends with the statues' god cape (the Mage Arena
	// plugin answers "mage-arena:cape-received"). Refused
	// before the runes; TSPS wording for the line.
	arena := &MageArenaCapeReceivedPayload{Player: player}
	emitPlayerEvent(player, "mage-arena:cape-received", arena)
	if arena.Received == nil || !*arena.Received {
		// A league relic may lift every spell's quest
		// requirement (Wiki: Grimoire), as for Dream
		// Mentor's spells.
		unlock := &MagicSpellUnlockedPayload{Player: player, Name: "charge"}
		emitPlayerEvent(player, "magic:spell-unlocked", unlock)
		if !unlock.Unlocked {
			player.SendMessage("You need to complete the Mage Arena to cast this spell.")
			return true
		}
	}
	if !s.CanCast(player, false) || !s.CanCast(player, true) {
		return true
	}
	if skills := player.Skills(); skills != nil {
		skills.AddExperiences(SkillMagic, 180)
	}
	until := combatCycle() + ChargeTicks
	player.SetAttribute(chargeUntilAttribute, until)
	player.SendMessage("<col=ef1020>You feel charged with magic power.</col>")
	// ChargeSpell.cast (CombatSpells.ts:68-70): the red line
	// above, human_casting (811), the teleport_casting
	// graphic (111) at delay 32 and height 128, and the
	// synth sound 1651 (sendSoundEffect, loop type 1,
	// delay 0, volume 255). The sound has no named Sound
	// static — Worker C's table carries no 1651 entry —
	// so it goes through SendSoundEffect directly.
	player.PerformAnimation(811, 0)
	player.PerformGraphic(111, 128, 32)
	SendSoundEffect(player, 1651, 1, 0, 255)
	emitPlayerEvent(player, "magic:charge-cast", &MagicChargeCastPayload{Player: player, Until: until})
	return true
}
