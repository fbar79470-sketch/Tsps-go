package definition

import (
	"path/filepath"
	"regexp"
	"strconv"
)

// loadExaminesAndSounds loads the files TypeScript reads lazily
// on first use: npc-examines.json (NpcDefinition.getExamine),
// object-examines.json + object-examine-overrides.json
// (ObjectDefinition.examineText), equipment-sounds.json
// (EquipmentSounds.load) and npc-combat-sounds.json
// (NpcCombatSounds.load). In TS each read throws when its file is
// missing, at first use; here they are read eagerly at Load and a
// missing file degrades to empty data with a warning, so a
// fixture definitions directory boots and the wrappers fall back
// to their generic text and defaults. A present but malformed
// file fails Load, as the TS parse would throw. This is a
// deliberate deviation, documented here and in the package
// comment.
func (s *Store) loadExaminesAndSounds() error {
	npcExamines, err := readOptionalStringMap(filepath.Join(s.dir, "npc-examines.json"), "npc-examines.json")
	if err != nil {
		return err
	}
	s.npcExamines = npcExamines

	objectExamines, err := readOptionalStringMap(filepath.Join(s.dir, "object-examines.json"), "object-examines.json")
	if err != nil {
		return err
	}
	overrides, err := readOptionalStringMap(filepath.Join(s.dir, "object-examine-overrides.json"), "object-examine-overrides.json")
	if err != nil {
		return err
	}
	// The overrides spread over the dump: recorded live text wins
	// over the Wiki's (ObjectDefinition.examineText).
	for id, text := range overrides {
		objectExamines[id] = text
	}
	s.objectExamines = objectExamines

	sounds, err := s.loadEquipmentSounds()
	if err != nil {
		return err
	}
	s.equipmentSounds = sounds

	combatSounds, err := s.loadNpcCombatSounds()
	if err != nil {
		return err
	}
	s.npcCombatSounds = combatSounds
	return nil
}

// --- Equipment sounds --------------------------------------------

// equipmentSlotNames is SLOT_NAMES (EquipmentSounds.ts:17-20):
// equipment slots by the names equipment-sounds.json uses.
var equipmentSlotNames = map[int]string{
	0: "head", 1: "cape", 2: "neck", 3: "weapon", 4: "body", 5: "shield",
	7: "legs", 9: "hands", 10: "feet", 12: "ring", 13: "ammo",
}

// soundRule is one {name?, sound} rule: the first whose name
// pattern matches the item name wins (no pattern matches any
// name). A rule whose pattern does not compile never matches —
// TypeScript throws on first use of such a pattern; here the
// failure is warned at Load and the rule is skipped, so a bad
// pattern cannot fail a boot.
type soundRule struct {
	matchAll bool
	pattern  *regexp.Regexp
	sound    *int
}

// soundRules is SoundRules (EquipmentSounds.ts:7): a single sound,
// or rules tried in order.
type soundRules struct {
	direct *int
	rules  []soundRule
}

// equipmentSoundData is EquipmentSoundData
// (EquipmentSounds.ts:9-13).
type equipmentSoundData struct {
	def         int
	weaponTypes map[string]soundRules
	slots       map[string]soundRules
}

// loadEquipmentSounds reads equipment-sounds.json. A missing file
// warns and yields empty data (default 0); a malformed file fails
// Load.
func (s *Store) loadEquipmentSounds() (*equipmentSoundData, error) {
	data := &equipmentSoundData{
		weaponTypes: make(map[string]soundRules),
		slots:       make(map[string]soundRules),
	}
	value, err := readJSONValueOptional(filepath.Join(s.dir, "equipment-sounds.json"))
	if err != nil {
		return nil, err
	}
	if value == nil {
		s.logf("[equipment-sounds] equipment-sounds.json missing - equipment sounds will fall back to defaults")
		return data, nil
	}
	if v, ok := finiteInt(mustNum(value, "default")); ok {
		data.def = v
	}
	for _, section := range []struct {
		key string
		dst map[string]soundRules
	}{
		{"weaponTypes", data.weaponTypes},
		{"slots", data.slots},
	} {
		entries := objField(value, section.key)
		for name, raw := range entries {
			rules, ok := parseSoundRules(raw, s.logf)
			if ok {
				section.dst[name] = rules
			}
		}
	}
	return data, nil
}

