package plugins

import "log"

// Gate contracts: the synchronous ask-the-plugins shapes, one generic helper
// per TS emit pattern. Each helper is grounded in the emit body it ports;
// there is deliberately NO uniform gate rule, because TS has none — the
// emits differ in reset behavior, short-circuiting, and return shape, and
// those differences are behavior content plugins rely on.
//
// Gates are wired per family as their systems land: the first-answer
// families onCanTrade, onCanShop, and onCanStockItem and the
// handled family onTradeRequest are ported (shops.go, trades.go)
// and fire from the shop/trade systems through the game-side
// seams; every other gate family below stays registered-but-unfired
// (see the unfired-family table in hooks.go) with no production
// call site yet. A future phase wires each remaining family to its
// helper with the family's concrete event type.
//
// All helpers take hookEntry slices (the same entries the registry stores),
// run handlers through executeHook (a panicking hook is logged and the gate
// continues), and read answers back from the event through accessor
// functions — TS gates mutate a shared event object (allow: boolean |
// null), and the accessors are the Go form of reading those fields. A nil
// answer (*bool nil) is TS null: undecided. A nil logger falls back to
// log.Default() via executeHook.
//
// Shapes TS uses that have NO helper here (each needs its own emit when its
// family lands, and must not be forced into the helpers below):
//
//   - run-all-then-read: emitNpcBeforeDeath (PluginManager.ts:1331-1336) and
//     emitPlayerBeforeDeath (1717-1722) run every hook and return
//     preventDeath; emitObjectRoute (1214-1224) runs until destination is
//     set; emitNpcRoute (1235-1243) runs every hook then clamps range;
//     emitAttackSpeed (1780-1787) and emitCombatAttackDistance (1817-1822)
//     run every hook then sanitize a number.
//   - run-all handled (no short-circuit): emitItemOnObject (1915-1923,
//     plus a fallback list), emitItemOnItem (1925-1930), emitItemOnNpc
//     (1932-1937), emitItemOnPlayer (1939-1944), emitItemOnGroundItem
//     (1946-1958), emitSpellOnObject (1960-1965), emitGroundItemPickup
//     (1908-1913), emitItemAction (2016-2043, plus a fallback list and the
//     item:operated custom emit), emitItemDropPolicy (2045-2050). These run
//     EVERY hook and return handled; runHandledGate short-circuits and must
//     not be substituted.
//   - veto-plus-accumulator: emitCanGainExperience (1665-1674) runs every
//     hook, returns immediately on a deny (with multiplier reset to 1), and
//     sanitizes the combined multiplier at the end.
//   - two-phase: emitCommand (2182-2205) runs the any-command hooks, then
//     the per-base handlers after a rights check.

// defaultLogoutRefusal is the reason emitCanLogout returns when a hook
// denies logout without giving a reason (PluginManager.ts:1398). The
// reason-gate helper takes the default as a parameter; this constant is
// what the future onCanLogout emit passes.
const defaultLogoutRefusal = "You can't log out right now."

// boolAnswer returns a fresh pointer to b: a decided tri-state answer. Gate
// helpers always return fresh pointers, never pointers into the event, so a
// later reset cannot rewrite an answer already returned.
func boolAnswer(b bool) *bool { return &b }

// BoolPtr returns a fresh pointer to b: the value a gate hook assigns to
// its event's answer field (TS `event.allow = false` becomes
// `event.Allow = plugins.BoolPtr(false)`). A nil answer is TS null
// (undecided); a non-nil pointer is the hook's answer. Gate emits
// return fresh pointers of their own (boolAnswer), never a pointer a
// hook stored, so a hook may reuse one pointer across emits.
func BoolPtr(b bool) *bool { return &b }

// runAllowVetoGate ports emitCanAttack (PluginManager.ts:1360-1377), the
// allow-veto contract:
//
//   - allow is reset to undecided (reset) before EACH hook, so a hook only
//     ever sees its own answer.
//   - A deny returns deny immediately; hooks after it never run.
//   - An allow sticks: later silence cannot un-allow it, but a later deny
//     still vetoes, so hooks after an allow DO run.
//   - The result is allow (some hook allowed, none denied) or undecided
//     (nil) — never a bare deny-from-silence.
//
// Families using this contract: onCanAttack only. No other TS emit resets
// the answer before each hook.
func runAllowVetoGate[E any](logger *log.Logger, label string, hooks []hookEntry[E], event E, reset func(E), answer func(E) *bool) *bool {
	allowed := false
	for _, hook := range hooks {
		reset(event)
		executeHook(logger, hook.pluginName, label, hook.handler, event)
		if a := answer(event); a != nil {
			if !*a {
				return boolAnswer(false)
			}
			allowed = true
		}
	}
	if allowed {
		return boolAnswer(true)
	}
	return nil
}

