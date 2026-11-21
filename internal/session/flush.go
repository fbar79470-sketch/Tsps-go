package session

import (
	"time"

	"tsps-go/internal/protocol"
)

// This file ports flushClient (PlayerSession.ts:355-660) into
// Session.FlushTick, reduced to the ported snapshot state. The world
// supplies each tick's visibility through FlushSource; this file owns
// the per-viewer sync state (scene bases, PlayerSyncState/NpcSyncState,
// appearance cache, scene replay bookkeeping) and the per-tick frame
// order. Every flushClient behavior that is not ported is marked
// DEFERRED or NOT STARTED at the site where TS performs it, never
// silently skipped; FlushTick's doc comment carries the full list.
//
// Thread-safety: the sync-state fields on Session (sceneBaseX/Y,
// replayedSceneBaseX/Y, replayedSceneLevel, replayedPrivateArea,
// replayedSceneVersion, replayedTemplated, hasReplayedScene,
// playerSyncState, npcSyncState, appearanceCache,
// pendingInitialLocalAppearance, the music state machine, the
// NPC-effects queue, lastGroundItemRegion/Level) are
// tick-goroutine-only — only FlushTick touches them, and World
// calls FlushTick from its flush phase — so Session.mu does not
// guard them. (The ground-item send state and the object send
// state have their own mutexes; see grounditems.go and
// objects.go.) Session.mu guards the packet queue and the flush wiring (syncSource, xteaProvider,
// regionReplacementSender, syncAppearance), which the coordinator glue sets from other
// goroutines. FlushTick snapshots the wiring under mu, releases mu,
// and only then calls FlushSource.SyncView: the adapter reads world
// state (and other sessions' SyncAppearance blocks), and holding mu
// across that call would invert the lock order against world code
// that calls SendPacket while holding a world lock. Snapshot first,
// then lock to queue and flush (SendPacket/Flush lock internally).

// SyncPlayerSnapshot is one visible player for a sync frame: the
// owner or one local player. It carries only the state the Go port
// has; everything createPlayerView reads in TS but Go lacks (update
// flags beyond appearance, equipment, facing, force movement, tint)
// is absent by design — see createPlayerView.
type SyncPlayerSnapshot struct {
	// Identity is the observer appearance-cache identity: the
	// *game.Player pointer, compared with ==. It must hold a
	// comparable value; a nil identity never matches a cached entry
	// whose identity is non-nil. The type is any so this package
	// need not import game.
	Identity any
	Index    int
	X, Y, Z  int
	// DisplayName is the name the appearance block carries (TS:
	// player.getDisplayName()).
	DisplayName string
	// AppearanceBlock is the encoded sync appearance block, or nil
	// for the default fallback (see defaultAppearanceBlock). The
	// adapter reads it from the player's session via
	// SyncAppearance. FlushTick copies it into the appearance
	// cache; callers must treat it as immutable.
	AppearanceBlock []byte
	// NeedsPlacement is player.isNeedsPlacement(): the client must
	// reset the player's path.
	NeedsPlacement bool
	// AppearanceFlag is Flag.APPEARANCE set this tick.
	AppearanceFlag bool
	// WalkingDx/WalkingDy and RunningDx/RunningDy are the step
	// deltas this tick; 0,0 means no step of that kind. They only
	// select the sync movement type (running -> 2, walking -> 1);
	// the encoder derives the actual displacement from tile
	// differences.
	WalkingDx, WalkingDy int
	RunningDx, RunningDy int
	// AudioSettings is the owner's audio-settings snapshot (music
	// mode varp 18, settings varp 19, volumes), read by the music
	// block. Only the owner snapshot carries it; local players'
	// snapshots leave it nil. A nil map reads as the TS defaults
	// (Area mode, loop on).
	AudioSettings map[int]int
	// ForcedChat is this tick's overhead chat text, or nil when
	// the player has none to send (Mobile.forcedChat, included
	// in the update block only when Flag.FORCED_CHAT is set and
	// the text is non-nil — PlayerSession.ts:813-820,
	// createActorUpdates). The flush source populates it; the
	// viewer relations gate (canReceivePublicChatFrom) is NOT
	// applied here: at the Go defaults it degenerates to
	// always-true, and the flush source knows the owner when
	// relations state lands in milestone 9.
	ForcedChat *string
	// ForcedMovement is this tick's forced-movement glide, or nil
	// when the player has none to send (PlayerSession.ts:766-782):
	// present only when Flag.FORCED_MOVEMENT is set this tick AND
	// the player has an active force movement. Start/end deltas
	// are relative to the player's current tile; the cycle offsets
	// are the movement's speed/reverseSpeed; the direction is the
	// client facing ([1024, 1536, 0, 512] over the 0-3 cardinal
	// direction). The flush source populates it from the player's
	// ForceMovement state (see syncsource.go).
	ForcedMovement *protocol.ForcedMovementView
	// ForcedMovementEnd is the glide's landing tile, or nil when
	// the player has no active force movement
	// (PlayerSession.ts:783-789): present whenever a force
	// movement is active, even on ticks where the flag did not
	// fire — the movement pre-pass reads the player's position
	// from it while gliding (sync.go). The flush source
	// populates it alongside ForcedMovement.
	ForcedMovementEnd *protocol.Tile
	// Animation is this tick's animation (Mobile.animation,
	// included in the update block only when Flag.ANIMATION
	// is set and the animation is non-nil —
	// PlayerSession.ts:824-826, createActorUpdates). The
	// flush source populates it from the mobile's
	// PerformAnimation state. A nil animation means no
	// animation block.
	Animation *protocol.AnimationView
	// Graphics are this tick's spotanims (Mobile.graphic
	// plus the slot graphics, included in the update block
	// only when Flag.GRAPHIC is set — PlayerSession.ts:827,
	// createActorUpdates; a cleared slot is id -1, see
	// graphicViews). The flush source populates them from
	// the mobile's PerformGraphic state, slot 0 first, then
	// the other slots in ascending slot order. Nil or empty
	// means no spotanim block.
	Graphics []protocol.GraphicView
	// Hits are this tick's landed hitsplats, in landing order
	// (game.TickHits mapped by the flush source; see hits.go).
	// Nil means no hitsplat block. Health is the hitpoints
	// bar sent with hits (nil = hidden/absent).
	Hits   []HitsplatSnapshot
	Health *HealthSnapshot
	// InteractionIndex is the sync interaction index of the
	// player's current interaction target (PlayerSession.ts:
	// 754, 884-886): -1 for none, the NPC index for an NPC
	// target, the player index + 0x10000 for a player target.
	// The flush source resolves it from the combat interaction
	// target. Nil reads as no interaction (the encoder writes
	// the same 0xffffff target as for -1), so the zero value
	// is safe for snapshots that predate the field.
	InteractionIndex *int
	// SyncArea describes the owner's private-area membership
	// for scene-rebuild decisions (see SyncAreaSnapshot). Only
	// the owner snapshot carries it; local players' snapshots
	// leave it nil. A nil SyncArea means the main world — the
	// adapter applies the syncArea rule (PlayerSession.ts:470,
	// BoatManager.syncArea): nil when the owner has no private
	// area or the area counts as the main world.
	SyncArea *SyncAreaSnapshot
	// MusicTile is the tile the music block reads areas from
	// (PlayerSession.ts:378-380): the templated instance's
	// source tile, else the root-location (sea) tile. Only the
	// owner snapshot carries it. Nil means the scene tile —
	// the adapter leaves it nil when it cannot render a
	// distinct tile (no sailing manager installed, no
	// templated instance), which is identical for a
	// non-aboard player.
	MusicTile *protocol.Tile
}

