package protocol

import (
	"bytes"
	"testing"
)

func TestEncodeHandshake(t *testing.T) {
	appearance := &PlayerAppearance{
		Gender: 1,
		Colors: []int{10, 20},
		Kits:   []int{30},
		Equip:  []int{40, 50},
	}
	// Icons 300 and -1 are filtered out; the empty string and the
	// admin/members bytes close the packet.
	want := []byte{
		2, 29,
		0, 0, 0, 1, // index
		'T', 'y', 'l', 'e', 'r', 0, // username
		1, 1, // appearance present, gender
		2, 10, 20, // colors as bytes
		1, 0, 30, // kits as shorts
		2, 0, 40, 0, 50, // equip as shorts
		2, 1, 2, // icons
		0, // empty string
		1, // admin
		1, // members world
	}
	got := EncodeHandshake(1, "Tyler", true, appearance, []int{1, 300, -1, 2}, true)
	if !bytes.Equal(got, want) {
		t.Fatalf("EncodeHandshake = %x, want %x", got, want)
	}
	// No appearance: the absent byte and nothing else for it.
	wantAbsent := []byte{
		2, 15,
		0, 0, 0, 1,
		'T', 'y', 'l', 'e', 'r', 0,
		0, // no appearance
		0, // no icons
		0, // empty string
		0, // not admin
		0, // not members
	}
	if got := EncodeHandshake(1, "Tyler", false, nil, nil, false); !bytes.Equal(got, wantAbsent) {
		t.Fatalf("EncodeHandshake(absent) = %x, want %x", got, wantAbsent)
	}
	// Icons are counted as filtered: 200 valid icons fit and the count
	// byte reads 200.
	icons := make([]int, 200)
	for i := range icons {
		icons[i] = 1
	}
	gotIcons := EncodeHandshake(1, "A", false, nil, icons, false)
	// Layout: opcode+len, id(4), "A\0"(2), absent(1), icon count(1).
	if gotIcons[9] != 200 {
		t.Fatalf("icon count = %d, want 200", gotIcons[9])
	}
	// More than 255 icons still throws: TS caps the list at 255, but
	// 255 icons plus the fixed overhead exceeds the 1-byte packet
	// length, so the packet() length check throws, as mustEncode does
	// here.
	func() {
		defer func() {
			if recover() == nil {
				t.Fatalf("EncodeHandshake(300 icons) did not panic")
			}
		}()
		many := make([]int, 300)
		for i := range many {
			many[i] = 1
		}
		EncodeHandshake(1, "A", false, nil, many, false)
	}()
}

func TestEncodeDefaultAnimations(t *testing.T) {
	want := []byte{
		22,
		0x03, 0x28, 0x03, 0x33, 0x03, 0x34, 0x03, 0x35, 0x03, 0x36,
		0x03, 0x38, 0x03, 0x38, 0x03, 0x38, 0x03, 0x38, 0x03, 0x37, 0x03, 0x37,
	}
	if got := EncodeDefaultAnimations(); !bytes.Equal(got, want) {
		t.Fatalf("EncodeDefaultAnimations = %x, want %x", got, want)
	}
}

func TestEncodeLogoutResponse(t *testing.T) {
	if got, want := EncodeLogoutResponse("", true), []byte{4, 2, 1, 0}; !bytes.Equal(got, want) {
		t.Fatalf("EncodeLogoutResponse(ok) = %x, want %x", got, want)
	}
	if got, want := EncodeLogoutResponse("bye", false), []byte{4, 5, 0, 'b', 'y', 'e', 0}; !bytes.Equal(got, want) {
		t.Fatalf("EncodeLogoutResponse(bye) = %x, want %x", got, want)
	}
}

