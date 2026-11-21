package definition

import (
	"testing"
)

const equipmentSoundsJSON = `{
	"default": 1000,
	"weaponTypes": {
		"Sword": [{"name": "bronze", "sound": 200}, {"sound": 201}],
		"Axe": 0,
		"Bow": [{"name": "magic", "sound": 300}]
	},
	"slots": {
		"weapon": 400,
		"head": [{"name": "ahrim", "sound": 500}, {"sound": 501}],
		"body": 0
	}
}`

func loadSounds(t *testing.T, files map[string]string, spec fakeCacheSpec) *Store {
	t.Helper()
	defs, gamevals := newFakeCache(t, spec)
	dir := fixtureDir(t, files)
	store, err := Load(dir, LoadOptions{Cache: defs, Gamevals: gamevals})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	return store
}

func TestEquipmentSoundTiers(t *testing.T) {
	// Items 4 and 9 need weapon slots, which the shared spec does
	// not give them.
	spec := itemCacheSpec()
	spec.objs[4] = objEntry("Untradable thing", objPrice(100), objWearPos(3), objInventoryAction(4, "Destroy"))
	spec.objs[9] = objEntry("Slow weapon", objWearPos(3), objParams([2]int{14, 0}))
	store := loadSounds(t, withFiles(map[string]string{
		"equipment-sounds.json": equipmentSoundsJSON,
		"item-gameplay.json": `[
			{"id": 2, "name": "Bronze sword", "weaponInterface": "Sword"},
			{"id": 5, "name": "Two-handed sword", "weaponInterface": "Sword"},
			{"id": 6, "name": "Helmet", "weaponInterface": "Sword", "equipSound": 109},
			{"id": 4, "name": "Untradable thing", "weaponInterface": "Axe"},
			{"id": 9, "name": "Slow weapon", "weaponInterface": "Bow"}
		]`,
	}), spec)

	// Weapon rule with a matching pattern.
	if got := store.EquipmentSoundID(store.Item(2)); got != 200 {
		t.Fatalf("bronze sword sound = %d", got)
	}
	// Weapon rule match-all fallback.
	if got := store.EquipmentSoundID(store.Item(5)); got != 201 {
		t.Fatalf("two-handed sword sound = %d", got)
	}
	// The item's own equipSound wins over every tier.
	if got := store.EquipmentSoundID(store.Item(6)); got != 109 {
		t.Fatalf("helmet sound = %d", got)
	}
	// A number rule wins even when it is 0.
	if got := store.EquipmentSoundID(store.Item(4)); got != 0 {
		t.Fatalf("axe sound = %d", got)
	}
	// No weapon rule matches: fall through to the slot rule.
	if got := store.EquipmentSoundID(store.Item(9)); got != 400 {
		t.Fatalf("bow sound = %d", got)
	}
}

func TestEquipmentSoundSlotAndDefault(t *testing.T) {
	spec := itemCacheSpec()
	spec.objs[7] = objEntry("Ahrim's hood", objWearPos(0))
	spec.objs[8] = objEntry("Platebody", objWearPos(4))
	store := loadSounds(t, withFiles(map[string]string{
		"equipment-sounds.json": equipmentSoundsJSON,
	}), spec)
	// Slot pattern match.
	if got := store.EquipmentSoundID(store.Item(7)); got != 500 {
		t.Fatalf("ahrim hood sound = %d", got)
	}
	// Slot match-all fallback.
	if got := store.EquipmentSoundID(store.Item(6)); got != 501 {
		t.Fatalf("helmet sound = %d", got)
	}
	// Slot number rule 0 wins.
	if got := store.EquipmentSoundID(store.Item(8)); got != 0 {
		t.Fatalf("platebody sound = %d", got)
	}
	// No slot, no rules: the default.
	if got := store.EquipmentSoundID(store.Item(4)); got != 1000 {
		t.Fatalf("unslotted sound = %d", got)
	}
	// Weapon rules do not apply outside the weapon slot.
	if got := store.EquipmentSoundID(store.Item(7)); got != 500 {
		t.Fatalf("hood sound = %d", got)
	}
}

func TestEquipmentSoundWeaponRulesNeedWeaponSlot(t *testing.T) {
	// A helmet carrying a weapon interface still uses slot rules.
	spec := itemCacheSpec()
	spec.objs[6] = objEntry("Bronze helmet", objWearPos(0), objTradable())
	store := loadSounds(t, withFiles(map[string]string{
		"equipment-sounds.json": equipmentSoundsJSON,
		"item-gameplay.json":    `[{"id": 6, "name": "Bronze helmet", "weaponInterface": "Sword"}]`,
	}), spec)
	if got := store.EquipmentSoundID(store.Item(6)); got != 501 {
		t.Fatalf("helmet with sword interface = %d, want slot rule 501", got)
	}
}

func TestEquipmentSoundMissingFile(t *testing.T) {
	store := loadSounds(t, minimalFiles(), itemCacheSpec())
	if got := store.EquipmentSoundID(store.Item(2)); got != 0 {
		t.Fatalf("missing file sound = %d", got)
	}
}

func TestEquipmentSoundInvalidPattern(t *testing.T) {
	var logs logCapture
	defs, gamevals := newFakeCache(t, itemCacheSpec())
	dir := fixtureDir(t, withFiles(map[string]string{
		"equipment-sounds.json": `{"default": 1, "slots": {"weapon": [{"name": "[", "sound": 9}, {"sound": 8}]}}`,
	}))
	store, err := Load(dir, LoadOptions{Cache: defs, Gamevals: gamevals, Logf: logs.logf})
	if err != nil {
		t.Fatal(err)
	}
	if !logs.contains("invalid sound pattern") {
		t.Fatalf("logs = %q", logs.joined())
	}
	// The broken rule never matches; the match-all rule wins.
	if got := store.EquipmentSoundID(store.Item(2)); got != 8 {
		t.Fatalf("sound = %d", got)
	}
}

