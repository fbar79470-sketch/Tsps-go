package game

// Combat sound pickers: the port of the sound-selection half of
// Phase 29 ("Sounds" in PHASE29_PLAN.md). The Sound table itself
// is NOT here: Sound.ts + Sounds.ts were already ported in
// Phase 21 and live in sounds.go — script-verified against the
// TS source (153 `new Sound` constructions in identical
// declaration order, 8 aliases, 161 named statics; see
// TestSoundTableCounts). No sound_table.go is created: it would
// duplicate those 161 variables.
//
// What this file owns:
//   - the five pickers Worker B calls (one call per combat
//     call site): AttackSoundFor, FireSoundFor, CastSoundFor,
//     ImpactSoundFor, DeathSoundFor;
//   - SpecialFireSoundsFor / PlaySpecialFireSound, the
//     special-attack fire-sound seam (Worker C, phase 30):
//     the armed special's own fire sounds, played instead of
//     the ordinary attack/fire sound;
//   - the magic cast/impact sound maps (MagicCombatMethod.ts
//     CAST_SOUNDS / IMPACT_SOUNDS), script-extracted verbatim;
//   - the ranged fire-sound tables (WeaponProfile.ts rows);
//   - MagicImpactSoundRadius, the area-sound radius B passes
//     for magic impacts and the fail splash.
//
// NO-SOUND SENTINEL. TS pickers return null when there is no
// sound (castSoundFor / resolveCastSound / resolveImpactSound
// return null; RangedCombatMethod returns before the sound for
// NPCs). Go has no null Sound, so the pickers return NoSound
// (the zero Sound, id 0). Callers MUST skip a picker result
// with ID() <= 0: SendSound drops such a sound (the TS
// `if (!sound) return` guard, Sounds.ts:45-47), and
// PlayAreaSound drops id <= 0 (Sounds.ts:108). Never send a
// NoSound through SendSoundEffect directly — that path is
// unguarded in TS and in Go.

// NoSound is the zero Sound: no sound to play. It is the Go
// form of a null Sound return in TS (MagicCombatMethod.ts
// castSoundFor / resolveCastSound / resolveImpactSound return
// null; RangedCombatMethod.ts:155-169 returns before the
// sound for NPCs). Its id is 0, which PlayAreaSound refuses
// (Sounds.ts:108) and SendSound refuses (see sounds.go).
var NoSound Sound

// MagicImpactSoundRadius is IMPACT_SOUND_RADIUS
// (MagicCombatMethod.ts:51): live plays a spell's impact —
// and its splash — as an area sound this far round the
// target's tile. Worker B passes it to PlayAreaSoundAt for
// both the accurate impact sound and the fail splash.
const MagicImpactSoundRadius = 10

// spellCastSounder is implemented by a CombatSpell with its
// own cast sound (CombatSpell.castSound, CombatSpell.ts:144-146;
// CombatNormalSpell.ts:54-55 returns the options' cast sound
// or null). A spell that does not implement it falls back to
// the spell-id map, exactly as TS falls through with ??.
type spellCastSounder interface {
	// CastSound returns the spell's own cast sound, or
	// NoSound when it has none (TS null).
	CastSound() Sound
}

// spellImpactSounder is implemented by a CombatSpell with its
// own impact sound (CombatSpell.impactSound, CombatSpell.ts:
// 139-141; CombatNormalSpell.ts:57-58). A spell that does
// not implement it falls back to the spell-id map.
type spellImpactSounder interface {
	// ImpactSound returns the spell's own impact sound, or
	// NoSound when it has none (TS null).
	ImpactSound() Sound
}

// npcDeathSounder is the death-sound half of an NPC
// definition (NpcDefinition.getDeathSound,
// NpcDefinition.ts:243-245). It is asserted, not required:
// the game-level NPCDefinition interface (npc.go) carries no
// death sound, and the DEFAULT shape has none. The
// Store-backed *definition.NpcDefinition implements it.
type npcDeathSounder interface {
	DeathSound() int
}

