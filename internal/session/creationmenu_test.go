package session

import (
	"bytes"
	"strings"
	"testing"

	"tsps-go/internal/game"
	"tsps-go/internal/protocol"
)

// wantWidgetRunScript builds the expected WIDGET_RUN_SCRIPT frame
// from first principles (encodeWidgetRunScript, ClientProtocol
// .ts:2471-2496): opcode 110, 2-byte length, 4-byte script id, arg
// count, one tag byte + value per arg (0 + latin1 + NUL for
// strings, 1 + int32 BE for numbers), then the u16 varp and
// varbit counts (0 each) and no inventories. It deliberately does
// not call protocol.EncodeWidgetRunScript, so a shared encoder
// bug cannot hide here.
func wantWidgetRunScript(scriptID int, args ...any) []byte {
	payload := []byte{byte(scriptID >> 24), byte(scriptID >> 16), byte(scriptID >> 8), byte(scriptID)}
	payload = append(payload, byte(len(args)))
	for _, arg := range args {
		switch v := arg.(type) {
		case int:
			payload = append(payload, 1, byte(v>>24), byte(v>>16), byte(v>>8), byte(v))
		case string:
			payload = append(payload, 0)
			payload = append(payload, v...)
			payload = append(payload, 0)
		default:
			panic("wantWidgetRunScript: unsupported arg type")
		}
	}
	payload = append(payload, 0, 0, 0, 0)
	return append([]byte{110, byte(len(payload) >> 8), byte(len(payload))}, payload...)
}

// wantFlagsRange builds the expected WIDGET_SET_FLAGS_RANGE frame
// (opcode 109, fixed 12 bytes) for one creation-menu item button.
func wantFlagsRange(uid int) []byte {
	return []byte{
		109,
		byte(uid >> 24), byte(uid >> 16), byte(uid >> 8), byte(uid),
		0, 0, // fromSlot
		0, 28, // toSlot
		0, 0, 0, 62, // flags
	}
}

func TestSendCreationMenuFrames(t *testing.T) {
	s := NewSession(NewHeadlessChannel(""))
	menu := game.NewCreationMenu("What would you like to make?", []int{52, 53}, nil, game.CreationMenuOptions{})
	names := map[int]string{52: "Arrow shaft"}
	s.SendCreationMenu(menu, func(id int) string { return names[id] }, 1, true)
	packets := queuedPackets(t, s)
	if len(packets) != 8 {
		t.Fatalf("queued packets = %d, want 8", len(packets))
	}
	ids := []any{52, 53}
	for i := 0; i < 16; i++ {
		ids = append(ids, -1)
	}
	args2046 := []any{0, "What would you like to make?|Arrow shaft|null", 28}
	args2046 = append(args2046, ids...)
	args2046 = append(args2046, 28)
	want := [][]byte{
		wantWidgetRunScript(2379),
		{42, 0x29, 0xAE, 0, 0, 0, 1},                                       // unclamp varbit 10670 = 1
		{106, 0x00, 0xA2, 0x02, 0x38, 0},                                   // modal target shown
		{103, 0, 12, 0x00, 0xA2, 0x02, 0x38, 0x01, 0x0E, 0, 0, 0, 0, 0, 0}, // open-sub group 270
		wantFlagsRange((270 << 16) | 15),
		wantFlagsRange((270 << 16) | 16),
		{40, 0x0A, 0x71, 1}, // varp 2673 = 1 (last item)
		wantWidgetRunScript(2046, args2046...),
	}
	for i := range want {
		if !bytes.Equal(packets[i], want[i]) {
			t.Errorf("packet %d = %x, want %x", i, packets[i], want[i])
		}
	}
	// The 2046 frame is opcode 110 (WIDGET_RUN_SCRIPT), not
	// opcode 170 (RUN_CLIENT_SCRIPT): the plan's "same frame"
	// claim is wrong for this call (see creationmenu.go).
	if packets[7][0] != 110 {
		t.Errorf("2046 frame opcode = %d, want 110", packets[7][0])
	}
	// Session state: group 270 is mounted as a sub-interface,
	// but — as in TS, which never calls sendChatboxInterface
	// here — it is NOT recorded as the chatbox group.
	if !s.IsSubInterfaceOpen(270) {
		t.Error("IsSubInterfaceOpen(270) = false after send")
	}
	if s.IsChatboxInterface(270) {
		t.Error("IsChatboxInterface(270) = true after send; TS leaves chatboxGroupId untouched")
	}
	if got := s.GetVarp(2673); got != 1 {
		t.Errorf("GetVarp(2673) = %d, want 1", got)
	}
}

