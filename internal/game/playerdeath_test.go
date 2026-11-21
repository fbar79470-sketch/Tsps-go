package game

import (
	"encoding/json"
	"testing"
)

// Player-death tests: hand-built fixtures only (no cache, no
// network, no JSON data). Item values come from
// newShopFixtureDefs: 100 iron sword (value 100, GE 150, HA
// 60), 101 feather (value 3, stackable, no GE, alch unset),
// 102 bond (value 1000, HA 0 set), 103 untradeable helm (value
// 50, not tradeable), 200 raw chicken (value 10), 201 pot
// (value 5), 300 tokkul (value 1, stackable), 400 PK sword
// (value 500), 995 coins (value 1, stackable).

// deathTestPlayer builds a registered player with the shop
// fixture definitions installed and a recording session.
func deathTestPlayer(t *testing.T, name string, x, y int) (*Player, *fakeSession) {
	t.Helper()
	p, fs := combatTestPlayer(t, name, x, y)
	p.SetItemDefinitions(newShopFixtureDefs())
	return p, fs
}

func TestKeepValue(t *testing.T) {
	defs := newShopFixtureDefs()
	// A high-alch-only item and a store-value-only item.
	defs.highAlch[500] = 80
	defs.hasHigh[500] = true
	defs.values[500] = 200
	defs.values[501] = 100
	cases := []struct {
		id   int
		want int
	}{
		{100, 150}, // GE quote wins over high alch 60.
		{500, 80},  // No GE quote: high alch.
		{102, 600}, // High alch set to 0 (a bond): 60% of 1000.
		{501, 60},  // No GE, alch unset: 60% of 100.
		{101, 1},   // floor(3 * 0.6) = 1.
		{995, 0},   // floor(1 * 0.6) = 0.
	}
	for _, tc := range cases {
		if got := KeepValue(NewItem(tc.id, 1), defs); got != tc.want {
			t.Errorf("KeepValue(%d) = %d, want %d", tc.id, got, tc.want)
		}
	}
	if got := KeepValue(NewItem(100, 1), nil); got != 0 {
		t.Errorf("KeepValue with nil source = %d, want 0", got)
	}
}

func TestAmountToKeep(t *testing.T) {
	p, _ := deathTestPlayer(t, "keeper", 3000, 3000)
	if got := AmountToKeep(p); got != 3 {
		t.Errorf("fresh AmountToKeep = %d, want 3", got)
	}
	p.SetSkullTimer(100)
	p.SetSkullType(SkullTypeWhite)
	if got := AmountToKeep(p); got != 0 {
		t.Errorf("skulled AmountToKeep = %d, want 0", got)
	}
	p.SetSkullType(SkullTypeRed)
	if got := AmountToKeep(p); got != 0 {
		t.Errorf("red skull AmountToKeep = %d, want 0", got)
	}
	p.ClearSkull()
	setPlayerLevels(p, map[int]int{SkillPrayer: 25})
	if !ActivatePrayer(p, PrayerProtectItem) {
		t.Fatal("ActivatePrayer(Protect Item) failed")
	}
	if got := AmountToKeep(p); got != 4 {
		t.Errorf("protect-item AmountToKeep = %d, want 4", got)
	}
	// Protect Item with no current Prayer is not enough.
	p.Skills().SetCurrentLevel(SkillPrayer, 0, false)
	if got := AmountToKeep(p); got != 3 {
		t.Errorf("drained-prayer AmountToKeep = %d, want 3", got)
	}
	// Skulled + Protect Item: 0 + 1.
	p.Skills().SetCurrentLevel(SkillPrayer, 25, false)
	p.SetSkullTimer(100)
	if got := AmountToKeep(p); got != 1 {
		t.Errorf("skulled protect-item AmountToKeep = %d, want 1", got)
	}
	if got := AmountToKeep(nil); got != 3 {
		t.Errorf("nil player AmountToKeep = %d, want 3", got)
	}
}

