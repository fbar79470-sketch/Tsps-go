// ThickVines is the Go port of the ThickVines sub-unit
// (server/plugins/skills/woodcutting/ThickVines.Woodcutting.js,
// 278 lines, commit bfd81e3): the clearable Tar Swamp vegetation.
// "Thick vine", "Thick vines" and "Tendrils" chop with any axe,
// while "Vines" clears with a rake, opening the swamp paths south
// to the Deranged archaeologist. One clearing gives 2 XP in the
// tool's skill and regrows after 29.4s (49 ticks).
//
// Line numbers in comments cite ThickVines.Woodcutting.js unless
// another file is named. The port is faithful: same constants,
// same group classification (by cache name + own option, so the
// Brimhaven Dungeon's "Vines" — chopped, not cleared — never
// reach the clear path), same pending-by-location rule (a double
// click pays XP and schedules regrowth only once), same +2-tick
// clear delay, same 49-tick regrow that re-registers only if the
// tile is empty.
//
// Deviations forced by missing Go seams, named inline:
//
//   - Tool use registers by name, not by one plain onItemOnObject
//     handler: ItemOnObjectEvent carries no object definition,
//     and content has no definition resolver, so groupFor cannot
//     run on a tool use. The named registrations
//     (OnItemOnObjectNamed per axe display name x chop name, and
//     "Rake" x "Vines") are the exact equivalent — the hub's name
//     match is the group filter — and the handlers still check the
//     item id (axeById / rake id) and the live plant, in TS order.
//     The axe display names are the cache display names, verified
//     against the real rev-242 cache (probe, 2026-11-20); a wrong
//     name would silently never fire, so they are pinned by
//     TestAxeDisplayNamesCoverAxes.
//   - The rake id resolves through Tables.RakeID (the gameval
//     "rake"), not a display-name scan: the woodcutting package
//     has no cache scan seam, and the probe verified the gameval
//     resolves to the scan's result (5341, the first unnoted
//     "rake" in id order; 5342 is noted, 13880 a second unnoted
//     rake).
//   - The face-object step uses the new player seam
//     (Player.ForcePositionToFace, internal/game/player_facing.go);
//     TS prefers forcePositionToFace and falls back to
//     setPositionToFace, and Go always has the forced form.
package woodcutting

import (
	"fmt"
	"strings"

	"tsps-go/internal/game"
	"tsps-go/internal/plugins"
)

// ---- constants (ThickVines.Woodcutting.js:15-31) ----

var (
	// thickVineChopNames are the choppable names (CHOP_NAMES).
	thickVineChopNames = []string{"Thick vine", "Thick vines", "Tendrils"}
	// thickVineClearNames are the clearable names (CLEAR_NAMES).
	thickVineClearNames = []string{"Vines"}
)

const (
	// thickVineChopAction is the chop option (CHOP_ACTION).
	thickVineChopAction = "Chop"
	// thickVineClearAction is the clear option (CLEAR_ACTION).
	thickVineClearAction = "Clear"
	// thickVineClearXP is one clearing's reward (CLEAR_XP).
	thickVineClearXP = 2
	// thickVineRespawnTicks is the regrowth delay (RESPAWN_TICKS).
	thickVineRespawnTicks = 49
	// thickVineClearDelayTicks is the ticks between the swing and
	// the plants falling (CLEAR_DELAY_TICKS).
	thickVineClearDelayTicks = 2
	// thickVineRakeAnimation is Farming's rake animation
	// (RAKE_ANIMATION, Patches.Farming.js ANIM.RAKE).
	thickVineRakeAnimation = 2273
	// thickVineNoAxeMessage is the missing-axe line
	// (NO_AXE_MESSAGE).
	thickVineNoAxeMessage = "You don't have an axe which you can use."
	// thickVineNoRakeMessage is Farming's missing-tool line
	// (NO_RAKE_MESSAGE, Patches.Farming.js requireTool).
	thickVineNoRakeMessage = "You need a rake to do that."
	// thickVineAlreadyChoppedMessage is the already-chopped line
	// (ALREADY_CHOPPED_MESSAGE).
	thickVineAlreadyChoppedMessage = "The vines have already been chopped down."
	// thickVineAlreadyClearedMessage is the already-cleared line
	// (ALREADY_CLEARED_MESSAGE).
	thickVineAlreadyClearedMessage = "The vines have already been cleared."
	// thickVineRakeDisplayName is the rake's cache display name
	// (the item-on-object registration matches by display name;
	// probe, 2026-11-20: id 5341 is "Rake").
	thickVineRakeDisplayName = "Rake"
)

