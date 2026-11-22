// Package doors is the Go port of the door mechanism in
// server/plugins/objects/Doors.plugin.js (upstream HEAD bfd81e3):
// the startup door catalog (buildDoorCatalog), the transcribed
// exception tables, and the accessors Worker B's toggle engine
// builds against. The catalog itself is derived from the cache at
// startup, exactly as upstream; the tables below hold only the
// pairs no heuristic finds, with the numeric ids transcribed from
// the read-only ObjectIdentifiers.ts (see the per-table citations).
package doors

import (
	"fmt"
	"sort"
	"strings"

	"tsps-go/internal/cache"
)

// CatalogDefs is the cache surface buildDoorCatalog needs. It is a
// narrow interface over the cache definitions so the catalog is a
// pure function of its inputs and unit-testable with fixture defs.
type CatalogDefs interface {
	// GetObject returns the loc definition for id, or nil.
	GetObject(id int) *cache.LocType
	// ObjectCount is the number of object definitions in the cache.
	ObjectCount() int
	// LocName returns the gameval name for a loc id, or "" when the
	// cache names none.
	LocName(id int) string
	// LocID resolves a gameval name to its loc id.
	LocID(name string) (int, bool)
}

// WoodenGate is one OSRS wooden gate: a hinge panel and an extension
// panel that pivot together around the hinge post; opening/closing
// moves both pieces. Doors.plugin.js:247-266 (WOODEN_GATES).
type WoodenGate struct {
	ClosedHinge     int
	ClosedExtension int
	OpenHinge       int
	OpenExtension   int
}

// CompositeLeaf is one leaf of a composite opening: [id, dx, dy,
// face] relative to the closed loc's tile.
// Doors.plugin.js:225-234 (COMPOSITE_DOOR_OPENINGS).
type CompositeLeaf struct {
	ID   int
	DX   int
	DY   int
	Face int
}

// PassThroughLeaf is one leaf of a pass-through gate: [id, x, y, z,
// face] of a wall loc, with Shape naming another shape when the TS
// entry carries a sixth value (shape 10 centrepieces, shape 9
// diagonal leaves); Shape is 0 otherwise.
// Doors.plugin.js:427-860 (PASS_THROUGH_GATES).
type PassThroughLeaf struct {
	ID    int
	X     int
	Y     int
	Z     int
	Face  int
	Shape int
}

// PassThroughGate is one gate that lets the player through and shuts
// behind them, never staying open. crossX/crossY is the boundary the
// player crosses; crossBetween with crossRows is a gate more than one
// tile thick whose tiles the map blocks (the player is carried
// across, from the low side to the high side or back, along their own
// row); crossDiagonal is a diagonal door crossed corner to corner, and
// CrossDiagonalEastward selects the eastwardFrom side test
// (here.x-here.y <= closed.x-closed.y) instead of the x test.
// OpenSoundID 0 means TS sets no sound for the gate.
// Doors.plugin.js:427-860 (PASS_THROUGH_GATES).
type PassThroughGate struct {
	Closed []PassThroughLeaf
	Opened []PassThroughLeaf

	CrossX               int
	HasCrossX            bool
	CrossY               int
	HasCrossY            bool
	CrossBetween         [2]int
	HasCrossBetween      bool
	CrossRows            []int
	CrossDiagonalOutside [2]int
	CrossDiagonalInside  [2]int
	HasCrossDiagonal     bool
	// CrossDiagonalEastward: the side comes from x-y, not x alone.
	CrossDiagonalEastward bool

	WalkOver    bool
	Carry       bool
	CloseTicks  int
	OpenSoundID int
}

// PassThroughFamily is a pass-through door placed many times: every
// placement of the door works like a PASS_THROUGH_GATES entry built
// from its own tile and face. OpenLeaf is the explicit open leaf id;
// when HasOpenLeaf is false the open leaf is the cache's `<name>_open`
// loc, resolved from the gameval name at runtime.
// Doors.plugin.js:871-889 (PASS_THROUGH_FAMILIES).
type PassThroughFamily struct {
	OpenLeaf    int
	HasOpenLeaf bool
	OpenSoundID int
	CloseTicks  int
}

