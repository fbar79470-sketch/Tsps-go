// Package cooking is the Go port of the Cooking skill plugin
// (server/plugins/skills/Cooking.plugin.js, 39 lines, commit
// bfd81e3): cooking on fires, ranges and ovens, doughs, pies and
// pizzas, wine, sacred eels, leaping fish, gnome cooking and
// dairy churns, from plugins/skills/data/cooking.json and rsprox
// captures. The plugin is F2P (no `members` export in the
// registrar); members items and recipes are gated per-item,
// through the members-world flag and the cache isMembers flag.
//
// File split (PHASE36_PLAN.md §3):
//
//   - tables.go (Worker A): the loader + all table types for
//     cookables (122), recipes (47), doughs, wines (2), churn.
//   - common.go (Worker A): the Common.Cooking.js shared
//     helpers + the cooking:raw-xp / cooking:cooked answerers.
//   - recipes.go (Worker A): the Recipes unit.
//   - cooking.go (this file, Worker B owns the wiring):
//     the plugin value, the shared instance, SetTables /
//     SetMembersWorld / SetCookerIDs / SetIsMembersItem /
//     SetObjectInfo, LoadCookerIDsFromCache, and Register.
//     Worker A created the minimal struct + Register skeleton
//     so its units compile; Worker B extended it with the
//     Cookers unit and the cmd/server wiring.
//   - cookers.go (Worker B), wine.go / churn.go / sacred_eel.go
//     / leaping_fish.go / gnome.go (Worker C).
//
// State lives in a *cooking created per Plugin value — NO
// package-level mutable maps (the Phase 21 race fix): the
// shared instance behind Plugin is the only one, and tests
// build their own with newCooking.
package cooking

import (
	"fmt"
	"math/rand/v2"

	"tsps-go/internal/cache"
	"tsps-go/internal/game"
	"tsps-go/internal/plugins"
)

// CookerIDs are the Cookers ids the JSON does not name
// (Cookers.Cooking.js): the poison karambwan, the cooking
// gauntlets, and the two cook animations. The JSON names the
// other ids the unit needs (raw karambwan, cooked/burnt
// meat and chicken), so the loader-resolved ItemIDs map
// carries those. LoadCookerIDsFromCache resolves the two
// anims against the cache gamevals and takes the two items
// from itemIDConstants (ItemIdentifiers.ts transcription —
// the cache has no such gamevals); cmd/server installs them
// before hub Load (the fishing LoadIDsFromCache precedent).
type CookerIDs struct {
	// PoisonKarambwanID is ItemIds.POISON_KARAMBWAN.
	PoisonKarambwanID int
	// CookingGauntletsID is ItemIds.COOKING_GAUNTLETS.
	CookingGauntletsID int
	// FireCookingSeqID is the "human_firecooking" seq.
	FireCookingSeqID int
	// CookingSeqID is the "human_cooking" seq.
	CookingSeqID int
}

// LoadCookerIDsFromCache resolves the Cookers ids against the
// loaded cache definitions. The two anims resolve as gameval
// seqs, as TS resolves them (Cookers.Cooking.js:105-106,
// Common.gameval("seq", ...)). The two items do NOT resolve
// as gamevals — TS reads them through core.ItemIds
// (Cookers.Cooking.js:79 ItemIds.POISON_KARAMBWAN,
// :121 ItemIds.COOKING_GAUNTLETS), and the rev-242 cache has
// no such gamevals (probe, 2026-11-21) — so they come from
// itemIDConstants, transcribed from ItemIdentifiers.ts. A nil
// cache or an unresolvable seq fails — a cache whose gameval
// names do not resolve is a config error, not a degradation.
func LoadCookerIDsFromCache(defs *cache.CacheDefinitions) (CookerIDs, error) {
	if defs == nil {
		return CookerIDs{}, fmt.Errorf("cooking: nil cache definitions")
	}
	seq := func(name string) (int, error) {
		id, ok, err := defs.GetSeqId(name)
		if err != nil {
			return 0, fmt.Errorf("cooking: resolving seq %q: %w", name, err)
		}
		if !ok {
			return 0, fmt.Errorf("cooking: unknown seq gameval %q", name)
		}
		return id, nil
	}
	ids := CookerIDs{
		PoisonKarambwanID:  itemIDConstants["POISON_KARAMBWAN"],
		CookingGauntletsID: itemIDConstants["COOKING_GAUNTLETS"],
	}
	var err error
	if ids.FireCookingSeqID, err = seq("human_firecooking"); err != nil {
		return CookerIDs{}, err
	}
	if ids.CookingSeqID, err = seq("human_cooking"); err != nil {
		return CookerIDs{}, err
	}
	return ids, nil
}