// thickVineAxeDisplay is one axe's display name, keyed by its
// AxeDefs gameval Item: the item-on-object registrations match by
// cache display name, so each axe the tables know needs its
// display name here. Verified against the real rev-242 cache
// (probe, 2026-11-20); several gamevals share a display name
// (the three uncharged infernal axes are all "Infernal axe
// (uncharged)", both trailblazer axes "Infernal axe (or)"), and
// the registration loop dedupes them.
type thickVineAxeDisplay struct {
	Item    string
	Display string
}

var thickVineAxeDisplays = []thickVineAxeDisplay{
	{Item: "bronze_axe_2h", Display: "Bronze felling axe"},
	{Item: "bronze_axe", Display: "Bronze axe"},
	{Item: "iron_axe_2h", Display: "Iron felling axe"},
	{Item: "iron_axe", Display: "Iron axe"},
	{Item: "steel_axe_2h", Display: "Steel felling axe"},
	{Item: "steel_axe", Display: "Steel axe"},
	{Item: "black_axe_2h", Display: "Black felling axe"},
	{Item: "black_axe", Display: "Black axe"},
	{Item: "mithril_axe_2h", Display: "Mithril felling axe"},
	{Item: "mithril_axe", Display: "Mithril axe"},
	{Item: "adamant_axe_2h", Display: "Adamant felling axe"},
	{Item: "adamant_axe", Display: "Adamant axe"},
	{Item: "trail_gilded_axe", Display: "Gilded axe"},
	{Item: "rune_axe_2h", Display: "Rune felling axe"},
	{Item: "rune_axe", Display: "Rune axe"},
	{Item: "3a_axe_2h", Display: "3rd Age felling axe"},
	{Item: "dragon_axe_2h", Display: "Dragon felling axe"},
	{Item: "3a_axe", Display: "3rd Age axe"},
	{Item: "dragon_axe", Display: "Dragon axe"},
	{Item: "infernal_axe_empty", Display: "Infernal axe (uncharged)"},
	{Item: "trailblazer_axe_empty", Display: "Infernal axe (uncharged)"},
	{Item: "trailblazer_reloaded_axe_empty", Display: "Infernal axe (uncharged)"},
	{Item: "crystal_axe_inactive", Display: "Crystal axe (inactive)"},
	{Item: "crystal_axe_2h_inactive", Display: "Crystal felling axe (inactive)"},
	{Item: "crystal_axe_2h", Display: "Crystal felling axe"},
	{Item: "crystal_axe", Display: "Crystal axe"},
	{Item: "infernal_axe", Display: "Infernal axe"},
	{Item: "trailblazer_axe", Display: "Infernal axe (or)"},
	{Item: "trailblazer_reloaded_axe", Display: "Infernal axe (or)"},
	{Item: "league_trailblazer_axe", Display: "Echo axe"},
}

// thickVineAxeDisplayNames returns the distinct axe display names
// in table order, for the item-on-object registrations.
func thickVineAxeDisplayNames() []string {
	seen := make(map[string]bool, len(thickVineAxeDisplays))
	var names []string
	for _, entry := range thickVineAxeDisplays {
		if seen[entry.Display] {
			continue
		}
		seen[entry.Display] = true
		names = append(names, entry.Display)
	}
	return names
}

// vineGroup is a clearing group: "chop" (any axe) or "clear" (rake).
type vineGroup string

const (
	// vineGroupChop is the axe group (Thick vine / Thick vines /
	// Tendrils).
	vineGroupChop vineGroup = "chop"
	// vineGroupClear is the rake group (Vines).
	vineGroupClear vineGroup = "clear"
)

// vineGroupFor classifies a live object by its cache name and its
// own option (groupFor): other locs share these names (the
// Brimhaven Dungeon's "Vines" are chopped down, not cleared), and
// a tool used on them must not clear them. The second result is
// false when the object belongs to neither group.
func vineGroupFor(name string, options []string) (vineGroup, bool) {
	if name == "" {
		return "", false
	}
	has := func(option string) bool {
		for _, o := range options {
			if o == option {
				return true
			}
		}
		return false
	}
	for _, chop := range thickVineChopNames {
		if name == chop && has(thickVineChopAction) {
			return vineGroupChop, true
		}
	}
	for _, clear := range thickVineClearNames {
		if name == clear && has(thickVineClearAction) {
			return vineGroupClear, true
		}
	}
	return "", false
}