func TestSendCreationMenuSendsNothingForNilOrEmpty(t *testing.T) {
	s := NewSession(NewHeadlessChannel(""))
	s.SendCreationMenu(nil, nil, 0, false)
	if got := queuedPackets(t, s); len(got) != 0 {
		t.Fatalf("nil menu queued %d packets, want 0", len(got))
	}
	s.SendCreationMenu(game.NewCreationMenu("t", nil, nil, game.CreationMenuOptions{}), nil, 0, false)
	if got := queuedPackets(t, s); len(got) != 0 {
		t.Fatalf("empty menu queued %d packets, want 0", len(got))
	}
}

func TestSendCreationMenuLabelsModesAndClamps(t *testing.T) {
	s := NewSession(NewHeadlessChannel(""))
	// Labels win over item names; mode passes through; a
	// maxAmount of 50 clamps to 28 in the script args, and the
	// last amount clamps to it.
	menu := game.NewCreationMenu("Cook", []int{52}, nil, game.CreationMenuOptions{
		Labels:    []string{"Make 1"},
		MaxAmount: creationMenuIntPtr(50),
		Mode:      3,
	})
	s.SendCreationMenu(menu, func(id int) string { return "Arrow shaft" }, 0, false)
	packets := queuedPackets(t, s)
	// No varp (no last item): 2379, varbit, unhide, open-sub,
	// one flags range, 2046.
	if len(packets) != 6 {
		t.Fatalf("queued packets = %d, want 6", len(packets))
	}
	want2046 := wantWidgetRunScript(2046, 3, "Cook|Make 1", 28, 52, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, 28)
	if !bytes.Equal(packets[5], want2046) {
		t.Errorf("2046 frame = %x, want %x", packets[5], want2046)
	}
	// A nil name resolver names every item "null".
	s2 := NewSession(NewHeadlessChannel(""))
	s2.SendCreationMenu(game.NewCreationMenu("t", []int{99999}, nil, game.CreationMenuOptions{}), nil, 0, false)
	packets2 := queuedPackets(t, s2)
	if !bytes.Contains(packets2[len(packets2)-1], []byte("t|null\x00")) {
		t.Errorf("2046 frame = %x, want the title string to carry \"null\"", packets2[len(packets2)-1])
	}
}

func TestSendCreationMenuSlicesToEighteen(t *testing.T) {
	s := NewSession(NewHeadlessChannel(""))
	items := make([]int, 20)
	for i := range items {
		items[i] = 100 + i
	}
	s.SendCreationMenu(game.NewCreationMenu("t", items, nil, game.CreationMenuOptions{}), func(id int) string { return "x" }, 0, false)
	packets := queuedPackets(t, s)
	// 2379, varbit, unhide, open-sub, 18 flags ranges, 2046.
	if len(packets) != 23 {
		t.Fatalf("queued packets = %d, want 23", len(packets))
	}
	// The 2046 frame carries exactly the first 18 ids, no -1
	// padding.
	ids := make([]any, 0, 18)
	for i := 0; i < 18; i++ {
		ids = append(ids, 100+i)
	}
	args := []any{0, "t|" + strings.Repeat("x|", 17) + "x", 28}
	args = append(args, ids...)
	args = append(args, 28)
	if want := wantWidgetRunScript(2046, args...); !bytes.Equal(packets[22], want) {
		t.Errorf("2046 frame = %x, want %x", packets[22], want)
	}
}

