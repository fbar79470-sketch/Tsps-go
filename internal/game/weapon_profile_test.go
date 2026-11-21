package game

import "testing"

// Fixture combat-styles JSON for the weapon-profile tests: the
// ACCURATE rows transcribed verbatim from
// data/definitions/item-combat-styles.json for every interface
// with a profile attackAnimation, plus UNARMED_PUNCH. Unit
// tests never read the shipped file.
const profileFixtureStylesJSON = `{
  "fightTypes": {
    "UNARMED_PUNCH": {"name": "Punch", "animation": 422, "childId": 0, "attackType": "CRUSH", "style": "ACCURATE", "sound": "WEAPON_UNARMED_PUNCH"},
    "SHORTBOW_ACCURATE": {"name": "Accurate", "animation": 426, "childId": 0, "attackType": "RANGE", "style": "ACCURATE", "sound": "WEAPON"},
    "LONGBOW_ACCURATE": {"name": "Accurate", "animation": 426, "childId": 0, "attackType": "RANGE", "style": "ACCURATE", "sound": "WEAPON"},
    "CROSSBOW_ACCURATE": {"name": "Accurate", "animation": 4230, "childId": 0, "attackType": "RANGE", "style": "ACCURATE", "sound": "WEAPON", "npcAnimation": 7552},
    "BLOWPIPE_ACCURATE": {"name": "Accurate", "animation": 5061, "childId": 0, "attackType": "RANGE", "style": "ACCURATE", "sound": "WEAPON"},
    "KNIFE_ACCURATE": {"name": "Accurate", "animation": 806, "childId": 0, "attackType": "RANGE", "style": "ACCURATE", "sound": "WEAPON"},
    "DART_ACCURATE": {"name": "Accurate", "animation": 806, "childId": 0, "attackType": "RANGE", "style": "ACCURATE", "sound": "WEAPON"},
    "JAVELIN_ACCURATE": {"name": "Accurate", "animation": 806, "childId": 0, "attackType": "RANGE", "style": "ACCURATE", "sound": "WEAPON"},
    "CHINCHOMPA_SHORT_FUSE": {"name": "Short fuse", "animation": 2779, "childId": 0, "attackType": "RANGE", "style": "ACCURATE", "sound": "WEAPON", "npcAnimation": 7618},
    "WHIP_FLICK": {"name": "Flick", "animation": 1658, "childId": 0, "attackType": "SLASH", "style": "ACCURATE", "sound": "WEAPON_WHIP"}
  },
  "weaponInterfaces": {
    "UNARMED": {"interfaceId": 5855, "nameLineId": 5857, "speed": 4, "category": 0, "specialBar": -1, "specialMeter": -1, "fightTypes": ["UNARMED_PUNCH"]},
    "SHORTBOW": {"interfaceId": 1764, "nameLineId": 1767, "speed": 4, "category": 3, "specialBar": 7549, "specialMeter": 7561, "fightTypes": ["SHORTBOW_ACCURATE"]},
    "LONGBOW": {"interfaceId": 1764, "nameLineId": 1767, "speed": 6, "category": 3, "specialBar": 7549, "specialMeter": 7561, "fightTypes": ["LONGBOW_ACCURATE"]},
    "CROSSBOW": {"interfaceId": 1764, "nameLineId": 1767, "speed": 6, "category": 5, "specialBar": 7549, "specialMeter": 7561, "fightTypes": ["CROSSBOW_ACCURATE"]},
    "BLOWPIPE": {"interfaceId": 1764, "nameLineId": 1767, "speed": 3, "category": 19, "specialBar": 7549, "specialMeter": 7561, "fightTypes": ["BLOWPIPE_ACCURATE"]},
    "KNIFE": {"interfaceId": 4446, "nameLineId": 4449, "speed": 3, "category": 19, "specialBar": 7649, "specialMeter": 7661, "fightTypes": ["KNIFE_ACCURATE"]},
    "DART": {"interfaceId": 4446, "nameLineId": 4449, "speed": 3, "category": 19, "specialBar": 7649, "specialMeter": 7661, "fightTypes": ["DART_ACCURATE"]},
    "JAVELIN": {"interfaceId": 4446, "nameLineId": 4449, "speed": 6, "category": 19, "specialBar": 7649, "specialMeter": 7661, "fightTypes": ["JAVELIN_ACCURATE"]},
    "CHINCHOMPA": {"interfaceId": 4446, "nameLineId": 4449, "speed": 4, "category": 7, "specialBar": -1, "specialMeter": -1, "fightTypes": ["CHINCHOMPA_SHORT_FUSE"]},
    "DARK_BOW": {"interfaceId": 1764, "nameLineId": 1767, "speed": 9, "category": 3, "specialBar": 7549, "specialMeter": 7561, "fightTypes": ["LONGBOW_ACCURATE"]},
    "WHIP": {"interfaceId": 12290, "nameLineId": 12293, "speed": 4, "category": 20, "specialBar": 12323, "specialMeter": 12335, "fightTypes": ["WHIP_FLICK"]}
  }
}`

