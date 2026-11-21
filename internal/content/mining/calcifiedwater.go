// Cam Torum mine's oozing water (CalcifiedWater.Mining.js):
// every 1-2 minutes water leaks from some calcified rocks,
// boosting the skilling success rate by 15%, and when it
// stops after 20-30 seconds those deposits are depleted.
package mining

import (
	"fmt"

	"tsps-go/internal/game"
	"tsps-go/internal/plugins"
)

// Calcified water constants (CalcifiedWater.Mining.js:21-31).
const (
	calcifiedRockName     = "Calcified rocks"
	calcifiedWaterLoc     = "varlamore_mining_special_rock"
	calcifiedWaterShape   = 4
	calcifiedOozeMessage  = "<col=a53fff>Some cracks around the cave begin to ooze water."
	calcifiedOozeSound    = 2041
	calcifiedCycleMin     = 117
	calcifiedCycleMax     = 146
	calcifiedWaterMin     = 38
	calcifiedWaterMax     = 54
	calcifiedMineX        = 1490
	calcifiedMineX2       = 1528
	calcifiedMineY        = 9530
	calcifiedMineY2       = 9553
	calcifiedMineZ        = 1
	calcifiedEastFromX    = 1510
	calcifiedWetPerHalf   = 2
	calcifiedSuccessBoost = 1.15
)

// calcifiedSite is one calcified rock site in the mine chamber:
// the rock as the map places it (or the _empty loc's rock).
type calcifiedSite struct {
	loc    game.Location
	rockID int
	face   int
	typ    int
}

// wetRock is a wet rock: the site and its water decoration.
type wetRock struct {
	site  calcifiedSite
	water plugins.GameObject
}

// calcifiedWaterState is the water cycle's state
// (CalcifiedWater.Mining.js:40-41).
type calcifiedWaterState struct {
	wet       map[string]*wetRock // by tile key
	tick      int
	nextOoze  int
	stopAt    int
	depleteAt int
	sites     []calcifiedSite
	sitesDone bool
	mine      *game.Area
}

func newCalcifiedWaterState() *calcifiedWaterState {
	return &calcifiedWaterState{
		wet:       make(map[string]*wetRock),
		nextOoze:  -1,
		stopAt:    -1,
		depleteAt: -1,
	}
}

// tileKey keys a location (CalcifiedWater.Mining.js:47-49).
func tileKey(loc game.Location) string {
	return fmt.Sprintf("%d,%d,%d", loc.X, loc.Y, loc.Z)
}

// calcifiedRock returns the "Calcified rocks" table entry.
func (m *mining) calcifiedRock() *Rock {
	if m.tables == nil {
		return nil
	}
	return m.tables.RockByName(calcifiedRockName)
}

// calcifiedSites finds the mine's calcified rocks, as the
// cache's map places them (sites, CalcifiedWater.Mining.js:64-86):
// found once, the first time they're needed. A rock mined out
// at that moment counts by its tile, as the rock its `_empty`
// loc stands in for, so it can still ooze once it respawns.
func (m *mining) calcifiedSites() []calcifiedSite {
	if m.water.sitesDone {
		return m.water.sites
	}
	m.water.sitesDone = true
	om := m.objectManager()
	rock := m.calcifiedRock()
	if om == nil || rock == nil || m.tables == nil {
		return nil
	}
	// Mined-out loc id -> the calcified rock it stands in for
	// (rockForEmpty, CalcifiedWater.Mining.js:52-62).
	empties := make(map[int]int)
	for rockGameval, emptyGameval := range rock.DepletedRocks {
		rockID := m.tables.LocID(rockGameval)
		emptyID := m.tables.LocID(emptyGameval)
		if rockID >= 0 && emptyID >= 0 {
			empties[emptyID] = rockID
		}
	}
	rockIDs := []int{
		m.tables.LocID("varlamore_mining_rock"),
		m.tables.LocID("varlamore_mining_rock02"),
		m.tables.LocID("varlamore_mining_rock03"),
		m.tables.LocID("varlamore_mining_rock04"),
	}
	byTile := make(map[string]calcifiedSite)
	for x := calcifiedMineX; x <= calcifiedMineX2; x++ {
		for y := calcifiedMineY; y <= calcifiedMineY2; y++ {
			loc := game.Location{X: x, Y: y, Z: calcifiedMineZ}
			key := tileKey(loc)
			if _, ok := byTile[key]; ok {
				continue
			}
			for _, id := range rockIDs {
				if id < 0 {
					continue
				}
				if obj := om.Get(id, loc); obj != nil && objectPrivateArea(obj) == nil {
					byTile[key] = calcifiedSite{loc: loc, rockID: id, face: obj.Face(), typ: obj.Type()}
					break
				}
			}
			if _, ok := byTile[key]; ok {
				continue
			}
			for emptyID, rockID := range empties {
				if obj := om.Get(emptyID, loc); obj != nil && objectPrivateArea(obj) == nil {
					byTile[key] = calcifiedSite{loc: loc, rockID: rockID, face: obj.Face(), typ: obj.Type()}
					break
				}
			}
		}
	}
	for _, site := range byTile {
		m.water.sites = append(m.water.sites, site)
	}
	return m.water.sites
}

// calcifiedStanding is the rock standing at a site now (null
// while it is mined out) (standing, CalcifiedWater.Mining.js:89-91).
func (m *mining) calcifiedStanding(site calcifiedSite) plugins.GameObject {
	om := m.objectManager()
	if om == nil {
		return nil
	}
	return om.Get(site.rockID, site.loc)
}