func TestEncodeGameframeBootstrap(t *testing.T) {
	packets := EncodeGameframeBootstrap("Tyler", 161, false, "REGULAR", true, 42)
	// 1 camera + 1 root + 20 mounts + 2 content mounts + 12 flags + 1
	// login script.
	if len(packets) != 37 {
		t.Fatalf("bootstrap packet count = %d, want 37", len(packets))
	}
	if got, want := packets[0], []byte{170, 0, 3, 2, 0x72, 0}; !bytes.Equal(got, want) {
		t.Fatalf("bootstrap[0] (camera) = %x, want %x", got, want)
	}
	if got, want := packets[1], []byte{102, 0, 0xA1}; !bytes.Equal(got, want) {
		t.Fatalf("bootstrap[1] (root) = %x, want %x", got, want)
	}
	// First mount: 161:96 -> group 162, no post-script.
	if got, want := packets[2], []byte{103, 0, 12, 0, 0xA1, 0, 0x60, 0, 0xA2, 1, 0, 0, 0, 0, 0}; !bytes.Equal(got, want) {
		t.Fatalf("bootstrap[2] (mount) = %x, want %x", got, want)
	}
	// Orbs mount (index 4, after pm mounts): 161:33 -> group 160.
	if got, want := packets[4], []byte{103, 0, 12, 0, 0xA1, 0, 0x21, 0, 0xA0, 1, 0, 0, 0, 0, 0}; !bytes.Equal(got, want) {
		t.Fatalf("bootstrap[4] (orbs) = %x, want %x", got, want)
	}
	// Mount with a post-script: 161:6 -> group 651, script 5929.
	if got, want := packets[5], []byte{103, 0, 17, 0, 0xA1, 0, 6, 2, 0x8B, 1, 0, 0, 0, 0, 1, 0, 0, 0x17, 0x29, 0}; !bytes.Equal(got, want) {
		t.Fatalf("bootstrap[5] (post-script mount) = %x, want %x", got, want)
	}
	// Last packet: script 876 with [mapClock, 0, name, label].
	wantLast := []byte{
		110, 0, 35,
		0, 0, 3, 0x6C,
		4,
		1, 0, 0, 0, 42,
		1, 0, 0, 0, 0,
		0, 'T', 'y', 'l', 'e', 'r', 0,
		0, 'R', 'E', 'G', 'U', 'L', 'A', 'R', 0,
		0, 0, 0, 0,
	}
	if got := packets[len(packets)-1]; !bytes.Equal(got, wantLast) {
		t.Fatalf("bootstrap[last] (login script) = %x, want %x", got, wantLast)
	}
	// The main-inventory flags packet is among the flag packets.
	wantFlags := EncodeWidgetSetFlagsRange(MainInventoryWidgetUID, 0, 27, MainInventorySlotFlags)
	found := false
	for _, p := range packets {
		if bytes.Equal(p, wantFlags) {
			found = true
		}
	}
	if !found {
		t.Fatalf("bootstrap lacks main-inventory flags packet %x", wantFlags)
	}
	// XP drops add one mount; hidden orbs remove one.
	if n := len(EncodeGameframeBootstrap("Tyler", 161, true, "REGULAR", true, 42)); n != 38 {
		t.Fatalf("bootstrap with xp drops = %d packets, want 38", n)
	}
	if n := len(EncodeGameframeBootstrap("Tyler", 161, false, "REGULAR", false, 42)); n != 36 {
		t.Fatalf("bootstrap without orbs = %d packets, want 36", n)
	}
	// The mobile root mounts the mobile orbs group.
	mobile := EncodeGameframeBootstrap("Tyler", 601, false, "REGULAR", true, 42)
	if got, want := mobile[4], []byte{103, 0, 12, 0, 0xA1, 0, 0x21, 3, 0x81, 1, 0, 0, 0, 0, 0}; !bytes.Equal(got, want) {
		t.Fatalf("mobile bootstrap[4] (orbs) = %x, want %x", got, want)
	}
}

func TestEncodeGameframeFlags(t *testing.T) {
	flags := EncodeGameframeFlags(161)
	if len(flags) != 12 {
		t.Fatalf("gameframe flags = %d packets, want 12", len(flags))
	}
	// The quest tab icon flags address the active root's child (161:61).
	wantQuest := EncodeWidgetSetFlags((161<<16)|61, (1<<1)|(1<<2)|(1<<3)|(1<<4))
	found := false
	for _, p := range flags {
		if bytes.Equal(p, wantQuest) {
			found = true
		}
	}
	if !found {
		t.Fatalf("flags lack quest tab icon packet %x", wantQuest)
	}
	// Root 548 uses child 66 for the quest tab icon.
	flags548 := EncodeGameframeFlags(548)
	wantQuest548 := EncodeWidgetSetFlags((548<<16)|66, (1<<1)|(1<<2)|(1<<3)|(1<<4))
	found = false
	for _, p := range flags548 {
		if bytes.Equal(p, wantQuest548) {
			found = true
		}
	}
	if !found {
		t.Fatalf("flags(548) lack quest tab icon packet %x", wantQuest548)
	}
}

func TestEncodeGameframeTabSetup(t *testing.T) {
	// Journal: the character-summary mount plus the four account-summary
	// flag packets (uid group 712).
	if got := EncodeGameframeTabSetup(629, 161); len(got) != 5 {
		t.Fatalf("tab setup(629) = %d packets, want 5", len(got))
	} else if want := encodeOpenSub(629, 43, 712); !bytes.Equal(got[0], want) {
		t.Fatalf("tab setup(629)[0] = %x, want %x", got[0], want)
	}
	// Side channels: just the chat-channel content mount.
	if got := EncodeGameframeTabSetup(707, 161); len(got) != 1 {
		t.Fatalf("tab setup(707) = %d packets, want 1", len(got))
	} else if want := encodeOpenSub(707, 7, 7); !bytes.Equal(got[0], want) {
		t.Fatalf("tab setup(707)[0] = %x, want %x", got[0], want)
	}
	// Magic tab: just its filter-menu flags.
	if got := EncodeGameframeTabSetup(218, 161); len(got) != 1 {
		t.Fatalf("tab setup(218) = %d packets, want 1", len(got))
	}
	// Prayer tab: just its filter-menu flags.
	if got := EncodeGameframeTabSetup(541, 161); len(got) != 1 {
		t.Fatalf("tab setup(541) = %d packets, want 1", len(got))
	}
}

func TestOrbsGroupFor(t *testing.T) {
	if got := OrbsGroupFor(161); got != 160 {
		t.Fatalf("OrbsGroupFor(161) = %d, want 160", got)
	}
	if got := OrbsGroupFor(601); got != 897 {
		t.Fatalf("OrbsGroupFor(601) = %d, want 897", got)
	}
}
