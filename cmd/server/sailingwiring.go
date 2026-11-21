package main

import (
	"encoding/json"

	"tsps-go/internal/game"
	"tsps-go/internal/listeners"
	"tsps-go/internal/persist"
	"tsps-go/internal/plugins"
	"tsps-go/internal/protocol"
	"tsps-go/internal/session"
)

// Sailing wiring (phase 21): the cmd/server adapter between the
// game-side sailing engine (internal/game sailing.go,
// boat_manager.go, world_entity_sync.go), the plugin hub's
// world-entity family (internal/plugins sailing.go), the
// listener Deps seams (internal/listeners worldentity.go),
// and the session's world-entity flush half
// (session.WorldEntityFlushSource). It follows the
// skillswiring.go/socialwiring.go precedents: narrow adapters
// here, where all sides are importable. Game never imports
// plugins; listeners never import plugins; the session never
// imports game.
//
// Entry points, wired by the coordinator:
//
//   - InstallSailing, once in buildServer, before ticking
//     starts: creates the BoatManager and Sailing runtimes,
//     installs the world's move_boats hook (BoatManager tick
//     + region refresh under boats), the teleport/after-tick
//     hooks (Sailing.Initialize), the custom-event sink, and
//     the process-wide current-manager/current-Sailing
//     publication. It returns the Sailing runtime: the
//     coordinator keeps it for the login/logout call sites
//     (Sailing.OnLogin/OnLogout) and for content type/dock
//     registration.
//   - installWorldEntityListenerDeps, once per Deps value in
//     handleTurnPacket's Deps construction, next to
//     installSocialListeners: installs the world-entity
//     gate/emit/helm seams onto the listener Deps.
//   - applyEntrySailing, in prepareCarriedSave next to
//     applyEntryRelations: loads the carried save's sailing
//     state onto the entering player.
//   - snapshotSailingJSON, in buildSnapshot next to the
//     relations snapshot: the normalized sailing JSON for the
//     OwnedState.Sailing field.
//
// NIL SAFETY. A nil hub leaves the custom-event sink unset
// (events fire through the subject players' EmitCustomEvent
// seams instead) and the listener emit seam nil. A nil world
// leaves the move_boats hook uninstalled and the manager
// reading no collision (no water anywhere) with a zero spawn;
// the runtimes are still created and published, so tests can
// install sailing without a world. A nil player in the flush
// adapter sends nothing.

// InstallSailing creates the boat runtime for world and
// installs it: the move_boats hook, the teleport and
// after-tick hooks, the custom-event sink, and the world/hub
// installs. Call once at boot, before ticking starts. It
// returns the Sailing runtime for the coordinator's
// login/logout call sites and content registration.
func InstallSailing(hub *plugins.Hub, world *game.World) *game.Sailing {
	var collision game.BoatCollisionSource
	var spawn game.Location
	if world != nil {
		collision = world.RegionManager()
		spawn = world.Spawn()
	}
	manager := game.NewBoatManager(collision)
	sailing := game.NewSailing(manager, spawn)
	if hub != nil {
		sailing.SetEmit(hub.EmitCustomEvent)
	}
	if world != nil {
		world.Hooks.Boats = func(cycle int64) {
			_ = cycle
			// World.ts tick order (move_boats, after
			// process_players, before walk_to_sweep/sync):
			// BoatManager.tick, then the region refresh
			// under boats. The refresh result is a no-op
			// until region activation lands (see
			// RefreshRegionsUnderBoats).
			manager.Tick()
			manager.RefreshRegionsUnderBoats()
		}
		// The world's manager: visibility, movement, and
		// the world-entity sync read it from the World.
		// There is deliberately no process-wide global
		// (see World.SetBoatManager).
		world.SetBoatManager(manager)
	}
	sailing.Initialize()
	if hub != nil {
		hub.SetSailing(sailing)
	}
	return sailing
}

// installWorldEntityListenerDeps wires the world-entity
// gate/emit/helm seams onto a Deps value. The coordinator
// calls it from handleTurnPacket's Deps construction with the
// world's BoatManager (from the installed Sailing) and hub.
// A nil hub leaves WorldEntityEmit nil; a nil manager leaves
// the gate and helm seams nil, so clicks and headings drop
// silently, as with no boats installed.
func installWorldEntityListenerDeps(deps *listeners.Deps, hub *plugins.Hub, manager *game.BoatManager) {
	if manager != nil {
		deps.WorldEntityInteractable = func(viewer *game.Player, entityIndex int) bool {
			return manager.CanInteractWithBoat(&viewer.Mobile, entityIndex)
		}
		deps.SetHelmHeading = func(player *game.Player, packedHeading int) {
			manager.SetHelmHeading(&player.Mobile, packedHeading)
		}
	}
	if hub != nil {
		deps.WorldEntityEmit = func(ev *listeners.WorldEntityInteractionEvent) bool {
			return hub.EmitWorldEntityInteraction(&plugins.WorldEntityInteractionEvent{
				Player:      ev.Player,
				EntityIndex: ev.EntityIndex,
				ClickType:   ev.ClickType,
				Handled:     ev.Handled,
			})
		}
	}
}

