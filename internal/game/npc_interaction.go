package game

// Following, interaction, and facing state for NPCs: the Mobile
// surface NPC.process and the follow driver rely on.
//
// In TS these fields live on Mobile (Mobile.ts:88, 93, 113):
// positionToFace, following, and interactingMobile, with
// setMobileInteraction (340-352), setPositionToFace (620-641),
// getFacingDirection (595-598), getFollowing/setFollowing
// (911-920), and getCombatFollowing/setCombatFollowing
// (932-944). Go's Mobile (entity.go) does not carry them, and
// only NPCs read them so far, so they live on NPC (see npc.go).
// The methods here port the Mobile semantics exactly, including
// the facing updates and update-flagging; the NPC
// setPositionToFace override (NPC.ts:107-122) adds only debug
// logging behind a disabled flag, so there is no separate NPC
// override in Go.
//
// MOBILE IDENTITY. A target is identified by the address of its
// embedded Mobile: &player.Mobile for a player, &npc.Mobile for
// an NPC. That is the pointer form Combat already uses (see
// combat.go: GetCombat allocates NewCombat(m) on the Mobile
// receiver), so comparisons between an interaction target, a
// follow target, and a combat target compare equal exactly when
// TS reference equality would. SetMobileInteraction and
// SetFollowing accept *Player, *NPC, *Mobile, or nil; the typed
// forms additionally record the typed identity the dialogue
// hold and the follow driver need (a bare *Mobile carries no
// path back to its Player or NPC).
//
// SYNC GAP. The session sync layer does not yet read this state:
// createNpcView sends interaction -1 always (no interacting
// mobiles in Go) and reads rotation from Mobile.Facing. The
// facing updates here DO reach clients through Facing; the
// interaction index and face-position block need session work
// the coordinator owns, outside this package's files.

// mobileTarget is one Mobile target plus its typed identity when
// known. The zero value is no target. It is a value type: NPC
// copies it freely, and the pointers inside stay valid for the
// target entity's lifetime.
type mobileTarget struct {
	mobile *Mobile
	player *Player
	npc    *NPC
}

// resolveMobileTarget resolves the accepted target forms to a
// mobileTarget. A typed-nil pointer and any other type resolve
// to no target: TS callers pass only Mobiles, and an unrecognized
// value must not invent a target.
func resolveMobileTarget(target any) mobileTarget {
	switch t := target.(type) {
	case nil:
		return mobileTarget{}
	case *Player:
		if t == nil {
			return mobileTarget{}
		}
		return mobileTarget{mobile: &t.Mobile, player: t}
	case *NPC:
		if t == nil {
			return mobileTarget{}
		}
		return mobileTarget{mobile: &t.Mobile, npc: t}
	case *Mobile:
		if t == nil {
			return mobileTarget{}
		}
		return mobileTarget{mobile: t}
	default:
		return mobileTarget{}
	}
}

// present reports whether the target names a Mobile.
func (t mobileTarget) present() bool { return t.mobile != nil }

// location returns the target's tile. It reports false only for
// an absent target; a present target always has a location.
func (t mobileTarget) location() (Location, bool) {
	if t.mobile == nil {
		return Location{}, false
	}
	return t.mobile.Location(), true
}

// registered reports whether the target holds a world-list slot
// (Mobile.ts:831-833, isRegistered). An absent target reads as
// unregistered.
func (t mobileTarget) registered() bool {
	return t.mobile != nil && t.mobile.IsRegistered()
}

// equals reports Mobile identity: the same embedded Mobile
// pointer. An absent target never equals anything, including
// another absent target.
func (t mobileTarget) equals(other mobileTarget) bool {
	return t.mobile != nil && t.mobile == other.mobile
}

// asTarget returns the most-typed form of the target for
// re-setting it through SetMobileInteraction: the *Player when
// known, else the *NPC, else the bare *Mobile, else nil.
func (t mobileTarget) asTarget() any {
	if t.player != nil {
		return t.player
	}
	if t.npc != nil {
		return t.npc
	}
	if t.mobile != nil {
		return t.mobile
	}
	return nil
}

