package definition

import (
	"tsps-go/internal/cache"
)

// ObjectDefinition is the cache-backed object wrapper
// (ObjectDefinition.ts). Instances are built by Store.Object; each
// call returns a fresh value. There is no DEFAULT shape: TS forId
// returns undefined out of range, and Store.Object reports that
// with false. (ObjectIdentifiers, the TS superclass, is a
// constants-only table of object ids; it is not ported here.)
type ObjectDefinition struct {
	id                 int
	name               string
	description        string
	clipType           int
	sizeX              int
	sizeY              int
	projectileBlocking bool
	blockingMask       int
	interactive        bool
	interactions       []string
	minimapFunction    int
	multiLoc           bool
	examineText        string
}

// Object returns the object definition for an id, or false when
// the id is out of range or there is no cache (ObjectDefinition.
// forId, ObjectDefinition.ts:58-66). TS gates on totalObjects,
// set by init() from the cache counts; with no cache that is 0,
// so every id is out of range — the nil-cache degradation here.
func (s *Store) Object(id int) (*ObjectDefinition, bool) {
	if s.cache == nil || id < 0 || id >= s.objectCount() {
		return nil, false
	}
	cached := s.cache.GetObject(id)
	def := &ObjectDefinition{
		id:                 id,
		name:               cached.Name,
		clipType:           cached.ClipType,
		sizeX:              cached.SizeX,
		sizeY:              cached.SizeY,
		projectileBlocking: cached.BlocksProjectile,
		blockingMask:       cached.ClipMask,
		interactive:        cached.IsInteractive == 1,
		minimapFunction:    cached.MapFunctionId,
		multiLoc:           len(cached.Transforms) > 0,
		examineText:        s.objectExamines[id],
	}
	if cached.Desc != nil {
		def.description = *cached.Desc
	}
	// interactions is null when no action is set: some(Boolean)
	// over the actions, which are "" when absent.
	for _, action := range cached.Actions {
		if action != "" {
			def.interactions = append([]string(nil), cached.Actions...)
			break
		}
	}
	return def, true
}

// ObjectExamineText reports the recorded examine text for an id:
// the Wiki/overrides text from object-examines.json and
// object-examine-overrides.json (ObjectDefinition.examineText,
// ObjectDefinition.ts:112-119, which returns null when the id has
// no recorded text). The boolean distinguishes "recorded" from
// "absent": the examine dispatch rule keys on null-ness, not on
// the text itself (a multi-loc's text may live under the variant
// the player sees). Unlike Examine, this never falls back to the
// cache description or a generic line.
func (s *Store) ObjectExamineText(id int) (string, bool) {
	text, ok := s.objectExamines[id]
	return text, ok
}

// ObjectForPlayer resolves the same per-player loc variant the
// client displays (ObjectDefinition.forPlayer,
// ObjectDefinition.ts:76-88). A non-multi-loc resolves to itself.
// The variant index comes from the transform varbit, else the
// transform varp, else -1; an in-range index selects that
// transform, anything else selects the last (default) transform,
// and a resolved id of -1 yields false. Note this deliberately
// does NOT use LocType.Transform: that method falls through to
// the default when an in-range var names -1, while forPlayer
// returns null in that case.
func (s *Store) ObjectForPlayer(id int, vars cache.VarReader) (*ObjectDefinition, bool) {
	def, ok := s.Object(id)
	if !ok || !def.multiLoc {
		return def, ok
	}
	cached := s.cache.GetObject(id)
	index := -1
	// TS always has a player here; a nil reader is a Go-only
	// degradation meaning "no vars set", leaving index at -1 so
	// the default (last) transform is used.
	if vars != nil {
		if cached.TransformVarbit != -1 {
			index = vars.GetVarbit(cached.TransformVarbit)
		} else if cached.TransformVarp != -1 {
			index = vars.GetVarp(cached.TransformVarp)
		}
	}
	resolved := cached.Transforms[len(cached.Transforms)-1]
	if index >= 0 && index < len(cached.Transforms)-1 {
		resolved = cached.Transforms[index]
	}
	if resolved == -1 {
		return nil, false
	}
	return s.Object(resolved)
}

