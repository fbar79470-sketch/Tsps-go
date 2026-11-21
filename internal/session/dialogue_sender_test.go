package session

import (
	"bytes"
	"testing"

	"tsps-go/internal/game"
	"tsps-go/internal/protocol"
)

func TestDialogueMountQueuesMountFramesAndRecordsGroup(t *testing.T) {
	s := NewSession(NewHeadlessChannel(""))
	if s.IsChatboxInterface(231) {
		t.Fatal("IsChatboxInterface(231) = true before any mount")
	}
	s.SendChatboxInterface(231, false)
	if !s.IsChatboxInterface(231) {
		t.Fatal("IsChatboxInterface(231) = false after mounting 231")
	}
	if s.IsChatboxInterface(217) {
		t.Fatal("IsChatboxInterface(217) = true after mounting 231")
	}
	packets := queuedPackets(t, s)
	want := protocol.EncodeChatboxInterfaceFrames(231, false, false)
	if len(packets) != len(want) {
		t.Fatalf("queued %d packets, want %d", len(packets), len(want))
	}
	for i := range want {
		if !bytes.Equal(packets[i], want[i]) {
			t.Errorf("packet %d = %x, want %x", i, packets[i], want[i])
		}
	}
	// A clamped mount after a clamped mount sends no varbit: two
	// frames only (unhide + open-sub).
	if len(packets) != 2 {
		t.Errorf("clamped-after-clamped mount queued %d packets, want 2 (no varbit)", len(packets))
	}
}

func TestDialogueUnclampVarbitTransitions(t *testing.T) {
	// Unclamped mount sends varbit 1 first.
	s := NewSession(NewHeadlessChannel(""))
	s.SendChatboxInterface(219, true)
	packets := queuedPackets(t, s)
	if len(packets) != 3 {
		t.Fatalf("unclamped mount queued %d packets, want 3", len(packets))
	}
	if want := protocol.EncodeVarbit(protocol.ChatboxUnclampVarbit, 1); !bytes.Equal(packets[0], want) {
		t.Errorf("first packet = %x, want unclamp varbit 1 %x", packets[0], want)
	}
	// Clamped mount after an unclamped one sends varbit 0 first.
	// (queuedPackets is cumulative, so slice past the first mount.)
	s.SendChatboxInterface(231, false)
	packets = queuedPackets(t, s)[3:]
	if len(packets) != 3 {
		t.Fatalf("clamped-after-unclamped mount queued %d packets, want 3", len(packets))
	}
	if want := protocol.EncodeVarbit(protocol.ChatboxUnclampVarbit, 0); !bytes.Equal(packets[0], want) {
		t.Errorf("first packet = %x, want unclamp varbit 0 %x", packets[0], want)
	}
	// Closing a clamped chatbox sends no varbit.
	s.SendInterfaceRemoval()
	packets = queuedPackets(t, s)[6:]
	if len(packets) != 2 {
		t.Fatalf("close of clamped chatbox queued %d packets, want 2", len(packets))
	}
	if s.IsChatboxInterface(231) {
		t.Fatal("IsChatboxInterface(231) = true after removal")
	}
	// Closing an unclamped chatbox sends varbit 0 first.
	s2 := NewSession(NewHeadlessChannel(""))
	s2.SendChatboxInterface(219, true)
	s2.SendInterfaceRemoval()
	packets = queuedPackets(t, s2)[3:]
	if len(packets) != 3 {
		t.Fatalf("close of unclamped chatbox queued %d packets, want 3", len(packets))
	}
	if want := protocol.EncodeVarbit(protocol.ChatboxUnclampVarbit, 0); !bytes.Equal(packets[0], want) {
		t.Errorf("first packet = %x, want unclamp varbit 0 %x", packets[0], want)
	}
}

func TestDialogueRemovalWithNothingMountedSendsNothing(t *testing.T) {
	s := NewSession(NewHeadlessChannel(""))
	s.SendInterfaceRemoval()
	if packets := queuedPackets(t, s); len(packets) != 0 {
		t.Fatalf("removal with nothing mounted queued %d packets, want 0", len(packets))
	}
}

func TestDialogueOwnerScoping(t *testing.T) {
	s := NewSession(NewHeadlessChannel(""))
	owner := struct{ name string }{"quest"}
	var nestedOwner any
	s.WithChatboxOwner(owner, func() {
		s.SendChatboxInterface(231, false)
		s.mu.Lock()
		nestedOwner = s.chatboxOwner
		s.mu.Unlock()
		// Nested presentation overrides, then restores.
		inner := struct{ name string }{"inner"}
		s.WithChatboxOwner(inner, func() {
			s.SendChatboxInterface(217, false)
			s.mu.Lock()
			if s.chatboxOwner != any(inner) {
				t.Errorf("nested mount owner = %v, want inner", s.chatboxOwner)
			}
			s.mu.Unlock()
		})
		s.mu.Lock()
		if s.presentingChatboxOwner != any(owner) {
			t.Errorf("presenting owner after nested presentation = %v, want outer", s.presentingChatboxOwner)
		}
		s.mu.Unlock()
	})
	if nestedOwner != any(owner) {
		t.Errorf("mount owner = %v, want outer owner", nestedOwner)
	}
	s.mu.Lock()
	restored := s.presentingChatboxOwner
	s.mu.Unlock()
	if restored != nil {
		t.Errorf("presenting owner after presentation = %v, want nil", restored)
	}
	// A mount outside any presentation records a nil owner.
	s.SendChatboxInterface(229, false)
	s.mu.Lock()
	if s.chatboxOwner != nil {
		t.Errorf("unscoped mount owner = %v, want nil", s.chatboxOwner)
	}
	s.mu.Unlock()
}

func TestDialoguePrimitiveSenders(t *testing.T) {
	s := NewSession(NewHeadlessChannel(""))
	uid := (231 << 16) | 2
	s.SendNpcHeadOnInterface(6797, uid)
	s.SendPlayerHeadOnInterface(uid)
	s.SendInterfaceAnimation(uid, 599)
	s.SendString("Hello.", uid)
	s.SendClientScript(58, game.ClientScriptString("Title"), game.ClientScriptInt(3))
	s.SendInterfaceFlagsRange(uid, 1, 3, 1)
	s.SendItemOnInterfaces(uid, 995, 400)
	s.SendInterfaceScript(2379)
	packets := queuedPackets(t, s)
	want := [][]byte{
		protocol.EncodeWidgetSetNpcHead(uid, 6797),
		protocol.EncodeWidgetSetPlayerHead(uid),
		protocol.EncodeWidgetSetAnimation(uid, 599),
		protocol.EncodeWidgetSetText(uid, "Hello.", nil),
		protocol.EncodeRunClientScript(58, []protocol.ScriptArg{protocol.ScriptArgString("Title"), protocol.ScriptArgInt(3)}),
		protocol.EncodeWidgetSetFlagsRange(uid, 1, 3, 1),
		protocol.EncodeWidgetSetItem(uid, 995, 400),
		protocol.EncodeWidgetRunScript(2379, nil, nil, nil, nil),
	}
	if len(packets) != len(want) {
		t.Fatalf("queued %d packets, want %d", len(packets), len(want))
	}
	for i := range want {
		if !bytes.Equal(packets[i], want[i]) {
			t.Errorf("packet %d = %x, want %x", i, packets[i], want[i])
		}
	}
}
