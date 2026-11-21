package game

import (
	"reflect"
	"testing"
)

// A SessionPort that does not implement ChatMessageSender: the
// assertion in SendMessage must drop, not panic, for it.
type chatlessSession struct{ *fakeSession }

func TestSendMessageForwardsToChatSender(t *testing.T) {
	p := NewPlayer("testplayer", Location{X: 3089, Y: 3524, Z: 0})
	sess := newFakeSession()
	p.Session = sess
	p.SendMessage("Hello, world.")
	p.SendMessage("")
	if want := []string{"Hello, world.", ""}; !reflect.DeepEqual(sess.messages, want) {
		t.Fatalf("messages = %v, want %v", sess.messages, want)
	}
}

func TestSendMessageDropsWithoutSession(t *testing.T) {
	// No session: nothing to send to, no panic. A nil player
	// likewise drops (the TS container null check,
	// ItemContainer.ts:539).
	p := NewPlayer("testplayer", Location{X: 3089, Y: 3524, Z: 0})
	p.SendMessage("dropped")
	var nilPlayer *Player
	nilPlayer.SendMessage("dropped")
}

func TestSendMessageDropsWithoutChatExtension(t *testing.T) {
	// A SessionPort without the ChatMessageSender extension
	// drops the message: the assertion fails closed.
	p := NewPlayer("testplayer", Location{X: 3089, Y: 3524, Z: 0})
	p.Session = bareSessionPort{}
	p.SendMessage("dropped")
	p.SendTypedMessage(105, "dropped", "")
}

// bareSessionPort implements SessionPort and nothing else.
type bareSessionPort struct{}

func (bareSessionPort) SendPacket([]byte)                 {}
func (bareSessionPort) SendRunEnergy(int, bool, int, int) {}
func (bareSessionPort) SendDestination(int, int)          {}
func (bareSessionPort) SendVarp(int, int)                 {}
func (bareSessionPort) FlushTick(int64) error             { return nil }
func (bareSessionPort) Close(int, string)                 {}
func (bareSessionPort) IsOpen() bool                      { return true }

func TestSendMessageWatchedBotReceives(t *testing.T) {
	// A bot WITH an attached session is a watched bot
	// (::botcontrol): TS tells it what a player is told
	// (Mobile.ts:999-1006, Player.ts:1538-1540). A bot with no
	// session drops, as any session-less player does.
	p := NewPlayer("bot", Location{X: 3089, Y: 3524, Z: 0})
	p.IsBot = true
	sess := newFakeSession()
	p.Session = sess
	p.SendMessage("watched")
	if want := []string{"watched"}; !reflect.DeepEqual(sess.messages, want) {
		t.Fatalf("watched bot messages = %v, want %v", sess.messages, want)
	}
	p.Session = nil
	p.SendMessage("unwatched")
	if len(sess.messages) != 1 {
		t.Fatalf("unwatched bot messages = %v, want no second send", sess.messages)
	}
}

func TestSendTypedMessageForwards(t *testing.T) {
	p := NewPlayer("testplayer", Location{X: 3089, Y: 3524, Z: 0})
	sess := newFakeSession()
	p.Session = sess
	// ChatTypeSpam = 105, ChatTypeNPCExamine = 28 (protocol
	// constants; the game layer passes plain ints).
	p.SendTypedMessage(105, "You're stunned!", "")
	p.SendTypedMessage(28, "A man.", "Man")
	want := [][3]any{{105, "You're stunned!", ""}, {28, "A man.", "Man"}}
	if !reflect.DeepEqual(sess.typedMessages, want) {
		t.Fatalf("typedMessages = %v, want %v", sess.typedMessages, want)
	}
	if len(sess.messages) != 0 {
		t.Fatalf("typed sends leaked into plain messages: %v", sess.messages)
	}
}

func TestSendTypedMessageDropsWithoutSession(t *testing.T) {
	p := NewPlayer("testplayer", Location{X: 3089, Y: 3524, Z: 0})
	p.SendTypedMessage(105, "dropped", "")
	var nilPlayer *Player
	nilPlayer.SendTypedMessage(105, "dropped", "")
}
