package game

import (
	"testing"
)

// Combat sound picker tests (Phase 29, Worker C). All fixtures
// are hand-built: no cache, no network, no JSON data files.
// The sound table itself is asserted here by count + spot
// values against the TS source (Sound.ts: 153 `new Sound`
// constructions, 8 aliases, 161 named statics, 145 distinct
// synth ids — script-verified; see TestSoundTableCounts).

// soundPickerStylesJSON is a minimal style registry for the
// fire-sound tests: one fight type per ranged interface, in
// the fixtureCombatStylesJSON shape. The interface names are
// the WeaponInterfaces keys; the fight-type sounds are all
// WEAPON (the picker never reads them — the interface name
// alone decides the fire sound).
const soundPickerStylesJSON = `{
  "fightTypes": {
    "UNARMED_PUNCH": {"name": "Punch", "animation": 422, "childId": 0, "attackType": "CRUSH", "style": "ACCURATE", "sound": "WEAPON_UNARMED_PUNCH"},
    "SHORTBOW_ACCURATE": {"name": "Accurate", "animation": 426, "childId": 0, "attackType": "RANGE", "style": "ACCURATE", "sound": "WEAPON"},
    "LONGBOW_ACCURATE": {"name": "Accurate", "animation": 426, "childId": 0, "attackType": "RANGE", "style": "ACCURATE", "sound": "WEAPON"},
    "CROSSBOW_ACCURATE": {"name": "Accurate", "animation": 4230, "childId": 0, "attackType": "RANGE", "style": "ACCURATE", "sound": "WEAPON"},
    "KNIFE_ACCURATE": {"name": "Accurate", "animation": 806, "childId": 0, "attackType": "RANGE", "style": "ACCURATE", "sound": "WEAPON"},
    "DART_ACCURATE": {"name": "Accurate", "animation": 806, "childId": 0, "attackType": "RANGE", "style": "ACCURATE", "sound": "WEAPON"},
    "THROWNAXE_ACCURATE": {"name": "Accurate", "animation": 806, "childId": 0, "attackType": "RANGE", "style": "ACCURATE", "sound": "WEAPON"},
    "CHINCHOMPA_ACCURATE": {"name": "Accurate", "animation": 806, "childId": 0, "attackType": "RANGE", "style": "ACCURATE", "sound": "WEAPON"},
    "DARK_BOW_ACCURATE": {"name": "Accurate", "animation": 426, "childId": 0, "attackType": "RANGE", "style": "ACCURATE", "sound": "WEAPON"}
  },
  "weaponInterfaces": {
    "UNARMED": {"interfaceId": 5855, "nameLineId": 5857, "speed": 4, "category": 0, "specialBar": -1, "specialMeter": -1, "fightTypes": ["UNARMED_PUNCH"]},
    "SHORTBOW": {"interfaceId": 1764, "nameLineId": 1767, "speed": 4, "category": 3, "specialBar": -1, "specialMeter": -1, "fightTypes": ["SHORTBOW_ACCURATE"]},
    "LONGBOW": {"interfaceId": 1764, "nameLineId": 1767, "speed": 6, "category": 3, "specialBar": -1, "specialMeter": -1, "fightTypes": ["LONGBOW_ACCURATE"]},
    "CROSSBOW": {"interfaceId": 1764, "nameLineId": 1767, "speed": 6, "category": 5, "specialBar": -1, "specialMeter": -1, "fightTypes": ["CROSSBOW_ACCURATE"]},
    "KNIFE": {"interfaceId": 4446, "nameLineId": 4449, "speed": 3, "category": 19, "specialBar": -1, "specialMeter": -1, "fightTypes": ["KNIFE_ACCURATE"]},
    "DART": {"interfaceId": 4446, "nameLineId": 4449, "speed": 3, "category": 19, "specialBar": -1, "specialMeter": -1, "fightTypes": ["DART_ACCURATE"]},
    "THROWNAXE": {"interfaceId": 4446, "nameLineId": 4449, "speed": 5, "category": 19, "specialBar": -1, "specialMeter": -1, "fightTypes": ["THROWNAXE_ACCURATE"]},
    "CHINCHOMPA": {"interfaceId": 4446, "nameLineId": 4449, "speed": 4, "category": 19, "specialBar": -1, "specialMeter": -1, "fightTypes": ["CHINCHOMPA_ACCURATE"]},
    "DARK_BOW": {"interfaceId": 1764, "nameLineId": 1767, "speed": 9, "category": 3, "specialBar": -1, "specialMeter": -1, "fightTypes": ["DARK_BOW_ACCURATE"]}
  }
}`

