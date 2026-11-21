package game

// Boundary is game/model/Boundary.ts: a closed x/y rectangle on one
// height level, used by AreaManager area lookups and by the
// Wilderness zone checks.
//
// HEIGHT SEMANTICS (read carefully — the TS behavior is a trap).
// The TS constructor takes an OPTIONAL height, and inside() tests
// it with LOOSE equality: `this.height == p.getZ()`
// (Boundary.ts:34). In JavaScript, `undefined == <number>` is
// always false, so a boundary constructed WITHOUT a height
// matches NO location, ever — not even plane 0. That is the
// faithful behavior, and this port reproduces it: a Boundary
// carries an explicit has-height flag, and Inside returns false
// on the height term whenever the flag is unset.
//
// In practice every boundary carries a height: world zones
// always carry z (parseWorldZone requires it whenever any bound
// key is present, WorldDefinition.ts:137-149, and unbounded
// zones expand to one boundary per plane, :237-242), and every
// area-constructed boundary sampled at commit bfd81e3 passes a
// height (TemplatedInstanceArea.ts:108-109, BoatDeckArea.ts:25,
// and all sampled plugin areas). A height-less boundary is
// therefore inert by construction, never "any plane". Do not
// "fix" Inside to treat an unset height as a wildcard: TS does
// not, and the AreaManager search index routes height-less
// boundaries to its fallback path on exactly that basis
// (AreaManager.ts:238-247, `!Number.isFinite(z)`).
//
// Boundary is a value type. TS exposes x/x2/y/y2 through
// getters and height as a public (but never reassigned — the
// only write is the constructor, Boundary.ts:14) field; here
// all fields are unexported and read through getters, and
// there is no setter: construct a new Boundary instead.
type Boundary struct {
	x, x2, y, y2 int
	height       int
	hasHeight    bool
}

// NewBoundary returns the boundary spanning x..x2 and y..y2
// inclusive on the given height level, mirroring the TS
// constructor (Boundary.ts:9-15). The height is optional, as
// in TS: pass exactly one height to set it, or none for a
// height-less (inert — see the type comment) boundary. Passing
// more than one height panics: TS has a single optional
// parameter, and silently ignoring extras would hide a
// call-site mistake.
func NewBoundary(x, x2, y, y2 int, height ...int) Boundary {
	if len(height) > 1 {
		panic("game: NewBoundary takes at most one height")
	}
	b := Boundary{x: x, x2: x2, y: y, y2: y2}
	if len(height) == 1 {
		b.height = height[0]
		b.hasHeight = true
	}
	return b
}

// X returns the minimum x (Boundary.ts:17-19, getX).
func (b Boundary) X() int { return b.x }

// X2 returns the maximum x (Boundary.ts:21-23, getX2).
func (b Boundary) X2() int { return b.x2 }

// Y returns the minimum y (Boundary.ts:25-27, getY).
func (b Boundary) Y() int { return b.y }

// Y2 returns the maximum y (Boundary.ts:29-31, getY2).
func (b Boundary) Y2() int { return b.y2 }

// HasHeight reports whether the boundary was constructed with
// a height level. A boundary without one matches nothing (see
// the type comment).
func (b Boundary) HasHeight() bool { return b.hasHeight }

// Height returns the boundary's height level, or 0 when it has
// none. The 0 default is the `boundary.height ?? 0` reading
// Wilderness.intersectsArea uses (Wilderness.ts:82); callers
// that need to distinguish "plane 0" from "no height" must
// check HasHeight first.
func (b Boundary) Height() int { return b.height }

// Inside reports whether p lies within the boundary: x in
// [x, x2], y in [y, y2], and the height equal to p's level
// (Boundary.ts:33-35). A height-less boundary never matches
// (see the type comment).
func (b Boundary) Inside(p Location) bool {
	return p.X >= b.x && p.X <= b.x2 && p.Y >= b.y && p.Y <= b.y2 &&
		b.hasHeight && b.height == p.Z
}
