package fishing

import (
	"testing"

	"tsps-go/internal/game"
)

// RadasBlessing tests: the worn tier's chance, the
// members-world gate, and the extra-fish roll
// (RadasBlessing.Fishing.js). Fixtures only.

func TestRadasExtraFishChance(t *testing.T) {
	f, _ := newTestFishing(t)
	ids := f.ids
	cases := []struct {
		name  string
		bless int
		want  float64
	}{
		{"none worn", -1, 0},
		{"blessing 1", ids.RadasBlessing1, 0.02},
		{"blessing 2", ids.RadasBlessing2, 0.04},
		{"blessing 3", ids.RadasBlessing3, 0.06},
		{"blessing 4", ids.RadasBlessing4, 0.08},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p := newTestPlayer("alice", 3000, 3000)
			if c.bless >= 0 {
				p.Equipment().SetItem(game.AmmunitionSlot, game.NewItem(c.bless, 1))
			}
			if got := f.radasExtraFishChance(p); got != c.want {
				t.Fatalf("radasExtraFishChance = %v, want %v", got, c.want)
			}
		})
	}
	// A blessing in the inventory (not worn) gives nothing.
	p := newTestPlayer("bob", 3000, 3000)
	p.Inventory().Adds(ids.RadasBlessing4, 1)
	if got := f.radasExtraFishChance(p); got != 0 {
		t.Fatalf("carried blessing chance = %v, want 0", got)
	}
	// Free world: a worn blessing gives nothing.
	f.membersWorld = false
	p2 := newTestPlayer("carol", 3000, 3000)
	p2.Equipment().SetItem(game.AmmunitionSlot, game.NewItem(ids.RadasBlessing4, 1))
	if got := f.radasExtraFishChance(p2); got != 0 {
		t.Fatalf("free-world blessing chance = %v, want 0", got)
	}
	if got := f.radasExtraFishChance(nil); got != 0 {
		t.Fatalf("nil player chance = %v, want 0", got)
	}
	if got := (&fishing{membersWorld: true}).radasExtraFishChance(p2); got != 0 {
		t.Fatalf("no-ids chance = %v, want 0", got)
	}
}

func TestRollExtraFish(t *testing.T) {
	f, _ := newTestFishing(t)
	ids := f.ids
	p := newTestPlayer("alice", 3000, 3000)
	// flakeChance 1: always flakes, whatever is worn.
	for i := 0; i < 50; i++ {
		if got := f.rollExtraFish(p, 1); got != "flakes" {
			t.Fatalf("rollExtraFish(flake 1) = %q, want flakes", got)
		}
	}
	// flakeChance 0, nothing worn: always "".
	for i := 0; i < 50; i++ {
		if got := f.rollExtraFish(p, 0); got != "" {
			t.Fatalf("rollExtraFish(flake 0, no blessing) = %q, want \"\"", got)
		}
	}
	// flakeChance 0, blessing 4: the 8% blessing fires
	// sometimes; 300 rolls make an all-miss run negligible.
	p.Equipment().SetItem(game.AmmunitionSlot, game.NewItem(ids.RadasBlessing4, 1))
	blessings := 0
	for i := 0; i < 300; i++ {
		if got := f.rollExtraFish(p, 0); got == "blessing" {
			blessings++
		} else if got != "" {
			t.Fatalf("rollExtraFish(flake 0, blessing 4) = %q, want blessing or \"\"", got)
		}
	}
	if blessings == 0 {
		t.Fatal("rollExtraFish never returned blessing in 300 rolls at 8%")
	}
	// flakeChance 0.5, nothing worn: flakes sometimes, never
	// blessing.
	p2 := newTestPlayer("bob", 3000, 3000)
	flakes := 0
	for i := 0; i < 300; i++ {
		got := f.rollExtraFish(p2, 0.5)
		switch got {
		case "flakes":
			flakes++
		case "":
		default:
			t.Fatalf("rollExtraFish(flake 0.5, no blessing) = %q, want flakes or \"\"", got)
		}
	}
	if flakes == 0 {
		t.Fatal("rollExtraFish never returned flakes in 300 rolls at 50%")
	}
	// The blessing message is the TS line, verbatim.
	if RadaBlessingExtraFishMessage != "Rada's blessing enabled you to catch an extra fish." {
		t.Fatalf("blessing message = %q", RadaBlessingExtraFishMessage)
	}
}
