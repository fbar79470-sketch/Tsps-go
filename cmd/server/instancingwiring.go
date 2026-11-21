package main

import (
	"tsps-go/internal/game"
	"tsps-go/internal/plugins"
	"tsps-go/internal/protocol"
	"tsps-go/internal/session"
)

// Instancing wiring (phase 21): the cmd/server adapter between the
// game-side instancing systems (internal/game private_area.go,
// templated_instance_area.go, template_region.go,
// map_region_replacement.go), the plugin hub's instancing surface
// (internal/plugins instancing.go), and the session's
// rebuild/replacement seams (internal/session flush.go). It
// follows the skillswiring.go precedent: narrow seams in game and
// session, translation adapters here, where all sides are
// importable. Game never imports plugins; the session never
// imports game.
//
// Two entry points, wired by buildServer / AddPlayer in server.go
// (the coordinator adds the calls; this file only exposes them):
//
//   - InstallInstancing, once in buildServer, after the
//     AreaManager install (near InstallSkillHooks) and before
//     ticking starts: installs the template-region source seams,
//     the replacement manager's native/archive seams, loads
//     data/regions packs, installs the destroy hook, and
//     installs the hub wiring (AreaManager handle + region
//     replacement wiring).
//   - InstallPlayerRegionReplacements, once per entering player
//     in AddPlayer's `if ep.Session != nil` wiring block, next
//     to SetXteaProvider: installs the per-session replacement
//     sender FlushTick calls when the replacement window
//     changes.
//
// ORDERING (coordinator): the replacement manager must be
// created BEFORE the RegionManager, so the RegionManager reads
// through the replacement-aware wrapper:
//
//	replacements := game.NewMapRegionReplacementManager()
//	wrapped := &game.ReplacementRegionSource{Base: sources, Replacements: replacements}
//	world.SetRegionManager(game.NewRegionManager(wrapped, sources, revision >= 209))
//
// The ObjectDefSource stays the unwrapped adapter: replacements
// change map data, never definitions. InstallInstancing then
// receives the same manager, plus the UNWRAPPED cache source for
// the template seam — template regions read CacheMaps directly
// in TS (TemplateRegionData.ts reads CacheMaps, never the
// replacement manager), so the template source must not see
// replacements.
//
// NIL SAFETY. A nil hub skips the hub installs only; a nil world
// skips the destroy hook, the AreaManager handle, and the
// reload/stream wiring (the wiring's Manager is still installed
// when replacements is non-nil, so content replace calls reach
// the manager but reload/stream are inert); a nil replacements
// manager skips the manager seams and the pack load, and the
// installed wiring carries a nil Manager (content replace calls
// warn and refuse); nil region/definition sources degrade the template
// and native seams to "no data", as in NoCache mode; a nil warn
// silences pack-load warnings. A nil session or manager
// installs no per-player sender.

// InstallInstancing installs the phase-21 instancing seams. See
// the file header for the call site, the ordering requirement,
// and the nil-safety contract. regions is the UNWRAPPED cache
// source (see the header); defs is the object-definition source
// the templated collision builder reads; newTerrainFormat is the
// revision >= 209 terrain-format flag, shared with the
// RegionManager; archiveIDs resolves a region id to its cache
// terrain/object archive ids (CacheMaps.GetArchiveIDs), or nil
// when the cache is absent; replacementDir is the data/regions
// directory scanned for .pack files ("" scans nothing); warn
// receives pack-load warnings (nil silences them).
func InstallInstancing(hub *plugins.Hub, world *game.World, replacements *game.MapRegionReplacementManager, regions game.RegionDataSource, defs game.ObjectDefSource, newTerrainFormat bool, archiveIDs func(int) (int, int, bool), replacementDir string, warn func(string)) {
	// Template seams (TemplateRegionData.ts): the decode-once
	// cache reads through the existing RegionDataSource seam,
	// and the templated collision builder reads object
	// definitions through the ObjectDefSource seam. Both
	// tolerate nil sources (no data).
	game.SetTemplateRegionSource(regions, newTerrainFormat)
	game.SetTemplateObjectDefSource(defs)

	if replacements != nil {
		// Native/archive seams: NativeRegionPacket rebuilds
		// the cache map for restore, and .pack loading
		// validates archive ids against the cache.
		replacements.SetNativeSource(regions)
		replacements.SetArchiveIDs(archiveIDs)
		// data/regions scan (RegionManager.ts:52-84): .pack
		// files win over loose .terrain.dat/.loc.dat pairs;
		// unreadable entries warn and continue.
		if replacementDir != "" {
			game.LoadRegionReplacementDir(replacementDir, replacements, warn)
		}
	}

	if world != nil {
		// Destroy hook (PrivateArea.ts destroyWorldEntities):
		// registered NPCs queue for removal and spawned
		// objects deregister. The hook runs with the member
		// and object lists still intact; it must not call
		// Destroy re-entrantly. GAPS (see private_area.go):
		// pending NPCs in the World's add queue cannot be
		// spliced (no API), and instance ground items cannot
		// deregister (Go ground items carry no area
		// reference). Players are left alone: TS destroy
		// removes NPCs and objects, and members leave
		// through the normal area path.
		game.SetPrivateAreaDestroyHook(func(pa *game.PrivateArea) {
			for _, actor := range pa.Entities() {
				if actor != nil && actor.IsNPC() {
					world.QueueRemoveNPC(actor.GetIndex())
				}
			}
			for _, obj := range pa.GetObjects() {
				world.GameObjects().Deregister(obj, false)
			}
		})
	}

	if hub == nil {
		return
	}
	if world != nil {
		hub.SetAreaManager(world.Areas())
	}
	// The wiring installs even with a nil manager: the Manager
	// half reads nil (replace/restore members inert) while the
	// reload/stream closures stay installed and inert (they
	// nil-check the manager and world). Installing
	// unconditionally also clears any stale wiring from an
	// earlier install with a manager.
	hub.SetRegionReplacementWiring(plugins.RegionReplacementWiring{
		Manager: replacements,
		ReloadRegion: func(regionID int) bool {
			if world == nil {
				return false
			}
			regions := world.RegionManager()
			if regions == nil {
				return false
			}
			return regions.ReloadRegion(regionID)
		},
		StreamToConnected: func(regionID int) int {
			return streamReplacementToConnected(world, replacements, regionID)
		},
		StreamPacketToConnected: func(packet *game.RegionReplacementPacket) bool {
			return streamPacketToConnected(world, replacements, packet)
		},
	})
}

