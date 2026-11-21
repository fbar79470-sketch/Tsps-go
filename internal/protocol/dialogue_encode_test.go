package protocol

import (
	"bytes"
	"strings"
	"testing"
)

func TestDialogueChatboxConstants(t *testing.T) {
	if ChatboxModalTargetUID != (162<<16)|568 {
		t.Errorf("ChatboxModalTargetUID = %d, want %d", ChatboxModalTargetUID, (162<<16)|568)
	}
	if ChatboxUnclampVarbit != 10670 {
		t.Errorf("ChatboxUnclampVarbit = %d, want 10670", ChatboxUnclampVarbit)
	}
}

func TestEncodeChatboxInterfaceFrames(t *testing.T) {
	// Clamped mount after a clamped mount: no varbit frame, just the
	// unhide + open-sub (type 0) frames.
	frames := EncodeChatboxInterfaceFrames(231, false, false)
	if len(frames) != 2 {
		t.Fatalf("frames = %d, want 2", len(frames))
	}
	wantHidden := []byte{106, 0x00, 0xA2, 0x02, 0x38, 0}
	if !bytes.Equal(frames[0], wantHidden) {
		t.Fatalf("unhide frame = %x, want %x", frames[0], wantHidden)
	}
	wantOpen := []byte{
		103, 0, 12,
		0x00, 0xA2, 0x02, 0x38, 0x00, 0xE7, 0, 0, 0, 0, 0, 0,
	}
	if !bytes.Equal(frames[1], wantOpen) {
		t.Fatalf("open-sub frame = %x, want %x", frames[1], wantOpen)
	}
	// The frames equal the primitive encoders' output.
	if !bytes.Equal(frames[0], EncodeWidgetSetHidden(ChatboxModalTargetUID, false)) {
		t.Error("unhide frame differs from EncodeWidgetSetHidden")
	}
	if !bytes.Equal(frames[1], EncodeWidgetOpenSub(ChatboxModalTargetUID, 231, 0, WidgetOpenSubOptions{})) {
		t.Error("open-sub frame differs from EncodeWidgetOpenSub")
	}

	// Unclamped mount: the varbit frame (value 1) comes first.
	frames = EncodeChatboxInterfaceFrames(219, true, false)
	if len(frames) != 3 {
		t.Fatalf("unclamped frames = %d, want 3", len(frames))
	}
	wantVarbit := []byte{42, 0x29, 0xAE, 0, 0, 0, 1}
	if !bytes.Equal(frames[0], wantVarbit) {
		t.Fatalf("varbit frame = %x, want %x", frames[0], wantVarbit)
	}

	// Clamped mount after an unclamped one: the varbit returns to 0.
	frames = EncodeChatboxInterfaceFrames(231, false, true)
	if len(frames) != 3 {
		t.Fatalf("re-clamp frames = %d, want 3", len(frames))
	}
	wantVarbit0 := []byte{42, 0x29, 0xAE, 0, 0, 0, 0}
	if !bytes.Equal(frames[0], wantVarbit0) {
		t.Fatalf("varbit frame = %x, want %x", frames[0], wantVarbit0)
	}

	// Unclamped mount after an unclamped one: varbit 1 again.
	frames = EncodeChatboxInterfaceFrames(219, true, true)
	if len(frames) != 3 || !bytes.Equal(frames[0], wantVarbit) {
		t.Fatalf("unclamped-after-unclamped frames = %x, want varbit 1 first", frames)
	}
}

func TestEncodeChatboxCloseFrames(t *testing.T) {
	frames := EncodeChatboxCloseFrames(false)
	if len(frames) != 2 {
		t.Fatalf("frames = %d, want 2", len(frames))
	}
	wantClose := []byte{104, 0x00, 0xA2, 0x02, 0x38}
	if !bytes.Equal(frames[0], wantClose) {
		t.Fatalf("close-sub frame = %x, want %x", frames[0], wantClose)
	}
	wantHide := []byte{106, 0x00, 0xA2, 0x02, 0x38, 1}
	if !bytes.Equal(frames[1], wantHide) {
		t.Fatalf("hide frame = %x, want %x", frames[1], wantHide)
	}

	// Closing an unclamped chatbox sends the varbit back to 0 first.
	frames = EncodeChatboxCloseFrames(true)
	if len(frames) != 3 {
		t.Fatalf("unclamped close frames = %d, want 3", len(frames))
	}
	wantVarbit0 := []byte{42, 0x29, 0xAE, 0, 0, 0, 0}
	if !bytes.Equal(frames[0], wantVarbit0) {
		t.Fatalf("varbit frame = %x, want %x", frames[0], wantVarbit0)
	}
	if !bytes.Equal(frames[1], wantClose) || !bytes.Equal(frames[2], wantHide) {
		t.Fatalf("close frames after varbit = %x %x", frames[1], frames[2])
	}
}