// installSoundPickerStyles installs the picker registry for
// one test and restores the previous registry afterwards.
func installSoundPickerStyles(t *testing.T) {
	t.Helper()
	reg, err := parseCombatStyles([]byte(soundPickerStylesJSON))
	if err != nil {
		t.Fatalf("parsing picker combat styles: %v", err)
	}
	previous := InstallCombatStyles(reg)
	t.Cleanup(func() { InstallCombatStyles(previous) })
}

// soundPickerDefs builds item defs mapping item ids to weapon
// interface names for the picker tests.
func soundPickerDefs(interfaces map[int]string) *styleTestDefs {
	return &styleTestDefs{
		testItemDefs: &testItemDefs{slots: map[int]int{}},
		interfaces:   interfaces,
	}
}

// soundPickerPlayer builds a player with the given weapon
// equipped (0 = unarmed) under the picker registry. It binds
// the player's combat identity through the typed GetCombat
// (combatant.go: combatantFor reads the bound identity; a
// bare NewPlayer's Mobile answers IsPlayer() == false until
// a typed Process binds it, as production does on login).
func soundPickerPlayer(t *testing.T, weaponID int, interfaces map[int]string) *Player {
	t.Helper()
	installSoundPickerStyles(t)
	p, _ := stylePlayer(soundPickerDefs(interfaces))
	_ = p.GetCombat()
	if weaponID > 0 {
		equipWeapon(p, weaponID)
	}
	return p
}

func TestSpecialFireSoundsFor(t *testing.T) {
	p, _ := combatTestPlayer(t, "special", 3000, 3000)
	special := &CombatSpecial{ID: "dragon-dagger", DrainAmount: 25, ItemIDs: []int{5000}, FireSounds: []Sound{SoundDragonDaggerSpecial}}
	RegisterCombatSpecial(special)
	t.Cleanup(func() {
		combatSpecialsByItem = map[int]*CombatSpecial{}
		combatSpecialsByID = map[string]*CombatSpecial{}
	})
	p.Equipment().Set(WeaponSlot, NewItem(5000, 1))
	// Unarmed: the ordinary sound plays.
	if got := SpecialFireSoundsFor(&p.Mobile); got != nil {
		t.Errorf("unarmed special sounds = %v, want nil", got)
	}
	p.GetCombat().SetSpecialActivated(true)
	if got := SpecialFireSoundsFor(&p.Mobile); len(got) != 1 || got[0] != SoundDragonDaggerSpecial {
		t.Errorf("armed special sounds = %v, want [2537]", got)
	}
	// A special with no FireSounds falls through to the
	// ordinary sound, as a TS method that does not override
	// fireSound does.
	special.FireSounds = nil
	if got := SpecialFireSoundsFor(&p.Mobile); got != nil {
		t.Errorf("soundless special sounds = %v, want nil", got)
	}
	// Nil attacker and NPCs have no special sound.
	if got := SpecialFireSoundsFor(nil); got != nil {
		t.Errorf("nil attacker special sounds = %v, want nil", got)
	}
	npc := combatTestNPC(t, 1, 3001, 3000, nil)
	if got := SpecialFireSoundsFor(&npc.Mobile); got != nil {
		t.Errorf("npc special sounds = %v, want nil", got)
	}
}

