// Plugin wiring: the Fletching plugin value, the shared
// instance, and the fletching:shafts answer
// (server/plugins/skills/Fletching.plugin.js, 36 lines, commit
// bfd81e3).
package fletching

import (
	"tsps-go/internal/plugins"
)

// shared is the instance behind Plugin; SetTables operates
// on it.
var shared = newFletching(nil)

// SetTables installs the resolved tables for the registered
// Plugin value. There is no CacheDefinitions accessor on Api,
// so the coordinator's integration step must call this with
// LoadFromCache's result BEFORE hub Load: Register captures
// the shared instance, so a call before Load reaches every
// handler; a call after Load is too late for the pair index
// the handlers read (the woodcutting SetTables precedent).
// With no tables installed, every handler degrades to a
// no-op (handlePair claims nothing, the spool's ids read 0).
func SetTables(t *Tables) {
	shared.setTables(t)
}

// answerShafts answers the "fletching:shafts" query
// (Fletching.plugin.js answerShafts): the arrow shafts
// request.logId fletches into, how many and the XP, for
// content that fletches as it chops (a league relic); a log
// with no arrow-shaft recipe answers nothing. The request is
// mutated in place, and Go's EmitCustomEvent passes the same
// pointer the emitter supplied (the woodcutting query
// precedent), so the mutation is visible to the emitter. No
// Go consumer exists yet (the Woodcutting league relic is
// unported).
func (f *fletching) answerShafts(payload any) {
	request, ok := payload.(*ShaftsRequest)
	if !ok || request == nil || f.tables == nil {
		return
	}
	for _, recipe := range f.tables.Recipes {
		if recipe.OutputID != f.tables.ArrowShaftID {
			continue
		}
		for _, material := range recipe.Materials {
			for _, id := range material.IDs {
				if id == request.LogID {
					request.ItemID = recipe.OutputID
					request.Amount = recipe.Amount
					request.XP = recipe.XP
					return
				}
			}
		}
	}
}

// plugin builds the registered Plugin value: name,
// description, and members flag verbatim (module.exports,
// Fletching.plugin.js), attach-only register.
//
// MEMBERS GATING: members:true. TS skips members plugins on
// free worlds (PluginManager.ts:751,767); Go mirrors it in the
// hub itself — filterToLoad drops Members plugins when the
// world is not a members world (plugins/loader.go), fed by
// HubOptions.MembersWorld (cmd/server/pluginwiring.go). There
// is deliberately no SetMembersWorld / SetIsMembersItem here:
// unlike Woodcutting (members axes, members rations),
// Fletching has no item-level members check anywhere in its
// TS — the gate is plugin-level only. The F2P skip is verified
// by TestMembersGatingSkipsFreeWorld.
func (f *fletching) plugin() plugins.Plugin {
	return plugins.Plugin{
		Name:        "Fletching",
		Description: "Fletching: every Wiki recipe for bows, arrows, bolts and more, with menus and animations, plus an arrow shaft event for other content.",
		Members:     true,
		Register: func(api *plugins.Api) {
			f.api = api
			f.emit = api.EmitCustomEvent
			// TS register order (Fletching.plugin.js): init
			// every unit, then attach every unit, then
			// onCustomEvent fletching:shafts. The Go init half
			// is SetTables (before Load) plus the constructor;
			// the attach half is below, in UNITS order
			// (Shared has no hooks of its own; Recipes, then
			// Spool, then Diaries).
			f.registerRecipes(api)
			f.registerSpool(api)
			f.registerDiaries(api)
			api.OnCustomEvent("fletching:shafts", f.answerShafts)
		},
	}
}

// Plugin is the registered Fletching plugin value.
var Plugin = shared.plugin()

func init() {
	plugins.Register(Plugin)
}
