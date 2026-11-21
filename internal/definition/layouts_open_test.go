package definition

import (
	"strings"
	"testing"
)

// recordingLayoutSender records the InterfacePacketSender calls
// an open makes.
type recordingLayoutSender struct {
	calls []string
}

func (s *recordingLayoutSender) SendInterface(interfaceID int) {
	s.calls = append(s.calls, "interface")
}

func (s *recordingLayoutSender) SendChatboxInterface(interfaceID int, unclamp bool) {
	if unclamp {
		s.calls = append(s.calls, "chatbox-unclamped")
	} else {
		s.calls = append(s.calls, "chatbox")
	}
}

func (s *recordingLayoutSender) SendWalkableInterface(interfaceID int) {
	s.calls = append(s.calls, "walkable")
}

func (s *recordingLayoutSender) SendTabInterface(tabID, interfaceID int) {
	s.calls = append(s.calls, "tab")
}

func (s *recordingLayoutSender) SendSidebarInterface(interfaceID int) {
	s.calls = append(s.calls, "sidebar")
}

func (s *recordingLayoutSender) SendInterfaceSet(interfaceID, sidebarInterfaceID int) {
	s.calls = append(s.calls, "set")
}

// fakeLayoutLookup is a hand-built LayoutLookup over a fixed
// list of definitions.
type fakeLayoutLookup struct {
	defs []*InterfaceLayoutDefinition
}

func (f *fakeLayoutLookup) Layout(key string) (*InterfaceLayoutDefinition, bool) {
	for _, def := range f.defs {
		if def.Key == key {
			return def, true
		}
	}
	return nil, false
}

func (f *fakeLayoutLookup) LayoutByID(id int) (*InterfaceLayoutDefinition, bool) {
	for _, def := range f.defs {
		if def.ID == id {
			return def, true
		}
	}
	return nil, false
}

func intPtr(v int) *int { return &v }

func TestOpenLayoutSurfaceDispatch(t *testing.T) {
	lookup := &fakeLayoutLookup{defs: []*InterfaceLayoutDefinition{
		{Key: "main-one", ID: 100, Name: "Main", Surface: SurfaceMain},
		{Key: "full-one", ID: 101, Name: "Full", Surface: SurfaceFullscreen},
		{Key: "chat-one", ID: 102, Name: "Chat", Surface: SurfaceChatbox},
		{Key: "walk-one", ID: 103, Name: "Walk", Surface: SurfaceWalkableOverlay},
		{Key: "side-one", ID: 104, Name: "Side", Surface: SurfaceSidebar, TabID: intPtr(3)},
		{Key: "overlay-one", ID: 105, Name: "Overlay", Surface: SurfaceSidebarOverlay},
		{Key: "both-one", ID: 106, Name: "Both", Surface: SurfaceMainWithSidebar, SidebarInterfaceID: intPtr(107)},
	}}
	cases := []struct {
		ref  any
		want string
	}{
		{"main-one", "interface"},
		{"full-one", "interface"},
		{"chat-one", "chatbox"},
		{"walk-one", "walkable"},
		{"side-one", "tab"},
		{"overlay-one", "sidebar"},
		{"both-one", "set"},
		{106, "set"}, // numeric reference resolves by id
	}
	for _, tc := range cases {
		sender := &recordingLayoutSender{}
		def, err := OpenLayout(lookup, sender, tc.ref)
		if err != nil {
			t.Errorf("OpenLayout(%v): unexpected error: %v", tc.ref, err)
			continue
		}
		if def == nil {
			t.Errorf("OpenLayout(%v): nil definition", tc.ref)
			continue
		}
		if len(sender.calls) != 1 || sender.calls[0] != tc.want {
			t.Errorf("OpenLayout(%v): calls = %v, want [%s]", tc.ref, sender.calls, tc.want)
		}
	}
}

func TestOpenLayoutUnknownReference(t *testing.T) {
	lookup := &fakeLayoutLookup{defs: []*InterfaceLayoutDefinition{
		{Key: "known", ID: 100, Name: "Known", Surface: SurfaceMain},
	}}
	sender := &recordingLayoutSender{}
	if _, err := OpenLayout(lookup, sender, "nope"); err == nil || !strings.Contains(err.Error(), "Unknown interface layout: nope") {
		t.Fatalf("OpenLayout(unknown key) error = %v, want unknown-layout error", err)
	}
	if _, err := OpenLayout(lookup, sender, 999); err == nil || !strings.Contains(err.Error(), "Unknown interface layout: 999") {
		t.Fatalf("OpenLayout(unknown id) error = %v, want unknown-layout error", err)
	}
	if _, err := OpenLayout(lookup, sender, 1.5); err == nil {
		t.Fatal("OpenLayout(float reference) error = nil, want error")
	}
	if _, err := OpenLayout(nil, sender, "known"); err == nil {
		t.Fatal("OpenLayout(nil lookup) error = nil, want error")
	}
	if len(sender.calls) != 0 {
		t.Fatalf("failed opens sent %v, want nothing", sender.calls)
	}
}

func TestOpenLayoutValidationErrors(t *testing.T) {
	lookup := &fakeLayoutLookup{defs: []*InterfaceLayoutDefinition{
		{Key: "no-tab", ID: 100, Name: "NoTab", Surface: SurfaceSidebar},
		{Key: "neg-tab", ID: 101, Name: "NegTab", Surface: SurfaceSidebar, TabID: intPtr(-1)},
		{Key: "no-side", ID: 102, Name: "NoSide", Surface: SurfaceMainWithSidebar},
		{Key: "neg-side", ID: 103, Name: "NegSide", Surface: SurfaceMainWithSidebar, SidebarInterfaceID: intPtr(-2)},
		{Key: "bogus", ID: 104, Name: "Bogus", Surface: InterfaceSurface("sideways")},
	}}
	cases := []struct {
		ref     string
		wantErr string
	}{
		{"no-tab", "Sidebar interface no-tab requires a tabId"},
		{"neg-tab", "Sidebar interface neg-tab requires a tabId"},
		{"no-side", "Interface no-side requires a sidebarInterfaceId"},
		{"neg-side", "Interface neg-side requires a sidebarInterfaceId"},
		{"bogus", "Unsupported interface surface: sideways"},
	}
	for _, tc := range cases {
		sender := &recordingLayoutSender{}
		if _, err := OpenLayout(lookup, sender, tc.ref); err == nil || err.Error() != tc.wantErr {
			t.Errorf("OpenLayout(%s) error = %v, want %q", tc.ref, err, tc.wantErr)
		}
		if len(sender.calls) != 0 {
			t.Errorf("OpenLayout(%s) sent %v, want nothing", tc.ref, sender.calls)
		}
	}
}

func TestRequireLayout(t *testing.T) {
	lookup := &fakeLayoutLookup{defs: []*InterfaceLayoutDefinition{
		{Key: "known", ID: 100, Name: "Known", Surface: SurfaceMain},
	}}
	if def, err := RequireLayout(lookup, "known"); err != nil || def.Key != "known" {
		t.Fatalf("RequireLayout(known) = %+v, %v", def, err)
	}
	if def, err := RequireLayout(lookup, 100); err != nil || def.Key != "known" {
		t.Fatalf("RequireLayout(100) = %+v, %v", def, err)
	}
	if _, err := RequireLayout(lookup, "nope"); err == nil {
		t.Fatal("RequireLayout(nope) error = nil")
	}
}