func TestPlaySpecialFireSound(t *testing.T) {
	p, _ := combatTestPlayer(t, "special", 3000, 3000)
	sess := newSoundFakeSession()
	p.Session = sess
	claws := &CombatSpecial{ID: "dragon-claws", DrainAmount: 50, ItemIDs: []int{5001}, FireSounds: []Sound{SoundDragonClawsSpecial1, SoundDragonClawsSpecial2, SoundDragonClawsSpecial3, SoundDragonClawsSpecial4}}
	RegisterCombatSpecial(claws)
	t.Cleanup(func() {
		combatSpecialsByItem = map[int]*CombatSpecial{}
		combatSpecialsByID = map[string]*CombatSpecial{}
	})
	p.Equipment().Set(WeaponSlot, NewItem(5001, 1))
	// Unarmed: nothing plays, the caller plays the ordinary
	// sound.
	if PlaySpecialFireSound(&p.Mobile) {
		t.Error("unarmed PlaySpecialFireSound = true, want false")
	}
	if len(sess.effects) != 0 {
		t.Errorf("unarmed effects = %v, want none", sess.effects)
	}
	// Armed: all four claw sounds play, in order, each with
	// its own delay (10/30/42/60 client cycles).
	p.GetCombat().SetSpecialActivated(true)
	if !PlaySpecialFireSound(&p.Mobile) {
		t.Error("armed PlaySpecialFireSound = false, want true")
	}
	if len(sess.effects) != 4 {
		t.Fatalf("claw effects = %v, want 4", sess.effects)
	}
	for i, want := range []Sound{SoundDragonClawsSpecial1, SoundDragonClawsSpecial2, SoundDragonClawsSpecial3, SoundDragonClawsSpecial4} {
		if sess.effects[i].soundID != want.ID() || sess.effects[i].delay != want.Delay() {
			t.Errorf("claw effect %d = %+v, want id %d delay %d", i, sess.effects[i], want.ID(), want.Delay())
		}
	}
}

func TestSoundTableCounts(t *testing.T) {
	// 161 named statics (153 constructions + 8 aliases).
	if len(knownSounds) != 161 {
		t.Errorf("known sounds = %d, want 161", len(knownSounds))
	}
	// 145 distinct synth ids (several names share an id:
	// e.g. WEAPON_DAGGER_SLASH / WEAPON_SCIMITAR are both
	// 2500, WEAPON / WEAPON_WARHAMMER are both 2567).
	if len(knownSoundsByID) != 145 {
		t.Errorf("distinct sound ids = %d, want 145", len(knownSoundsByID))
	}
	// Spot-check the ids the task brief names.
	for id, want := range map[int]Sound{
		2702: SoundShootArrow,
		2720: SoundWeaponWhip,
		227:  SoundSpellFailSplash,
		394:  SoundNpcAttacking,
		3734: SoundDarkBowFire,
		6797: SoundVenatorBowShoot,
	} {
		got, ok := ResolveKnownSound(id)
		if !ok || got != want {
			t.Errorf("ResolveKnownSound(%d) = %+v/%v, want %+v", id, got, ok, want)
		}
	}
	if s, ok := ResolveKnownSound("SHOOT_ARROW"); !ok || s != SoundShootArrow {
		t.Errorf("ResolveKnownSound(SHOOT_ARROW) = %+v/%v", s, ok)
	}
}

func TestSendSoundDropsNoSound(t *testing.T) {
	p, sess := soundTestPlayer("sounder", Location{X: 3200, Y: 3200, Z: 0})
	SendSound(p, NoSound)
	if len(sess.effects) != 0 {
		t.Errorf("SendSound(NoSound) sent %d effects, want 0", len(sess.effects))
	}
	// A real sound still goes through the guard.
	SendSound(p, SoundWeaponWhip)
	if len(sess.effects) != 1 || sess.effects[0].soundID != 2720 {
		t.Errorf("SendSound(WEAPON_WHIP) effects = %v, want one 2720", sess.effects)
	}
}

