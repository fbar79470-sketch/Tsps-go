package dateminutes

import (
	"testing"

	"tsps-go/internal/game"
	"tsps-go/internal/plugins"
)

// recordingSession is a minimal game.SessionPort double capturing
// SendVarp calls. It mirrors the fakes in internal/game tests; this
// package cannot reuse them (they are test-only in another package).
type recordingSession struct {
	open  bool
	varps [][2]int
}

func (s *recordingSession) SendPacket([]byte)                 {}
func (s *recordingSession) SendRunEnergy(int, bool, int, int) {}
func (s *recordingSession) SendDestination(int, int)          {}
func (s *recordingSession) SendVarp(id, value int) {
	s.varps = append(s.varps, [2]int{id, value})
}
func (s *recordingSession) FlushTick(int64) error { return nil }
func (s *recordingSession) Close(int, string)     { s.open = false }
func (s *recordingSession) IsOpen() bool          { return s.open }

func newTestPlayer() (*game.Player, *recordingSession) {
	p := game.NewPlayer("testplayer", game.Location{X: 3200, Y: 3200, Z: 0})
	sess := &recordingSession{open: true}
	p.Session = sess
	return p, sess
}

func TestDateMinutes(t *testing.T) {
	if got := dateMinutes(0); got != 0 {
		t.Errorf("dateMinutes(0) = %d, want 0", got)
	}
	if got := dateMinutes(59_999); got != 0 {
		t.Errorf("dateMinutes(59999) = %d, want 0", got)
	}
	if got := dateMinutes(60_000); got != 1 {
		t.Errorf("dateMinutes(60000) = %d, want 1", got)
	}
	if got := dateMinutes(60_000 * 29_500_000); got != 29_500_000 {
		t.Errorf("dateMinutes(large) = %d, want 29500000", got)
	}
}

func TestSendDedupsWithinMinute(t *testing.T) {
	now := int64(60_000 * 100)
	s := newPiece(func() int64 { return now })
	p, sess := newTestPlayer()
	s.send(p)
	s.send(p)
	if len(sess.varps) != 1 {
		t.Fatalf("varps = %v, want exactly one send", sess.varps)
	}
	if sess.varps[0] != [2]int{DateMinutesVarp, 100} {
		t.Errorf("varps[0] = %v, want [3078 100]", sess.varps[0])
	}
}

func TestSendResendsOnRollover(t *testing.T) {
	now := int64(60_000 * 100)
	s := newPiece(func() int64 { return now })
	p, sess := newTestPlayer()
	s.send(p)
	now = 60_000*101 + 30_000 // Halfway through the next minute.
	s.send(p)
	if len(sess.varps) != 2 {
		t.Fatalf("varps = %v, want two sends", sess.varps)
	}
	if sess.varps[1] != [2]int{DateMinutesVarp, 101} {
		t.Errorf("varps[1] = %v, want [3078 101]", sess.varps[1])
	}
}

func TestSendTracksPlayersIndependently(t *testing.T) {
	s := newPiece(func() int64 { return 60_000 * 100 })
	p1, sess1 := newTestPlayer()
	p2, sess2 := newTestPlayer()
	s.send(p1)
	s.send(p2)
	s.send(p1)
	if len(sess1.varps) != 1 || len(sess2.varps) != 1 {
		t.Fatalf("varps = %v / %v, want one send each", sess1.varps, sess2.varps)
	}
}

func TestForgetClearsDedup(t *testing.T) {
	s := newPiece(func() int64 { return 60_000 * 100 })
	p, sess := newTestPlayer()
	s.send(p)
	s.forget(p)
	// Same minute, but the entry is gone: a relogin resends.
	s.send(p)
	if len(sess.varps) != 2 {
		t.Fatalf("varps = %v, want two sends after forget", sess.varps)
	}
}

func TestPluginRegistersThroughHub(t *testing.T) {
	// The exported Plugin value registers its hooks through a real
	// Hub.Load, and a login emit sends the varp exactly once even
	// when a process emit follows in the same minute.
	now := int64(60_000 * 100)
	s := newPiece(func() int64 { return now })
	hub := plugins.NewHub([]plugins.Plugin{s.plugin()}, plugins.HubOptions{})
	hub.Load()
	if got := hub.Loaded(); len(got) != 1 || got[0] != "DateMinutes" {
		t.Fatalf("Loaded() = %v, want [DateMinutes]", got)
	}
	p, sess := newTestPlayer()
	hub.EmitPlayerLogin(&plugins.PlayerLoginEvent{Player: p, Username: p.Username})
	hub.EmitPlayerProcess(&plugins.PlayerProcessEvent{Player: p})
	if len(sess.varps) != 1 {
		t.Fatalf("varps = %v, want exactly one send", sess.varps)
	}
	hub.EmitPlayerDisconnect(&plugins.PlayerDisconnectEvent{Player: p, Username: p.Username, Source: "removed"})
	hub.EmitPlayerLogin(&plugins.PlayerLoginEvent{Player: p, Username: p.Username})
	if len(sess.varps) != 2 {
		t.Fatalf("varps = %v, want a resend after disconnect cleanup", sess.varps)
	}
}

func TestPluginValueShape(t *testing.T) {
	if Plugin.Name != "DateMinutes" {
		t.Errorf("Plugin.Name = %q, want DateMinutes", Plugin.Name)
	}
	if Plugin.Register == nil {
		t.Error("Plugin.Register is nil")
	}
	if Plugin.Members {
		t.Error("Plugin.Members is true; the TS plugin sets no members flag")
	}
	if len(Plugin.DependsOn) != 0 {
		t.Errorf("Plugin.DependsOn = %v, want none", Plugin.DependsOn)
	}
}
