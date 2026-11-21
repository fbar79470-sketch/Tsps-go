package game

import (
	"testing"
)

func TestForceChat(t *testing.T) {
	p := NewPlayer("chatter", Location{X: 3089, Y: 3524, Z: 0})
	if got := p.GetForcedChat(); got != nil {
		t.Fatalf("initial GetForcedChat = %q, want nil", *got)
	}
	if p.Flags.Has(FlagForcedChat) {
		t.Fatal("initial FlagForcedChat set, want unset")
	}

	p.ForceChat("hello")
	got := p.GetForcedChat()
	if got == nil || *got != "hello" {
		t.Fatalf("GetForcedChat = %v, want \"hello\"", got)
	}
	if !p.Flags.Has(FlagForcedChat) {
		t.Fatal("FlagForcedChat unset after ForceChat")
	}

	// A second call in the same tick replaces the text.
	p.ForceChat("again")
	if got := p.GetForcedChat(); got == nil || *got != "again" {
		t.Fatalf("GetForcedChat = %v, want \"again\"", got)
	}

	// ResetUpdating clears both the text and the flag: overhead
	// chat lasts exactly one sync frame (Mobile.ts:264-278).
	p.ResetUpdating()
	if got := p.GetForcedChat(); got != nil {
		t.Fatalf("GetForcedChat after reset = %q, want nil", *got)
	}
	if p.Flags.Has(FlagForcedChat) {
		t.Fatal("FlagForcedChat set after ResetUpdating")
	}
}

func TestForceChatNPC(t *testing.T) {
	// ForceChat is Mobile-level: NPCs share the field, the flag,
	// and the reset (their producers are NOT STARTED, but the
	// mechanism must work for the NPC sync block).
	n := NewNPC(1, Location{X: 3089, Y: 3524, Z: 0})
	n.ForceChat("Baa!")
	if got := n.GetForcedChat(); got == nil || *got != "Baa!" {
		t.Fatalf("GetForcedChat = %v, want \"Baa!\"", got)
	}
	if !n.Flags.Has(FlagForcedChat) {
		t.Fatal("FlagForcedChat unset after ForceChat")
	}
	n.ResetUpdating()
	if got := n.GetForcedChat(); got != nil {
		t.Fatalf("GetForcedChat after reset = %q, want nil", *got)
	}
	if n.Flags.Has(FlagForcedChat) {
		t.Fatal("FlagForcedChat set after ResetUpdating")
	}
}

func TestChatIconsEmpty(t *testing.T) {
	// GAP, pinned: no backing field exists yet, so the accessor
	// always reads empty (see ChatIcons in chat.go).
	p := NewPlayer("chatter", Location{X: 3089, Y: 3524, Z: 0})
	if icons := p.ChatIcons(); len(icons) != 0 {
		t.Fatalf("ChatIcons() = %v, want empty", icons)
	}
}

func TestCanReceivePublicChatFrom(t *testing.T) {
	p := NewPlayer("listener", Location{X: 3089, Y: 3524, Z: 0})
	other := NewPlayer("speaker", Location{X: 3090, Y: 3524, Z: 0})

	// The mode-0/empty-list degeneration (PlayerRelations.ts:90-97):
	// standard chat from any sender is received, autochat is not,
	// and a nil sender never delivers.
	if !p.CanReceivePublicChatFrom(other, false) {
		t.Error("standard chat rejected, want received")
	}
	if p.CanReceivePublicChatFrom(other, true) {
		t.Error("autochat received in mode 0, want rejected")
	}
	if p.CanReceivePublicChatFrom(nil, false) {
		t.Error("nil sender received, want rejected")
	}
	// The relations method itself does not exempt the speaker;
	// the broadcast loop applies the self exemption (the caller
	// skips the gate for recipient == speaker).
	if !p.CanReceivePublicChatFrom(p, false) {
		t.Error("own standard chat rejected by the gate, want received")
	}
}

// TestSetChatIcons pins the setChatIcons filter (Player.ts:1019-1024):
// ids outside 0..255 are dropped, at most 255 are kept, ChatIcons
// returns a copy, and a second set replaces the first.
func TestSetChatIcons(t *testing.T) {
	p := NewPlayer("chatter", Location{X: 3089, Y: 3524, Z: 0})
	p.SetChatIcons([]int{0, 1, 255, -1, 256, 300})
	if got := p.ChatIcons(); len(got) != 3 || got[0] != 0 || got[1] != 1 || got[2] != 255 {
		t.Fatalf("ChatIcons() = %v, want [0 1 255]", got)
	}
	// Mutating the returned slice must not mutate the player.
	got := p.ChatIcons()
	got[0] = 99
	if icons := p.ChatIcons(); icons[0] != 0 {
		t.Fatalf("ChatIcons() after mutation = %v, want first element 0", icons)
	}
	// The 255 cap: 300 valid ids keep the first 255.
	many := make([]int, 300)
	for i := range many {
		many[i] = i % 256
	}
	p.SetChatIcons(many)
	if icons := p.ChatIcons(); len(icons) != 255 {
		t.Fatalf("ChatIcons() length = %d, want 255", len(icons))
	}
	// A second set replaces.
	p.SetChatIcons(nil)
	if icons := p.ChatIcons(); len(icons) != 0 {
		t.Fatalf("ChatIcons() after clearing = %v, want empty", icons)
	}
}
