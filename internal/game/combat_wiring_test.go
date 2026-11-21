package game

import "testing"

// TestInteractionIndexFor pins the sync interaction-index
// encoding (PlayerSession.ts:101): players carry the
// 0x10000 offset, NPCs their plain index, and anything
// without a bound identity (nil, unbound) reads as -1.
func TestInteractionIndexFor(t *testing.T) {
	if got := InteractionIndexFor(nil); got != -1 {
		t.Fatalf("nil = %d, want -1", got)
	}
	p := NewPlayer("alice", Location{X: 10, Y: 10, Z: 0})
	if got := InteractionIndexFor(&p.Mobile); got != -1 {
		t.Fatalf("unbound player = %d, want -1", got)
	}
	n := NewNPC(1, Location{X: 10, Y: 10, Z: 0})
	if got := InteractionIndexFor(&n.Mobile); got != -1 {
		t.Fatalf("unbound NPC = %d, want -1", got)
	}
	// Bind the typed identities and assign indices the way
	// the world lists do.
	p.GetCombat()
	n.GetCombat()
	p.SetIndex(3)
	n.SetIndex(7)
	if got := InteractionIndexFor(&p.Mobile); got != PlayerInteractionOffset+3 {
		t.Fatalf("player = %d, want %d", got, PlayerInteractionOffset+3)
	}
	if got := InteractionIndexFor(&n.Mobile); got != 7 {
		t.Fatalf("NPC = %d, want 7", got)
	}
	// NPC.InteractionIndex reports the TARGET's index: -1
	// with no interaction, the player's offset index once
	// the NPC interacts with the bound player.
	if got := n.InteractionIndex(); got != -1 {
		t.Fatalf("NPC.InteractionIndex with no target = %d, want -1", got)
	}
	n.SetMobileInteraction(p)
	if got := n.InteractionIndex(); got != PlayerInteractionOffset+3 {
		t.Fatalf("NPC.InteractionIndex = %d, want %d", got, PlayerInteractionOffset+3)
	}
}

// TestPlayerProcessSeedsAggressionTolerance pins the combat
// phase wiring in Player.Process: one tick seeds the
// tolerance attribute (AggressionTolerance.update via
// ProcessPlayerAggression), proving the aggression block
// runs inside Process.
func TestPlayerProcessSeedsAggressionTolerance(t *testing.T) {
	InstallAggressionCombatBridge()
	t.Cleanup(func() { SetAggressionCombatBridge(nil) })
	p := NewPlayer("alice", Location{X: 3089, Y: 3524, Z: 0})
	p.SetRegistered(true)
	p.Process(1)
	if !p.HasAttribute("npc-aggression:tolerance") {
		t.Fatal("one Process tick must seed the tolerance attribute")
	}
}
