package game

import "testing"

// Mobile animation/graphic state tests (Phase 29
// foundations): the Mobile.performAnimation /
// performGraphic / performGraphicInSlot priority rules
// (Mobile.ts:369-442), the Player arrival-hold override
// (Player.ts:446-463, 525-528), the stall measurement
// through OnAnimationStarted (Combat.ts:198), and the
// attack/block/death accessors (Player.ts:333-348,
// NPC.ts:677-685). Hand-built fixtures only.

func TestPerformAnimationPriorityRules(t *testing.T) {
	m := NewMobile(Location{X: 3200, Y: 3200})
	// A LOW animation is accepted and flags ANIMATION.
	m.PerformAnimation(422, 0)
	if m.Animation() == nil || m.Animation().ID != 422 || m.Animation().Delay != 0 {
		t.Fatalf("Animation = %+v, want id 422 delay 0", m.Animation())
	}
	if !m.Flags.Has(FlagAnimation) {
		t.Error("FlagAnimation must be set")
	}
	gen := m.AnimationGeneration()
	if gen != 1 {
		t.Errorf("generation = %d, want 1", gen)
	}
	// A second LOW animation at equal priority replaces
	// the first when the forced priorities tie (both
	// default 5 with no source installed).
	m.PerformAnimation(423, 2)
	if m.Animation().ID != 423 || m.Animation().Delay != 2 {
		t.Errorf("Animation = %+v, want id 423 delay 2", m.Animation())
	}
	if m.AnimationGeneration() != 2 {
		t.Errorf("generation = %d, want 2", m.AnimationGeneration())
	}
}

func TestPerformAnimationHigherPriorityWins(t *testing.T) {
	m := NewMobile(Location{X: 3200, Y: 3200})
	// Install a HIGH-priority animation directly (the
	// state machine takes Animation values; PerformAnimation
	// always builds LOW ones).
	m.performAnimationValue(&Animation{ID: 100, Priority: AnimationPriorityHigh})
	m.PerformAnimation(422, 0)
	if m.Animation().ID != 100 {
		t.Errorf("Animation = %d, want 100 (HIGH beats LOW)", m.Animation().ID)
	}
	// A HIGH animation replaces a LOW one.
	m.performAnimationValue(&Animation{ID: 200, Priority: AnimationPriorityLow})
	if m.Animation().ID != 100 {
		t.Fatalf("setup: Animation = %d, want 100", m.Animation().ID)
	}
	if !m.performAnimationValue(&Animation{ID: 201, Priority: AnimationPriorityHigh}) {
		t.Error("HIGH must replace LOW")
	}
	if m.Animation().ID != 201 {
		t.Errorf("Animation = %d, want 201", m.Animation().ID)
	}
}

func TestPerformAnimationForcedPriorityDecides(t *testing.T) {
	// Two LOW animations on one tick: the cache's
	// interrupt priority decides. Attacks are 6, blocks
	// 5, so a block never replaces this tick's swing.
	SetSequenceForcedPrioritySource(func(id int) int {
		if id == 422 {
			return 6
		}
		return 5
	})
	t.Cleanup(func() { SetSequenceForcedPrioritySource(nil) })
	m := NewMobile(Location{X: 3200, Y: 3200})
	m.PerformAnimation(422, 0)
	m.PerformAnimation(424, 0) // block (5) vs swing (6)
	if m.Animation().ID != 422 {
		t.Errorf("Animation = %d, want 422 (block must not cut the swing)", m.Animation().ID)
	}
	// The reverse order: an attack replaces a block.
	m2 := NewMobile(Location{X: 3200, Y: 3200})
	m2.PerformAnimation(424, 0)
	m2.PerformAnimation(422, 0)
	if m2.Animation().ID != 422 {
		t.Errorf("Animation = %d, want 422 (attack replaces block)", m2.Animation().ID)
	}
}

