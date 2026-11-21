package game

import (
	"sort"
	"strconv"
	"strings"
)

// Sound and Sounds: the port of game/Sound.ts (280 lines) and
// game/Sounds.ts (162 lines).
//
// Sound is a synth id plus its playback shape (volume, delay,
// loopType). The constructor argument order (id, volume,
// delay, loopType) is ported exactly — note delay precedes
// loopType — and the 161 named statics below (153 distinct
// constructions plus 8 aliases) are transcribed verbatim
// from Sound.ts, in declaration order, with the same values.
// The order matters: the known-sound registries resolve ids
// first-wins in declaration order, as TS Object.entries does.
//
// Sounds is the send path: SendSound resolves a player for a
// private synth or a location for an area sound,
// SendSoundEffect sends direct, PlayAreaSoundAt /
// PlayAreaSound broadcast, and ResolveKnownSound /
// KnownSoundNames expose the registries. The wire encoding
// lives in internal/protocol (EncodeSound); delivery goes
// through the SoundSender session extension, following the
// DialogueSender / GroundItemSender precedent — the game
// layer never imports protocol or session.
//
// This file does NOT touch audio settings: internal/game
// /audio.go owns the varps, and neither file sends a frame
// on its own. Sounds sends through player sessions only.
//
// PLAYER SOURCE SEAM. TS Sounds reads World.getPlayers()
// (playAreaSound) and Projectile reads
// World.forEachNetworkPlayer (sendProjectile); Go has no
// global world, so both read the installed network-player
// source below (SetNetworkPlayerSource), installed once at
// boot by cmd/server/soundwiring.go from the world's player
// list. With no source installed, broadcasts reach nobody
// and direct sends are unaffected. The seam is shared with
// projectile.go; it lives here because Sounds is the
// alphabetically-first consumer, and both files document it.

// Sound is one sound effect (Sound, Sound.ts:250-280): a
// synth id, a volume, a delay in client cycles, and a loop
// type. It is a value type; the named statics below are the
// TS static instances. TS aliases (e.g. IMP_ATTACKING =
// NPC_ATTACKING) share one object; the Go aliases are copies
// of the same value, which compare equal with ==.
type Sound struct {
	id       int
	volume   int
	delay    int
	loopType int
}

// NewSound builds a Sound: constructor (id, volume, delay,
// loopType) (Sound.ts:258-264). The argument ORDER is part
// of the port: delay precedes loopType, as in TS.
func NewSound(id, volume, delay, loopType int) Sound {
	return Sound{id: id, volume: volume, delay: delay, loopType: loopType}
}

// ID returns the synth id (getId, Sound.ts:266-268).
func (s Sound) ID() int { return s.id }

// Volume returns the volume (getVolume, Sound.ts:270-272).
func (s Sound) Volume() int { return s.volume }

// ClientVolume returns the volume clamped to 0..10
// (getClientVolume, Sound.ts:274-276). Note the clamp is
// applied by Sounds.sendSound when it reads the volume —
// and PacketSender.sendSoundEffect then drops the volume
// entirely (the encoder has no volume field); see the
// session sender.
func (s Sound) ClientVolume() int {
	if s.volume < 0 {
		return 0
	}
	if s.volume > 10 {
		return 10
	}
	return s.volume
}

// Delay returns the delay in client cycles (getDelay,
// Sound.ts:278-280).
func (s Sound) Delay() int { return s.delay }

// LoopType returns the loop type (getLoopType, Sound.ts:282).
func (s Sound) LoopType() int { return s.loopType }

