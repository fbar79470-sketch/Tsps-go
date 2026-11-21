package game

import "sort"

// WorldEntitySync keeps each player's client in step with the
// boats around them, like NPC sync: a boat that comes into
// range is built (its deck scene, then the spawn, then its
// deck locs), a boat in range is moved, and one that leaves
// range is removed. Ported from
// game/content/sailing/WorldEntitySync.ts.
//
// The computation lives here; the encoding lives in
// internal/protocol (Worker A's world-entity encoders) and the
// adapter in cmd/server/sailingwiring.go, because this package
// must not import the protocol package. Flush therefore
// returns a WorldEntityFlushPlan — scenes, the info update,
// and deck locs, in send order — and the adapter encodes each
// part. The flush runs before player sync each tick
// (PlayerSession.ts:616: "Boats first, so a player on a deck
// refers to a boat the client already has").

// BoatViewDistance is how far from a viewer (their root tile,
// in tiles) a boat is shown.
const BoatViewDistance = 32

// WORLDENTITY_INFO has a one-byte length. An update is at most
// 19 bytes and a spawn 25, so capping the list at 8 boats (and
// 4 new ones a tick) keeps it under 255.
const maxBoatsInView = 8
const maxNewBoatsPerTick = 4

// WorldEntityPosition is a boat's sent position: fine X, 0,
// fine Y, and its angle.
type WorldEntityPosition struct {
	X           int
	Y           int
	Z           int
	Orientation int
}

// WorldEntityUpdateKind is the updateType of a
// WorldEntityUpdate: 0 despawn, 1 unchanged, 2 move smoothly
// by Delta, 3 snap by Delta.
type WorldEntityUpdateKind int

const (
	WorldEntityDespawn   WorldEntityUpdateKind = 0
	WorldEntityUnchanged WorldEntityUpdateKind = 1
	WorldEntityMove      WorldEntityUpdateKind = 2
	WorldEntitySnap      WorldEntityUpdateKind = 3
)

// WorldEntityUpdate is one per-tick update for an entity the
// client already has, in the client's order (the order of the
// previous flush). EntityIndex names the boat for the plan
// reader; the wire form carries no index (order-implicit).
// Delta is read only for Move/Snap updates.
type WorldEntityUpdate struct {
	EntityIndex int
	Kind        WorldEntityUpdateKind
	Delta       *WorldEntityPosition
}

// WorldEntitySpawnPlan is one new world entity: the spawn row
// of the info packet.
type WorldEntitySpawnPlan struct {
	EntityIndex int
	SizeX       int
	SizeZ       int
	ConfigID    int
	DrawMode    int
	Position    WorldEntityPosition
}

// WorldEntityScenePlan is a deck scene: the rebuild packet's
// arguments. RegionID is the cache region the scene's XTEA
// key comes from; the adapter fetches the key.
type WorldEntityScenePlan struct {
	EntityIndex    int
	ConfigID       int
	SizeX          int
	SizeZ          int
	RegionX        int
	RegionY        int
	RegionID       int
	TemplateChunks [4][13][13]int32
}

// DeckLocPlan is one deck loc to send: the loc, at its absolute
// deck tile (X, Y). Removed locs send as LOC_DEL, the rest as
// LOC_ADD_CHANGE with the loc's opFlags when present.
type DeckLocPlan struct {
	Loc BoatDeckLoc
	X   int
	Y   int
}

// WorldEntityFlushPlan is one tick's world-entity packets for
// one viewer, in send order: the scenes, then (when HasInfo)
// the info packet and the spawn locs, then the changed locs.
// A deck-loc change on a boat the client already has is sent
// even when nothing else changed (it does not set HasInfo),
// exactly as TS returns changedLocs without the info packet.
type WorldEntityFlushPlan struct {
	Scenes      []WorldEntityScenePlan
	HasInfo     bool
	Updates     []WorldEntityUpdate
	Spawns      []WorldEntitySpawnPlan
	SpawnLocs   []DeckLocPlan
	ChangedLocs []DeckLocPlan
}

// IsEmpty reports whether the plan sends nothing.
func (p WorldEntityFlushPlan) IsEmpty() bool {
	return len(p.Scenes) == 0 && !p.HasInfo && len(p.ChangedLocs) == 0
}

type worldEntityViewerState struct {
	// order are the boats the client has, in the client's order.
	order []int
	// boats is the boat sent for each entity index (an index
	// is reused once its boat is gone).
	boats map[int]*Boat
	// positions is the position each boat was last sent at.
	positions map[int]WorldEntityPosition
	// locChanges counts how many of each boat's deck loc
	// changes the client has.
	locChanges map[int]int
	// teleports is each boat's teleport count when last sent:
	// a newer one is sent as a jump (update type 3).
	teleports map[int]int
}

// WorldEntitySync is the per-viewer boat sync state, owned by
// the World (World.WorldEntities). Construct with
// NewWorldEntitySync; the manager arrives through SetBoats
// (World.SetBoatManager wires it). Like the TS WeakMap keyed by
// Player, viewer state is dropped when the player is forgotten
// (Forget); the coordinator calls World.ForgetWorldEntityViewer
// on logout/disconnect so a stale viewer never survives a
// relogin. One sync per world: viewer state must never be
// shared across worlds.
type WorldEntitySync struct {
	boats   *BoatManager
	viewers map[*Player]*worldEntityViewerState
}

