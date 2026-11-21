package definition

import (
	"math"
	"strings"
	"testing"
)

func npcCacheSpec() fakeCacheSpec {
	return fakeCacheSpec{
		npcs: map[int][]byte{
			0: npcEntry("Goblin", npcAction(1, "Attack"), npcCombatLevel(2), npcIdleSeq(10),
				npcLevel(74, 1), npcLevel(75, 1), npcLevel(76, 1), npcLevel(77, 5), npcLevel(78, 1), npcLevel(79, 1)),
			1: npcEntry("Man", npcIdleSeq(11)),
			2: npcEntry("Dragon", npcAction(1, "Attack"), npcSize(2), npcIdleSeq(20)),
			3: npcEntry("Guard", npcDesc("A guard."), npcAction(1, "Attack"), npcIdleSeq(11)),
			4: npcEntry("Coal", npcIdleSeq(11)),
		},
		seqNames: map[int]string{
			10: "goblin_ready",
			11: "human_ready",
			12: "human_unarmedblock",
			13: "goblin_attack",
			14: "goblin_death",
			15: "dragon_melee",
			16: "dragon_death",
			17: "dragon_block",
			20: "dragon_idle",
			21: "human_unarmedpunch",
		},
	}
}

func loadNPCs(t *testing.T, files map[string]string, spec fakeCacheSpec) *Store {
	t.Helper()
	defs, gamevals := newFakeCache(t, spec)
	dir := fixtureDir(t, files)
	store, err := Load(dir, LoadOptions{Cache: defs, Gamevals: gamevals})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	return store
}

func TestNpcHydrateFromCache(t *testing.T) {
	store := loadNPCs(t, minimalFiles(), npcCacheSpec())

	goblin := store.NPC(0)
	if goblin.ID() != 0 || goblin.Name() != "Goblin" || goblin.Size() != 1 {
		t.Fatalf("goblin = %+v", goblin)
	}
	if !goblin.IsAttackable() || goblin.CombatLevel() != 2 || goblin.Hitpoints() != 5 {
		t.Fatalf("goblin attackable=%v cb=%d hp=%d", goblin.IsAttackable(), goblin.CombatLevel(), goblin.Hitpoints())
	}
	stats := goblin.Stats()
	if stats[0] != 1 || stats[1] != 1 || stats[2] != 1 || stats[3] != 1 || stats[4] != 1 {
		t.Fatalf("goblin stats = %v", stats)
	}
	if goblin.DefenceAnim() != -1 {
		t.Fatalf("goblin defence anim = %d (no human rig)", goblin.DefenceAnim())
	}

	// Human rigs inherit the legacy human block.
	man := store.NPC(1)
	if man.DefenceAnim() != 12 {
		t.Fatalf("man defence anim = %d, want 12", man.DefenceAnim())
	}
	if man.IsAttackable() {
		t.Fatal("man must not be attackable")
	}

	dragon := store.NPC(2)
	if dragon.Size() != 2 {
		t.Fatalf("dragon size = %d", dragon.Size())
	}
	if dragon.Examine() != "It's a Dragon." {
		t.Fatalf("dragon examine = %q", dragon.Examine())
	}
}

func TestNpcDefaults(t *testing.T) {
	store := loadNPCs(t, minimalFiles(), npcCacheSpec())
	def := store.NPC(999)
	if def.ID() != -1 || def.Name() != "null" || def.Size() != 1 {
		t.Fatalf("default = %+v", def)
	}
	if def.Respawn() != 25 || def.MaxHit() != 1 || def.Hitpoints() != 10 || def.AttackSpeed() != 4 {
		t.Fatal("default combat values wrong")
	}
	if def.AttackAnim() != 422 || def.DefenceAnim() != -1 || def.DeathAnim() != 836 || def.DeathSound() != -1 || def.DeathTicks() != 2 {
		t.Fatal("default anims wrong")
	}
	if _, ok := def.SpawnAnim(); ok {
		t.Fatal("default spawn anim must be absent")
	}
	if def.HuntRange() != 3 || def.CombatFollowDistance() != 7 || def.AttackType() != CombatTypeMelee {
		t.Fatal("default ranges/type wrong")
	}
	if !def.BuildsAggressionTolerance() || !def.DoesFightBack() {
		t.Fatal("default flags wrong")
	}
	stats := def.Stats()
	if len(stats) != 18 || stats[0] != 1 || stats[5] != 0 {
		t.Fatalf("default stats = %v", stats)
	}
}