// Named sounds: the 161 statics of Sound.ts, in declaration
// order. Values are transcribed verbatim; do not reorder or
// "fix" them (the phase 21 plan §B). The 8 aliases are
// assignments from the aliased var, as in TS. TS comments
// recording capture provenance are not repeated here; the
// values are the port.
var (
	SoundCutting             = NewSound(2605, 1, 0, 0)
	SoundCrossbowLimbs       = NewSound(2922, 1, 0, 0)
	SoundShearSheep          = NewSound(761, 1, 0, 0)
	SoundCookingCook         = NewSound(2577, 1, 0, 0)
	SoundCraftRunes          = NewSound(2710, 1, 0, 0)
	SoundMiningMine          = NewSound(3220, 1, 0, 0)
	SoundFishingFish         = NewSound(2600, 1, 0, 0)
	SoundWoodcuttingTreeDown = NewSound(2734, 1, 0, 0)
	SoundPlayerGettingHit    = NewSound(513, 1, 0, 0)
	SoundDefenceBlock        = NewSound(511, 1, 0, 0)
	SoundNpcAttacking        = NewSound(394, 1, 0, 0)
	// IMP_ATTACKING is the NPC_ATTACKING alias (Sound.ts: Sound.NPC_ATTACKING).
	SoundImpAttacking    = SoundNpcAttacking
	SoundShootArrow      = NewSound(2702, 1, 0, 0)
	SoundShootCrossbow   = NewSound(2695, 1, 0, 0)
	SoundShootBowQuiet   = NewSound(2700, 1, 0, 0)
	SoundVenatorBowShoot = NewSound(6797, 1, 0, 0)
	SoundThrowDart       = NewSound(2696, 1, 0, 0)
	SoundWeapon          = NewSound(2567, 1, 0, 0)
	SoundWeaponGodsword  = NewSound(3847, 1, 0, 0)
	SoundWeaponStaff     = NewSound(2556, 1, 0, 0)
	// WEAPON_BOW is the SHOOT_ARROW alias (Sound.ts: Sound.SHOOT_ARROW).
	SoundWeaponBow         = SoundShootArrow
	SoundWeaponBattleAxe   = NewSound(2498, 1, 0, 0)
	SoundWeaponTwoHander   = NewSound(2503, 1, 0, 0)
	SoundWeaponScimitar    = NewSound(2500, 1, 0, 0)
	SoundWeaponWhip        = NewSound(2720, 1, 0, 0)
	SoundWeaponDaggerStab  = NewSound(2501, 1, 0, 0)
	SoundWeaponDaggerSlash = NewSound(2500, 1, 0, 0)
	// WEAPON_DRAGON_DAGGER_STAB is the WEAPON_DAGGER_STAB alias (Sound.ts: Sound.WEAPON_DAGGER_STAB).
	SoundWeaponDragonDaggerStab    = SoundWeaponDaggerStab
	SoundWeaponSwordStab           = NewSound(2501, 1, 0, 0)
	SoundWeaponSwordSlash          = NewSound(2500, 1, 0, 0)
	SoundWeaponMaceStab            = NewSound(2509, 1, 0, 0)
	SoundWeaponMaceCrush           = NewSound(2508, 1, 0, 0)
	SoundWeaponWarhammer           = NewSound(2567, 1, 0, 0)
	SoundWeaponSpearStab           = NewSound(2549, 1, 0, 0)
	SoundWeaponSpearSlash          = NewSound(2548, 1, 0, 0)
	SoundWeaponSpearCrush          = NewSound(2547, 1, 0, 0)
	SoundWeaponScytheStab          = NewSound(2525, 1, 0, 0)
	SoundWeaponScytheSlash         = NewSound(2524, 1, 0, 0)
	SoundWeaponHallowedFlail       = NewSound(1713, 1, 0, 0)
	SoundWeaponInfernalTecpatl     = NewSound(11456, 1, 0, 0)
	SoundWeaponUnarmedPunch        = NewSound(2566, 1, 0, 0)
	SoundWeaponUnarmedKick         = NewSound(2565, 1, 0, 0)
	SoundWeaponDharokGreataxe      = NewSound(1321, 1, 0, 0)
	SoundWeaponDharokGreataxeCrush = NewSound(1316, 1, 0, 0)
	SoundWeaponVeracFlail          = NewSound(1335, 1, 0, 0)
	// WEAPON_GUTHAN_WARSPEAR is the WEAPON_SPEAR_STAB alias (Sound.ts: Sound.WEAPON_SPEAR_STAB).
	SoundWeaponGuthanWarspear = SoundWeaponSpearStab
	SoundWeaponToragHammer    = NewSound(1332, 1, 0, 0)
	// WEAPON_GRANITE_MAUL is the WEAPON_WARHAMMER alias (Sound.ts: Sound.WEAPON_WARHAMMER).
	SoundWeaponGraniteMaul        = SoundWeaponWarhammer
	SoundVengeance                = NewSound(2907, 1, 0, 0)
	SoundVengeanceOther           = NewSound(2908, 1, 0, 0)
	SoundDragonDaggerSpecial      = NewSound(2537, 1, 0, 0)
	SoundMagicShortbowSpecial     = NewSound(2545, 1, 0, 0)
	SoundDragonMaceSpecial        = NewSound(2541, 1, 0, 0)
	SoundDragonSpearSpecial       = NewSound(2544, 1, 0, 0)
	SoundDragonBattleaxeSpecial   = NewSound(2538, 1, 0, 1)
	SoundDragonAxeSpecial         = NewSound(2531, 1, 0, 1)
	SoundDragonLongswordSpecial   = NewSound(2529, 1, 0, 0)
	SoundMagicLongbowSpecial      = NewSound(2536, 1, 0, 0)
	SoundWhipSpecial              = NewSound(2713, 1, 0, 0)
	SoundGodswordSpecial          = NewSound(3869, 1, 0, 0)
	SoundDragonClawsSpecial1      = NewSound(4138, 1, 10, 0)
	SoundDragonClawsSpecial2      = NewSound(4140, 1, 30, 0)
	SoundDragonClawsSpecial3      = NewSound(4141, 1, 42, 0)
	SoundDragonClawsSpecial4      = NewSound(4141, 1, 60, 0)
	SoundDragonScimitarSpecial    = NewSound(2540, 1, 0, 0)
	SoundDragonHalberdSpecial     = NewSound(2533, 1, 0, 0)
	SoundDragonWarhammerSpecial   = NewSound(2520, 1, 0, 0)
	SoundGraniteMaulSpecial       = NewSound(2715, 1, 0, 0)
	SoundBarrelchestAnchorSpecial = NewSound(3481, 1, 0, 0)
	SoundBrineSabreSpecial        = NewSound(3473, 1, 0, 0)
	SoundRuneClawsSpecial         = NewSound(2534, 1, 0, 0)
	SoundBurningClawsSpecial1     = NewSound(9316, 1, 29, 0)
	SoundBurningClawsSpecial2     = NewSound(9316, 1, 60, 0)
	SoundSeercullSpecial          = NewSound(2546, 1, 0, 0)
	SoundExcaliburSpecial         = NewSound(2539, 1, 0, 0)
	SoundArmadylCrossbowSpecial   = NewSound(3892, 1, 15, 0)
	SoundToxicBlowpipeSpecial     = NewSound(800, 1, 32, 0)
	SoundDarkBowDoublefire        = NewSound(3731, 1, 0, 0)
	SoundDarkBowFire              = NewSound(3734, 1, 0, 0)
	SoundDarkBowImpact            = NewSound(3737, 1, 0, 0)
	SoundDarkBowDragonSpecial     = NewSound(3733, 1, 0, 0)
	SoundDarkBowShadowSpecial     = NewSound(3736, 1, 0, 0)
	SoundEmberlightSpecial1       = NewSound(9319, 1, 11, 0)
	SoundEmberlightSpecial2       = NewSound(9320, 1, 28, 0)
	SoundEmberlightSpecial3       = NewSound(9321, 1, 53, 0)
	SoundVoidwakerSpecial1        = NewSound(5027, 1, 0, 0)
	SoundVoidwakerSpecial2        = NewSound(6182, 1, 30, 0)
	SoundSpellFailSplash          = NewSound(227, 1, 0, 0)
	SoundTelekineticGrab          = NewSound(192, 1, 0, 0)
	SoundHighAlchemy              = NewSound(97, 1, 0, 0)
	SoundLowAlchemy               = NewSound(98, 1, 0, 0)
	SoundSuperheatItem            = NewSound(190, 1, 0, 0)
	SoundTeleport                 = NewSound(200, 1, 0, 0)
	SoundIceBarrageImpact         = NewSound(168, 1, 0, 0)
	// ICA_BARRAGE_IMPACT is the ICE_BARRAGE_IMPACT alias (Sound.ts: Sound.ICE_BARRAGE_IMPACT).
	SoundIcaBarrageImpact         = SoundIceBarrageImpact
	SoundBloodBlitzCast           = NewSound(103, 1, 0, 0)
	SoundIceBlitzCast             = NewSound(169, 1, 0, 0)
	SoundDropItem                 = NewSound(2739, 1, 0, 0)
	SoundPickUpItem               = NewSound(2582, 1, 0, 0)
	SoundContainerOpen            = NewSound(2021, 1, 0, 0)
	SoundContainerClose           = NewSound(326, 1, 0, 0)
	SoundDoorOpen                 = NewSound(62, 1, 0, 0)
	SoundDoorClose                = NewSound(60, 1, 0, 0)
	SoundGateOpen                 = NewSound(67, 1, 0, 0)
	SoundGateClose                = NewSound(66, 1, 0, 0)
	SoundEquipmentOn              = NewSound(358, 1, 0, 0)
	SoundEquipmentOff             = NewSound(376, 1, 0, 0)
	SoundFireLight                = NewSound(2599, 1, 0, 0)
	SoundFireSuccessful           = NewSound(2596, 1, 0, 0)
	SoundFireFirstAttempt         = NewSound(2584, 1, 0, 0)
	SoundPotionMix                = NewSound(2611, 1, 0, 0)
	SoundSlashWeb                 = NewSound(237, 1, 0, 0)
	SoundFailSlashWeb             = NewSound(2548, 1, 0, 0)
	SoundFoodEat                  = NewSound(2393, 1, 0, 0)
	SoundDrink                    = NewSound(2401, 1, 0, 0)
	SoundPickLock                 = NewSound(2402, 1, 0, 0)
	SoundGenieLamp                = NewSound(430, 1, 0, 0)
	SoundBuryBones                = NewSound(2738, 1, 0, 0)
	SoundWildernessDitchJump      = NewSound(2462, 1, 0, 0)
	SoundThievingPickpocket       = NewSound(2581, 1, 0, 0)
	SoundPick                     = NewSound(2581, 1, 0, 0)
	SoundThievingStunned          = NewSound(2727, 1, 0, 0)
	SoundLevelUp                  = NewSound(2396, 1, 0, 0)
	SoundGemCutting               = NewSound(2586, 1, 0, 0)
	SoundSmithing                 = NewSound(3790, 1, 0, 0)
	SoundSmelting                 = NewSound(2725, 1, 0, 0)
	SoundPrayerDepleted           = NewSound(2672, 1, 0, 0)
	SoundPrayerSharpEye           = NewSound(2685, 1, 0, 0)
	SoundPrayerMysticWill         = NewSound(2670, 1, 0, 0)
	SoundPrayerProtectItem        = NewSound(1982, 1, 0, 0)
	SoundPrayerHawkEye            = NewSound(2666, 1, 0, 0)
	SoundPrayerMysticLore         = NewSound(2668, 1, 0, 0)
	SoundPrayerEagleEye           = NewSound(2665, 1, 0, 0)
	SoundPrayerMysticMight        = NewSound(2669, 1, 0, 0)
	SoundPrayerRetribution        = NewSound(2682, 1, 0, 0)
	SoundPrayerRedemption         = NewSound(2680, 1, 0, 0)
	SoundPrayerSmite              = NewSound(2686, 1, 0, 0)
	SoundPrayerChivalry           = NewSound(3826, 1, 0, 0)
	SoundPrayerPiety              = NewSound(3825, 1, 0, 0)
	SoundPrayerProtectMelee       = NewSound(2676, 1, 0, 0)
	SoundPrayerSuperhumanStrength = NewSound(2689, 1, 0, 0)
	SoundPrayerTurnOff            = NewSound(2663, 1, 0, 0)
	SoundPrayerClarityOfThought   = NewSound(2664, 1, 0, 0)
	SoundPrayerProtectMagic       = NewSound(2675, 1, 0, 0)
	SoundPrayerSteelSkin          = NewSound(2687, 1, 0, 0)
	SoundPrayerIncredibleReflexes = NewSound(2667, 1, 0, 0)
	SoundPrayerRockSkin           = NewSound(2684, 1, 0, 0)
	SoundPrayerRecharge           = NewSound(2674, 1, 0, 0)
	SoundPrayerRapidHeal          = NewSound(2678, 1, 0, 0)
	SoundPrayerProtectRange       = NewSound(2677, 1, 0, 0)
	SoundPrayerThickSkin          = NewSound(2690, 1, 0, 0)
	SoundPrayerInsufficient       = NewSound(447, 1, 0, 0)
	SoundPrayerImprovedReflexes   = NewSound(2662, 1, 0, 0)
	SoundPrayerBurstOfStrength    = NewSound(2688, 1, 0, 0)
	SoundPrayerUltimateStrength   = NewSound(2691, 1, 0, 0)
	SoundPrayerRapidRestore       = NewSound(2679, 1, 0, 0)
	// RUNECRAFTING is the CRAFT_RUNES alias (Sound.ts: Sound.CRAFT_RUNES).
	SoundRunecrafting         = SoundCraftRunes
	SoundHomeTeleport         = NewSound(193, 1, 0, 0)
	SoundCollectionLogNewItem = NewSound(2304, 1, 0, 0)
	SoundXpReward             = NewSound(2655, 1, 0, 0)
	// HOME_TELEPORT_ALT is the HOME_TELEPORT alias (Sound.ts: Sound.HOME_TELEPORT).
	SoundHomeTeleportAlt = SoundHomeTeleport
)

