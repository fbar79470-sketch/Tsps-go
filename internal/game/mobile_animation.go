package game

// Mobile animation/graphic state: the port of
// Mobile.performAnimation / performGraphic /
// performGraphicInSlot / delayedAnimation
// (Mobile.ts:369-442), the Animation model
// (model/Animation.ts), the Graphic model
// (model/Graphic.ts) and the GraphicHeight values
// (model/GraphicHeight.ts), plus the attack/block/death
// animation accessors (Player.ts:333-348,
// NPC.ts:677-685).
//
// The state lives on Mobile and is per-tick: ResetUpdating
// clears it (Mobile.ts:264-278), after the sync snapshot
// has been taken (world.go flushes before resetting).
// Animation/graphic producers (combat methods, Worker B)
// call PerformAnimation / PerformGraphic; they never set
// the state directly.

// AnimationPriority is Priority (model/Priority.ts):
// LOW 0, MEDIUM 1, HIGH 2.
type AnimationPriority int

const (
	// AnimationPriorityLow is Priority.LOW.
	AnimationPriorityLow AnimationPriority = iota
	// AnimationPriorityMedium is Priority.MEDIUM.
	AnimationPriorityMedium
	// AnimationPriorityHigh is Priority.HIGH.
	AnimationPriorityHigh
)

// Graphic heights (model/GraphicHeight.ts). The
// unexported names match the call sites Worker B already
// landed in combat_ranged.go; the values are the TS
// enum's.
const (
	graphicHeightLow         = 0
	graphicHeightMiddle      = 50
	graphicHeightHigh        = 100
	graphicHeightSurgeCast   = 92
	graphicHeightSurgeImpact = 124
	// graphicHeightArcherDraw is GraphicHeight.
	// ARCHER_DRAW: captured on every archer's arrow-draw
	// spotanim (rsprox 1579, 4101, 470-476, ...).
	graphicHeightArcherDraw = 96
)

// ResetAnimationID is Animation.DEFAULT_RESET_ANIMATION's
// id (model/Animation.ts): the reset marker, 65535. It is
// not a sequence the cache has a config for, so content
// that asks for one (a teleport, a death) means to stop
// what plays.
const ResetAnimationID = 65535

// Animation is an animation id plus its delay and
// priority (model/Animation.ts). The constructor default
// is delay 0, priority LOW; PerformAnimation builds
// exactly that shape.
type Animation struct {
	ID       int
	Delay    int
	Priority AnimationPriority
}

// Graphic is a spotanim id plus its height, delay and
// priority (model/Graphic.ts). The constructor default
// is delay 0, height LOW, priority LOW; PerformGraphic
// builds exactly that shape.
type Graphic struct {
	ID       int
	Height   int
	Delay    int
	Priority AnimationPriority
}

// ---- Sequence-priority seams (NpcAnimationScanner) ----
//
// The game package never imports internal/cache (the
// established seam convention), so the sequence lookups
// the priority rules need arrive as package-level funcs
// the coordinator installs at boot from the cache-backed
// scanner (InstallSequenceAnimations, cmd/server).
// The defaults are the no-cache behavior: no sequence has
// a configured forced priority (5, the scanner's -1 -> 5)
// or standing priority (-1, unset).

// sequenceForcedPrioritySource, when set, answers
// getSequenceForcedPriority (NpcAnimationScanner.ts).
var sequenceForcedPrioritySource func(id int) int

// SetSequenceForcedPrioritySource installs the
// forced-priority lookup. Call once at boot, before
// ticking; a nil source restores the no-cache default.
func SetSequenceForcedPrioritySource(source func(id int) int) {
	sequenceForcedPrioritySource = source
}

// sequenceForcedPriority returns the sequence's interrupt
// priority, 5 when unset (attacks are 6, blocks 5).
func sequenceForcedPriority(id int) int {
	if sequenceForcedPrioritySource != nil {
		return sequenceForcedPrioritySource(id)
	}
	return 5
}