// installProfileStyles installs the profile fixture registry
// for one test and restores the previous registry afterwards.
func installProfileStyles(t *testing.T) *CombatStyleRegistry {
	t.Helper()
	reg, err := parseCombatStyles([]byte(profileFixtureStylesJSON))
	if err != nil {
		t.Fatalf("parsing profile fixture combat styles: %v", err)
	}
	previous := InstallCombatStyles(reg)
	t.Cleanup(func() { InstallCombatStyles(previous) })
	return reg
}

// profileTestDefs is a testItemDefs extended with the weapon
// interface name and attack animation per id.
type profileTestDefs struct {
	*testItemDefs
	interfaces  map[int]string
	attackAnims map[int]int
}

func (d *profileTestDefs) WeaponInterfaceName(id int) string { return d.interfaces[id] }
func (d *profileTestDefs) WeaponAttackAnim(id int, attackType string) int {
	if anim, ok := d.attackAnims[id]; ok {
		return anim
	}
	return -1
}
func (d *profileTestDefs) WeaponAttackSpeed(id int) int { return 4 }
func (d *profileTestDefs) WeaponAttackRange(id int) int { return 6 }
func (d *profileTestDefs) WeaponBlockAnim(id int) int   { return 424 }

func TestWeaponProfileRegistryCoversAllEntries(t *testing.T) {
	// The 18 attackAnimation entries in WeaponProfile.ts
	// (WeaponProfile.ts:183, 201, 222, 233, 243, 253, 277,
	// 291, 305, 314, 322, 345, 353, 367, 395, 405, 424, 431):
	// 9 by weapon interface, and by item the crystal bows
	// (25 ids), Faerdhinen bows (11), twisted, scorching,
	// 21902, ascension, comp bows (4), 3rd age bow, black
	// chinchompa, blisterwood stake = 47 item ids.
	if got := len(weaponProfilesByWeapon); got != 9 {
		t.Fatalf("weapon profiles = %d, want 9", got)
	}
	if got := len(weaponProfilesByItem); got != 47 {
		t.Fatalf("item profiles = %d, want 47", got)
	}
	weaponCases := map[string]WeaponProfile{
		"SHORTBOW": weaponProfile(426, -1),
		"LONGBOW":  weaponProfile(426, -1),
		"CROSSBOW": weaponProfile(4230, 7552),
		"BLOWPIPE": weaponProfile(5061, -1),
		"KNIFE":    weaponProfile(806, -1),
		"DART":     weaponProfile(806, -1),
		"JAVELIN":  weaponProfile(929, -1),
		"WHIP":     weaponProfile(1658, -1),
		"DARK_BOW": weaponProfile(426, -1),
	}
	for name, want := range weaponCases {
		got := WeaponProfileForWeapon(name)
		if got == nil || *got != want {
			t.Fatalf("profile for %s = %+v, want %+v", name, got, want)
		}
	}
	if got := WeaponProfileForWeapon("HALBERD"); got != nil {
		t.Fatalf("HALBERD profile = %+v, want nil (no attackAnimation)", got)
	}
	itemCases := map[int]WeaponProfile{
		4212:  weaponProfile(426, -1), // New crystal bow.
		4214:  weaponProfile(426, -1), // Crystal bow full.
		11758: weaponProfile(426, -1), // Crystal bow 1/10 (i).
		23983: weaponProfile(426, -1), // Crystal bow.
		24123: weaponProfile(426, -1), // Crystal bow (3).
		25865: weaponProfile(426, -1), // Bow of Faerdhinen.
		33021: weaponProfile(426, -1), // Bow of Faerdhinen (c, Deadman).
		20997: weaponProfile(426, -1), // Twisted bow.
		29591: weaponProfile(426, -1), // Scorching bow.
		21902: weaponProfile(4230, 7552),
		34681: weaponProfile(15113, 15114), // Ascension crossbows.
		4827:  weaponProfile(426, -1),      // Comp ogre bow.
		10280: weaponProfile(426, -1),      // Composite bow.
		12424: weaponProfile(426, -1),      // 3rd Age bow.
		11959: weaponProfile(7618, -1),     // Black chinchompa.
		33716: weaponProfile(7617, 7617),   // Blisterwood stake.
	}
	for id, want := range itemCases {
		got := WeaponProfileForItem(id)
		if got == nil || *got != want {
			t.Fatalf("profile for item %d = %+v, want %+v", id, got, want)
		}
	}
	// Registers without an attackAnimation hold no profile:
	// the venator bow (27610) and the regular chinchompas
	// (10033) register in TS for other fields only.
	for _, id := range []int{27610, 27612, 10033, 10034, 999999} {
		if got := WeaponProfileForItem(id); got != nil {
			t.Fatalf("profile for item %d = %+v, want nil", id, got)
		}
	}
}