var knownSounds = []knownSound{
	{"CUTTING", SoundCutting},
	{"CROSSBOW_LIMBS", SoundCrossbowLimbs},
	{"SHEAR_SHEEP", SoundShearSheep},
	{"COOKING_COOK", SoundCookingCook},
	{"CRAFT_RUNES", SoundCraftRunes},
	{"MINING_MINE", SoundMiningMine},
	{"FISHING_FISH", SoundFishingFish},
	{"WOODCUTTING_TREE_DOWN", SoundWoodcuttingTreeDown},
	{"PLAYER_GETTING_HIT", SoundPlayerGettingHit},
	{"DEFENCE_BLOCK", SoundDefenceBlock},
	{"NPC_ATTACKING", SoundNpcAttacking},
	{"IMP_ATTACKING", SoundImpAttacking},
	{"SHOOT_ARROW", SoundShootArrow},
	{"SHOOT_CROSSBOW", SoundShootCrossbow},
	{"SHOOT_BOW_QUIET", SoundShootBowQuiet},
	{"VENATOR_BOW_SHOOT", SoundVenatorBowShoot},
	{"THROW_DART", SoundThrowDart},
	{"WEAPON", SoundWeapon},
	{"WEAPON_GODSWORD", SoundWeaponGodsword},
	{"WEAPON_STAFF", SoundWeaponStaff},
	{"WEAPON_BOW", SoundWeaponBow},
	{"WEAPON_BATTLE_AXE", SoundWeaponBattleAxe},
	{"WEAPON_TWO_HANDER", SoundWeaponTwoHander},
	{"WEAPON_SCIMITAR", SoundWeaponScimitar},
	{"WEAPON_WHIP", SoundWeaponWhip},
	{"WEAPON_DAGGER_STAB", SoundWeaponDaggerStab},
	{"WEAPON_DAGGER_SLASH", SoundWeaponDaggerSlash},
	{"WEAPON_DRAGON_DAGGER_STAB", SoundWeaponDragonDaggerStab},
	{"WEAPON_SWORD_STAB", SoundWeaponSwordStab},
	{"WEAPON_SWORD_SLASH", SoundWeaponSwordSlash},
	{"WEAPON_MACE_STAB", SoundWeaponMaceStab},
	{"WEAPON_MACE_CRUSH", SoundWeaponMaceCrush},
	{"WEAPON_WARHAMMER", SoundWeaponWarhammer},
	{"WEAPON_SPEAR_STAB", SoundWeaponSpearStab},
	{"WEAPON_SPEAR_SLASH", SoundWeaponSpearSlash},
	{"WEAPON_SPEAR_CRUSH", SoundWeaponSpearCrush},
	{"WEAPON_SCYTHE_STAB", SoundWeaponScytheStab},
	{"WEAPON_SCYTHE_SLASH", SoundWeaponScytheSlash},
	{"WEAPON_HALLOWED_FLAIL", SoundWeaponHallowedFlail},
	{"WEAPON_INFERNAL_TECPATL", SoundWeaponInfernalTecpatl},
	{"WEAPON_UNARMED_PUNCH", SoundWeaponUnarmedPunch},
	{"WEAPON_UNARMED_KICK", SoundWeaponUnarmedKick},
	{"WEAPON_DHAROK_GREATAXE", SoundWeaponDharokGreataxe},
	{"WEAPON_DHAROK_GREATAXE_CRUSH", SoundWeaponDharokGreataxeCrush},
	{"WEAPON_VERAC_FLAIL", SoundWeaponVeracFlail},
	{"WEAPON_GUTHAN_WARSPEAR", SoundWeaponGuthanWarspear},
	{"WEAPON_TORAG_HAMMER", SoundWeaponToragHammer},
	{"WEAPON_GRANITE_MAUL", SoundWeaponGraniteMaul},
	{"VENGEANCE", SoundVengeance},
	{"VENGEANCE_OTHER", SoundVengeanceOther},
	{"DRAGON_DAGGER_SPECIAL", SoundDragonDaggerSpecial},
	{"MAGIC_SHORTBOW_SPECIAL", SoundMagicShortbowSpecial},
	{"DRAGON_MACE_SPECIAL", SoundDragonMaceSpecial},
	{"DRAGON_SPEAR_SPECIAL", SoundDragonSpearSpecial},
	{"DRAGON_BATTLEAXE_SPECIAL", SoundDragonBattleaxeSpecial},
	{"DRAGON_AXE_SPECIAL", SoundDragonAxeSpecial},
	{"DRAGON_LONGSWORD_SPECIAL", SoundDragonLongswordSpecial},
	{"MAGIC_LONGBOW_SPECIAL", SoundMagicLongbowSpecial},
	{"WHIP_SPECIAL", SoundWhipSpecial},
	{"GODSWORD_SPECIAL", SoundGodswordSpecial},
	{"DRAGON_CLAWS_SPECIAL_1", SoundDragonClawsSpecial1},
	{"DRAGON_CLAWS_SPECIAL_2", SoundDragonClawsSpecial2},
	{"DRAGON_CLAWS_SPECIAL_3", SoundDragonClawsSpecial3},
	{"DRAGON_CLAWS_SPECIAL_4", SoundDragonClawsSpecial4},
	{"DRAGON_SCIMITAR_SPECIAL", SoundDragonScimitarSpecial},
	{"DRAGON_HALBERD_SPECIAL", SoundDragonHalberdSpecial},
	{"DRAGON_WARHAMMER_SPECIAL", SoundDragonWarhammerSpecial},
	{"GRANITE_MAUL_SPECIAL", SoundGraniteMaulSpecial},
	{"BARRELCHEST_ANCHOR_SPECIAL", SoundBarrelchestAnchorSpecial},
	{"BRINE_SABRE_SPECIAL", SoundBrineSabreSpecial},
	{"RUNE_CLAWS_SPECIAL", SoundRuneClawsSpecial},
	{"BURNING_CLAWS_SPECIAL_1", SoundBurningClawsSpecial1},
	{"BURNING_CLAWS_SPECIAL_2", SoundBurningClawsSpecial2},
	{"SEERCULL_SPECIAL", SoundSeercullSpecial},
	{"EXCALIBUR_SPECIAL", SoundExcaliburSpecial},
	{"ARMADYL_CROSSBOW_SPECIAL", SoundArmadylCrossbowSpecial},
	{"TOXIC_BLOWPIPE_SPECIAL", SoundToxicBlowpipeSpecial},
	{"DARK_BOW_DOUBLEFIRE", SoundDarkBowDoublefire},
	{"DARK_BOW_FIRE", SoundDarkBowFire},
	{"DARK_BOW_IMPACT", SoundDarkBowImpact},
	{"DARK_BOW_DRAGON_SPECIAL", SoundDarkBowDragonSpecial},
	{"DARK_BOW_SHADOW_SPECIAL", SoundDarkBowShadowSpecial},
	{"EMBERLIGHT_SPECIAL_1", SoundEmberlightSpecial1},
	{"EMBERLIGHT_SPECIAL_2", SoundEmberlightSpecial2},
	{"EMBERLIGHT_SPECIAL_3", SoundEmberlightSpecial3},
	{"VOIDWAKER_SPECIAL_1", SoundVoidwakerSpecial1},
	{"VOIDWAKER_SPECIAL_2", SoundVoidwakerSpecial2},
	{"SPELL_FAIL_SPLASH", SoundSpellFailSplash},
	{"TELEKINETIC_GRAB", SoundTelekineticGrab},
	{"HIGH_ALCHEMY", SoundHighAlchemy},
	{"LOW_ALCHEMY", SoundLowAlchemy},
	{"SUPERHEAT_ITEM", SoundSuperheatItem},
	{"TELEPORT", SoundTeleport},
	{"ICE_BARRAGE_IMPACT", SoundIceBarrageImpact},
	{"ICA_BARRAGE_IMPACT", SoundIcaBarrageImpact},
	{"BLOOD_BLITZ_CAST", SoundBloodBlitzCast},
	{"ICE_BLITZ_CAST", SoundIceBlitzCast},
	{"DROP_ITEM", SoundDropItem},
	{"PICK_UP_ITEM", SoundPickUpItem},
	{"CONTAINER_OPEN", SoundContainerOpen},
	{"CONTAINER_CLOSE", SoundContainerClose},
	{"DOOR_OPEN", SoundDoorOpen},
	{"DOOR_CLOSE", SoundDoorClose},
	{"GATE_OPEN", SoundGateOpen},
	{"GATE_CLOSE", SoundGateClose},
	{"EQUIPMENT_ON", SoundEquipmentOn},
	{"EQUIPMENT_OFF", SoundEquipmentOff},
	{"FIRE_LIGHT", SoundFireLight},
	{"FIRE_SUCCESSFUL", SoundFireSuccessful},
	{"FIRE_FIRST_ATTEMPT", SoundFireFirstAttempt},
	{"POTION_MIX", SoundPotionMix},
	{"SLASH_WEB", SoundSlashWeb},
	{"FAIL_SLASH_WEB", SoundFailSlashWeb},
	{"FOOD_EAT", SoundFoodEat},
	{"DRINK", SoundDrink},
	{"PICK_LOCK", SoundPickLock},
	{"GENIE_LAMP", SoundGenieLamp},
	{"BURY_BONES", SoundBuryBones},
	{"WILDERNESS_DITCH_JUMP", SoundWildernessDitchJump},
	{"THIEVING_PICKPOCKET", SoundThievingPickpocket},
	{"PICK", SoundPick},
	{"THIEVING_STUNNED", SoundThievingStunned},
	{"LEVEL_UP", SoundLevelUp},
	{"GEM_CUTTING", SoundGemCutting},
	{"SMITHING", SoundSmithing},
	{"SMELTING", SoundSmelting},
	{"PRAYER_DEPLETED", SoundPrayerDepleted},
	{"PRAYER_SHARP_EYE", SoundPrayerSharpEye},
	{"PRAYER_MYSTIC_WILL", SoundPrayerMysticWill},
	{"PRAYER_PROTECT_ITEM", SoundPrayerProtectItem},
	{"PRAYER_HAWK_EYE", SoundPrayerHawkEye},
	{"PRAYER_MYSTIC_LORE", SoundPrayerMysticLore},
	{"PRAYER_EAGLE_EYE", SoundPrayerEagleEye},
	{"PRAYER_MYSTIC_MIGHT", SoundPrayerMysticMight},
	{"PRAYER_RETRIBUTION", SoundPrayerRetribution},
	{"PRAYER_REDEMPTION", SoundPrayerRedemption},
	{"PRAYER_SMITE", SoundPrayerSmite},
	{"PRAYER_CHIVALRY", SoundPrayerChivalry},
	{"PRAYER_PIETY", SoundPrayerPiety},
	{"PRAYER_PROTECT_MELEE", SoundPrayerProtectMelee},
	{"PRAYER_SUPERHUMAN_STRENGTH", SoundPrayerSuperhumanStrength},
	{"PRAYER_TURN_OFF", SoundPrayerTurnOff},
	{"PRAYER_CLARITY_OF_THOUGHT", SoundPrayerClarityOfThought},
	{"PRAYER_PROTECT_MAGIC", SoundPrayerProtectMagic},
	{"PRAYER_STEEL_SKIN", SoundPrayerSteelSkin},
	{"PRAYER_INCREDIBLE_REFLEXES", SoundPrayerIncredibleReflexes},
	{"PRAYER_ROCK_SKIN", SoundPrayerRockSkin},
	{"PRAYER_RECHARGE", SoundPrayerRecharge},
	{"PRAYER_RAPID_HEAL", SoundPrayerRapidHeal},
	{"PRAYER_PROTECT_RANGE", SoundPrayerProtectRange},
	{"PRAYER_THICK_SKIN", SoundPrayerThickSkin},
	{"PRAYER_INSUFFICIENT", SoundPrayerInsufficient},
	{"PRAYER_IMPROVED_REFLEXES", SoundPrayerImprovedReflexes},
	{"PRAYER_BURST_OF_STRENGTH", SoundPrayerBurstOfStrength},
	{"PRAYER_ULTIMATE_STRENGTH", SoundPrayerUltimateStrength},
	{"PRAYER_RAPID_RESTORE", SoundPrayerRapidRestore},
	{"RUNECRAFTING", SoundRunecrafting},
	{"HOME_TELEPORT", SoundHomeTeleport},
	{"COLLECTION_LOG_NEW_ITEM", SoundCollectionLogNewItem},
	{"XP_REWARD", SoundXpReward},
	{"HOME_TELEPORT_ALT", SoundHomeTeleportAlt},
}