// NewWorldEntitySync returns a sync reading boats from boats.
// A nil manager reads no boats; SetBoats installs one later.
func NewWorldEntitySync(boats *BoatManager) *WorldEntitySync {
	return &WorldEntitySync{boats: boats, viewers: make(map[*Player]*worldEntityViewerState)}
}

// SetBoats installs the manager the sync reads. A nil manager
// restores the no-boats behavior. The World calls this from
// SetBoatManager; call it only before ticking starts.
func (s *WorldEntitySync) SetBoats(m *BoatManager) { s.boats = m }

// Forget drops a viewer's sync state.
func (s *WorldEntitySync) Forget(player *Player) {
	delete(s.viewers, player)
}

func (s *WorldEntitySync) manager() *BoatManager {
	return s.boats
}

// Flush returns the packets to send this tick, before player
// sync, or an empty plan if nothing changed.
func (s *WorldEntitySync) Flush(player *Player) WorldEntityFlushPlan {
	state := s.viewers[player]
	if state == nil {
		state = &worldEntityViewerState{
			boats:      make(map[int]*Boat),
			positions:  make(map[int]WorldEntityPosition),
			locChanges: make(map[int]int),
			teleports:  make(map[int]int),
		}
		s.viewers[player] = state
	}
	manager := s.manager()
	visible := s.boatsInView(player, manager)
	visibleIDs := make(map[int]bool, len(visible))
	for _, boat := range visible {
		visibleIDs[boat.EntityIndex] = true
	}

	var updates []WorldEntityUpdate
	var kept []int
	// changedLocs are deck locs changed (facilities built or
	// removed) on boats the client already has.
	var changedLocs []DeckLocPlan
	oldOrder := state.order
	for _, entityIndex := range oldOrder {
		var boat *Boat
		if manager != nil {
			boat = manager.GetBoat(entityIndex)
		}
		if boat == nil || boat != state.boats[entityIndex] || !visibleIDs[entityIndex] {
			updates = append(updates, WorldEntityUpdate{EntityIndex: entityIndex, Kind: WorldEntityDespawn})
			delete(state.boats, entityIndex)
			delete(state.positions, entityIndex)
			delete(state.locChanges, entityIndex)
			delete(state.teleports, entityIndex)
			continue
		}
		kept = append(kept, entityIndex)
		count, changes := manager.DeckLocChanges(boat, state.locChanges[entityIndex])
		state.locChanges[entityIndex] = count
		for _, loc := range changes {
			changedLocs = append(changedLocs, deckLocPlan(boat, loc))
		}
		position := positionOf(boat)
		last := state.positions[entityIndex]
		delta := &WorldEntityPosition{
			X:           position.X - last.X,
			Y:           0,
			Z:           position.Z - last.Z,
			Orientation: position.Orientation - last.Orientation,
		}
		if boat.TeleportCount != state.teleports[entityIndex] {
			// A teleport: the client places the boat there at
			// once instead of sailing it over.
			updates = append(updates, WorldEntityUpdate{EntityIndex: entityIndex, Kind: WorldEntitySnap, Delta: delta})
			state.positions[entityIndex] = position
			state.teleports[entityIndex] = boat.TeleportCount
		} else if delta.X == 0 && delta.Z == 0 && delta.Orientation == 0 {
			updates = append(updates, WorldEntityUpdate{EntityIndex: entityIndex, Kind: WorldEntityUnchanged})
		} else {
			updates = append(updates, WorldEntityUpdate{EntityIndex: entityIndex, Kind: WorldEntityMove, Delta: delta})
			state.positions[entityIndex] = position
		}
	}

	var scenes []WorldEntityScenePlan
	var spawns []WorldEntitySpawnPlan
	var spawnLocs []DeckLocPlan
	room := min(maxNewBoatsPerTick, maxBoatsInView-len(kept))
	oldOrderSet := make(map[int]bool, len(oldOrder))
	for _, index := range oldOrder {
		oldOrderSet[index] = true
	}
	for _, boat := range visible {
		if len(spawns) >= room {
			break
		}
		// An index despawned this tick can't be respawned in
		// the same packet.
		if oldOrderSet[boat.EntityIndex] {
			continue
		}
		position := positionOf(boat)
		scenes = append(scenes, scenePlan(manager, boat))
		spawns = append(spawns, WorldEntitySpawnPlan{
			EntityIndex: boat.EntityIndex,
			SizeX:       boat.SizeX,
			SizeZ:       boat.SizeZ,
			ConfigID:    boat.ConfigID,
			DrawMode:    0,
			Position:    position,
		})
		state.boats[boat.EntityIndex] = boat
		state.positions[boat.EntityIndex] = position
		count, _ := manager.DeckLocChanges(boat, 0)
		state.locChanges[boat.EntityIndex] = count
		state.teleports[boat.EntityIndex] = boat.TeleportCount
		spawnLocs = append(spawnLocs, deckLocPlans(manager, boat)...)
	}

	changed := len(spawns) > 0
	for _, update := range updates {
		if update.Kind != WorldEntityUnchanged {
			changed = true
			break
		}
	}
	order := append(append([]int(nil), kept...), spawnIndices(spawns)...)
	state.order = order
	plan := WorldEntityFlushPlan{Scenes: scenes, ChangedLocs: changedLocs}
	if !changed {
		return plan
	}
	plan.HasInfo = true
	plan.Updates = updates
	plan.Spawns = spawns
	plan.SpawnLocs = spawnLocs
	return plan
}

