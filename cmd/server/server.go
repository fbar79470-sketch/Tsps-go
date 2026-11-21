// Command server is the Go port of the TSPS game server (server only).
//
// Boot order mirrors Server.main in
// src/main/typescript/elvarg/Server.ts: cache, then game systems, then
// the network listener. Definitions load from the cache at boot
// (unless --no-cache), JSON definition data loads from
// data/definitions/, spawns enter the world, and the plugin layer
// and content are NOT STARTED (see PORT_STATUS.md).
//
// Layout: the process runs with its working directory set to the data
// root (the --root flag, default "."), which must contain target.txt
// and data/definitions/world.json, exactly like the TypeScript server
// directory. The cache lives under <root>/caches/<target>/ and is
// downloaded from the OpenRS2 archive on first start unless --no-cache
// is given. Cache data is never bundled with this repository.
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"tsps-go/internal/cache"
	"tsps-go/internal/config"
	"tsps-go/internal/definition"
	"tsps-go/internal/game"
	"tsps-go/internal/listeners"
	"tsps-go/internal/music"
	"tsps-go/internal/persist"
	"tsps-go/internal/plugins"
	"tsps-go/internal/protocol"
	"tsps-go/internal/session"
)

// Options configures one server build. Tests construct Servers with
// in-memory saves and short tick rates through the same constructor
// production uses.
type Options struct {
	// NoCache skips the OpenRS2 download: the revision is read from
	// target.txt but no cache directory is required. The server can
	// then boot and answer logins without cache data; the definition
	// facades and XTEA keys stay unloaded (nil), and
	// definition-backed behavior falls back: the login bootstrap
	// sends run-energy weight 0 and XTEA lookups answer the zero key.
	NoCache bool
	// TickRateMs overrides the 600 ms tick rate. Zero means the
	// configured default (config.TickRateMs).
	TickRateMs int
	// SaveBackend selects the character-save store: "json" (one file
	// per character under the legacy save directory, the TS JSON
	// backend), "sqlite" (the TS SQLite backend: player_saves table
	// at config.Env.PlayerSaveDatabasePath), or "memory" (in-memory,
	// for tests). Anything else is an error.
	SaveBackend string
	// AddTimeout bounds how long world entry waits for the tick loop
	// to assign a player index. Zero means the default (3 seconds,
	// five ticks at the standard rate).
	AddTimeout time.Duration
	// Log is the logger used for boot and runtime messages. Nil
	// means log.Default().
	Log *log.Logger
	// Port is the --port flag value (0 = WEBSOCKET_PORT env, then
	// 43594, via Server.Port). It is the port the gateway
	// jav_config's worldlist URL names, so it is resolved in
	// buildServer before the gateway target is constructed.
	Port int
}

// Server wires the ported packages into a runnable game server: world
// definition, cache target, accounts, saves, the tick engine, and the
// WebSocket handler.
type Server struct {
	env        config.Env
	worldDef   *config.World
	active     *cache.ActiveCache // nil in NoCache mode
	loaded     *loadedDefinitions // nil in NoCache mode; owns the open store
	revision   int
	world      *game.World
	engine     *game.Engine
	accounts   *persist.Accounts
	saves      persist.SaveStore
	adapter    *worldAdapter
	loginDeps  *session.LoginDeps
	handler    http.Handler
	tickRateMs int
	logger     *log.Logger
	// plugins is the loaded plugin hub (see pluginwiring.go). Nil
	// for a Server built without buildServer (tests only): Start
	// and Stop guard the emits.
	plugins *plugins.Hub
	// started records whether Start has ever run: Stop snapshots
	// online players directly only when the engine was never started
	// (no tick goroutine can exist); otherwise the snapshot is taken
	// on the tick goroutine via the save coordinator.
	started atomic.Bool
}