// thickVines is the ThickVines unit's per-registration state:
// the pending clearings (keyed by loc, so a double click pays XP
// and schedules regrowth only once) and the cached axe-by-id map
// and rake id, rebuilt when the tables change (SetTables can
// replace them after construction, so the cache is keyed by the
// tables pointer). It lives on the woodcutting instance — no
// package-level mutable maps (the Phase 21 race fix).
type thickVines struct {
	pending map[string]bool
	axeByID map[int]*Axe
	tables  *Tables
	rakeID  int
}

func newThickVines() *thickVines {
	return &thickVines{pending: make(map[string]bool)}
}

// resolve rebuilds the cached axe-by-id map and rake id for the
// current tables (attach: axes = woodcutting.axesBestFirst).
func (v *thickVines) resolve(tables *Tables) {
	if v == nil {
		return
	}
	if v.tables == tables && v.axeByID != nil {
		return
	}
	v.tables = tables
	v.axeByID = nil
	v.rakeID = -1
	if tables == nil {
		return
	}
	v.axeByID = make(map[int]*Axe, len(tables.Axes))
	for i := range tables.Axes {
		v.axeByID[tables.Axes[i].ID] = &tables.Axes[i]
	}
	v.rakeID = tables.RakeID
}

// vineKey keys a clearing by loc (vineKey): "<id>:<x>,<y>,<z>".
func vineKey(object plugins.GameObject) string {
	loc := object.Location()
	return fmt.Sprintf("%d:%d,%d,%d", object.ID(), loc.X, loc.Y, loc.Z)
}

// vineDisplayName is the lower-cased definition name for the
// swing/clear messages (displayName), or "vines" with no
// definition.
func vineDisplayName(def plugins.ObjectDefinition) string {
	if def == nil {
		return "vines"
	}
	if name := def.Name(); name != "" {
		return strings.ToLower(name)
	}
	return "vines"
}

// plantAt returns the live map object, or nil once cleared (or
// never there). It mirrors treeAt's lookup legs (MapObjects.get
// first, then the runtime objects on the tile), matched on id,
// tile, and private area — without treeAt's type/face terms,
// which MapObjects.get does not carry.
func (w *woodcutting) plantAt(object plugins.GameObject) plugins.GameObject {
	if object == nil {
		return nil
	}
	objects := w.objectManager()
	if objects == nil {
		return nil
	}
	area := privateAreaOf(object)
	if area == nil {
		if current := objects.Get(object.ID(), object.Location()); current != nil {
			if privateAreaOf(current) == area {
				return current
			}
			return nil
		}
	}
	for _, obj := range objects.ObjectsAt(object.Location()) {
		if obj != nil && obj.ID() == object.ID() && privateAreaOf(obj) == area {
			return obj
		}
	}
	return nil
}

// playerAxe returns the best axe held anywhere, ignoring wield
// levels (playerAxe / findAnyAxe): any axe cuts. The equipped
// weapon or any inventory axe, best-first; nil when the player
// holds none.
func (w *woodcutting) playerAxe(player *game.Player) *Axe {
	if w.tables == nil || player == nil {
		return nil
	}
	equippedWeaponID := -1
	if equipment := player.Equipment(); equipment != nil {
		equippedWeaponID = equipment.ItemAt(game.WeaponSlot).ID()
	}
	inventory := player.Inventory()
	holds := func(id int) bool {
		return equippedWeaponID == id || (inventory != nil && inventory.Contains(id))
	}
	for i := range w.tables.AxesBestFirst {
		axe := &w.tables.AxesBestFirst[i]
		if holds(axe.ID) {
			return axe
		}
	}
	return nil
}

// resolveRakeID returns the unnoted "rake" id, resolved once per
// tables (resolveRakeId, memoised; -1 when the tables carry none).
func (w *woodcutting) resolveRakeID() int {
	if w.vines == nil {
		return -1
	}
	w.vines.resolve(w.tables)
	return w.vines.rakeID
}

// hasRake reports whether the player holds the rake, inventory or
// equipped (hasRake).
func (w *woodcutting) hasRake(player *game.Player) bool {
	id := w.resolveRakeID()
	if id < 0 || player == nil {
		return false
	}
	if inventory := player.Inventory(); inventory != nil && inventory.Contains(id) {
		return true
	}
	if equipment := player.Equipment(); equipment != nil && equipment.Contains(id) {
		return true
	}
	return false
}

