package protocol

import (
	"bytes"
	"testing"
)

// Forced-chat (overhead chat) byte vectors: the FORCED_CHAT update
// block as writePlayerUpdateBlock/writeNpcUpdateBlock render it
// (ClientProtocol.ts lines 3266-3306, 3308-3357). The mask bit is
// PLAYER_MASK.FORCED_CHAT 0x01 / NPC_MASK.FORCED_CHAT 0x40, and
// the text is a latin1 string plus NUL (writeText). Field order
// differs between the two blocks: the player block writes the
// text FIRST (before face direction, interaction, animation,
// hits, appearance); the NPC block writes hits first, then the
// text. These vectors pin both orders plus the mask-only cases.
// The chat listener sets the text (game.Mobile.ForceChat); the
// flush source populates ForcedChat on the views (coordinator
// integration) — the encoder side is what is tested here.

func forcedChatStringPtr(s string) *string { return &s }

func TestPlayerUpdateBlockForcedChatOnly(t *testing.T) {
	view := &PlayerView{
		ActorUpdateView: ActorUpdateView{ForcedChat: forcedChatStringPtr("hi")},
	}
	// Mask 0x01, then "hi" + NUL. No other block is written:
	// writeMovementType, writeAppearance, and writeInteraction
	// are all false.
	want := []byte{0x01, 'h', 'i', 0}
	if got := writePlayerUpdateBlock(view, false, 0, false, false); !bytes.Equal(got, want) {
		t.Fatalf("block = %v, want %v", got, want)
	}
}

func TestPlayerUpdateBlockForcedChatOrder(t *testing.T) {
	// Text before face direction (shortLE) and before the
	// appearance block (byteC length + bytes): the TS write
	// order in writePlayerUpdateBlock.
	view := &PlayerView{
		ActorUpdateView: ActorUpdateView{ForcedChat: forcedChatStringPtr("hi")},
		Appearance:      []byte{'A', 'B'},
		FaceDirection:   intPtr(5),
	}
	// Mask 0x01 | 0x02 | 0x04 = 0x07; text; face direction 5 as
	// shortLE (0x05, 0x00); appearance length 2 as byteC
	// ((-2) & 0xff = 254), then the two appearance bytes.
	want := []byte{0x07, 'h', 'i', 0, 0x05, 0x00, 254, 'A', 'B'}
	if got := writePlayerUpdateBlock(view, false, 0, true, false); !bytes.Equal(got, want) {
		t.Fatalf("block = %v, want %v", got, want)
	}
}

func TestPlayerUpdateBlockNoForcedChat(t *testing.T) {
	// A nil ForcedChat sets no mask bit and writes no text: an
	// otherwise empty block is a single zero mask byte.
	view := &PlayerView{}
	want := []byte{0x00}
	if got := writePlayerUpdateBlock(view, false, 0, false, false); !bytes.Equal(got, want) {
		t.Fatalf("block = %v, want %v", got, want)
	}
	// An empty (non-nil) string still sets the bit and writes
	// just the terminator: presence, not content, drives the
	// mask (TS: view.forcedChat !== undefined).
	view.ForcedChat = forcedChatStringPtr("")
	want = []byte{0x01, 0}
	if got := writePlayerUpdateBlock(view, false, 0, false, false); !bytes.Equal(got, want) {
		t.Fatalf("block = %v, want %v", got, want)
	}
}

func TestPlayerUpdateBlockForcedChatLatin1(t *testing.T) {
	// writeText encodes latin1: é (U+00E9) is one byte 0xE9.
	view := &PlayerView{
		ActorUpdateView: ActorUpdateView{ForcedChat: forcedChatStringPtr("café")},
	}
	want := []byte{0x01, 'c', 'a', 'f', 0xE9, 0}
	if got := writePlayerUpdateBlock(view, false, 0, false, false); !bytes.Equal(got, want) {
		t.Fatalf("block = %v, want %v", got, want)
	}
}

func TestNpcUpdateBlockForcedChatOnly(t *testing.T) {
	view := &NpcView{
		ActorUpdateView: ActorUpdateView{ForcedChat: forcedChatStringPtr("Baa!")},
	}
	// Mask 0x40, then the text + NUL.
	want := []byte{0x40, 'B', 'a', 'a', '!', 0}
	if got := writeNpcUpdateBlock(view, false, false); !bytes.Equal(got, want) {
		t.Fatalf("block = %v, want %v", got, want)
	}
}

func TestNpcUpdateBlockForcedChatAfterHits(t *testing.T) {
	// The NPC block writes hits BEFORE the text (the reverse of
	// no other ordering: interaction, hits, text, tint,
	// graphics, animation). An empty non-nil Hits slice still
	// sets the HIT bit (a JS array is truthy even when empty).
	view := &NpcView{
		ActorUpdateView: ActorUpdateView{
			ForcedChat: forcedChatStringPtr("hi"),
			Hits:       []HitsplatView{},
		},
	}
	// Mask 0x20 | 0x40 = 0x60; hits: byteS(0) = 128, no hits,
	// byteA(0 bars) = 128; then the text + NUL.
	want := []byte{0x60, 128, 128, 'h', 'i', 0}
	if got := writeNpcUpdateBlock(view, false, false); !bytes.Equal(got, want) {
		t.Fatalf("block = %v, want %v", got, want)
	}
}

func TestForcedChatMasks(t *testing.T) {
	player := &PlayerView{ActorUpdateView: ActorUpdateView{ForcedChat: forcedChatStringPtr("x")}}
	if got := playerUpdateMask(player, false, false, false); got != playerMaskForcedChat {
		t.Fatalf("player mask = %#x, want %#x", got, playerMaskForcedChat)
	}
	npc := &NpcView{ActorUpdateView: ActorUpdateView{ForcedChat: forcedChatStringPtr("x")}}
	if got := npcUpdateMask(npc, false, false); got != npcMaskForcedChat {
		t.Fatalf("npc mask = %#x, want %#x", got, npcMaskForcedChat)
	}
}
