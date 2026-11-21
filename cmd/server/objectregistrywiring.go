package main

import (
	"tsps-go/internal/cache"
	"tsps-go/internal/definition"
	"tsps-go/internal/game"
)

// Object-registry seam wiring: installs the definition-backed
// seams on the world's object registry at boot (see
// internal/game/objects.go). The registry resolves loc variants
// through ObjectDefinition.forPlayer and gates its developer
// notice on the carried-save rights id; both seams are pure
// lookups that run under the registry mutex, so they must never
// call back into the registry.
//
// The vars seam (SetVarsSource) is deliberately NOT installed:
// the varp store is per-session (phase 18) while the registry
// resolves variants world-wide, so variant resolution runs with
// a nil reader and resolves multi-loc variants to the default
// (last) transform — the documented Store degradation, shared
// with the examine and listener seams. Wiring a per-player
// reader into the registry is a separate decision.

// storeVariantSource implements game.ObjectVariantSource over a
// *definition.Store: ObjectDefinition.forPlayer reduced to ids
// (ObjectDefinition.ts:81-93, via Store.ObjectForPlayer).
type storeVariantSource struct {
	store *definition.Store
}

var _ game.ObjectVariantSource = storeVariantSource{}

// objectVarReader adapts a game.ObjectVarReader to
// cache.VarReader (identical method sets, separate packages to
// keep internal/game cache-free).
type objectVarReader struct {
	vars game.ObjectVarReader
}

var _ cache.VarReader = objectVarReader{}

func (r objectVarReader) GetVarp(id int) int   { return r.vars.GetVarp(id) }
func (r objectVarReader) GetVarbit(id int) int { return r.vars.GetVarbit(id) }

// ObjectVariantForPlayer resolves the variant id the player
// sees, or false when forPlayer returns null (resolved
// transform -1, or no definition). A nil vars reader means
// "no vars set" and resolves to the default transform.
func (s storeVariantSource) ObjectVariantForPlayer(id int, vars game.ObjectVarReader) (int, bool) {
	var reader cache.VarReader
	if vars != nil {
		reader = objectVarReader{vars: vars}
	}
	def, ok := s.store.ObjectForPlayer(id, reader)
	if !ok {
		return 0, false
	}
	return def.ID(), true
}

// installObjectRegistrySources installs the variant and rights
// seams on world's registry. A nil world or registry installs
// nothing; a nil store leaves identity variant resolution; a
// nil rights source leaves every player reading as RightsNone.
// Call only before the engine starts (single-threaded setup).
func installObjectRegistrySources(world *game.World, store *definition.Store, rights func(*game.Player) int) {
	if world == nil {
		return
	}
	reg := world.GameObjects()
	if reg == nil {
		return
	}
	if store != nil {
		reg.SetVariantSource(storeVariantSource{store: store})
	}
	if rights != nil {
		reg.SetRightsSource(rights)
	}
}