// sequencePrioritySource, when set, answers
// getSequencePriority (NpcAnimationScanner.ts).
var sequencePrioritySource func(id int) int

// SetSequencePrioritySource installs the standing-priority
// lookup. Call once at boot, before ticking; a nil source
// restores the no-cache default (-1, unset).
func SetSequencePrioritySource(source func(id int) int) {
	sequencePrioritySource = source
}

// sequencePriority returns the sequence's standing
// priority, -1 when unset (1 means the client drops the
// seq when the actor takes a step).
func sequencePriority(id int) int {
	if sequencePrioritySource != nil {
		return sequencePrioritySource(id)
	}
	return -1
}

// delayedAnimationSubmit, when set, schedules a
// one-shot task after delay ticks (TaskManager.submit of
// a MobileTask in TypeScript). It is installed at boot
// from the world's TaskManager; with none installed a
// delayed animation is dropped (standalone fighters and
// unit tests have no task manager — the same fail-closed
// degradation as the other uninstalled world seams).
var delayedAnimationSubmit func(delay int, action func())

// SetDelayedAnimationSubmit installs the delayed-task
// scheduler. Call once at boot, before ticking; a nil
// submitter restores the dropped-task default.
func SetDelayedAnimationSubmit(submit func(delay int, action func())) {
	delayedAnimationSubmit = submit
}

// PerformAnimation plays an animation id after delay
// ticks (Mobile.performAnimation, Mobile.ts:369-392, via
// the Animation(id, delay) constructor: priority LOW).
// A higher-priority existing animation wins; at equal
// priority the cache's interrupt priority decides (see
// performAnimationValue). A blocked animation is never
// measured as a stall. The stall measurement runs here
// (see measureAnimation): in TypeScript every
// performAnimation on a Player dispatches virtually to
// Player.performAnimation, which measures the accepted
// animation — Mobile callers (combat methods hold a
// *Mobile) reach the same behavior through this method.
func (m *Mobile) PerformAnimation(id, delay int) {
	anim := &Animation{ID: id, Delay: delay, Priority: AnimationPriorityLow}
	if m.performAnimationValue(anim) {
		m.measureAnimation(anim)
	}
}

// measureAnimation measures an accepted animation as a
// swing stall (Combat.onAnimationStarted, via
// Player.performAnimation, Mobile.ts:450-463 /
// Player.ts:456-462): only for players, only when an
// attack armed the measurement this cycle —
// OnAnimationStarted enforces both, so this is a no-op
// for NPCs, unarmed callers, and animations the priority
// rules turned away. The arrival-animation release path
// calls performAnimationValue directly instead, matching
// TypeScript's super.performAnimation there, which never
// measures.
func (m *Mobile) measureAnimation(anim *Animation) {
	if m == nil || anim == nil || !m.IsPlayer() {
		return
	}
	m.GetCombat().OnAnimationStarted(anim.ID)
}

// performAnimationValue is the Mobile.performAnimation
// state machine (Mobile.ts:369-392), reporting whether
// the animation was accepted. A nil animation is refused
// (TypeScript returns early on null).
func (m *Mobile) performAnimationValue(anim *Animation) bool {
	if m == nil || anim == nil {
		return false
	}
	if m.animation != nil {
		if m.animation.Priority > anim.Priority {
			return false
		}
		// Two on one tick at the same priority: the
		// cache's interrupt priority decides, as the
		// client does for one already playing, so a block
		// never replaces this tick's swing. The reset
		// marker is not a sequence the cache has a config
		// for, so it has nothing to arbitrate with:
		// content that asks for one (a teleport, a death)
		// means to stop what plays, and the stall measured
		// from the swing it clears ends with it.
		if m.animation.Priority == anim.Priority && anim.ID >= 0 &&
			anim.ID != ResetAnimationID && m.animation.ID >= 0 &&
			sequenceForcedPriority(anim.ID) < sequenceForcedPriority(m.animation.ID) {
			return false
		}
	}
	m.animation = anim
	m.animationGeneration++
	m.Flags = m.Flags.Set(FlagAnimation)
	return true
}

