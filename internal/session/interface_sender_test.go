package session

import (
	"bytes"
	"strings"
	"testing"

	"tsps-go/internal/definition"
	"tsps-go/internal/protocol"
)

func TestSendInterfaceSetsIdAndMounts(t *testing.T) {
	s := NewSession(NewHeadlessChannel(""))
	rec := &recordingInterfaceDeps{interfaceID: -1}
	s.SetInterfaceDeps(rec.deps())
	s.SendInterface(300)
	if rec.interfaceID != 300 {
		t.Fatalf("interface id = %d, want 300", rec.interfaceID)
	}
	mount, ok := s.GetSubInterfaceAt(mainModalTargetUID)
	if !ok || mount.GroupID != 300 || mount.Type != 0 {
		t.Fatalf("main modal mount = %+v, %v, want group 300 type 0", mount, ok)
	}
	if len(rec.calls) != 2 || rec.calls[0] != "setId" || rec.calls[1] != "opened" {
		t.Fatalf("deps calls = %v, want [setId opened]", rec.calls)
	}
}

func TestSendWalkableSidebarAndSet(t *testing.T) {
	s := NewSession(NewHeadlessChannel(""))
	s.SendWalkableInterface(100)
	s.SendSidebarInterface(101)
	packets := queuedPackets(t, s)
	if len(packets) != 2 {
		t.Fatalf("queued %d packets, want 2", len(packets))
	}
	if want := protocol.EncodeWidgetOpen(100, false); !bytes.Equal(packets[0], want) {
		t.Fatalf("packet 0 = %x, want %x", packets[0], want)
	}
	if want := protocol.EncodeWidgetOpen(101, false); !bytes.Equal(packets[1], want) {
		t.Fatalf("packet 1 = %x, want %x", packets[1], want)
	}
	if s.IsSubInterfaceOpen(100) || s.IsSubInterfaceOpen(101) {
		t.Fatal("walkable/sidebar opens are tracked; they must not be")
	}
	// Interface set: main mount then sidebar open, in order.
	s2 := NewSession(NewHeadlessChannel(""))
	rec := &recordingInterfaceDeps{interfaceID: -1}
	s2.SetInterfaceDeps(rec.deps())
	s2.SendInterfaceSet(300, 301)
	if rec.interfaceID != 300 {
		t.Fatalf("interface id = %d, want 300", rec.interfaceID)
	}
	packets = queuedPackets(t, s2)
	if len(packets) != 2 {
		t.Fatalf("queued %d packets, want 2", len(packets))
	}
	if want := protocol.EncodeWidgetOpenSub(mainModalTargetUID, 300, 0, protocol.WidgetOpenSubOptions{}); !bytes.Equal(packets[0], want) {
		t.Fatalf("packet 0 = %x, want main mount %x", packets[0], want)
	}
	if want := protocol.EncodeWidgetOpen(301, false); !bytes.Equal(packets[1], want) {
		t.Fatalf("packet 1 = %x, want sidebar open %x", packets[1], want)
	}
}

func TestSendTabInterfaceTabZero(t *testing.T) {
	s := NewSession(NewHeadlessChannel(""))
	// The interfaceID argument is ignored on tab 0.
	s.SendTabInterface(0, 9999)
	packets := queuedPackets(t, s)
	targetUID := (161 << 16) | 76
	want := [][]byte{
		protocol.EncodeWidgetCloseSub(targetUID),
		protocol.EncodeWidgetOpenSub(targetUID, 593, 1, protocol.WidgetOpenSubOptions{}),
		protocol.EncodeWidgetSetHidden((593<<16)|23, false),
		protocol.EncodeWidgetSetHidden((593<<16)|28, false),
	}
	if len(packets) != len(want) {
		t.Fatalf("queued %d packets, want %d", len(packets), len(want))
	}
	for i := range want {
		if !bytes.Equal(packets[i], want[i]) {
			t.Fatalf("packet %d = %x, want %x", i, packets[i], want[i])
		}
	}
	if !s.IsSubInterfaceOpen(593) {
		t.Fatal("group 593 not tracked after tab-0 mount")
	}
}

