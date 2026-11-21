package game

import (
	"reflect"
	"testing"
)

// botOwner wraps fakeOwner as a player bot: fakeOwner's IsPlayerBot
// always returns false, and it lives in movement_queue_test.go (owned
// by another worker), so the bot double is defined here. All other
// MovementOwner methods promote from the embedded fakeOwner, so
// SendDestination calls still record on its destinations slice.
type botOwner struct{ *fakeOwner }

func (o *botOwner) IsPlayerBot() bool { return true }

var _ MovementOwner = (*botOwner)(nil)

func TestSyncDestinationFlagSendsLastPointOnly(t *testing.T) {
	owner := newFakeOwner(Location{X: 3089, Y: 3524, Z: 0})
	q := NewMovementQueue(owner, nil, nil)
	q.AddCheckpoint(Location{X: 3090, Y: 3524, Z: 0})
	q.AddCheckpoint(Location{X: 3091, Y: 3525, Z: 0})
	q.AddCheckpoint(Location{X: 3092, Y: 3526, Z: 0})
	q.syncDestinationFlagToRoute()
	want := [][2]int{{3092, 3526}}
	if !reflect.DeepEqual(owner.destinations, want) {
		t.Fatalf("destinations = %v, want %v (last queued point only)", owner.destinations, want)
	}
}

func TestSyncDestinationFlagDedups(t *testing.T) {
	owner := newFakeOwner(Location{X: 3089, Y: 3524, Z: 0})
	q := NewMovementQueue(owner, nil, nil)
	q.AddCheckpoint(Location{X: 3095, Y: 3530, Z: 0})
	q.syncDestinationFlagToRoute()
	q.syncDestinationFlagToRoute()
	q.syncDestinationFlagToRoute()
	if len(owner.destinations) != 1 {
		t.Fatalf("destinations = %v, want exactly 1 send for a repeated sync", owner.destinations)
	}
	// A changed destination sends again, then dedups anew.
	q.AddCheckpoint(Location{X: 3100, Y: 3530, Z: 0})
	q.syncDestinationFlagToRoute()
	q.syncDestinationFlagToRoute()
	want := [][2]int{{3095, 3530}, {3100, 3530}}
	if !reflect.DeepEqual(owner.destinations, want) {
		t.Fatalf("destinations = %v, want %v", owner.destinations, want)
	}
}

func TestSyncDestinationFlagEmptyQueueSendsZeroZero(t *testing.T) {
	owner := newFakeOwner(Location{X: 3089, Y: 3524, Z: 0})
	q := NewMovementQueue(owner, nil, nil)
	q.syncDestinationFlagToRoute()
	q.syncDestinationFlagToRoute()
	want := [][2]int{{0, 0}}
	if !reflect.DeepEqual(owner.destinations, want) {
		t.Fatalf("destinations = %v, want %v (empty queue sends (0, 0) once)", owner.destinations, want)
	}
}

func TestSyncDestinationFlagSentinelInitialization(t *testing.T) {
	// NewMovementQueue initializes lastSentDestX/Y to destUnsent (the
	// Go form of Number.MIN_SAFE_INTEGER, MovementQueue.ts:124-125),
	// so the first sync always sends — even for the (0, 0)
	// empty-queue value, which no real tile can collide with.
	owner := newFakeOwner(Location{X: 3089, Y: 3524, Z: 0})
	q := NewMovementQueue(owner, nil, nil)
	if q.lastSentDestX != destUnsent || q.lastSentDestY != destUnsent {
		t.Fatalf("lastSent = (%d, %d), want sentinel (%d, %d)", q.lastSentDestX, q.lastSentDestY, destUnsent, destUnsent)
	}
	q.syncDestinationFlagToRoute()
	if q.lastSentDestX != 0 || q.lastSentDestY != 0 {
		t.Fatalf("lastSent = (%d, %d), want (0, 0) after the first sync", q.lastSentDestX, q.lastSentDestY)
	}
	if len(owner.destinations) != 1 {
		t.Fatalf("destinations = %v, want exactly 1 send", owner.destinations)
	}
}

func TestSyncDestinationFlagEmptyAfterRouteSendsZeroZero(t *testing.T) {
	owner := newFakeOwner(Location{X: 3089, Y: 3524, Z: 0})
	q := NewMovementQueue(owner, nil, nil)
	q.AddCheckpoint(Location{X: 3095, Y: 3530, Z: 0})
	q.syncDestinationFlagToRoute()
	// Reset(false) empties the queue without syncing (as in
	// applyWalkRoute's reset), so the next sync must report the
	// emptied route as (0, 0).
	q.Reset(false)
	q.syncDestinationFlagToRoute()
	q.syncDestinationFlagToRoute()
	want := [][2]int{{3095, 3530}, {0, 0}}
	if !reflect.DeepEqual(owner.destinations, want) {
		t.Fatalf("destinations = %v, want %v", owner.destinations, want)
	}
}

