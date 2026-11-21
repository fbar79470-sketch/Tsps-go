package game

import "math"

// Run-energy state and formulas: the port of the run-energy surface in
// Player.ts (get/set/addRunEnergy, getWeightKg), MovementQueue.ts
// (runEnergyDrainUnits/runEnergyRestoreUnits/drainRunEnergy), and the
// EquipmentEffects.ts modifier hooks.
//
// UNITS: energy is stored in OSRS units, 0..10,000, 100 units = 1%
// (GameConstants.ts:30, RUN_ENERGY_MAX_UNITS). The save stores a
// percent float; the conversions live on Player (GetRunEnergy /
// SetRunEnergy) and in persist.OwnedState, both following
// Player.ts:1257-1277 exactly.
//
// TICK-GOROUTINE DISCIPLINE: energy state is tick-goroutine-only like
// the rest of Player. Entry application (cmd/server) sets it before
// the player is queued; snapshots read it on the tick goroutine.
//
// DELIBERATE OMISSIONS (documented, not silent):
//   - The EquipmentEffects modifier registry (see
//     equipment_effects.go) applies at the two formula call sites
//     through the player-aware helpers (runEnergyDrainUnits /
//     runEnergyRestoreUnits); the exported pure formula functions
//     below read the unmodified base (nil player, identity
//     apply), preserving the phase 7 arithmetic vectors. The
//     status modifiers (applyRunEnergyStatusModifiers,
//     EquipmentEffects.ts:311-316) are NOT STARTED: see the
//     stamina bullet below.
//   - Stamina is always 0 ticks: no stamina system exists, and the
//     status modifiers start from 0 ticks
//     (applyRunEnergyStatusModifiers, EquipmentEffects.ts:311-316),
//     so every run-energy send carries staminaTicks 0. The status
//     modifier function itself is not ported: there is no status
//     object to modify, and the send sites hardcode 0 with a
//     citation.
//   - Agility is the live SkillManager current level (phase 9):
//     boosts, XP, and leveling exist on the manager; no boost
//     source exists yet, so the level only moves through
//     SetSkills/AddExperience (see GetCurrentAgilityLevel).
//   - Weight is the LIVE carried weight, computed from the
//     inventory + equipment containers on every read (see
//     GetWeightKg / weight.go).

// RunEnergyMaxUnits is RUN_ENERGY_MAX_UNITS (GameConstants.ts:30):
// OSRS run energy is 0-10,000 units; the orb shows only the whole
// percentage.
const RunEnergyMaxUnits = 10000

// RunEnergyDrainUnits ports MovementQueue.runEnergyDrainUnits
// (MovementQueue.ts:803-808): the energy units one running tick
// costs, WITHOUT modifiers. Weight is clamped to 0..64 kg;
// agility is the boosted current level (here: the live
// SkillManager current level — see GetCurrentAgilityLevel). The
// arithmetic follows TS op-for-op in float64, so boundary values
// floor exactly as the JS does:
//
//	base = floor((60 + floor(67*weight/64)) * (1 - agility/300))
//
// then the result is floored and clamped >= 0. This is the
// no-player form: the modifiers need the player (and its
// installed world registry), so the live call sites use
// runEnergyDrainUnits with the resolved player instead, which
// applies the drain modifiers at the TS position (after the
// base, before the floor/clamp). A tick with less energy left
// only spends what remains: the caller subtracts through
// SetRunEnergyUnits, whose clamp does that (MovementQueue.ts:803
// doc comment). Agility is NOT clamped here: TS reads it through
// getCurrentLevel, which sanitizes, and the formula itself applies
// no bounds — callers must pass the sanitized current level.
func RunEnergyDrainUnits(weightKg, agility int) int {
	return runEnergyDrainUnits(nil, weightKg, agility)
}

// runEnergyDrainUnits is the player-aware drain formula: the same
// arithmetic as RunEnergyDrainUnits, with the drain modifiers of
// p's installed world registry applied at the TS position
// (MovementQueue.ts:808: max(0, floor(apply*Modifiers(p,
// base)))). A nil player, or a player with no registry
// installed, reads the unmodified base.
func runEnergyDrainUnits(p *Player, weightKg, agility int) int {
	weight := weightKg
	if weight < 0 {
		weight = 0
	}
	if weight > 64 {
		weight = 64
	}
	base := math.Floor((60 + math.Floor(67*float64(weight)/64)) * (1 - float64(agility)/300))
	drained := int(math.Floor(applyRunEnergyDrainModifiers(p, base)))
	if drained < 0 {
		return 0
	}
	return drained
}

// RunEnergyRestoreUnits ports MovementQueue.runEnergyRestoreUnits
// (MovementQueue.ts:811-815): the energy units regained per tick
// not spent running, WITHOUT modifiers:
//
//	base = floor(agility/10) + 15
//
// then the result is floored and clamped >= 0. This is the
// no-player form: the live call site (Player.Process) uses
// runEnergyRestoreUnits with the player, which applies the
// restore modifiers (the graceful set bonus) at the TS position.
// As in RunEnergyDrainUnits, agility is the sanitized current
// level and is not clamped here.
func RunEnergyRestoreUnits(agility int) int {
	return runEnergyRestoreUnits(nil, agility)
}