// pickWetRocks picks two standing rocks from each half of the
// mine (pickWetRocks, CalcifiedWater.Mining.js:94-103).
func (m *mining) pickWetRocks() []calcifiedSite {
	var picked []calcifiedSite
	for _, east := range []bool{false, true} {
		var half []calcifiedSite
		for _, site := range m.calcifiedSites() {
			if (site.loc.X >= calcifiedEastFromX) == east && m.calcifiedStanding(site) != nil {
				half = append(half, site)
			}
		}
		for n := 0; n < calcifiedWetPerHalf && len(half) > 0; n++ {
			i := m.randInt(0, len(half)-1)
			picked = append(picked, half[i])
			half = append(half[:i], half[i+1:]...)
		}
	}
	return picked
}

// startWater starts an ooze cycle (startWater,
// CalcifiedWater.Mining.js:105-117).
func (m *mining) startWater() {
	om := m.objectManager()
	rock := m.calcifiedRock()
	if om == nil || rock == nil || m.tables == nil {
		return
	}
	waterID := m.tables.LocID(calcifiedWaterLoc)
	for _, site := range m.pickWetRocks() {
		if waterID < 0 {
			continue
		}
		water := game.NewGameObject(waterID, site.loc, calcifiedWaterShape, site.face, nil)
		om.Register(water, true)
		m.water.wet[tileKey(site.loc)] = &wetRock{site: site, water: water}
	}
	if m.water.mine != nil {
		for _, player := range m.water.mine.GetPlayers() {
			if player == nil {
				continue
			}
			player.SendMessage(calcifiedOozeMessage)
			game.SendSoundEffect(player, calcifiedOozeSound, 1, 0, 1)
		}
	}
	m.water.stopAt = m.water.tick + m.randInt(calcifiedWaterMin, calcifiedWaterMax)
}

// stopWater stops the ooze (stopWater,
// CalcifiedWater.Mining.js:119-123).
func (m *mining) stopWater() {
	om := m.objectManager()
	if om != nil {
		for _, wet := range m.water.wet {
			om.Deregister(wet.water, true)
		}
	}
	m.water.stopAt = -1
	m.water.depleteAt = m.water.tick + 1
}

// depleteWetRocks mines out every rock the water ran from
// (depleteWetRocks, CalcifiedWater.Mining.js:126-136).
func (m *mining) depleteWetRocks() {
	rock := m.calcifiedRock()
	if rock == nil {
		m.water.wet = make(map[string]*wetRock)
		m.water.depleteAt = -1
		return
	}
	var viewers []*game.Player
	if m.water.mine != nil {
		viewers = m.water.mine.GetPlayers()
	}
	for _, wet := range m.water.wet {
		object := m.calcifiedStanding(wet.site)
		// A miner whose rocks do not deplete (Demonic Pacts'
		// Endless Harvest) keeps the rock and their mining.
		if object != nil && !m.minersKeepStanding(object, rock) {
			m.exhaustRock(object, rock, viewers, object)
		}
	}
	m.water.wet = make(map[string]*wetRock)
	m.water.depleteAt = -1
}

// tickWater runs one tick of the water cycle (processWater,
// CalcifiedWater.Mining.js:138-145). The cycle runs on the
// MiningTask: it starts on the first Mine, not at boot (the
// task only exists once mining starts — see ensureTask).
func (m *mining) tickWater() {
	if m.water == nil {
		return
	}
	if m.water.nextOoze < 0 {
		m.water.nextOoze = m.randInt(calcifiedCycleMin, calcifiedCycleMax)
	}
	m.water.tick++
	if m.water.tick == m.water.depleteAt {
		m.depleteWetRocks()
	}
	if m.water.tick == m.water.stopAt {
		m.stopWater()
	}
	if m.water.tick >= m.water.nextOoze {
		m.water.nextOoze = m.water.tick + m.randInt(calcifiedCycleMin, calcifiedCycleMax)
		if m.water.stopAt < 0 && m.water.depleteAt < 0 {
			m.startWater()
		}
	}
}

// isWetRock reports whether a rock is wet (isWet,
// CalcifiedWater.Mining.js:147-149).
func (m *mining) isWetRock(object plugins.GameObject) bool {
	if object == nil || m.water == nil {
		return false
	}
	_, ok := m.water.wet[tileKey(object.Location())]
	return ok && m.water.stopAt >= 0
}

// waterBoost answers "mining:attempt" (waterBoost,
// CalcifiedWater.Mining.js:152-160): a roll on a wet rock has
// its chance raised by 15%, so a miss turns into a hit with the
// probability that makes up the difference.
func (m *mining) waterBoost(payload any) {
	attempt, ok := payload.(*attemptEvent)
	if !ok || attempt == nil || attempt.Success || attempt.Rock != calcifiedRockName || !m.isWetRock(attempt.Object) {
		return
	}
	chance := attempt.Chance
	if chance >= 1 || chance < 0 {
		return
	}
	boosted := chance * calcifiedSuccessBoost
	if boosted > 1 {
		boosted = 1
	}
	if m.randFloat() < (boosted-chance)/(1-chance) {
		attempt.Success = true
	}
}

// registerCalcifiedWater wires the CalcifiedWater registrations
// (CalcifiedWater.Mining.js:181-196): the mine area and the
// attempt boost. The water cycle itself runs on the MiningTask
// (tickWater), not on a Register-time task submit: the hub
// installs the TaskManager after plugin Load, so a
// Register-time capture reads nil (the hub-contract lesson).
func (m *mining) registerCalcifiedWater(api *plugins.Api) {
	if api == nil {
		return
	}
	mine := game.NewArea("Cam Torum mine", []game.Boundary{
		game.NewBoundary(calcifiedMineX, calcifiedMineX2, calcifiedMineY, calcifiedMineY2, calcifiedMineZ),
	})
	m.water.mine = mine
	api.RegisterArea(mine)
	api.OnCustomEvent("mining:attempt", m.waterBoost)
}