// deadOrDying reports whether the target is dead or dying, as
// the follow end-conditions read it (MovementQueue.ts:918-924):
// an NPC target with hitpoints <= 0 or dying. A player target
// has no hitpoints in Go (no combat/death yet), so it never
// reads dead here; a bare *Mobile target has no NPC identity to
// read either. The zero-means-unset hitpoints convention (see
// npc.go) means an NPC at exactly 0 without the Dying flag
// reads alive — the same rule the aggression check uses.
func (t mobileTarget) deadOrDying() bool {
	if t.npc == nil {
		return false
	}
	return t.npc.Dying || t.npc.Hitpoints < 0
}

// queue returns the target's movement queue, or nil when the
// target's typed identity is unknown (a bare *Mobile) or the
// entity has none (a zero-value NPC).
func (t mobileTarget) queue() *MovementQueue {
	if t.player != nil {
		return t.player.MovementQueue()
	}
	if t.npc != nil {
		return t.npc.MovementQueue()
	}
	return nil
}

// size returns the target's footprint: 1 for a player
// (Player.ts:93-95), the definition size for an NPC, 1 for a
// bare *Mobile whose identity is unknown.
func (t mobileTarget) size() int {
	if t.npc != nil {
		return t.npc.Size()
	}
	return 1
}

// targetFacingDirection is the compass direction from the NPC to
// the target's tile (Mobile.ts:605-611, targetFacingDirection
// via directionTo): DirNone when the target is absent or shares
// the NPC's tile.
func (n *NPC) targetFacingDirection(target mobileTarget) Direction {
	loc, ok := target.location()
	if !ok {
		return DirNone
	}
	return DirectionFromDeltas(loc.X-n.Loc.X, loc.Y-n.Loc.Y)
}

// updateFacingDirection turns the NPC toward target, when the
// target is on another tile (Mobile.ts:613-618). A nil target
// leaves Facing untouched.
func (n *NPC) updateFacingDirection(target *Location) {
	if target == nil {
		return
	}
	if dir := DirectionFromDeltas(target.X-n.Loc.X, target.Y-n.Loc.Y); dir != DirNone {
		n.Facing = dir
	}
}

// SetMobileInteraction faces the NPC toward target through
// entity interaction (Mobile.ts:340-352, setMobileInteraction):
// Facing is first set to the direction toward the PREVIOUS
// target (when it was on another tile), then the target is
// stored and the generation bumped, then Facing is set to the
// direction toward the NEW target (when it is on another tile),
// and FlagEntityInteraction is flagged — unconditionally, even
// when the target did not change. A nil target clears the
// interaction. See the file header for the accepted forms.
func (n *NPC) SetMobileInteraction(target any) {
	if prev := n.targetFacingDirection(n.interacting); prev != DirNone {
		n.Facing = prev
	}
	n.interacting = resolveMobileTarget(target)
	n.interactionGeneration++
	if next := n.targetFacingDirection(n.interacting); next != DirNone {
		n.Facing = next
	}
	n.Flags = n.Flags.Set(FlagEntityInteraction)
}

// GetInteractingMobile returns the interaction target's Mobile,
// or nil (Mobile.ts:685-687, getInteractingMobile).
func (n *NPC) GetInteractingMobile() *Mobile { return n.interacting.mobile }

// PlayerInteractionOffset is the player-index offset in sync
// interaction indices (PlayerSession.ts:101,
// PLAYER_INTERACTION_OFFSET = 0x10000): a player target's
// index is offset, an NPC target's is not.
const PlayerInteractionOffset = 0x10000