// runEnergyRestoreUnits is the player-aware restore formula: the
// same arithmetic as RunEnergyRestoreUnits, with the restore
// modifiers of p's installed world registry applied at the TS
// position (MovementQueue.ts:814: max(0, floor(apply*Modifiers(p,
// base)))). A nil player, or a player with no registry
// installed, reads the unmodified base.
func runEnergyRestoreUnits(p *Player, agility int) int {
	base := math.Floor(float64(agility)/10) + 15
	restored := int(math.Floor(applyRunEnergyRestoreModifiers(p, base)))
	if restored < 0 {
		return 0
	}
	return restored
}

// GetRunEnergy returns run energy as a percentage (0-100), for
// content that works in whole or fractional percent.
// Player.ts:1257-1259 (getRunEnergy: runEnergy / 100). The save
// snapshot writes this value (PlayerSave.ts:762, fromPlayer).
func (p *Player) GetRunEnergy() float64 {
	return float64(p.runEnergyUnits) / 100
}

// SetRunEnergy sets run energy from a percent, following
// Player.ts:1261-1264 (setRunEnergy): units =
// floor(percent*100 + 1e-6), via SetRunEnergyUnits. The +1e-6 hair
// keeps floor semantics for real fractions: a percent read back
// from units (0.29 * 100 is 28.999... in floating point) must not
// lose a unit on every relog. The clamp runs in the float domain
// before the int conversion so an extreme percent cannot overflow
// the conversion; TS clamps after flooring in setRunEnergyUnits,
// which SetRunEnergyUnits also does for its int input.
func (p *Player) SetRunEnergy(percent float64) {
	units := math.Floor(percent*100 + 1e-6)
	if math.IsNaN(units) || units < 0 {
		units = 0
	} else if units > RunEnergyMaxUnits {
		units = RunEnergyMaxUnits
	}
	p.SetRunEnergyUnits(int(units))
}

// GetRunEnergyUnits returns run energy in OSRS units (0-10,000), the
// precision the formulas and the client use.
// Player.ts:1267-1269 (getRunEnergyUnits). It satisfies the
// MovementOwner interface (the queue's drain reads it).
func (p *Player) GetRunEnergyUnits() int {
	return p.runEnergyUnits
}

// SetRunEnergyUnits sets run energy in OSRS units, clamped to
// 0..10,000. Player.ts:1271-1273 (setRunEnergyUnits: Math.max(0,
// Math.min(RUN_ENERGY_MAX_UNITS, Math.floor(units)))). Go int inputs
// are already integral, so there is no floor step — the same
// rationale as SetFixedViewDistance. It satisfies the MovementOwner
// interface (the queue's drain and bot pinning write through it).
func (p *Player) SetRunEnergyUnits(units int) {
	if units < 0 {
		units = 0
	} else if units > RunEnergyMaxUnits {
		units = RunEnergyMaxUnits
	}
	p.runEnergyUnits = units
}

// AddRunEnergyUnits adds units to run energy, clamped to 0..10,000
// through SetRunEnergyUnits. Player.ts:1275-1277
// (addRunEnergyUnits). The restore branch in Player.Process is the
// only caller so far.
func (p *Player) AddRunEnergyUnits(units int) {
	p.SetRunEnergyUnits(p.runEnergyUnits + units)
}

// GetWeightKg returns the carried weight in whole kilograms, the
// fraction dropped toward zero as the client shows it.
// Player.ts:1314-1316 (getWeightKg: Math.trunc(getCarriedWeightGrams()
// / 1000)).
//
// LIVE COMPUTATION (phase 9): the weight is computed from the
// live equipment + inventory containers on every call
// (CarriedWeightKg, weight.go), exactly as TS computes it. It
// satisfies the MovementOwner interface (the queue's drain reads
// it, and Player.Process's restore branch sends it).
func (p *Player) GetWeightKg() int {
	return p.CarriedWeightKg()
}

// GetCurrentAgilityLevel returns the Agility current level the
// drain/restore formulas read: the Go form of
// getSkillManager().getCurrentLevel(Skill.AGILITY)
// (SkillManager.ts:425-433), read live from the player's
// SkillManager (phase 9). The sanitize rule lives on the manager:
// a negative level is written back as 0 and 0 is returned. A
// player with no SkillManager (only a zero-value Player, which
// NewPlayer never produces) reads 0. It satisfies the
// MovementOwner interface (the queue's drain reads it).
func (p *Player) GetCurrentAgilityLevel() int {
	if p.skills == nil {
		return 0
	}
	return p.skills.CurrentLevel(SkillAgility)
}
