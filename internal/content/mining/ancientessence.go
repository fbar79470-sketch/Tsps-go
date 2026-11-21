// Ancient essence crystals in the Ghorrock Dungeon mine
// (AncientEssence.Mining.js). Only one spot is active at a
// time, switching between two other nearby spots randomly with
// 60 to 90 seconds between each movement.
package mining

import (
	"tsps-go/internal/game"
	"tsps-go/internal/plugins"
)

// Ancient essence constants (AncientEssence.Mining.js:13-25).
var ancientSpots = []ancientSpot{
	{x: 2908, y: 10311, face: 3},
	{x: 2915, y: 10311, face: 0},
	{x: 2912, y: 10307, face: 1},
}

const (
	ancientCrystalsID = 46701 // CRYSTALS_ID
	ancientWallID     = 46702 // WALL_ID
	ancientMinTicks   = 100
	ancientMaxTicks   = 150
)

// ancientSpot is one of the three mine spots
// (AncientEssence.Mining.js:13-17).
type ancientSpot struct {
	x, y int
	face int
}

// ancientState is the spot state (AncientEssence.Mining.js:27-28).
type ancientState struct {
	active *ancientSpot
	wait   int
}

func newAncientState() *ancientState {
	return &ancientState{}
}

// ancientEssenceAmount is the essence a success gives at this
// visible Mining level (essenceAmount,
// AncientEssence.Mining.js:31-35): 1-2 at 75-77, 2-3 at 78-89,
// 3-4 from 90.
func (m *mining) ancientEssenceAmount(level int) int {
	min, max := 1, 2
	switch {
	case level >= 90:
		min, max = 3, 4
	case level >= 78:
		min, max = 2, 3
	}
	return m.randInt(min, max)
}

// ancientLocAt is the loc at a spot (locAt,
// AncientEssence.Mining.js:37-39).
func (m *mining) ancientLocAt(spot *ancientSpot, locID int) plugins.GameObject {
	om := m.objectManager()
	if om == nil || spot == nil {
		return nil
	}
	return om.Get(locID, game.Location{X: spot.x, Y: spot.y, Z: 0})
}

// ancientSwap swaps one loc for another at a spot (swap,
// AncientEssence.Mining.js:41-45).
func (m *mining) ancientSwap(from, to plugins.GameObject) {
	om := m.objectManager()
	if om == nil || from == nil || to == nil {
		return
	}
	om.Deregister(from, false)
	om.Register(to, true)
}

// ancientShowCrystals swaps a spot's wall for the crystals
// (showCrystals, AncientEssence.Mining.js:47-52).
func (m *mining) ancientShowCrystals(spot *ancientSpot) bool {
	wall := m.ancientLocAt(spot, ancientWallID)
	if wall == nil {
		return false
	}
	m.ancientSwap(wall, game.NewGameObject(ancientCrystalsID, wall.Location(), wall.Type(), wall.Face(), nil))
	return true
}

// ancientHideCrystals swaps a spot's crystals back for the wall
// (hideCrystals, AncientEssence.Mining.js:54-58).
func (m *mining) ancientHideCrystals(spot *ancientSpot) {
	crystals := m.ancientLocAt(spot, ancientCrystalsID)
	if crystals == nil {
		return
	}
	m.ancientSwap(crystals, game.NewGameObject(ancientWallID, crystals.Location(), crystals.Type(), crystals.Face(), nil))
}

// ancientNextWait rolls the ticks to the next move (nextWait,
// AncientEssence.Mining.js:60-62).
func (m *mining) ancientNextWait() int {
	return m.randInt(ancientMinTicks, ancientMaxTicks)
}

// ancientMove moves the crystals to a random one of the other
// spots, or starts them at a random spot (move,
// AncientEssence.Mining.js:65-73).
func (m *mining) ancientMove() bool {
	var others []*ancientSpot
	for i := range ancientSpots {
		if m.ancient.active == nil || &ancientSpots[i] != m.ancient.active {
			others = append(others, &ancientSpots[i])
		}
	}
	if len(others) == 0 {
		return false
	}
	target := others[m.randInt(0, len(others)-1)]
	if !m.ancientShowCrystals(target) {
		return false
	}
	if m.ancient.active != nil {
		m.ancientHideCrystals(m.ancient.active)
	}
	m.ancient.active = target
	m.ancient.wait = m.ancientNextWait()
	return true
}

// tickAncient runs one tick of the spot movement (tick,
// AncientEssence.Mining.js:75-82). Like the water cycle, it
// runs on the MiningTask: it starts on the first Mine, not at
// boot.
func (m *mining) tickAncient() {
	if m.ancient == nil {
		return
	}
	if m.ancient.active == nil {
		m.ancientMove()
		return
	}
	m.ancient.wait--
	if m.ancient.wait <= 0 {
		m.ancientMove()
	}
}

// registerAncientEssence wires the AncientEssence registration
// (AncientEssence.Mining.js:93-96). The movement runs on the
// MiningTask (tickAncient), not on a server-startup task: the
// hub installs the TaskManager after plugin Load, and a
// startup hook would call Schedule off the tick goroutine
// (see ensureTask).
func (m *mining) registerAncientEssence(api *plugins.Api) {
	if api == nil {
		return
	}
	// No hook to register: the movement is driven by the
	// MiningTask once mining starts.
}
