package session

import (
	"bytes"
	"testing"

	"tsps-go/internal/protocol"
)

// recordingInterfaceDeps captures the interface deps calls a
// session makes, in order.
type recordingInterfaceDeps struct {
	calls       []string
	interfaceID int
	owner       any
	prompt      bool
}

func (r *recordingInterfaceDeps) deps() InterfaceDeps {
	return InterfaceDeps{
		EmitInterfaceClosed: func(id int) { r.calls = append(r.calls, "closed") },
		EmitInterfaceOpened: func(groupID, targetUID, typ int) {
			r.calls = append(r.calls, "opened")
		},
		GetInterfaceID:      func() int { return r.interfaceID },
		SetInterfaceID:      func(id int) { r.interfaceID = id; r.calls = append(r.calls, "setId") },
		ResetDialogue:       func() { r.calls = append(r.calls, "resetDialogue") },
		DialogueOwner:       func() any { return r.owner },
		HasInputPrompt:      func() bool { return r.prompt },
		AbortEnteredActions: func() { r.calls = append(r.calls, "abortEntered") },
		CloseShop:           func() { r.calls = append(r.calls, "closeShop") },
		SetStatusNone:       func() { r.calls = append(r.calls, "setStatusNone") },
		SetDestroyPending:   func() { r.calls = append(r.calls, "destroyPending") },
		SetCanChangeAppearance: func(can bool) {
			r.calls = append(r.calls, "appearance")
		},
		EndBankSearch: func(id int) { r.calls = append(r.calls, "endBankSearch") },
	}
}

func TestMountSubInterfaceTracksAndEmits(t *testing.T) {
	s := NewSession(NewHeadlessChannel(""))
	rec := &recordingInterfaceDeps{interfaceID: -1}
	s.SetInterfaceDeps(rec.deps())
	mount := s.MountSubInterface((161<<16)|16, 300, 0)
	if mount == nil || mount.TargetUID != (161<<16)|16 || mount.GroupID != 300 || mount.Type != 0 {
		t.Fatalf("mount = %+v, want target 161:16 group 300 type 0", mount)
	}
	if !s.IsSubInterfaceOpen(300) {
		t.Fatal("IsSubInterfaceOpen(300) = false after mount")
	}
	got, ok := s.GetSubInterfaceAt((161 << 16) | 16)
	if !ok || got != mount {
		t.Fatalf("GetSubInterfaceAt = %+v, %v, want the mount", got, ok)
	}
	if len(rec.calls) != 1 || rec.calls[0] != "opened" {
		t.Fatalf("deps calls = %v, want [opened]", rec.calls)
	}
	packets := queuedPackets(t, s)
	want := protocol.EncodeWidgetOpenSub((161<<16)|16, 300, 0, protocol.WidgetOpenSubOptions{})
	if len(packets) != 1 || !bytes.Equal(packets[0], want) {
		t.Fatalf("packets = %x, want [%x]", packets, want)
	}
}

func TestMountSameGroupElsewhereClosesOldMount(t *testing.T) {
	s := NewSession(NewHeadlessChannel(""))
	s.MountSubInterface((161<<16)|16, 300, 0)
	s.MountSubInterface((161<<16)|17, 300, 0)
	if _, ok := s.GetSubInterfaceAt((161 << 16) | 16); ok {
		t.Fatal("old target still has a mount after remount elsewhere")
	}
	if !s.IsSubInterfaceOpen(300) {
		t.Fatal("group 300 not tracked after remount")
	}
	packets := queuedPackets(t, s)
	// mount, close-sub for the old target, mount.
	if len(packets) != 3 {
		t.Fatalf("queued %d packets, want 3", len(packets))
	}
	if want := protocol.EncodeWidgetCloseSub((161 << 16) | 16); !bytes.Equal(packets[1], want) {
		t.Fatalf("packet 1 = %x, want close-sub %x", packets[1], want)
	}
}

