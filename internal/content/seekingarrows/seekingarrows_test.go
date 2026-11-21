package seekingarrows

import (
	"testing"

	"tsps-go/internal/game"
	"tsps-go/internal/plugins"
)

// Fixture ammunition ids. Only 33595 is real (the seeking dragon
// arrow); the rune arrow id is the TS Ammunition.RUNE_ARROW item
// id (RangedData.ts:32), used as the wrong-ammunition control.
const runeArrowItemID = 892

func newTestPlayer() *game.Player {
	p := game.NewPlayer("seeker", game.Location{X: 3200, Y: 3200, Z: 0})
	p.GetCombat() // binds the player identity, as the pipeline does
	return p
}

func newTestTarget() *game.Mobile {
	return &game.NewNPC(1, game.Location{X: 3201, Y: 3200, Z: 0}).Mobile
}

// armedEvent builds a hit-damage event that passes every guard:
// an accurate RANGED hit from a player with seeking dragon
// arrows and no active special, carrying the given damage.
func armedEvent(t *testing.T, damage int) (*game.Player, *plugins.CombatHitDamageEvent) {
	t.Helper()
	p := newTestPlayer()
	p.GetCombat().SetAmmunition(&game.AmmunitionData{ItemID: SeekingDragonArrowItemID})
	ev := &plugins.CombatHitDamageEvent{
		Attacker:   &p.Mobile,
		Target:     newTestTarget(),
		CombatType: plugins.CombatTypeRanged,
		Damage:     game.NewHitDamage(damage, game.HitMaskRed),
		Accurate:   true,
	}
	return p, ev
}

func damageOf(t *testing.T, ev *plugins.CombatHitDamageEvent) int {
	t.Helper()
	d, ok := ev.Damage.(*game.HitDamage)
	if !ok {
		t.Fatalf("event Damage = %T, want *game.HitDamage", ev.Damage)
	}
	return d.Damage()
}

// TestSeekingMinimumFloor pins the damage floor
// (SeekingArrows.plugin.js:10): accurate seeking-arrow hits land
// at 3 or higher, never below.
func TestSeekingMinimumFloor(t *testing.T) {
	for _, tc := range []struct{ in, want int }{
		{0, 3}, {1, 3}, {2, 3}, {3, 3}, {5, 5}, {10, 10},
	} {
		_, ev := armedEvent(t, tc.in)
		seekingMinimum(ev)
		if got := damageOf(t, ev); got != tc.want {
			t.Errorf("damage %d -> %d, want %d", tc.in, got, tc.want)
		}
	}
}

// TestSeekingMinimumGuards pins every guard in TS order
// (SeekingArrows.plugin.js:6-10): any failing guard leaves the
// damage untouched.
func TestSeekingMinimumGuards(t *testing.T) {
	newNPCAttacker := func() *game.Mobile {
		n := game.NewNPC(1, game.Location{X: 3200, Y: 3200, Z: 0})
		n.GetCombat() // binds the NPC identity
		return &n.Mobile
	}
	unboundPlayer := func() *game.Mobile {
		// A Player value whose identity was never bound
		// (typed GetCombat never called): AsPlayer fails
		// closed, so the handler returns.
		return &game.NewPlayer("unbound", game.Location{X: 3200, Y: 3200, Z: 0}).Mobile
	}

	cases := []struct {
		name   string
		mutate func(*game.Player, *plugins.CombatHitDamageEvent)
	}{
		{"inaccurate", func(_ *game.Player, ev *plugins.CombatHitDamageEvent) { ev.Accurate = false }},
		{"nil attacker", func(_ *game.Player, ev *plugins.CombatHitDamageEvent) { ev.Attacker = nil }},
		{"NPC attacker", func(_ *game.Player, ev *plugins.CombatHitDamageEvent) { ev.Attacker = newNPCAttacker() }},
		{"unbound player", func(_ *game.Player, ev *plugins.CombatHitDamageEvent) { ev.Attacker = unboundPlayer() }},
		{"melee", func(_ *game.Player, ev *plugins.CombatHitDamageEvent) { ev.CombatType = plugins.CombatTypeMelee }},
		{"magic", func(_ *game.Player, ev *plugins.CombatHitDamageEvent) { ev.CombatType = plugins.CombatTypeMagic }},
		{"special active", func(p *game.Player, _ *plugins.CombatHitDamageEvent) { p.GetCombat().SetSpecialActivated(true) }},
		{"wrong ammunition", func(p *game.Player, _ *plugins.CombatHitDamageEvent) {
			p.GetCombat().SetAmmunition(&game.AmmunitionData{ItemID: runeArrowItemID})
		}},
		{"noted form", func(p *game.Player, _ *plugins.CombatHitDamageEvent) {
			// 33596 is the noted seeking dragon arrow: no
			// Ammunition entry, so TS never matches it.
			p.GetCombat().SetAmmunition(&game.AmmunitionData{ItemID: 33596})
		}},
		{"nil ammunition", func(p *game.Player, _ *plugins.CombatHitDamageEvent) { p.GetCombat().SetAmmunition(nil) }},
		{"nil damage", func(_ *game.Player, ev *plugins.CombatHitDamageEvent) { ev.Damage = nil }},
		{"non-HitDamage damage", func(_ *game.Player, ev *plugins.CombatHitDamageEvent) { ev.Damage = "not damage" }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p, ev := armedEvent(t, 1)
			tc.mutate(p, ev)
			seekingMinimum(ev)
			if ev.Damage == nil {
				return // nil damage stays nil; nothing to read back
			}
			if d, ok := ev.Damage.(*game.HitDamage); ok && d.Damage() != 1 {
				t.Errorf("damage = %d, want untouched 1", d.Damage())
			}
		})
	}
}

// TestSeekingMinimumNilEvent pins the nil-event guard: the hub
// emit never passes a nil event, but a direct call must not
// panic.
func TestSeekingMinimumNilEvent(t *testing.T) {
	seekingMinimum(nil)
}

// TestPluginIdentity pins the plugin value verbatim
// (SeekingArrows.plugin.js:14-16).
func TestPluginIdentity(t *testing.T) {
	if Plugin.Name != "SeekingArrows" {
		t.Errorf("Name = %q, want SeekingArrows", Plugin.Name)
	}
	want := "Makes successful bow hits with seeking dragon arrows deal at least three damage."
	if Plugin.Description != want {
		t.Errorf("Description = %q, want %q", Plugin.Description, want)
	}
	if !Plugin.Members {
		t.Error("Members = false, want true")
	}
}

// TestHubRegistrationFires pins the hook through a members-world
// hub: the registered handler floors an accurate seeking-arrow
// hit, and a free world drops the members-only piece.
func TestHubRegistrationFires(t *testing.T) {
	hub := plugins.NewHub([]plugins.Plugin{Plugin}, plugins.HubOptions{MembersWorld: true})
	hub.Load()
	_, ev := armedEvent(t, 2)
	hub.EmitCombatHitDamage(ev)
	if got := damageOf(t, ev); got != 3 {
		t.Fatalf("hub-fired damage = %d, want 3", got)
	}

	free := plugins.NewHub([]plugins.Plugin{Plugin}, plugins.HubOptions{})
	free.Load()
	for _, name := range free.Loaded() {
		if name == "SeekingArrows" {
			t.Fatal("free world loaded SeekingArrows, want it members-gated out")
		}
	}
}