// ID returns the object id.
func (d *ObjectDefinition) ID() int { return d.id }

// Name returns the object name.
func (d *ObjectDefinition) Name() string { return d.name }

// Description returns the cache desc, or "" when the cache has
// none.
func (d *ObjectDefinition) Description() string { return d.description }

// ClipType returns the clip type.
func (d *ObjectDefinition) ClipType() int { return d.clipType }

// Examine returns the examine text: the recorded text from
// object-examines.json / object-examine-overrides.json (overrides
// win), else the cache description, else a generic line
// (ObjectDefinition.getExamine, ObjectDefinition.ts:93-99).
// TypeScript reads the examine files lazily on first use and
// throws when either is missing; this package reads them eagerly
// at Load and treats a missing file as empty — see
// loadExaminesAndSounds.
func (d *ObjectDefinition) Examine() string {
	if d.examineText != "" {
		return d.examineText
	}
	if d.description != "" {
		return d.description
	}
	if d.name == "" || d.name == "null" {
		return "It's nothing special."
	}
	return "It's " + anOrA(d.name) + " " + d.name + "."
}

// SizeX returns the footprint size in x.
func (d *ObjectDefinition) SizeX() int { return d.sizeX }

// SizeY returns the footprint size in y.
func (d *ObjectDefinition) SizeY() int { return d.sizeY }

// HasActions reports whether the object is interactive.
func (d *ObjectDefinition) HasActions() bool { return d.interactive }

// IsSolid reports whether the object blocks movement
// (clipType != 0).
func (d *ObjectDefinition) IsSolid() bool { return d.clipType != 0 }

// IsClippedDecoration reports whether the object is a floor
// decoration (shape 22) that blocks movement only when its
// blockWalk is 1 (ObjectDefinition.ts:68-72).
func (d *ObjectDefinition) IsClippedDecoration() bool { return d.clipType == 1 }

// IsImpenetrable reports whether the object blocks projectiles.
func (d *ObjectDefinition) IsImpenetrable() bool { return d.projectileBlocking }

// BlockingMask returns the blocking mask.
func (d *ObjectDefinition) BlockingMask() int { return d.blockingMask }

// Interactions returns the menu interactions, or nil when the
// object has none.
func (d *ObjectDefinition) Interactions() []string {
	if d.interactions == nil {
		return nil
	}
	return append([]string(nil), d.interactions...)
}

// Actions is an alias for Interactions: plugins call the cache's
// own field name.
func (d *ObjectDefinition) Actions() []string { return d.Interactions() }

// MinimapFunction returns the minimap function id.
func (d *ObjectDefinition) MinimapFunction() int { return d.minimapFunction }

// IsMultiLoc reports whether what a player sees depends on a
// varbit/varp.
func (d *ObjectDefinition) IsMultiLoc() bool { return d.multiLoc }

// barrowsStaircaseSize2 are the Barrows crypt staircases whose
// size is 2 (ObjectDefinition.ts:139-140).
var barrowsStaircaseSize2 = map[int]bool{20667: true, 20668: true, 20669: true, 20670: true, 20672: true}

// barrowsStaircaseSize3 is Torag's staircase (20671), whose size
// is 3.
const barrowsStaircaseSize3 = 20671

// Size returns the object's size: the Barrows crypt staircases
// are special-cased (the cache names all six generically
// "Staircase"), otherwise sizeX + sizeY - 1.
func (d *ObjectDefinition) Size() int {
	if d.id == barrowsStaircaseSize3 {
		return 3
	}
	if barrowsStaircaseSize2[d.id] {
		return 2
	}
	return d.sizeX + d.sizeY - 1
}
