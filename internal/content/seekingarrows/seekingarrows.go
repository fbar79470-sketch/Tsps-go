// Package seekingarrows is the Go port of the SeekingArrows plugin
// (server/plugins/items/SeekingArrows.plugin.js): seeking dragon
// arrows raise successful bow hits to at least three damage, and
// special minima do not stack.
//
// The port is faithful: same plugin name, description, and members
// flag, same attach-only register, same guard order, same damage
// floor. Line numbers in comments cite SeekingArrows.plugin.js.
//
// Event inventory:
//
//   - Listens to onCombatHitDamage (an already-firable hub family
//     since phase 16): produced by the hit pipeline through the
//     cmd/server adapter on every rolled hitsplat, before the
//     damage caps to the target's remaining hitpoints.
//   - Emits nothing; the TS plugin emits no custom events.
//
// Production gap, documented not hidden: no ranged-data resolver
// is registered in production (game.RegisterRangedDataResolver
// has zero callers), so no ranged weapon is ever detected and
// every fighter attacks in melee — the handler's RANGED gate
// never passes until ranged weapon/ammunition data is ported.
// The phase smoke test registers a test-only resolver to prove
// the pipeline plus handler end to end; no production resolver
// is invented here.
//
// Two deviations, both forced by the language change:
//
//   - Damage arrives as `any` (the hub event keeps Hit/Damage
//     opaque, mirroring the `any` fields of
//     PluginCombatHitDamageEvent, PluginTypes.ts:647-655); the
//     handler type-asserts it to *game.HitDamage and no-ops on
//     any other type. The production adapter always supplies
//     the shared *game.HitDamage, so the assertion holds on
//     every real hit.
//   - The ammunition check compares the resolved ammunition's
//     item id against SeekingDragonArrowItemID instead of
//     testing singleton identity as TS does (ammunition ===
//     core.Ammunition.SEEKING_DRAGON_ARROW): the TS singleton
//     has no Go form, and AmmunitionData identity is
//     pointer-based. The id is the singleton's own item id, so
//     the check matches exactly the ammunition TS matches —
//     including excluding the noted form (33596), which has no
//     Ammunition entry (each Ammunition registers only its own
//     item id, RangedData.ts:309).
package seekingarrows

import (
	"tsps-go/internal/game"
	"tsps-go/internal/plugins"
)

// SeekingDragonArrowItemID is the item id of the seeking dragon
// arrow (ItemIdentifiers.SEEKING_DRAGON_ARROW,
// ItemIdentifiers.ts:31169), the ammunition whose singleton
// Ammunition.SEEKING_DRAGON_ARROW (RangedData.ts:43) the TS
// handler tests by identity.
const SeekingDragonArrowItemID = 33595

// seekingMinimum raises a successful bow hit with seeking dragon
// arrows to at least three damage (seekingMinimum,
// SeekingArrows.plugin.js:4-11), in the TS guard order:
// accurate, player attacker, RANGED combat type, no active
// special, seeking dragon arrow ammunition. The isPlayer and
// getAsPlayer steps collapse into one AsPlayer call — nil
// exactly when TS returns — and the emit fires before the
// hitpoints cap, so a floored splat still caps to the target's
// remaining hitpoints, as in TS.
func seekingMinimum(event *plugins.CombatHitDamageEvent) {
	if event == nil || !event.Accurate {
		return
	}
	// attacker?.isPlayer?.() (line 6): a nil attacker, or a
	// non-player attacker, yields no player and returns.
	// AsPlayer is nil-safe on a nil Mobile.
	player := event.Attacker.AsPlayer()
	if player == nil {
		return
	}
	if event.CombatType != plugins.CombatTypeRanged {
		return
	}
	combat := player.GetCombat()
	// Special minima do not stack (line 8): a special with its
	// own minimum keeps it, and this floor does not apply.
	if combat.IsSpecialActivated() {
		return
	}
	ammunition := combat.Ammunition()
	if ammunition == nil || ammunition.ItemID != SeekingDragonArrowItemID {
		return
	}
	damage, ok := event.Damage.(*game.HitDamage)
	if !ok || damage == nil {
		return
	}
	damage.SetDamage(max(3, damage.Damage()))
}

// Plugin is the registered SeekingArrows plugin value: name,
// description, and members flag verbatim
// (SeekingArrows.plugin.js:14-16), attach-only register (lines
// 17-20). No dependsOn, no config declarations.
var Plugin = plugins.Plugin{
	Name:        "SeekingArrows",
	Description: "Makes successful bow hits with seeking dragon arrows deal at least three damage.",
	Members:     true,
	Register: func(api *plugins.Api) {
		api.OnCombatHitDamage(seekingMinimum)
	},
}

func init() {
	plugins.Register(Plugin)
}
