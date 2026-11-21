package game

// MovementRequest is one decoded walk/click request: the destination
// tile, the run choice, and the click's modifier flags. It is the Go
// form of the TS "move" message (ClientProtocol.ts MOVE_GAMECLICK and
// WALK cases, lines 811-821), as the network dispatch hands it to
// walk() (NetworkBuilder.ts:738-740, 1739-1752).
//
// The two packet forms collapse into this one shape, as in TS:
//   - WALK carries the run bit explicitly (flags & 1); Run is that bit.
//   - MOVE_GAMECLICK carries no run bit: its decoded message has no
//     run field, so walk() runs with forceRun=false and the run
//     choice comes from clickRunOverride(modifierFlags) instead
//     (NetworkBuilder.ts:238-244, 1745). Run is false for gameclicks,
//     and ModifierFlags carries the input that override reads.
//
// TargetLocID is the MOVE_GAMECLICK target location id (zero for a
// walk-here click). TS reads it and discards it — the "move" message
// has no target field — so nothing may act on it until the walk-to
// interaction layer lands; it is carried here so the decode does not
// lose information the client sent. It is always 0 for WALK.
//
// SCOPE: this type is only the decoded request. Movement execution
// (MovementQueue, PathFinder/WalkRoute, RegionManager collision,
// stepping, position changes) IS ported: the wired server installs
// Player.OnMovementRequest with the walk() hook (see
// cmd/server/server.go, AddPlayer), which routes the request through
// the player's MovementQueue. See movement_queue.go for the ported
// subset and its degenerations.
type MovementRequest struct {
	// X, Y is the destination tile (world coordinates).
	X, Y int
	// Run is the WALK force-run bit. Always false for MOVE_GAMECLICK.
	Run bool
	// ModifierFlags is the click's modifier flags: WALK flags >> 1, or
	// the MOVE_GAMECLICK byteNeg value verbatim.
	ModifierFlags int
	// TargetLocID is the MOVE_GAMECLICK target location id, 0 for WALK
	// and for walk-here clicks. TS discards it; see the type comment.
	TargetLocID int
}

// ClickRunOverride is the run choice a click's modifier flags ask
// for: whether to run, or nil to follow the run setting. It ports
// clickRunOverride (NetworkBuilder.ts:238-244) with the player's run
// setting passed in (TS reads player.isRunningReturn()):
//
//   - Modifier flags with the Ctrl bit (1) clear — except flags == 2
//     (Ctrl+Shift, a staff teleport that plugins take over, which
//     walks like a Ctrl-click for everyone else) — return nil.
//   - Otherwise the click inverts the run mode as runInvertAllowed
//     permits, and runs only when the inverted mode is running.
//
// Two TS dependencies degenerate here, exactly as documented:
//   - runInvertAllowed (NetworkBuilder.ts:231-236) reads the Settings
//     "Ctrl+click to invert run mode" varbit (13132, enum 4074: 0
//     Never, 1 Walk->Run Only, 2 Run->Walk Only, 3 Always) through
//     the packet sender's varbit store, and returns true when the
//     cache pipeline is not initialized. Go has no varbit store and
//     no settings system, so inversion is always allowed — the live
//     default (Always) and the uninitialized-cache answer agree on
//     true. A player who configured Never / Walk-only / Run-only
//     would still invert in Go until settings/varbits are ported.
//   - The energy check (TS: getRunEnergy() > 0) has no Go equivalent:
//     no live run-energy state exists. Energy is treated as
//     available, so a Ctrl-click while walking always returns true
//     (run) and while running always returns false (walk). When run
//     energy lands, this must re-check it: a Ctrl-click at zero
//     energy must not force a run.
//
// The result feeds MovementQueue.RequestWalk's runOverride: the WALK
// force-run bit bypasses this function (TS walk(), NetworkBuilder
// .ts:1745: forceRun ? true : clickRunOverride(...)).
func ClickRunOverride(running bool, modifierFlags int) *bool {
	if modifierFlags != 2 && modifierFlags&1 == 0 {
		return nil
	}
	// runInvertAllowed: always true in Go; see the doc comment.
	// Energy check: energy treated as available; see the doc comment.
	inverted := !running
	return &inverted
}