// buildServer loads configuration, ensures the cache target (unless
// NoCache), opens accounts/saves, and constructs the world, engine,
// and HTTP/WebSocket handler. The caller must be running with its
// working directory set to the data root (see the package comment). It
// does not start the engine or listen; call Start, then serve
// Handler().
func buildServer(opts Options) (*Server, error) {
	logger := opts.Log
	if logger == nil {
		logger = log.Default()
	}
	tickRateMs := opts.TickRateMs
	if tickRateMs <= 0 {
		tickRateMs = config.TickRateMs
	}
	addTimeout := opts.AddTimeout
	if addTimeout <= 0 {
		addTimeout = 3 * time.Second
	}

	env := config.LoadEnv()

	worldDef, err := config.LoadWorld("data")
	if err != nil {
		return nil, fmt.Errorf("loading world definition: %w", err)
	}

	// Boot-time definition loading runs before the plugin hub:
	// content tables that resolve gameval names against the
	// cache (Woodcutting's, see installWoodcuttingTables) must
	// be installed before hub Load, and hub Load runs inside
	// newPluginHub below. The hub itself reads no definitions
	// at load time (plugin Register functions are attach-only),
	// so loading the cache first changes nothing else. In
	// NoCache mode the facades stay nil and the server boots
	// and answers logins without them.
	var active *cache.ActiveCache
	var revision int
	if opts.NoCache {
		raw, err := os.ReadFile("target.txt")
		if err != nil {
			return nil, fmt.Errorf("reading target.txt: %w", err)
		}
		target, err := cache.ParseTarget(string(raw))
		if err != nil {
			return nil, fmt.Errorf("target.txt: %w", err)
		}
		revision = target.Revision
		logger.Printf("[cache] no-cache mode: revision %d from target.txt (no cache data loaded; definitions unavailable)", revision)
	} else {
		active, err = cache.EnsureCache(".")
		if err != nil {
			return nil, fmt.Errorf("ensuring cache: %w", err)
		}
		revision = active.Revision
		logger.Printf("[cache] active %s (revision %d)", active.Name, active.Revision)
	}

	// With a cache present, open the store and construct the
	// facades before anything that reads definitions runs. A
	// failure here fails the boot (see loadDefinitions).
	var loaded *loadedDefinitions
	if active != nil {
		loaded, err = loadDefinitions(active)
		if err != nil {
			return nil, fmt.Errorf("loading definitions: %w", err)
		}
		counts := loaded.defs.GetCounts()
		logger.Printf("[cache] definitions loaded: %d npcs, %d items, %d objects, %d seqs",
			counts.NPCs, counts.Items, counts.Objects, counts.Seqs)
	}
	var defs *cache.CacheDefinitions
	if loaded != nil {
		defs = loaded.defs
	}

	// Plugin hub (see pluginwiring.go): loaded before the world is
	// built, as TS loads plugins before GameBuilder constructs the
	// game. The world-dependent seams (cycle source, process emit)
	// are installed after the world exists, below.
	hub, err := newPluginHub(worldDef, logger, defs)
	if err != nil {
		return nil, fmt.Errorf("loading plugins: %w", err)
	}
	// The plugin stock veto (see shopwiring.go) must bind into
	// the definition loader before loadGameDefinitions runs
	// below: the veto fires per stock entry during Load, and a
	// later install would never see the loaded shops.
	installCanStockItemHook(hub)

	savesDir := filepath.Dir(env.AccountsFile)
	accounts, err := persist.LoadAccounts(env.AccountsFile)
	if err != nil {
		return nil, fmt.Errorf("loading accounts: %w", err)
	}
	displayNamesPath := os.Getenv("TSPS_DISPLAY_NAMES_FILE")
	if displayNamesPath == "" {
		displayNamesPath = filepath.Join(savesDir, "display-names.json")
	}
	displayNames, err := persist.LoadDisplayNames(displayNamesPath)
	if err != nil {
		return nil, fmt.Errorf("loading display names: %w", err)
	}
	punishment, err := persist.LoadPunishment(savesDir)
	if err != nil {
		return nil, fmt.Errorf("loading punishments: %w", err)
	}

	var saves persist.SaveStore
	switch opts.SaveBackend {
	case "", "json":
		saves = persist.NewJSONFileStore(env.LegacySaveDir)
	case "sqlite":
		// The SQLite backend's path comes from the environment
		// (PLAYER_SAVE_DATABASE_PATH, default
		// data/saves/players.sqlite) via config.Env — the same
		// resolvedPath precedent as LegacySaveDir. A failure to
		// open the database fails the boot: a present-but-unusable
		// backend must not silently serve a different store.
		sqliteStore, err := persist.NewSQLiteStore(env.PlayerSaveDatabasePath)
		if err != nil {
			return nil, fmt.Errorf("opening SQLite save database: %w", err)
		}
		saves = sqliteStore
	case "memory":
		saves = persist.NewMemoryStore()
	default:
		return nil, fmt.Errorf("unknown save backend %q (want json, memory, or sqlite)", opts.SaveBackend)
	}

	spawn := game.Location{X: worldDef.Spawn.X, Y: worldDef.Spawn.Y, Z: worldDef.Spawn.Z}
	world := game.NewWorld(spawn)
	installPluginWorldSeams(hub, world)
	// Run-energy equipment effects (see equipmentwiring.go):
	// install the world's modifier registry on the hub,
	// flushing the modifiers queued during plugin loading into
	// it before the engine starts.
	installEquipmentEffectsWiring(hub, world)
	// Collision wiring: with definitions loaded, replace the world's
	// empty open-ground RegionManager with a cache-backed one over
	// the CacheMaps/CacheDefinitions facades (the adapter satisfies
	// both RegionDataSource and ObjectDefSource). The new-terrain-
	// format flag follows the revision: terrain decoding switches
	// format at revision 209 (PHASE4_PLAN §1). In NoCache mode the
	// default empty manager stays, and every tile reads open —
	// matching TS getClipping for a missing region.
	// Map-region replacements (see instancingwiring.go): the
	// manager is created even in NoCache mode (it answers
	// empty), and the region-data source is wrapped so
	// replacement map data answers region reads
	// (RegionManager.ts:46-52), while the object-definition
	// source stays unwrapped. The adapter keeps the manager
	// for per-player streaming (set below, once the adapter
	// exists); the hoisted sources variable feeds the
	// instancing install.
	replacements := game.NewMapRegionReplacementManager()
	var sources *cacheRegionSources
	if loaded != nil {
		sources = newCacheRegionSources(loaded.maps, loaded.defs)
		wrapped := &game.ReplacementRegionSource{Base: sources, Replacements: replacements}
		world.SetRegionManager(game.NewRegionManager(wrapped, sources, revision >= 209))
	}
	// Area + Wilderness wiring (see areaswiring.go): constructed
	// after the RegionManager is installed (the Wilderness roof
	// check reads through it) and before the engine starts. The
	// manager is installed on the World (players/NPCs read it on
	// add); the Wilderness is kept by the adapter for the
	// drop-listener seam. Content areas register through the
	// hub's registerArea, which pushes onto this manager.
	areaManager, wilderness := newAreaManagerAndWilderness(worldDef, world.RegionManager())
	world.SetAreaManager(areaManager)
	installFishingWilderness(wilderness)
	// Definition loading + custom overlays + spawns (see
	// definitionwiring.go): after the RegionManager is installed
	// (object spawns clip through it) and before the engine
	// starts. In NoCache mode the facades are nil and loading
	// degrades to the no-cache behavior.
	var gamevals *cache.Gamevals
	if loaded != nil {
		gamevals = loaded.gamevals
	}
	store, err := loadGameDefinitions(filepath.Join("data", "definitions"), world, defs, gamevals, logger.Printf)
	if err != nil {
		return nil, fmt.Errorf("loading game definitions: %w", err)
	}
	// Shop manager (see shopwiring.go): built after the
	// definition load (the runtime shops initialize from the
	// loaded definitions) and before the dialogue playback
	// (its shop steps bind the live opener) and the engine
	// start. The restock pass runs behind Hooks.Shops and the
	// per-player auto-close check behind
	// Hooks.ShopProcessPlayer; the trade seam installs after
	// the ground-item manager and save coordinator exist,
	// below.
	shops := newShopManager(store, defs)
	installShopHooks(shops, hub)
	installShopHubSurfaces(hub, shops, store)
	installShopkeepers(hub, store, defs, logger)
	world.Hooks.Shops = func(cycle int64) { shops.Tick() }
	world.Hooks.ShopProcessPlayer = func(p *game.Player) { shops.ProcessPlayer(p) }
	// Dialogue data + transcript playback (see
	// dialoguewiring.go): loaded after the definition Store
	// exists (the speaker index's cache-name pass reads NPC
	// names from it) and before the engine starts. A load
	// failure fails the boot, as the TS loader's throw fails
	// the NpcDialogues registration. The MultiChatboxPrompt
	// registry is created first: the playback presents its
	// choice menus through it, the resume dispatch checks it
	// before the dialogue manager, and content reaches it
	// through Api.SendMultiChatboxPrompt.
	prompts := game.NewMultiChatboxPrompt(logger.Printf)
	hub.SetMultiChatboxPrompt(prompts)
	dialoguePlayback, err := loadDialoguePlayback(filepath.Join("data", "definitions"), store, world, hub, prompts, logger.Printf, shopOpener{manager: shops, store: store})
	if err != nil {
		return nil, err
	}
	SubscribeDialogueStart(hub, dialoguePlayback)
	// Ground-item manager + spawns (see grounditemswiring.go):
	// constructed after the definition load so the
	// definition-backed sources see the loaded Store, spawns
	// applied before the engine starts, per-tick pass behind the
	// GroundItems hook.
	groundItems := newGroundItemManager(world, store)
	applyGroundItemSpawns(groundItems, store)
	world.Hooks.GroundItems = func(cycle int64) { groundItems.Process(cycle) }
	// Music data (see musicwiring.go): areas from the definitions
	// directory, lengths from DbTable 44 when the cache facade
	// exists. Installed on entering sessions by the adapter.
	musicData := loadMusicData(filepath.Join("data", "definitions"), defs, logger.Printf)
	InstallCombatStyles(filepath.Join("data", "definitions"), logger.Printf)
	adapter := newWorldAdapter(world, spawn, addTimeout)
	adapter.groundItems = groundItems
	adapter.musicData = musicData
	adapter.wilderness = wilderness
	adapter.itemDefinitions = newItemDefinitionSource(store, defs)
	adapter.plugins = hub
	adapter.varbitDefs = defs
	adapter.store = store
	adapter.shops = shops
	if loaded != nil {
		adapter.keys = loaded.keys
	}
	// Save wiring: the coordinator owns the carried-save registry,
	// the autosave schedule, and the shutdown snapshot handshake (see
	// savewiring.go). The adapter applies saves at entry and saves on
	// disconnect through it; the dev-username list grants DEVELOPER
	// rights on new-character saves (NetworkBuilder.ts:1670).
	intervalTicks, minIntervalTicks := autosaveIntervalsFromEnv()
	coordinator := newSaveCoordinator(saves, world, logger, tickRateMs, intervalTicks, minIntervalTicks)
	adapter.saves = coordinator
	adapter.devUsernames = env.DevUsernames
	// Trade seams (see shopwiring.go): the hub trade families,
	// the ground-item fallback for aborted offers, and the
	// post-trade save. Installed after the ground-item manager
	// and save coordinator exist; the seam is inert until this
	// runs, and ticking starts only after buildServer returns.
	installTradeHooks(hub, coordinator, groundItems, logger)
	// Death wiring (see combatwiring.go): the death task's
	// world-backed seams — the "death"/"respawn" save
	// requests, the ground-item spawn for bones and dropped
	// items, and the respawn teleport target from the world's
	// live spawn. Installed after the ground-item manager and
	// save coordinator exist, like the trade seams above.
	InstallDeathWiring(coordinator, groundItems, world, logger)
	// Chat + command wiring (see commandwiring.go and
	// chatwiring.go): the command registry with its messenger
	// and rights seams, the punishment lists for the chat mute
	// gate, and the examine source + object-examine intercept.
	// The rights gate reads the carried save's Rights.ID: the
	// carried save is never mutated (see snapshotSave), and
	// carriedSave is mutex-guarded, so the read is safe from
	// the tick goroutine. A player with no carried save reads
	// as RightsNone (fail-closed: unrestricted commands run,
	// restricted ones deny). The same source feeds the object
	// registry's developer-notice gate (see
	// installObjectRegistrySources).
	rightsID := func(p *game.Player) int {
		if save, ok := coordinator.carriedSave(p); ok && save != nil {
			return save.Rights.ID
		}
		return game.RightsNone
	}
	adapter.commands = wireCommandRegistry(hub, func(p *game.Player, msg string) { p.SendMessage(msg) }, rightsID)
	adapter.punishment = punishment
	adapter.examine = storeExamineSource{store: store}
	adapter.objectExamineIntercept = objectExamineIntercept(hub)
	// Object wiring (see objectwiring.go,
	// objectlistenerwiring.go, objectregistrywiring.go): the
	// hub's object resolvers + content registry surface, the
	// per-player definition source, the world-shared sync
	// queries + delta feed, and the registry's variant/rights
	// seams (same carried-save rights source as the command
	// registry above).
	InstallObjectResolvers(hub, store)
	// Item-action wiring (see itemactionlistenerwiring.go):
	// the hub's item-actions resolver over the Store/cache
	// facade, so named onItemAction matching can read the
	// inventory actions. Installed next to the object
	// resolvers above.
	InstallItemActionResolvers(hub, store, defs)
	hub.SetObjectManager(NewObjectManager(world.GameObjects()))
	// Area wiring (see areaswiring.go): the hub's registerArea
	// surface over the world's AreaManager (flushing areas
	// queued during plugin loading, in registration order),
	// and the isolating AreaMethodCaller (a throwing area
	// method is logged, never fatal to the tick). Both install
	// before the engine starts.
	hub.SetAreaRegistry(NewAreaRegistry(world.Areas()))
	game.SetAreaMethodCaller(hub.AreaMethodCaller())
	// Combat wiring (see combatwiring.go): the game-side
	// combat event seam populated from the hub, and the NPC
	// combat-method provider registry installed on the hub
	// (flushing providers queued during plugin loading).
	// Installs before the engine starts, like the area
	// wiring above.
	InstallCombatHooks(hub, world)
	// Ranged data (see combatwiring.go): the production ranged
	// weapon/ammunition resolver and ammunition data, plus the
	// charge-bow decrement skip. Installs before the engine
	// starts, like the combat wiring above; with no resolver
	// every fighter would attack in melee.
	InstallRangedData()
	// Standard spellbook (internal/game/combat_spells.go):
	// registers the standard combat spells and the utility
	// spells by id and name, so GetCombatSpell resolves the
	// autocast slots and the listeners resolve cast spells.
	// Installs before the engine starts, like the ranged
	// data above; with no spells registered every autocast
	// slot would read nil.
	game.InstallStandardSpells()
	// Combat sources (see combatwiring.go): the world-backed
	// cycle/clipping/multi/Wilderness sources, the combat_hits
	// phase, NPC respawn + special-restore wiring, and the
	// aggression bridge. After the AreaManager/RegionManager
	// installs above, before the engine starts.
	InstallCombatSources(world, wilderness)
	// Sequence animations (see combatwiring.go): the
	// cache-backed sequence scanner behind the stall,
	// forced-priority and standing-priority seams, plus the
	// delayed-animation task submitter. After the combat
	// sources above (the cycle source must exist before any
	// animation can be measured), before the engine starts;
	// with no scanner installed every sequence reads the
	// no-cache defaults and no swing is ever stalled.
	InstallSequenceAnimations(defs, world)
	// Skills wiring (see skillswiring.go): the game-side skills
	// event seam populated from the hub, the engine TaskManager
	// handle on the hub, and the world experience rate from the
	// world definition. Before the engine starts, like the
	// combat wiring above; with no skills hooks registered the
	// seam is behavior-identical to unwired.
	InstallSkillHooks(hub, world)
	installSkillExperienceRate(worldDef.ExperienceMultiplier)
	// Sound wiring (see soundwiring.go): the world player-list
	// source the Sounds/projectile broadcast paths read.
	InstallSoundWiring(world)
	// Sailing wiring (see sailingwiring.go): the boat manager
	// + Sailing runtime, the Boats tick hook, and the
	// process-wide publication. The runtime is kept on the
	// adapter for the login/logout call sites.
	adapter.sailing = InstallSailing(hub, world)
	adapter.replacements = replacements
	// Instancing wiring (see instancingwiring.go): the
	// template-region seams, the replacement manager's
	// native/archive seams + data/regions scan, the
	// private-area destroy hook, and the hub's area-manager +
	// replacement wiring. The template seam deliberately
	// receives the UNWRAPPED cache source (template decode
	// ignores replacements, as TS reads CacheMaps directly).
	var regions game.RegionDataSource
	var objDefs game.ObjectDefSource
	var archiveIDs func(int) (int, int, bool)
	if sources != nil {
		regions, objDefs = sources, sources
	}
	if loaded != nil && loaded.maps != nil {
		archiveIDs = loaded.maps.GetArchiveIDs
	}
	InstallInstancing(hub, world, replacements, regions, objDefs, revision >= 209, archiveIDs, filepath.Join("data", "regions"), func(msg string) { logger.Printf("%s", msg) })
	adapter.objectDefs = NewObjectDefinitionSource(store)
	adapter.objectQueries = NewRegistryQueries(world.GameObjects())
	adapter.objectFeed = NewObjectDeltaFeed(adapter.objectQueries)
	installObjectRegistrySources(world, store, rightsID)
	// Dialogue wiring (see dialoguewiring.go): the playback
	// engine, the per-player NPC-definition source, the
	// NPC-name resolver for chatheads, and the shared
	// MultiChatboxPrompt registry for choice menus.
	adapter.dialoguePlayback = dialoguePlayback
	adapter.prompts = prompts
	adapter.npcDefs = NewNpcDefinitionSource(store)
	adapter.npcNameResolver = npcNameResolver(store)
	// OnAdded is assigned before the engine starts (single-threaded
	// setup) and never reassigned; it notifies logins waiting for
	// their player index. See worldAdapter.onPlayerAdded.
	world.Players().OnAdded = adapter.onPlayerAdded
	world.Hooks.OnPlayerDisconnect = adapter.onPlayerDisconnect
	world.Hooks.OnLogoutAccepted = adapter.onLogoutAccepted
	// Relations seams (see socialwiring.go): the world iteration
	// + lookup seam and the shown-name seam, installed once at
	// boot before the engine starts. The display-names registry
	// is the one loaded above; a standalone adapter (tests)
	// leaves the defaults (self-only world, identity names).
	installRelationsSeams(world, displayNames)
	// Autosave (Autosave.plugin.js): a TaskManager task that runs
	// every tick, registered before the engine starts. Scheduling
	// here is safe because no tick is running yet (single-threaded
	// setup, as with the hook assignments above); TaskManager is
	// otherwise tick-goroutine-only.
	world.Tasks().Schedule(coordinator.autosaveTask())

	loginDeps := &session.LoginDeps{
		ActiveRevision: revision,
		Accounts:       accounts,
		Punishment:     punishment,
		DisplayNames:   displayNames,
		Profile:        persist.ProfileMain,
		Saves:          saveAdapter{store: saves},
		IsOnline:       adapter.isOnline,
		ReserveName:    adapter.reserveName,
		ReleaseName:    adapter.releaseName,
		WorldFull:      world.IsFull,
		// The bootstrap's world-level facts: the members flag from the
		// world definition, the default world-type label (no Go config
		// carries a setWorldType override; GameConstants.ts:54), and
		// the configured developer usernames (DEV_USERNAME).
		BootstrapWorld: &session.BootstrapWorld{
			MembersWorld: worldDef.MembersWorld,
			TypeLabel:    "REGULAR",
			DevUsernames: env.DevUsernames,
		},
	}
	if loaded != nil {
		// The bootstrap's run-energy weight reads real item
		// definitions; without them ItemWeights stays nil and the
		// bootstrap sends weight 0.
		loginDeps.ItemWeights = itemWeights{defs: loaded.defs}
	}

	s := &Server{
		env:        env,
		worldDef:   worldDef,
		active:     active,
		loaded:     loaded,
		revision:   revision,
		world:      world,
		accounts:   accounts,
		saves:      saves,
		adapter:    adapter,
		loginDeps:  loginDeps,
		tickRateMs: tickRateMs,
		logger:     logger,
		plugins:    hub,
	}
	s.engine = game.NewEngine(tickRateMs, func(cycle int64) {
		world.Process()
	}, func(cycle int64, err error) {
		// TS saves all players on a tick error (GameEngine.ts:93,
		// World.savePlayers("tick-error")). This callback runs on
		// the engine's tick goroutine, from runTick's deferred
		// recover, serially after the failed tick and while the
		// engine's in-progress flag is still held (engine.go:
		// runTick defers the flag clear after the recover defer, so
		// the flag clears only after this callback returns) — no
		// tick can overlap it, so reading live player state in
		// saveAll is safe. Best-effort: saveAll recovers per player
		// and logs, and the engine continues ticking.
		saved, failed := coordinator.saveAll("tick-error")
		logger.Printf("[tick] cycle %d error: %v (tick-error save: saved=%d failed=%d)", cycle, err, saved, failed)
	})
	s.handler = session.NewHandler(func(ch session.Channel) *session.Connection {
		return session.NewConnection(ch, loginDeps, adapter, tickRateMs)
	}, session.NewGatewayTarget(
		// The gateway worldlist URL names the actual listen port
		// (NetworkBuilder.initialize passes port to
		// GatewayTarget.resolve). s.Port is the same deterministic
		// computation main.go runs again for ListenAndServe; s.env
		// and s.logger are already set in the Server literal above.
		// Type flags 0 / activity "" are the TS defaults
		// (GameConstants.WORLD_TYPE, GameConstants.ts:54): the Go
		// world definition carries no world-type source (no
		// setWorldType equivalent; world.json has no
		// type/flags/activity key).
		s.Port(opts.Port), s.revision, worldDef.MembersWorld, 0, "", s.logger,
	))
	return s, nil
}