// PerformGraphic plays a spotanim at height after delay
// ticks (Mobile.performGraphic, Mobile.ts:394-402, via
// the Graphic(id, delay, height) constructor: priority
// LOW). A higher-priority existing graphic wins. It
// reports whether the graphic was accepted — the Go form
// of TS's `target.getGraphic() === delayed` identity
// check in MagicCombatMethod.queueProjectileGraphic
// (MagicCombatMethod.ts:211): the delayed graphic is the
// target's graphic exactly when it was accepted, since
// every play allocates a fresh Graphic.
func (m *Mobile) PerformGraphic(id, height, delay int) bool {
	return m.performGraphicValue(&Graphic{ID: id, Height: height, Delay: delay, Priority: AnimationPriorityLow})
}

// performGraphicValue is the Mobile.performGraphic state
// machine (Mobile.ts:394-402). A nil graphic is refused.
func (m *Mobile) performGraphicValue(graphic *Graphic) bool {
	if m == nil || graphic == nil {
		return false
	}
	if m.graphic != nil {
		if m.graphic.Priority > graphic.Priority {
			return false
		}
	}
	m.graphic = graphic
	m.Flags = m.Flags.Set(FlagGraphic)
	return true
}

// PerformGraphicInSlot plays a graphic in one of the
// actor's spotanim slots, which show at once
// (Mobile.performGraphicInSlot, Mobile.ts:413-421).
// Slot 0 is PerformGraphic's; any other slot is stored
// in the slot map. The slot is truncated to a byte, as
// TypeScript's `Math.trunc(slot) & 0xff` does. A graphic
// cannot be cleared through this signature (TypeScript
// takes Graphic | null); ClearGraphicInSlot is the
// null-graphic form.
func (m *Mobile) PerformGraphicInSlot(slot, id, height, delay int) {
	if m == nil {
		return
	}
	index := slot & 0xff
	if index == 0 {
		m.PerformGraphic(id, height, delay)
		return
	}
	if m.slotGraphics == nil {
		m.slotGraphics = make(map[int]*Graphic)
	}
	m.slotGraphics[index] = &Graphic{ID: id, Height: height, Delay: delay, Priority: AnimationPriorityLow}
	m.Flags = m.Flags.Set(FlagGraphic)
}

// ClearGraphicInSlot clears a slot graphic
// (Mobile.performGraphicInSlot with a null graphic,
// Mobile.ts:413-421): the slot stays in the map as a nil
// entry, which the sync layer sends as id -1 (a cleared
// slot, PlayerSession.graphicViews). Slot 0 is a no-op:
// TypeScript's null-graphic branch only performs a
// non-null graphic there.
func (m *Mobile) ClearGraphicInSlot(slot int) {
	if m == nil {
		return
	}
	index := slot & 0xff
	if index == 0 {
		return
	}
	if m.slotGraphics == nil {
		m.slotGraphics = make(map[int]*Graphic)
	}
	m.slotGraphics[index] = nil
	m.Flags = m.Flags.Set(FlagGraphic)
}

// WithdrawGraphicInSlot takes back a graphic queued this
// tick in a slot other than 0, so none is sent there
// (Mobile.withdrawGraphicInSlot, Mobile.ts:426-430). It
// deletes the map entry (unlike ClearGraphicInSlot, which
// leaves a nil entry that syncs as id -1), and sets no
// flag: the flag was set when the graphic was queued.
func (m *Mobile) WithdrawGraphicInSlot(slot int) {
	if m == nil || m.slotGraphics == nil {
		return
	}
	delete(m.slotGraphics, slot&0xff)
}

// DelayedAnimation plays an animation after ticks ticks
// (Mobile.delayedAnimation, Mobile.ts:434-438). With no
// task scheduler installed the animation is dropped (see
// delayedAnimationSubmit).
func (m *Mobile) DelayedAnimation(id, delay, ticks int) {
	if m == nil || delayedAnimationSubmit == nil {
		return
	}
	delayedAnimationSubmit(ticks, func() { m.PerformAnimation(id, delay) })
}