// Catalog is the startup door catalog: every closed door paired with
// its open leaf, plus the transcribed exception tables. ClosedToOpen
// maps a closed id to its open id (self-opening doors map to
// themselves); OpenToClosed is the reverse map, first wins for
// shared leaves.
type Catalog struct {
	ClosedToOpen map[int]int
	OpenToClosed map[int]int

	selfOpening           map[int]bool
	woodenGates           map[int]*WoodenGate
	doubleDoorFamilies    map[int][]int
	doubleDoorPartners    map[int][]int
	specialDoubleDoorOpen map[int]int
	passThrough           map[passThroughKey]*PassThroughGate
	passThroughFamilies   map[int]*PassThroughFamily
	inPlace               map[int]bool
	ownTile               map[int]bool
	composite             map[int][]CompositeLeaf
	dorgesh               map[int]bool
}

type passThroughKey struct {
	id, x, y, z int
}

// DoorNameList returns the display names the catalog scans for, in a
// stable order. Doors.plugin.js:30-52 (DOOR_NAMES).
func DoorNameList() []string {
	names := make([]string, 0, len(DoorNames))
	for name := range DoorNames {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// BuildCatalog scans every loc definition and pairs each closed door
// with its open leaf, following buildDoorCatalog in
// Doors.plugin.js:1056-1180, in TS order: wooden-gate skip,
// self-opening set, gameval-named open leaf, classic id+1, same-model
// consecutive Open-only leaves (double-door registration), id+2
// skipped-actionless, id+3 interleaved pairs, then the
// SINGLE_DOOR_OPEN_IDS overrides, the same-model double-door
// registration, and the mirrored-leaf double-door detection over the
// paired map.
func BuildCatalog(defs CatalogDefs) (*Catalog, error) {
	if defs == nil {
		return nil, fmt.Errorf("doors: nil CatalogDefs")
	}
	c := &Catalog{
		ClosedToOpen:          map[int]int{},
		OpenToClosed:          map[int]int{},
		selfOpening:           map[int]bool{},
		woodenGates:           map[int]*WoodenGate{},
		doubleDoorFamilies:    map[int][]int{},
		doubleDoorPartners:    map[int][]int{},
		specialDoubleDoorOpen: map[int]int{},
		passThrough:           map[passThroughKey]*PassThroughGate{},
		passThroughFamilies:   map[int]*PassThroughFamily{},
		inPlace:               map[int]bool{},
		ownTile:               map[int]bool{},
		composite:             map[int][]CompositeLeaf{},
		dorgesh:               map[int]bool{},
	}
	for id := range selfOpeningDoorIDs {
		c.selfOpening[id] = true
	}
	for i := range woodenGates {
		gate := woodenGates[i]
		// Open panels can be shared (the survival gate opens into
		// 8812/8813); the first gate keeps them.
		for _, id := range []int{gate.ClosedHinge, gate.ClosedExtension, gate.OpenHinge, gate.OpenExtension} {
			if _, ok := c.woodenGates[id]; !ok {
				g := gate
				c.woodenGates[id] = &g
			}
		}
	}
	for _, family := range doubleDoorIDFamilies {
		ids := append([]int(nil), family...)
		for _, id := range ids {
			c.doubleDoorFamilies[id] = ids
		}
	}
	for _, pair := range specialDoubleDoorPairs {
		a, b := pair[0], pair[1]
		c.doubleDoorPartners[a] = append(c.doubleDoorPartners[a], b)
		c.doubleDoorPartners[b] = append(c.doubleDoorPartners[b], a)
	}
	for closed, open := range specialDoubleDoorOpenIDsByClosedID {
		c.specialDoubleDoorOpen[closed] = open
	}
	for _, gate := range passThroughGates {
		for _, leaf := range gate.Closed {
			c.passThrough[passThroughKey{leaf.ID, leaf.X, leaf.Y, leaf.Z}] = gate
		}
	}
	for id, family := range passThroughFamilies {
		c.passThroughFamilies[id] = family
	}
	for id := range inPlaceDoorIDs {
		c.inPlace[id] = true
	}
	for id := range ownTileDoorIDs {
		c.ownTile[id] = true
	}
	for id, leaves := range compositeDoorOpenings {
		c.composite[id] = append([]CompositeLeaf(nil), leaves...)
	}
	for id := range dorgeshCityDoorIDs {
		c.dorgesh[id] = true
	}

	total := defs.ObjectCount()
	var sameModelPairs []int
	for id := 0; id < total; id++ {
		def := defs.GetObject(id)
		if def == nil || !DoorNames[def.Name] || !hasAction(def.Actions, "open") {
			continue
		}
		// Wooden gates are tracked by WOODEN_GATES, never by the
		// generic pairing heuristics.
		if _, ok := c.woodenGates[id]; ok {
			continue
		}
		if c.selfOpening[id] {
			c.ClosedToOpen[id] = id
			continue
		}
		// "_reverse" doors open on their own tile.
		if strings.HasSuffix(defs.LocName(id), "_reverse") {
			c.ownTile[id] = true
		}
		namedOpenID, namedOpenOK := namedOpenLeaf(defs, id)
		namedOpen := defs.GetObject(namedOpenID)
		if !isSameModelLeafPair(defs, id, id+1) && !isSameModelLeafPair(defs, id-1, id) &&
			!c.hasDoubleDoorFamily(id) && !c.hasDoubleDoorFamily(namedOpenID) &&
			namedOpenOK && namedOpen != nil && namedOpen.Name == def.Name &&
			hasAction(namedOpen.Actions, "close") && !hasAction(namedOpen.Actions, "open") {
			c.ClosedToOpen[id] = namedOpenID
			c.OpenToClosed[namedOpenID] = id
			continue
		}
		partner := defs.GetObject(id + 1)
		if partner == nil || partner.Name != def.Name {
			continue
		}
		if hasAction(partner.Actions, "close") {
			// The open variant only closes back (the classic
			// closed id N, open id N+1).
			if !hasAction(partner.Actions, "open") {
				c.ClosedToOpen[id] = id + 1
				c.OpenToClosed[id+1] = id
			}
			continue
		}
		// Two consecutive same-model locs that both offer only
		// "Open" are two closed doors, never a door and its open
		// form. Each swings open under its own id; the pair is
		// also registered as a double door below (see
		// addSameModelDoubleDoor). Skip double-door leaves and
		// already-paired ids so chains don't form.
		if hasAction(partner.Actions, "open") &&
			!c.selfOpening[id+1] &&
			sameModels(def, partner) &&
			!c.hasDoubleDoorFamily(id) && !c.hasDoubleDoorFamily(id+1) &&
			!c.hasOpenToClosed(id) && !c.hasClosedToOpen(id) &&
			!c.hasClosedToOpen(id+1) {
			c.ClosedToOpen[id] = id
			c.ClosedToOpen[id+1] = id + 1
			sameModelPairs = append(sameModelPairs, id)
		}
		// An unused, actionless variant can sit between a door and
		// its open loc.
		skipped := defs.GetObject(id + 2)
		if !hasAnyAction(partner.Actions) &&
			skipped != nil && skipped.Name == def.Name &&
			hasAction(skipped.Actions, "close") && !hasAction(skipped.Actions, "open") &&
			sameModels(def, skipped) &&
			!c.hasDoubleDoorFamily(id) && !c.hasDoubleDoorFamily(id+2) &&
			!c.hasOpenToClosed(id) && !c.hasClosedToOpen(id) {
			c.ClosedToOpen[id] = id + 2
			c.OpenToClosed[id+2] = id
		}
		// Some pairs interleave: closed A, closed B, open B, open
		// A. Require both matching model pairs so the neighbouring
		// door is never used as A's open leaf.
		outerOpen := defs.GetObject(id + 3)
		if !c.hasClosedToOpen(id) && !c.hasOpenToClosed(id+3) &&
			len(def.Models) > 0 && len(partner.Models) > 0 &&
			hasAction(partner.Actions, "open") && !hasAction(partner.Actions, "close") &&
			skipped != nil && skipped.Name == def.Name &&
			hasAction(skipped.Actions, "close") && !hasAction(skipped.Actions, "open") &&
			sameModels(partner, skipped) &&
			outerOpen != nil && outerOpen.Name == def.Name &&
			hasAction(outerOpen.Actions, "close") && !hasAction(outerOpen.Actions, "open") &&
			sameModels(outerOpen, def) &&
			!c.hasDoubleDoorFamily(id) && !c.hasDoubleDoorFamily(id+1) &&
			!c.hasDoubleDoorFamily(id+2) && !c.hasDoubleDoorFamily(id+3) {
			c.ClosedToOpen[id] = id + 3
			c.OpenToClosed[id+3] = id
		}
	}
	// Single-door overrides. A shared open leaf keeps the door the
	// scan already paired it with; an opened door closes from its
	// tracked state.
	for closedID, openID := range singleDoorOpenIDs {
		c.ClosedToOpen[closedID] = openID
		if _, ok := c.OpenToClosed[openID]; !ok {
			c.OpenToClosed[openID] = closedID
		}
	}
	for _, left := range sameModelPairs {
		if _, ok := singleDoorOpenIDs[left]; !ok {
			if _, ok := singleDoorOpenIDs[left+1]; !ok {
				c.addSameModelDoubleDoor(defs, left, left+1)
			}
		}
	}
	// Mirrored leaves that each have their own "Close" loc are one
	// double door too: paired as two single doors, both leaves turned
	// the same way. Iterate closed ids in ascending order so the
	// result is deterministic (TS iterates the paired map in scan
	// order, which is ascending apart from the appended overrides —
	// and override ids are excluded by the SINGLE_DOOR_OPEN_IDS
	// guard below).
	closedIDs := make([]int, 0, len(c.ClosedToOpen))
	for closedID := range c.ClosedToOpen {
		closedIDs = append(closedIDs, closedID)
	}
	sort.Ints(closedIDs)
	for _, closedID := range closedIDs {
		openID := c.ClosedToOpen[closedID]
		partnerID := closedID + 2
		if openID != closedID+1 {
			continue
		}
		partnerOpen, ok := c.ClosedToOpen[partnerID]
		if !ok || partnerOpen != partnerID+1 {
			continue
		}
		family := []int{closedID, partnerID, openID, partnerID + 1}
		excluded := false
		for _, id := range family {
			if c.hasDoubleDoorFamily(id) {
				excluded = true
				break
			}
			if _, ok := singleDoorOpenIDs[id]; ok {
				excluded = true
				break
			}
		}
		if excluded {
			continue
		}
		if !isMirroredLeafName(defs.LocName(closedID), defs.LocName(partnerID)) {
			continue
		}
		c.addCatalogDoubleDoor(closedID, partnerID, openID, partnerID+1)
	}
	return c, nil
}

func (c *Catalog) hasClosedToOpen(id int) bool {
	_, ok := c.ClosedToOpen[id]
	return ok
}

func (c *Catalog) hasOpenToClosed(id int) bool {
	_, ok := c.OpenToClosed[id]
	return ok
}

func (c *Catalog) hasDoubleDoorFamily(id int) bool {
	_, ok := c.doubleDoorFamilies[id]
	return ok
}

// OpenID returns the open leaf for a closed door id.
func (c *Catalog) OpenID(closedID int) (int, bool) {
	open, ok := c.ClosedToOpen[closedID]
	return open, ok
}

// ClosedID returns the closed door for an open leaf id. Shared leaves
// resolve to the first door that paired with them.
func (c *Catalog) ClosedID(openID int) (int, bool) {
	closed, ok := c.OpenToClosed[openID]
	return closed, ok
}

// IsSelfOpening reports whether a door swings in place under its own
// id (open id == closed id).
func (c *Catalog) IsSelfOpening(id int) bool {
	return c.selfOpening[id]
}

// WoodenGate returns the wooden gate a panel id belongs to (hinge,
// extension, opened hinge or opened extension), if any.
func (c *Catalog) WoodenGate(id int) (*WoodenGate, bool) {
	gate, ok := c.woodenGates[id]
	return gate, ok
}

// DoubleDoorFamily returns the family ids containing id: the closed
// and open leaves of one double door.
func (c *Catalog) DoubleDoorFamily(id int) ([]int, bool) {
	family, ok := c.doubleDoorFamilies[id]
	if !ok {
		return nil, false
	}
	return append([]int(nil), family...), true
}

// DoubleDoorPartners returns the ids partnered with id in a special
// double-door pair (both leaves of each pair are partners of the
// other), if any.
func (c *Catalog) DoubleDoorPartners(id int) []int {
	partners, ok := c.doubleDoorPartners[id]
	if !ok {
		return nil
	}
	return append([]int(nil), partners...)
}

// SpecialDoubleDoorOpen returns the open leaf a closed double-door
// leaf swings into, where it is not closedId+1.
func (c *Catalog) SpecialDoubleDoorOpen(id int) (int, bool) {
	open, ok := c.specialDoubleDoorOpen[id]
	return open, ok
}

// IsDoubleDoor reports whether id is a leaf of a double door (table,
// special pair, or catalog-discovered same-model / mirrored-leaf
// family).
func (c *Catalog) IsDoubleDoor(id int) bool {
	return c.hasDoubleDoorFamily(id)
}

// PassThroughGate returns the pass-through gate whose closed leaf
// with the given id stands at (x, y, z), if any.
func (c *Catalog) PassThroughGate(id, x, y, z int) (*PassThroughGate, bool) {
	gate, ok := c.passThrough[passThroughKey{id, x, y, z}]
	return gate, ok
}

// PassThroughFamily returns the pass-through family for a closed door
// id, if it is placed many times as a family.
func (c *Catalog) PassThroughFamily(id int) (*PassThroughFamily, bool) {
	family, ok := c.passThroughFamilies[id]
	return family, ok
}

// IsInPlace reports whether a door replaces its model on the same
// tile instead of swinging (curtains, floor trapdoors).
func (c *Catalog) IsInPlace(id int) bool {
	return c.inPlace[id]
}

// IsOwnTile reports whether a door's open leaf stays on the closed
// leaf's tile, turned a quarter (including every `*_reverse` door).
func (c *Catalog) IsOwnTile(id int) bool {
	return c.ownTile[id]
}

// CompositeOpening returns the leaves one loc opens into, if it is a
// composite opening (one loc -> two leaves).
func (c *Catalog) CompositeOpening(id int) ([]CompositeLeaf, bool) {
	leaves, ok := c.composite[id]
	if !ok {
		return nil, false
	}
	return append([]CompositeLeaf(nil), leaves...), true
}

// IsDorgeshDoor reports whether a door belongs to LocTeleports
// (Dorgesh-Kaan city entrance) and must not be swung here.
func (c *Catalog) IsDorgeshDoor(id int) bool {
	return c.dorgesh[id]
}

// IsDoorName reports whether a cache display name is one of the
// DOOR_NAMES the catalog scans for.
func IsDoorName(name string) bool {
	return DoorNames[name]
}

func hasAction(actions []string, keyword string) bool {
	for _, action := range actions {
		if strings.EqualFold(action, keyword) {
			return true
		}
	}
	return false
}

func hasAnyAction(actions []string) bool {
	for _, action := range actions {
		if action != "" {
			return true
		}
	}
	return false
}

func sameModels(a, b *cache.LocType) bool {
	if a == nil || b == nil || len(a.Models) == 0 || len(b.Models) == 0 {
		return false
	}
	if len(a.Models) != len(b.Models) {
		return false
	}
	for i := range a.Models {
		if len(a.Models[i]) != len(b.Models[i]) {
			return false
		}
		for j := range a.Models[i] {
			if a.Models[i][j] != b.Models[i][j] {
				return false
			}
		}
	}
	return true
}

// namedOpenLeaf is the open loc the cache's gameval names for a closed
// leaf (`<name>_open`, or `closed` -> `open`), when it has the same
// models. Doors.plugin.js:1117-1127 (namedOpenLeaf).
func namedOpenLeaf(defs CatalogDefs, closedID int) (int, bool) {
	name := defs.LocName(closedID)
	if name == "" {
		return 0, false
	}
	closed := defs.GetObject(closedID)
	for _, candidate := range []string{name + "_open", strings.Replace(name, "closed", "open", 1)} {
		if candidate == name {
			continue
		}
		id, ok := defs.LocID(candidate)
		if !ok {
			continue
		}
		if sameModels(closed, defs.GetObject(id)) {
			return id, true
		}
	}
	return 0, false
}

// isSameModelLeafPair: two consecutive locs that are the closed leaves
// of one same-model double door: same name and models, both offering
// only "Open", with mirrored gameval leaf names and each with a
// gameval-named open loc. Doors.plugin.js:1086-1099.
func isSameModelLeafPair(defs CatalogDefs, a, b int) bool {
	left := defs.GetObject(a)
	right := defs.GetObject(b)
	if left == nil || right == nil || left.Name != right.Name {
		return false
	}
	if len(left.Models) == 0 || !sameModels(left, right) {
		return false
	}
	openOnly := func(def *cache.LocType) bool {
		return hasAction(def.Actions, "open") && !hasAction(def.Actions, "close")
	}
	if !openOnly(left) || !openOnly(right) {
		return false
	}
	if !isMirroredLeafName(defs.LocName(a), defs.LocName(b)) {
		return false
	}
	if _, ok := namedOpenLeaf(defs, a); !ok {
		return false
	}
	if _, ok := namedOpenLeaf(defs, b); !ok {
		return false
	}
	return true
}

// isMirroredLeafName: the two gameval names differ only by the leaf's
// side: gatel/gater, _left/_right, door/doorl, face/face_mirr.
// Neighbouring variants (archeuus_door_double_left_green/_red) are
// not leaves. Doors.plugin.js:1101-1115 (isMirroredLeafName).
var leafSideSuffixes = map[string]bool{
	"l|r": true, "left|right": true, "|l": true, "|r": true, "|_mirr": true, "|_mirror": true,
}

func isMirroredLeafName(a, b string) bool {
	if a == "" || b == "" || a == b {
		return false
	}
	start := 0
	for start < len(a) && start < len(b) && a[start] == b[start] {
		start++
	}
	end := 0
	for end < len(a)-start && end < len(b)-start && a[len(a)-1-end] == b[len(b)-1-end] {
		end++
	}
	// Back the shared tail off a letter at a time: left_closed /
	// right_closed share "t_closed".
	for ; end >= 0; end-- {
		x := a[start : len(a)-end]
		y := b[start : len(b)-end]
		if x > y {
			x, y = y, x
		}
		if leafSideSuffixes[x+"|"+y] {
			return true
		}
	}
	return false
}

// addSameModelDoubleDoor registers two same-model "Open" leaves as a
// double door that opens into their gameval-named open locs, or swings
// them in place under their own ids when the cache names none.
// Doors.plugin.js:1130-1136.
func (c *Catalog) addSameModelDoubleDoor(defs CatalogDefs, a, b int) {
	openA, okA := namedOpenLeaf(defs, a)
	openB, okB := namedOpenLeaf(defs, b)
	nextA, nextB := a, b
	if okA && okB {
		nextA, nextB = openA, openB
	}
	c.addCatalogDoubleDoor(a, b, nextA, nextB)
}

// addCatalogDoubleDoor registers closed leaves a and b as one double
// door whose leaves open into nextA and nextB.
// Doors.plugin.js:1139-1150.
func (c *Catalog) addCatalogDoubleDoor(a, b, nextA, nextB int) {
	seen := map[int]bool{}
	family := []int{}
	for _, id := range []int{a, b, nextA, nextB} {
		if seen[id] {
			continue
		}
		seen[id] = true
		family = append(family, id)
	}
	for _, id := range family {
		c.doubleDoorFamilies[id] = family
	}
	// The catalog-discovered pair replaces any table-derived partner
	// list for these ids, exactly as in TS.
	c.doubleDoorPartners[a] = []int{b}
	c.doubleDoorPartners[b] = []int{a}
	c.doubleDoorPartners[nextA] = []int{nextB}
	c.doubleDoorPartners[nextB] = []int{nextA}
	c.specialDoubleDoorOpen[a] = nextA
	c.specialDoubleDoorOpen[b] = nextB
}