func TestSendTabInterfacePrayerAndDefault(t *testing.T) {
	s := NewSession(NewHeadlessChannel(""))
	rec := &recordingInterfaceDeps{interfaceID: -1}
	s.SetInterfaceDeps(rec.deps())
	s.SendTabInterface(5, 1)
	packets := queuedPackets(t, s)
	setup := protocol.EncodeGameframeTabSetup(541, 161)
	want := append([][]byte{
		protocol.EncodeWidgetOpenSub((161<<16)|81, 541, 1, protocol.WidgetOpenSubOptions{}),
	}, setup...)
	if len(packets) != len(want) {
		t.Fatalf("tab 5 queued %d packets, want %d", len(packets), len(want))
	}
	for i := range want {
		if !bytes.Equal(packets[i], want[i]) {
			t.Fatalf("tab 5 packet %d = %x, want %x", i, packets[i], want[i])
		}
	}
	// Tab paths emit no interface:opened.
	if len(rec.calls) != 0 {
		t.Fatalf("tab 5 emitted %v, want nothing", rec.calls)
	}
	// The 17200 variant mounts group 77.
	s2 := NewSession(NewHeadlessChannel(""))
	s2.SendTabInterface(5, 17200)
	packets = queuedPackets(t, s2)
	if want := protocol.EncodeWidgetOpenSub((161<<16)|81, 77, 1, protocol.WidgetOpenSubOptions{}); !bytes.Equal(packets[0], want) {
		t.Fatalf("tab 5 variant packet 0 = %x, want %x", packets[0], want)
	}
	// Default tab: mount at 161:tabId, no setup, no emit.
	s3 := NewSession(NewHeadlessChannel(""))
	s3.SendTabInterface(3, 3213)
	packets = queuedPackets(t, s3)
	if len(packets) != 1 {
		t.Fatalf("default tab queued %d packets, want 1", len(packets))
	}
	if want := protocol.EncodeWidgetOpenSub((161<<16)|3, 3213, 1, protocol.WidgetOpenSubOptions{}); !bytes.Equal(packets[0], want) {
		t.Fatalf("default tab packet = %x, want %x", packets[0], want)
	}
	if !s3.IsSubInterfaceOpen(3213) {
		t.Fatal("default tab mount not tracked")
	}
}

func TestSendTabInterfaceSpellbookUnlocksOnce(t *testing.T) {
	s := NewSession(NewHeadlessChannel(""))
	s.SendTabInterface(6, 12855)
	first := queuedPackets(t, s)
	// Unlock varbits (14) + unlock varps (13) + spellbook varbit
	// + open-sub + tab setup.
	setup := protocol.EncodeGameframeTabSetup(218, 161)
	if len(first) != 14+13+1+1+len(setup) {
		t.Fatalf("first spellbook mount queued %d packets, want %d", len(first), 14+13+1+1+len(setup))
	}
	if want := protocol.EncodeVarbit(4070, 1); !bytes.Equal(first[27], want) {
		t.Fatalf("packet 27 = %x, want spellbook varbit 1 %x", first[27], want)
	}
	if want := protocol.EncodeWidgetOpenSub((161<<16)|82, 218, 1, protocol.WidgetOpenSubOptions{}); !bytes.Equal(first[28], want) {
		t.Fatalf("packet 28 = %x, want open-sub %x", first[28], want)
	}
	// Second mount: no unlock tables, only the spellbook varbit
	// (Lunar = 2), the mount, and the setup.
	s.SendTabInterface(6, 29999)
	second := queuedPackets(t, s)[len(first):]
	if len(second) != 1+1+len(setup) {
		t.Fatalf("second spellbook mount queued %d packets, want %d", len(second), 1+1+len(setup))
	}
	if want := protocol.EncodeVarbit(4070, 2); !bytes.Equal(second[0], want) {
		t.Fatalf("second mount packet 0 = %x, want spellbook varbit 2 %x", second[0], want)
	}
}