func TestAttackSoundFor(t *testing.T) {
	if got := AttackSoundFor(nil); got != NoSound {
		t.Errorf("nil attacker = %+v, want NoSound", got)
	}
	npc := NewNPC(1, Location{X: 3200, Y: 3200, Z: 0})
	if got := AttackSoundFor(&npc.Mobile); got != SoundNpcAttacking {
		t.Errorf("NPC attack sound = %d, want 394 (NPC_ATTACKING)", got.ID())
	}
	// Unarmed player: the UNARMED_PUNCH fight type's sound.
	p := soundPickerPlayer(t, 0, nil)
	if got := AttackSoundFor(&p.Mobile); got != SoundWeaponUnarmedPunch {
		t.Errorf("unarmed attack sound = %d, want 2566", got.ID())
	}
	// A fight type with a set sound wins over unarmed.
	p.GetCombat().SetFightType(&FightType{Name: "SCIMITAR_SLASH", Sound: SoundWeaponScimitar})
	if got := AttackSoundFor(&p.Mobile); got != SoundWeaponScimitar {
		t.Errorf("scimitar fight type sound = %d, want 2500", got.ID())
	}
	// A fight type with no sound reads WEAPON.
	p.GetCombat().SetFightType(&FightType{Name: "SOUNDLESS"})
	if got := AttackSoundFor(&p.Mobile); got != SoundWeapon {
		t.Errorf("soundless fight type = %d, want 2567 (WEAPON)", got.ID())
	}
}

func TestAttackSoundForWhip(t *testing.T) {
	// The whip is the only WeaponProfile row with an
	// attackSound (WeaponProfile.ts:277): it beats the fight
	// type's sound.
	installFixtureStyles(t)
	p, _ := stylePlayer(whipDefs())
	_ = p.GetCombat() // bind the player identity (see soundPickerPlayer)
	equipWeapon(p, 4151)
	if got := AttackSoundFor(&p.Mobile); got != SoundWeaponWhip {
		t.Errorf("whip attack sound = %d, want 2720", got.ID())
	}
}

func TestFireSoundFor(t *testing.T) {
	if got := FireSoundFor(nil); got != NoSound {
		t.Errorf("nil attacker = %+v, want NoSound", got)
	}
	npc := NewNPC(1, Location{X: 3200, Y: 3200, Z: 0})
	if got := FireSoundFor(&npc.Mobile); got != NoSound {
		t.Errorf("NPC fire sound = %+v, want NoSound (TS returns before the sound)", got)
	}
	cases := []struct {
		name       string
		weaponID   int
		interfaces map[int]string
		want       Sound
	}{
		{"unarmed falls back to SHOOT_ARROW", 0, nil, SoundShootArrow},
		{"shortbow", 841, map[int]string{841: "SHORTBOW"}, SoundShootArrow},
		{"longbow quiet", 839, map[int]string{839: "LONGBOW"}, SoundShootBowQuiet},
		{"crossbow", 9000, map[int]string{9000: "CROSSBOW"}, SoundShootCrossbow},
		{"knife dart", 9001, map[int]string{9001: "KNIFE"}, SoundThrowDart},
		{"thrownaxe has no row, falls back", 9002, map[int]string{9002: "THROWNAXE"}, SoundShootArrow},
		{"venator item override", 27610, map[int]string{27610: "SHORTBOW"}, SoundVenatorBowShoot},
		{"venator uncharged item override", 27612, map[int]string{27612: "SHORTBOW"}, SoundVenatorBowShoot},
		{"comp ogre bow item override", 4827, map[int]string{4827: "SHORTBOW"}, SoundShootBowQuiet},
		{"3rd age bow item override", 12424, map[int]string{12424: "SHORTBOW"}, SoundShootBowQuiet},
		{"chinchompa item override", 10033, map[int]string{10033: "CHINCHOMPA"}, SoundThrowDart},
		{"blisterwood stake is DART already", 33716, map[int]string{33716: "DART"}, SoundThrowDart},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := soundPickerPlayer(t, tc.weaponID, tc.interfaces)
			if got := FireSoundFor(&p.Mobile); got != tc.want {
				t.Errorf("fire sound = %d, want %d", got.ID(), tc.want.ID())
			}
		})
	}
}

