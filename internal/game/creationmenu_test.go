package game

import (
	"testing"
)

func creationMenuIntPtr(v int) *int { return &v }

func TestCreationMenuExecuteIncludesCheck(t *testing.T) {
	var got []int
	menu := NewCreationMenu("What would you like to make?", []int{52, 53}, func(itemID, amount int) {
		got = append(got, itemID, amount)
	}, CreationMenuOptions{})
	// An offered item runs the action.
	menu.Execute(53, 5)
	if len(got) != 2 || got[0] != 53 || got[1] != 5 {
		t.Fatalf("Execute(53, 5) = %v, want [53 5]", got)
	}
	// An unoffered item does not (CreationMenu.ts:61-63).
	got = nil
	menu.Execute(54, 5)
	if len(got) != 0 {
		t.Fatalf("Execute(54, 5) = %v, want no call", got)
	}
	// A nil action never panics.
	NewCreationMenu("title", []int{52}, nil, CreationMenuOptions{}).Execute(52, 1)
	// A nil menu never panics.
	var nilMenu *CreationMenu
	nilMenu.Execute(52, 1)
	if nilMenu.Title() != "" || nilMenu.Items() != nil {
		t.Fatalf("nil menu accessors = %q %v, want empty", nilMenu.Title(), nilMenu.Items())
	}
}

func TestCreationMenuEffectiveAmounts(t *testing.T) {
	// Unset options: max 28, last opens on the maximum.
	menu := NewCreationMenu("t", []int{52}, nil, CreationMenuOptions{})
	if got := menu.EffectiveMaxAmount(); got != 28 {
		t.Errorf("unset maxAmount = %d, want 28", got)
	}
	if got := menu.EffectiveLastAmount(); got != 28 {
		t.Errorf("unset lastAmount = %d, want 28", got)
	}
	// maxAmount clamps to [0, 28] (PacketSender.ts:1405-1407).
	menu = NewCreationMenu("t", []int{52}, nil, CreationMenuOptions{MaxAmount: creationMenuIntPtr(50)})
	if got := menu.EffectiveMaxAmount(); got != 28 {
		t.Errorf("maxAmount 50 = %d, want 28", got)
	}
	menu = NewCreationMenu("t", []int{52}, nil, CreationMenuOptions{MaxAmount: creationMenuIntPtr(0)})
	if got := menu.EffectiveMaxAmount(); got != 0 {
		t.Errorf("maxAmount 0 = %d, want 0", got)
	}
	if got := menu.EffectiveLastAmount(); got != 0 {
		t.Errorf("lastAmount with max 0 = %d, want 0", got)
	}
	// lastAmount clamps to [0, maxAmount] (PacketSender.ts:1408).
	menu = NewCreationMenu("t", []int{52}, nil, CreationMenuOptions{MaxAmount: creationMenuIntPtr(5), LastAmount: creationMenuIntPtr(100)})
	if got := menu.EffectiveLastAmount(); got != 5 {
		t.Errorf("lastAmount 100 with max 5 = %d, want 5", got)
	}
	menu = NewCreationMenu("t", []int{52}, nil, CreationMenuOptions{MaxAmount: creationMenuIntPtr(5), LastAmount: creationMenuIntPtr(3)})
	if got := menu.EffectiveLastAmount(); got != 3 {
		t.Errorf("lastAmount 3 with max 5 = %d, want 3", got)
	}
	// The receive clamp reads the RAW option, not the
	// 28-clamped send value (NetworkBuilder.ts:927-931).
	menu = NewCreationMenu("t", []int{52}, nil, CreationMenuOptions{MaxAmount: creationMenuIntPtr(50)})
	if got := menu.AmountLimit(); got != 50 {
		t.Errorf("AmountLimit with maxAmount 50 = %d, want 50", got)
	}
	menu = NewCreationMenu("t", []int{52}, nil, CreationMenuOptions{})
	if got := menu.AmountLimit(); got != 28 {
		t.Errorf("AmountLimit unset = %d, want 28", got)
	}
}