func TestCloseCreationMenuFrames(t *testing.T) {
	s := NewSession(NewHeadlessChannel(""))
	s.SendCreationMenu(game.NewCreationMenu("t", []int{52}, nil, game.CreationMenuOptions{}), func(id int) string { return "x" }, 0, false)
	if !s.IsSubInterfaceOpen(270) {
		t.Fatal("IsSubInterfaceOpen(270) = false after send")
	}
	before := len(queuedPackets(t, s))
	s.CloseCreationMenu()
	packets := queuedPackets(t, s)[before:]
	want := [][]byte{
		{42, 0x29, 0xAE, 0, 0, 0, 0},     // unclamp varbit back to 0
		{104, 0x00, 0xA2, 0x02, 0x38},    // close-sub modal target
		{106, 0x00, 0xA2, 0x02, 0x38, 1}, // modal target hidden
	}
	if len(packets) != len(want) {
		t.Fatalf("close queued %d packets, want %d", len(packets), len(want))
	}
	for i := range want {
		if !bytes.Equal(packets[i], want[i]) {
			t.Errorf("close packet %d = %x, want %x", i, packets[i], want[i])
		}
	}
	if s.IsSubInterfaceOpen(270) {
		t.Error("IsSubInterfaceOpen(270) = true after close")
	}
}

func TestSendInterfaceScriptWithArgsIsWidgetRunScript(t *testing.T) {
	s := NewSession(NewHeadlessChannel(""))
	s.SendInterfaceScriptWithArgs(2046, game.ClientScriptInt(0), game.ClientScriptString("x"), game.ClientScriptInt(28))
	packets := queuedPackets(t, s)
	if len(packets) != 1 {
		t.Fatalf("queued packets = %d, want 1", len(packets))
	}
	want := wantWidgetRunScript(2046, 0, "x", 28)
	if !bytes.Equal(packets[0], want) {
		t.Fatalf("frame = %x, want %x", packets[0], want)
	}
	// The same call through SendClientScript would be opcode
	// 170 with a 2-byte script id — a different frame.
	if other := protocol.EncodeRunClientScript(2046, []protocol.ScriptArg{
		protocol.ScriptArgInt(0), protocol.ScriptArgString("x"), protocol.ScriptArgInt(28),
	}); bytes.Equal(packets[0], other) {
		t.Fatal("frame equals the RUN_CLIENT_SCRIPT encoding; 2046 must go out as WIDGET_RUN_SCRIPT")
	}
}

// recordingMenuDeps records the SetCreationMenu calls a session
// makes, in order (nil entries are clears).
type recordingMenuDeps struct {
	installed []*game.CreationMenu
}

func (r *recordingMenuDeps) deps() InterfaceDeps {
	return InterfaceDeps{SetCreationMenu: func(menu *game.CreationMenu) {
		r.installed = append(r.installed, menu)
	}}
}

func TestCreationMenuDepsInstallAndClear(t *testing.T) {
	s := NewSession(NewHeadlessChannel(""))
	rec := &recordingMenuDeps{}
	s.SetInterfaceDeps(rec.deps())
	menu := game.NewCreationMenu("t", []int{52}, nil, game.CreationMenuOptions{})
	s.SendCreationMenu(menu, nil, 0, false)
	if len(rec.installed) != 1 || rec.installed[0] != menu {
		t.Fatalf("installed = %v, want [menu]", rec.installed)
	}
	s.CloseCreationMenu()
	if len(rec.installed) != 2 || rec.installed[1] != nil {
		t.Fatalf("installed = %v, want [menu nil]", rec.installed)
	}
	// Interface removal clears the menu too
	// (resetInterfaceState, PacketSender.ts:894).
	s.SendCreationMenu(menu, nil, 0, false)
	s.SendInterfaceRemoval()
	if len(rec.installed) != 4 || rec.installed[3] != nil {
		t.Fatalf("installed = %v, want [menu nil menu nil]", rec.installed)
	}
	// An unwired session still sends the frames.
	s2 := NewSession(NewHeadlessChannel(""))
	s2.SendCreationMenu(menu, nil, 0, false)
	if got := queuedPackets(t, s2); len(got) != 6 {
		t.Fatalf("unwired send queued %d packets, want 6", len(got))
	}
}

func creationMenuIntPtr(v int) *int { return &v }