// TemplatedSceneSnapshot describes one templated instance's
// scene for a rebuild-region packet. It is the session-facing
// form of game.TemplatedInstanceArea's scene description: the
// session package must not import game, so the flush source
// adapts the area into this snapshot (see SyncAreaSnapshot).
type TemplatedSceneSnapshot struct {
	// SceneVersion is the instance's scene version
	// (TemplatedInstanceArea.getSceneVersion): it bumps on
	// every chunk change, invalidating the replayed scene.
	SceneVersion int
	// Build renders the scene palette around a center chunk:
	// the [plane][13][13] chunk palette and the region ids
	// whose XTEA keys the packet carries, in first-seen order
	// (TemplatedInstanceArea.buildScenePalette). The center
	// chunk is the scene tile's chunk (sceneTile >> 3).
	// FlushTick resolves the keys through the XTEA provider.
	// A nil Build degrades to no rebuild packet (the area
	// cannot describe its scene); the adapter must install
	// it for the templated path to send anything.
	Build func(centerChunkX, centerChunkY int) (palette [4][13][13]int32, xteaRegionIDs []int)
}

// SyncAreaSnapshot describes the owner's private-area
// membership for the sceneRebuildFor decision
// (PlayerSession.ts:84-93, 470-491). The adapter builds it
// from the owner's PrivateArea each tick, applying the
// syncArea rule: the snapshot is nil (on the owner snapshot)
// when the owner has no private area or the area counts as
// the main world.
type SyncAreaSnapshot struct {
	// Identity is the private area's identity (the
	// *game.PrivateArea pointer), compared with == against
	// the replayed area. It must hold a comparable value
	// and be non-nil whenever the snapshot exists; a nil
	// identity reads as the main world.
	Identity any
	// Templated is non-nil exactly when the area is a
	// templated instance (TemplatedInstanceArea). A plain
	// private area carries a nil Templated and triggers no
	// rebuild on entry (sceneRebuildFor returns null for
	// it); a templated one triggers a rebuild-region packet.
	Templated *TemplatedSceneSnapshot
}