func spawnIndices(spawns []WorldEntitySpawnPlan) []int {
	indices := make([]int, len(spawns))
	for i, spawn := range spawns {
		indices[i] = spawn.EntityIndex
	}
	return indices
}

// boatsInView are the boats near the player's root tile,
// nearest first, with the one they're on always shown.
func (s *WorldEntitySync) boatsInView(player *Player, manager *BoatManager) []*Boat {
	if manager == nil {
		return nil
	}
	root := manager.RootLocation(&player.Mobile)
	aboard := manager.GetBoatAboard(&player.Mobile)
	distance := func(boat *Boat) int {
		return max(absInt(boat.TileX()-root.X), absInt(boat.TileY()-root.Y))
	}
	var visible []*Boat
	for _, boat := range manager.All() {
		if boat == aboard || (boat.Level == root.Z && distance(boat) <= BoatViewDistance && manager.CanSeeBoat(&player.Mobile, boat)) {
			visible = append(visible, boat)
		}
	}
	sort.SliceStable(visible, func(i, j int) bool {
		a, b := visible[i], visible[j]
		if a == aboard && b != aboard {
			return true
		}
		if b == aboard && a != aboard {
			return false
		}
		return distance(a) < distance(b)
	})
	if len(visible) > maxBoatsInView {
		visible = visible[:maxBoatsInView]
	}
	return visible
}

func positionOf(boat *Boat) WorldEntityPosition {
	return WorldEntityPosition{X: boat.FineX, Y: 0, Z: boat.FineY, Orientation: boat.Angle}
}

// scenePlan is the deck scene: the boat's template zones (8x8
// tiles each, a sloop's are 1x2) copied into a 13x13 scene
// from its centre chunk.
func scenePlan(manager *BoatManager, boat *Boat) WorldEntityScenePlan {
	spec, _ := manager.GetSpec(boat)
	var chunks [4][13][13]int32
	for plane := 0; plane < 4; plane++ {
		for x := 0; x < 13; x++ {
			for y := 0; y < 13; y++ {
				chunks[plane][x][y] = -1
			}
		}
	}
	zonesX := (spec.SizeX + 7) / 8
	zonesY := (spec.SizeZ + 7) / 8
	for plane := 0; plane < 4; plane++ {
		for zoneX := 0; zoneX < zonesX; zoneX++ {
			for zoneY := 0; zoneY < zonesY; zoneY++ {
				// packTemplateChunk (TemplatedInstanceArea.ts:
				// 49-54): ((plane&3)<<24)|((chunkX&0x3ff)<<14)
				// |((chunkY&0x7ff)<<3)|((rotation&3)<<1), with
				// rotation 0. Inlined here so this file does
				// not depend on Worker B's helper name.
				chunkX := spec.TemplateChunkX + zoneX
				chunkY := spec.TemplateChunkY + zoneY
				chunks[plane][6+zoneX][6+zoneY] = int32(((plane & 3) << 24) | ((chunkX & 0x3ff) << 14) | ((chunkY & 0x7ff) << 3))
			}
		}
	}
	return WorldEntityScenePlan{
		EntityIndex:    boat.EntityIndex,
		ConfigID:       boat.ConfigID,
		SizeX:          boat.SizeX,
		SizeZ:          boat.SizeZ,
		RegionX:        boat.DeckRegionX,
		RegionY:        boat.DeckRegionY,
		RegionID:       ((spec.TemplateChunkX >> 3) << 8) | (spec.TemplateChunkY >> 3),
		TemplateChunks: chunks,
	}
}

// deckLocPlans are the deck locs sent with a spawn: a static
// ship's template already draws its own locs, so only its
// explicit dynamic additions/overrides are sent; player boats
// send their full deck loc set.
func deckLocPlans(manager *BoatManager, boat *Boat) []DeckLocPlan {
	spec, _ := manager.GetSpec(boat)
	locs := spec.Locs
	if boat.Static {
		locs = spec.SyncLocs
	}
	plans := make([]DeckLocPlan, 0, len(locs))
	for _, loc := range locs {
		plans = append(plans, deckLocPlan(boat, loc))
	}
	return plans
}

func deckLocPlan(boat *Boat, loc BoatDeckLoc) DeckLocPlan {
	return DeckLocPlan{Loc: loc, X: boat.DeckBaseX() + loc.X, Y: boat.DeckBaseY() + loc.Y}
}