func TestMountDifferentGroupAtTargetForgetsSilently(t *testing.T) {
	s := NewSession(NewHeadlessChannel(""))
	s.MountSubInterface((161<<16)|16, 300, 0)
	s.MountSubInterface((161<<16)|16, 301, 0)
	if s.IsSubInterfaceOpen(300) {
		t.Fatal("group 300 still tracked after replacement at its target")
	}
	if !s.IsSubInterfaceOpen(301) {
		t.Fatal("group 301 not tracked after mount")
	}
	// Two open-sub frames only: no close frame for the replaced
	// group.
	packets := queuedPackets(t, s)
	if len(packets) != 2 {
		t.Fatalf("queued %d packets, want 2", len(packets))
	}
}

func TestMountInventoryGroupSendsFlagsRange(t *testing.T) {
	s := NewSession(NewHeadlessChannel(""))
	s.MountSubInterface((161<<16)|79, protocol.MainInventoryGroupID, 1)
	packets := queuedPackets(t, s)
	if len(packets) != 2 {
		t.Fatalf("queued %d packets, want 2 (open-sub + flags range)", len(packets))
	}
	want := protocol.EncodeWidgetSetFlagsRange(protocol.MainInventoryWidgetUID, 0, 27, protocol.MainInventorySlotFlags)
	if !bytes.Equal(packets[1], want) {
		t.Fatalf("packet 1 = %x, want flags range %x", packets[1], want)
	}
}

func TestHasInterruptibleInterfaceTypeRule(t *testing.T) {
	s := NewSession(NewHeadlessChannel(""))
	if s.HasInterruptibleInterface() {
		t.Fatal("HasInterruptibleInterface = true with nothing mounted")
	}
	s.MountSubInterface((161<<16)|79, 149, 1)
	if s.HasInterruptibleInterface() {
		t.Fatal("type-1 mount counts as interruptible")
	}
	s.MountSubInterface((161<<16)|80, 200, 2)
	if s.HasInterruptibleInterface() {
		t.Fatal("type-2 mount counts as interruptible")
	}
	s.MountSubInterface((161<<16)|81, 201, 3)
	if !s.HasInterruptibleInterface() {
		t.Fatal("type-3 mount does not count as interruptible")
	}
	s.CloseSubInterface((161 << 16) | 81)
	if s.HasInterruptibleInterface() {
		t.Fatal("type-3 mount still counts after close")
	}
	s.MountSubInterface((161<<16)|16, 300, 0)
	if !s.HasInterruptibleInterface() {
		t.Fatal("type-0 mount does not count as interruptible")
	}
}

func TestCloseSubInterfaceMountIdentity(t *testing.T) {
	s := NewSession(NewHeadlessChannel(""))
	first := s.MountSubInterface((161<<16)|16, 300, 0)
	second := s.MountSubInterface((161<<16)|16, 300, 0)
	if first == second {
		t.Fatal("remount returned the same mount identity")
	}
	// The replaced mount cannot close the replacement.
	if s.CloseSubInterfaceMount(first) {
		t.Fatal("CloseSubInterfaceMount(stale) = true")
	}
	if !s.IsSubInterfaceOpen(300) {
		t.Fatal("stale close removed the current mount")
	}
	if !s.CloseSubInterfaceMount(second) {
		t.Fatal("CloseSubInterfaceMount(current) = false")
	}
	if s.IsSubInterfaceOpen(300) {
		t.Fatal("current mount still tracked after close")
	}
	if s.CloseSubInterfaceMount(nil) {
		t.Fatal("CloseSubInterfaceMount(nil) = true")
	}
}