// SyncNpcSnapshot is one visible NPC for a sync frame, reduced to the
// ported state. See createNpcView for everything the TS npcViews
// mapping reads that Go lacks.
type SyncNpcSnapshot struct {
	// Identity is the NPC object identity (the *game.NPC pointer),
	// compared with == by the NPC-specific-effects send loop
	// (see effects.go). It must hold a comparable value.
	Identity any
	Index    int
	TypeID   int
	X, Y, Z  int
	// FacingDx/FacingDy is the facing step; 0,0 means no facing
	// (clientDirection -> -1). All deltas in this snapshot are unit
	// steps (-1..1) from a Direction, never combined displacements:
	// the running delta is the second step's direction, as TS
	// getRunningDirection supplies it (see clientDirection).
	FacingDx, FacingDy int
	// WalkingDx/WalkingDy and RunningDx/RunningDy are the step
	// deltas this tick; 0,0 means no step of that kind.
	WalkingDx, WalkingDy int
	RunningDx, RunningDy int
	// ForcedChat is this tick's overhead chat text, or nil when
	// the NPC has none to send (same gate as the player
	// snapshot's: flag set and text non-nil). NPCs have no
	// producers yet (combat/NPC content NOT STARTED), so this
	// stays nil until content sets one.
	ForcedChat *string
	// Animation is this tick's animation (Mobile.animation,
	// same gate as the player snapshot's: Flag.ANIMATION set
	// and the animation non-nil). The flush source populates
	// it from the mobile's PerformAnimation state.
	Animation *protocol.AnimationView
	// Graphics are this tick's spotanims (Mobile.graphic plus
	// the slot graphics, same gate as the player snapshot's:
	// Flag.GRAPHIC set; a cleared slot is id -1). The flush
	// source populates them from the mobile's PerformGraphic
	// state, slot 0 first, then the other slots in ascending
	// slot order.
	Graphics []protocol.GraphicView
	// Hits are this tick's landed hitsplats, in landing order
	// (game.TickHits mapped by the flush source; see hits.go).
	// Nil means no hitsplat block. Health is the hitpoints
	// bar sent with hits (nil = hidden/absent).
	Hits   []HitsplatSnapshot
	Health *HealthSnapshot
	// InteractionIndex is the sync interaction index of the
	// NPC's current interaction target (PlayerSession.ts:555,
	// 884-886), with the same -1 / NPC-index / player-index +
	// 0x10000 encoding as the player snapshot's. The flush
	// source resolves it from the NPC interaction target; nil
	// reads as no interaction, as for players.
	InteractionIndex *int
	// FaceTile is the tile the NPC turns to once, when the
	// FACE_POSITION flag is set (PlayerSession.ts:550-552,
	// 567-569: getFaceTile, else the flagged position-to-face).
	// Nil means no face-tile block.
	FaceTile *protocol.FaceTile
}

// FlushSource supplies one tick's sync view: the owner, the local
// players (excluding the owner, in the order TS getLocalPlayers
// returns them), and the local NPCs. It is called from FlushTick
// without Session.mu held, on the tick goroutine. Implementations
// must not mutate the returned snapshots after returning; FlushTick
// copies appearance payloads into its cache.
type FlushSource interface {
	SyncView() (owner SyncPlayerSnapshot, locals []SyncPlayerSnapshot, npcs []SyncNpcSnapshot)
}

// WorldEntityFlushSource is the optional world-entity half of a
// FlushSource: a source that implements it supplies this tick's
// world-entity packets (WorldEntitySync.flush,
// PlayerSession.ts:616), encoded and in send order (deck
// scenes, the info packet, spawn locs, changed locs). FlushTick
// sends them between the tick frame and the player sync
// ("Boats first, so a player on a deck refers to a boat the
// client already has"). The XTEA provider resolves deck-scene
// region keys; a nil provider means all-zero keys, as for the
// map rebuilds. A source that does not implement it sends no
// world-entity packets.
type WorldEntityFlushSource interface {
	WorldEntityPackets(provider XteaProvider) [][]byte
}

// XteaProvider resolves a region id ((mapX << 8) | mapY) to its XTEA
// key, mirroring CachePipeline.getXtea (CachePipeline.ts:62-64),
// which returns [0, 0, 0, 0] for an unknown region. A nil provider
// means all-zero keys.
type XteaProvider func(regionID int) [4]int32

// AttachSyncSource installs the FlushSource FlushTick reads each
// tick. A nil source degrades FlushTick to the queue-only flush it
// performed before this port, preserving existing behavior for
// sessions with no world attached.
func (s *Session) AttachSyncSource(src FlushSource) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.syncSource = src
}

// SetXteaProvider installs the XTEA key provider rebuild packets use.
// A nil provider means all-zero keys, mirroring CachePipeline.getXtea
// on a region with no key.
func (s *Session) SetXteaProvider(p XteaProvider) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.xteaProvider = p
}

// SetRegionReplacementSender installs the hook FlushTick calls
// when the replacement window changes (never replayed, or the
// scene base moved; PlayerSession.ts:492-495): it sends the
// region replacements visible from the given tile (the scene
// center, sceneBase+48) through the game
// MapRegionReplacementManager. The coordinator glue installs it
// per entering player (see cmd/server/instancingwiring.go); a
// nil hook sends nothing, preserving the pre-Phase-21 behavior
// for sessions with no replacement system attached. The hook
// runs on the tick goroutine, without Session.mu held, and must
// frame through SendPacket (which locks internally).
func (s *Session) SetRegionReplacementSender(fn func(tileX, tileY int)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.regionReplacementSender = fn
}

// SetSyncAppearance stores this session's OWN encoded appearance
// block, built at bootstrap from the save (see the coordinator's
// bootstrap hookup). The adapter's FlushSource implementation reads
// OTHER players' blocks through SyncAppearance when building
// snapshots. The payload is copied; a nil payload clears the stored
// block, and snapshots for this player then use the default fallback.
func (s *Session) SetSyncAppearance(payload []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.syncAppearance = append([]byte(nil), payload...)
}

// SyncAppearance returns a copy of the block stored by
// SetSyncAppearance, or nil when none was stored. The copy keeps
// callers from mutating the stored block.
func (s *Session) SyncAppearance() []byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]byte(nil), s.syncAppearance...)
}

