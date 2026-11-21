package main

import (
	"sort"

	"tsps-go/internal/game"
	"tsps-go/internal/protocol"
	"tsps-go/internal/session"
)

// playerFlushSource implements session.FlushSource for one player: it
// snapshots the owner's tile and the per-tick local lists World
// maintains (phase 13, rebuild_update_buckets + update_players_npcs)
// into the session package's snapshot types, so Session.FlushTick can
// build the sync frame without the session package importing game.
//
// SyncView is called from FlushTick on the tick goroutine, without
// Session.mu held. It reads tick-goroutine-only Player state (Loc,
// Flags, step directions, local lists) plus each visible player's
// stored appearance block through the mutex-guarded
// Session.SyncAppearance. Local lists are re-fetched on every call:
// UpdateLocal* reslices them each tick, so a retained slice would go
// stale (see game.Player.LocalPlayers). The worldAdapter attaches one
// source per entering player in AddPlayer, before queueing the
// player for addition, so the player's first FlushTick already has a
// view to send; no detach step exists because removed players are
// never flushed again and a closed session short-circuits FlushTick.
type playerFlushSource struct {
	owner *game.Player
	// boats is the world's boat manager, or nil in worlds
	// without sailing: the music tile reads the root location
	// through it. worldEntities is the world's per-viewer
	// boat sync state, or nil: WorldEntityPackets reads it.
	// Both are installed by AddPlayer from the world's
	// installs; sources built directly in tests leave them
	// nil and degenerate to the no-boats behavior.
	boats         *game.BoatManager
	worldEntities *game.WorldEntitySync
}

// SyncView returns the owner's snapshot, the local players
// (excluding the owner, in local-list order), and the local NPCs, in
// the order TS flushClient consumes them: [player,
// ...getLocalPlayers()] for the player sync, getLocalNpcs() for the
// NPC sync (PlayerSession.ts:526-527, 546).
func (s *playerFlushSource) SyncView() (session.SyncPlayerSnapshot, []session.SyncPlayerSnapshot, []session.SyncNpcSnapshot) {
	owner := playerSnapshot(s.owner, s.owner)
	// Only the owner snapshot carries audio settings: the music
	// block reads the owner's settings (PlayerSession.ts:382),
	// and local players' settings are never read.
	owner.AudioSettings = s.owner.GetAudioSettings()
	// Private-area scene + music tiles (PlayerSession.ts:369-380,
	// 470-491). The SyncArea snapshot applies the syncArea rule
	// (BoatManager.syncArea: nil for no area or a
	// main-world-counting area, e.g. a boat deck); the music
	// tile is the templated instance's source tile, else the
	// root-location (sea) tile. Only the owner snapshot carries
	// them.
	if pa := s.owner.PrivateArea(); pa != nil && !pa.CountsAsMainWorld() {
		snap := &session.SyncAreaSnapshot{Identity: pa}
		if tia := pa.Templated(); tia != nil {
			snap.Templated = &session.TemplatedSceneSnapshot{
				SceneVersion: tia.GetSceneVersion(),
				Build: func(centerChunkX, centerChunkY int) ([4][13][13]int32, []int) {
					return tia.BuildScenePalette(centerChunkX, centerChunkY)
				},
			}
		}
		owner.SyncArea = snap
	}
	owner.MusicTile = musicTileFor(s.owner, s.boats)
	locals := make([]session.SyncPlayerSnapshot, 0, len(s.owner.LocalPlayers()))
	for _, p := range s.owner.LocalPlayers() {
		if p == nil {
			// Cannot occur via production paths (local lists
			// never hold nils); skip defensively so a corrupt
			// list degrades to a missing actor instead of a
			// flush-phase panic, which would log the owner
			// out.
			continue
		}
		locals = append(locals, playerSnapshot(s.owner, p))
	}
	npcs := make([]session.SyncNpcSnapshot, 0, len(s.owner.LocalNPCs()))
	for _, n := range s.owner.LocalNPCs() {
		if n == nil {
			continue
		}
		npcs = append(npcs, npcSnapshot(s.owner, n))
	}
	return owner, locals, npcs
}