// knownSound is one registry entry: a TS static name and
// its Sound, in declaration order.
type knownSound struct {
	name  string
	sound Sound
}

// knownSoundsByName is knownSoundsByName (Sounds.ts:4-10):
// every named static, by its TS name.
var knownSoundsByName = buildKnownSoundsByName()

func buildKnownSoundsByName() map[string]Sound {
	out := make(map[string]Sound, len(knownSounds))
	for _, entry := range knownSounds {
		out[entry.name] = entry.sound
	}
	return out
}

// knownSoundsByID is knownSoundsById (Sounds.ts:12-20):
// the FIRST name in declaration order wins each id.
var knownSoundsByID = buildKnownSoundsByID()

func buildKnownSoundsByID() map[int]Sound {
	out := make(map[int]Sound)
	for _, entry := range knownSounds {
		if _, ok := out[entry.sound.ID()]; !ok {
			out[entry.sound.ID()] = entry.sound
		}
	}
	return out
}

// ResolveKnownSound resolves a sound by id or name
// (resolveKnownSound, Sounds.ts:131-152): an int resolves
// by id; a string is trimmed, uppercased, stripped of a
// leading "SOUND." prefix, and resolved as a decimal id
// when all digits, else as a name. Anything else —
// including an empty name after normalization — resolves
// to nothing. TS returns null; Go returns false.
func ResolveKnownSound(token any) (Sound, bool) {
	switch value := token.(type) {
	case int:
		sound, ok := knownSoundsByID[value]
		return sound, ok
	case string:
		normalized := strings.ToUpper(strings.TrimSpace(value))
		normalized = strings.TrimPrefix(normalized, "SOUND.")
		if normalized == "" {
			return Sound{}, false
		}
		if isAllDigits(normalized) {
			id, err := strconv.Atoi(normalized)
			if err != nil {
				return Sound{}, false
			}
			sound, ok := knownSoundsByID[id]
			return sound, ok
		}
		sound, ok := knownSoundsByName[normalized]
		return sound, ok
	default:
		return Sound{}, false
	}
}

