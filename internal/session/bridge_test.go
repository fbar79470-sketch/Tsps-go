package session

import (
	"testing"

	"tsps-go/internal/protocol"
)

func TestTurnSinkDelivery(t *testing.T) {
	f := newConnFixture(t)
	var sunk []protocol.Packet
	f.world.sink = func(pkt protocol.Packet) bool {
		sunk = append(sunk, pkt)
		return true
	}
	f.login.seedAccount(t, "Alice", "secret")
	f.loginAndEnter(t, "Alice", "secret")

	walk := clientFrame(protocol.ClientWalk, []byte{0x0c, 0x35, 0x0d, 0xc4, 0x00})
	if err := f.conn.HandleFrame(walk); err != nil {
		t.Fatalf("walk frame: %v", err)
	}
	if err := f.conn.HandleFrame(walk); err != nil {
		t.Fatalf("second walk frame: %v", err)
	}
	if len(sunk) != 2 {
		t.Fatalf("sink received %d packets, want 2", len(sunk))
	}
	for _, pkt := range sunk {
		if pkt.Opcode != protocol.ClientWalk {
			t.Errorf("sunk opcode = %d, want WALK", pkt.Opcode)
		}
	}
	// The local queue stays empty: no double delivery.
	if got := f.conn.TurnPackets(); len(got) != 0 {
		t.Errorf("TurnPackets = %d packets with a sink installed, want 0", len(got))
	}
	// Immediate packets still bypass the sink.
	if err := f.conn.HandleFrame(clientFrame(protocol.ClientPing, pingPayload(1))); err != nil {
		t.Fatalf("ping frame: %v", err)
	}
	if len(sunk) != 2 {
		t.Errorf("sink received %d packets after a ping, want 2", len(sunk))
	}
}

func TestTurnSinkFullDisconnects1008(t *testing.T) {
	f := newConnFixture(t)
	f.world.sink = func(pkt protocol.Packet) bool { return false } // player queue full
	f.login.seedAccount(t, "Alice", "secret")
	f.loginAndEnter(t, "Alice", "secret")

	walk := clientFrame(protocol.ClientWalk, []byte{0x0c, 0x35, 0x0d, 0xc4, 0x00})
	if err := f.conn.HandleFrame(walk); err != nil {
		t.Fatalf("walk frame: %v", err)
	}
	if !f.conn.IsClosed() || len(f.ch.closes) != 1 || f.ch.closes[0][0] != 1008 {
		t.Errorf("sink refusal: closed=%v closes=%v, want one 1008", f.conn.IsClosed(), f.ch.closes)
	}
	if len(f.world.removed) != 1 {
		t.Errorf("player not queued for removal on sink refusal: %v", f.world.removed)
	}
}

func TestTurnSinkOnlyAfterEnter(t *testing.T) {
	f := newConnFixture(t)
	var sunk int
	f.world.sink = func(pkt protocol.Packet) bool {
		sunk++
		return true
	}
	// A walk before login is dropped, not sunk.
	walk := clientFrame(protocol.ClientWalk, []byte{0x0c, 0x35, 0x0d, 0xc4, 0x00})
	if err := f.conn.HandleFrame(walk); err != nil {
		t.Fatalf("pre-login walk: %v", err)
	}
	if sunk != 0 {
		t.Errorf("sink received %d pre-login packets, want 0", sunk)
	}
}