func TestSendMagicTabInterface(t *testing.T) {
	s := NewSession(NewHeadlessChannel(""))
	s.SendMagicTabInterface(522)
	packets := queuedPackets(t, s)
	if len(packets) != 1 {
		t.Fatalf("queued %d packets, want 1", len(packets))
	}
	if want := protocol.EncodeWidgetOpenSub((161<<16)|82, 522, 1, protocol.WidgetOpenSubOptions{}); !bytes.Equal(packets[0], want) {
		t.Fatalf("packet = %x, want %x", packets[0], want)
	}
	if !s.IsSubInterfaceOpen(522) {
		t.Fatal("magic tab mount not tracked")
	}
}

func TestAbortInputPrompt(t *testing.T) {
	// Unwired: no frame, no panic.
	s := NewSession(NewHeadlessChannel(""))
	if s.HasInputPrompt() {
		t.Fatal("HasInputPrompt = true unwired")
	}
	s.AbortInputPrompt()
	if len(queuedPackets(t, s)) != 0 {
		t.Fatal("unwired abort queued frames")
	}
	// Armed prompt: abort frame + dep abort.
	s2 := NewSession(NewHeadlessChannel(""))
	rec := &recordingInterfaceDeps{interfaceID: -1, prompt: true}
	s2.SetInterfaceDeps(rec.deps())
	if !s2.HasInputPrompt() {
		t.Fatal("HasInputPrompt = false with armed prompt")
	}
	s2.AbortInputPrompt()
	packets := queuedPackets(t, s2)
	if len(packets) != 1 {
		t.Fatalf("queued %d packets, want 1 (abort frame)", len(packets))
	}
	want, err := protocol.EncodePacket(protocol.ServerTriggerOndialogabort, nil)
	if err != nil {
		t.Fatalf("EncodePacket: %v", err)
	}
	if !bytes.Equal(packets[0], want) {
		t.Fatalf("packet = %x, want abort frame %x", packets[0], want)
	}
	if len(rec.calls) != 1 || rec.calls[0] != "abortEntered" {
		t.Fatalf("deps calls = %v, want [abortEntered]", rec.calls)
	}
	// Unarmed prompt with deps: dep abort still runs (TS clears
	// unconditionally), but no frame goes out.
	s3 := NewSession(NewHeadlessChannel(""))
	rec3 := &recordingInterfaceDeps{interfaceID: -1}
	s3.SetInterfaceDeps(rec3.deps())
	s3.AbortInputPrompt()
	if len(queuedPackets(t, s3)) != 0 {
		t.Fatal("unarmed abort queued a frame")
	}
	if len(rec3.calls) != 1 || rec3.calls[0] != "abortEntered" {
		t.Fatalf("deps calls = %v, want [abortEntered]", rec3.calls)
	}
}

func TestCloseInterruptibleInterfacesNothingOpen(t *testing.T) {
	s := NewSession(NewHeadlessChannel(""))
	rec := &recordingInterfaceDeps{interfaceID: -1}
	s.SetInterfaceDeps(rec.deps())
	s.CloseInterruptibleInterfaces()
	if len(queuedPackets(t, s)) != 0 {
		t.Fatal("close with nothing open queued frames")
	}
	// Reset still ran its dep calls (shop close, status reset,
	// abort, dialogue reset, destroy pending, setId,
	// appearance, bank-search end) even though nothing was
	// open — TS resetInterfaceState + endBankSearch are
	// unconditional; the bank-search dep filters on id 12
	// itself, as TS endBankSearch does.
	want := []string{"closeShop", "setStatusNone", "abortEntered", "resetDialogue", "destroyPending", "setId", "appearance", "endBankSearch"}
	if len(rec.calls) != len(want) {
		t.Fatalf("deps calls = %v, want %v", rec.calls, want)
	}
	for i := range want {
		if rec.calls[i] != want[i] {
			t.Fatalf("deps calls = %v, want %v", rec.calls, want)
		}
	}
}