// AttackSoundFor is the melee attack sound (MeleeCombatMethod
// .start, MeleeCombatMethod.ts:9-14, which plays it only when
// the attack animation is not -1 — Worker B owns that gate;
// this picker only resolves the sound):
//   - NPC: Sound.NPC_ATTACKING, always (NPC.getAttackSound,
//     NPC.ts:681-683).
//   - Player: the equipped weapon's profile attackSound, else
//     the fight type's attack sound (Player.getAttackSound,
//     Player.ts:339-341). The only WeaponProfile row carrying
//     an attackSound is the whip's (WeaponProfile.ts:277,
//     WEAPON_WHIP); every other profile falls through to the
//     fight type, so the check is the weapon interface name,
//     resolved the same way CurrentWeaponInterface resolves
//     it. A fight type with no sound reads WEAPON, as the
//     style loader does for an unknown sound name
//     (combat_styles.go).
//
// A nil attacker has no sound.
func AttackSoundFor(attacker *Mobile) Sound {
	if attacker == nil {
		return NoSound
	}
	if !attacker.IsPlayer() {
		return SoundNpcAttacking
	}
	player := attacker.AsPlayer()
	if player == nil {
		return SoundNpcAttacking
	}
	if iface := CurrentWeaponInterface(player); iface != nil && iface.Name == "WHIP" {
		return SoundWeaponWhip
	}
	if fightType := player.GetCombat().FightType(); fightType != nil && fightType.Sound.ID() > 0 {
		return fightType.Sound
	}
	return SoundWeapon
}

// rangedFireSoundByInterface is the WeaponProfile.ts
// interface-row fireSound, by WeaponInterfaces name. Rows
// with no fireSound (THROWNAXE :269-275, HALBERD :276,
// SALAMANDER :426) are absent: the caller falls through to
// SHOOT_ARROW, exactly as RangedCombatMethod.ts:182-183
// falls through with `profileSound ?? Sound.SHOOT_ARROW`.
var rangedFireSoundByInterface = map[string]Sound{
	// SHORTBOW (WeaponProfile.ts:182-191): the bow row.
	"SHORTBOW": SoundShootArrow,
	// LONGBOW (WeaponProfile.ts:192-198).
	"LONGBOW": SoundShootBowQuiet,
	// CROSSBOW (WeaponProfile.ts:199-210).
	"CROSSBOW": SoundShootCrossbow,
	// KARILS_CROSSBOW (WeaponProfile.ts:211-219).
	"KARILS_CROSSBOW": SoundShootCrossbow,
	// BLOWPIPE (WeaponProfile.ts:220-230).
	"BLOWPIPE": SoundThrowDart,
	// KNIFE (WeaponProfile.ts:231-240).
	"KNIFE": SoundThrowDart,
	// DART (WeaponProfile.ts:241-250).
	"DART": SoundThrowDart,
	// JAVELIN (WeaponProfile.ts:251-260).
	"JAVELIN": SoundThrowDart,
	// OBBY_RINGS (WeaponProfile.ts:261-268).
	"OBBY_RINGS": SoundThrowDart,
	// BALLISTA (WeaponProfile.ts:278-289).
	"BALLISTA": SoundShootCrossbow,
	// DARK_BOW (WeaponProfile.ts:290-301): its double shot
	// still fires on the quiet bow sound; the single-arrow
	// case is SoundDarkBowFire, resolved in FireSoundFor.
	"DARK_BOW": SoundShootBowQuiet,
}

// rangedFireSoundByItem is the per-item WeaponProfile.ts
// register() fireSound, by weapon item id — the byItem half
// of WeaponProfiles.get (WeaponProfile.ts:89-94), which
// spreads over the interface row. Only registers whose sound
// differs from the interface row's are listed; the rest
// (crystal bow, Bow of Faerdhinen, twisted bow, scorching
// bow, 21902, ascension crossbows, blisterwood stake) equal
// their interface sound and need no entry.
var rangedFireSoundByItem = map[int]Sound{
	// Venator bow (WeaponProfile.ts:333-342): 6797, where
	// its SHORTBOW interface would give SHOOT_ARROW. The
	// echo venator (30434/30436) is NOT registered in TS
	// and is deliberately absent here.
	27610: SoundVenatorBowShoot,
	27612: SoundVenatorBowShoot,
	// Comp ogre bow + composite bows (WeaponProfile.ts:
	// 393-402): the quiet bow sound, where SHORTBOW
	// would give SHOOT_ARROW.
	4827:  SoundShootBowQuiet,
	10280: SoundShootBowQuiet,
	10282: SoundShootBowQuiet,
	10284: SoundShootBowQuiet,
	// 3rd Age bow (WeaponProfile.ts:403-412): likewise.
	12424: SoundShootBowQuiet,
	// Chinchompas (WeaponProfile.ts:413-424): the dart
	// sound; their CHINCHOMPA interface has no profile
	// row, so without this entry they would fall through
	// to SHOOT_ARROW.
	10033: SoundThrowDart,
	10034: SoundThrowDart,
	11959: SoundThrowDart,
}