// InteractionIndexFor resolves target to its sync interaction
// index (PlayerSession.ts:884-886, interactionIndex): -1 when
// there is no target, the NPC index for an NPC target, and the
// player index plus PlayerInteractionOffset for a player
// target. Resolution runs through the combat identity
// (combatantFor): a Mobile whose fighter identity was never
// bound (no typed GetCombat call) resolves to -1 rather than
// a possibly wrong index — the identity is bound at spawn for
// NPCs and on the first Process tick for players, so a live
// fighter always resolves.
func InteractionIndexFor(target *Mobile) int {
	if target == nil {
		return -1
	}
	fighter := combatantFor(target)
	if p := fighter.Player(); p != nil {
		return p.Index + PlayerInteractionOffset
	}
	if npc := fighter.NPC(); npc != nil {
		return npc.Index
	}
	return -1
}

// InteractionIndex returns the sync interaction index for
// this NPC's current interaction target (see
// InteractionIndexFor), or -1 when it interacts with nothing.
// The flush source reads it for the npcViews mapping
// (PlayerSession.ts:555).
func (n *NPC) InteractionIndex() int {
	return InteractionIndexFor(n.GetInteractingMobile())
}

// InteractingPlayer returns the interaction target as a *Player,
// or nil when the target is not a player (or is unknown-typed).
// It is the Go form of the isPlayer/getAsPlayer derivation
// updateDialogueHold performs (NPC.ts:274-275).
func (n *NPC) InteractingPlayer() *Player { return n.interacting.player }

// SetPositionToFace turns the NPC toward a tile (Mobile.ts:620-
// 641, setPositionToFace): a no-op when the new target has the
// same coordinates as the current one, or when both are nil;
// otherwise Facing updates toward the tile and FlagFacePosition
// is flagged. (TS also short-circuits on pointer identity; Go
// stores a copy, so the coordinate check subsumes it.) A nil
// target clears the face target (flagged, unless already
// clear). The stored tile is a copy: callers cannot mutate the
// NPC's face target through the pointer they passed.
func (n *NPC) SetPositionToFace(position *Location) {
	current := n.positionToFace
	if current != nil && position != nil && current.Equals(*position) {
		return
	}
	if current == nil && position == nil {
		return
	}
	n.updateFacingDirection(position)
	if position == nil {
		n.positionToFace = nil
	} else {
		stored := *position
		n.positionToFace = &stored
	}
	n.Flags = n.Flags.Set(FlagFacePosition)
}

// PositionToFace returns the face-target tile and whether one is
// set (Mobile.ts:587-589, getPositionToFace).
func (n *NPC) PositionToFace() (Location, bool) {
	if n.positionToFace == nil {
		return Location{}, false
	}
	return *n.positionToFace, true
}

// GetFacingDirection returns the current 8-way compass direction
// (Mobile.ts:595-598, getFacingDirection): an active interaction
// target on another tile overrides the last walk/face heading.
// The sync layer reads Mobile.Facing directly today (see the
// file header); this is the logic-facing accessor.
func (n *NPC) GetFacingDirection() Direction {
	if dir := n.targetFacingDirection(n.interacting); dir != DirNone {
		return dir
	}
	return n.Facing
}

// SetDirection turns the NPC toward direction, facing the
// adjacent tile in that direction (Mobile.ts:689-691,
// setDirection).
func (n *NPC) SetDirection(direction Direction) {
	target := n.Location().Transform(direction.Dx(), direction.Dy())
	n.SetPositionToFace(&target)
}

// GetFollowing returns the followed Mobile, or nil (Mobile.ts:
// 911-913, getFollowing).
func (n *NPC) GetFollowing() *Mobile { return n.following.mobile }

// FollowingPlayer returns the followed target as a *Player, or
// nil when the target is not a player. See InteractingPlayer.
func (n *NPC) FollowingPlayer() *Player { return n.following.player }

// FollowingNPC returns the followed target as an *NPC, or nil
// when the target is not an NPC.
func (n *NPC) FollowingNPC() *NPC { return n.following.npc }

