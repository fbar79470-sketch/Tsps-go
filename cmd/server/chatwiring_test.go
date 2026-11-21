package main

import (
	"bytes"
	"log"
	"os"
	"path/filepath"
	"testing"

	"tsps-go/internal/definition"
	"tsps-go/internal/game"
	"tsps-go/internal/listeners"
	"tsps-go/internal/plugins"
	"tsps-go/internal/protocol"
)

// packetRecordingSession is a game.SessionPort test double
// recording the raw packets it receives.
type packetRecordingSession struct {
	packets [][]byte
}

func (s *packetRecordingSession) SendPacket(packet []byte)          { s.packets = append(s.packets, packet) }
func (s *packetRecordingSession) SendRunEnergy(int, bool, int, int) {}
func (s *packetRecordingSession) SendDestination(int, int)          {}
func (s *packetRecordingSession) SendVarp(int, int)                 {}
func (s *packetRecordingSession) FlushTick(int64) error             { return nil }
func (s *packetRecordingSession) Close(int, string)                 {}
func (s *packetRecordingSession) IsOpen() bool                      { return true }

var _ game.SessionPort = (*packetRecordingSession)(nil)

// TestChatDelivery pins the broadcast adapter mapping: one
// CHAT_MESSAGE frame per call, with the public kind, the plain
// text, the icon-prefixed from name, the speaker index, the
// autochat type override when present, and the style passthrough.
// The encoder's own bytes are pinned in the protocol package;
// this pins the adapter's field mapping against EncodeChatMessage.
func TestChatDelivery(t *testing.T) {
	recipient := game.NewPlayer("bob", game.Location{X: 3094, Y: 3524, Z: 0})
	sess := &packetRecordingSession{}
	recipient.Session = sess

	style := &protocol.ChatStyle{Colour: 1, Effect: 2}
	chatType := protocol.ChatTypeAutotyper
	chatDelivery{}.DeliverPublicChat(recipient, listeners.PublicChatMessage{
		Text:         "hello",
		From:         "<img=0>Alice",
		SpeakerIndex: 7,
		AutoChat:     true,
		ChatType:     &chatType,
		Style:        style,
	})
	if len(sess.packets) != 1 {
		t.Fatalf("packets = %d, want 1", len(sess.packets))
	}
	want := protocol.EncodeChatMessage(protocol.OutgoingChatMessage{
		Kind:        protocol.ChatKindPublic,
		Text:        "hello",
		From:        "<img=0>Alice",
		PlayerID:    7,
		ChatType:    protocol.ChatTypeAutotyper,
		HasChatType: true,
		Style:       style,
	})
	if !bytes.Equal(sess.packets[0], want) {
		t.Errorf("packet = %x\nwant   %x", sess.packets[0], want)
	}

	// No chat-type override: the default public type applies and
	// no style is written.
	sess.packets = nil
	chatDelivery{}.DeliverPublicChat(recipient, listeners.PublicChatMessage{Text: "hi", From: "Alice", SpeakerIndex: 7})
	want = protocol.EncodeChatMessage(protocol.OutgoingChatMessage{
		Kind:     protocol.ChatKindPublic,
		Text:     "hi",
		From:     "Alice",
		PlayerID: 7,
	})
	if len(sess.packets) != 1 || !bytes.Equal(sess.packets[0], want) {
		t.Errorf("packets = %x, want %x", sess.packets, want)
	}

	// No session, or no recipient: nothing is sent, no panic.
	recipient.Session = nil
	chatDelivery{}.DeliverPublicChat(recipient, listeners.PublicChatMessage{Text: "hi"})
	chatDelivery{}.DeliverPublicChat(nil, listeners.PublicChatMessage{Text: "hi"})
	if len(sess.packets) != 1 {
		t.Errorf("packets = %d after session-less delivery, want 1", len(sess.packets))
	}
}

// TestStoreExamineSource pins the examine adapter over a no-cache
// Store: unknown ids answer the DEFAULT shapes (the generic line
// for NPCs/objects, the empty examine-or-name for items), and a
// nil store degrades without panicking. The multi-loc variant
// rule composes Store.ObjectExamineText + Store.ObjectForPlayer,
// both covered by the definition package's own tests; no fake
// cache facade exists to drive the variant branch here.
func TestStoreExamineSource(t *testing.T) {
	dir := t.TempDir()
	for name, content := range map[string]string{
		"interface-layouts.json": `{}`,
		"object-spawns.json":     `[]`,
		"ground-items.json":      `[]`,
		"npc-combat-defs.json":   `{}`,
		"npc-animations.json":    `{}`,
		"shops.json":             `[]`,
		"custom-shops.json":      `[]`,
		"npc-spawns.json":        `[]`,
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	store, err := definition.Load(dir, definition.LoadOptions{})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	src := storeExamineSource{store: store}
	if got := src.NpcExamine(999); got != "It's nothing special." {
		t.Errorf("NpcExamine = %q", got)
	}
	if got := src.ObjectExamine(999); got != "It's nothing special." {
		t.Errorf("ObjectExamine = %q", got)
	}
	if got := src.ItemExamine(999); got != "" {
		t.Errorf("ItemExamine = %q, want empty (DEFAULT examine + name)", got)
	}

	nilSrc := storeExamineSource{}
	if got := nilSrc.NpcExamine(1); got != "It's nothing special." {
		t.Errorf("nil NpcExamine = %q", got)
	}
	if got := nilSrc.ObjectExamine(1); got != "It's nothing special." {
		t.Errorf("nil ObjectExamine = %q", got)
	}
	if got := nilSrc.ItemExamine(1); got != "" {
		t.Errorf("nil ItemExamine = %q", got)
	}
}

// TestObjectExamineIntercept pins the hub adapter: a nil hub never
// intercepts, a handler that sets Handled suppresses the answer,
// and a handler that leaves it unset lets the answer through.
func TestObjectExamineIntercept(t *testing.T) {
	p := game.NewPlayer("testplayer", game.Location{X: 3089, Y: 3524, Z: 0})
	if objectExamineIntercept(nil)(p, 1) {
		t.Error("nil hub intercepted")
	}

	handled := false
	hub := plugins.NewHub([]plugins.Plugin{{
		Name: "Probe",
		Register: func(api *plugins.Api) {
			api.OnCustomEvent("world:object-examine", func(payload any) {
				event, ok := payload.(*plugins.ObjectExamineEvent)
				if !ok {
					t.Errorf("payload type %T, want *ObjectExamineEvent", payload)
					return
				}
				if event.ObjectID != 42 {
					t.Errorf("ObjectID = %d, want 42", event.ObjectID)
				}
				event.Handled = handled
			})
		},
	}}, plugins.HubOptions{Logger: log.Default()})
	hub.Load()

	intercept := objectExamineIntercept(hub)
	if intercept(p, 42) {
		t.Error("unset Handled intercepted")
	}
	handled = true
	if !intercept(p, 42) {
		t.Error("set Handled did not intercept")
	}
}