// applyEntrySailing loads the carried save's sailing state
// onto the entering player: the enterWorld equivalent of the
// applyToPlayer sailing load (PlayerSave.ts:680, via
// normalizeSailingState). It runs in both prepareCarriedSave
// branches, after applyEntryRelations, reading only the
// sailing field of the (already migrated) save. A missing or
// malformed value loads the empty state, as
// normalizeSailingState(undefined) does.
func applyEntrySailing(p *game.Player, carried *persist.PlayerSave) {
	p.SetSailing(game.ParseSailingStateJSON([]byte(carried.Sailing)))
}

// snapshotSailingJSON returns the live sailing state as
// normalized save JSON for the OwnedState.Sailing snapshot
// field (fromPlayer, PlayerSave.ts:810, via
// normalizeSailingState). A marshal failure fails the
// snapshot, as a non-JSON-compatible live attribute does —
// unreachable for states this package builds (all fields are
// plain JSON values).
func snapshotSailingJSON(p *game.Player) (json.RawMessage, error) {
	return game.MarshalSailingState(*p.GetSailing())
}

// WorldEntityPackets implements session.WorldEntityFlushSource
// for one player: this tick's world-entity packets, encoded
// and in send order (deck scenes, the info packet, spawn
// locs, changed locs). It runs from FlushTick on the tick
// goroutine, without Session.mu held, like SyncView. The XTEA
// provider resolves deck-scene region keys; a nil provider
// yields the all-zero key, as CachePipeline.getXtea does for
// an unknown region.
//
// An encoder error drops that packet: the world-entity
// encoders fail only on oversize payloads, which the bounded
// scene/update counts make unreachable (see
// WorldEntitySync.ts:19-24). A nil owner sends nothing.
func (s *playerFlushSource) WorldEntityPackets(provider session.XteaProvider) [][]byte {
	if s == nil || s.owner == nil || s.worldEntities == nil {
		return nil
	}
	plan := s.worldEntities.Flush(s.owner)
	if plan.IsEmpty() {
		return nil
	}
	var packets [][]byte
	for _, scene := range plan.Scenes {
		key := [4]int32{0, 0, 0, 0}
		if provider != nil {
			key = provider(scene.RegionID)
		}
		encoded, err := protocol.EncodeRebuildWorldEntity(
			scene.EntityIndex, scene.ConfigID, scene.SizeX, scene.SizeZ,
			scene.RegionX, scene.RegionY, scene.TemplateChunks, [][4]int32{key},
		)
		if err != nil {
			continue
		}
		packets = append(packets, encoded)
	}
	if plan.HasInfo {
		updates := make([]protocol.WorldEntityUpdate, 0, len(plan.Updates))
		for _, update := range plan.Updates {
			encoded := protocol.WorldEntityUpdate{UpdateType: protocol.WorldEntityUpdateKind(update.Kind)}
			if update.Delta != nil {
				encoded.Delta = &protocol.WorldEntityPosition{
					X:           update.Delta.X,
					Y:           update.Delta.Y,
					Z:           update.Delta.Z,
					Orientation: update.Delta.Orientation,
				}
			}
			updates = append(updates, encoded)
		}
		spawns := make([]protocol.WorldEntitySpawn, 0, len(plan.Spawns))
		for _, spawn := range plan.Spawns {
			spawns = append(spawns, protocol.WorldEntitySpawn{
				EntityIndex: spawn.EntityIndex,
				SizeX:       spawn.SizeX,
				SizeZ:       spawn.SizeZ,
				ConfigID:    spawn.ConfigID,
				DrawMode:    spawn.DrawMode,
				Position: protocol.WorldEntityPosition{
					X:           spawn.Position.X,
					Y:           spawn.Position.Y,
					Z:           spawn.Position.Z,
					Orientation: spawn.Position.Orientation,
				},
			})
		}
		if encoded, err := protocol.EncodeWorldEntityInfo(updates, spawns); err == nil {
			packets = append(packets, encoded)
		}
		for _, loc := range plan.SpawnLocs {
			packets = append(packets, encodeDeckLocPlan(loc))
		}
	}
	for _, loc := range plan.ChangedLocs {
		packets = append(packets, encodeDeckLocPlan(loc))
	}
	return packets
}

// encodeDeckLocPlan encodes one deck loc (WorldEntitySync.ts:
// encodeDeckLoc): a removed loc sends as LOC_DEL; the rest
// send as LOC_ADD_CHANGE, with the 9-byte opFlags form when
// the loc carries opFlags. The loc level is the template's
// deck plane (loc.Level), as TS sends it; the click tile is
// level 0 (see BoatDeckArea.locLocation).
func encodeDeckLocPlan(plan game.DeckLocPlan) []byte {
	loc := plan.Loc
	if loc.Removed {
		return protocol.EncodeLocDel(plan.X, plan.Y, loc.Level, loc.Shape, loc.Rotation)
	}
	if loc.OpFlags != nil {
		return protocol.EncodeLocAddChangeWithOpFlags(loc.ID, plan.X, plan.Y, loc.Level, loc.Shape, loc.Rotation, *loc.OpFlags)
	}
	return protocol.EncodeLocAddChange(loc.ID, plan.X, plan.Y, loc.Level, loc.Shape, loc.Rotation)
}
