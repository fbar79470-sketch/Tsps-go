package session

import (
	"testing"

	"tsps-go/internal/game"
)

// The session package must not import game in non-test code (game must
// never import session), but Session has to satisfy game.SessionPort
// structurally. This test-only assertion pins that contract: if either
// side drifts, the build fails here rather than in the coordinator.
var _ game.SessionPort = (*Session)(nil)

func TestSessionSatisfiesSessionPort(t *testing.T) {
	var port game.SessionPort = NewSession(NewHeadlessChannel(""))
	port.SendPacket([]byte{1})
	if err := port.FlushTick(1); err != nil {
		t.Fatalf("FlushTick: %v", err)
	}
	if !port.IsOpen() {
		t.Fatal("headless port not open")
	}
	port.Close(1000, "test")
	if port.IsOpen() {
		t.Fatal("port open after Close")
	}
}
