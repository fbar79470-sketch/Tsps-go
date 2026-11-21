package plugins

import (
	"strings"
	"testing"

	"tsps-go/internal/game"
)

// Combat-family tests: the registration methods, emits, gate
// contracts, and provider registration in combat.go, grounded in
// PluginManager.ts (emitNpcDeath 1322-1329, emitNpcAttackOption
// 1277-1287, emitPlayerAttack 1501-1508, emitPlayerDeath 1724-1735,
// emitPlayerDeathItemDrop 1591-1616, emitPlayerDeathDrops 1618-1625,
// emitShouldDropItemsOnDeath 1548-1569,
// emitShouldKeepItemOnDeath 1571-1589, emitPlayerDefeated 1901-1905,
// emitCombatHitPrepare/Roll/Damage/Resolved 1789-1815,
// emitCombatAttackDistance 1817-1822, registration guards
// 3472-3476, 3528-3541, 3729-3742, 3781-3828, 3885-3898, 3946-3977,
// 4028-4041, registerNpcCombatMethodProvider 4926-4965, 5241-5250) and
// PluginTypes.ts (event shapes). Fixtures only: newTestHub and
// testPlayer come from api_test.go / hooks_test.go.

func combatTestNPC() *game.NPC {
	npc := game.NewNPC(200, game.Location{X: 3090, Y: 3524, Z: 0})
	npc.Hitpoints = 10
	return npc
}

func TestCombatTypeOrder(t *testing.T) {
	// CombatType.ts enum order: MELEE, RANGED, MAGIC.
	if CombatTypeMelee != 0 || CombatTypeRanged != 1 || CombatTypeMagic != 2 {
		t.Fatalf("CombatType values = %d/%d/%d, want 0/1/2", CombatTypeMelee, CombatTypeRanged, CombatTypeMagic)
	}
}

func TestOnNpcDeathRunAllAndWrapper(t *testing.T) {
	h, _ := newTestHub()
	a := newApi(h, "DeathWatch")
	calls := 0
	a.OnNpcDeath(func(*NpcDeathEvent) { calls++ })
	a.OnNpcDeath(func(*NpcDeathEvent) { calls++ })
	killer := testPlayer("killer")
	npc := combatTestNPC()
	h.EmitNpcDeath(&NpcDeathEvent{Killer: killer, NPC: npc, NpcID: 200})
	if calls != 2 {
		t.Fatalf("calls = %d, want 2 (run-all)", calls)
	}
	// No killer: the registration wrapper skips the handler.
	h.EmitNpcDeath(&NpcDeathEvent{NPC: npc, NpcID: 200})
	if calls != 2 {
		t.Fatalf("calls after killer-less emit = %d, want 2", calls)
	}
	// No NPC: likewise skipped.
	h.EmitNpcDeath(&NpcDeathEvent{Killer: killer, NpcID: 200})
	if calls != 2 {
		t.Fatalf("calls after npc-less emit = %d, want 2", calls)
	}
	// Nil event, nil hub: no panic, no calls.
	h.EmitNpcDeath(nil)
	var nilHub *Hub
	nilHub.EmitNpcDeath(&NpcDeathEvent{Killer: killer, NPC: npc})
	if calls != 2 {
		t.Fatalf("calls after nil emits = %d, want 2", calls)
	}
	// Remains written back through the event pointer.
	a2 := newApi(h, "Remains")
	_ = a2
	h2, _ := newTestHub()
	newApi(h2, "Remains").OnNpcDeath(func(ev *NpcDeathEvent) {
		ev.Remains = &NpcDeathRemains{Ticks: 5, RespawnTicks: 50, HasRespawnTicks: true}
	})
	ev := &NpcDeathEvent{Killer: killer, NPC: npc, NpcID: 200}
	h2.EmitNpcDeath(ev)
	if ev.Remains == nil || ev.Remains.Ticks != 5 || !ev.Remains.HasRespawnTicks || ev.Remains.RespawnTicks != 50 {
		t.Fatalf("remains = %+v, want ticks 5 respawn 50", ev.Remains)
	}
}

func TestOnNpcDeathNilHandlerIgnored(t *testing.T) {
	h, _ := newTestHub()
	newApi(h, "Nil").OnNpcDeath(nil)
	h.EmitNpcDeath(&NpcDeathEvent{Killer: testPlayer("k"), NPC: combatTestNPC()})
}