// FlushTick is the per-tick flush entry point, porting flush(tick) ->
// flushClient(tick) (PlayerSession.ts:232-234, 355-660) reduced to the
// ported snapshot state. cycle is the world's process cycle: it is
// the tick the TICK packet and both sync packets carry.
//
// Included, in TS order: the channel-open gate; the initial-sync
// hide-ops packets; scene-base init/recentering; scene-change
// detection and the rebuild decision (sceneRebuildFor: the
// templated rebuild-region packet with the scene palette and
// XTEA keys, or the normal rebuild with forceReload when
// leaving an area); the replacement-window send through the
// installed sender hook; the initial player sync on the first
// flush and player/NPC sync views thereafter; the frame order
// tick, player sync, NPC sync, queued via SendPacket and
// flushed; scene-replay bookkeeping (including the replayed
// area, scene version, and templated-ness); the region_updates
// block (ground-item region-change sends and the music state
// machine, see music.go); the object-delta drain before it
// (the perform() fan-out, see objects.go) and the object scene
// snapshots at the two onRegionChange sites (initial: before
// the player sync; non-initial: after the NPC-effects loop,
// before the replay bookkeeping); the NPC-specific-effects take
// at the top and the post-sync send loop (see effects.go).
//
// Omitted, each marked at its site below:
//   - NOT STARTED: the
//     plugin events (network:sync-views 581-586, network:npc-sync
//     644-646, player:sync-view 793) — no plugin system exists.
//     The music block's plugin-owned pieces (music:unlock-track
//     events, the getMusicTrack area override, the
//     music:shuffle-track pick) are likewise absent; shuffle runs
//     the TS fallback and unlocks never happen (see music.go).
//     NPC effects have no producers yet (see effects.go).
//   - Absent by design: the boat rootLocation terms (the scene
//     tile and NPC local tile are the owner tile: no
//     BoatManager.rootLocation in the flush path — see the
//     Phase 21 report for the follow-up; the syncArea rule
//     itself is applied by the flush-source adapter, which
//     reports a nil SyncArea for main-world-counting areas),
//     backpressure guards (the existing Go deviation — see
//     SendPacket), ServerPerf measures (no-op; the phases run
//     inline), recordNetworkPerf (no perf infra). The music
//     block's music tile is the owner snapshot's MusicTile
//     (the templated source tile, else the root-location
//     tile; nil falls back to the scene tile — see
//     syncsource.go and music.go).
//
// With no FlushSource attached, FlushTick degrades to the queue-only
// flush (Flush), preserving the pre-Phase-3 behavior and its tests.
// On a closed channel it returns promptly without sending: Flush
// drops the pending queue and reports ErrSessionClosed when the
// queue was non-empty, exactly as the queue-only path does, which
// World's flushPlayerGuarded treats as a logout request.
func (s *Session) FlushTick(cycle int64) error {
	// Take the queued NPC-specific effects FIRST, before the
	// channel-open gate (PlayerSession.ts:357, before the :359
	// return): undeliverable one-shots are discarded even on a
	// closed channel, never retried. The post-sync loop below
	// sends the survivors. See effects.go.
	specificEffects := s.takeNpcSpecificEffects()

	// Channel-open gate (PlayerSession.ts:359): TS returns early
	// when there is no player or the channel is closed. The Go
	// equivalent of "no player" is "no sync source", handled below;
	// the channel check comes first so a closed channel never
	// triggers a SyncView call. No backpressure guard: the Go port
	// deliberately does not reproduce the OUTBOUND_WS_BUFFER_*
	// guards (see SendPacket).
	if !s.IsOpen() {
		return s.Flush()
	}

	// Snapshot the wiring, then call the source WITHOUT holding
	// Session.mu (see the file header for why).
	s.mu.Lock()
	src := s.syncSource
	provider := s.xteaProvider
	replacementSender := s.regionReplacementSender
	s.mu.Unlock()
	if src == nil {
		return s.Flush()
	}
	owner, locals, npcs := src.SyncView()

	// The tile under the owner. TS distinguishes the main-world
	// scene tile (BoatManager.rootLocation, PlayerSession.ts:369)
	// from the owner's own tile; the root-location scene tile is
	// a known gap (Phase 21 report), so sceneTile == current and
	// a deck player's scene follows the deck tile, not the sea
	// under the boat. The MUSIC tile does use the root location
	// (see MusicTile below).
	current := protocol.Tile{X: owner.X, Y: owner.Y, Level: owner.Z}
	sceneTile := current

	// The object-delta drain (ObjectManager.perform,
	// ObjectManager.ts:326-375): in TS the spawn/despawn fan-out
	// runs synchronously at register/deregister time, during
	// the process phases, so its frames queue before anything
	// flushClient sends; in Go the registry accumulates deltas
	// and they drain here, before the region_updates block,
	// gated on the replayed scene (see objects.go). With no
	// object source installed this sends nothing.
	s.objects.DrainDeltas(cycle)

	// The region_updates block (PlayerSession.ts:375-452): the
	// ground-item region-change send, then the music state
	// machine. The music tile comes from the owner snapshot
	// (MusicTile: the templated instance's source tile, else
	// the root-location tile); nil falls back to the scene
	// tile, identical for a non-aboard player.
	groundItemRegion := ((sceneTile.X >> 6) << 8) | (sceneTile.Y >> 6)
	if groundItemRegion != s.lastGroundItemRegion || current.Level != s.lastGroundItemLevel {
		s.lastGroundItemRegion = groundItemRegion
		s.lastGroundItemLevel = current.Level
		// ItemOnGroundManager.onRegionChange
		// (PlayerSession.ts:384-390): the full visible
		// snapshot, sent through the per-session source the
		// adapter installs. With no source installed this
		// sends nothing.
		s.groundItems.SendVisibleGroundItems()
	}
	musicTile := sceneTile
	if owner.MusicTile != nil {
		musicTile = *owner.MusicTile
	}
	s.updateMusic(cycle, musicTile, owner.AudioSettings)

	initialSync := s.playerSyncState == nil
	if initialSync {
		// Live sends hidenpcops=false and hideobjops=false at
		// login, so a world switch cannot inherit the previous
		// world's cutscene state (PlayerSession.ts:460-462).
		s.SendPacket(protocol.EncodeHideNpcOps(false))
		s.SendPacket(protocol.EncodeHideObjOps(false))
		s.sceneBaseX = max(0, (sceneTile.X-48)&^7)
		s.sceneBaseY = max(0, (sceneTile.Y-48)&^7)
	} else {
		// Recenter when the owner leaves the 16..88 local window
		// (PlayerSession.ts:464-469).
		if localX := sceneTile.X - s.sceneBaseX; localX < 16 || localX >= 88 {
			s.sceneBaseX = max(0, (sceneTile.X-48)&^7)
		}
		if localY := sceneTile.Y - s.sceneBaseY; localY < 16 || localY >= 88 {
			s.sceneBaseY = max(0, (sceneTile.Y-48)&^7)
		}
	}

	// The private area and scene version (PlayerSession.ts:470-473):
	// the adapter has already applied the syncArea rule, so a nil
	// SyncArea means the main world (no area, or an area that
	// counts as the main world). The scene version is the
	// templated instance's version, or 0 for the main world and
	// plain private areas (TS: templated?.getSceneVersion() ?? 0).
	var privateArea any
	var templated *TemplatedSceneSnapshot
	sceneVersion := 0
	if owner.SyncArea != nil {
		privateArea = owner.SyncArea.Identity
		templated = owner.SyncArea.Templated
		if templated != nil {
			sceneVersion = templated.SceneVersion
		}
	}
	// sceneChanged (PlayerSession.ts:475-479): the replay/base/
	// level terms plus the replayed-area and scene-version terms.
	// An area change or a scene-version bump replays the scene
	// bookkeeping even when the base and level are unchanged.
	sceneChanged := !s.hasReplayedScene ||
		s.replayedSceneBaseX != s.sceneBaseX ||
		s.replayedSceneBaseY != s.sceneBaseY ||
		s.replayedSceneLevel != sceneTile.Level ||
		s.replayedPrivateArea != privateArea ||
		s.replayedSceneVersion != sceneVersion
	// sceneRebuildFor (PlayerSession.ts:80-91, called at 481-487)
	// receives a change flag WITHOUT the level term: a pure plane
	// change replays the scene bookkeeping but needs no map
	// rebuild. The decision: no change -> no rebuild; a
	// templated instance -> "region"; no area, or a replayed
	// scene that was templated -> "normal"; a plain private area
	// -> no rebuild (its map is the main-world map).
	rebuildChange := !s.hasReplayedScene ||
		s.replayedSceneBaseX != s.sceneBaseX ||
		s.replayedSceneBaseY != s.sceneBaseY ||
		s.replayedPrivateArea != privateArea ||
		s.replayedSceneVersion != sceneVersion
	rebuildRegion := false
	rebuildNormal := false
	if rebuildChange {
		switch {
		case templated != nil:
			rebuildRegion = true
		case privateArea == nil || s.replayedTemplated:
			rebuildNormal = true
		}
	}
	if rebuildRegion {
		// TemplatedInstanceArea.encodeScene
		// (TemplatedInstanceArea.ts, via PlayerSession.ts:484):
		// the scene palette around the center chunk, with XTEA
		// keys resolved in the palette's region-id order, sent
		// as REBUILD_REGION with forceReload. A nil Build
		// degrades to no packet (see TemplatedSceneSnapshot).
		if templated.Build != nil {
			centerChunkX, centerChunkY := sceneTile.X>>3, sceneTile.Y>>3
			palette, regionIDs := templated.Build(centerChunkX, centerChunkY)
			keys := make([][4]int32, 0, len(regionIDs))
			for _, regionID := range regionIDs {
				var key [4]int32
				if provider != nil {
					key = provider(regionID)
				}
				keys = append(keys, key)
			}
			packet, err := protocol.EncodeRebuildRegion(uint16(centerChunkX), uint16(centerChunkY), true, palette, keys)
			if err != nil {
				return err
			}
			s.SendPacket(packet)
		}
		// TS returns early when the rebuild packet cannot be
		// queued (PlayerSession.ts:488-491). SendPacket drops
		// silently on a closed channel, so re-check openness
		// here instead of building sync views for nobody.
		if !s.IsOpen() {
			return s.Flush()
		}
	} else if rebuildNormal {
		// Leaving an area (or entering the main world) reloads
		// the map: forceReload is replayedPrivateArea != null
		// (PlayerSession.ts:486).
		if err := s.sendRebuildNormal(sceneTile.X>>3, sceneTile.Y>>3, provider, s.replayedPrivateArea != nil); err != nil {
			return err
		}
		if !s.IsOpen() {
			return s.Flush()
		}
	}

	// When the replacement window changes (never replayed, or the
	// scene base moved; PlayerSession.ts:492-495),
	// MapRegionReplacementManager.sendVisibleReplacementsToPlayer
	// sends the visible region replacements (497-505). The
	// installed sender hook performs that send from the scene
	// center; with no hook installed nothing streams.
	if replacementSender != nil &&
		(!s.hasReplayedScene ||
			s.replayedSceneBaseX != s.sceneBaseX ||
			s.replayedSceneBaseY != s.sceneBaseY) {
		replacementSender(s.sceneBaseX+48, s.sceneBaseY+48)
	}

	// ObjectManager.onRegionChange on the initial sync when the
	// scene changed (PlayerSession.ts:509-513): the full scene
	// snapshot, after the rebuild and the replacements, before
	// the player sync is built. The channel re-check mirrors
	// the rebuild site above (TS returns early at :512 when the
	// channel closed). See objects.go.
	if initialSync && sceneChanged {
		s.objects.OnRegionChange(s.sceneBaseX, s.sceneBaseY)
		if !s.IsOpen() {
			return s.Flush()
		}
	}

	var playerSync []byte
	var playerViews []protocol.PlayerView
	if initialSync {
		playerSync = protocol.EncodeInitialPlayerSync(owner.Index, current.X, current.Y, current.Level, int(cycle))
		s.playerSyncState = protocol.CreatePlayerSyncState(owner.Index, current)
	} else {
		playerViews = make([]protocol.PlayerView, 0, len(locals)+1)
		playerViews = append(playerViews, s.createPlayerView(owner, s.pendingInitialLocalAppearance))
		for _, local := range locals {
			playerViews = append(playerViews, s.createPlayerView(local, false))
		}
		s.pendingInitialLocalAppearance = false
		playerSync = protocol.EncodePlayerSync(owner.Index, s.sceneBaseX, s.sceneBaseY, int(cycle), playerViews, s.playerSyncState)
	}

	npcViews := make([]protocol.NpcView, 0, len(npcs))
	for _, npc := range npcs {
		npcViews = append(npcViews, createNpcView(npc))
	}

	// NOT STARTED: the "network:sync-views" plugin event
	// (PlayerSession.ts:581-586), emitted only on non-initial ticks
	// with exactly the views prepared for this session. No plugin
	// system exists in Go. (playerViews is nil on the initial tick,
	// matching TS's undefined observerViews.)

	// NPCs live in the main world, so a player on a deck sees them
	// from the tile under them (PlayerSession.ts:583-596). With no
	// boats and no force movement, npcLocal degenerates to the
	// owner's tile.
	npcLocal := current
	npcSync := protocol.EncodeNpcSync(int(cycle), npcLocal, npcViews, s.npcSyncState,
		nil,                                     // deckLocal: nil — the player never stands on a boat deck.
		func(int) *protocol.Tile { return nil }, // deckBase: no boats to look up.
		false,                                   // crossFloor: CrossFloor.enabled() has no Go equivalent; main-plane visibility only.
	)

	// Socket send (PlayerSession.ts:611-626): the tick frame, then
	// the world-entity packets, then the player sync, then the
	// NPC sync. The world-entity packets come from the flush
	// source when it implements WorldEntityFlushSource
	// (WorldEntitySync.flush, PlayerSession.ts:616); a source
	// without it sends none. recordNetworkPerf (623) is omitted:
	// no perf infra exists. ServerPerf.measurePhase wrappers
	// elsewhere in flushClient are likewise no-ops; the phases
	// run inline, in TS order. EncodeTick's serverTime is
	// Date.now() truncated to int32 (encodeTick writes
	// writeInt32BE(serverTime | 0), ClientProtocol.ts:2746-2750);
	// int32(time.Now().UnixMilli()) wraps identically.
	s.SendPacket(protocol.EncodeTick(int32(cycle), int32(time.Now().UnixMilli())))
	if wes, ok := src.(WorldEntityFlushSource); ok {
		for _, packet := range wes.WorldEntityPackets(provider) {
			s.SendPacket(packet)
		}
	}
	s.SendPacket(playerSync)
	s.SendPacket(npcSync)

	// The NPC-specific-effects loop (PlayerSession.ts:636-639):
	// taken effects send only to NPCs visible and encoded this
	// tick. Early returns above skip the loop, matching TS
	// (syncSent false returns at :630). See effects.go.
	s.sendNpcSpecificEffects(specificEffects, npcs)

	// NOT STARTED: the "network:npc-sync" plugin event
	// (PlayerSession.ts:644-646). No plugin system exists in Go.

	// ObjectManager.onRegionChange on non-initial ticks when the
	// scene changed (PlayerSession.ts:648-650): the full scene
	// snapshot, after the sync frames and the NPC-effects loop,
	// before the replay bookkeeping below. See objects.go.
	if !initialSync && sceneChanged {
		s.objects.OnRegionChange(s.sceneBaseX, s.sceneBaseY)
		if !s.IsOpen() {
			return s.Flush()
		}
	}

	if sceneChanged {
		s.replayedSceneBaseX = s.sceneBaseX
		s.replayedSceneBaseY = s.sceneBaseY
		s.replayedSceneLevel = sceneTile.Level
		s.replayedPrivateArea = privateArea
		s.replayedSceneVersion = sceneVersion
		s.replayedTemplated = templated != nil
		s.hasReplayedScene = true
	}
	return s.Flush()
}