func TestCloseInterruptibleInterfacesRestoreBranch(t *testing.T) {
	for _, id := range []int{300, 334, 335} {
		s := NewSession(NewHeadlessChannel(""))
		rec := &recordingInterfaceDeps{interfaceID: id}
		s.SetInterfaceDeps(rec.deps())
		s.MountSubInterface(mainModalTargetUID, id, 0)
		rec.calls = nil
		s.CloseInterruptibleInterfaces()
		packets := queuedPackets(t, s)
		// Mount (1) + close-sub (1) + restore mount (open-sub +
		// flags range) (2).
		if len(packets) != 4 {
			t.Fatalf("id %d: queued %d packets, want 4", id, len(packets))
		}
		if want := protocol.EncodeWidgetCloseSub(mainModalTargetUID); !bytes.Equal(packets[1], want) {
			t.Fatalf("id %d: packet 1 = %x, want close-sub %x", id, packets[1], want)
		}
		if want := protocol.EncodeWidgetOpenSub((161<<16)|79, protocol.MainInventoryGroupID, 1, protocol.WidgetOpenSubOptions{}); !bytes.Equal(packets[2], want) {
			t.Fatalf("id %d: packet 2 = %x, want restore mount %x", id, packets[2], want)
		}
		if !s.IsSubInterfaceOpen(protocol.MainInventoryGroupID) {
			t.Fatalf("id %d: inventory group not tracked after restore", id)
		}
		// The closed emit for the outgoing id precedes the
		// restore mount's opened emit.
		closed, opened := -1, -1
		for i, call := range rec.calls {
			if call == "closed" {
				closed = i
			}
			if call == "opened" {
				opened = i
			}
		}
		if closed < 0 || opened < 0 || closed > opened {
			t.Fatalf("id %d: deps calls = %v, want closed before opened", id, rec.calls)
		}
	}
}

func TestCloseInterruptibleInterfacesNoRestoreForOtherIds(t *testing.T) {
	s := NewSession(NewHeadlessChannel(""))
	rec := &recordingInterfaceDeps{interfaceID: 301}
	s.SetInterfaceDeps(rec.deps())
	s.MountSubInterface(mainModalTargetUID, 301, 0)
	s.CloseInterruptibleInterfaces()
	packets := queuedPackets(t, s)
	// Mount + close-sub only: no restore, no fallback close.
	if len(packets) != 2 {
		t.Fatalf("queued %d packets, want 2", len(packets))
	}
}

func TestCloseInterruptibleInterfacesFallbackClose(t *testing.T) {
	s := NewSession(NewHeadlessChannel(""))
	rec := &recordingInterfaceDeps{interfaceID: 300}
	s.SetInterfaceDeps(rec.deps())
	// No tracked mounts: the fallback widget-close frame.
	s.CloseInterruptibleInterfaces()
	packets := queuedPackets(t, s)
	if len(packets) != 1 {
		t.Fatalf("queued %d packets, want 1 (fallback close)", len(packets))
	}
	if want := protocol.EncodeWidgetClose(300); !bytes.Equal(packets[0], want) {
		t.Fatalf("packet = %x, want widget close %x", packets[0], want)
	}
}

func TestCloseInterruptibleInterfacesBankBranch(t *testing.T) {
	s := NewSession(NewHeadlessChannel(""))
	rec := &recordingInterfaceDeps{interfaceID: 12}
	s.SetInterfaceDeps(rec.deps())
	s.MountSubInterface(mainModalTargetUID, 12, 0)
	rec.calls = nil
	s.CloseInterruptibleInterfaces()
	packets := queuedPackets(t, s)
	// Mount (1) + varp 548 (1) + varbit 12393 (1) + close-sub (1).
	// The bank config sends precede the close: reset runs first.
	if len(packets) != 4 {
		t.Fatalf("queued %d packets, want 4", len(packets))
	}
	if want := protocol.EncodeVarp(548, 0); !bytes.Equal(packets[1], want) {
		t.Fatalf("packet 1 = %x, want varp 548 %x", packets[1], want)
	}
	if want := protocol.EncodeVarbit(12393, 0); !bytes.Equal(packets[2], want) {
		t.Fatalf("packet 2 = %x, want varbit 12393 %x", packets[2], want)
	}
	// EndBankSearch ran with the closed id.
	found := false
	for _, call := range rec.calls {
		if call == "endBankSearch" {
			found = true
		}
	}
	if !found {
		t.Fatalf("deps calls = %v, want endBankSearch", rec.calls)
	}
}