func TestCloseSubInterfaceForgetsAndSendsAlways(t *testing.T) {
	s := NewSession(NewHeadlessChannel(""))
	s.MountSubInterface((161<<16)|16, 300, 0)
	s.CloseSubInterface((161 << 16) | 16)
	if s.IsSubInterfaceOpen(300) {
		t.Fatal("mount still tracked after CloseSubInterface")
	}
	// Closing an untracked target still sends the frame.
	s.CloseSubInterface((161 << 16) | 99)
	packets := queuedPackets(t, s)
	if len(packets) != 3 {
		t.Fatalf("queued %d packets, want 3 (open + 2 closes)", len(packets))
	}
	if want := protocol.EncodeWidgetCloseSub((161 << 16) | 99); !bytes.Equal(packets[2], want) {
		t.Fatalf("packet 2 = %x, want close-sub %x", packets[2], want)
	}
}

func TestCloseSubInterfaceChatboxResetsUnclamp(t *testing.T) {
	s := NewSession(NewHeadlessChannel(""))
	s.SendChatboxInterface(219, true)
	queuedPackets(t, s) // drain the mount frames
	s.CloseSubInterface(protocol.ChatboxModalTargetUID)
	packets := queuedPackets(t, s)
	// Mount frames (3) + varbit 0 + close-sub.
	if len(packets) != 5 {
		t.Fatalf("queued %d packets, want 5", len(packets))
	}
	if want := protocol.EncodeVarbit(protocol.ChatboxUnclampVarbit, 0); !bytes.Equal(packets[3], want) {
		t.Fatalf("packet 3 = %x, want unclamp varbit 0 %x", packets[3], want)
	}
	if want := protocol.EncodeWidgetCloseSub(protocol.ChatboxModalTargetUID); !bytes.Equal(packets[4], want) {
		t.Fatalf("packet 4 = %x, want close-sub %x", packets[4], want)
	}
	// A second close sends no varbit: already clamped.
	s.CloseSubInterface(protocol.ChatboxModalTargetUID)
	packets = queuedPackets(t, s)
	if len(packets) != 6 {
		t.Fatalf("queued %d packets, want 6 (one more close-sub only)", len(packets))
	}
}

func TestChatboxGroupAndOwnerAccessors(t *testing.T) {
	s := NewSession(NewHeadlessChannel(""))
	if got := s.GetChatboxGroupId(); got != -1 {
		t.Fatalf("GetChatboxGroupId() = %d, want -1", got)
	}
	if got := s.GetChatboxOwner(); got != nil {
		t.Fatalf("GetChatboxOwner() = %v, want nil", got)
	}
	owner := "dialogue-owner"
	s.WithChatboxOwner(owner, func() { s.SendChatboxInterface(231, false) })
	if got := s.GetChatboxGroupId(); got != 231 {
		t.Fatalf("GetChatboxGroupId() = %d, want 231", got)
	}
	if got := s.GetChatboxOwner(); got != owner {
		t.Fatalf("GetChatboxOwner() = %v, want the owner", got)
	}
	if !s.IsChatboxInterface(231) || s.IsChatboxInterface(217) {
		t.Fatal("IsChatboxInterface mismatch after mount")
	}
}

