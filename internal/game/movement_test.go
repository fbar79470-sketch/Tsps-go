package game

import "testing"

// The movement hook is a plain field: nil by default (requests drop),
// and a set hook receives exactly the request and cycle it is given.
// Dispatch through TurnHandler lives in the coordinator (cmd/server),
// which owns the decoding; this test pins the hook contract only.
func TestMovementHookDefaultNil(t *testing.T) {
	p := NewPlayer("alice", Location{X: 3089, Y: 3524, Z: 0})
	if p.OnMovementRequest != nil {
		t.Error("OnMovementRequest non-nil on a fresh player")
	}
}

func TestMovementHookReceivesRequest(t *testing.T) {
	p := NewPlayer("alice", Location{X: 3089, Y: 3524, Z: 0})
	var got *MovementRequest
	var gotCycle int64
	p.OnMovementRequest = func(pl *Player, req MovementRequest, cycle int64) {
		if pl != p {
			t.Errorf("hook player = %p, want %p", pl, p)
		}
		got = &req
		gotCycle = cycle
	}
	want := MovementRequest{X: 3090, Y: 3525, Run: true, ModifierFlags: 2, TargetLocID: 0}
	p.OnMovementRequest(p, want, 42)
	if got == nil || *got != want || gotCycle != 42 {
		t.Errorf("hook got (%v, %d), want (%v, 42)", got, gotCycle, want)
	}
	// The hook contract changes no position on its own: Loc is
	// untouched by recording the request.
	if p.Loc.X != 3089 || p.Loc.Y != 3524 {
		t.Errorf("Loc moved to %+v", p.Loc)
	}
}