// later schedules a one-shot task after ticks (later: a Task with
// delay max(1, ticks) that stops after one execution). A nil
// TaskManager schedules nothing (the depleteTree pattern).
func (w *woodcutting) later(name string, ticks int, action func()) {
	tm := w.taskManager()
	if tm == nil || action == nil {
		return
	}
	task := game.NewTask(max(1, ticks), func(cycle int64) bool {
		action()
		return true
	})
	task.Name = name
	tm.Schedule(task)
}

// useTool swings the tool at the plant (useTool) in TS order:
// pending keyed by loc (a double click pays once) → face the
// object via the player seam → start message + animation → +2
// ticks: pending cleared, player registered + alive, plant still
// live → snapshot → Deregister → XP → done message → regrow
// after 49 ticks, re-Registering only if the tile is empty.
func (w *woodcutting) useTool(player *game.Player, object plugins.GameObject, skill, xp, animID int, startMessage, doneMessage string) {
	if player == nil || object == nil || w.vines == nil {
		return
	}
	key := vineKey(object)
	if w.vines.pending[key] {
		return
	}
	w.vines.pending[key] = true
	loc := object.Location()
	player.ForcePositionToFace(&loc)
	player.SendMessage(startMessage)
	player.PerformAnimation(animID, 0)
	w.later("ThickVinesClear", thickVineClearDelayTicks, func() {
		delete(w.vines.pending, key)
		if !player.IsRegistered() || skillLevel(player, game.SkillHitpoints) <= 0 {
			return
		}
		live := w.plantAt(object)
		if live == nil {
			return
		}
		location := live.Location()
		snapshot := struct {
			id          int
			x, y, z     int
			typ, face   int
			privateArea *game.PrivateArea
		}{id: live.ID(), x: location.X, y: location.Y, z: location.Z, typ: live.Type(), face: live.Face(), privateArea: privateAreaOf(live)}
		objects := w.objectManager()
		if objects == nil {
			return
		}
		objects.Deregister(live, true)
		if skills := player.Skills(); skills != nil {
			skills.AddExperiences(skill, xp)
		}
		player.SendMessage(doneMessage)
		w.later("ThickVinesRegrow", thickVineRespawnTicks, func() {
			objects := w.objectManager()
			if objects == nil {
				return
			}
			location := game.Location{X: snapshot.x, Y: snapshot.y, Z: snapshot.z}
			if objects.Get(snapshot.id, location) != nil {
				return
			}
			objects.Register(game.NewGameObject(snapshot.id, location, snapshot.typ, snapshot.face, snapshot.privateArea), true)
		})
	})
}

// chopUse swings an axe at the plant (chopUse): Woodcutting XP 2
// x the lumberjack multiplier (truncated to int, the structural
// integer-experience deviation — see chopSuccess).
func (w *woodcutting) chopUse(player *game.Player, object plugins.GameObject, axe *Axe, display string) {
	if axe == nil {
		return
	}
	w.useTool(player, object, game.SkillWoodcutting, int(float64(thickVineClearXP)*w.lumberjackMultiplier(player)), axe.AnimationID,
		fmt.Sprintf("You swing your axe at the %s.", display),
		fmt.Sprintf("You chop the %s down.", display))
}

// clearUse rakes the plant (clearUse): Farming XP 2, rake animation.
func (w *woodcutting) clearUse(player *game.Player, object plugins.GameObject, display string) {
	w.useTool(player, object, game.SkillFarming, thickVineClearXP, thickVineRakeAnimation,
		fmt.Sprintf("You rake the %s.", display),
		fmt.Sprintf("You clear the %s.", display))
}

// chopVegetation handles a Chop click (chopVegetation) in TS order:
// already chopped → message; no axe → message; else chop. It
// always claims the event (handled = true in every TS path). A
// definition whose group is not chop falls through (the
// Brimhaven guard, via vineGroupFor); with no definition the
// registration's name+option match stands.
func (w *woodcutting) chopVegetation(event *plugins.ObjectInteractionEvent) bool {
	if event == nil || w.tables == nil {
		return false
	}
	if event.Definition != nil {
		if group, ok := vineGroupFor(event.Definition.Name(), event.Definition.Interactions()); !ok || group != vineGroupChop {
			return false
		}
	}
	if w.plantAt(event.Object) == nil {
		event.Player.SendMessage(thickVineAlreadyChoppedMessage)
		return true
	}
	axe := w.playerAxe(event.Player)
	if axe == nil {
		event.Player.SendMessage(thickVineNoAxeMessage)
		return true
	}
	w.chopUse(event.Player, event.Object, axe, vineDisplayName(event.Definition))
	return true
}