// sendRebuildNormal ports PlayerSession.sendRebuildNormal
// (PlayerSession.ts:265-290): the XTEA keys for the map regions
// around the center chunk, enumerated mapX-outer/mapY-inner, then
// the REBUILD_NORMAL packet. Go integer division truncates toward
// zero, matching Math.trunc. forceReload is replayedPrivateArea !=
// null at the call site (PlayerSession.ts:486): leaving an area
// reloads the map. EncodeRebuildNormal can only fail if the payload
// exceeded 64 KiB, which the bounded key count (at most 16 regions)
// makes unreachable; an error is still returned rather than
// swallowed.
func (s *Session) sendRebuildNormal(centerChunkX, centerChunkY int, provider XteaProvider, forceReload bool) error {
	minMapX := (centerChunkX - 6) / 8
	maxMapX := (centerChunkX + 6) / 8
	minMapY := (centerChunkY - 6) / 8
	maxMapY := (centerChunkY + 6) / 8
	keys := make([][4]int32, 0, (maxMapX-minMapX+1)*(maxMapY-minMapY+1))
	for mapX := minMapX; mapX <= maxMapX; mapX++ {
		for mapY := minMapY; mapY <= maxMapY; mapY++ {
			// The region id (mapX << 8) | mapY matches the TS
			// bitwise computation for these magnitudes; a nil
			// provider yields the all-zero key, as
			// CachePipeline.getXtea does for an unknown region.
			var key [4]int32
			if provider != nil {
				key = provider((mapX << 8) | mapY)
			}
			keys = append(keys, key)
		}
	}
	packet, err := protocol.EncodeRebuildNormal(uint16(centerChunkX), uint16(centerChunkY), forceReload, keys)
	if err != nil {
		return err
	}
	s.SendPacket(packet)
	return nil
}