func TestSyncDestinationFlagExcludesBots(t *testing.T) {
	t.Run("fake owner bot", func(t *testing.T) {
		inner := newFakeOwner(Location{X: 3089, Y: 3524, Z: 0})
		q := NewMovementQueue(&botOwner{inner}, nil, nil)
		q.AddCheckpoint(Location{X: 3095, Y: 3530, Z: 0})
		q.syncDestinationFlagToRoute()
		if len(inner.destinations) != 0 {
			t.Fatalf("destinations = %v, want none for a bot", inner.destinations)
		}
		// The bot return runs before the dedup update (TS 447), so
		// the sentinel survives: a bot never disturbs lastSent.
		if q.lastSentDestX != destUnsent || q.lastSentDestY != destUnsent {
			t.Fatalf("lastSent = (%d, %d), want sentinel", q.lastSentDestX, q.lastSentDestY)
		}
	})
	t.Run("real player bot with session", func(t *testing.T) {
		p := NewPlayer("botplayer", Location{X: 3089, Y: 3524, Z: 0})
		p.IsBot = true
		session := newFakeSession()
		p.Session = session
		p.MovementQueue().AddCheckpoint(Location{X: 3095, Y: 3530, Z: 0})
		p.MovementQueue().syncDestinationFlagToRoute()
		if len(session.destinations) != 0 {
			t.Fatalf("session destinations = %v, want none for a bot", session.destinations)
		}
	})
}

func TestSyncDestinationFlagSkipsNonPlayers(t *testing.T) {
	// TS 447: !this.player returns before any send or dedup update.
	owner := newFakeOwner(Location{X: 3089, Y: 3524, Z: 0})
	owner.player = false
	q := NewMovementQueue(owner, nil, nil)
	q.AddCheckpoint(Location{X: 3095, Y: 3530, Z: 0})
	q.syncDestinationFlagToRoute()
	if len(owner.destinations) != 0 {
		t.Fatalf("destinations = %v, want none for a non-player owner", owner.destinations)
	}
	if q.lastSentDestX != destUnsent || q.lastSentDestY != destUnsent {
		t.Fatalf("lastSent = (%d, %d), want sentinel", q.lastSentDestX, q.lastSentDestY)
	}
}

func TestSyncDestinationFlagThroughPlayerSession(t *testing.T) {
	// Drives the real Player.SendDestination forward onto a
	// fakeSession, recording through the SessionPort.
	p := NewPlayer("destplayer", Location{X: 3089, Y: 3524, Z: 0})
	session := newFakeSession()
	p.Session = session
	q := p.MovementQueue()
	q.AddCheckpoint(Location{X: 3095, Y: 3530, Z: 0})
	q.syncDestinationFlagToRoute()
	q.syncDestinationFlagToRoute()
	q.AddCheckpoint(Location{X: 3100, Y: 3535, Z: 0})
	q.syncDestinationFlagToRoute()
	want := [][2]int{{3095, 3530}, {3100, 3535}}
	if !reflect.DeepEqual(session.destinations, want) {
		t.Fatalf("session destinations = %v, want %v", session.destinations, want)
	}
}

func TestSyncDestinationFlagNilSessionDoesNotPanic(t *testing.T) {
	// A NewPlayer with no session attached: Player.SendDestination
	// drops the packet nil-safely, and sync still records lastSent.
	p := NewPlayer("nosession", Location{X: 3089, Y: 3524, Z: 0})
	q := p.MovementQueue()
	q.AddCheckpoint(Location{X: 3095, Y: 3530, Z: 0})
	q.syncDestinationFlagToRoute()
	if q.lastSentDestX != 3095 || q.lastSentDestY != 3530 {
		t.Fatalf("lastSent = (%d, %d), want (3095, 3530)", q.lastSentDestX, q.lastSentDestY)
	}
	q.Reset(false)
	q.syncDestinationFlagToRoute()
	if q.lastSentDestX != 0 || q.lastSentDestY != 0 {
		t.Fatalf("lastSent = (%d, %d), want (0, 0)", q.lastSentDestX, q.lastSentDestY)
	}
}

func TestSyncDestinationFlagResendsAfterInvalidate(t *testing.T) {
	// InvalidateDestinationFlagCache (MovementQueue.ts:530-533)
	// restores the sentinel, so the next sync re-sends even an
	// unchanged destination. TS calls it from
	// Mobile.setNeedsPlacement; Go has no such setter yet, so this
	// pins the queue-side half only.
	owner := newFakeOwner(Location{X: 3089, Y: 3524, Z: 0})
	q := NewMovementQueue(owner, nil, nil)
	q.AddCheckpoint(Location{X: 3095, Y: 3530, Z: 0})
	q.syncDestinationFlagToRoute()
	q.InvalidateDestinationFlagCache()
	q.syncDestinationFlagToRoute()
	want := [][2]int{{3095, 3530}, {3095, 3530}}
	if !reflect.DeepEqual(owner.destinations, want) {
		t.Fatalf("destinations = %v, want %v", owner.destinations, want)
	}
}