func TestNpcExamineFallbacks(t *testing.T) {
	store := loadNPCs(t, withFiles(map[string]string{
		"npc-examines.json": `{"0": "Wiki goblin text.", "2": "Wiki dragon text."}`,
	}), npcCacheSpec())
	// Cache desc wins over the wiki file.
	if got := store.NPC(3).Examine(); got != "A guard." {
		t.Fatalf("guard examine = %q", got)
	}
	// Wiki file next.
	if got := store.NPC(0).Examine(); got != "Wiki goblin text." {
		t.Fatalf("goblin examine = %q", got)
	}
	// Generic lines.
	if got := store.NPC(1).Examine(); got != "It's a Man." {
		t.Fatalf("man examine = %q", got)
	}
	if got := store.NPC(4).Examine(); got != "It's some Coal." {
		t.Fatalf("coal examine = %q", got)
	}
	if got := store.NPC(999).Examine(); got != "It's nothing special." {
		t.Fatalf("default examine = %q", got)
	}
}

func TestMonsterDumpApplication(t *testing.T) {
	monsters := `{
		"0": {"name": "  GOBLIN ", "examine": "A nasty goblin.", "hitpoints": 50,
			"attack_level": 10, "strength_level": 11, "defence_level": 12,
			"ranged_level": 13, "magic_level": 14, "attack_speed": 5, "max_hit": 8,
			"aggressive": true, "poisonous": true, "venomous": false,
			"attributes": ["demon", "undead"], "members": true,
			"elemental_weakness_type": 2, "elemental_weakness_percent": 50.9,
			"attack_type": ["stab", "ranged"], "attack_bonus": 5, "attack_magic": 6, "attack_ranged": 7,
			"defence_stab": 8, "defence_slash": 9, "defence_crush": 10, "defence_magic": 11,
			"defence_ranged_standard": 12, "defence_ranged_light": 13, "defence_ranged_heavy": 14,
			"slayer_monster": true, "slayer_level": 5},
		"1": {"name": "Not a man", "hitpoints": 99, "aggressive": true},
		"2": {"name": "Dragon", "elemental_weakness_type": 5, "elemental_weakness_percent": 80,
			"attack_type": ["ranged"], "slayer_monster": false, "slayer_level": 9}
	}`
	var logs logCapture
	defs, gamevals := newFakeCache(t, npcCacheSpec())
	dir := fixtureDir(t, withFiles(map[string]string{"monsters-complete.json": monsters}))
	store, err := Load(dir, LoadOptions{Cache: defs, Gamevals: gamevals, Logf: logs.logf})
	if err != nil {
		t.Fatal(err)
	}
	goblin := store.NPC(0)
	if goblin.Examine() != "A nasty goblin." || goblin.Hitpoints() != 50 || goblin.AttackSpeed() != 5 || goblin.MaxHit() != 8 {
		t.Fatalf("goblin = examine %q hp %d speed %d max %d", goblin.Examine(), goblin.Hitpoints(), goblin.AttackSpeed(), goblin.MaxHit())
	}
	stats := goblin.Stats()
	want := []int{10, 11, 12, 13, 14, 5, 0, 6, 0, 7, 8, 9, 10, 11, 12, 0, 0, 0}
	for i, w := range want {
		if stats[i] != w {
			t.Fatalf("stats[%d] = %d, want %d (stats=%v)", i, stats[i], w, stats)
		}
	}
	if !goblin.IsAggressive() || !goblin.IsPoisonous() || goblin.IsVenomous() || !goblin.IsDemon() || !goblin.IsUndead() || !goblin.IsMembers() {
		t.Fatal("goblin flags wrong")
	}
	if goblin.AttackType() != CombatTypeMelee || goblin.AttackStyle() != "stab" || goblin.SlayerLevel() != 5 {
		t.Fatalf("goblin type=%d style=%q slayer=%d", goblin.AttackType(), goblin.AttackStyle(), goblin.SlayerLevel())
	}
	if weakness, ok := goblin.ElementalWeakness(); !ok || weakness.Type != 2 || weakness.Percent != 50 {
		t.Fatalf("weakness = %+v ok=%v", weakness, ok)
	}
	if goblin.RangedDefenceBonus(RangedDefenceStandard) != 12 ||
		goblin.RangedDefenceBonus(RangedDefenceLight) != 13 ||
		goblin.RangedDefenceBonus(RangedDefenceHeavy) != 14 {
		t.Fatalf("ranged defence = %d/%d/%d",
			goblin.RangedDefenceBonus("standard"), goblin.RangedDefenceBonus("light"), goblin.RangedDefenceBonus("heavy"))
	}
	// Name mismatch: the dump is dropped, not applied.
	man := store.NPC(1)
	if man.Hitpoints() != 10 || man.IsAggressive() {
		t.Fatalf("man must keep defaults: hp=%d aggressive=%v", man.Hitpoints(), man.IsAggressive())
	}
	if !logs.contains("mismatched=1") {
		t.Fatalf("logs = %q", logs.joined())
	}
	// Weakness type out of range zeroes both fields; pure ranged
	// switches style; slayer_monster false drops the level.
	dragon := store.NPC(2)
	if _, ok := dragon.ElementalWeakness(); ok {
		t.Fatal("dragon must have no weakness")
	}
	if dragon.AttackType() != CombatTypeRanged || dragon.SlayerLevel() != 0 {
		t.Fatalf("dragon type=%d slayer=%d", dragon.AttackType(), dragon.SlayerLevel())
	}
}