// createPlayerView ports PlayerSession.createPlayerView
// (PlayerSession.ts:662-795) reduced to the ported snapshot state.
// The appearance-cache rule is TS's: a cached payload is reused only
// when the cached identity is the same player object and the view is
// not dirty; otherwise the snapshot's block is cached, or the
// default fallback block is built and cached when the snapshot
// carries none.
//
// Absent actor updates (createActorUpdates, PlayerSession.ts:809-831):
//   - forcedChat: no chat/relations system, so no forced chat is
//     ever received.
//   - animation/graphics: ported in Phase 29 (mobile_animation.go):
//     Animation carries the snapshot's animation (flag-gated by
//     the flush source) and Graphics its spotanims (slot 0 first,
//     then the other slots; a cleared slot is id -1).
//   - hits/health: ported in Phase 16 (hits.go): Hits carries
//     the tick's landed hitsplats and Health the hitpoints
//     bar, both populated by the flush source from
//     game.TickHits / HitpointsOf. Nil Hits means no
//     hitsplat block (an empty non-nil slice would still set
//     the HIT mask bit); Health is sent only with hits.
//   - The flag-gated interactionIndex from createActorUpdates is
//     overridden in TS by the live interacting target on every view
//     (PlayerSession.ts:743); here the snapshot's
//     InteractionIndex carries the resolved target (nil reads
//     as no interaction).
//
// Also absent: faceDirection (players carry no face-position
// state), forcedMovement/forcedMovementEnd (no
// force-movement state), tint (no tint state), worldView (no
// boats), finePosition (no smooth-movement state).
//
// movementType follows TS exactly (PlayerSession.ts:757-759): a
// running step present means 2, a walking step means 1, and no step
// means absent — 0 in Go, which EncodePlayerSync treats as absent
// and derives from the step distance, matching the TS
// `movementType ?? (distance === 2 ? 2 : 1)`.
func (s *Session) createPlayerView(snap SyncPlayerSnapshot, forceAppearance bool) protocol.PlayerView {
	known := false
	if s.playerSyncState != nil {
		_, known = s.playerSyncState.LastTiles[snap.Index]
	}
	// dirty = forceAppearance || !lastTiles.has(index) ||
	// Flag.APPEARANCE (PlayerSession.ts:666-668). A nil sync state
	// reads as unknown, matching the TS `?.` chain.
	dirty := forceAppearance || !known || snap.AppearanceFlag

	var payload []byte
	if cached, ok := s.appearanceCache[snap.Index]; ok && cached.identity == snap.Identity && !dirty {
		payload = cached.payload
	}
	if payload == nil {
		if snap.AppearanceBlock != nil {
			payload = append([]byte(nil), snap.AppearanceBlock...)
		} else {
			payload = defaultAppearanceBlock(snap.DisplayName)
		}
		s.appearanceCache[snap.Index] = struct {
			identity any
			payload  []byte
		}{identity: snap.Identity, payload: payload}
	}

	movementType := 0
	if snap.RunningDx != 0 || snap.RunningDy != 0 {
		movementType = 2
	} else if snap.WalkingDx != 0 || snap.WalkingDy != 0 {
		movementType = 1
	}

	// The view always carries a non-nil interaction pointer: a
	// nil snapshot field reads as -1 (no interaction), matching
	// the pre-Phase-16 behavior of always sending the
	// no-interaction target.
	interaction := -1
	if snap.InteractionIndex != nil {
		interaction = *snap.InteractionIndex
	}
	view := protocol.PlayerView{
		Tile:              protocol.Tile{X: snap.X, Y: snap.Y, Level: snap.Z},
		ActorUpdateView:   protocol.ActorUpdateView{InteractionIndex: &interaction, ForcedChat: snap.ForcedChat, Animation: snap.Animation, Graphics: snap.Graphics},
		Index:             snap.Index,
		Appearance:        payload,
		MovementType:      movementType,
		ResetPath:         snap.NeedsPlacement,
		AppearanceDirty:   dirty,
		ForcedMovement:    snap.ForcedMovement,
		ForcedMovementEnd: snap.ForcedMovementEnd,
	}
	if len(snap.Hits) > 0 {
		view.Hits = hitsplatViews(snap.Hits)
		view.Health = healthView(snap.Health)
	}
	// NOT STARTED: the "player:sync-view" plugin event
	// (PlayerSession.ts:793), which lets content mutate the view.
	// No plugin system exists in Go.
	return view
}

