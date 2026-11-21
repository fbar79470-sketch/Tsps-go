package session

import (
	"bytes"
	"testing"

	"tsps-go/internal/protocol"
)

// queuedPackets returns the session's pending queue contents. The
// test reads them under mu, as Flush does.
func queuedPackets(t *testing.T, s *Session) [][]byte {
	t.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([][]byte, len(s.pending))
	copy(out, s.pending)
	return out
}

func TestSendSkillDelta(t *testing.T) {
	s := NewSession(NewHeadlessChannel(""))
	// Array index 16 (Agility) maps to client id 16; the view
	// carries base == virtual, boost = current - base, and the
	// totals, exactly as PacketSender.skillView builds it.
	s.SendSkillDelta(16, 7, 5, 100, 33, 3)
	packets := queuedPackets(t, s)
	if len(packets) != 1 {
		t.Fatalf("queued packets = %d, want 1", len(packets))
	}
	want := protocol.EncodeSkillsDelta([]protocol.SkillView{
		{ID: 16, XP: 100, BaseLevel: 5, VirtualLevel: 5, Boost: 2, CurrentLevel: 7},
	}, 33, 3)
	if !bytes.Equal(packets[0], want) {
		t.Errorf("queued packet = %x, want %x", packets[0], want)
	}
}

func TestSendSkillDeltaClientIDs(t *testing.T) {
	s := NewSession(NewHeadlessChannel(""))
	// Construction (index 21) -> client id 22; Hunter (index 22)
	// -> client id 21 (Skill.getClientId; skillClientIDs).
	s.SendSkillDelta(21, 1, 1, 0, 33, 3)
	s.SendSkillDelta(22, 1, 1, 0, 33, 3)
	packets := queuedPackets(t, s)
	if len(packets) != 2 {
		t.Fatalf("queued packets = %d, want 2", len(packets))
	}
	for i, wantID := range []int{22, 21} {
		want := protocol.EncodeSkillsDelta([]protocol.SkillView{
			{ID: wantID, XP: 0, BaseLevel: 1, VirtualLevel: 1, Boost: 0, CurrentLevel: 1},
		}, 33, 3)
		if !bytes.Equal(packets[i], want) {
			t.Errorf("packet %d = %x, want %x (client id %d)", i, packets[i], want, wantID)
		}
	}
}

func TestSendSkillDeltaIgnoresInvalidSkill(t *testing.T) {
	s := NewSession(NewHeadlessChannel(""))
	s.SendSkillDelta(-1, 1, 1, 0, 33, 3)
	s.SendSkillDelta(24, 1, 1, 0, 33, 3)
	if got := queuedPackets(t, s); len(got) != 0 {
		t.Errorf("queued packets = %d, want 0", len(got))
	}
}

// TestSendExpDropQueuesNothing pins the deliberate no-op: TS drops
// the PacketBuilder in the no-op write(), so no frame is ever
// queued — for a plain skill, for Construction (index 21, client
// id 22) and Hunter (index 22, client id 21), whose translation
// through skillClientIDs is exercised by the discarded build.
// The translated byte layout is pinned by the protocol encoder
// tests (TestEncodeExpDrop); here only the wire effect — none —
// is observable.
func TestSendExpDropQueuesNothing(t *testing.T) {
	s := NewSession(NewHeadlessChannel(""))
	s.SendExpDrop(3, 100)
	s.SendExpDrop(21, 100)
	s.SendExpDrop(22, 100)
	if got := queuedPackets(t, s); len(got) != 0 {
		t.Errorf("queued packets = %d, want 0 (sendExpDrop is a no-op in TS)", len(got))
	}
}

func TestSendExpDropIgnoresInvalidSkill(t *testing.T) {
	s := NewSession(NewHeadlessChannel(""))
	s.SendExpDrop(-1, 100)
	s.SendExpDrop(24, 100)
	if got := queuedPackets(t, s); len(got) != 0 {
		t.Errorf("queued packets = %d, want 0", len(got))
	}
}
