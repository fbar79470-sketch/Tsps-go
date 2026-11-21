package main

import (
	"tsps-go/internal/config"
	"tsps-go/internal/game"
	"tsps-go/internal/plugins"
	"tsps-go/internal/session"
)

// Area + Wilderness wiring: the cmd/server adapter between the
// loaded world definition, the game layer's AreaManager /
// Wilderness, and the session layer's music-track override.
//
// TS keeps both systems as statics: AreaManager.areas (pushed by
// api.registerArea) and WORLD_ZONE_BOUNDARIES (built from
// world.json zones at module load). Go constructs one
// AreaManager and one Wilderness per world in buildServer,
// installs the Wilderness on the manager (the AreaManager
// InMulti fallback), installs the manager on the World (players
// and NPCs read it through the areaManager field World installs
// on add), and hands the Wilderness to the drop listener
// through the Deps seam. Areas register through the hub's
// registerArea (see pluginwiring.go), which pushes onto the
// world's manager. Nothing about areas or timers persists.
//
// The game layer never imports config: the zone boundaries
// arrive through the game.ZoneBoundarySource seam, adapted here
// from the loaded *config.World (see internal/config
// ZoneBoundaries, which reproduces the WORLD_ZONE_BOUNDARIES
// build rules).

// zoneBoundarySource adapts a loaded *config.World to
// game.ZoneBoundarySource: tag -> []game.Boundary, in zone
// order, with unbounded zones already expanded to four
// per-plane boundaries by the config view.
type zoneBoundarySource struct {
	boundaries map[string][]config.ZoneBoundary
}

// newZoneBoundarySource snapshots the world's zone boundaries.
// A nil world yields an empty source: no PvP ground, no safe
// zones, no multi-combat (the empty-world reading).
func newZoneBoundarySource(worldDef *config.World) *zoneBoundarySource {
	if worldDef == nil {
		return &zoneBoundarySource{}
	}
	return &zoneBoundarySource{boundaries: worldDef.ZoneBoundaries()}
}

// ZoneBoundaries returns the boundaries tagged tag, converted
// to game boundaries. An unknown tag yields none.
func (s *zoneBoundarySource) ZoneBoundaries(tag string) []game.Boundary {
	raw := s.boundaries[tag]
	out := make([]game.Boundary, len(raw))
	for i, b := range raw {
		out[i] = game.NewBoundary(b.MinX, b.MaxX, b.MinY, b.MaxY, b.Z)
	}
	return out
}

// newAreaManagerAndWilderness constructs the world's AreaManager
// and Wilderness from the loaded world definition and region
// manager: the Wilderness reads zone boundaries through the
// config adapter and roof state through RegionRoofChecker, and
// the manager falls back to it for multi-combat. Call before
// the engine starts; the manager is installed on the World by
// the caller (see buildServer), and the Wilderness is kept by
// the adapter for the drop-listener seam.
func newAreaManagerAndWilderness(worldDef *config.World, rm *game.RegionManager) (*game.AreaManager, *game.Wilderness) {
	wilderness := game.NewWilderness(newZoneBoundarySource(worldDef), game.RegionRoofChecker(rm))
	manager := game.NewAreaManager()
	manager.SetWilderness(wilderness)
	return manager, wilderness
}

// installAreaMusicSource installs the area music-track override
// on an entering session: every flush, the music block asks the
// player's current area for a track (PlayerSession.ts:393-395,
// via callArea), and a valid track replaces the music-area
// track list. The closure captures the player; it runs on the
// tick goroutine inside FlushTick, so reading the player's
// area reference races nothing. A nil area yields no opinion
// (see game.CallAreaMusicTrack).
func installAreaMusicSource(sess *session.Session, p *game.Player) {
	sess.SetMusicAreaTrackSource(func() (int, bool) {
		return game.CallAreaMusicTrack(p.Area(), p)
	})
}

// areaRegistryAdapter adapts a *game.AreaManager to the hub's
// plugins.AreaRegistry surface (the ObjectManagerAPI precedent:
// the adapter lives here, in the package that imports both
// sides, and game never imports plugins). Api.RegisterArea
// pushes content areas onto the world's manager through it.
type areaRegistryAdapter struct {
	manager *game.AreaManager
}

var _ plugins.AreaRegistry = areaRegistryAdapter{}

// Register appends area to the world's area registry, in
// registration order (AreaManager priority order).
func (a areaRegistryAdapter) Register(area *game.Area) {
	a.manager.Register(area)
}

// NewAreaRegistry returns the hub's AreaRegistry surface over
// manager, for Hub.SetAreaRegistry at boot. Installing it
// flushes any areas queued during plugin loading, in
// registration order (see Hub.SetAreaRegistry).
func NewAreaRegistry(manager *game.AreaManager) plugins.AreaRegistry {
	return areaRegistryAdapter{manager: manager}
}