func TestEncodeChatTextFrames(t *testing.T) {
	textUID := (231 << 16) | 6
	continueUID := (231 << 16) | 5
	frames := EncodeChatTextFrames("Hi.", textUID, continueUID)
	if len(frames) != 4 {
		t.Fatalf("frames = %d, want 4", len(frames))
	}
	wantText := []byte{105, 0, 8, 0x00, 0xE7, 0x00, 0x06, 'H', 'i', '.', 0}
	if !bytes.Equal(frames[0], wantText) {
		t.Fatalf("text frame = %x, want %x", frames[0], wantText)
	}
	wantScript := []byte{
		170, 0, 23,
		0x02, 0x58, 4,
		1, 0, 0, 0, 1,
		1, 0, 0, 0, 1,
		1, 0, 0, 0, 16,
		1, 0x00, 0xE7, 0x00, 0x06,
	}
	if !bytes.Equal(frames[1], wantScript) {
		t.Fatalf("script frame = %x, want %x", frames[1], wantScript)
	}
	wantContinue := []byte{105, 0, 27, 0x00, 0xE7, 0x00, 0x05}
	wantContinue = append(wantContinue, []byte("Click here to continue")...)
	wantContinue = append(wantContinue, 0)
	if !bytes.Equal(frames[2], wantContinue) {
		t.Fatalf("continue frame = %x, want %x", frames[2], wantContinue)
	}
	wantFlags := []byte{109, 0x00, 0xE7, 0x00, 0x05, 0xFF, 0xFF, 0xFF, 0xFF, 0, 0, 0, 1}
	if !bytes.Equal(frames[3], wantFlags) {
		t.Fatalf("flags frame = %x, want %x", frames[3], wantFlags)
	}
	// The frames equal the primitive encoders' output.
	if !bytes.Equal(frames[0], EncodeWidgetSetText(textUID, "Hi.", nil)) {
		t.Error("text frame differs from EncodeWidgetSetText")
	}
	if !bytes.Equal(frames[3], EncodeWidgetSetFlagsRange(continueUID, -1, -1, 1)) {
		t.Error("flags frame differs from EncodeWidgetSetFlagsRange")
	}
}

func TestEncodeBoxChatLine(t *testing.T) {
	if got := EncodeBoxChatLine(""); got != nil {
		t.Fatalf("empty text = %x, want nil", got)
	}
	want := []byte{120, 0, 11, 'h', 'e', 'l', 'l', 'o', 0, 115, 0, 0, 0xFF, 0xFF}
	if got := EncodeBoxChatLine("hello"); !bytes.Equal(got, want) {
		t.Fatalf("EncodeBoxChatLine(hello) = %x, want %x", got, want)
	}
	// "<br>" becomes a space before encoding.
	got := EncodeBoxChatLine("a<br>b")
	if !bytes.Contains(got, []byte("a b\x00")) {
		t.Fatalf("break replacement missing in %x", got)
	}
	// Long lines stop at 200 characters: payload 200 + NUL + 1 + 1 + 1 + 2.
	long := EncodeBoxChatLine(strings.Repeat("x", 250))
	if len(long) != 3+206 {
		t.Fatalf("truncated frame length = %d, want %d", len(long), 3+206)
	}
	if long[1] != 0 || long[2] != 206 {
		t.Fatalf("truncated length prefix = %d %d, want 0 206", long[1], long[2])
	}
	// Astral text encodes without panicking (the 200-unit cut is
	// covered by TestTruncateUTF16UnitsProtocol).
	if got := EncodeBoxChatLine("a\U0001F600b"); got == nil {
		t.Fatal("astral text encoded to nil")
	}
}

func TestTruncateUTF16UnitsProtocol(t *testing.T) {
	if got := truncateUTF16Units("abcdef", 3); got != "abc" {
		t.Errorf("ascii truncation = %q", got)
	}
	if got := truncateUTF16Units("a\U0001F600b", 2); got != "a" {
		t.Errorf("astral at limit = %q, want %q", got, "a")
	}
	if got := truncateUTF16Units("a\U0001F600b", 3); got != "a\U0001F600" {
		t.Errorf("astral within limit = %q", got)
	}
}