func TestProtectsCosmetics(t *testing.T) {
	p, fs := deathTestPlayer(t, "vain", 3000, 3000)
	if !ProtectsCosmetics(p, nil) {
		t.Error("environmental death should protect cosmetics")
	}
	killer, _ := deathTestPlayer(t, "killer", 3000, 3000)
	if ProtectsCosmetics(p, killer) {
		t.Error("PvP kill must not protect cosmetics")
	}
	// Self-kill (killer == player) still protects.
	if !ProtectsCosmetics(p, p) {
		t.Error("self-kill should protect cosmetics")
	}
	// Wilderness: no protection even without a killer.
	SetCombatWildernessSource(func(loc Location) (bool, bool) { return true, false })
	t.Cleanup(func() { SetCombatWildernessSource(nil) })
	if ProtectsCosmetics(p, nil) {
		t.Error("Wilderness death must not protect cosmetics")
	}
	SetCombatWildernessSource(nil)
	// Varbit 10987 = 1 (waived): no protection.
	fs.SendVarbit(DeathWaiveCosmeticsVarbit, 1)
	if ProtectsCosmetics(p, nil) {
		t.Error("waived cosmetic protection must not protect")
	}
	fs.SendVarbit(DeathWaiveCosmeticsVarbit, 0)
	if !ProtectsCosmetics(p, nil) {
		t.Error("unwaived cosmetic protection should protect")
	}
}

// deathItems stocks the player with a known spread: inventory
// 400 (500), 100 (150), 200 (6), 201 (3), 995x1000 (0);
// equipment 103 untradeable helm (30). Keep values in
// parentheses.
func deathItems(t *testing.T, p *Player) {
	t.Helper()
	p.Inventory().AddItem(NewItem(400, 1))
	p.Inventory().AddItem(NewItem(100, 1))
	p.Inventory().AddItem(NewItem(200, 1))
	p.Inventory().AddItem(NewItem(201, 1))
	p.Inventory().AddItem(NewItem(995, 1000))
	p.Equipment().Set(HeadSlot, NewItem(103, 1))
}

func itemIDs(items []Item) []int {
	ids := make([]int, len(items))
	for i, item := range items {
		ids[i] = item.ID()
	}
	return ids
}

func TestGetKeptItemsRanking(t *testing.T) {
	resetCombatHooks(t)
	p, _ := deathTestPlayer(t, "ranked", 3000, 3000)
	deathItems(t, p)
	ranked, always, cosmetic := GetKeptItems(p, 3, false)
	if got := itemIDs(ranked); len(got) != 3 || got[0] != 400 || got[1] != 100 || got[2] != 103 {
		t.Errorf("ranked = %v, want [400 100 103]", got)
	}
	if len(always) != 0 || len(cosmetic) != 0 {
		t.Errorf("always = %v, cosmetic = %v, want both empty", itemIDs(always), itemIDs(cosmetic))
	}
	// Untradeables compete for ranked slots (the helm at 30
	// outranks the chicken at 6), and lost-on-death items
	// never take one: flag the helm untradeable-meta and it
	// drops out of the ranking.
	helm := NewItem(103, 1)
	helm.SetMetaValue(ItemUntradeableMeta, json.RawMessage("true"))
	p.Equipment().Set(HeadSlot, helm)
	ranked, _, _ = GetKeptItems(p, 3, false)
	if got := itemIDs(ranked); len(got) != 3 || got[0] != 400 || got[1] != 100 || got[2] != 200 {
		t.Errorf("ranked with lost helm = %v, want [400 100 200]", got)
	}
}

func TestGetKeptItemsGates(t *testing.T) {
	resetCombatHooks(t)
	p, _ := deathTestPlayer(t, "gated", 3000, 3000)
	deathItems(t, p)
	Hooks.ShouldKeepItemOnDeath = func(player *Player, item Item) *bool {
		switch item.ID() {
		case 201:
			return boolPtr(true)
		case 400:
			return boolPtr(false)
		}
		return nil
	}
	ranked, always, _ := GetKeptItems(p, 3, false)
	if got := itemIDs(always); len(got) != 1 || got[0] != 201 {
		t.Errorf("always = %v, want [201]", got)
	}
	if got := itemIDs(ranked); len(got) != 3 || got[0] != 100 || got[1] != 103 || got[2] != 200 {
		t.Errorf("ranked = %v, want [100 103 200] (400 vetoed)", got)
	}
}