func TestEmitNpcAttackOptionGate(t *testing.T) {
	h, _ := newTestHub()
	a := newApi(h, "Gate")
	second := false
	a.OnNpcAttackOption(func(ev *NpcAttackOptionEvent) { ev.Handled = true })
	a.OnNpcAttackOption(func(*NpcAttackOptionEvent) { second = true })
	p := testPlayer("attacker")
	npc := combatTestNPC()
	ev := &NpcAttackOptionEvent{Player: p, NPC: npc, NpcID: 200, ClickType: 1}
	if !h.EmitNpcAttackOption(ev) {
		t.Fatal("EmitNpcAttackOption = false, want true (first hook handled)")
	}
	if second {
		t.Fatal("second hook ran after handled (must short-circuit)")
	}
	// No hook handles: false.
	h2, _ := newTestHub()
	newApi(h2, "Watch").OnNpcAttackOption(func(*NpcAttackOptionEvent) {})
	if h2.EmitNpcAttackOption(&NpcAttackOptionEvent{Player: p, NPC: npc, NpcID: 200, ClickType: 1}) {
		t.Fatal("EmitNpcAttackOption = true, want false (unhandled)")
	}
	// Entry guards: nil event/player/NPC -> false.
	if h2.EmitNpcAttackOption(nil) || h2.EmitNpcAttackOption(&NpcAttackOptionEvent{NPC: npc}) ||
		h2.EmitNpcAttackOption(&NpcAttackOptionEvent{Player: p}) {
		t.Fatal("EmitNpcAttackOption guard failed: want false for nil event/player/npc")
	}
	// Pre-handled: TRUE with zero hooks run (the TS loop shape).
	ran := false
	h3, _ := newTestHub()
	newApi(h3, "Watch").OnNpcAttackOption(func(*NpcAttackOptionEvent) { ran = true })
	if !h3.EmitNpcAttackOption(&NpcAttackOptionEvent{Player: p, NPC: npc, Handled: true}) {
		t.Fatal("pre-handled EmitNpcAttackOption = false, want true")
	}
	if ran {
		t.Fatal("hook ran for a pre-handled event")
	}
	// Error isolation: a panicking hook is logged, the next runs.
	h4, buf := newTestHub()
	newApi(h4, "Boom").OnNpcAttackOption(func(*NpcAttackOptionEvent) { panic("boom") })
	after := false
	newApi(h4, "After").OnNpcAttackOption(func(*NpcAttackOptionEvent) { after = true })
	h4.EmitNpcAttackOption(&NpcAttackOptionEvent{Player: p, NPC: npc})
	if !after {
		t.Fatal("hook after a panicking hook did not run")
	}
	if !strings.Contains(buf.String(), "npc_attack_option hook failed (Boom)") {
		t.Fatalf("log = %q, want the npc_attack_option failure line", buf.String())
	}
	// Api member delegates; nil Api yields false.
	if !newApi(h, "Caller").EmitNpcAttackOption(&NpcAttackOptionEvent{Player: p, NPC: npc, Handled: true}) {
		t.Fatal("Api.EmitNpcAttackOption pre-handled = false, want true")
	}
	var nilApi *Api
	if nilApi.EmitNpcAttackOption(&NpcAttackOptionEvent{Player: p, NPC: npc}) {
		t.Fatal("nil Api.EmitNpcAttackOption = true, want false")
	}
}

func TestOnPlayerAttackRunAllAndWrapper(t *testing.T) {
	h, _ := newTestHub()
	calls := 0
	newApi(h, "Watch").OnPlayerAttack(func(*PlayerAttackEvent) { calls++ })
	newApi(h, "Watch2").OnPlayerAttack(func(*PlayerAttackEvent) { calls++ })
	p, target := testPlayer("a"), testPlayer("b")
	h.EmitPlayerAttack(&PlayerAttackEvent{Player: p, Target: target})
	if calls != 2 {
		t.Fatalf("calls = %d, want 2 (run-all)", calls)
	}
	// Wrapper: no target -> skipped.
	h.EmitPlayerAttack(&PlayerAttackEvent{Player: p})
	if calls != 2 {
		t.Fatalf("calls after target-less emit = %d, want 2", calls)
	}
	h.EmitPlayerAttack(nil)
	if calls != 2 {
		t.Fatalf("calls after nil emit = %d, want 2", calls)
	}
}