func TestCloseChatboxForOwnerRules(t *testing.T) {
	owner := "owner-a"
	other := "owner-b"
	newMounted := func() (*Session, *recordingInterfaceDeps) {
		s := NewSession(NewHeadlessChannel(""))
		rec := &recordingInterfaceDeps{interfaceID: -1, owner: owner}
		s.SetInterfaceDeps(rec.deps())
		s.WithChatboxOwner(owner, func() { s.SendChatboxInterface(231, false) })
		return s, rec
	}
	// Wrong owner: no close, no frames beyond the mount.
	s, _ := newMounted()
	before := len(queuedPackets(t, s))
	if s.CloseChatboxFor(other) {
		t.Fatal("CloseChatboxFor(other) = true")
	}
	if len(queuedPackets(t, s)) != before {
		t.Fatal("CloseChatboxFor(other) queued frames")
	}
	if s.GetChatboxGroupId() != 231 {
		t.Fatal("CloseChatboxFor(other) cleared the chatbox")
	}
	// Right owner: close, owner cleared, dialogue reset (the
	// dialogue owner matches), closed emitted.
	s, rec := newMounted()
	rec.calls = nil
	if !s.CloseChatboxFor(owner) {
		t.Fatal("CloseChatboxFor(owner) = false")
	}
	if got := s.GetChatboxGroupId(); got != -1 {
		t.Fatalf("GetChatboxGroupId() = %d, want -1", got)
	}
	if got := s.GetChatboxOwner(); got != nil {
		t.Fatalf("GetChatboxOwner() = %v, want nil", got)
	}
	packets := queuedPackets(t, s)
	// Mount (2) + close-sub + hide.
	if len(packets) != 4 {
		t.Fatalf("queued %d packets, want 4", len(packets))
	}
	if want := protocol.EncodeWidgetCloseSub(protocol.ChatboxModalTargetUID); !bytes.Equal(packets[2], want) {
		t.Fatalf("packet 2 = %x, want close-sub %x", packets[2], want)
	}
	if want := protocol.EncodeWidgetSetHidden(protocol.ChatboxModalTargetUID, true); !bytes.Equal(packets[3], want) {
		t.Fatalf("packet 3 = %x, want hide %x", packets[3], want)
	}
	wantCalls := []string{"resetDialogue", "closed"}
	if len(rec.calls) != len(wantCalls) {
		t.Fatalf("deps calls = %v, want %v", rec.calls, wantCalls)
	}
	for i, want := range wantCalls {
		if rec.calls[i] != want {
			t.Fatalf("deps calls = %v, want %v", rec.calls, wantCalls)
		}
	}
	// No chatbox mounted: false.
	s2 := NewSession(NewHeadlessChannel(""))
	if s2.CloseChatboxFor(owner) {
		t.Fatal("CloseChatboxFor with no chatbox = true")
	}
	// Dialogue owned by someone else: close without reset.
	s, rec = newMounted()
	rec.owner = other
	rec.calls = nil
	if !s.CloseChatboxFor(owner) {
		t.Fatal("CloseChatboxFor(owner) = false with foreign dialogue owner")
	}
	for _, call := range rec.calls {
		if call == "resetDialogue" {
			t.Fatalf("deps calls = %v, want no dialogue reset", rec.calls)
		}
	}
}

func TestMountChatboxInterfaceReplaceEmitsClosedAndTracks(t *testing.T) {
	s := NewSession(NewHeadlessChannel(""))
	rec := &recordingInterfaceDeps{interfaceID: -1}
	s.SetInterfaceDeps(rec.deps())
	s.mountChatboxInterface(231, false)
	if !s.IsSubInterfaceOpen(231) {
		t.Fatal("chatbox group 231 not tracked after mount")
	}
	if !s.HasInterruptibleInterface() {
		t.Fatal("mounted chatbox not interruptible")
	}
	rec.calls = nil
	s.mountChatboxInterface(217, false)
	if s.IsSubInterfaceOpen(231) {
		t.Fatal("replaced chatbox group still tracked")
	}
	if !s.IsSubInterfaceOpen(217) {
		t.Fatal("new chatbox group not tracked")
	}
	if len(rec.calls) != 1 || rec.calls[0] != "closed" {
		t.Fatalf("deps calls = %v, want [closed]", rec.calls)
	}
	// Wire output is the chatbox bundle for each mount: 2 + 2.
	packets := queuedPackets(t, s)
	if len(packets) != 4 {
		t.Fatalf("queued %d packets, want 4", len(packets))
	}
	want := protocol.EncodeChatboxInterfaceFrames(217, false, false)
	if !bytes.Equal(packets[2], want[0]) || !bytes.Equal(packets[3], want[1]) {
		t.Fatalf("second mount packets = %x %x, want %x %x", packets[2], packets[3], want[0], want[1])
	}
	// Same group again: no closed emit.
	rec.calls = nil
	s.mountChatboxInterface(217, false)
	if len(rec.calls) != 0 {
		t.Fatalf("same-group remount emitted %v", rec.calls)
	}
}
