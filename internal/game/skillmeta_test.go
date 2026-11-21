package game

import "testing"

func TestSkillMetadata(t *testing.T) {
	// Full table from Skill.ts:7-31 (constructor args are
	// (chatboxInterface, button, clientId?)) and Skill.NAMES
	// (Skill.ts:59-84), in declaration order.
	want := []struct {
		name    string
		button  int
		chatbox int
		client  int
	}{
		{"Attack", 8654, 6247, 0},
		{"Defence", 8660, 6253, 1},
		{"Strength", 8657, 6206, 2},
		{"Hitpoints", 8655, 6216, 3},
		{"Ranged", 8663, 4443, 4},
		{"Prayer", 8666, 6242, 5},
		{"Magic", 8669, 6211, 6},
		{"Cooking", 8665, 6226, 7},
		{"Woodcutting", 8671, 4272, 8},
		{"Fletching", 8670, 6231, 9},
		{"Fishing", 8662, 6258, 10},
		{"Firemaking", 8668, 4282, 11},
		{"Crafting", 8667, 6263, 12},
		{"Smithing", 8659, 6221, 13},
		{"Mining", 8656, 4416, 14},
		{"Herblore", 8661, 6237, 15},
		{"Agility", 8658, 4277, 16},
		{"Thieving", 8664, 4261, 17},
		{"Slayer", 12162, 12122, 18},
		{"Farming", 13928, 5267, 19},
		{"Runecrafting", 8672, 4267, 20},
		{"Construction", 18801, 7267, 22},
		{"Hunter", 18829, 8267, 21},
		{"Sailing", -1, -1, 23},
	}
	if len(want) != SkillCount {
		t.Fatalf("table rows = %d, want SkillCount %d", len(want), SkillCount)
	}
	for i, w := range want {
		if got := SkillName(i); got != w.name {
			t.Errorf("SkillName(%d) = %q, want %q", i, got, w.name)
		}
		if got := SkillButton(i); got != w.button {
			t.Errorf("SkillButton(%d) = %d, want %d", i, got, w.button)
		}
		if got := SkillChatboxInterface(i); got != w.chatbox {
			t.Errorf("SkillChatboxInterface(%d) = %d, want %d", i, got, w.chatbox)
		}
		if got := SkillClientID(i); got != w.client {
			t.Errorf("SkillClientID(%d) = %d, want %d", i, got, w.client)
		}
	}
	// The Construction/Hunter client-id swap, pinned explicitly.
	if got := SkillClientID(SkillConstruction); got != 22 {
		t.Errorf("SkillClientID(Construction) = %d, want 22", got)
	}
	if got := SkillClientID(SkillHunter); got != 21 {
		t.Errorf("SkillClientID(Hunter) = %d, want 21", got)
	}
	// Out-of-range indices: "" / -1 per the skills.go guard policy.
	for _, bad := range []int{-1, SkillCount, 99} {
		if got := SkillName(bad); got != "" {
			t.Errorf("SkillName(%d) = %q, want \"\"", bad, got)
		}
		if got := SkillButton(bad); got != -1 {
			t.Errorf("SkillButton(%d) = %d, want -1", bad, got)
		}
		if got := SkillChatboxInterface(bad); got != -1 {
			t.Errorf("SkillChatboxInterface(%d) = %d, want -1", bad, got)
		}
		if got := SkillClientID(bad); got != -1 {
			t.Errorf("SkillClientID(%d) = %d, want -1", bad, got)
		}
	}
}