// musicTileFor renders the owner's music tile
// (PlayerSession.ts:378-380): a templated instance plays the
// music of the cache-map tiles it copies (instanceSourceTile);
// anything else plays the root-location tile (the sea under a
// boat deck, the owner's own tile otherwise). It returns nil
// when no boat manager is installed and the owner is in no
// templated instance, in which case FlushTick falls back to the
// scene tile (identical for a non-aboard player).
func musicTileFor(owner *game.Player, boats *game.BoatManager) *protocol.Tile {
	loc := owner.Location()
	if pa := owner.PrivateArea(); pa != nil {
		if tia := pa.Templated(); tia != nil {
			if sx, sy, sp, ok := tia.InstanceSourceTile(loc.X, loc.Y, loc.Z); ok {
				return &protocol.Tile{X: sx, Y: sy, Level: sp}
			}
			// Outside the instance's chunks: TS reads
			// NO_TRACKS and keeps the previous area; the
			// instance tile reads no music area either
			// (instance scenes sit outside the music-area
			// map), so the empty-tracks rule keeps the
			// previous area the same way.
			return &protocol.Tile{X: loc.X, Y: loc.Y, Level: loc.Z}
		}
	}
	if boats != nil {
		root := boats.RootLocation(&owner.Mobile)
		return &protocol.Tile{X: root.X, Y: root.Y, Level: root.Z}
	}
	return nil
}

// playerSnapshot renders one player as a sync snapshot. The identity
// is the *game.Player pointer itself: FlushTick compares it with ==
// against its appearance cache, mirroring TS's
// `cached?.player === player` check (PlayerSession.ts:669).
// viewer is the observing player: hitsplat types and the health
// bar are viewer-relative (see hitSnapshots).
func playerSnapshot(viewer, p *game.Player) session.SyncPlayerSnapshot {
	block := appearanceBlock(p)
	hits, health := hitSnapshots(viewer, &p.Mobile, p == viewer)
	interaction := game.InteractionIndexFor(p.GetCombat().Interacting())
	anim := animationView(&p.Mobile)
	forcedMovement, forcedMovementEnd := forcedMovementViews(p, &anim)
	return session.SyncPlayerSnapshot{
		Identity:        p,
		Index:           p.Index,
		X:               p.Loc.X,
		Y:               p.Loc.Y,
		Z:               p.Loc.Z,
		DisplayName:     p.GetDisplayName(),
		AppearanceBlock: block,
		NeedsPlacement:  p.NeedsPlacement,
		// AppearanceFlag is the live Flag.APPEARANCE, plus a
		// bootstrap-race guard: while a player has no stored
		// appearance block (bootstrap has not run yet — the
		// player is registered and visible from the same tick
		// it is added, but sendBootstrapLocked runs on the
		// connection goroutine), every snapshot is dirty, so
		// observers recompute the payload each tick and pick
		// up the real block on the tick after it is stored
		// instead of caching the default fallback forever.
		// Once the block exists the flag is purely the live
		// update flag again. Players with no session at all
		// (bots) recompute the fallback every tick; that is
		// correct, if slightly wasteful, and there are no
		// bots in production yet.
		AppearanceFlag:    p.Flags.Has(game.FlagAppearance) || block == nil,
		WalkingDx:         p.WalkingDirection.Dx(),
		WalkingDy:         p.WalkingDirection.Dy(),
		RunningDx:         p.RunningDirection.Dx(),
		RunningDy:         p.RunningDirection.Dy(),
		ForcedChat:        forcedChatText(&p.Mobile),
		Animation:         anim,
		ForcedMovement:    forcedMovement,
		ForcedMovementEnd: forcedMovementEnd,
		Graphics:          graphicViews(&p.Mobile),
		Hits:              hits,
		Health:            health,
		InteractionIndex:  &interaction,
	}
}