func TestEmitPlayerDeathGate(t *testing.T) {
	h, _ := newTestHub()
	second := false
	newApi(h, "Quest").OnPlayerDeath(func(ev *PlayerDeathEvent) { ev.Handled = true })
	newApi(h, "Late").OnPlayerDeath(func(*PlayerDeathEvent) { second = true })
	p, killer := testPlayer("victim"), testPlayer("killer")
	ev := &PlayerDeathEvent{Player: p, Killer: killer, ItemsLost: true, HasItemsLost: true}
	if !h.EmitPlayerDeath(ev) {
		t.Fatal("EmitPlayerDeath = false, want true (first hook handled)")
	}
	if second {
		t.Fatal("second hook ran after handled (must short-circuit)")
	}
	// Pre-handled returns FALSE (unlike emitNpcAttackOption).
	h2, _ := newTestHub()
	ran := false
	newApi(h2, "Watch").OnPlayerDeath(func(*PlayerDeathEvent) { ran = true })
	if h2.EmitPlayerDeath(&PlayerDeathEvent{Player: p, Handled: true}) {
		t.Fatal("pre-handled EmitPlayerDeath = true, want false")
	}
	if ran {
		t.Fatal("hook ran for a pre-handled event")
	}
	// Nil guards.
	if h2.EmitPlayerDeath(nil) || h2.EmitPlayerDeath(&PlayerDeathEvent{}) {
		t.Fatal("EmitPlayerDeath guard failed: want false for nil event/player")
	}
	// Unhandled: false, all hooks ran.
	h3, _ := newTestHub()
	calls := 0
	newApi(h3, "A").OnPlayerDeath(func(*PlayerDeathEvent) { calls++ })
	newApi(h3, "B").OnPlayerDeath(func(*PlayerDeathEvent) { calls++ })
	if h3.EmitPlayerDeath(&PlayerDeathEvent{Player: p, Killer: killer}) || calls != 2 {
		t.Fatalf("unhandled = calls %d, want 2 hooks and false", calls)
	}
}

func TestEmitPlayerDeathItemDropGate(t *testing.T) {
	h, _ := newTestHub()
	p, killer := testPlayer("victim"), testPlayer("killer")
	item := game.NewItem(995, 100)
	// Suppress + extra drops are read back even when unhandled.
	newApi(h, "Grave").OnPlayerDeathItemDrop(func(ev *PlayerDeathItemDropEvent) {
		ev.SuppressDefaultDrop = true
		ev.ExtraDrops = append(ev.ExtraDrops, game.NewItem(526, 1))
	})
	ev := &PlayerDeathItemDropEvent{Player: p, Killer: killer, Item: item, DropEligible: true, ShouldDropItems: true}
	if h.EmitPlayerDeathItemDrop(ev) {
		t.Fatal("EmitPlayerDeathItemDrop = true, want false (unhandled)")
	}
	if !ev.SuppressDefaultDrop || len(ev.ExtraDrops) != 1 || ev.ExtraDrops[0].ID() != 526 {
		t.Fatalf("read-back = suppress %v extras %+v, want suppress + 1 bones", ev.SuppressDefaultDrop, ev.ExtraDrops)
	}
	// Handled short-circuits.
	h2, _ := newTestHub()
	second := false
	newApi(h2, "Taker").OnPlayerDeathItemDrop(func(ev *PlayerDeathItemDropEvent) { ev.Handled = true })
	newApi(h2, "Late").OnPlayerDeathItemDrop(func(*PlayerDeathItemDropEvent) { second = true })
	if !h2.EmitPlayerDeathItemDrop(&PlayerDeathItemDropEvent{Player: p, Item: item}) {
		t.Fatal("EmitPlayerDeathItemDrop = false, want true (handled)")
	}
	if second {
		t.Fatal("second hook ran after handled")
	}
	// Guards: nil event/player/pre-handled -> false.
	if h2.EmitPlayerDeathItemDrop(nil) ||
		h2.EmitPlayerDeathItemDrop(&PlayerDeathItemDropEvent{Item: item}) ||
		h2.EmitPlayerDeathItemDrop(&PlayerDeathItemDropEvent{Player: p, Item: item, Handled: true}) {
		t.Fatal("EmitPlayerDeathItemDrop guard failed")
	}
}