// InstallPlayerRegionReplacements installs the per-session
// replacement sender for one entering player: on every
// replacement-window change, FlushTick calls the hook with the
// scene center, and the visible replacements stream to this
// player (PlayerSession.ts:497-505,
// sendVisibleReplacementsToPlayer with radius 6 and
// allowReload=true). A nil session or manager installs nothing.
func InstallPlayerRegionReplacements(sess *session.Session, replacements *game.MapRegionReplacementManager) {
	if sess == nil || replacements == nil {
		return
	}
	sess.SetRegionReplacementSender(func(tileX, tileY int) {
		replacements.SendVisibleReplacements(replacementSink(sess), tileX, tileY, 6, nil, true)
	})
}

// replacementSink frames one prepared replacement packet for one
// session: protocol.EncodePacket over the opcode/payload pair,
// queued via SendPacket. It answers false when the session
// cannot send (closed channel) or framing fails, mirroring
// sendClientPacket's false (MapRegionReplacementManager.ts:
// sendPayloadToPlayer returns false without a live session).
// The sink closes over the *session.Session; the
// StreamToConnected helpers below build one per player from the
// player's SessionPort.
func replacementSink(sess *session.Session) game.RegionReplacementSink {
	return func(opcode int, payload []byte) bool {
		if sess == nil || !sess.IsOpen() {
			return false
		}
		frame, err := protocol.EncodePacket(byte(opcode), payload)
		if err != nil {
			return false
		}
		sess.SendPacket(frame)
		return true
	}
}

// replacementPortSink is replacementSink over a SessionPort: the
// form the connected-player streams use, since the World hands
// out players, not sessions. A nil port answers false.
func replacementPortSink(port game.SessionPort) game.RegionReplacementSink {
	return func(opcode int, payload []byte) bool {
		if port == nil || !port.IsOpen() {
			return false
		}
		frame, err := protocol.EncodePacket(byte(opcode), payload)
		if err != nil {
			return false
		}
		port.SendPacket(frame)
		return true
	}
}

// streamReplacementToConnected streams one region's replacement
// to every connected player (applyPluginRegionReplacement,
// PluginManager.ts:237-252: World.getPlayers filtered by
// isPlayerSessionConnected, sendReplacementToPlayer each) and
// reports how many players received it. A nil world streams to
// nobody.
func streamReplacementToConnected(world *game.World, replacements *game.MapRegionReplacementManager, regionID int) int {
	if world == nil || replacements == nil {
		return 0
	}
	streamed := 0
	for _, p := range world.Players().All() {
		if p == nil || p.Session == nil || !p.Session.IsOpen() {
			continue
		}
		if replacements.SendReplacement(replacementPortSink(p.Session), regionID, true) {
			streamed++
		}
	}
	return streamed
}

// streamPacketToConnected streams one prepared packet (the
// native map on restore) to every connected player
// (PluginManager.ts:4583-4592: one failed send does not stop
// the rest) and reports whether every send succeeded. A nil
// world or packet reports success (no sends attempted).
func streamPacketToConnected(world *game.World, replacements *game.MapRegionReplacementManager, packet *game.RegionReplacementPacket) bool {
	if world == nil || replacements == nil || packet == nil {
		return true
	}
	delivered := true
	for _, p := range world.Players().All() {
		if p == nil || p.Session == nil || !p.Session.IsOpen() {
			continue
		}
		if !replacements.SendRegionPacket(replacementPortSink(p.Session), *packet) {
			delivered = false
		}
	}
	return delivered
}
