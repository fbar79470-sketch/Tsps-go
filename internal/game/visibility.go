package game

import (
	"fmt"
	"math"
	"sort"
)

// World visibility: update buckets and per-player local player/NPC lists,
// ported from game/World.ts (rebuildUpdateBuckets, collectFromBuckets,
// getBucketNearbyPlayersForUpdate, getNearbyNpcsForUpdate,
// updateLocalPlayers, updateLocalNpcs, and the update_players_npcs phase
// of World.process).
//
// Everything in this file is tick-goroutine-only: RebuildUpdateBuckets
// runs in the rebuild_update_buckets phase, the Nearby/Update methods run
// in the update_players_npcs phase, and the flush phase reads the local
// lists later in the same tick. The bucket maps and the local lists are
// not synchronized; no other goroutine may call these methods or read the
// lists. (The MobileList reads inside them are safe regardless — the
// lists are internally locked — but the buckets and local lists are not.)
//
// Degenerations versus TS, all deliberate and documented again at each
// site where TS consults the missing system:
//
//   - CrossFloor: no plane policy is registered in Go, so
//     CrossFloor.visible degenerates to same-plane-only plus Chebyshev
//     distance (CrossFloor.ts:22-45: visibleOnPlane is viewer == target
//     without a policy; distance is Chebyshev). The planes loops in the
//     Nearby methods therefore visit only the origin plane.
//   - Boats: BoatManager.rootLocation, syncArea, and canSeeAboard
//     are LIVE through the World's BoatManager (see
//     boat_manager.go: rootLocationOf/syncAreaOf/canSeeAboardBy),
//     degenerating to the entity's own Loc, nil, and true when
//     no manager is installed. A deck counts as the main world
//     for sync-area purposes, so people aboard and ashore see
//     each other; an actor on a boat the viewer cannot see is
//     hidden (BoatManager.ts:179-185, 227-238).
//   - Region gating: PROCESS_NPCS_BY_ACTIVE_REGIONS / the active-region
//     index is NOT STARTED, so activeNpcsForUpdate is every registered
//     NPC, matching the existing processNPCs/resetNPCs behavior.
//     RebuildUpdateBuckets scans the live lists, which naturally covers
//     the npcsAddedSinceLastCollection reconciliation in TS
//     (World.ts:521-532) — there is no separate pending set in Go.
//   - staysInViewWhenPlaced is always false: exact-move, transparency,
//     and telejump state do not exist in Go.
//   - No pet special-case (World.ts:836-852) and no owner-only check:
//     there is no attributes/pets system and no NPC ownership in Go.
//   - getNpcsNear (World.ts:707-724) is NOT ported: it needs the region
//     occupant index (npcRegionOccupants), which is NOT STARTED.

// updateBucketKey returns the update-bucket key for a tile: `z:(x>>3):(y>>3)`.
// World.ts:270-272 (getUpdateBucketKey). Go's >> on ints is an arithmetic
// shift, matching TS >> for negative coordinates as well.
func updateBucketKey(x, y, z int) string {
	return bucketKey(x>>3, y>>3, z)
}

// bucketKey returns the update-bucket key for chunk coordinates (already
// divided by 8), used by collectFromBuckets when walking neighboring
// chunks. Same `z:chunkX:chunkY` shape as updateBucketKey.
func bucketKey(chunkX, chunkY, z int) string {
	return fmt.Sprintf("%d:%d:%d", z, chunkX, chunkY)
}

// addToUpdateBucket files entity in buckets under the bucket key of loc.
// World.ts:502-515 (addToUpdateBucket). The loc argument is the entity's
// root location (BoatManager.rootLocation: the tile under its deck tile
// when aboard, else its own tile); callers pass rootLocationOf.
func addToUpdateBucket[T any](buckets map[string][]T, loc Location, entity T) {
	key := updateBucketKey(loc.X, loc.Y, loc.Z)
	buckets[key] = append(buckets[key], entity)
}