func TestGetKeptItemsCosmetic(t *testing.T) {
	resetCombatHooks(t)
	p, _ := deathTestPlayer(t, "cosmetic", 3000, 3000)
	deathItems(t, p)
	p.EmitCustomEvent = func(name string, payload any) {
		if name != DeathCosmeticEventName {
			return
		}
		if q, ok := payload.(*DeathCosmeticQuery); ok && q.Item.ID() == 200 {
			q.Cosmetic = true
		}
	}
	_, _, cosmetic := GetKeptItems(p, 3, true)
	if got := itemIDs(cosmetic); len(got) != 1 || got[0] != 200 {
		t.Errorf("cosmetic = %v, want [200]", got)
	}
	// Cosmetics off: nothing is cosmetic.
	_, _, cosmetic = GetKeptItems(p, 3, false)
	if len(cosmetic) != 0 {
		t.Errorf("cosmetic with protection off = %v, want empty", itemIDs(cosmetic))
	}
}

func TestDeathSlotLayout(t *testing.T) {
	p, _ := deathTestPlayer(t, "layout", 3000, 3000)
	p.Inventory().AddItem(NewItem(400, 1))
	p.Equipment().Set(WeaponSlot, NewItem(100, 1))
	layout := DeathSlotLayout(p)
	if len(layout.Inventory) != 28 || layout.Inventory[0] != 400 || layout.Inventory[1] != -1 {
		t.Errorf("inventory layout[0:2] = %v, want [400 -1]", layout.Inventory[:2])
	}
	if got := layout.Equipment[WeaponSlot]; got != 100 {
		t.Errorf("equipment layout[weapon] = %d, want 100", got)
	}
	if got := layout.Equipment[HeadSlot]; got != -1 {
		t.Errorf("equipment layout[head] = %d, want -1", got)
	}
}

func TestDeathLootOwner(t *testing.T) {
	p, _ := deathTestPlayer(t, "victim", 3000, 3000)
	killer, _ := deathTestPlayer(t, "killer", 3000, 3000)
	other, _ := deathTestPlayer(t, "other", 3000, 3000)
	if got := DeathLootOwner(p, nil); got != p {
		t.Error("no killer: loot owner must be the victim")
	}
	if got := DeathLootOwner(p, p); got != p {
		t.Error("self-kill: loot owner must be the victim")
	}
	if got := DeathLootOwner(p, killer); got != killer {
		t.Error("PvP kill: loot owner must be the killer")
	}
	p.EmitCustomEvent = func(name string, payload any) {
		if name != DeathLootOwnerEventName {
			return
		}
		if q, ok := payload.(*DeathLootOwnerQuery); ok {
			q.Owner = other
		}
	}
	if got := DeathLootOwner(p, killer); got != other {
		t.Error("loot-owner query must override the killer")
	}
}

func TestSplitIntoComponents(t *testing.T) {
	resetCombatHooks(t)
	p, _ := deathTestPlayer(t, "doomed", 3000, 3000)
	killer, _ := deathTestPlayer(t, "killer", 3000, 3000)
	defs := p.ItemDefinitions().(*shopFixtureDefs)
	// The PK sword splits like a Nightmare staff: staff + orb.
	defs.deathComponents = map[int][]int{400: {100, 101}}
	// A lost-on-death helm with components never splits.
	defs.deathComponents[103] = []int{100, 101}
	SetCombatWildernessSource(func(loc Location) (bool, bool) { return true, false })
	t.Cleanup(func() { SetCombatWildernessSource(nil) })

	staff := NewItem(400, 1)
	chicken := NewItem(200, 1)
	// Lost-on-death is the item meta flag, not the definition
	// tradeable flag (Item.isLostOnDeath): set it explicitly.
	helm := NewItem(103, 1)
	helm.SetMetaValue(ItemUntradeableMeta, json.RawMessage("true"))
	if got := itemIDs(splitIntoComponents(p, killer, nil, []Item{staff, chicken})); len(got) != 3 || got[0] != 100 || got[1] != 101 || got[2] != 200 {
		t.Errorf("wilderness PvP split = %v, want [100 101 200]", got)
	}
	// Each component carries the whole item's amount.
	if got := splitIntoComponents(p, killer, nil, []Item{NewItem(400, 3)}); len(got) != 2 || got[0].Amount() != 3 || got[1].Amount() != 3 {
		t.Errorf("split amounts = %v, want two stacks of 3", got)
	}
	// A kept item drops whole.
	if got := itemIDs(splitIntoComponents(p, killer, []Item{staff}, []Item{staff})); len(got) != 1 || got[0] != 400 {
		t.Errorf("kept split = %v, want [400]", got)
	}
	// A lost-on-death item drops whole.
	if got := itemIDs(splitIntoComponents(p, killer, nil, []Item{helm})); len(got) != 1 || got[0] != 103 {
		t.Errorf("lost-on-death split = %v, want [103]", got)
	}
	// No killer, a self-kill, and outside the Wilderness: whole.
	if got := itemIDs(splitIntoComponents(p, nil, nil, []Item{staff})); len(got) != 1 || got[0] != 400 {
		t.Errorf("no-killer split = %v, want [400]", got)
	}
	if got := itemIDs(splitIntoComponents(p, p, nil, []Item{staff})); len(got) != 1 || got[0] != 400 {
		t.Errorf("self-kill split = %v, want [400]", got)
	}
	SetCombatWildernessSource(func(loc Location) (bool, bool) { return false, false })
	if got := itemIDs(splitIntoComponents(p, killer, nil, []Item{staff})); len(got) != 1 || got[0] != 400 {
		t.Errorf("non-Wilderness split = %v, want [400]", got)
	}
}