// Start begins the tick engine. The returned context is cancelled by
// Stop; callers that need their own cancellation should pass a
// child context here instead — Start uses context.Background and
// Stop cancels it.
//
// The server-startup plugin hooks fire once, on the first Start,
// after the engine starts (TS: emitServerStartup in GameBuilder,
// after the game is built). A second Start restarts nothing and
// emits nothing.
func (s *Server) Start() {
	first := !s.started.Swap(true)
	s.engine.Start(context.Background())
	if first && s.plugins != nil {
		s.plugins.EmitServerStartup(&plugins.ServerLifecycleEvent{Timestamp: time.Now().UnixMilli()})
	}
}

// Stop saves online players, halts the tick engine, flushes account
// state, and closes the cache store when one was opened at boot
// (Store.Close is idempotent, so repeated stops are safe). Callers
// must stop the HTTP server separately.
//
// Shutdown order (Server.ts gracefulShutdown, ~200-203): save all
// online players with reason "shutdown", then flush accounts, then
// flush persistence (the Go stores write synchronously, so there is
// no persistence flush step). The save-all snapshots on the tick
// goroutine before the engine stops (see
// saveCoordinator.requestShutdownSnapshot) and writes the immutable
// snapshots after the engine stops, when no tick can move a player.
//
// DEVIATION from TS: TS cancels the shutdown when a save fails
// (Server.ts:204-211). A SIGINT shutdown cannot be cancelled here,
// so save failures are logged and shutdown proceeds.
//
// The server-shutdown plugin hooks fire after the engine stops and
// before the shutdown saves are written, matching the TS order
// (Server.ts:199-200: emitServerShutdown, then savePlayers). They
// run after the engine stops so no tick can race a hook touching
// player state; the shutdown snapshots were already taken on the
// tick goroutine before the engine stopped, so a shutdown hook
// that mutates live player state is NOT reflected in the shutdown
// saves.
func (s *Server) Stop() {
	var snapshots []*persist.PlayerSave
	if s.adapter != nil && s.adapter.saves != nil {
		snapshots = s.adapter.saves.requestShutdownSnapshot(s.started.Load(), s.adapter.saves.shutdownWaitTimeout())
	}
	s.engine.Stop()
	if s.plugins != nil {
		s.plugins.EmitServerShutdown(&plugins.ServerLifecycleEvent{Timestamp: time.Now().UnixMilli()})
	}
	if len(snapshots) > 0 && s.adapter != nil && s.adapter.saves != nil {
		saved, failed := s.adapter.saves.writeSnapshots(snapshots, "shutdown")
		s.logger.Printf("[shutdown] saved online players: saved=%d failed=%d", saved, failed)
	}
	// Plugin-registered objects are runtime state only (nothing
	// about objects persists); TS removes them after persistence
	// purely to clean up (Server.ts:223, ObjectManager
	// removePluginObjects). The engine is stopped, so no tick
	// can race the removal.
	if s.adapter != nil && s.adapter.world != nil && s.adapter.world.GameObjects() != nil {
		removed := s.adapter.world.GameObjects().RemovePluginObjects()
		s.logger.Printf("[shutdown] removed plugin objects: %d", removed)
	}
	if err := s.accounts.Flush(); err != nil {
		s.logger.Printf("[shutdown] flushing accounts: %v", err)
	}
	// The SQLite store holds its database connection open; close it
	// after the last write above (shutdown save-all runs before this
	// point, so no save can follow the close). Other backends have
	// nothing to close. Close is idempotent, so repeated stops are
	// safe.
	if store, ok := s.saves.(*persist.SQLiteStore); ok {
		if err := store.Close(); err != nil {
			s.logger.Printf("[shutdown] closing SQLite save database: %v", err)
		}
	}
	if s.loaded != nil && s.loaded.store != nil {
		if err := s.loaded.store.Close(); err != nil {
			s.logger.Printf("[shutdown] closing cache store: %v", err)
		}
	}
}