// clearVegetation handles a Clear click (clearVegetation) in TS
// order: already cleared → message; no rake → message; else
// clear. It always claims the event.
func (w *woodcutting) clearVegetation(event *plugins.ObjectInteractionEvent) bool {
	if event == nil || w.tables == nil {
		return false
	}
	if event.Definition != nil {
		if group, ok := vineGroupFor(event.Definition.Name(), event.Definition.Interactions()); !ok || group != vineGroupClear {
			return false
		}
	}
	if w.plantAt(event.Object) == nil {
		event.Player.SendMessage(thickVineAlreadyClearedMessage)
		return true
	}
	if !w.hasRake(event.Player) {
		event.Player.SendMessage(thickVineNoRakeMessage)
		return true
	}
	w.clearUse(event.Player, event.Object, vineDisplayName(event.Definition))
	return true
}

// toolOnChopVegetation handles an axe used on a chop-group plant
// (toolOnVegetation, chop branch): the named registration already
// matched the object's name, so the group is known; the item id
// must still be an axe (a non-axe item falls through, as TS
// returns unhandled). Already chopped → message; else chop.
func (w *woodcutting) toolOnChopVegetation(event *plugins.ItemOnObjectEvent, objectName string) bool {
	if event == nil || w.tables == nil {
		return false
	}
	if w.vines == nil {
		return false
	}
	w.vines.resolve(w.tables)
	axe := w.vines.axeByID[event.ItemID]
	if axe == nil {
		return false
	}
	if w.plantAt(event.Object) == nil {
		event.Player.SendMessage(thickVineAlreadyChoppedMessage)
		return true
	}
	w.chopUse(event.Player, event.Object, axe, strings.ToLower(objectName))
	return true
}

// toolOnClearVegetation handles the rake used on a clear-group
// plant (toolOnVegetation, clear branch): the named registration
// already matched ("Rake", "Vines"); any other item falls through,
// as TS returns unhandled for a non-rake item. Already cleared →
// message; else clear.
func (w *woodcutting) toolOnClearVegetation(event *plugins.ItemOnObjectEvent, objectName string) bool {
	if event == nil || w.tables == nil {
		return false
	}
	if event.ItemID != w.resolveRakeID() {
		return false
	}
	if w.plantAt(event.Object) == nil {
		event.Player.SendMessage(thickVineAlreadyClearedMessage)
		return true
	}
	w.clearUse(event.Player, event.Object, strings.ToLower(objectName))
	return true
}

// registerThickVines wires the ThickVines registrations (attach):
// onObjectInteraction Chop/Clear per name, and onItemOnObject tool
// use — one named registration per axe display name per chop name
// plus the rake on "Vines". The tool handlers take the object name
// through a forwarding closure (the OnItemFirstAction precedent in
// Register): ItemOnObjectEvent carries no object definition, so
// the name cannot be read off the event.
func (w *woodcutting) registerThickVines(api *plugins.Api) {
	for _, name := range thickVineChopNames {
		api.OnObjectInteractionNamed(name, map[string]plugins.ObjectInteractionHandler{thickVineChopAction: w.chopVegetation})
	}
	for _, name := range thickVineClearNames {
		api.OnObjectInteractionNamed(name, map[string]plugins.ObjectInteractionHandler{thickVineClearAction: w.clearVegetation})
	}
	for _, display := range thickVineAxeDisplayNames() {
		for _, name := range thickVineChopNames {
			objectName := name
			api.OnItemOnObjectNamed(display, objectName, func(event *plugins.ItemOnObjectEvent) bool {
				return w.toolOnChopVegetation(event, objectName)
			}, nil)
		}
	}
	for _, name := range thickVineClearNames {
		objectName := name
		api.OnItemOnObjectNamed(thickVineRakeDisplayName, objectName, func(event *plugins.ItemOnObjectEvent) bool {
			return w.toolOnClearVegetation(event, objectName)
		}, nil)
	}
}
