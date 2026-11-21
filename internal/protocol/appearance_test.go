package protocol

import (
	"bytes"
	"testing"
)

func TestEncodePlayerAppearanceKits(t *testing.T) {
	// No equipment: kits show through. Wire slots 4,6,7,8,9,10,11 map
	// to kits 2,3,5,0,4,6,1 (+256); slots 0,1,2,3,5 write a zero byte.
	// The 12-slot row is written twice.
	appearance := PlayerAppearance{
		Gender: 0,
		Colors: []int{1, 2, 3, 4, 5},
		Kits:   []int{0, 1, 2, 3, 4, 5, 6},
		Equip:  []int{},
	}
	row := []byte{
		0, 0, 0, 0, 0x01, 0x02, 0, 0x01, 0x03, 0x01, 0x05, 0x01, 0x00,
		0x01, 0x04, 0x01, 0x06, 0x01, 0x01,
	}
	want := []byte{0, 0xFF, 0xFF}
	want = append(want, row...)
	want = append(want, row...)
	want = append(want,
		1, 2, 3, 4, 5, // colors
		0x03, 0x28, 0x03, 0x33, 0x03, 0x34, 0x03, 0x35, 0x03, 0x36, 0x03, 0x38, 0x03, 0x38, // animations
		'T', 'y', 'l', 'e', 'r', 0, // name
		3,          // combat level
		0x04, 0xF3, // skill level 1267
		0,    // hidden
		0, 0, // no colour/texture overrides
		0, 0, 0, // three empty strings
		0,          // pronouns fall back to gender
		0, 0, 0, 0, // equipQty[13]
		0xFF, 0xFF, 0xFF, 0xFF, // equip[13]
		0xFF, 0xFF, 0xFF, 0xFF, // equip[12]
		0, 0, 0, 0, // equipQty[3]
	)
	got := EncodePlayerAppearance(appearance, "Tyler", 3, 1267, []int{808, 819, 820, 821, 822, 824, 824})
	if !bytes.Equal(got, want) {
		t.Fatalf("EncodePlayerAppearance(kits) =\n%x\nwant\n%x", got, want)
	}
	if len(got) != 92 {
		t.Fatalf("appearance length = %d, want 92", len(got))
	}
}

func TestEncodePlayerAppearanceEquipment(t *testing.T) {
	// Equipment overrides kits: equip[0]=100 shows as 100+2048 on wire
	// slot 0; every other slot is empty (kits absent). Head icons,
	// pronouns, and hidden are all set; animations default to 0xffff.
	equip := make([]int, 14)
	for i := range equip {
		equip[i] = -1
	}
	equip[0] = 100
	equip[12] = 3000
	equip[13] = 2000
	equipQty := make([]int, 14)
	equipQty[3] = 7
	equipQty[13] = 5
	appearance := PlayerAppearance{
		Gender:    1,
		Hidden:    true,
		Pronouns:  intPtr(1),
		Equip:     equip,
		EquipQty:  equipQty,
		HeadIcons: &HeadIcons{Skull: 1, Prayer: 2},
	}
	row := []byte{0x08, 0x64, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}
	want := []byte{1, 1, 2}
	want = append(want, row...)
	want = append(want, row...)
	want = append(want, 0, 0, 0, 0, 0) // colors default to 0
	for i := 0; i < 7; i++ {
		want = append(want, 0xFF, 0xFF) // animations default to 0xffff
	}
	want = append(want,
		'A', 0, // name
		126,        // combat level
		0x07, 0xD0, // skill level 2000
		1,    // hidden
		0, 0, // no overrides
		0, 0, 0, // three empty strings
		1,          // pronouns
		0, 0, 0, 5, // equipQty[13]
		0, 0, 0x07, 0xD0, // equip[13] = 2000
		0, 0, 0x0B, 0xB8, // equip[12] = 3000
		0, 0, 0, 7, // equipQty[3]
	)
	got := EncodePlayerAppearance(appearance, "A", 126, 2000, nil)
	if !bytes.Equal(got, want) {
		t.Fatalf("EncodePlayerAppearance(equipment) =\n%x\nwant\n%x", got, want)
	}
}

func TestEncodePlayerAppearanceNpcTransform(t *testing.T) {
	// An NPC transformation replaces both 12-slot rows with 0xffff plus
	// the NPC id.
	appearance := PlayerAppearance{
		Gender:              0,
		Colors:              []int{9, 9, 9, 9, 9},
		NpcTransformationID: intPtr(1234),
	}
	want := []byte{0, 0xFF, 0xFF, 0xFF, 0xFF, 0x04, 0xD2}
	want = append(want, 9, 9, 9, 9, 9)
	for _, anim := range []int{1, 2, 3, 4, 5, 6, 7} {
		want = append(want, 0, byte(anim))
	}
	want = append(want,
		'X', 0,
		1,    // combat level
		0, 1, // skill level
		0,    // hidden
		0, 0, // no overrides
		0, 0, 0, // three empty strings
		0, // pronouns fall back to gender
		0, 0, 0, 0,
		0xFF, 0xFF, 0xFF, 0xFF,
		0xFF, 0xFF, 0xFF, 0xFF,
		0, 0, 0, 0,
	)
	got := EncodePlayerAppearance(appearance, "X", 1, 1, []int{1, 2, 3, 4, 5, 6, 7})
	if !bytes.Equal(got, want) {
		t.Fatalf("EncodePlayerAppearance(npc) =\n%x\nwant\n%x", got, want)
	}
}