func TestDropDeathItemsSplitsComponents(t *testing.T) {
	resetCombatHooks(t)
	p, _ := deathTestPlayer(t, "doomed", 3000, 3000)
	killer, _ := deathTestPlayer(t, "killer", 3000, 3000)
	defs := p.ItemDefinitions().(*shopFixtureDefs)
	defs.deathComponents = map[int][]int{400: {100, 101}}
	SetCombatWildernessSource(func(loc Location) (bool, bool) { return true, false })
	t.Cleanup(func() { SetCombatWildernessSource(nil) })
	// Skulled: nothing is kept, so the sword is free to split.
	p.SetSkullTimer(100)
	p.SetSkullType(SkullTypeWhite)
	p.Inventory().AddItem(NewItem(400, 1))
	p.Inventory().AddItem(NewItem(200, 1))
	var grounded []Item
	Hooks.DropGroundItem = func(item Item, loc Location, owner *Player) { grounded = append(grounded, item) }
	kept := DropDeathItems(p, killer, p.Location())
	if len(kept) != 0 {
		t.Errorf("kept = %v, want none (skulled)", itemIDs(kept))
	}
	if got := itemIDs(grounded); len(got) != 3 || got[0] != 100 || got[1] != 101 || got[2] != 200 {
		t.Errorf("grounded = %v, want [100 101 200] (sword split, chicken whole)", got)
	}
}

func TestDropDeathItems(t *testing.T) {
	resetCombatHooks(t)
	p, _ := deathTestPlayer(t, "doomed", 3000, 3000)
	killer, kfs := deathTestPlayer(t, "killer", 3000, 3000)
	deathItems(t, p)
	var grounded []Item
	var groundOwner *Player
	Hooks.DropGroundItem = func(item Item, loc Location, owner *Player) {
		grounded = append(grounded, item)
		groundOwner = owner
	}
	kept := DropDeathItems(p, killer, p.Location())
	if got := itemIDs(kept); len(got) != 3 || got[0] != 400 || got[1] != 100 || got[2] != 103 {
		t.Errorf("kept = %v, want [400 100 103]", got)
	}
	// The helm is untradeable but not lost-on-death: it took
	// a ranked slot, so nothing lands in lostUntradeables.
	if len(kept) != 3 {
		t.Fatalf("kept len = %d, want 3", len(kept))
	}
	if got := itemIDs(grounded); len(got) != 3 || got[0] != 200 || got[1] != 201 || got[2] != 995 {
		t.Errorf("grounded = %v, want [200 201 995]", got)
	}
	if groundOwner != killer {
		t.Error("drops must be owned by the killer")
	}
	if len(p.Inventory().GetValidItems()) != 0 || len(p.Equipment().GetValidItems()) != 0 {
		t.Error("inventory + equipment must be empty after the drop")
	}
	if len(kfs.messages) != 0 {
		t.Errorf("killer messages = %v, want none (items dropped)", kfs.messages)
	}
}

