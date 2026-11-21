package game

// Destination-flag sync: the sending half of
// syncDestinationFlagToRoute (MovementQueue.ts:446-461). The queue
// state it reads (points, lastSentDestX/Y, the destUnsent sentinel)
// lives in movement_queue.go; the game package never imports the
// protocol package, so the packet goes out through the
// MovementOwner.SendDestination forward (Player implements it
// nil-safely; the session package encodes it). The exported
// SyncDestinationFlagToRoute in movement_queue.go delegates here;
// every production call site (applyWalkRoute's failure and success
// branches, Process when the queue empties, Reset with
// clearDestination) already calls that exported method.
//
// Tick-goroutine-only, like the rest of the queue: see
// movement_queue.go.

// syncDestinationFlagToRoute sends the client's destination flag for
// the current route destination, only when it moves.
// MovementQueue.ts:446-461 (syncDestinationFlagToRoute).
func (q *MovementQueue) syncDestinationFlagToRoute() {
	// TS: if (!this.player || (isUnwatchedBot ?? isPlayerBot) === true)
	// return. Go collapses the two bot predicates into the single
	// MovementOwner.IsPlayerBot (Player maps it to IsBot), so ALL bots
	// are excluded — including a watched bot (admin-driven, with a
	// network client), which TS would send to because isUnwatchedBot
	// is false for it (Player.ts:1538-1540). Deliberate deviation,
	// inherited from the coordinator's groundwork: no
	// hasNetworkClient/unwatched distinction exists in Go. The early
	// return runs before the dedup update, as in TS, so a bot (or a
	// non-player) never disturbs lastSentDestX/Y.
	if !q.owner.IsPlayer() || q.owner.IsPlayerBot() {
		return
	}
	// The destination is the last queued point's position, or (0, 0)
	// when the queue is empty (TS peekLast()?.position ?? 0).
	// GetDestination is the existing accessor over the points slice
	// (MovementQueue.ts:435-437).
	dest, ok := q.GetDestination()
	x, y := 0, 0
	if ok {
		x, y = dest.X, dest.Y
	}
	// Combat re-affirms the route every cycle; only tell the client
	// when the flag actually moves. MovementQueue.ts:453-455.
	if q.lastSentDestX == x && q.lastSentDestY == y {
		return
	}
	// Recorded before sending, as in TS (457-460).
	q.lastSentDestX = x
	q.lastSentDestY = y
	q.owner.SendDestination(x, y)
}
