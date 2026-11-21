package listeners

import (
	"testing"

	"tsps-go/internal/game"
)

// TestMovePreamble pins the move-path WORLD_INTERACTIONS preamble
// (NetworkBuilder.ts:680-699): the TRADING skip and the
// player:world-input emit are shared with worldInputPreamble, and
// the move-only rule is the canMove gate — an immobile player's
// click leaves the interface open.
func TestMovePreamble(t *testing.T) {
	stage := func() (*game.Player, *interfaceRuntimeSession) {
		p := testPlayer()
		sess := &interfaceRuntimeSession{}
		p.Session = sess
		p.SetInterfaceId(300)
		return p, sess
	}

	t.Run("mobile player closes", func(t *testing.T) {
		p, sess := stage()
		if skip := MovePreamble(p, game.MovementRequest{X: 1, Y: 2}, Deps{}); skip {
			t.Fatal("mobile preamble skipped the packet")
		}
		if sess.closes != 1 {
			t.Fatalf("closes = %d, want 1", sess.closes)
		}
	})

	t.Run("immobile player keeps the interface", func(t *testing.T) {
		p, sess := stage()
		p.MovementQueue().SetBlockMovement(true)
		if skip := MovePreamble(p, game.MovementRequest{X: 1, Y: 2}, Deps{}); skip {
			t.Fatal("immobile preamble skipped the packet")
		}
		if sess.closes != 0 {
			t.Fatalf("closes = %d, want 0", sess.closes)
		}
	})

	t.Run("handled world-input skips without closing", func(t *testing.T) {
		p, sess := stage()
		deps := Deps{}
		deps.InterfaceHooks.EmitWorldInput = func(ev *WorldInputEvent) bool {
			if ev.PacketType != "move" {
				t.Errorf("PacketType = %q, want move", ev.PacketType)
			}
			ev.Handled = true
			return true
		}
		if skip := MovePreamble(p, game.MovementRequest{X: 1, Y: 2}, deps); !skip {
			t.Fatal("handled world-input did not skip the packet")
		}
		if sess.closes != 0 {
			t.Fatalf("closes = %d, want 0", sess.closes)
		}
	})

	t.Run("nil player proceeds", func(t *testing.T) {
		if skip := MovePreamble(nil, game.MovementRequest{}, Deps{}); skip {
			t.Fatal("nil player skipped")
		}
	})
}