// Handler serves the game WebSocket plus 404s for other HTTP routes
// (see session.NewHandler).
func (s *Server) Handler() http.Handler { return s.handler }

// Port returns the configured WebSocket/HTTP port: the --port flag
// wins, then WEBSOCKET_PORT, then the code default. An invalid value
// falls back to the default with a warning; it is an operational
// choice, not TS behavior (TS would listen on whatever parseInt
// produced, including 0 for garbage input).
func (s *Server) Port(flagPort int) int {
	port := flagPort
	if port == 0 {
		port = s.env.WebsocketPort
	}
	if port <= 0 || port > 65535 {
		// 43594 is the code default (NetworkConstants.WEBSOCKET_PORT,
		// net/NetworkConstants.ts:3).
		const defaultPort = 43594
		s.logger.Printf("[net] invalid WebSocket port %d, using default %d", port, defaultPort)
		return defaultPort
	}
	return port
}

// Revision returns the active cache revision clients must report.
func (s *Server) Revision() int { return s.revision }

// WorldName returns the configured world name.
func (s *Server) WorldName() string { return s.worldDef.Name }

// saveAdapter adapts a persist.SaveStore to the session package's
// SaveStore port. The two differ in exactly two ways: a missing save
// is (nil, nil) for the session layer but persist.ErrSaveNotFound for
// persist, and persist.Save derives its key from the save itself
// while the session Save passes the key explicitly.
type saveAdapter struct {
	store persist.SaveStore
}