func TestPerformAnimationResetCutsThrough(t *testing.T) {
	// The reset marker (65535) is not a sequence the cache
	// has a config for: it stops what plays even against a
	// higher forced priority.
	SetSequenceForcedPrioritySource(func(id int) int {
		if id == 422 {
			return 6
		}
		return 5
	})
	t.Cleanup(func() { SetSequenceForcedPrioritySource(nil) })
	m := NewMobile(Location{X: 3200, Y: 3200})
	m.PerformAnimation(422, 0)
	m.PerformAnimation(ResetAnimationID, 0)
	if m.Animation().ID != ResetAnimationID {
		t.Errorf("Animation = %d, want reset 65535", m.Animation().ID)
	}
}

func TestPerformGraphicRules(t *testing.T) {
	m := NewMobile(Location{X: 3200, Y: 3200})
	m.PerformGraphic(100, 50, 1)
	if m.Graphic() == nil || m.Graphic().ID != 100 || m.Graphic().Height != 50 || m.Graphic().Delay != 1 {
		t.Fatalf("Graphic = %+v, want id 100 height 50 delay 1", m.Graphic())
	}
	if !m.Flags.Has(FlagGraphic) {
		t.Error("FlagGraphic must be set")
	}
	// A higher-priority existing graphic wins; equal
	// priority replaces (graphics have no forced-priority
	// arbitration in TS).
	m.performGraphicValue(&Graphic{ID: 200, Priority: AnimationPriorityHigh})
	m.PerformGraphic(101, 0, 0)
	if m.Graphic().ID != 200 {
		t.Errorf("Graphic = %d, want 200 (HIGH beats LOW)", m.Graphic().ID)
	}
}

func TestPerformGraphicInSlot(t *testing.T) {
	m := NewMobile(Location{X: 3200, Y: 3200})
	// Slot 0 is PerformGraphic's.
	m.PerformGraphicInSlot(0, 100, 0, 0)
	if m.Graphic() == nil || m.Graphic().ID != 100 {
		t.Fatalf("slot 0 Graphic = %+v, want id 100", m.Graphic())
	}
	if len(m.SlotGraphics()) != 0 {
		t.Errorf("slot map must stay empty for slot 0, got %v", m.SlotGraphics())
	}
	// Other slots land in the map and never touch slot 0.
	m.PerformGraphicInSlot(2, 200, 100, 1)
	m.PerformGraphicInSlot(3, 300, 0, 0)
	slots := m.SlotGraphics()
	if len(slots) != 2 || slots[2] == nil || slots[2].ID != 200 || slots[3] == nil || slots[3].ID != 300 {
		t.Errorf("slots = %v, want {2:200, 3:300}", slots)
	}
	if m.Graphic().ID != 100 {
		t.Errorf("slot 0 Graphic = %d, want 100", m.Graphic().ID)
	}
	// Clearing a slot leaves a nil entry (syncs as id -1);
	// withdrawing deletes it (syncs nothing).
	m.ClearGraphicInSlot(2)
	if slots = m.SlotGraphics(); slots[2] != nil {
		t.Errorf("cleared slot 2 = %+v, want nil entry", slots[2])
	}
	m.WithdrawGraphicInSlot(3)
	if slots = m.SlotGraphics(); len(slots) != 1 {
		t.Errorf("slots after withdraw = %v, want only slot 2", slots)
	}
	// Slot 0 clear is a no-op in TS.
	m.ClearGraphicInSlot(0)
	if m.Graphic() == nil || m.Graphic().ID != 100 {
		t.Errorf("slot 0 Graphic after clear = %+v, want id 100", m.Graphic())
	}
	// The returned map is a copy: mutating it does not
	// touch the mobile.
	slots[2] = &Graphic{ID: 999}
	if m.SlotGraphics()[2] != nil {
		t.Error("SlotGraphics must return a copy")
	}
}

