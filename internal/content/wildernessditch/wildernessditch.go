// Package wildernessditch is the Go port of the WildernessDitch plugin
// (server/plugins/objects/WildernessDitch.plugin.js), the "Cross"
// interaction on the Wilderness Ditch object (23271) that jumps the
// player 3 tiles across, in and out of the Wilderness.
//
// The port is faithful to the 62-line plugin: resolveJump's facing
// rules (face 0/2 runs east-west, crossed north/south; face 1/3 runs
// north-south, crossed east/west), the two guards (no active
// ForceMovement, 250 ms click delay), the ForceMovementTask (3 ticks,
// speed 0, reverseSpeed 70, animation 6132), the jump sound (synth
// 2462, delay 25), and the click-delay reset after a cross. The
// ForceMovement execution path the plugin needs lives in
// internal/game (forcemovement.go); the sync view it rides lives in
// cmd/server/syncsource.go (PlayerSession.ts:742-789).
//
// One handler-shape note: TS crossDitch returns undefined, and under
// the Go void | boolean handler shape (plugins/objects.go) any result
// other than exactly false claims the event — so the handler returns
// true on every path, including the guard paths: a blocked Cross is
// consumed silently, exactly as in TS. The only exception is a nil
// TaskManager, which cannot happen in production (the hub installs
// the world's TaskManager at boot); there the handler returns false
// so the failure is visible instead of silently swallowed.
package wildernessditch

import (
	"tsps-go/internal/game"
	"tsps-go/internal/plugins"
)

// Jump constants (WildernessDitch.plugin.js:1-3): the wild_ditch_jump
// animation, the rsprox-recorded jump sound (synth 2462) with its
// delay, and the ForceMovement speed/reverseSpeed the plugin passes.
const (
	jumpAnimation    = 6132
	jumpSoundDelay   = 25
	jumpSpeed        = 0
	jumpReverseSpeed = 70
	jumpTaskDelay    = 3
	clickDelayMs     = 250
)

// defaultThresholdY is the TS default threshold (3522) resolveJump uses
// when the ditch location is absent (WildernessDitch.plugin.js
// resolveJump: `ditch?.y ? ditch.y + 1 : 3522`).
const defaultThresholdY = 3522

// resolveJump ports resolveJump (WildernessDitch.plugin.js): the jump
// across a ditch piece. The ditch is 1x2, so a piece facing 0/2 runs
// east-west (the long stretch at y 3521, crossed north/south) and one
// facing 1/3 runs north-south (where it turns north at 2996,3530-3533,
// crossed east/west). The player lands 3 tiles on, away from the side
// they stand on. from is the player's location; ditch is the ditch
// object's tile (nil when absent); source is the source location (the
// player's tile at arrival — nil falls back to from, as in TS).
// Returns the delta and the facing direction (0 north, 1 east, 2
// south, 3 west).
func resolveJump(from game.Location, ditch *game.Location, source *game.Location, face int) (dx, dy, direction int) {
	if face&1 == 1 {
		fromX := from.X
		if source != nil {
			fromX = source.X
		}
		threshold := fromX
		if ditch != nil {
			threshold = ditch.X + 1
		}
		if fromX < threshold {
			return 3, 0, 1
		}
		return -3, 0, 3
	}
	fromY := from.Y
	if source != nil {
		fromY = source.Y
	}
	threshold := defaultThresholdY
	if ditch != nil {
		threshold = ditch.Y + 1
	}
	if fromY < threshold {
		return 0, 3, 0
	}
	return 0, -3, 2
}

// crossDitch is the Cross handler (WildernessDitch.plugin.js
// crossDitch / tryCrossWildernessDitch), in exact TS order: the two
// guards, then the task submit, the sound, and the click-delay reset.
func crossDitch(api *plugins.Api, e *plugins.ObjectInteractionEvent) bool {
	p := e.Player
	if p == nil {
		return true
	}
	if p.ForceMovement() != nil {
		return true
	}
	if !p.ClickDelayElapsed(clickDelayMs) {
		return true
	}
	ditchLoc := e.Location
	face := 0
	if e.Object != nil {
		face = e.Object.Face()
	}
	dx, dy, dir := resolveJump(p.Location(), &ditchLoc, e.SourceLocation, face)
	tm := api.GetTaskManager()
	if tm == nil {
		return false
	}
	fm := &game.ForceMovement{
		Start:        p.Location(),
		Delta:        game.Location{X: dx, Y: dy},
		Speed:        jumpSpeed,
		ReverseSpeed: jumpReverseSpeed,
		Direction:    dir,
		Animation:    jumpAnimation,
	}
	tm.Schedule(game.NewForceMovementTask(p, jumpTaskDelay, fm).Task())
	game.SendSound(p, game.NewSound(game.SoundWildernessDitchJump.ID(), 1, jumpSoundDelay, 0))
	p.ResetClickDelay()
	return true
}

// Plugin is the registered WildernessDitch plugin value: name and
// description from the plugin file (WildernessDitch.plugin.js).
var Plugin = plugins.Plugin{
	Name:        "WildernessDitch",
	Description: "Crossing the Wilderness Ditch with the jump animation and sound, landing three tiles on, in and out of the Wilderness.",
	Register: func(api *plugins.Api) {
		api.OnObjectInteractionNamed("Wilderness Ditch", map[string]plugins.ObjectInteractionHandler{
			"Cross": func(e *plugins.ObjectInteractionEvent) bool { return crossDitch(api, e) },
		})
	},
}

func init() {
	plugins.Register(Plugin)
}
