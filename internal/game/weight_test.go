package game

import "testing"

func TestCarriedWeightVectors(t *testing.T) {
	src := &testItemDefs{
		stackable: map[int]bool{995: true},
		noted:     map[int]bool{996: true},
		weights:   map[int]int{995: 5000, 996: 5000, 4151: 2500, 1127: 8000, 2000: -1500},
	}
	p := testPlayerWithDefs(src)
	// Empty containers weigh 0.
	if p.CarriedWeightGrams() != 0 || p.CarriedWeightKg() != 0 {
		t.Fatal("empty player weighs something")
	}
	// Stackable and noted items weigh 0 even with a cache weight.
	p.Inventory().SetItem(0, NewItem(995, 1000))
	p.Inventory().SetItem(1, NewItem(996, 1000))
	if p.CarriedWeightGrams() != 0 {
		t.Fatalf("stackable/noted weight = %d", p.CarriedWeightGrams())
	}
	// Each occupied slot counts once regardless of amount.
	p.Equipment().Set(WeaponSlot, NewItem(4151, 50))
	p.Inventory().SetItem(2, NewItem(1127, 3))
	if got := p.CarriedWeightGrams(); got != 10500 {
		t.Fatalf("weight = %d, want 10500", got)
	}
	if got := p.CarriedWeightKg(); got != 10 {
		t.Fatalf("kg = %d, want 10 (truncation)", got)
	}
	// Amount-0 and id -1 slots weigh 0.
	p.Inventory().SetItem(3, NewItem(1127, 0))
	if got := p.CarriedWeightGrams(); got != 10500 {
		t.Fatalf("zero-amount slot weighed: %d", got)
	}
	// Truncation toward zero, including negative totals.
	p2 := testPlayerWithDefs(src)
	p2.Equipment().Set(BodySlot, NewItem(2000, 1)) // -1500g
	if got := p2.CarriedWeightKg(); got != -1 {
		t.Fatalf("negative kg = %d, want -1 (Math.trunc)", got)
	}
	p2.Equipment().Set(WeaponSlot, NewItem(4151, 1)) // +2500g = 1000g
	if got := p2.CarriedWeightKg(); got != 1 {
		t.Fatalf("mixed kg = %d, want 1", got)
	}
}

func TestCarriedWeightNilSource(t *testing.T) {
	p := testPlayerWithDefs(nil)
	p.Inventory().SetItem(0, NewItem(4151, 1))
	p.Equipment().Set(WeaponSlot, NewItem(4151, 1))
	if p.CarriedWeightGrams() != 0 || p.CarriedWeightKg() != 0 {
		t.Fatal("nil-source weight is not 0 (no-cache behavior)")
	}
	// A zero-value player and a nil player weigh 0 without panicking.
	var zero Player
	if zero.CarriedWeightKg() != 0 {
		t.Fatal("zero-value player weighs something")
	}
	var nilPlayer *Player
	if nilPlayer.CarriedWeightKg() != 0 {
		t.Fatal("nil player weighs something")
	}
}

func TestGetWeightKgReadsLiveWeight(t *testing.T) {
	src := &testItemDefs{weights: map[int]int{4151: 2500}}
	p := testPlayerWithDefs(src)
	// Empty containers weigh 0; the live computation feeds the
	// drain read.
	if got := p.GetWeightKg(); got != 0 {
		t.Fatalf("GetWeightKg with empty containers = %d, want live 0", got)
	}
	p.Inventory().SetItem(0, NewItem(4151, 1))
	if got := p.GetWeightKg(); got != 2 {
		t.Fatalf("GetWeightKg after equipping/carrying = %d, want 2", got)
	}
	// Unequipping / dropping updates the read immediately.
	p.Inventory().SetItem(0, NewItem(-1, 0))
	if got := p.GetWeightKg(); got != 0 {
		t.Fatalf("GetWeightKg after clearing = %d, want 0", got)
	}
}

func TestDrainReadsLiveWeight(t *testing.T) {
	// The drain formula's weight input is the live weight: equip
	// weight raises the drain, dropping it lowers it again.
	src := &testItemDefs{weights: map[int]int{1127: 64000}}
	p := testPlayerWithDefs(src)
	// Agility defaults to 1 (fresh-character skills).
	light := RunEnergyDrainUnits(p.GetWeightKg(), p.GetCurrentAgilityLevel())
	p.Equipment().Set(BodySlot, NewItem(1127, 1)) // 64 kg: max clamp
	heavy := RunEnergyDrainUnits(p.GetWeightKg(), p.GetCurrentAgilityLevel())
	if light == heavy {
		t.Fatalf("drain unchanged by live weight: light=%d heavy=%d", light, heavy)
	}
	if heavy <= light {
		t.Fatalf("heavy drain %d <= light drain %d", heavy, light)
	}
}