// parseSoundRules parses one SoundRules value: a number, or an
// array of {name?, sound} rules. Any other shape is not rules at
// all (TypeScript would throw calling .find on it at lookup); it
// is skipped with the rules reported absent.
func parseSoundRules(raw any, logf func(format string, args ...any)) (soundRules, bool) {
	if v, ok := finiteInt(raw); ok {
		return soundRules{direct: &v}, true
	}
	arr, ok := raw.([]any)
	if !ok {
		return soundRules{}, false
	}
	var out soundRules
	for _, element := range arr {
		var rule soundRule
		nameValue, present := field(element, "name")
		if !present || !jsTruthy(nameValue) {
			rule.matchAll = true
		} else {
			source := jsString(nameValue, true)
			pattern, err := regexp.Compile("(?i)" + source)
			if err != nil {
				logf("[equipment-sounds] invalid sound pattern %q: %v", source, err)
				continue
			}
			rule.pattern = pattern
		}
		if v, ok := finiteInt(mustNum(element, "sound")); ok {
			rule.sound = &v
		}
		out.rules = append(out.rules, rule)
	}
	return out, true
}

// applySoundRules is EquipmentSounds.apply
// (EquipmentSounds.ts:42-46): a number returns itself (even 0 —
// ?? only falls through on undefined); a list returns the first
// matching rule's sound, or nothing when no rule matches. A
// matching rule with no usable sound also yields nothing, but
// stops the search, as the TS ?.sound does.
func applySoundRules(rules soundRules, present bool, name string) (int, bool) {
	if !present {
		return 0, false
	}
	if rules.direct != nil {
		return *rules.direct, true
	}
	for _, rule := range rules.rules {
		if !rule.matchAll && (rule.pattern == nil || !rule.pattern.MatchString(name)) {
			continue
		}
		if rule.sound == nil {
			return 0, false
		}
		return *rule.sound, true
	}
	return 0, false
}

// EquipmentSoundID returns the sound for wearing or removing an
// item (EquipmentSounds.soundIdFor, EquipmentSounds.ts:28-37):
// the item's own equipSound when > 0, else its weapon type's
// rules (only when worn in the weapon slot), else its slot's
// rules, else the default. The TS soundFor wraps this id in a
// Sound for playback; Sound construction and playback belong to
// the consumer, which has the player and the packet sender.
func (s *Store) EquipmentSoundID(def *ItemDefinition) int {
	if own := def.EquipSound(); own > 0 {
		return own
	}
	data := s.equipmentSounds
	if data == nil {
		return 0
	}
	name := def.Name()
	slotName, hasSlot := equipmentSlotNames[def.EquipmentSlot()]
	weaponType := def.WeaponInterfaceName()
	if hasSlot && slotName == "weapon" && weaponType != "" {
		if rules, ok := data.weaponTypes[weaponType]; ok {
			if id, found := applySoundRules(rules, true, name); found {
				return id
			}
		}
	}
	if hasSlot {
		if rules, ok := data.slots[slotName]; ok {
			if id, found := applySoundRules(rules, true, name); found {
				return id
			}
		}
	}
	return data.def
}

// --- NPC combat sounds -------------------------------------------

// NpcSoundKinds are the keyof NpcSounds kinds
// (NpcCombatSounds.ts:10-14).
const (
	NpcSoundHit    = "hit"
	NpcSoundAttack = "attack"
	NpcSoundDeath  = "death"
)

// NpcSoundEntry is a SoundEntry (NpcCombatSounds.ts:7): a sound
// and its delay in client ticks (20 ms each).
type NpcSoundEntry struct {
	Sound int
	Delay int
}

// npcSounds is NpcSounds (NpcCombatSounds.ts:10-14).
type npcSounds struct {
	hit    []NpcSoundEntry
	attack []NpcSoundEntry
	death  []NpcSoundEntry
}

