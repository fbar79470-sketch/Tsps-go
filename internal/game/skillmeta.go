package game

// Skill metadata: the identity table in game/model/Skill.ts
// (187 lines) at commit bfd81e3. Each skill is constructed in
// declaration order (the nextIndex counter assigns the array
// index, Skill.ts:7-31) with (chatboxInterface, button,
// clientId?): the chatbox interface sent on level-up, the skills
// tab button id, and an optional client id overriding the index
// (getClientId, Skill.ts: getClientId returns clientId ?? index).
// Only Construction (index 21, client id 22) and Hunter (index
// 22, client id 21) differ; Sailing (index 23) has no legacy
// level-up chatbox or skills tab button (-1, -1) and client id
// 23, as in OSRS. Names are Skill.NAMES (Skill.ts:59-84), in the
// same order as the VALUES list (Skill.ts:33-58).
//
// The session package keeps its own client-id copy
// (skillClientIDs, bootstrap.go) for the bootstrap snapshot and
// must not be refactored to share this table (the session package
// never imports game); SkillClientID here is the game-side
// reading, for content and future level-up feedback. The names
// duplicate equipmentRequirementSkillNames (equipment.go),
// which stays hardcoded so the requirements check does not
// depend on this table; the two lists are the same Skill.NAMES
// order and TestSkillMetadata pins this one.

// skillMetadata is one row per skill index: name, tab button id,
// level-up chatbox interface id, and client id.
var skillMetadata = [SkillCount]struct {
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

// SkillName ports Skill.getName (Skill.ts): the skill's display
// name. An out-of-range index returns "" per the skills.go guard
// policy (TS returns "Skill" for a missing name, but no Go caller
// passes an invalid index; the empty string keeps corrupt input
// from naming a real skill).
func SkillName(skill int) string {
	if !validSkill(skill) {
		return ""
	}
	return skillMetadata[skill].name
}

// SkillButton ports Skill.getButton (Skill.ts): the skill's
// button id in the skills tab interface. An out-of-range index
// returns -1 per the skills.go guard policy. Sailing legitimately
// returns -1: it has no skills tab button.
func SkillButton(skill int) int {
	if !validSkill(skill) {
		return -1
	}
	return skillMetadata[skill].button
}

// SkillChatboxInterface ports Skill.getChatboxInterface
// (Skill.ts): the chatbox interface sent on level-up. An
// out-of-range index returns -1 per the skills.go guard policy.
// Sailing legitimately returns -1: it has no legacy level-up
// chatbox.
func SkillChatboxInterface(skill int) int {
	if !validSkill(skill) {
		return -1
	}
	return skillMetadata[skill].chatbox
}

// SkillClientID ports Skill.getClientId (Skill.ts): the id the
// client knows the skill by. It equals the index except for
// Construction (21 -> 22) and Hunter (22 -> 21). An out-of-range
// index returns -1 per the skills.go guard policy.
func SkillClientID(skill int) int {
	if !validSkill(skill) {
		return -1
	}
	return skillMetadata[skill].client
}