func TestFireSoundForDarkBow(t *testing.T) {
	// Two arrows: the DARK_BOW interface sound.
	p := soundPickerPlayer(t, 11235, map[int]string{11235: "DARK_BOW"})
	p.GetCombat().SetRangedWeapon(&RangedWeaponData{AmmoRequired: 2})
	p.Equipment().SetItem(AmmunitionSlot, NewItem(892, 5))
	if got := FireSoundFor(&p.Mobile); got != SoundShootBowQuiet {
		t.Errorf("dark bow two arrows = %d, want 2700", got.ID())
	}
	// One arrow left: the single-fire sound (3734).
	p.Equipment().SetItem(AmmunitionSlot, NewItem(892, 1))
	if got := FireSoundFor(&p.Mobile); got != SoundDarkBowFire {
		t.Errorf("dark bow one arrow = %d, want 3734", got.ID())
	}
}

func TestMagicSoundMaps(t *testing.T) {
	if len(spellCastSounds) != 49 {
		t.Errorf("cast sounds = %d entries, want 49", len(spellCastSounds))
	}
	if len(spellImpactSounds) != 49 {
		t.Errorf("impact sounds = %d entries, want 49", len(spellImpactSounds))
	}
	// Spot-checks against MagicCombatMethod.ts CAST_SOUNDS /
	// IMPACT_SOUNDS, incl. the id-1 rows and Undead Grasp.
	for id, sounds := range map[int][2]int{
		1:     {2540, 1460},
		1152:  {220, 221},
		1157:  {3011, 3010},
		1572:  {101, 99},
		12891: {171, 168},
		21832: {5030, 5055},
		21876: {4028, 4027},
	} {
		cast, impact := sounds[0], sounds[1]
		if spellCastSounds[id] != cast {
			t.Errorf("cast sound %d = %d, want %d", id, spellCastSounds[id], cast)
		}
		if spellImpactSounds[id] != impact {
			t.Errorf("impact sound %d = %d, want %d", id, spellImpactSounds[id], impact)
		}
	}
}

// soundTestSpell is a fakeSpell with an overridable id and
// optional own cast/impact sounds.
type soundTestSpell struct {
	*fakeSpell
	id          int
	castSound   Sound
	impactSound Sound
	hasCast     bool
	hasImpact   bool
}

func (s *soundTestSpell) SpellID() int { return s.id }
func (s *soundTestSpell) CastSound() Sound {
	if s.hasCast {
		return s.castSound
	}
	return NoSound
}
func (s *soundTestSpell) ImpactSound() Sound {
	if s.hasImpact {
		return s.impactSound
	}
	return NoSound
}

func TestCastSoundFor(t *testing.T) {
	if got := CastSoundFor(nil); got != NoSound {
		t.Errorf("nil spell = %+v, want NoSound", got)
	}
	// Wind Strike (1152) resolves through the map.
	if got := CastSoundFor(newFakeSpell()); got.ID() != 220 {
		t.Errorf("Wind Strike cast sound = %d, want 220", got.ID())
	}
	// Bind (1572) through the map.
	bind := &soundTestSpell{fakeSpell: newFakeSpell(), id: 1572}
	if got := CastSoundFor(bind); got.ID() != 101 {
		t.Errorf("Bind cast sound = %d, want 101", got.ID())
	}
	// An unknown spell id has no sound.
	unknown := &soundTestSpell{fakeSpell: newFakeSpell(), id: 9999}
	if got := CastSoundFor(unknown); got != NoSound {
		t.Errorf("unknown spell cast sound = %+v, want NoSound", got)
	}
	// A spell's own cast sound beats the map.
	own := &soundTestSpell{fakeSpell: newFakeSpell(), id: 1152, castSound: SoundWeaponWhip, hasCast: true}
	if got := CastSoundFor(own); got != SoundWeaponWhip {
		t.Errorf("own cast sound = %d, want 2720", got.ID())
	}
	// A spell whose own cast sound is NoSound falls back to
	// the map (TS null → ?? resolveCastSound).
	none := &soundTestSpell{fakeSpell: newFakeSpell(), id: 1152, hasCast: true}
	if got := CastSoundFor(none); got.ID() != 220 {
		t.Errorf("null own cast sound = %d, want 220 (map fallback)", got.ID())
	}
}