// RebuildUpdateBuckets clears both update-bucket maps and refiles every
// registered player and every registered visible NPC, in list (index)
// order. World.ts:517-546 (rebuildUpdateBuckets).
//
// TS files NPCs from activeNpcsForUpdate (the region-collected set,
// reconciled with npcsAddedSinceLastCollection); region gating is NOT
// STARTED in Go, so every registered NPC is filed — the same set
// processNPCs processes and resetNPCs resets. Because the rebuild scans
// the live lists, NPCs registered earlier in this same tick are included
// in this tick's sync (the TS reconciliation's purpose, World.ts:518-520)
// with no separate pending set. Invisible NPCs are excluded, as in TS
// (World.ts:542: `!npc.isVisible?.()` skips the NPC).
//
// Tick-goroutine-only; callers must run this before the Nearby/Update
// methods each tick (World.Process does, in phase order).
func (w *World) RebuildUpdateBuckets() {
	if w.playerUpdateBuckets == nil {
		w.playerUpdateBuckets = make(map[string][]*Player)
	} else {
		clear(w.playerUpdateBuckets)
	}
	if w.npcUpdateBuckets == nil {
		w.npcUpdateBuckets = make(map[string][]*NPC)
	} else {
		clear(w.npcUpdateBuckets)
	}
	w.players.ForEach(func(p *Player) {
		addToUpdateBucket(w.playerUpdateBuckets, w.rootLocationOf(&p.Mobile), p)
	})
	for _, n := range w.npcs.All() {
		if !n.Visible {
			continue
		}
		addToUpdateBucket(w.npcUpdateBuckets, w.rootLocationOf(&n.Mobile), n)
	}
}

// collectFromBuckets walks buckets nearest-first and stops once budget
// candidates are collected. World.ts:647-682 (collectFromBuckets).
// Ring 0 is the origin chunk; each further ring visits only its newly
// reached edge (dx/dy iteration order matches TS: dx outer, dy inner).
// A budget <= 0 collects nothing; a nil include accepts every entry.
func collectFromBuckets[T any](buckets map[string][]T, location Location, radius int, budget int, include func(T) bool) []T {
	var results []T
	if budget <= 0 {
		return results
	}
	baseChunkX := location.X >> 3
	baseChunkY := location.Y >> 3
	z := location.Z
	for ring := 0; ring <= radius; ring++ {
		for dx := -ring; dx <= ring; dx++ {
			for dy := -ring; dy <= ring; dy++ {
				// Only the newly-reached edge of this ring; inner
				// tiles already ran.
				if ring != 0 && dx != ring && dx != -ring && dy != ring && dy != -ring {
					continue
				}
				bucket := buckets[bucketKey(baseChunkX+dx, baseChunkY+dy, z)]
				if len(bucket) == 0 {
					continue
				}
				for _, entry := range bucket {
					if include != nil && !include(entry) {
						continue
					}
					results = append(results, entry)
					if len(results) >= budget {
						return results
					}
				}
			}
		}
	}
	return results
}

// ceilDiv8 returns ceil(distance / 8) for a non-negative distance, the
// integer form of TS Math.ceil(viewDistance / 8) (World.ts:691, 730).
// Distances are clamped to >= 1 by the Player setters, so the d <= 0
// branch only covers a zero-value Player; it yields radius 0.
func ceilDiv8(distance int) int {
	if distance <= 0 {
		return 0
	}
	return (distance + 7) / 8
}

// crossFloorVisible reports whether target is visible from viewer within
// range tiles. With no CrossFloor policy registered this is same-plane
// plus Chebyshev distance (CrossFloor.ts:22-45: visibleOnPlane is
// viewer == target without a policy, and distance is Chebyshev), which
// is exactly Location.IsWithinDistance.
func crossFloorVisible(viewer, target Location, distance int) bool {
	return viewer.IsWithinDistance(target, distance)
}

// staysInViewWhenPlaced reports whether a teleported NPC stays in view
// the tick it moves. World.ts:800-802 (staysInViewWhenPlaced): an exact
// move, a transparency fade, or a telejump keeps it. None of that state
// exists in Go (NPC resetUpdating notes the same gap), so this is ALWAYS
// false here: a needs-placement NPC is dropped and added back next tick.
// The call sites keep the TS `(!NeedsPlacement || staysInViewWhenPlaced)`
// shape so the behavior lands with the state that backs it.
func staysInViewWhenPlaced(_ *NPC) bool { return false }

// visibilityPlanes returns the planes bucket collection visits: only the
// origin plane. TS visits all four planes when the origin plane has a
// CrossFloor policy (World.ts:694, 732: includesOtherPlanes); no policy
// is registered in Go, so CrossFloor.includesOtherPlanes is always false
// (CrossFloor.ts:26-28) and the loop below runs exactly once. The loop
// is kept so a future policy plugs in here.
func visibilityPlanes(z int) []int { return []int{z} }

