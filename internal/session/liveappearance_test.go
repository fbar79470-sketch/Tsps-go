package session

import (
	"bytes"
	"encoding/json"
	"testing"

	"tsps-go/internal/persist"
	"tsps-go/internal/protocol"
)

// inputFromSave derives an AppearanceInput from a save exactly as a
// caller with only save data would: the look and equipment arrays
// verbatim, pronouns through the attribute rule, the skull icon
// through the skulled rule, head hint -1, and the bootstrap's skill
// totals. It mirrors the bootstrap's derivations (bootstrap.go) so
// the byte-equality test below proves the builder shares them.
func inputFromSave(save *persist.PlayerSave, displayName string) AppearanceInput {
	_, total, combat := bootstrapSkills(save)
	look := save.Appearance
	equipment := make([]EquippedItem, len(save.Equipment))
	for i, item := range save.Equipment {
		equipment[i] = EquippedItem{ID: item.ID, Amount: item.Amount}
	}
	gender := 0
	if len(look) > lookGender && look[lookGender] == 1 {
		gender = 1
	}
	pronouns := gender
	if v, ok := attributeInt(save.Attributes, "appearance:pronouns"); ok && v >= 0 && v <= 2 {
		pronouns = v
	}
	skull := -1
	if save.SkullTimer > 0 {
		skull = save.SkullType.IconID
	}
	return AppearanceInput{
		Look:        look,
		Equipment:   equipment,
		DisplayName: displayName,
		CombatLevel: combat,
		TotalLevel:  total,
		Animations:  defaultSyncAnimations,
		SkullIcon:   skull,
		Pronouns:    pronouns,
		HeadHint:    -1,
		Hidden:      false,
	}
}

func TestBuildAppearanceBlockMatchesBootstrapNilSave(t *testing.T) {
	_, total, combat := bootstrapSkills(nil)
	want := protocol.EncodePlayerAppearance(
		*bootstrapAppearance(nil), "testuser", combat, total, defaultSyncAnimations)
	got := BuildAppearanceBlock(AppearanceInput{
		// No display name retained: the bootstrap falls back to
		// the username, so the builder input carries it directly.
		DisplayName: "testuser",
		CombatLevel: combat,
		TotalLevel:  total,
		Animations:  defaultSyncAnimations,
		SkullIcon:   -1,
		Pronouns:    0,
		HeadHint:    -1,
	})
	if !bytes.Equal(got, want) {
		t.Errorf("builder block = %x\nbootstrap block = %x", got, want)
	}
}

func TestBuildAppearanceBlockMatchesBootstrapSave(t *testing.T) {
	skills := persist.DefaultSkills()
	skills.Experience[SkillAttackIndexForTest] = 13034431
	skills.MaxLevel[SkillAttackIndexForTest] = 99
	save := &persist.PlayerSave{
		// Female look with an out-of-range head (999): both paths
		// sanitize through sanitizeLook, so both must clamp it
		// to the female head range identically.
		Appearance: []int{1, 999, 57, 65, 68, 77, 80, -1, 2, 14, 5, 4, 0},
		Equipment: []persist.Item{
			{ID: 1163, Amount: 1}, {ID: -1, Amount: 0}, {ID: 1712, Amount: 1},
			{ID: 1305, Amount: 1}, {ID: 1127, Amount: 1}, {ID: -1, Amount: 0},
			{ID: -1, Amount: 0}, {ID: 1079, Amount: 1}, {ID: -1, Amount: 0},
			{ID: 7462, Amount: 1}, {ID: 3105, Amount: 1}, {ID: -1, Amount: 0},
			{ID: 2550, Amount: 1}, {ID: 892, Amount: 500},
		},
		Attributes: map[string]json.RawMessage{
			"appearance:pronouns": json.RawMessage("2"),
		},
		SkullTimer: 5,
		SkullType:  persist.SkullType{IconID: 1},
		Skills:     skills,
	}
	_, total, combat := bootstrapSkills(save)
	want := protocol.EncodePlayerAppearance(
		*bootstrapAppearance(save), "Test User", combat, total, defaultSyncAnimations)
	got := BuildAppearanceBlock(inputFromSave(save, "Test User"))
	if !bytes.Equal(got, want) {
		t.Errorf("builder block = %x\nbootstrap block = %x", got, want)
	}
}

// SkillAttackIndexForTest is the Attack array index (0). The session
// package must not import game, so the literal is pinned here with
// the Skill.ts citation.
const SkillAttackIndexForTest = 0

func TestBuildAppearanceBlockEquipmentBounds(t *testing.T) {
	base := AppearanceInput{
		Look:        []int{0, 3, 18, 26, 34, 38, 42, 14, 2, 14, 5, 4, 0},
		DisplayName: "testuser",
		CombatLevel: 3,
		TotalLevel:  33,
		Animations:  defaultSyncAnimations,
		SkullIcon:   -1,
		Pronouns:    0,
		HeadHint:    -1,
	}
	// Slots past the 14 equipment slots are ignored.
	long := base
	long.Equipment = make([]EquippedItem, 20)
	for i := range long.Equipment {
		long.Equipment[i] = EquippedItem{ID: 100 + i, Amount: 1}
	}
	truncated := base
	truncated.Equipment = long.Equipment[:persist.EquipmentSize]
	if !bytes.Equal(BuildAppearanceBlock(long), BuildAppearanceBlock(truncated)) {
		t.Error("equipment past slot 13 changed the block")
	}
	// A short equipment list reads the rest as empty slots.
	short := base
	short.Equipment = []EquippedItem{{ID: 1163, Amount: 1}}
	empty := base
	empty.Equipment = nil
	if bytes.Equal(BuildAppearanceBlock(short), BuildAppearanceBlock(empty)) {
		t.Error("one equipped item produced the empty-equipment block")
	}
}

func TestBuildAppearanceBlockHeadHintAndSkull(t *testing.T) {
	// The head icons land in the block's skull/prayer bytes:
	// compare against a hand-built PlayerAppearance with the same
	// derivation the builder applies, pinning the field placement.
	in := AppearanceInput{
		Look:        []int{0, 3, 18, 26, 34, 38, 42, 14, 2, 14, 5, 4, 0},
		DisplayName: "testuser",
		CombatLevel: 3,
		TotalLevel:  33,
		Animations:  defaultSyncAnimations,
		SkullIcon:   0,
		Pronouns:    0,
		HeadHint:    2,
	}
	pronouns := 0
	equipIDs := make([]int, persist.EquipmentSize)
	equipQty := make([]int, persist.EquipmentSize)
	for i := range equipIDs {
		equipIDs[i] = -1
	}
	want := protocol.EncodePlayerAppearance(protocol.PlayerAppearance{
		Gender:   0,
		Pronouns: &pronouns,
		Colors:   []int{2, 14, 5, 4, 0},
		Kits:     []int{3, 14, 18, 26, 34, 38, 42},
		Equip:    equipIDs,
		EquipQty: equipQty,
		HeadIcons: &protocol.HeadIcons{
			Skull:  0,
			Prayer: 2,
		},
	}, "testuser", 3, 33, defaultSyncAnimations)
	if got := BuildAppearanceBlock(in); !bytes.Equal(got, want) {
		t.Errorf("builder block = %x\nwant            = %x", got, want)
	}
	// And the icons actually vary the block: skull -1 / hint -1
	// must differ from skull 0 / hint 2.
	plain := in
	plain.SkullIcon = -1
	plain.HeadHint = -1
	if bytes.Equal(BuildAppearanceBlock(in), BuildAppearanceBlock(plain)) {
		t.Error("head icons did not change the block")
	}
}