func TestDropDeathItemsNothingDropped(t *testing.T) {
	resetCombatHooks(t)
	p, _ := deathTestPlayer(t, "poor", 3000, 3000)
	killer, kfs := deathTestPlayer(t, "killer", 3000, 3000)
	p.Inventory().AddItem(NewItem(201, 1))
	var grounded []Item
	Hooks.DropGroundItem = func(item Item, loc Location, owner *Player) { grounded = append(grounded, item) }
	kept := DropDeathItems(p, killer, p.Location())
	if len(kept) != 1 || kept[0].ID() != 201 {
		t.Errorf("kept = %v, want [201]", itemIDs(kept))
	}
	if len(grounded) != 0 {
		t.Errorf("grounded = %v, want none", itemIDs(grounded))
	}
	if len(kfs.messages) != 1 || kfs.messages[0] != "poor had no valuable items to be dropped." {
		t.Errorf("killer messages = %v, want the no-valuables line", kfs.messages)
	}
}

func TestDropDeathItemsHooks(t *testing.T) {
	resetCombatHooks(t)
	p, _ := deathTestPlayer(t, "hooked", 3000, 3000)
	killer, kfs := deathTestPlayer(t, "killer", 3000, 3000)
	deathItems(t, p)
	// The per-item hook takes the chicken over (handled) and
	// suppresses the pot's default drop; the drops hook takes
	// the coins out of the floor pile (a grave).
	Hooks.OnPlayerDeathItemDrop = func(e *DeathItemDropEvent) bool {
		switch e.Item.ID() {
		case 200:
			return true
		case 201:
			e.SuppressDefaultDrop = true
		}
		return false
	}
	Hooks.OnPlayerDeathDrops = func(e *DeathDropsEvent) {
		kept := e.Items[:0]
		for _, item := range e.Items {
			if item.ID() != 995 {
				kept = append(kept, item)
			}
		}
		e.Items = kept
	}
	var grounded []Item
	Hooks.DropGroundItem = func(item Item, loc Location, owner *Player) { grounded = append(grounded, item) }
	kept := DropDeathItems(p, killer, p.Location())
	if got := itemIDs(kept); len(got) != 3 || got[0] != 400 || got[1] != 100 || got[2] != 103 {
		t.Errorf("kept = %v, want [400 100 103]", got)
	}
	if len(grounded) != 0 {
		t.Errorf("grounded = %v, want none (chicken handled, pot suppressed, coins taken)", itemIDs(grounded))
	}
	if len(kfs.messages) != 0 {
		t.Errorf("killer messages = %v, want none (a hook handled the drop)", kfs.messages)
	}
}

func TestReturnKeptItemsOverflow(t *testing.T) {
	resetCombatHooks(t)
	p, _ := deathTestPlayer(t, "full", 3000, 3000)
	// Fill all 28 slots with distinct non-stackable items.
	for i := 0; i < 28; i++ {
		p.Inventory().AddItem(NewItem(1000+i, 1))
	}
	var grounded []Item
	Hooks.DropGroundItem = func(item Item, loc Location, owner *Player) { grounded = append(grounded, item) }
	ReturnKeptItems(p, []Item{NewItem(400, 1)})
	if len(grounded) != 1 || grounded[0].ID() != 400 {
		t.Errorf("grounded = %v, want [400] (no room, never eaten)", itemIDs(grounded))
	}
	if len(p.Inventory().GetValidItems()) != 28 {
		t.Errorf("inventory size = %d, want 28", len(p.Inventory().GetValidItems()))
	}
	// With room, the kept item lands in the inventory.
	p.Inventory().ResetItems()
	ReturnKeptItems(p, []Item{NewItem(400, 1), NewItem(100, 1)})
	if got := itemIDs(p.Inventory().GetValidItems()); len(got) != 2 {
		t.Errorf("inventory after return = %v, want 2 items", got)
	}
	if len(grounded) != 1 {
		t.Errorf("grounded after roomy return = %v, want no more", itemIDs(grounded))
	}
}