func TestEmitPlayerDeathDropsRunAll(t *testing.T) {
	h, _ := newTestHub()
	p := testPlayer("victim")
	newApi(h, "Grave").OnPlayerDeathDrops(func(ev *PlayerDeathDropsEvent) {
		// Take the coins: remove them from Items.
		ev.Items = ev.Items[1:]
	})
	calls := 0
	newApi(h, "Watch").OnPlayerDeathDrops(func(*PlayerDeathDropsEvent) { calls++ })
	ev := &PlayerDeathDropsEvent{
		Player: p,
		Items:  []game.Item{game.NewItem(995, 100), game.NewItem(526, 1)},
		Layout: PlayerDeathLayout{Inventory: []int{995, -1}, Equipment: []int{-1}},
	}
	h.EmitPlayerDeathDrops(ev)
	if len(ev.Items) != 1 || ev.Items[0].ID() != 526 {
		t.Fatalf("Items = %+v, want only bones (grave took the coins)", ev.Items)
	}
	if calls != 1 {
		t.Fatalf("second hook calls = %d, want 1 (run-all)", calls)
	}
	// Guards: nil event/player run nothing.
	h.EmitPlayerDeathDrops(nil)
	h.EmitPlayerDeathDrops(&PlayerDeathDropsEvent{})
	if calls != 1 {
		t.Fatalf("calls after guard emits = %d, want 1", calls)
	}
	// Even an empty death emits (a death still changes a grave).
	empty := &PlayerDeathDropsEvent{Player: p}
	h.EmitPlayerDeathDrops(empty)
	if calls != 2 {
		t.Fatalf("calls after empty emit = %d, want 2", calls)
	}
}

func TestEmitShouldDropItemsOnDeathGate(t *testing.T) {
	h, _ := newTestHub()
	p, killer := testPlayer("victim"), testPlayer("killer")
	// No hooks: nil (the death defaults to dropping items).
	if got := h.EmitShouldDropItemsOnDeath(p, killer); got != nil {
		t.Fatalf("no-hook answer = %v, want nil", *got)
	}
	// First answer wins; a later deny never runs.
	second := false
	newApi(h, "Safe").OnShouldDropItemsOnDeath(func(ev *ShouldDropItemsOnDeathEvent) {
		ev.ShouldDrop = BoolPtr(false)
	})
	newApi(h, "Late").OnShouldDropItemsOnDeath(func(*ShouldDropItemsOnDeathEvent) { second = true })
	if got := h.EmitShouldDropItemsOnDeath(p, killer); got == nil || *got {
		t.Fatalf("answer = %v, want false (first hook)", got)
	}
	if second {
		t.Fatal("second hook ran after the first answer")
	}
	// Silence from every hook yields nil.
	h2, _ := newTestHub()
	newApi(h2, "Quiet").OnShouldDropItemsOnDeath(func(*ShouldDropItemsOnDeathEvent) {})
	if got := h2.EmitShouldDropItemsOnDeath(p, killer); got != nil {
		t.Fatalf("silent answer = %v, want nil", *got)
	}
	// Nil killer (environmental death) passes through to hooks.
	sawNilKiller := false
	newApi(h2, "Watch").OnShouldDropItemsOnDeath(func(ev *ShouldDropItemsOnDeathEvent) {
		sawNilKiller = ev.Killer == nil
	})
	h2.EmitShouldDropItemsOnDeath(p, nil)
	if !sawNilKiller {
		t.Fatal("nil killer must reach the hook (environmental death)")
	}
	// Guards: a player-less event runs no hook; nil hub yields nil.
	h3, _ := newTestHub()
	ran := false
	newApi(h3, "Watch").OnShouldDropItemsOnDeath(func(*ShouldDropItemsOnDeathEvent) { ran = true })
	h3.EmitShouldDropItemsOnDeath(nil, killer)
	if ran {
		t.Fatal("hook ran for a player-less event")
	}
	var nilHub *Hub
	if got := nilHub.EmitShouldDropItemsOnDeath(p, killer); got != nil {
		t.Fatalf("nil-hub answer = %v, want nil", *got)
	}
	// Nil handler ignored silently.
	newApi(h3, "Nil").OnShouldDropItemsOnDeath(nil)
	if got := h3.EmitShouldDropItemsOnDeath(p, killer); got != nil {
		t.Fatalf("answer after nil registration = %v, want nil", *got)
	}
}