// ObjectInfo is the object-definition surface the Cookers unit
// needs for the item-on-object path (Cookers.Cooking.js
// isCooker / handleCook): the hub's EmitItemOnObject carries
// no Definition, so the info comes from this func, installed
// by cmd/server from the cache (the woodcutting
// SetIsMembersItem precedent). Interactions are the cache
// actions ("" for absent slots); SizeX/SizeY are the
// footprint (1x1 when the definition has none).
type ObjectInfo struct {
	Name         string
	Interactions []string
	SizeX        int
	SizeY        int
}

// cooking is the plugin's per-registration state: the TS
// module-level state (api, the indexed tables) as struct
// fields, per the Phase 21 race fix. One instance serves the
// registered Plugin value; tests build their own with
// newCooking.
type cooking struct {
	tables *Tables
	// api is the Register-time Api, held so the hub surfaces
	// that install after plugin Load (TaskManager) can be
	// resolved lazily at use time through taskManager. Nil in
	// unit tests, which set tasks directly.
	api  *plugins.Api
	emit func(name string, payload any)
	// tasks is the directly-set TaskManager override (unit
	// tests set it); production leaves it nil and resolves
	// through the api instead — a Register-time capture would
	// read nil, because the TaskManager installs after Load.
	tasks *game.TaskManager
	// membersWorld defaults true: the TS no-world fixture path
	// allows everything, and the dev world is a members world.
	// cmd/server installs the real flag through
	// SetMembersWorld before hub Load.
	membersWorld bool
	// objects is the directly-set ObjectManagerAPI override
	// (unit tests set it); production leaves it nil and
	// resolves through the api instead — the same lazy
	// contract as tasks.
	objects plugins.ObjectManagerAPI
	// sessions maps each cooking player to their session
	// (Cookers.Cooking.js activeSessions).
	sessions map[*game.Player]*cookSession
	// taskScheduled guards ensureCookTask: the 1-tick global
	// task schedules once, on the first cook.
	taskScheduled bool
	// cookCycle is the global cooking task's cycle counter (the
	// TS cookingTask.cycle): nextCookTick values read it.
	cookCycle int64
	// cookerIDs are the Cookers ids the JSON does not name
	// (LoadCookerIDsFromCache); zero until SetCookerIDs.
	cookerIDs CookerIDs
	// isMembersItem answers an item definition's members flag;
	// nil means every item counts as non-members. cmd/server
	// installs it before hub Load, backed by the loaded cache
	// definitions.
	isMembersItem func(id int) bool
	// objectInfo resolves an object's display info for the
	// item-on-object path (no event Definition there);
	// cmd/server installs it before hub Load, backed by the
	// loaded cache definitions.
	objectInfo func(id int) (ObjectInfo, bool)
	// resolve answers the cache gameval names the Wine,
	// SacredEel, LeapingFish and Gnome units need outside
	// cooking.json (wine.go newGamevalResolver); nil resolves
	// nothing and those units fail closed. cmd/server installs
	// it before hub Load, backed by the loaded cache
	// definitions.
	resolve gamevalResolver
	// randFloat rolls the burn chance (math/rand/v2 Float64 by
	// default); tests override it for determinism.
	randFloat func() float64
}

// newCooking builds a cooking instance over tables (nil tables
// degrade every handler to a no-op; see Register).
func newCooking(tables *Tables) *cooking {
	return &cooking{
		tables:       tables,
		membersWorld: true,
		sessions:     make(map[*game.Player]*cookSession),
		randFloat:    rand.Float64,
	}
}

// shared is the instance behind Plugin; SetTables and
// SetMembersWorld operate on it.
var shared = newCooking(nil)

// SetTables installs the resolved tables for the registered
// Plugin value. There is no CacheDefinitions accessor on Api,
// so the coordinator's integration step must call this with
// LoadFromCache's result BEFORE hub Load: Register captures
// the shared instance, so a call before Load reaches every
// handler. With no tables installed, every handler degrades to
// a no-op. (The woodcutting/fletching SetTables precedent.)
func SetTables(t *Tables) {
	shared.tables = t
}

// SetMembersWorld installs the members-world flag for the
// registered Plugin value (TS: pluginApi.core.WorldDefinition.
// isMembersWorld()). cmd/server calls it before hub Load with
// the world definition's flag; it defaults true (the TS
// no-world fixture path allows everything, and the dev world
// is a members world).
func SetMembersWorld(members bool) {
	shared.membersWorld = members
}