// forcedMovementViews renders the player's forced-movement sync state
// (PlayerSession.ts:742-789): the landing tile whenever a force
// movement is active, the glide view only when Flag.FORCED_MOVEMENT
// fired this tick, and the movement's animation as the animation view
// when no animation is already set. anim is the already-computed
// animation view; it is replaced (not appended to) by the movement's
// animation, exactly as TS assigns updates.animation from the force
// movement. A nil return for both values means no active force
// movement.
func forcedMovementViews(p *game.Player, anim **protocol.AnimationView) (*protocol.ForcedMovementView, *protocol.Tile) {
	fm := p.ForceMovement()
	if fm == nil {
		return nil, nil
	}
	// The landing tile, present whenever a force movement is active
	// (PlayerSession.ts:783-789): start + delta, same level.
	end := &protocol.Tile{X: fm.Start.X + fm.Delta.X, Y: fm.Start.Y + fm.Delta.Y, Level: p.Loc.Z}
	if !p.Flags.Has(game.FlagForcedMovement) {
		return nil, end
	}
	loc := p.Loc
	view := &protocol.ForcedMovementView{
		StartDeltaX: fm.Start.X - loc.X,
		StartDeltaY: fm.Start.Y - loc.Y,
		// The end tile from the current one, as the start is: a slide
		// whose player is already on its end tile (moved there the
		// same tick, as live jumps a stepping stone) ends at 0.
		EndDeltaX:        end.X - loc.X,
		EndDeltaY:        end.Y - loc.Y,
		StartCycleOffset: fm.Speed,
		EndCycleOffset:   fm.ReverseSpeed,
		Direction:        forcedMovementDirection(fm.Direction),
	}
	if *anim == nil && fm.Animation > 0 {
		*anim = &protocol.AnimationView{ID: fm.Animation, Delay: 0}
	}
	return view, end
}

// forcedMovementDirection maps a ForceMovement direction (0 north,
// 1 east, 2 south, 3 west) to the client facing (PlayerSession.ts:777:
// [1024, 1536, 0, 512][direction] ?? direction).
func forcedMovementDirection(d int) int {
	if d >= 0 && d < 4 {
		return [...]int{1024, 1536, 0, 512}[d]
	}
	return d
}

// npcSnapshot renders one NPC as a sync snapshot. Facing is the
// closest ported state to TS npc.getFace()?.getDirection() (the
// rotation in the npcViews mapping, PlayerSession.ts:563): the
// dedicated Face object is NOT STARTED, so the last faced/stepped
// direction stands in — the interaction/facing code keeps Facing
// current on interaction and face-position changes (see
// npc_interaction.go). Step deltas are unit steps from Direction,
// never combined displacements, as clientDirection requires (see
// flush.go). viewer is the observing player (see hitSnapshots).
func npcSnapshot(viewer *game.Player, n *game.NPC) session.SyncNpcSnapshot {
	hits, health := hitSnapshots(viewer, &n.Mobile, false)
	interaction := n.InteractionIndex()
	snap := session.SyncNpcSnapshot{
		Identity:         n,
		Index:            n.Index,
		TypeID:           n.ContentID(),
		X:                n.Loc.X,
		Y:                n.Loc.Y,
		Z:                n.Loc.Z,
		FacingDx:         n.Facing.Dx(),
		FacingDy:         n.Facing.Dy(),
		WalkingDx:        n.WalkingDirection.Dx(),
		WalkingDy:        n.WalkingDirection.Dy(),
		RunningDx:        n.RunningDirection.Dx(),
		RunningDy:        n.RunningDirection.Dy(),
		ForcedChat:       forcedChatText(&n.Mobile),
		Animation:        animationView(&n.Mobile),
		Graphics:         graphicViews(&n.Mobile),
		Hits:             hits,
		Health:           health,
		InteractionIndex: &interaction,
	}
	// FaceTile: the FACE_POSITION-flagged position-to-face
	// (PlayerSession.ts:550-552, 567-569). The getFaceTile()
	// half has no Go state; only the flagged position is sent.
	if n.Flags.Has(game.FlagFacePosition) {
		if pos, ok := n.PositionToFace(); ok {
			snap.FaceTile = &protocol.FaceTile{X: pos.X, Y: pos.Y}
		}
	}
	return snap
}