func TestEmitShouldKeepItemOnDeathGate(t *testing.T) {
	h, _ := newTestHub()
	p := testPlayer("victim")
	item := game.NewItem(995, 100)
	if got := h.EmitShouldKeepItemOnDeath(p, item); got != nil {
		t.Fatalf("no-hook answer = %v, want nil", *got)
	}
	second := false
	newApi(h, "Keep").OnShouldKeepItemOnDeath(func(ev *ShouldKeepItemOnDeathEvent) {
		ev.Keep = BoolPtr(true)
	})
	newApi(h, "Late").OnShouldKeepItemOnDeath(func(*ShouldKeepItemOnDeathEvent) { second = true })
	if got := h.EmitShouldKeepItemOnDeath(p, item); got == nil || !*got {
		t.Fatalf("answer = %v, want true (first hook)", got)
	}
	if second {
		t.Fatal("second hook ran after the first answer")
	}
	// A deny is also first-answer (not a veto that keeps asking).
	h2, _ := newTestHub()
	newApi(h2, "Drop").OnShouldKeepItemOnDeath(func(ev *ShouldKeepItemOnDeathEvent) {
		ev.Keep = BoolPtr(false)
	})
	if got := h2.EmitShouldKeepItemOnDeath(p, item); got == nil || *got {
		t.Fatalf("answer = %v, want false", got)
	}
	// Guards: player-less runs nothing; nil hub yields nil.
	h3, _ := newTestHub()
	ran := false
	newApi(h3, "Watch").OnShouldKeepItemOnDeath(func(*ShouldKeepItemOnDeathEvent) { ran = true })
	h3.EmitShouldKeepItemOnDeath(nil, item)
	if ran {
		t.Fatal("hook ran for a player-less event")
	}
	var nilHub *Hub
	if got := nilHub.EmitShouldKeepItemOnDeath(p, item); got != nil {
		t.Fatalf("nil-hub answer = %v, want nil", *got)
	}
}

func TestEmitPlayerDefeatedRunAll(t *testing.T) {
	h, _ := newTestHub()
	p, killer := testPlayer("victim"), testPlayer("killer")
	calls := 0
	newApi(h, "Clean").OnPlayerDefeated(func(*PlayerDefeatedEvent) { calls++ })
	newApi(h, "Watch").OnPlayerDefeated(func(*PlayerDefeatedEvent) { calls++ })
	h.EmitPlayerDefeated(&PlayerDefeatedEvent{Killer: killer, Victim: p})
	if calls != 2 {
		t.Fatalf("calls = %d, want 2 (run-all, every death)", calls)
	}
	// Environmental death (nil killer) still fires.
	h.EmitPlayerDefeated(&PlayerDefeatedEvent{Victim: p})
	if calls != 4 {
		t.Fatalf("calls after killer-less emit = %d, want 4", calls)
	}
	// No victim: the wrapper skips the handler.
	h.EmitPlayerDefeated(&PlayerDefeatedEvent{Killer: killer})
	if calls != 4 {
		t.Fatalf("calls after victim-less emit = %d, want 4", calls)
	}
	// Nil event, nil hub: no panic, no calls.
	h.EmitPlayerDefeated(nil)
	var nilHub *Hub
	nilHub.EmitPlayerDefeated(&PlayerDefeatedEvent{Killer: killer, Victim: p})
	if calls != 4 {
		t.Fatalf("calls after nil emits = %d, want 4", calls)
	}
}

func combatMobiles(t *testing.T) (*game.Mobile, *game.Mobile) {
	t.Helper()
	a := testPlayer("attacker")
	b := testPlayer("target")
	return &a.Mobile, &b.Mobile
}