// occupiesPlayerSlot reports whether p still occupies its own slot in
// the world player list: the TS `World.getPlayers().get(index) ===
// candidate` check (World.ts:698, 764).
func (w *World) occupiesPlayerSlot(p *Player) bool {
	got, ok := w.players.Get(p.Index)
	return ok && got == p
}

// occupiesNPCSlot reports whether n still occupies its own slot in the
// world NPC list: the TS `World.getNpcs().get(index) === npc` check
// (World.ts:820, 840, 857).
func (w *World) occupiesNPCSlot(n *NPC) bool {
	got, ok := w.npcs.Get(n.Index)
	return ok && got == n
}

// NearbyPlayersForUpdate returns the bucket-collected player candidates
// for p's local-player rebuild, nearest-first. World.ts:685-705
// (getBucketNearbyPlayersForUpdate, via getNearbyPlayersForUpdate).
// Radius is max(0, ceil(viewDistance/8)) chunks, the budget is
// MaxLocalPlayers + MaxNewLocalPlayersPerCycle (280), and candidates
// must still be registered at the same index, and in the same sync
// area as p (World.ts:698: syncArea(candidate) === privateArea, where
// privateArea is syncArea(p); a deck counts as the main world, so
// aboard and ashore share the nil area). There is no sort: TS
// notes nearest-first collection is the tie-break (World.ts:686-687).
// Planes: own plane only (see
// visibilityPlanes).
//
// Tick-goroutine-only; the buckets must have been rebuilt this tick.
func (w *World) NearbyPlayersForUpdate(p *Player) []*Player {
	origin := w.rootLocationOf(&p.Mobile)
	privateArea := w.syncAreaOf(&p.Mobile)
	radius := max(0, ceilDiv8(p.ViewDistance()))
	budget := MaxLocalPlayers + MaxNewLocalPlayersPerCycle
	var found []*Player
	for _, z := range visibilityPlanes(origin.Z) {
		found = append(found, collectFromBuckets(w.playerUpdateBuckets,
			Location{X: origin.X, Y: origin.Y, Z: z}, radius, budget-len(found),
			func(candidate *Player) bool {
				return w.occupiesPlayerSlot(candidate) && w.syncAreaOf(&candidate.Mobile) == privateArea
			})...)
	}
	return found
}

// NearbyNPCsForUpdate returns the bucket-collected NPC candidates for
// p's local-NPC rebuild. World.ts:729-738 (getNearbyNpcsForUpdate).
// Radius is max(UpdateBucketRadius, ceil(npcViewDistance/8)) chunks, with
// no budget and no include filter (TS passes neither). The result is
// sorted same-plane-first then index ascending, as in TS (World.ts:734-
// 736); with a single plane that is index order, but the sort is kept
// for fidelity so a future CrossFloor policy keeps the TS ordering.
// Planes: own plane only (see visibilityPlanes).
//
// Tick-goroutine-only; the buckets must have been rebuilt this tick.
func (w *World) NearbyNPCsForUpdate(p *Player) []*NPC {
	radius := max(UpdateBucketRadius, ceilDiv8(p.NpcViewDistance()))
	origin := w.rootLocationOf(&p.Mobile)
	var nearby []*NPC
	for _, z := range visibilityPlanes(origin.Z) {
		nearby = append(nearby, collectFromBuckets(w.npcUpdateBuckets,
			Location{X: origin.X, Y: origin.Y, Z: z}, radius, math.MaxInt, nil)...)
	}
	sort.Slice(nearby, func(i, j int) bool {
		aOffPlane := w.rootLocationOf(&nearby[i].Mobile).Z != origin.Z
		bOffPlane := w.rootLocationOf(&nearby[j].Mobile).Z != origin.Z
		if aOffPlane != bOffPlane {
			return !aOffPlane
		}
		return nearby[i].Index < nearby[j].Index
	})
	return nearby
}

