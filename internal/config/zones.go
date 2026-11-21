package config

// Zone boundaries: the WORLD_ZONE_BOUNDARIES view over the
// already-parsed world zones (WorldDefinition.ts:229-242,
// 274-286). This file adds a read-only view only; it does not
// change zone parsing (parseWorldZone in world.go) or any
// other parsing behavior.
//
// TS builds WORLD_ZONE_BOUNDARIES at module load in
// syncRuntime(): for each zone, zoneBoundaries yields either
// one boundary (a bounded zone, carrying the zone's z) or
// four full-map boundaries, one per plane (a zone without
// bounds); then, for each of the zone's tags except "f2p",
// those boundaries are appended to that tag's list.
// WORLD_ZONE_BOUNDARIES is pre-seeded with five keys (duel,
// pvp, safe, multi-combat, all-buildings-safe), and any other
// tag encountered gains an entry (the `??=` at
// WorldDefinition.ts:282). "f2p" zones are EXCLUDED: free
// land is matched on x/y across all planes via the separate
// free-land index (IsMembersArea), so single-plane Boundary
// objects for f2p zones "would only mislead"
// (WorldDefinition.ts:280-281).
//
// ZoneBoundaries reproduces exactly that. It returns plain
// data — ints plus a tag key — because the consumer,
// internal/game, must never import this package: the
// coordinator's cmd/server adapter converts each
// ZoneBoundary to a game.Boundary with
// game.NewBoundary(b.MinX, b.MaxX, b.MinY, b.MaxY, b.Z) and
// serves the map through the game's ZoneBoundarySource seam.
// The conversion is total: every ZoneBoundary carries a Z
// (bounded zones require z at parse time — plane() in
// WorldDefinition.ts:118-124, planeField here — and unbounded
// zones expand to explicit per-plane boundaries), so no
// height-less boundary can arise from this view.

// ZoneBoundary is one zone boundary as plain data: the
// closed rectangle [MinX, MaxX] x [MinY, MaxY] on plane Z.
// It is the config-side shape of game.Boundary; see the file
// comment for the adapter.
type ZoneBoundary struct {
	MinX, MaxX, MinY, MaxY, Z int
}

// zoneBoundarySeedTags are the WORLD_ZONE_BOUNDARIES keys
// pre-seeded before any zone is processed
// (WorldDefinition.ts:229-235). They are always present in
// the ZoneBoundaries result, even when no zone carries the
// tag.
var zoneBoundarySeedTags = []string{
	"duel",
	"pvp",
	"safe",
	"multi-combat",
	"all-buildings-safe",
}

// ZoneBoundaries returns the world zone boundaries grouped
// by tag, per WORLD_ZONE_BOUNDARIES semantics (see the file
// comment). The map always carries the five seeded tags;
// tags from zones appear in first-seen order only in the
// sense that map iteration is unordered — but each tag's
// LIST preserves zone order, and an unbounded zone's four
// per-plane boundaries appear in plane order 0..3, as TS
// appends them. The result is freshly built on each call;
// mutating it does not affect the World. w must be non-nil.
func (w *World) ZoneBoundaries() map[string][]ZoneBoundary {
	out := make(map[string][]ZoneBoundary, len(zoneBoundarySeedTags))
	for _, tag := range zoneBoundarySeedTags {
		out[tag] = nil
	}
	for _, zone := range w.Zones {
		boundaries := zoneBoundaries(zone)
		for _, tag := range zone.Tags {
			// "f2p" zones are matched via the free-land
			// index instead (WorldDefinition.ts:280-282);
			// they contribute no boundaries to any tag.
			if tag == "f2p" {
				continue
			}
			out[tag] = append(out[tag], boundaries...)
		}
	}
	return out
}

// zoneBoundaries mirrors zoneBoundaries
// (WorldDefinition.ts:237-242): a zone without bounds covers
// the whole map on every plane (0..0x3fff, z 0..3); a bounded
// zone yields its single rectangle on its own plane. A zone
// is unbounded exactly when no bound key was present at
// parse time (Zone.Bounded); a bounded zone always carries
// all five coordinates, since parseWorldZone requires each
// one whenever any is present.
func zoneBoundaries(zone Zone) []ZoneBoundary {
	if !zone.Bounded {
		return []ZoneBoundary{
			{MinX: 0, MaxX: 0x3fff, MinY: 0, MaxY: 0x3fff, Z: 0},
			{MinX: 0, MaxX: 0x3fff, MinY: 0, MaxY: 0x3fff, Z: 1},
			{MinX: 0, MaxX: 0x3fff, MinY: 0, MaxY: 0x3fff, Z: 2},
			{MinX: 0, MaxX: 0x3fff, MinY: 0, MaxY: 0x3fff, Z: 3},
		}
	}
	return []ZoneBoundary{
		{MinX: zone.MinX, MaxX: zone.MaxX, MinY: zone.MinY, MaxY: zone.MaxY, Z: zone.Z},
	}
}