// runFirstAnswerGate ports the first-answer contract: the first non-null
// answer wins and later hooks never run. The answer is NOT reset between
// hooks (unlike the allow-veto gate), and an allow returns just as
// immediately as a deny. TS implements it both inline (emitCanTeleport,
// PluginManager.ts:1379-1392; emitCanEat, 1404-1416; emitCanDrink,
// 1442-1454; emitCanTrade, 1456-1469; emitCanBank, 1510-1522;
// emitCanBankItem, 1524-1532; emitCanShop, 1534-1546;
// emitShouldDropItemsOnDeath, 1548-1569; emitShouldKeepItemOnDeath,
// 1571-1589; emitCanEquip, 1627-1637; emitCanUnequip, 1706-1715;
// emitSpellDisabled, 1824-1849; emitSpellRuneBypass, 1851-1876;
// emitNpcAggressionTolerance, 1878-1899) and through the shared firstAnswer
// helper (PluginManager.ts:1639-1652: emitCanUseItem, emitCanSpawnNpc,
// emitCanStockItem, and — on the "disabled" key, with a message mapping
// applied by the emit afterwards — emitPrayerDisabled and
// emitSpecialAttackDisabled). The answer key differs per family ("allow",
// "disabled", "bypass", "override", "shouldDrop", "keep"); the accessor
// abstracts it, and the message mapping for the disabled families stays in
// the future emit, not in this helper.
//
// Families using this contract: onCanTeleport, onCanEat, onCanDrink,
// onCanTrade, onCanBank, onCanBankItem, onCanShop,
// onShouldDropItemsOnDeath, onShouldKeepItemOnDeath, onCanEquip,
// onCanUnequip, onCanUseItem, onCanSpawnNpc, onCanStockItem,
// onSpellDisabled, onSpellRuneBypass, onNpcAggressionTolerance,
// onPrayerDisabled, onSpecialAttackDisabled.
func runFirstAnswerGate[E any](logger *log.Logger, label string, hooks []hookEntry[E], event E, answer func(E) *bool) *bool {
	for _, hook := range hooks {
		executeHook(logger, hook.pluginName, label, hook.handler, event)
		if a := answer(event); a != nil {
			return boolAnswer(*a)
		}
	}
	return nil
}

// runReasonGate ports emitCanLogout (PluginManager.ts:1394-1402), the
// reason-gate contract: the first decided answer wins (an allow returns
// immediately with no reason, like the first-answer gate), and a deny
// returns the event's reason, defaulting to defaultReason when the event
// carries none. The return is "" for allowed-or-undecided (TS null) and a
// non-empty reason for denied.
//
// The reason accessor returns (reason, present): present distinguishes "the
// hook set a reason" from "no reason was set", mirroring TS
// `event.reason ?? "You can't log out right now."` — ?? triggers only on
// null/undefined, so a present-but-empty reason is returned as-is, and the
// caller treats "" as allowed (as TS does: the refusal is checked for
// truthiness). Collapsing present-but-empty into the default would turn a
// TS allow into a refusal.
//
// Families using this contract: onCanLogout only.
func runReasonGate[E any](logger *log.Logger, label string, hooks []hookEntry[E], event E, answer func(E) *bool, reason func(E) (string, bool), defaultReason string) string {
	for _, hook := range hooks {
		executeHook(logger, hook.pluginName, label, hook.handler, event)
		if a := answer(event); a != nil {
			if !*a {
				if r, ok := reason(event); ok {
					return r
				}
				return defaultReason
			}
			return ""
		}
	}
	return ""
}