// FireSoundFor is the ranged fire sound (RangedCombatMethod
// .start, RangedCombatMethod.ts:182-183):
// `arrows < 2 && profile?.singleFireSound ? profile.singleFireSound : profile?.fireSound`,
// then `this.fireSound(character) ?? profileSound ??
// Sound.SHOOT_ARROW`. The base method's fireSound() returns
// undefined (RangedCombatMethod.ts:38-40); a special attack's
// own launch sound overrides it — that override is the
// SpecialFireSoundsFor seam below, and the ranged Start call
// site asks it first, so this picker is the base path only.
//
// Resolution, in TS order: a dark bow loosing one arrow plays
// its singleFireSound (DARK_BOW_FIRE, 3734); the dark bow is
// the only WeaponProfile row with singleFireSound
// (WeaponProfile.ts:299) and the only weapon with
// AmmoRequired 2 (ranged_data.go), so AmmoRequired > 1 with
// fewer than 2 arrows is the exact test. Otherwise the
// per-item register wins over the interface row, which wins
// over SHOOT_ARROW.
//
// NPCs play NO ranged fire sound in TS: RangedCombatMethod
// start() returns early for NPCs (RangedCombatMethod.ts:
// 155-169) before the sound line, so an NPC attacker returns
// NoSound and Worker B skips the send. A nil attacker has no
// sound.
func FireSoundFor(attacker *Mobile) Sound {
	if attacker == nil || !attacker.IsPlayer() {
		return NoSound
	}
	player := attacker.AsPlayer()
	if player == nil {
		return NoSound
	}
	if weapon := player.GetCombat().RangedWeapon(); weapon != nil && weapon.AmmoRequired > 1 && arrowsPerShot(player) < 2 {
		return SoundDarkBowFire
	}
	if weapon := player.Equipment(); weapon != nil {
		if sound, ok := rangedFireSoundByItem[weapon.GetWeapon().ID()]; ok {
			return sound
		}
	}
	if iface := CurrentWeaponInterface(player); iface != nil {
		if sound, ok := rangedFireSoundByInterface[iface.Name]; ok {
			return sound
		}
	}
	return SoundShootArrow
}

// SpecialFireSoundsFor is the special-attack fire sound
// (phase 30, item 5): the armed special's own sounds, played
// instead of the weapon's ordinary attack/fire sound on a
// special swing.
//
// TS ground: a special's combat method owns its sound — a
// ranged special overrides fireSound() (RangedCombatMethod
// .ts:38-40 base returns undefined; :182-183 plays
// `this.fireSound(character) ?? profileSound ??
// Sound.SHOOT_ARROW`, so the special's sound wins), and a
// melee special overrides start() to play its sound instead
// of getAttackSound (MeleeCombatMethod.ts:9-14). The plugin
// methods are not in the sparse checkout, so Go carries the
// sounds on the CombatSpecial definition (FireSounds) and
// this picker reads them. The gate mirrors
// CombatFactory.getMethod (CombatFactory.ts:159-163): the
// special owns the swing only while armed, so an unarmed
// special, a fighter with no registered special, an NPC
// (GetCombatSpecial is player-only), and a nil attacker all
// answer nil and the ordinary sound plays. A special with no
// FireSounds likewise answers nil: a TS method that does not
// override fireSound falls through to the profile sound.
//
// The returned slice is the definition's own: treat it as
// read-only, as callers do Traits.
func SpecialFireSoundsFor(attacker *Mobile) []Sound {
	if attacker == nil || !attacker.IsPlayer() {
		return nil
	}
	combat := attacker.GetCombat()
	if combat == nil || !combat.IsSpecialActivated() {
		return nil
	}
	special := combat.GetCombatSpecial()
	if special == nil || len(special.FireSounds) == 0 {
		return nil
	}
	return special.FireSounds
}