func TestResetUpdatingClearsAnimationGraphic(t *testing.T) {
	m := NewMobile(Location{X: 3200, Y: 3200})
	m.PerformAnimation(422, 0)
	m.PerformGraphic(100, 0, 0)
	m.PerformGraphicInSlot(2, 200, 0, 0)
	m.ResetUpdating()
	if m.Animation() != nil || m.Graphic() != nil || len(m.SlotGraphics()) != 0 {
		t.Errorf("after ResetUpdating: anim=%+v graphic=%+v slots=%v", m.Animation(), m.Graphic(), m.SlotGraphics())
	}
	if m.Flags.Has(FlagAnimation) || m.Flags.Has(FlagGraphic) {
		t.Error("animation/graphic flags must clear")
	}
}

func TestDelayedAnimationSubmitter(t *testing.T) {
	var gotDelay int
	var gotAction func()
	SetDelayedAnimationSubmit(func(delay int, action func()) {
		gotDelay, gotAction = delay, action
	})
	t.Cleanup(func() { SetDelayedAnimationSubmit(nil) })
	m := NewMobile(Location{X: 3200, Y: 3200})
	m.DelayedAnimation(422, 3, 5)
	if gotDelay != 5 || gotAction == nil {
		t.Fatalf("submit = delay %d action nil=%v, want 5 non-nil", gotDelay, gotAction == nil)
	}
	gotAction()
	if m.Animation() == nil || m.Animation().ID != 422 || m.Animation().Delay != 3 {
		t.Errorf("delayed Animation = %+v, want id 422 delay 3", m.Animation())
	}
	// With no submitter installed the call is a safe
	// no-op, not a panic.
	SetDelayedAnimationSubmit(nil)
	m2 := NewMobile(Location{X: 3200, Y: 3200})
	m2.DelayedAnimation(422, 0, 1)
	if m2.Animation() != nil {
		t.Errorf("unsubmitted Animation = %+v, want nil", m2.Animation())
	}
	m2.DelayedGraphic(100, 0, 0, 1)
	if m2.Graphic() != nil {
		t.Errorf("unsubmitted Graphic = %+v, want nil", m2.Graphic())
	}
}

func TestPlayerPerformAnimationMeasuresStall(t *testing.T) {
	setCycle := setCombatCycle(t, 100)
	// The scanner's real value for seq 422 (unarmed punch)
	// is 2 ticks (38 client cycles); the source stands in
	// for it here, and the cmd/server wiring test proves
	// the real scanner is what gets installed.
	SetSequenceStallSource(func(id int) int {
		if id == 422 {
			return 2
		}
		return 0
	})
	t.Cleanup(func() { SetSequenceStallSource(nil) })
	p, _ := combatTestPlayer(t, "alice", 3200, 3200)
	combat := p.GetCombat()
	combat.ExpectAttackAnimation()
	p.PerformAnimation(422, 0)
	if got := combat.GetIncomingHitCycle(); got != 102 {
		t.Errorf("incoming-hit cycle = %d, want 102 (armed swing stalls 2 ticks)", got)
	}
	// A blocked animation is never measured: a HIGH
	// animation already plays, so the swing is refused.
	p2, _ := combatTestPlayer(t, "bob", 3200, 3200)
	p2.Mobile.performAnimationValue(&Animation{ID: 999, Priority: AnimationPriorityHigh})
	p2.GetCombat().ExpectAttackAnimation()
	p2.PerformAnimation(422, 0)
	if got := p2.GetCombat().GetIncomingHitCycle(); got != -1 {
		t.Errorf("blocked swing incoming-hit cycle = %d, want -1", got)
	}
	// An animation on a later cycle than the arm is never
	// measured either.
	p3, _ := combatTestPlayer(t, "carol", 3200, 3200)
	p3.GetCombat().ExpectAttackAnimation()
	setCycle(101)
	p3.PerformAnimation(422, 0)
	if got := p3.GetCombat().GetIncomingHitCycle(); got != -1 {
		t.Errorf("late swing incoming-hit cycle = %d, want -1", got)
	}
}

