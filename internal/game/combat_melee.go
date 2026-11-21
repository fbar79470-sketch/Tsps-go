package game

// MeleeCombatMethod is the port of
// method/impl/MeleeCombatMethod.ts: one instant hit, the
// weapon's melee permission, and the weapon's melee attack
// distance. Start plays the attack animation and the attack
// sound (Phase 29; the sound is Worker C's picker,
// AttackSoundFor).

// MeleeMethod is the shared melee combat method singleton
// (CombatFactory.MELEE_COMBAT).
var MeleeMethod CombatMethod = &meleeCombatMethod{}

type meleeCombatMethod struct{ BaseCombatMethod }

func (m *meleeCombatMethod) Type() CombatType { return CombatTypeMelee }

// sendCombatSound sends a combat sound for a fighter
// (Sounds.sendSound, Sounds.ts:44-68): a player hears a
// private synth; anything else becomes an area sound at its
// tile. The combat methods hold a *Mobile, and SendSound
// type-switches a *Mobile to the area path — so the player
// must be resolved first through the bound identity
// (AsPlayer): a *Mobile that is a player would otherwise
// broadcast an area sound to everyone nearby instead of
// hearing its own synth. A zero Sound (NoSound) sends
// nothing.
func sendCombatSound(fighter *Mobile, sound Sound) {
	if sound.ID() <= 0 {
		return
	}
	if player := fighter.AsPlayer(); player != nil {
		SendSound(player, sound)
		return
	}
	SendSound(fighter, sound)
}

// Start plays the attack animation and sound in TS. The
// sound call is Worker C's picker (AttackSoundFor,
// combat_sounds.go).
func (m *meleeCombatMethod) Start(attacker, target *Mobile) {
	// MeleeCombatMethod.start (MeleeCombatMethod.ts:9-14):
	// the weapon's attack animation, skipped when -1, and
	// the attack sound inside the same guard (Worker C's
	// picker, combat_sounds.go).
	if anim := attacker.GetAttackAnim(target); anim != -1 {
		attacker.PerformAnimation(anim, 0)
		// MeleeCombatMethod.ts:12: the attack sound plays
		// inside the same guard (Worker C's picker). A
		// special attack's fire sound replaces the ordinary
		// attack sound (phase 30, item 5).
		if !PlaySpecialFireSound(attacker) {
			sendCombatSound(attacker, AttackSoundFor(attacker))
		}
	}
}

// Hits is MeleeCombatMethod.hits: a single instant hit.
func (m *meleeCombatMethod) Hits(attacker, target *Mobile) []*PendingHit {
	return []*PendingHit{NewPendingHit(attacker, target, m, 0)}
}

// CanAttack is MeleeCombatMethod.canAttack: a weapon that
// allows no melee keeps its target but never swings
// (WeaponProfiles.meleeAllowed). The seam defaults to true;
// content registers the weapon-profile lookup.
func (m *meleeCombatMethod) CanAttack(attacker, target *Mobile) bool {
	if !combatantFor(attacker).IsPlayer() {
		return true
	}
	return meleeAllowed(combatantFor(attacker).Player())
}

// AttackDistance is MeleeCombatMethod.attackDistance: the
// weapon's melee distance (default 1), 1 for NPCs.
func (m *meleeCombatMethod) AttackDistance(attacker *Mobile) int {
	if combatantFor(attacker).IsPlayer() {
		return meleeAttackDistance(combatantFor(attacker).Player(), 1)
	}
	return 1
}

// meleeAllowedFunc, when set, answers
// WeaponProfiles.meleeAllowed for a player's weapon. The
// default (nil) allows melee: unarmed and every weapon
// without a registered profile.
var meleeAllowedFunc func(player *Player) bool

// RegisterMeleeAllowed installs the melee-allowed lookup.
func RegisterMeleeAllowed(lookup func(player *Player) bool) {
	meleeAllowedFunc = lookup
}

func meleeAllowed(player *Player) bool {
	if meleeAllowedFunc != nil {
		return meleeAllowedFunc(player)
	}
	return true
}

// meleeDistanceFunc, when set, answers
// WeaponProfiles.meleeAttackDistance (halberds reach 2).
// The default is the fallback distance.
var meleeDistanceFunc func(player *Player, fallback int) int

// RegisterMeleeAttackDistance installs the melee-distance lookup.
func RegisterMeleeAttackDistance(lookup func(player *Player, fallback int) int) {
	meleeDistanceFunc = lookup
}

func meleeAttackDistance(player *Player, fallback int) int {
	if meleeDistanceFunc != nil {
		return meleeDistanceFunc(player, fallback)
	}
	return fallback
}