// PlaySpecialFireSound plays the armed special's fire sounds
// for a swing (SpecialFireSoundsFor), through sendCombatSound
// (a player hears a private synth; anything else becomes an
// area sound). It reports whether any sound played, so the
// call site can skip the ordinary sound: true means the
// special sound replaced it, false means the caller plays the
// ordinary attack/fire sound. Sounds with ID() <= 0 are
// skipped by sendCombatSound and do not count. A nil
// attacker plays nothing. Multi-sound specials send every
// sound at swing start; each Sound carries its own
// client-cycle delay, so the sequence lands on the TS timing
// (dragon claws 1-4 at delays 10/30/42/60, Sound.ts:121-124).
func PlaySpecialFireSound(attacker *Mobile) bool {
	sounds := SpecialFireSoundsFor(attacker)
	played := false
	for _, sound := range sounds {
		if sound.ID() <= 0 {
			continue
		}
		sendCombatSound(attacker, sound)
		played = true
	}
	return played
}

// spellCastSounds is CAST_SOUNDS (MagicCombatMethod.ts:
// 34-50), script-extracted verbatim: the cast sound id by
// spell id. Spell id 1 is TS's entry for the id-1 spell; no
// Go spell carries id 1 today, so the entry is inert until
// one does. The surge rows (21876-21879) are the OSRS Wiki
// list of sound ids, as the TS comment records.
var spellCastSounds = map[int]int{
	1:    2540,
	1152: 220, 1153: 119, 1154: 211, 1156: 132, 1157: 3011, 1158: 160,
	1160: 218, 1161: 127, 1163: 209, 1166: 130, 1169: 157, 1171: 122,
	1172: 216, 1175: 207, 1177: 128, 1181: 155, 1183: 222, 1185: 213,
	1188: 134, 1189: 162, 1542: 3009, 1543: 148, 1562: 3004, 1572: 101,
	1582: 3003, 1592: 151, 12037: 1718,
	12861: 6589, 12871: 6589, 12881: 6589, 12891: 171,
	12901: 6589, 12911: 6589, 12919: 6589, 12929: 6589, 12939: 6589,
	12951: 6589, 12963: 6589, 12975: 6589, 12987: 6589, 12999: 6589,
	13011: 6589, 13023: 6589,
	// Capture: Undead Grasp.
	21832: 5030,
	// OSRS Wiki List of sound IDs, Surge casts.
	21876: 4028, 21877: 4030, 21878: 4025, 21879: 4032,
}

// spellImpactSounds is IMPACT_SOUNDS (MagicCombatMethod.ts:
// 52-68), script-extracted verbatim: the impact sound id by
// spell id. The surge rows (21876-21879) are the OSRS Wiki
// list of sound ids, as the TS comment records.
var spellImpactSounds = map[int]int{
	1:    1460,
	1152: 221, 1153: 121, 1154: 212, 1156: 133, 1157: 3010, 1158: 161,
	1160: 219, 1161: 126, 1163: 210, 1166: 131, 1169: 158, 1171: 124,
	1172: 217, 1175: 208, 1177: 129, 1181: 156, 1183: 223, 1185: 214,
	1188: 135, 1189: 163, 1542: 3008, 1543: 150, 1562: 3005, 1572: 99,
	1582: 3002, 1592: 153, 12037: 174,
	12861: 173, 12871: 169, 12881: 170, 12891: 168,
	12901: 110, 12911: 104, 12919: 105, 12929: 102, 12939: 185,
	12951: 181, 12963: 182, 12975: 180, 12987: 179, 12999: 176,
	13011: 177, 13023: 175,
	// Capture: Undead Grasp (an area sound at the target).
	21832: 5055,
	// OSRS Wiki List of sound IDs, Surge impacts.
	21876: 4027, 21877: 4029, 21878: 4026, 21879: 4031,
}

