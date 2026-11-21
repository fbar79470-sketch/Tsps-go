// Recipes unit: the item-on-item entry point for every
// fletching recipe, the Celastrus bark Fletch action, and the
// disconnect / level-up lifecycle hooks
// (server/plugins/skills/fletching/Recipes.Fletching.js, 34
// lines, commit bfd81e3). The pairs, menus, and timing live in
// shared.go; this file only wires the hooks.
package fletching

import (
	"tsps-go/internal/plugins"
	"tsps-go/internal/plugins/skillactions"
)

// registerRecipes attaches the Recipes unit's hooks, in TS
// attach order (Recipes.Fletching.js attach): the generic
// onItemOnItem (noted:false), the Celastrus bark Fletch
// action, onPlayerDisconnect, and the level-up pause.
func (f *fletching) registerRecipes(api *plugins.Api) {
	api.OnItemOnItem(f.itemOnItem, &plugins.ItemUseFilter{Noted: plugins.BoolPtr(false)})
	api.OnItemActionNamed("Celastrus bark", map[string]plugins.ItemActionHandler{"Fletch": f.fletchBark}, nil)
	api.OnPlayerDisconnect(f.onPlayerDisconnect)
	api.OnCustomEvent(skillactions.LevelUpPopupOpenedEventName, f.pauseFletching)
}

// itemOnItem is the generic item-on-item handler
// (Recipes.Fletching.js itemOnItem): every recipe starts here.
// It claims the use when Fletching took the click.
func (f *fletching) itemOnItem(event *plugins.PluginItemOnItemEvent) {
	if event == nil || event.Player == nil {
		return
	}
	if f.handlePair(event.Player, event.UsedItemID, event.UsedWithItemID) {
		event.Handled = true
	}
}

// fletchBark is the Celastrus bark Fletch option
// (Recipes.Fletching.js fletchBark): the knife-on-bark pair,
// which opens the knife menu for the battlestaff recipe. It
// returns false (falls through) when the pair took nothing —
// with no tables installed there is no recipe to open, so
// another handler may still claim the click.
func (f *fletching) fletchBark(event *plugins.PluginItemActionEvent) bool {
	if event == nil || event.Player == nil || f.tables == nil {
		return false
	}
	return f.handlePair(event.Player, f.tables.KnifeID, f.tables.CelastrusBarkID)
}

// onPlayerDisconnect stops the player's task with no
// animation reset (Recipes.Fletching.js stop;
// Hub.EmitPlayerDisconnect, plugins/hooks.go:1264).
func (f *fletching) onPlayerDisconnect(event *plugins.PlayerDisconnectEvent) {
	if event == nil {
		return
	}
	f.stopFletching(event.Player, false)
}