func TestNPCPerformAnimationNeverMeasuresStall(t *testing.T) {
	setCombatCycle(t, 100)
	SetSequenceStallSource(func(id int) int { return 2 })
	t.Cleanup(func() { SetSequenceStallSource(nil) })
	n := combatTestNPC(t, 1, 3200, 3200, nil)
	n.GetCombat().ExpectAttackAnimation()
	n.PerformAnimation(422, 0)
	if got := n.GetCombat().GetIncomingHitCycle(); got != -1 {
		t.Errorf("NPC incoming-hit cycle = %d, want -1", got)
	}
}

func TestPlayerArrivalAnimationHold(t *testing.T) {
	setCycle := setCombatCycle(t, 100)
	SetSequencePrioritySource(func(id int) int {
		if id == 999 {
			return 1
		}
		return -1
	})
	t.Cleanup(func() { SetSequencePrioritySource(nil) })
	p, _ := combatTestPlayer(t, "alice", 3200, 3200)
	// The player stepped this cycle: a priority-1 seq is
	// held, not played.
	p.movementQueue.lastMoveCycle = 100
	p.PerformAnimation(999, 0)
	if p.Animation() != nil {
		t.Errorf("held Animation = %+v, want nil", p.Animation())
	}
	if p.arrivalAnimation == nil || p.arrivalAnimation.ID != 999 {
		t.Errorf("arrivalAnimation = %+v, want id 999", p.arrivalAnimation)
	}
	// Next cycle, no step: the release plays it through
	// the Mobile state machine.
	p.movementQueue.movedCycle = false
	p.releaseArrivalAnimation()
	if p.Animation() == nil || p.Animation().ID != 999 {
		t.Errorf("released Animation = %+v, want id 999", p.Animation())
	}
	if p.arrivalAnimation != nil {
		t.Error("arrivalAnimation must clear on release")
	}
	// Held again, but the player stepped again: the release
	// drops it, as the client would.
	setCycle(101)
	p.movementQueue.lastMoveCycle = 101
	p.PerformAnimation(999, 0)
	p.movementQueue.movedCycle = true
	p.releaseArrivalAnimation()
	if p.Animation() == nil || p.Animation().ID != 999 {
		t.Errorf("Animation after dropped release = %+v, want the earlier 999 still playing", p.Animation())
	}
	if p.arrivalAnimation != nil {
		t.Error("arrivalAnimation must clear even when the release drops it")
	}
	// A non-priority-1 animation is never held, even on a
	// step tick.
	p.movementQueue.lastMoveCycle = 101
	p.PerformAnimation(422, 0)
	if p.Animation() == nil || p.Animation().ID != 422 {
		t.Errorf("Animation = %+v, want id 422 (not held)", p.Animation())
	}
}

func TestGetAttackAnimPlayer(t *testing.T) {
	defs := newCombatTestItemDefs()
	defs.attackAnims[4151] = map[string]int{"slash": 1658}
	// Unarmed: the fight-type fallback (UNARMED_PUNCH 422).
	p, _ := combatTestPlayer(t, "alice", 3200, 3200)
	p.SetItemDefinitions(defs)
	npc := combatTestNPC(t, 1, 3201, 3200, nil)
	if got := p.GetAttackAnim(&npc.Mobile); got != 422 {
		t.Errorf("unarmed vs NPC = %d, want 422", got)
	}
	if got := p.GetAttackAnim(nil); got != 422 {
		t.Errorf("unarmed vs nil = %d, want 422", got)
	}
	// A style with separate NPC animation: AnimationAgainst
	// picks it against NPCs, the plain animation otherwise.
	p.GetCombat().SetFightType(&FightType{
		Name: "TEST", Animation: 1234, NPCAnimation: 5678,
		BonusType: BonusAttackSlash, Style: FightStyleAccurate,
	})
	if got := p.GetAttackAnim(&npc.Mobile); got != 5678 {
		t.Errorf("styled vs NPC = %d, want 5678", got)
	}
	other, _ := combatTestPlayer(t, "bob", 3201, 3200)
	if got := p.GetAttackAnim(&other.Mobile); got != 1234 {
		t.Errorf("styled vs player = %d, want 1234", got)
	}
	// The item-def layer wins over the style fallback:
	// whip 4151 attacks 1658 on slash.
	p.Equipment().Set(WeaponSlot, NewItem(4151, 1))
	if got := p.GetAttackAnim(&npc.Mobile); got != 1658 {
		t.Errorf("whip vs NPC = %d, want 1658 (item def)", got)
	}
	// An item with no attack anim for the style falls back
	// to the style animation.
	p.Equipment().Set(WeaponSlot, NewItem(9999, 1))
	if got := p.GetAttackAnim(&npc.Mobile); got != 5678 {
		t.Errorf("unknown weapon vs NPC = %d, want 5678 (fallback)", got)
	}
	// With no item-definition source at all the fallback
	// still applies.
	p.SetItemDefinitions(nil)
	if got := p.GetAttackAnim(&npc.Mobile); got != 5678 {
		t.Errorf("no-source vs NPC = %d, want 5678", got)
	}
}