// CastSoundFor is the magic cast sound (MagicCombatMethod
// .start, MagicCombatMethod.ts:278-281): `spell.castSound()
// ?? resolveCastSound(spellId)`, sent with Sounds.sendSound
// (private synth for a player caster, area sound at an NPC
// caster's tile) and skipped when null. The map lookup
// builds a Sound with volume 1, delay 0, loopType 0, as
// resolveCastSound does (MagicCombatMethod.ts:303-306: `new
// Sound(soundId, 1, 0, 0)`). A nil spell has no sound.
func CastSoundFor(spell CombatSpell) Sound {
	if spell == nil {
		return NoSound
	}
	if sounder, ok := spell.(spellCastSounder); ok {
		if sound := sounder.CastSound(); sound.ID() > 0 {
			return sound
		}
	}
	if id, ok := spellCastSounds[spell.SpellID()]; ok {
		return NewSound(id, 1, 0, 0)
	}
	return NoSound
}

// ImpactSoundFor is the magic impact sound for an ACCURATE
// hit (MagicCombatMethod.handleAfterHitEffects,
// MagicCombatMethod.ts:358-372): `spell.impactSound() ??
// resolveImpactSound(spellId)`, played as an area sound at
// the target's tile with MagicImpactSoundRadius — a private
// synth never reached the caster, and a range-0 area sound
// is dropped by the client (the TS comment at :363-366).
// Worker B calls this picker for the accurate case only.
//
// THE SPLASH CASE does not go through this picker. On an
// inaccurate hit TS plays Sound.SPELL_FAIL_SPLASH (227) as
// an area sound at the target with the same radius
// (MagicCombatMethod.ts:371-373: `Sounds.playAreaSoundAt
// (target, Sound.SPELL_FAIL_SPLASH, MagicCombatMethod.
// IMPACT_SOUND_RADIUS)`), i.e. Worker B calls
// `PlayAreaSoundAt(target, SoundSpellFailSplash,
// MagicImpactSoundRadius)` directly. A nil spell has no
// sound.
func ImpactSoundFor(spell CombatSpell) Sound {
	if spell == nil {
		return NoSound
	}
	if sounder, ok := spell.(spellImpactSounder); ok {
		if sound := sounder.ImpactSound(); sound.ID() > 0 {
			return sound
		}
	}
	if id, ok := spellImpactSounds[spell.SpellID()]; ok {
		return NewSound(id, 1, 0, 0)
	}
	return NoSound
}

// DeathSoundFor is the NPC death sound (NpcCombatSounds.death,
// NpcCombatSounds.ts:42-48, called from NPCDeathTask case 1
// alongside the death animation, NPCDeathTask.ts:44-45): the
// definition's own death sound (NpcDefinition.getDeathSound,
// NpcDefinition.ts:243-245) wins when > 0, with delay 0 —
// in TS `animation.sounds?.death` from npc-animations.json,
// falling back to the cache-decoded definition
// (NpcDefinitionLoader.ts:481). A definition without one
// returns NoSound.
//
// THE CAPTURED FALLBACK IS NOT HERE. TS falls back to a
// captured death entry from npc-combat-sounds.json
// (NpcCombatSounds.pick) when the definition has none. That
// file is shipped in the repo since 2026-11-19 (Tyler-approved
// verbatim import, SHA-256 verified; the definition Store
// loads it — see NpcCombatSoundEntries), but its NpcDeathSound
// has no game-level seam (no game or cmd caller exists), so
// this picker is definition-only until a coordinator wires it.
// In the current repo the definition sound is also absent for
// every NPC (npc-animations.json is a `{}` starter), so
// DeathSoundFor returns NoSound in practice — the honest,
// TS-faithful result for the data state, not a stub: TS
// would likewise play nothing.
//
// WORKER B CALL CONTRACT. TS sends the death sound as a
// private synth to every player who fought the NPC
// (NpcCombatSounds.play → Sounds.sendSound(player, ...)):
// the damagers, else the killer alone (NPCDeathTask.ts:45).
// B fans out with `SendSound(listener, DeathSoundFor(npc))`
// after the NoSound check, in the death task next to the
// death animation.
func DeathSoundFor(npc *NPC) Sound {
	if npc == nil {
		return NoSound
	}
	if sounder, ok := npc.Definition().(npcDeathSounder); ok {
		if id := sounder.DeathSound(); id > 0 {
			return NewSound(id, 1, 0, 0)
		}
	}
	return NoSound
}