// loadNpcCombatSounds reads npc-combat-sounds.json ({npcs: {id:
// {hit?, attack?, death?}}}). A missing file yields no sounds; a
// malformed file fails Load. Entries that are not [sound, delay]
// number pairs are skipped; TypeScript would carry them to a
// playback that reads undefined fields.
func (s *Store) loadNpcCombatSounds() (map[int]npcSounds, error) {
	out := make(map[int]npcSounds)
	value, err := readJSONValueOptional(filepath.Join(s.dir, "npc-combat-sounds.json"))
	if err != nil {
		return nil, err
	}
	if value == nil {
		return out, nil
	}
	npcs := objField(value, "npcs")
	for key, raw := range npcs {
		id, err := strconv.Atoi(key)
		if err != nil || strconv.Itoa(id) != key {
			continue
		}
		var sounds npcSounds
		sounds.hit = parseNpcSoundEntries(mustNum(raw, "hit"))
		sounds.attack = parseNpcSoundEntries(mustNum(raw, "attack"))
		sounds.death = parseNpcSoundEntries(mustNum(raw, "death"))
		out[id] = sounds
	}
	return out, nil
}

func parseNpcSoundEntries(raw any) []NpcSoundEntry {
	arr, ok := raw.([]any)
	if !ok {
		return nil
	}
	var entries []NpcSoundEntry
	for _, element := range arr {
		pair, ok := element.([]any)
		if !ok || len(pair) < 2 {
			continue
		}
		sound, okSound := finiteInt(pair[0])
		delay, okDelay := finiteInt(pair[1])
		if !okSound || !okDelay {
			continue
		}
		entries = append(entries, NpcSoundEntry{Sound: sound, Delay: delay})
	}
	return entries
}

// NpcCombatSoundEntries returns the captured sound entries for an
// NPC id and kind ("hit", "attack", "death"), or nil.
func (s *Store) NpcCombatSoundEntries(id int, kind string) []NpcSoundEntry {
	sounds, ok := s.npcCombatSounds[id]
	if !ok {
		return nil
	}
	var entries []NpcSoundEntry
	switch kind {
	case NpcSoundHit:
		entries = sounds.hit
	case NpcSoundAttack:
		entries = sounds.attack
	case NpcSoundDeath:
		entries = sounds.death
	default:
		return nil
	}
	return append([]NpcSoundEntry(nil), entries...)
}

// PickNpcCombatSound picks one captured entry for an NPC id and
// kind (NpcCombatSounds.pick, NpcCombatSounds.ts:52-56): no entry
// when there are none, the single entry when there is one, else
// randIntn(len) — the caller's random source, standing in for
// Math.random. A nil randIntn picks the first entry. Playback
// (Sounds.sendSound, with the entry's delay) belongs to the
// consumer.
func (s *Store) PickNpcCombatSound(id int, kind string, randIntn func(n int) int) (NpcSoundEntry, bool) {
	entries := s.NpcCombatSoundEntries(id, kind)
	if len(entries) == 0 {
		return NpcSoundEntry{}, false
	}
	if len(entries) == 1 || randIntn == nil {
		return entries[0], true
	}
	index := randIntn(len(entries))
	if index < 0 || index >= len(entries) {
		return NpcSoundEntry{}, false
	}
	return entries[index], true
}

// NpcDeathSound resolves an NPC's death sound
// (NpcCombatSounds.death, NpcCombatSounds.ts:42-48): the
// definition's own death sound (npc-combat-defs.json
// sounds.death) wins when > 0, with delay 0; otherwise a captured
// death entry is picked. Pass the NPC's current definition (after
// any morph), as TS reads getCurrentDefinition. It returns false
// when the NPC has no death sound at all.
func (s *Store) NpcDeathSound(def *NpcDefinition, randIntn func(n int) int) (NpcSoundEntry, bool) {
	if own := def.DeathSound(); own > 0 {
		return NpcSoundEntry{Sound: own, Delay: 0}, true
	}
	return s.PickNpcCombatSound(def.ID(), NpcSoundDeath, randIntn)
}