func TestGetBlockAnimPlayer(t *testing.T) {
	defs := newCombatTestItemDefs()
	defs.blockAnims[1171] = 500 // shield
	defs.blockAnims[4151] = 501 // weapon
	p, _ := combatTestPlayer(t, "alice", 3200, 3200)
	p.SetItemDefinitions(defs)
	// No shield: the weapon's block animation.
	p.Equipment().Set(WeaponSlot, NewItem(4151, 1))
	if got := p.GetBlockAnim(); got != 501 {
		t.Errorf("weapon block = %d, want 501", got)
	}
	// A shield overrides the weapon.
	p.Equipment().Set(ShieldSlot, NewItem(1171, 1))
	if got := p.GetBlockAnim(); got != 500 {
		t.Errorf("shield block = %d, want 500", got)
	}
	// With no item-definition source the DEFAULT 424
	// applies.
	p.SetItemDefinitions(nil)
	if got := p.GetBlockAnim(); got != 424 {
		t.Errorf("no-source block = %d, want 424", got)
	}
}

func TestGetAnimsNPC(t *testing.T) {
	def := newCombatFakeDef()
	def.attackAnim = 123
	def.defenceAnim = 456
	def.deathAnim = 789
	n := combatTestNPC(t, 1, 3200, 3200, def)
	if got := n.GetAttackAnim(nil); got != 123 {
		t.Errorf("NPC attack = %d, want 123", got)
	}
	if got := n.GetBlockAnim(); got != 456 {
		t.Errorf("NPC block = %d, want 456", got)
	}
	if got := n.GetDeathAnim(); got != 789 {
		t.Errorf("NPC death = %d, want 789", got)
	}
	// No definition installed: the NpcDefinition DEFAULTs
	// (attack 422, defence -1, death 836).
	bare := NewNPC(2, Location{X: 3200, Y: 3200})
	bare.GetCombat() // bind identity
	if got := bare.GetAttackAnim(nil); got != 422 {
		t.Errorf("default NPC attack = %d, want 422", got)
	}
	if got := bare.GetBlockAnim(); got != -1 {
		t.Errorf("default NPC block = %d, want -1", got)
	}
	if got := bare.GetDeathAnim(); got != 836 {
		t.Errorf("default NPC death = %d, want 836", got)
	}
}

func TestGetDeathAnimPlayer(t *testing.T) {
	// A player has no death-animation accessor in TS: the
	// death animation 836 is Worker B's constant in the
	// death path, so this returns -1.
	p, _ := combatTestPlayer(t, "alice", 3200, 3200)
	if got := p.GetDeathAnim(); got != -1 {
		t.Errorf("player death = %d, want -1", got)
	}
	// A bare Mobile with no bound identity has no anims.
	var m *Mobile
	if got := m.GetAttackAnim(nil); got != -1 {
		t.Errorf("nil attack = %d, want -1", got)
	}
	if got := m.GetBlockAnim(); got != -1 {
		t.Errorf("nil block = %d, want -1", got)
	}
	if got := m.GetDeathAnim(); got != -1 {
		t.Errorf("nil death = %d, want -1", got)
	}
}
