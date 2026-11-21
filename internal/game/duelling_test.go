package game

import (
	"testing"
	"time"
)

func TestDuelingAccessorIdentity(t *testing.T) {
	p := NewPlayer("Alice", Location{X: 3200, Y: 3200, Z: 0})
	if p.Dueling() == nil || p.Dueling() != p.Dueling() {
		t.Fatal("Dueling() must return the same instance on repeat access")
	}
	q := NewPlayer("Bob", Location{X: 3200, Y: 3200, Z: 0})
	if p.Dueling() == q.Dueling() {
		t.Fatal("distinct players share a Dueling instance")
	}
	var nilPlayer *Player
	if nilPlayer.Dueling() != nil {
		t.Fatal("nil player Dueling() must be nil")
	}
}

func TestDuelingDefaults(t *testing.T) {
	d := NewDueling()
	if d.State() != DuelStateNone || d.Interact() != nil || d.InDuel() {
		t.Fatalf("defaults: state=%v interact=%v inDuel=%v", d.State(), d.Interact(), d.InDuel())
	}
	if len(d.Rules()) != 28 {
		t.Fatalf("rules length = %d, want 28", len(d.Rules()))
	}
	for i, rule := range d.Rules() {
		if rule {
			t.Fatalf("rule %d set by default", i)
		}
	}
	if d.ButtonDelay() == nil || !d.ButtonDelay().Finished() {
		t.Fatal("button delay must start stopped")
	}
}

func TestDuelingRulesAreLive(t *testing.T) {
	d := NewDueling()
	// The Duel Arena mutates the returned array in place; the
	// mutation is the duel's state.
	d.Rules()[DuelRuleNoMovement.ButtonID()] = true
	if !d.RuleSet(DuelRuleNoMovement) {
		t.Fatal("rule set through the returned slice must read back")
	}
	for i := range d.Rules() {
		d.Rules()[i] = false
	}
	if d.RuleSet(DuelRuleNoMovement) {
		t.Fatal("rules cleared through the returned slice must read back cleared")
	}
}

func TestDuelingInDuel(t *testing.T) {
	d := NewDueling()
	for _, state := range []DuelState{
		DuelStateNone, DuelStateRequestedDuel, DuelStateDuelScreen,
		DuelStateAcceptedDuelScreen, DuelStateConfirmScreen,
		DuelStateAcceptedConfirmScreen,
	} {
		d.SetState(state)
		if d.InDuel() {
			t.Fatalf("InDuel() true in state %v", state)
		}
	}
	for _, state := range []DuelState{DuelStateStartingDuel, DuelStateInDuel} {
		d.SetState(state)
		if !d.InDuel() {
			t.Fatalf("InDuel() false in state %v", state)
		}
	}
}

func TestDuelRuleIDs(t *testing.T) {
	for _, tc := range []struct {
		rule   DuelRule
		button int
		config int
		equip  int
	}{
		{DuelRuleNoForfeit, 0, 1, -1},
		{DuelRuleNoMovement, 1, 2, -1},
		{DuelRuleLockWeapon, 2, 4, -1},
		{DuelRuleShowInventories, 3, 8, -1},
		{DuelRuleNoRanged, 4, 16, -1},
		{DuelRuleNoMelee, 5, 32, -1},
		{DuelRuleNoMagic, 6, 64, -1},
		{DuelRuleNoPotions, 7, 128, -1},
		{DuelRuleNoFood, 8, 256, -1},
		{DuelRuleNoPrayer, 9, 512, -1},
		{DuelRuleFunWeapons, 12, 4096, -1},
		{DuelRuleNoSpecialAttacks, 13, 8192, -1},
		{DuelRuleNoHelm, 14, 16384, HeadSlot},
		{DuelRuleNoCape, 15, 32768, CapeSlot},
		{DuelRuleNoAmulet, 16, 65536, AmuletSlot},
		{DuelRuleNoWeapon, 17, 131072, WeaponSlot},
		{DuelRuleNoBody, 18, 262144, BodySlot},
		{DuelRuleNoShield, 19, 524288, ShieldSlot},
		{DuelRuleNoLegs, 21, 2097152, LegSlot},
		{DuelRuleNoGloves, 23, 8388608, HandsSlot},
		{DuelRuleNoBoots, 24, 16777216, FeetSlot},
		{DuelRuleNoRing, 26, 67108864, RingSlot},
		{DuelRuleNoAmmunition, 27, 134217728, AmmunitionSlot},
	} {
		if tc.rule.ButtonID() != tc.button || tc.rule.ConfigID() != tc.config || tc.rule.EquipmentSlot() != tc.equip {
			t.Fatalf("rule bit %d: button=%d config=%d equip=%d, want %d/%d/%d",
				tc.button, tc.rule.ButtonID(), tc.rule.ConfigID(), tc.rule.EquipmentSlot(),
				tc.button, tc.config, tc.equip)
		}
	}
}

func TestDuelingInteract(t *testing.T) {
	a := NewPlayer("Alice", Location{X: 3200, Y: 3200, Z: 0})
	b := NewPlayer("Bob", Location{X: 3200, Y: 3200, Z: 0})
	a.Dueling().SetInteract(b)
	if a.Dueling().Interact() != b {
		t.Fatal("interact must round-trip")
	}
	a.Dueling().SetInteract(nil)
	if a.Dueling().Interact() != nil {
		t.Fatal("interact must clear")
	}
}

func TestTradeHookSeamNilSafe(t *testing.T) {
	// With the zero seam, every seam read in Trading is a nil
	// check: the full flow runs, drops its external sends, and
	// never saves.
	withTradeHooks(t, TradeHooks{})
	a, b, _, _, clock := tradePair(t, newTradeTestDefs())
	a.Inventory().AddItem(NewItem(995, 100))
	a.Trading().RequestTrade(b)
	b.Trading().RequestTrade(a)
	a.Trading().HandleItem(995, 50, 0, a.Inventory(), a.Trading().Container())
	a.Trading().AcceptTrade()
	b.Trading().AcceptTrade()
	clock.now = clock.now.Add(2 * time.Second)
	a.Trading().AcceptTrade()
	b.Trading().AcceptTrade()
	if b.Inventory().GetAmount(995) != 50 {
		t.Fatalf("Bob coins = %d, want 50", b.Inventory().GetAmount(995))
	}
}

func TestTradeSenderNilSafe(t *testing.T) {
	// A player with no session, and a player whose session
	// lacks the TradeSender extension, both yield a nil sender.
	p := NewPlayer("Alice", Location{X: 3200, Y: 3200, Z: 0})
	if p.tradeSender() != nil {
		t.Fatal("session-less player must have no trade sender")
	}
	p.Session = newFakeSession()
	if p.tradeSender() != nil {
		t.Fatal("plain session must have no trade sender")
	}
	s := newTradeTestSession()
	p.Session = s
	if p.tradeSender() == nil {
		t.Fatal("trade session must yield a sender")
	}
	var nilPlayer *Player
	if nilPlayer.tradeSender() != nil {
		t.Fatal("nil player must have no trade sender")
	}
}