func TestImpactSoundFor(t *testing.T) {
	if got := ImpactSoundFor(nil); got != NoSound {
		t.Errorf("nil spell = %+v, want NoSound", got)
	}
	if got := ImpactSoundFor(newFakeSpell()); got.ID() != 221 {
		t.Errorf("Wind Strike impact sound = %d, want 221", got.ID())
	}
	bind := &soundTestSpell{fakeSpell: newFakeSpell(), id: 1572}
	if got := ImpactSoundFor(bind); got.ID() != 99 {
		t.Errorf("Bind impact sound = %d, want 99", got.ID())
	}
	unknown := &soundTestSpell{fakeSpell: newFakeSpell(), id: 9999}
	if got := ImpactSoundFor(unknown); got != NoSound {
		t.Errorf("unknown spell impact sound = %+v, want NoSound", got)
	}
	own := &soundTestSpell{fakeSpell: newFakeSpell(), id: 1152, impactSound: SoundIceBarrageImpact, hasImpact: true}
	if got := ImpactSoundFor(own); got != SoundIceBarrageImpact {
		t.Errorf("own impact sound = %d, want 168", got.ID())
	}
	none := &soundTestSpell{fakeSpell: newFakeSpell(), id: 1152, hasImpact: true}
	if got := ImpactSoundFor(none); got.ID() != 221 {
		t.Errorf("null own impact sound = %d, want 221 (map fallback)", got.ID())
	}
	// The fail splash is a table constant, not a picker
	// result: it is what Worker B sends on a miss.
	if SoundSpellFailSplash.ID() != 227 {
		t.Errorf("SPELL_FAIL_SPLASH = %d, want 227", SoundSpellFailSplash.ID())
	}
	if MagicImpactSoundRadius != 10 {
		t.Errorf("MagicImpactSoundRadius = %d, want 10", MagicImpactSoundRadius)
	}
}

// deathSoundFakeDef is a fakeNPCDefinition carrying a death
// sound, for DeathSoundFor tests.
type deathSoundFakeDef struct {
	*fakeNPCDefinition
	deathSound int
}

func (d *deathSoundFakeDef) DeathSound() int { return d.deathSound }

func TestDeathSoundFor(t *testing.T) {
	if got := DeathSoundFor(nil); got != NoSound {
		t.Errorf("nil npc = %+v, want NoSound", got)
	}
	// No definition installed: the DEFAULT shape has no
	// death sound.
	npc := NewNPC(1, Location{X: 3200, Y: 3200, Z: 0})
	if got := DeathSoundFor(npc); got != NoSound {
		t.Errorf("default definition = %+v, want NoSound", got)
	}
	// A definition death sound resolves with volume 1,
	// delay 0, loopType 0 (NpcCombatSounds.death: [own, 0]).
	npc.SetDefinition(&deathSoundFakeDef{fakeNPCDefinition: &fakeNPCDefinition{}, deathSound: 1234})
	if got := DeathSoundFor(npc); got.ID() != 1234 || got.Volume() != 1 || got.Delay() != 0 || got.LoopType() != 0 {
		t.Errorf("death sound = %d/%d/%d/%d, want 1234/1/0/0", got.ID(), got.Volume(), got.Delay(), got.LoopType())
	}
	// -1 (the NpcDefinition default) and 0 play nothing.
	npc.SetDefinition(&deathSoundFakeDef{fakeNPCDefinition: &fakeNPCDefinition{}, deathSound: -1})
	if got := DeathSoundFor(npc); got != NoSound {
		t.Errorf("death sound -1 = %+v, want NoSound", got)
	}
	npc.SetDefinition(&deathSoundFakeDef{fakeNPCDefinition: &fakeNPCDefinition{}, deathSound: 0})
	if got := DeathSoundFor(npc); got != NoSound {
		t.Errorf("death sound 0 = %+v, want NoSound", got)
	}
}