// SetFollowing starts or stops following target (Mobile.ts:
// 915-921, setFollowing): setting the same target is a no-op;
// otherwise the target is stored. The followed teleport serial
// (Mobile.ts:920, 927-931) is NOT ported: no teleport path
// exists in Go, so no teleport counter exists to record — a
// leader that teleports is caught by the follow driver's
// distance/plane checks instead (see processFollowing).
func (n *NPC) SetFollowing(target any) {
	resolved := resolveMobileTarget(target)
	// The same present target is a no-op (Mobile.ts:916-918).
	// Clearing an already-absent target still lands below, but
	// assigning the absent value again is unobservable.
	if n.following.equals(resolved) {
		return
	}
	n.following = resolved
}

// GetCombatFollowing returns the combat target (Mobile.ts:932-
// 934, getCombatFollowing delegates to Combat.getTarget).
func (n *NPC) GetCombatFollowing() *Mobile { return n.GetCombat().Target() }

// SetCombatFollowing is NOT ported: TS (Mobile.ts:936-944) resets
// combat for a nil target and attacks a non-nil one, and the
// Combat stub exposes neither Reset nor Attack. The combat-core
// work owns those methods; wiring this setter is its follow-up.
// Callers that need to stop combat following must go through
// the combat system once it lands. (Deliberately absent: no
// stub that silently drops the call.)

// updateDialogueHold ports NPC.ts:271-303: an NPC talking to a
// player stands still while the conversation lasts, from its
// first line of the chat, until the dialogue closes or the
// player is more than a tile away. The player's own opening line
// does not hold it.
//
// Degenerations, each at its site below: the "player's
// interaction is this NPC" term has no Go form (players carry
// no interaction state), and the transformation-id and
// content-id speaker terms have none either (no transformations,
// no CacheDefinitions resolution in this package) — so the
// speaker check reduces to the dialogue entry's NPC id equalling
// this NPC's id. A dialogue entry that is not an NPC line
// (player lines, options, statements) carries no NPC id and
// never holds.
func (n *NPC) updateDialogueHold() bool {
	player := n.InteractingPlayer()
	if player == nil {
		n.heldByDialogue = nil
		return false
	}
	dialogue := player.GetDialogueManager()
	if dialogue == nil || !dialogue.IsActive() {
		n.heldByDialogue = nil
		return false
	}
	here := n.Location()
	there := player.Location()
	if here.Z != there.Z || here.Distance(there) > 1 {
		n.heldByDialogue = nil
		return false
	}
	if n.heldByDialogue != player {
		// Only the NPC speaking is held (NPC.ts:288-297), not
		// a monster or pet that happens to face a player
		// reading someone else's line. The speaker is the
		// current entry's NPC id when the entry is an NPC
		// line; the TS disjuncts for the player's interaction
		// target, the transformation id, and the content id
		// have no Go form (see the method comment), so only
		// the id equality remains.
		speaking := false
		if entry, ok := dialogue.GetCurrent().(interface{ GetNpcID() int }); ok && entry != nil {
			speaking = entry.GetNpcID() == n.ID
		}
		if speaking {
			n.heldByDialogue = player
		} else {
			n.heldByDialogue = nil
		}
	}
	return n.heldByDialogue != nil
}

// HeldByDialogue reports the player holding this NPC still, or
// nil. It is the accessor for the updateDialogueHold state
// (NPC.ts:173); tests and content read it, Process maintains it.
func (n *NPC) HeldByDialogue() *Player { return n.heldByDialogue }

// petFollowAutoTeleportDistance is
// GameConstants.PET_FOLLOW_AUTO_TELEPORT_DISTANCE
// (GameConstants.ts:34): 15 tiles. A follower further than this
// from its leader stops following (for a non-player follower;
// pets teleport back instead — see processFollowing).
const petFollowAutoTeleportDistance = 15

