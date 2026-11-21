package game

// Combat bridge for NPC aggression: the AggressionCombat
// implementation the aggression loop engages through
// (NpcAggression.ts:15-100). Worker 1's npc_aggression.go
// defines the interface and the default no-combat bridge;
// this file installs the real one, backed by the combat core.
//
// TS grounding: NpcAggression checks CombatFactory.inCombat
// and canAttack, then starts the attack with Combat.attack.
// The bridge collapses the CanAttackResponse to the boolean
// the aggression loop needs (CAN_ATTACK only), exactly as
// the interface contract documents.

// combatAggressionBridge is the combat-backed AggressionCombat.
type combatAggressionBridge struct{}

// InCombat reports whether m is attacking or being attacked
// (CombatFactory.ts:1237-1239, via InCombat).
func (combatAggressionBridge) InCombat(m *Mobile) bool {
	return InCombat(m)
}

// CanAttack reports whether npc may attack target: the
// CAN_ATTACK answer of CanAttack with no method (the
// method's own checks do not apply to the aggression
// decision; CanAttackPermission is nil-method safe). The
// typed GetCombat calls bind both fighters' combat
// identities first: ValidTarget fails closed on an unbound
// identity (see combat.go), and aggression must not refuse
// every attack for a fighter that simply never fought.
func (combatAggressionBridge) CanAttack(npc *NPC, target *Player) bool {
	if npc == nil || target == nil {
		return false
	}
	npc.GetCombat()
	target.GetCombat()
	return CanAttack(&npc.Mobile, &target.Mobile, nil, false, false) == CanAttackOK
}

// Attack starts npc attacking target (Combat.ts:269,
// attack), through the typed GetCombat so the NPC identity
// is bound before the attack resolves its method.
func (combatAggressionBridge) Attack(npc *NPC, target *Player) {
	if npc == nil || target == nil {
		return
	}
	npc.GetCombat().Attack(&target.Mobile)
}

// InstallAggressionCombatBridge installs the combat-backed
// aggression bridge. Call once at boot, before ticking
// starts; the cmd/server wiring calls it from buildServer.
// Until installed, the default bridge decides but never
// engages (see npc_aggression.go).
func InstallAggressionCombatBridge() {
	SetAggressionCombatBridge(combatAggressionBridge{})
}