// DelayedGraphic plays a graphic after ticks ticks
// (Mobile.delayedGraphic, Mobile.ts:440-442). With no
// task scheduler installed the graphic is dropped (see
// delayedAnimationSubmit).
func (m *Mobile) DelayedGraphic(id, height, delay, ticks int) {
	if m == nil || delayedAnimationSubmit == nil {
		return
	}
	delayedAnimationSubmit(ticks, func() { m.PerformGraphic(id, height, delay) })
}

// Animation returns this tick's animation, or nil
// (Mobile.getAnimation, Mobile.ts:535-537).
func (m *Mobile) Animation() *Animation {
	if m == nil {
		return nil
	}
	return m.animation
}

// AnimationGeneration returns the animation generation
// (Mobile.getAnimationGeneration, Mobile.ts:539-541): it
// bumps on every accepted animation.
func (m *Mobile) AnimationGeneration() int {
	if m == nil {
		return 0
	}
	return m.animationGeneration
}

// Graphic returns this tick's graphic, or nil
// (Mobile.getGraphic, Mobile.ts:531-533).
func (m *Mobile) Graphic() *Graphic {
	if m == nil {
		return nil
	}
	return m.graphic
}

// SlotGraphics returns this tick's slot graphics
// (Mobile.getSlotGraphics, Mobile.ts:431-433): a copy of
// the slot map, so callers cannot mutate the mobile's
// state. A nil value is a cleared slot (syncs as id -1);
// slot 0 never appears here (it is Graphic()).
func (m *Mobile) SlotGraphics() map[int]*Graphic {
	if m == nil || m.slotGraphics == nil {
		return nil
	}
	out := make(map[int]*Graphic, len(m.slotGraphics))
	for slot, graphic := range m.slotGraphics {
		out[slot] = graphic
	}
	return out
}

// ---- Attack/block/death animation accessors ----

// attackTypeNames is ATTACK_TYPE_NAMES
// (WeaponProfile.ts:14): the fight-type bonus-type names
// item data uses, indexed by FightType.BonusType
// (BonusAttackStab..BonusAttackRange).
var attackTypeNames = [...]string{"stab", "slash", "crush", "magic", "range"}

// GetAttackAnim returns the attack animation for an attack
// on target (Player.getAttackAnim, Player.ts:333-338 /
// NPC.getAttackAnim, NPC.ts:677-679). Players resolve the
// weapon's animation (see playerAttackAnim); NPCs read
// their current definition. An unknown fighter (a Mobile
// with no bound identity) has no attack animation: -1.
func (m *Mobile) GetAttackAnim(target *Mobile) int {
	if m == nil {
		return -1
	}
	c := combatantFor(m)
	if c.IsPlayer() {
		return playerAttackAnim(c.Player(), target)
	}
	if n := c.NPC(); n != nil {
		if d := npcCombatDefinition(n); d != nil {
			return d.AttackAnim()
		}
		// NpcDefinition.ts:54: the DEFAULT attackAnim.
		return 422
	}
	return -1
}