// createNpcView ports the npcViews mapping in flushClient
// (PlayerSession.ts:546-573) reduced to the ported snapshot state.
// Absent: headIcon (no head-icon state), exactMove (no NPC glide
// state), bars (no headbar state), crawl (no crawl state),
// modelCustomisation / baseAnimations / transparency / tint (no
// per-NPC visual overrides), worldView (no boats).
// Animation/graphics are ported in Phase 29 (mobile_animation.go):
// Animation carries the snapshot's animation (flag-gated by the
// flush source) and Graphics its spotanims (slot 0 first, then the
// other slots; a cleared slot is id -1).
// WalkDirection and RunDirection are -1 when the NPC does not step,
// matching the NpcView contract in sync.go.
func createNpcView(snap SyncNpcSnapshot) protocol.NpcView {
	// As for players: a nil snapshot field reads as -1, and the
	// view always carries a non-nil pointer.
	interaction := -1
	if snap.InteractionIndex != nil {
		interaction = *snap.InteractionIndex
	}
	view := protocol.NpcView{
		Tile:            protocol.Tile{X: snap.X, Y: snap.Y, Level: snap.Z},
		ActorUpdateView: protocol.ActorUpdateView{InteractionIndex: &interaction, ForcedChat: snap.ForcedChat, Animation: snap.Animation, Graphics: snap.Graphics},
		Index:           snap.Index,
		TypeID:          snap.TypeID,
		Rotation:        clientDirection(snap.FacingDx, snap.FacingDy),
		WalkDirection:   clientDirection(snap.WalkingDx, snap.WalkingDy),
		RunDirection:    clientDirection(snap.RunningDx, snap.RunningDy),
		FaceTile:        snap.FaceTile,
	}
	if len(snap.Hits) > 0 {
		view.Hits = hitsplatViews(snap.Hits)
		view.Health = healthView(snap.Health)
	}
	return view
}