func TestResetDeathAttributes(t *testing.T) {
	resetCombatHooks(t)
	p, fs := deathTestPlayer(t, "reset", 3000, 3000)
	setPlayerLevels(p, map[int]int{SkillAttack: 99, SkillHitpoints: 99, SkillPrayer: 25})
	p.Skills().SetCurrentLevel(SkillAttack, 1, false)
	p.Skills().SetCurrentLevel(SkillHitpoints, 1, false)
	ActivatePrayer(p, PrayerProtectItem)
	p.GetCombat().SetSpecialActivated(true)
	p.GetCombat().SetVengeance(true)
	p.GetCombat().SetCastSpell(&fakeSpell{})
	p.GetCombat().SetPreviousCast(&fakeSpell{})
	p.GetTimers().AddOrSet(TimerKeyFreeze, 10)
	p.SetAttribute(RecoilDamageAttribute, 50)
	p.SetSkullTimer(100)
	p.SetSkullType(SkullTypeRed)
	// Poisoned + venomed with an immunity dose in the clock
	// (Player.ts:264-265, 274-275: clock stopped + phase
	// reset, poison 0, venomed false).
	p.SetPoisonDamage(6)
	p.SetVenomed(true)
	p.SetLastVenomDamage(6)
	p.GetCombat().PoisonClock().Start(180) // counter 10
	p.SetRunEnergy(12)
	p.MovementQueue().SetBlockMovement(true)
	blockedGen := p.MovementQueue().BlockMovementGeneration()
	resetDeathAttributes(p)
	if got := p.Skills().CurrentLevel(SkillAttack); got != 99 {
		t.Errorf("attack after reset = %d, want 99", got)
	}
	if got := p.Skills().CurrentLevel(SkillHitpoints); got != 99 {
		t.Errorf("hitpoints after reset = %d, want 99", got)
	}
	if PrayerActive(&p.Mobile, PrayerProtectItem) {
		t.Error("prayers must deactivate on reset")
	}
	if p.GetCombat().IsSpecialActivated() {
		t.Error("special must deactivate on reset")
	}
	if p.GetCombat().HasVengeance() {
		t.Error("vengeance must clear on reset")
	}
	if p.GetCombat().CastSpell() != nil || p.GetCombat().PreviousCast() != nil {
		t.Error("cast spells must clear on reset")
	}
	if p.GetTimers().Has(TimerKeyFreeze) {
		t.Error("freeze timer must cancel on reset")
	}
	if p.PoisonDamage() != 0 || p.IsVenomed() {
		t.Errorf("poison after reset = damage %d venomed %v, want 0/false",
			p.PoisonDamage(), p.IsVenomed())
	}
	if clock := p.GetCombat().PoisonClock(); clock.HasCounter() || !clock.Finished() {
		t.Errorf("poison clock after reset = counter %d finished %v, want 0/true",
			clock.Counter(), clock.Finished())
	}
	if got, _ := attributeInteger(p.GetAttribute(RecoilDamageAttribute)); got != 0 {
		t.Errorf("recoil attribute = %d, want 0", got)
	}
	if p.SkullTimer() != 0 || p.SkullType() != SkullTypeWhite {
		t.Errorf("skull after reset = timer %d type %v, want 0 WHITE", p.SkullTimer(), p.SkullType())
	}
	if got := p.GetRunEnergy(); got != 100 {
		t.Errorf("run energy = %v, want 100", got)
	}
	if len(fs.runEnergy) == 0 || fs.runEnergy[len(fs.runEnergy)-1][0] != 10000 {
		t.Errorf("run-energy sends = %v, want a 10000-unit send", fs.runEnergy)
	}
	if p.MovementQueue().BlockMovementGeneration() == blockedGen {
		t.Error("movement must unblock on reset (generation must advance)")
	}
}