// UpdateLocalPlayers maintains p's local-player list, the list the
// per-tick sync path reads to know who to include in the player's view.
// World.ts:747-790 (updateLocalPlayers), in TS order:
//
//  1. In-place write-pointer compaction of the existing list, keeping
//     entries that are within the 255 cap, not p itself, deduplicated,
//     still registered at the same index, visible from p's root
//     location within the view distance, in the same sync area, and
//     visible aboard (World.ts:768-769: syncArea and canSeeAboard),
//     and not needing placement.
//  2. Additions from NearbyPlayersForUpdate, up to the 255 cap and
//     MaxNewLocalPlayersPerCycle (25) per cycle, applying the
//     visibility/area/aboard checks. Note the TS quirk preserved here:
//     the add loop has NO needs-placement check (World.ts:778-787),
//     unlike the compaction pass — so a player dropped by compaction
//     for needing placement is re-added by the add loop in the same
//     tick (consuming one of the 25 adds), as long as it is visible.
//  3. ResizeViewDistance with the resulting local count.
//
// TS reuses a static scratch Set for deduplication
// (World.ts:740: localPlayerScratch); Go allocates a fresh set per call
// — same semantics, no shared mutable state.
//
// A nil entry in the existing list panics, exactly as TS throws calling
// getIndex() on one (TS has no nil check in the compaction loop,
// unlike the add loop): nil entries cannot occur through production
// paths (buckets are built from the registered lists, which never hold
// nil), and in World.Process the per-player guard converts the panic
// into an error report plus a logout request, mirroring the TS
// try/catch (World.ts:1403-1413).
//
// Tick-goroutine-only; the buckets must have been rebuilt this tick.
func (w *World) UpdateLocalPlayers(p *Player) {
	locals := p.localPlayers
	origin := w.rootLocationOf(&p.Mobile)
	privateArea := w.syncAreaOf(&p.Mobile)
	viewDistance := p.ViewDistance()
	seen := make(map[int]bool, len(locals))

	// Compact in place rather than filter()+append, which would
	// allocate per player per cycle (World.ts:758-759).
	write := 0
	for read := 0; read < len(locals); read++ {
		local := locals[read]
		if write < MaxLocalPlayers &&
			local != p &&
			!seen[local.Index] &&
			w.occupiesPlayerSlot(local) &&
			crossFloorVisible(origin, w.rootLocationOf(&local.Mobile), viewDistance) &&
			w.syncAreaOf(&local.Mobile) == privateArea &&
			w.canSeeAboardBy(&p.Mobile, &local.Mobile) &&
			!local.NeedsPlacement {
			locals[write] = local
			write++
			seen[local.Index] = true
		}
	}
	// Drop references to compacted-out players, then truncate (TS:
	// localPlayers.length = write, World.ts:775).
	for i := write; i < len(locals); i++ {
		locals[i] = nil
	}
	locals = locals[:write]

	added := 0
	for _, candidate := range w.NearbyPlayersForUpdate(p) {
		if len(locals) >= MaxLocalPlayers || added >= MaxNewLocalPlayersPerCycle {
			break
		}
		if candidate == nil || candidate == p || seen[candidate.Index] {
			continue
		}
		if !crossFloorVisible(origin, w.rootLocationOf(&candidate.Mobile), viewDistance) {
			continue
		}
		if w.syncAreaOf(&candidate.Mobile) != privateArea || !w.canSeeAboardBy(&p.Mobile, &candidate.Mobile) {
			continue
		}
		locals = append(locals, candidate)
		seen[candidate.Index] = true
		added++
	}
	p.localPlayers = locals

	p.ResizeViewDistance(len(locals))
}