func TestEquipmentSoundMalformedFails(t *testing.T) {
	defs, _ := newFakeCache(t, itemCacheSpec())
	dir := fixtureDir(t, withFiles(map[string]string{"equipment-sounds.json": `{bad`}))
	if _, err := Load(dir, LoadOptions{Cache: defs}); err == nil {
		t.Fatal("malformed equipment-sounds.json must fail")
	}
}

const npcCombatSoundsJSON = `{"npcs": {
	"0": {"hit": [[10, 0], [11, 5]], "attack": [[20, 2]], "death": [[30, 0]]},
	"1": {"hit": [[12, 0], "bad", [13], [14, 1, 99]]},
	"x": {"hit": [[1, 0]]}
}}`

func TestNpcCombatSoundEntries(t *testing.T) {
	store := loadSounds(t, withFiles(map[string]string{
		"npc-combat-sounds.json": npcCombatSoundsJSON,
	}), npcCacheSpec())
	hits := store.NpcCombatSoundEntries(0, NpcSoundHit)
	if len(hits) != 2 || hits[0] != (NpcSoundEntry{Sound: 10, Delay: 0}) || hits[1] != (NpcSoundEntry{Sound: 11, Delay: 5}) {
		t.Fatalf("hits = %v", hits)
	}
	if attacks := store.NpcCombatSoundEntries(0, NpcSoundAttack); len(attacks) != 1 || attacks[0].Sound != 20 || attacks[0].Delay != 2 {
		t.Fatalf("attacks = %v", attacks)
	}
	if len(store.NpcCombatSoundEntries(0, "bogus")) != 0 {
		t.Fatal("unknown kind must yield no entries")
	}
	if len(store.NpcCombatSoundEntries(99, NpcSoundHit)) != 0 {
		t.Fatal("unknown id must yield no entries")
	}
	// Malformed entries are skipped; the extra third element is
	// ignored.
	manHits := store.NpcCombatSoundEntries(1, NpcSoundHit)
	if len(manHits) != 2 || manHits[0].Sound != 12 || manHits[1].Sound != 14 || manHits[1].Delay != 1 {
		t.Fatalf("man hits = %v", manHits)
	}
}

func TestPickNpcCombatSound(t *testing.T) {
	store := loadSounds(t, withFiles(map[string]string{
		"npc-combat-sounds.json": npcCombatSoundsJSON,
	}), npcCacheSpec())
	// Injected randomness picks the second entry.
	entry, ok := store.PickNpcCombatSound(0, NpcSoundHit, func(n int) int { return 1 })
	if !ok || entry != (NpcSoundEntry{Sound: 11, Delay: 5}) {
		t.Fatalf("pick = %+v ok=%v", entry, ok)
	}
	// Nil randomness picks the first.
	entry, ok = store.PickNpcCombatSound(0, NpcSoundHit, nil)
	if !ok || entry.Sound != 10 {
		t.Fatalf("pick nil = %+v ok=%v", entry, ok)
	}
	// Single entry: no randomness needed.
	entry, ok = store.PickNpcCombatSound(0, NpcSoundAttack, func(n int) int { t.Fatal("must not randomize a single entry"); return 0 })
	if !ok || entry.Sound != 20 {
		t.Fatalf("pick single = %+v ok=%v", entry, ok)
	}
	if _, ok := store.PickNpcCombatSound(99, NpcSoundHit, nil); ok {
		t.Fatal("unknown id must not pick")
	}
	if _, ok := store.PickNpcCombatSound(0, "bogus", nil); ok {
		t.Fatal("unknown kind must not pick")
	}
}

func TestNpcDeathSound(t *testing.T) {
	store := loadSounds(t, withFiles(map[string]string{
		"npc-combat-sounds.json": npcCombatSoundsJSON,
		"npc-combat-defs.json":   `{"npcs": {"1": {"anims": {"attack": 200}, "sounds": {"death": 300}}}}`,
	}), npcCacheSpec())
	// The definition's own death sound wins, with delay 0.
	entry, ok := store.NpcDeathSound(store.NPC(1), nil)
	if !ok || entry != (NpcSoundEntry{Sound: 300, Delay: 0}) {
		t.Fatalf("man death = %+v ok=%v", entry, ok)
	}
	// Otherwise the captured pick.
	entry, ok = store.NpcDeathSound(store.NPC(0), nil)
	if !ok || entry.Sound != 30 {
		t.Fatalf("goblin death = %+v ok=%v", entry, ok)
	}
	// Neither: nothing.
	if _, ok := store.NpcDeathSound(store.NPC(2), nil); ok {
		t.Fatal("dragon must have no death sound")
	}
}

func TestNpcCombatSoundsMissingAndMalformed(t *testing.T) {
	store := loadSounds(t, minimalFiles(), npcCacheSpec())
	if _, ok := store.PickNpcCombatSound(0, NpcSoundHit, nil); ok {
		t.Fatal("missing file must yield no sounds")
	}
	defs, _ := newFakeCache(t, npcCacheSpec())
	dir := fixtureDir(t, withFiles(map[string]string{"npc-combat-sounds.json": `[bad`}))
	if _, err := Load(dir, LoadOptions{Cache: defs}); err == nil {
		t.Fatal("malformed npc-combat-sounds.json must fail")
	}
}