func TestRespawnPlayerFullFlow(t *testing.T) {
	advance := setCombatCycle(t, 100)
	resetCombatHooks(t)
	p, fs := deathTestPlayer(t, "doomed", 3000, 3000)
	setPlayerLevels(p, map[int]int{SkillHitpoints: 99})
	deathItems(t, p)
	var grounded []struct {
		item  Item
		owner *Player
	}
	Hooks.DropGroundItem = func(item Item, loc Location, owner *Player) {
		grounded = append(grounded, struct {
			item  Item
			owner *Player
		}{item, owner})
	}
	var saves []string
	Hooks.RequestSave = func(player *Player, reason string) { saves = append(saves, reason) }
	defeated := false
	Hooks.OnPlayerDefeated = func(killer, victim *Player) { defeated = true }
	var customEvents []string
	p.EmitCustomEvent = func(name string, payload any) { customEvents = append(customEvents, name) }
	var death *CombatDeathEvent
	Hooks.OnPlayerDeath = func(e *CombatDeathEvent) bool { death = e; return false }
	DecrementHealth(&p.Mobile, NewHitDamage(99, HitMaskRed))
	for cycle := int64(101); cycle <= 104; cycle++ {
		advance(cycle)
		p.GetCombat().ProcessHitQueue(cycle)
	}
	if death == nil {
		t.Fatal("player death never dispatched")
	}
	if !death.HasItemsLost || !death.ItemsLost {
		t.Errorf("death event ItemsLost = %v (has %v), want true", death.ItemsLost, death.HasItemsLost)
	}
	if p.Location() != DefaultRespawnLocation {
		t.Errorf("respawn location = %v, want %v", p.Location(), DefaultRespawnLocation)
	}
	if got := p.Skills().CurrentLevel(SkillHitpoints); got != 99 {
		t.Errorf("respawn HP = %d, want 99", got)
	}
	if len(fs.messages) == 0 || fs.messages[len(fs.messages)-1] != DefaultDeathMessage {
		t.Errorf("death messages = %v", fs.messages)
	}
	if len(fs.jingles) != 1 || fs.jingles[0] != [2]int{DeathJingleID, 0} {
		t.Errorf("jingles = %v, want [[90 0]]", fs.jingles)
	}
	// Bones (526) + the three lost items, all owned by the
	// victim (no killer).
	if len(grounded) != 4 {
		t.Fatalf("grounded = %d items, want 4 (bones + 3 lost)", len(grounded))
	}
	if grounded[0].item.ID() != DeathBonesItemID {
		t.Errorf("first grounded = %d, want bones %d", grounded[0].item.ID(), DeathBonesItemID)
	}
	for _, g := range grounded {
		if g.owner != p {
			t.Errorf("ground owner = %v, want the victim (no killer)", g.owner)
		}
	}
	// The 3 most valuable kept: 400, 100, 103 (helm, ranked).
	if got := itemIDs(p.Inventory().GetValidItems()); len(got) != 3 {
		t.Errorf("inventory after respawn = %v, want 3 kept items", got)
	}
	if !defeated {
		t.Error("emitPlayerDefeated must fire for every death")
	}
	if len(saves) != 2 || saves[0] != "death" || saves[1] != "respawn" {
		t.Errorf("saves = %v, want [death respawn]", saves)
	}
	// The reset also emits skills:current-level-changed for
	// the HP restore (and the lethal hit emitted one on the
	// way down); filter to the death events.
	var deathEvents []string
	for _, name := range customEvents {
		switch name {
		case DeathBonesEventName, PlayerDiedEventName, PlayerRespawnEventName:
			deathEvents = append(deathEvents, name)
		}
	}
	wantEvents := []string{DeathBonesEventName, PlayerDiedEventName, PlayerRespawnEventName}
	if len(deathEvents) != len(wantEvents) {
		t.Fatalf("death custom events = %v, want %v (all events %v)", deathEvents, wantEvents, customEvents)
	}
	for i, name := range wantEvents {
		if deathEvents[i] != name {
			t.Errorf("death custom event %d = %s, want %s", i, deathEvents[i], name)
		}
	}
}