func TestResolveAttackType(t *testing.T) {
	cases := []struct {
		input any
		want  CombatType
	}{
		{[]any{"stab"}, CombatTypeMelee},
		{[]any{"CRUSH"}, CombatTypeMelee},
		{[]any{"ranged"}, CombatTypeRanged},
		{[]any{"magic"}, CombatTypeMagic},
		{[]any{"ranged", "magic"}, CombatTypeRanged},
		{[]any{"magic", "melee"}, CombatTypeMelee},
		{[]any{"dragonfire"}, CombatTypeMelee},
		{[]any{"typeless"}, CombatTypeMelee},
		{[]any{}, CombatTypeMelee},
		{"ranged", CombatTypeMelee},
		{nil, CombatTypeMelee},
	}
	for _, tc := range cases {
		if got := ResolveAttackType(tc.input); got != tc.want {
			t.Fatalf("ResolveAttackType(%v) = %d, want %d", tc.input, got, tc.want)
		}
	}
}

func TestStatusImmunities(t *testing.T) {
	status := `{"schema_version": 1, "npcs": {
		"0": {"name": "Goblin", "immune_poison": true, "immune_venom": true, "freeze_resistance": 50},
		"1": {"name": "Wrong", "immune_poison": true, "freeze_resistance": 90},
		"2": {"name": "Dragon", "freeze_resistance": 150},
		"3": {"name": "Guard", "freeze_resistance": "50"}
	}}`
	store := loadNPCs(t, withFiles(map[string]string{"npc-status-immunities.json": status}), npcCacheSpec())
	goblin := store.NPC(0)
	if !goblin.IsPoisonImmune() || !goblin.IsVenomImmune() || goblin.FreezeResistance() != 50 {
		t.Fatalf("goblin immunities wrong: %+v", goblin)
	}
	if store.NPC(1).IsPoisonImmune() {
		t.Fatal("mismatched status entry must not apply")
	}
	if got := store.NPC(2).FreezeResistance(); got != 100 {
		t.Fatalf("freeze clamped = %d", got)
	}
	if got := store.NPC(3).FreezeResistance(); got != 0 {
		t.Fatalf("string freeze resistance = %d", got)
	}
}