func TestCombatHitEmits(t *testing.T) {
	h, _ := newTestHub()
	attacker, target := combatMobiles(t)
	prepare, roll, damage, resolved := 0, 0, 0, 0
	newApi(h, "Watch").OnCombatHitPrepare(func(*CombatHitPrepareEvent) { prepare++ })
	newApi(h, "Watch").OnCombatHitRoll(func(ev *CombatHitRollEvent) {
		roll++
		ev.ForceMaxHit = true
		ev.BypassProtectionPrayer = true
	})
	newApi(h, "Watch").OnCombatHitDamage(func(ev *CombatHitDamageEvent) {
		damage++
		ev.Accurate = true
	})
	newApi(h, "Watch").OnCombatHitResolved(func(*CombatHitResolvedEvent) { resolved++ })

	h.EmitCombatHitPrepare(&CombatHitPrepareEvent{Attacker: attacker, Target: target, CombatType: CombatTypeMelee})
	rollEv := &CombatHitRollEvent{Attacker: attacker, Target: target, CombatType: CombatTypeRanged}
	h.EmitCombatHitRoll(rollEv)
	if !rollEv.ForceMaxHit || !rollEv.BypassProtectionPrayer {
		t.Fatal("roll read-back failed: hook mutations not visible")
	}
	dmgEv := &CombatHitDamageEvent{Attacker: attacker, Target: target, CombatType: CombatTypeMagic, Hit: "pending", Damage: "hitdamage"}
	h.EmitCombatHitDamage(dmgEv)
	if !dmgEv.Accurate {
		t.Fatal("damage read-back failed: Accurate not visible")
	}
	h.EmitCombatHitResolved(&CombatHitResolvedEvent{Attacker: attacker, Target: target, Hit: "hit"})
	if prepare != 1 || roll != 1 || damage != 1 || resolved != 1 {
		t.Fatalf("calls = %d/%d/%d/%d, want 1 each", prepare, roll, damage, resolved)
	}

	// Guards: missing attacker/target run nothing.
	h.EmitCombatHitPrepare(&CombatHitPrepareEvent{Target: target})
	h.EmitCombatHitRoll(&CombatHitRollEvent{Attacker: attacker})
	h.EmitCombatHitResolved(&CombatHitResolvedEvent{Attacker: attacker, Target: target})
	if prepare != 1 || roll != 1 || resolved != 1 {
		t.Fatalf("calls after guard emits = %d/%d/%d, want 1 each", prepare, roll, resolved)
	}
	// Damage: nil Damage runs nothing; nil Hit with Damage DOES run
	// (TS guards damage only).
	h.EmitCombatHitDamage(&CombatHitDamageEvent{Attacker: attacker, Target: target, Hit: "h"})
	if damage != 1 {
		t.Fatalf("damage calls after nil-damage emit = %d, want 1", damage)
	}
	h.EmitCombatHitDamage(&CombatHitDamageEvent{Attacker: attacker, Target: target, Damage: "d"})
	if damage != 2 {
		t.Fatalf("damage calls after nil-hit emit = %d, want 2", damage)
	}
	// Nil events / nil hub: no panic.
	h.EmitCombatHitPrepare(nil)
	h.EmitCombatHitRoll(nil)
	h.EmitCombatHitDamage(nil)
	h.EmitCombatHitResolved(nil)
	var nilHub *Hub
	nilHub.EmitCombatHitPrepare(&CombatHitPrepareEvent{Attacker: attacker, Target: target})
}

func TestCombatHitErrorIsolation(t *testing.T) {
	h, buf := newTestHub()
	attacker, target := combatMobiles(t)
	newApi(h, "Boom").OnCombatHitPrepare(func(*CombatHitPrepareEvent) { panic("boom") })
	after := false
	newApi(h, "After").OnCombatHitPrepare(func(*CombatHitPrepareEvent) { after = true })
	h.EmitCombatHitPrepare(&CombatHitPrepareEvent{Attacker: attacker, Target: target})
	if !after {
		t.Fatal("hook after a panicking hook did not run")
	}
	if !strings.Contains(buf.String(), "combat_hit_prepare hook failed (Boom)") {
		t.Fatalf("log = %q, want the combat_hit_prepare failure line", buf.String())
	}
}

func TestEmitCombatAttackDistance(t *testing.T) {
	attacker, target := combatMobiles(t)
	// No hooks: the input distance passes through.
	h, _ := newTestHub()
	if got := h.EmitCombatAttackDistance(&CombatAttackDistanceEvent{Attacker: attacker, Target: target, Distance: 7}); got != 7 {
		t.Fatalf("no-hooks distance = %d, want 7", got)
	}
	// A hook may lower or raise; the floor is 1.
	newApi(h, "Shorten").OnCombatAttackDistance(func(ev *CombatAttackDistanceEvent) { ev.Distance = 2 })
	if got := h.EmitCombatAttackDistance(&CombatAttackDistanceEvent{Attacker: attacker, Target: target, Distance: 7}); got != 2 {
		t.Fatalf("lowered distance = %d, want 2", got)
	}
	h2, _ := newTestHub()
	newApi(h2, "Zero").OnCombatAttackDistance(func(ev *CombatAttackDistanceEvent) { ev.Distance = 0 })
	if got := h2.EmitCombatAttackDistance(&CombatAttackDistanceEvent{Attacker: attacker, Target: target, Distance: 7}); got != 1 {
		t.Fatalf("zeroed distance = %d, want 1 (floor)", got)
	}
	newApi(h2, "Negative").OnCombatAttackDistance(func(ev *CombatAttackDistanceEvent) { ev.Distance = -5 })
	if got := h2.EmitCombatAttackDistance(&CombatAttackDistanceEvent{Attacker: attacker, Target: target, Distance: 7}); got != 1 {
		t.Fatalf("negative distance = %d, want 1 (floor)", got)
	}
	// Nil event / nil hub: the floor.
	if got := h.EmitCombatAttackDistance(nil); got != 1 {
		t.Fatalf("nil-event distance = %d, want 1", got)
	}
	var nilHub *Hub
	if got := nilHub.EmitCombatAttackDistance(&CombatAttackDistanceEvent{Distance: 7}); got != 1 {
		t.Fatalf("nil-hub distance = %d, want 1", got)
	}
	// ManualCast passes through untouched.
	h3, _ := newTestHub()
	seen := false
	newApi(h3, "Watch").OnCombatAttackDistance(func(ev *CombatAttackDistanceEvent) { seen = ev.ManualCast })
	h3.EmitCombatAttackDistance(&CombatAttackDistanceEvent{Attacker: attacker, Target: target, CombatType: CombatTypeMagic, ManualCast: true, Distance: 10})
	if !seen {
		t.Fatal("ManualCast not visible to the hook")
	}
}