func TestRespawnPlayerHandledDeath(t *testing.T) {
	advance := setCombatCycle(t, 100)
	resetCombatHooks(t)
	p, _ := deathTestPlayer(t, "quester", 3000, 3000)
	setPlayerLevels(p, map[int]int{SkillHitpoints: 99})
	var customEvents []string
	p.EmitCustomEvent = func(name string, payload any) { customEvents = append(customEvents, name) }
	Hooks.OnPlayerDeath = func(e *CombatDeathEvent) bool { return true }
	DecrementHealth(&p.Mobile, NewHitDamage(99, HitMaskRed))
	for cycle := int64(101); cycle <= 104; cycle++ {
		advance(cycle)
		p.GetCombat().ProcessHitQueue(cycle)
	}
	if p.Location().X != 3000 || p.Location().Y != 3000 {
		t.Errorf("handled death teleported to %v, want to stay at (3000, 3000)", p.Location())
	}
	for _, name := range customEvents {
		if name == PlayerRespawnEventName {
			t.Error("handled death must not emit player:respawn")
		}
	}
	if got := p.Skills().CurrentLevel(SkillHitpoints); got != 99 {
		t.Errorf("handled-death HP = %d, want 99 (revival still runs)", got)
	}
}

func TestRespawnPlayerNoItemLoss(t *testing.T) {
	advance := setCombatCycle(t, 100)
	resetCombatHooks(t)
	p, _ := deathTestPlayer(t, "safe", 3000, 3000)
	setPlayerLevels(p, map[int]int{SkillHitpoints: 99})
	p.Inventory().AddItem(NewItem(400, 1))
	p.Inventory().AddItem(NewItem(100, 1))
	Hooks.ShouldDropItemsOnDeath = func(player, killer *Player) *bool { return boolPtr(false) }
	var grounded []Item
	Hooks.DropGroundItem = func(item Item, loc Location, owner *Player) { grounded = append(grounded, item) }
	DecrementHealth(&p.Mobile, NewHitDamage(99, HitMaskRed))
	for cycle := int64(101); cycle <= 104; cycle++ {
		advance(cycle)
		p.GetCombat().ProcessHitQueue(cycle)
	}
	// Only the bones ground: the items stay.
	if len(grounded) != 1 || grounded[0].ID() != DeathBonesItemID {
		t.Errorf("grounded = %v, want bones only", itemIDs(grounded))
	}
	if got := itemIDs(p.Inventory().GetValidItems()); len(got) != 2 {
		t.Errorf("inventory after no-loss death = %v, want both items kept", got)
	}
	if p.Location() != DefaultRespawnLocation {
		t.Errorf("location = %v, want home %v", p.Location(), DefaultRespawnLocation)
	}
}

func TestRespawnPlayerBonesSuppressed(t *testing.T) {
	advance := setCombatCycle(t, 100)
	resetCombatHooks(t)
	p, _ := deathTestPlayer(t, "boneless", 3000, 3000)
	setPlayerLevels(p, map[int]int{SkillHitpoints: 99})
	p.EmitCustomEvent = func(name string, payload any) {
		if name == DeathBonesEventName {
			if ev, ok := payload.(*DeathBonesEvent); ok {
				ev.Allow = false
			}
		}
	}
	var grounded []Item
	Hooks.DropGroundItem = func(item Item, loc Location, owner *Player) { grounded = append(grounded, item) }
	DecrementHealth(&p.Mobile, NewHitDamage(99, HitMaskRed))
	for cycle := int64(101); cycle <= 104; cycle++ {
		advance(cycle)
		p.GetCombat().ProcessHitQueue(cycle)
	}
	if len(grounded) != 0 {
		t.Errorf("grounded = %v, want none (bones suppressed)", itemIDs(grounded))
	}
}

func TestLoseItemsAsDeath(t *testing.T) {
	resetCombatHooks(t)
	p, _ := deathTestPlayer(t, "raider", 3000, 3000)
	deathItems(t, p)
	var saves []string
	Hooks.RequestSave = func(player *Player, reason string) { saves = append(saves, reason) }
	var grounded []Item
	Hooks.DropGroundItem = func(item Item, loc Location, owner *Player) { grounded = append(grounded, item) }
	LoseItemsAsDeath(p, p.Location())
	if len(grounded) != 3 {
		t.Errorf("grounded = %v, want the 3 lost items", itemIDs(grounded))
	}
	if got := itemIDs(p.Inventory().GetValidItems()); len(got) != 3 {
		t.Errorf("inventory after lose-items-as-death = %v, want 3 kept", got)
	}
	if len(saves) != 1 || saves[0] != "death" {
		t.Errorf("saves = %v, want [death]", saves)
	}
}

// boolPtr is defined in movement_queue_test.go; the death
// tests reuse it.