// interactionIndex mirrors PlayerSession.interactionIndex(nil)
// (PlayerSession.ts:884-886): with no interacting mobiles in Go, the
// target is always nil, so the interacting-entity field is always
// -1 (no target). The sync encoders write 0xffffff when the value
// changes from the last sent one.
// clientDirection ports PlayerSession.clientDirection
// (PlayerSession.ts:894-901): the 8-way step direction in client
// order, -1 for no step. dx/dy are unit steps (-1..1), the Direction
// domain TS feeds it (getWalkingDirection/getRunningDirection return
// Directions, and (0,0) is NONE); a lookup outside the table reads as
// -1, matching the TS `?? -1` on an out-of-range index. Callers must
// pass single-step directions, not combined displacements: a running
// NPC's delta is its second step's direction, as TS supplies it.
func clientDirection(dx, dy int) int {
	if dx == 0 && dy == 0 {
		return -1
	}
	index := (dy+1)*3 + dx + 1
	if index < 0 || index >= 9 {
		return -1
	}
	return [...]int{0, 1, 2, 3, -1, 4, 5, 6, 7}[index]
}

// defaultSyncAnimations is the no-weapon/no-skill/no-render/
// no-transformation animation array from createPlayerView
// (PlayerSession.ts:688-696): the weapon stand/walk/run `??`
// defaults (808/819/824) with the fixed turn animations.
// Equipment-driven animations need the equipment system (NOT
// STARTED); until then every fallback block uses these.
var defaultSyncAnimations = []int{808, 823, 819, 820, 821, 822, 824}

// defaultAppearanceBlock builds the appearance block used when a sync
// snapshot carries no encoded block: the new-character look and skill
// levels from the bootstrap helpers over a nil save (a sane default:
// the default male look, total level 33, combat level 3), with the
// default animation array. It mirrors the createPlayerView fallback
// branch, which reaches the same defaults when no skill animation,
// render animations, transformation, or weapon definition applies.
func defaultAppearanceBlock(displayName string) []byte {
	_, totalLevel, combatLevel := bootstrapSkills(nil)
	return protocol.EncodePlayerAppearance(*bootstrapAppearance(nil), displayName, combatLevel, totalLevel, defaultSyncAnimations)
}