func TestCombatDefsApplication(t *testing.T) {
	combatDefs := `{
		"refs": {"npcs": [[0, 100, 101, 102]]},
		"npcs": {
			"1": {"anims": {"attack": 200, "spawn": 201}, "sounds": {"death": 300},
			      "deathTicks": 3, "projectile": 400, "launchGraphic": 401,
			      "respawn": 10, "huntRange": 5},
			"2": {"projectile": 402, "respawn": -1, "huntRange": 0},
			"3": {"anims": {"attack": 202}, "deathTicks": 2.5}
		}
	}`
	store := loadNPCs(t, withFiles(map[string]string{"npc-combat-defs.json": combatDefs}), npcCacheSpec())
	goblin := store.NPC(0)
	if goblin.AttackAnim() != 100 || goblin.DefenceAnim() != 101 || goblin.DeathAnim() != 102 {
		t.Fatalf("goblin anims = %d/%d/%d", goblin.AttackAnim(), goblin.DefenceAnim(), goblin.DeathAnim())
	}
	man := store.NPC(1)
	if man.AttackAnim() != 200 || man.DeathSound() != 300 || man.DeathTicks() != 3 {
		t.Fatalf("man anims/sound/ticks = %d/%d/%d", man.AttackAnim(), man.DeathSound(), man.DeathTicks())
	}
	if spawn, ok := man.SpawnAnim(); !ok || spawn != 201 {
		t.Fatalf("man spawn anim = %d ok=%v", spawn, ok)
	}
	if man.ProjectileID() != 400 || man.LaunchGraphicID() != 401 || man.Respawn() != 10 || man.HuntRange() != 5 {
		t.Fatalf("man projectile=%d launch=%d respawn=%d hunt=%d",
			man.ProjectileID(), man.LaunchGraphicID(), man.Respawn(), man.HuntRange())
	}
	// Projectile without anims still applies; invalid respawn/hunt
	// values are ignored.
	dragon := store.NPC(2)
	if dragon.ProjectileID() != 402 || dragon.Respawn() != 25 || dragon.HuntRange() != 3 {
		t.Fatalf("dragon projectile=%d respawn=%d hunt=%d", dragon.ProjectileID(), dragon.Respawn(), dragon.HuntRange())
	}
	// Non-integer deathTicks is ignored.
	if store.NPC(3).DeathTicks() != 2 {
		t.Fatalf("guard deathTicks = %d", store.NPC(3).DeathTicks())
	}
}

func TestCapturedRoles(t *testing.T) {
	captured := `{"npcs": {
		"1": {"hits": [20, 15], "seen": {}},
		"0": {"seen": {"30": [3, 0, 0, 0], "31": [0, 10, 1, 0]}}
	}}`
	store := loadNPCs(t, withFiles(map[string]string{"npc-captured-animations.json": captured}), npcCacheSpec())
	// Hit often, mostly unanswered: no block, overriding the human
	// default.
	if got := store.NPC(1).DefenceAnim(); got != -1 {
		t.Fatalf("man defence anim = %d, want -1 sentinel", got)
	}
	// Timing alone: a death, and an almost-only-on-hits block.
	goblin := store.NPC(0)
	if goblin.DeathAnim() != 30 || goblin.DefenceAnim() != 31 {
		t.Fatalf("goblin death=%d block=%d", goblin.DeathAnim(), goblin.DefenceAnim())
	}
}

func TestCapturedNameRoles(t *testing.T) {
	captured := `{"npcs": {"0": {"seen": {"13": [0, 0, 5, 0], "14": [3, 0, 0, 0]}}}}`
	store := loadNPCs(t, withFiles(map[string]string{"npc-captured-animations.json": captured}), npcCacheSpec())
	goblin := store.NPC(0)
	if goblin.AttackAnim() != 13 || goblin.DeathAnim() != 14 {
		t.Fatalf("goblin attack=%d death=%d", goblin.AttackAnim(), goblin.DeathAnim())
	}
}

func TestObservedInference(t *testing.T) {
	observed := `{"2": [30, 31, 32], "0": [13, 14]}`
	store := loadNPCs(t, withFiles(map[string]string{"npc-animations.json": observed}), npcCacheSpec())
	// Named roles from the cache names.
	goblin := store.NPC(0)
	if goblin.AttackAnim() != 13 || goblin.DeathAnim() != 14 {
		t.Fatalf("goblin attack=%d death=%d", goblin.AttackAnim(), goblin.DeathAnim())
	}
	// Unnamed: attack prefers index 1, block index 0, death
	// reversed.
	dragon := store.NPC(2)
	if dragon.AttackAnim() != 31 || dragon.DefenceAnim() != 30 || dragon.DeathAnim() != 32 {
		t.Fatalf("dragon attack=%d block=%d death=%d", dragon.AttackAnim(), dragon.DefenceAnim(), dragon.DeathAnim())
	}
}

func TestObservedGuessingSkippedWhenConfigured(t *testing.T) {
	store := loadNPCs(t, withFiles(map[string]string{
		"npc-combat-defs.json": `{"refs": {"npcs": [[2, 100, 101, 102]]}}`,
		"npc-animations.json":  `{"2": [30, 31, 32]}`,
	}), npcCacheSpec())
	dragon := store.NPC(2)
	if dragon.AttackAnim() != 100 || dragon.DefenceAnim() != 101 || dragon.DeathAnim() != 102 {
		t.Fatalf("configured anims must win: %d/%d/%d", dragon.AttackAnim(), dragon.DefenceAnim(), dragon.DeathAnim())
	}
}