// hitSnapshots maps this tick's landed hits on m to the
// viewer-relative hitsplat snapshots plus the health bar
// (PlayerSession.ts createActorUpdates / hitView /
// maxHitsplatType, via session/hits.go). onMe reports whether
// m is the viewer. A nil Hits slice means no hitsplat block:
// the slice stays nil when no hits landed (an empty non-nil
// slice would still set the HIT mask bit). Health rides with
// hits whenever hits exist: no health-bar-hidden state exists
// in Go, so the bar is never hidden (the TS
// isHealthBarHidden gate degenerates to always-false).
// MaxHitThreshold is 0: the flush source has no varbit reader
// wired (the varp store is per-session; see session/hits.go),
// and 0 reads as "any max hit qualifies", matching the TS
// getVarbit default.
func hitSnapshots(viewer *game.Player, m *game.Mobile, onMe bool) ([]session.HitsplatSnapshot, *session.HealthSnapshot) {
	tickHits := m.GetCombat().TickHits()
	if len(tickHits) == 0 {
		return nil, nil
	}
	var viewerMobile *game.Mobile
	if viewer != nil {
		viewerMobile = &viewer.Mobile
	}
	hits := make([]session.HitsplatSnapshot, 0, len(tickHits))
	for _, hit := range tickHits {
		hits = append(hits, session.HitsplatSnapshot{
			Mask:            int(hit.Mask),
			Damage:          hit.Damage,
			OnMe:            onMe,
			SourceIsViewer:  hit.Source != nil && hit.Source == viewerMobile,
			IsMaxHit:        hit.RolledMaximum > 0 && hit.Damage == hit.RolledMaximum,
			MaxHitThreshold: 0,
			SplatMine:       hit.SplatMine,
			SplatOthers:     hit.SplatOthers,
		})
	}
	current, max := game.HitpointsOf(m)
	return hits, &session.HealthSnapshot{Current: current, Max: max}
}

// animationView returns this tick's animation view, or nil
// (createActorUpdates, PlayerSession.ts:824-826): only when
// Flag.ANIMATION is set and the mobile has an animation.
// The flag gate matters: an animation cleared by
// ResetUpdating leaves the flag cleared too, so a stale
// animation is never re-sent.
func animationView(m *game.Mobile) *protocol.AnimationView {
	if m == nil || !m.Flags.Has(game.FlagAnimation) {
		return nil
	}
	if anim := m.Animation(); anim != nil {
		return &protocol.AnimationView{ID: anim.ID, Delay: anim.Delay}
	}
	return nil
}

// graphicViews returns this tick's spotanim views, or nil
// (createActorUpdates graphicViews, PlayerSession.ts:827,
// 838-848): only when Flag.GRAPHIC is set. Slot 0's graphic
// comes first, then the other slots in ascending slot order
// (TypeScript iterates the slot map in insertion order; slot
// order carries no meaning on the wire, and ascending order
// keeps the frame deterministic). A cleared slot is id -1.
func graphicViews(m *game.Mobile) []protocol.GraphicView {
	if m == nil || !m.Flags.Has(game.FlagGraphic) {
		return nil
	}
	var views []protocol.GraphicView
	if graphic := m.Graphic(); graphic != nil {
		views = append(views, protocol.GraphicView{Slot: 0, ID: graphic.ID, Height: graphic.Height, Delay: graphic.Delay})
	}
	slots := m.SlotGraphics()
	keys := make([]int, 0, len(slots))
	for slot := range slots {
		keys = append(keys, slot)
	}
	sort.Ints(keys)
	for _, slot := range keys {
		if graphic := slots[slot]; graphic != nil {
			views = append(views, protocol.GraphicView{Slot: slot, ID: graphic.ID, Height: graphic.Height, Delay: graphic.Delay})
		} else {
			views = append(views, protocol.GraphicView{Slot: slot, ID: -1, Height: 0, Delay: 0})
		}
	}
	return views
}

// forcedChatText returns the overhead chat text to include in a
// sync snapshot: the mobile's forced-chat text when the
// FORCED_CHAT flag is set and the text is non-nil (the
// createActorUpdates gate, PlayerSession.ts:813-820), else nil.
// The viewer relations gate (canReceivePublicChatFrom) is not
// applied: at the Go defaults it degenerates to always-true, and
// this site knows the owner when relations state lands in
// milestone 9.
func forcedChatText(m *game.Mobile) *string {
	if m == nil || !m.Flags.Has(game.FlagForcedChat) {
		return nil
	}
	return m.GetForcedChat()
}

// appearanceBlock returns the encoded sync appearance block stored on
// the player's session at bootstrap, or nil when the player has no
// session, the session is not a *session.Session (test doubles), or
// no block was stored yet. FlushTick falls back to the default block
// for nil. SyncAppearance returns a copy, so the snapshot never
// aliases the session's stored block.
func appearanceBlock(p *game.Player) []byte {
	if p.Session == nil {
		return nil
	}
	s, ok := p.Session.(interface{ SyncAppearance() []byte })
	if !ok {
		return nil
	}
	return s.SyncAppearance()
}

// Ensure playerFlushSource satisfies the session interface at
// compile time.
var _ session.FlushSource = (*playerFlushSource)(nil)