func TestGetAttackAnimProfileLayers(t *testing.T) {
	reg := installProfileStyles(t)
	npc := combatTestNPC(t, 1, 3001, 3000, nil)
	targetPlayer, _ := combatTestPlayer(t, "target", 3001, 3000)

	profilePlayer := func(weaponID int, iface, fightType string, attackAnims map[int]int) *Player {
		t.Helper()
		p, _ := combatTestPlayer(t, "attacker", 3000, 3000)
		p.SetItemDefinitions(&profileTestDefs{
			testItemDefs: &testItemDefs{},
			interfaces:   map[int]string{weaponID: iface},
			attackAnims:  attackAnims,
		})
		equipWeapon(p, weaponID)
		p.GetCombat().SetFightType(reg.FightType(fightType))
		return p
	}

	cases := []struct {
		name     string
		weaponID int
		iface    string
		fight    string
		anims    map[int]int
		target   *Mobile
		want     int
	}{
		// Layer 1 beats the style fallback: ascension
		// crossbows read 15113/15114, not the CROSSBOW
		// style's 4230/7552.
		{"ascension vs player", 34681, "CROSSBOW", "CROSSBOW_ACCURATE", nil, &targetPlayer.Mobile, 15113},
		{"ascension vs npc", 34681, "CROSSBOW", "CROSSBOW_ACCURATE", nil, &npc.Mobile, 15114},
		// Layer 1 beats the item def: a fake item anim 999
		// loses to the ascension profile.
		{"item profile beats item def", 34681, "CROSSBOW", "CROSSBOW_ACCURATE", map[int]int{34681: 999}, &targetPlayer.Mobile, 15113},
		// Layer 2 beats layer 3: a shortbow whose item def
		// says 999 reads 999, not the SHORTBOW profile's 426.
		{"item def beats weapon profile", 841, "SHORTBOW", "SHORTBOW_ACCURATE", map[int]int{841: 999}, &targetPlayer.Mobile, 999},
		// Layer 3 beats the fallback: a javelin-interface
		// weapon (fixture id 9999) reads the JAVELIN profile's
		// 929, not the JAVELIN style's 806.
		{"weapon profile beats fallback", 9999, "JAVELIN", "JAVELIN_ACCURATE", nil, &targetPlayer.Mobile, 929},
		// Layer 3 NPC animation: a plain crossbow reads 7552
		// against an NPC, 4230 against a player.
		{"crossbow vs npc", 9185, "CROSSBOW", "CROSSBOW_ACCURATE", nil, &npc.Mobile, 7552},
		{"crossbow vs player", 9185, "CROSSBOW", "CROSSBOW_ACCURATE", nil, &targetPlayer.Mobile, 4230},
		// Black chinchompa vs a player reads the profile's
		// 7618, not the style's 2779; against an NPC both
		// layers agree on 7618.
		{"black chinchompa vs player", 11959, "CHINCHOMPA", "CHINCHOMPA_SHORT_FUSE", nil, &targetPlayer.Mobile, 7618},
		{"black chinchompa vs npc", 11959, "CHINCHOMPA", "CHINCHOMPA_SHORT_FUSE", nil, &npc.Mobile, 7618},
		// Blisterwood stake reads 7617 either way, not the
		// DART style's 806.
		{"blisterwood vs player", 33716, "DART", "DART_ACCURATE", nil, &targetPlayer.Mobile, 7617},
		{"blisterwood vs npc", 33716, "DART", "DART_ACCURATE", nil, &npc.Mobile, 7617},
		// No profile anywhere: the whip agrees (1658 both
		// ways), the blowpipe agrees (5061), and an unarmed
		// fighter falls back to the punch (422).
		{"whip agrees", 4151, "WHIP", "WHIP_FLICK", nil, &targetPlayer.Mobile, 1658},
		{"blowpipe agrees", 12926, "BLOWPIPE", "BLOWPIPE_ACCURATE", nil, &npc.Mobile, 5061},
		{"dark bow agrees", 11235, "DARK_BOW", "LONGBOW_ACCURATE", nil, &targetPlayer.Mobile, 426},
	}
	for _, tc := range cases {
		p := profilePlayer(tc.weaponID, tc.iface, tc.fight, tc.anims)
		if got := p.GetAttackAnim(tc.target); got != tc.want {
			t.Errorf("%s: GetAttackAnim = %d, want %d", tc.name, got, tc.want)
		}
	}

	// Layer 4 alone: a fighter with no weapon at all falls
	// back to the style — unarmed punch 422.
	bare, _ := combatTestPlayer(t, "bare", 3000, 3000)
	if got := bare.GetAttackAnim(&targetPlayer.Mobile); got != 422 {
		t.Errorf("unarmed GetAttackAnim = %d, want 422", got)
	}
	// A nil target reads the player animation, not the NPC one.
	p := profilePlayer(34681, "CROSSBOW", "CROSSBOW_ACCURATE", nil)
	if got := p.GetAttackAnim(nil); got != 15113 {
		t.Errorf("nil-target GetAttackAnim = %d, want 15113", got)
	}
}