func TestCombatRegistrationAfterLoadIgnored(t *testing.T) {
	h, buf := newTestHub()
	h.Load() // Seals the registry (no plugins registered).
	newApi(h, "Late").OnNpcDeath(func(*NpcDeathEvent) {})
	newApi(h, "Late").OnCombatHitPrepare(func(*CombatHitPrepareEvent) {})
	if !strings.Contains(buf.String(), "onNpcDeath registration ignored") {
		t.Fatalf("log = %q, want the post-load refusal for onNpcDeath", buf.String())
	}
	if !strings.Contains(buf.String(), "onCombatHitPrepare registration ignored") {
		t.Fatalf("log = %q, want the post-load refusal for onCombatHitPrepare", buf.String())
	}
}

// fakeProviderRegistry records provider registrations for the
// registerNpcCombatMethodProvider tests.
type fakeProviderRegistry struct {
	entries []npcCombatMethodProvider
}

func (r *fakeProviderRegistry) RegisterProvider(pluginName string, npcIDs []int, provide func(npc *game.NPC) any) {
	r.entries = append(r.entries, npcCombatMethodProvider{pluginName: pluginName, npcIDs: npcIDs, provide: provide})
}

func TestRegisterNpcCombatMethodProviderValidation(t *testing.T) {
	h, buf := newTestHub()
	reg := &fakeProviderRegistry{}
	h.SetNpcCombatMethodProviderRegistry(reg)
	a := newApi(h, "Boss")
	// Empty ids: warned, dropped.
	a.RegisterNpcCombatMethodProvider(nil, func(*game.NPC) any { return "m" })
	if len(reg.entries) != 0 {
		t.Fatalf("entries = %d, want 0 after empty ids", len(reg.entries))
	}
	if !strings.Contains(buf.String(), "provided invalid npc ids for combat method registration") {
		t.Fatalf("log = %q, want the invalid-ids warning", buf.String())
	}
	// Nil constructor: warned, dropped.
	a.RegisterNpcCombatMethodProvider([]int{200}, nil)
	if len(reg.entries) != 0 {
		t.Fatalf("entries = %d, want 0 after nil constructor", len(reg.entries))
	}
	if !strings.Contains(buf.String(), "provided invalid combat method constructor") {
		t.Fatalf("log = %q, want the invalid-constructor warning", buf.String())
	}
	// Too many singleton values: panic.
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected a panic for two singleton values")
		}
	}()
	a.RegisterNpcCombatMethodProvider([]int{200}, func(*game.NPC) any { return "m" }, true, false)
}

