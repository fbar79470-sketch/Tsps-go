// Camdozaal fishing: the tools factory of
// server/plugins/skills/fishing/Camdozaal.Fishing.js (upstream
// commit bfd81e3), transcribed with line citations. The Wiki
// pages for raw guppy/cavefish/tetra/catfish explicitly lack
// success charts; the rolls [[32, 192]] below are the file's
// estimate, not a measured retail success curve (see the file
// header and docs/camdozaal.md upstream).
//
// The factory's two tools register into
// SpotToolsByNPC[10686..10688] (Fishing.plugin.js:474-480);
// the four fish join Tables.Fish. This file is pure table
// data, so it lives in the tables slice (Worker A).
package fishing

// camdozaalFishDefs are the Camdozaal fish
// (Camdozaal.Fishing.js:6-9): makeFish(id, level, xp, caught)
// with rolls [[32, 192]] throughout.
var camdozaalFishDefs = []fishDef{
	// guppy: level 7, 8 XP (Camdozaal.Fishing.js:6).
	{Name: "GUPPY", Item: "raw_guppy", Level: 7, Rolls: [][2]int{{32, 192}}, XP: 8, Caught: "a raw guppy"},
	// cavefish: level 20, 16 XP (Camdozaal.Fishing.js:7).
	{Name: "CAVEFISH", Item: "raw_cavefish", Level: 20, Rolls: [][2]int{{32, 192}}, XP: 16, Caught: "a raw cavefish"},
	// tetra: level 33, 24 XP (Camdozaal.Fishing.js:8).
	{Name: "TETRA", Item: "raw_tetra", Level: 33, Rolls: [][2]int{{32, 192}}, XP: 24, Caught: "a raw tetra"},
	// catfish: level 46, 33 XP (Camdozaal.Fishing.js:9).
	{Name: "CATFISH", Item: "raw_catfish", Level: 46, Rolls: [][2]int{{32, 192}}, XP: 33, Caught: "a raw catfish"},
}

// camdozaalToolDefs are the Camdozaal tools
// (Camdozaal.Fishing.js:10-16), keyed by internal name; Load
// exposes them as Tables.CamdozaalTools keyed by option
// ("Small Net", "Big Net") and wires them into
// SpotToolsByNPC[10686..10688]. makeTool(id, level,
// animation, fish, clueBase): stationary, interval 6,
// firstRoll 3, start "You cast out your net...".
var camdozaalToolDefs = map[string]toolDef{
	// Small Net: tetra, cavefish, guppy, highest first
	// (Camdozaal.Fishing.js:14).
	"CAMDOZAAL_SMALL_NET": {
		Name: "CAMDOZAAL_SMALL_NET", Item: "net", Level: 7, Animation: AnimNet,
		Fish:     []string{"TETRA", "CAVEFISH", "GUPPY"},
		Interval: NetCageHarpoonIntervalTicks, FirstRoll: NetCageFirstRollTicks, HasFirstRoll: true,
		Stationary: true, StartMessage: CastNetMessage,
	},
	// Big Net: catfish, tetra, clueBase 100000
	// (Camdozaal.Fishing.js:15).
	"CAMDOZAAL_BIG_NET": {
		Name: "CAMDOZAAL_BIG_NET", Item: "big_net", Level: 33, Animation: AnimBigNet,
		Fish:     []string{"CATFISH", "TETRA"},
		Interval: NetCageHarpoonIntervalTicks, FirstRoll: NetCageFirstRollTicks, HasFirstRoll: true,
		Stationary: true, StartMessage: CastNetMessage, ClueBase: 100000,
	},
}