// footprintGap is the Chebyshev gap between two footprints: 0 when
// they overlap, 1 when they stand edge-to-edge one tile apart. It
// is the Go form of Location.calculateDistance over the two
// footprints' tiles (Mobile.ts:484-488, calculateDistance), via
// the bounds arithmetic of CombatRange.distance
// (CombatRange.ts:243-251). A cross-plane pair reads as
// unreachableDistance, where TS Location distance reads
// MAX_SAFE_INTEGER.
func footprintGap(a Location, aSize int, b Location, bSize int) int {
	if a.Z != b.Z {
		return unreachableDistance
	}
	aSize = max(aSize, 1)
	bSize = max(bSize, 1)
	dx := max(b.X-(a.X+aSize-1), a.X-(b.X+bSize-1), 0)
	dy := max(b.Y-(a.Y+aSize-1), a.Y-(b.Y+bSize-1), 0)
	return max(dx, dy)
}

// outerTiles returns the ring of tiles around a footprint
// (Mobile.ts:457-470, outterTiles — the Go name drops the TS
// typo): the south and north rows plus the west and east
// columns, in TS order. The plane is the footprint's plane.
func outerTiles(loc Location, size int) []Location {
	size = max(size, 1)
	tiles := make([]Location, 0, size*4)
	for x := 0; x < size; x++ {
		tiles = append(tiles, loc.Transform(x, -1))
		tiles = append(tiles, loc.Transform(x, size))
	}
	for y := 0; y < size; y++ {
		tiles = append(tiles, loc.Transform(-1, y))
		tiles = append(tiles, loc.Transform(size, y))
	}
	return tiles
}

// processFollowing ports the NPC-relevant subset of
// MovementQueue.processFollowing (MovementQueue.ts:896-1040),
// in TS order: end conditions, the too-far end, interaction and
// facing toward the leader, the mobility gate, the adjacent-idle
// rule, and routing to the leader's trail tile or the closest
// outer tile. It runs from Process inside the movement gate, in
// the position MovementQueue.process runs processFollowing
// (MovementQueue.ts:580-586).
//
// NOT ported (each needs a system outside this milestone):
//   - The pet branches: waiting out the owner's death, the
//     area follower bar, and the pet auto-teleport
//     (MovementQueue.ts:901-908, 933-938, 973-978, 1069-1097).
//     Pets follow by the same routing as any NPC here, and a
//     far leader ends the follow instead of teleporting the
//     pet back.
//   - The private-area branch (MovementQueue.ts:940-948): no
//     private areas exist in Go.
//   - The teleport-serial branch (MovementQueue.ts:950-961):
//     player-following-player only, and no teleport serials
//     exist in Go.
//   - The trailsLeader rule (MovementQueue.ts:1008-1012):
//     player-following-player only.
//   - The bot repath throttle (MovementQueue.ts:1031-1035):
//     no bot repath state exists in Go.
//   - The follow-blocked event (MovementQueue.ts:1043-1045,
//     "movement:follow-blocked"): no plugin system; a blocked
//     follow simply routes nowhere.
//   - The blocked-tile filter on outer tiles
//     (MovementQueue.ts:1048-1065, RegionManager.blocked): the
//     queue's clipping is not queryable from here, so the
//     closest outer tile is picked by distance alone; the
//     route finder still refuses a route through a wall, and
//     the step validation still refuses a blocked step.
func (n *NPC) processFollowing() {
	target := n.following
	if !target.present() {
		return
	}
	// End conditions (MovementQueue.ts:910-931): following
	// itself, an unregistered leader, or a dead/dying leader
	// ends the follow. The player-dying term has no Go form
	// (no player death state); see deadOrDying.
	if target.mobile == &n.Mobile || !target.registered() || target.deadOrDying() {
		n.endFollowing()
		return
	}
	targetLoc, _ := target.location()
	// A leader further than the pet auto-teleport distance —
	// including across planes, where the gap reads unreachable
	// — ends a non-player follow (MovementQueue.ts:967-981).
	// The pet teleport that would rescue a pet is not ported
	// (see the method comment).
	if footprintGap(n.Location(), n.Size(), targetLoc, target.size()) > petFollowAutoTeleportDistance {
		n.endFollowing()
		return
	}
	n.SetMobileInteraction(target.asTarget())
	n.SetPositionToFace(&targetLoc)
	if !n.canMove() {
		return
	}
	distance := footprintGap(n.Location(), n.Size(), targetLoc, target.size())
	leaderQueue := target.queue()
	leaderMoving := leaderQueue != nil && leaderQueue.IsMoving()
	// When not overlapping and already adjacent to a stationary
	// leader, do not re-route (MovementQueue.ts:992-1000).
	// The blocked-adjacent event has no Go form (see above).
	if distance <= 1 && !leaderMoving && distance != 0 {
		return
	}
	destination, ok := n.followDestination(target, targetLoc)
	if !ok {
		return
	}
	// Route to the destination, replacing the queued walk
	// (MovementQueue.ts:1037-1039). A route with zero
	// checkpoints while still separated simply leaves the NPC
	// standing; the follow-blocked event is not ported.
	if n.coordinator != nil {
		n.coordinator.routeTo(destination)
	}
}