func TestCloseInterruptibleInterfacesChatbox(t *testing.T) {
	s := NewSession(NewHeadlessChannel(""))
	owner := "owner-a"
	rec := &recordingInterfaceDeps{interfaceID: -1}
	s.SetInterfaceDeps(rec.deps())
	s.WithChatboxOwner(owner, func() { s.mountChatboxInterface(231, false) })
	rec.calls = nil
	s.CloseInterruptibleInterfaces()
	if got := s.GetChatboxGroupId(); got != -1 {
		t.Fatalf("GetChatboxGroupId() = %d, want -1", got)
	}
	// TS leaves the chatbox owner stale on this path (only
	// closeCurrentChatbox clears it).
	if got := s.GetChatboxOwner(); got != owner {
		t.Fatalf("GetChatboxOwner() = %v, want the stale owner", got)
	}
	packets := queuedPackets(t, s)
	// Mount (2) + close-sub + hide.
	if len(packets) != 4 {
		t.Fatalf("queued %d packets, want 4", len(packets))
	}
	if want := protocol.EncodeWidgetSetHidden(protocol.ChatboxModalTargetUID, true); !bytes.Equal(packets[3], want) {
		t.Fatalf("packet 3 = %x, want hide %x", packets[3], want)
	}
	// Closed emitted exactly once, for the chatbox group (the
	// interface id -1 emits nothing), in the TS position:
	// closeTrackedInterfaces emits it before endBankSearch
	// runs.
	closedCount := 0
	closedIndex, bankIndex := -1, -1
	for i, call := range rec.calls {
		if call == "closed" {
			closedCount++
			closedIndex = i
		}
		if call == "endBankSearch" {
			bankIndex = i
		}
	}
	if closedCount != 1 || closedIndex < 0 || bankIndex < 0 || closedIndex > bankIndex {
		t.Fatalf("deps calls = %v, want exactly one closed before endBankSearch", rec.calls)
	}
}

func TestCloseInterfaceRoutes(t *testing.T) {
	// Invalid id: no-op.
	s := NewSession(NewHeadlessChannel(""))
	s.CloseInterface(-5)
	if len(queuedPackets(t, s)) != 0 {
		t.Fatal("CloseInterface(-5) queued frames")
	}
	// Main interface id: full removal (fallback close when
	// nothing is tracked).
	s2 := NewSession(NewHeadlessChannel(""))
	rec := &recordingInterfaceDeps{interfaceID: 300}
	s2.SetInterfaceDeps(rec.deps())
	s2.CloseInterface(300)
	packets := queuedPackets(t, s2)
	if len(packets) != 1 || !bytes.Equal(packets[0], protocol.EncodeWidgetClose(300)) {
		t.Fatalf("CloseInterface(main) packets = %x, want [widget close 300]", packets)
	}
	if rec.interfaceID != -1 {
		t.Fatalf("interface id = %d, want -1 after removal", rec.interfaceID)
	}
	// Chatbox id: closeCurrentChatbox (owner cleared).
	s3 := NewSession(NewHeadlessChannel(""))
	owner := "owner-a"
	rec3 := &recordingInterfaceDeps{interfaceID: -1, owner: owner}
	s3.SetInterfaceDeps(rec3.deps())
	s3.WithChatboxOwner(owner, func() { s3.mountChatboxInterface(231, false) })
	s3.CloseInterface(231)
	if s3.GetChatboxGroupId() != -1 || s3.GetChatboxOwner() != nil {
		t.Fatal("CloseInterface(chatbox) did not clear group+owner")
	}
	// Tracked sub-interface by name: close-sub + closed emit.
	s4 := NewSession(NewHeadlessChannel(""))
	rec4 := &recordingInterfaceDeps{interfaceID: -1}
	s4.SetInterfaceDeps(rec4.deps())
	s4.MountSubInterface((161<<16)|50, 400, 1)
	rec4.calls = nil
	s4.CloseInterface(400)
	if s4.IsSubInterfaceOpen(400) {
		t.Fatal("CloseInterface(sub) left the mount tracked")
	}
	packets = queuedPackets(t, s4)
	if want := protocol.EncodeWidgetCloseSub((161 << 16) | 50); !bytes.Equal(packets[len(packets)-1], want) {
		t.Fatalf("last packet = %x, want close-sub %x", packets[len(packets)-1], want)
	}
	if len(rec4.calls) != 1 || rec4.calls[0] != "closed" {
		t.Fatalf("deps calls = %v, want [closed]", rec4.calls)
	}
	// Unknown id: no-op.
	s5 := NewSession(NewHeadlessChannel(""))
	s5.CloseInterface(999)
	if len(queuedPackets(t, s5)) != 0 {
		t.Fatal("CloseInterface(unknown) queued frames")
	}
}