func TestFamilyRoles(t *testing.T) {
	store := loadNPCs(t, minimalFiles(), npcCacheSpec())
	// Dragon: stand name "dragon_idle" gives the dragon family.
	dragon := store.NPC(2)
	if dragon.AttackAnim() != 15 || dragon.DefenceAnim() != 17 || dragon.DeathAnim() != 16 {
		t.Fatalf("dragon family anims = %d/%d/%d", dragon.AttackAnim(), dragon.DefenceAnim(), dragon.DeathAnim())
	}
	// Human family is too broad: the man keeps defaults (plus the
	// human block from hydration).
	man := store.NPC(1)
	if man.AttackAnim() != 422 || man.DeathAnim() != 836 || man.DefenceAnim() != 12 {
		t.Fatalf("man anims = %d/%d/%d", man.AttackAnim(), man.DefenceAnim(), man.DeathAnim())
	}
	// The goblin family has attack/death but no block word: block
	// stays unset.
	goblin := store.NPC(0)
	if goblin.AttackAnim() != 13 || goblin.DeathAnim() != 14 || goblin.DefenceAnim() != -1 {
		t.Fatalf("goblin family anims = %d/%d/%d", goblin.AttackAnim(), goblin.DefenceAnim(), goblin.DeathAnim())
	}
}

func TestAnimationRoleByName(t *testing.T) {
	cases := []struct {
		name string
		want combatAnimationRole
	}{
		{"snake_death", roleDeath},
		{"npc_x_melee_death", roleDeath},
		{"death_anim", roleDeath},
		{"specter_parry", roleBlock},
		{"human_unarmedblock", roleBlock},
		{"npc_defend", roleBlock},
		{"human_unarmedpunch", roleAttack},
		{"zombie_update_attack_weapon", roleAttack},
		{"dragon_attack_special", roleAttack},
		{"cast_fire", roleAttack},
		{"dragon_melee", roleAttack},
		{"top_spider_ranged_idle", ""},
		{"human_ready", ""},
		{"goblin_ready", ""},
		{"", ""},
		{"kick_boxer", roleAttack},
		{"strike_down", roleAttack},
	}
	for _, tc := range cases {
		if got := animationRoleByName(tc.name); got != tc.want {
			t.Fatalf("animationRoleByName(%q) = %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestNpcAggressionRange(t *testing.T) {
	store := loadNPCs(t, minimalFiles(), npcCacheSpec())
	def := store.NPC(0)
	if got := def.AggressionRange(3.7); got != 10 {
		t.Fatalf("aggression range = %d", got)
	}
	if got := def.AggressionRange(math.NaN()); got != 8 {
		t.Fatalf("NaN aggression range = %d", got)
	}
	if got := def.AggressionRange(0); got != 8 {
		t.Fatalf("zero aggression range = %d", got)
	}
}

func TestNpcRangedDefenceFallback(t *testing.T) {
	store := loadNPCs(t, minimalFiles(), npcCacheSpec())
	def := store.NPC(0)
	if got := def.RangedDefenceBonus("light"); got != def.Stats()[14] {
		t.Fatalf("light fallback = %d", got)
	}
	if got := def.RangedDefenceBonus("bogus"); got != def.Stats()[14] {
		t.Fatalf("unknown kind = %d", got)
	}
}

func TestNpcNilCacheKeepsDefaults(t *testing.T) {
	dir := fixtureDir(t, withFiles(map[string]string{
		"monsters-complete.json": `{"0": {"name": "Goblin", "hitpoints": 50}}`,
		"npc-combat-defs.json":   `{"refs": {"npcs": [[0, 1, 2, 3]]}}`,
	}))
	store, err := Load(dir, LoadOptions{})
	if err != nil {
		t.Fatal(err)
	}
	def := store.NPC(0)
	if def.ID() != -1 || def.Hitpoints() != 10 || def.AttackAnim() != 422 {
		t.Fatalf("nil-cache NPC must be DEFAULT: %+v", def)
	}
	if !strings.Contains(def.Examine(), "nothing special") {
		t.Fatalf("examine = %q", def.Examine())
	}
}