// followDestination picks the tile to walk to: the tile the
// leader just stepped from (its queue's followX/followY), when
// the leader has one and the NPC does not already stand on it;
// otherwise the closest outer tile of the leader's footprint
// (MovementQueue.ts:1002-1028, getClosestFollowTile). It
// reports false when no destination exists (the NPC already
// stands on the only candidate). The trail tile wins even when
// the leader is adjacent: TS only falls back to the closest
// outer tile when the trail tile is missing, occupied by the
// follower, or the follower is more than a tile away without
// the trailsLeader rule — and trailsLeader never applies to an
// NPC follower, so distance > 1 always falls back here.
func (n *NPC) followDestination(target mobileTarget, targetLoc Location) (Location, bool) {
	here := n.Location()
	distance := footprintGap(here, n.Size(), targetLoc, target.size())
	var destination *Location
	if queue := target.queue(); queue != nil && queue.FollowX != -1 && queue.FollowY != -1 {
		trail := Location{X: queue.FollowX, Y: queue.FollowY, Z: here.Z}
		destination = &trail
	}
	if destination == nil || destination.Equals(here) || distance > 1 {
		closest := n.closestOuterTile(targetLoc, target.size())
		if closest != nil {
			destination = closest
		}
	}
	if destination == nil || destination.Equals(here) {
		return Location{}, false
	}
	return *destination, true
}

// closestOuterTile returns the leader footprint's outer tile
// nearest to the NPC (MovementQueue.ts:1047-1065,
// getClosestFollowTile), by Chebyshev distance from the NPC's
// tile, first-ties-wins in outer-tile order. The blocked-tile
// filter is not ported (see processFollowing). It never returns
// nil: a footprint always has outer tiles.
func (n *NPC) closestOuterTile(targetLoc Location, targetSize int) *Location {
	here := n.Location()
	var best *Location
	bestDistance := 0
	for _, tile := range outerTiles(targetLoc, targetSize) {
		distance := tile.Distance(here)
		if distance < 0 {
			continue
		}
		if best == nil || distance < bestDistance {
			tile := tile
			best = &tile
			bestDistance = distance
		}
	}
	return best
}

// endFollowing stops following: the follow target, interaction,
// and face target are cleared and the queue is reset
// (MovementQueue.ts:918-931, the end branches). The combat-
// following clear (setCombatFollowing(null)) is not ported
// with it: see SetCombatFollowing.
func (n *NPC) endFollowing() {
	n.following = mobileTarget{}
	n.SetMobileInteraction(nil)
	n.SetPositionToFace(nil)
	if queue := n.movementQueue; queue != nil {
		queue.Reset(true)
	}
}

// unreachableDistance is the Go form of
// Number.MAX_SAFE_INTEGER as a tile distance: larger than any
// reachable range in the 32768-tile world. Cross-plane distance
// measures return it so range comparisons fail closed.
const unreachableDistance = 1 << 30
