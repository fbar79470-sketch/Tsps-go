// Smithing: the plugin, the entry router and the shared
// instance (Smithing.plugin.js). The units hang off the same
// *smithing receiver.
package smithing

import (
	"math/rand"

	"tsps-go/internal/game"
	"tsps-go/internal/plugins"
)

// furnaceNames is FURNACE_NAMES (Smithing.plugin.js:21).
var furnaceNames = map[string]bool{
	"Furnace": true, "Small furnace": true, "Lovakite furnace": true,
}

// anvilNames is Anvil.ANVIL_NAMES.
var anvilNames = []string{"Anvil", "An experimental anvil", "Rusted anvil"}

func isAnvilName(name string) bool {
	for _, n := range anvilNames {
		if n == name {
			return true
		}
	}
	return false
}

// smithing is the shared plugin instance behind Plugin.
type smithing struct {
	api           *plugins.Api
	emit          func(name string, payload any)
	membersWorld  bool
	tasks         *game.TaskManager
	taskScheduled bool
	tick          int64
	sessions      map[*game.Player]*smithSession
	everyTick     []func()
	menuExtras    []func(*game.Player) []smeltMenuEntry
	randFloat     func() float64

	// Anvil state (Anvil.Smithing.js OPEN_MENUS /
	// OPEN_STATIONS / SELECTED_QUANTITY).
	openMenus        map[*game.Player]int
	openStations     map[*game.Player]*stationSnapshot
	selectedQuantity map[*game.Player]int

	// objectNameResolver resolves an object's display name by
	// id (the cooking SetObjectInfo precedent). Wiring
	// installs it from the cache definitions; without it the
	// item-on-object router cannot match furnace/anvil names
	// and degrades to a no-op (gap 26f). Object interactions
	// do not need it: the hub resolves their names itself.
	objectNameResolver func(id int) string
}

func newSmithing() *smithing {
	return &smithing{
		sessions:         make(map[*game.Player]*smithSession),
		openMenus:        make(map[*game.Player]int),
		openStations:     make(map[*game.Player]*stationSnapshot),
		selectedQuantity: make(map[*game.Player]int),
		membersWorld:     true,
		randFloat:        func() float64 { return rand.Float64() },
	}
}

// shared is the instance behind Plugin; SetTables (tables.go)
// installs the resolved tables on the package state this
// instance reads (the tables worker's frozen API: the package
// var `tables`, not a per-instance field).
var shared = newSmithing()

func init() {
	plugins.Register(Plugin)
}

// SetMembersWorld installs the members-world flag (TS:
// WorldDefinition.isMembersWorld()). cmd/server calls it
// before hub Load with the world definition's flag; it
// defaults true.
func SetMembersWorld(members bool) { shared.membersWorld = members }

// SetObjectNameResolver installs the object-id → display-name
// lookup for the item-on-object router (gap 26f). Wiring
// installs it from the cache definitions; nil clears it.
func SetObjectNameResolver(fn func(id int) string) { shared.objectNameResolver = fn }

// objectNameOf resolves an object's display name, or "" when
// unknown (no resolver installed).
func (s *smithing) objectNameOf(id int) string {
	if s == nil || s.objectNameResolver == nil {
		return ""
	}
	return s.objectNameResolver(id)
}

// ---- plugin ----

var Plugin = shared.plugin()

func (s *smithing) plugin() plugins.Plugin {
	return plugins.Plugin{
		Name:        "Smithing",
		Description: "Smelting at furnaces and smithing at anvils, plus cannonballs, godswords, dragon and visage shields, barbarian spears, Varrock armour and Superheat Item.",
		Members:     false,
		Register: func(api *plugins.Api) {
			s.api = api
			s.emit = api.EmitCustomEvent
			s.register(api)
		},
	}
}

// register wires the hooks (Smithing.plugin.js register). It
// no-ops when the tables are not installed, the mining
// precedent: with no tables every handler degrades to a no-op.
func (s *smithing) register(api *plugins.Api) {
	if tables == nil {
		return
	}
	api.OnServerStartup(func(*plugins.ServerLifecycleEvent) { s.ensureTask() })
	api.OnPlayerDisconnect(s.disconnect)
	api.OnCustomEvent("level-up:popup-opened", s.pauseSessions)
	api.OnCustomEvent("smithing:bar-for-ore", s.answerBarForOre)
	api.OnCustomEvent("smithing:smelt-outcome", s.smeltOutcome)
	api.OnItemOnObject(s.useItemOnObject, &plugins.ItemUseFilter{Noted: plugins.BoolPtr(false)})
	s.registerSmelting(api)
	s.registerAnvil(api)
	s.registerCannonballs(api)
	s.registerSpears(api)
	s.registerGear(api)
	s.registerGodswords(api)
	s.registerDragonShields(api)
	s.registerSuperheat(api)
}

// disconnect stops the session and forgets the anvil state
// (Smithing.plugin.js disconnect).
func (s *smithing) disconnect(event *plugins.PlayerDisconnectEvent) {
	if event == nil {
		return
	}
	s.stopSession(event.Player, false)
	s.forgetAnvil(event.Player)
}

// useItemOnObject routes an item used on an object by object
// name (Smithing.plugin.js useItemOnObject): furnaces →
// Cannonballs then Smelting; "Rusted anvil" → Anvil; anvil
// names → Godswords → DragonShields → Anvil; the shield
// machine loc id → DragonShields (wyvern shield).
func (s *smithing) useItemOnObject(event *plugins.ItemOnObjectEvent) {
	if event == nil || event.Player == nil || event.Object == nil || tables == nil {
		return
	}
	name := s.objectNameOf(event.ObjectID)
	switch {
	case furnaceNames[name]:
		if s.cannonballsUseOnFurnace(event) {
			return
		}
		s.smeltingUseOnFurnace(event)
	case name == "Rusted anvil":
		s.anvilUseOnAnvil(event)
	case isAnvilName(name):
		if TryGodswordOnAnvil(event) || TryDragonShieldOnAnvil(event) {
			return
		}
		s.anvilUseOnAnvil(event)
	case event.ObjectID == ShieldMachineLocID:
		TryWyvernShield(event)
	}
}

// ---- exports for reuse (Smithing.plugin.js module.exports)
// ----

// IsSmeltingActive reports whether the player has a smelt
// session (isSmeltingActive).
func IsSmeltingActive(player *game.Player) bool {
	return shared.sessionTypeOf(player) == "smelt"
}

// QuantityForSmeltAction is the quantity a Smelt-menu action
// makes (Smelting.Smithing.js quantityForSmeltAction): the
// action's amount, capped at 28; a bare click (0) makes one.
func QuantityForSmeltAction(action int) int {
	if action > 0 {
		if action > smeltMaxQuantity {
			return smeltMaxQuantity
		}
		return action
	}
	return 1
}

// StartBotSmelting starts a bot smelting a recipe
// (startBotSmelting).
func StartBotSmelting(player *game.Player, recipe *SmeltRecipe, amount int) bool {
	return shared.startSmelting(player, recipe, amount, -1)
}

// SmeltRecipes returns the installed smelting recipes, or nil
// (SMELTING_RECIPES, for the Blast Furnace's loadData reuse).
func SmeltRecipes() []SmeltRecipe {
	if tables == nil {
		return nil
	}
	return append([]SmeltRecipe(nil), tables.Smelting...)
}