func TestCreationMenuItemAt(t *testing.T) {
	menu := NewCreationMenu("t", []int{52, 53}, nil, CreationMenuOptions{})
	if id, ok := menu.ItemAt(1); !ok || id != 53 {
		t.Fatalf("ItemAt(1) = %d %v, want 53 true", id, ok)
	}
	if _, ok := menu.ItemAt(2); ok {
		t.Fatal("ItemAt(2) = true, want false (past the items)")
	}
	if _, ok := menu.ItemAt(-1); ok {
		t.Fatal("ItemAt(-1) = true, want false")
	}
	// Items returns a copy: mutating it cannot corrupt the menu.
	items := menu.Items()
	items[0] = 999
	if id, _ := menu.ItemAt(0); id != 52 {
		t.Fatalf("ItemAt(0) after mutating Items() = %d, want 52", id)
	}
}

func TestPlayerCreationMenuSlot(t *testing.T) {
	p := NewPlayer("alice", Location{X: 3089, Y: 3524, Z: 0})
	if p.GetCreationMenu() != nil {
		t.Fatal("GetCreationMenu = non-nil on a fresh player")
	}
	menu := NewCreationMenu("t", []int{52}, nil, CreationMenuOptions{})
	p.SetCreationMenu(menu)
	if p.GetCreationMenu() != menu {
		t.Fatal("GetCreationMenu != menu after SetCreationMenu")
	}
	p.SetCreationMenu(nil)
	if p.GetCreationMenu() != nil {
		t.Fatal("GetCreationMenu = non-nil after clearing")
	}
	// A nil player records nothing and reads nil.
	var nilPlayer *Player
	nilPlayer.SetCreationMenu(menu)
	if nilPlayer.GetCreationMenu() != nil {
		t.Fatal("nil player GetCreationMenu = non-nil")
	}
}

func TestPlayerCreationMenuLastItem(t *testing.T) {
	p := NewPlayer("alice", Location{X: 3089, Y: 3524, Z: 0})
	if _, ok := p.CreationMenuLastItem(); ok {
		t.Fatal("CreationMenuLastItem = true with no attribute")
	}
	p.SetAttribute(CreationMenuLastItemAttribute, 3)
	if got, ok := p.CreationMenuLastItem(); !ok || got != 3 {
		t.Fatalf("CreationMenuLastItem = %d %v, want 3 true", got, ok)
	}
	// A value reloaded from the save as a float still counts
	// when integral (Number.isInteger).
	p.SetAttribute(CreationMenuLastItemAttribute, 2.0)
	if got, ok := p.CreationMenuLastItem(); !ok || got != 2 {
		t.Fatalf("CreationMenuLastItem float 2.0 = %d %v, want 2 true", got, ok)
	}
	p.SetAttribute(CreationMenuLastItemAttribute, 2.5)
	if _, ok := p.CreationMenuLastItem(); ok {
		t.Fatal("CreationMenuLastItem float 2.5 = true, want false")
	}
	p.SetAttribute(CreationMenuLastItemAttribute, "x")
	if _, ok := p.CreationMenuLastItem(); ok {
		t.Fatal("CreationMenuLastItem string = true, want false")
	}
}

func TestStopSkillableClearsCreationMenu(t *testing.T) {
	p := NewPlayer("alice", Location{X: 3089, Y: 3524, Z: 0})
	sm := p.Skills()
	menu := NewCreationMenu("t", []int{52}, nil, CreationMenuOptions{})
	p.SetCreationMenu(menu)
	sm.StopSkillable()
	if p.GetCreationMenu() != nil {
		t.Fatal("GetCreationMenu = non-nil after StopSkillable")
	}
	// A manager with no player does nothing (no panic).
	var nilManager *SkillManager
	nilManager.StopSkillable()
}