func isAllDigits(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return len(s) > 0
}

// KnownSoundNames returns the known sound names, sorted
// (knownSoundNames, Sounds.ts:154-156).
func KnownSoundNames() []string {
	names := make([]string, 0, len(knownSoundsByName))
	for name := range knownSoundsByName {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// SoundSender is the optional SessionPort extension through
// which sounds reach a player's client: the Go form of
// PacketSender.sendSoundEffect / sendAreaSound
// (PacketSender.ts:252-261, 293-306). *session.Session
// implements it (internal/session/sound_sender.go); a
// session-less player, or a SessionPort without the
// extension, drops the sound silently, as Player.SendVarp
// drops a varp with no session.
type SoundSender interface {
	// SendSoundEffect sends an unpositioned sound. The
	// volume is accepted and dropped by the encoder, as
	// TS drops it (see the session sender).
	SendSoundEffect(soundID, loopType, delay, volume int)
	// SendAreaSound sends a positioned sound.
	SendAreaSound(soundID, x, y, level, loops, delay, radius, attenuation int)
}

// SoundSource is anything with a world location and an
// area identity: the Go form of the entity side of
// Sounds.sendSound / playAreaSoundAt (Sounds.ts:53-56,
// 72-75), which read entity.getLocation() and, for area
// matching, entity.getPrivateArea(). *Player and *NPC both
// satisfy it through Mobile.
type SoundSource interface {
	Location() Location
	Area() *Area
}

// networkPlayerSource lists the registered players in slot
// order for sound/projectile broadcasts (World.getPlayers,
// World.ts:232-234; World.forEachNetworkPlayer, :159-169).
// It is set once at boot via SetNetworkPlayerSource, before
// the world starts ticking, and never changed afterwards;
// like relationsWorld and areaCaller it is deliberately
// unsynchronized. Nil means no players: broadcasts send
// nothing.
var networkPlayerSource func() []*Player

// SetNetworkPlayerSource installs the player source
// broadcasts iterate. A nil source uninstalls (tests use
// this to restore the unwired state). The coordinator
// installs a closure over the world's player list.
func SetNetworkPlayerSource(src func() []*Player) {
	networkPlayerSource = src
}

// networkPlayers returns the broadcast iteration list, or
// nil when no source is installed.
func networkPlayers() []*Player {
	if networkPlayerSource == nil {
		return nil
	}
	return networkPlayerSource()
}

// SendSound sends a sound for an entity (sendSound,
// Sounds.ts:44-68): a player hears a private synth through
// sendSoundEffect (with the bot guard below); anything
// else with a location becomes an area sound at its tile,
// owned by its area. A nil entity, or one with no
// location, sends nothing.
//
// TS resolves the player by duck typing (resolvePlayer,
// Sounds.ts:22-42: a Mobile-like with isPlayer()/
// getAsPlayer(), or a player-like with getPacketSender()/
// getUsername()). Go has exactly one player shape,
// *Player, so the resolution is a type switch: *Player
// first, then the SoundSource area path. A *Mobile never
// represents a player in Go, so it takes the area path.
//
// BOT GUARD, DEGENERATED AS DOCUMENTED. TS skips the
// private synth when `player.isUnwatchedBot?.() ??
// player.isPlayerBot?.()` (Sounds.ts:48): an unwatched bot
// (a bot with no network client, Player.ts:1538-1540)
// hears nothing. Go has bots (Player.IsBot) but no
// watched/unwatched distinction, so ALL bots are skipped
// — the same documented deviation as the ground-item
// fan-out (grounditems.go) and the destination flag. The
// direct SendSoundEffect below has NO guard, exactly as
// the TS static has none: the guard lives here, in
// sendSound, not in the direct send.
func SendSound(entity any, sound Sound) {
	// A zero Sound is TS's null sound (Sounds.ts:45-47:
	// `if (!sound) return`): nothing is sent. Callers pass
	// picker results here (see combat_sounds.go); the guard
	// keeps a NoSound from reaching the encoder as id 0.
	// SendSoundEffect below stays unguarded, as in TS.
	if sound.ID() <= 0 {
		return
	}
	switch e := entity.(type) {
	case *Player:
		if e == nil || e.IsBot {
			return
		}
		SendSoundEffect(e, sound.ID(), sound.LoopType(), sound.Delay(), sound.ClientVolume())
	case *NPC:
		if e == nil {
			return
		}
		sendAreaSoundFrom(e, sound)
	case *Mobile:
		// A nil Mobile has no location; a live one takes
		// the area path below through the SoundSource
		// assertion in the default branch's shape. It is
		// routed here only for the nil guard.
		if e == nil {
			return
		}
		sendAreaSoundFrom(e, sound)
	default:
		if entity == nil {
			return
		}
		if src, ok := entity.(SoundSource); ok {
			sendAreaSoundFromSource(src, entity, sound)
		}
	}
}

// sendAreaSoundFrom sends sound as an area sound at src's
// tile, owned by src's area (the location branch of
// sendSound, Sounds.ts:59-67).
func sendAreaSoundFrom(src SoundSource, sound Sound) {
	sendAreaSoundFromSource(src, src, sound)
}

func sendAreaSoundFromSource(src SoundSource, source any, sound Sound) {
	loc := src.Location()
	PlayAreaSound(PlayAreaSoundOptions{
		SoundID: sound.ID(),
		Source:  source,
		X:       loc.X,
		Y:       loc.Y,
		Level:   loc.Z,
		Loops:   sound.LoopType(),
		Delay:   sound.Delay(),
	})
}

// SendSoundEffect sends a sound direct to one player
// (sendSoundEffect, Sounds.ts:70-72): the player's
// PacketSender.sendSoundEffect, unguarded — TS applies no
// bot or registration check here. A nil player, or a
// player whose session lacks the SoundSender extension,
// drops the sound silently.
func SendSoundEffect(player *Player, soundID, loopType, delay, volume int) {
	if player == nil || player.Session == nil {
		return
	}
	if sender, ok := player.Session.(SoundSender); ok {
		sender.SendSoundEffect(soundID, loopType, delay, volume)
	}
}

// PlayAreaSoundAt plays a sound at an entity's own tile
// (playAreaSoundAt, Sounds.ts:79-94): an area sound with
// the given radius (TS default 5; Go callers pass it
// explicitly — live sends most melee special attacks with
// radius 5, and vengeance with 10). An entity with no
// location sends nothing.
func PlayAreaSoundAt(entity any, sound Sound, radius int) {
	switch e := entity.(type) {
	case *Player:
		if e == nil {
			return
		}
	case *NPC:
		if e == nil {
			return
		}
	case *Mobile:
		if e == nil {
			return
		}
	}
	if entity == nil {
		return
	}
	src, ok := entity.(SoundSource)
	if !ok {
		return
	}
	loc := src.Location()
	PlayAreaSound(PlayAreaSoundOptions{
		SoundID: sound.ID(),
		Source:  entity,
		X:       loc.X,
		Y:       loc.Y,
		Level:   loc.Z,
		Loops:   sound.LoopType(),
		Delay:   sound.Delay(),
		Radius:  radius,
	})
}

// PlayAreaSoundOptions are playAreaSound's options object
// (Sounds.ts:96-110). Level defaults to 0, Loops/Delay/
// Radius/Attenuation to 0 (the encoder clamps loops to a
// minimum of 1); Source is the entity whose area owns the
// sound; Exclude is one player who never hears it.
type PlayAreaSoundOptions struct {
	SoundID     int
	X, Y, Level int
	Loops       int
	Delay       int
	Radius      int
	Attenuation int
	Source      any
	Exclude     *Player
}

// PlayAreaSound broadcasts a positioned sound
// (playAreaSound, Sounds.ts:96-127): radius clamped 0..31,
// broadcast radius max(15, radius+1), same-plane check,
// private-area matching, bot guard, exclude support. A
// sound id of 0 or less sends nothing.
//
// AREA-MATCHING RULE (phase 21 plan, contract 2): TS
// compares getPrivateArea() identities — the source's
// private area against each player's — and skips the check
// entirely when the source has no getPrivateArea method.
// Go compares Mobile.Area() pointers (nil == nil plays;
// different areas skip). This is behaviorally equivalent
// GIVEN contract 1: PrivateArea.Add sets the actor's Area
// to the embedded *Area (TS entity.setArea(this)), so two
// actors share an Area() pointer exactly when they share
// a private area, and main-world actors share nil. A
// source with no area identity (no SoundSource, or nil)
// applies no area filtering, as in TS. Worker B MUST
// verify area evaluation does not clear a private-area
// member's Area() — if it does, this equivalence breaks.
//
// The bot guard is the same degeneration as SendSound:
// all bots skipped (no watched/unwatched distinction in
// Go). No session-connected check is applied: TS iterates
// World.getPlayers(), not forEachNetworkPlayer, and a
// closed channel drops the queued packet in SendPacket —
// a nil session drops here through the SoundSender
// assertion, exactly as SendSoundEffect drops.
func PlayAreaSound(opts PlayAreaSoundOptions) {
	if !(opts.SoundID > 0) {
		return
	}
	level := opts.Level
	radius := opts.Radius
	if radius < 0 {
		radius = 0
	}
	if radius > 31 {
		radius = 31
	}
	broadcastRadius := radius + 1
	if broadcastRadius < 15 {
		broadcastRadius = 15
	}
	sourceArea, hasArea := soundArea(opts.Source)
	for _, player := range networkPlayers() {
		if player == nil || player.IsBot || player == opts.Exclude {
			continue
		}
		if hasArea && player.Area() != sourceArea {
			continue
		}
		loc := player.Location()
		if loc.Z != level {
			continue
		}
		if chebyshevDistance(loc.X, loc.Y, opts.X, opts.Y) > broadcastRadius {
			continue
		}
		sendAreaSoundTo(player, opts, level, radius)
	}
}

// soundArea returns the source's area identity and whether
// the source HAS one (TS: source?.getPrivateArea
// truthiness). A typed-nil *Player/*NPC has none; any
// SoundSource has its Area(), nil included (nil is the
// main-world identity, and it filters: private-area
// players skip a main-world sound).
func soundArea(source any) (*Area, bool) {
	switch e := source.(type) {
	case nil:
		return nil, false
	case *Player:
		if e == nil {
			return nil, false
		}
		return e.Area(), true
	case *NPC:
		if e == nil {
			return nil, false
		}
		return e.Area(), true
	case *Mobile:
		if e == nil {
			return nil, false
		}
		return e.Area(), true
	default:
		if src, ok := source.(SoundSource); ok {
			return src.Area(), true
		}
		return nil, false
	}
}

// sendAreaSoundTo delivers one area-sound copy through the
// player's session, dropping silently without a SoundSender
// session.
func sendAreaSoundTo(player *Player, opts PlayAreaSoundOptions, level, radius int) {
	if player.Session == nil {
		return
	}
	if sender, ok := player.Session.(SoundSender); ok {
		sender.SendAreaSound(opts.SoundID, opts.X, opts.Y, level, opts.Loops, opts.Delay, radius, opts.Attenuation)
	}
}

func chebyshevDistance(x1, y1, x2, y2 int) int {
	dx := x1 - x2
	if dx < 0 {
		dx = -dx
	}
	dy := y1 - y2
	if dy < 0 {
		dy = -dy
	}
	if dx > dy {
		return dx
	}
	return dy
}