// runHandledGate ports the short-circuit handled contract: hooks run in
// order until the event is handled, checking handled before each hook, and
// the result is whether the event ended handled. It mirrors the loop shape
// shared by emitSocialPacket (PluginManager.ts:836-843), emitNpcInteraction
// (1246-1263), emitWorldEntityInteraction (1265-1275), emitNpcAttackOption
// (1277-1287), emitTradeRequest (1471-1481), emitPlayerDeath (1724-1735),
// emitPlayerOption (1737-1748), emitButtonClick (2052-2074),
// emitInterfaceActionClick (2076-2105), emitSpellOnNpc (1967-1973),
// emitSpellOnItem (1975-1985), emitSpellCast (1987-1993),
// emitGroundItemInteraction (1995-2014), emitFiremakingBlocked (1418-1440),
// emitPlayerDeathItemDrop (1591-1616), emitWalkRoute (1226-1233), and the
// before/general loops of emitObjectInteraction (1170-1212; the
// before-hooks, the merged ordered lists, and the definition/option
// fill-ins are per-emit work, not part of this helper).
//
// A pre-handled event runs zero hooks and returns true, matching
// emitNpcInteraction. Per-emit entry guards live in the future emit, not
// here: emitSocialPacket and several others early-return FALSE for a
// pre-handled event (and check player/object presence), which differs from
// the loop shape and must be reproduced at the emit site. Do not use this
// helper for the run-all handled families listed in the file header.
//
// Families using this contract (short-circuit form): onSocialPacket,
// onNpcInteraction (including onBeforeNpcInteraction, onNpcsInteraction,
// onAnyNpcInteraction, and the onNpcClick family, whose routing feeds the
// same loop), onWorldEntityInteraction, onNpcAttackOption,
// onObjectInteraction (including onBeforeObjectInteraction and the
// onObjectClick family), onTradeRequest, onPlayerDeath, onPlayerOption,
// onButtonClick, onButton, onInterfaceActionClick,
// onInterfaceActionButton, onSpellOnNpc, onSpellOnItem, onSpellCast,
// onGroundItemClick, onFiremakingBlocked, onPlayerDeathItemDrop,
// onWalkRoute.
func runHandledGate[E any](logger *log.Logger, label string, hooks []hookEntry[E], event E, handled func(E) bool) bool {
	for _, hook := range hooks {
		if handled(event) {
			break
		}
		executeHook(logger, hook.pluginName, label, hook.handler, event)
	}
	return handled(event)
}

// dialogueHook is one dialogue-variant/condition handler: unlike other
// hooks, it RETURNS an answer instead of mutating the event. It mirrors the
// hook shapes in npcDialogueVariantHooks / npcDialogueConditionHooks
// (PluginManager.ts:313-320).
type dialogueHook[E any, R any] struct {
	pluginName string
	handler    func(E) R
}

// runFirstDialogueAnswer ports the dialogue first-answer contract shared by
// emitNpcDialogueVariant (PluginManager.ts:1289-1304) and
// emitNpcDialogueCondition (1306-1320): hooks run in order, the first
// answer wins, and later hooks never run. An answer is whatever isAnswer
// accepts: a truthy result for variants (a non-empty variant name or a
// non-nil {page, variant} — the future emit supplies the predicate), a
// boolean for conditions. No answer yields (zero, false).
//
// Failures differ from executeHook on purpose: TS does NOT route dialogue
// hooks through executeHook. A throwing hook is caught, warned as
// "[plugins] <label> hook threw (<pluginName>)" — note "threw", not "hook
// failed" — and the gate continues with the next hook. The caller supplies
// label as the full TS phrase ("npc dialogue variant" or "npc dialogue
// condition") so the message matches the source exactly.
//
// Families using this contract: onNpcDialogueVariant, onNpcDialogueCondition.
func runFirstDialogueAnswer[E any, R any](logger *log.Logger, label string, hooks []dialogueHook[E, R], event E, isAnswer func(R) bool) (R, bool) {
	var zero R
	for _, hook := range hooks {
		result, ok := runDialogueHook(logger, label, hook, event)
		if !ok {
			continue
		}
		if isAnswer(result) {
			return result, true
		}
	}
	return zero, false
}

// runDialogueHook runs one dialogue hook, recovering a panic into a warning
// (see runFirstDialogueAnswer). It reports false when the hook panicked, so
// the gate skips its (zero) result and continues.
func runDialogueHook[E any, R any](logger *log.Logger, label string, hook dialogueHook[E, R], event E) (result R, ok bool) {
	defer func() {
		if r := recover(); r != nil {
			if logger == nil {
				logger = log.Default()
			}
			logger.Printf("[plugins] %s hook threw (%s): %v", label, hook.pluginName, r)
			var zero R
			result, ok = zero, false
		}
	}()
	return hook.handler(event), true
}