// UpdateLocalNPCs maintains p's local-NPC list from the nearby NPCs
// collected by NearbyNPCsForUpdate. World.ts:809-867 (updateLocalNpcs),
// in TS order:
//
//  1. Splice-removal pass over the existing list, keeping entries that
//     are within the 255 cap, deduplicated, still registered at the
//     same index, registered, visible, within the NPC view distance
//     of p's root location, in the same sync area, and visible
//     aboard (World.ts:825-826: syncArea and canSeeAboard), and
//     either not needing placement or staying in view when placed.
//     The TS
//     owner-only conjunct (World.ts:827) is OMITTED: NPC ownership
//     does not exist in Go, so no NPC can be owner-only.
//  2. Additions from nearby, up to the 255 cap, with the same checks
//     (in the TS add-loop order, World.ts:854-865).
//
// The pet special-case between the two passes (World.ts:836-852: keep
// the owner's active pet in the list) is OMITTED: there is no
// attributes system (no "pets:current" attribute), no pets, and no NPC
// ownership in Go.
//
// As in UpdateLocalPlayers, a nil entry in the existing list panics
// (TS has no nil check in this loop either); nil entries cannot occur
// through production paths, and the per-player guard in World.Process
// isolates the failure.
//
// Tick-goroutine-only; nearby must come from NearbyNPCsForUpdate for
// the same tick (World.Process collects it before UpdateLocalPlayers,
// as in TS, World.ts:1405-1409).
func (w *World) UpdateLocalNPCs(p *Player, nearby []*NPC) {
	locals := p.localNPCs
	viewDistance := p.NpcViewDistance()
	origin := w.rootLocationOf(&p.Mobile)
	privateArea := w.syncAreaOf(&p.Mobile)
	seen := make(map[int]bool, len(locals))
	for idx := 0; idx < len(locals); {
		npc := locals[idx]
		if idx < MaxLocalNPCs &&
			!seen[npc.Index] &&
			w.occupiesNPCSlot(npc) &&
			npc.IsRegistered() &&
			npc.Visible &&
			crossFloorVisible(origin, w.rootLocationOf(&npc.Mobile), viewDistance) &&
			w.syncAreaOf(&npc.Mobile) == privateArea &&
			w.canSeeAboardBy(&p.Mobile, &npc.Mobile) &&
			(!npc.NeedsPlacement || staysInViewWhenPlaced(npc)) {
			// The owner-only check (World.ts:827) is omitted:
			// NPC ownership does not exist in Go.
			seen[npc.Index] = true
			idx++
		} else {
			locals = spliceOut(locals, idx)
		}
	}

	// Pet special-case omitted (World.ts:836-852): no attributes/pets
	// system and no NPC ownership in Go. See the doc comment above.

	for _, npc := range nearby {
		if len(locals) >= MaxLocalNPCs {
			break
		}
		if npc == nil || seen[npc.Index] || !npc.Visible {
			continue
		}
		if !npc.IsRegistered() || !w.occupiesNPCSlot(npc) {
			continue
		}
		if npc.NeedsPlacement && !staysInViewWhenPlaced(npc) {
			continue
		}
		if w.syncAreaOf(&npc.Mobile) != privateArea || !w.canSeeAboardBy(&p.Mobile, &npc.Mobile) {
			continue
		}
		// The owner-only check
		// (World.ts:861) is omitted: no NPC ownership in Go.
		if !crossFloorVisible(origin, w.rootLocationOf(&npc.Mobile), viewDistance) {
			continue
		}
		locals = append(locals, npc)
		seen[npc.Index] = true
	}
	p.localNPCs = locals
}

// updateVisibilityPlayers runs the update_players_npcs phase when no
// hook overrides it: for each visibility player, collect nearby NPCs,
// then update the local player and NPC lists, with per-player failure
// isolation. World.ts:1403-1413 (update_players_npcs phase) with
// forEachVisibilityPlayer (World.ts:915-919).
//
// The visibility gate is Session != nil && Session.IsOpen(), mirroring
// the existing flushPlayers gate. TS shouldRunVisibilityUpdates
// (World.ts:909-913) delegates to shouldRunNetworkUpdates
// (World.ts:921-935), which additionally includes bots under the
// STRESS_BOT_UPDATES flag and excludes bots without a network client
// via hasNetworkClient. Neither nuance is ported: there is no stress
// flag in Go, and a bot with an administrator-driven client has an
// open session, so the session gate covers the hasNetworkClient case
// the same way flushPlayers does. Bots without sessions get no
// visibility maintenance, matching TS.
func (w *World) updateVisibilityPlayers() {
	w.players.ForEach(func(p *Player) {
		if p.Session == nil || !p.Session.IsOpen() {
			return
		}
		w.updateVisibilityPlayerGuarded(p)
	})
}

// updateVisibilityPlayerGuarded runs one player's visibility update,
// recovering a panic into an OnPlayerError report plus a logout request
// without aborting other players — mirroring flushPlayerGuarded/
// processPlayerGuarded and the TS per-player try/catch (World.ts:1404-
// 1412: catch logs "[World] Player/NPC updating failure" and calls
// requestLogout). Nearby NPCs are collected before the local-player
// update, as in TS (World.ts:1405-1409).
func (w *World) updateVisibilityPlayerGuarded(p *Player) {
	defer func() {
		if r := recover(); r != nil {
			w.playerError(p, "update_players_npcs", fmt.Errorf("panic: %v", r))
			p.RequestLogout()
		}
	}()
	nearby := w.NearbyNPCsForUpdate(p)
	w.UpdateLocalPlayers(p)
	w.UpdateLocalNPCs(p, nearby)
}