// fakeSessionLayoutLookup is a hand-built definition.LayoutLookup
// for SendConfiguredInterface tests.
type fakeSessionLayoutLookup struct {
	defs []*definition.InterfaceLayoutDefinition
}

func (f *fakeSessionLayoutLookup) Layout(key string) (*definition.InterfaceLayoutDefinition, bool) {
	for _, def := range f.defs {
		if def.Key == key {
			return def, true
		}
	}
	return nil, false
}

func (f *fakeSessionLayoutLookup) LayoutByID(id int) (*definition.InterfaceLayoutDefinition, bool) {
	for _, def := range f.defs {
		if def.ID == id {
			return def, true
		}
	}
	return nil, false
}

func sessionIntPtr(v int) *int { return &v }

func TestSendConfiguredInterface(t *testing.T) {
	lookup := &fakeSessionLayoutLookup{defs: []*definition.InterfaceLayoutDefinition{
		{Key: "bank", ID: 5292, Name: "Bank", Surface: definition.SurfaceMainWithSidebar, SidebarInterfaceID: sessionIntPtr(5063)},
		{Key: "chat", ID: 231, Name: "Chat", Surface: definition.SurfaceChatbox},
		{Key: "tab", ID: 3213, Name: "Tab", Surface: definition.SurfaceSidebar, TabID: sessionIntPtr(3)},
	}}
	s := NewSession(NewHeadlessChannel(""))
	rec := &recordingInterfaceDeps{interfaceID: -1}
	s.SetInterfaceDeps(rec.deps())
	s.SetInterfaceLayouts(lookup)
	def, err := s.SendConfiguredInterface("bank")
	if err != nil || def.Key != "bank" {
		t.Fatalf("SendConfiguredInterface(bank) = %+v, %v", def, err)
	}
	if rec.interfaceID != 5292 {
		t.Fatalf("interface id = %d, want 5292", rec.interfaceID)
	}
	packets := queuedPackets(t, s)
	if len(packets) != 2 {
		t.Fatalf("bank open queued %d packets, want 2 (mount + sidebar)", len(packets))
	}
	// Chatbox surface mounts clamped through the existing
	// SendChatboxInterface (2 frames, no varbit).
	if _, err := s.SendConfiguredInterface("chat"); err != nil {
		t.Fatalf("SendConfiguredInterface(chat): %v", err)
	}
	if got := s.GetChatboxGroupId(); got != 231 {
		t.Fatalf("GetChatboxGroupId() = %d, want 231", got)
	}
	// Sidebar surface by numeric id.
	if _, err := s.SendConfiguredInterface(3213); err != nil {
		t.Fatalf("SendConfiguredInterface(3213): %v", err)
	}
	if !s.IsSubInterfaceOpen(3213) {
		t.Fatal("tab mount not tracked after configured open")
	}
	// Unknown reference: error, nothing sent.
	before := len(queuedPackets(t, s))
	if _, err := s.SendConfiguredInterface("nope"); err == nil || !strings.Contains(err.Error(), "Unknown interface layout: nope") {
		t.Fatalf("SendConfiguredInterface(nope) error = %v", err)
	}
	if len(queuedPackets(t, s)) != before {
		t.Fatal("unknown configured open queued frames")
	}
	// No layouts installed: every reference is unknown.
	s2 := NewSession(NewHeadlessChannel(""))
	if _, err := s2.SendConfiguredInterface("bank"); err == nil {
		t.Fatal("SendConfiguredInterface without layouts: error = nil")
	}
}