// playerAttackAnim is Player.getAttackAnim
// (Player.ts:333-338) via WeaponProfiles.attackAnimation
// (WeaponProfile.ts:111-119), in the TS layer order:
//  1. the profile registered for the wielded item id
//     (weapon_profile.go, layer 1),
//  2. the item's own data (attackAnim in item-gameplay.json,
//     ItemDefinition.getAttackAnim, ItemDefinition.ts:308-311),
//  3. the profile for the wielded weapon's type
//     (weapon_profile.go, layer 3),
//  4. the fallback (the style's animation against the target
//     kind, FightType.AnimationAgainst).
//
// Against an NPC each profile layer prefers its
// npcAttackAnimation (WeaponProfile.ts:115-116). Where a
// profile and the item def agree (the whip's 1658, carried by
// both the WHIP profile and the style) the result is unchanged
// from before this layering; where they disagree the profile
// now wins, as in TS — with the shipped data the live changes
// are the ascension crossbows (4230/7552 -> 15113/15114), the
// black chinchompa against players (2779 -> 7618), the
// blisterwood stake (806 -> 7617), and javelins (806 -> 929,
// once the weapon-interface seam covers them; see
// weapon_profile.go).
func playerAttackAnim(p *Player, target *Mobile) int {
	if p == nil {
		return -1
	}
	fightType := p.GetCombat().FightType()
	againstNPC := target != nil && !target.IsPlayer()
	attackType := ""
	if bt := fightType.BonusType; bt >= 0 && bt < len(attackTypeNames) {
		attackType = attackTypeNames[bt]
	}
	weaponID := -1
	if eq := p.Equipment(); eq != nil {
		weaponID = eq.GetWeapon().ID()
	}
	// Layer 1: the profile registered for the item.
	if weaponID > 0 {
		if anim := weaponProfileAttackAnim(WeaponProfileForItem(weaponID), againstNPC); anim > 0 {
			return anim
		}
	}
	// Layer 2: the item's own data.
	if weaponID > 0 {
		if src, ok := p.ItemDefinitions().(WeaponInfoSource); ok && src != nil {
			if anim := src.WeaponAttackAnim(weaponID, attackType); anim > 0 {
				return anim
			}
		}
	}
	// Layer 3: the profile for the weapon's type.
	if iface := CurrentWeaponInterface(p); iface != nil {
		if anim := weaponProfileAttackAnim(WeaponProfileForWeapon(iface.Name), againstNPC); anim > 0 {
			return anim
		}
	}
	return fightType.AnimationAgainst(againstNPC)
}

// GetBlockAnim returns the block animation
// (Player.getBlockAnim, Player.ts:340-346 /
// NPC.getBlockAnim, NPC.ts:681-683). Players read the
// shield's definition when one is worn, else the
// weapon's (ItemDefinition.getBlockAnim, default 424);
// NPCs read their current definition. An unknown fighter
// has no block animation: -1.
func (m *Mobile) GetBlockAnim() int {
	if m == nil {
		return -1
	}
	c := combatantFor(m)
	if c.IsPlayer() {
		return playerBlockAnim(c.Player())
	}
	if n := c.NPC(); n != nil {
		if d := npcCombatDefinition(n); d != nil {
			return d.DefenceAnim()
		}
		// NpcDefinition.ts:55: the DEFAULT defenceAnim.
		return -1
	}
	return -1
}

// playerBlockAnim is Player.getBlockAnim
// (Player.ts:340-346): the shield when its id is above 0,
// else the weapon, and that item's block animation. With
// no item-definition source the DEFAULT shape's 424
// applies (ItemDefinition.ts:94).
func playerBlockAnim(p *Player) int {
	if p == nil {
		return 424
	}
	defID := -1
	if eq := p.Equipment(); eq != nil {
		if shield := eq.ItemAt(ShieldSlot); shield.ID() > 0 {
			defID = shield.ID()
		} else {
			defID = eq.GetWeapon().ID()
		}
	}
	if src, ok := p.ItemDefinitions().(WeaponInfoSource); ok && src != nil {
		return src.WeaponBlockAnim(defID)
	}
	return 424
}

// GetDeathAnim returns the death animation
// (NPC.getDeathAnim via its current definition;
// NpcDefinition.ts:56, the DEFAULT deathAnim 836). A
// player has no death-animation accessor in TypeScript:
// the player death animation 836 is Worker B's constant
// in the death path (combat_death.go), so this returns
// -1 for players. An unknown fighter returns -1.
func (m *Mobile) GetDeathAnim() int {
	if m == nil {
		return -1
	}
	c := combatantFor(m)
	if c.IsPlayer() {
		return -1
	}
	if n := c.NPC(); n != nil {
		if d := npcCombatDefinition(n); d != nil {
			return d.DeathAnim()
		}
		// NpcDefinition.ts:56: the DEFAULT deathAnim.
		return 836
	}
	return -1
}