func (a saveAdapter) Load(username string, profile persist.Profile) (*persist.PlayerSave, error) {
	save, err := a.store.Load(username, profile)
	if err != nil {
		if errors.Is(err, persist.ErrSaveNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return save, nil
}

func (a saveAdapter) Save(username string, profile persist.Profile, save *persist.PlayerSave) error {
	if save == nil {
		return errors.New("saveAdapter: cannot save a nil PlayerSave")
	}
	// The store keys the save by save.DisplayName (the login name as
	// first typed, capitals included) and save.Profile. The session
	// layer passes the key it loaded under; keep the stored display
	// name when present (it wins over the typed form) and fill it
	// from the key only when the save has none.
	save.Profile = profile
	if save.DisplayName == "" {
		save.DisplayName = username
	}
	return a.store.Save(save, "login")
}

// pendingKey renders a name in the key form the login layer uses for
// online/pending checks: DisplayNames.Key, then protocol form. It
// reproduces session.pendingKey (unexported) exactly, from the same
// exported pieces, so the adapter's online set and the session's
// IsOnline queries agree.
func pendingKey(name string) string {
	return session.FormatNameForProtocol(persist.Key(name))
}

// worldAdapter implements session.WorldPort against a game.World. It
// owns the cross-goroutine bookkeeping TS gets for free from its
// single event loop: pending-name reservations, the online-key set
// (including players still queued for addition), and the wait that
// turns the world's queued addition into the synchronous index
// session.Connection.EnterWorld expects.
type worldAdapter struct {
	world      *game.World
	spawn      game.Location
	addTimeout time.Duration
	keys       cache.Keys // nil in NoCache mode; Keys.Get is nil-safe
	// devUsernames grants DEVELOPER rights on new-character saves
	// (DEV_USERNAME, via prepareCarriedSave). Set by buildServer.
	devUsernames []string
	// saves is the save coordinator (nil for an adapter built
	// standalone in tests: entry still applies the save to the live
	// player, but no carried save is registered and no disconnect
	// save runs). Set by buildServer.
	saves *saveCoordinator
	// musicData is the boot-loaded music data (areas + track
	// lengths), installed on every entering session. Nil for a
	// standalone adapter: sessions then run the TS
	// empty-areas/no-lengths behavior. Set by buildServer.
	musicData *music.Data
	// groundItems is the world's ground-item manager, installed
	// on the world's GroundItems hook. Nil for a standalone
	// adapter: entering sessions get an empty visible-item
	// source. Set by buildServer.
	groundItems *game.GroundItemManager
	// wilderness is the world's Wilderness (PvP/safe/multi
	// helpers over the world.json zone boundaries), wired to
	// the drop listener through the Deps seam. Nil for a
	// standalone adapter: drops degrade to the pre-area
	// always-register behavior. Set by buildServer.
	wilderness *game.Wilderness
	// itemDefinitions is the game-layer item-definition source
	// over the loaded definition Store, installed on every
	// entering player by prepareCarriedSave before the live
	// containers load (bank loading stacks through it). Nil for
	// a standalone adapter or with no definitions loaded: the
	// game layer tolerates a nil source with documented
	// defaults. Set by buildServer.
	itemDefinitions game.ItemDefinitionSource
	// plugins is the loaded plugin hub (see pluginwiring.go). Nil
	// for a standalone adapter: login/logout/disconnect emits are
	// skipped. Set by buildServer.
	plugins *plugins.Hub
	// sailing is the Sailing runtime (see sailingwiring.go),
	// installed once at boot. Nil for a standalone adapter:
	// login/logout sailing transitions are skipped. Set by
	// buildServer.
	sailing *game.Sailing
	// replacements is the world's map-region replacement
	// manager (see instancingwiring.go), installed once at
	// boot. Nil for a standalone adapter: per-player
	// replacement streaming is skipped. Set by buildServer.
	replacements *game.MapRegionReplacementManager
	// varbitDefs is the cache definition facade for varbit
	// lookups (the varp store's resolver source, phase 18).
	// Nil in NoCache mode or for a standalone adapter:
	// sessions then run with no resolver and every varbit
	// reads 0. Set by buildServer.
	varbitDefs *cache.CacheDefinitions
	// commands is the hub's command registry, wired by
	// wireCommandRegistry (see commandwiring.go). Nil for a
	// standalone adapter: "::" text drops with ReasonCommand.
	// Set by buildServer.
	commands *game.CommandRegistry
	// punishment is the loaded mute/ban lists, read by the chat
	// listener's mute gate. Nil for a standalone adapter:
	// nobody reads as muted. Set by buildServer.
	punishment *persist.Punishment
	// examine is the definition-backed examine source (see
	// chatwiring.go). Nil for a standalone adapter: examine
	// routes drop with their examine reasons. Set by
	// buildServer.
	examine listeners.ExamineSource
	// objectExamineIntercept runs the world:object-examine
	// plugin event (see chatwiring.go). Nil for a standalone
	// adapter: the no-hooks default applies. Set by buildServer.
	objectExamineIntercept func(*game.Player, int) bool
	// objectDefs is the definition-backed object-definition
	// source for the object-action / item-on-object listeners
	// (see objectlistenerwiring.go). Nil for a standalone
	// adapter: the listeners drop with their unwired reasons.
	// Set by buildServer.
	objectDefs listeners.ObjectDefinitionSource
	// objectQueries/objectFeed are the world-shared object
	// registry queries + delta feed for the per-session object
	// sync (see objectwiring.go). Set by buildServer.
	objectQueries ObjectRegistryQueries
	objectFeed    *session.SharedObjectDeltaFeed
	// dialoguePlayback is the transcript playback engine (see
	// dialoguewiring.go), built over the loaded dialogue
	// catalogue. Nil for a standalone adapter: NPC clicks fall
	// through the dialogue seam and resume packets drop with
	// ReasonDialogueUnwired. Set by buildServer.
	dialoguePlayback *game.DialoguePlayback
	// prompts is the world-shared MultiChatboxPrompt registry
	// (see dialoguewiring.go): the playback presents choice
	// menus through it and the resume dispatch checks it
	// before the dialogue manager. Nil for a standalone
	// adapter: menus reach the unavailable path. Set by
	// buildServer.
	prompts *game.MultiChatboxPrompt
	// npcDefs is the definition-backed NPC-definition source
	// for the NPC-option listener (see dialoguewiring.go). Nil
	// for a standalone adapter: the listener drops with its
	// unwired reason. Set by buildServer.
	npcDefs listeners.NpcDefinitionSource
	// store is the loaded definition Store (immutable after
	// Load), read by the shop seams: the NPC static-interaction
	// source and the dialogue shop-name lookup. Nil for a
	// standalone adapter: those seams stay unset. Set by
	// buildServer.
	store *definition.Store
	// shops is the world's ShopManager (see shopwiring.go),
	// wired to the NPC Trade dispatch, the shop widget branch,
	// and the disconnect close. Nil for a standalone adapter:
	// shop clicks fall through and NPC Trade clicks degrade
	// to the dialogue seam. Set by buildServer.
	shops *game.ShopManager
	// npcNameResolver is the NPC-name lookup installed on every
	// entering player's DialogueManager (NpcDialogue.ts:35).
	// Nil for a standalone adapter: chatheads read the DEFAULT
	// name ("null"). Set by buildServer.
	npcNameResolver func(int) string

	mu          sync.Mutex
	waiters     map[*game.Player]chan int
	cancelled   map[*game.Player]bool
	onlineKeys  map[string]*game.Player
	playerKeys  map[*game.Player]string
	pendingName map[string]bool
}

func newWorldAdapter(world *game.World, spawn game.Location, addTimeout time.Duration) *worldAdapter {
	return &worldAdapter{
		world:       world,
		spawn:       spawn,
		addTimeout:  addTimeout,
		waiters:     map[*game.Player]chan int{},
		cancelled:   map[*game.Player]bool{},
		onlineKeys:  map[string]*game.Player{},
		playerKeys:  map[*game.Player]string{},
		pendingName: map[string]bool{},
	}
}

// IsFull reports whether the world player list is full.
func (a *worldAdapter) IsFull() bool { return a.world.IsFull() }

// XteaKey returns the XTEA key for a region, or the all-zero key
// when the region has none or no keys were loaded (NoCache mode).
// The per-tick flush glue uses it for region rebuilds.
func (a *worldAdapter) XteaKey(regionID int) [4]int32 { return a.keys.Get(regionID) }

// AddPlayer queues the entering player for addition on the next tick
// and waits for the tick loop to assign its index (normally under one
// tick). TS adds the player synchronously because network handling
// and the tick share one event loop; in Go the wait preserves the
// synchronous contract session.Connection expects without touching
// world state off the tick goroutine.
//
// It returns the turn-packet sink bound to the admitted player (see
// session.TurnSink): the connection hands entered, non-immediate
// packets to it, and it enqueues them on the player's turn queue for
// Player.Process to drain on the next tick. A nil sink is never
// returned on success.
//
// The entering save IS applied: prepareCarriedSave (savewiring.go)
// migrates the save, applies its position/running to the player (or
// builds a new-character save), and the result is registered as the
// player's carried save — activated only when the add-wait succeeds,
// so a login that never enters never writes a save. The autosave
// schedule starts on successful entry; the disconnect save runs from
// onPlayerDisconnect. See saveCoordinator.
func (a *worldAdapter) AddPlayer(ep *session.EnteringPlayer) (int, session.TurnSink, error) {
	if a.world.IsFull() {
		return 0, nil, errors.New("world full")
	}
	p := game.NewPlayer(ep.Username, a.spawn)
	p.DisplayName = ep.DisplayName
	p.Profile = ep.Profile
	// Dialogue wiring: install the NPC-name resolver before
	// queueing, so no tick can send a chathead before the
	// resolver exists (see dialoguewiring.go). A standalone
	// adapter leaves it nil and chatheads read "null".
	if a.npcNameResolver != nil {
		p.GetDialogueManager().SetNpcNameResolver(a.npcNameResolver)
	}
	// Assign the session only when one exists: ep.Session is a
	// concrete *session.Session, and assigning a nil one would store
	// a typed-nil SessionPort interface, which compares non-nil and
	// panics on IsOpen. A nil session (tests only) leaves the
	// interface nil, and World skips flushing session-less players.
	if ep.Session != nil {
		p.Session = ep.Session
	}
	p.RemoteAddr = ep.RemoteAddr
	// Entry application (enterWorld order, NetworkBuilder.ts:1653-1676):
	// migrate + apply the save, or build a new-character save. The
	// player is not yet queued, so mutating it here races no tick.
	carried := a.prepareCarriedSave(p, ep)
	// Container refresh wiring: after entry loading (so
	// save-loaded banks receive hooks) and before queueing (so no
	// tick can refresh a container before its hook exists). The
	// native-client flag gates the worn-inventory run-script
	// packet on equipment refresh (see installContainerRefresh).
	installContainerRefresh(p, ep.Capabilities&protocol.HandshakeCapabilityNativeClient != 0)
	// Per-tick sync wiring: attach the flush source and the XTEA
	// key provider before queueing, so the player's first
	// FlushTick — possibly the same tick it is added, since the
	// add phase precedes the flush phase — already has a view to
	// send and real rebuild keys. In NoCache mode XteaKey answers
	// all-zero keys. A nil session (tests only) gets no wiring;
	// World skips flushing session-less players.
	// The world's boat manager, or nil when sailing is not
	// installed (standalone adapters): the flush source, the
	// world-entity listener seams, the movement queue, and
	// helm steering all read this world's manager, never a
	// process-wide global (see World.SetBoatManager).
	// BoatManager is safe on a nil Sailing.
	boats := a.sailing.BoatManager()
	if ep.Session != nil {
		ep.Session.AttachSyncSource(&playerFlushSource{owner: p, boats: boats, worldEntities: a.world.WorldEntities()})
		ep.Session.SetXteaProvider(a.XteaKey)
		// Region-replacement streaming (see
		// instancingwiring.go): on every replacement-window
		// change the visible replacements stream to this
		// player. Nil-safe: a nil manager installs nothing.
		InstallPlayerRegionReplacements(ep.Session, a.replacements)
		ep.Session.SetMusicData(a.musicData)
		installAreaMusicSource(ep.Session, p)
		installGroundItemSource(ep.Session, a.groundItems, a.world, p)
		InstallObjectSync(ep.Session, NewRegistryObjectSource(a.objectQueries, p, a.objectFeed), p)
		// Skills/appearance wiring: the skill-update hook and
		// the equipment-change appearance rebuild (see
		// installSkillsAppearanceWiring). The skull icon in
		// each rebuild comes from the player's live skull
		// state (HeadSkullIcon), already loaded from the
		// carried save by prepareCarriedSave — no entry-time
		// snapshot is closed over, so a skull gained or lost
		// mid-session reaches the sync block at the next
		// rebuild (a skull change triggers one, see skull.go).
		installSkillsAppearanceWiring(p, ep.Session)
		// Skills exp-drop wiring (see skillswiring.go): the
		// SkillManager exp-drop hook calling
		// Session.SendExpDrop (a deliberate no-op on the wire,
		// exactly as TS — see installSkillExpDropWiring).
		// A parallel install next to the appearance wiring
		// above; neither disturbs the other.
		installSkillExpDropWiring(p, ep.Session)
		// Interface runtime wiring (see interfacewiring.go):
		// the per-player interface deps, the varbit resolver
		// for the session's varp store, and the login audio
		// replay. The resolver reads varbit definitions from
		// the cache facade; an undefined varbit resolves to
		// not-ok, matching the TS default-constructed
		// VarBitType behavior. The audio replay ports the
		// onLogin audio loop (Player.ts:801-810): each saved
		// audio setting goes out via sendConfig, with varp 19
		// merged over the live value. prepareCarriedSave has
		// already applied the save's audio settings to the
		// player, so the replay sends the carried values.
		installInterfaceRuntime(ep.Session, p, a.store, a.shops, a.plugins)
		if a.varbitDefs != nil {
			defs := a.varbitDefs
			ep.Session.SetVarbitResolver(func(id int) (int, int, int, bool) {
				def := defs.GetVarbit(id)
				if def == nil || !def.Defined() {
					return 0, 0, 0, false
				}
				return def.BaseVar, def.StartBit, def.EndBit, true
			})
		}
		ep.Session.ApplyAudioSettings(p.GetAudioSettings())
		// Combat-style login assign (Player.ts:793): after
		// equipment restore and interface-runtime install,
		// assign the combat-tab interface for the equipped
		// weapon (varbit 357, tab 593, varp 43/172). A nil
		// session skips it; session-less test players are
		// never flushed.
		game.AssignWeaponInterface(p)
		// Autocast login restore (Player.ts:795-800, see
		// restoreEntryAutocast in spellwiring.go): after the
		// weapon-interface assign, restore the saved autocast
		// spell through setAutocast.
		restoreEntryAutocast(p, carried)
	}
	// The turn handler decodes WALK / MOVE_GAMECLICK packets into
	// movement requests (see handleTurnPacket); every other turn
	// opcode goes to listeners.Handle with the world's
	// ground-item store, which the drop and pickup listeners
	// register with and resolve from (see Deps). The field is
	// set only when the manager is non-nil: assigning a nil
	// *GroundItemManager would store a typed-nil interface that
	// compares non-nil and panics on use. A nil manager
	// (standalone adapter) degrades those listeners to
	// ReasonGroundItemsUnwired without disconnecting.
	deps := listeners.Deps{}
	if a.groundItems != nil {
		deps.GroundItems = a.groundItems
	}
	// Wilderness drop seam (see areaswiring.go): the drop
	// listener registers Wilderness drops globally. A nil
	// Wilderness (standalone adapter) leaves the seam unset
	// and drops keep the always-register behavior.
	if a.wilderness != nil {
		deps.WildernessIn = a.wilderness.IsInLocation
	}
	// Chat + command + examine wiring (see chatwiring.go and
	// commandwiring.go): the Deps value is built once per
	// entering player and captured by the TurnHandler closure
	// below, so the warned flag pointer is shared across this
	// player's packets, as the per-connection TS flag is. A
	// standalone adapter leaves the seams nil and the listeners
	// degrade to their documented no-op/drop behavior. The
	// registry assignment is guarded: assigning a nil
	// *CommandRegistry would store a typed-nil interface that
	// compares non-nil and misreports "::" text as acted.
	if a.commands != nil {
		deps.Chat.Commands = a.commands
	}
	deps.Chat.Delivery = chatDelivery{}
	deps.Chat.NearbyPlayers = a.world.NearbyPlayersForUpdate
	if a.punishment != nil {
		pun := a.punishment
		deps.Chat.Muted = func(username, host string) bool { return pun.Muted(username) || pun.IPMuted(host) }
	}
	deps.Chat.SocialIntercept = socialPacketIntercept(a.plugins)
	// Social-packet dispatch (see socialwiring.go): the five
	// social opcodes + the CHAT clan/friends-chat kinds emit
	// through the hub. A nil hub leaves SocialEmit nil.
	installSocialListeners(&deps, a.plugins)
	deps.Examine = a.examine
	deps.ObjectExamineIntercept = a.objectExamineIntercept
	InstallObjectListeners(&deps, a.plugins, a.world.GameObjects(), a.objectDefs)
	InstallItemActionListeners(&deps, a.plugins)
	InstallDialogueListeners(&deps, a.dialoguePlayback, a.plugins, a.prompts, a.world.NPCs(), a.npcDefs)
	// Combat wiring (see combatwiring.go): the player lookup,
	// the player-option hub adapters, the NPC attack-option
	// gate adapter, and the combat-attack seam. Runs after
	// InstallDialogueListeners: InstallCombatListeners sets
	// only the EmitNpcAttackOption field on the NpcHooks
	// struct InstallDialogueListeners installed, and running
	// first would have the field wiped.
	InstallCombatListeners(&deps, a.plugins, a.world.Players())
	// Spell wiring (see spellwiring.go): the magic listeners'
	// spell seams — cache spell-name lookup, spellbook read,
	// combat spell cast, rune/disabled gates, and tick-task
	// scheduling. The definitions facade is nil in NoCache
	// mode, so spell ids resolve by id only there.
	InstallSpellListeners(&deps, a.varbitDefs, a.world)
	// Shop wiring (see shopwiring.go): the NPC static-interaction
	// source + OpenShop on the NpcHooks struct, and the shop
	// widget handler. Runs after InstallDialogueListeners
	// (which replaces deps.NpcHooks wholesale) and
	// InstallCombatListeners, for the same reason as the
	// combat ordering above.
	InstallShopListeners(&deps, a.store, a.shops)
	// Interface-action wiring (see
	// interfaceactionwiring.go): the hub emits for
	// interface-action clicks / widget-action / world-input /
	// interface-close, and the session-side interface sends.
	// A nil hub leaves the hub seams unset; the sends install
	// regardless.
	InstallInterfaceActionListeners(&deps, a.plugins)
	// World-entity wiring (see sailingwiring.go): the
	// world-entity interaction gate/emit + helm-heading seams
	// for opcodes 220-225/214. A nil hub/manager leaves the
	// seams unset and the clicks drop.
	installWorldEntityListenerDeps(&deps, a.plugins, boats)
	warned := false
	deps.UnresolvedInventoryWarned = &warned
	p.TurnHandler = func(pl *game.Player, pkt game.TurnPacket, cycle int64) {
		handleTurnPacketWithDeps(pl, pkt, cycle, deps)
	}
	// Movement wiring: attach the world's collision/clipping to the
	// player's queue (open ground in NoCache mode, where the world
	// keeps its default empty RegionManager), and install the walk()
	// hook (NetworkBuilder.ts:1739-1752) that routes decoded
	// requests through the queue. The hook runs on the tick
	// goroutine as Player.Process drains the turn queue.
	rm := a.world.RegionManager()
	// Actor-aware surfaces (see LookupForActor): step
	// validation and route-finding read the player's current
	// private area fresh on every call, so boarding a boat
	// or entering an instance re-routes on the area's clips
	// without reinstalling. The NPC queues get the same
	// install in World.drainAddNPCs.
	area := func() *game.PrivateArea { return p.PrivateArea() }
	p.MovementQueue().SetCollision(rm.CollisionForActor(area))
	p.MovementQueue().SetClipping(rm.LookupForActor(area))
	// The world's boat manager, so deck walk-tos project onto
	// the main world (nil in worlds without sailing).
	p.MovementQueue().SetBoatManager(boats)
	p.OnMovementRequest = func(pl *game.Player, req game.MovementRequest, cycle int64) {
		_ = cycle // Routing is immediate; stepping happens on later ticks via Player.Process.
		// TS walk() steers a boat helm first: at a helm a
		// click sets the heading instead of walking
		// (NetworkBuilder.ts:1742-1743). The combat reset
		// walk() also runs stays omitted, as before.
		// TargetLocID stays discarded: the TS "move" message
		// has no target field (see game.MovementRequest).
		if boats != nil && boats.SteerToward(&pl.Mobile, req.X, req.Y) {
			return
		}
		destination := game.Location{X: req.X, Y: req.Y, Z: pl.Loc.Z}
		var runOverride *bool
		if req.Run {
			run := true
			runOverride = &run
		} else {
			runOverride = game.ClickRunOverride(pl.IsRunning(), req.ModifierFlags)
		}
		pl.MovementQueue().RequestWalk(destination, runOverride)
	}

	key := pendingKey(ep.Username)
	notify := make(chan int, 1)
	a.mu.Lock()
	a.waiters[p] = notify
	a.onlineKeys[key] = p
	a.playerKeys[p] = key
	a.mu.Unlock()

	// Register the carried save BEFORE queueing: it is pending until
	// the add-wait below succeeds (see carriedEntry). A standalone
	// adapter has no coordinator and registers nothing.
	if a.saves != nil {
		a.saves.registerPending(p, carried)
	}

	a.world.QueueAddPlayer(p)

	sink := func(pkt protocol.Packet) bool {
		// protocol.Packet payloads are copies owned by the packet
		// (DecodeFrame), so retaining the slice in the turn queue
		// is safe: nothing mutates it afterwards.
		return p.QueueTurnPacket(game.TurnPacket{Opcode: pkt.Opcode, Payload: pkt.Payload})
	}

	timer := time.NewTimer(a.addTimeout)
	defer timer.Stop()
	var index int
	entered := false
	select {
	case index = <-notify:
		entered = true
	case <-timer.C:
		// A notification may have landed just as the timer fired;
		// take it instead of failing a login that succeeded.
		select {
		case index = <-notify:
			entered = true
		default:
		}
	}
	if entered {
		// Activation: the player is in the world, so its carried
		// save may now be written, and its autosave schedule
		// starts. Both happen before returning, so no removal can
		// observe an unactivated entry for an entered player.
		if a.saves != nil {
			a.saves.activate(p)
			a.saves.schedule(p)
		}
		// Back aboard a boat they logged out on at sea
		// (NetworkBuilder.ts:1692: after the login
		// extension, before the login emit + bootstrap).
		// Nil for a standalone adapter: skipped.
		if a.sailing != nil {
			a.sailing.OnLogin(p)
		}
		// Player-login plugin hooks (TS: emitPlayerLogin in
		// enterWorld, NetworkBuilder.ts:1694-1700, after world
		// entry and before the bootstrap, which the session
		// layer sends after this returns). IsNewAccount is true
		// only when no save existed for the profile
		// (pending.save == null).
		if a.plugins != nil {
			a.plugins.EmitPlayerLogin(&plugins.PlayerLoginEvent{
				Player:       p,
				Username:     p.Username,
				IsNewAccount: ep.Save == nil,
				Profile:      p.Profile,
			})
		}
		return index, sink, nil
	}
	a.mu.Lock()
	delete(a.waiters, p)
	// The player may still be added by a later tick; mark it
	// cancelled so onPlayerAdded removes it immediately
	// instead of orphaning it in the world.
	a.cancelled[p] = true
	a.mu.Unlock()
	if a.saves != nil {
		// A login that never entered must never write a save:
		// drop the pending entry without saving (see
		// carriedEntry). The late add, if any, is removed by
		// onPlayerAdded's cancelled path, and its disconnect
		// finds no entry and saves nothing.
		a.saves.removeEntry(p)
	}
	return 0, nil, errors.New("timed out waiting for world entry")
}

// ProcessCycle reports the world's current process cycle for the login
// bootstrap's map clock.
func (a *worldAdapter) ProcessCycle() int64 { return a.world.ProcessCycle() }

// handleTurnPacket is the game.Player TurnHandler: it runs on the tick
// goroutine as Player.Process drains the turn queue, porting the
// "move" case of dispatch() (NetworkBuilder.ts:738-740) in decoded
// form. WALK (opcode 210) and MOVE_GAMECLICK (native opcode 16) decode
// into a game.MovementRequest delivered to the player's
// OnMovementRequest hook, which the wired server installs with the
// MovementQueue request path (see AddPlayer); a nil hook (a bare
// NewPlayer outside the wired server) drops the movement request.
// Every other turn opcode goes to listeners.Handle (phase 6): each
// packet listener decodes its payload and either acts (only where
// the target system exists in Go) or drops it with a named,
// documented reason — see internal/listeners and PORT_STATUS.md.
// Opcodes no listener recognizes are dropped by the default case.
// A malformed payload is dropped: DecodeFrame already enforced the
// exact lengths, so a decode failure here is unreachable for
// well-formed frames.
//
// handleTurnPacket carries no listener dependencies: it delegates
// to handleTurnPacketWithDeps with empty Deps, so drop/pickup
// degrade to ReasonGroundItemsUnwired. The wired server installs
// a closure over handleTurnPacketWithDeps with the world's
// ground-item manager in AddPlayer; tests call this function
// directly.
func handleTurnPacket(p *game.Player, pkt game.TurnPacket, cycle int64) {
	handleTurnPacketWithDeps(p, pkt, cycle, listeners.Deps{})
}

// handleTurnPacketWithDeps is handleTurnPacket with the listener
// dependencies supplied: deps.GroundItems is the world ground-item
// store the drop and pickup listeners register with and resolve
// from. It is a free function (not a worldAdapter method) so the
// 3-argument handleTurnPacket keeps its existing signature for
// direct callers and tests.
func handleTurnPacketWithDeps(p *game.Player, pkt game.TurnPacket, cycle int64, deps listeners.Deps) {
	switch pkt.Opcode {
	case protocol.ClientWalk:
		if p.OnMovementRequest == nil {
			return
		}
		x, y, run, mods, err := protocol.DecodeWalkFull(pkt.Payload)
		if err != nil {
			return
		}
		req := game.MovementRequest{X: x, Y: y, Run: run, ModifierFlags: mods}
		if listeners.MovePreamble(p, req, deps) {
			return
		}
		p.OnMovementRequest(p, req, cycle)
	case protocol.NativeMoveGameclick:
		if p.OnMovementRequest == nil {
			return
		}
		x, y, mods, target, err := protocol.DecodeMoveGameclick(pkt.Payload)
		if err != nil {
			return
		}
		req := game.MovementRequest{X: x, Y: y, ModifierFlags: mods, TargetLocID: target}
		if listeners.MovePreamble(p, req, deps) {
			return
		}
		p.OnMovementRequest(p, req, cycle)
	default:
		listeners.Handle(p, pkt, cycle, deps)
	}
}

// onPlayerAdded runs from MobileList.Add (tick goroutine) after the
// index is assigned, without any list lock held. It wakes the login
// waiting for this player, or immediately removes a player whose
// login already timed out. It must not block: the notification
// channel is buffered and the send is guarded.
func (a *worldAdapter) onPlayerAdded(p *game.Player) {
	p.LoggedIn = true
	a.mu.Lock()
	notify, waiting := a.waiters[p]
	cancelled := a.cancelled[p]
	if waiting {
		delete(a.waiters, p)
	}
	a.mu.Unlock()
	if cancelled {
		a.world.QueueRemovePlayer(p.Index)
		return
	}
	if waiting {
		select {
		case notify <- p.Index:
		default:
			// The waiter went away; the player stays queued for
			// removal by the connection cleanup path.
		}
	}
}

// onPlayerDisconnect runs from the remove-players phase when a
// player actually leaves the world. It clears the adapter's online
// bookkeeping for exactly this player instance: a same-name login
// that replaced this player owns the key now and must not be
// unregistered by a stale disconnect. It then saves the player with
// reason "logout" through the save coordinator (see
// saveCoordinator.disconnectSave for the grounding); the save runs
// after the bookkeeping lock is released, so a slow store never
// blocks online/pending checks. `source` is the plugin disconnect
// source and does not affect the save: every removal saves as
// "logout" in TS.
func (a *worldAdapter) onPlayerDisconnect(p *game.Player, source string) {
	// Player-disconnect plugin hooks fire first, before the
	// disconnect save below: TS emits the disconnect event in the
	// removal phase (World.ts:1260-1264, non-bot registered
	// players only — the gate is applied by drainRemovePlayers
	// before this hook runs), and content that touches player
	// state should see the save that follows. The source default
	// ("removed") is applied by the caller.
	if a.plugins != nil {
		a.plugins.EmitPlayerDisconnect(&plugins.PlayerDisconnectEvent{Player: p, Username: p.Username, Source: source})
	}
	// A pending multi-chatbox prompt dies with the player
	// (the TS WeakMap entry vanishes with the player); without
	// this the registry entry lingers until its TTL prunes it.
	if a.prompts != nil {
		a.prompts.Forget(p)
	}
	// World-entity viewer state dies with the player too (TS
	// WorldEntitySync.viewers is a WeakMap; Go's map must be
	// told), covering disconnect-without-logout. Safe on a
	// nil world (standalone adapters).
	a.world.ForgetWorldEntityViewer(p)
	// An open shop closes and an in-flight trade aborts before
	// the disconnect save below: the shop close drops the
	// open-shop state (ShopManager.processPlayer would
	// auto-close on interface loss, and no interface state
	// exists to notice), and the trade abort returns offered
	// items to the inventory (Trading.abort's live-slot sweep,
	// with the ground-item fallback) and notifies the partner,
	// so the save captures the post-trade inventory. Both are
	// no-ops when nothing is open. This is the single removal
	// site: logout responses only close the socket, and the
	// removal + save happen here on the next tick.
	if a.shops != nil {
		a.shops.Close(p)
	}
	p.Trading().CloseTrade()
	// Relations presence goes offline AFTER the logout hooks
	// above (EmitPlayerDisconnect fired first; the accepted-
	// logout path fired EmitPlayerLogout in onLogoutAccepted)
	// and BEFORE the disconnect save below: the TS onLogout
	// order (Player.ts:725-765: emitPlayerLogout, then
	// updateLists(false), then save). This is the single
	// removal site, so one call covers the logout button, the
	// socket close, and kicks. The sends inside updateLists
	// are the legacy no-ops until content ports the snapshot.
	p.Relations().UpdateLists(false)
	a.mu.Lock()
	key, ok := a.playerKeys[p]
	if ok {
		delete(a.playerKeys, p)
		if a.onlineKeys[key] == p {
			delete(a.onlineKeys, key)
		}
		delete(a.cancelled, p)
	}
	a.mu.Unlock()
	if a.saves != nil {
		a.saves.disconnectSave(p)
	}
}

// onLogoutAccepted runs when a logout request is accepted. TS
// World.answerLogoutRequest calls session.logout()
// (PlayerSession.ts:323-329): the client is told with a
// LOGOUT_RESPONSE (encodeLogoutResponse defaults: reason "", success
// true), the queue is flushed, then the socket closes with 1000.
// Session.Close flushes pending packets first, so sending the
// response through the session before closing preserves that order.
//
// GAP: TS logout() also sends the walk interaction-mode reset
// (encodeWalkInteractionMode) before the logout response. No Go
// encoder for it exists yet, so it is omitted; the client clears
// interaction modes itself on logout (PlayerSession.ts logout
// comment, citing live behavior).
func (a *worldAdapter) onLogoutAccepted(p *game.Player) {
	// Record a boat at sea (and step ashore) before the
	// interface removal and the save below (Player.onLogout,
	// Player.ts:738: after closeTrade, before
	// sendInterfaceRemoval). Nil for a standalone adapter.
	if a.sailing != nil {
		a.sailing.OnLogout(p)
	}
	// Interface removal runs before the logout hooks, as in TS
	// Player.onLogout (Player.ts:739): closeTrade, then
	// sendInterfaceRemoval, then the area leave and the
	// emitPlayerLogout below. The removal frames queue before
	// the logout response, so they flush before the socket
	// closes. A session-less player has no interface to remove.
	if remover, ok := p.Session.(interface{ SendInterfaceRemoval() }); ok && remover != nil {
		remover.SendInterfaceRemoval()
	}
	// Player-logout plugin hooks fire after the removal and
	// before the logout save (Player.onLogout, Player.ts:752-755;
	// the disconnect save runs later, in the removal phase) and
	// before the channel disconnect. Emitting before the response
	// keeps a logout hook's packet from being lost to the closing
	// socket.
	if a.plugins != nil {
		a.plugins.EmitPlayerLogout(&plugins.PlayerLogoutEvent{Player: p, Username: p.Username})
	}
	if p.Session != nil {
		p.Session.SendPacket(protocol.EncodeLogoutResponse("", true))
		p.Session.Close(1000, "logout")
	}
}

// RemovePlayer queues the registered player at index for removal on
// the next tick. Online bookkeeping is cleared when the player
// actually leaves (onPlayerDisconnect), so a combat-held player
// still counts as online until then.
func (a *worldAdapter) RemovePlayer(index int) {
	a.world.QueueRemovePlayer(index)
}

// QueueLogout queues a logout request for the next tick.
func (a *worldAdapter) QueueLogout(index int) {
	a.world.QueueLogout(index)
}

// isOnline reports whether any in-world or queued player holds the
// given pendingKey-form key. Pending (reserved but not yet entered)
// names are NOT included: Authenticate checks those separately
// through ReserveName.
func (a *worldAdapter) isOnline(key string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	_, ok := a.onlineKeys[key]
	return ok
}

// reserveName atomically reserves a pending-login key, returning
// false when another login already holds it.
func (a *worldAdapter) reserveName(key string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.pendingName[key] {
		return false
	}
	a.pendingName[key] = true
	return true
}

// releaseName releases a key reserved through reserveName.
func (a *worldAdapter) releaseName(key string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	delete(a.pendingName, key)
}
