package game

import "testing"

// TestMobileIsPlayerAsPlayer pins the Mobile kind predicate seam
// (Mobile.ts:967 isPlayer, :981 getAsPlayer): a player-bound
// Mobile answers true and yields its Player; an NPC-bound
// Mobile answers false and yields nil; an unbound Mobile and a
// nil Mobile fail closed (false/nil), the unknownCombatant
// answer. Binding happens through the typed GetCombat
// (combat.go), as in the combat pipeline.
func TestMobileIsPlayerAsPlayer(t *testing.T) {
	loc := Location{X: 3200, Y: 3200, Z: 0}

	p := NewPlayer("seeker", loc)
	p.GetCombat() // binds the player identity
	m := &p.Mobile
	if !m.IsPlayer() {
		t.Error("player-bound Mobile IsPlayer() = false, want true")
	}
	if got := m.AsPlayer(); got != p {
		t.Errorf("player-bound Mobile AsPlayer() = %v, want the player", got)
	}

	n := NewNPC(1, loc)
	n.GetCombat() // binds the NPC identity
	nm := &n.Mobile
	if nm.IsPlayer() {
		t.Error("NPC-bound Mobile IsPlayer() = true, want false")
	}
	if got := nm.AsPlayer(); got != nil {
		t.Errorf("NPC-bound Mobile AsPlayer() = %v, want nil", got)
	}

	// A bare Mobile whose owner never bound an identity fails
	// closed, even though a Player value embeds it.
	bare := NewPlayer("unbound", loc)
	bm := &bare.Mobile
	if bm.IsPlayer() {
		t.Error("unbound Mobile IsPlayer() = true, want false")
	}
	if got := bm.AsPlayer(); got != nil {
		t.Errorf("unbound Mobile AsPlayer() = %v, want nil", got)
	}

	var nilMobile *Mobile
	if nilMobile.IsPlayer() {
		t.Error("nil Mobile IsPlayer() = true, want false")
	}
	if got := nilMobile.AsPlayer(); got != nil {
		t.Errorf("nil Mobile AsPlayer() = %v, want nil", got)
	}
}

// TestMobileIsPlayerShadowing pins that the typed receivers keep
// their own answers: Player.IsPlayer/AsPlayer and
// NPC.IsPlayer/AsPlayer shadow the Mobile methods and do not
// depend on identity binding.
func TestMobileIsPlayerShadowing(t *testing.T) {
	loc := Location{X: 3200, Y: 3200, Z: 0}
	p := NewPlayer("shadow", loc)
	if !p.IsPlayer() || p.AsPlayer() != p {
		t.Error("Player.IsPlayer/AsPlayer must answer true/self without binding")
	}
	n := NewNPC(1, loc)
	if n.IsPlayer() || n.AsPlayer() != nil {
		t.Error("NPC.IsPlayer/AsPlayer must answer false/nil without binding")
	}
}
