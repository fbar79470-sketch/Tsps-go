// Package crafting is the Go port of the Crafting skill plugin
// (server/plugins/skills/Crafting.plugin.js, 31 lines, commit
// bfd81e3): sewing leather and hides, cutting gems and amethyst,
// gold and silver jewellery, glassblowing, pottery, spinning and
// weaving, battlestaves, snelms, limestone bricks, and gems on
// silver sickles (b), from data/definitions/crafting.json. The
// plugin is F2P (no `members` export in the registrar); members
// recipes are gated per-recipe through the members-world flag.
//
// File split (PHASE44_PLAN.md §3, foundation worker):
//
//   - tables.go: the loader + all table types for the 16 JSON
//     sections.
//   - item_ids.go: the fallback item ids (ItemIdentifiers.ts
//     transcription).
//   - shared.go: the Shared.Crafting.js port — level/actions/
//     interval queries, XP, members filter, spam, nextTick,
//     openMakeMenu, and the startMaking make-many loop (sessions
//     map + one global 1-tick task, the mining/smithing shape).
//   - types.go: the custom-event payloads.
//   - crafting.go (this file): the plugin value, the shared
//     instance wiring, the crafting:cut-gem answerer, and the
//     12 unit attach entry points (stubs for the unit worker).
//   - leather.go, gems.go, jewellery.go, glassblowing.go,
//     pottery.go, spinning.go, battlestaves.go, snelms.go,
//     limestone.go, lightorbs.go, buttons.go, gemsickles.go
//     (unit worker): one file per TS unit.
//
// State lives in a *crafting created per Plugin value — NO
// package-level mutable maps (the Phase 21 race fix): the
// shared instance behind Plugin is the only one, and tests
// build their own with newCrafting.
package crafting

import (
	"tsps-go/internal/plugins"
)

// crafting is the plugin's per-registration state; see
// shared.go for its fields.
func (c *crafting) register(api *plugins.Api) {
	if api == nil || c.tables == nil {
		return
	}
	c.api = api
	c.emit = api.EmitCustomEvent
	api.PersistAttribute(ThreadUsesAttribute, 0)
	api.OnCustomEvent(InteractionClearedEventName, c.stopCraftingOnInteractionCleared)
	api.OnCustomEvent(CutGemEventName, c.answerCutGem)
	c.AttachLeather(api)
	c.AttachGems(api)
	c.AttachSnelms(api)
	c.AttachLimestone(api)
	c.AttachButtons(api)
	c.AttachJewellery(api)
	c.AttachGlassblowing(api)
	c.AttachLightOrbs(api)
	c.AttachPottery(api)
	c.AttachSpinning(api)
	c.AttachBattlestaves(api)
	c.AttachGemSickles(api)
}

// plugin builds the registered Plugin value: name and
// description verbatim (module.exports, Crafting.plugin.js),
// attach-only register. Crafting is F2P: the registrar exports
// no `members` flag, so Members stays false and the hub loads
// the plugin on free worlds too; members recipes are gated
// per-recipe through membersWorld (Shared.available).
func (c *crafting) plugin() plugins.Plugin {
	return plugins.Plugin{
		Name:        "Crafting",
		Description: "Crafting: leather, gems, jewellery, glassblowing, pottery, spinning, weaving, battlestaves, snelms, limestone bricks and light orbs.",
		Members:     false,
		Register: func(api *plugins.Api) {
			c.register(api)
		},
	}
}

// Plugin is the registered Crafting plugin value.
var Plugin = shared.plugin()

func init() {
	plugins.Register(Plugin)
}

// answerCutGem answers the "crafting:cut-gem" query
// (Gems.Crafting.js answerCutGem): the cut gem and XP for
// request.UncutID, for content that cuts as it mines (a league
// relic). A miss leaves the request unanswered. No Go asker
// exists yet — the answerer is ported anyway, mirroring the
// cooking:raw-xp precedent (cooking/common.go answerRawXP).
func (c *crafting) answerCutGem(payload any) {
	request, ok := payload.(*CutGemRequest)
	if !ok || request == nil || c == nil || c.tables == nil {
		return
	}
	cutID, xp, ok := c.tables.CutGem(request.UncutID)
	if !ok {
		return
	}
	request.CutID = cutID
	request.XP = xp
}