// SetCookerIDs installs the Cookers ids for the registered
// Plugin value (LoadCookerIDsFromCache's result). cmd/server
// calls it before hub Load with the resolved ids; zero ids
// degrade the gauntlets check, the poison-karambwan result and
// the cook animations to never-match / 0, the same
// no-definitions degradation as the rest of the server.
func SetCookerIDs(ids CookerIDs) {
	shared.cookerIDs = ids
}

// SetIsMembersItem installs the item-members lookup for the
// registered Plugin value (TS: CacheDefinitions.getItem(id)?.
// isMembers). cmd/server calls it before hub Load with a
// closure over the loaded cache definitions; nil means every
// item counts as non-members (the woodcutting precedent).
func SetIsMembersItem(fn func(id int) bool) {
	shared.isMembersItem = fn
}

// SetObjectInfo installs the object-info lookup for the
// registered Plugin value (TS: the object definition's name,
// interactions and footprint). cmd/server calls it before hub
// Load with a closure over the loaded cache definitions; nil
// means the item-on-object path cannot name its object and
// isCooker reads false there (the object-click path still
// reads the event Definition).
func SetObjectInfo(fn func(id int) (ObjectInfo, bool)) {
	shared.objectInfo = fn
}

// SetGamevalResolver installs the cache gameval resolver for
// the registered Plugin value (wine.go newGamevalResolver over
// the loaded cache definitions). cmd/server calls it before
// hub Load; nil defs resolve nothing and the Wine, SacredEel,
// LeapingFish and Gnome units fail closed.
func SetGamevalResolver(defs *cache.CacheDefinitions) {
	shared.resolve = newGamevalResolver(defs)
}

// emitEvent emits a custom event through the captured emit
// func; a nil emit (before Register) drops the emit, which is
// the no-hooks default for every query.
func (c *cooking) emitEvent(name string, payload any) {
	if c == nil || c.emit == nil {
		return
	}
	c.emit(name, payload)
}

// taskManager resolves the world's TaskManager lazily: the hub
// installs it after plugin Load (nil until installed, per the
// hub contract — cmd/server installs it from the built world),
// so a Register-time capture reads nil in production. The live
// Api value wins when present; the directly-set tasks field is
// the unit-test override. (The woodcutting/fletching
// taskManager pattern, Phase 32 hub-contract lesson: lazy
// resolution, Schedule only on the tick goroutine.)
func (c *cooking) taskManager() *game.TaskManager {
	if c == nil {
		return nil
	}
	if c.api != nil {
		if tm := c.api.GetTaskManager(); tm != nil {
			return tm
		}
	}
	return c.tasks
}

// register wires the units in TS attach order
// (Cooking.plugin.js register): Cookers first, then Recipes,
// then Wine, SacredEel, LeapingFish, Gnome, Churn. The two
// query answerers are registered here, once (common.go
// registerAnswerers, Worker A), and must NOT be registered
// again elsewhere.
//
// The Wine, SacredEel, LeapingFish and Gnome units take the
// gameval resolver as a second argument (their register*
// signatures); a nil resolver fails those units closed.
func (c *cooking) register(api *plugins.Api) {
	if api == nil {
		return
	}
	c.api = api
	c.emit = api.EmitCustomEvent
	c.registerCookers(api)
	c.registerRecipes(api)
	c.registerWine(api, c.resolve)
	c.registerSacredEel(api, c.resolve)
	c.registerLeapingFish(api, c.resolve)
	c.registerGnome(api, c.resolve)
	c.registerChurn(api)
	c.registerAnswerers(api)
}

// plugin builds the registered Plugin value: name and
// description verbatim (module.exports, Cooking.plugin.js),
// attach-only register. Cooking is F2P: the registrar exports
// no `members` flag, so Members stays false and the hub loads
// the plugin on free worlds too; members items and recipes are
// gated per-item through membersWorld + the cache isMembers
// flag (Recipes.isMembersRecipe, Cookers.canCook).
func (c *cooking) plugin() plugins.Plugin {
	return plugins.Plugin{
		Name:        "Cooking",
		Description: "Cooking on fires, ranges and ovens, plus doughs, pies, pizzas, wine, sacred eels, leaping fish, gnome cooking and dairy churns.",
		Members:     false,
		Register: func(api *plugins.Api) {
			c.register(api)
		},
	}
}

// Plugin is the registered Cooking plugin value.
var Plugin = shared.plugin()

func init() {
	plugins.Register(Plugin)
}