func TestRegisterNpcCombatMethodProviderQueueAndFlush(t *testing.T) {
	h, _ := newTestHub()
	a := newApi(h, "Boss")
	// Registered before installation: queued, in order.
	a.RegisterNpcCombatMethodProvider([]int{200}, func(*game.NPC) any { return "first" })
	newApi(h, "Other").RegisterNpcCombatMethodProvider([]int{300}, func(*game.NPC) any { return "second" })
	reg := &fakeProviderRegistry{}
	h.SetNpcCombatMethodProviderRegistry(reg)
	if len(reg.entries) != 2 || reg.entries[0].pluginName != "Boss" || reg.entries[1].pluginName != "Other" {
		t.Fatalf("flushed = %+v, want Boss then Other", reg.entries)
	}
	// Registered after installation: direct.
	a.RegisterNpcCombatMethodProvider([]int{400}, func(*game.NPC) any { return "third" })
	if len(reg.entries) != 3 || reg.entries[2].pluginName != "Boss" {
		t.Fatalf("entries = %d, want 3 with Boss third", len(reg.entries))
	}
	// Install(nil) keeps the queue: a later install flushes it.
	h2, _ := newTestHub()
	newApi(h2, "Queued").RegisterNpcCombatMethodProvider([]int{200}, func(*game.NPC) any { return "q" })
	h2.SetNpcCombatMethodProviderRegistry(nil)
	reg2 := &fakeProviderRegistry{}
	h2.SetNpcCombatMethodProviderRegistry(reg2)
	if len(reg2.entries) != 1 {
		t.Fatalf("entries after nil-install cycle = %d, want 1", len(reg2.entries))
	}
	// Read half.
	if h.NpcCombatMethodProviderRegistry() == nil {
		t.Fatal("NpcCombatMethodProviderRegistry() = nil after install")
	}
	if h2.NpcCombatMethodProviderRegistry() == nil {
		t.Fatal("NpcCombatMethodProviderRegistry() = nil after install (h2)")
	}
	var nilHub *Hub
	if nilHub.NpcCombatMethodProviderRegistry() != nil {
		t.Fatal("nil hub registry read = non-nil")
	}
	nilHub.SetNpcCombatMethodProviderRegistry(reg)
	// Nil Api: no-op, no panic.
	var nilApi *Api
	nilApi.RegisterNpcCombatMethodProvider([]int{200}, func(*game.NPC) any { return "m" })
}

func TestNpcCombatMethodProviderSingleton(t *testing.T) {
	h, _ := newTestHub()
	reg := &fakeProviderRegistry{}
	h.SetNpcCombatMethodProviderRegistry(reg)
	calls := 0
	var gotNPC *game.NPC
	newApi(h, "Boss").RegisterNpcCombatMethodProvider([]int{200, 201}, func(npc *game.NPC) any {
		calls++
		gotNPC = npc
		return "method"
	})
	if len(reg.entries) != 1 {
		t.Fatalf("entries = %d, want 1", len(reg.entries))
	}
	provide := reg.entries[0].provide
	npc := game.NewNPC(200, game.Location{})
	if got := provide(npc); got != "method" {
		t.Fatalf("provide = %v, want the method", got)
	}
	if gotNPC != nil {
		t.Fatal("singleton constructor received a non-nil NPC (TS calls new methodCtor() with no argument)")
	}
	if got := provide(game.NewNPC(201, game.Location{})); got != "method" || calls != 1 {
		t.Fatalf("second provide = %v calls = %d, want same instance, 1 call", got, calls)
	}
	// Non-matching id: nil, no construction.
	if got := provide(game.NewNPC(999, game.Location{})); got != nil {
		t.Fatalf("provide(999) = %v, want nil", got)
	}
	// Nil NPC: nil.
	if got := provide(nil); got != nil {
		t.Fatalf("provide(nil) = %v, want nil", got)
	}
	if calls != 1 {
		t.Fatalf("calls = %d, want 1", calls)
	}
	// Ids carried through for the factory's own check.
	if len(reg.entries[0].npcIDs) != 2 {
		t.Fatalf("npcIDs = %v, want [200 201]", reg.entries[0].npcIDs)
	}
}

func TestNpcCombatMethodProviderPerAttack(t *testing.T) {
	h, _ := newTestHub()
	reg := &fakeProviderRegistry{}
	h.SetNpcCombatMethodProviderRegistry(reg)
	calls := 0
	var seen []*game.NPC
	newApi(h, "Boss").RegisterNpcCombatMethodProvider([]int{200}, func(npc *game.NPC) any {
		calls++
		seen = append(seen, npc)
		return calls
	}, false)
	provide := reg.entries[0].provide
	npc := game.NewNPC(200, game.Location{})
	if got := provide(npc); got != 1 {
		t.Fatalf("first provide = %v, want 1", got)
	}
	if got := provide(npc); got != 2 {
		t.Fatalf("second provide = %v, want 2 (fresh per attack)", got)
	}
	if len(seen) != 2 || seen[0] != npc || seen[1] != npc {
		t.Fatal("per-attack constructor did not receive the attacking NPC each time")
	}
}
